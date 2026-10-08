package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCronAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, secret, query string
		headers             []string
		allowed             bool
	}{
		{name: "valid", secret: "test-secret", headers: []string{"Bearer test-secret"}, allowed: true},
		{name: "missing header", secret: "test-secret"},
		{name: "empty header", secret: "test-secret", headers: []string{""}},
		{name: "wrong secret", secret: "test-secret", headers: []string{"Bearer wrong"}},
		{name: "missing bearer", secret: "test-secret", headers: []string{"test-secret"}},
		{name: "wrong scheme", secret: "test-secret", headers: []string{"Basic test-secret"}},
		{name: "lowercase scheme", secret: "test-secret", headers: []string{"bearer test-secret"}},
		{name: "empty bearer", secret: "test-secret", headers: []string{"Bearer "}},
		{name: "extra space", secret: "test-secret", headers: []string{"Bearer  test-secret"}},
		{name: "trailing space", secret: "test-secret", headers: []string{"Bearer test-secret "}},
		{name: "duplicate headers", secret: "test-secret", headers: []string{"Bearer test-secret", "Bearer test-secret"}},
		{name: "combined headers", secret: "test-secret", headers: []string{"Bearer test-secret, Bearer test-secret"}},
		{name: "query token", secret: "test-secret", query: "?token=test-secret"},
		{name: "empty configured secret", headers: []string{"Bearer "}},
		{name: "blank configured secret", secret: " \t", headers: []string{"Bearer  \t"}},
		{name: "malformed configured secret", secret: "test secret", headers: []string{"Bearer test secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			called := false
			r.GET("/cron", CronAuth(tc.secret), func(c *gin.Context) {
				called = true
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/cron"+tc.query, nil)
			for _, header := range tc.headers {
				req.Header.Add("Authorization", header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, tc.allowed, called)
			if tc.allowed {
				require.Equal(t, http.StatusNoContent, w.Code)
			} else {
				require.Equal(t, http.StatusUnauthorized, w.Code)
				require.NotContains(t, w.Body.String(), "test-secret")
			}
		})
	}
}
