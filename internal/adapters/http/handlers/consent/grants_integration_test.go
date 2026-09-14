package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type permissivePermissionSetQuerier struct {
	serviceIDs []id.ServiceID
}

func newPermissivePermissionSetQuerier(serviceIDs ...id.ServiceID) *permissivePermissionSetQuerier {
	return &permissivePermissionSetQuerier{serviceIDs: serviceIDs}
}

func (q *permissivePermissionSetQuerier) ValidateIDs(context.Context, []id.PermissionSetID) error {
	return nil
}

func (q *permissivePermissionSetQuerier) GetByIDs(_ context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	permissionSets := make([]*storage.PermissionSet, 0, len(ids))
	for _, permissionSetID := range ids {
		serviceScopes := make([]storage.ServiceScope, 0, len(q.serviceIDs))
		for _, serviceID := range q.serviceIDs {
			serviceScopes = append(serviceScopes, storage.ServiceScope{
				ServiceID:       serviceID,
				Scopes:          []string{"test-scope"},
				RequirementType: storage.RequirementTypeOptional,
			})
		}
		permissionSets = append(permissionSets, &storage.PermissionSet{
			ID:            permissionSetID,
			Name:          "Test Permission Set",
			Description:   "Test Permission Set",
			ServiceScopes: serviceScopes,
		})
	}
	return permissionSets, nil
}

func seedActiveSession(t *testing.T, repo ports.UserSessionRepository, principal id.Principal, serviceID id.ServiceID) {
	t.Helper()
	require.NoError(t, repo.Create(context.Background(), &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            principal,
		ServiceID:            serviceID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		EncryptionContext:    storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}))
}

// TestGrantsIntegration_CreateUpdateRevoke tests the full lifecycle of a grant
// with real in-memory repositories. This verifies:
// - Grant creation (T075, T076, T079)
// - Grant update (upsert semantics)
// - Grant revocation (empty tokens)
// - Validation (T077)
func TestGrantsIntegration_CreateUpdateRevoke(t *testing.T) {
	// Setup real repositories
	agentRepo := memory.NewAgentRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create providerService to handle encryption context binding (simulates domain layer)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())

	// Seed test data
	ctx := context.Background()

	testAgentID := id.NewAgentID()
	githubServiceID := id.NewServiceID()
	googleServiceID := id.NewServiceID()

	// Create agent
	agent := &storage.Agent{
		ID:          testAgentID,
		ClientID:    ptr.To(id.ClientID("client-test")),
		DisplayName: "Test Agent",
		Description: "Integration test agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: id.NewPermissionSetID(),
			RequirementType: storage.RequirementTypeOptional,
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := agentRepo.Create(ctx, agent)
	require.NoError(t, err)

	// Create GitHub service via providerService
	githubService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          githubServiceID,
		DisplayName: "GitHub",
		ClientID:    "github-client",
		Secret:      model.NewPlaintextSecret("github-secret"),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Full control of private repositories"},
			{ScopeValue: "user:email", Description: "Access user email addresses"},
			{ScopeValue: "read:user", Description: "Read user profile data"},
		},
	}
	err = providerService.Create(ctx, githubService)
	require.NoError(t, err)

	// Create Google service via providerService
	googleService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          googleServiceID,
		DisplayName: "Google",
		ClientID:    "google-client",
		Secret:      model.NewPlaintextSecret("google-secret"),
		IssuerURI:   "https://accounts.google.com",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://oauth2.googleapis.com/token",
			AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "openid", Description: "OpenID Connect"},
			{ScopeValue: "email", Description: "View email address"},
			{ScopeValue: "profile", Description: "View basic profile info"},
		},
	}
	err = providerService.Create(ctx, googleService)
	require.NoError(t, err)

	sessionRepo := memory.NewInMemoryUserSessionRepository()
	psService := newPermissivePermissionSetQuerier(githubServiceID, googleServiceID)
	seedActiveSession(t, sessionRepo, id.Principal("alice@example.com"), githubServiceID)
	seedActiveSession(t, sessionRepo, id.Principal("alice@example.com"), googleServiceID)

	// Create consent service
	consentService := consent.NewService(agentRepo, providerService, grantRepo, sessionRepo, psService, slog.Default())

	// Create handler
	handler := NewGrantsHandler(consentService, nil, newTestSessionTokenValidator())

	// Test 1: Create initial grant
	t.Run("create_grant", func(t *testing.T) {
		futureTime := time.Now().Add(30 * 24 * time.Hour) // 30 days
		psID := id.NewPermissionSetID()
		reqBody := GrantRequest{
			ValidUntil:            &futureTime,
			GrantedPermissionSets: map[string][]string{psID.String(): {githubServiceID.String()}},
		}

		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", bytes.NewBuffer(jsonBody))
		ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", testAgentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, req)

		// Verify response
		assert.Equal(t, http.StatusCreated, rr.Code)

		var envelope map[string]GrantResponse
		err := json.NewDecoder(rr.Body).Decode(&envelope)
		require.NoError(t, err)

		response, ok := envelope["data"]
		require.True(t, ok, "expected 'data' field in response")

		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "alice@example.com", response.Principal)
		assert.Equal(t, testAgentID.String(), response.AgentID)
		assert.NotNil(t, response.ValidUntil)
		assert.Len(t, response.GrantedPermissionSets, 1)
		if _, ok := response.GrantedPermissionSets[psID.String()]; !ok {
			t.Errorf("expected permission set ID '%s' to be present", psID.String())
		}
	})

	// Test 2: Update grant (upsert semantics)
	t.Run("update_grant", func(t *testing.T) {
		futureTime := time.Now().Add(60 * 24 * time.Hour) // 60 days
		reqBody := GrantRequest{
			ValidUntil:            &futureTime,
			GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {githubServiceID.String()}, id.NewPermissionSetID().String(): {googleServiceID.String()}},
		}

		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/consent/agent/"+testAgentID.String()+"/grants", bytes.NewBuffer(jsonBody))
		//nolint:staticcheck // Using string key for test simplicity
		ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", testAgentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, req)

		// Verify response
		assert.Equal(t, http.StatusCreated, rr.Code)

		var envelope map[string]GrantResponse
		err := json.NewDecoder(rr.Body).Decode(&envelope)
		require.NoError(t, err)

		response, ok := envelope["data"]
		require.True(t, ok, "expected 'data' field in response")

		// Grant was updated, not created (same principal+agent)
		assert.Equal(t, "alice@example.com", response.Principal)
		assert.Equal(t, testAgentID.String(), response.AgentID)
		assert.Len(t, response.GrantedPermissionSets, 2)
	})

	// Test 3: Retrieve grants
	t.Run("get_grants", func(t *testing.T) {
		agentGrantsHandler := NewGrantsHandler(consentService, slog.Default(), newTestSessionTokenValidator())

		req := httptest.NewRequest("GET", "/api/consent/agent/"+testAgentID.String()+"/grants", nil)
		//nolint:staticcheck // Using string key for test simplicity
		ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", testAgentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		agentGrantsHandler.GetGrant(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)

		var envelope struct {
			Data *GrantResponse `json:"data"`
		}
		err := json.NewDecoder(rr.Body).Decode(&envelope)
		require.NoError(t, err)
		require.NotNil(t, envelope.Data, "expected grant in response")

		assert.Equal(t, "alice@example.com", envelope.Data.Principal)
		assert.Equal(t, testAgentID.String(), envelope.Data.AgentID)
		assert.Len(t, envelope.Data.GrantedPermissionSets, 2)
	})

	// Test 4: Revoke grant via DELETE /grants
	t.Run("revoke_grant", func(t *testing.T) {
		revokeHandler := NewGrantsHandler(consentService, nil, newTestSessionTokenValidator())

		req := httptest.NewRequest("DELETE", "/api/consent/agent/"+testAgentID.String()+"/grants", nil)
		//nolint:staticcheck // Using string key for test simplicity
		ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", testAgentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		revokeHandler.RevokeGrant(rr, req)

		// Verify response
		assert.Equal(t, http.StatusNoContent, rr.Code)
	})
}

func TestGrantsIntegration_SessionToken_CreateGrantWithRedirect(t *testing.T) {
	agentRepo := memory.NewAgentRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())

	ctx := context.Background()
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()
	principalValue := id.Principal("alice@example.com")
	permissionSetID := id.NewPermissionSetID()
	originalURL := "https://agent.example.com/authorize?response_type=code"

	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("client-test")),
		DisplayName: "Test Agent",
		Description: "Integration test agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetID,
			RequirementType: storage.RequirementTypeOptional,
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, agentRepo.Create(ctx, agent))

	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		DisplayName: "GitHub",
		ClientID:    "github-client",
		Secret:      model.NewPlaintextSecret("github-secret"),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Full control of private repositories"},
			{ScopeValue: "read:user", Description: "Read user profile data"},
		},
	}
	require.NoError(t, providerService.Create(ctx, service))

	ts := newTestJWETokenService()
	sessionToken := newTestSessionToken(ts, agentID, principalValue.String(), originalURL)
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	seedActiveSession(t, sessionRepo, principalValue, serviceID)

	consentService := consent.NewService(agentRepo, providerService, grantRepo, sessionRepo, newPermissivePermissionSetQuerier(serviceID), slog.Default())
	handler := NewGrantsHandler(consentService, nil, newTestSessionTokenValidator())

	validUntil := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	reqBody := GrantRequest{
		ValidUntil:            &validUntil,
		GrantedPermissionSets: map[string][]string{permissionSetID.String(): {serviceID.String()}},
	}
	jsonBody, marshalErr := json.Marshal(reqBody)
	require.NoError(t, marshalErr)

	req := httptest.NewRequest("POST", "/api/consent/agent/"+agentID.String()+"/grants?session_token="+sessionToken, bytes.NewBuffer(jsonBody))
	req = req.WithContext(principal.WithPrincipal(req.Context(), principalValue.String()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.CreateGrant(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, originalURL, resp["redirect_url"])

	storedGrant, err := grantRepo.FindByPrincipalAndAgent(ctx, principalValue, agentID)
	require.NoError(t, err)
	require.NotNil(t, storedGrant)
	require.Len(t, storedGrant.GrantedPermissionSets, 1)
	assert.Equal(t, permissionSetID, storedGrant.GrantedPermissionSets[0].PermissionSetID)
	assert.Equal(t, []id.ServiceID{serviceID}, storedGrant.GrantedPermissionSets[0].IncludedServiceIDs)
}

// TestGrantsIntegration_OptionalOnlyAgent verifies that agents with only optional service
// requirements can grant the optional service and that an ignored redirect_uri does not
// create a redirect response without a session token.
func TestGrantsIntegration_OptionalOnlyAgent(t *testing.T) {
	agentRepo := memory.NewAgentRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())
	ctx := context.Background()

	// Agent with only optional service requirements
	optionalAgentID := id.NewAgentID()
	optionalServiceID := id.NewServiceID()
	permissionSetID := id.NewPermissionSetID()

	optionalAgent := &storage.Agent{
		ID:          optionalAgentID,
		ClientID:    ptr.To(id.ClientID("client-optional")),
		DisplayName: "Optional-Only Agent",
		Description: "Agent with only optional service requirements",
		ServiceRequirements: []storage.ServiceRequirement{
			{
				ServiceID:       optionalServiceID,
				RequirementType: storage.RequirementTypeOptional,
				RequiredScopes:  []string{"read"},
			},
		},
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetID,
			RequirementType: storage.RequirementTypeOptional,
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := agentRepo.Create(ctx, optionalAgent)
	require.NoError(t, err)

	optionalService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          optionalServiceID,
		DisplayName: "Optional Service",
		ClientID:    "opt-client",
		Secret:      model.NewPlaintextSecret("opt-secret"),
		IssuerURI:   "https://optional.example.com",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://optional.example.com/token",
			AuthorizeEndpoint: "https://optional.example.com/auth",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
		},
	}
	err = providerService.Create(ctx, optionalService)
	require.NoError(t, err)
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	seedActiveSession(t, sessionRepo, id.Principal("bob@example.com"), optionalServiceID)

	consentService := consent.NewService(agentRepo, providerService, grantRepo, sessionRepo, newPermissivePermissionSetQuerier(optionalServiceID), slog.Default())
	handler := NewGrantsHandler(consentService, nil, newTestSessionTokenValidator())

	t.Run("approve_optional_service", func(t *testing.T) {
		reqBody := GrantRequest{
			GrantedPermissionSets: map[string][]string{permissionSetID.String(): {optionalServiceID.String()}},
		}

		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/consent/agent/"+optionalAgentID.String()+"/grants", bytes.NewBuffer(jsonBody))
		ctx := principal.WithPrincipal(req.Context(), "bob@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", optionalAgentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		data := resp["data"].(map[string]interface{})
		assert.Equal(t, optionalAgentID.String(), data["agent_id"])
		grantedPermissionSets, ok := data["granted_permission_sets"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, []interface{}{optionalServiceID.String()}, grantedPermissionSets[permissionSetID.String()])
	})

	// redirect_uri without session_token must be ignored — no redirect_url in response (SR-004, ADR 016).
	t.Run("redirect_uri_without_session_token_is_ignored", func(t *testing.T) {
		reqBody := GrantRequest{
			GrantedPermissionSets: map[string][]string{permissionSetID.String(): {optionalServiceID.String()}},
		}

		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/consent/agent/"+optionalAgentID.String()+"/grants?redirect_uri=%2Fcallback", bytes.NewBuffer(jsonBody))
		ctx := principal.WithPrincipal(req.Context(), "bob@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", optionalAgentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		assert.NotContains(t, resp, "redirect_url", "redirect_uri fallback is removed per SR-004")
	})
}

// TestGrantsIntegration_Validation tests validation logic (T077)
func TestGrantsIntegration_Validation(t *testing.T) {
	// Setup repositories
	agentRepo := memory.NewAgentRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create providerService to handle encryption context binding (simulates domain layer)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())

	ctx := context.Background()

	testAgentID := id.NewAgentID()
	testServiceID := id.NewServiceID()

	// Create agent
	agent := &storage.Agent{
		ID:          testAgentID,
		ClientID:    ptr.To(id.ClientID("client-validate")),
		DisplayName: "Validation Test Agent",
		Description: "Test validation",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: id.NewPermissionSetID(),
			RequirementType: storage.RequirementTypeOptional,
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := agentRepo.Create(ctx, agent)
	require.NoError(t, err)

	// Create service with limited scopes via providerService
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          testServiceID,
		DisplayName: "Test Service",
		ClientID:    "test-client",
		Secret:      model.NewPlaintextSecret("test-secret"),
		IssuerURI:   "https://example.com",
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://example.com/token",
			AuthorizeEndpoint: "https://example.com/auth",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
			{ScopeValue: "write", Description: "Write access"},
		},
	}
	err = providerService.Create(ctx, service)
	require.NoError(t, err)

	sessionRepo := memory.NewInMemoryUserSessionRepository()
	psService := newPermissivePermissionSetQuerier(service.ID)
	seedActiveSession(t, sessionRepo, id.Principal("test@example.com"), service.ID)

	consentService := consent.NewService(agentRepo, providerService, grantRepo, sessionRepo, psService, slog.Default())
	handler := NewGrantsHandler(consentService, nil, newTestSessionTokenValidator())

	tests := []struct {
		name           string
		agentID        string
		reqBody        GrantRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name:    "invalid_permission_set_id",
			agentID: testAgentID.String(),
			reqBody: GrantRequest{
				GrantedPermissionSets: map[string][]string{"not-a-valid-uuid": {id.NewServiceID().String()}},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid request",
		},
		{
			name:    "nonexistent_agent",
			agentID: id.NewAgentID().String(),
			reqBody: GrantRequest{
				GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {service.ID.String()}},
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "agent not found",
		},
		{
			name:    "valid_request",
			agentID: testAgentID.String(),
			reqBody: GrantRequest{
				GrantedPermissionSets: map[string][]string{id.NewPermissionSetID().String(): {service.ID.String()}},
			},
			expectedStatus: http.StatusCreated,
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.reqBody)
			req := httptest.NewRequest("POST", "/api/consent/agent/"+tt.agentID+"/grants", bytes.NewBuffer(jsonBody))
			//nolint:staticcheck // Using string key for test simplicity
			ctx := principal.WithPrincipal(req.Context(), "test@example.com")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("agent-id", tt.agentID)
			req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

			rr := httptest.NewRecorder()
			handler.CreateGrant(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var errResp ErrorResponse
				err := json.NewDecoder(rr.Body).Decode(&errResp)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedError, errResp.Error)
			}
		})
	}
}

// TestGrantsIntegration_FR020_UnconnectedServices tests that submitting a grant
// with included services that have no active OAuth2 session returns HTTP 400 (FR-020).
func TestGrantsIntegration_FR020_UnconnectedServices(t *testing.T) {
	agentRepo := memory.NewAgentRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	unconnectedServiceID := id.NewServiceID()

	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())
	ctx := context.Background()

	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("client-fr020")),
		DisplayName: "FR-020 Test Agent",
		Description: "Agent for FR-020 unconnected services test",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: id.NewPermissionSetID(),
			RequirementType: storage.RequirementTypeOptional,
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, agentRepo.Create(ctx, agent))

	// Wire session repo so FR-020 validation is active; no sessions are seeded.
	consentService := consent.NewService(agentRepo, providerService, grantRepo, sessionRepo, newPermissivePermissionSetQuerier(unconnectedServiceID), slog.Default())
	handler := NewGrantsHandler(consentService, nil, newTestSessionTokenValidator())

	t.Run("returns_400_when_included_service_has_no_active_session", func(t *testing.T) {
		psID := id.NewPermissionSetID()
		reqBody := GrantRequest{
			GrantedPermissionSets: map[string][]string{psID.String(): {unconnectedServiceID.String()}},
		}
		jsonBody, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/consent/agent/"+agentID.String()+"/grants", bytes.NewBuffer(jsonBody))
		ctx := principal.WithPrincipal(req.Context(), "carol@example.com")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", agentID.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		var errResp ErrorResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&errResp))
		assert.Equal(t, "unconnected services", errResp.Error)
	})
}
