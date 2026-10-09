package oauth2session_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func credentialState(t *testing.T, service *oauth2session.OAuth2SessionService, principal id.Principal, serviceID id.ServiceID, established *id.ClientID) string {
	t.Helper()
	now := time.Now()
	state, err := service.CreateStateToken(&oauth2session.OAuth2StateTokenClaims{Principal: principal, ServiceID: serviceID, UpstreamClientID: established, PKCEVerifier: "credential-test-verifier", RedirectURI: "https://broker.example.com/sessions", IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
	require.NoError(t, err)
	return state
}

func (h *credentialSessionHarness) transition(t *testing.T, serviceID id.ServiceID, source model.CredentialSource, clientID, secret string) {
	t.Helper()
	provider, err := h.providers.Get(context.Background(), serviceID)
	require.NoError(t, err)
	provider.CredentialSource = source
	provider.CredentialSourceProvided = true
	provider.ClientID = ""
	provider.Secret = model.NewAbsentSecret()
	provider.ClientIDProvided = false
	provider.ClientSecretProvided = false
	if source == model.CredentialSourceStored {
		provider.ClientID = id.ClientID(clientID)
		provider.Secret = model.NewPlaintextSecret(secret)
		provider.ClientIDProvided = true
		provider.ClientSecretProvided = true
	}
	version := provider.Version
	require.NoError(t, h.providers.Update(context.Background(), provider, &version))
}

func TestCredentialIdentity_RecordedIdentityIsEnforcedInBothSources(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			t.Run(string(source)+"/"+operation, func(t *testing.T) {
				binding := credentialPair(t, "client-B", "secret-B")
				h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"identity-service": binding})
				clientID, secret := "client-B", "secret-B"
				if source == model.CredentialSourceFilesystem {
					clientID, secret = "", ""
				}
				provider := h.register(t, source, "identity-service", clientID, secret)
				principal := id.Principal("identity@example.com")
				flow := h.initiate(t, principal, provider.ID)
				claims, err := h.service.ValidateStateToken(flow.StateToken, principal, provider.ID)
				require.NoError(t, err)
				require.NotNil(t, claims.UpstreamClientID)
				assert.Equal(t, id.ClientID("client-B"), *claims.UpstreamClientID)
				established := id.ClientID("client-A")
				before, err := h.records.Get(context.Background(), provider.ID)
				require.NoError(t, err)
				if operation == "callback" {
					state := credentialState(t, h.service, principal, provider.ID, &established)
					_, err = h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "client-A-code", State: state})
				} else {
					session := h.seedSession(t, principal, provider.ID, &established)
					originalAccess := append([]byte(nil), session.EncryptedAccessToken...)
					originalRefresh := append([]byte(nil), session.EncryptedRefreshToken...)
					if operation == "explicit refresh" {
						_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
					} else {
						_, _, err = h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
					}
					persisted, findErr := h.sessions.Get(context.Background(), session.ID)
					require.NoError(t, findErr)
					assert.Equal(t, &established, persisted.UpstreamClientID)
					assert.Equal(t, originalAccess, persisted.EncryptedAccessToken)
					assert.Equal(t, originalRefresh, persisted.EncryptedRefreshToken)
				}
				assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
				assert.Empty(t, h.endpoint.captured(t))
				after, getErr := h.records.Get(context.Background(), provider.ID)
				require.NoError(t, getErr)
				assert.Equal(t, before, after)
				if source == model.CredentialSourceStored {
					ids, pairs := h.reader.counts()
					assert.Zero(t, ids)
					assert.Zero(t, pairs)
				}
			})
		}
	}
}

func TestCredentialIdentity_SourceTransitionsPreserveAndCheckEstablishedIdentity(t *testing.T) {
	for _, initial := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		for _, identity := range []string{"matching", "mismatching", "missing"} {
			for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
				t.Run(string(initial)+"/"+identity+"/"+operation, func(t *testing.T) {
					selectedID := "client-A"
					if identity == "mismatching" {
						selectedID = "client-B"
					}
					binding := credentialPair(t, "client-A", "file-secret-before")
					h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"transition-service": binding})
					clientID, secret := "client-A", "stored-secret-before"
					if initial == model.CredentialSourceFilesystem {
						clientID, secret = "", ""
					}
					provider := h.register(t, initial, "transition-service", clientID, secret)
					principal := id.Principal("transition@example.com")
					var established *id.ClientID
					if identity != "missing" {
						value := id.ClientID("client-A")
						established = &value
					}
					var state string
					if operation == "callback" {
						if established == nil {
							state = credentialState(t, h.service, principal, provider.ID, nil)
						} else {
							state = h.initiate(t, principal, provider.ID).StateToken
						}
					} else {
						h.seedSession(t, principal, provider.ID, established)
					}
					finalSource, finalSecret := model.CredentialSourceFilesystem, "file-secret-after"
					if initial == model.CredentialSourceStored {
						require.NoError(t, publishCredential(binding.ClientIDFile, selectedID))
						require.NoError(t, publishCredential(binding.ClientSecretFile, finalSecret))
						h.transition(t, provider.ID, finalSource, "", "")
					} else {
						finalSource, finalSecret = model.CredentialSourceStored, "explicit-stored-secret"
						h.transition(t, provider.ID, finalSource, selectedID, finalSecret)
					}
					transitioned, err := h.records.Get(context.Background(), provider.ID)
					require.NoError(t, err)
					assert.True(t, transitioned.CredentialSourceTransitioned)
					var callbackResult *oauth2session.HandleCallbackResult
					switch operation {
					case "callback":
						callbackResult, err = h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "before-transition-code", State: state})
					case "explicit refresh":
						_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
					default:
						_, _, err = h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
					}
					if identity == "matching" {
						require.NoError(t, err)
						requests := h.endpoint.captured(t)
						require.Len(t, requests, 1)
						assert.Equal(t, selectedID, requests[0].clientID)
						assert.Equal(t, finalSecret, requests[0].secret)
						if callbackResult != nil {
							assert.Equal(t, established, callbackResult.Session.UpstreamClientID)
						}
					} else {
						reason := model.CredentialSourceReasonIdentityMismatch
						if identity == "missing" {
							reason = model.CredentialSourceReasonIdentityMissing
						}
						assertCredentialSourceReason(t, err, reason)
						assert.Empty(t, h.endpoint.captured(t))
					}
					if operation != "callback" {
						session, findErr := h.sessions.FindByPrincipalAndService(context.Background(), principal, provider.ID)
						require.NoError(t, findErr)
						require.NotNil(t, session)
						assert.Equal(t, established, session.UpstreamClientID, "source changes and refresh must not infer or replace identity")
					}
					unchanged, getErr := h.records.Get(context.Background(), provider.ID)
					require.NoError(t, getErr)
					assert.Equal(t, transitioned, unchanged)
				})
			}
		}
	}
}

func TestCredentialIdentity_LegacyAbsenceAllowedOnlyForNeverTransitionedStored(t *testing.T) {
	for _, history := range []string{"never transitioned stored", "filesystem", "stored filesystem stored round trip"} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			t.Run(history+"/"+operation, func(t *testing.T) {
				binding := credentialPair(t, "legacy-client", "file-secret")
				h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"legacy-service": binding})
				source, clientID, secret := model.CredentialSourceStored, "legacy-client", "stored-secret"
				if history == "filesystem" {
					source, clientID, secret = model.CredentialSourceFilesystem, "", ""
				}
				provider := h.register(t, source, "legacy-service", clientID, secret)
				principal := id.Principal("legacy@example.com")
				state := credentialState(t, h.service, principal, provider.ID, nil)
				if operation != "callback" {
					h.seedSession(t, principal, provider.ID, nil)
				}
				if history == "stored filesystem stored round trip" {
					h.transition(t, provider.ID, model.CredentialSourceFilesystem, "", "")
					h.transition(t, provider.ID, model.CredentialSourceStored, "legacy-client", "explicit-return-secret")
				}
				var err error
				switch operation {
				case "callback":
					var result *oauth2session.HandleCallbackResult
					result, err = h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "legacy-code", State: state})
					if history == "never transitioned stored" {
						require.NoError(t, err)
						require.NotNil(t, result)
						assert.Nil(t, result.Session.UpstreamClientID, "a legacy state is not evidence of an established client identity")
					}
				case "explicit refresh":
					_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
				default:
					_, _, err = h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
				}
				if history == "never transitioned stored" {
					require.NoError(t, err)
					requests := h.endpoint.captured(t)
					require.Len(t, requests, 1)
					assert.Equal(t, "legacy-client", requests[0].clientID)
					assert.Equal(t, "stored-secret", requests[0].secret)
				} else {
					assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMissing)
					assert.Empty(t, h.endpoint.captured(t))
				}
				if operation != "callback" {
					session, findErr := h.sessions.FindByPrincipalAndService(context.Background(), principal, provider.ID)
					require.NoError(t, findErr)
					assert.Nil(t, session.UpstreamClientID)
				}
			})
		}
	}
}

func TestCredentialIdentity_FileClientChangeBlocksOldContextsAndNewConnectionReplacesSessionIdentity(t *testing.T) {
	binding := credentialPair(t, "client-A", "secret-A")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"rotating-identity": binding})
	provider := h.register(t, model.CredentialSourceFilesystem, "rotating-identity", "", "")
	principal := id.Principal("reconnect@example.com")
	oldFlow := h.initiate(t, principal, provider.ID)
	oldSession := h.callback(t, principal, provider.ID, oldFlow.StateToken)
	oldSessionID := oldSession.ID
	require.NoError(t, publishCredential(binding.ClientIDFile, "client-B"))
	require.NoError(t, publishCredential(binding.ClientSecretFile, "secret-B"))
	_, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "old-A-code", State: oldFlow.StateToken})
	assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
	_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
	assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
	h.expire(t, principal, provider.ID)
	_, _, err = h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
	assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
	require.Len(t, h.endpoint.captured(t), 1, "old contexts must produce no additional provider authentication")
	newFlow := h.initiate(t, principal, provider.ID)
	claims, err := h.service.ValidateStateToken(newFlow.StateToken, principal, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, claims.UpstreamClientID)
	assert.Equal(t, id.ClientID("client-B"), *claims.UpstreamClientID)
	newSession := h.callback(t, principal, provider.ID, newFlow.StateToken)
	assert.Equal(t, oldSessionID, newSession.ID)
	assert.Equal(t, claims.UpstreamClientID, newSession.UpstreamClientID)
	persisted, err := h.sessions.Get(context.Background(), oldSessionID)
	require.NoError(t, err)
	assert.Equal(t, claims.UpstreamClientID, persisted.UpstreamClientID)
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 2)
	assert.Equal(t, "client-A", requests[0].clientID)
	assert.Equal(t, "client-B", requests[1].clientID)
	assert.Equal(t, "secret-B", requests[1].secret)
}

// emptyIdentityRefreshRepository injects a malformed record at the storage read boundary.
// Normal writes must continue to reject empty recorded identities.
type emptyIdentityRefreshRepository struct {
	ports.UserSessionRefreshRepository
}

func (r emptyIdentityRefreshRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	return r.UserSessionRefreshRepository.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
		invalid := *session
		empty := id.ClientID("")
		invalid.UpstreamClientID = &empty
		return refresh(ctx, &invalid)
	})
}

func TestCredentialIdentity_EmptyRecordedIdentityIsNotLegacyAbsence(t *testing.T) {
	// Match setupServiceWithConfig's JWE key, but encrypt the malformed claims directly.
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))
	jweService := domjwe.New(key)
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(source), func(t *testing.T) {
			binding := credentialPair(t, "current-client", "file-secret")
			h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"empty-identity": binding})
			clientID, secret := "current-client", "stored-secret"
			if source == model.CredentialSourceFilesystem {
				clientID, secret = "", ""
			}
			provider := h.register(t, source, "empty-identity", clientID, secret)
			principal := id.Principal("empty-identity@example.com")
			empty := id.ClientID("")
			now := time.Now()
			claims := &oauth2session.OAuth2StateTokenClaims{Principal: principal, ServiceID: provider.ID, UpstreamClientID: &empty, PKCEVerifier: "credential-test-verifier", RedirectURI: "https://broker.example.com/sessions", IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
			_, err := h.service.CreateStateToken(claims)
			require.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
			state, err := jweService.Encrypt(claims)
			require.NoError(t, err)
			_, err = h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "invalid-identity-code", State: state})
			assertCredentialSourceNotProviderFailure(t, err, model.CredentialSourceReasonIdentityMissing)
			assertOperationMetadata(t, err, oauth2session.OperationCodeExchange, oauth2session.DetailCredentialSourceUnavailable)
			h.seedSession(t, principal, provider.ID, nil)
			config := oauth2session.DefaultConfig()
			config.CallbackBaseURL = "https://broker.example.com"
			config.CredentialFiles = map[string]ports.CredentialFileBinding{"empty-identity": binding}
			h.service = oauth2session.NewOAuth2SessionService(h.providers, h.sessions,
				emptyIdentityRefreshRepository{h.sessions.(ports.UserSessionRefreshRepository)},
				memory.NewUserGrantRepository(), memory.NewAgentRepository(), newTestEncryption(t),
				h.endpoint.server.Client(), jweService, config, slog.Default()).WithCredentialFileReader(h.reader)
			_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
			assertCredentialSourceNotProviderFailure(t, err, model.CredentialSourceReasonIdentityMissing)
			assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailCredentialSourceUnavailable)
			assert.Empty(t, h.endpoint.captured(t))
		})
	}
}
