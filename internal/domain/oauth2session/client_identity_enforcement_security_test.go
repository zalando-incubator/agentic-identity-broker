package oauth2session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var identityVariants = []string{"stored confidential", "public", "google", "cimd public", "cimd confidential", "dcr", "filesystem"}

type enforcementCredentialReader struct {
	err error
}

func (r enforcementCredentialReader) ReadClientID(string) (string, error) {
	return "identity-current-sentinel", r.err
}

func (r enforcementCredentialReader) ReadPair(string, string) (string, string, error) {
	return "identity-current-sentinel", "identity-secret-sentinel", r.err
}

type identityGrantRepository struct {
	ports.UserGrantRepository
}

func (identityGrantRepository) CountAgentsByPrincipalAndServiceID(context.Context, id.Principal, id.ServiceID) (int, error) {
	return 0, nil
}

func identityVariant(provider *model.ThirdpartyOAuth2ProviderEntity, variant string) {
	provider.CredentialSource = model.CredentialSourceStored
	provider.ClientID = "identity-current-sentinel"
	provider.ClientIdentity = identityPointer(provider.ClientID.String())
	provider.TokenEndpointAuthMethod = ""
	provider.Secret = model.NewPlaintextSecret("identity-secret-sentinel")
	switch variant {
	case "public", "cimd public":
		provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
		provider.Secret = model.NewAbsentSecret()
	case "cimd confidential":
		provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodPrivateKeyJWT
		provider.Secret = model.NewAbsentSecret()
	case "google":
		provider.Flavor = model.OAuth2FlavorGoogle
	case "dcr":
		resource := "https://resource.example/api"
		provider.Discovery = model.DiscoveryConfig{EnableDiscovery: true, ClientMethod: model.ClientBootstrapDCR, ResourceURL: &resource}
		provider.IssuerURI = "https://provider.example"
		provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodClientSecretPost
		provider.AuthorizationParams = map[string]string{"resource": resource}
	case "filesystem":
		canonical := "first"
		provider.CanonicalID = &canonical
		provider.CredentialSource = model.CredentialSourceFilesystem
		provider.ClientID = ""
		provider.Secret = model.NewAbsentSecret()
	}
	if strings.HasPrefix(variant, "cimd ") {
		provider.ClientRegistration = model.ClientRegistrationCIMD
	}
}

func newIdentityEnforcementFixture(t *testing.T, variant string) (*OAuth2SessionService, *model.ThirdpartyOAuth2ProviderEntity, *storage.UserSession, *backgroundStore, *backgroundLog) {
	t.Helper()
	logs := new(backgroundLog)
	service, provider, session := newOperationTestService(t, logs)
	identityVariant(provider, variant)
	provider.ClientIdentityRevision = 4
	service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(identityProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		return provider.Copy(), nil
	}}, operationEncryption{}, operationBranchKeyManager{}, nil, false, service.logger)
	session.UpstreamClientID = identityPointer("identity-current-sentinel")
	session.UpstreamClientRevision = 4
	store := newBackgroundStore(session)
	service.sessionRepo = operationSessionRepository{find: store.find}
	service.refreshRepo = operationRefreshRepository(store.withLocked)
	service.config.CredentialFiles = map[string]ports.CredentialFileBinding{"first": {ClientIDFile: "/identity/path-sentinel/id-basename-sentinel"}}
	service = NewOAuth2SessionService(service.providerService, service.sessionRepo, service.refreshRepo, identityGrantRepository{}, nil, service.encryption, service.httpClient, nil, service.config, service.logger)
	t.Cleanup(func() { assert.NoError(t, service.Close(context.Background())) })
	service.credentialFileReader = enforcementCredentialReader{}
	service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
		return ports.ClientIdentityState{Identity: identityPointer("identity-current-sentinel"), Revision: provider.ClientIdentityRevision}, true, false, nil
	}})
	service.discoveryTokenHTTPClient = service.httpClient
	service.cimdAssertionSigner = operationSigner(func(context.Context, id.ClientID, string) (string, error) { return "identity-assertion-sentinel", nil })
	return service, provider, session, store, logs
}

func assertIdentityFailureEvent(t *testing.T, logs *backgroundLog, serviceID id.ServiceID, operation string, reason model.CredentialSourceReason) {
	t.Helper()
	events := logs.events(t, "session.oauth2.credential_source_failed")
	require.Len(t, events, 1, "one handled denial must produce one source failure")
	event := events[0]
	level := "ERROR"
	if reason == model.CredentialSourceReasonIdentityMissing || reason == model.CredentialSourceReasonIdentityMismatch || reason == model.CredentialSourceReasonIdentitySuperseded {
		level = "WARN"
	}
	assert.Equal(t, level, event["level"])
	assert.Equal(t, serviceID.String(), event["service_id"])
	assert.Equal(t, operation, event["operation"])
	assert.Equal(t, "failed", event["outcome"])
	assert.Equal(t, string(reason), event["reason"])
	metadataOperation := "session_lookup"
	switch operation {
	case "refresh", "code_exchange":
		metadataOperation = operation
	case "authorization_initiation":
		metadataOperation = "code_exchange"
	}
	assert.Equal(t, map[string]any{
		"operation": metadataOperation, "failure_detail": "credential_source_unavailable",
		"error_kind": "infrastructure", "dependency": "credential_source",
		"http_status_code": float64(0), "oauth_error_code": "",
	}, event["oauth2_session"])
	assert.Empty(t, logs.events(t, "session.oauth2.refresh_failed"))
	failures := 0
	for _, line := range strings.Split(strings.TrimSpace(logs.text()), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if metadata, ok := record["oauth2_session"].(map[string]any); ok && metadata["failure_detail"] == "credential_source_unavailable" {
			failures++
		}
		for _, forbidden := range []string{"action", "err", "cause", "client_id", "upstream_client_id", "path"} {
			assert.NotContains(t, record, forbidden)
		}
	}
	assert.Equal(t, 1, failures, "returning helpers and wrappers must not log this denial again")
	for _, forbidden := range []string{"identity-current-sentinel", "identity-previous-sentinel", "identity-secret-sentinel", "identity-assertion-sentinel", "/identity/path-sentinel", "id-basename-sentinel", "old-access", "old-refresh"} {
		assert.NotContains(t, logs.text(), forbidden)
	}
}

func TestClientIdentityTokenAcquisitionDeniesBeforeDecryptOrBackgroundSubmission(t *testing.T) {
	for _, variant := range identityVariants {
		for _, expiry := range []string{"valid", "unknown", "expired"} {
			for _, reason := range []model.CredentialSourceReason{model.CredentialSourceReasonIdentityMissing, model.CredentialSourceReasonIdentityMismatch, model.CredentialSourceReasonIdentitySuperseded} {
				t.Run(fmt.Sprintf("%s/%s/%s", variant, expiry, reason), func(t *testing.T) {
					reader := backgroundMetrics(t)
					service, _, session, store, logs := newIdentityEnforcementFixture(t, variant)
					future := time.Now().Add(time.Minute)
					switch expiry {
					case "valid":
						session.AccessTokenExpiresAt = &future
					case "unknown":
						session.AccessTokenExpiresAt = nil
					}
					switch reason {
					case model.CredentialSourceReasonIdentityMissing:
						session.UpstreamClientID = nil
					case model.CredentialSourceReasonIdentityMismatch:
						session.UpstreamClientID = identityPointer("identity-previous-sentinel")
					case model.CredentialSourceReasonIdentitySuperseded:
						session.UpstreamClientRevision = 2
					}
					before, err := store.find(context.Background(), session.Principal, session.ServiceID)
					require.NoError(t, err)
					var decrypts, calls atomic.Int32
					service.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
						decrypts.Add(1)
						return data, nil
					}}
					service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
						calls.Add(1)
						return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
					})
					service.config.RefreshLookahead = 5 * time.Minute
					got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
					assert.Nil(t, got)
					assert.Empty(t, token)
					var source *model.CredentialSourceError
					require.ErrorAs(t, err, &source)
					assert.Equal(t, reason, source.Reason)
					require.NoError(t, service.Close(context.Background()))
					assert.Zero(t, decrypts.Load())
					assert.Zero(t, calls.Load())
					assertBackgroundMetrics(t, reader, map[string]int64{"proactive_refresh_triggered_total": 0, "proactive_refresh_dropped_total": 0, "proactive_refresh_failed_total": 0})
					after, err := store.find(context.Background(), session.Principal, session.ServiceID)
					require.NoError(t, err)
					assert.Equal(t, before, after)
					assertIdentityFailureEvent(t, logs, session.ServiceID, "token_acquisition", reason)
				})
			}
		}
	}
}

func TestClientIdentityTokenAcquisitionFailsClosedOnUnavailableIdentity(t *testing.T) {
	for _, expiry := range []string{"valid", "unknown", "expired"} {
		for _, reason := range []model.CredentialSourceReason{model.CredentialSourceReasonMissingBinding, model.CredentialSourceReasonGenerationChanged, model.CredentialSourceReasonNotFound, model.CredentialSourceReasonPermissionDenied, model.CredentialSourceReasonNotRegular, model.CredentialSourceReasonTooLarge, model.CredentialSourceReasonEmpty, model.CredentialSourceReasonReadFailed} {
			t.Run(expiry+"/"+string(reason), func(t *testing.T) {
				service, _, session, store, logs := newIdentityEnforcementFixture(t, "filesystem")
				switch expiry {
				case "valid":
					future := time.Now().Add(time.Hour)
					session.AccessTokenExpiresAt = &future
				case "unknown":
					session.AccessTokenExpiresAt = nil
				}
				service.credentialFileReader = enforcementCredentialReader{err: model.NewCredentialSourceError(reason)}
				decrypts := 0
				service.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
					decrypts++
					return data, nil
				}}
				before, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
				assert.Nil(t, got)
				assert.Empty(t, token)
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				assert.Equal(t, reason, source.Reason)
				assert.Zero(t, decrypts)
				after, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				assertIdentityFailureEvent(t, logs, session.ServiceID, "token_acquisition", reason)
			})
		}
	}
}

func TestClientIdentityLegacyTokenExceptionEndsAfterFirstChange(t *testing.T) {
	for _, variant := range identityVariants {
		for _, revision := range []int64{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/revision=%d", variant, revision), func(t *testing.T) {
				service, provider, session, store, _ := newIdentityEnforcementFixture(t, variant)
				provider.ClientIdentityRevision = revision
				session.UpstreamClientID = nil
				session.UpstreamClientRevision = 0
				session.AccessTokenExpiresAt = nil
				got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
				if revision == 0 && variant != "filesystem" {
					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, "old-access", token)
				} else {
					assert.Nil(t, got)
					assert.Empty(t, token)
					var source *model.CredentialSourceError
					require.ErrorAs(t, err, &source)
					assert.Equal(t, model.CredentialSourceReasonIdentityMissing, source.Reason)
				}
				persisted, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				assert.Nil(t, persisted.UpstreamClientID, "never infer legacy establishment evidence")
				assert.Zero(t, persisted.UpstreamClientRevision)
			})
		}
	}
}

func TestClientIdentityRecordedRejectionPrecedesIdentityRead(t *testing.T) {
	service, _, session, _, logs := newIdentityEnforcementFixture(t, "filesystem")
	now := time.Now()
	session.RefreshRejectedAt = &now
	session.AccessTokenExpiresAt = nil
	service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(identityProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		t.Error("recorded rejection must win without reading identity")
		return nil, errors.New("unavailable")
	}}, identityEncryption{}, identityBranchKeys{}, nil, false, service.logger)
	got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
	assert.Nil(t, got)
	assert.Empty(t, token)
	assert.ErrorIs(t, err, ErrRefreshRejected)
	assert.Empty(t, logs.events(t, "session.oauth2.credential_source_failed"))
}

func TestClientIdentityRefreshRechecksCurrentIdentityInsideSessionLock(t *testing.T) {
	for _, trigger := range []RefreshTrigger{RefreshTriggerOnDemand, RefreshTriggerBackground, RefreshTriggerSweep} {
		for _, mutation := range []string{"change", "rollback"} {
			t.Run(string(trigger)+"/"+mutation, func(t *testing.T) {
				service, provider, session, store, _ := newIdentityEnforcementFixture(t, "public")
				before, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				service.refreshRepo = operationRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, update func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
					provider.ClientIdentityRevision++
					if mutation == "change" {
						provider.ClientID = "identity-previous-sentinel"
					}
					return store.withLocked(ctx, principal, serviceID, func(ctx context.Context, current *storage.UserSession) (bool, error) {
						changed, err := update(ctx, current)
						assert.False(t, changed, "denial must not request persistence")
						return changed, err
					})
				})
				decrypts, calls := 0, 0
				service.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
					decrypts++
					return data, nil
				}}
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
				})
				_, refreshed, _, err := service.refreshDueSession(context.Background(), session.Principal, session.ServiceID, time.Now, trigger)
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				want := model.CredentialSourceReasonIdentitySuperseded
				if mutation == "change" {
					want = model.CredentialSourceReasonIdentityMismatch
				}
				assert.Equal(t, want, source.Reason)
				assert.False(t, refreshed)
				assert.Zero(t, decrypts)
				assert.Zero(t, calls)
				after, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
			})
		}
	}
}

func TestClientIdentityInFlightRefreshCannotReleasePreviousRevision(t *testing.T) {
	for _, response := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "success", status: 200, body: `{"access_token":"previous-revision-result","refresh_token":"previous-revision-refresh","expires_in":3600}`},
		{name: "rejection", status: 400, body: `{"error":"invalid_grant"}`},
	} {
		t.Run(response.name, func(t *testing.T) {
			service, provider, session, store, _ := newIdentityEnforcementFixture(t, "public")
			before, err := store.find(context.Background(), session.Principal, session.ServiceID)
			require.NoError(t, err)
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				provider.ClientIdentityRevision += 2
				return operationTokenResponse(response.status, response.body), nil
			})
			got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
			assert.Nil(t, got)
			assert.Empty(t, token)
			var source *model.CredentialSourceError
			require.ErrorAs(t, err, &source)
			assert.Equal(t, model.CredentialSourceReasonIdentitySuperseded, source.Reason)
			persisted, err := store.find(context.Background(), session.Principal, session.ServiceID)
			require.NoError(t, err)
			assert.Equal(t, before, persisted)
			got, token, err = service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
			assert.Nil(t, got)
			assert.Empty(t, token)
			assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
		})
	}
}

func TestClientIdentityMatchingFastPathUsesOneMetadataReadWithoutSecretDecryption(t *testing.T) {
	for _, variant := range identityVariants {
		for _, unknownExpiry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/unknown=%t", variant, unknownExpiry), func(t *testing.T) {
				service, provider, session, _, _ := newIdentityEnforcementFixture(t, variant)
				future := time.Now().Add(time.Hour)
				session.AccessTokenExpiresAt = &future
				if unknownExpiry {
					session.AccessTokenExpiresAt = nil
				}
				gets, secrets, tokenDecrypts, fileReads, observations := 0, 0, 0, 0, 0
				service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(identityProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
					gets++
					copy := provider.Copy()
					if !copy.Secret.IsAbsent() {
						copy.Secret = model.NewEncryptedSecret([]byte("identity-ciphertext-sentinel"))
					}
					return copy, nil
				}}, operationEncryption{decrypt: func(context.Context, []byte, map[string]string) ([]byte, error) {
					secrets++
					return []byte("identity-secret-sentinel"), nil
				}}, operationBranchKeyManager{}, nil, false, service.logger)
				service.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
					tokenDecrypts++
					return data, nil
				}}
				service.credentialFileReader = identityReader{read: func(string) (string, error) { fileReads++; return "identity-current-sentinel", nil }}
				service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
					observations++
					return ports.ClientIdentityState{Identity: identityPointer("identity-current-sentinel"), Revision: 4}, true, false, nil
				}})
				got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "old-access", token)
				assert.Equal(t, 1, gets)
				assert.Zero(t, secrets)
				assert.Equal(t, 1, tokenDecrypts)
				if variant == "filesystem" {
					assert.Equal(t, 1, fileReads)
					assert.Equal(t, 1, observations)
				} else {
					assert.Zero(t, fileReads)
					assert.Zero(t, observations)
				}
			})
		}
	}
}

func TestClientIdentityTokenAcquisitionFailsClosedWhenServiceMetadataIsUnavailable(t *testing.T) {
	for _, unknownExpiry := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown=%t", unknownExpiry), func(t *testing.T) {
			service, _, session, store, logs := newIdentityEnforcementFixture(t, "public")
			future := time.Now().Add(time.Hour)
			session.AccessTokenExpiresAt = &future
			if unknownExpiry {
				session.AccessTokenExpiresAt = nil
			}
			service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(identityProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return nil, storage.NewStorageError("Get", storage.ErrorKindTimeout, errors.New("identity-private-storage-cause"), "identity metadata unavailable")
			}}, identityEncryption{}, identityBranchKeys{}, nil, false, service.logger)
			decrypts := 0
			service.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
				decrypts++
				return data, nil
			}}
			got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
			assert.Nil(t, got)
			assert.Empty(t, token)
			assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
			assert.Zero(t, decrypts)
			persisted, findErr := store.find(context.Background(), session.Principal, session.ServiceID)
			require.NoError(t, findErr)
			assert.Equal(t, session, persisted)
			assertIdentityFailureEvent(t, logs, session.ServiceID, "token_acquisition", model.CredentialSourceReasonReadFailed)
			assert.NotContains(t, logs.text(), "identity-private-storage-cause")
		})
	}
}
