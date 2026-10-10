package oauth2session

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type operationBranchKeyManager struct{}

func (operationBranchKeyManager) Create(context.Context, domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

type operationSessionRepository struct {
	ports.UserSessionRepository
	find func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error)
}

func (r operationSessionRepository) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	return r.find(ctx, principal, serviceID)
}

type operationRefreshRepository func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error)

func (r operationRefreshRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	return r(ctx, principal, serviceID, refresh)
}

type operationProviderRepository struct {
	ports.ThirdpartyOAuth2ProviderRepository
	get func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
}

func (r operationProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return r.get(ctx, serviceID)
}

type operationEncryption struct {
	encrypt func(context.Context, []byte, map[string]string) ([]byte, error)
	decrypt func(context.Context, []byte, map[string]string) ([]byte, error)
}

func (e operationEncryption) Encrypt(ctx context.Context, data []byte, binding map[string]string) ([]byte, error) {
	if e.encrypt != nil {
		return e.encrypt(ctx, data, binding)
	}
	return data, nil
}

func (e operationEncryption) Decrypt(ctx context.Context, data []byte, binding map[string]string) ([]byte, error) {
	if e.decrypt != nil {
		return e.decrypt(ctx, data, binding)
	}
	return data, nil
}

type operationTransport func(*http.Request) (*http.Response, error)

func (f operationTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type operationSigner func(context.Context, id.ClientID, string) (string, error)

func (f operationSigner) SignClientAssertion(ctx context.Context, clientID id.ClientID, audience string) (string, error) {
	return f(ctx, clientID, audience)
}

func newOperationTestService(t *testing.T, logs io.Writer) (*OAuth2SessionService, *model.ThirdpartyOAuth2ProviderEntity, *storage.UserSession) {
	t.Helper()
	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID: id.NewServiceID(), ClientID: "public-client", Secret: model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
		Endpoints:               model.OAuth2Endpoints{TokenEndpoint: "https://provider.example/token"},
	}
	expired := time.Now().Add(-time.Hour)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: "user", ServiceID: provider.ID,
		EncryptedAccessToken: []byte("old-access"), EncryptedRefreshToken: []byte("old-refresh"),
		AccessTokenExpiresAt: &expired,
	}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	repository := operationSessionRepository{find: func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
		return session, nil
	}}
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(operationProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		return provider, nil
	}}, operationEncryption{}, operationBranchKeyManager{}, nil, false, logger)
	service := &OAuth2SessionService{
		providerService: providerService, sessionRepo: repository, encryption: operationEncryption{},
		logger: logger, config: DefaultConfig(),
		httpClient: &http.Client{Transport: operationTransport(func(*http.Request) (*http.Response, error) {
			return operationTokenResponse(200, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`), nil
		})},
		refreshRepo: operationRefreshRepository(func(ctx context.Context, _ id.Principal, _ id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
			current := *session
			_, err := refresh(ctx, &current)
			return &current, err
		}),
	}
	return service, provider, session
}

func operationTokenResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func requireOperationError(t *testing.T, err error, operation Operation, detail ErrorDetail) ErrorMetadata {
	t.Helper()
	var failure *OperationError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, operation, failure.Metadata().Operation())
	assert.Equal(t, detail, failure.Metadata().Detail())
	return failure.Metadata()
}

func TestGetValidAccessTokenClassifiesEveryFailingDependencyOrigin(t *testing.T) {
	cause := errors.New("sentinel-private-dependency-cause")
	for _, tc := range []struct {
		name       string
		operation  Operation
		detail     ErrorDetail
		dependency Dependency
		configure  func(*OAuth2SessionService, *model.ThirdpartyOAuth2ProviderEntity, *storage.UserSession)
	}{
		{"repository", OperationSessionLookup, DetailRepositoryUnavailable, DependencySessionRepository, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.sessionRepo = operationSessionRepository{find: func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) { return nil, cause }}
		}},
		{"valid access decryption", OperationSessionLookup, DetailDecryptionFailed, DependencyEncryption, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, session *storage.UserSession) {
			session.AccessTokenExpiresAt = nil
			s.encryption = operationEncryption{decrypt: func(context.Context, []byte, map[string]string) ([]byte, error) { return nil, cause }}
		}},
		{"provider repository", OperationRefresh, DetailRepositoryUnavailable, DependencyProviderRepository, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(operationProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) { return nil, cause }}, operationEncryption{}, operationBranchKeyManager{}, nil, false, s.logger)
		}},
		{"session lock", OperationRefresh, DetailRepositoryUnavailable, DependencySessionRepository, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.refreshRepo = operationRefreshRepository(func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
				return nil, cause
			})
		}},
		{"refresh decryption", OperationRefresh, DetailDecryptionFailed, DependencyEncryption, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.encryption = operationEncryption{decrypt: func(context.Context, []byte, map[string]string) ([]byte, error) { return nil, cause }}
		}},
		{"provider network", OperationRefresh, DetailProviderUnavailable, DependencyProvider, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) { return nil, cause })
		}},
		{"access encryption", OperationRefresh, DetailEncryptionFailed, DependencyEncryption, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.encryption = operationEncryption{encrypt: func(context.Context, []byte, map[string]string) ([]byte, error) { return nil, cause }}
		}},
		{"refresh encryption", OperationRefresh, DetailEncryptionFailed, DependencyEncryption, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.encryption = operationEncryption{encrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
				if string(data) == "new-refresh" {
					return nil, cause
				}
				return data, nil
			}}
		}},
		{"persistence", OperationRefresh, DetailPersistenceFailed, DependencySessionRepository, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, session *storage.UserSession) {
			s.refreshRepo = operationRefreshRepository(func(ctx context.Context, _ id.Principal, _ id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
				current := *session
				_, err := refresh(ctx, &current)
				if err != nil {
					return nil, err
				}
				return nil, cause
			})
		}},
		{"refreshed access decryption", OperationRefresh, DetailDecryptionFailed, DependencyEncryption, func(s *OAuth2SessionService, _ *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			s.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
				if string(data) == "new-access" {
					return nil, cause
				}
				return data, nil
			}}
		}},
		{"assertion signing", OperationRefresh, DetailProviderUnavailable, DependencySigning, func(s *OAuth2SessionService, provider *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodPrivateKeyJWT
			s.cimdAssertionSigner = operationSigner(func(context.Context, id.ClientID, string) (string, error) { return "", cause })
		}},
		{"provider secret decryption", OperationRefresh, DetailDecryptionFailed, DependencyEncryption, func(s *OAuth2SessionService, provider *model.ThirdpartyOAuth2ProviderEntity, _ *storage.UserSession) {
			provider.TokenEndpointAuthMethod = ""
			provider.Secret = model.NewEncryptedSecret([]byte("secret-ciphertext"))
			encryption := operationEncryption{decrypt: func(context.Context, []byte, map[string]string) ([]byte, error) { return nil, cause }}
			s.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(operationProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return provider, nil
			}}, encryption, operationBranchKeyManager{}, nil, false, s.logger)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs strings.Builder
			service, provider, session := newOperationTestService(t, &logs)
			tc.configure(service, provider, session)
			_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
			metadata := requireOperationError(t, err, tc.operation, tc.detail)
			assert.Equal(t, tc.dependency, metadata.Dependency())
			assert.ErrorIs(t, err, cause)
			assert.NotContains(t, err.Error(), cause.Error())
			assert.NotContains(t, logs.String(), cause.Error())
		})
	}
}

func TestGetValidAccessTokenPreservesMissingSessionCause(t *testing.T) {
	for _, locked := range []bool{false, true} {
		service, provider, session := newOperationTestService(t, io.Discard)
		if locked {
			service.refreshRepo = operationRefreshRepository(func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
				return nil, ports.ErrNotFound
			})
		} else {
			service.sessionRepo = operationSessionRepository{find: func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
				return nil, ports.ErrNotFound
			}}
		}
		_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
		assert.ErrorIs(t, err, ports.ErrNotFound)
		assert.ErrorIs(t, err, ErrSessionNotFound)
		requireOperationError(t, err, OperationSessionLookup, DetailSessionMissing)
	}
}

func TestRefreshAccessTokenClassifiesDirectFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		detail    ErrorDetail
		configure func(*OAuth2SessionService, **model.ThirdpartyOAuth2ProviderEntity, *string)
	}{
		{"nil provider", DetailConfiguration, func(_ *OAuth2SessionService, provider **model.ThirdpartyOAuth2ProviderEntity, _ *string) {
			*provider = nil
		}},
		{"empty refresh", DetailRefreshUnavailable, func(_ *OAuth2SessionService, _ **model.ThirdpartyOAuth2ProviderEntity, token *string) { *token = "" }},
		{"encrypted client secret", DetailDecryptionFailed, func(_ *OAuth2SessionService, provider **model.ThirdpartyOAuth2ProviderEntity, _ *string) {
			(*provider).TokenEndpointAuthMethod = ""
			(*provider).Secret = model.NewEncryptedSecret([]byte("encrypted-secret"))
		}},
		{"invalid endpoint", DetailConfiguration, func(_ *OAuth2SessionService, provider **model.ThirdpartyOAuth2ProviderEntity, _ *string) {
			(*provider).Endpoints.TokenEndpoint = "://sentinel-url"
		}},
		{"missing signer", DetailConfiguration, func(_ *OAuth2SessionService, provider **model.ThirdpartyOAuth2ProviderEntity, _ *string) {
			(*provider).TokenEndpointAuthMethod = model.TokenEndpointAuthMethodPrivateKeyJWT
		}},
		{"invalid JSON", DetailProviderResponseInvalid, func(s *OAuth2SessionService, _ **model.ThirdpartyOAuth2ProviderEntity, _ *string) {
			s.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				return operationTokenResponse(200, "sentinel-invalid-json"), nil
			})
		}},
		{"missing access token", DetailProviderResponseInvalid, func(s *OAuth2SessionService, _ **model.ThirdpartyOAuth2ProviderEntity, _ *string) {
			s.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) { return operationTokenResponse(200, `{}`), nil })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, provider, _ := newOperationTestService(t, io.Discard)
			token := "refresh"
			tc.configure(service, &provider, &token)
			_, err := service.RefreshAccessToken(context.Background(), provider, token)
			metadata := requireOperationError(t, err, OperationRefresh, tc.detail)
			if tc.detail == DetailProviderResponseInvalid {
				assert.Equal(t, http.StatusOK, metadata.StatusCode())
			}
		})
	}
}

func TestRefreshDeadlineClassificationRequiresCallerEvidence(t *testing.T) {
	for _, shared := range []bool{false, true} {
		service, provider, _ := newOperationTestService(t, io.Discard)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		if shared {
			ctx = context.WithValue(ctx, sharedRefreshContextKey{}, true)
		}
		_, err := service.RefreshAccessToken(ctx, provider, "refresh")
		cancel()
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		detail := DetailCallerCanceled
		if shared {
			detail = DetailProviderUnavailable
		}
		requireOperationError(t, err, OperationRefresh, detail)
	}
	service, provider, session := newOperationTestService(t, io.Discard)
	service.httpClient.Timeout = time.Millisecond
	service.httpClient.Transport = operationTransport(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	metadata := requireOperationError(t, err, OperationRefresh, DetailProviderUnavailable)
	assert.Equal(t, KindInfrastructure, metadata.Kind())
}

func TestSessionRepositoryDeadlineIsNotCallerCancellation(t *testing.T) {
	service, provider, session := newOperationTestService(t, io.Discard)
	service.sessionRepo = operationSessionRepository{find: func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
		return nil, context.DeadlineExceeded
	}}
	_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	metadata := requireOperationError(t, err, OperationSessionLookup, DetailRepositoryUnavailable)
	assert.Equal(t, KindInfrastructure, metadata.Kind())
}

func TestExchangeAndRefreshLogsOmitURLAndCredentialMaterial(t *testing.T) {
	const endpoint = "https://sentinel-user:sentinel-password@sentinel-host.example/sentinel-path?secret=sentinel-query#sentinel-fragment"
	for _, mode := range []string{"refresh success", "refresh rejected", "refresh network", "exchange network"} {
		t.Run(mode, func(t *testing.T) {
			var logs strings.Builder
			service, provider, session := newOperationTestService(t, &logs)
			provider.Endpoints.TokenEndpoint = endpoint
			provider.ClientID = "sentinel-client-id"
			provider.TokenEndpointAuthMethod = ""
			provider.Secret = model.NewEncryptedSecret([]byte("sentinel-client-secret"))
			cause := errors.New("sentinel-transport-body-header-token")
			service.httpClient.Transport = operationTransport(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, endpoint, req.URL.String(), "telemetry sanitization must not rewrite network requests")
				switch mode {
				case "refresh success":
					return operationTokenResponse(200, `{"access_token":"sentinel-access","refresh_token":"sentinel-refresh","expires_in":3600}`), nil
				case "refresh rejected":
					response := operationTokenResponse(400, `{"error":"sentinel-error-code","error_description":"sentinel-description","error_uri":"sentinel-error-uri"}`)
					response.Header.Set("X-Secret", "sentinel-header")
					return response, nil
				default:
					return nil, cause
				}
			})
			if mode == "exchange network" {
				provider, err := service.providerService.GetForTokenAcquisition(context.Background(), provider.ID)
				require.NoError(t, err)
				service.config.MaxRetries = 2
				service.config.RetryBaseDelay = time.Nanosecond
				cfg := &oauth2.Config{ClientID: provider.ClientID.String(), Endpoint: oauth2.Endpoint{TokenURL: endpoint, AuthStyle: oauth2.AuthStyleInParams}}
				_, err = service.exchangeCodeWithRetry(context.Background(), cfg, provider, nil, "sentinel-code", "sentinel-verifier", nil)
				assert.ErrorIs(t, err, cause)
				assert.NotContains(t, err.Error(), "sentinel-")
			} else {
				_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
				if mode == "refresh success" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					assert.NotContains(t, err.Error(), "sentinel-")
				}
			}
			assert.Contains(t, logs.String(), provider.ID.String())
			assert.NotContains(t, logs.String(), "sentinel-")
			for _, field := range []string{`"error":`, `"err":`, `"token_endpoint":`, `"callback_url":`, `"client_id":`} {
				assert.NotContains(t, logs.String(), field)
			}
		})
	}
}

func TestForceRefreshValidAccessWithoutRefreshIsUnavailableNotExpired(t *testing.T) {
	service, provider, session := newOperationTestService(t, io.Discard)
	session.AccessTokenExpiresAt = nil
	session.EncryptedRefreshToken = nil
	_, err := service.ForceRefreshSession(context.Background(), session.Principal, provider.ID)
	requireOperationError(t, err, OperationRefresh, DetailRefreshUnavailable)
	assert.ErrorIs(t, err, ErrRefreshNotAvailable)
	assert.NotErrorIs(t, err, ErrRefreshTokenExpired)
	assert.NotErrorIs(t, err, ErrSessionExpired)
}

type operationFailingReader struct{ cause error }

func (r operationFailingReader) Read([]byte) (int, error) { return 0, r.cause }

func TestRefreshRejectionBodyReadPreservesDependencyCause(t *testing.T) {
	cause := errors.New("sentinel-private-body-read-cause")
	service, provider, _ := newOperationTestService(t, io.Discard)
	service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(operationFailingReader{cause: cause})}, nil
	})
	_, err := service.RefreshAccessToken(context.Background(), provider, "refresh")
	metadata := requireOperationError(t, err, OperationRefresh, DetailProviderUnavailable)
	assert.Equal(t, http.StatusBadRequest, metadata.StatusCode())
	assert.ErrorIs(t, err, cause)
	var retrieveError *oauth2.RetrieveError
	require.ErrorAs(t, err, &retrieveError)
	assert.NotContains(t, err.Error(), cause.Error())
}
