package consent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/go-chi/chi/v5"
)

func TestRevokeGrant_Success(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	revokeCalled := false
	mockService := &mockConsentService{
		revokeConsentForPrincipalFunc: func(ctx context.Context, p id.Principal, agentID id.AgentID) error {
			revokeCalled = true
			if p != id.Principal("user@example.com") {
				t.Errorf("expected principal 'user@example.com', got '%s'", p)
			}
			if agentID != testAgentID {
				t.Errorf("expected agentID '%s', got '%s'", testAgentID, agentID)
			}
			return nil
		},
	}
	handler := NewGrantsHandler(mockService, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())

	req := newRequestWithPrincipal(http.MethodDelete, "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if !revokeCalled {
		t.Error("expected RevokeConsentForPrincipal to be called")
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestRevokeGrant_NoPrincipal(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	handler := NewGrantsHandler(nil, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodDelete, "/api/consent/agent/"+testAgentID.String()+"/grants", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got '%s'", errResp.Error)
	}
}

func TestRevokeGrant_NoPrincipal_InvalidUUID(t *testing.T) {
	t.Parallel()

	handler := NewGrantsHandler(nil, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodDelete, "/api/consent/agent/not-a-uuid/grants", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d (principal checked first), got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestRevokeGrant_InvalidAgentID(t *testing.T) {
	t.Parallel()

	handler := NewGrantsHandler(nil, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())
	req := newRequestWithPrincipal(http.MethodDelete, "/api/consent/agent/not-a-uuid/grants", "user@example.com", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestRevokeGrant_NotFound(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	mockService := &mockConsentService{
		revokeConsentForPrincipalFunc: func(ctx context.Context, p id.Principal, agentID id.AgentID) error {
			return fmt.Errorf("wrapped: %w", consent.ErrGrantNotFound)
		},
	}
	handler := NewGrantsHandler(mockService, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())

	req := newRequestWithPrincipal(http.MethodDelete, "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error != "not found" {
		t.Errorf("expected error 'not found', got '%s'", errResp.Error)
	}
}

func TestRevokeGrant_ServiceError(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	mockService := &mockConsentService{
		revokeConsentForPrincipalFunc: func(ctx context.Context, p id.Principal, agentID id.AgentID) error {
			return errors.New("database connection lost")
		},
	}
	handler := NewGrantsHandler(mockService, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())

	req := newRequestWithPrincipal(http.MethodDelete, "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestRevokeGrant_PrincipalCheckedFirst(t *testing.T) {
	t.Parallel()

	handler := NewGrantsHandler(nil, slog.New(slog.DiscardHandler), newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodDelete, "/api/consent/agent/bad-uuid/grants", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", "bad-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.RevokeGrant(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized (principal checked before UUID), got %d", rr.Code)
	}
}
