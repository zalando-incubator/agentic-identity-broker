package oauth2session_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/credentialfile"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionDisplayEncryption struct {
	ports.EncryptionPort
	decryptions int
}

func (e *sessionDisplayEncryption) Decrypt(ctx context.Context, value []byte, aad map[string]string) ([]byte, error) {
	e.decryptions++
	return e.EncryptionPort.Decrypt(ctx, value, aad)
}

type unavailableDisplayProviderRepository struct {
	ports.ThirdpartyOAuth2ProviderRepository
}

func (unavailableDisplayProviderRepository) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, errors.New("provider metadata unavailable")
}

func (unavailableDisplayProviderRepository) GetByIDs(context.Context, []id.ServiceID) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, errors.New("provider metadata unavailable")
}

func TestSessionSummaryIdentityState(t *testing.T) {
	for _, tc := range []struct {
		name           string
		filesystem     bool
		changed        bool
		rollback       bool
		outage         bool
		metadataOutage bool
		expired        bool
		rejected       bool
		wantLogin      bool
	}{
		{name: "stored unchanged"},
		{name: "stored changed with valid access", changed: true, wantLogin: true},
		{name: "stored rollback", changed: true, rollback: true, wantLogin: true},
		{name: "filesystem changed with valid access", filesystem: true, changed: true, wantLogin: true},
		{name: "filesystem unavailable with matching persisted evidence", filesystem: true, outage: true},
		{name: "filesystem unavailable with superseded persisted evidence", filesystem: true, changed: true, outage: true, wantLogin: true},
		{name: "provider metadata unavailable with valid access", metadataOutage: true, wantLogin: true},
		{name: "expired access remains refreshable", expired: true},
		{name: "recorded rejection remains unusable", rejected: true, wantLogin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			logger := slog.New(slog.DiscardHandler)
			encryption := &sessionDisplayEncryption{EncryptionPort: newTestEncryption(t)}
			providers := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
			sessions := memory.NewInMemoryUserSessionRepository()
			provider := createTestService(id.NewServiceID())
			provider.Secret = model.NewEncryptedSecret([]byte("private-provider-ciphertext"))
			initialID := provider.ClientID
			canonicalID := "reports"
			provider.CanonicalID = &canonicalID
			config := oauth2session.DefaultConfig()
			var binding ports.CredentialFileBinding
			if tc.filesystem {
				binding = credentialPair(t, initialID.String(), "private-file-secret")
				provider.CredentialSource = model.CredentialSourceFilesystem
				provider.ClientID = ""
				provider.Secret = model.NewAbsentSecret()
				config.CredentialFiles = map[string]ports.CredentialFileBinding{canonicalID: binding}
			}
			require.NoError(t, providers.Create(ctx, provider))
			observe := func(value id.ClientID) {
				t.Helper()
				state, applied, _, err := providers.RecordObservedClientIdentity(ctx, provider.ID, value, ports.ClientIdentityObservation{ServiceVersion: provider.Version, CanonicalID: canonicalID, Revision: provider.ClientIdentityRevision})
				require.NoError(t, err)
				require.True(t, applied)
				provider.ClientIdentity, provider.ClientIdentityRevision = state.Identity, state.Revision
			}
			if tc.filesystem {
				observe(initialID)
			}
			if tc.changed {
				if tc.filesystem {
					require.NoError(t, publishCredential(binding.ClientIDFile, "replacement-client"))
					if tc.outage {
						observe("replacement-client")
					}
				} else {
					provider.ClientID = "replacement-client"
					_, err := providers.Update(ctx, provider, nil)
					require.NoError(t, err)
				}
			}
			if tc.rollback {
				provider.ClientID = initialID
				_, err := providers.Update(ctx, provider, nil)
				require.NoError(t, err)
			}
			if tc.outage {
				require.NoError(t, os.Remove(binding.ClientIDFile))
			}
			expiry := time.Now().Add(time.Hour)
			if tc.expired {
				expiry = time.Now().Add(-time.Hour)
			}
			session := &storage.UserSession{
				ID: id.NewSessionID(), Principal: "alice@example.test", ServiceID: provider.ID,
				UpstreamClientID: &initialID, TokenType: "Bearer", CallbackID: "established",
				EncryptedAccessToken: []byte("private-access-ciphertext"), EncryptedRefreshToken: []byte("private-refresh-ciphertext"),
				AccessTokenExpiresAt: &expiry, InitiatedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}
			if tc.rejected {
				rejectedAt := time.Now()
				session.RefreshRejectedAt = &rejectedAt
			}
			_, err := sessions.CreateIfAbsent(ctx, session)
			require.NoError(t, err)
			var displayProviders ports.ThirdpartyOAuth2ProviderRepository = providers
			if tc.metadataOutage {
				displayProviders = unavailableDisplayProviderRepository{providers}
			}
			providerService := thirdparty.NewThirdpartyOAuth2ProviderService(displayProviders, encryption, newNoopBranchKeyManager(), nil, false, logger)
			service := oauth2session.NewOAuth2SessionService(providerService, sessions, sessions, memory.NewUserGrantRepository(), memory.NewAgentRepository(), encryption, &http.Client{}, nil, config, logger).
				WithCredentialFileReader(credentialfile.New()).WithClientIdentityRepository(providers)
			for _, surface := range []string{"list", "detail"} {
				t.Run(surface, func(t *testing.T) {
					var summary *storage.UserSessionSummary
					var response any
					if surface == "list" {
						listed, err := service.ListUserSessions(ctx, session.Principal)
						require.NoError(t, err)
						require.Len(t, listed, 1)
						summary, response = listed[0], listed
					} else {
						detail, err := service.GetSessionWithAgents(ctx, session.Principal, provider.ID)
						require.NoError(t, err)
						response = detail
						summary = detail.Session.UserSessionSummary
					}
					assert.Equal(t, session.ID, summary.ID)
					assert.Equal(t, tc.wantLogin, summary.RequiresReauthentication)
					assert.Equal(t, summary.RequiresReauthentication, summary.IsExpired)
					assert.Equal(t, tc.expired, summary.AccessTokenExpired)
					encoded, err := json.Marshal(response)
					require.NoError(t, err)
					for _, hidden := range []string{"upstream_client", "client_identity", "private-provider", "private-access", "private-refresh", "private-file"} {
						assert.NotContains(t, string(encoded), hidden)
					}
				})
			}
			assert.Zero(t, encryption.decryptions, "usability displays must not decrypt provider secrets or tokens")
			persisted, err := sessions.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, session, persisted)
		})
	}
}

func TestForceRefreshSummaryAfterIdentityReconnect(t *testing.T) {
	ctx := context.Background()
	h := newCredentialSessionHarness(t, nil)
	provider := h.register(t, model.CredentialSourceStored, "reports", "previous-client", "provider-secret")
	owner := id.Principal("alice@example.test")
	h.callback(t, owner, provider.ID, h.initiate(t, owner, provider.ID).StateToken)
	provider.ClientID = "current-client"
	provider.Secret = model.NewPlaintextSecret("replacement-secret")
	require.NoError(t, h.providers.Update(ctx, provider, nil))
	reconnected := h.callback(t, owner, provider.ID, h.initiate(t, owner, provider.ID).StateToken)
	require.Equal(t, int64(1), reconnected.UpstreamClientRevision)
	summary, err := h.service.ForceRefreshSession(ctx, owner, provider.ID)
	require.NoError(t, err)
	assert.False(t, summary.RequiresReauthentication)
	assert.Equal(t, summary.RequiresReauthentication, summary.IsExpired)
	assert.False(t, summary.AccessTokenExpired)
	assert.Equal(t, reconnected.ID, summary.ID)
	persisted, err := h.sessions.Get(ctx, reconnected.ID)
	require.NoError(t, err)
	assert.Equal(t, reconnected.UpstreamClientID, persisted.UpstreamClientID)
	assert.Equal(t, reconnected.UpstreamClientRevision, persisted.UpstreamClientRevision)
}
