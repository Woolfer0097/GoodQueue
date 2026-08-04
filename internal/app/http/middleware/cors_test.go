package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsFrontendPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/products", nil)
	request.Header.Set("Origin", frontendOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != frontendOrigin {
		t.Fatalf("allow origin = %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
	if recorder.Header().Get("Access-Control-Allow-Methods") != "GET, POST, DELETE, OPTIONS" {
		t.Fatalf("allow methods = %q", recorder.Header().Get("Access-Control-Allow-Methods"))
	}
	if recorder.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-User-ID, X-Request-ID" {
		t.Fatalf("allow headers = %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestCORSRejectsUnknownPreflightOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/products", nil)
	request.Header.Set("Origin", "https://example.invalid")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected allow origin %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSLeavesRequestsWithoutOriginAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected allow origin %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
}
