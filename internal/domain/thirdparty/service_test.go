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
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

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

func TestThirdpartyOAuth2ProviderService_FindByProtectedResource_DecryptsSecret(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	storedEntity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     svcID,
		Secret: model.NewEncryptedSecret([]byte("ciphertext")),
	}
	mockRepo.On("FindByProtectedResource", ctx, "https://api.example.com").Return(storedEntity, nil)
	mockEnc.On("Decrypt", ctx, []byte("ciphertext"), map[string]string{"service_id": svcID.String()}).
		Return([]byte("plaintext"), nil)

	result, err := svc.FindByProtectedResource(ctx, "https://api.example.com")

	require.NoError(t, err)
	assert.Equal(t, svcID, result.ID)
	assert.True(t, result.Secret.IsPlaintext())
	mockEnc.AssertExpectations(t)
	mockRepo.AssertExpectations(t)
}

func TestThirdpartyOAuth2ProviderService_FindByProtectedResource_DecryptionFailure_ReturnsEncryptedEntity(t *testing.T) {
	mockRepo := new(MockRepository)
	mockEnc := new(MockEncryption)
	svc := NewThirdpartyOAuth2ProviderService(mockRepo, mockEnc, newNoopBranchKeyManager(), nil, false, slog.Default())

	ctx := context.Background()
	svcID := id.NewServiceID()
	storedEntity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:     svcID,
		Secret: model.NewEncryptedSecret([]byte("old-ciphertext")),
	}
	mockRepo.On("FindByProtectedResource", ctx, "https://api.example.com").Return(storedEntity, nil)
	mockEnc.On("Decrypt", ctx, []byte("old-ciphertext"), map[string]string{"service_id": svcID.String()}).
		Return(nil, errors.New("decryption failed: wrong encryption backend"))

	result, err := svc.FindByProtectedResource(ctx, "https://api.example.com")

	// Returns entity with encrypted secret instead of failing
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, svcID, result.ID)
	assert.True(t, result.Secret.IsEncrypted(), "secret should remain encrypted on decryption failure")
	mockEnc.AssertExpectations(t)
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
