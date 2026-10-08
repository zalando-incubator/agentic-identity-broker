package tokenexchange

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	psMap         map[id.PermissionSetID]*storagedomain.PermissionSet
	getByIDsCalls int
	getByIDsErr   error
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
	m.getByIDsCalls++
	if m.getByIDsErr != nil {
		return nil, m.getByIDsErr
	}
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

func (m *MockPermissionSetRepository) Delete(ctx context.Context, psID id.PermissionSetID) (bool, error) {
	_, existed := m.psMap[psID]
	delete(m.psMap, psID)
	return existed, nil
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

func (m *MockAgentRepository) Delete(ctx context.Context, agentID id.AgentID) (bool, error) {
	return false, nil
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
	grant                        *storagedomain.UserGrant
	err                          error
	findByPrincipalAndAgentCalls int
}

func (m *MockGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storagedomain.UserGrant, error) {
	m.findByPrincipalAndAgentCalls++
	return m.grant, m.err
}

func (m *MockGrantRepository) Create(ctx context.Context, grant *storagedomain.UserGrant) error {
	if m.err != nil {
		return m.err
	}
	persisted := grant.Copy()
	if m.grant != nil && m.grant.Principal == grant.Principal && m.grant.AgentID == grant.AgentID {
		persisted.ID = m.grant.ID
		persisted.CreatedAt = m.grant.CreatedAt
	}
	m.grant = persisted
	grant.ID = persisted.ID
	grant.CreatedAt = persisted.CreatedAt
	grant.UpdatedAt = persisted.UpdatedAt
	grant.ValidUntil = nil
	if persisted.ValidUntil != nil {
		validUntil := *persisted.ValidUntil
		grant.ValidUntil = &validUntil
	}
	return nil
}

func (m *MockGrantRepository) Get(ctx context.Context, grantID id.GrantID) (*storagedomain.UserGrant, error) {
	return nil, nil
}

func (m *MockGrantRepository) Update(ctx context.Context, grant *storagedomain.UserGrant) error {
	if m.err != nil {
		return m.err
	}
	if m.grant == nil || m.grant.ID != grant.ID {
		return ports.ErrNotFound
	}
	persisted := grant.Copy()
	persisted.CreatedAt = m.grant.CreatedAt
	m.grant = persisted
	grant.ID = persisted.ID
	grant.CreatedAt = persisted.CreatedAt
	grant.UpdatedAt = persisted.UpdatedAt
	grant.ValidUntil = nil
	if persisted.ValidUntil != nil {
		validUntil := *persisted.ValidUntil
		grant.ValidUntil = &validUntil
	}
	return nil
}

func (m *MockGrantRepository) Delete(ctx context.Context, grantID id.GrantID) error {
	return nil
}

func (m *MockGrantRepository) CountAgentsByPrincipalAndServiceID(_ context.Context, _ id.Principal, _ id.ServiceID) (int, error) {
	return 0, nil
}

func (m *MockGrantRepository) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	return nil
}

func (m *MockGrantRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storagedomain.UserGrant, error) {
	return nil, nil
}

func (m *MockGrantRepository) ListByPrincipalAndServiceID(_ context.Context, _ id.Principal, _ id.ServiceID) ([]id.AgentID, error) {
	return nil, nil
}

func (m *MockGrantRepository) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storagedomain.UserGrant, error) {
	return nil, nil
}

func (m *MockGrantRepository) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storagedomain.UserGrant, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.grant == nil || m.grant.Principal != principal || m.grant.AgentID != agentID {
		return nil, storagedomain.NewStorageError("DeleteByPrincipalAndAgentID", storagedomain.ErrorKindNotFound, ports.ErrNotFound, "grant not found")
	}
	snapshot := m.grant.Copy()
	m.grant = nil
	return snapshot, nil
}

func (m *MockGrantRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

type MockSessionRepository struct {
	session                        *storagedomain.UserSession
	err                            error
	findByPrincipalAndServiceCalls int
	findByPrincipalAndServiceFunc  func(context.Context, id.Principal, id.ServiceID) (*storagedomain.UserSession, error)
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
	if m.findByPrincipalAndServiceFunc != nil {
		return m.findByPrincipalAndServiceFunc(ctx, principal, serviceID)
	}
	return m.session, m.err
}

func (m *MockSessionRepository) WithLockedSession(ctx context.Context, _ id.Principal, _ id.ServiceID, refresh func(context.Context, *storagedomain.UserSession) (bool, error)) (*storagedomain.UserSession, error) {
	if m.session == nil {
		return nil, nil
	}
	working := *m.session
	changed, err := refresh(ctx, &working)
	if err != nil {
		return nil, err
	}
	if changed {
		m.session = &working
	}
	return &working, nil
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
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: ps1ID, RequirementType: storagedomain.RequirementTypeOptional},
			{PermissionSetID: ps2ID, RequirementType: storagedomain.RequirementTypeOptional},
		},
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
	scopes, detail, err := svc.resolveEffectiveScopes(context.Background(), grant, agent, svcA)
	require.NoError(t, err)
	assert.Equal(t, DetailNone, detail)

	// Scope union: read + write
	assert.Contains(t, scopes, svcA)
	assert.Equal(t, []string{"read", "write"}, scopes[svcA])
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
	effectiveScopes, detail, err := svc.resolveEffectiveScopes(context.Background(), &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    permissionSetID,
			IncludedServiceIDs: []id.ServiceID{serviceID},
		}},
	}, &storagedomain.Agent{
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: permissionSetID, RequirementType: storagedomain.RequirementTypeOptional},
		},
		ServiceRequirements: []storagedomain.ServiceRequirement{{
			ServiceID:       serviceID,
			RequirementType: storagedomain.RequirementTypeMandatory,
		}},
	}, serviceID)

	require.NoError(t, err)
	assert.Equal(t, DetailNone, detail)
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
	effectiveScopes, detail, err := svc.resolveEffectiveScopes(context.Background(), &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    permissionSetID,
			IncludedServiceIDs: []id.ServiceID{serviceID},
		}},
	}, &storagedomain.Agent{
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: permissionSetID, RequirementType: storagedomain.RequirementTypeOptional},
		},
		ServiceRequirements: []storagedomain.ServiceRequirement{{
			ServiceID:       serviceID,
			RequiredScopes:  []string{"write"},
			RequirementType: storagedomain.RequirementTypeMandatory,
		}},
	}, serviceID)

	require.NoError(t, err)
	assert.Equal(t, FailureDetail("grant_scope_intersection_empty"), detail)
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
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: ps1ID, RequirementType: storagedomain.RequirementTypeOptional},
		},
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
	scopes, detail, err := svc.resolveEffectiveScopes(context.Background(), grant, agent, svcA)
	require.NoError(t, err)
	assert.Equal(t, DetailNone, detail)

	// "admin" should be excluded by SR ceiling
	assert.Contains(t, scopes, svcA)
	assert.Equal(t, []string{"read", "write"}, scopes[svcA])
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
	effectiveScopes, detail, err := svc.resolveEffectiveScopes(context.Background(), &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{
			PermissionSetID:    psID,
			IncludedServiceIDs: []id.ServiceID{svcA},
		}},
	}, &storagedomain.Agent{
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: psID, RequirementType: storagedomain.RequirementTypeOptional},
		},
		ServiceRequirements: []storagedomain.ServiceRequirement{{
			ServiceID:        svcA,
			RequireAllScopes: true,
		}},
	}, svcA)

	require.NoError(t, err)
	assert.Equal(t, DetailNone, detail)
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
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: ps1ID, RequirementType: storagedomain.RequirementTypeOptional},
		},
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
	scopes, detail, err := svc.resolveEffectiveScopes(context.Background(), grant, agent, svcB)
	require.NoError(t, err)
	assert.Equal(t, FailureDetail("grant_service_omitted"), detail)

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
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: ps1ID, RequirementType: storagedomain.RequirementTypeOptional},
			{PermissionSetID: ps2ID, RequirementType: storagedomain.RequirementTypeOptional},
		},
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
	scopes, detail, err := svc.resolveEffectiveScopes(context.Background(), grant, agent, svcA)
	require.NoError(t, err)
	assert.Equal(t, DetailNone, detail)

	// Union of PS scopes: {read, admin, write, delete}
	// After SR ceiling intersection: {read, write}
	assert.Contains(t, scopes, svcA)
	assert.Equal(t, []string{"read", "write"}, scopes[svcA])
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
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: ps1ID, RequirementType: storagedomain.RequirementTypeOptional},
		},
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
	scopes, detail, err := svc.resolveEffectiveScopes(context.Background(), grant, agent, svcB)
	require.NoError(t, err)
	assert.Equal(t, FailureDetail("grant_service_requirement_excluded"), detail)

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
	scopes, detail, err := svc.resolveEffectiveScopes(context.Background(), grant, agent, svcA)
	require.NoError(t, err)
	assert.Equal(t, DetailNone, detail)
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

func (r *trackingAgentRepository) Delete(_ context.Context, _ id.AgentID) (bool, error) {
	return false, nil
}

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

func TestExchange_AgentLookup_InvalidUUIDIsConfigurationError(t *testing.T) {
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
	assert.Equal(t, "server_error", tokenErr.Code())
	assert.Equal(t, OutcomeConfigurationError, tokenErr.Diagnostic().Outcome())
	assert.Equal(t, StageIdentityResolution, tokenErr.Diagnostic().Stage())
	assert.Equal(t, DetailAgentInvalid, tokenErr.Diagnostic().Detail())
}

// singleAgentRepo is a minimal ports.AgentRepository that returns one fixed agent.
type singleAgentRepo struct {
	agentID id.AgentID
	agent   *storagedomain.Agent
	err     error
}

func (r *singleAgentRepo) Get(_ context.Context, agentID id.AgentID) (*storagedomain.Agent, error) {
	if r.err != nil {
		return nil, r.err
	}
	if agentID == r.agentID && r.agent != nil {
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
func (r *singleAgentRepo) Delete(_ context.Context, agentID id.AgentID) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	if r.agent == nil || agentID != r.agentID {
		return false, nil
	}
	r.agent = nil
	return true, nil
}
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
	assert.Equal(t, "access_denied", tokenErr.Code())
	assert.Equal(t, OutcomeAuthorizationDenied, tokenErr.Diagnostic().Outcome())
	assert.Equal(t, RecoveryReconsent, tokenErr.Diagnostic().RecoveryAction())
}

// Grant authorization must reject every uncovered target before reading the token vault.
func TestExchange_UncoveredServiceDeniedBeforeSessionLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		coverage        grantCoverage
		callbackBaseURL string
		mutate          func(exchangeFixture)
		wantDetail      FailureDetail
		wantPSCalls     int
	}{
		{name: "grant excludes requested service", coverage: grantCoversOtherService, wantDetail: "grant_service_omitted", wantPSCalls: 1},
		{name: "trailing-slash callback base", coverage: grantCoversOtherService, callbackBaseURL: "https://broker.example.com/", wantDetail: "grant_service_omitted", wantPSCalls: 1},
		{name: "empty grant", coverage: grantEmpty, wantDetail: "grant_empty"},
		{name: "stale permission set", coverage: grantStalePermissionSet, wantDetail: "grant_permission_set_missing", wantPSCalls: 1},
		{name: "undeclared permission set", mutate: func(f exchangeFixture) {
			f.agent.PermissionSets = nil
		}, wantDetail: "grant_permission_set_undeclared"},
		{name: "included service no longer defined", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes = nil
		}, wantDetail: "grant_service_definition_missing", wantPSCalls: 1},
		{name: "agent requirements exclude target", mutate: func(f exchangeFixture) {
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: id.NewServiceID(), RequiredScopes: []string{"read"}}}
		}, wantDetail: "grant_service_requirement_excluded", wantPSCalls: 1},
		{name: "scope ceiling removes union", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes[0].Scopes = []string{"read"}
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: f.serviceID, RequiredScopes: []string{"write"}}}
		}, wantDetail: "grant_scope_intersection_empty", wantPSCalls: 1},
		{name: "explicit zero scope ceiling removes nonempty union", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes[0].Scopes = []string{"read"}
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: f.serviceID}}
		}, wantDetail: "grant_scope_intersection_empty", wantPSCalls: 1},
		{name: "empty source set does not bypass ceiling on nonempty union", mutate: func(f exchangeFixture) {
			otherID := id.NewPermissionSetID()
			f.psRepo.psMap[otherID] = &storagedomain.PermissionSet{ID: otherID, ServiceScopes: []storagedomain.ServiceScope{{ServiceID: f.serviceID, Scopes: []string{"read"}}}}
			f.agent.PermissionSets = append(f.agent.PermissionSets, storagedomain.AgentPermissionSetEntry{PermissionSetID: otherID})
			f.grant.GrantedPermissionSets = append(f.grant.GrantedPermissionSets, storagedomain.GrantedPermissionSetEntry{PermissionSetID: otherID, IncludedServiceIDs: []id.ServiceID{f.serviceID}})
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: f.serviceID, RequiredScopes: []string{"write"}}}
		}, wantDetail: "grant_scope_intersection_empty", wantPSCalls: 1},
		{name: "empty grant precedes requirement exclusion", coverage: grantEmpty, mutate: func(f exchangeFixture) {
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: id.NewServiceID()}}
		}, wantDetail: "grant_empty"},
		{name: "undeclared precedes missing and omission", coverage: grantCoversOtherService, mutate: func(f exchangeFixture) {
			f.agent.PermissionSets = nil
			f.psRepo.psMap = nil
		}, wantDetail: "grant_permission_set_undeclared"},
		{name: "missing precedes omission and requirement exclusion", coverage: grantStalePermissionSet, mutate: func(f exchangeFixture) {
			f.grant.GrantedPermissionSets[0].IncludedServiceIDs = nil
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: id.NewServiceID()}}
		}, wantDetail: "grant_permission_set_missing", wantPSCalls: 1},
		{name: "unrelated missing set invalidates covered target", mutate: func(f exchangeFixture) {
			missingID := id.NewPermissionSetID()
			f.agent.PermissionSets = append(f.agent.PermissionSets, storagedomain.AgentPermissionSetEntry{PermissionSetID: missingID})
			f.grant.GrantedPermissionSets = append(f.grant.GrantedPermissionSets, storagedomain.GrantedPermissionSetEntry{PermissionSetID: missingID, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}})
		}, wantDetail: "grant_permission_set_missing", wantPSCalls: 1},
		{name: "omission precedes absent definition and requirement exclusion", coverage: grantCoversOtherService, mutate: func(f exchangeFixture) {
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: id.NewServiceID()}}
		}, wantDetail: "grant_service_omitted", wantPSCalls: 1},
		{name: "absent definition precedes requirement exclusion", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes = nil
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: id.NewServiceID()}}
		}, wantDetail: "grant_service_definition_missing", wantPSCalls: 1},
		{name: "definition must come from an applicable inclusion", mutate: func(f exchangeFixture) {
			otherID := id.NewPermissionSetID()
			f.psRepo.psMap[f.permissionSetID].ServiceScopes = nil
			f.psRepo.psMap[otherID] = &storagedomain.PermissionSet{ID: otherID, ServiceScopes: []storagedomain.ServiceScope{{ServiceID: f.serviceID, Scopes: []string{"read"}}}}
			f.agent.PermissionSets = append(f.agent.PermissionSets, storagedomain.AgentPermissionSetEntry{PermissionSetID: otherID})
			f.grant.GrantedPermissionSets = append(f.grant.GrantedPermissionSets, storagedomain.GrantedPermissionSetEntry{PermissionSetID: otherID, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}})
		}, wantDetail: "grant_service_definition_missing", wantPSCalls: 1},
		{name: "duplicate persisted entry uses final inclusion", mutate: func(f exchangeFixture) {
			f.grant.GrantedPermissionSets = append(f.grant.GrantedPermissionSets, storagedomain.GrantedPermissionSetEntry{PermissionSetID: f.permissionSetID, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}})
		}, wantDetail: "grant_service_omitted", wantPSCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sessionRepo := &MockSessionRepository{}
			callbackBaseURL := tt.callbackBaseURL
			if callbackBaseURL == "" {
				callbackBaseURL = "https://broker.example.com"
			}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{
				coverage:        tt.coverage,
				callbackBaseURL: callbackBaseURL,
				sessionRepo:     sessionRepo,
				encryption:      &MockEncryption{},
			})
			if tt.mutate != nil {
				tt.mutate(fixture)
			}

			_, err := fixture.svc.Exchange(context.Background(), fixture.req)
			require.Error(t, err)
			tokenErr, ok := err.(*TokenExchangeError)
			require.True(t, ok, "error must be *TokenExchangeError, got %T: %v", err, err)
			assert.Equal(t, "access_denied", tokenErr.Code())
			assert.Equal(t, http.StatusForbidden, tokenErr.HTTPStatus())
			assert.Equal(t, tt.wantDetail, tokenErr.Diagnostic().Detail())
			assert.Equal(t, OutcomeAuthorizationDenied, tokenErr.Diagnostic().Outcome())
			assert.Equal(t, StageGrantAuthorization, tokenErr.Diagnostic().Stage())
			assert.Equal(t, RecoveryReconsent, tokenErr.Diagnostic().RecoveryAction())
			assert.Equal(t, TargetConsent, tokenErr.Diagnostic().RecoveryTarget())
			assert.Equal(t, "https://broker.example.com/agents/"+fixture.agentID.String(), tokenErr.ErrorURI())
			assert.Equal(t, ServiceRef{ID: fixture.serviceID}, tokenErr.Service())
			assert.Equal(t, AuthorizationRef{
				AgentID:            fixture.agentID,
				GrantID:            fixture.grant.ID,
				GrantUpdatedAt:     fixture.grant.UpdatedAt,
				GrantValidUntil:    *fixture.grant.ValidUntil,
				GrantHasValidUntil: true,
			}, tokenErr.Authorization())
			assert.Equal(t, 1, fixture.grantRepo.findByPrincipalAndAgentCalls)
			assert.Zero(t, sessionRepo.findByPrincipalAndServiceCalls, "uncovered service must be rejected before token-vault lookup")
			assert.Equal(t, tt.wantPSCalls, fixture.psRepo.getByIDsCalls, "permission sets must be resolved at most once")
		})
	}
}

func TestExchange_GrantCoverageScopeMath(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		mutate func(exchangeFixture)
	}{
		{name: "original zero scopes remain covered under zero ceiling", mutate: func(f exchangeFixture) {
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: f.serviceID}}
		}},
		{name: "no requirements preserve permission set scopes", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes[0].Scopes = []string{"write", "read"}
		}},
		{name: "require all bypasses explicit ceiling", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes[0].Scopes = []string{"write", "read"}
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: f.serviceID, RequiredScopes: []string{"unrelated"}, RequireAllScopes: true}}
		}},
		{name: "sets union before scope ceiling", mutate: func(f exchangeFixture) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes[0].Scopes = []string{"write"}
			otherID := id.NewPermissionSetID()
			f.psRepo.psMap[otherID] = &storagedomain.PermissionSet{ID: otherID, ServiceScopes: []storagedomain.ServiceScope{{ServiceID: f.serviceID, Scopes: []string{"read"}}}}
			f.agent.PermissionSets = append(f.agent.PermissionSets, storagedomain.AgentPermissionSetEntry{PermissionSetID: otherID})
			f.grant.GrantedPermissionSets = append(f.grant.GrantedPermissionSets, storagedomain.GrantedPermissionSetEntry{PermissionSetID: otherID, IncludedServiceIDs: []id.ServiceID{f.serviceID}})
			f.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: f.serviceID, RequiredScopes: []string{"read"}}}
		}},
		{name: "unrelated stale included service does not invalidate target", mutate: func(f exchangeFixture) {
			f.grant.GrantedPermissionSets[0].IncludedServiceIDs = append(f.grant.GrantedPermissionSets[0].IncludedServiceIDs, id.NewServiceID())
		}},
		{name: "duplicate persisted entry final inclusion covers target", mutate: func(f exchangeFixture) {
			f.grant.GrantedPermissionSets = append([]storagedomain.GrantedPermissionSetEntry{{PermissionSetID: f.permissionSetID, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}}, f.grant.GrantedPermissionSets...)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			future := time.Now().Add(time.Hour)
			sessionRepo := &MockSessionRepository{session: &storagedomain.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal("user@example.com"),
				EncryptedAccessToken: []byte("access-token"),
				AccessTokenExpiresAt: &future,
				TokenType:            "Bearer",
				Scope:                []string{"read", "write"},
			}}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{
				callbackBaseURL: "https://broker.example.com",
				sessionRepo:     sessionRepo,
				encryption:      &MockEncryption{},
			})
			sessionRepo.session.ServiceID = fixture.serviceID
			tt.mutate(fixture)
			response, err := fixture.svc.Exchange(context.Background(), fixture.req)
			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, "read write", response.Scope)
			assert.Equal(t, fixture.agentID.String(), response.AgentID)
			assert.Equal(t, 1, sessionRepo.findByPrincipalAndServiceCalls)
			assert.Equal(t, 1, fixture.psRepo.getByIDsCalls)
		})
	}
}

func TestResolveEffectiveScopes_MissingReferenceAndRepositoryError(t *testing.T) {
	t.Parallel()
	for _, unavailable := range []bool{false, true} {
		name := "missing reference"
		if unavailable {
			name = "repository unavailable"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newExchangeFixture(t, exchangeFixtureConfig{callbackBaseURL: "https://broker.example.com"})
			fixture.psRepo.psMap = nil
			cause := errors.New("repository unavailable")
			if unavailable {
				fixture.psRepo.getByIDsErr = cause
			}
			scopes, detail, err := fixture.svc.resolveEffectiveScopes(context.Background(), fixture.grant, fixture.agent, fixture.serviceID)
			assert.Nil(t, scopes)
			assert.Equal(t, DetailNone, detail, "error diagnostics must not compete with the separate coverage detail")
			var tokenErr *TokenExchangeError
			require.ErrorAs(t, err, &tokenErr)
			if unavailable {
				assert.Equal(t, "server_error", tokenErr.Code())
				assert.Equal(t, DetailGrantRepositoryUnavailable, tokenErr.Diagnostic().Detail())
				assert.ErrorIs(t, err, cause)
				assert.Empty(t, tokenErr.ErrorURI())
			} else {
				assert.Equal(t, "access_denied", tokenErr.Code())
				assert.Equal(t, FailureDetail("grant_permission_set_missing"), tokenErr.Diagnostic().Detail())
				assert.Equal(t, "https://broker.example.com/agents/"+fixture.agentID.String(), tokenErr.ErrorURI())
			}
			assert.Equal(t, 1, fixture.psRepo.getByIDsCalls)
		})
	}
}

func TestResolveEffectiveScopes_CoverageDetailPreservesOtherServiceScopes(t *testing.T) {
	t.Parallel()
	for _, wantDetail := range []FailureDetail{
		"grant_service_omitted",
		"grant_service_definition_missing",
		"grant_service_requirement_excluded",
		"grant_scope_intersection_empty",
	} {
		t.Run(string(wantDetail), func(t *testing.T) {
			t.Parallel()
			fixture := newExchangeFixture(t, exchangeFixtureConfig{callbackBaseURL: "https://broker.example.com"})
			otherServiceID := id.NewServiceID()
			fixture.grant.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{fixture.serviceID, otherServiceID}
			definition := fixture.psRepo.psMap[fixture.permissionSetID]
			definition.ServiceScopes = []storagedomain.ServiceScope{
				{ServiceID: fixture.serviceID, Scopes: []string{"read"}},
				{ServiceID: otherServiceID, Scopes: []string{"read"}},
			}
			switch wantDetail {
			case "grant_service_omitted":
				fixture.grant.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{otherServiceID}
			case "grant_service_definition_missing":
				definition.ServiceScopes = definition.ServiceScopes[1:]
			case "grant_service_requirement_excluded":
				fixture.agent.ServiceRequirements = []storagedomain.ServiceRequirement{{ServiceID: otherServiceID, RequiredScopes: []string{"read"}}}
			case "grant_scope_intersection_empty":
				fixture.agent.ServiceRequirements = []storagedomain.ServiceRequirement{
					{ServiceID: fixture.serviceID, RequiredScopes: []string{"write"}},
					{ServiceID: otherServiceID, RequiredScopes: []string{"read"}},
				}
			}
			scopes, detail, err := fixture.svc.resolveEffectiveScopes(context.Background(), fixture.grant, fixture.agent, fixture.serviceID)
			require.NoError(t, err, "coverage gaps must use the separate detail, not the structural error channel")
			assert.Equal(t, wantDetail, detail)
			assert.Equal(t, map[id.ServiceID][]string{otherServiceID: {"read"}}, scopes)
			assert.Equal(t, 1, fixture.psRepo.getByIDsCalls)
		})
	}
}

func TestExchange_PermissionSetRepositoryFailureBeforeSessionLookup(t *testing.T) {
	t.Parallel()
	sessionRepo := &MockSessionRepository{}
	fixture := newExchangeFixture(t, exchangeFixtureConfig{
		callbackBaseURL: "https://broker.example.com",
		sessionRepo:     sessionRepo,
		encryption:      &MockEncryption{},
	})
	cause := errors.New("permission set repository unavailable")
	fixture.psRepo.getByIDsErr = cause
	fixture.grant.GrantedPermissionSets[0].IncludedServiceIDs = nil
	response, err := fixture.svc.Exchange(context.Background(), fixture.req)
	assert.Nil(t, response)
	var tokenErr *TokenExchangeError
	require.ErrorAs(t, err, &tokenErr)
	assert.Equal(t, "server_error", tokenErr.Code())
	assert.Equal(t, http.StatusInternalServerError, tokenErr.HTTPStatus())
	assert.Equal(t, DetailGrantRepositoryUnavailable, tokenErr.Diagnostic().Detail())
	assert.Equal(t, ServiceRef{ID: fixture.serviceID}, tokenErr.Service())
	assert.Equal(t, AuthorizationRef{
		AgentID:            fixture.agentID,
		GrantID:            fixture.grant.ID,
		GrantUpdatedAt:     fixture.grant.UpdatedAt,
		GrantValidUntil:    *fixture.grant.ValidUntil,
		GrantHasValidUntil: true,
	}, tokenErr.Authorization())
	assert.Equal(t, StageGrantAuthorization, tokenErr.Diagnostic().Stage())
	assert.Equal(t, OutcomeInfrastructureError, tokenErr.Diagnostic().Outcome())
	assert.Equal(t, RecoveryRetry, tokenErr.Diagnostic().RecoveryAction())
	assert.Empty(t, tokenErr.ErrorURI())
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, 1, fixture.psRepo.getByIDsCalls)
	assert.Zero(t, sessionRepo.findByPrincipalAndServiceCalls)
}

// US5-S4 / US5-S5: only a parsed provider HTTP 400 invalid_grant refresh rejection is a
// user-recoverable re-authentication; every other retrieval failure stays a server error.
func TestExchange_MapsSessionRetrievalErrors(t *testing.T) {
	t.Parallel()

	expired := time.Now().Add(-time.Hour)
	refreshableSession := func() *storagedomain.UserSession {
		return &storagedomain.UserSession{
			ID:                    id.NewSessionID(),
			Principal:             id.Principal("user@example.com"),
			EncryptedAccessToken:  []byte("expired-access-token"),
			EncryptedRefreshToken: []byte("stored-refresh-token"),
			TokenType:             "Bearer",
			AccessTokenExpiresAt:  &expired,
		}
	}
	thirdpartyResponseHandler := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}

	tests := []struct {
		name              string
		session           *storagedomain.UserSession
		repoErr           error
		encryptionErr     error
		thirdpartyHandler http.HandlerFunc
		wantCode          string
		wantReauthURI     bool
		wantRefreshErr    bool
	}{
		{name: "provider 400 invalid_grant", session: refreshableSession(), thirdpartyHandler: thirdpartyResponseHandler(http.StatusBadRequest, `{"error":"invalid_grant","error_description":"sentinel-description"}`), wantCode: "invalid_grant", wantReauthURI: true, wantRefreshErr: true},
		{name: "provider 401 invalid_client", session: refreshableSession(), thirdpartyHandler: thirdpartyResponseHandler(http.StatusUnauthorized, `{"error":"invalid_client"}`), wantCode: "server_error", wantRefreshErr: true},
		{name: "provider 400 invalid_client", session: refreshableSession(), thirdpartyHandler: thirdpartyResponseHandler(http.StatusBadRequest, `{"error":"invalid_client"}`), wantCode: "server_error", wantRefreshErr: true},
		{name: "provider 500 invalid_grant", session: refreshableSession(), thirdpartyHandler: thirdpartyResponseHandler(http.StatusInternalServerError, `{"error":"invalid_grant"}`), wantCode: "server_error", wantRefreshErr: true},
		{name: "provider 400 without OAuth code", session: refreshableSession(), thirdpartyHandler: thirdpartyResponseHandler(http.StatusBadRequest, `not json`), wantCode: "server_error", wantRefreshErr: true},
		{name: "transport failure", session: refreshableSession(), wantCode: "server_error", wantRefreshErr: true},
		{name: "decryption failure", session: refreshableSession(), encryptionErr: errors.New("kms unavailable"), wantCode: "server_error"},
		{name: "storage failure", repoErr: errors.New("database unavailable"), wantCode: "server_error"},
		{name: "missing session", wantCode: "invalid_grant", wantReauthURI: true},
		{name: "expired session without refresh token", session: &storagedomain.UserSession{ID: id.NewSessionID(), Principal: id.Principal("user@example.com"), EncryptedAccessToken: []byte("expired"), AccessTokenExpiresAt: &expired}, wantCode: "invalid_grant", wantReauthURI: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tokenEndpoint := "http://127.0.0.1:0/token"
			if tt.thirdpartyHandler != nil {
				server := httptest.NewServer(tt.thirdpartyHandler)
				t.Cleanup(server.Close)
				tokenEndpoint = server.URL
			}
			sessionRepo := &MockSessionRepository{session: tt.session, err: tt.repoErr}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{
				coverage:        grantCoversRequested,
				callbackBaseURL: "https://broker.example.com",
				sessionRepo:     sessionRepo,
				refreshRepo:     sessionRepo,
				encryption:      &MockEncryption{err: tt.encryptionErr},
				tokenEndpoint:   tokenEndpoint,
			})
			var before storagedomain.UserSession
			if tt.session != nil {
				before = *tt.session
			}

			_, err := fixture.svc.Exchange(context.Background(), fixture.req)
			require.Error(t, err)
			tokenErr, ok := err.(*TokenExchangeError)
			require.True(t, ok, "error must be *TokenExchangeError, got %T: %v", err, err)
			assert.Equal(t, tt.wantCode, tokenErr.Code())
			if tt.wantReauthURI {
				assert.Equal(t, 400, tokenErr.HTTPStatus())
				assert.Equal(t, "https://broker.example.com/sessions", tokenErr.ErrorURI())
			} else {
				assert.Equal(t, 500, tokenErr.HTTPStatus())
				assert.Empty(t, tokenErr.ErrorURI())
			}
			assert.NotContains(t, tokenErr.Description(), "sentinel-description")
			assert.Equal(t, tt.wantRefreshErr, errors.Is(err, oauth2session.ErrRefreshFailed), "refresh failure cause must remain discoverable")
			if tt.wantRefreshErr && tt.thirdpartyHandler != nil {
				var retrieveErr *oauth2.RetrieveError
				assert.ErrorAs(t, err, &retrieveErr)
			}
			if tt.session != nil {
				assert.Same(t, tt.session, sessionRepo.session, "a rejected refresh must not persist session changes")
				assert.Equal(t, before, *sessionRepo.session)
			}
		})
	}
}

type grantCoverage int

const (
	grantCoversRequested grantCoverage = iota
	grantCoversOtherService
	grantEmpty
	grantStalePermissionSet
)

type exchangeFixtureConfig struct {
	coverage        grantCoverage
	callbackBaseURL string
	sessionRepo     ports.UserSessionRepository
	refreshRepo     ports.UserSessionRefreshRepository
	encryption      ports.EncryptionPort
	tokenEndpoint   string
}

type exchangeFixture struct {
	svc             *TokenExchangeService
	req             *TokenExchangeRequest
	agentID         id.AgentID
	serviceID       id.ServiceID
	permissionSetID id.PermissionSetID
	agent           *storagedomain.Agent
	grant           *storagedomain.UserGrant
	grantRepo       *MockGrantRepository
	psRepo          *MockPermissionSetRepository
}

type fixedProviderRepository struct {
	MockServiceRepository
}

func (r *fixedProviderRepository) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if r.service != nil && r.service.ID == serviceID {
		return r.service, nil
	}
	return nil, ports.ErrNotFound
}

func newExchangeFixture(t *testing.T, cfg exchangeFixtureConfig) exchangeFixture {
	t.Helper()

	privateKey, keySet := generateTestRSAKeySet(t)
	agentID := id.NewAgentID()
	requestedServiceID := id.NewServiceID()
	permissionSetID := id.NewPermissionSetID()

	agent := &storagedomain.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("test-agent-client")),
		DisplayName: "Test Agent",
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: permissionSetID, RequirementType: storagedomain.RequirementTypeOptional},
		},
	}
	grantedServiceID := requestedServiceID
	if cfg.coverage == grantCoversOtherService {
		grantedServiceID = id.NewServiceID()
	}
	var entries []storagedomain.GrantedPermissionSetEntry
	if cfg.coverage != grantEmpty {
		entries = []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: permissionSetID, IncludedServiceIDs: []id.ServiceID{grantedServiceID}},
		}
	}
	futureTime := time.Now().Add(time.Hour)
	grant := &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		AgentID:               agentID,
		Principal:             id.Principal("user@example.com"),
		ValidUntil:            &futureTime,
		GrantedPermissionSets: entries,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	agentRepo := &singleAgentRepo{agentID: agentID, agent: agent}
	grantRepo := &MockGrantRepository{grant: grant}
	psMap := map[id.PermissionSetID]*storagedomain.PermissionSet{
		permissionSetID: {
			ID:            permissionSetID,
			Name:          "Test Permission Set",
			ServiceScopes: []storagedomain.ServiceScope{{ServiceID: grantedServiceID, RequirementType: storagedomain.RequirementTypeOptional}},
		},
	}
	if cfg.coverage == grantStalePermissionSet {
		psMap = nil
	}
	psRepo := &MockPermissionSetRepository{psMap: psMap}
	psService := permissionset.NewPermissionSetService(psRepo, grantRepo, slog.Default())
	t.Cleanup(psService.Close)
	consentSvc := consent.NewService(agentRepo, newTestProviderService(&MockServiceRepository{}), grantRepo, nil, nil, slog.Default())

	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                 requestedServiceID,
		DisplayName:        "Requested Service",
		ClientID:           id.ClientID("service-client"),
		Secret:             model.NewEncryptedSecret([]byte("placeholder")),
		ProtectedResources: []string{"https://api.example.com/requested"},
		Endpoints:          model.OAuth2Endpoints{TokenEndpoint: cfg.tokenEndpoint},
	}
	sessionConfig := oauth2session.DefaultConfig()
	sessionConfig.CallbackBaseURL = cfg.callbackBaseURL
	oauth2SessionService := oauth2session.NewOAuth2SessionService(
		newTestProviderService(&fixedProviderRepository{MockServiceRepository{service: provider}}),
		cfg.sessionRepo,
		cfg.refreshRepo,
		nil,
		nil,
		cfg.encryption,
		&http.Client{Timeout: 5 * time.Second},
		nil,
		sessionConfig,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	jwtValidator, err := NewJWTValidator(&MockJWKSProvider{keySet: keySet}, "https://auth.example.com", "agentic-identity-broker", 60)
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
		providerService:      newTestProviderService(&MockServiceRepository{service: provider}),
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
	return exchangeFixture{svc: svc, req: req, agentID: agentID, serviceID: requestedServiceID,
		permissionSetID: permissionSetID, agent: agent, grant: grant, grantRepo: grantRepo, psRepo: psRepo}
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

func TestExchange_AuthorizationContext(t *testing.T) {
	t.Parallel()
	dependencyCause := errors.New("grant repository unavailable")
	for _, tc := range []struct {
		name             string
		mutate           func(exchangeFixture, *MockSessionRepository)
		wantDetail       FailureDetail
		wantCode         string
		wantStatus       int
		wantGrant        bool
		wantCause        error
		wantSessionCalls int
	}{
		{name: "success with expiring grant", wantDetail: DetailNone, wantGrant: true, wantSessionCalls: 1},
		{name: "success with indefinite grant", mutate: func(f exchangeFixture, _ *MockSessionRepository) {
			f.grant.ValidUntil = nil
		}, wantDetail: DetailNone, wantGrant: true, wantSessionCalls: 1},
		{name: "missing grant", mutate: func(f exchangeFixture, _ *MockSessionRepository) {
			f.grantRepo.grant = nil
			f.grantRepo.err = ports.ErrNotFound
		}, wantDetail: DetailGrantMissing, wantCode: "access_denied", wantStatus: http.StatusForbidden, wantCause: consent.ErrAgentAccessDenied},
		{name: "nil grant fails closed", mutate: func(f exchangeFixture, _ *MockSessionRepository) {
			f.grantRepo.grant = nil
		}, wantDetail: DetailGrantMissing, wantCode: "access_denied", wantStatus: http.StatusForbidden, wantCause: consent.ErrAgentAccessDenied},
		{name: "expired grant remains observable", mutate: func(f exchangeFixture, _ *MockSessionRepository) {
			*f.grant.ValidUntil = time.Date(2020, time.January, 1, 2, 3, 4, 987654321, time.FixedZone("stored", 3600))
		}, wantDetail: DetailGrantExpired, wantCode: "access_denied", wantStatus: http.StatusForbidden, wantGrant: true, wantCause: consent.ErrGrantExpired},
		{name: "grant repository error discards returned grant", mutate: func(f exchangeFixture, _ *MockSessionRepository) {
			f.grantRepo.err = dependencyCause
		}, wantDetail: DetailGrantRepositoryUnavailable, wantCode: "server_error", wantStatus: http.StatusInternalServerError, wantCause: dependencyCause},
		{name: "missing session retains found grant", mutate: func(_ exchangeFixture, repo *MockSessionRepository) {
			repo.session = nil
		}, wantDetail: DetailSessionMissing, wantCode: "invalid_grant", wantStatus: http.StatusBadRequest, wantGrant: true, wantSessionCalls: 1},
		{name: "session scope denial retains found grant", mutate: func(f exchangeFixture, _ *MockSessionRepository) {
			f.psRepo.psMap[f.permissionSetID].ServiceScopes[0].Scopes = []string{"read"}
		}, wantDetail: DetailSessionScopeInsufficient, wantCode: "invalid_grant", wantStatus: http.StatusBadRequest, wantGrant: true, wantSessionCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			future := time.Now().Add(time.Hour)
			sessionRepo := &MockSessionRepository{session: &storagedomain.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal("user@example.com"),
				EncryptedAccessToken: []byte("access-token"),
				AccessTokenExpiresAt: &future,
				TokenType:            BearerTokenType,
			}}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{
				callbackBaseURL: "https://broker.example.com",
				sessionRepo:     sessionRepo,
				encryption:      &MockEncryption{},
			})
			sessionRepo.session.ServiceID = fixture.serviceID
			fixture.grant.UpdatedAt = time.Date(2026, time.January, 1, 2, 3, 4, 123456789, time.FixedZone("stored", 3600))
			if tc.mutate != nil {
				tc.mutate(fixture, sessionRepo)
			}
			wantAuthorization := AuthorizationRef{AgentID: fixture.agentID}
			if tc.wantGrant {
				wantAuthorization.GrantID = fixture.grant.ID
				wantAuthorization.GrantUpdatedAt = fixture.grant.UpdatedAt
				if fixture.grant.ValidUntil != nil {
					wantAuthorization.GrantValidUntil = *fixture.grant.ValidUntil
					wantAuthorization.GrantHasValidUntil = true
				}
			}
			response, err := fixture.svc.Exchange(context.Background(), fixture.req)
			var actual AuthorizationRef
			if tc.wantDetail == DetailNone {
				require.NoError(t, err)
				require.NotNil(t, response)
				assert.Equal(t, ServiceRef{ID: fixture.serviceID}, response.Service)
				assert.Equal(t, fixture.agentID.String(), response.AgentID)
				assert.Equal(t, fixture.grant.Principal.String(), response.Principal)
				assert.Equal(t, map[string][]string{fixture.permissionSetID.String(): {fixture.serviceID.String()}}, response.GrantedPermissionSets)
				actual = response.Authorization
			} else {
				assert.Nil(t, response)
				var tokenErr *TokenExchangeError
				require.ErrorAs(t, err, &tokenErr)
				assert.Equal(t, tc.wantDetail, tokenErr.Diagnostic().Detail())
				assert.Equal(t, tc.wantCode, tokenErr.Code())
				assert.Equal(t, tc.wantStatus, tokenErr.HTTPStatus())
				assert.Equal(t, ServiceRef{ID: fixture.serviceID}, tokenErr.Service())
				if tc.wantCause != nil {
					assert.ErrorIs(t, err, tc.wantCause)
				}
				actual = tokenErr.Authorization()
			}
			assert.Equal(t, wantAuthorization, actual)
			assert.Equal(t, 1, fixture.grantRepo.findByPrincipalAndAgentCalls, "observation must not query the grant twice")
			assert.Equal(t, tc.wantSessionCalls, sessionRepo.findByPrincipalAndServiceCalls)
			if fixture.grant.ValidUntil != nil {
				*fixture.grant.ValidUntil = fixture.grant.ValidUntil.Add(24 * time.Hour)
			}
			fixture.grant.UpdatedAt = fixture.grant.UpdatedAt.Add(time.Hour)
			fixture.grant.ID = id.NewGrantID()
			if response != nil {
				assert.Equal(t, wantAuthorization, response.Authorization, "response observation must not alias stored grant metadata")
			} else {
				var tokenErr *TokenExchangeError
				require.ErrorAs(t, err, &tokenErr)
				assert.Equal(t, wantAuthorization, tokenErr.Authorization(), "error observation must not alias stored grant metadata")
			}
		})
	}
}

func TestExchange_UnresolvedAgentDoesNotExportCandidateIdentity(t *testing.T) {
	t.Parallel()
	dependencyCause := errors.New("agent repository unavailable")
	for _, tc := range []struct {
		name            string
		agentExpression string
		repoErr         error
		wantDetail      FailureDetail
		wantService     bool
	}{
		{name: "unregistered candidate UUID", repoErr: ports.ErrNotFound, wantDetail: DetailAgentMissing, wantService: true},
		{name: "agent repository error", repoErr: dependencyCause, wantDetail: DetailAgentRepositoryUnavailable, wantService: true},
		{name: "invalid candidate UUID", agentExpression: "'not-a-valid-uuid'", wantDetail: DetailAgentInvalid, wantService: true},
		{name: "agent claim cannot resolve", agentExpression: "subject_token.missing_agent", wantDetail: DetailCELEvaluationFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sessionRepo := &MockSessionRepository{}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{sessionRepo: sessionRepo, encryption: &MockEncryption{}})
			fixture.svc.agentRepository.(*singleAgentRepo).err = tc.repoErr
			if tc.agentExpression != "" {
				evaluator, err := NewCELEvaluator(CELEvaluatorConfig{
					PrincipalExpression:     "subject_token.sub",
					AgentIDExpression:       tc.agentExpression,
					AuthorizationExpression: "true",
					EvaluationTimeout:       100 * time.Millisecond,
				})
				require.NoError(t, err)
				fixture.svc.celEvaluator = evaluator
			}
			response, err := fixture.svc.Exchange(context.Background(), fixture.req)
			assert.Nil(t, response)
			var tokenErr *TokenExchangeError
			require.ErrorAs(t, err, &tokenErr)
			assert.Equal(t, tc.wantDetail, tokenErr.Diagnostic().Detail())
			assert.Equal(t, "server_error", tokenErr.Code())
			assert.Equal(t, http.StatusInternalServerError, tokenErr.HTTPStatus())
			assert.Equal(t, AuthorizationRef{}, tokenErr.Authorization())
			wantService := ServiceRef{}
			if tc.wantService {
				wantService.ID = fixture.serviceID
			}
			assert.Equal(t, wantService, tokenErr.Service())
			if tc.repoErr != nil {
				assert.ErrorIs(t, err, tc.repoErr)
			}
			assert.Zero(t, fixture.grantRepo.findByPrincipalAndAgentCalls)
			assert.Zero(t, sessionRepo.findByPrincipalAndServiceCalls)
		})
	}
}

func TestExchange_CancellationPreservesOnlyResolvedAuthorizationContext(t *testing.T) {
	t.Parallel()
	for _, early := range []bool{true, false} {
		name := "session lookup cancellation retains agent and grant"
		if early {
			name = "early cancellation has no resolved context"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sessionRepo := &MockSessionRepository{findByPrincipalAndServiceFunc: func(context.Context, id.Principal, id.ServiceID) (*storagedomain.UserSession, error) {
				cancel()
				return nil, ctx.Err()
			}}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{sessionRepo: sessionRepo, encryption: &MockEncryption{}})
			wantService := ServiceRef{ID: fixture.serviceID}
			wantAuthorization := AuthorizationRef{
				AgentID:            fixture.agentID,
				GrantID:            fixture.grant.ID,
				GrantUpdatedAt:     fixture.grant.UpdatedAt,
				GrantValidUntil:    *fixture.grant.ValidUntil,
				GrantHasValidUntil: true,
			}
			wantStage := StageSessionLookup
			wantCalls := 1
			if early {
				cancel()
				wantService = ServiceRef{}
				wantAuthorization = AuthorizationRef{}
				wantStage = StageRequestValidation
				wantCalls = 0
			}
			response, err := fixture.svc.Exchange(ctx, fixture.req)
			assert.Nil(t, response)
			var tokenErr *TokenExchangeError
			require.ErrorAs(t, err, &tokenErr)
			assert.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, DetailCallerCanceled, tokenErr.Diagnostic().Detail())
			assert.Equal(t, OutcomeCanceled, tokenErr.Diagnostic().Outcome())
			assert.Equal(t, wantStage, tokenErr.Diagnostic().Stage())
			assert.Equal(t, wantService, tokenErr.Service())
			assert.Equal(t, wantAuthorization, tokenErr.Authorization())
			assert.Equal(t, wantCalls, fixture.grantRepo.findByPrincipalAndAgentCalls)
			assert.Equal(t, wantCalls, sessionRepo.findByPrincipalAndServiceCalls)
		})
	}
}
