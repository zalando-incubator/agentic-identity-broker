package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// MockRepository mocks ports.ThirdpartyOAuth2ProviderRepository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx, serviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx, canonicalID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockRepository) GetCanonicalIDs(ctx context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error) {
	args := m.Called(ctx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[id.ServiceID]string), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	args := m.Called(ctx, entity, expectedVersion)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, serviceID id.ServiceID) error {
	args := m.Called(ctx, serviceID)
	return args.Error(0)
}

func (m *MockRepository) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockRepository) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx, resourceURI)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockRepository) AddProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	args := m.Called(ctx, serviceID, resourceURI)
	return args.Get(0).(ports.ProtectedResourceMutationResult), args.Error(1)
}

func (m *MockRepository) RemoveProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	args := m.Called(ctx, serviceID, resourceURI)
	return args.Get(0).(ports.ProtectedResourceMutationResult), args.Error(1)
}

func (m *MockRepository) RenameProtectedResource(ctx context.Context, serviceID id.ServiceID, fromURI, toURI string) (ports.ProtectedResourceMutationResult, error) {
	args := m.Called(ctx, serviceID, fromURI, toURI)
	return args.Get(0).(ports.ProtectedResourceMutationResult), args.Error(1)
}

func (m *MockRepository) ListProtectedResources(ctx context.Context, serviceID id.ServiceID) ([]string, int64, error) {
	args := m.Called(ctx, serviceID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]string), args.Get(1).(int64), args.Error(2)
}

// MockEncryption mocks ports.EncryptionPort
type MockEncryption struct {
	mock.Mock
}

func (m *MockEncryption) Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	args := m.Called(ctx, plaintext, encryptionContext)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockEncryption) Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error) {
	args := m.Called(ctx, ciphertext, encryptionContext)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// MockBranchKeyManager mocks ports.BranchKeyManager
type MockBranchKeyManager struct {
	mock.Mock
}

func (m *MockBranchKeyManager) Create(ctx context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	args := m.Called(ctx, subject)
	return args.String(0), args.Error(1)
}

type noopBranchKeyManager struct{}

func newNoopBranchKeyManager() *noopBranchKeyManager {
	return &noopBranchKeyManager{}
}

func (m *noopBranchKeyManager) Create(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

type functionFieldProviderRepository struct {
	ports.ThirdpartyOAuth2ProviderRepository

	createFn func(context.Context, *model.ThirdpartyOAuth2ProviderEntity) error
	getFn    func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
	updateFn func(context.Context, *model.ThirdpartyOAuth2ProviderEntity, *int64) error
	listFn   func(context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error)
}

func (m *functionFieldProviderRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	if m.createFn == nil {
		return errors.New("unexpected repository Create call")
	}
	return m.createFn(ctx, entity)
}

func (m *functionFieldProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if m.getFn == nil {
		return nil, errors.New("unexpected repository Get call")
	}
	return m.getFn(ctx, serviceID)
}

func (m *functionFieldProviderRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	if m.updateFn == nil {
		return errors.New("unexpected repository Update call")
	}
	return m.updateFn(ctx, entity, expectedVersion)
}

func (m *functionFieldProviderRepository) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	if m.listFn == nil {
		return nil, errors.New("unexpected repository List call")
	}
	return m.listFn(ctx)
}

type functionFieldEncryption struct {
	encryptFn func(context.Context, []byte, map[string]string) ([]byte, error)
	decryptFn func(context.Context, []byte, map[string]string) ([]byte, error)
}

func (m *functionFieldEncryption) Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	if m.encryptFn == nil {
		return nil, errors.New("unexpected encryption Encrypt call")
	}
	return m.encryptFn(ctx, plaintext, encryptionContext)
}

func (m *functionFieldEncryption) Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error) {
	if m.decryptFn == nil {
		return nil, errors.New("unexpected encryption Decrypt call")
	}
	return m.decryptFn(ctx, ciphertext, encryptionContext)
}

type functionFieldBranchKeyManager struct {
	createFn func(context.Context, domainencryption.BranchKeySubject) (string, error)
}

func (m *functionFieldBranchKeyManager) Create(ctx context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	if m.createFn == nil {
		return "", errors.New("unexpected branch key Create call")
	}
	return m.createFn(ctx, subject)
}

// minimalValidEntity returns the smallest ThirdpartyOAuth2ProviderEntity that passes
// ValidateForCreate. Use this as the base for tests focused on service behavior
// (encryption, branch keys, storage) rather than validation logic.
func serviceSubject(serviceID id.ServiceID) domainencryption.BranchKeySubject {
	return domainencryption.NewServiceBranchKeySubject(serviceID)
}

func minimalValidEntity(svcID id.ServiceID, secret model.Secret) *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: "Test Provider",
		ClientID:    id.ClientID("test-client-id"),
		Secret:      secret,
		IssuerURI:   "https://issuer.example.com",
		Discovery:   model.DiscoveryConfig{EnableDiscovery: true},
		Scopes:      []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}},
	}
}

func TestNewThirdpartyOAuth2ProviderService_PanicsOnNilRequiredDependencies(t *testing.T) {
	validRepo := new(MockRepository)
	validEncryption := new(MockEncryption)
	validBranchKeyManager := newNoopBranchKeyManager()

	t.Run("nil repo", func(t *testing.T) {
		assert.PanicsWithValue(t,
			"thirdparty.NewThirdpartyOAuth2ProviderService: repo must not be nil",
			func() {
				NewThirdpartyOAuth2ProviderService(nil, validEncryption, validBranchKeyManager, nil, false, slog.Default())
			},
		)
	})

	t.Run("nil encryption", func(t *testing.T) {
		assert.PanicsWithValue(t,
			"thirdparty.NewThirdpartyOAuth2ProviderService: encryption must not be nil",
			func() {
				NewThirdpartyOAuth2ProviderService(validRepo, nil, validBranchKeyManager, nil, false, slog.Default())
			},
		)
	})

	t.Run("nil branch key manager", func(t *testing.T) {
		assert.PanicsWithValue(t,
			"thirdparty.NewThirdpartyOAuth2ProviderService: branchKeyManager must not be nil",
			func() {
				NewThirdpartyOAuth2ProviderService(validRepo, validEncryption, nil, nil, false, slog.Default())
			},
		)
	})
}

// =============================================================================
// Validation tests (ValidateForCreate / ValidateForUpdate)
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Create_ValidationRejectsBeforeIDGeneration(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	// Entity missing required DisplayName — validation must reject before ID is assigned
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     id.ServiceID{}, // deliberately zero to observe ID generation behavior
		Secret: model.NewPlaintextSecret("secret"),
	}

	err := svc.Create(ctx, entity)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider validation failed")
	assert.True(t, entity.ID.IsZero(), "ID must NOT be generated when validation fails")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Create")
}

func TestThirdpartyOAuth2ProviderService_Create_ValidationRejectsBeforeBranchKey(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockBKM := new(MockBranchKeyManager)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, mockBKM, nil, false, slog.Default())

	ctx := context.Background()
	// Invalid entity (missing DisplayName) — branch key must NOT be provisioned
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     id.NewServiceID(),
		Secret: model.NewPlaintextSecret("secret"),
	}

	err := svc.Create(ctx, entity)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider validation failed")
	mockBKM.AssertNotCalled(t, "Create")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Create")
}

func TestThirdpartyOAuth2ProviderService_Create_HTTPIssuerRejectedByDefault(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("secret"))
	entity.IssuerURI = "http://issuer.example.com" // HTTP non-localhost

	err := svc.Create(ctx, entity)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider validation failed")
	mockEnc.AssertNotCalled(t, "Encrypt")
}

func TestThirdpartyOAuth2ProviderService_Create_HTTPIssuerAllowedWithSkipHTTPS(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, true, slog.Default())

	ctx := context.Background()
	entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("secret"))
	entity.IssuerURI = "http://issuer.example.com" // HTTP non-localhost, allowed in dev

	mockEnc.On("Encrypt", ctx, mock.Anything, mock.Anything).Return([]byte("enc"), nil)
	mockRepo.On("Create", ctx, mock.Anything).Return(nil)

	err := svc.Create(ctx, entity)

	require.NoError(t, err)
	mockEnc.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Update_ValidationRejectsBeforeEncryption(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	// Entity missing required DisplayName — encryption must NOT be attempted
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     id.NewServiceID(),
		Secret: model.NewPlaintextSecret("new-secret"),
	}

	err := svc.Update(ctx, entity, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider validation failed")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Update")
}

// =============================================================================
// Create tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Create_EncryptsAndStores(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("supersecret"))

	expectedEncContext := map[string]string{"service_id": entity.ID.String()}
	mockEnc.On("Encrypt", ctx, []byte("supersecret"), expectedEncContext).
		Return([]byte("encrypted-bytes"), nil)
	mockRepo.On("Create", ctx, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
		ct, err := e.Secret.GetCiphertext()
		return err == nil && e.Secret.IsEncrypted() && string(ct) == "encrypted-bytes"
	})).Return(nil)

	err := svc.Create(ctx, entity)

	require.NoError(t, err)
	// Secret must be in encrypted state after Create
	assert.True(t, entity.Secret.IsEncrypted())
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Create_NormalizesProtectedResourcesBeforePersist(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("supersecret"))
	entity.ProtectedResources = []string{
		"https://api.example.com/",
		"https://api.example.com/v1///",
	}

	mockEnc.On("Encrypt", ctx, []byte("supersecret"), map[string]string{"service_id": entity.ID.String()}).
		Return([]byte("encrypted-bytes"), nil)
	mockRepo.On("Create", ctx, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
		return e.Secret.IsEncrypted() &&
			assert.ObjectsAreEqual([]string{
				"https://api.example.com",
				"https://api.example.com/v1",
			}, e.ProtectedResources)
	})).Return(nil)

	err := svc.Create(ctx, entity)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://api.example.com",
		"https://api.example.com/v1",
	}, entity.ProtectedResources)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Create_GeneratesIDIfEmpty(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	entity := minimalValidEntity(id.ServiceID{}, model.NewPlaintextSecret("my-secret"))

	mockEnc.On("Encrypt", ctx, []byte("my-secret"), mock.MatchedBy(func(ec map[string]string) bool {
		sid, ok := ec["service_id"]
		return ok && sid != ""
	})).Return([]byte("enc"), nil)
	mockRepo.On("Create", ctx, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
		return !e.ID.IsZero()
	})).Return(nil)

	err := svc.Create(ctx, entity)

	require.NoError(t, err)
	assert.False(t, entity.ID.IsZero(), "ID should be generated")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Create_WithBranchKeyManager(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockBKM := new(MockBranchKeyManager)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, mockBKM, nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("secret"))

	callOrder := make([]string, 0)
	mockBKM.On("Create", ctx, serviceSubject(svcID)).Return("bk-1", nil).Run(func(_ mock.Arguments) {
		callOrder = append(callOrder, "branch_key")
	})
	mockEnc.On("Encrypt", ctx, mock.Anything, mock.Anything).Return([]byte("enc"), nil).Run(func(_ mock.Arguments) {
		callOrder = append(callOrder, "encrypt")
	})
	mockRepo.On("Create", ctx, mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
		callOrder = append(callOrder, "repo_create")
	})

	err := svc.Create(ctx, entity)

	require.NoError(t, err)
	assert.Equal(t, []string{"branch_key", "encrypt", "repo_create"}, callOrder,
		"branch key must be provisioned before encryption and repo storage")
	mockBKM.AssertExpectations(t)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Create_BranchKeyFailure_AbortCreate(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockBKM := new(MockBranchKeyManager)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, mockBKM, nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("secret"))

	mockBKM.On("Create", ctx, serviceSubject(svcID)).Return("", errors.New("KMS unavailable"))

	err := svc.Create(ctx, entity)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "branch key provisioning failed")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Create")
}

func TestThirdpartyOAuth2ProviderService_Create_EncryptionFailure(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("secret"))

	mockEnc.On("Encrypt", ctx, mock.Anything, mock.Anything).Return(nil, errors.New("KMS error"))

	err := svc.Create(ctx, entity)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encrypt")
	mockRepo.AssertNotCalled(t, "Create")
}

func TestThirdpartyOAuth2ProviderService_Create_EncryptedSecretFails(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	// Entity with encrypted secret — ValidateForCreate requires plaintext state
	entity := minimalValidEntity(id.NewServiceID(), model.NewEncryptedSecret([]byte("already-encrypted")))

	err := svc.Create(ctx, entity)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client_secret is required for create")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Create")
}

func TestThirdpartyOAuth2ProviderService_Create_PublicClient_ProvisionsBranchKeyAndStoresAbsentSecret(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	entity := minimalValidEntity(serviceID, model.NewAbsentSecret())
	entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	entity.AuthorizationParams = map[string]string{"audience": "sentinel-client-secret"}

	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{
		createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
			stored = provider
			return nil
		},
	}
	var branchKeySubjects []domainencryption.BranchKeySubject
	branchKeyManager := &functionFieldBranchKeyManager{
		createFn: func(_ context.Context, subject domainencryption.BranchKeySubject) (string, error) {
			branchKeySubjects = append(branchKeySubjects, subject)
			return "public-service-key", nil
		},
	}
	logOutput := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, branchKeyManager, nil, false, slog.New(slog.NewJSONHandler(logOutput, nil)))

	require.NoError(t, service.Create(ctx, entity))
	require.NotNil(t, stored)
	assert.True(t, entity.Secret.IsAbsent())
	assert.True(t, stored.Secret.IsAbsent())
	assert.Equal(t, []domainencryption.BranchKeySubject{serviceSubject(serviceID)}, branchKeySubjects)
	assertPublicClientAuditEvent(t, logOutput, "service.thirdparty.provider_created", serviceID, "sentinel-client-secret")
}

func assertPublicClientAuditEvent(t *testing.T, logs *bytes.Buffer, event string, serviceID id.ServiceID, sentinels ...string) {
	t.Helper()

	var auditEvent map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["event"] == event {
			auditEvent = entry
			break
		}
	}

	require.NotNil(t, auditEvent, "expected %s audit event", event)
	assert.Equal(t, serviceID.String(), auditEvent["service_id"])
	assert.Equal(t, true, auditEvent["public_client"])
	for _, field := range []string{"client_secret", "client_assertion", "code", "code_verifier", "code_challenge", "access_token", "refresh_token"} {
		assert.NotContains(t, auditEvent, field)
	}
	for _, sentinel := range sentinels {
		assert.NotContains(t, logs.String(), sentinel)
	}
}

func brokerHostedCIMDClientID(serviceID id.ServiceID) id.ClientID {
	return id.ClientID("https://broker.example/.well-known/oauth-client/" + serviceID.String())
}

func minimalValidCIMDProvider(
	serviceID id.ServiceID,
	_ id.ClientID,
	authMethod model.TokenEndpointAuthMethod,
) *model.ThirdpartyOAuth2ProviderEntity {
	provider := minimalValidEntity(serviceID, model.NewAbsentSecret())
	provider.ClientID = ""
	provider.TokenEndpointAuthMethod = authMethod
	provider.Discovery = model.DiscoveryConfig{EnableDiscovery: false}
	provider.Endpoints = model.OAuth2Endpoints{
		TokenEndpoint:     "https://issuer.example.com/oauth/token",
		AuthorizeEndpoint: "https://issuer.example.com/oauth/authorize",
	}
	return provider
}

type readyCIMDKeyReadiness struct{}

func (readyCIMDKeyReadiness) RequireUsablePublishedKey(context.Context) error { return nil }

type unavailableCIMDKeyReadiness struct{}

func (unavailableCIMDKeyReadiness) RequireUsablePublishedKey(context.Context) error {
	return ports.ErrCIMDPublicKeyUnavailable
}

func assertCIMDProviderAuditRecord(t *testing.T, logs *bytes.Buffer, serviceID id.ServiceID, operation, wantOutcome string, sentinels ...string) {
	t.Helper()

	var auditRecord map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}

		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["service_id"] == serviceID.String() && entry["operation"] == operation && entry["outcome"] == wantOutcome {
			auditRecord = entry
			break
		}
	}

	require.NotNil(t, auditRecord, "expected %s/%s audit record for service %s", operation, wantOutcome, serviceID)
	for _, field := range []string{
		"client_secret",
		"secret",
		"ciphertext",
		"private_key",
		"private_key_encrypted",
		"client_assertion",
		"authorization_code",
		"code",
		"code_verifier",
		"code_challenge",
		"access_token",
		"refresh_token",
		"token",
	} {
		assert.NotContains(t, auditRecord, field)
	}
	for _, sentinel := range sentinels {
		assert.NotContains(t, logs.String(), sentinel)
	}
}

func assertCredentialFreeProviderUpdateEvent(t *testing.T, logs *bytes.Buffer, serviceID id.ServiceID, publicClient bool) {
	t.Helper()

	var auditEvent map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}

		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["event"] == "service.thirdparty.provider_updated" && entry["service_id"] == serviceID.String() {
			auditEvent = entry
			break
		}
	}

	require.NotNil(t, auditEvent, "expected update audit event for service %s", serviceID)
	assert.Equal(t, serviceID.String(), auditEvent["service_id"])
	assert.Equal(t, publicClient, auditEvent["public_client"])
	for _, field := range []string{
		"client_secret",
		"secret",
		"ciphertext",
		"private_key",
		"private_key_encrypted",
		"client_assertion",
		"authorization_code",
		"code",
		"access_token",
		"refresh_token",
		"token",
	} {
		assert.NotContains(t, auditEvent, field)
	}
}

// =============================================================================
// CIMD confidential-client lifecycle tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Create_CIMDValidatesCompleteConfigurationBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	authMethod := model.TokenEndpointAuthMethod("private_key_jwt")
	entity := minimalValidCIMDProvider(serviceID, brokerHostedCIMDClientID(serviceID), authMethod)
	entity.Endpoints.TokenEndpoint = "http://issuer.example.com/oauth/token"
	entity.AuthorizationParams = map[string]string{"audience": "sentinel-client-assertion"}

	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockBKM := new(MockBranchKeyManager)
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(
		mockRepo,
		mockEnc,
		mockBKM,
		nil,
		false,
		slog.New(slog.NewJSONHandler(logs, nil)),
	)

	err := service.Create(ctx, entity)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_endpoint")
	assert.True(t, entity.Secret.IsAbsent(), "validation must not introduce a shared secret")
	mockBKM.AssertNotCalled(t, "Create")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Create")
	assertCIMDProviderAuditRecord(t, logs, serviceID, "create", "rejected", "sentinel-client-assertion")
}

func TestThirdpartyOAuth2ProviderService_CreateAndGet_CIMDUsesAllocatedIdentityWithoutSecretCrypto(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	generatedClientID := brokerHostedCIMDClientID(serviceID)
	authMethod := model.TokenEndpointAuthMethod("private_key_jwt")
	entity := minimalValidCIMDProvider(serviceID, generatedClientID, authMethod)
	entity.AuthorizationParams = map[string]string{"audience": "sentinel-client-assertion"}

	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{
		createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
			stored = provider.Copy()
			return nil
		},
		getFn: func(_ context.Context, requestedID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
			assert.Equal(t, serviceID, requestedID)
			return stored.Copy(), nil
		},
	}
	var encryptCalls, decryptCalls int
	encryption := &functionFieldEncryption{
		encryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			encryptCalls++
			return nil, errors.New("CIMD services must not encrypt an absent secret")
		},
		decryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			decryptCalls++
			return nil, errors.New("CIMD services must not decrypt an absent secret")
		},
	}
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(
		repo,
		encryption,
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(logs, nil)),
	).WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readyCIMDKeyReadiness{})

	require.NoError(t, service.Create(ctx, entity))
	require.NotNil(t, stored)
	assert.Equal(t, serviceID, stored.ID)
	assert.Equal(t, brokerHostedCIMDClientID(stored.ID), stored.ClientID, "broker identity must be derived after service-ID allocation")
	assert.Equal(t, generatedClientID, stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
	assert.Zero(t, encryptCalls)
	assertCIMDProviderAuditRecord(t, logs, serviceID, "create", "success", "sentinel-client-assertion")

	provider, err := service.Get(ctx, serviceID)

	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.Equal(t, generatedClientID, provider.ClientID)
	assert.True(t, provider.Secret.IsAbsent())
	assert.Zero(t, decryptCalls)
}

func TestThirdpartyOAuth2ProviderService_CIMDProvisionsServiceBranchKeyBeforePersistence(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			serviceID := id.NewServiceID()
			entity := minimalValidCIMDProvider(serviceID, "", model.TokenEndpointAuthMethodPrivateKeyJWT)
			var calls []string
			repo := &functionFieldProviderRepository{
				getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
					return minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("previous-secret"))), nil
				},
				createFn: func(_ context.Context, stored *model.ThirdpartyOAuth2ProviderEntity) error {
					calls = append(calls, "persist")
					assert.True(t, stored.Secret.IsAbsent())
					return nil
				},
				updateFn: func(_ context.Context, stored *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
					calls = append(calls, "persist")
					assert.True(t, stored.Secret.IsAbsent())
					return nil
				},
			}
			branchKeys := &functionFieldBranchKeyManager{createFn: func(_ context.Context, subject domainencryption.BranchKeySubject) (string, error) {
				assert.Equal(t, serviceSubject(serviceID), subject)
				calls = append(calls, "provision")
				return "service-key", nil
			}}
			service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, branchKeys, nil, false, slog.Default()).WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readyCIMDKeyReadiness{})
			var err error
			if operation == "create" {
				err = service.Create(ctx, entity)
			} else {
				err = service.Update(ctx, entity, nil)
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"provision", "persist"}, calls)
		})
	}
}

func TestThirdpartyOAuth2ProviderService_CIMDRejectsUnavailableKeyBeforeSideEffects(t *testing.T) {
	for _, readiness := range []struct {
		name  string
		check ports.CIMDClientKeyReadiness
	}{
		{name: "missing"},
		{name: "unavailable", check: unavailableCIMDKeyReadiness{}},
	} {
		for _, operation := range []string{"create", "update"} {
			t.Run(readiness.name+"/"+operation, func(t *testing.T) {
				ctx := context.Background()
				serviceID := id.NewServiceID()
				entity := minimalValidCIMDProvider(serviceID, "", model.TokenEndpointAuthMethodPrivateKeyJWT)
				repo := new(MockRepository)
				if operation == "update" {
					persisted := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("previous-secret")))
					repo.On("Get", ctx, serviceID).Return(persisted, nil).Once()
				}
				encryption := new(MockEncryption)
				branchKeys := new(MockBranchKeyManager)
				logs := new(bytes.Buffer)
				service := NewThirdpartyOAuth2ProviderService(repo, encryption, branchKeys, nil, false, slog.New(slog.NewJSONHandler(logs, nil))).WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readiness.check)

				var err error
				if operation == "create" {
					err = service.Create(ctx, entity)
				} else {
					err = service.Update(ctx, entity, nil)
				}

				require.ErrorIs(t, err, ports.ErrCIMDPublicKeyUnavailable)
				assert.True(t, entity.Secret.IsAbsent())
				repo.AssertExpectations(t)
				repo.AssertNotCalled(t, "Create")
				repo.AssertNotCalled(t, "Update")
				branchKeys.AssertNotCalled(t, "Create")
				encryption.AssertNotCalled(t, "Encrypt")
				assertCIMDProviderAuditRecord(t, logs, serviceID, operation, "rejected")
			})
		}
	}
}

func TestThirdpartyOAuth2ProviderService_CIMDBranchKeyFailureRejectsWrite(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			serviceID := id.NewServiceID()
			entity := minimalValidCIMDProvider(serviceID, "", model.TokenEndpointAuthMethodPrivateKeyJWT)
			persisted := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("previous-secret")))
			repo := &functionFieldProviderRepository{getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return persisted, nil
			}}
			branchKeys := &functionFieldBranchKeyManager{createFn: func(_ context.Context, subject domainencryption.BranchKeySubject) (string, error) {
				assert.Equal(t, serviceSubject(serviceID), subject)
				return "", errors.New("key store unavailable")
			}}
			logs := new(bytes.Buffer)
			service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, branchKeys, nil, false, slog.New(slog.NewJSONHandler(logs, nil))).WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readyCIMDKeyReadiness{})
			var err error
			if operation == "create" {
				err = service.Create(ctx, entity)
			} else {
				err = service.Update(ctx, entity, nil)
			}
			require.ErrorContains(t, err, "branch key provisioning failed")
			assert.True(t, persisted.Secret.IsEncrypted())
			assertCIMDProviderAuditRecord(t, logs, serviceID, operation, "rejected")
		})
	}
}

func TestThirdpartyOAuth2ProviderService_CIMDRejectsCallerSuppliedIdentityBeforeSideEffects(t *testing.T) {
	serviceID := id.NewServiceID()
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			entity := minimalValidCIMDProvider(serviceID, "", model.TokenEndpointAuthMethodPrivateKeyJWT)
			entity.ClientID = id.ClientID("https://attacker.example/.well-known/oauth-client/" + serviceID.String())
			repo := new(MockRepository)
			encryption := new(MockEncryption)
			branchKeys := new(MockBranchKeyManager)
			service := NewThirdpartyOAuth2ProviderService(repo, encryption, branchKeys, nil, false, slog.Default()).WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readyCIMDKeyReadiness{})

			var err error
			if operation == "create" {
				err = service.Create(context.Background(), entity)
			} else {
				err = service.Update(context.Background(), entity, nil)
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "client_id must be absent")
			branchKeys.AssertNotCalled(t, "Create")
			encryption.AssertNotCalled(t, "Encrypt")
			repo.AssertNotCalled(t, "Create")
			repo.AssertNotCalled(t, "Get")
			repo.AssertNotCalled(t, "Update")
		})
	}
}

func TestThirdpartyOAuth2ProviderService_HasCompatibleCIMDServicesIgnoresOriginWithoutCIMDServices(t *testing.T) {
	serviceID := id.NewServiceID()
	repo := &functionFieldProviderRepository{
		listFn: func(context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
			return []*model.ThirdpartyOAuth2ProviderEntity{minimalValidEntity(serviceID, model.NewPlaintextSecret("secret"))}, nil
		},
	}
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.Default()).WithCIMDPublicURL("http://localhost:8000")

	hasCIMDServices, err := service.HasCompatibleCIMDServices(context.Background())

	require.NoError(t, err)
	assert.False(t, hasCIMDServices)
}

func TestThirdpartyOAuth2ProviderService_Update_CIMDValidatesReplacementBeforeRemovingStoredSecret(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	persisted := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("sentinel-existing-ciphertext")))
	authMethod := model.TokenEndpointAuthMethod("private_key_jwt")
	replacement := minimalValidCIMDProvider(serviceID, brokerHostedCIMDClientID(serviceID), authMethod)
	replacement.Endpoints.TokenEndpoint = ""
	replacement.AuthorizationParams = map[string]string{"audience": "sentinel-client-assertion"}

	var updateCalled bool
	repo := &functionFieldProviderRepository{
		updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
			updateCalled = true
			persisted = provider.Copy()
			return nil
		},
	}
	var encryptCalls int
	encryption := &functionFieldEncryption{
		encryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			encryptCalls++
			return nil, errors.New("CIMD replacement must not encrypt a secret")
		},
	}
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(
		repo,
		encryption,
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(logs, nil)),
	)

	err := service.Update(ctx, replacement, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "token_endpoint")
	assert.False(t, updateCalled, "invalid replacement must not clear the persisted shared secret")
	assert.True(t, persisted.Secret.IsEncrypted(), "the existing encrypted secret must remain stored")
	assert.Zero(t, encryptCalls)
	assertCIMDProviderAuditRecord(t, logs, serviceID, "update", "rejected", "sentinel-existing-ciphertext", "sentinel-client-assertion")
}

// =============================================================================
// CIMD authentication-posture transition lifecycle tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Update_StaticToCIMDReplacesEncryptedSecretWithAbsentState(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	persisted := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("existing-static-secret-ciphertext")))
	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{
		getFn: func(_ context.Context, requestedID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
			assert.Equal(t, serviceID, requestedID)
			return persisted, nil
		},
		updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
			stored = provider.Copy()
			return nil
		},
	}
	var encryptCalls, decryptCalls int
	encryption := &functionFieldEncryption{
		encryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			encryptCalls++
			return nil, errors.New("CIMD replacement must not encrypt a shared secret")
		},
		decryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			decryptCalls++
			return nil, errors.New("CIMD replacement must not decrypt a shared secret")
		},
	}
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(
		repo,
		encryption,
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(logs, nil)),
	).WithCIMDPublicURL("https://broker.example").WithCIMDKeyReadiness(readyCIMDKeyReadiness{})
	replacement := minimalValidCIMDProvider(
		serviceID,
		brokerHostedCIMDClientID(serviceID),
		model.TokenEndpointAuthMethodPrivateKeyJWT,
	)

	require.True(t, persisted.Secret.IsEncrypted())
	require.NoError(t, service.Update(ctx, replacement, nil))

	require.NotNil(t, stored)
	assert.Equal(t, model.TokenEndpointAuthMethodPrivateKeyJWT, stored.TokenEndpointAuthMethod)
	assert.Equal(t, brokerHostedCIMDClientID(serviceID), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent(), "CIMD replacements must remove the stored static secret")
	assert.Zero(t, encryptCalls)
	assert.Zero(t, decryptCalls)
	assertCIMDProviderAuditRecord(t, logs, serviceID, "update", "success")
}

func TestThirdpartyOAuth2ProviderService_Update_CIMDToStaticRequiresNewSecretAndEncryptsReplacement(t *testing.T) {
	t.Run("rejects absent or empty replacement secret before persistence", func(t *testing.T) {
		ctx := context.Background()
		serviceID := id.NewServiceID()
		persisted := minimalValidCIMDProvider(
			serviceID,
			brokerHostedCIMDClientID(serviceID),
			model.TokenEndpointAuthMethodPrivateKeyJWT,
		)
		var updateCalls, encryptCalls, branchKeyCalls int
		repo := &functionFieldProviderRepository{
			updateFn: func(_ context.Context, _ *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
				updateCalls++
				return nil
			},
		}
		encryption := &functionFieldEncryption{
			encryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
				encryptCalls++
				return nil, errors.New("missing static secret must reject before encryption")
			},
		}
		branchKeyManager := &functionFieldBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				branchKeyCalls++
				return "", errors.New("missing static secret must reject before branch-key provisioning")
			},
		}
		service := NewThirdpartyOAuth2ProviderService(repo, encryption, branchKeyManager, nil, false, slog.Default())

		for _, secret := range []model.Secret{model.NewAbsentSecret(), model.NewPlaintextSecret("")} {
			replacement := minimalValidEntity(serviceID, secret)

			err := service.Update(ctx, replacement, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "client_secret")
		}
		assert.True(t, persisted.Secret.IsAbsent(), "validation failure must not mutate the prior CIMD secret state")
		assert.Zero(t, updateCalls)
		assert.Zero(t, encryptCalls)
		assert.Zero(t, branchKeyCalls)
	})

	t.Run("encrypts the supplied static replacement secret", func(t *testing.T) {
		ctx := context.Background()
		serviceID := id.NewServiceID()
		persisted := minimalValidCIMDProvider(
			serviceID,
			brokerHostedCIMDClientID(serviceID),
			model.TokenEndpointAuthMethodPrivateKeyJWT,
		)
		var stored *model.ThirdpartyOAuth2ProviderEntity
		repo := &functionFieldProviderRepository{
			updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
				stored = provider.Copy()
				return nil
			},
		}
		var encryptedPlaintext []byte
		var encryptionContext map[string]string
		var decryptCalls int
		encryption := &functionFieldEncryption{
			encryptFn: func(_ context.Context, plaintext []byte, context map[string]string) ([]byte, error) {
				encryptedPlaintext = append([]byte(nil), plaintext...)
				encryptionContext = context
				return []byte("replacement-static-secret-ciphertext"), nil
			},
			decryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
				decryptCalls++
				return nil, errors.New("static replacement must not decrypt the removed CIMD secret")
			},
		}
		logs := new(bytes.Buffer)
		service := NewThirdpartyOAuth2ProviderService(
			repo,
			encryption,
			newNoopBranchKeyManager(),
			nil,
			false,
			slog.New(slog.NewJSONHandler(logs, nil)),
		)
		replacement := minimalValidEntity(serviceID, model.NewPlaintextSecret("replacement-static-secret"))

		require.True(t, persisted.Secret.IsAbsent())
		require.NoError(t, service.Update(ctx, replacement, nil))

		require.NotNil(t, stored)
		assert.True(t, stored.TokenEndpointAuthMethod.IsAbsent())
		assert.True(t, stored.Secret.IsEncrypted(), "static confidential replacements must persist only encrypted secret state")
		assert.Equal(t, []byte("replacement-static-secret"), encryptedPlaintext)
		assert.Equal(t, map[string]string{"service_id": serviceID.String()}, encryptionContext)
		assert.Zero(t, decryptCalls)
		assertCredentialFreeProviderUpdateEvent(t, logs, serviceID, false)
	})
}

func TestThirdpartyOAuth2ProviderService_Update_CIMDToPublicPersistsAbsentSecretWithoutCrypto(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	persisted := minimalValidCIMDProvider(
		serviceID,
		brokerHostedCIMDClientID(serviceID),
		model.TokenEndpointAuthMethodPrivateKeyJWT,
	)
	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{
		updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
			stored = provider.Copy()
			return nil
		},
	}
	var encryptCalls, decryptCalls int
	encryption := &functionFieldEncryption{
		encryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			encryptCalls++
			return nil, errors.New("public replacement must not encrypt a shared secret")
		},
		decryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
			decryptCalls++
			return nil, errors.New("public replacement must not decrypt a shared secret")
		},
	}
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(
		repo,
		encryption,
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(logs, nil)),
	)
	replacement := minimalValidEntity(serviceID, model.NewAbsentSecret())
	replacement.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone

	require.True(t, persisted.Secret.IsAbsent())
	require.NoError(t, service.Update(ctx, replacement, nil))

	require.NotNil(t, stored)
	assert.Equal(t, model.TokenEndpointAuthMethodNone, stored.TokenEndpointAuthMethod)
	assert.True(t, stored.Secret.IsAbsent(), "public replacements must persist no shared-secret state")
	assert.Zero(t, encryptCalls)
	assert.Zero(t, decryptCalls)
	assertCredentialFreeProviderUpdateEvent(t, logs, serviceID, true)
}

// =============================================================================
// Get tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Get_DecryptsSecret(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	storedEntity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: "GitHub",
		Secret:      model.NewEncryptedSecret([]byte("ciphertext")),
	}
	mockRepo.On("Get", ctx, svcID).Return(storedEntity, nil)
	mockEnc.On("Decrypt", ctx, []byte("ciphertext"), map[string]string{"service_id": svcID.String()}).
		Return([]byte("plaintext-secret"), nil)

	result, err := svc.Get(ctx, svcID)

	require.NoError(t, err)
	assert.Equal(t, svcID, result.ID)
	assert.Equal(t, "GitHub", result.DisplayName)
	assert.True(t, result.Secret.IsPlaintext())
	pt, err := result.Secret.GetPlaintext()
	require.NoError(t, err)
	assert.Equal(t, "plaintext-secret", pt)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Get_PublicClientReturnsAbsentWithoutDecryptWarning(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	stored := minimalValidEntity(serviceID, model.NewAbsentSecret())
	stored.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone

	var logs bytes.Buffer
	service := NewThirdpartyOAuth2ProviderService(
		&functionFieldProviderRepository{
			getFn: func(_ context.Context, requestedID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				assert.Equal(t, serviceID, requestedID)
				return stored, nil
			},
		},
		&functionFieldEncryption{},
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})),
	)

	provider, err := service.Get(ctx, serviceID)

	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.True(t, provider.Secret.IsAbsent())
	assert.Empty(t, logs.String(), "public clients must not cause a decrypt warning or error")
}

func TestThirdpartyOAuth2ProviderService_Get_NotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	nonexistentID := id.NewServiceID()
	mockRepo.On("Get", ctx, nonexistentID).Return(nil, errors.New("not found"))

	result, err := svc.Get(ctx, nonexistentID)

	assert.Error(t, err)
	assert.Nil(t, result)
	mockEnc.AssertNotCalled(t, "Decrypt")
}

// CRITICAL SECURITY TEST: cross-service token swap prevention via context binding
// Even though Get is graceful on decryption failure (returns entity with encrypted secret),
// the security property is maintained: the entity's Secret is NOT in plaintext state,
// so any caller attempting to use the secret (e.g. OAuth2SessionService) will fail
// because Secret.GetPlaintext() returns an error for encrypted secrets.
func TestThirdpartyOAuth2ProviderService_CrossServiceProtection(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()

	// Service A's data stored with "service-a" binding
	svcAID := id.NewServiceID()
	storedEntityA := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     svcAID,
		Secret: model.NewEncryptedSecret([]byte("encrypted-for-a")),
	}
	mockRepo.On("Get", ctx, svcAID).Return(storedEntityA, nil)

	// Decryption fails because Service A's context is bound to its ID
	contextMismatchErr := errors.New("encryption context mismatch")
	mockEnc.On("Decrypt", ctx, []byte("encrypted-for-a"), map[string]string{"service_id": svcAID.String()}).
		Return(nil, contextMismatchErr)

	result, err := svc.Get(ctx, svcAID)

	// Get returns the entity gracefully (no error) but secret remains encrypted
	require.NoError(t, err)
	require.NotNil(t, result)
	// Security assertion: secret is NOT in plaintext state — callers cannot extract it
	assert.True(t, result.Secret.IsEncrypted(), "secret must remain encrypted when decryption fails")
	assert.False(t, result.Secret.IsPlaintext(), "secret must NOT be plaintext when decryption fails")
	_, ptErr := result.Secret.GetPlaintext()
	assert.Error(t, ptErr, "GetPlaintext must fail for encrypted secret — prevents cross-service token swap")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Get_DecryptionFailure_ReturnsEncryptedEntity(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	storedEntity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: "GitHub",
		Secret:      model.NewEncryptedSecret([]byte("old-encryption-ciphertext")),
	}
	mockRepo.On("Get", ctx, svcID).Return(storedEntity, nil)
	mockEnc.On("Decrypt", ctx, []byte("old-encryption-ciphertext"), map[string]string{"service_id": svcID.String()}).
		Return(nil, errors.New("decryption failed: wrong encryption backend"))

	result, err := svc.Get(ctx, svcID)

	// Get succeeds gracefully — entity returned with encrypted secret
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, svcID, result.ID)
	assert.Equal(t, "GitHub", result.DisplayName)
	assert.True(t, result.Secret.IsEncrypted(), "secret should remain encrypted on decryption failure")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// Update tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Update_WithNewSecret(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("new-secret"))

	mockEnc.On("Encrypt", ctx, []byte("new-secret"), map[string]string{"service_id": svcID.String()}).
		Return([]byte("new-encrypted"), nil)
	mockRepo.On("Update", ctx, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
		return e.Secret.IsEncrypted()
	}), (*int64)(nil)).Return(nil)

	err := svc.Update(ctx, entity, nil)

	require.NoError(t, err)
	assert.True(t, entity.Secret.IsEncrypted(), "secret must be encrypted after update")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Update_NormalizesProtectedResourcesBeforePersist(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("new-secret"))
	entity.ProtectedResources = []string{
		"https://api.example.com/",
		"https://api.example.com/v1///",
	}

	mockEnc.On("Encrypt", ctx, []byte("new-secret"), map[string]string{"service_id": svcID.String()}).
		Return([]byte("new-encrypted"), nil)
	mockRepo.On("Update", ctx, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
		return e.Secret.IsEncrypted() &&
			assert.ObjectsAreEqual([]string{
				"https://api.example.com",
				"https://api.example.com/v1",
			}, e.ProtectedResources)
	}), (*int64)(nil)).Return(nil)

	err := svc.Update(ctx, entity, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://api.example.com",
		"https://api.example.com/v1",
	}, entity.ProtectedResources)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Update_ProvisionsBranchKey(t *testing.T) {
	// Verifies that Update provisions the branch key BEFORE encrypting. This is the
	// migration path: a service created with a different encryption backend (e.g. raw AES)
	// has no branch key in the KMS key store; Update must provision it so encryption succeeds.
	// Call-order is asserted via an atomic sequence counter: Create must increment it before
	// Encrypt reads it, so the value seen by Encrypt is always > 0.
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockBKM := new(MockBranchKeyManager)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, mockBKM, nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("new-secret"))

	var callSeq atomic.Int32 // incremented by each call; used to verify ordering

	var createSeq int32
	mockBKM.On("Create", ctx, serviceSubject(svcID)).
		Return("sentinel-branch-key-id", nil).
		Run(func(args mock.Arguments) {
			createSeq = callSeq.Add(1)
		})

	var encryptSeq int32
	mockEnc.On("Encrypt", ctx, []byte("new-secret"), map[string]string{"service_id": svcID.String()}).
		Return([]byte("new-encrypted"), nil).
		Run(func(args mock.Arguments) {
			encryptSeq = callSeq.Add(1)
		})

	mockRepo.On("Update", ctx, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
		return e.Secret.IsEncrypted()
	}), (*int64)(nil)).Return(nil)

	err := svc.Update(ctx, entity, nil)

	require.NoError(t, err)
	assert.True(t, entity.Secret.IsEncrypted(), "secret must be encrypted after update")
	assert.Less(t, createSeq, encryptSeq, "branch key Create must be called before Encrypt")
	mockBKM.AssertExpectations(t)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Update_BranchKeyProvisioningFailure_AbortUpdate(t *testing.T) {
	// If branch key provisioning fails (e.g. DynamoDB unavailable), Update must fail
	// before attempting encryption so no partial state is written.
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockBKM := new(MockBranchKeyManager)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, mockBKM, nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("new-secret"))

	mockBKM.On("Create", ctx, serviceSubject(svcID)).Return("", fmt.Errorf("DynamoDB unavailable"))

	err := svc.Update(ctx, entity, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch key provisioning failed")
	mockBKM.AssertExpectations(t)
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Update")
}

func TestThirdpartyOAuth2ProviderService_Update_EncryptedSecretFails(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	// Passing an already-encrypted secret must be rejected — callers must always supply
	// plaintext so re-encryption always runs (prevents silent bypass during key rotation).
	entity := minimalValidEntity(id.NewServiceID(), model.NewEncryptedSecret([]byte("existing-ciphertext")))

	err := svc.Update(ctx, entity, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider validation failed")
	assert.Contains(t, err.Error(), "client_secret is required for update")
	mockEnc.AssertNotCalled(t, "Encrypt")
	mockRepo.AssertNotCalled(t, "Update")
}

func TestThirdpartyOAuth2ProviderService_Update_PublicClient_ProvisionsBranchKeyAndStoresAbsentSecret(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	entity := minimalValidEntity(serviceID, model.NewAbsentSecret())
	entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	entity.AuthorizationParams = map[string]string{"audience": "sentinel-client-secret"}

	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{
		updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
			stored = provider
			return nil
		},
	}
	var branchKeySubjects []domainencryption.BranchKeySubject
	branchKeyManager := &functionFieldBranchKeyManager{
		createFn: func(_ context.Context, subject domainencryption.BranchKeySubject) (string, error) {
			branchKeySubjects = append(branchKeySubjects, subject)
			return "public-service-key", nil
		},
	}
	logOutput := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, branchKeyManager, nil, false, slog.New(slog.NewJSONHandler(logOutput, nil)))

	require.NoError(t, service.Update(ctx, entity, nil))
	require.NotNil(t, stored)
	assert.True(t, entity.Secret.IsAbsent())
	assert.True(t, stored.Secret.IsAbsent())
	assert.Equal(t, []domainencryption.BranchKeySubject{serviceSubject(serviceID)}, branchKeySubjects)
	assertPublicClientAuditEvent(t, logOutput, "service.thirdparty.provider_updated", serviceID, "sentinel-client-secret")
}

func TestThirdpartyOAuth2ProviderService_Update_TransitionsCredentialStorage(t *testing.T) {
	t.Run("confidential to public clears stored ciphertext", func(t *testing.T) {
		ctx := context.Background()
		serviceID := id.NewServiceID()
		persisted := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("old-ciphertext")))
		require.True(t, persisted.Secret.IsEncrypted())

		repo := &functionFieldProviderRepository{
			updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
				persisted = provider.Copy()
				return nil
			},
		}
		var encryptCalls int
		encryption := &functionFieldEncryption{
			encryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
				encryptCalls++
				return nil, errors.New("public client must not encrypt a secret")
			},
		}
		service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.Default())
		publicUpdate := minimalValidEntity(serviceID, model.NewAbsentSecret())
		publicUpdate.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone

		require.NoError(t, service.Update(ctx, publicUpdate, nil))
		assert.Zero(t, encryptCalls)
		assert.True(t, persisted.Secret.IsAbsent(), "repo.Update must receive the absent secret that clears stored ciphertext")
		assert.Equal(t, model.TokenEndpointAuthMethodNone, persisted.TokenEndpointAuthMethod)
	})

	t.Run("public to confidential encrypts and stores supplied secret", func(t *testing.T) {
		ctx := context.Background()
		serviceID := id.NewServiceID()
		persisted := minimalValidEntity(serviceID, model.NewAbsentSecret())
		persisted.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone

		repo := &functionFieldProviderRepository{
			updateFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity, _ *int64) error {
				persisted = provider.Copy()
				return nil
			},
		}
		var encryptedPlaintext []byte
		var encryptionContext map[string]string
		encryption := &functionFieldEncryption{
			encryptFn: func(_ context.Context, plaintext []byte, context map[string]string) ([]byte, error) {
				encryptedPlaintext = append([]byte(nil), plaintext...)
				encryptionContext = context
				return []byte("new-ciphertext"), nil
			},
		}
		service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.Default())
		confidentialUpdate := minimalValidEntity(serviceID, model.NewPlaintextSecret("new-secret"))

		require.NoError(t, service.Update(ctx, confidentialUpdate, nil))
		assert.Equal(t, []byte("new-secret"), encryptedPlaintext)
		assert.Equal(t, map[string]string{"service_id": serviceID.String()}, encryptionContext)
		assert.True(t, confidentialUpdate.Secret.IsEncrypted())
		assert.True(t, persisted.Secret.IsEncrypted())
		assert.True(t, persisted.TokenEndpointAuthMethod.IsAbsent())
		ciphertext, err := persisted.Secret.GetCiphertext()
		require.NoError(t, err)
		assert.Equal(t, []byte("new-ciphertext"), ciphertext)
	})
}

// =============================================================================
// List tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_List_DecryptsAll(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	var logs bytes.Buffer
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.New(slog.NewJSONHandler(&logs, nil)))

	ctx := context.Background()
	svc1ID := id.NewServiceID()
	svc2ID := id.NewServiceID()
	entities := []*model.ThirdpartyOAuth2ProviderEntity{
		{ID: svc1ID, Secret: model.NewEncryptedSecret([]byte("enc-1"))},
		{ID: svc2ID, Secret: model.NewEncryptedSecret([]byte("enc-2"))},
	}
	mockRepo.On("List", ctx).Return(entities, nil)
	mockEnc.On("Decrypt", ctx, []byte("enc-1"), map[string]string{"service_id": svc1ID.String()}).
		Return([]byte("secret-1"), nil)
	mockEnc.On("Decrypt", ctx, []byte("enc-2"), map[string]string{"service_id": svc2ID.String()}).
		Return([]byte("secret-2"), nil)

	results, err := svc.List(ctx)

	require.NoError(t, err)
	require.Len(t, results, 2)
	pt1, _ := results[0].Secret.GetPlaintext()
	pt2, _ := results[1].Secret.GetPlaintext()
	assert.Equal(t, "secret-1", pt1)
	assert.Equal(t, "secret-2", pt2)

	require.NotEmpty(t, logs.String(), "successful decryptions must be auditable at the default Info level")
	audited := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		if event["msg"] != "service_secret_decrypted" {
			continue
		}
		assert.Equal(t, "INFO", event["level"])
		serviceID, ok := event["service_id"].(string)
		require.True(t, ok)
		assert.False(t, audited[serviceID], "duplicate decryption audit event for %s", serviceID)
		audited[serviceID] = true
	}
	assert.Equal(t, map[string]bool{svc1ID.String(): true, svc2ID.String(): true}, audited)
	assert.NotContains(t, logs.String(), "secret-1")
	assert.NotContains(t, logs.String(), "secret-2")
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_List_PublicClientReturnsAbsentWithoutDecryptWarning(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	stored := minimalValidEntity(serviceID, model.NewAbsentSecret())
	stored.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone

	var logs bytes.Buffer
	service := NewThirdpartyOAuth2ProviderService(
		&functionFieldProviderRepository{
			listFn: func(context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
				return []*model.ThirdpartyOAuth2ProviderEntity{stored}, nil
			},
		},
		&functionFieldEncryption{},
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})),
	)

	providers, err := service.List(ctx)

	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.True(t, providers[0].Secret.IsAbsent())
	assert.Empty(t, logs.String(), "public clients must not cause a decrypt warning or error")
}

func TestThirdpartyOAuth2ProviderService_List_CIMDClientReturnsAbsentWithoutDecryptWarning(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	stored := minimalValidCIMDProvider(
		serviceID,
		brokerHostedCIMDClientID(serviceID),
		model.TokenEndpointAuthMethodPrivateKeyJWT,
	)
	stored.Secret = model.NewEncryptedSecret([]byte("sentinel-ciphertext"))

	var logs bytes.Buffer
	service := NewThirdpartyOAuth2ProviderService(
		&functionFieldProviderRepository{
			listFn: func(context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
				return []*model.ThirdpartyOAuth2ProviderEntity{stored}, nil
			},
		},
		&functionFieldEncryption{
			decryptFn: func(context.Context, []byte, map[string]string) ([]byte, error) {
				return nil, errors.New("CIMD list must not decrypt a shared secret")
			},
		},
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})),
	)

	providers, err := service.List(ctx)

	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.True(t, providers[0].Secret.IsEncrypted())
	assert.Empty(t, logs.String(), "CIMD clients must not cause a decrypt warning or error")
}

func TestThirdpartyOAuth2ProviderService_List_GracefulDecryptionFailure(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svc1ID := id.NewServiceID()
	svc2ID := id.NewServiceID()
	entities := []*model.ThirdpartyOAuth2ProviderEntity{
		{ID: svc1ID, Secret: model.NewEncryptedSecret([]byte("enc-1"))},
		{ID: svc2ID, Secret: model.NewEncryptedSecret([]byte("corrupted"))},
	}
	mockRepo.On("List", ctx).Return(entities, nil)
	mockEnc.On("Decrypt", ctx, []byte("enc-1"), map[string]string{"service_id": svc1ID.String()}).
		Return([]byte("secret-1"), nil)
	mockEnc.On("Decrypt", ctx, []byte("corrupted"), map[string]string{"service_id": svc2ID.String()}).
		Return(nil, errors.New("decryption failed"))

	results, err := svc.List(ctx)

	// List succeeds even when individual decryption fails
	require.NoError(t, err)
	require.Len(t, results, 2)

	// First entity is decrypted successfully
	pt1, err := results[0].Secret.GetPlaintext()
	require.NoError(t, err)
	assert.Equal(t, "secret-1", pt1)

	// Second entity is returned with encrypted secret (decryption failed gracefully)
	assert.True(t, results[1].Secret.IsEncrypted(), "failed entity should retain encrypted secret")
	assert.Equal(t, svc2ID, results[1].ID)
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// Delete tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_Delete(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	mockRepo.On("Delete", ctx, svcID).Return(nil)

	err := svc.Delete(ctx, svcID)

	require.NoError(t, err)
	mockEnc.AssertNotCalled(t, "Decrypt")
	mockRepo.AssertExpectations(t)
}

// =============================================================================
// FindByProtectedResource tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_FindByProtectedResource_DoesNotDecryptSecret(t *testing.T) {
	ctx := context.Background()
	stored := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     id.NewServiceID(),
		Secret: model.NewEncryptedSecret([]byte("ciphertext")),
	}
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockRepo.On("FindByProtectedResource", ctx, "https://api.example.com").Return(stored, nil)
	service := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	provider, err := service.FindByProtectedResource(ctx, "https://api.example.com")
	require.NoError(t, err)
	require.True(t, provider.Secret.IsEncrypted(), "resource resolution must not expose a plaintext secret")
	mockEnc.AssertNotCalled(t, "Decrypt")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_FindByProtectedResource_PublicClientReturnsAbsentWithoutDecryptWarning(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	stored := minimalValidEntity(serviceID, model.NewAbsentSecret())
	stored.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone

	var logs bytes.Buffer
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	mockRepo.On("FindByProtectedResource", ctx, "https://api.example.com").Return(stored, nil)
	service := NewThirdpartyOAuth2ProviderService(
		mockRepo,
		mockEnc,
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})),
	)

	provider, err := service.FindByProtectedResource(ctx, "https://api.example.com")

	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.True(t, provider.Secret.IsAbsent())
	assert.Empty(t, logs.String(), "public clients must not cause a decrypt warning or error")
	mockEnc.AssertNotCalled(t, "Decrypt")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_Create_ServiceIDOnlyContext(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := minimalValidEntity(svcID, model.NewPlaintextSecret("secret"))

	// ADR 008: only service_id in context, no principal
	expectedContext := map[string]string{"service_id": svcID.String()}
	mockEnc.On("Encrypt", ctx, []byte("secret"), expectedContext).Return([]byte("enc"), nil)
	mockRepo.On("Create", ctx, mock.Anything).Return(nil)

	err := svc.Create(ctx, entity)

	require.NoError(t, err)
	mockEnc.AssertExpectations(t)
}

// =============================================================================
// ValidateServiceRequirements tests
// =============================================================================

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_Empty(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	err := svc.ValidateServiceRequirements(context.Background(), nil)
	assert.NoError(t, err)

	err = svc.ValidateServiceRequirements(context.Background(), []storage.ServiceRequirement{})
	assert.NoError(t, err)

	mockRepo.AssertNotCalled(t, "Get")
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_ValidScopes(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: "GitHub",
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo"},
			{ScopeValue: "user:email"},
		},
		Secret: model.NewEncryptedSecret([]byte("ciphertext")),
	}
	mockRepo.On("Get", ctx, svcID).Return(entity, nil)

	serviceReqs := []storage.ServiceRequirement{
		{
			ServiceID:       svcID,
			RequirementType: storage.RequirementTypeMandatory,
			RequiredScopes:  []string{"repo", "user:email"},
		},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_MultipleServices(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	githubID := id.NewServiceID()
	gitlabID := id.NewServiceID()
	entity1 := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          githubID,
		DisplayName: "GitHub",
		Scopes:      []model.OAuthScope{{ScopeValue: "repo"}, {ScopeValue: "user:email"}},
		Secret:      model.NewEncryptedSecret([]byte("enc1")),
	}
	entity2 := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          gitlabID,
		DisplayName: "GitLab",
		Scopes:      []model.OAuthScope{{ScopeValue: "api"}, {ScopeValue: "read_user"}},
		Secret:      model.NewEncryptedSecret([]byte("enc2")),
	}
	mockRepo.On("Get", ctx, githubID).Return(entity1, nil)
	mockRepo.On("Get", ctx, gitlabID).Return(entity2, nil)

	serviceReqs := []storage.ServiceRequirement{
		{ServiceID: githubID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo"}},
		{ServiceID: gitlabID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"api"}},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_ServiceNotFound(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	missingSvcID := id.NewServiceID()
	notFoundErr := storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "not found")
	mockRepo.On("Get", ctx, missingSvcID).Return(nil, notFoundErr)

	serviceReqs := []storage.ServiceRequirement{
		{ServiceID: missingSvcID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read"}},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
	assert.Contains(t, storageErr.Message, missingSvcID.String())
	assert.Contains(t, storageErr.Message, "not found")
	assert.Contains(t, storageErr.Message, "index 0")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_InvalidScope(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: "GitHub",
		Scopes:      []model.OAuthScope{{ScopeValue: "repo"}, {ScopeValue: "user:email"}},
		Secret:      model.NewEncryptedSecret([]byte("ciphertext")),
	}
	mockRepo.On("Get", ctx, svcID).Return(entity, nil)

	serviceReqs := []storage.ServiceRequirement{
		{ServiceID: svcID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo", "invalid:scope"}},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
	assert.Contains(t, storageErr.Message, "invalid:scope")
	assert.Contains(t, storageErr.Message, "GitHub")
	assert.Contains(t, storageErr.Message, "index 0")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_SecondIndexError(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svc1ID := id.NewServiceID()
	missingSvcID := id.NewServiceID()
	entity1 := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svc1ID,
		DisplayName: "GitHub",
		Scopes:      []model.OAuthScope{{ScopeValue: "repo"}},
		Secret:      model.NewEncryptedSecret([]byte("enc1")),
	}
	notFoundErr := storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "not found")
	mockRepo.On("Get", ctx, svc1ID).Return(entity1, nil)
	mockRepo.On("Get", ctx, missingSvcID).Return(nil, notFoundErr)

	serviceReqs := []storage.ServiceRequirement{
		{ServiceID: svc1ID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo"}},
		{ServiceID: missingSvcID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"read"}},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
	assert.Contains(t, storageErr.Message, "index 1")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_StorageError(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svc1ID := id.NewServiceID()
	connErr := storage.NewStorageError("Get", storage.ErrorKindConnection, nil, "database connection failed")
	mockRepo.On("Get", ctx, svc1ID).Return(nil, connErr)

	serviceReqs := []storage.ServiceRequirement{
		{ServiceID: svc1ID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo"}},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindConnection, storageErr.Kind)
	assert.Contains(t, storageErr.Message, "database connection failed")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ValidateServiceRequirements_CaseSensitiveScopes(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: "GitHub",
		Scopes:      []model.OAuthScope{{ScopeValue: "repo"}},
		Secret:      model.NewEncryptedSecret([]byte("ciphertext")),
	}
	mockRepo.On("Get", ctx, svcID).Return(entity, nil)

	// "REPO" must not match "repo" — OAuth2 scopes are case-sensitive
	serviceReqs := []storage.ServiceRequirement{
		{ServiceID: svcID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"REPO"}},
	}

	err := svc.ValidateServiceRequirements(ctx, serviceReqs)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
	assert.Contains(t, storageErr.Message, "REPO")
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_ProtectedResourceMutations_NormalizeAndRejectInvalidInput(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	tests := []struct {
		name      string
		call      func(*ThirdpartyOAuth2ProviderService) (ports.ProtectedResourceMutationResult, error)
		configure func(*MockRepository)
		wantErr   bool
	}{
		{
			name: "adds normalized URI",
			call: func(s *ThirdpartyOAuth2ProviderService) (ports.ProtectedResourceMutationResult, error) {
				return s.AddProtectedResource(ctx, serviceID, "https://api.example.com/resource/")
			},
			configure: func(repo *MockRepository) {
				repo.On("AddProtectedResource", ctx, serviceID, "https://api.example.com/resource").Return(ports.ProtectedResourceMutationResult{Resource: "https://api.example.com/resource", ProtectedResources: []string{"https://api.example.com/resource"}, Version: 4, Changed: true}, nil)
			},
		},
		{
			name: "removes normalized URI",
			call: func(s *ThirdpartyOAuth2ProviderService) (ports.ProtectedResourceMutationResult, error) {
				return s.RemoveProtectedResource(ctx, serviceID, "https://api.example.com/resource/")
			},
			configure: func(repo *MockRepository) {
				repo.On("RemoveProtectedResource", ctx, serviceID, "https://api.example.com/resource").Return(ports.ProtectedResourceMutationResult{Resource: "https://api.example.com/resource", ProtectedResources: []string{}, Version: 5, Changed: true}, nil)
			},
		},
		{
			name: "renames independently normalized URIs",
			call: func(s *ThirdpartyOAuth2ProviderService) (ports.ProtectedResourceMutationResult, error) {
				return s.RenameProtectedResource(ctx, serviceID, "https://api.example.com/from/", "https://api.example.com/to/")
			},
			configure: func(repo *MockRepository) {
				repo.On("RenameProtectedResource", ctx, serviceID, "https://api.example.com/from", "https://api.example.com/to").Return(ports.ProtectedResourceMutationResult{Resource: "https://api.example.com/to", ProtectedResources: []string{"https://api.example.com/to"}, Version: 6, Changed: true}, nil)
			},
		},
		{
			name: "rejects malformed add before repository",
			call: func(s *ThirdpartyOAuth2ProviderService) (ports.ProtectedResourceMutationResult, error) {
				return s.AddProtectedResource(ctx, serviceID, "://not-a-uri")
			},
			configure: func(_ *MockRepository) {},
			wantErr:   true,
		},
		{
			name: "rejects malformed rename target before repository",
			call: func(s *ThirdpartyOAuth2ProviderService) (ports.ProtectedResourceMutationResult, error) {
				return s.RenameProtectedResource(ctx, serviceID, "https://api.example.com/from", "://not-a-uri")
			},
			configure: func(_ *MockRepository) {},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepository)
			tt.configure(repo)
			service := NewThirdpartyOAuth2ProviderService(repo, new(MockEncryption), newNoopBranchKeyManager(), nil, false, slog.Default())

			result, err := tt.call(service)
			if tt.wantErr {
				require.Error(t, err)
				var storageErr *storage.StorageError
				require.ErrorAs(t, err, &storageErr)
				assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
				repo.AssertNotCalled(t, "AddProtectedResource", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "RenameProtectedResource", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, result.Resource)
			assert.NotZero(t, result.Version)
			assert.NotNil(t, result.ProtectedResources)
			repo.AssertExpectations(t)
		})
	}
}

func TestThirdpartyOAuth2ProviderService_ProtectedResourceMutationErrorsAndList(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	notFound := storage.NewStorageError("resource", storage.ErrorKindNotFound, errors.New("missing"), "missing")
	conflict := storage.NewStorageError("resource", storage.ErrorKindConflict, errors.New("owned"), "owned")

	tests := []struct {
		name      string
		configure func(*MockRepository)
		call      func(*ThirdpartyOAuth2ProviderService) error
		want      error
	}{
		{
			name: "remove missing resource preserves not found",
			configure: func(repo *MockRepository) {
				repo.On("RemoveProtectedResource", ctx, serviceID, "https://api.example.com/missing").Return(ports.ProtectedResourceMutationResult{}, notFound)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, err := s.RemoveProtectedResource(ctx, serviceID, "https://api.example.com/missing")
				return err
			},
			want: notFound,
		},
		{
			name: "add missing service preserves not found",
			configure: func(repo *MockRepository) {
				repo.On("AddProtectedResource", ctx, serviceID, "https://api.example.com/new").Return(ports.ProtectedResourceMutationResult{}, notFound)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, err := s.AddProtectedResource(ctx, serviceID, "https://api.example.com/new")
				return err
			},
			want: notFound,
		},
		{
			name: "add cross-service owner conflict is preserved",
			configure: func(repo *MockRepository) {
				repo.On("AddProtectedResource", ctx, serviceID, "https://api.example.com/owned").Return(ports.ProtectedResourceMutationResult{}, conflict)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, err := s.AddProtectedResource(ctx, serviceID, "https://api.example.com/owned")
				return err
			},
			want: conflict,
		},
		{
			name: "remove missing service preserves not found",
			configure: func(repo *MockRepository) {
				repo.On("RemoveProtectedResource", ctx, serviceID, "https://api.example.com/present").Return(ports.ProtectedResourceMutationResult{}, notFound)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, err := s.RemoveProtectedResource(ctx, serviceID, "https://api.example.com/present")
				return err
			},
			want: notFound,
		},
		{
			name: "rename owned target preserves conflict",
			configure: func(repo *MockRepository) {
				repo.On("RenameProtectedResource", ctx, serviceID, "https://api.example.com/from", "https://api.example.com/owned").Return(ports.ProtectedResourceMutationResult{}, conflict)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, err := s.RenameProtectedResource(ctx, serviceID, "https://api.example.com/from", "https://api.example.com/owned")
				return err
			},
			want: conflict,
		},
		{
			name: "rename missing source preserves not found",
			configure: func(repo *MockRepository) {
				repo.On("RenameProtectedResource", ctx, serviceID, "https://api.example.com/missing", "https://api.example.com/to").Return(ports.ProtectedResourceMutationResult{}, notFound)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, err := s.RenameProtectedResource(ctx, serviceID, "https://api.example.com/missing", "https://api.example.com/to")
				return err
			},
			want: notFound,
		},
		{
			name: "list returns normalized state and current version",
			configure: func(repo *MockRepository) {
				repo.On("ListProtectedResources", ctx, serviceID).Return([]string{"https://api.example.com/a", "https://api.example.com/b"}, int64(9), nil)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				resources, version, err := s.ListProtectedResources(ctx, serviceID)
				require.NoError(t, err)
				assert.Equal(t, []string{"https://api.example.com/a", "https://api.example.com/b"}, resources)
				assert.EqualValues(t, 9, version)
				return nil
			},
		},
		{
			name: "list missing service preserves not found",
			configure: func(repo *MockRepository) {
				repo.On("ListProtectedResources", ctx, serviceID).Return([]string(nil), int64(0), notFound)
			},
			call: func(s *ThirdpartyOAuth2ProviderService) error {
				_, _, err := s.ListProtectedResources(ctx, serviceID)
				return err
			},
			want: notFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepository)
			tt.configure(repo)
			service := NewThirdpartyOAuth2ProviderService(repo, new(MockEncryption), newNoopBranchKeyManager(), nil, false, slog.Default())
			err := tt.call(service)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			} else {
				require.NoError(t, err)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestThirdpartyOAuth2ProviderService_RenameProtectedResource_SameURINoOp(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	repo := new(MockRepository)
	repo.On("RenameProtectedResource", ctx, serviceID, "https://api.example.com/resource", "https://api.example.com/resource").Return(ports.ProtectedResourceMutationResult{
		Resource:           "https://api.example.com/resource",
		ProtectedResources: []string{"https://api.example.com/resource"},
		Version:            10,
		Changed:            false,
	}, nil)
	service := NewThirdpartyOAuth2ProviderService(repo, new(MockEncryption), newNoopBranchKeyManager(), nil, false, slog.Default())

	result, err := service.RenameProtectedResource(ctx, serviceID, "https://api.example.com/resource/", "https://api.example.com/resource")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/resource", result.Resource)
	assert.Equal(t, []string{"https://api.example.com/resource"}, result.ProtectedResources)
	assert.EqualValues(t, 10, result.Version)
	assert.False(t, result.Changed)
	repo.AssertExpectations(t)
}

func TestResolveIDAcceptsUUIDCanonicalAndRejectsUnknown(t *testing.T) {
	serviceID := id.NewServiceID()
	canonicalID := "github-service"
	repo := new(MockRepository)
	repo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(&model.ThirdpartyOAuth2ProviderEntity{ID: serviceID}, nil)
	repo.On("GetByCanonicalID", mock.Anything, "unknown-service").Return(nil, ports.ErrNotFound)
	service := NewThirdpartyOAuth2ProviderService(repo, new(MockEncryption), newNoopBranchKeyManager(), nil, false, slog.Default())
	for _, value := range []string{serviceID.String(), canonicalID} {
		resolved, err := service.ResolveID(context.Background(), value)
		require.NoError(t, err)
		assert.Equal(t, serviceID, resolved)
	}
	_, err := service.ResolveID(context.Background(), "unknown-service")
	require.Error(t, err)
	repo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_GetCIMDClientServiceReturnsCredentialFreeProjection(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	clientID := brokerHostedCIMDClientID(serviceID)
	entity := minimalValidCIMDProvider(serviceID, clientID, model.TokenEndpointAuthMethod("private_key_jwt"))
	entity.ClientID = clientID
	entity.Secret = model.NewEncryptedSecret([]byte("sentinel-ciphertext"))

	service := NewThirdpartyOAuth2ProviderService(
		&functionFieldProviderRepository{
			getFn: func(_ context.Context, requestedID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				assert.Equal(t, serviceID, requestedID)
				return entity, nil
			},
		},
		&functionFieldEncryption{
			decryptFn: func(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
				return nil, errors.New("metadata projection must not decrypt credentials")
			},
		},
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.Default(),
	)

	projection, err := service.GetCIMDClientService(ctx, serviceID)

	require.NoError(t, err)
	assert.Equal(t, &ports.CIMDClientServiceProjection{
		ID:                      serviceID,
		ClientID:                clientID,
		TokenEndpointAuthMethod: "private_key_jwt",
	}, projection)
}

func TestThirdpartyOAuth2ProviderService_GetCIMDClientServiceRejectsNonCIMDServices(t *testing.T) {
	ctx := context.Background()
	serviceID := id.NewServiceID()
	entity := minimalValidEntity(serviceID, model.NewEncryptedSecret([]byte("sentinel-ciphertext")))

	service := NewThirdpartyOAuth2ProviderService(
		&functionFieldProviderRepository{
			getFn: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return entity, nil
			},
		},
		&functionFieldEncryption{},
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.Default(),
	)

	projection, err := service.GetCIMDClientService(ctx, serviceID)

	require.ErrorIs(t, err, ports.ErrNotFound)
	assert.Nil(t, projection)
}

type countingCIMDKeyReadiness struct {
	calls int
	err   error
}

func (r *countingCIMDKeyReadiness) RequireUsablePublishedKey(context.Context) error {
	r.calls++
	return r.err
}

func TestThirdpartyOAuth2ProviderService_Create_DiscoveryCommitsSelectedCIMDAndDerivedResourceTogether(t *testing.T) {
	entity := discoveredProvider(discoveryResource, "")
	entity.ID = id.ServiceID{}
	client := &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {status: 401, challenges: []string{`DPoP resource_metadata="` + discoveryResourcePath + `"`}}},
		gets: map[string]discoveryJSONReply{
			discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			discoveryIssuerOAuth:  {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
		},
	}
	readiness := new(countingCIMDKeyReadiness)
	var saved *model.ThirdpartyOAuth2ProviderEntity
	var createCalls int
	repo := &functionFieldProviderRepository{
		createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
			createCalls++
			saved = provider.Copy()
			return nil
		},
		getFn: func(_ context.Context, requestedID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
			if saved == nil || requestedID != saved.ID {
				return nil, errors.New("service was not saved")
			}
			return saved.Copy(), nil
		},
	}
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.Default()).
		WithOAuthDiscoveryClient(client).
		WithCIMDPublicURL("https://broker.example.test").
		WithCIMDKeyReadiness(readiness)

	err := service.Create(context.Background(), entity)
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls, "compatible CIMD must not register through DCR")
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, 1, createCalls)
	assert.Equal(t, 1, readiness.calls)
	assert.False(t, saved.ID.IsZero())
	assert.Equal(t, id.ClientID("https://broker.example.test/.well-known/oauth-client/"+saved.ID.String()), saved.ClientID)
	assert.Equal(t, model.ClientBootstrapCIMD, saved.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodPrivateKeyJWT, saved.TokenEndpointAuthMethod)
	assert.Equal(t, discoveryIssuer, saved.IssuerURI)
	assert.Equal(t, discoveryIssuer+"/authorize", saved.Endpoints.AuthorizeEndpoint)
	assert.Equal(t, discoveryIssuer+"/token", saved.Endpoints.TokenEndpoint)
	assert.Equal(t, discoveryResource, saved.AuthorizationParams["resource"])
	assert.False(t, saved.ResourceExplicit)
	assert.True(t, saved.Secret.IsAbsent())
	require.NotNil(t, saved.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, saved.DiscoveryStatus.LastSuccessAt)
	assert.False(t, saved.DiscoveryStatus.LastSuccessAt.Before(*saved.DiscoveryStatus.LastAttemptAt))
	assert.Nil(t, saved.DiscoveryStatus.FailureReason)

	read, err := service.Get(context.Background(), saved.ID)
	require.NoError(t, err)
	assert.Equal(t, discoveryResource, read.AuthorizationParams["resource"])
	assert.Equal(t, model.ClientBootstrapCIMD, read.Discovery.ClientMethod)
}

func TestThirdpartyOAuth2ProviderService_Create_SelectedCIMDKeyFailureNeverAttemptsDCRorStores(t *testing.T) {
	client := &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
		gets: map[string]discoveryJSONReply{
			discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			discoveryIssuerOAuth:  {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
		},
	}
	readiness := &countingCIMDKeyReadiness{err: ports.ErrCIMDPublicKeyUnavailable}
	createCalls := 0
	repo := &functionFieldProviderRepository{createFn: func(context.Context, *model.ThirdpartyOAuth2ProviderEntity) error {
		createCalls++
		return nil
	}}
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.Default()).
		WithOAuthDiscoveryClient(client).
		WithCIMDPublicURL("https://broker.example.test").
		WithCIMDKeyReadiness(readiness)

	err := service.Create(context.Background(), discoveredProvider(discoveryResource, ""))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls)
	assert.Equal(t, 1, readiness.calls, "selected ES256 CIMD method must check its published key")
	assert.Zero(t, createCalls, "a failed discovery cannot persist a partially configured service")
	require.ErrorContains(t, err, "cimd_unavailable")
}

func TestThirdpartyOAuth2ProviderService_Create_RejectsCIMDWithoutES256BeforeKeyCheck(t *testing.T) {
	client := &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
		gets: map[string]discoveryJSONReply{
			discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			discoveryIssuerOAuth:  {body: []byte(`{"issuer":"https://auth.example.test/tenant","authorization_endpoint":"https://auth.example.test/tenant/authorize","token_endpoint":"https://auth.example.test/tenant/token","client_id_metadata_document_supported":true,"token_endpoint_auth_methods_supported":["private_key_jwt"],"token_endpoint_auth_signing_alg_values_supported":["RS256"]}`)},
		},
	}
	readiness := new(countingCIMDKeyReadiness)
	repo := &functionFieldProviderRepository{createFn: func(context.Context, *model.ThirdpartyOAuth2ProviderEntity) error {
		return errors.New("unsupported method cannot persist")
	}}
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.Default()).
		WithOAuthDiscoveryClient(client).
		WithCIMDPublicURL("https://broker.example.test").
		WithCIMDKeyReadiness(readiness)

	err := service.Create(context.Background(), discoveredProvider(discoveryResource, ""))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls)
	assert.Zero(t, readiness.calls, "RS256 alone cannot select an ES256 hosted CIMD identity")
	require.ErrorContains(t, err, "no_compatible_client_method")
}

func TestThirdpartyOAuth2ProviderService_Create_ManualStillIgnoresInjectedDiscoveryClient(t *testing.T) {
	client := new(recordingOAuthDiscoveryClient)
	entity := minimalValidEntity(id.NewServiceID(), model.NewPlaintextSecret("manual-secret"))
	entity.Discovery = model.DiscoveryConfig{EnableDiscovery: false}
	entity.Endpoints = model.OAuth2Endpoints{
		AuthorizeEndpoint: "https://issuer.example.com/authorize",
		TokenEndpoint:     "https://issuer.example.com/token",
	}
	var saved *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
		saved = provider.Copy()
		return nil
	}}
	encryption := &functionFieldEncryption{encryptFn: func(_ context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
		assert.Equal(t, []byte("manual-secret"), plaintext)
		assert.Equal(t, serviceSubject(entity.ID).EncryptionContext(), encryptionContext)
		return []byte("encrypted-secret"), nil
	}}
	service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.Default()).WithOAuthDiscoveryClient(client)

	err := service.Create(context.Background(), entity)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Empty(t, client.calls)
	assert.Equal(t, id.ClientID("test-client-id"), saved.ClientID)
	assert.True(t, saved.Secret.IsEncrypted())
	assert.Empty(t, saved.Discovery.ClientMethod)
	assert.Nil(t, saved.DiscoveryStatus.LastSuccessAt)
}

func TestThirdpartyOAuth2ProviderService_DCRPersistsIssuerScopedEncryptedCredentialWithoutReregistering(t *testing.T) {
	ctx := context.Background()
	entity := discoveredProvider(discoveryResource, "")
	entity.DisplayName = "Files MCP"
	entity.AuthorizationParams = map[string]string{"resource": "https://audience.example.test/files"}
	const clientSecret = "provider-issued-secret"
	const managementToken = "registration-management-token"
	const managementURL = discoveryIssuer + "/register/manage/client-42"
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
	client.postFn = func(_ context.Context, rawURL string, _ []byte) ([]byte, error) {
		require.Equal(t, discoveryIssuer+"/register", rawURL)
		response := dcrResponse(dcrCallback(entity.ID), model.TokenEndpointAuthMethodClientSecretBasic, clientSecret)
		response["registration_access_token"] = managementToken
		response["registration_client_uri"] = managementURL
		return dcrJSON(response), nil
	}

	var saved *model.ThirdpartyOAuth2ProviderEntity
	var createCalls, encryptCalls, decryptCalls int
	repo := &functionFieldProviderRepository{
		createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
			createCalls++
			saved = provider.Copy()
			return nil
		},
		getFn: func(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
			require.NotNil(t, saved)
			require.Equal(t, saved.ID, serviceID)
			return saved.Copy(), nil
		},
	}
	encryption := &functionFieldEncryption{
		encryptFn: func(_ context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
			encryptCalls++
			assert.Equal(t, []byte(clientSecret), plaintext)
			assert.Equal(t, map[string]string{"service_id": entity.ID.String()}, encryptionContext)
			return []byte("opaque-ciphertext"), nil
		},
		decryptFn: func(_ context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error) {
			decryptCalls++
			assert.Equal(t, []byte("opaque-ciphertext"), ciphertext)
			assert.Equal(t, map[string]string{"service_id": entity.ID.String()}, encryptionContext)
			return []byte(clientSecret), nil
		},
	}
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.New(slog.NewJSONHandler(logs, nil))).
		WithOAuthDiscoveryClient(client).
		WithCIMDPublicURL(dcrBrokerOrigin).
		WithDCRClientName("Example Platform")

	require.NoError(t, service.Create(ctx, entity))
	require.NotNil(t, saved)
	assert.Equal(t, 1, createCalls)
	assert.Equal(t, 1, encryptCalls)
	assert.Equal(t, discoveryIssuer, saved.IssuerURI)
	assert.Equal(t, id.ClientID("registered-client-42"), saved.ClientID)
	assert.Equal(t, model.ClientBootstrapDCR, saved.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodClientSecretBasic, saved.TokenEndpointAuthMethod)
	assert.Equal(t, "Files MCP", saved.DisplayName)
	assert.Equal(t, "https://audience.example.test/files", saved.AuthorizationParams["resource"])
	assert.True(t, saved.ResourceExplicit)
	assert.True(t, saved.Secret.IsEncrypted())
	ciphertext, err := saved.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, []byte("opaque-ciphertext"), ciphertext)
	assert.NotContains(t, string(ciphertext), clientSecret)
	persisted, err := json.Marshal(saved)
	require.NoError(t, err)
	assert.NotContains(t, string(persisted), managementToken)
	assert.NotContains(t, string(persisted), managementURL)

	for range 2 {
		loaded, err := service.Get(ctx, saved.ID)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, discoveryIssuer, loaded.IssuerURI)
		assert.Equal(t, saved.ClientID, loaded.ClientID)
		plaintext, err := loaded.Secret.GetPlaintext()
		require.NoError(t, err)
		assert.Equal(t, clientSecret, plaintext)
	}
	assert.Equal(t, 2, decryptCalls)
	assert.Equal(t, 1, createCalls)
	assert.Equal(t, 1, encryptCalls)
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register"}, client.calls, "loading the established identity must not register a new client")
	assert.NotContains(t, logs.String(), managementToken)
	assert.NotContains(t, logs.String(), managementURL)
	assert.NotContains(t, logs.String(), clientSecret)
}

func TestThirdpartyOAuth2ProviderService_Create_DuplicateDCRIdentityPreservesExistingCredential(t *testing.T) {
	const secondResource = "https://mcp.example.test/other"
	const secondMetadata = "https://mcp.example.test/.well-known/oauth-protected-resource/other"
	first := discoveredProvider(discoveryResource, "")
	second := discoveredProvider(secondResource, "")
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"none", "client_secret_basic"}, []string{"S256"}))
	client.probes[secondResource] = discoveryProbeReply{status: 200}
	client.gets[secondMetadata] = discoveryJSONReply{body: protectedResourceDocument(secondResource, discoveryIssuer)}
	registrations := 0
	client.postFn = func(_ context.Context, rawURL string, _ []byte) ([]byte, error) {
		require.Equal(t, discoveryIssuer+"/register", rawURL)
		registrations++
		switch registrations {
		case 1:
			return dcrJSON(dcrResponse(dcrCallback(first.ID), model.TokenEndpointAuthMethodClientSecretBasic, "original-secret")), nil
		case 2:
			return dcrJSON(dcrResponse(dcrCallback(second.ID), model.TokenEndpointAuthMethodClientSecretBasic, "replacement-secret")), nil
		default:
			return nil, errors.New("unexpected registration retry")
		}
	}

	var stored []*model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
		for _, existing := range stored {
			if existing.Discovery.ClientMethod == model.ClientBootstrapDCR && existing.IssuerURI == provider.IssuerURI && existing.ClientID == provider.ClientID {
				return storage.NewStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindConflict, storage.ErrDuplicateDCRClientIdentity, "provider DCR issuer and client_id already exist")
			}
		}
		stored = append(stored, provider.Copy())
		return nil
	}}
	encryption := &functionFieldEncryption{encryptFn: func(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
		switch string(plaintext) {
		case "original-secret":
			return []byte("opaque-original"), nil
		case "replacement-secret":
			return []byte("opaque-replacement"), nil
		default:
			return nil, errors.New("unexpected DCR credential")
		}
	}}
	logs := new(bytes.Buffer)
	service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.New(slog.NewJSONHandler(logs, nil))).
		WithOAuthDiscoveryClient(client).
		WithCIMDPublicURL(dcrBrokerOrigin).
		WithDCRClientName("Example Platform")

	require.NoError(t, service.Create(context.Background(), first))
	err := service.Create(context.Background(), second)
	assert.Equal(t, []string{
		"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register",
		"probe " + secondResource, "get " + secondMetadata, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register",
	}, client.calls, "duplicate registration must not retry as a public client")
	assert.Equal(t, 2, registrations)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	require.Len(t, stored, 1, "a duplicate identity must not create another service")
	var rejectedAudit map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["service_id"] == second.ID.String() && entry["outcome"] == "rejected" {
			rejectedAudit = entry
			break
		}
	}
	require.NotNil(t, rejectedAudit)
	assert.Equal(t, "create", rejectedAudit["operation"])
	assert.Equal(t, discoveryIssuer, rejectedAudit["issuer"])
	assert.Equal(t, "dcr", rejectedAudit["client_method"])
	assert.Equal(t, "duplicate_client_identity", rejectedAudit["failure_code"])
	assert.NotContains(t, logs.String(), "original-secret")
	assert.NotContains(t, logs.String(), "replacement-secret")
	assert.Equal(t, first.ID, stored[0].ID)
	assert.Equal(t, discoveryIssuer, stored[0].IssuerURI)
	assert.Equal(t, id.ClientID("registered-client-42"), stored[0].ClientID)
	secret, secretErr := stored[0].Secret.GetCiphertext()
	require.NoError(t, secretErr)
	assert.Equal(t, []byte("opaque-original"), secret, "rejected registration must not replace the active credential")
}

// refreshTestRepository keeps the service row and its failure-only status write
// separate, as the production storage port does. It rejects stale attempts.
type refreshTestRepository struct {
	*functionFieldProviderRepository
	stored       *model.ThirdpartyOAuth2ProviderEntity
	updates      int
	statusWrites int
	lastVersion  int64
	lastCode     string
}

func newRefreshTestRepository(stored *model.ThirdpartyOAuth2ProviderEntity) *refreshTestRepository {
	repo := &refreshTestRepository{stored: stored.Copy()}
	repo.functionFieldProviderRepository = &functionFieldProviderRepository{
		getFn: func(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
			if repo.stored.ID != serviceID {
				return nil, ports.ErrNotFound
			}
			return repo.stored.Copy(), nil
		},
		updateFn: func(_ context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
			if expectedVersion == nil || *expectedVersion != repo.stored.Version {
				return storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "stale service version")
			}
			previousVersion := repo.stored.Version
			repo.updates++
			repo.stored = entity.Copy()
			repo.stored.Version = previousVersion + 1
			return nil
		},
	}
	return repo
}

func (r *refreshTestRepository) RecordDiscoveryFailure(_ context.Context, serviceID id.ServiceID, expectedVersion int64, completedAt time.Time, failureCode string) error {
	r.statusWrites++
	r.lastVersion = expectedVersion
	r.lastCode = failureCode
	if r.stored.ID != serviceID || r.stored.Version != expectedVersion ||
		r.stored.Discovery.ResourceURL == nil || r.stored.DiscoveryStatus.LastAttemptAt == nil ||
		!completedAt.After(*r.stored.DiscoveryStatus.LastAttemptAt) ||
		(r.stored.DiscoveryStatus.LastSuccessAt != nil && !completedAt.After(*r.stored.DiscoveryStatus.LastSuccessAt)) {
		return storage.NewStorageError("RecordDiscoveryFailure", storage.ErrorKindConflict, nil, "stale discovery attempt")
	}
	r.stored.DiscoveryStatus.LastAttemptAt = &completedAt
	r.stored.DiscoveryStatus.FailureReason = &failureCode
	return nil
}

var _ ports.ThirdpartyOAuth2ProviderDiscoveryStatusWriter = (*refreshTestRepository)(nil)

type refreshSessionRepository struct {
	ports.UserSessionRepository
	count int
	calls int
}

func (r *refreshSessionRepository) CountByService(context.Context, id.ServiceID) (int, error) {
	r.calls++
	return r.count, nil
}

func readyRefreshProvider(method model.TokenEndpointAuthMethod) *model.ThirdpartyOAuth2ProviderEntity {
	provider := discoveredProvider(discoveryResource, discoveryIssuer)
	provider.ClientID = id.ClientID("original-registered-client")
	provider.Discovery.ClientMethod = model.ClientBootstrapDCR
	provider.TokenEndpointAuthMethod = method
	provider.Secret = model.NewEncryptedSecret([]byte("original-sealed-secret"))
	provider.Endpoints = model.OAuth2Endpoints{AuthorizeEndpoint: discoveryIssuer + "/authorize", TokenEndpoint: discoveryIssuer + "/token"}
	provider.AuthorizationParams = map[string]string{"resource": discoveryResource}
	provider.Version = 7
	succeededAt := time.Now().UTC().Add(-time.Hour)
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &succeededAt, LastSuccessAt: &succeededAt}
	return provider
}

func refreshRequest(stored *model.ThirdpartyOAuth2ProviderEntity) *model.ThirdpartyOAuth2ProviderEntity {
	return discoveredProvider(*stored.Discovery.ResourceURL, "")
}

func refreshTestService(repo *refreshTestRepository, client ports.OAuthDiscoveryClient, encryption ports.EncryptionPort, logs *bytes.Buffer) *ThirdpartyOAuth2ProviderService {
	return NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.New(slog.NewJSONHandler(logs, nil))).
		WithOAuthDiscoveryClient(client).
		WithCIMDPublicURL(dcrBrokerOrigin).
		WithCIMDKeyReadiness(readyCIMDKeyReadiness{}).
		WithDCRClientName("Example Platform").
		WithUserSessions(&refreshSessionRepository{}).
		WithDiscoveryStatusWriter(repo)
}

func TestThirdpartyOAuth2ProviderService_Update_RefreshKeepsExactDCRCredentialAndOmittedOverride(t *testing.T) {
	ctx := context.Background()
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretBasic)
	stored.AuthorizationParams["resource"] = "https://audience.example.test/explicit"
	stored.ResourceExplicit = true
	repo := newRefreshTestRepository(stored)
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_post", "client_secret_basic", "none"}, []string{"S256"}))
	client.gets[discoveryIssuerOAuth] = discoveryJSONReply{body: []byte(`{"issuer":"https://auth.example.test/tenant","authorization_endpoint":"https://auth.example.test/new-authorize","token_endpoint":"https://auth.example.test/new-token","registration_endpoint":"https://auth.example.test/register","token_endpoint_auth_methods_supported":["client_secret_post","client_secret_basic","none"]}`)}
	logs := new(bytes.Buffer)
	service := refreshTestService(repo, client, &functionFieldEncryption{}, logs)
	replacement := refreshRequest(stored)
	replacement.ID = stored.ID

	require.NoError(t, service.Update(ctx, replacement, &stored.Version))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls, "refresh must not re-register or select the newly preferred POST method")
	assert.Equal(t, 1, repo.updates)
	assert.Zero(t, repo.statusWrites)
	assert.Equal(t, stored.ClientID, repo.stored.ClientID)
	assert.Equal(t, stored.Discovery.ClientMethod, repo.stored.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodClientSecretBasic, repo.stored.TokenEndpointAuthMethod)
	assert.Equal(t, stored.Secret, repo.stored.Secret)
	assert.Equal(t, "https://auth.example.test/new-token", repo.stored.Endpoints.TokenEndpoint)
	assert.Equal(t, "https://audience.example.test/explicit", repo.stored.AuthorizationParams["resource"])
	assert.True(t, repo.stored.ResourceExplicit)
	assert.Equal(t, stored.Version+1, repo.stored.Version)
	require.NotNil(t, repo.stored.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, repo.stored.DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, *repo.stored.DiscoveryStatus.LastAttemptAt, *repo.stored.DiscoveryStatus.LastSuccessAt)
	assert.True(t, repo.stored.DiscoveryStatus.LastSuccessAt.After(*stored.DiscoveryStatus.LastSuccessAt))
	assert.Nil(t, repo.stored.DiscoveryStatus.FailureReason)
	view, err := service.GetDiscoveryStatus(ctx, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, "ready", view.Status)
	require.NotNil(t, view.IssuerURI)
	assert.Equal(t, discoveryIssuer, *view.IssuerURI)
	assert.NotContains(t, logs.String(), "original-sealed-secret")
}

func TestThirdpartyOAuth2ProviderService_Update_ExplicitIssuerChangeRegistersOneClientWithStoredPOSTMethod(t *testing.T) {
	const nextIssuer = "https://other-auth.example.test/tenant"
	const nextMetadata = "https://other-auth.example.test/.well-known/oauth-authorization-server/tenant"
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretPost)
	repo := newRefreshTestRepository(stored)
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_post"}, nil))
	client.gets[discoveryResourcePath] = discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer, nextIssuer)}
	client.gets[nextMetadata] = discoveryJSONReply{body: []byte(`{"issuer":"https://other-auth.example.test/tenant","authorization_endpoint":"https://other-auth.example.test/tenant/authorize","token_endpoint":"https://other-auth.example.test/tenant/token","registration_endpoint":"https://other-auth.example.test/tenant/register","token_endpoint_auth_methods_supported":["client_secret_basic","client_secret_post","none"]}`)}
	registrationCount := 0
	client.postFn = func(_ context.Context, rawURL string, body []byte) ([]byte, error) {
		registrationCount++
		assert.Equal(t, nextIssuer+"/register", rawURL)
		var registration map[string]any
		require.NoError(t, json.Unmarshal(body, &registration))
		assert.Equal(t, string(model.TokenEndpointAuthMethodClientSecretPost), registration["token_endpoint_auth_method"], "the old POST method wins over the new issuer's Basic preference")
		assert.Equal(t, "Example Platform", registration["client_name"])
		response := dcrResponse(dcrCallback(stored.ID), model.TokenEndpointAuthMethodClientSecretPost, "new-issued-secret")
		response["client_id"] = "new-issuer-client"
		return dcrJSON(response), nil
	}
	encryptions := 0
	encryption := &functionFieldEncryption{encryptFn: func(_ context.Context, plaintext []byte, aad map[string]string) ([]byte, error) {
		encryptions++
		assert.Equal(t, []byte("new-issued-secret"), plaintext)
		assert.Equal(t, map[string]string{"service_id": stored.ID.String()}, aad)
		return []byte("new-sealed-secret"), nil
	}}
	service := refreshTestService(repo, client, encryption, new(bytes.Buffer))
	replacement := refreshRequest(stored)
	replacement.IssuerURI = nextIssuer
	replacement.ID = stored.ID

	require.NoError(t, service.Update(context.Background(), replacement, &stored.Version))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + nextMetadata, "post " + nextIssuer + "/register"}, client.calls)
	assert.Equal(t, 1, registrationCount)
	assert.Equal(t, 1, encryptions)
	assert.Equal(t, nextIssuer, repo.stored.IssuerURI)
	assert.Equal(t, id.ClientID("new-issuer-client"), repo.stored.ClientID)
	assert.Equal(t, model.ClientBootstrapDCR, repo.stored.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodClientSecretPost, repo.stored.TokenEndpointAuthMethod)
	secret, err := repo.stored.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, []byte("new-sealed-secret"), secret)
	assert.Equal(t, stored.Version+1, repo.stored.Version)
	assert.Zero(t, repo.statusWrites)
}

func TestThirdpartyOAuth2ProviderService_Update_DuplicateNewIssuerClientRecordsFailedStatus(t *testing.T) {
	const nextIssuer = "https://other-auth.example.test/tenant"
	const nextMetadata = "https://other-auth.example.test/.well-known/oauth-authorization-server/tenant"
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretPost)
	repo := newRefreshTestRepository(stored)
	repo.updateFn = func(_ context.Context, _ *model.ThirdpartyOAuth2ProviderEntity, version *int64) error {
		require.Equal(t, stored.Version, *version)
		return storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict,
			storage.ErrDuplicateDCRClientIdentity, "provider DCR issuer and client_id already exist")
	}
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_post"}, nil))
	client.gets[discoveryResourcePath] = discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer, nextIssuer)}
	client.gets[nextMetadata] = discoveryJSONReply{body: []byte(strings.ReplaceAll(string(issuerDCRDocument([]string{"client_secret_post"}, nil)), discoveryIssuer, nextIssuer))}
	registrations := 0
	client.postFn = func(_ context.Context, rawURL string, _ []byte) ([]byte, error) {
		registrations++
		assert.Equal(t, nextIssuer+"/register", rawURL)
		return dcrJSON(dcrResponse(dcrCallback(stored.ID), model.TokenEndpointAuthMethodClientSecretPost, "new-issued-secret")), nil
	}
	encryption := &functionFieldEncryption{encryptFn: func(_ context.Context, plaintext []byte, aad map[string]string) ([]byte, error) {
		assert.Equal(t, []byte("new-issued-secret"), plaintext)
		assert.Equal(t, map[string]string{"service_id": stored.ID.String()}, aad)
		return []byte("new-sealed-secret"), nil
	}}
	logs := new(bytes.Buffer)
	service := refreshTestService(repo, client, encryption, logs)
	replacement := refreshRequest(stored)
	replacement.ID = stored.ID
	replacement.IssuerURI = nextIssuer

	err := service.Update(context.Background(), replacement, &stored.Version)
	require.ErrorIs(t, err, storage.ErrDuplicateDCRClientIdentity)
	assert.Equal(t, 1, registrations)
	assert.Equal(t, 1, repo.statusWrites)
	assert.Equal(t, stored.Version, repo.lastVersion)
	assert.Equal(t, "duplicate_client_identity", repo.lastCode)
	assert.Zero(t, repo.updates)
	assert.Equal(t, stored.IssuerURI, repo.stored.IssuerURI)
	assert.Equal(t, stored.ClientID, repo.stored.ClientID)
	assert.Equal(t, stored.Secret, repo.stored.Secret)
	assert.Equal(t, stored.Version, repo.stored.Version)
	assert.Equal(t, stored.DiscoveryStatus.LastSuccessAt, repo.stored.DiscoveryStatus.LastSuccessAt)
	require.NotNil(t, repo.stored.DiscoveryStatus.FailureReason)
	assert.Equal(t, "duplicate_client_identity", *repo.stored.DiscoveryStatus.FailureReason)
	var audit map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &audit))
	assert.Equal(t, nextIssuer, audit["issuer"])
	assert.Equal(t, "duplicate_client_identity", audit["failure_code"])
	assert.NotContains(t, logs.String(), "new-issued-secret")
}

func TestThirdpartyOAuth2ProviderService_Update_CIMDIssuerChangeKeepsHostedIdentityWithoutDCR(t *testing.T) {
	const nextIssuer = "https://other-auth.example.test/tenant"
	const nextMetadata = "https://other-auth.example.test/.well-known/oauth-authorization-server/tenant"
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodPrivateKeyJWT)
	stored.Discovery.ClientMethod = model.ClientBootstrapCIMD
	stored.ClientID = id.ClientID(dcrBrokerOrigin + "/.well-known/oauth-client/" + stored.ID.String())
	stored.Secret = model.NewAbsentSecret()
	repo := newRefreshTestRepository(stored)
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
	client.gets[discoveryResourcePath] = discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer, nextIssuer)}
	client.gets[nextMetadata] = discoveryJSONReply{body: issuerCIMDDocument(nextIssuer, nextIssuer+"/authorize")}
	service := refreshTestService(repo, client, &functionFieldEncryption{}, new(bytes.Buffer))
	replacement := refreshRequest(stored)
	replacement.ID = stored.ID
	replacement.IssuerURI = nextIssuer

	require.NoError(t, service.Update(context.Background(), replacement, &stored.Version))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + nextMetadata}, client.calls, "an existing hosted client must not be registered through DCR")
	assert.Equal(t, nextIssuer, repo.stored.IssuerURI)
	assert.Equal(t, stored.ClientID, repo.stored.ClientID)
	assert.Equal(t, model.ClientBootstrapCIMD, repo.stored.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodPrivateKeyJWT, repo.stored.TokenEndpointAuthMethod)
	assert.True(t, repo.stored.Secret.IsAbsent())
	assert.Equal(t, nextIssuer+"/token", repo.stored.Endpoints.TokenEndpoint)
	assert.Equal(t, stored.Version+1, repo.stored.Version)
	assert.Zero(t, repo.statusWrites)
}

func TestThirdpartyOAuth2ProviderService_Update_IssuerChangeRequiresZeroSessionsBeforeDiscovery(t *testing.T) {
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretBasic)
	repo := newRefreshTestRepository(stored)
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
	sessions := &refreshSessionRepository{count: 1}
	logs := new(bytes.Buffer)
	service := refreshTestService(repo, client, &functionFieldEncryption{}, logs).WithUserSessions(sessions)
	replacement := refreshRequest(stored)
	replacement.ID = stored.ID
	replacement.IssuerURI = "https://other-auth.example.test/tenant"

	err := service.Update(context.Background(), replacement, &stored.Version)
	require.ErrorContains(t, err, "issuer_change_requires_no_sessions")
	assert.Equal(t, 1, sessions.calls)
	assert.Empty(t, client.calls, "an active session must block discovery and registration at the new issuer")
	assert.Zero(t, repo.updates)
	assert.Equal(t, 1, repo.statusWrites)
	assert.Equal(t, "issuer_change_requires_no_sessions", repo.lastCode)
	assert.Equal(t, stored.ClientID, repo.stored.ClientID)
	assert.Equal(t, stored.Secret, repo.stored.Secret)
	assert.Equal(t, stored.Version, repo.stored.Version)
	assert.Equal(t, "failed", repo.stored.DiscoveryStatus.Status(repo.stored.Discovery.ResourceURL))
	var audit map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &audit))
	assert.Equal(t, stored.IssuerURI, audit["issuer"])
	assert.Equal(t, string(stored.Discovery.ClientMethod), audit["client_method"])
	assert.Equal(t, "issuer_change_requires_no_sessions", audit["failure_code"])
	assert.NotContains(t, logs.String(), replacement.IssuerURI, "the requested issuer has not been verified")
}

func TestThirdpartyOAuth2ProviderService_Update_RejectsAuthenticationDowngradesWithoutReplacingActiveService(t *testing.T) {
	const nextIssuer = "https://other-auth.example.test/tenant"
	const nextMetadata = "https://other-auth.example.test/.well-known/oauth-authorization-server/tenant"
	cases := []struct {
		name         string
		method       model.TokenEndpointAuthMethod
		metadata     []byte
		cimd         bool
		issuerChange bool
	}{
		{name: "Basic-to-POST", method: model.TokenEndpointAuthMethodClientSecretBasic, metadata: issuerDCRDocument([]string{"client_secret_post", "none"}, []string{"S256"})},
		{name: "confidential-to-public-on-new-issuer", method: model.TokenEndpointAuthMethodClientSecretBasic, metadata: issuerDCRDocument([]string{"none"}, []string{"S256"}), issuerChange: true},
		{name: "CIMD-to-DCR-on-new-issuer", method: model.TokenEndpointAuthMethodPrivateKeyJWT, metadata: issuerDCRDocument([]string{"client_secret_basic", "none"}, []string{"S256"}), cimd: true, issuerChange: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := readyRefreshProvider(tc.method)
			if tc.cimd {
				stored.Discovery.ClientMethod = model.ClientBootstrapCIMD
				stored.ClientID = id.ClientID(dcrBrokerOrigin + "/.well-known/oauth-client/" + stored.ID.String())
				stored.Secret = model.NewAbsentSecret()
			}
			repo := newRefreshTestRepository(stored)
			client := dcrDiscoveryClient(tc.metadata)
			wantCalls := []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}
			if tc.issuerChange {
				client.gets[discoveryResourcePath] = discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer, nextIssuer)}
				client.gets[nextMetadata] = discoveryJSONReply{body: []byte(strings.ReplaceAll(string(tc.metadata), discoveryIssuer, nextIssuer))}
				wantCalls[2] = "get " + nextMetadata
			}
			logs := new(bytes.Buffer)
			service := refreshTestService(repo, client, &functionFieldEncryption{}, logs)
			replacement := refreshRequest(stored)
			replacement.ID = stored.ID
			if tc.issuerChange {
				replacement.IssuerURI = nextIssuer
			}

			err := service.Update(context.Background(), replacement, &stored.Version)
			require.ErrorContains(t, err, "client_method_changed")
			assert.Equal(t, wantCalls, client.calls, "do not fall back to another client bootstrap or token authentication method")
			assert.Zero(t, repo.updates)
			assert.Equal(t, 1, repo.statusWrites, "a remote refresh failure must persist a safe failure status")
			assert.Equal(t, stored.Version, repo.lastVersion)
			assert.Equal(t, "client_method_changed", repo.lastCode)
			assert.Equal(t, stored.Version, repo.stored.Version, "failure must not change ETag")
			assert.Equal(t, stored.IssuerURI, repo.stored.IssuerURI)
			assert.Equal(t, stored.ClientID, repo.stored.ClientID)
			assert.Equal(t, stored.Discovery.ClientMethod, repo.stored.Discovery.ClientMethod)
			assert.Equal(t, stored.TokenEndpointAuthMethod, repo.stored.TokenEndpointAuthMethod)
			assert.Equal(t, stored.Secret, repo.stored.Secret)
			assert.Equal(t, stored.Endpoints, repo.stored.Endpoints)
			assert.Equal(t, stored.AuthorizationParams, repo.stored.AuthorizationParams)
			assert.Equal(t, stored.DiscoveryStatus.LastSuccessAt, repo.stored.DiscoveryStatus.LastSuccessAt)
			require.NotNil(t, repo.stored.DiscoveryStatus.FailureReason)
			assert.Equal(t, "client_method_changed", *repo.stored.DiscoveryStatus.FailureReason)
			require.NotNil(t, repo.stored.DiscoveryStatus.LastAttemptAt)
			assert.True(t, repo.stored.DiscoveryStatus.LastAttemptAt.After(*stored.DiscoveryStatus.LastAttemptAt))
			view, viewErr := service.GetDiscoveryStatus(context.Background(), stored.ID)
			require.NoError(t, viewErr)
			require.NotNil(t, view.IssuerURI)
			assert.Equal(t, "failed", view.Status)
			assert.Equal(t, discoveryIssuer, *view.IssuerURI)
			assert.NotContains(t, logs.String(), "original-sealed-secret")
		})
	}
}

// The issuer metadata read is the concurrency point: a later refresh can commit
// while this earlier read is in flight. Its subsequent failure must not win.
type interleavedRefreshClient struct {
	*recordingOAuthDiscoveryClient
	beforeIssuerRead func()
}

func (c *interleavedRefreshClient) GetJSON(ctx context.Context, rawURL string) ([]byte, error) {
	if rawURL == discoveryIssuerOAuth {
		c.beforeIssuerRead()
	}
	return c.recordingOAuthDiscoveryClient.GetJSON(ctx, rawURL)
}

func TestThirdpartyOAuth2ProviderService_Update_StaleFailureDoesNotReplaceNewerSuccessOrLeakProviderBody(t *testing.T) {
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretBasic)
	repo := newRefreshTestRepository(stored)
	const hostileBody = `{"issuer":"https://auth.example.test/tenant","authorization_endpoint":"https://auth.example.test/authorize?access_token=private-token","client_secret":"provider-secret"}`
	client := &interleavedRefreshClient{recordingOAuthDiscoveryClient: dcrDiscoveryClient([]byte(hostileBody))}
	client.beforeIssuerRead = func() {
		repo.stored.Endpoints.TokenEndpoint = discoveryIssuer + "/new-token"
		repo.stored.Version++
		committedAt := time.Now().UTC()
		repo.stored.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &committedAt, LastSuccessAt: &committedAt}
	}
	logs := new(bytes.Buffer)
	service := refreshTestService(repo, client, &functionFieldEncryption{}, logs)
	replacement := refreshRequest(stored)
	replacement.ID = stored.ID

	err := service.Update(context.Background(), replacement, &stored.Version)
	require.Error(t, err)
	require.ErrorContains(t, err, "authorization_server_metadata_invalid")
	assert.Equal(t, 1, repo.statusWrites, "the failure writer receives the observed, not the newly committed, version")
	assert.Equal(t, stored.Version, repo.lastVersion)
	assert.Equal(t, "authorization_server_metadata_invalid", repo.lastCode)
	assert.Zero(t, repo.updates)
	assert.Equal(t, stored.Version+1, repo.stored.Version)
	assert.Equal(t, discoveryIssuer+"/new-token", repo.stored.Endpoints.TokenEndpoint)
	assert.Equal(t, stored.ClientID, repo.stored.ClientID)
	assert.Equal(t, stored.Secret, repo.stored.Secret)
	assert.Nil(t, repo.stored.DiscoveryStatus.FailureReason)
	assert.Equal(t, repo.stored.DiscoveryStatus.LastSuccessAt, repo.stored.DiscoveryStatus.LastAttemptAt)
	assert.Equal(t, "ready", repo.stored.DiscoveryStatus.Status(repo.stored.Discovery.ResourceURL))
	var rejectedAudit map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		if event["operation"] == "update" && event["outcome"] == "rejected" {
			rejectedAudit = event
			break
		}
	}
	require.NotNil(t, rejectedAudit)
	assert.Equal(t, stored.ID.String(), rejectedAudit["service_id"])
	assert.Equal(t, "authorization_server_metadata_invalid", rejectedAudit["failure_code"])
	assert.NotEqual(t, "https://auth.example.test/tenant?access_token=private-token", rejectedAudit["issuer"], "audit issuer must never come from an unverified provider claim")
	assert.NotContains(t, logs.String(), "private-token")
	assert.NotContains(t, logs.String(), "provider-secret")
	assert.NotContains(t, logs.String(), "original-sealed-secret")
}

func TestThirdpartyOAuth2ProviderService_Update_ResourceSourceTracksVerifiedURLAndExplicitReplacement(t *testing.T) {
	const otherResource = "https://mcp.example.test/other"
	const otherMetadata = "https://mcp.example.test/.well-known/oauth-protected-resource/other"
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretBasic)
	repo := newRefreshTestRepository(stored)
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
	client.probes[otherResource] = discoveryProbeReply{status: 200}
	client.gets[otherMetadata] = discoveryJSONReply{body: protectedResourceDocument(otherResource, discoveryIssuer)}
	service := refreshTestService(repo, client, &functionFieldEncryption{}, new(bytes.Buffer))

	update := func(resource string, params map[string]string) {
		t.Helper()
		replacement := discoveredProvider(resource, "")
		replacement.ID = stored.ID
		replacement.AuthorizationParams = params
		version := repo.stored.Version
		require.NoError(t, service.Update(context.Background(), replacement, &version))
		assert.Equal(t, stored.ClientID, repo.stored.ClientID)
		assert.Equal(t, stored.Secret, repo.stored.Secret)
		assert.Equal(t, model.TokenEndpointAuthMethodClientSecretBasic, repo.stored.TokenEndpointAuthMethod)
	}

	update(otherResource, nil)
	assert.Equal(t, otherResource, repo.stored.AuthorizationParams["resource"], "derived resource follows a new verified URL")
	assert.False(t, repo.stored.ResourceExplicit)
	update(otherResource, map[string]string{"resource": "https://audience.example.test/explicit"})
	assert.True(t, repo.stored.ResourceExplicit)
	update(discoveryResource, nil)
	assert.Equal(t, "https://audience.example.test/explicit", repo.stored.AuthorizationParams["resource"], "omitted authorization_params retains the explicit override")
	assert.True(t, repo.stored.ResourceExplicit)
	update(discoveryResource, map[string]string{"prompt": "consent"})
	assert.Equal(t, discoveryResource, repo.stored.AuthorizationParams["resource"], "replacement without resource restores the verified URL")
	assert.Equal(t, "consent", repo.stored.AuthorizationParams["prompt"])
	assert.False(t, repo.stored.ResourceExplicit)
	assert.Zero(t, repo.statusWrites)
	for _, call := range client.calls {
		assert.NotContains(t, call, "post ", "same-issuer resource changes must not register another client")
	}
}

func TestThirdpartyOAuth2ProviderService_Update_InvalidRequestDoesNotRecordDiscoveryAttempt(t *testing.T) {
	stored := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretBasic)
	repo := newRefreshTestRepository(stored)
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
	service := refreshTestService(repo, client, &functionFieldEncryption{}, new(bytes.Buffer))
	replacement := refreshRequest(stored)
	replacement.ID = stored.ID
	badResource := "https://mcp.example.test/mcp#fragment"
	replacement.Discovery.ResourceURL = &badResource

	require.Error(t, service.Update(context.Background(), replacement, &stored.Version))
	assert.Empty(t, client.calls, "request-shape failures must happen before untrusted network access")
	assert.Zero(t, repo.statusWrites)
	assert.Zero(t, repo.updates)
	assert.Equal(t, stored.Version, repo.stored.Version)
	assert.Equal(t, stored.DiscoveryStatus, repo.stored.DiscoveryStatus)
	assert.Equal(t, stored.Secret, repo.stored.Secret)
}

func TestThirdpartyOAuth2ProviderService_AuditFinalIssuerConflictUsesSafeCode(t *testing.T) {
	logs := new(bytes.Buffer)
	service := &ThirdpartyOAuth2ProviderService{logger: slog.New(slog.NewJSONHandler(logs, nil))}
	provider := readyRefreshProvider(model.TokenEndpointAuthMethodClientSecretBasic)
	err := storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict,
		storage.ErrIssuerChangeHasSessions, "provider has user sessions")
	service.auditDiscovery(provider, "update", err, provider)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
	assert.Equal(t, "rejected", entry["outcome"])
	assert.Equal(t, "issuer_change_requires_no_sessions", entry["failure_code"])
	assert.Equal(t, provider.IssuerURI, entry["issuer"])
	assert.NotContains(t, logs.String(), "original-sealed-secret")
}
