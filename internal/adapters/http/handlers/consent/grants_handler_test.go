package consent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestJWETokenService returns a real JWE token service backed by a deterministic test key.
func newTestJWETokenService() *domjwe.TokenService {
	keyBytes, err := base64.StdEncoding.DecodeString("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=")
	if err != nil {
		panic("grants_handler_test: failed to decode test JWE key: " + err.Error())
	}
	jweKey, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		panic("grants_handler_test: failed to import test JWE key: " + err.Error())
	}
	return domjwe.New(jweKey)
}

// newTestSessionTokenValidator returns a SessionTokenValidator backed by a test JWE key.
func newTestSessionTokenValidator() ports.SessionTokenValidator {
	return sessiontoken.NewService(newTestJWETokenService())
}

// newTestSessionToken creates a valid JWE session token for the given agent and principal.
func newTestSessionToken(ts *domjwe.TokenService, agentID id.AgentID, principalVal string, originalURL string) string {
	claims, err := sessiontoken.NewAuthorizationSessionClaims(agentID, id.Principal(principalVal), originalURL, nil)
	if err != nil {
		panic("newTestSessionToken: invalid claims: " + err.Error())
	}
	token, err := ts.Encrypt(claims)
	if err != nil {
		panic("newTestSessionToken: failed to encrypt claims: " + err.Error())
	}
	return token
}

// newExpiredTestSessionToken creates a JWE session token whose TTL has already elapsed.
func newExpiredTestSessionToken(ts *domjwe.TokenService, agentID id.AgentID, principalVal string) string {
	past := time.Now().Add(-time.Hour)
	claims := &sessiontoken.AuthorizationSessionClaims{
		AgentID:     agentID,
		Principal:   id.Principal(principalVal),
		OriginalURL: "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
		IssuedAt:    past,
		ExpiresAt:   past,
	}
	token, err := ts.Encrypt(claims)
	if err != nil {
		panic("newExpiredTestSessionToken: failed to encrypt claims: " + err.Error())
	}
	return token
}

// Helper to create request with principal context
func newRequestWithPrincipal(method, path, principalValue string, body any) *http.Request {
	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer([]byte{})
	}

	req := httptest.NewRequest(method, path, reqBody)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	return req.WithContext(ctx)
}

func TestCreateGrant_NoPrincipal(t *testing.T) {
	t.Parallel()
	handler := NewGrantsHandler(nil, nil, newTestSessionTokenValidator())
	testAgentID := id.NewAgentID()

	reqBody := GrantRequest{
		GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {id.NewServiceID().String()}},
	}

	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", bytes.NewBuffer(jsonBody))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()

	handler.CreateGrant(rr, req)

	// Verify response
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestCreateGrant_InvalidJSON(t *testing.T) {
	t.Parallel()
	handler := NewGrantsHandler(nil, nil, newTestSessionTokenValidator())
	testAgentID := id.NewAgentID()

	req := newRequestWithPrincipal("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", nil)
	req.Body = httptest.NewRequest("POST", "/", bytes.NewBuffer([]byte("invalid json"))).Body

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()

	handler.CreateGrant(rr, req)

	// Verify response
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if errResp.Error != "invalid request" {
		t.Errorf("expected error 'invalid request', got '%s'", errResp.Error)
	}
}

func TestToGrantResponse(t *testing.T) {
	t.Parallel()
	handler := NewGrantsHandler(nil, nil, newTestSessionTokenValidator())

	testGrantID := id.NewGrantID()
	testAgentID := id.NewAgentID()
	testPSID1 := id.NewPermissionSetID()
	testPSID2 := id.NewPermissionSetID()

	validUntil := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	grant := &storage.UserGrant{
		ID:                    testGrantID,
		Principal:             id.Principal("user@example.com"),
		AgentID:               testAgentID,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: testPSID1, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}, {PermissionSetID: testPSID2, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
	}

	response := handler.toGrantResponse(grant)

	// Verify basic fields
	if response.ID != testGrantID.String() {
		t.Errorf("expected ID '%s', got '%s'", testGrantID.String(), response.ID)
	}
	if response.Principal != "user@example.com" {
		t.Errorf("expected principal 'user@example.com', got '%s'", response.Principal)
	}
	if response.AgentID != testAgentID.String() {
		t.Errorf("expected agentID '%s', got '%s'", testAgentID.String(), response.AgentID)
	}

	// Verify valid_until
	if response.ValidUntil == nil {
		t.Error("expected valid_until to be set")
	} else if !response.ValidUntil.Equal(validUntil) {
		t.Errorf("expected valid_until %v, got %v", validUntil, *response.ValidUntil)
	}

	// Verify permission sets
	if len(response.GrantedPermissionSets) != 2 {
		t.Fatalf("expected 2 permission sets, got %d", len(response.GrantedPermissionSets))
	}

	if _, ok := response.GrantedPermissionSets[testPSID1.String()]; !ok {
		t.Errorf("expected permission set ID '%s' to be present", testPSID1.String())
	}
	if _, ok := response.GrantedPermissionSets[testPSID2.String()]; !ok {
		t.Errorf("expected permission set ID '%s' to be present", testPSID2.String())
	}

	// Verify timestamps are formatted correctly (RFC3339)
	if response.CreatedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("expected created_at '2025-01-01T00:00:00Z', got '%s'", response.CreatedAt)
	}
	if response.UpdatedAt != "2025-06-01T00:00:00Z" {
		t.Errorf("expected updated_at '2025-06-01T00:00:00Z', got '%s'", response.UpdatedAt)
	}
}

// Integration-style test that verifies error handling for service errors
func TestCreateGrant_ServiceErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		serviceError   error
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "agent not found",
			serviceError:   consent.ErrAgentNotFound,
			expectedStatus: http.StatusNotFound,
			expectedError:  "agent not found",
		},
		{
			name:           "invalid scopes",
			serviceError:   consent.ErrInvalidScopes,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid scopes",
		},
		{
			name:           "service not found",
			serviceError:   consent.ErrServiceNotFound,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "service not found",
		},
		{
			name:           "grant validation failed",
			serviceError:   consent.ErrGrantValidation,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid request",
		},
		{
			name:           "generic error",
			serviceError:   errors.New("database error"),
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testAgentID := id.NewAgentID()

			// Create mock service that returns the error
			mockService := &mockConsentService{
				grantConsentFunc: func(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error) {
					return nil, tt.serviceError
				},
			}
			handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

			// Create valid request
			reqBody := GrantRequest{
				GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {id.NewServiceID().String()}},
			}

			req := newRequestWithPrincipal("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", reqBody)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("agent-id", testAgentID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rr := httptest.NewRecorder()

			handler.CreateGrant(rr, req)

			// Verify response code
			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// Verify error message
			var errResp ErrorResponse
			if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}

			if errResp.Error != tt.expectedError {
				t.Errorf("expected error '%s', got '%s'", tt.expectedError, errResp.Error)
			}
		})
	}
}

// TestCreateGrant_Success verifies successful grant creation
func TestCreateGrant_Success(t *testing.T) {
	t.Parallel()
	// Create mock service
	testAgentID := id.NewAgentID()
	testGrantID := id.NewGrantID()
	now := time.Now()
	futureTime := now.Add(24 * time.Hour)

	var capturedRequest *consent.GrantRequest
	mockService := &mockConsentService{
		grantConsentFunc: func(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error) {
			capturedRequest = req
			return &storage.UserGrant{
				ID:                    testGrantID,
				Principal:             id.Principal("user@example.com"),
				AgentID:               testAgentID,
				ValidUntil:            &futureTime,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
				CreatedAt:             now,
				UpdatedAt:             now,
			}, nil
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	// Create request
	reqBody := GrantRequest{
		ValidUntil:            &futureTime,
		GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {id.NewServiceID().String()}},
	}

	req := newRequestWithPrincipal("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", reqBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()

	handler.CreateGrant(rr, req)

	// Verify service was called and request was captured
	if capturedRequest == nil {
		t.Fatal("expected grant request to be passed to service")
	}
	if capturedRequest.Principal != id.Principal("user@example.com") {
		t.Errorf("expected principal 'user@example.com', got '%s'", capturedRequest.Principal)
	}
	if capturedRequest.AgentID != testAgentID {
		t.Errorf("expected agent_id '%s', got '%s'", testAgentID, capturedRequest.AgentID)
	}

	// Verify response
	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var envelope map[string]GrantResponse
	if err := json.NewDecoder(rr.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	response, ok := envelope["data"]
	if !ok {
		t.Fatal("expected 'data' field in response")
	}

	if response.ID != testGrantID.String() {
		t.Errorf("expected ID '%s', got '%s'", testGrantID.String(), response.ID)
	}
	if response.Principal != "user@example.com" {
		t.Errorf("expected principal 'user@example.com', got '%s'", response.Principal)
	}
	if response.AgentID != testAgentID.String() {
		t.Errorf("expected agent_id '%s', got '%s'", testAgentID.String(), response.AgentID)
	}
}

// TestCreateGrant_WithoutRedirectURI tests approval without redirect_uri (success page)
func TestCreateGrant_WithoutRedirectURI(t *testing.T) {
	t.Parallel()
	// T050: Test case 1 - Approval without redirect_uri should return success page
	testAgentID := id.NewAgentID()
	testGrantID := id.NewGrantID()
	now := time.Now()
	futureTime := now.Add(24 * time.Hour)

	mockService := &mockConsentService{
		grantConsentFunc: func(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error) {
			return &storage.UserGrant{
				ID:                    testGrantID,
				Principal:             id.Principal("user@example.com"),
				AgentID:               testAgentID,
				ValidUntil:            &futureTime,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
				CreatedAt:             now,
				UpdatedAt:             now,
			}, nil
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	reqBody := GrantRequest{
		GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {id.NewServiceID().String()}},
	}

	req := newRequestWithPrincipal("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", "user@example.com", reqBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()

	handler.CreateGrant(rr, req)

	// Should return 201 Created (not a redirect)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var envelope map[string]GrantResponse
	if err := json.NewDecoder(rr.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := envelope["data"]; !ok {
		t.Error("expected 'data' field in success response")
	}
}

// TestCreateGrant_SessionToken_ValidFlow verifies that a valid JWE session token
// produces a 201 with redirect_url from the token claims.
func TestCreateGrant_SessionToken_ValidFlow(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	testGrantID := id.NewGrantID()
	now := time.Now()
	futureTime := now.Add(24 * time.Hour)
	principalVal := "user@example.com"
	originalURL := "https://agent.example.com/authorize"

	ts := newTestJWETokenService()
	sessionToken := newTestSessionToken(ts, testAgentID, principalVal, originalURL)

	mockService := &mockConsentService{
		grantConsentFunc: func(_ context.Context, _ *consent.GrantRequest) (*storage.UserGrant, error) {
			return &storage.UserGrant{
				ID:                    testGrantID,
				Principal:             id.Principal(principalVal),
				AgentID:               testAgentID,
				ValidUntil:            &futureTime,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{},
				CreatedAt:             now,
				UpdatedAt:             now,
			}, nil
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+testAgentID.String()+"/grants?session_token="+sessionToken,
		principalVal,
		GrantRequest{GrantedPermissionSets: map[string][]string{}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, originalURL, resp["redirect_url"])
}

// TestCreateGrant_SessionToken_InvalidToken verifies that a tampered or invalid
// JWE session token produces 400.
func TestCreateGrant_SessionToken_InvalidToken(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	principalVal := "user@example.com"

	handler := NewGrantsHandler(&mockConsentService{}, nil, newTestSessionTokenValidator())

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+testAgentID.String()+"/grants?session_token=notavalidjwetoken",
		principalVal,
		GrantRequest{GrantedPermissionSets: map[string][]string{}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

// TestCreateGrant_SessionToken_Expired verifies that a valid but expired JWE session
// token produces 400 with "session_expired" error code.
func TestCreateGrant_SessionToken_Expired(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	principalVal := "user@example.com"

	ts := newTestJWETokenService()
	expiredToken := newExpiredTestSessionToken(ts, testAgentID, principalVal)

	handler := NewGrantsHandler(&mockConsentService{}, nil, newTestSessionTokenValidator())

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+testAgentID.String()+"/grants?session_token="+expiredToken,
		principalVal,
		GrantRequest{GrantedPermissionSets: map[string][]string{}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "session_expired", resp.Error)
}

// TestCreateGrant_SessionToken_AgentMismatch verifies that a session token issued for
// a different agent produces 400.
func TestCreateGrant_SessionToken_AgentMismatch(t *testing.T) {
	t.Parallel()

	agentA := id.NewAgentID()
	agentB := id.NewAgentID()
	principalVal := "user@example.com"

	ts := newTestJWETokenService()
	tokenForAgentA := newTestSessionToken(ts, agentA, principalVal, "/callback")

	handler := NewGrantsHandler(&mockConsentService{}, nil, newTestSessionTokenValidator())

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+agentB.String()+"/grants?session_token="+tokenForAgentA,
		principalVal,
		GrantRequest{GrantedPermissionSets: map[string][]string{}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentB.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

// TestCreateGrant_SessionToken_PrincipalMismatch verifies that a session token issued
// for a different user produces 403.
func TestCreateGrant_SessionToken_PrincipalMismatch(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()

	ts := newTestJWETokenService()
	tokenForUserA := newTestSessionToken(ts, testAgentID, "userA@example.com", "/callback")

	handler := NewGrantsHandler(&mockConsentService{}, nil, newTestSessionTokenValidator())

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+testAgentID.String()+"/grants?session_token="+tokenForUserA,
		"userB@example.com",
		GrantRequest{GrantedPermissionSets: map[string][]string{}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

// mockTokenValidator is a configurable SessionTokenValidator for tests that need
// to inject specific claims or errors without a real JWE round-trip.
type mockTokenValidator struct {
	claims *ports.AuthorizationSession
	err    error
}

func (m *mockTokenValidator) ValidateAuthorizationSessionToken(_ string, _ id.AgentID, _ id.Principal) (*ports.AuthorizationSession, error) {
	return m.claims, m.err
}

// TestCreateGrant_RedirectURIWithoutSessionTokenIsIgnored verifies spec scenario 3.2:
// a redirect_uri query param without a session_token is silently ignored (not echoed back).
// This pins the behaviour so any future re-introduction of a redirect_uri fallback is caught.
func TestCreateGrant_RedirectURIWithoutSessionTokenIsIgnored(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()
	testGrantID := id.NewGrantID()
	permissionSetID := id.NewPermissionSetID()
	serviceID := id.NewServiceID()
	now := time.Now()
	futureTime := now.Add(24 * time.Hour)

	mockService := &mockConsentService{
		grantConsentFunc: func(_ context.Context, _ *consent.GrantRequest) (*storage.UserGrant, error) {
			return &storage.UserGrant{
				ID:                    testGrantID,
				Principal:             id.Principal("user@example.com"),
				AgentID:               testAgentID,
				ValidUntil:            &futureTime,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: permissionSetID, IncludedServiceIDs: []id.ServiceID{serviceID}}},
				CreatedAt:             now,
				UpdatedAt:             now,
			}, nil
		},
	}

	handler := NewGrantsHandler(mockService, nil, newTestSessionTokenValidator())

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+testAgentID.String()+"/grants?redirect_uri=https://evil.example.com",
		"user@example.com",
		GrantRequest{GrantedPermissionSets: map[string][]string{permissionSetID.String(): {serviceID.String()}}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	_, hasRedirectURL := resp["redirect_url"]
	assert.False(t, hasRedirectURL, "redirect_url must not appear in response when only redirect_uri (no session_token) is provided")
}

// TestCreateGrant_ValidTokenWithEmptyOriginalURLReturns500 verifies the defense-in-depth
// guard at the handler layer: if a validated token somehow has an empty OriginalURL,
// the handler returns 500 rather than silently succeeding with no redirect.
func TestCreateGrant_ValidTokenWithEmptyOriginalURLReturns500(t *testing.T) {
	t.Parallel()

	testAgentID := id.NewAgentID()

	validator := &mockTokenValidator{
		claims: &ports.AuthorizationSession{OriginalURL: ""},
		err:    nil,
	}
	handler := NewGrantsHandler(&mockConsentService{}, nil, validator)

	req := newRequestWithPrincipal(
		"POST",
		"/api/consent/agent/"+testAgentID.String()+"/grants?session_token=dummy",
		"user@example.com",
		GrantRequest{GrantedPermissionSets: map[string][]string{}},
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
}
