package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestCORSMiddleware_RegularRequest(t *testing.T) {
	cfg := ports.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}}
	handler := CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify CORS origin header is set
	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:3000', got %q", origin)
	}
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	cfg := ports.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}}
	handler := CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should not reach here"))
	}))

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Preflight should return a successful 2xx status (typically 200 or 204)
	if rr.Code < 200 || rr.Code >= 300 {
		t.Errorf("expected 2xx status for preflight, got %d", rr.Code)
	}

	// Verify CORS headers are set on preflight
	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:3000', got %q", origin)
	}

	methods := rr.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("expected Access-Control-Allow-Methods to be set")
	}

	maxAge := rr.Header().Get("Access-Control-Max-Age")
	if maxAge != "86400" {
		t.Errorf("expected Access-Control-Max-Age '86400', got %q", maxAge)
	}
}

func TestCORSMiddleware_AllowedHeaders(t *testing.T) {
	cfg := ports.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}}
	handler := CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// X-Custom-Principal should be allowed
	headers := rr.Header().Get("Access-Control-Allow-Headers")
	if headers == "" {
		t.Error("expected Access-Control-Allow-Headers to be set")
	}
}

func TestCORSMiddleware_RequestWithoutOrigin(t *testing.T) {
	cfg := ports.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}}
	nextCalled := false
	handler := CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	// No Origin header — same-origin request

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !nextCalled {
		t.Error("next handler should be called for same-origin requests")
	}
}

func TestCORSMiddleware_DisabledWhenNoOrigins(t *testing.T) {
	// Production default: empty AllowedOrigins = no CORS headers
	cfg := ports.CORSConfig{}
	nextCalled := false
	handler := CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://evil.example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !nextCalled {
		t.Error("next handler should be called")
	}
	// No CORS headers should be added
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no Access-Control-Allow-Origin header when CORS is disabled")
	}
}

func TestCORSMiddleware_CustomMethodsAndHeaders(t *testing.T) {
	cfg := ports.CORSConfig{
		AllowedOrigins: []string{"http://app.example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type"},
		MaxAge:         3600,
	}
	handler := CORSMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d for preflight, got %d", http.StatusOK, rr.Code)
	}

	maxAge := rr.Header().Get("Access-Control-Max-Age")
	if maxAge != "3600" {
		t.Errorf("expected Access-Control-Max-Age '3600', got %q", maxAge)
	}
}
