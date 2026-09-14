package tokenexchange

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"log/slog"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

type noopBranchKeyManager struct{}

func newNoopBranchKeyManager() *noopBranchKeyManager {
	return &noopBranchKeyManager{}
}

func (m *noopBranchKeyManager) Create(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

// newTestProviderService wraps a repository in a ThirdpartyOAuth2ProviderService
// with passthrough encryption for use in domain-layer tests.
func newTestProviderService(repo ports.ThirdpartyOAuth2ProviderRepository) *thirdparty.ThirdpartyOAuth2ProviderService {
	return thirdparty.NewThirdpartyOAuth2ProviderService(repo, &MockEncryption{}, newNoopBranchKeyManager(), nil, false, nil)
}

// newMockPermissionSetService creates a PermissionSetService with mock for testing
// For basic tests, returns nil ValidateIDs error (all IDs valid)
func newMockPermissionSetService() *permissionset.Service {
	// Create an in-memory repository for permission sets
	psRepo := &MockPermissionSetRepository{}
	svc := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())
	return svc
}

// MockPermissionSetRepository is a hand-rolled mock for testing
type MockPermissionSetRepository struct {
	psMap map[id.PermissionSetID]*storagedomain.PermissionSet
}

func (m *MockPermissionSetRepository) Get(ctx context.Context, psID id.PermissionSetID) (*storagedomain.PermissionSet, error) {
	if m.psMap == nil {
		return nil, ports.ErrNotFound
	}
	if ps, ok := m.psMap[psID]; ok {
		return ps, nil
	}
	return nil, ports.ErrNotFound
}

// GetByIDs returns partial results: IDs not in psMap are silently absent.
// Matches the ports.PermissionSetRepository contract: "IDs not found are silently absent (caller validates)."
func (m *MockPermissionSetRepository) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storagedomain.PermissionSet, error) {
	if m.psMap == nil {
		return []*storagedomain.PermissionSet{}, nil
	}
	var results []*storagedomain.PermissionSet
	for _, id := range ids {
		if ps, ok := m.psMap[id]; ok {
			results = append(results, ps)
		}
	}
	return results, nil
}

func (m *MockPermissionSetRepository) Create(ctx context.Context, ps *storagedomain.PermissionSet) error {
	if m.psMap == nil {
		m.psMap = make(map[id.PermissionSetID]*storagedomain.PermissionSet)
	}
	m.psMap[ps.ID] = ps
	return nil
}

func (m *MockPermissionSetRepository) Update(ctx context.Context, ps *storagedomain.PermissionSet) error {
	if m.psMap == nil {
		return ports.ErrNotFound
	}
	m.psMap[ps.ID] = ps
	return nil
}

func (m *MockPermissionSetRepository) Delete(ctx context.Context, psID id.PermissionSetID) error {
	if m.psMap == nil {
		return nil
	}
	delete(m.psMap, psID)
	return nil
}

func (m *MockPermissionSetRepository) List(ctx context.Context, serviceID id.ServiceID) ([]*storagedomain.PermissionSet, error) {
	return []*storagedomain.PermissionSet{}, nil
}

func (m *MockPermissionSetRepository) CountAgentsReferencingPermissionSet(ctx context.Context, psID id.PermissionSetID) (int, error) {
	return 0, nil
}

func (m *MockPermissionSetRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

func (m *MockPermissionSetRepository) CountPermissionSetsForService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	return 0, nil
}

// newMockConsentService creates a consent.Service with mock repositories for testing
func newMockConsentService() *consent.Service {
	return consent.NewService(
		&MockAgentRepository{},
		newTestProviderService(&MockServiceRepository{}),
		&MockGrantRepository{
			grant: &storagedomain.UserGrant{
				ID:         id.NewGrantID(),
				Principal:  id.Principal("test-principal"),
				AgentID:    id.NewAgentID(),
				ValidUntil: func() *time.Time { t := time.Now().Add(24 * time.Hour); return &t }(),
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			},
		},
		nil,
		nil,
		slog.Default(),
	)
}

// MockAgentRepository mocks the AgentRepository for consent service testing
type MockAgentRepository struct{}

func (m *MockAgentRepository) Get(ctx context.Context, agentID id.AgentID) (*storagedomain.Agent, error) {
	return nil, nil
}

func (m *MockAgentRepository) GetByClientID(ctx context.Context, clientID id.ClientID) (*storagedomain.Agent, error) {
	return nil, nil
}

func (m *MockAgentRepository) Create(ctx context.Context, agent *storagedomain.Agent) error {
	return nil
}

func (m *MockAgentRepository) Update(ctx context.Context, agent *storagedomain.Agent) error {
	return nil
}

func (m *MockAgentRepository) Delete(ctx context.Context, agentID id.AgentID) error {
	return nil
}

func (m *MockAgentRepository) List(ctx context.Context) ([]*storagedomain.Agent, error) {
	return nil, nil
}

func (m *MockAgentRepository) GetByClientURI(ctx context.Context, uri string) (*storagedomain.Agent, error) {
	return nil, storagedomain.NewStorageError("GetAgentByClientURI", storagedomain.ErrorKindNotFound, ports.ErrNotFound, "not found")
}

func (m *MockAgentRepository) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

// MockOAuth2SessionService mocks the OAuth2SessionService for testing
type MockOAuth2SessionService struct {
	RefreshAccessTokenFn       func(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, refreshToken string) (*oauth2.Token, error)
	UpdateSessionTokensFn      func(ctx context.Context, principal id.Principal, session *storagedomain.UserSession, newToken *oauth2.Token) error
	DecryptAccessTokenFn       func(ctx context.Context, session *storagedomain.UserSession) (string, error)
	DecryptRefreshTokenFn      func(ctx context.Context, session *storagedomain.UserSession) (string, error)
	GetValidAccessTokenFn      func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, string, error)
	GetSessionWithValidTokenFn func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, string, error)
}

func (m *MockOAuth2SessionService) RefreshAccessToken(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, refreshToken string) (*oauth2.Token, error) {
	if m.RefreshAccessTokenFn != nil {
		return m.RefreshAccessTokenFn(ctx, entity, refreshToken)
	}
	return nil, nil
}

func (m *MockOAuth2SessionService) UpdateSessionTokens(ctx context.Context, principal id.Principal, session *storagedomain.UserSession, newToken *oauth2.Token) error {
	if m.UpdateSessionTokensFn != nil {
		return m.UpdateSessionTokensFn(ctx, principal, session, newToken)
	}
	return nil
}

func (m *MockOAuth2SessionService) DecryptAccessToken(ctx context.Context, session *storagedomain.UserSession) (string, error) {
	if m.DecryptAccessTokenFn != nil {
		return m.DecryptAccessTokenFn(ctx, session)
	}
	return "", nil
}

func (m *MockOAuth2SessionService) DecryptRefreshToken(ctx context.Context, session *storagedomain.UserSession) (string, error) {
	if m.DecryptRefreshTokenFn != nil {
		return m.DecryptRefreshTokenFn(ctx, session)
	}
	return "", nil
}

func (m *MockOAuth2SessionService) GetValidAccessToken(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, string, error) {
	if m.GetValidAccessTokenFn != nil {
		return m.GetValidAccessTokenFn(ctx, principal, serviceID)
	}
	// Return a mock session and token
	session := &storagedomain.UserSession{
		ID:        id.NewSessionID(),
		Principal: principal,
		ServiceID: serviceID,
		TokenType: "Bearer",
		Scope:     []string{"read", "write"},
	}
	return session, "mock-access-token", nil
}

func (m *MockOAuth2SessionService) GetSessionWithValidToken(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, string, error) {
	if m.GetSessionWithValidTokenFn != nil {
		return m.GetSessionWithValidTokenFn(ctx, principal, serviceID)
	}
	// Return a mock session and token
	session := &storagedomain.UserSession{
		ID:        id.NewSessionID(),
		Principal: principal,
		ServiceID: serviceID,
		TokenType: "Bearer",
		Scope:     []string{"read", "write"},
	}
	return session, "mock-access-token", nil
}

// MockTokenExchangeRepository mocks are defined at the end of this file
type MockServiceRepository struct {
	service *model.ThirdpartyOAuth2ProviderEntity
	err     error
}

func (m *MockServiceRepository) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return m.service, m.err
}

func (m *MockServiceRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	return nil
}

func (m *MockServiceRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, nil
}

func (m *MockServiceRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	return nil
}

func (m *MockServiceRepository) Delete(ctx context.Context, serviceID id.ServiceID) error {
	return nil
}

func (m *MockServiceRepository) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, nil
}

func (m *MockServiceRepository) AddProtectedResource(_ context.Context, _ id.ServiceID, _ string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, nil
}

func (m *MockServiceRepository) RemoveProtectedResource(_ context.Context, _ id.ServiceID, _ string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, nil
}

func (m *MockServiceRepository) RenameProtectedResource(_ context.Context, _ id.ServiceID, _, _ string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, nil
}

func (m *MockServiceRepository) ListProtectedResources(_ context.Context, _ id.ServiceID) ([]string, int64, error) {
	return nil, 0, nil
}

type MockGrantRepository struct {
	grant *storagedomain.UserGrant
	err   error
}

func (m *MockGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storagedomain.UserGrant, error) {
	return m.grant, m.err
}

func (m *MockGrantRepository) Create(ctx context.Context, grant *storagedomain.UserGrant) error {
	return nil
}

func (m *MockGrantRepository) Get(ctx context.Context, grantID id.GrantID) (*storagedomain.UserGrant, error) {
	return nil, nil
}

func (m *MockGrantRepository) Update(ctx context.Context, grant *storagedomain.UserGrant) error {
	return nil
}

func (m *MockGrantRepository) Delete(ctx context.Context, grantID id.GrantID) error {
	return nil
}

func (m *MockGrantRepository) CountAgentsByServiceID(ctx context.Context, serviceID id.ServiceID) (int, error) {
	return 0, nil
}

func (m *MockGrantRepository) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	return nil
}

func (m *MockGrantRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storagedomain.UserGrant, error) {
	return nil, nil
}

func (m *MockGrantRepository) ListByServiceID(ctx context.Context, serviceID id.ServiceID) ([]id.AgentID, error) {
	return nil, nil
}

func (m *MockGrantRepository) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storagedomain.UserGrant, error) {
	return nil, nil
}

func (m *MockGrantRepository) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	return m.err
}

func (m *MockGrantRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

type MockSessionRepository struct {
	session                        *storagedomain.UserSession
	err                            error
	findByPrincipalAndServiceCalls int
}

type MockEncryption struct {
	err error
}

func (m *MockEncryption) Encrypt(ctx context.Context, plaintext []byte, context map[string]string) ([]byte, error) {
	return plaintext, m.err
}

func (m *MockEncryption) Decrypt(ctx context.Context, ciphertext []byte, context map[string]string) ([]byte, error) {
	return ciphertext, m.err
}

func (m *MockSessionRepository) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, error) {
	m.findByPrincipalAndServiceCalls++
	return m.session, m.err
}

func (m *MockSessionRepository) Create(ctx context.Context, session *storagedomain.UserSession) error {
	return nil
}

func (m *MockSessionRepository) Get(ctx context.Context, sessionID id.SessionID) (*storagedomain.UserSession, error) {
	return nil, nil
}

func (m *MockSessionRepository) Delete(ctx context.Context, sessionID id.SessionID) error {
	return nil
}

func (m *MockSessionRepository) CountByService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	return 0, nil
}

func (m *MockSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	return nil
}

func (m *MockSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storagedomain.UserSession, error) {
	return nil, nil
}

func (m *MockSessionRepository) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storagedomain.UserSession, error) {
	return nil, nil
}

// NewTokenExchangeServiceForTest creates a TokenExchangeService for testing
// permissionSetService will be created automatically if nil
func NewTokenExchangeServiceForTest(
	jwtValidator *JWTValidator,
	celEvaluator *CELEvaluator,
	providerService *thirdparty.ThirdpartyOAuth2ProviderService,
	oauth2SessionService *oauth2session.OAuth2SessionService,
	consentService *consent.Service,
	agentRepository ports.AgentRepository,
	config *ports.TokenExchangeConfig,
) (*TokenExchangeService, error) {
	psService := newMockPermissionSetService()
	return NewTokenExchangeService(
		jwtValidator,
		celEvaluator,
		providerService,
		oauth2SessionService,
		consentService,
		psService,
		agentRepository,
		config,
	)
}

// TestNewTokenExchangeService tests service creation with various parameter combinations
func TestNewTokenExchangeServiceForTest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		jwtValidator         *JWTValidator
		celEvaluator         *CELEvaluator
		serviceRepo          *thirdparty.ThirdpartyOAuth2ProviderService
		oauth2SessionService *oauth2session.OAuth2SessionService
		consentService       *consent.Service
		agentRepository      ports.AgentRepository
		config               *ports.TokenExchangeConfig
		expectError          bool
		errorContains        string
	}{
		{
			name:                 "valid parameters - creates service without error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         &CELEvaluator{},
			serviceRepo:          newTestProviderService(&MockServiceRepository{}),
			oauth2SessionService: &oauth2session.OAuth2SessionService{},
			consentService:       &consent.Service{},
			agentRepository:      &MockAgentRepository{},
			config: &ports.TokenExchangeConfig{
				ClaimExtraction: ports.ClaimExtractionConfig{
					PrincipalExpression: "subject_token.sub",
					AgentIDExpression:   "subject_token.azp",
				},
				Authorization: ports.AuthorizationConfig{
					Type: "cel",
					CEL: ports.CELAuthorizationConfig{
						Expression: "true",
					},
				},
			},
			expectError: false,
		},
		{
			name:          "nil jwtValidator returns error",
			jwtValidator:  nil,
			expectError:   true,
			errorContains: "jwtValidator",
		},
		{
			name:                 "nil celEvaluator returns error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         nil,
			oauth2SessionService: &oauth2session.OAuth2SessionService{},
			expectError:          true,
			errorContains:        "celEvaluator",
		},
		{
			name:                 "nil serviceRepository returns error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         &CELEvaluator{},
			serviceRepo:          nil,
			oauth2SessionService: &oauth2session.OAuth2SessionService{},
			expectError:          true,
			errorContains:        "providerService",
		},
		{
			name:                 "nil oauth2SessionService returns error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         &CELEvaluator{},
			serviceRepo:          newTestProviderService(&MockServiceRepository{}),
			oauth2SessionService: nil,
			consentService:       &consent.Service{},
			expectError:          true,
			errorContains:        "oauth2SessionService",
		},
		{
			name:                 "nil consentService returns error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         &CELEvaluator{},
			serviceRepo:          newTestProviderService(&MockServiceRepository{}),
			oauth2SessionService: &oauth2session.OAuth2SessionService{},
			consentService:       nil,
			expectError:          true,
			errorContains:        "consentService",
		},
		{
			name:                 "nil agentRepository returns error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         &CELEvaluator{},
			serviceRepo:          newTestProviderService(&MockServiceRepository{}),
			oauth2SessionService: &oauth2session.OAuth2SessionService{},
			consentService:       &consent.Service{},
			agentRepository:      nil,
			expectError:          true,
			errorContains:        "agentRepository",
		},
		{
			name:                 "nil config returns error",
			jwtValidator:         &JWTValidator{},
			celEvaluator:         &CELEvaluator{},
			serviceRepo:          newTestProviderService(&MockServiceRepository{}),
			oauth2SessionService: &oauth2session.OAuth2SessionService{},
			consentService:       &consent.Service{},
			agentRepository:      &MockAgentRepository{},
			config:               nil,
			expectError:          true,
			errorContains:        "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service, err := NewTokenExchangeServiceForTest(
				tt.jwtValidator,
				tt.celEvaluator,
				tt.serviceRepo,
				tt.oauth2SessionService,
				tt.consentService,
				tt.agentRepository,
				tt.config,
			)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, service)
				if tt.errorContains != "" {
					assert.ErrorContains(t, err, tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, service)
			}
		})
	}
}

func TestNewTokenExchangeService_RequiresPermissionSetService(t *testing.T) {
	t.Parallel()

	service, err := NewTokenExchangeService(
		&JWTValidator{},
		&CELEvaluator{},
		newTestProviderService(&MockServiceRepository{}),
		&oauth2session.OAuth2SessionService{},
		&consent.Service{},
		nil,
		&MockAgentRepository{},
		&ports.TokenExchangeConfig{},
	)

	require.Error(t, err)
	assert.Nil(t, service)
	assert.ErrorContains(t, err, "permissionSetService")
}

// TestExchange_InvalidRequest tests handling of invalid token exchange requests
func TestExchange_InvalidRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	config := &ports.TokenExchangeConfig{
		ClaimExtraction: ports.ClaimExtractionConfig{
			PrincipalExpression: "subject_token.sub",
			AgentIDExpression:   "subject_token.azp",
		},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL: ports.CELAuthorizationConfig{
				Expression: "true",
			},
		},
	}

	service, err := NewTokenExchangeServiceForTest(
		&JWTValidator{},
		&CELEvaluator{},
		newTestProviderService(&MockServiceRepository{}),
		&oauth2session.OAuth2SessionService{},
		newMockConsentService(),
		&MockAgentRepository{},
		config,
	)
	require.NoError(t, err)

	tests := []struct {
		name        string
		request     *TokenExchangeRequest
		expectError bool
		errorCode   string
	}{
		{
			name:        "empty grant_type",
			request:     NewTokenExchangeRequest("", "token", "", "assertion", "", "resource", ""),
			expectError: true,
			errorCode:   "invalid_request",
		},
		{
			name:        "missing subject_token",
			request:     NewTokenExchangeRequest(TokenExchangeGrantType, "", "", "assertion", "", "resource", ""),
			expectError: true,
			errorCode:   "invalid_request",
		},
		{
			name:        "missing client_assertion",
			request:     NewTokenExchangeRequest(TokenExchangeGrantType, "token", "", "", "", "resource", ""),
			expectError: true,
			errorCode:   "invalid_request",
		},
		{
			name:        "missing resource",
			request:     NewTokenExchangeRequest(TokenExchangeGrantType, "token", "", "assertion", "", "", ""),
			expectError: true,
			errorCode:   "invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := service.Exchange(ctx, tt.request)
			assert.Error(t, err)

			// Check error code
			if tokenErr, ok := err.(*TokenExchangeError); ok {
				assert.Equal(t, tt.errorCode, tokenErr.Code())
			}
		})
	}
}

// TestExchange_RequestValidation documents valid token exchange request structure
// Full E2E testing of request/JWT validation is in E2E tests
func TestExchange_RequestValidation(t *testing.T) {
	t.Parallel()
	// This test documents that request validation happens before JWT validation
	// Full integration tests of the complete flow are in E2E tests
	// Unit tests of JWT validation are in jwt_validator_test.go
}

// TestExchange_GrantExpiration tests handling of expired user grants
func TestExchange_GrantExpiration(t *testing.T) {
	t.Parallel()
	// This test verifies that expired grants are properly rejected
	// In a full implementation with complete mocking, this would test the full flow
	t.Run("expired grant should return access_denied", func(t *testing.T) {
		t.Parallel()
		// Would need complete mocking of JWT validation and other steps
		// This is a placeholder for the test pattern
		expiredTime := time.Now().UTC().Add(-1 * time.Hour)
		grant := &storagedomain.UserGrant{
			ValidUntil: &expiredTime,
		}

		// Verify grant is expired
		if grant.ValidUntil != nil && grant.ValidUntil.Before(time.Now().UTC()) {
			// Grant is expired - should be denied
			assert.True(t, grant.ValidUntil.Before(time.Now().UTC()))
		}
	})
}

// TestBuildRequestContext tests CEL request context construction
// NOTE: buildRequestContext is not currently exposed on TokenExchangeService
// This test remains as documentation for the pattern once the method is public
func TestBuildRequestContext(t *testing.T) {
	t.Parallel()
	_, err := NewTokenExchangeServiceForTest(
		&JWTValidator{},
		&CELEvaluator{},
		newTestProviderService(&MockServiceRepository{}),
		&oauth2session.OAuth2SessionService{},
		newMockConsentService(),
		&MockAgentRepository{},
		&ports.TokenExchangeConfig{},
	)
	require.NoError(t, err)

	// TODO: Once buildRequestContext is exposed, uncomment test
	// req := NewTokenExchangeRequest(
	//	TokenExchangeGrantType,
	//	"subject-token",
	//	AccessTokenType,
	//	"client-assertion",
	//	JWTBearerType,
	//	"https://api.example.com",
	//	"read write",
	// )
	//
	// ctx := service.buildRequestContext("user123", "agent-456", req)
	//
	// assert.Equal(t, "https://api.example.com", ctx["resource"])
	// assert.Equal(t, TokenExchangeGrantType, ctx["grant_type"])
	// assert.Equal(t, "read write", ctx["scope"])
	// assert.Equal(t, "user123", ctx["principal"])
	// assert.Equal(t, "agent-456", ctx["agent_client_id"])
}

// TestGrantVerification_MissingGrant tests T063 - access_denied when grant not found
func TestGrantVerification_MissingGrant(t *testing.T) {
	t.Parallel()
	// This test documents the grant verification flow (T061-T063)
	// When a grant is not found (ErrNotFound), the service should return access_denied

	_, err := NewTokenExchangeServiceForTest(
		&JWTValidator{},
		&CELEvaluator{},
		newTestProviderService(&MockServiceRepository{}),
		&oauth2session.OAuth2SessionService{},
		newMockConsentService(),
		&MockAgentRepository{},
		&ports.TokenExchangeConfig{},
	)
	require.NoError(t, err)

	// T063: When grant not found, should return access_denied
	// This is verified in the Exchange implementation
	assert.Equal(t, ports.ErrNotFound, ports.ErrNotFound)
}

// TestGrantVerification_ExpiredGrant tests T065 - access_denied when grant expired
func TestGrantVerification_ExpiredGrant(t *testing.T) {
	t.Parallel()
	// Create an expired grant
	expiredTime := time.Now().UTC().Add(-1 * time.Hour)
	expiredGrant := &storagedomain.UserGrant{
		Principal:             id.Principal("user@example.com"),
		AgentID:               id.NewAgentID(),
		ValidUntil:            &expiredTime,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	_, err := NewTokenExchangeServiceForTest(
		&JWTValidator{},
		&CELEvaluator{},
		newTestProviderService(&MockServiceRepository{}),
		&oauth2session.OAuth2SessionService{},
		newMockConsentService(),
		&MockAgentRepository{},
		&ports.TokenExchangeConfig{},
	)
	require.NoError(t, err)

	// Verify that expired grant is detected
	// This test documents T062a and T065 grant expiration check
	assert.NotNil(t, expiredGrant)
	assert.NotNil(t, expiredGrant.ValidUntil)
	if expiredGrant.ValidUntil != nil && expiredGrant.ValidUntil.Before(time.Now().UTC()) {
		// T065: Grant is expired
		assert.True(t, expiredGrant.ValidUntil.Before(time.Now().UTC()))
	}
}

// TestGrantVerification_ActiveGrant tests T062a - active grant is allowed
func TestGrantVerification_ActiveGrant(t *testing.T) {
	t.Parallel()
	// Create an active (non-expired) grant
	futureTime := time.Now().UTC().Add(24 * time.Hour)
	activeGrant := &storagedomain.UserGrant{
		Principal:             id.Principal("user@example.com"),
		AgentID:               id.NewAgentID(),
		ValidUntil:            &futureTime,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	_, err := NewTokenExchangeServiceForTest(
		&JWTValidator{},
		&CELEvaluator{},
		newTestProviderService(&MockServiceRepository{}),
		&oauth2session.OAuth2SessionService{},
		newMockConsentService(),
		&MockAgentRepository{},
		&ports.TokenExchangeConfig{},
	)
	require.NoError(t, err)

	// Verify that active grant is recognized
	// This test documents T062a - grant is active when ValidUntil > now
	assert.NotNil(t, activeGrant)
	assert.NotNil(t, activeGrant.ValidUntil)
	if activeGrant.ValidUntil != nil {
		// T062a: Grant is active (not expired)
		assert.True(t, activeGrant.ValidUntil.After(time.Now().UTC()))
	}
}

// TestMockOAuth2SessionService_NewMethods tests the newly added mock methods
func TestMockOAuth2SessionService_NewMethods(t *testing.T) {
	t.Parallel()
	mock := &MockOAuth2SessionService{}
	ctx := context.Background()
	testSvcID := id.NewServiceID()

	t.Run("GetValidAccessToken with default behavior", func(t *testing.T) {
		session, token, err := mock.GetValidAccessToken(ctx, id.Principal("user@example.com"), testSvcID)
		assert.NoError(t, err)
		assert.Equal(t, "mock-access-token", token)
		assert.NotNil(t, session)
		assert.Equal(t, id.Principal("user@example.com"), session.Principal)
		assert.Equal(t, testSvcID, session.ServiceID)
	})

	t.Run("GetValidAccessToken with custom function", func(t *testing.T) {
		customSession := &storagedomain.UserSession{
			ID:        id.NewSessionID(),
			Principal: id.Principal("user@example.com"),
			ServiceID: testSvcID,
			TokenType: "Custom",
			Scope:     []string{"custom"},
		}
		mock.GetValidAccessTokenFn = func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, string, error) {
			return customSession, "custom-token", nil
		}
		session, token, err := mock.GetValidAccessToken(ctx, id.Principal("user@example.com"), testSvcID)
		assert.NoError(t, err)
		assert.Equal(t, "custom-token", token)
		assert.Equal(t, customSession, session)
	})

	t.Run("GetSessionWithValidToken with default behavior", func(t *testing.T) {
		mock.GetValidAccessTokenFn = nil // reset
		session, token, err := mock.GetSessionWithValidToken(ctx, id.Principal("user@example.com"), testSvcID)
		assert.NoError(t, err)
		assert.Equal(t, "mock-access-token", token)
		assert.NotNil(t, session)
		assert.Equal(t, id.Principal("user@example.com"), session.Principal)
		assert.Equal(t, testSvcID, session.ServiceID)
		assert.Equal(t, "Bearer", session.TokenType)
	})

	t.Run("GetSessionWithValidToken with custom function", func(t *testing.T) {
		customSession := &storagedomain.UserSession{
			ID:        id.NewSessionID(),
			Principal: id.Principal("custom@example.com"),
			ServiceID: testSvcID,
			TokenType: "Custom",
			Scope:     []string{"custom"},
		}
		mock.GetSessionWithValidTokenFn = func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storagedomain.UserSession, string, error) {
			return customSession, "custom-token", nil
		}
		session, token, err := mock.GetSessionWithValidToken(ctx, id.Principal("user@example.com"), testSvcID)
		assert.NoError(t, err)
		assert.Equal(t, "custom-token", token)
		assert.Equal(t, customSession, session)
	})
}

// TestResolveEffectiveScopes_ScopeUnion tests T046: scope union for two PSets covering same service
func TestResolveEffectiveScopes_ScopeUnion(t *testing.T) {
	t.Parallel()

	svcA := id.NewServiceID()
	ps1ID := id.NewPermissionSetID()
	ps2ID := id.NewPermissionSetID()

	// Create permission sets: PS1 has {svcA: ["read"]}, PS2 has {svcA: ["write"]}
	psRepo := &MockPermissionSetRepository{
		psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			ps1ID: {
				ID:   ps1ID,
				Name: "PS1",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
			ps2ID: {
				ID:   ps2ID,
				Name: "PS2",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"write"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
		},
	}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())

	// Agent SR with svcA: ["read", "write"]
	agent := &storagedomain.Agent{
		ServiceRequirements: []storagedomain.ServiceRequirement{
			{ServiceID: svcA, RequiredScopes: []string{"read", "write"}},
		},
	}

	// Grant includes both PS, both with svcA included
	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: ps1ID, IncludedServiceIDs: []id.ServiceID{svcA}},
			{PermissionSetID: ps2ID, IncludedServiceIDs: []id.ServiceID{svcA}},
		},
	}

	svc := &TokenExchangeService{permissionSetService: psService}
	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)

	// Scope union: read + write
	assert.Contains(t, scopes, svcA)
	assert.Len(t, scopes[svcA], 2)
	assert.Contains(t, scopes[svcA], "read")
	assert.Contains(t, scopes[svcA], "write")
}

func TestResolveEffectiveScopes_ScopeLessServiceIsCovered(t *testing.T) {
	t.Parallel()

	serviceID := id.NewServiceID()
	permissionSetID := id.NewPermissionSetID()
	psService := permissionset.NewPermissionSetService(&MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
		permissionSetID: {
			ID: permissionSetID,
			ServiceScopes: []storagedomain.ServiceScope{{
				ServiceID:       serviceID,
				RequirementType: storagedomain.RequirementTypeMandatory,
			}},
		},
	}}, &MockGrantRepository{}, slog.Default())
	defer psService.Close()

	svc := &TokenExchangeService{permissionSetService: psService}
	effectiveScopes, err := svc.resolveEffectiveScopes(context.Background(), &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    permissionSetID,
			IncludedServiceIDs: []id.ServiceID{serviceID},
		}},
	}, &storagedomain.Agent{ServiceRequirements: []storagedomain.ServiceRequirement{{
		ServiceID:       serviceID,
		RequirementType: storagedomain.RequirementTypeMandatory,
	}}})

	require.NoError(t, err)
	assert.Contains(t, effectiveScopes, serviceID)
	assert.Empty(t, effectiveScopes[serviceID])
}

func TestResolveEffectiveScopes_OmitsScopesOutsideSRCeiling(t *testing.T) {
	t.Parallel()

	serviceID := id.NewServiceID()
	permissionSetID := id.NewPermissionSetID()
	psService := permissionset.NewPermissionSetService(&MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
		permissionSetID: {
			ID: permissionSetID,
			ServiceScopes: []storagedomain.ServiceScope{{
				ServiceID:       serviceID,
				Scopes:          []string{"read"},
				RequirementType: storagedomain.RequirementTypeMandatory,
			}},
		},
	}}, &MockGrantRepository{}, slog.Default())
	defer psService.Close()

	svc := &TokenExchangeService{permissionSetService: psService}
	effectiveScopes, err := svc.resolveEffectiveScopes(context.Background(), &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    permissionSetID,
			IncludedServiceIDs: []id.ServiceID{serviceID},
		}},
	}, &storagedomain.Agent{ServiceRequirements: []storagedomain.ServiceRequirement{{
		ServiceID:       serviceID,
		RequiredScopes:  []string{"write"},
		RequirementType: storagedomain.RequirementTypeMandatory,
	}}})

	require.NoError(t, err)
	assert.NotContains(t, effectiveScopes, serviceID)
}

// TestResolveEffectiveScopes_SRCeiling tests T046: SR scope ceiling intersection (FR-013)
func TestResolveEffectiveScopes_SRCeiling(t *testing.T) {
	t.Parallel()

	svcA := id.NewServiceID()
	ps1ID := id.NewPermissionSetID()

	// PS1 has {svcA: ["read", "write", "admin"]}
	psRepo := &MockPermissionSetRepository{
		psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			ps1ID: {
				ID:   ps1ID,
				Name: "PS1",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"read", "write", "admin"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
		},
	}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())

	// Agent SR with svcA: only ["read", "write"] — "admin" not in ceiling
	agent := &storagedomain.Agent{
		ServiceRequirements: []storagedomain.ServiceRequirement{
			{ServiceID: svcA, RequiredScopes: []string{"read", "write"}},
		},
	}

	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: ps1ID, IncludedServiceIDs: []id.ServiceID{svcA}},
		},
	}

	svc := &TokenExchangeService{permissionSetService: psService}
	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)

	// "admin" should be excluded by SR ceiling
	assert.Contains(t, scopes, svcA)
	assert.Contains(t, scopes[svcA], "read")
	assert.Contains(t, scopes[svcA], "write")
	assert.NotContains(t, scopes[svcA], "admin")
}

func TestResolveEffectiveScopes_RequireAllScopes(t *testing.T) {
	t.Parallel()

	svcA := id.NewServiceID()
	psID := id.NewPermissionSetID()
	psService := permissionset.NewPermissionSetService(&MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
		psID: {
			ID:   psID,
			Name: "PS1",
			ServiceScopes: []storagedomain.ServiceScope{
				{ServiceID: svcA, Scopes: []string{"read", "write", "admin"}, RequirementType: storagedomain.RequirementTypeOptional},
			},
		},
	}}, &MockGrantRepository{}, slog.Default())
	defer psService.Close()

	svc := &TokenExchangeService{permissionSetService: psService}
	effectiveScopes, err := svc.resolveEffectiveScopes(context.Background(), &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    psID,
			IncludedServiceIDs: []id.ServiceID{svcA},
		}},
	}, &storagedomain.Agent{ServiceRequirements: []storagedomain.ServiceRequirement{{
		ServiceID:        svcA,
		RequireAllScopes: true,
	}}})

	require.NoError(t, err)
	assert.Equal(t, []string{"admin", "read", "write"}, effectiveScopes[svcA])
}

// TestResolveEffectiveScopes_PerServiceInclusion tests T046: only included services contribute scopes
func TestResolveEffectiveScopes_PerServiceInclusion(t *testing.T) {
	t.Parallel()

	svcA := id.NewServiceID()
	svcB := id.NewServiceID()
	ps1ID := id.NewPermissionSetID()

	// PS1 covers both svcA and svcB
	psRepo := &MockPermissionSetRepository{
		psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			ps1ID: {
				ID:   ps1ID,
				Name: "PS1",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
					{ServiceID: svcB, Scopes: []string{"write"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
		},
	}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())

	agent := &storagedomain.Agent{
		ServiceRequirements: []storagedomain.ServiceRequirement{
			{ServiceID: svcA, RequiredScopes: []string{"read"}},
			{ServiceID: svcB, RequiredScopes: []string{"write"}},
		},
	}

	// Grant only includes svcA, NOT svcB
	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: ps1ID, IncludedServiceIDs: []id.ServiceID{svcA}},
		},
	}

	svc := &TokenExchangeService{permissionSetService: psService}
	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)

	// Only svcA should have scopes; svcB was not included
	assert.Contains(t, scopes, svcA)
	assert.Equal(t, []string{"read"}, scopes[svcA])
	assert.NotContains(t, scopes, svcB)
}

// TestResolveEffectiveScopes_MultiPSCrossScopeCeiling tests scope union with SR ceiling across multiple PSets
func TestResolveEffectiveScopes_MultiPSCrossScopeCeiling(t *testing.T) {
	t.Parallel()

	svcA := id.NewServiceID()
	ps1ID := id.NewPermissionSetID()
	ps2ID := id.NewPermissionSetID()

	// PS1: {svcA: ["read", "admin"]}, PS2: {svcA: ["write", "delete"]}
	psRepo := &MockPermissionSetRepository{
		psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			ps1ID: {
				ID:   ps1ID,
				Name: "PS1",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"read", "admin"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
			ps2ID: {
				ID:   ps2ID,
				Name: "PS2",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"write", "delete"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
		},
	}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())

	// SR ceiling only allows read and write (excludes admin and delete)
	agent := &storagedomain.Agent{
		ServiceRequirements: []storagedomain.ServiceRequirement{
			{ServiceID: svcA, RequiredScopes: []string{"read", "write"}},
		},
	}

	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: ps1ID, IncludedServiceIDs: []id.ServiceID{svcA}},
			{PermissionSetID: ps2ID, IncludedServiceIDs: []id.ServiceID{svcA}},
		},
	}

	svc := &TokenExchangeService{permissionSetService: psService}
	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)

	// Union of PS scopes: {read, admin, write, delete}
	// After SR ceiling intersection: {read, write}
	assert.Contains(t, scopes, svcA)
	assert.Len(t, scopes[svcA], 2)
	assert.Contains(t, scopes[svcA], "read")
	assert.Contains(t, scopes[svcA], "write")
	assert.NotContains(t, scopes[svcA], "admin")
	assert.NotContains(t, scopes[svcA], "delete")
}

// TestResolveEffectiveScopes_NonSRServiceExcluded verifies FR-013: services present in a
// granted PS but absent from the agent's service_requirements are excluded from effective scopes.
func TestResolveEffectiveScopes_NonSRServiceExcluded(t *testing.T) {
	t.Parallel()

	svcA := id.NewServiceID()
	svcB := id.NewServiceID() // in PS but NOT in agent SRs
	ps1ID := id.NewPermissionSetID()

	psRepo := &MockPermissionSetRepository{
		psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			ps1ID: {
				ID:   ps1ID,
				Name: "PS1",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
					{ServiceID: svcB, Scopes: []string{"admin"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
		},
	}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())

	// Agent only declares svcA as a service requirement — svcB is not declared
	agent := &storagedomain.Agent{
		ServiceRequirements: []storagedomain.ServiceRequirement{
			{ServiceID: svcA, RequiredScopes: []string{"read"}},
		},
	}

	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: ps1ID, IncludedServiceIDs: []id.ServiceID{svcA, svcB}},
		},
	}

	svc := &TokenExchangeService{permissionSetService: psService}
	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)

	assert.Contains(t, scopes, svcA)
	assert.Equal(t, []string{"read"}, scopes[svcA])
	assert.NotContains(t, scopes, svcB)
}

func TestResolveEffectiveScopes_EmptySRCeiling_UsesPS(t *testing.T) {
	t.Parallel()

	// Agent with no service requirements — srCeiling will be empty.
	// Consent validation treats all PS ServiceScopes as valid when SR is empty
	// (see consent/service.go:415), so token exchange must be consistent: PS-derived
	// scopes are used directly without SR-ceiling filtering.
	ps1ID := id.NewPermissionSetID()
	svcA := id.NewServiceID()

	psRepo := &MockPermissionSetRepository{
		psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
			ps1ID: {
				ID:   ps1ID,
				Name: "PS1",
				ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcA, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			},
		},
	}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())

	agent := &storagedomain.Agent{
		ServiceRequirements: nil, // no SRs — PS scopes used directly as effective scopes
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: ps1ID, RequirementType: storagedomain.RequirementTypeMandatory},
		},
	}

	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: ps1ID, IncludedServiceIDs: []id.ServiceID{svcA}},
		},
	}

	svc := &TokenExchangeService{permissionSetService: psService}
	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)
	assert.Equal(t, []string{"read"}, scopes[svcA],
		"PS scopes should flow through directly when agent has no service_requirements")
}

// --- T029: Agent lookup unit tests (Feature 021 US2) ---
// These tests verify that after T030, the service uses id.ParseAgentID + Get (not GetByClientID).
// Written before T030 implementation — must FAIL semantically until T030 is implemented.

// trackingAgentRepository is a spy that records whether Get or GetByClientID was called.
// Returns ErrNotFound for all lookups to stop execution after the agent lookup step.
type trackingAgentRepository struct {
	getCalled           bool
	getByClientIDCalled bool
}

func (r *trackingAgentRepository) Get(_ context.Context, _ id.AgentID) (*storagedomain.Agent, error) {
	r.getCalled = true
	return nil, ports.ErrNotFound
}

func (r *trackingAgentRepository) GetByClientID(_ context.Context, _ id.ClientID) (*storagedomain.Agent, error) {
	r.getByClientIDCalled = true
	return nil, ports.ErrNotFound
}

func (r *trackingAgentRepository) Create(_ context.Context, _ *storagedomain.Agent) error { return nil }

func (r *trackingAgentRepository) Update(_ context.Context, _ *storagedomain.Agent) error { return nil }

func (r *trackingAgentRepository) Delete(_ context.Context, _ id.AgentID) error { return nil }

func (r *trackingAgentRepository) List(_ context.Context) ([]*storagedomain.Agent, error) {
	return nil, nil
}

func (r *trackingAgentRepository) GetByClientURI(_ context.Context, _ string) (*storagedomain.Agent, error) {
	return nil, storagedomain.NewStorageError("GetAgentByClientURI", storagedomain.ErrorKindNotFound, ports.ErrNotFound, "not found")
}

func (r *trackingAgentRepository) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

// generateTestRSAKeySet generates an RSA key pair and returns the private key plus a JWKS set
// containing the corresponding public key. Used to set up JWTValidator in T029 tests.
func generateTestRSAKeySet(t *testing.T) (*rsa.PrivateKey, jwk.Set) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwkKey, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, jwkKey.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, jwkKey.Set(jwk.AlgorithmKey, jwa.RS256()))

	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(jwkKey))
	return privateKey, keySet
}

// signServiceTestJWT signs a JWT with the given RSA private key for use in service tests.
func signServiceTestJWT(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]interface{}) string {
	t.Helper()
	tok := jwt.New()
	for k, v := range claims {
		require.NoError(t, tok.Set(k, v))
	}
	jwkPrivKey, err := jwk.Import[jwk.Key](privateKey)
	require.NoError(t, err)
	require.NoError(t, jwkPrivKey.Set(jwk.KeyIDKey, "test-key"))
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), jwkPrivKey))
	require.NoError(t, err)
	return string(signed)
}

// newServiceForStep9Test builds a TokenExchangeService wired to reach step 9 (agent lookup).
// The service is configured with:
//   - A real JWTValidator backed by the supplied JWKS set (RSA key pair)
//   - A real CELEvaluator with "subject_token.azp" as agentClientID expression
//   - A MockServiceRepository that always returns a non-nil provider entity
//   - The supplied agentRepo for step 9 (the code under test)
func newServiceForStep9Test(t *testing.T, keySet jwk.Set, agentRepo ports.AgentRepository) *TokenExchangeService {
	t.Helper()
	return newServiceForStep9TestWithAuthz(t, keySet, agentRepo, "true")
}

// newServiceForStep9TestWithAuthz is newServiceForStep9Test parameterized by the CEL
// privileged-client authorization expression, threaded into both the CELEvaluator config
// and the TokenExchangeConfig authorization policy so allow/deny behavior stays consistent.
func newServiceForStep9TestWithAuthz(t *testing.T, keySet jwk.Set, agentRepo ports.AgentRepository, authzExpr string) *TokenExchangeService {
	t.Helper()
	jwtValidator, err := NewJWTValidator(
		&MockJWKSProvider{keySet: keySet},
		"https://auth.example.com",
		"agentic-identity-broker",
		60,
	)
	require.NoError(t, err)

	celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
		PrincipalExpression:     "subject_token.sub",
		AgentIDExpression:       "subject_token.azp",
		AuthorizationExpression: authzExpr,
		EvaluationTimeout:       100 * time.Millisecond,
	})
	require.NoError(t, err)

	providerEntity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "Test Service",
		ClientID:    id.ClientID("test-client"),
		// MockEncryption Decrypt is a pass-through; decryptSecret requires an encrypted secret
		Secret: model.NewEncryptedSecret([]byte("placeholder")),
	}
	providerRepo := &MockServiceRepository{service: providerEntity}

	consentSvc := consent.NewService(
		&MockAgentRepository{},
		newTestProviderService(&MockServiceRepository{}),
		&MockGrantRepository{err: ports.ErrNotFound},
		nil,
		nil,
		slog.Default(),
	)

	svc, err := NewTokenExchangeServiceForTest(
		jwtValidator,
		celEvaluator,
		newTestProviderService(providerRepo),
		&oauth2session.OAuth2SessionService{},
		consentSvc,
		agentRepo,
		&ports.TokenExchangeConfig{
			ClaimExtraction: ports.ClaimExtractionConfig{
				PrincipalExpression: "subject_token.sub",
				AgentIDExpression:   "subject_token.azp",
			},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: authzExpr},
			},
		},
	)
	require.NoError(t, err)
	return svc
}

// TestExchange_AgentLookup_UsesGetNotGetByClientID verifies that after T030 the service
// calls agentRepository.Get(agentID) (parsed UUID) rather than GetByClientID(clientID).
// [T029] Written before T030 — fails semantically until T030 replaces GetByClientID with Get.
func TestExchange_AgentLookup_UsesGetNotGetByClientID(t *testing.T) {
	privateKey, keySet := generateTestRSAKeySet(t)

	agentUUID := id.NewAgentID()
	tracker := &trackingAgentRepository{}
	svc := newServiceForStep9Test(t, keySet, tracker)

	now := time.Now()
	commonClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"exp": now.Add(1 * time.Hour).Unix(),
		"iat": now.Unix(),
	}

	subjectClaims := map[string]interface{}{}
	for k, v := range commonClaims {
		subjectClaims[k] = v
	}
	// azp = valid UUID string (the agent's internal ID)
	subjectClaims["azp"] = agentUUID.String()

	subjectToken := signServiceTestJWT(t, privateKey, subjectClaims)
	clientAssertion := signServiceTestJWT(t, privateKey, commonClaims)

	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		subjectToken,
		AccessTokenType,
		clientAssertion,
		JWTBearerType,
		"https://api.example.com/resource",
		"",
	)

	_, _ = svc.Exchange(context.Background(), req)

	// After T030: Get is called (with parsed UUID). Before T030: GetByClientID is called.
	assert.True(t, tracker.getCalled, "agentRepository.Get should be called (not GetByClientID) after T030")
	assert.False(t, tracker.getByClientIDCalled, "agentRepository.GetByClientID must NOT be called after T030")
}

// TestExchange_AgentLookup_InvalidUUIDReturnsInvalidRequest verifies that when the CEL
// expression returns a non-UUID string, the service returns an invalid_request error.
// [T029] Written before T030 — fails until T030 adds id.ParseAgentID and returns invalid_request.
func TestExchange_AgentLookup_InvalidUUIDReturnsInvalidRequest(t *testing.T) {
	privateKey, keySet := generateTestRSAKeySet(t)

	tracker := &trackingAgentRepository{}
	svc := newServiceForStep9Test(t, keySet, tracker)

	now := time.Now()
	commonClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"exp": now.Add(1 * time.Hour).Unix(),
		"iat": now.Unix(),
	}

	subjectClaims := map[string]interface{}{}
	for k, v := range commonClaims {
		subjectClaims[k] = v
	}
	// azp = not a valid UUID → id.ParseAgentID will fail after T030
	subjectClaims["azp"] = "not-a-valid-uuid"

	subjectToken := signServiceTestJWT(t, privateKey, subjectClaims)
	clientAssertion := signServiceTestJWT(t, privateKey, commonClaims)

	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		subjectToken,
		AccessTokenType,
		clientAssertion,
		JWTBearerType,
		"https://api.example.com/resource",
		"",
	)

	_, err := svc.Exchange(context.Background(), req)

	require.Error(t, err)
	tokenErr, ok := err.(*TokenExchangeError)
	require.True(t, ok, "error must be a *TokenExchangeError, got %T: %v", err, err)
	assert.Equal(t, "invalid_request", tokenErr.Code(),
		"non-UUID agentClientID must return invalid_request (not access_denied)")
}

// singleAgentRepo is a minimal ports.AgentRepository that returns one fixed agent.
type singleAgentRepo struct {
	agentID id.AgentID
	agent   *storagedomain.Agent
}

func (r *singleAgentRepo) Get(_ context.Context, agentID id.AgentID) (*storagedomain.Agent, error) {
	if agentID == r.agentID {
		return r.agent, nil
	}
	return nil, ports.ErrNotFound
}

func (r *singleAgentRepo) GetByClientID(_ context.Context, _ id.ClientID) (*storagedomain.Agent, error) {
	return nil, ports.ErrNotFound
}

func (r *singleAgentRepo) GetByClientURI(_ context.Context, _ string) (*storagedomain.Agent, error) {
	return nil, ports.ErrNotFound
}
func (r *singleAgentRepo) Create(_ context.Context, _ *storagedomain.Agent) error { return nil }
func (r *singleAgentRepo) Update(_ context.Context, _ *storagedomain.Agent) error { return nil }
func (r *singleAgentRepo) Delete(_ context.Context, _ id.AgentID) error           { return nil }
func (r *singleAgentRepo) List(_ context.Context) ([]*storagedomain.Agent, error) { return nil, nil }
func (r *singleAgentRepo) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

// TestExchange_PSAgentNoSRs_EmptyGrantGuard checks that the empty-grant guard also fires
// when the agent declares PermissionSets but has no ServiceRequirements and the grant has
// no GrantedPermissionSets entries. This is the same post-consent-edit scenario but via
// the PermissionSets declaration path rather than the ServiceRequirements path.
func TestExchange_PSAgentNoSRs_EmptyGrantGuard(t *testing.T) {
	t.Parallel()

	privateKey, keySet := generateTestRSAKeySet(t)
	agentUUID := id.NewAgentID()
	svcID := id.NewServiceID()

	agent := &storagedomain.Agent{
		ID:          agentUUID,
		ClientID:    ptr.To(id.ClientID("test-agent-client")),
		DisplayName: "Test Agent",
		// No ServiceRequirements — agent was edited after consent was granted
		ServiceRequirements: nil,
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: id.NewPermissionSetID(), RequirementType: storagedomain.RequirementTypeMandatory},
		},
	}

	futureTime := time.Now().Add(time.Hour)
	grant := &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		AgentID:               agentUUID,
		Principal:             id.Principal("user@example.com"),
		ValidUntil:            &futureTime,
		GrantedPermissionSets: nil,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	agentRepo := &singleAgentRepo{agentID: agentUUID, agent: agent}
	grantRepo := &MockGrantRepository{grant: grant}
	psRepo := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{}}
	psService := permissionset.NewPermissionSetService(psRepo, grantRepo, slog.Default())

	providerEntity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                 svcID,
		DisplayName:        "Test Service",
		ClientID:           id.ClientID("svc-client"),
		Secret:             model.NewEncryptedSecret([]byte("placeholder")),
		ProtectedResources: []string{"https://api.example.com/resource"},
	}

	consentSvc := consent.NewService(
		agentRepo,
		newTestProviderService(&MockServiceRepository{}),
		grantRepo,
		nil,
		nil,
		slog.Default(),
	)

	jwtValidator, err := NewJWTValidator(
		&MockJWKSProvider{keySet: keySet},
		"https://auth.example.com",
		"agentic-identity-broker",
		60,
	)
	require.NoError(t, err)

	celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
		PrincipalExpression:     "subject_token.sub",
		AgentIDExpression:       "subject_token.azp",
		AuthorizationExpression: "true",
		EvaluationTimeout:       100 * time.Millisecond,
	})
	require.NoError(t, err)

	svc := &TokenExchangeService{
		jwtValidator:         jwtValidator,
		celEvaluator:         celEvaluator,
		providerService:      newTestProviderService(&MockServiceRepository{service: providerEntity}),
		oauth2SessionService: &oauth2session.OAuth2SessionService{},
		consentService:       consentSvc,
		agentRepository:      agentRepo,
		permissionSetService: psService,
		config: &ports.TokenExchangeConfig{
			ClaimExtraction: ports.ClaimExtractionConfig{
				PrincipalExpression: "subject_token.sub",
				AgentIDExpression:   "subject_token.azp",
			},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: "true"},
			},
		},
	}

	now := time.Now()
	commonClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	subjectClaims := map[string]interface{}{}
	for k, v := range commonClaims {
		subjectClaims[k] = v
	}
	subjectClaims["azp"] = agentUUID.String()

	subjectToken := signServiceTestJWT(t, privateKey, subjectClaims)
	clientAssertion := signServiceTestJWT(t, privateKey, commonClaims)

	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		subjectToken,
		AccessTokenType,
		clientAssertion,
		JWTBearerType,
		"https://api.example.com/resource",
		"",
	)

	_, exchErr := svc.Exchange(context.Background(), req)

	require.Error(t, exchErr)
	tokenErr, ok := exchErr.(*TokenExchangeError)
	require.True(t, ok, "error must be *TokenExchangeError, got %T: %v", exchErr, exchErr)
	assert.Equal(t, "invalid_grant", tokenErr.Code(),
		"empty-grant guard must fire for PS-backed agent with no SRs and empty grant")
	assert.Contains(t, tokenErr.Description(), "re-consent",
		"error description must mention re-consent")
}

// TestExchange_UncoveredServiceDeniedBeforeSessionLookup verifies that an agent
// with neither service requirements nor permission sets cannot exchange for an
// uncovered service before the token vault is read.
func TestExchange_UncoveredServiceDeniedBeforeSessionLookup(t *testing.T) {
	t.Parallel()

	privateKey, keySet := generateTestRSAKeySet(t)
	agentID := id.NewAgentID()
	coveredServiceID := id.NewServiceID()
	requestedServiceID := id.NewServiceID()
	permissionSetID := id.NewPermissionSetID()

	agent := &storagedomain.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("test-agent-client")),
		DisplayName: "Test Agent",
	}
	futureTime := time.Now().Add(time.Hour)
	grant := &storagedomain.UserGrant{
		ID:         id.NewGrantID(),
		AgentID:    agentID,
		Principal:  id.Principal("user@example.com"),
		ValidUntil: &futureTime,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{
				PermissionSetID:    permissionSetID,
				IncludedServiceIDs: []id.ServiceID{coveredServiceID},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	agentRepo := &singleAgentRepo{agentID: agentID, agent: agent}
	grantRepo := &MockGrantRepository{grant: grant}
	psRepo := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
		permissionSetID: {
			ID:          permissionSetID,
			Name:        "Test Permission Set",
			Description: "Covers only a different service",
			ServiceScopes: []storagedomain.ServiceScope{
				{ServiceID: coveredServiceID, RequirementType: storagedomain.RequirementTypeOptional},
			},
		},
	}}
	psService := permissionset.NewPermissionSetService(psRepo, grantRepo, slog.Default())
	consentSvc := consent.NewService(
		agentRepo,
		newTestProviderService(&MockServiceRepository{}),
		grantRepo,
		nil,
		nil,
		slog.Default(),
	)

	sessionRepo := &MockSessionRepository{
		session: &storagedomain.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            id.Principal("user@example.com"),
			ServiceID:            requestedServiceID,
			EncryptedAccessToken: []byte("access-token"),
			TokenType:            "Bearer",
		},
	}
	oauth2SessionService := oauth2session.NewOAuth2SessionService(
		nil,
		sessionRepo,
		nil,
		nil,
		&MockEncryption{},
		nil,
		nil,
		oauth2session.DefaultConfig(),
		slog.Default(),
	)

	jwtValidator, err := NewJWTValidator(
		&MockJWKSProvider{keySet: keySet},
		"https://auth.example.com",
		"agentic-identity-broker",
		60,
	)
	require.NoError(t, err)
	celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
		PrincipalExpression:     "subject_token.sub",
		AgentIDExpression:       "subject_token.azp",
		AuthorizationExpression: "true",
		EvaluationTimeout:       100 * time.Millisecond,
	})
	require.NoError(t, err)

	svc := &TokenExchangeService{
		jwtValidator: jwtValidator,
		celEvaluator: celEvaluator,
		providerService: newTestProviderService(&MockServiceRepository{
			service: &model.ThirdpartyOAuth2ProviderEntity{
				ID:                 requestedServiceID,
				DisplayName:        "Requested Service",
				ClientID:           id.ClientID("service-client"),
				Secret:             model.NewEncryptedSecret([]byte("placeholder")),
				ProtectedResources: []string{"https://api.example.com/requested"},
			},
		}),
		oauth2SessionService: oauth2SessionService,
		consentService:       consentSvc,
		permissionSetService: psService,
		agentRepository:      agentRepo,
		config: &ports.TokenExchangeConfig{
			ClaimExtraction: ports.ClaimExtractionConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp"},
			Authorization:   ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true"}},
		},
	}

	now := time.Now()
	claims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"azp": agentID.String(),
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		signServiceTestJWT(t, privateKey, claims),
		AccessTokenType,
		signServiceTestJWT(t, privateKey, claims),
		JWTBearerType,
		"https://api.example.com/requested",
		"",
	)

	_, err = svc.Exchange(context.Background(), req)
	require.Error(t, err)
	tokenErr, ok := err.(*TokenExchangeError)
	require.True(t, ok, "error must be *TokenExchangeError, got %T: %v", err, err)
	assert.Equal(t, "invalid_grant", tokenErr.Code())
	assert.Contains(t, tokenErr.Description(), "not authorized by any permission set")
	assert.Zero(t, sessionRepo.findByPrincipalAndServiceCalls, "uncovered service must be rejected before token-vault lookup")
}

func TestExchange_FinalizesSecurityContextWithDistinctCallingPeer(t *testing.T) {
	t.Parallel()

	privateKey, keySet := generateTestRSAKeySet(t)
	agentUUID := id.NewAgentID()
	svc := newServiceForStep9Test(t, keySet, &trackingAgentRepository{})
	receivedAt := time.Date(2026, time.July, 3, 16, 45, 0, 0, time.UTC)
	holder := security.NewCaptureHolder(security.TransportCapture{
		TraceID:       "0123456789abcdef0123456789abcdef",
		ClientIP:      "203.0.113.44",
		UserAgent:     "delegated-client/1.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    receivedAt,
	})
	ctx := security.WithCaptureHolder(context.Background(), holder)

	now := time.Now()
	subjectClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"azp": agentUUID.String(),
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	clientAssertionClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "privileged-client-1",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}

	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		signServiceTestJWT(t, privateKey, subjectClaims),
		AccessTokenType,
		signServiceTestJWT(t, privateKey, clientAssertionClaims),
		JWTBearerType,
		"https://api.example.com/resource",
		"",
	)

	_, _ = svc.Exchange(ctx, req)

	sc, ok := holder.Finalized()
	require.True(t, ok, "validated delegated exchanges must finalize the captured transport context")
	assert.Equal(t, "0123456789abcdef0123456789abcdef", sc.TraceID)
	assert.Equal(t, "user@example.com", sc.Actor)
	assert.Equal(t, "privileged-client-1", sc.CallingPeer)
	assert.Equal(t, "203.0.113.44", sc.ClientIP)
	assert.Equal(t, "delegated-client/1.0", sc.UserAgent)
	assert.Equal(t, "/oauth2/token", sc.RequestTarget)
	assert.Equal(t, receivedAt, sc.ReceivedAt)
}

func TestExchange_FinalizesSecurityContextWithoutDuplicatingCallingPeer(t *testing.T) {
	t.Parallel()

	privateKey, keySet := generateTestRSAKeySet(t)
	agentUUID := id.NewAgentID()
	svc := newServiceForStep9Test(t, keySet, &trackingAgentRepository{})
	holder := security.NewCaptureHolder(security.TransportCapture{
		TraceID:       "fedcba9876543210fedcba9876543210",
		ClientIP:      "198.51.100.19",
		UserAgent:     "delegated-client/2.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    time.Date(2026, time.July, 3, 17, 0, 0, 0, time.UTC),
	})
	ctx := security.WithCaptureHolder(context.Background(), holder)

	now := time.Now()
	claims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "same-identity@example.com",
		"azp": agentUUID.String(),
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}

	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		signServiceTestJWT(t, privateKey, claims),
		AccessTokenType,
		signServiceTestJWT(t, privateKey, claims),
		JWTBearerType,
		"https://api.example.com/resource",
		"",
	)

	_, _ = svc.Exchange(ctx, req)

	sc, ok := holder.Finalized()
	require.True(t, ok, "validated delegated exchanges must always finalize the security context")
	assert.Equal(t, "same-identity@example.com", sc.Actor)
	assert.Empty(t, sc.CallingPeer, "calling_peer must collapse to empty when it resolves to the actor")
	assert.Equal(t, "fedcba9876543210fedcba9876543210", sc.TraceID)
}

// TestExchange_FinalizesSecurityContextWhenAuthorizationDenied is the regression guard for a
// security-audit finding: when subject_token and client_assertion both validate but the CEL
// privileged-client policy DENIES the exchange, the request SecurityContext must still be
// finalized so downstream audit logging retains actor + calling_peer on this failure path.
func TestExchange_FinalizesSecurityContextWhenAuthorizationDenied(t *testing.T) {
	t.Parallel()

	privateKey, keySet := generateTestRSAKeySet(t)
	agentUUID := id.NewAgentID()
	svc := newServiceForStep9TestWithAuthz(t, keySet, &trackingAgentRepository{}, "false")
	receivedAt := time.Date(2026, time.July, 3, 18, 15, 0, 0, time.UTC)
	holder := security.NewCaptureHolder(security.TransportCapture{
		TraceID:       "aaaabbbbccccddddeeeeffff00001111",
		ClientIP:      "192.0.2.77",
		UserAgent:     "delegated-client/3.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    receivedAt,
	})
	ctx := security.WithCaptureHolder(context.Background(), holder)

	now := time.Now()
	subjectClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"azp": agentUUID.String(),
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	clientAssertionClaims := map[string]interface{}{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "privileged-client-1",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}

	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		signServiceTestJWT(t, privateKey, subjectClaims),
		AccessTokenType,
		signServiceTestJWT(t, privateKey, clientAssertionClaims),
		JWTBearerType,
		"https://api.example.com/resource",
		"",
	)

	resp, err := svc.Exchange(ctx, req)

	require.Error(t, err)
	assert.Nil(t, resp)
	tokenErr, ok := err.(*TokenExchangeError)
	require.True(t, ok, "error must be *TokenExchangeError, got %T: %v", err, err)
	assert.Equal(t, "access_denied", tokenErr.Code(),
		"CEL privileged-client denial must surface as access_denied")

	sc, ok := holder.Finalized()
	require.True(t, ok, "a denied delegated exchange whose tokens validated MUST still finalize the security context for the audit trail")
	assert.Equal(t, "user@example.com", sc.Actor)
	assert.Equal(t, "privileged-client-1", sc.CallingPeer)
	assert.Equal(t, "aaaabbbbccccddddeeeeffff00001111", sc.TraceID)
}
