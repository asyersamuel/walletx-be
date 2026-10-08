package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"walletx-be/internal/app"

	"github.com/stretchr/testify/require"
)

func TestHealthDoesNotInitializeApplication(t *testing.T) {
	// Deliberately unusable dependencies must not affect health requests.
	t.Setenv("DATABASE_URL", "invalid-database-url")
	t.Setenv("GEMINI_API_KEY", "")
	previousHandler, previousErr := httpHandler, initializationErr
	t.Cleanup(func() {
		httpHandler, initializationErr = previousHandler, previousErr
		appOnce = sync.Once{}
	})
	for _, failed := range []bool{false, true} {
		appOnce = sync.Once{}
		httpHandler = nil
		initializationErr = nil
		if failed {
			initializationErr = errors.New("application initialization failed")
		}
		w := httptest.NewRecorder()
		Handler(w, httptest.NewRequest(http.MethodGet, app.HealthPath, nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, `{"status":"ok"}`, w.Body.String())
		// If Handler consumed appOnce, this callback would never run.
		unused := false
		appOnce.Do(func() { unused = true })
		require.True(t, unused, "health consumed the application initialization guard")
		require.Nil(t, httpHandler)
	}
}
