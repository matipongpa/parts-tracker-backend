package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func corsRouter(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCORS(origins))
	router.POST("/api/auth/refresh", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func preflight(router *gin.Engine, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/refresh", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCORSAllowsCredentialsForConfiguredOriginOnly(t *testing.T) {
	router := corsRouter([]string{"https://ui.example.com"})

	rec := preflight(router, "https://ui.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.com" {
		t.Errorf("Allow-Origin = %q, want the configured origin echoed (never *)", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true so the browser sends the refresh cookie", got)
	}

	rec = preflight(router, "https://evil.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin for an unknown origin = %q, want empty", got)
	}
}
