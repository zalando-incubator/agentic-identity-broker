package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

// testConfig creates a minimal config for tests with HTTPS validation enabled (strict mode).
func testConfig() *ports.Config {
	return &ports.Config{
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{PublicURL: "https://broker.example.com"},
		},
		Security: ports.SecurityConfig{
			SkipThirdpartyHTTPSValidation: false,
		},
	}
}

// MockProviderRepository is a mock implementation of ports.ThirdpartyOAuth2ProviderRepository.
type MockProviderRepository struct {
	mock.Mock
}

func (m *MockProviderRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx, serviceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockProviderRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx, canonicalID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockProviderRepository) GetCanonicalIDs(ctx context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error) {
	args := m.Called(ctx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[id.ServiceID]string), args.Error(1)
}

func (m *MockProviderRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	args := m.Called(ctx, entity, expectedVersion)
	return args.Error(0)
}

func (m *MockProviderRepository) RecordDiscoveryFailure(ctx context.Context, serviceID id.ServiceID, version int64, completedAt time.Time, code string) error {
	args := m.Called(ctx, serviceID, version, completedAt, code)
	return args.Error(0)
}

func (m *MockProviderRepository) Delete(ctx context.Context, serviceID id.ServiceID) error {
	args := m.Called(ctx, serviceID)
	return args.Error(0)
}

func (m *MockProviderRepository) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockProviderRepository) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	args := m.Called(ctx, resourceURI)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ThirdpartyOAuth2ProviderEntity), args.Error(1)
}

func (m *MockProviderRepository) AddProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	args := m.Called(ctx, serviceID, resourceURI)
	return args.Get(0).(ports.ProtectedResourceMutationResult), args.Error(1)
}

func (m *MockProviderRepository) RemoveProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	args := m.Called(ctx, serviceID, resourceURI)
	return args.Get(0).(ports.ProtectedResourceMutationResult), args.Error(1)
}

func (m *MockProviderRepository) RenameProtectedResource(ctx context.Context, serviceID id.ServiceID, fromURI, toURI string) (ports.ProtectedResourceMutationResult, error) {
	args := m.Called(ctx, serviceID, fromURI, toURI)
	return args.Get(0).(ports.ProtectedResourceMutationResult), args.Error(1)
}

func (m *MockProviderRepository) ListProtectedResources(ctx context.Context, serviceID id.ServiceID) ([]string, int64, error) {
	args := m.Called(ctx, serviceID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]string), args.Get(1).(int64), args.Error(2)
}

// newTestEncryption returns a real encryption adapter backed by the shared deterministic test key.
func newTestEncryption() ports.EncryptionPort {
	return testutil.NewPanicTestEncryptionAdapter()
}

type readyCIMDKeyReadiness struct{}

func (readyCIMDKeyReadiness) RequireUsablePublishedKey(context.Context) error { return nil }

// setupHandler creates a handler backed by a mock repository and test encryption.
func setupHandler(t *testing.T, mockRepo *MockProviderRepository) *ServicesHandler {
	return setupHandlerWithConfig(t, mockRepo, testConfig())
}

func setupHandlerWithConfig(t *testing.T, mockRepo *MockProviderRepository, config *ports.Config) *ServicesHandler {
	t.Helper()
	svc := thirdparty.NewThirdpartyOAuth2ProviderService(mockRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default()).
		WithCIMDPublicURL(config.Server.EndUser.PublicURL).
		WithCIMDKeyReadiness(readyCIMDKeyReadiness{}).
		WithDiscoveryStatusWriter(mockRepo)
	return NewServicesHandler(svc, config, slog.Default())
}

// encryptSecretForTest encrypts a plaintext secret using the test encryption adapter.
// The service_id is used as the encryption context.
func encryptSecretForTest(serviceID, secret string) []byte {
	enc := newTestEncryption()
	ciphertext, err := enc.Encrypt(context.Background(), []byte(secret), map[string]string{"service_id": serviceID})
	if err != nil {
		panic("encryptSecretForTest: " + err.Error())
	}
	return ciphertext
}

// encryptedEntity creates an entity with encrypted secret (as it would come from the repository).
// The secret is properly encrypted with the test key so the domain service can decrypt it.
func encryptedEntity(serviceID id.ServiceID, displayName string, clientID id.ClientID, clientSecret, issuerURI string, scopes []model.OAuthScope) *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		DisplayName: displayName,
		ClientID:    clientID,
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(serviceID.String(), clientSecret)),
		IssuerURI:   issuerURI,
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://" + strings.TrimPrefix(issuerURI, "https://") + "/token",
			AuthorizeEndpoint: "https://" + strings.TrimPrefix(issuerURI, "https://") + "/authorize",
		},
		Scopes:    scopes,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestServicesHandler_CreateService(t *testing.T) {
	t.Run("successful creation without discovery", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "GitHub",
			ClientID:     "github-client-id",
			ClientSecret: "github-client-secret",
			IssuerURI:    "https://github.com",
			Discovery: DiscoveryConfigRequest{
				EnableDiscovery: false,
			},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://github.com/login/oauth/access_token",
				AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			},
			Scopes: []OAuthScopeRequest{
				{ScopeValue: "repo", Description: "Repository access"},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		// The domain service encrypts and then calls repo.Create with encrypted entity.
		mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
			return e.DisplayName == "GitHub" && e.ClientID == "github-client-id" && e.Secret.IsEncrypted()
		})).Return(nil)

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var resp ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, "GitHub", resp.DisplayName)
		assert.Equal(t, "github-client-id", resp.ClientID)
		require.NotNil(t, resp.ClientSecret)
		assert.Equal(t, "REDACTED", *resp.ClientSecret) // Secret must be redacted
		assert.False(t, resp.Discovery.EnableDiscovery)
		assert.Len(t, resp.Scopes, 1)

		mockRepo.AssertExpectations(t)
	})

	t.Run("creates a scope-less service when scopes are omitted", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		reqBody := ServiceRequest{
			DisplayName:  "Scope-less IdP",
			ClientID:     "scope-less-client",
			ClientSecret: "scope-less-secret",
			IssuerURI:    "https://idp.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints:    &OAuth2EndpointsRequest{TokenEndpoint: "https://idp.example.com/token", AuthorizeEndpoint: "https://idp.example.com/authorize"},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
			return len(entity.Scopes) == 0
		})).Return(nil)

		w := httptest.NewRecorder()
		handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))

		require.Equal(t, http.StatusCreated, w.Code)
		var response ServiceResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Empty(t, response.Scopes)
		mockRepo.AssertExpectations(t)
	})

	t.Run("maps authorization_params", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		reqBody := ServiceRequest{DisplayName: "Provider", ClientID: "client", ClientSecret: "secret", IssuerURI: "https://example.com", Discovery: DiscoveryConfigRequest{}, Endpoints: &OAuth2EndpointsRequest{TokenEndpoint: "https://example.com/token", AuthorizeEndpoint: "https://example.com/authorize"}, Scopes: []OAuthScopeRequest{{ScopeValue: "read", Description: "Read"}}, AuthorizationParams: map[string]string{"business_partner_id": "12345"}}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
			return entity.AuthorizationParams["business_partner_id"] == "12345"
		})).Return(nil)
		w := httptest.NewRecorder()
		handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))
		require.Equal(t, http.StatusCreated, w.Code)
		var response ServiceResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "12345", response.AuthorizationParams["business_partner_id"])
		mockRepo.AssertExpectations(t)
	})

	t.Run("invalid request body", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid request body", resp.Error)
	})

	t.Run("validation error - missing display name", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName: "", // Invalid: empty display name
			ClientID:    "test-client",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		// Validation happens before any repo call
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "validation failed", resp.Error)
		assert.Contains(t, resp.Message, "display_name is required")

		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("protected_resources are normalized before create", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes: []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{
				"https://api.example.com/",
				"https://api.example.com/v2",
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("no service configured for the requested resource", "resource_not_found")).Once()
		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com/v2").
			Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("no service configured for the requested resource", "resource_not_found")).Once()

		mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
			return len(e.ProtectedResources) == 2 &&
				e.ProtectedResources[0] == "https://api.example.com" &&
				e.ProtectedResources[1] == "https://api.example.com/v2"
		})).Return(nil)

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"https://api.example.com",
			"https://api.example.com/v2",
		}, resp.ProtectedResources)

		mockRepo.AssertExpectations(t)
	})

	t.Run("normalized protected_resource conflict returns 409 on create", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		conflictingServiceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes:             []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{"https://api.example.com/"},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(&model.ThirdpartyOAuth2ProviderEntity{ID: conflictingServiceID}, nil).Once()

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "conflict", resp.Error)
		assert.Contains(t, resp.Message, "protected resource URI already configured for another service")

		mockRepo.AssertNotCalled(t, "Create")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ambiguous protected_resource lookup returns conflict on create", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes:             []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{"https://api.example.com/"},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("multiple services configured for the same resource", "resource_ambiguous")).Once()

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "conflict", resp.Error)
		assert.Contains(t, resp.Message, "protected resource URI already configured for another service")

		mockRepo.AssertNotCalled(t, "Create")
		mockRepo.AssertExpectations(t)
	})

	t.Run("protected_resource lookup storage error aborts create", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes:             []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{"https://api.example.com/"},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(nil, storage.NewStorageError("FindByProtectedResource", storage.ErrorKindConnection, nil, "database unavailable")).Once()

		req := httptest.NewRequest(http.MethodPost, "/api/third-party/oauth2/clients", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.CreateService(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "internal server error", resp.Error)

		mockRepo.AssertNotCalled(t, "Create")
		mockRepo.AssertExpectations(t)
	})
	t.Run("rejects insecure token endpoint before create", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "Provider",
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			IssuerURI:    "https://issuer.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		w := httptest.NewRecorder()

		handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))

		require.Equal(t, http.StatusBadRequest, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "validation failed", response.Error)
		assert.Equal(t, "token_endpoint must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)", response.Message)
		mockRepo.AssertNotCalled(t, "Create")
	})

	t.Run("rejects insecure fallback token endpoint before create", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "Provider",
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			IssuerURI:    "https://issuer.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: true},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := httptest.NewRecorder()

		handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)).WithContext(ctx))

		require.Equal(t, http.StatusBadRequest, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "validation failed", response.Error)
		assert.Equal(t, "token_endpoint must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)", response.Message)
		mockRepo.AssertNotCalled(t, "Create")
	})
}

func TestServicesHandler_CreateServiceTokenEndpointAuthMethodMapping(t *testing.T) {
	tests := []struct {
		name          string
		includeMethod bool
		method        any
		includeSecret bool
		secret        any
		wantPublic    bool
		wantError     string
	}{
		{
			name:          "omitted method creates a confidential service",
			includeSecret: true,
			secret:        "confidential-secret",
		},
		{
			name:          "null method creates a confidential service",
			includeMethod: true,
			method:        nil,
			includeSecret: true,
			secret:        "confidential-secret",
		},
		{
			name:      "omitted method without a secret is rejected",
			wantError: "client_secret is required",
		},
		{
			name:          "null method without a secret is rejected",
			includeMethod: true,
			method:        nil,
			wantError:     "client_secret is required",
		},
		{
			name:          "none method with an omitted secret creates a public service",
			includeMethod: true,
			method:        "none",
			wantPublic:    true,
		},
		{
			name:          "none method with an empty secret creates a public service",
			includeMethod: true,
			method:        "none",
			includeSecret: true,
			secret:        "",
			wantPublic:    true,
		},
		{
			name:          "none method with a null secret creates a public service",
			includeMethod: true,
			method:        "none",
			includeSecret: true,
			secret:        nil,
			wantPublic:    true,
		},
		{
			name:          "none method with a non-empty secret is rejected",
			includeMethod: true,
			method:        "none",
			includeSecret: true,
			secret:        "must-not-be-stored",
			wantError:     `client_secret must not have a non-empty value when token_endpoint_auth_method is "none"`,
		},
		{
			name:          "empty method is rejected",
			includeMethod: true,
			method:        "",
			includeSecret: true,
			secret:        "confidential-secret",
			wantError:     `token_endpoint_auth_method: only "none" and "private_key_jwt" are accepted`,
		},
		{
			name:          "unknown method is rejected",
			includeMethod: true,
			method:        "client_secret_post",
			includeSecret: true,
			secret:        "confidential-secret",
			wantError:     `token_endpoint_auth_method: only "none" and "private_key_jwt" are accepted`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockProviderRepository)
			handler := setupHandler(t, mockRepo)
			requestBody := map[string]any{
				"display_name": "Provider",
				"client_id":    "provider-client-id",
				"issuer_uri":   "https://provider.example.com",
				"discovery": map[string]any{
					"enable_discovery": false,
				},
				"endpoints": map[string]any{
					"token_endpoint":     "https://provider.example.com/token",
					"authorize_endpoint": "https://provider.example.com/authorize",
				},
			}
			if tt.includeMethod {
				requestBody["token_endpoint_auth_method"] = tt.method
			}
			if tt.includeSecret {
				requestBody["client_secret"] = tt.secret
			}

			body, err := json.Marshal(requestBody)
			require.NoError(t, err)

			if tt.wantError == "" {
				mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
					if tt.wantPublic {
						return entity.TokenEndpointAuthMethod == model.TokenEndpointAuthMethodNone && entity.Secret.IsAbsent()
					}
					return entity.TokenEndpointAuthMethod.IsAbsent() && !entity.Secret.IsAbsent()
				})).Return(nil).Once()
			}
			if tt.wantError != "" {
				mockRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
			}

			w := httptest.NewRecorder()
			handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))

			if tt.wantError != "" {
				require.Equal(t, http.StatusBadRequest, w.Code)
				var response ErrorResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
				assert.Equal(t, "validation failed", response.Error)
				mockRepo.AssertNotCalled(t, "Create")
				return
			}

			require.Equal(t, http.StatusCreated, w.Code)
			var response map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			method, methodPresent := response["token_endpoint_auth_method"]
			require.True(t, methodPresent)

			if tt.wantPublic {
				assert.JSONEq(t, `"none"`, string(method))
				assert.NotContains(t, response, "client_secret")
			} else {
				assert.JSONEq(t, "null", string(method))
				secret, secretPresent := response["client_secret"]
				require.True(t, secretPresent)
				assert.JSONEq(t, `"REDACTED"`, string(secret))
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestServicesHandler_RejectsInvalidClientAuthenticationBeforeDiscovery(t *testing.T) {
	var discoveryRequests atomic.Int32
	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		discoveryRequests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer discoveryServer.Close()

	tests := []struct {
		name        string
		requestBody map[string]any
		wantError   string
	}{
		{
			name: "contradictory public secret",
			requestBody: map[string]any{
				"display_name":               "Provider",
				"client_id":                  "provider-client-id",
				"client_secret":              "must-not-be-stored",
				"token_endpoint_auth_method": "none",
				"issuer_uri":                 discoveryServer.URL,
				"discovery":                  map[string]any{"enable_discovery": true},
			},
			wantError: `client_secret must not have a non-empty value when token_endpoint_auth_method is "none"`,
		},
		{
			name: "missing public client ID",
			requestBody: map[string]any{
				"display_name":               "Provider",
				"token_endpoint_auth_method": "none",
				"issuer_uri":                 discoveryServer.URL,
				"discovery":                  map[string]any{"enable_discovery": true},
			},
			wantError: "client_id is required",
		},
	}

	assertRejected := func(t *testing.T, request *http.Request, handler http.Handler, repo *MockProviderRepository, wantError string) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		require.Equal(t, http.StatusBadRequest, response.Code)
		var body ErrorResponse
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
		assert.Equal(t, "validation failed", body.Error)
		assert.Equal(t, wantError, body.Message)
		assert.Zero(t, discoveryRequests.Load())
		repo.AssertNotCalled(t, "Create")
		repo.AssertNotCalled(t, "Update")
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			requestBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			t.Run("create", func(t *testing.T) {
				discoveryRequests.Store(0)
				repo := new(MockProviderRepository)
				handler := setupHandler(t, repo)
				request := httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(requestBody))

				assertRejected(t, request, http.HandlerFunc(handler.CreateService), repo, tt.wantError)
			})

			t.Run("update", func(t *testing.T) {
				discoveryRequests.Store(0)
				repo := new(MockProviderRepository)
				handler := setupHandler(t, repo)
				serviceID := id.NewServiceID()
				request := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(requestBody))
				routeContext := chi.NewRouteContext()
				routeContext.URLParams.Add("service-id", serviceID.String())
				request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))

				assertRejected(t, request, http.HandlerFunc(handler.UpdateService), repo, tt.wantError)
			})
		})
	}
}

func TestServicesHandler_RejectsCredentialedDiscoveredPublicTokenEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		tokenEndpoint string
		wantError     string
	}{
		{
			name:          "userinfo",
			tokenEndpoint: "https://username:password@issuer.example.com/token",
			wantError:     "token_endpoint must not include userinfo for public clients",
		},
		{
			name:          "client secret query parameter",
			tokenEndpoint: "https://issuer.example.com/token?client_secret=secret",
			wantError:     "token_endpoint must not include client authentication parameter for public clients: client_secret",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var discoveryRequests atomic.Int32
			discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				discoveryRequests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{
					"token_endpoint":         tt.tokenEndpoint,
					"authorization_endpoint": "https://issuer.example.com/authorize",
				})
			}))
			defer discoveryServer.Close()

			requestBody, err := json.Marshal(map[string]any{
				"display_name":               "Provider",
				"client_id":                  "provider-client-id",
				"token_endpoint_auth_method": "none",
				"issuer_uri":                 discoveryServer.URL,
				"discovery":                  map[string]any{"enable_discovery": true},
			})
			require.NoError(t, err)

			repo := new(MockProviderRepository)
			handler := setupHandler(t, repo)
			response := httptest.NewRecorder()
			handler.CreateService(response, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(requestBody)))

			require.Equal(t, http.StatusBadRequest, response.Code)
			var body ErrorResponse
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			assert.Equal(t, tt.wantError, body.Message)
			assert.Equal(t, int32(1), discoveryRequests.Load())
			repo.AssertNotCalled(t, "Create")

			updateRepo := new(MockProviderRepository)
			updateHandler := setupHandler(t, updateRepo)
			serviceID := id.NewServiceID()
			updateRequest := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(requestBody))
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("service-id", serviceID.String())
			updateRequest = updateRequest.WithContext(context.WithValue(updateRequest.Context(), chi.RouteCtxKey, routeContext))
			updateResponse := httptest.NewRecorder()
			updateHandler.UpdateService(updateResponse, updateRequest)

			require.Equal(t, http.StatusBadRequest, updateResponse.Code)
			var updateBody ErrorResponse
			require.NoError(t, json.NewDecoder(updateResponse.Body).Decode(&updateBody))
			assert.Equal(t, tt.wantError, updateBody.Message)
			assert.Equal(t, int32(2), discoveryRequests.Load())
			updateRepo.AssertNotCalled(t, "Update")
		})
	}
}
func TestServicesHandler_CreateCIMDConfidentialService(t *testing.T) {
	mockRepo := new(MockProviderRepository)
	handler := setupHandler(t, mockRepo)

	mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
		return entity.TokenEndpointAuthMethod == model.TokenEndpointAuthMethod("private_key_jwt") &&
			entity.Secret.IsAbsent() &&
			entity.ClientID == id.ClientID("https://broker.example.com/.well-known/oauth-client/"+entity.ID.String())
	})).Return(nil).Once()

	body, err := json.Marshal(map[string]any{
		"display_name":               "CIMD Provider",
		"token_endpoint_auth_method": "private_key_jwt",
		"issuer_uri":                 "https://issuer.example.com",
		"discovery":                  map[string]any{"enable_discovery": false},
		"endpoints": map[string]any{
			"token_endpoint":     "https://issuer.example.com/token",
			"authorize_endpoint": "https://issuer.example.com/authorize",
		},
	})
	require.NoError(t, err)

	response := httptest.NewRecorder()
	handler.CreateService(response, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))

	require.Equal(t, http.StatusCreated, response.Code)
	var payload map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	assert.JSONEq(t, `"private_key_jwt"`, string(payload["token_endpoint_auth_method"]))
	assert.NotContains(t, payload, "client_secret")

	var serviceID, clientID string
	require.NoError(t, json.Unmarshal(payload["id"], &serviceID))
	require.NoError(t, json.Unmarshal(payload["client_id"], &clientID))
	assert.Equal(t, "https://broker.example.com/.well-known/oauth-client/"+serviceID, clientID)
	mockRepo.AssertExpectations(t)
}

func TestServicesHandler_RejectsCIMDCallerCredentialsBeforeDiscovery(t *testing.T) {
	var discoveryRequests atomic.Int32
	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		discoveryRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token_endpoint":         "https://issuer.example.com/token",
			"authorization_endpoint": "https://issuer.example.com/authorize",
		})
	}))
	defer discoveryServer.Close()

	tests := []struct {
		name         string
		requestField string
		requestValue string
	}{
		{name: "caller client ID", requestField: "client_id", requestValue: "operator-client-id"},
		{name: "caller shared secret", requestField: "client_secret", requestValue: "operator-secret"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"display_name":               "CIMD Provider",
				"token_endpoint_auth_method": "private_key_jwt",
				"issuer_uri":                 discoveryServer.URL,
				"discovery":                  map[string]any{"enable_discovery": true},
				tt.requestField:              tt.requestValue,
			})
			require.NoError(t, err)

			for _, operation := range []string{"create", "update"} {
				operation := operation
				t.Run(operation, func(t *testing.T) {
					discoveryRequests.Store(0)
					repo := new(MockProviderRepository)
					config := testConfig()
					config.Security.SkipThirdpartyHTTPSValidation = true
					handler := setupHandlerWithConfig(t, repo, config)
					request := httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body))
					if operation == "update" {
						serviceID := id.NewServiceID()
						request = httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
						routeContext := chi.NewRouteContext()
						routeContext.URLParams.Add("service-id", serviceID.String())
						request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
					}

					response := httptest.NewRecorder()
					if operation == "create" {
						handler.CreateService(response, request)
					} else {
						handler.UpdateService(response, request)
					}

					require.Equal(t, http.StatusBadRequest, response.Code)
					var errorResponse ErrorResponse
					require.NoError(t, json.NewDecoder(response.Body).Decode(&errorResponse))
					assert.Equal(t, "validation failed", errorResponse.Error)
					assert.Contains(t, errorResponse.Message, tt.requestField)
					assert.Contains(t, errorResponse.Message, "private_key_jwt")
					assert.Zero(t, discoveryRequests.Load())
					repo.AssertNotCalled(t, "Create")
					repo.AssertNotCalled(t, "Update")
				})
			}
		})
	}
}

func TestServicesHandler_RejectsCIMDForGoogleFlavor(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"display_name":               "Google",
		"token_endpoint_auth_method": "private_key_jwt",
		"oauth2_flavor":              "google",
		"issuer_uri":                 "https://accounts.google.com",
		"discovery":                  map[string]any{"enable_discovery": false},
		"endpoints": map[string]any{
			"token_endpoint":     "https://oauth2.googleapis.com/token",
			"authorize_endpoint": "https://accounts.google.com/o/oauth2/v2/auth",
		},
	})
	require.NoError(t, err)

	for _, operation := range []string{"create", "update"} {
		operation := operation
		t.Run(operation, func(t *testing.T) {
			repo := new(MockProviderRepository)
			handler := setupHandler(t, repo)
			request := httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body))
			if operation == "update" {
				serviceID := id.NewServiceID()
				request = httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
				routeContext := chi.NewRouteContext()
				routeContext.URLParams.Add("service-id", serviceID.String())
				request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
			}

			response := httptest.NewRecorder()
			if operation == "create" {
				handler.CreateService(response, request)
			} else {
				handler.UpdateService(response, request)
			}

			require.Equal(t, http.StatusBadRequest, response.Code)
			var errorResponse ErrorResponse
			require.NoError(t, json.NewDecoder(response.Body).Decode(&errorResponse))
			assert.Equal(t, "validation failed", errorResponse.Error)
			assert.Contains(t, errorResponse.Message, "google")
			assert.Contains(t, errorResponse.Message, "private_key_jwt")
			repo.AssertNotCalled(t, "Create")
			repo.AssertNotCalled(t, "Update")
		})
	}
}

func TestServicesHandler_GetService(t *testing.T) {
	t.Run("successful get", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		entity := encryptedEntity(serviceID, "GitHub", "github-client-id", "secret", "https://github.com",
			[]model.OAuthScope{{ScopeValue: "repo", Description: "Repository access"}})
		entity.Version = 12
		mockRepo.On("Get", mock.Anything, serviceID).Return(entity, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/third-party/oauth2/clients/"+serviceID.String(), nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetService(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, `"12"`, w.Header().Get("ETag"))

		var resp ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, serviceID.String(), resp.ID)
		assert.Equal(t, "GitHub", resp.DisplayName)
		require.NotNil(t, resp.ClientSecret)
		assert.Equal(t, "REDACTED", *resp.ClientSecret) // Secret must be redacted
	})

	t.Run("service not found", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		notFoundID := id.NewServiceID()
		mockRepo.On("Get", mock.Anything, notFoundID).Return(nil,
			storage.NewStorageError("GetService", storage.ErrorKindNotFound, nil, "service not found"))

		req := httptest.NewRequest(http.MethodGet, "/api/third-party/oauth2/clients/"+notFoundID.String(), nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", notFoundID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetService(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "service not found", resp.Error)
	})
}

func TestServicesHandler_GetServiceAuthenticationPostures(t *testing.T) {
	now := time.Now().UTC()
	staticID := id.NewServiceID()
	publicID := id.NewServiceID()
	cimdID := id.NewServiceID()
	staticService := encryptedEntity(staticID, "Static provider", "static-client-id", "static-secret", "https://static.example.com", nil)
	publicService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      publicID,
		DisplayName:             "Public provider",
		ClientID:                "public-client-id",
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
		IssuerURI:               "https://public.example.com",
		Endpoints:               model.OAuth2Endpoints{TokenEndpoint: "https://public.example.com/token", AuthorizeEndpoint: "https://public.example.com/authorize"},
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	cimdClientID := "https://broker.example.com/.well-known/oauth-client/" + cimdID.String()
	cimdService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      cimdID,
		DisplayName:             "CIMD provider",
		ClientID:                id.ClientID(cimdClientID),
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethod("private_key_jwt"),
		IssuerURI:               "https://issuer.example.com",
		Endpoints:               model.OAuth2Endpoints{TokenEndpoint: "https://issuer.example.com/token", AuthorizeEndpoint: "https://issuer.example.com/authorize"},
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	tests := []struct {
		name             string
		service          *model.ThirdpartyOAuth2ProviderEntity
		wantMethod       string
		wantClientID     string
		wantClientSecret bool
	}{
		{name: "static confidential", service: staticService, wantMethod: "null", wantClientID: "static-client-id", wantClientSecret: true},
		{name: "public", service: publicService, wantMethod: `"none"`, wantClientID: "public-client-id"},
		{name: "CIMD confidential", service: cimdService, wantMethod: `"private_key_jwt"`, wantClientID: cimdClientID},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			handler := setupHandler(t, repo)
			repo.On("Get", mock.Anything, tt.service.ID).Return(tt.service, nil).Once()

			request := httptest.NewRequest(http.MethodGet, "/api/services/"+tt.service.ID.String(), nil)
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("service-id", tt.service.ID.String())
			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
			response := httptest.NewRecorder()

			handler.GetService(response, request)

			require.Equal(t, http.StatusOK, response.Code)
			var payload map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
			assert.JSONEq(t, tt.wantMethod, string(payload["token_endpoint_auth_method"]))
			var clientID string
			require.NoError(t, json.Unmarshal(payload["client_id"], &clientID))
			assert.Equal(t, tt.wantClientID, clientID)
			if tt.wantClientSecret {
				assert.JSONEq(t, `"REDACTED"`, string(payload["client_secret"]))
			} else {
				assert.NotContains(t, payload, "client_secret")
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestServicesHandler_GetServiceETag(t *testing.T) {
	mockRepo := new(MockProviderRepository)
	handler := setupHandler(t, mockRepo)
	serviceID := id.NewServiceID()
	entity := encryptedEntity(serviceID, "GitHub", "github-client-id", "secret", "https://github.com", nil)
	entity.Version = 5
	mockRepo.On("Get", mock.Anything, serviceID).Return(entity, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/services/"+serviceID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("service-id", serviceID.String())
	w := httptest.NewRecorder()
	handler.GetService(w, req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `"5"`, w.Header().Get("ETag"))
	mockRepo.AssertExpectations(t)
}

func TestServicesHandler_UpdateService(t *testing.T) {
	t.Run("successful update with new secret", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "GitHub Updated",
			ClientID:     "github-client-id-new",
			ClientSecret: "new-secret",
			IssuerURI:    "https://github.com",
			Discovery: DiscoveryConfigRequest{
				EnableDiscovery: false,
			},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://github.com/login/oauth/access_token",
				AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			},
			Scopes: []OAuthScopeRequest{
				{ScopeValue: "repo", Description: "Repository access"},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
			return e.ID == serviceID && e.DisplayName == "GitHub Updated" && e.Secret.IsEncrypted()
		}), (*int64)(nil)).Return(nil)

		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+serviceID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "GitHub Updated", resp.DisplayName)
		require.NotNil(t, resp.ClientSecret)
		assert.Equal(t, "REDACTED", *resp.ClientSecret)

		mockRepo.AssertExpectations(t)
	})

	t.Run("updates a service with an empty scope list", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		serviceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "Scope-less IdP",
			ClientID:     "scope-less-client",
			ClientSecret: "scope-less-secret",
			IssuerURI:    "https://idp.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints:    &OAuth2EndpointsRequest{TokenEndpoint: "https://idp.example.com/token", AuthorizeEndpoint: "https://idp.example.com/authorize"},
			Scopes:       []OAuthScopeRequest{},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
			return entity.ID == serviceID && len(entity.Scopes) == 0
		}), (*int64)(nil)).Return(nil)
		req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()

		handler.UpdateService(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		mockRepo.AssertExpectations(t)
	})

	t.Run("invalid authorization_params does not update service", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		serviceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:         "GitHub Updated",
			ClientID:            "github-client-id",
			ClientSecret:        "new-secret",
			IssuerURI:           "https://github.com",
			Discovery:           DiscoveryConfigRequest{},
			Endpoints:           &OAuth2EndpointsRequest{TokenEndpoint: "https://github.com/token", AuthorizeEndpoint: "https://github.com/authorize"},
			Scopes:              []OAuthScopeRequest{{ScopeValue: "repo", Description: "Repository access"}},
			AuthorizationParams: map[string]string{"state": "unsafe"},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockRepo.AssertNotCalled(t, "Update")
	})

	t.Run("missing client_secret returns 400", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "GitHub Updated",
			ClientID:     "github-client-id",
			ClientSecret: "", // Missing — must be rejected per OpenAPI contract
			IssuerURI:    "https://github.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Scopes:       []OAuthScopeRequest{{ScopeValue: "repo", Description: "Repository access"}},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+serviceID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "validation failed", resp.Error)
		assert.Contains(t, resp.Message, "client_secret is required")

		// No repo calls must have been made
		mockRepo.AssertNotCalled(t, "Get")
		mockRepo.AssertNotCalled(t, "Update")
	})

	t.Run("service not found returns 404", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		nonexistentID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "GitHub Updated",
			ClientID:     "github-client-id",
			ClientSecret: "new-secret",
			IssuerURI:    "https://github.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://github.com/login/oauth/access_token",
				AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			},
			Scopes: []OAuthScopeRequest{
				{ScopeValue: "repo", Description: "Repository access"},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
			return e.ID == nonexistentID && e.Secret.IsEncrypted()
		}), (*int64)(nil)).Return(storage.NewStorageError("UpdateService", storage.ErrorKindNotFound, nil, "service not found"))

		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+nonexistentID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", nonexistentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "service not found", resp.Error)

		mockRepo.AssertExpectations(t)
	})

	t.Run("validation error after valid client_secret - no Update called", func(t *testing.T) {
		// Regression guard for KMS ordering: ValidateForUpdate must fire before
		// providerService.Update (which triggers encryption). If Update is called
		// despite a validation failure, the ordering is broken.
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		reqBody := ServiceRequest{
			DisplayName:  "", // Invalid: triggers ValidateForUpdate error
			ClientID:     "github-client-id",
			ClientSecret: "valid-secret", // Valid: passes the early client_secret guard
			IssuerURI:    "https://github.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://github.com/login/oauth/access_token",
				AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			},
			Scopes: []OAuthScopeRequest{
				{ScopeValue: "repo", Description: "Repository access"},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		serviceID := id.NewServiceID()
		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+serviceID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "validation failed", resp.Error)
		assert.Contains(t, resp.Message, "display_name is required")

		// Validation must fire before any storage or encryption call.
		mockRepo.AssertNotCalled(t, "Update")
	})

	t.Run("protected_resources are normalized before update", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes: []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{
				"https://api.example.com/",
				"https://api.example.com/v2",
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("no service configured for the requested resource", "resource_not_found")).Once()
		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com/v2").
			Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("no service configured for the requested resource", "resource_not_found")).Once()

		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(e *model.ThirdpartyOAuth2ProviderEntity) bool {
			return e.ID == serviceID &&
				len(e.ProtectedResources) == 2 &&
				e.ProtectedResources[0] == "https://api.example.com" &&
				e.ProtectedResources[1] == "https://api.example.com/v2" &&
				e.Secret.IsEncrypted()
		}), mock.MatchedBy(func(version *int64) bool { return version != nil && *version == 1 })).Run(func(args mock.Arguments) { args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity).Version = 2 }).Return(nil)

		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+serviceID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", `"1"`)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"https://api.example.com",
			"https://api.example.com/v2",
		}, resp.ProtectedResources)

		mockRepo.AssertExpectations(t)
	})

	t.Run("normalized protected_resource conflict returns 409 on update", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		conflictingServiceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes:             []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{"https://api.example.com/"},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(&model.ThirdpartyOAuth2ProviderEntity{ID: conflictingServiceID}, nil).Once()

		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+serviceID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", `"1"`)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "conflict", resp.Error)
		assert.Contains(t, resp.Message, "protected resource URI already configured for another service")

		mockRepo.AssertNotCalled(t, "Update")
		mockRepo.AssertExpectations(t)
	})

	t.Run("protected_resource lookup storage error aborts update", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		reqBody := ServiceRequest{
			DisplayName:  "API Service",
			ClientID:     "api-client-id",
			ClientSecret: "api-client-secret",
			IssuerURI:    "https://api.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://api.example.com/token",
				AuthorizeEndpoint: "https://api.example.com/authorize",
			},
			Scopes:             []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
			ProtectedResources: []string{"https://api.example.com/"},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com").
			Return(nil, storage.NewStorageError("FindByProtectedResource", storage.ErrorKindConnection, nil, "database unavailable")).Once()

		req := httptest.NewRequest(http.MethodPut, "/api/third-party/oauth2/clients/"+serviceID.String(), bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", `"1"`)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.UpdateService(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "internal server error", resp.Error)

		mockRepo.AssertNotCalled(t, "Update")
		mockRepo.AssertExpectations(t)
	})
	t.Run("rejects insecure authorization endpoint before update", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		serviceID := id.NewServiceID()

		reqBody := ServiceRequest{
			DisplayName:  "Provider",
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			IssuerURI:    "https://issuer.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: false},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://issuer.example.com/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()

		handler.UpdateService(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "validation failed", response.Error)
		assert.Equal(t, "authorize_endpoint must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)", response.Message)
		mockRepo.AssertNotCalled(t, "Update")
	})

	t.Run("rejects insecure fallback authorization endpoint before update", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		serviceID := id.NewServiceID()

		reqBody := ServiceRequest{
			DisplayName:  "Provider",
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			IssuerURI:    "https://issuer.example.com",
			Discovery:    DiscoveryConfigRequest{EnableDiscovery: true},
			Endpoints: &OAuth2EndpointsRequest{
				TokenEndpoint:     "https://issuer.example.com/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
		}
		body, err := json.Marshal(reqBody)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body)).WithContext(ctx)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()

		handler.UpdateService(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "validation failed", response.Error)
		assert.Equal(t, "authorize_endpoint must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)", response.Message)
		mockRepo.AssertNotCalled(t, "Update")
	})
}

func TestServicesHandler_UpdateCIMDConfidentialService(t *testing.T) {
	repo := new(MockProviderRepository)
	handler := setupHandler(t, repo)
	serviceID := id.NewServiceID()
	clientID := "https://broker.example.com/.well-known/oauth-client/" + serviceID.String()
	persisted := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      serviceID,
		ClientID:                id.ClientID(clientID),
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethodPrivateKeyJWT,
	}
	repo.On("Get", mock.Anything, serviceID).Return(persisted, nil).Once()

	repo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
		return entity.ID == serviceID &&
			entity.TokenEndpointAuthMethod == model.TokenEndpointAuthMethod("private_key_jwt") &&
			entity.ClientID == id.ClientID(clientID) &&
			entity.Secret.IsAbsent()
	}), (*int64)(nil)).Return(nil).Once()

	body, err := json.Marshal(map[string]any{
		"display_name":               "CIMD Provider",
		"token_endpoint_auth_method": "private_key_jwt",
		"issuer_uri":                 "https://issuer.example.com",
		"discovery":                  map[string]any{"enable_discovery": false},
		"endpoints": map[string]any{
			"token_endpoint":     "https://issuer.example.com/token",
			"authorize_endpoint": "https://issuer.example.com/authorize",
		},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("service-id", serviceID.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()

	handler.UpdateService(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var payload map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	assert.JSONEq(t, `"private_key_jwt"`, string(payload["token_endpoint_auth_method"]))
	assert.JSONEq(t, `"`+clientID+`"`, string(payload["client_id"]))
	assert.NotContains(t, payload, "client_secret")
	repo.AssertExpectations(t)
}

func TestServicesHandler_UpdateServiceTokenEndpointAuthMethodMapping(t *testing.T) {
	tests := []struct {
		name          string
		includeMethod bool
		method        any
		includeSecret bool
		secret        any
		wantPublic    bool
		wantError     string
	}{
		{
			name:          "omitted method with a secret keeps the service confidential",
			includeSecret: true,
			secret:        "confidential-secret",
		},
		{
			name:          "null method from a read response keeps the service confidential",
			includeMethod: true,
			method:        nil,
			includeSecret: true,
			secret:        "confidential-secret",
		},
		{
			name:      "public service update omitting method and secret is rejected",
			wantError: "client_secret is required",
		},
		{
			name:          "null method without a secret is rejected",
			includeMethod: true,
			method:        nil,
			wantError:     "client_secret is required",
		},
		{
			name:          "none method with an omitted secret makes the service public",
			includeMethod: true,
			method:        "none",
			wantPublic:    true,
		},
		{
			name:          "none method with an empty secret makes the service public",
			includeMethod: true,
			method:        "none",
			includeSecret: true,
			secret:        "",
			wantPublic:    true,
		},
		{
			name:          "none method with a null secret makes the service public",
			includeMethod: true,
			method:        "none",
			includeSecret: true,
			secret:        nil,
			wantPublic:    true,
		},
		{
			name:          "empty method is rejected",
			includeMethod: true,
			method:        "",
			includeSecret: true,
			secret:        "confidential-secret",
			wantError:     `token_endpoint_auth_method: only "none" and "private_key_jwt" are accepted`,
		},
		{
			name:          "none method with a non-empty secret is rejected",
			includeMethod: true,
			method:        "none",
			includeSecret: true,
			secret:        "must-not-be-stored",
			wantError:     `client_secret must not have a non-empty value when token_endpoint_auth_method is "none"`,
		},
		{
			name:          "unknown method is rejected",
			includeMethod: true,
			method:        "client_secret_post",
			includeSecret: true,
			secret:        "confidential-secret",
			wantError:     `token_endpoint_auth_method: only "none" and "private_key_jwt" are accepted`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockProviderRepository)
			handler := setupHandler(t, mockRepo)
			serviceID := id.NewServiceID()
			requestBody := map[string]any{
				"display_name": "Provider Updated",
				"client_id":    "provider-client-id",
				"issuer_uri":   "https://provider.example.com",
				"discovery": map[string]any{
					"enable_discovery": false,
				},
				"endpoints": map[string]any{
					"token_endpoint":     "https://provider.example.com/token",
					"authorize_endpoint": "https://provider.example.com/authorize",
				},
			}
			if tt.includeMethod {
				requestBody["token_endpoint_auth_method"] = tt.method
			}
			if tt.includeSecret {
				requestBody["client_secret"] = tt.secret
			}

			body, err := json.Marshal(requestBody)
			require.NoError(t, err)

			if tt.wantError == "" {
				mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
					if tt.wantPublic {
						return entity.ID == serviceID &&
							entity.TokenEndpointAuthMethod == model.TokenEndpointAuthMethodNone &&
							entity.Secret.IsAbsent()
					}
					return entity.ID == serviceID &&
						entity.TokenEndpointAuthMethod.IsAbsent() &&
						entity.Secret.IsEncrypted()
				}), (*int64)(nil)).Return(nil).Once()
			}
			if tt.wantError != "" {
				mockRepo.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}

			req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("service-id", serviceID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			w := httptest.NewRecorder()

			handler.UpdateService(w, req)

			if tt.wantError != "" {
				require.Equal(t, http.StatusBadRequest, w.Code)
				var response ErrorResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
				assert.Equal(t, "validation failed", response.Error)
				mockRepo.AssertNotCalled(t, "Update")
				return
			}

			require.Equal(t, http.StatusOK, w.Code)
			var response map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			method, methodPresent := response["token_endpoint_auth_method"]
			require.True(t, methodPresent)

			if tt.wantPublic {
				assert.JSONEq(t, `"none"`, string(method))
				assert.NotContains(t, response, "client_secret")
			} else {
				assert.JSONEq(t, "null", string(method))
				secret, secretPresent := response["client_secret"]
				require.True(t, secretPresent)
				assert.JSONEq(t, `"REDACTED"`, string(secret))
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestServicesHandler_DeleteService(t *testing.T) {
	t.Run("successful deletion", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		mockRepo.On("Delete", mock.Anything, serviceID).Return(nil)

		req := httptest.NewRequest(http.MethodDelete, "/api/third-party/oauth2/clients/"+serviceID.String(), nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.DeleteService(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		mockRepo.AssertExpectations(t)
	})

	t.Run("deletion blocked by grants", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID := id.NewServiceID()
		mockRepo.On("Delete", mock.Anything, serviceID).Return(
			storage.NewStorageError("DeleteService", storage.ErrorKindConflict, nil, "cannot delete service: 5 grants reference it"))

		req := httptest.NewRequest(http.MethodDelete, "/api/third-party/oauth2/clients/"+serviceID.String(), nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.DeleteService(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "conflict", resp.Error)
		assert.Contains(t, resp.Message, "5 grants reference it")

		mockRepo.AssertExpectations(t)
	})

	t.Run("service not found", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		notFoundID := id.NewServiceID()
		mockRepo.On("Delete", mock.Anything, notFoundID).Return(
			storage.NewStorageError("DeleteService", storage.ErrorKindNotFound, nil, "service not found"))

		req := httptest.NewRequest(http.MethodDelete, "/api/third-party/oauth2/clients/"+notFoundID.String(), nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", notFoundID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.DeleteService(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		mockRepo.AssertExpectations(t)
	})
}

func TestServicesHandler_ListServices(t *testing.T) {
	t.Run("successful list", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		serviceID1 := id.NewServiceID()
		serviceID2 := id.NewServiceID()
		entities := []*model.ThirdpartyOAuth2ProviderEntity{
			encryptedEntity(serviceID1, "GitHub", "github-client", "secret1", "https://github.com",
				[]model.OAuthScope{{ScopeValue: "repo", Description: "Repository access"}}),
			encryptedEntity(serviceID2, "Google", "google-client", "secret2", "https://accounts.google.com",
				[]model.OAuthScope{{ScopeValue: "email", Description: "Email access"}}),
		}

		mockRepo.On("List", mock.Anything).Return(entities, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/third-party/oauth2/clients", nil)
		w := httptest.NewRecorder()

		handler.ListServices(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp []ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Len(t, resp, 2)
		assert.Equal(t, "GitHub", resp[0].DisplayName)
		assert.Equal(t, "Google", resp[1].DisplayName)
		require.NotNil(t, resp[0].ClientSecret)
		require.NotNil(t, resp[1].ClientSecret)
		assert.Equal(t, "REDACTED", *resp[0].ClientSecret)
		assert.Equal(t, "REDACTED", *resp[1].ClientSecret)

		mockRepo.AssertExpectations(t)
	})

	t.Run("empty list", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		mockRepo.On("List", mock.Anything).Return([]*model.ThirdpartyOAuth2ProviderEntity{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/third-party/oauth2/clients", nil)
		w := httptest.NewRecorder()

		handler.ListServices(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp []ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Len(t, resp, 0)

		mockRepo.AssertExpectations(t)
	})
}

func TestServicesHandler_SecretRedaction(t *testing.T) {
	t.Run("secret redacted in all responses", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)

		svcID := id.NewServiceID()
		entity := encryptedEntity(svcID, "Test Service", id.ClientID("test-client"), "super-secret-value", "https://example.com",
			[]model.OAuthScope{{ScopeValue: "read", Description: "Read access"}})

		// Test Get endpoint
		mockRepo.On("Get", mock.Anything, svcID).Return(entity, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/services/"+svcID.String(), nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", svcID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetService(w, req)

		var resp ServiceResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)

		// Verify secret is redacted, not the actual value
		require.NotNil(t, resp.ClientSecret)
		assert.Equal(t, "REDACTED", *resp.ClientSecret)
		assert.NotEqual(t, "super-secret-value", *resp.ClientSecret)
	})
}

func TestServicesHandler_UpdateServiceProtectedResourcesETag(t *testing.T) {
	serviceID := id.NewServiceID()
	request := ServiceRequest{
		DisplayName: "API Service", ClientID: "api-client-id", ClientSecret: "api-client-secret", IssuerURI: "https://api.example.com",
		Discovery:          modelDiscoveryDisabled(),
		Endpoints:          &OAuth2EndpointsRequest{TokenEndpoint: "https://api.example.com/token", AuthorizeEndpoint: "https://api.example.com/authorize"},
		Scopes:             []OAuthScopeRequest{{ScopeValue: "read", Description: "Read access"}},
		ProtectedResources: []string{"https://api.example.com/resource/"},
	}
	body, err := json.Marshal(request)
	require.NoError(t, err)

	newRequest := func(body []byte, ifMatch string) (*http.Request, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("service-id", serviceID.String())
		return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)), httptest.NewRecorder()
	}

	t.Run("requires If-Match when replacing resources", func(t *testing.T) {
		handler := setupHandler(t, new(MockProviderRepository))
		req, recorder := newRequest(body, "")
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusPreconditionRequired, recorder.Code)
	})

	t.Run("uses strong ETag and emits replacement version", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com/resource").Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("not found", "resource_not_found"))
		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
			return entity.ProtectedResources[0] == "https://api.example.com/resource" && entity.Secret.IsEncrypted()
		}), mock.MatchedBy(func(version *int64) bool { return version != nil && *version == 7 })).Run(func(args mock.Arguments) { args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity).Version = 8 }).Return(nil)
		req, recorder := newRequest(body, `"7"`)
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, `"8"`, recorder.Header().Get("ETag"))
		var response ServiceResponse
		require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
		require.NotNil(t, response.ClientSecret)
		assert.Equal(t, "REDACTED", *response.ClientSecret)
		mockRepo.AssertExpectations(t)
	})

	t.Run("maps a stale resource replacement to 412", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("FindByProtectedResource", mock.Anything, "https://api.example.com/resource").Return(nil, tokenexchange.NewInvalidTargetErrorWithDetails("not found", "resource_not_found"))
		mockRepo.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(storage.NewStorageError("Update", storage.ErrorKindConflict, nil, "provider version is stale"))
		req, recorder := newRequest(body, `"7"`)
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusPreconditionFailed, recorder.Code)
		mockRepo.AssertExpectations(t)
	})

	t.Run("omitted resources preserve without If-Match", func(t *testing.T) {
		omittedRequest := request
		omittedRequest.ProtectedResources = nil
		omittedBody, err := json.Marshal(omittedRequest)
		require.NoError(t, err)
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool { return entity.ProtectedResources == nil }), (*int64)(nil)).Run(func(args mock.Arguments) { args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity).Version = 9 }).Return(nil)
		req, recorder := newRequest(omittedBody, "")
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, `"9"`, recorder.Header().Get("ETag"))
		mockRepo.AssertExpectations(t)
	})

	t.Run("null resources preserve without If-Match", func(t *testing.T) {
		nullBody := bytes.Replace(body, []byte(`"protected_resources":["https://api.example.com/resource/"]`), []byte(`"protected_resources":null`), 1)
		require.NotEqual(t, body, nullBody)
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool { return entity.ProtectedResources == nil }), (*int64)(nil)).Run(func(args mock.Arguments) { args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity).Version = 10 }).Return(nil)
		req, recorder := newRequest(nullBody, "")
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, `"10"`, recorder.Header().Get("ETag"))
		mockRepo.AssertExpectations(t)
	})

	t.Run("empty resource list requires If-Match", func(t *testing.T) {
		emptyBody := bytes.Replace(body, []byte(`"protected_resources":["https://api.example.com/resource/"]`), []byte(`"protected_resources":[]`), 1)
		require.NotEqual(t, body, emptyBody)
		handler := setupHandler(t, new(MockProviderRepository))
		req, recorder := newRequest(emptyBody, "")
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusPreconditionRequired, recorder.Code)
	})
	t.Run("empty resource list replaces with If-Match", func(t *testing.T) {
		emptyBody := bytes.Replace(body, []byte(`"protected_resources":["https://api.example.com/resource/"]`), []byte(`"protected_resources":[]`), 1)
		require.NotEqual(t, body, emptyBody)
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
			return entity.ProtectedResources != nil && len(entity.ProtectedResources) == 0
		}), mock.MatchedBy(func(version *int64) bool { return version != nil && *version == 7 })).Run(func(args mock.Arguments) { args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity).Version = 11 }).Return(nil)
		req, recorder := newRequest(emptyBody, `"7"`)
		handler.UpdateService(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, `"11"`, recorder.Header().Get("ETag"))
		mockRepo.AssertExpectations(t)
	})
}

func modelDiscoveryDisabled() DiscoveryConfigRequest {
	return DiscoveryConfigRequest{EnableDiscovery: false}
}

func TestServicesHandler_GetServiceByCanonicalID(t *testing.T) {
	mockRepo := new(MockProviderRepository)
	handler := setupHandler(t, mockRepo)
	serviceID := id.NewServiceID()
	canonicalID := "canonical-service"
	entity := encryptedEntity(serviceID, "Canonical Service", "canonical-service-client", "secret", "https://service.example.com", nil)
	entity.CanonicalID = &canonicalID
	mockRepo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(entity, nil)
	mockRepo.On("Get", mock.Anything, serviceID).Return(entity, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/services/"+canonicalID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("service-id", canonicalID)
	w := httptest.NewRecorder()
	handler.GetService(w, req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))
	require.Equal(t, http.StatusOK, w.Code)
	var response ServiceResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, serviceID.String(), response.ID)
	require.NotNil(t, response.CanonicalID)
	assert.Equal(t, canonicalID, *response.CanonicalID)
	mockRepo.AssertExpectations(t)
}

func TestServicesHandler_CanonicalCreateValidationAndConflict(t *testing.T) {
	canonicalID := "canonical-service"
	request := ServiceRequest{CanonicalID: &canonicalID, DisplayName: "Canonical Service", ClientID: "canonical-service-client", ClientSecret: "secret", IssuerURI: "https://service.example.com", Discovery: DiscoveryConfigRequest{}, Endpoints: &OAuth2EndpointsRequest{TokenEndpoint: "https://service.example.com/token", AuthorizeEndpoint: "https://service.example.com/authorize"}, Scopes: []OAuthScopeRequest{{ScopeValue: "read", Description: "Read"}}}
	t.Run("creates a service with canonical metadata", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("Create", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
			return entity.CanonicalID != nil && *entity.CanonicalID == canonicalID
		})).Return(nil)
		body, err := json.Marshal(request)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))
		require.Equal(t, http.StatusCreated, w.Code)
		var response ServiceResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		require.NotNil(t, response.CanonicalID)
		assert.Equal(t, canonicalID, *response.CanonicalID)
		mockRepo.AssertExpectations(t)
	})

	t.Run("maps a canonical ID conflict to 409", func(t *testing.T) {
		mockRepo := new(MockProviderRepository)
		handler := setupHandler(t, mockRepo)
		mockRepo.On("Create", mock.Anything, mock.Anything).Return(storage.NewStorageError("Create", storage.ErrorKindConflict, nil, "canonical_id already exists"))
		body, err := json.Marshal(request)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		handler.CreateService(w, httptest.NewRequest(http.MethodPost, "/api/services", bytes.NewReader(body)))
		assert.Equal(t, http.StatusConflict, w.Code)
		mockRepo.AssertExpectations(t)
	})
}

func TestServicesHandler_CanonicalListRepresentation(t *testing.T) {
	repo := new(MockProviderRepository)
	handler := setupHandler(t, repo)
	serviceID := id.NewServiceID()
	canonicalID := "listed-service"
	entity := encryptedEntity(serviceID, "Listed Service", "listed-service-client", "secret", "https://service.example.com", nil)
	entity.CanonicalID = &canonicalID
	repo.On("List", mock.Anything).Return([]*model.ThirdpartyOAuth2ProviderEntity{entity}, nil)
	w := httptest.NewRecorder()
	handler.ListServices(w, httptest.NewRequest(http.MethodGet, "/api/services", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var response []ServiceResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	require.Len(t, response, 1)
	assert.Equal(t, serviceID.String(), response[0].ID)
	require.NotNil(t, response[0].CanonicalID)
	assert.Equal(t, canonicalID, *response[0].CanonicalID)
	assert.Contains(t, w.Header().Get("Vary"), "Prefer")
	repo.AssertExpectations(t)
}

func TestServicesHandler_UpdateReturnsCanonicalIDFromRepository(t *testing.T) {
	repo := new(MockProviderRepository)
	handler := setupHandler(t, repo)
	serviceID := id.NewServiceID()
	canonicalID := "preserved-service"
	request := ServiceRequest{DisplayName: "Updated Service", ClientID: "preserved-service-client", ClientSecret: "secret", IssuerURI: "https://service.example.com", Discovery: DiscoveryConfigRequest{}, Endpoints: &OAuth2EndpointsRequest{TokenEndpoint: "https://service.example.com/token", AuthorizeEndpoint: "https://service.example.com/authorize"}, Scopes: []OAuthScopeRequest{{ScopeValue: "read", Description: "Read"}}}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
		return entity.ID == serviceID && entity.CanonicalID == nil && !entity.ClearCanonicalID
	}), (*int64)(nil)).Run(func(args mock.Arguments) {
		entity := args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity)
		entity.CanonicalID = &canonicalID
		entity.Version = 2
	}).Return(nil)
	req := httptest.NewRequest(http.MethodPut, "/api/services/"+serviceID.String(), bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("service-id", serviceID.String())
	w := httptest.NewRecorder()
	handler.UpdateService(w, req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))
	require.Equal(t, http.StatusOK, w.Code)
	var response ServiceResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	require.NotNil(t, response.CanonicalID)
	assert.Equal(t, canonicalID, *response.CanonicalID)
	repo.AssertExpectations(t)
}

const (
	adminTestResource = "https://mcp.example.test/mcp"
	adminTestIssuer   = "https://auth.example.test/tenant"
)

type adminDiscoveryClient struct {
	t          *testing.T
	documents  map[string]string
	probeCalls []string
	getCalls   []string
	postCalls  []string
}

func (c *adminDiscoveryClient) Probe(_ context.Context, rawURL string) (int, []string, error) {
	c.probeCalls = append(c.probeCalls, rawURL)
	if rawURL != adminTestResource {
		c.t.Errorf("unexpected protected resource probe: %s", rawURL)
		return 0, nil, ports.ErrOAuthDiscoveryUnsafeDestination
	}
	return http.StatusUnauthorized, []string{`Bearer resource_metadata="https://mcp.example.test/.well-known/oauth-protected-resource/mcp"`}, nil
}

func (c *adminDiscoveryClient) GetJSON(_ context.Context, rawURL string) ([]byte, error) {
	c.getCalls = append(c.getCalls, rawURL)
	document, ok := c.documents[rawURL]
	if !ok {
		c.t.Errorf("unexpected discovery metadata request: %s", rawURL)
		return nil, ports.ErrOAuthDiscoveryNotFound
	}
	return []byte(document), nil
}

func (c *adminDiscoveryClient) PostJSON(_ context.Context, rawURL string, _ []byte) ([]byte, error) {
	c.postCalls = append(c.postCalls, rawURL)
	return nil, ports.ErrOAuthDiscoveryRejected
}

func adminCIMDDiscoveryClient(t *testing.T) *adminDiscoveryClient {
	return &adminDiscoveryClient{t: t, documents: map[string]string{
		"https://mcp.example.test/.well-known/oauth-protected-resource/mcp":       `{"resource":"https://mcp.example.test/mcp","authorization_servers":["https://auth.example.test/tenant"]}`,
		"https://auth.example.test/.well-known/oauth-authorization-server/tenant": `{"issuer":"https://auth.example.test/tenant","authorization_endpoint":"https://auth.example.test/tenant/authorize","token_endpoint":"https://auth.example.test/tenant/token","client_id_metadata_document_supported":true,"token_endpoint_auth_methods_supported":["private_key_jwt","client_secret_basic"],"token_endpoint_auth_signing_alg_values_supported":["ES256"],"registration_endpoint":"https://auth.example.test/tenant/register"}`,
	}}
}

func adminDiscoveryRequest(t *testing.T, method, serviceID string, body any) *http.Request {
	t.Helper()
	path := "/api/services"
	if serviceID != "" {
		path += "/" + serviceID
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Remote-User", "admin@example.test")
	if serviceID != "" {
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("service-id", serviceID)
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	}
	return request
}

func adminDiscoveryPayload(t *testing.T, body []byte, serviceID, clientMethod, tokenMethod, clientID, resource string) {
	t.Helper()
	var response struct {
		ID                      string `json:"id"`
		ClientID                string `json:"client_id"`
		IssuerURI               string `json:"issuer_uri"`
		TokenEndpointAuthMethod string `json:"token_endpoint_auth_method"`
		Discovery               struct {
			EnableDiscovery bool   `json:"enable_discovery"`
			ResourceURL     string `json:"resource_url"`
			ClientMethod    string `json:"client_method"`
		} `json:"discovery"`
		Endpoints struct {
			TokenEndpoint     string `json:"token_endpoint"`
			AuthorizeEndpoint string `json:"authorize_endpoint"`
		} `json:"endpoints"`
		AuthorizationParams map[string]string `json:"authorization_params"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, serviceID, response.ID)
	assert.Equal(t, clientID, response.ClientID)
	assert.Equal(t, adminTestIssuer, response.IssuerURI)
	assert.Equal(t, tokenMethod, response.TokenEndpointAuthMethod)
	assert.True(t, response.Discovery.EnableDiscovery)
	assert.Equal(t, adminTestResource, response.Discovery.ResourceURL)
	assert.Equal(t, clientMethod, response.Discovery.ClientMethod)
	assert.Equal(t, "https://auth.example.test/tenant/authorize", response.Endpoints.AuthorizeEndpoint)
	assert.Equal(t, "https://auth.example.test/tenant/token", response.Endpoints.TokenEndpoint)
	assert.Equal(t, resource, response.AuthorizationParams["resource"])
	assert.NotContains(t, string(body), `"client_secret"`)
}

func TestServicesHandler_ResourceDiscoveryCreateSelectsHostedCIMD(t *testing.T) {
	repo := new(MockProviderRepository)
	client := adminCIMDDiscoveryClient(t)
	handler := setupHandler(t, repo)
	handler.providerService.WithOAuthDiscoveryClient(client)
	var saved *model.ThirdpartyOAuth2ProviderEntity
	repo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		saved = args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity).Copy()
	}).Return(nil).Once()

	request := adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
		"display_name": "MCP service",
		"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
	})
	response := httptest.NewRecorder()
	handler.CreateService(response, request)

	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var identity struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &identity))
	adminDiscoveryPayload(t, response.Body.Bytes(), identity.ID, "cimd", "private_key_jwt",
		"https://broker.example.com/.well-known/oauth-client/"+identity.ID, adminTestResource)
	require.NotNil(t, saved)
	assert.Equal(t, adminTestResource, saved.AuthorizationParams["resource"])
	assert.Equal(t, model.ClientBootstrapCIMD, saved.Discovery.ClientMethod)
	assert.Equal(t, []string{adminTestResource}, client.probeCalls)
	assert.Equal(t, []string{
		"https://mcp.example.test/.well-known/oauth-protected-resource/mcp",
		"https://auth.example.test/.well-known/oauth-authorization-server/tenant",
	}, client.getCalls)
	assert.Empty(t, client.postCalls, "compatible CIMD must not fall back to DCR")
	repo.AssertExpectations(t)
}

func TestServicesHandler_ResourceDiscoveryReadAndListRepresentActiveClient(t *testing.T) {
	for _, tc := range []struct {
		name, clientMethod, tokenMethod, clientID, resource string
		confidentialDCR                                     bool
	}{
		{"hosted CIMD with derived resource", "cimd", "private_key_jwt", "https://broker.example.com/.well-known/oauth-client/", adminTestResource, false},
		{"confidential DCR with explicit resource", "dcr", "client_secret_basic", "registered-client", "https://mcp.example.test/audience", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			handler := setupHandler(t, repo)
			serviceID := id.NewServiceID()
			clientID := tc.clientID
			if !tc.confidentialDCR {
				clientID += serviceID.String()
			}
			entity := &model.ThirdpartyOAuth2ProviderEntity{
				ID: serviceID, DisplayName: "MCP service", ClientID: id.ClientID(clientID),
				Secret: model.NewAbsentSecret(), TokenEndpointAuthMethod: model.TokenEndpointAuthMethod(tc.tokenMethod),
				IssuerURI:           adminTestIssuer,
				Discovery:           model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: ptrAdminResourceURL(), ClientMethod: model.ClientBootstrapMethod(tc.clientMethod)},
				Endpoints:           model.OAuth2Endpoints{AuthorizeEndpoint: adminTestIssuer + "/authorize", TokenEndpoint: adminTestIssuer + "/token"},
				AuthorizationParams: map[string]string{"resource": tc.resource}, ResourceExplicit: tc.confidentialDCR,
				CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Version: 7,
			}
			if tc.confidentialDCR {
				entity.Secret = model.NewEncryptedSecret(encryptSecretForTest(serviceID.String(), "registered-secret-not-for-admin"))
			}
			repo.On("Get", mock.Anything, serviceID).Return(entity, nil).Once()
			repo.On("List", mock.Anything).Return([]*model.ThirdpartyOAuth2ProviderEntity{entity}, nil).Once()

			read := httptest.NewRecorder()
			handler.GetService(read, adminDiscoveryRequest(t, http.MethodGet, serviceID.String(), nil))
			require.Equal(t, http.StatusOK, read.Code, read.Body.String())
			assert.Equal(t, `"7"`, read.Header().Get("ETag"))
			adminDiscoveryPayload(t, read.Body.Bytes(), serviceID.String(), tc.clientMethod, tc.tokenMethod, clientID, tc.resource)

			listed := httptest.NewRecorder()
			handler.ListServices(listed, adminDiscoveryRequest(t, http.MethodGet, "", nil))
			require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
			var entries []json.RawMessage
			require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &entries))
			require.Len(t, entries, 1)
			adminDiscoveryPayload(t, entries[0], serviceID.String(), tc.clientMethod, tc.tokenMethod, clientID, tc.resource)
			assert.NotContains(t, listed.Body.String(), "registered-secret-not-for-admin")
			repo.AssertExpectations(t)
		})
	}
}

func ptrAdminResourceURL() *string {
	resource := adminTestResource
	return &resource
}

func TestServicesHandler_ResourceDiscoveryUpdatePreservesIdentityAndETag(t *testing.T) {
	repo := new(MockProviderRepository)
	client := adminCIMDDiscoveryClient(t)
	handler := setupHandler(t, repo)
	handler.providerService.WithOAuthDiscoveryClient(client)
	serviceID := id.NewServiceID()
	clientID := "https://broker.example.com/.well-known/oauth-client/" + serviceID.String()
	persisted := &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "Original", ClientID: id.ClientID(clientID), Secret: model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethod("private_key_jwt"), IssuerURI: adminTestIssuer,
		Discovery:           model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: ptrAdminResourceURL(), ClientMethod: model.ClientBootstrapCIMD},
		Endpoints:           model.OAuth2Endpoints{TokenEndpoint: adminTestIssuer + "/token", AuthorizeEndpoint: adminTestIssuer + "/authorize"},
		AuthorizationParams: map[string]string{"resource": adminTestResource}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Version: 7,
	}
	repo.On("Get", mock.Anything, serviceID).Return(persisted, nil)
	repo.On("Update", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		replacement := args.Get(1).(*model.ThirdpartyOAuth2ProviderEntity)
		*persisted = *replacement.Copy()
		persisted.Version = 8
		replacement.Version = 8
	}).Return(nil).Once()

	before := httptest.NewRecorder()
	handler.GetService(before, adminDiscoveryRequest(t, http.MethodGet, serviceID.String(), nil))
	require.Equal(t, http.StatusOK, before.Code)
	assert.Equal(t, `"7"`, before.Header().Get("ETag"))
	request := adminDiscoveryRequest(t, http.MethodPut, serviceID.String(), map[string]any{
		"display_name": "Renamed MCP service",
		"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
	})
	request.Header.Set("If-Match", before.Header().Get("ETag"))
	updated := httptest.NewRecorder()
	handler.UpdateService(updated, request)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Equal(t, `"8"`, updated.Header().Get("ETag"))
	adminDiscoveryPayload(t, updated.Body.Bytes(), serviceID.String(), "cimd", "private_key_jwt", clientID, adminTestResource)
	var updatedName struct {
		DisplayName string `json:"display_name"`
	}
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &updatedName))
	assert.Equal(t, "Renamed MCP service", updatedName.DisplayName)
	assert.Equal(t, []string{adminTestResource}, client.probeCalls)
	assert.Empty(t, client.postCalls)

	after := httptest.NewRecorder()
	handler.GetService(after, adminDiscoveryRequest(t, http.MethodGet, serviceID.String(), nil))
	require.Equal(t, http.StatusOK, after.Code)
	assert.Equal(t, updated.Header().Get("ETag"), after.Header().Get("ETag"))
	adminDiscoveryPayload(t, after.Body.Bytes(), serviceID.String(), "cimd", "private_key_jwt", clientID, adminTestResource)
	var readName struct {
		DisplayName string `json:"display_name"`
	}
	require.NoError(t, json.Unmarshal(after.Body.Bytes(), &readName))
	assert.Equal(t, "Renamed MCP service", readName.DisplayName)
	repo.AssertExpectations(t)
}

func TestServicesHandler_ResourceDiscoveryRejectsManualFallbackInputs(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
	}{
		{"caller client ID", "client_id", "caller-client"},
		{"caller secret", "client_secret", "caller-secret"},
		{"caller authentication mode", "token_endpoint_auth_method", "private_key_jwt"},
		{"manual endpoint fallback", "endpoints", map[string]string{"token_endpoint": adminTestIssuer + "/token", "authorize_endpoint": adminTestIssuer + "/authorize"}},
		{"direct metadata fallback", "metadata_url", adminTestIssuer + "/.well-known/oauth-authorization-server"},
	} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				repo := new(MockProviderRepository)
				client := adminCIMDDiscoveryClient(t)
				handler := setupHandler(t, repo)
				handler.providerService.WithOAuthDiscoveryClient(client)
				discovery := map[string]any{"enable_discovery": true, "resource_url": adminTestResource}
				body := map[string]any{"display_name": "MCP service", "discovery": discovery}
				if tc.field == "metadata_url" {
					discovery[tc.field] = tc.value
				} else {
					body[tc.field] = tc.value
				}
				serviceID := ""
				if method == http.MethodPut {
					serviceID = id.NewServiceID().String()
				}
				response := httptest.NewRecorder()
				request := adminDiscoveryRequest(t, method, serviceID, body)
				if method == http.MethodPost {
					handler.CreateService(response, request)
				} else {
					handler.UpdateService(response, request)
				}
				require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				var failure ErrorResponse
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &failure))
				assert.Equal(t, "validation failed", failure.Error)
				assert.Empty(t, client.probeCalls)
				assert.Empty(t, client.getCalls)
				assert.Empty(t, client.postCalls)
				repo.AssertNotCalled(t, "Create")
				repo.AssertNotCalled(t, "Update")
			})
		}
	}
}

func TestServicesHandler_ResourceMetadataFailureDoesNotCreateOrExposeRemoteData(t *testing.T) {
	repo := new(MockProviderRepository)
	client := adminCIMDDiscoveryClient(t)
	client.documents["https://mcp.example.test/.well-known/oauth-protected-resource/mcp"] =
		`{"resource":"https://other.example.test/mcp?secret=remote-secret-not-for-admin","authorization_servers":["https://auth.example.test/tenant"]}`
	handler := setupHandler(t, repo)
	handler.providerService.WithOAuthDiscoveryClient(client)
	response := httptest.NewRecorder()
	handler.CreateService(response, adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
		"display_name": "MCP service",
		"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
	}))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	var failure ErrorResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &failure))
	assert.Equal(t, "discovery failed", failure.Error)
	assert.Equal(t, "resource_mismatch", failure.Message)
	assert.NotContains(t, response.Body.String(), "remote-secret-not-for-admin")
	assert.Equal(t, []string{adminTestResource}, client.probeCalls)
	assert.Equal(t, []string{"https://mcp.example.test/.well-known/oauth-protected-resource/mcp"}, client.getCalls)
	assert.Empty(t, client.postCalls)
	repo.AssertNotCalled(t, "Create")
}

func TestServicesHandler_DirectMetadataModeKeepsExistingCredentials(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_endpoint":"https://issuer.example.test/token","authorization_endpoint":"https://issuer.example.test/authorize"}`))
	}))
	defer server.Close()
	repo := new(MockProviderRepository)
	config := testConfig()
	config.Security.SkipThirdpartyHTTPSValidation = true
	handler := setupHandlerWithConfig(t, repo, config)
	client := &adminDiscoveryClient{t: t}
	handler.providerService.WithOAuthDiscoveryClient(client)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	response := httptest.NewRecorder()
	handler.CreateService(response, adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
		"display_name": "Direct metadata service", "client_id": "existing-client", "client_secret": "manual-secret-not-for-admin",
		"issuer_uri": server.URL,
		"discovery":  map[string]any{"enable_discovery": true, "metadata_url": server.URL},
	}))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var representation ServiceResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &representation))
	assert.Equal(t, "existing-client", representation.ClientID)
	require.NotNil(t, representation.ClientSecret)
	assert.Equal(t, "REDACTED", *representation.ClientSecret)
	require.NotNil(t, representation.Discovery.MetadataURL)
	assert.Equal(t, server.URL, *representation.Discovery.MetadataURL)
	assert.Empty(t, representation.AuthorizationParams["resource"])
	assert.NotContains(t, response.Body.String(), "manual-secret-not-for-admin")
	assert.Equal(t, int32(1), requests.Load())
	assert.Empty(t, client.probeCalls)
	assert.Empty(t, client.getCalls)
	assert.Empty(t, client.postCalls)
	repo.AssertExpectations(t)
}

func TestServicesHandler_ManualModeDoesNotUseResourceDiscovery(t *testing.T) {
	repo := new(MockProviderRepository)
	client := &adminDiscoveryClient{t: t}
	handler := setupHandler(t, repo)
	handler.providerService.WithOAuthDiscoveryClient(client)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	response := httptest.NewRecorder()
	handler.CreateService(response, adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
		"display_name": "Manual service", "client_id": "manual-client", "client_secret": "manual-secret-not-for-admin",
		"issuer_uri": "https://manual.example.test",
		"discovery":  map[string]any{"enable_discovery": false},
		"endpoints":  map[string]string{"token_endpoint": "https://manual.example.test/token", "authorize_endpoint": "https://manual.example.test/authorize"},
	}))
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var representation ServiceResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &representation))
	assert.Equal(t, "manual-client", representation.ClientID)
	assert.Equal(t, "https://manual.example.test/token", representation.Endpoints.TokenEndpoint)
	assert.False(t, representation.Discovery.EnableDiscovery)
	require.NotNil(t, representation.ClientSecret)
	assert.Equal(t, "REDACTED", *representation.ClientSecret)
	assert.Empty(t, representation.AuthorizationParams["resource"])
	assert.NotContains(t, response.Body.String(), "manual-secret-not-for-admin")
	assert.Empty(t, client.probeCalls)
	assert.Empty(t, client.getCalls)
	assert.Empty(t, client.postCalls)
	repo.AssertExpectations(t)
}

func TestServicesHandler_ResourceMetadataFailureKeepsActiveService(t *testing.T) {
	repo := new(MockProviderRepository)
	client := adminCIMDDiscoveryClient(t)
	client.documents["https://mcp.example.test/.well-known/oauth-protected-resource/mcp"] =
		`{"resource":"https://other.example.test/mcp","authorization_servers":["https://auth.example.test/tenant"]}`
	handler := setupHandler(t, repo)
	handler.providerService.WithOAuthDiscoveryClient(client)
	serviceID := id.NewServiceID()
	clientID := "https://broker.example.com/.well-known/oauth-client/" + serviceID.String()
	persisted := &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "Active MCP", ClientID: id.ClientID(clientID), Secret: model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethod("private_key_jwt"), IssuerURI: adminTestIssuer,
		Discovery:           model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: ptrAdminResourceURL(), ClientMethod: model.ClientBootstrapCIMD},
		Endpoints:           model.OAuth2Endpoints{TokenEndpoint: adminTestIssuer + "/token", AuthorizeEndpoint: adminTestIssuer + "/authorize"},
		AuthorizationParams: map[string]string{"resource": adminTestResource}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Version: 9,
	}
	repo.On("Get", mock.Anything, serviceID).Return(persisted, nil)
	initial := httptest.NewRecorder()
	handler.GetService(initial, adminDiscoveryRequest(t, http.MethodGet, serviceID.String(), nil))
	require.Equal(t, http.StatusOK, initial.Code)
	assert.Equal(t, `"9"`, initial.Header().Get("ETag"))

	request := adminDiscoveryRequest(t, http.MethodPut, serviceID.String(), map[string]any{
		"display_name": "Failed replacement",
		"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
	})
	request.Header.Set("If-Match", initial.Header().Get("ETag"))
	repo.On("RecordDiscoveryFailure", mock.Anything, serviceID, int64(9), mock.AnythingOfType("time.Time"), "resource_mismatch").Return(nil).Once()
	failed := httptest.NewRecorder()
	handler.UpdateService(failed, request)
	require.Equal(t, http.StatusBadRequest, failed.Code, failed.Body.String())
	var failure ErrorResponse
	require.NoError(t, json.Unmarshal(failed.Body.Bytes(), &failure))
	assert.Equal(t, "discovery failed", failure.Error)
	assert.Equal(t, "resource_mismatch", failure.Message)
	repo.AssertNotCalled(t, "Update")
	assert.Equal(t, []string{adminTestResource}, client.probeCalls)
	assert.Equal(t, []string{"https://mcp.example.test/.well-known/oauth-protected-resource/mcp"}, client.getCalls)
	assert.Empty(t, client.postCalls)

	active := httptest.NewRecorder()
	handler.GetService(active, adminDiscoveryRequest(t, http.MethodGet, serviceID.String(), nil))
	require.Equal(t, http.StatusOK, active.Code)
	assert.Equal(t, initial.Header().Get("ETag"), active.Header().Get("ETag"))
	adminDiscoveryPayload(t, active.Body.Bytes(), serviceID.String(), "cimd", "private_key_jwt", clientID, adminTestResource)
	var readBack struct {
		DisplayName string `json:"display_name"`
	}
	require.NoError(t, json.Unmarshal(active.Body.Bytes(), &readBack))
	assert.Equal(t, "Active MCP", readBack.DisplayName)
	repo.AssertExpectations(t)
}

func TestServicesHandler_ResourceDiscoveryFailureReturnsOnlyValidatedIssuerChoices(t *testing.T) {
	const privateValue = "remote-credential-not-for-admin"
	for _, tc := range []struct {
		name       string
		document   string
		wantCode   string
		wantChoice []string
	}{
		{
			name:       "multiple valid issuers in published order",
			document:   `{"resource":"https://mcp.example.test/mcp","authorization_servers":["https://login-b.example.test/tenant","https://login-a.example.test"]}`,
			wantCode:   "issuer_selection_required",
			wantChoice: []string{"https://login-b.example.test/tenant", "https://login-a.example.test"},
		},
		{
			name:     "mismatched resource never releases candidate issuers",
			document: `{"resource":"https://mcp.example.test/other?token=remote-credential-not-for-admin","authorization_servers":["https://login-a.example.test","https://login-b.example.test"]}`,
			wantCode: "resource_mismatch",
		},
		{
			name:     "query-bearing issuer never appears as a candidate",
			document: `{"resource":"https://mcp.example.test/mcp","authorization_servers":["https://login-a.example.test","https://login-b.example.test/?token=remote-credential-not-for-admin"]}`,
			wantCode: "resource_metadata_invalid",
		},
		{
			name:     "duplicate issuer never appears as a candidate",
			document: `{"resource":"https://mcp.example.test/mcp","authorization_servers":["https://login-a.example.test","https://login-a.example.test"]}`,
			wantCode: "resource_metadata_invalid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			client := adminCIMDDiscoveryClient(t)
			client.documents["https://mcp.example.test/.well-known/oauth-protected-resource/mcp"] = tc.document
			handler := setupHandler(t, repo)
			handler.providerService.WithOAuthDiscoveryClient(client)
			response := httptest.NewRecorder()
			handler.CreateService(response, adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
				"display_name": "MCP service",
				"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
			}))

			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			want := map[string]any{"error": "discovery failed", "message": tc.wantCode}
			if tc.wantChoice != nil {
				want["authorization_servers"] = []any{tc.wantChoice[0], tc.wantChoice[1]}
			}
			assert.Equal(t, want, payload, "only validated issuer choices may leave the discovery boundary")
			assert.NotContains(t, response.Body.String(), privateValue)
			assert.Equal(t, []string{adminTestResource}, client.probeCalls)
			assert.Equal(t, []string{"https://mcp.example.test/.well-known/oauth-protected-resource/mcp"}, client.getCalls)
			assert.Empty(t, client.postCalls)
			repo.AssertNotCalled(t, "Create")
		})
	}
}

func TestServicesHandler_DiscoveryErrorMapsSafeStatusesWithoutLeakingOtherIssuerLists(t *testing.T) {
	for _, tc := range []struct {
		name, code, category string
		status               int
	}{
		{name: "destination rejected", code: "unsafe_destination", category: "discovery failed", status: http.StatusBadRequest},
		{name: "DCR rejected", code: "client_registration_rejected", category: "client registration failed", status: http.StatusBadRequest},
		{name: "discovery timed out", code: "timeout", category: "discovery failed", status: http.StatusGatewayTimeout},
		{name: "duplicate registered client", code: "duplicate_client_identity", category: "conflict", status: http.StatusConflict},
		{name: "unknown provider error", code: "remote-secret-not-for-admin", category: "internal server error", status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := setupHandler(t, new(MockProviderRepository))
			response := httptest.NewRecorder()
			handler.writeDiscoveryError(response, &thirdparty.DiscoveryError{
				Code: tc.code, AuthorizationServers: []string{"https://login.example.test/?client_secret=remote-secret-not-for-admin"},
			})

			require.Equal(t, tc.status, response.Code, response.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			want := map[string]any{"error": tc.category}
			if tc.status != http.StatusInternalServerError {
				want["message"] = tc.code
			}
			assert.Equal(t, want, payload, "non-selection failures must not include authorization_servers")
			assert.NotContains(t, response.Body.String(), "remote-secret-not-for-admin")
		})
	}
}

func TestServicesHandler_SelectionErrorRefusesUnvalidatedIssuerChoices(t *testing.T) {
	for _, tc := range []struct {
		name    string
		issuers []string
	}{
		{name: "single choice", issuers: []string{"https://login-a.example.test"}},
		{name: "duplicate choices", issuers: []string{"https://login-a.example.test", "https://login-a.example.test"}},
		{name: "non-HTTPS choice", issuers: []string{"https://login-a.example.test", "http://login-b.example.test"}},
		{name: "query containing credential", issuers: []string{"https://login-a.example.test", "https://login-b.example.test/?secret=remote-secret-not-for-admin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := setupHandler(t, new(MockProviderRepository))
			response := httptest.NewRecorder()
			handler.writeDiscoveryError(response, &thirdparty.DiscoveryError{Code: "issuer_selection_required", AuthorizationServers: tc.issuers})

			require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			assert.Equal(t, map[string]any{"error": "internal server error"}, payload)
			assert.NotContains(t, response.Body.String(), "remote-secret-not-for-admin")
		})
	}
}

type adminRegisteredDCRClient struct{ *adminDiscoveryClient }

func (c *adminRegisteredDCRClient) PostJSON(_ context.Context, rawURL string, _ []byte) ([]byte, error) {
	c.postCalls = append(c.postCalls, rawURL)
	return []byte(`{"client_id":"registered-duplicate","client_secret":"registered-secret-not-for-admin","token_endpoint_auth_method":"client_secret_basic"}`), nil
}

func TestServicesHandler_DuplicateDCRIdentityConflictRequiresTypedStorageCause(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cause    error
		wantCode string
	}{
		{name: "duplicate registered identity", cause: storage.ErrDuplicateDCRClientIdentity, wantCode: "duplicate_client_identity"},
		{name: "other storage conflict", wantCode: "service conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			client := &adminRegisteredDCRClient{adminDiscoveryClient: adminCIMDDiscoveryClient(t)}
			client.documents["https://auth.example.test/.well-known/oauth-authorization-server/tenant"] =
				`{"issuer":"https://auth.example.test/tenant","authorization_endpoint":"https://auth.example.test/tenant/authorize","token_endpoint":"https://auth.example.test/tenant/token","registration_endpoint":"https://auth.example.test/tenant/register","token_endpoint_auth_methods_supported":["client_secret_basic"]}`
			handler := setupHandler(t, repo)
			handler.providerService.WithOAuthDiscoveryClient(client).WithDCRClientName("Broker")
			repo.On("Create", mock.Anything, mock.MatchedBy(func(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
				return entity.Discovery.ClientMethod == model.ClientBootstrapDCR && entity.ClientID.String() == "registered-duplicate" && entity.Secret.IsEncrypted()
			})).Return(storage.NewStorageError("Create", storage.ErrorKindConflict, tc.cause, "credential registered-secret-not-for-admin already exists")).Once()

			response := httptest.NewRecorder()
			handler.CreateService(response, adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
				"display_name": "MCP service",
				"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
			}))

			require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			assert.Equal(t, map[string]any{"error": "conflict", "message": tc.wantCode}, payload)
			assert.NotContains(t, response.Body.String(), "registered-secret-not-for-admin")
			assert.Equal(t, []string{"https://auth.example.test/tenant/register"}, client.postCalls)
			repo.AssertExpectations(t)
		})
	}
}

// The status resource reads persisted state, even when the active DCR credential
// cannot be decrypted by the HTTP read path.
func TestServicesHandler_DiscoveryStatusRepresentsCommittedStateWithoutCredentials(t *testing.T) {
	serviceID := id.NewServiceID()
	lastSuccess := time.Date(2026, time.October, 6, 10, 0, 0, 0, time.UTC)
	lastAttempt := lastSuccess.Add(time.Hour)
	failureCode := "authorization_server_metadata_invalid"
	for _, tc := range []struct {
		name   string
		entity *model.ThirdpartyOAuth2ProviderEntity
		want   string
	}{
		{
			name:   "manual configuration does not expose its issuer or discovery fields",
			entity: encryptedEntity(serviceID, "Manual service", "manual-client", "private-manual-secret", adminTestIssuer, nil),
			want:   `{"status":"not_applicable","resource_url":null,"issuer_uri":null,"client_method":null,"last_attempt_at":null,"last_success_at":null,"failure_reason":null}`,
		},
		{
			name: "ready configuration has equal successful timestamps",
			entity: &model.ThirdpartyOAuth2ProviderEntity{
				ID: serviceID, IssuerURI: adminTestIssuer,
				Discovery:       model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: ptrAdminResourceURL(), ClientMethod: model.ClientBootstrapCIMD},
				DiscoveryStatus: model.DiscoveryStatus{LastAttemptAt: &lastSuccess, LastSuccessAt: &lastSuccess},
			},
			want: `{"status":"ready","resource_url":"https://mcp.example.test/mcp","issuer_uri":"https://auth.example.test/tenant","client_method":"cimd","last_attempt_at":"2026-10-06T10:00:00Z","last_success_at":"2026-10-06T10:00:00Z","failure_reason":null}`,
		},
		{
			name: "failed refresh reports the active DCR identity and safe failure code",
			entity: &model.ThirdpartyOAuth2ProviderEntity{
				ID: serviceID, ClientID: "registered-client", IssuerURI: adminTestIssuer,
				Secret:          model.NewEncryptedSecret(encryptSecretForTest(serviceID.String(), "private-registered-secret")),
				Discovery:       model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: ptrAdminResourceURL(), ClientMethod: model.ClientBootstrapDCR},
				DiscoveryStatus: model.DiscoveryStatus{LastAttemptAt: &lastAttempt, LastSuccessAt: &lastSuccess, FailureReason: &failureCode},
			},
			want: `{"status":"failed","resource_url":"https://mcp.example.test/mcp","issuer_uri":"https://auth.example.test/tenant","client_method":"dcr","last_attempt_at":"2026-10-06T11:00:00Z","last_success_at":"2026-10-06T10:00:00Z","failure_reason":"authorization_server_metadata_invalid"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			client := adminCIMDDiscoveryClient(t)
			handler := setupHandler(t, repo)
			handler.providerService.WithOAuthDiscoveryClient(client)
			repo.On("Get", mock.Anything, serviceID).Return(tc.entity, nil).Once()
			router := chi.NewRouter()
			router.With(middleware.RequirePrincipalMiddleware(ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}}, nil, slog.Default())).Get("/api/services/{service-id}/discovery-status", handler.GetDiscoveryStatus)

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/services/"+serviceID.String()+"/discovery-status", nil)
			request.Header.Set("X-Remote-User", "admin@example.test")
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.JSONEq(t, tc.want, response.Body.String())
			assert.Empty(t, client.probeCalls)
			assert.Empty(t, client.getCalls)
			assert.Empty(t, client.postCalls)
			repo.AssertExpectations(t)
		})
	}
}

func TestServicesHandler_DiscoveryStatusRequiresPrincipalAndResolvesCanonicalID(t *testing.T) {
	repo := new(MockProviderRepository)
	client := adminCIMDDiscoveryClient(t)
	handler := setupHandler(t, repo)
	handler.providerService.WithOAuthDiscoveryClient(client)
	serviceID := id.NewServiceID()
	canonicalID := "connected-mcp"
	lastSuccess := time.Date(2026, time.October, 6, 10, 0, 0, 0, time.UTC)
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, CanonicalID: &canonicalID, IssuerURI: adminTestIssuer,
		Discovery:       model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: ptrAdminResourceURL(), ClientMethod: model.ClientBootstrapCIMD},
		DiscoveryStatus: model.DiscoveryStatus{LastAttemptAt: &lastSuccess, LastSuccessAt: &lastSuccess},
	}
	repo.On("Get", mock.Anything, serviceID).Return(entity, nil).Twice()
	repo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(entity, nil).Once()
	router := chi.NewRouter()
	router.Use(middleware.RequirePrincipalMiddleware(ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
	}, nil, slog.Default()))
	router.Get("/api/services/{service-id}/discovery-status", handler.GetDiscoveryStatus)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/services/"+serviceID.String()+"/discovery-status", nil))
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, unauthenticated.Body.String())
	assert.NotContains(t, unauthenticated.Body.String(), adminTestResource)

	for _, pathID := range []string{serviceID.String(), canonicalID} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/services/"+pathID+"/discovery-status", nil)
		request.Header.Set("X-Remote-User", "admin@example.test")
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, `{"status":"ready","resource_url":"https://mcp.example.test/mcp","issuer_uri":"https://auth.example.test/tenant","client_method":"cimd","last_attempt_at":"2026-10-06T10:00:00Z","last_success_at":"2026-10-06T10:00:00Z","failure_reason":null}`, response.Body.String())
	}
	assert.Empty(t, client.probeCalls)
	assert.Empty(t, client.getCalls)
	assert.Empty(t, client.postCalls)
	repo.AssertExpectations(t)
}

func TestServicesHandler_DiscoveryStatusRejectsMalformedAndMissingIDs(t *testing.T) {
	unknownID := id.NewServiceID()
	for _, tc := range []struct {
		name, pathID string
		wantStatus   int
		canonical    bool
	}{
		{"malformed identifier", "bad!id", http.StatusBadRequest, true},
		{"unknown UUID", unknownID.String(), http.StatusNotFound, false},
		{"unknown canonical ID", "unknown-service", http.StatusNotFound, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			missing := storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "service not found")
			if tc.canonical {
				lookup := repo.On("GetByCanonicalID", mock.Anything, tc.pathID).Return(nil, missing)
				if tc.wantStatus == http.StatusBadRequest {
					lookup.Maybe()
				} else {
					lookup.Once()
				}
			} else {
				repo.On("Get", mock.Anything, id.MustParseServiceID(tc.pathID)).Return(nil, missing).Once()
			}
			handler := setupHandler(t, repo)
			router := chi.NewRouter()
			router.With(middleware.RequirePrincipalMiddleware(ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}}, nil, slog.Default())).Get("/api/services/{service-id}/discovery-status", handler.GetDiscoveryStatus)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/services/"+tc.pathID+"/discovery-status", nil)
			request.Header.Set("X-Remote-User", "admin@example.test")
			router.ServeHTTP(response, request)
			require.Equal(t, tc.wantStatus, response.Code, response.Body.String())
			repo.AssertExpectations(t)
		})
	}
}

func TestServicesHandler_DiscoveryStatusReturnsNotFoundAfterDeletion(t *testing.T) {
	repo := new(MockProviderRepository)
	handler := setupHandler(t, repo)
	serviceID := id.NewServiceID()
	repo.On("Delete", mock.Anything, serviceID).Return(nil).Once()
	repo.On("Get", mock.Anything, serviceID).Return(nil,
		storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "service not found")).Once()
	router := chi.NewRouter()
	router.Delete("/api/services/{service-id}", handler.DeleteService)
	router.With(middleware.RequirePrincipalMiddleware(ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}}, nil, slog.Default())).Get("/api/services/{service-id}/discovery-status", handler.GetDiscoveryStatus)

	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/api/services/"+serviceID.String(), nil))
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())

	status := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/services/"+serviceID.String()+"/discovery-status", nil)
	statusRequest.Header.Set("X-Remote-User", "admin@example.test")
	router.ServeHTTP(status, statusRequest)
	require.Equal(t, http.StatusNotFound, status.Code, status.Body.String())
	repo.AssertExpectations(t)
}

func TestServicesHandler_ConcurrentIssuerChangeConflictUsesSafeCode(t *testing.T) {
	handler := NewServicesHandler(nil, nil, nil)
	response := httptest.NewRecorder()
	err := storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict,
		storage.ErrIssuerChangeHasSessions, "issuer change requires no user sessions")
	handler.handleServiceMutationError(response, httptest.NewRequest(http.MethodPut, "/api/services/example", nil), "UpdateService", err, true)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.JSONEq(t, `{"error":"conflict","message":"issuer_change_requires_no_sessions"}`, response.Body.String())
}
