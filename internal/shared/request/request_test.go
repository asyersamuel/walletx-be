package request

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ParseUserID must never panic regardless of what upstream middleware placed
// under "user_id": a missing key or a non-string value yields a clean 401, a
// malformed UUID string yields 400, and only a valid UUID string is accepted.

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	return c, w
}

func requireEnvelope(t *testing.T, w *httptest.ResponseRecorder) (string, string, int) {
	t.Helper()
	var body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Status, body.Message, w.Code
}

func TestParseUserID(t *testing.T) {
	t.Run("missing user_id context key returns 401", func(t *testing.T) {
		c, w := newTestContext()

		userID, ok := ParseUserID(c)

		assert.False(t, ok)
		assert.Equal(t, uuid.Nil, userID)
		status, message, code := requireEnvelope(t, w)
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.Equal(t, "fail", status)
		assert.Equal(t, "User not authenticated", message)
	})

	t.Run("non-string user_id returns 401 instead of panicking", func(t *testing.T) {
		c, w := newTestContext()
		c.Set("user_id", uuid.New()) // uuid.UUID, not string

		var userID uuid.UUID
		var ok bool
		require.NotPanics(t, func() {
			userID, ok = ParseUserID(c)
		})

		assert.False(t, ok)
		assert.Equal(t, uuid.Nil, userID)
		status, message, code := requireEnvelope(t, w)
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.Equal(t, "fail", status)
		assert.Equal(t, "User not authenticated", message)
	})

	t.Run("valid uuid string is parsed and accepted", func(t *testing.T) {
		c, w := newTestContext()
		expected := uuid.New()
		c.Set("user_id", expected.String())

		userID, ok := ParseUserID(c)

		require.True(t, ok)
		assert.Equal(t, expected, userID)
		assert.Equal(t, http.StatusOK, w.Code, "no error response must be written on success")
		assert.Empty(t, w.Body.String())
	})

	t.Run("malformed uuid string returns 400", func(t *testing.T) {
		c, w := newTestContext()
		c.Set("user_id", "not-a-uuid")

		userID, ok := ParseUserID(c)

		assert.False(t, ok)
		assert.Equal(t, uuid.Nil, userID)
		status, message, code := requireEnvelope(t, w)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "fail", status)
		assert.Equal(t, "Invalid user ID format", message)
	})
}
