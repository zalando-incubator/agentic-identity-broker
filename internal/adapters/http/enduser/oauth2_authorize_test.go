package enduser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestJWETokenService() *domjwe.TokenService {
	keyBytes, err := base64.StdEncoding.DecodeString("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=")
	if err != nil {
		panic("oauth2_authorize_test: failed to decode test JWE key: " + err.Error())
	}
	jweKey, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		panic("oauth2_authorize_test: failed to import test JWE key: " + err.Error())
	}
	return domjwe.New(jweKey)
}

func newTestSessionTokenSvc() *sessiontoken.Service {
	return sessiontoken.NewService(newTestJWETokenService())
}

// TestOAuth2AuthorizeHandler_ServeHTTP_MissingPrincipal tests handler when principal is not provided
func TestOAuth2AuthorizeHandler_ServeHTTP_MissingPrincipal(t *testing.T) {
	// Setup
	agentRepo := newMockAgentRepo()
	svc := oauth2.NewAuthorizationService(
		newMockGrantRepo(),
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)

	handler := &OAuth2AuthorizeHandler{
		Service: svc,
	}

	// Request without principal in context (missing principal)
	// MustFromContext panics when principal is missing
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&redirect_uri=https://client.example.com/callback&response_type=code",
		nil,
	)
	w := httptest.NewRecorder()

	// Execute - expect panic since principal is missing from context
	// In production, RequirePrincipalMiddleware prevents this
	assert.Panics(t, func() {
		handler.ServeHTTP(w, req)
	})
}

// TestOAuth2AuthorizeHandler_ServeHTTP_MissingParameters tests handler when required OAuth2 parameters missing
func TestOAuth2AuthorizeHandler_ServeHTTP_MissingParameters(t *testing.T) {
	agentRepo := newMockAgentRepo()
	svc := oauth2.NewAuthorizationService(
		newMockGrantRepo(),
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)

	handler := &OAuth2AuthorizeHandler{
		Service: svc,
	}

	tests := []struct {
		name             string
		queryPath        string
		wantErrorCode    string
		wantErrorDescKey string
	}{
		{
			name:             "missing client_id",
			queryPath:        "?redirect_uri=https://client.example.com/callback&response_type=code",
			wantErrorCode:    "invalid_request",
			wantErrorDescKey: "client_id",
		},
		{
			name:             "missing redirect_uri",
			queryPath:        "?client_id=550e8400-e29b-41d4-a716-446655440000&response_type=code",
			wantErrorCode:    "invalid_request",
			wantErrorDescKey: "redirect_uri",
		},
		{
			name:             "missing response_type",
			queryPath:        "?client_id=550e8400-e29b-41d4-a716-446655440000&redirect_uri=https://client.example.com/callback",
			wantErrorCode:    "invalid_request",
			wantErrorDescKey: "response_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				"GET",
				"https://broker.example.com/oauth2/authorize"+tt.queryPath,
				nil,
			)
			req.Header.Set("X-Remote-User", "user@example.com")
			ctx := principal.WithPrincipal(req.Context(), "user@example.com")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var body struct {
				Error            string `json:"error"`
				ErrorDescription string `json:"error_description"`
			}
			err := json.NewDecoder(w.Body).Decode(&body)
			require.NoError(t, err, "response body must be valid JSON")
			assert.Equal(t, tt.wantErrorCode, body.Error)
			assert.Contains(t, body.ErrorDescription, tt.wantErrorDescKey)
		})
	}
}

// TestOAuth2AuthorizeHandler_ServeHTTP_MalformedClientID tests direct 400 for an unregistered client_id.
// The service returns invalid_client when no agent matches, without redirecting to an unverified redirect_uri.
func TestOAuth2AuthorizeHandler_ServeHTTP_MalformedClientID(t *testing.T) {
	mockAgentRepo := newMockAgentRepo()
	handler := &OAuth2AuthorizeHandler{
		Service: oauth2.NewAuthorizationService(
			newMockGrantRepo(),
			&noopSessionRepository{},
			oauth2.NewAgentClientResolver(mockAgentRepo, nil),
			&oauth2.OAuth2Config{
				ModeStrategy:              oauth2.NewProxyModeStrategy(),
				UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
				PublicURL:                 "https://broker.example.com",
			},
			nil,
			newTestSessionTokenSvc(),
		),
	}

	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id=not-a-uuid&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123",
		nil,
	)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(body), `"invalid_client"`)
}

// TestOAuth2AuthorizeHandler_ServeHTTP_UnknownAgent tests that a valid UUID client_id
// that does not match any registered agent returns a direct 400 JSON response.
// RFC 6749 §4.1.2.1: MUST NOT redirect when the client cannot be verified.
func TestOAuth2AuthorizeHandler_ServeHTTP_UnknownAgent(t *testing.T) {
	agentRepo := newMockAgentRepo()
	svc := oauth2.NewAuthorizationService(
		newMockGrantRepo(),
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)

	handler := &OAuth2AuthorizeHandler{
		Service: svc,
	}

	unknownUUID := id.NewAgentID().String()
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+unknownUUID+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var errResp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "invalid_client", errResp["error"])
	assert.NotEmpty(t, errResp["error_description"])
}

// TestOAuth2AuthorizeHandler_ServeHTTP_NoGrantRedirectsToConsent tests redirect to consent UI when no active grant
func TestOAuth2AuthorizeHandler_ServeHTTP_NoGrantRedirectsToConsent(t *testing.T) {
	agentRepo := newMockAgentRepo()
	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	svc := oauth2.NewAuthorizationService(
		newMockGrantRepo(),
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		sessiontoken.NewService(newTestJWETokenService()),
	)

	handler := &OAuth2AuthorizeHandler{
		Service: svc,
	}

	// Feature 021: client_id is now the agent UUID, not the upstream client_id
	originalURL := "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String() + "&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123"
	req := httptest.NewRequest("GET", originalURL, nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should redirect to consent UI with a JWE session token
	assert.Equal(t, http.StatusFound, w.Code)
	redirectURL := w.Header().Get("Location")
	assert.Contains(t, redirectURL, "https://broker.example.com/agents/"+agentID.String())
	assert.Contains(t, redirectURL, "session_token=")
}

// TestOAuth2AuthorizeHandler_ServeHTTP_ActiveGrantRedirectsToUpstream tests redirect to upstream with active grant
func TestOAuth2AuthorizeHandler_ServeHTTP_ActiveGrantRedirectsToUpstream(t *testing.T) {
	agentRepo := newMockAgentRepo()
	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	grantRepo := newMockGrantRepo()
	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := oauth2.NewAuthorizationService(
		grantRepo,
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)

	handler := &OAuth2AuthorizeHandler{
		Service:        svc,
		ProceedHandler: NewProxyProceedStrategy(),
	}

	// Feature 021: client_id is now the agent UUID, not the upstream client_id
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

	// Should redirect to upstream
	assert.Equal(t, http.StatusFound, w.Code)
	redirectURL := w.Header().Get("Location")
	assert.Contains(t, redirectURL, "https://auth.example.com/authorize")
	assert.Contains(t, redirectURL, "client_id=client-1")
	assert.Contains(t, redirectURL, "state=xyz123")
	assert.Contains(t, redirectURL, "response_type=code")
}

// TestOAuth2AuthorizeHandler_ServeHTTP_PreservesOAuth2Parameters tests all OAuth2 parameters preserved
func TestOAuth2AuthorizeHandler_ServeHTTP_PreservesOAuth2Parameters(t *testing.T) {
	agentRepo := newMockAgentRepo()
	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		DisplayName:  "Test Client",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	grantRepo := newMockGrantRepo()
	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := oauth2.NewAuthorizationService(
		grantRepo,
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)

	handler := &OAuth2AuthorizeHandler{
		Service:        svc,
		ProceedHandler: NewProxyProceedStrategy(),
	}

	// Feature 021: client_id is now the agent UUID, not the upstream client_id
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz123&scope=openid+profile+email&code_challenge=E9Mrozoa2owQB2dSBnnNBvjrNqtPTUAwY5uQp41VN-I&code_challenge_method=S256",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	redirectURL := w.Header().Get("Location")

	// Verify all parameters preserved
	parsedURL, _ := url.Parse(redirectURL)
	query := parsedURL.Query()
	assert.Equal(t, "client-1", query.Get("client_id"))
	assert.Equal(t, "https://client.example.com/callback", query.Get("redirect_uri"))
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Equal(t, "xyz123", query.Get("state"))
	assert.Equal(t, "openid profile email", query.Get("scope"))
	assert.Equal(t, "E9Mrozoa2owQB2dSBnnNBvjrNqtPTUAwY5uQp41VN-I", query.Get("code_challenge"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
}

// TestOAuth2AuthorizeHandler_ServeHTTP_JSONResponseFormat tests error responses use proper JSON format
func TestOAuth2AuthorizeHandler_ServeHTTP_JSONResponseFormat(t *testing.T) {
	agentRepo := newMockAgentRepo()
	svc := oauth2.NewAuthorizationService(
		newMockGrantRepo(),
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)

	handler := &OAuth2AuthorizeHandler{
		Service: svc,
	}

	// Missing required parameter
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?redirect_uri=https://client.example.com/callback",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body, _ := io.ReadAll(w.Body)

	// For parameter validation errors, expect plain text error
	assert.Contains(t, string(body), "client_id")
}

type mockCodeIssuer struct{}

func (m *mockCodeIssuer) IssueAuthorizationCode(_ context.Context, _ *ports.AuthorizationRequest, _ id.Principal) (string, error) {
	return "test-code", nil
}

type errCodeIssuer struct{ err error }

func (m *errCodeIssuer) IssueAuthorizationCode(_ context.Context, _ *ports.AuthorizationRequest, _ id.Principal) (string, error) {
	return "", m.err
}

type proceedOAuth2Service struct{}

func (s *proceedOAuth2Service) HandleAuthorization(_ context.Context, _ *ports.AuthorizationRequest, _ id.Principal) (*ports.AuthorizationDecision, error) {
	return ports.ProceedDecision("", storage.LocalClient), nil
}

func (s *proceedOAuth2Service) GenerateMetadata(_ context.Context) (*ports.MetadataResponse, error) {
	return &ports.MetadataResponse{}, nil
}

func (s *proceedOAuth2Service) ResolveForTokenGrant(_ context.Context, _ id.ClientID) (*ports.TokenGrantResolution, error) {
	return nil, errors.New("not implemented")
}

// TestOAuth2AuthorizeHandler_LocalMode_CodeIssuerErrors tests that IssueAuthorizationCode
// domain errors produce correct HTTP status codes and JSON bodies.
func TestOAuth2AuthorizeHandler_LocalMode_CodeIssuerErrors(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantErrCode string
		isRedirect  bool
	}{
		// ErrInvalidClient: HTTP 401 — never redirect (RFC 6749 §4.1.2.1)
		{"unknown client", oauth2server.ErrInvalidClient, http.StatusUnauthorized, "invalid_client", false},
		// ErrInvalidRedirectURI: HTTP 400 — never redirect (redirect_uri unverified)
		{"invalid redirect uri", oauth2server.ErrInvalidRedirectURI, http.StatusBadRequest, "invalid_request", false},
		// ErrServerError: HTTP 500 — never redirect when validation state is uncertain
		{"server error", oauth2server.ErrServerError, http.StatusInternalServerError, "server_error", false},
		// Unexpected non-OAuth error: HTTP 500 — never redirect
		{"unexpected error", errors.New("boom"), http.StatusInternalServerError, "server_error", false},
		// ErrInvalidRequest: redirect is safe
		{"invalid request", oauth2server.NewRFC6749Error("invalid_request", "bad request", http.StatusBadRequest, oauth2server.ErrInvalidRequest), http.StatusFound, "invalid_request", true},
		// ErrUnsupportedResponseType: redirect is safe
		{"unsupported response type", oauth2server.NewRFC6749Error("unsupported_response_type", "unsupported", http.StatusBadRequest, oauth2server.ErrUnsupportedResponseType), http.StatusFound, "unsupported_response_type", true},
		// ErrInvalidScope: redirect is safe
		{"invalid scope", oauth2server.NewRFC6749Error("invalid_scope", "invalid scope", http.StatusBadRequest, oauth2server.ErrInvalidScope), http.StatusFound, "invalid_scope", true},
	}

	agentID := id.NewAgentID()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &OAuth2AuthorizeHandler{
				Service:        &proceedOAuth2Service{},
				ProceedHandler: NewLocalProceedStrategy(&errCodeIssuer{err: tc.err}, nil),
			}

			req := httptest.NewRequest(
				"GET",
				"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code&state=xyz",
				nil,
			)
			ctx := principal.WithPrincipal(req.Context(), "user@example.com")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.isRedirect {
				// Scope errors redirect to redirect_uri with error query parameters
				loc := w.Header().Get("Location")
				redirectURL, err := url.Parse(loc)
				require.NoError(t, err)
				q := redirectURL.Query()
				assert.Equal(t, tc.wantErrCode, q.Get("error"))
				assert.NotEmpty(t, q.Get("error_description"))
				assert.Equal(t, "xyz", q.Get("state"))
			} else {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
				body, _ := io.ReadAll(w.Body)
				var errResp map[string]string
				require.NoError(t, json.Unmarshal(body, &errResp), "body must be valid JSON: %s", string(body))
				assert.Equal(t, tc.wantErrCode, errResp["error"])
				assert.NotEmpty(t, errResp["error_description"])
			}
		})
	}
}

// Helper functions for test setup

type mockAgentRepository struct {
	agents map[id.AgentID]*storage.Agent
}

func newMockAgentRepo() *mockAgentRepository {
	return &mockAgentRepository{
		agents: make(map[id.AgentID]*storage.Agent),
	}
}

func (m *mockAgentRepository) Create(ctx context.Context, agent *storage.Agent) error {
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	agent, ok := m.agents[agentID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return agent, nil
}

func (m *mockAgentRepository) Update(ctx context.Context, agent *storage.Agent) error {
	if _, ok := m.agents[agent.ID]; !ok {
		return ports.ErrNotFound
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepository) Delete(ctx context.Context, agentID id.AgentID) error {
	delete(m.agents, agentID)
	return nil
}

func (m *mockAgentRepository) List(ctx context.Context) ([]*storage.Agent, error) {
	var agents []*storage.Agent
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	return agents, nil
}

func (m *mockAgentRepository) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	for _, agent := range m.agents {
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

func (m *mockAgentRepository) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindNotFound, ports.ErrNotFound, "not found")
}

func (m *mockAgentRepository) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

type mockGrantRepository struct {
	grants map[id.GrantID]*storage.UserGrant
}

func newMockGrantRepo() *mockGrantRepository {
	return &mockGrantRepository{
		grants: make(map[id.GrantID]*storage.UserGrant),
	}
}

func (m *mockGrantRepository) Create(ctx context.Context, grant *storage.UserGrant) error {
	m.grants[grant.ID] = grant
	return nil
}

func (m *mockGrantRepository) Get(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	grant, ok := m.grants[grantID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return grant, nil
}

func (m *mockGrantRepository) Update(ctx context.Context, grant *storage.UserGrant) error {
	if _, ok := m.grants[grant.ID]; !ok {
		return ports.ErrNotFound
	}
	m.grants[grant.ID] = grant
	return nil
}

func (m *mockGrantRepository) Delete(ctx context.Context, grantID id.GrantID) error {
	delete(m.grants, grantID)
	return nil
}

func (m *mockGrantRepository) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	var grants []*storage.UserGrant
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID && grant.IsActive() {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (m *mockGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			return grant, nil
		}
	}
	return nil, nil
}

func (m *mockGrantRepository) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	for grantID, grant := range m.grants {
		if grant.AgentID == agentID {
			delete(m.grants, grantID)
		}
	}
	return nil
}

func (m *mockGrantRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error) {
	var grants []storage.UserGrant
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.IsActive() {
			grants = append(grants, *grant)
		}
	}
	return grants, nil
}

func (m *mockGrantRepository) CountAgentsByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (int, error) {
	agents := make(map[id.AgentID]bool)
	for _, grant := range m.grants {
		if grant.Principal != principal {
			continue
		}
		for _, entry := range grant.GrantedPermissionSets {
			for _, svcID := range entry.IncludedServiceIDs {
				if svcID == serviceID {
					agents[grant.AgentID] = true
				}
			}
		}
	}
	return len(agents), nil
}

type noopSessionRepository struct{}

func (n *noopSessionRepository) Create(_ context.Context, _ *storage.UserSession) error { return nil }
func (n *noopSessionRepository) Get(_ context.Context, _ id.SessionID) (*storage.UserSession, error) {
	return nil, &storage.StorageError{Kind: storage.ErrorKindNotFound}
}
func (n *noopSessionRepository) FindByPrincipalAndService(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
	return nil, nil
}
func (n *noopSessionRepository) ListByPrincipal(_ context.Context, _ id.Principal) ([]*storage.UserSession, error) {
	return nil, nil
}
func (n *noopSessionRepository) ListActiveByPrincipal(_ context.Context, _ id.Principal) ([]*storage.UserSession, error) {
	return nil, nil
}
func (n *noopSessionRepository) Delete(_ context.Context, _ id.SessionID) error { return nil }
func (n *noopSessionRepository) DeleteByPrincipalAndService(_ context.Context, _ id.Principal, _ id.ServiceID) error {
	return nil
}
func (n *noopSessionRepository) CountByService(_ context.Context, _ id.ServiceID) (int, error) {
	return 0, nil
}

func (m *mockGrantRepository) ListByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) ([]id.AgentID, error) {
	agents := make(map[id.AgentID]bool)
	for _, grant := range m.grants {
		if grant.Principal != principal {
			continue
		}
		for _, entry := range grant.GrantedPermissionSets {
			for _, svcID := range entry.IncludedServiceIDs {
				if svcID == serviceID {
					agents[grant.AgentID] = true
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

func (m *mockGrantRepository) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	for grantID, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			delete(m.grants, grantID)
			return nil
		}
	}
	return ports.ErrNotFound
}

func (m *mockGrantRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

// errorAgentRepository returns a configurable error from Get to simulate infrastructure failures.
type errorAgentRepository struct {
	mockAgentRepository
	getErr error
}

func (r *errorAgentRepository) Get(_ context.Context, _ id.AgentID) (*storage.Agent, error) {
	return nil, r.getErr
}

// TestOAuth2AuthorizeHandler_ServeHTTP_StorageErrorReturns500 verifies that a storage-level
// error in the service layer (no valid redirect_uri to redirect to) produces HTTP 500, not 400.
func TestOAuth2AuthorizeHandler_ServeHTTP_StorageErrorReturns500(t *testing.T) {
	storageErr := storage.NewStorageError("Get", storage.ErrorKindConnection, nil, "connection refused")
	agentRepo := &errorAgentRepository{
		mockAgentRepository: *newMockAgentRepo(),
		getErr:              storageErr,
	}

	svc := oauth2.NewAuthorizationService(
		newMockGrantRepo(),
		&noopSessionRepository{},
		oauth2.NewAgentClientResolver(agentRepo, nil),
		&oauth2.OAuth2Config{
			ModeStrategy:              oauth2.NewProxyModeStrategy(),
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
		},
		nil,
		newTestSessionTokenSvc(),
	)
	handler := &OAuth2AuthorizeHandler{Service: svc}

	agentID := id.NewAgentID()
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/callback&response_type=code",
		nil,
	)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var body struct {
		Error string `json:"error"`
	}
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "server_error", body.Error)
}

type erroringOAuth2Service struct{}

func (s *erroringOAuth2Service) HandleAuthorization(_ context.Context, _ *ports.AuthorizationRequest, _ id.Principal) (*ports.AuthorizationDecision, error) {
	return nil, errors.New("storage unavailable")
}

func (s *erroringOAuth2Service) GenerateMetadata(_ context.Context) (*ports.MetadataResponse, error) {
	return &ports.MetadataResponse{}, nil
}

func (s *erroringOAuth2Service) ResolveForTokenGrant(_ context.Context, _ id.ClientID) (*ports.TokenGrantResolution, error) {
	return nil, errors.New("not implemented")
}

// TestOAuth2AuthorizeHandler_LogsAuthorizationRequestFailed verifies that when
// HandleAuthorization returns an error, the logger receives an authorization_request_failed
// entry. This guards against the Logger field being silently unwired.
func TestOAuth2AuthorizeHandler_LogsAuthorizationRequestFailed(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	handler := &OAuth2AuthorizeHandler{
		Service: &erroringOAuth2Service{},
		Logger:  logger,
	}

	agentID := id.NewAgentID()
	req := httptest.NewRequest(
		"GET",
		"/?client_id="+agentID.String()+"&redirect_uri=https://client.example.com/cb&response_type=code",
		nil,
	)
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, buf.String(), "authorization_request_failed")
}
