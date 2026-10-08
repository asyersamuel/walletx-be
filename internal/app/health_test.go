package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHealthRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewHealthRouter(routerLogger{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, HealthPath, nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, `{"status":"ok"}`, w.Body.String())
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, HealthPath, nil))
	require.NotEqual(t, http.StatusOK, w.Code)
}
