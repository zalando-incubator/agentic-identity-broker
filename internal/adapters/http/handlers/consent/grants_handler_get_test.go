package consent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

type getGrantResponse struct {
	Data []GrantResponse `json:"data"`
}

func TestGetGrant_Success(t *testing.T) {
	t.Parallel()

	principalValue := "user@example.com"
	agentID := id.NewAgentID()
	grantID := id.NewGrantID()
	serviceID := id.NewServiceID()
	validUntil := time.Now().Add(24 * time.Hour)

	mockService := &mockConsentService{
		getUserGrantsFunc: func(ctx context.Context, p id.Principal, agID id.AgentID) ([]*storage.UserGrant, error) {
			if p != id.Principal(principalValue) {
				t.Errorf("expected principal %s, got %s", principalValue, p)
			}
			if agID != agentID {
				t.Errorf("expected agentID %s, got %s", agentID, agID)
			}

			return []*storage.UserGrant{{
				ID:                    grantID,
				Principal:             id.Principal(principalValue),
				AgentID:               agentID,
				ValidUntil:            &validUntil,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{serviceID}}},
				CreatedAt:             time.Now().Add(-48 * time.Hour),
				UpdatedAt:             time.Now().Add(-1 * time.Hour),
			}}, nil
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String()+"/grants", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response getGrantResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected one grant to be returned, got %d", len(response.Data))
	}
	grant := response.Data[0]
	if grant.ID != grantID.String() {
		t.Errorf("expected grant ID %q, got %s", grantID.String(), grant.ID)
	}
	if grant.Principal != principalValue {
		t.Errorf("expected principal %s, got %s", principalValue, grant.Principal)
	}
	if grant.ValidUntil == nil {
		t.Error("expected ValidUntil to be set")
	}
	if len(grant.GrantedPermissionSets) != 1 {
		t.Errorf("expected 1 permission set, got %d", len(grant.GrantedPermissionSets))
	}
}

func TestGetGrant_EmptyGrants(t *testing.T) {
	t.Parallel()

	principalValue := "user@example.com"
	agentID := id.NewAgentID()
	mockService := &mockConsentService{
		getUserGrantsFunc: func(ctx context.Context, p id.Principal, agID id.AgentID) ([]*storage.UserGrant, error) {
			return nil, nil
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String()+"/grants", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response getGrantResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Data == nil || len(response.Data) != 0 {
		t.Errorf("expected a non-nil empty grants array, got %v", response.Data)
	}
}

func TestGetGrant_MissingPrincipal(t *testing.T) {
	t.Parallel()

	handler := NewGrantsHandler(&mockConsentService{}, nil, newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/agent-123/grants", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", "agent-123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	var response ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if response.Error != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %s", response.Error)
	}
}

func TestGetGrant_MissingAgentID(t *testing.T) {
	t.Parallel()

	handler := NewGrantsHandler(&mockConsentService{}, nil, newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent//grants", nil)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	rctx := chi.NewRouteContext()
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var response ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if response.Error != "bad request" {
		t.Errorf("expected error 'bad request', got %s", response.Error)
	}
}

func TestGetGrant_AgentNotFound(t *testing.T) {
	t.Parallel()

	principalValue := "user@example.com"
	agentID := id.NewAgentID()
	mockService := &mockConsentService{
		getUserGrantsFunc: func(ctx context.Context, p id.Principal, agID id.AgentID) ([]*storage.UserGrant, error) {
			return nil, consent.ErrAgentNotFound
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String()+"/grants", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}

	var response ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if response.Error != "not found" {
		t.Errorf("expected error 'not found', got %s", response.Error)
	}
}

func TestGetGrant_ServiceError(t *testing.T) {
	t.Parallel()

	principalValue := "user@example.com"
	agentID := id.NewAgentID()
	mockService := &mockConsentService{
		getUserGrantsFunc: func(ctx context.Context, p id.Principal, agID id.AgentID) ([]*storage.UserGrant, error) {
			return nil, errors.New("database connection failed")
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String()+"/grants", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	var response ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if response.Error != "internal server error" {
		t.Errorf("expected error 'internal server error', got %s", response.Error)
	}
}
