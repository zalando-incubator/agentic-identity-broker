package storagecontracts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func SessionClientIdentityRevisionFence(t *testing.T, providers ports.ThirdpartyOAuth2ProviderRepository, identities ports.ThirdpartyOAuth2ClientIdentityRepository, sessions ports.UserSessionRepository, refresh ports.UserSessionRefreshRepository) {
	t.Helper()
	for _, filesystem := range []bool{false, true} {
		for _, reconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("filesystem=%t/reconnect=%t", filesystem, reconnect), func(t *testing.T) {
				ctx := context.Background()
				serviceID := id.NewServiceID()
				provider := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: new("reports-" + serviceID.String()), DisplayName: "Reports", ClientID: "reports-v1", Secret: model.NewEncryptedSecret([]byte("ciphertext"))}
				if filesystem {
					provider.CredentialSource = model.CredentialSourceFilesystem
					provider.ClientID = ""
					provider.Secret = model.NewAbsentSecret()
				}
				require.NoError(t, providers.Create(ctx, provider))
				advance := func(values ...id.ClientID) {
					for _, value := range values {
						if filesystem {
							_, applied, _, err := identities.RecordObservedClientIdentity(ctx, serviceID, value, ports.ClientIdentityObservation{ServiceVersion: provider.Version, CanonicalID: *provider.CanonicalID, Revision: provider.ClientIdentityRevision})
							require.NoError(t, err)
							require.True(t, applied)
						} else {
							provider.ClientID = value
							_, err := providers.Update(ctx, provider, &provider.Version)
							require.NoError(t, err)
						}
						var err error
						provider, err = providers.Get(ctx, serviceID)
						require.NoError(t, err)
					}
				}
				advance("reports-v1")
				now := time.Now().UTC().Truncate(time.Microsecond)
				candidate := &storage.UserSession{
					ID: id.NewSessionID(), Principal: "callbacks@example.com", ServiceID: serviceID, CallbackID: "baseline",
					UpstreamClientID: new(id.ClientID("reports-v1")), UpstreamClientRevision: provider.ClientIdentityRevision,
					EncryptedAccessToken: []byte("old-access"), TokenType: "Bearer", Scope: []string{},
					EncryptionContext: storage.EncryptionContext{ServiceID: serviceID}, InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
				}
				if reconnect {
					require.NoError(t, sessions.Create(ctx, candidate))
				}
				before, err := sessions.FindByPrincipalAndService(ctx, candidate.Principal, serviceID)
				require.NoError(t, err)
				var result *storage.UserSession
				if reconnect {
					result, err = refresh.WithLockedSession(ctx, candidate.Principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
						advance("reports-v2", "reports-v1")
						current.CallbackID = "obsolete-callback"
						current.EncryptedAccessToken = []byte("obsolete-access")
						return true, nil
					})
				} else {
					advance("reports-v2", "reports-v1")
					result, err = sessions.CreateIfAbsent(ctx, candidate)
				}
				assert.Nil(t, result)
				var conflict *storage.StorageError
				require.ErrorAs(t, err, &conflict)
				assert.Equal(t, storage.ErrorKindConflict, conflict.Kind)
				assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				after, err := sessions.FindByPrincipalAndService(ctx, candidate.Principal, serviceID)
				require.NoError(t, err)
				assert.Equal(t, before, after, "obsolete work must not consume the current authorization's baseline")
				require.EqualValues(t, 2, provider.ClientIdentityRevision)
				if reconnect {
					result, err = refresh.WithLockedSession(ctx, candidate.Principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
						require.Equal(t, before.CallbackID, current.CallbackID)
						current.CallbackID = "current-callback"
						current.UpstreamClientRevision = provider.ClientIdentityRevision
						current.EncryptedAccessToken = []byte("current-access")
						return true, nil
					})
				} else {
					candidate.UpstreamClientRevision = provider.ClientIdentityRevision
					candidate.EncryptedAccessToken = []byte("current-access")
					result, err = sessions.CreateIfAbsent(ctx, candidate)
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, provider.ClientIdentityRevision, result.UpstreamClientRevision)
				assert.Equal(t, []byte("current-access"), result.EncryptedAccessToken)
			})
		}
	}
}
