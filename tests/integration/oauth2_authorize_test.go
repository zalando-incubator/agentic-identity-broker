package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIntegrationJWETokenService() *domjwe.TokenService {
	keyBytes, err := base64.StdEncoding.DecodeString("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=")
	if err != nil {
		panic("newIntegrationJWETokenService: " + err.Error())
	}
	jweKey, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		panic("newIntegrationJWETokenService: " + err.Error())
	}
	return domjwe.New(jweKey)
}

func newIntegrationSessionTokenSvc() *sessiontoken.Service {
	return sessiontoken.NewService(newIntegrationJWETokenService())
}

// TestOAuth2AuthorizeEndpoint_NonUUIDClientIDError tests that a non-UUID client_id
// returns a direct 400 invalid_client (not a redirect).
func TestOAuth2AuthorizeEndpoint_NonUUIDClientIDError(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	}, nil, newIntegrationSessionTokenSvc())
	handler := &enduser.OAuth2AuthorizeHandler{Service: svc}

	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id=not-a-uuid&redirect_uri=https://client.example.com/callback&response_type=code&state=abc123",
		nil,
	)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestOAuth2AuthorizeEndpoint_UnknownAgentUUIDDirectError tests that a well-formed
// UUID that is not registered as an agent returns a direct 400 invalid_client JSON
// response (no redirect, per RFC 6749 §4.1.2.1 — redirect_uri cannot be validated).
func TestOAuth2AuthorizeEndpoint_UnknownAgentUUIDDirectError(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	}, nil, newIntegrationSessionTokenSvc())
	handler := &enduser.OAuth2AuthorizeHandler{Service: svc}

	unknownUUID := id.NewAgentID().String()
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+unknownUUID+"&redirect_uri=https://client.example.com/callback&response_type=code&state=abc123",
		nil,
	)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "invalid_client", errResp["error"])
	assert.NotEmpty(t, errResp["error_description"])
}

// TestOAuth2AuthorizeEndpoint_MissingParameterError tests validation of required parameters
func TestOAuth2AuthorizeEndpoint_MissingParameterError(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc())

	handler := &enduser.OAuth2AuthorizeHandler{
		Service: svc,
	}

	validUUID := id.NewAgentID().String()
	tests := []struct {
		name        string
		queryString string
	}{
		{"missing client_id", "?redirect_uri=https://client.example.com/callback&response_type=code"},
		{"missing redirect_uri", "?client_id=" + validUUID + "&response_type=code"},
		{"missing response_type", "?client_id=" + validUUID + "&redirect_uri=https://client.example.com/callback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				"GET",
				"https://broker.example.com/oauth2/authorize"+tt.queryString,
				nil,
			)
			req.Header.Set("X-Remote-User", "user@example.com")
			ctx := principal.WithPrincipal(req.Context(), "user@example.com")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			body, _ := io.ReadAll(w.Body)
			assert.True(t, len(body) > 0, "error message should be present")
		})
	}
}

// TestOAuth2AuthorizeEndpoint_NoGrantRedirectsToConsent tests redirect to consent UI
func TestOAuth2AuthorizeEndpoint_NoGrantRedirectsToConsent(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	// Register agent
	agent := &storage.Agent{
		ID:           id.NewAgentID(),
		ClientID:     ptr.To(id.NewClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, sessiontoken.NewService(newIntegrationJWETokenService()))

	handler := &enduser.OAuth2AuthorizeHandler{
		Service: svc,
	}

	originalURL := "https://broker.example.com/oauth2/authorize?client_id=" + agent.ID.String() + "&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123"
	req := httptest.NewRequest("GET", originalURL, nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify redirect to consent with session_token state transport
	assert.Equal(t, http.StatusFound, w.Code)
	redirectURL := w.Header().Get("Location")
	assert.Contains(t, redirectURL, "https://broker.example.com/agents/"+agent.ID.String())
	assert.Contains(t, redirectURL, "session_token=")
	assert.NotContains(t, redirectURL, "redirect_uri=")
}

// TestOAuth2AuthorizeEndpoint_ActiveGrantRedirectsToUpstream tests redirect to upstream with active grant
func TestOAuth2AuthorizeEndpoint_ActiveGrantRedirectsToUpstream(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	// Register agent
	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.NewClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	// Create active grant for user
	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil, // Indefinite grant
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc())

	handler := &enduser.OAuth2AuthorizeHandler{
		Service:        svc,
		ProceedHandler: enduser.NewProxyProceedStrategy(),
	}

	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123&scope=openid+profile",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify redirect to upstream
	assert.Equal(t, http.StatusFound, w.Code)
	redirectURL := w.Header().Get("Location")
	assert.Contains(t, redirectURL, "https://auth.example.com/authorize")

	// Verify parameters preserved
	parsedURL, _ := url.Parse(redirectURL)
	query := parsedURL.Query()
	assert.Equal(t, "client-1", query.Get("client_id"))
	assert.Equal(t, "https://client.example.com/callback", query.Get("redirect_uri"))
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Equal(t, "xyz123", query.Get("state"))
}

// TestOAuth2AuthorizeEndpoint_ExpiredGrantRedirectsToConsent tests redirect to consent with expired grant
func TestOAuth2AuthorizeEndpoint_ExpiredGrantRedirectsToConsent(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	// Register agent
	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.NewClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	// Create expired grant
	expiredTime := time.Now().Add(-1 * time.Hour)
	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            &expiredTime,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, sessiontoken.NewService(newIntegrationJWETokenService()))

	handler := &enduser.OAuth2AuthorizeHandler{
		Service: svc,
	}

	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify redirect to consent (not upstream) with session_token state transport
	assert.Equal(t, http.StatusFound, w.Code)
	redirectURL := w.Header().Get("Location")
	assert.Contains(t, redirectURL, "https://broker.example.com/agents/"+agent.ID.String())
	assert.Contains(t, redirectURL, "session_token=")
	assert.NotContains(t, redirectURL, "redirect_uri=")
	assert.NotContains(t, redirectURL, "https://auth.example.com/authorize")
}

// TestOAuth2AuthorizeEndpoint_WithMiddleware tests complete flow with audit middleware
func TestOAuth2AuthorizeEndpoint_WithMiddleware(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.NewClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc())

	handler := &enduser.OAuth2AuthorizeHandler{
		Service:        svc,
		ProceedHandler: enduser.NewProxyProceedStrategy(),
	}

	// Wrap with audit middleware
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auditMiddleware := middleware.OAuth2AuditMiddleware(logger)
	wrappedHandler := auditMiddleware(handler)

	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	req.Header.Set("X-Request-ID", "req-123")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	// Verify successful redirect
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "https://auth.example.com/authorize")
}

// TestOAuth2AuthorizeEndpoint_PKCEParametersPreserved tests PKCE parameters preserved
func TestOAuth2AuthorizeEndpoint_PKCEParametersPreserved(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.NewClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc())

	handler := &enduser.OAuth2AuthorizeHandler{
		Service:        svc,
		ProceedHandler: enduser.NewProxyProceedStrategy(),
	}

	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123&code_challenge=E9Mrozoa2owQB2dSBnnNBvjrNqtPTUAwY5uQp41VN-I&code_challenge_method=S256",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify PKCE parameters preserved in upstream URL
	redirectURL := w.Header().Get("Location")
	parsedURL, _ := url.Parse(redirectURL)
	query := parsedURL.Query()

	assert.Equal(t, "E9Mrozoa2owQB2dSBnnNBvjrNqtPTUAwY5uQp41VN-I", query.Get("code_challenge"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
}

// In-memory repository implementations for testing

type inMemoryAgentRepo struct {
	agents map[id.AgentID]*storage.Agent
}

func newInMemoryAgentRepo() *inMemoryAgentRepo {
	return &inMemoryAgentRepo{
		agents: make(map[id.AgentID]*storage.Agent),
	}
}

func (r *inMemoryAgentRepo) Create(ctx context.Context, agent *storage.Agent) error {
	r.agents[agent.ID] = agent
	return nil
}

func (r *inMemoryAgentRepo) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	agent, ok := r.agents[agentID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return agent, nil
}

func (r *inMemoryAgentRepo) Update(ctx context.Context, agent *storage.Agent) error {
	if _, ok := r.agents[agent.ID]; !ok {
		return ports.ErrNotFound
	}
	r.agents[agent.ID] = agent
	return nil
}

func (r *inMemoryAgentRepo) Delete(ctx context.Context, agentID id.AgentID) error {
	delete(r.agents, agentID)
	return nil
}

func (r *inMemoryAgentRepo) List(ctx context.Context) ([]*storage.Agent, error) {
	var agents []*storage.Agent
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	return agents, nil
}

func (r *inMemoryAgentRepo) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	for _, agent := range r.agents {
		if agent.ClientID != nil && *agent.ClientID == clientID {
			return agent, nil
		}
	}
	return nil, storage.NewStorageError(
		"GetAgentByClientID",
		storage.ErrorKindNotFound,
		ports.ErrNotFound,
		"agent not found",
	)
}

func (r *inMemoryAgentRepo) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	return nil, storage.NewStorageError(
		"GetAgentByClientURI",
		storage.ErrorKindNotFound,
		ports.ErrNotFound,
		"agent not found",
	)
}

func (r *inMemoryAgentRepo) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

type inMemoryGrantRepo struct {
	grants map[id.GrantID]*storage.UserGrant
}

func newInMemoryGrantRepo() *inMemoryGrantRepo {
	return &inMemoryGrantRepo{
		grants: make(map[id.GrantID]*storage.UserGrant),
	}
}

func (r *inMemoryGrantRepo) Create(ctx context.Context, grant *storage.UserGrant) error {
	r.grants[grant.ID] = grant
	return nil
}

func (r *inMemoryGrantRepo) Get(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	grant, ok := r.grants[grantID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return grant, nil
}

func (r *inMemoryGrantRepo) Update(ctx context.Context, grant *storage.UserGrant) error {
	if _, ok := r.grants[grant.ID]; !ok {
		return ports.ErrNotFound
	}
	r.grants[grant.ID] = grant
	return nil
}

func (r *inMemoryGrantRepo) Delete(ctx context.Context, grantID id.GrantID) error {
	delete(r.grants, grantID)
	return nil
}

func (r *inMemoryGrantRepo) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	var grants []*storage.UserGrant
	for _, grant := range r.grants {
		if grant.Principal == principal && grant.AgentID == agentID && grant.IsActive() {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (r *inMemoryGrantRepo) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	for _, grant := range r.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			return grant, nil
		}
	}
	return nil, nil
}

func (r *inMemoryGrantRepo) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	for grantID, grant := range r.grants {
		if grant.AgentID == agentID {
			delete(r.grants, grantID)
		}
	}
	return nil
}

func (r *inMemoryGrantRepo) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error) {
	var grants []storage.UserGrant
	for _, grant := range r.grants {
		if grant.Principal == principal && grant.IsActive() {
			grants = append(grants, *grant)
		}
	}
	return grants, nil
}

func (r *inMemoryGrantRepo) CountAgentsByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (int, error) {
	agents := make(map[id.AgentID]bool)
	for _, grant := range r.grants {
		if grant.Principal != principal {
			continue
		}
		for _, entry := range grant.GrantedPermissionSets {
			for _, svcID := range entry.IncludedServiceIDs {
				if svcID == serviceID {
					agents[grant.AgentID] = true
					break
				}
			}
		}
	}
	return len(agents), nil
}

func (r *inMemoryGrantRepo) ListByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) ([]id.AgentID, error) {
	agents := make(map[id.AgentID]bool)
	for _, grant := range r.grants {
		if grant.Principal != principal {
			continue
		}
		for _, entry := range grant.GrantedPermissionSets {
			for _, svcID := range entry.IncludedServiceIDs {
				if svcID == serviceID {
					agents[grant.AgentID] = true
					break
				}
			}
		}
	}
	var agentIDs []id.AgentID
	for agentID := range agents {
		agentIDs = append(agentIDs, agentID)
	}
	return agentIDs, nil
}

func (r *inMemoryGrantRepo) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	for grantID, grant := range r.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			delete(r.grants, grantID)
			return nil
		}
	}
	return ports.ErrNotFound
}

func (r *inMemoryGrantRepo) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}
