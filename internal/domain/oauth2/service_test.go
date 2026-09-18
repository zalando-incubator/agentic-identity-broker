package oauth2

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newServiceTestJWETokenService returns a JWE token service backed by a deterministic test key.
func newServiceTestJWETokenService() *domjwe.TokenService {
	keyBytes, err := base64.StdEncoding.DecodeString("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=")
	if err != nil {
		panic("newServiceTestJWETokenService: failed to decode key: " + err.Error())
	}
	jweKey, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		panic("newServiceTestJWETokenService: failed to import key: " + err.Error())
	}
	return domjwe.New(jweKey)
}

func newTestSessionTokenService() *sessiontoken.Service {
	return sessiontoken.NewService(newServiceTestJWETokenService())
}

func newTestAuthorizationService(agentRepo ports.AgentRepository, grantRepo ports.UserGrantRepository, cfg *OAuth2Config) ports.OAuth2Service {
	return NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), cfg, nil, newTestSessionTokenService())
}

func newTestAuthorizationServiceWithSessions(agentRepo ports.AgentRepository, grantRepo ports.UserGrantRepository, sessionRepo ports.UserSessionRepository, cfg *OAuth2Config) ports.OAuth2Service {
	return NewAuthorizationService(grantRepo, sessionRepo, NewAgentClientResolver(agentRepo, nil), cfg, nil, newTestSessionTokenService())
}

type MockAgentRepository struct {
	agents            map[id.AgentID]*storage.Agent
	byURI             map[string]*storage.Agent
	getByClientURIErr error
}

func NewMockAgentRepository() *MockAgentRepository {
	return &MockAgentRepository{
		agents: make(map[id.AgentID]*storage.Agent),
		byURI:  make(map[string]*storage.Agent),
	}
}

func (m *MockAgentRepository) Create(ctx context.Context, agent *storage.Agent) error {
	m.agents[agent.ID] = agent
	return nil
}

func (m *MockAgentRepository) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	agent, ok := m.agents[agentID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return agent, nil
}

func (m *MockAgentRepository) Update(ctx context.Context, agent *storage.Agent) error {
	if _, ok := m.agents[agent.ID]; !ok {
		return ports.ErrNotFound
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *MockAgentRepository) Delete(ctx context.Context, agentID id.AgentID) error {
	delete(m.agents, agentID)
	return nil
}

func (m *MockAgentRepository) List(ctx context.Context) ([]*storage.Agent, error) {
	var agents []*storage.Agent
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	return agents, nil
}

func (m *MockAgentRepository) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	for _, agent := range m.agents {
		if agent.ClientID != nil && *agent.ClientID == clientID {
			return agent, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (m *MockAgentRepository) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	if m.getByClientURIErr != nil {
		return nil, m.getByClientURIErr
	}
	if a, ok := m.byURI[uri]; ok {
		return a, nil
	}
	return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindNotFound, ports.ErrNotFound, "not found")
}

func (m *MockAgentRepository) RegisterURI(uri string, agent *storage.Agent) {
	m.byURI[uri] = agent
}

func (m *MockAgentRepository) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

// MockGrantRepository is a test double for UserGrantRepository
type MockGrantRepository struct {
	grants map[id.GrantID]*storage.UserGrant
}

func NewMockGrantRepository() *MockGrantRepository {
	return &MockGrantRepository{
		grants: make(map[id.GrantID]*storage.UserGrant),
	}
}

func (m *MockGrantRepository) Create(ctx context.Context, grant *storage.UserGrant) error {
	m.grants[grant.ID] = grant
	return nil
}

func (m *MockGrantRepository) Get(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	grant, ok := m.grants[grantID]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return grant, nil
}

func (m *MockGrantRepository) Update(ctx context.Context, grant *storage.UserGrant) error {
	if _, ok := m.grants[grant.ID]; !ok {
		return ports.ErrNotFound
	}
	m.grants[grant.ID] = grant
	return nil
}

func (m *MockGrantRepository) Delete(ctx context.Context, grantID id.GrantID) error {
	delete(m.grants, grantID)
	return nil
}

func (m *MockGrantRepository) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	var grants []*storage.UserGrant
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID && grant.IsActive() {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (m *MockGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			return grant, nil
		}
	}
	return nil, nil
}

func (m *MockGrantRepository) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	for grantKey, grant := range m.grants {
		if grant.AgentID == agentID {
			delete(m.grants, grantKey)
		}
	}
	return nil
}

func (m *MockGrantRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error) {
	var grants []storage.UserGrant
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.IsActive() {
			grants = append(grants, *grant)
		}
	}
	return grants, nil
}

func (m *MockGrantRepository) CountAgentsByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (int, error) {
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

func (m *MockGrantRepository) ListByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) ([]id.AgentID, error) {
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

func (m *MockGrantRepository) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	for grantKey, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			delete(m.grants, grantKey)
			return nil
		}
	}
	return ports.ErrNotFound
}

func (m *MockGrantRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

// MockSessionRepository is a test double for UserSessionRepository.
// Uses configurable findFunc and listFunc so each test case can define its own
// behaviour for FindByPrincipalAndService and ListByPrincipal.
// All other methods are intentionally no-op stubs.
type MockSessionRepository struct {
	findFunc func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error)
	listFunc func(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error)
}

func NewMockSessionRepository() *MockSessionRepository {
	return &MockSessionRepository{}
}

func (m *MockSessionRepository) Create(ctx context.Context, session *storage.UserSession) error {
	return nil
}

// Get is not exercised by the authorization flow tests but must satisfy the interface.
func (m *MockSessionRepository) Get(ctx context.Context, sessionID id.SessionID) (*storage.UserSession, error) {
	return nil, &storage.StorageError{Kind: storage.ErrorKindNotFound}
}

func (m *MockSessionRepository) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	if m.findFunc != nil {
		return m.findFunc(ctx, principal, serviceID)
	}
	return nil, nil
}

// ListByPrincipal supports session expiry checks in the authorization flow.
// Uses listFunc if set; returns empty slice otherwise.
func (m *MockSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, principal)
	}
	return nil, nil
}

func (m *MockSessionRepository) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, principal)
	}
	return nil, nil
}

func (m *MockSessionRepository) Delete(ctx context.Context, sessionID id.SessionID) error {
	return nil
}

func (m *MockSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	return nil
}

func (m *MockSessionRepository) CountByService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	return 0, nil
}

// TestService_HandleAuthorization tests the HandleAuthorization method with table-driven tests
func TestService_HandleAuthorization(t *testing.T) {
	testAgentID := id.NewAgentID()

	tests := []struct {
		name       string
		setupAgent func(*MockAgentRepository)
		setupGrant func(*MockGrantRepository)
		authReq    *ports.AuthorizationRequest
		principal  id.Principal
		wantAction string
	}{
		{
			name:       "unregistered agent UUID returns error redirect",
			setupAgent: func(r *MockAgentRepository) {},
			setupGrant: func(r *MockGrantRepository) {},
			authReq: &ports.AuthorizationRequest{
				ClientID:     id.ClientID(id.NewAgentID().String()), // valid UUID but not in repo
				RedirectURI:  "https://client.example.com/callback",
				State:        "xyz123",
				ResponseType: "code",
			},
			principal:  id.NewPrincipal("user@example.com"),
			wantAction: "error",
		},
		{
			name: "valid UUID client_id with no grant redirects to consent UI",
			setupAgent: func(r *MockAgentRepository) {
				agent := &storage.Agent{
					ID:           testAgentID,
					ClientID:     ptr.To(id.ClientID("client-1")),
					DisplayName:  "Test Client",
					RedirectURIs: []string{"https://client.example.com/callback"},
				}
				_ = r.Create(context.Background(), agent)
			},
			setupGrant: func(r *MockGrantRepository) {},
			authReq: &ports.AuthorizationRequest{
				ClientID:     id.ClientID(testAgentID.String()),
				RedirectURI:  "https://client.example.com/callback",
				State:        "xyz123",
				ResponseType: "code",
				OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + testAgentID.String(),
			},
			principal:  id.NewPrincipal("user@example.com"),
			wantAction: "redirect_to_consent",
		},
		{
			name: "valid UUID client_id with active grant redirects to upstream",
			setupAgent: func(r *MockAgentRepository) {
				agent := &storage.Agent{
					ID:           testAgentID,
					ClientID:     ptr.To(id.ClientID("client-1")),
					DisplayName:  "Test Client",
					RedirectURIs: []string{"https://client.example.com/callback"},
				}
				_ = r.Create(context.Background(), agent)
			},
			setupGrant: func(r *MockGrantRepository) {
				grant := &storage.UserGrant{
					ID:                    id.NewGrantID(),
					Principal:             id.Principal("user@example.com"),
					AgentID:               testAgentID,
					ValidUntil:            nil,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
				}
				_ = r.Create(context.Background(), grant)
			},
			authReq: &ports.AuthorizationRequest{
				ClientID:     id.ClientID(testAgentID.String()),
				RedirectURI:  "https://client.example.com/callback",
				Scope:        "openid profile",
				State:        "xyz123",
				ResponseType: "code",
			},
			principal:  id.NewPrincipal("user@example.com"),
			wantAction: "proceed",
		},
		{
			name: "expired grant redirects to consent UI",
			setupAgent: func(r *MockAgentRepository) {
				agent := &storage.Agent{
					ID:           testAgentID,
					ClientID:     ptr.To(id.ClientID("client-1")),
					DisplayName:  "Test Client",
					RedirectURIs: []string{"https://client.example.com/callback"},
				}
				_ = r.Create(context.Background(), agent)
			},
			setupGrant: func(r *MockGrantRepository) {
				expiredTime := time.Now().Add(-1 * time.Hour)
				grant := &storage.UserGrant{
					ID:                    id.NewGrantID(),
					Principal:             id.Principal("user@example.com"),
					AgentID:               testAgentID,
					ValidUntil:            &expiredTime,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
				}
				_ = r.Create(context.Background(), grant)
			},
			authReq: &ports.AuthorizationRequest{
				ClientID:     id.ClientID(testAgentID.String()),
				RedirectURI:  "https://client.example.com/callback",
				State:        "xyz123",
				ResponseType: "code",
				OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + testAgentID.String(),
			},
			principal:  id.NewPrincipal("user@example.com"),
			wantAction: "redirect_to_consent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentRepo := NewMockAgentRepository()
			grantRepo := NewMockGrantRepository()

			tt.setupAgent(agentRepo)
			tt.setupGrant(grantRepo)

			svc := newTestAuthorizationService(agentRepo, grantRepo, &OAuth2Config{
				UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
				PublicURL:                 "https://broker.example.com",
				ModeStrategy:              NewProxyModeStrategy(),
				SupportedResponseTypes:    []string{"code"},
				SupportedGrantTypes:       []string{"authorization_code"},
			})

			ctx := context.Background()
			decision, err := svc.HandleAuthorization(ctx, tt.authReq, tt.principal)

			require.NoError(t, err, "HandleAuthorization should not error")
			require.NotNil(t, decision, "Decision should not be nil")

			assert.Equal(t, tt.wantAction, decision.Action, "Action mismatch")
			if tt.wantAction == "redirect_to_consent" {
				assert.Contains(t, decision.RedirectURL, "session_token=", "redirect_to_consent must include a session_token")
				assert.NotContains(t, decision.RedirectURL, "redirect_uri=", "redirect_to_consent must not fall back to redirect_uri")
			}
		})
	}
}

func TestService_HandleAuthorization_AllowsOfflineAccessReservedScope(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()
	require.NoError(t, agentRepo.Create(context.Background(), &storage.Agent{
		ID:            agentID,
		ClientID:      ptr.To(id.ClientID(agentID.String())),
		DisplayName:   "Offline Scope Agent",
		Description:   "Agent that allows read but not offline_access explicitly",
		RedirectURIs:  []string{"https://client.example.com/callback"},
		AllowedScopes: []string{"read"},
	}))

	svc := newTestAuthorizationService(agentRepo, grantRepo, &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	})

	decision, err := svc.HandleAuthorization(context.Background(), &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		Scope:        "read offline_access",
		State:        "xyz",
		OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
	}, id.NewPrincipal("user@example.com"))
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "redirect_to_consent", decision.Action)
}

// TestService_HandleAuthorization_SessionExpiry tests that expired delegated sessions
// redirect back to consent even when the grant itself is still active.
func TestService_HandleAuthorization_SessionExpiry(t *testing.T) {
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()

	activeGrant := func(r *MockGrantRepository) {
		grant := &storage.UserGrant{
			ID:                    id.NewGrantID(),
			Principal:             id.Principal("user@example.com"),
			AgentID:               agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{serviceID}}},
		}
		_ = r.Create(context.Background(), grant)
	}

	cfg := &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
	}

	authReq := &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
	}

	tests := []struct {
		name         string
		setupSession func(*MockSessionRepository)
		wantAction   string
	}{
		{
			name: "active grant with valid session redirects to upstream",
			setupSession: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return &storage.UserSession{
						ID:        id.NewSessionID(),
						Principal: id.Principal("user@example.com"),
						ServiceID: serviceID,
						TokenType: "Bearer",
					}, nil
				}
			},
			wantAction: "proceed",
		},
		{
			name: "active grant with expired session redirects to consent",
			setupSession: func(r *MockSessionRepository) {
				expiredAt := time.Now().Add(-1 * time.Hour)
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return &storage.UserSession{
						ID:                    id.NewSessionID(),
						Principal:             id.Principal("user@example.com"),
						ServiceID:             serviceID,
						TokenType:             "Bearer",
						RefreshTokenExpiresAt: &expiredAt,
					}, nil
				}
			},
			wantAction: "redirect_to_consent",
		},
		{
			name: "active grant with no session yet still redirects to upstream",
			setupSession: func(r *MockSessionRepository) {
				// findFunc returns nil, nil — no session established yet.
				// Absence of a session is not an expiry; mandatory-requirements
				// validation (Step 5) handles that case separately.
			},
			wantAction: "proceed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentRepo := NewMockAgentRepository()
			grantRepo := NewMockGrantRepository()
			sessionRepo := NewMockSessionRepository()

			_ = agentRepo.Create(context.Background(), &storage.Agent{
				ID:           agentID,
				ClientID:     ptr.To(id.ClientID("client-1")),
				DisplayName:  "Test Client",
				RedirectURIs: []string{"https://client.example.com/callback"},
			})
			activeGrant(grantRepo)
			tt.setupSession(sessionRepo)

			svc := newTestAuthorizationServiceWithSessions(agentRepo, grantRepo, sessionRepo, cfg)

			decision, err := svc.HandleAuthorization(context.Background(), authReq, id.NewPrincipal("user@example.com"))

			require.NoError(t, err)
			require.NotNil(t, decision)
			assert.Equal(t, tt.wantAction, decision.Action)
			if tt.wantAction == "redirect_to_consent" {
				assert.Contains(t, decision.RedirectURL, "session_token=", "redirect_to_consent must include a session_token")
				assert.NotContains(t, decision.RedirectURL, "redirect_uri=", "redirect_to_consent must not fall back to redirect_uri")
			}
		})
	}
}

// TestService_HandleAuthorization_PreservesParameters tests that OAuth2 parameters are preserved
func TestService_HandleAuthorization_PreservesParameters(t *testing.T) {
	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	agentID := id.NewAgentID()

	// Add agent
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	// Add active grant
	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
	}, nil, newTestSessionTokenService())

	authReq := &ports.AuthorizationRequest{
		ClientID:            id.ClientID(agentID.String()),
		RedirectURI:         "https://client.example.com/callback",
		Scope:               "openid profile email",
		State:               "state123",
		ResponseType:        "code",
		CodeChallenge:       "E9Mrozoa2owQB2dSBnnNBvjrNqtPTUAwY5uQp41VN-I",
		CodeChallengeMethod: "S256",
	}

	decision, err := svc.HandleAuthorization(context.Background(), authReq, id.NewPrincipal("user@example.com"))

	require.NoError(t, err)
	require.Equal(t, "proceed", decision.Action)

	// Verify the redirect URL contains all parameters
	redirectURL := decision.RedirectURL
	assert.Contains(t, redirectURL, "client_id=client-1")
	assert.Contains(t, redirectURL, "redirect_uri=https%3A%2F%2Fclient.example.com%2Fcallback")
	assert.Contains(t, redirectURL, "scope=openid+profile+email")
	assert.Contains(t, redirectURL, "state=state123")
	assert.Contains(t, redirectURL, "response_type=code")
	assert.Contains(t, redirectURL, "code_challenge=E9Mrozoa2owQB2dSBnnNBvjrNqtPTUAwY5uQp41VN-I")
	assert.Contains(t, redirectURL, "code_challenge_method=S256")
}

// TestService_HandleAuthorization_UUIDResolution verifies agent resolution via the authorize endpoint.
// The client_id value is parsed as a UUID internally by the service to look up the agent.
func TestService_HandleAuthorization_UUIDResolution(t *testing.T) {
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()

	setupAgent := func(r *MockAgentRepository) {
		agent := &storage.Agent{
			ID:           agentID,
			ClientID:     ptr.To(id.ClientID("upstream-client-1")), // upstream OAuth2 client ID
			DisplayName:  "Test Agent",
			RedirectURIs: []string{"https://client.example.com/callback"},
		}
		_ = r.Create(context.Background(), agent)
	}

	setupActiveGrant := func(r *MockGrantRepository) {
		grant := &storage.UserGrant{
			ID:        id.NewGrantID(),
			Principal: id.Principal("user@example.com"),
			AgentID:   agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{serviceID}},
			},
		}
		_ = r.Create(context.Background(), grant)
	}

	unknownAgentID := id.NewAgentID()

	tests := []struct {
		name          string
		setupAgent    func(*MockAgentRepository)
		setupGrant    func(*MockGrantRepository)
		clientID      id.ClientID
		wantAction    string
		wantErrorCode string
	}{
		{
			name:          "valid agent UUID resolves agent and redirects to upstream",
			setupAgent:    setupAgent,
			setupGrant:    setupActiveGrant,
			clientID:      id.ClientID(agentID.String()),
			wantAction:    "proceed",
			wantErrorCode: "",
		},
		{
			name:          "well-formed UUID that is not a registered agent returns invalid_client",
			setupAgent:    func(r *MockAgentRepository) {}, // empty repo
			setupGrant:    func(r *MockGrantRepository) {},
			clientID:      id.ClientID(unknownAgentID.String()),
			wantAction:    "error",
			wantErrorCode: "invalid_client",
		},
		{
			name:          "malformed UUID string returns invalid_client",
			setupAgent:    func(r *MockAgentRepository) {},
			setupGrant:    func(r *MockGrantRepository) {},
			clientID:      id.ClientID("not-a-uuid"),
			wantAction:    "error",
			wantErrorCode: "invalid_client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentRepo := NewMockAgentRepository()
			grantRepo := NewMockGrantRepository()

			tt.setupAgent(agentRepo)
			tt.setupGrant(grantRepo)

			svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
				UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
				PublicURL:                 "https://broker.example.com",
				ModeStrategy:              NewProxyModeStrategy(),
				SupportedResponseTypes:    []string{"code"},
			}, nil, newTestSessionTokenService())

			req := &ports.AuthorizationRequest{
				ClientID:     tt.clientID,
				RedirectURI:  "https://client.example.com/callback",
				ResponseType: "code",
				State:        "state123",
				OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + tt.clientID.String(),
			}

			decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))

			require.NoError(t, err)
			require.NotNil(t, decision)
			assert.Equal(t, tt.wantAction, decision.Action)
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, decision.ErrorCode)
			}
		})
	}
}

// TestService_HandleAuthorization_UUIDResolution_UpstreamClientID verifies that the
// upstream authorize URL uses agent.ClientID (upstream OAuth2 client ID), NOT the
// broker's internal agent UUID.
func TestService_HandleAuthorization_UUIDResolution_UpstreamClientID(t *testing.T) {
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()

	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("upstream-client-abc")), // this is what should appear in upstream URL
		DisplayName:  "Test Agent",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}
	_ = agentRepo.Create(context.Background(), agent)

	grant := &storage.UserGrant{
		ID:        id.NewGrantID(),
		Principal: id.Principal("user@example.com"),
		AgentID:   agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
			{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{serviceID}},
		},
	}
	_ = grantRepo.Create(context.Background(), grant)

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
	}, nil, newTestSessionTokenService())

	req := &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		State:        "state123",
	}

	decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))

	require.NoError(t, err)
	assert.Equal(t, "proceed", decision.Action)

	// The upstream URL MUST use the agent's upstream ClientID, NOT the internal UUID
	assert.Contains(t, decision.RedirectURL, "client_id=upstream-client-abc",
		"upstream URL must use agent.ClientID (upstream OAuth2 client ID), not the broker UUID")
	assert.NotContains(t, decision.RedirectURL, agentID.String(),
		"upstream URL must NOT expose the broker's internal agent UUID as client_id")
}

// TestService_GenerateMetadata tests RFC 8414 metadata generation
func TestService_GenerateMetadata(t *testing.T) {
	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	config := &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code", "refresh_token"},
	}

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), config, nil, newTestSessionTokenService())

	tests := []struct {
		name string
		want *ports.MetadataResponse
	}{
		{
			name: "valid metadata generation",
			want: &ports.MetadataResponse{
				Issuer:                            "https://broker.example.com",
				AuthorizationEndpoint:             "https://broker.example.com/oauth2/authorize",
				TokenEndpoint:                     "https://broker.example.com/oauth2/token",
				ResponseTypesSupported:            []string{"code"},
				GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
				TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "client_secret_basic"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := svc.GenerateMetadata(context.Background())

			require.NoError(t, err)
			require.NotNil(t, metadata)

			assert.Equal(t, tt.want.Issuer, metadata.Issuer)
			assert.Equal(t, tt.want.AuthorizationEndpoint, metadata.AuthorizationEndpoint)
			assert.Equal(t, tt.want.TokenEndpoint, metadata.TokenEndpoint)
			assert.Equal(t, tt.want.ResponseTypesSupported, metadata.ResponseTypesSupported)
			assert.Equal(t, tt.want.GrantTypesSupported, metadata.GrantTypesSupported)
			assert.Equal(t, tt.want.TokenEndpointAuthMethodsSupported, metadata.TokenEndpointAuthMethodsSupported)
		})
	}
}

// TestService_HandleAuthorization_MultiAgentParamInjection tests that the agent UUID
// is appended to the upstream authorize URL when multi_agent_client is enabled,
// and is absent when the feature is disabled (Feature 021).
func TestService_HandleAuthorization_MultiAgentParamInjection(t *testing.T) {
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()

	makeRepos := func() (*MockAgentRepository, *MockGrantRepository) {
		agentRepo := NewMockAgentRepository()
		grantRepo := NewMockGrantRepository()

		agent := &storage.Agent{
			ID:           agentID,
			ClientID:     ptr.To(id.ClientID("shared-upstream-client")),
			DisplayName:  "Test Agent",
			RedirectURIs: []string{"https://client.example.com/callback"},
		}
		_ = agentRepo.Create(context.Background(), agent)

		grant := &storage.UserGrant{
			ID:        id.NewGrantID(),
			Principal: id.Principal("user@example.com"),
			AgentID:   agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{serviceID}},
			},
		}
		_ = grantRepo.Create(context.Background(), grant)
		return agentRepo, grantRepo
	}

	req := &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		State:        "state-xyz",
	}

	t.Run("param injected when multi_agent_client enabled", func(t *testing.T) {
		agentRepo, grantRepo := makeRepos()
		svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
			ModeStrategy:              NewProxyModeStrategy(),
			MultiAgentClient: ports.MultiAgentClientConfig{
				Enabled:          true,
				AgentIDParamName: "x_agent_id",
				AgentIDClaimName: "x_agent_id",
			},
		}, nil, newTestSessionTokenService())

		decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))
		require.NoError(t, err)
		assert.Equal(t, "proceed", decision.Action)
		assert.Contains(t, decision.RedirectURL, "x_agent_id="+agentID.String(),
			"agent UUID must be injected as x_agent_id param when multi_agent_client is enabled")
	})

	t.Run("param absent when multi_agent_client disabled", func(t *testing.T) {
		agentRepo, grantRepo := makeRepos()
		svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			PublicURL:                 "https://broker.example.com",
			ModeStrategy:              NewProxyModeStrategy(),
			MultiAgentClient:          ports.MultiAgentClientConfig{Enabled: false},
		}, nil, newTestSessionTokenService())

		decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))
		require.NoError(t, err)
		assert.Equal(t, "proceed", decision.Action)
		assert.NotContains(t, decision.RedirectURL, "x_agent_id",
			"agent UUID param must not be present when multi_agent_client is disabled")
	})
}

// TestService_HandleAuthorization_RedirectURIValidation verifies that when an agent has
// registered redirect URIs, only those URIs are accepted. Agents with no registered URIs
// are rejected — fail closed per RFC 6749 §4.1.2.1.
func TestService_HandleAuthorization_RedirectURIValidation(t *testing.T) {
	agentID := id.NewAgentID()
	registeredURI := "https://client.example.com/callback"
	unregisteredURI := "https://evil.example.com/steal"

	makeAgent := func(uris []string) *storage.Agent {
		return &storage.Agent{
			ID:           agentID,
			ClientID:     ptr.To(id.ClientID("client-1")),
			DisplayName:  "Test Agent",
			RedirectURIs: uris,
		}
	}

	tests := []struct {
		name          string
		redirectURIs  []string // agent's registered URIs
		requestURI    string   // URI from the incoming request
		wantAction    string
		wantErrorCode string
	}{
		{
			name:          "agent with no registered URIs rejects any redirect_uri (fail closed)",
			redirectURIs:  nil,
			requestURI:    unregisteredURI,
			wantAction:    "error",
			wantErrorCode: "invalid_redirect_uri",
		},
		{
			name:         "matching registered URI is accepted",
			redirectURIs: []string{registeredURI},
			requestURI:   registeredURI,
			wantAction:   "redirect_to_consent",
		},
		{
			name:          "non-matching URI returns invalid_redirect_uri with no redirect",
			redirectURIs:  []string{registeredURI},
			requestURI:    unregisteredURI,
			wantAction:    "error",
			wantErrorCode: "invalid_redirect_uri",
		},
		{
			name:          "non-local http URI in registry rejected at runtime (legacy data guard)",
			redirectURIs:  []string{"http://legacy.example.com/callback"},
			requestURI:    "http://legacy.example.com/callback",
			wantAction:    "error",
			wantErrorCode: "invalid_redirect_uri",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentRepo := NewMockAgentRepository()
			grantRepo := NewMockGrantRepository()
			_ = agentRepo.Create(context.Background(), makeAgent(tt.redirectURIs))

			svc := newTestAuthorizationService(agentRepo, grantRepo, &OAuth2Config{
				PublicURL:    "https://broker.example.com",
				ModeStrategy: NewProxyModeStrategy(),
			})

			req := &ports.AuthorizationRequest{
				ClientID:     id.ClientID(agentID.String()),
				RedirectURI:  tt.requestURI,
				ResponseType: "code",
				OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
			}

			decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))

			require.NoError(t, err)
			require.NotNil(t, decision)
			assert.Equal(t, tt.wantAction, decision.Action)
			if tt.wantAction == "redirect_to_consent" {
				assert.Contains(t, decision.RedirectURL, "session_token=", "redirect_to_consent must include a session_token")
				assert.NotContains(t, decision.RedirectURL, "redirect_uri=", "redirect_to_consent must not fall back to redirect_uri")
			}
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, decision.ErrorCode)
				assert.Empty(t, decision.RedirectURL, "invalid_redirect_uri must not include a redirect URL")
			}
		})
	}
}

// TestService_HandleAuthorization_ScopeValidation verifies that when an agent has
// registered allowed scopes, only those scopes may be requested. Agents with no
// allowed scopes accept any requested scope.
func TestService_HandleAuthorization_ScopeValidation(t *testing.T) {
	agentID := id.NewAgentID()

	makeAgent := func(allowed []string) *storage.Agent {
		return &storage.Agent{
			ID:            agentID,
			ClientID:      ptr.To(id.ClientID("client-1")),
			DisplayName:   "Test Agent",
			RedirectURIs:  []string{"https://client.example.com/callback"},
			AllowedScopes: allowed,
		}
	}

	tests := []struct {
		name          string
		allowedScopes []string
		requestScope  string
		wantAction    string
		wantErrorCode string
		wantRedirect  bool // true if error should carry a redirect URL
	}{
		{
			name:          "agent with no allowed scopes accepts any requested scope",
			allowedScopes: nil,
			requestScope:  "openid profile email",
			wantAction:    "redirect_to_consent",
		},
		{
			name:          "requesting only allowed scopes is accepted",
			allowedScopes: []string{"openid", "profile"},
			requestScope:  "openid profile",
			wantAction:    "redirect_to_consent",
		},
		{
			name:          "requesting an unauthorized scope returns invalid_scope redirect",
			allowedScopes: []string{"openid"},
			requestScope:  "openid admin",
			wantAction:    "error",
			wantErrorCode: "invalid_scope",
			wantRedirect:  true,
		},
		{
			name:          "empty scope with restricted agent is accepted",
			allowedScopes: []string{"openid"},
			requestScope:  "",
			wantAction:    "redirect_to_consent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentRepo := NewMockAgentRepository()
			grantRepo := NewMockGrantRepository()
			_ = agentRepo.Create(context.Background(), makeAgent(tt.allowedScopes))

			svc := newTestAuthorizationService(agentRepo, grantRepo, &OAuth2Config{
				PublicURL:    "https://broker.example.com",
				ModeStrategy: NewProxyModeStrategy(),
			})

			req := &ports.AuthorizationRequest{
				ClientID:     id.ClientID(agentID.String()),
				RedirectURI:  "https://client.example.com/callback",
				ResponseType: "code",
				Scope:        tt.requestScope,
				State:        "state-abc",
				OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
			}

			decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))

			require.NoError(t, err)
			require.NotNil(t, decision)
			assert.Equal(t, tt.wantAction, decision.Action)
			if tt.wantAction == "redirect_to_consent" {
				assert.Contains(t, decision.RedirectURL, "session_token=", "redirect_to_consent must include a session_token")
				assert.NotContains(t, decision.RedirectURL, "redirect_uri=", "redirect_to_consent must not fall back to redirect_uri")
			}
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, decision.ErrorCode)
			}
			if tt.wantRedirect {
				assert.NotEmpty(t, decision.RedirectURL, "invalid_scope should include a redirect URL")
				assert.Contains(t, decision.RedirectURL, "error=invalid_scope")
				assert.Contains(t, decision.RedirectURL, "state=state-abc")
			}
		})
	}
}

// TestService_GenerateMetadata_IssuerURIOverride verifies that when IssuerURI differs from
// PublicURL, GenerateMetadata uses IssuerURI so the discovery document matches token iss claims.
func TestService_GenerateMetadata_IssuerURIOverride(t *testing.T) {
	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	config := &OAuth2Config{
		PublicURL:              "https://broker.example.com",
		IssuerURI:              "https://sso.example.com",
		ModeStrategy:           NewLocalModeStrategy(),
		SupportedResponseTypes: []string{"code"},
		SupportedGrantTypes:    []string{"authorization_code"},
	}
	svc := newTestAuthorizationService(agentRepo, grantRepo, config)
	metadata, err := svc.GenerateMetadata(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "https://sso.example.com", metadata.Issuer)
	assert.Equal(t, "https://sso.example.com/oauth2/authorize", metadata.AuthorizationEndpoint)
	assert.Equal(t, "https://sso.example.com/oauth2/token", metadata.TokenEndpoint)
	assert.Equal(t, "https://sso.example.com/oauth2/jwks.json", metadata.JWKSURI)
}

// TestService_GenerateMetadata_RFC8414Compliance tests RFC 8414 compliance
func TestService_GenerateMetadata_RFC8414Compliance(t *testing.T) {
	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	config := &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	}

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), config, nil, newTestSessionTokenService())
	metadata, err := svc.GenerateMetadata(context.Background())

	require.NoError(t, err)

	// RFC 8414 Section 2 requires these fields
	assert.NotEmpty(t, metadata.Issuer, "issuer must not be empty")
	assert.NotEmpty(t, metadata.AuthorizationEndpoint, "authorization_endpoint must not be empty")
	assert.NotEmpty(t, metadata.TokenEndpoint, "token_endpoint must not be empty")
	assert.NotEmpty(t, metadata.ResponseTypesSupported, "response_types_supported must not be empty")
	assert.NotEmpty(t, metadata.GrantTypesSupported, "grant_types_supported must not be empty")

	// Verify issuer is HTTPS
	assert.True(t, strings.HasPrefix(metadata.Issuer, "https://"), "issuer must use HTTPS")

	// Verify endpoints are HTTPS
	assert.True(t, strings.HasPrefix(metadata.AuthorizationEndpoint, "https://"), "authorization_endpoint must use HTTPS")
	assert.True(t, strings.HasPrefix(metadata.TokenEndpoint, "https://"), "token_endpoint must use HTTPS")
}

// errorGrantRepository is a mock that returns a configurable error from FindByPrincipalAndAgent.
type errorGrantRepository struct {
	MockGrantRepository
	findErr error
}

func (r *errorGrantRepository) FindByPrincipalAndAgent(_ context.Context, _ id.Principal, _ id.AgentID) (*storage.UserGrant, error) {
	return nil, r.findErr
}

// TestService_HandleAuthorization_GrantLookupError verifies that when the grant repository
// returns a non-not-found error (e.g., connection failure) after redirect_uri is validated,
// the service returns a server_error decision with a redirect URL (safe to redirect because
// redirect_uri has already been verified).
func TestService_HandleAuthorization_GrantLookupError(t *testing.T) {
	agentID := id.NewAgentID()
	connErr := storage.NewStorageError("FindByPrincipalAndAgent", storage.ErrorKindConnection, nil, "connection refused")

	agentRepo := NewMockAgentRepository()
	_ = agentRepo.Create(context.Background(), &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		DisplayName:  "Test Agent",
		RedirectURIs: []string{"https://client.example.com/callback"},
	})

	grantRepo := &errorGrantRepository{
		MockGrantRepository: *NewMockGrantRepository(),
		findErr:             connErr,
	}

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
		PublicURL:    "https://broker.example.com",
		ModeStrategy: NewProxyModeStrategy(),
	}, nil, newTestSessionTokenService())

	req := &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		State:        "abc123",
		OriginalURL:  "https://broker.example.com/oauth2/authorize",
	}

	decision, err := svc.HandleAuthorization(context.Background(), req, id.NewPrincipal("user@example.com"))

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "error", decision.Action)
	assert.Equal(t, "server_error", decision.ErrorCode)
	assert.NotEmpty(t, decision.RedirectURL, "post-validation server_error should carry a redirect URL")
	assert.Contains(t, decision.RedirectURL, "https://client.example.com/callback")
	assert.Contains(t, decision.RedirectURL, "error=server_error")
}

// TestService_HandleAuthorization_MandatoryRequirements tests that step 5 of HandleAuthorization
// enforces mandatory service requirements on agents with active grants.
func TestService_HandleAuthorization_MandatoryRequirements(t *testing.T) {
	agentID := id.NewAgentID()
	serviceID := id.NewServiceID()

	cfg := &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
	}

	authReq := &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		OriginalURL:  "https://broker.example.com/oauth2/authorize",
	}

	// activeGrant sets up an agent with a mandatory service requirement and an active grant.
	// DelegatedOAuth2Tokens is empty so step 4 (session expiry) does not interfere.
	setupAgent := func(agentRepo *MockAgentRepository, scopeLess bool) {
		requiredScopes := []string{"repo", "user:email"}
		if scopeLess {
			requiredScopes = nil
		}
		_ = agentRepo.Create(context.Background(), &storage.Agent{
			ID:           agentID,
			ClientID:     ptr.To(id.ClientID("client-1")),
			DisplayName:  "Test Agent",
			RedirectURIs: []string{"https://client.example.com/callback"},
			ServiceRequirements: []storage.ServiceRequirement{
				{
					ServiceID:       serviceID,
					RequirementType: storage.RequirementTypeMandatory,
					RequiredScopes:  requiredScopes,
				},
			},
		})
	}

	setupGrant := func(grantRepo *MockGrantRepository) {
		_ = grantRepo.Create(context.Background(), &storage.UserGrant{
			ID:                    id.NewGrantID(),
			Principal:             id.Principal("user@example.com"),
			AgentID:               agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{},
		})
	}

	tests := []struct {
		name          string
		setupSess     func(*MockSessionRepository)
		wantAction    string
		wantErrorCode string
		scopeLess     bool
	}{
		{
			name: "active session with required scopes proceeds",
			setupSess: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return &storage.UserSession{
						ID:        id.NewSessionID(),
						Principal: id.Principal("user@example.com"),
						ServiceID: serviceID,
						Scope:     []string{"repo", "user:email", "read:user"},
					}, nil
				}
			},
			wantAction: "proceed",
		},
		{
			name: "storage error returns server error",
			setupSess: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return nil, storage.NewStorageError(
						"FindByPrincipalAndService",
						storage.ErrorKindConnection,
						nil,
						"database unavailable",
					)
				}
			},
			wantAction:    "error",
			wantErrorCode: "server_error",
		},
		{
			name: "missing session redirects to consent",
			setupSess: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return nil, nil
				}
			},
			wantAction: "redirect_to_consent",
		},
		{
			name:      "scope-less mandatory requirement without session redirects to consent",
			scopeLess: true,
			setupSess: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return nil, nil
				}
			},
			wantAction: "redirect_to_consent",
		},
		{
			name:      "scope-less mandatory requirement with active session proceeds",
			scopeLess: true,
			setupSess: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return &storage.UserSession{ID: id.NewSessionID(), Principal: id.Principal("user@example.com"), ServiceID: serviceID}, nil
				}
			},
			wantAction: "proceed",
		},
		{
			name: "session with insufficient scopes redirects to consent",
			setupSess: func(r *MockSessionRepository) {
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return &storage.UserSession{
						ID:        id.NewSessionID(),
						Principal: id.Principal("user@example.com"),
						ServiceID: serviceID,
						Scope:     []string{"repo"}, // missing user:email
					}, nil
				}
			},
			wantAction: "redirect_to_consent",
		},
		{
			name: "expired session redirects to consent",
			setupSess: func(r *MockSessionRepository) {
				expiredAt := time.Now().Add(-1 * time.Hour)
				r.findFunc = func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
					return &storage.UserSession{
						ID:                    id.NewSessionID(),
						Principal:             id.Principal("user@example.com"),
						ServiceID:             serviceID,
						Scope:                 []string{"repo", "user:email"},
						RefreshTokenExpiresAt: &expiredAt,
					}, nil
				}
			},
			wantAction: "redirect_to_consent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentRepo := NewMockAgentRepository()
			grantRepo := NewMockGrantRepository()
			sessionRepo := NewMockSessionRepository()

			setupAgent(agentRepo, tt.scopeLess)
			setupGrant(grantRepo)
			tt.setupSess(sessionRepo)

			svc := newTestAuthorizationServiceWithSessions(agentRepo, grantRepo, sessionRepo, cfg)
			decision, err := svc.HandleAuthorization(context.Background(), authReq, id.NewPrincipal("user@example.com"))

			require.NoError(t, err)
			require.NotNil(t, decision)
			assert.Equal(t, tt.wantAction, decision.Action)
			if tt.wantAction == "redirect_to_consent" {
				assert.Contains(t, decision.RedirectURL, "session_token=", "redirect_to_consent must include a session_token")
				assert.NotContains(t, decision.RedirectURL, "redirect_uri=", "redirect_to_consent must not fall back to redirect_uri")
			}
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, decision.ErrorCode)
			}
		})
	}
}

func TestService_HandleAuthorization_InvalidUpstreamAuthorizeURL(t *testing.T) {
	agentID := id.NewAgentID()

	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	require.NoError(t, agentRepo.Create(context.Background(), &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		DisplayName:  "Test Agent",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}))

	require.NoError(t, grantRepo.Create(context.Background(), &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{},
	}))

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
		UpstreamAuthorizeEndpoint: "%",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewProxyModeStrategy(),
	}, nil, newTestSessionTokenService())

	decision, err := svc.HandleAuthorization(context.Background(), &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		State:        "xyz",
		OriginalURL:  "https://broker.example.com/oauth2/authorize",
	}, id.NewPrincipal("user@example.com"))

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "error", decision.Action)
	assert.Equal(t, "server_error", decision.ErrorCode)
	assert.Contains(t, decision.RedirectURL, "error=server_error")
}

// TestService_HandleAuthorization_LocalClientInHybridMode verifies that a LocalClient
// agent (nil ClientID) in hybrid mode proceeds to local token issuance even when
// UpstreamAuthorizeEndpoint is configured (spec FR-008, scenario 2).
func TestService_HandleAuthorization_LocalClientInHybridMode(t *testing.T) {
	agentID := id.NewAgentID()

	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	// LocalClient: no ClientID, no ClientURIs — local token issuance.
	require.NoError(t, agentRepo.Create(context.Background(), &storage.Agent{
		ID:           agentID,
		ClientID:     nil,
		DisplayName:  "Local Agent",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}))
	require.NoError(t, grantRepo.Create(context.Background(), &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{},
	}))

	// Hybrid mode: UpstreamAuthorizeEndpoint set for proxy agents; local agents must not be blocked.
	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolver(agentRepo, nil), &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewHybridModeStrategy(),
	}, nil, newTestSessionTokenService())

	decision, err := svc.HandleAuthorization(context.Background(), &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		State:        "xyz",
		OriginalURL:  "https://broker.example.com/oauth2/authorize",
	}, id.NewPrincipal("user@example.com"))

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "proceed", decision.Action)
	assert.Equal(t, "", decision.RedirectURL, "local agents must not get an upstream redirect URL")
	assert.Equal(t, storage.LocalClient, decision.ClientType)
}

// TestService_HandleAuthorization_CIMDClientInHybridMode verifies that a CIMDClient
// agent (client_uris set, no ClientID) in hybrid mode proceeds to local token issuance
// even when UpstreamAuthorizeEndpoint is configured (spec FR-008, FR-009).
func TestService_HandleAuthorization_CIMDClientInHybridMode(t *testing.T) {
	agentID := id.NewAgentID()

	agentRepo := NewMockAgentRepository()
	grantRepo := NewMockGrantRepository()

	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     nil,
		ClientURIs:   []string{"https://cimd.example.com/agent.json"},
		DisplayName:  "CIMD Agent",
		RedirectURIs: []string{"https://cimd.example.com/callback"},
	}
	require.NoError(t, agentRepo.Create(context.Background(), agent))
	agentRepo.RegisterURI("https://cimd.example.com/agent.json", agent)
	require.NoError(t, grantRepo.Create(context.Background(), &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{},
	}))

	cimdBody := `{"client_id":"https://cimd.example.com/agent.json","client_name":"CIMD Agent","redirect_uris":["https://cimd.example.com/callback"]}`
	cimdFetch := &ports.CIMDFetchResult{Body: []byte(cimdBody), CacheControl: "max-age=300"}
	cimdSvc := cimdServiceForTest(t, cimdFetch, nil)

	svc := NewAuthorizationService(grantRepo, NewMockSessionRepository(), NewAgentClientResolverWithCIMD(agentRepo, cimdSvc, nil), &OAuth2Config{
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
		ModeStrategy:              NewHybridModeStrategy(),
	}, nil, newTestSessionTokenService())

	decision, err := svc.HandleAuthorization(context.Background(), &ports.AuthorizationRequest{
		ClientID:     id.ClientID("https://cimd.example.com/agent.json"),
		RedirectURI:  "https://cimd.example.com/callback",
		ResponseType: "code",
		State:        "xyz",
		OriginalURL:  "https://broker.example.com/oauth2/authorize",
	}, id.NewPrincipal("user@example.com"))

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "proceed", decision.Action)
	assert.Equal(t, "", decision.RedirectURL, "CIMD agents must not get an upstream redirect URL")
	assert.Equal(t, storage.CIMDClient, decision.ClientType)
}

// TestService_GenerateMetadata_TokenExchangeGrant verifies that the token-exchange
// grant type is included in discovery only when TokenExchangeEnabled is true, and
// is filtered out when false — even if manually present in SupportedGrantTypes.
func TestService_GenerateMetadata_TokenExchangeGrant(t *testing.T) {
	const tokenExchangeGrant = "urn:ietf:params:oauth:grant-type:token-exchange"
	baseGrants := []string{"authorization_code", "refresh_token"}

	t.Run("enabled — appended when absent from SupportedGrantTypes", func(t *testing.T) {
		svc := newTestAuthorizationService(NewMockAgentRepository(), NewMockGrantRepository(), &OAuth2Config{
			PublicURL:            "https://broker.example.com",
			ModeStrategy:         NewProxyModeStrategy(),
			SupportedGrantTypes:  baseGrants,
			TokenExchangeEnabled: true,
		})
		metadata, err := svc.GenerateMetadata(context.Background())
		require.NoError(t, err)
		assert.Contains(t, metadata.GrantTypesSupported, tokenExchangeGrant)
	})

	t.Run("enabled — not duplicated when already in SupportedGrantTypes", func(t *testing.T) {
		svc := newTestAuthorizationService(NewMockAgentRepository(), NewMockGrantRepository(), &OAuth2Config{
			PublicURL:            "https://broker.example.com",
			ModeStrategy:         NewProxyModeStrategy(),
			SupportedGrantTypes:  append(slices.Clone(baseGrants), tokenExchangeGrant),
			TokenExchangeEnabled: true,
		})
		metadata, err := svc.GenerateMetadata(context.Background())
		require.NoError(t, err)
		count := 0
		for _, g := range metadata.GrantTypesSupported {
			if g == tokenExchangeGrant {
				count++
			}
		}
		assert.Equal(t, 1, count, "token-exchange grant must not be duplicated")
	})

	t.Run("disabled — removed when manually present in SupportedGrantTypes", func(t *testing.T) {
		svc := newTestAuthorizationService(NewMockAgentRepository(), NewMockGrantRepository(), &OAuth2Config{
			PublicURL:            "https://broker.example.com",
			ModeStrategy:         NewProxyModeStrategy(),
			SupportedGrantTypes:  append(slices.Clone(baseGrants), tokenExchangeGrant),
			TokenExchangeEnabled: false,
		})
		metadata, err := svc.GenerateMetadata(context.Background())
		require.NoError(t, err)
		assert.NotContains(t, metadata.GrantTypesSupported, tokenExchangeGrant,
			"token-exchange grant must not appear in discovery when service is not wired")
	})

	t.Run("disabled — fallback to mode baseline when token-exchange is the only configured grant", func(t *testing.T) {
		svc := newTestAuthorizationService(NewMockAgentRepository(), NewMockGrantRepository(), &OAuth2Config{
			PublicURL:            "https://broker.example.com",
			ModeStrategy:         NewLocalModeStrategy(),
			SupportedGrantTypes:  []string{tokenExchangeGrant},
			TokenExchangeEnabled: false,
		})
		metadata, err := svc.GenerateMetadata(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, metadata.GrantTypesSupported, "discovery must never return an empty grant list")
		assert.NotContains(t, metadata.GrantTypesSupported, tokenExchangeGrant)
		assert.Contains(t, metadata.GrantTypesSupported, "authorization_code")
	})
}

// TestService_ResolveForTokenGrant_ModeBoundary verifies that ModeStrategy.AcceptsClientType
// is enforced on the token endpoint: a proxy-mode server must reject LocalClient agents and
// vice versa, while a nil strategy must accept all modes.
func TestService_ResolveForTokenGrant_ModeBoundary(t *testing.T) {
	ctx := context.Background()

	proxyAgent := &storage.Agent{
		ID:          id.NewAgentID(),
		DisplayName: "Proxy Agent",
		ClientID:    ptr.To(id.ClientID("upstream-client")),
	}
	localAgent := &storage.Agent{
		ID:          id.NewAgentID(),
		DisplayName: "Local Agent",
	}
	cimdAgent := &storage.Agent{
		ID:          id.NewAgentID(),
		DisplayName: "CIMD Agent",
		ClientURIs:  []string{"https://agent.example.com/client"},
	}

	buildSvc := func(strategy ModeStrategy, agents ...*storage.Agent) ports.OAuth2Service {
		repo := NewMockAgentRepository()
		for _, a := range agents {
			_ = repo.Create(ctx, a)
		}
		return newTestAuthorizationService(repo, NewMockGrantRepository(), &OAuth2Config{ModeStrategy: strategy})
	}

	cases := []struct {
		name        string
		strategy    ModeStrategy
		agent       *storage.Agent
		wantMode    storage.ClientType
		wantErrCode string
	}{
		{
			name:     "proxy-mode accepts ProxyClient",
			strategy: NewProxyModeStrategy(),
			agent:    proxyAgent,
			wantMode: storage.ProxyClient,
		},
		{
			name:        "proxy-mode rejects LocalClient",
			strategy:    NewProxyModeStrategy(),
			agent:       localAgent,
			wantErrCode: "unauthorized_client",
		},
		{
			// OpaqueClientResolver rejects CIMDClient agents by UUID before mode strategy
			// fires — the agent returns invalid_client, not unauthorized_client.
			name:        "proxy-mode: CIMD agent rejected by resolver before mode check",
			strategy:    NewProxyModeStrategy(),
			agent:       cimdAgent,
			wantErrCode: "invalid_client",
		},
		{
			name:     "local-mode accepts LocalClient",
			strategy: NewLocalModeStrategy(),
			agent:    localAgent,
			wantMode: storage.LocalClient,
		},
		{
			name:        "local-mode rejects ProxyClient",
			strategy:    NewLocalModeStrategy(),
			agent:       proxyAgent,
			wantErrCode: "unauthorized_client",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := buildSvc(tc.strategy, tc.agent)
			res, err := svc.ResolveForTokenGrant(ctx, id.ClientID(tc.agent.ID.String()))

			if tc.wantErrCode != "" {
				require.Error(t, err)
				var clientErr *ports.ClientIDError
				require.True(t, errors.As(err, &clientErr))
				assert.Equal(t, tc.wantErrCode, clientErr.Code)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, tc.wantMode, res.ClientType)
			}
		})
	}
}

// TestService_HandleAuthorization_ModeBoundary verifies that ModeStrategy.AcceptsClientType
// is enforced on the authorize endpoint, mirroring TestService_ResolveForTokenGrant_ModeBoundary.
func TestService_HandleAuthorization_ModeBoundary(t *testing.T) {
	ctx := context.Background()

	proxyAgent := &storage.Agent{
		ID:           id.NewAgentID(),
		DisplayName:  "Proxy Agent",
		Description:  "proxy",
		ClientID:     ptr.To(id.ClientID("upstream-client")),
		RedirectURIs: []string{"https://app.example.com/callback"},
	}
	localAgent := &storage.Agent{
		ID:           id.NewAgentID(),
		DisplayName:  "Local Agent",
		Description:  "local",
		RedirectURIs: []string{"https://app.example.com/callback"},
	}

	principal := id.NewPrincipal("user@example.com")

	buildSvc := func(strategy ModeStrategy, agents ...*storage.Agent) ports.OAuth2Service {
		repo := NewMockAgentRepository()
		grantRepo := NewMockGrantRepository()
		for _, a := range agents {
			_ = repo.Create(ctx, a)
			_ = grantRepo.Create(ctx, &storage.UserGrant{
				ID:        id.NewGrantID(),
				Principal: principal,
				AgentID:   a.ID,
			})
		}
		return newTestAuthorizationService(repo, grantRepo, &OAuth2Config{ModeStrategy: strategy})
	}

	authReq := func(agentID id.AgentID) *ports.AuthorizationRequest {
		return &ports.AuthorizationRequest{
			ClientID:     id.ClientID(agentID.String()),
			RedirectURI:  "https://app.example.com/callback",
			ResponseType: "code",
		}
	}

	cases := []struct {
		name        string
		strategy    ModeStrategy
		agent       *storage.Agent
		wantAction  string
		wantErrCode string
	}{
		{
			name:       "proxy-mode accepts ProxyClient",
			strategy:   NewProxyModeStrategy(),
			agent:      proxyAgent,
			wantAction: "proceed",
		},
		{
			name:        "proxy-mode rejects LocalClient",
			strategy:    NewProxyModeStrategy(),
			agent:       localAgent,
			wantAction:  "error",
			wantErrCode: "unauthorized_client",
		},
		{
			name:       "local-mode accepts LocalClient",
			strategy:   NewLocalModeStrategy(),
			agent:      localAgent,
			wantAction: "proceed",
		},
		{
			name:        "local-mode rejects ProxyClient",
			strategy:    NewLocalModeStrategy(),
			agent:       proxyAgent,
			wantAction:  "error",
			wantErrCode: "unauthorized_client",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := buildSvc(tc.strategy, tc.agent)
			decision, err := svc.HandleAuthorization(ctx, authReq(tc.agent.ID), principal)

			require.NoError(t, err)
			require.NotNil(t, decision)
			assert.Equal(t, tc.wantAction, decision.Action)
			if tc.wantErrCode != "" {
				assert.Equal(t, tc.wantErrCode, decision.ErrorCode)
			}
		})
	}
}

// TestBuildConsentURL_AlwaysProducesSessionToken verifies that buildConsentURL always
// generates a session_token URL regardless of whether cimdMeta is nil (T008).
func TestBuildConsentURL_AlwaysProducesSessionToken(t *testing.T) {
	agentID := id.NewAgentID()

	svc := NewAuthorizationService(
		NewMockGrantRepository(),
		NewMockSessionRepository(),
		NewAgentClientResolver(NewMockAgentRepository(), nil),
		&OAuth2Config{PublicURL: "https://broker.example.com", ModeStrategy: NewProxyModeStrategy()},
		nil,
		newTestSessionTokenService(),
	)

	req := &ports.AuthorizationRequest{
		ClientID:     id.ClientID(agentID.String()),
		RedirectURI:  "https://client.example.com/callback",
		ResponseType: "code",
		OriginalURL:  "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
	}
	agent := &storage.Agent{
		ID:           agentID,
		ClientID:     ptr.To(id.ClientID("client-1")),
		DisplayName:  "Test Agent",
		RedirectURIs: []string{"https://client.example.com/callback"},
	}

	// T008: nil cimdMeta must produce session_token URL (not redirect_uri fallback)
	consentURL, err := svc.buildConsentURL(context.Background(), req, id.NewPrincipal("user@example.com"), agent, nil)
	require.NoError(t, err)
	assert.Contains(t, consentURL, "session_token=", "buildConsentURL must always produce session_token")
	assert.NotContains(t, consentURL, "redirect_uri=", "buildConsentURL must never produce redirect_uri fallback")
	assert.True(t, strings.HasPrefix(consentURL, "https://broker.example.com/agents/"+agentID.String()+"?session_token="))
}
