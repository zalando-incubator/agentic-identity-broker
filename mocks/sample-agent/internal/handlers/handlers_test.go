package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/sample-agent/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/oauth2"
)

// TestCallMCPNoSession tests that CallMCP returns 401 when no session exists
func TestCallMCPNoSession(t *testing.T) {
	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: "http://localhost:4000/mcp",
		},
	}
	h := New(&oauth2.Config{}, cfg)

	// No session cookie
	req := httptest.NewRequest(http.MethodPost, "/call-mcp", nil)
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %s", resp["error"])
	}
}

// TestCallMCPInvalidSession tests that CallMCP returns 401 for invalid session ID
func TestCallMCPInvalidSession(t *testing.T) {
	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: "http://localhost:4000/mcp",
		},
	}
	h := New(&oauth2.Config{}, cfg)

	req := httptest.NewRequest(http.MethodPost, "/call-mcp", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: "unknown-session-id",
	})
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestCallMCPExpiredToken tests that CallMCP returns 401 for expired token
func TestCallMCPExpiredToken(t *testing.T) {
	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: "http://localhost:4000/mcp",
		},
	}
	h := New(&oauth2.Config{}, cfg)

	// Create session with expired token
	sessionID := "test-session"
	expiredToken := &oauth2.Token{
		AccessToken: "expired-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(-1 * time.Hour), // expired 1 hour ago
	}

	h.sessionsMu.Lock()
	h.sessions[sessionID] = &Session{
		Token:     expiredToken,
		UserInfo:  &UserInfo{Sub: "test-user"},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
	}
	h.sessionsMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/call-mcp", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

// TestCallMCPNoGatewayConfig tests that CallMCP returns 500 when gateway not configured
func TestCallMCPNoGatewayConfig(t *testing.T) {
	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: "", // not configured
		},
	}
	h := New(&oauth2.Config{}, cfg)

	// Create valid session with non-expired token
	sessionID := "test-session"
	validToken := &oauth2.Token{
		AccessToken: "valid-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}

	h.sessionsMu.Lock()
	h.sessions[sessionID] = &Session{
		Token:     validToken,
		UserInfo:  &UserInfo{Sub: "test-user"},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	}
	h.sessionsMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/call-mcp", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for no gateway config, got %d", w.Code)
	}
}

// TestCallMCPUnreachableGateway tests that CallMCP returns 502 when gateway is unreachable
func TestCallMCPUnreachableGateway(t *testing.T) {
	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: "http://localhost:19999/mcp", // port that should be unreachable
		},
	}
	h := New(&oauth2.Config{}, cfg)

	// Create valid session with non-expired token
	sessionID := "test-session"
	validToken := &oauth2.Token{
		AccessToken: "valid-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour), // ensure token doesn't expire during test
	}

	h.sessionsMu.Lock()
	h.sessions[sessionID] = &Session{
		Token:     validToken,
		UserInfo:  &UserInfo{Sub: "test-user"},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	}
	h.sessionsMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/call-mcp", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for unreachable gateway, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "mcp_call_failed" {
		t.Errorf("expected error=mcp_call_failed, got %s", resp["error"])
	}
}

// TestHomePageWithoutSession tests that home page shows login button when not authenticated
func TestHomePageWithoutSession(t *testing.T) {
	cfg := &config.Config{}
	h := New(&oauth2.Config{}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.Home(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected HTML content, got empty body")
	}

	// Should show login page content, not call MCP button
	if body == "" {
		t.Error("expected login page content")
	}
}

// TestHomePageWithValidSession tests that home page shows MCP button when authenticated
func TestHomePageWithValidSession(t *testing.T) {
	cfg := &config.Config{}
	h := New(&oauth2.Config{}, cfg)

	// Create valid session
	sessionID := "test-session"
	validToken := &oauth2.Token{
		AccessToken: "valid-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}

	h.sessionsMu.Lock()
	h.sessions[sessionID] = &Session{
		Token:     validToken,
		UserInfo:  &UserInfo{Sub: "test-user"},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	}
	h.sessionsMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})
	w := httptest.NewRecorder()

	h.Home(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `id="mcp-approval-btn"`) {
		t.Error("expected user info page to include the approval-required MCP button")
	}
	if !strings.Contains(body, "Call MCP Tool: create_issue (approval required)") {
		t.Error("expected approval-required MCP button label")
	}
}

func TestApprovalElicitationURL(t *testing.T) {
	url, ok := approvalElicitationURL(mcp.URLElicitationRequiredError{
		Elicitations: []mcp.ElicitationParams{{URL: "https://broker.example/approvals/123"}},
	})
	if !ok || url != "https://broker.example/approvals/123" {
		t.Fatalf("approvalElicitationURL() = (%q, %t), want approval URL", url, ok)
	}

	if _, ok := approvalElicitationURL(mcp.URLElicitationRequiredError{}); ok {
		t.Fatal("approvalElicitationURL() accepted an elicitation without a URL")
	}
}

func TestWriteApprovalRequired(t *testing.T) {
	w := httptest.NewRecorder()
	writeApprovalRequired(w, "https://broker.example/approvals/123", "http://agentgateway:4000/mcp")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var response struct {
		Error       string `json:"error"`
		ApprovalURL string `json:"approval_url"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != "approval_required" {
		t.Fatalf("error = %q, want approval_required", response.Error)
	}
	if response.ApprovalURL != "https://broker.example/approvals/123" {
		t.Fatalf("approval_url = %q, want broker approval URL", response.ApprovalURL)
	}
}

// TestCallMCPWrongMethod tests that CallMCP rejects non-POST requests
func TestCallMCPWrongMethod(t *testing.T) {
	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: "http://localhost:4000/mcp",
		},
	}
	h := New(&oauth2.Config{}, cfg)

	// Test GET request
	req := httptest.NewRequest(http.MethodGet, "/call-mcp", nil)
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET request, got %d", w.Code)
	}
}

// TestCallMCPUnmarshalError tests graceful handling of response parse errors
func TestCallMCPResponseParseError(t *testing.T) {
	// Create a test server that returns invalid JSON
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		AgentGateway: config.AgentGatewayConfig{
			MCPURL: mockServer.URL + "/mcp",
		},
	}
	h := New(&oauth2.Config{}, cfg)

	// Create valid session
	sessionID := "test-session"
	validToken := &oauth2.Token{
		AccessToken: "valid-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}

	h.sessionsMu.Lock()
	h.sessions[sessionID] = &Session{
		Token:     validToken,
		UserInfo:  &UserInfo{Sub: "test-user"},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	}
	h.sessionsMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/call-mcp", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	})
	w := httptest.NewRecorder()

	h.CallMCP(w, req)

	// Should return 502 on parse error
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for response parse error, got %d", w.Code)
	}
}

func TestToolArguments(t *testing.T) {
	arguments := toolArguments("create_issue")
	if arguments["repository"] != "acme/sample-agent" || arguments["title"] != "Review Compose approval" {
		t.Fatalf("create_issue arguments = %#v", arguments)
	}
	if arguments := toolArguments("whoami"); arguments != nil {
		t.Fatalf("whoami arguments = %#v, want nil", arguments)
	}
}
