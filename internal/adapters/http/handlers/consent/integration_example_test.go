package consent_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/consent"
	memorystorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	consentservice "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

func newIntegrationJWETokenService() *domjwe.TokenService {
	keyBytes, err := base64.StdEncoding.DecodeString("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=")
	if err != nil {
		panic("newIntegrationJWETokenService: " + err.Error())
	}
	key, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		panic("newIntegrationJWETokenService: " + err.Error())
	}
	return domjwe.New(key)
}

func newIntegrationSessionTokenValidator() ports.SessionTokenValidator {
	return sessiontoken.NewService(newIntegrationJWETokenService())
}

func newIntegrationProviderService(t *testing.T) *thirdparty.ThirdpartyOAuth2ProviderService {
	t.Helper()
	repo := memorystorage.NewInMemoryThirdpartyOAuth2ProviderRepository()
	return thirdparty.NewThirdpartyOAuth2ProviderService(repo, testutil.NewTestEncryptionAdapter(t), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())
}

func testAgentPermissionSets() []storage.AgentPermissionSetEntry {
	return []storage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: storage.RequirementTypeOptional,
	}}
}

type testPermissionSetQuerier struct{}

func (testPermissionSetQuerier) ValidateIDs(context.Context, []id.PermissionSetID) error {
	return nil
}

func (testPermissionSetQuerier) GetByIDs(_ context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	permissionSets := make([]*storage.PermissionSet, 0, len(ids))
	for _, permissionSetID := range ids {
		permissionSets = append(permissionSets, &storage.PermissionSet{
			ID:          permissionSetID,
			Name:        "Test Permission Set",
			Description: "Test permission set for integration coverage",
		})
	}
	return permissionSets, nil
}

func newGitHubServiceEntity() *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "GitHub",
		ClientID:    "github-client-id",
		Secret:      model.NewPlaintextSecret("github-client-secret"),
		IssuerURI:   "https://github.com",
		Discovery:   model.DiscoveryConfig{EnableDiscovery: false},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://github.com/oauth/token",
			AuthorizeEndpoint: "https://github.com/oauth/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read:user", Description: "Read user profile"},
			{ScopeValue: "repo", Description: "Full control of repositories"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// TestIntegration_GetAgentDetail exercises the GET /api/consent/agent/:agentId endpoint
// with real in-memory storage and encryption. No service requirements → Services list is empty.
func TestIntegration_GetAgentDetail(t *testing.T) {
	agentRepo := memorystorage.NewAgentRepository()
	providerService := newIntegrationProviderService(t)
	grantRepo := memorystorage.NewUserGrantRepository()

	ctx := context.Background()
	principalValue := "user@example.com"
	testAgentID := id.NewAgentID()

	govURL := "https://example.com/governance"
	docsURL := "https://example.com/docs"
	agent := &storage.Agent{
		ID:                   testAgentID,
		ClientID:             ptr.To(id.ClientID("client-123")),
		DisplayName:          "Example AI Agent",
		Description:          "An example AI agent for demonstrations",
		GovernanceURL:        &govURL,
		UserDocumentationURL: &docsURL,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		PermissionSets:       testAgentPermissionSets(),
	}
	if err := agentRepo.Create(ctx, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	if err := providerService.Create(ctx, newGitHubServiceEntity()); err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	consentSvc := consentservice.NewService(agentRepo, providerService, grantRepo, memorystorage.NewInMemoryUserSessionRepository(), testPermissionSetQuerier{}, slog.Default())
	handler := consent.NewAgentDetailHandler(consentSvc, nil, newIntegrationSessionTokenValidator())

	reqCtx := principal.WithPrincipal(ctx, principalValue)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	reqCtx = context.WithValue(reqCtx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+testAgentID.String(), nil)
	req = req.WithContext(reqCtx)

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response consent.GetAgentDetailResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Data.Agent.ID != testAgentID.String() {
		t.Errorf("expected agent ID %q, got %q", testAgentID.String(), response.Data.Agent.ID)
	}
	if response.Data.Agent.DisplayName != "Example AI Agent" {
		t.Errorf("expected display name %q, got %q", "Example AI Agent", response.Data.Agent.DisplayName)
	}
	if response.Data.Agent.Description != "An example AI agent for demonstrations" {
		t.Errorf("expected description %q, got %q", "An example AI agent for demonstrations", response.Data.Agent.Description)
	}
	// Agent has no ServiceRequirements, so the services list must be empty.
	if len(response.Data.Services) != 0 {
		t.Errorf("expected 0 services (no requirements declared), got %d", len(response.Data.Services))
	}
}

// TestIntegration_GetAgentGrants exercises the GET /api/consent/agent/:agentId/grants endpoint.
func TestIntegration_GetAgentGrants(t *testing.T) {
	agentRepo := memorystorage.NewAgentRepository()
	providerService := newIntegrationProviderService(t)
	grantRepo := memorystorage.NewUserGrantRepository()

	ctx := context.Background()
	principalValue := "user@example.com"
	testAgentID := id.NewAgentID()
	testGrantID := id.NewGrantID()
	testPermissionSetID := id.NewPermissionSetID()

	agent := &storage.Agent{
		ID:             testAgentID,
		ClientID:       ptr.To(id.ClientID("client-456")),
		DisplayName:    "Example Agent",
		Description:    "An example agent",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		PermissionSets: testAgentPermissionSets(),
	}
	if err := agentRepo.Create(ctx, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	validUntil := time.Now().Add(30 * 24 * time.Hour)
	grant := &storage.UserGrant{
		ID:                    testGrantID,
		Principal:             id.Principal(principalValue),
		AgentID:               testAgentID,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: testPermissionSetID, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
	if err := grantRepo.Create(ctx, grant); err != nil {
		t.Fatalf("failed to create grant: %v", err)
	}

	consentSvc := consentservice.NewService(agentRepo, providerService, grantRepo, nil, testPermissionSetQuerier{}, slog.Default())
	handler := consent.NewGrantsHandler(consentSvc, nil, newIntegrationSessionTokenValidator())

	reqCtx := principal.WithPrincipal(ctx, principalValue)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	reqCtx = context.WithValue(reqCtx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+testAgentID.String()+"/grants", nil)
	req = req.WithContext(reqCtx)

	rr := httptest.NewRecorder()
	handler.GetGrant(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response struct {
		Data *consent.GrantResponse `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Data == nil {
		t.Fatal("expected a grant in response, got nil")
	}
	if response.Data.AgentID != testAgentID.String() {
		t.Errorf("expected agent ID %q, got %q", testAgentID.String(), response.Data.AgentID)
	}
	if response.Data.Principal != principalValue {
		t.Errorf("expected principal %q, got %q", principalValue, response.Data.Principal)
	}
	if len(response.Data.GrantedPermissionSets) != 1 {
		t.Fatalf("expected 1 permission set, got %d", len(response.Data.GrantedPermissionSets))
	}
	if _, ok := response.Data.GrantedPermissionSets[testPermissionSetID.String()]; !ok {
		t.Errorf("expected permission set ID %q to be present", testPermissionSetID.String())
	}
}

// TestIntegration_AgentDetailFlow tests the complete consent detail + grants flow.
func TestIntegration_AgentDetailFlow(t *testing.T) {
	agentRepo := memorystorage.NewAgentRepository()
	providerService := newIntegrationProviderService(t)
	grantRepo := memorystorage.NewUserGrantRepository()

	ctx := context.Background()
	principalValue := "alice@example.com"
	testAgentID := id.NewAgentID()
	githubServiceID := id.NewServiceID()
	googleServiceID := id.NewServiceID()
	testGrantID := id.NewGrantID()

	govURL := "https://myagent.ai/governance"
	docsURL := "https://docs.myagent.ai"
	interfaceURL := "https://chat.myagent.ai"
	agent := &storage.Agent{
		ID:                   testAgentID,
		ClientID:             ptr.To(id.ClientID("client-789")),
		DisplayName:          "MyAgent AI Assistant",
		Description:          "A helpful AI assistant that can access your data",
		GovernanceURL:        &govURL,
		UserDocumentationURL: &docsURL,
		AgentInterfaceURL:    &interfaceURL,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		PermissionSets:       testAgentPermissionSets(),
	}
	if err := agentRepo.Create(ctx, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	services := []*model.ThirdpartyOAuth2ProviderEntity{
		{
			ID:          githubServiceID,
			DisplayName: "GitHub",
			ClientID:    "github-client",
			Secret:      model.NewPlaintextSecret("github-secret"),
			IssuerURI:   "https://github.com",
			Discovery:   model.DiscoveryConfig{EnableDiscovery: false},
			Endpoints: model.OAuth2Endpoints{
				TokenEndpoint:     "https://github.com/oauth/token",
				AuthorizeEndpoint: "https://github.com/oauth/authorize",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "read:user", Description: "Read user profile"},
				{ScopeValue: "repo", Description: "Full control of repositories"},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:          googleServiceID,
			DisplayName: "Google",
			ClientID:    "google-client",
			Secret:      model.NewPlaintextSecret("google-secret"),
			IssuerURI:   "https://accounts.google.com",
			Discovery:   model.DiscoveryConfig{EnableDiscovery: false},
			Endpoints: model.OAuth2Endpoints{
				TokenEndpoint:     "https://oauth2.googleapis.com/token",
				AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "email", Description: "View email address"},
				{ScopeValue: "profile", Description: "View basic profile info"},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	for _, svc := range services {
		if err := providerService.Create(ctx, svc); err != nil {
			t.Fatalf("failed to create service %s: %v", svc.ID, err)
		}
	}

	validUntil := time.Now().Add(30 * 24 * time.Hour)
	existingGrant := &storage.UserGrant{
		ID:                    testGrantID,
		Principal:             id.Principal(principalValue),
		AgentID:               testAgentID,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now().Add(-7 * 24 * time.Hour),
		UpdatedAt:             time.Now().Add(-7 * 24 * time.Hour),
	}
	if err := grantRepo.Create(ctx, existingGrant); err != nil {
		t.Fatalf("failed to create grant: %v", err)
	}

	consentSvc := consentservice.NewService(agentRepo, providerService, grantRepo, memorystorage.NewInMemoryUserSessionRepository(), testPermissionSetQuerier{}, slog.Default())

	t.Run("GetAgentDetail", func(t *testing.T) {
		handler := consent.NewAgentDetailHandler(consentSvc, nil, newIntegrationSessionTokenValidator())

		reqCtx := principal.WithPrincipal(context.Background(), principalValue)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", testAgentID.String())
		reqCtx = context.WithValue(reqCtx, chi.RouteCtxKey, rctx)

		req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+testAgentID.String(), nil)
		req = req.WithContext(reqCtx)

		rr := httptest.NewRecorder()
		handler.GetAgentDetail(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var response consent.GetAgentDetailResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Data.Agent.DisplayName != "MyAgent AI Assistant" {
			t.Errorf("unexpected display name: %s", response.Data.Agent.DisplayName)
		}
		if response.Data.Agent.ID != testAgentID.String() {
			t.Errorf("expected agent ID %q, got %q", testAgentID.String(), response.Data.Agent.ID)
		}
		// Agent has no ServiceRequirements, so no services are returned.
		if len(response.Data.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(response.Data.Services))
		}
	})

	t.Run("GetAgentGrants", func(t *testing.T) {
		handler := consent.NewGrantsHandler(consentSvc, nil, newIntegrationSessionTokenValidator())

		reqCtx := principal.WithPrincipal(context.Background(), principalValue)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", testAgentID.String())
		reqCtx = context.WithValue(reqCtx, chi.RouteCtxKey, rctx)

		req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+testAgentID.String()+"/grants", nil)
		req = req.WithContext(reqCtx)

		rr := httptest.NewRecorder()
		handler.GetGrant(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var response struct {
			Data *consent.GrantResponse `json:"data"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Data == nil {
			t.Fatal("expected a grant, got nil")
		}
		if response.Data.AgentID != testAgentID.String() {
			t.Errorf("expected agent ID %q, got %q", testAgentID.String(), response.Data.AgentID)
		}
		if len(response.Data.GrantedPermissionSets) != 1 {
			t.Fatalf("expected 1 delegated token, got %d", len(response.Data.GrantedPermissionSets))
		}
	})
}
