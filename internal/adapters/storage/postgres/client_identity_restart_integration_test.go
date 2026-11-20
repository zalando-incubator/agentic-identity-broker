//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresClientIdentityPersistsAcrossAdapterRestartsAndRollback(t *testing.T) {
	connString, cleanup := setupMigratedConnString(t)
	defer cleanup()
	ctx := context.Background()
	withAdapter := func(run func(*Adapter)) {
		adapter, err := NewAdapter(testStorageConfig(connString))
		require.NoError(t, err)
		require.NoError(t, adapter.Initialize(ctx))
		defer func() { require.NoError(t, adapter.Close(ctx)) }()
		run(adapter)
	}
	var stored, filesystem *model.ThirdpartyOAuth2ProviderEntity
	var sessions []*storage.UserSession
	withAdapter(func(adapter *Adapter) {
		providers := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
		repo := NewUserSessionRepository(adapter)
		stored = createClientIdentityProvider(t, providers, false)
		filesystem = createClientIdentityProvider(t, providers, true)
		_, applied, changed, err := providers.RecordObservedClientIdentity(ctx, filesystem.ID, stored.ClientID, clientIdentityObservation(filesystem))
		require.NoError(t, err)
		require.True(t, applied)
		require.False(t, changed)
		for _, provider := range []*model.ThirdpartyOAuth2ProviderEntity{stored, filesystem} {
			fixture := provider.Copy()
			fixture.ClientID = stored.ClientID
			for _, legacy := range []bool{false, true} {
				session := createClientIdentitySession(t, repo, fixture)
				if legacy {
					_, err := adapter.db.ExecContext(ctx, `UPDATE user_sessions SET upstream_client_id = NULL WHERE id = $1`, session.ID)
					require.NoError(t, err)
				}
				persisted, err := repo.Get(ctx, session.ID)
				require.NoError(t, err)
				sessions = append(sessions, persisted)
			}
		}
		stored.ClientID = "reports-v2"
		changed, err = providers.Update(ctx, stored, &stored.Version)
		require.NoError(t, err)
		require.True(t, changed)
		_, applied, changed, err = providers.RecordObservedClientIdentity(ctx, filesystem.ID, "reports-v2", clientIdentityObservation(filesystem))
		require.NoError(t, err)
		require.True(t, applied)
		require.True(t, changed)
		_, err = adapter.db.ExecContext(ctx, `ALTER TABLE thirdparty_oauth2_services ADD COLUMN future_identity_metadata TEXT; ALTER TABLE user_sessions ADD COLUMN future_session_metadata TEXT;`)
		require.NoError(t, err)
	})

	for _, revision := range []int64{1, 2} {
		withAdapter(func(adapter *Adapter) {
			providers := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
			repo := NewUserSessionRepository(adapter)
			for _, serviceID := range []id.ServiceID{stored.ID, filesystem.ID} {
				provider, err := providers.Get(ctx, serviceID)
				require.NoError(t, err)
				assert.Equal(t, revision, provider.ClientIdentityRevision)
				expected := id.ClientID("reports-v2")
				if revision == 2 {
					expected = "client-123"
				}
				assert.Equal(t, new(expected), provider.ClientIdentity)
				if serviceID == stored.ID {
					ciphertext, err := provider.Secret.GetCiphertext()
					require.NoError(t, err)
					assert.Equal(t, testCiphertext, ciphertext)
				} else {
					assert.Equal(t, filesystem.Version, provider.Version)
					assert.Equal(t, filesystem.UpdatedAt, provider.UpdatedAt)
				}
				if revision == 1 {
					fixture := provider.Copy()
					fixture.ClientID = *provider.ClientIdentity
					sessions = append(sessions, createClientIdentitySession(t, repo, fixture))
					if provider.CredentialSource == model.CredentialSourceStored {
						provider.ClientID = "client-123"
						changed, err := providers.Update(ctx, provider, &provider.Version)
						require.NoError(t, err)
						require.True(t, changed)
						assert.Equal(t, int64(2), provider.ClientIdentityRevision)
					} else {
						state, applied, changed, err := providers.RecordObservedClientIdentity(ctx, provider.ID, "client-123", clientIdentityObservation(provider))
						require.NoError(t, err)
						require.True(t, applied)
						require.True(t, changed)
						assert.Equal(t, int64(2), state.Revision)
					}
				}
			}
			for _, original := range sessions {
				found, err := repo.FindByPrincipalAndService(ctx, original.Principal, original.ServiceID)
				require.NoError(t, err)
				require.NotNil(t, found)
				assert.Equal(t, original, found, "identity changes must not rewrite historical evidence or ciphertext")
				byID, err := repo.Get(ctx, original.ID)
				require.NoError(t, err)
				assert.Equal(t, original, byID)
				listed, err := repo.ListByPrincipal(ctx, original.Principal)
				require.NoError(t, err)
				require.Len(t, listed, 1)
				assert.Equal(t, original, listed[0])
				active, err := repo.ListActiveByPrincipal(ctx, original.Principal)
				require.NoError(t, err)
				require.Len(t, active, 1)
				assert.Equal(t, original, active[0])
				summaries, err := repo.ListSummariesByPrincipal(ctx, original.Principal)
				require.NoError(t, err)
				require.Len(t, summaries, 1)
				assert.Equal(t, original.UpstreamClientID, summaries[0].UpstreamClientID)
				assert.Equal(t, original.UpstreamClientRevision, summaries[0].UpstreamClientRevision)
				if found.UpstreamClientID != nil {
					*found.UpstreamClientID = "mutated read"
				}
				found.EncryptedAccessToken[0] ^= 0xff
			}
			expiring, err := repo.ListExpiringSessions(ctx, time.Now().Add(2*time.Hour), storage.SessionExpiryCursor{}, 100)
			require.NoError(t, err)
			require.Len(t, expiring, len(sessions))
			byID := make(map[id.SessionID]*storage.UserSession, len(expiring))
			for _, session := range expiring {
				byID[session.ID] = session
			}
			for _, original := range sessions {
				assert.Equal(t, original, byID[original.ID])
			}
		})
	}
}

func TestPostgresSessionClientIdentityLockAndCallbackCopies(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	providers := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := createClientIdentityProvider(t, providers, false)
	firstRepo := NewUserSessionRepository(adapter)
	secondAdapter, err := NewAdapter(adapter.config)
	require.NoError(t, err)
	require.NoError(t, secondAdapter.Initialize(ctx))
	defer func() { require.NoError(t, secondAdapter.Close(context.Background())) }()
	secondRepo := NewUserSessionRepository(secondAdapter)
	session := createClientIdentitySession(t, firstRepo, provider)
	provider.ClientID = "reports-v2"
	changed, err := providers.Update(ctx, provider, &provider.Version)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(1), provider.ClientIdentityRevision)
	type result struct {
		session *storage.UserSession
		err     error
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	firstDone, secondDone := make(chan result, 1), make(chan result, 1)
	go func() {
		updated, err := firstRepo.WithLockedSession(ctx, session.Principal, provider.ID, func(ctx context.Context, current *storage.UserSession) (bool, error) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			current.CallbackID = "winning-callback"
			current.UpstreamClientID = new(provider.ClientID)
			current.UpstreamClientRevision = provider.ClientIdentityRevision
			current.EncryptedAccessToken = []byte("callback-access-ciphertext")
			return true, nil
		})
		firstDone <- result{updated, err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var firstPID int
	require.NoError(t, adapter.db.GetContext(ctx, &firstPID, `SELECT pid FROM pg_stat_activity
		WHERE datname = current_database() AND state = 'idle in transaction'
		AND query LIKE '%FROM user_sessions%FOR UPDATE'`))
	type evidence struct {
		identity id.ClientID
		revision int64
		callback string
	}
	seen := make(chan evidence, 1)
	go func() {
		updated, err := secondRepo.WithLockedSession(ctx, session.Principal, provider.ID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			seen <- evidence{*current.UpstreamClientID, current.UpstreamClientRevision, current.CallbackID}
			*current.UpstreamClientID = "refresh-must-not-rebind"
			current.UpstreamClientRevision = 99
			current.EncryptedAccessToken = []byte("refreshed-access-ciphertext")
			return true, nil
		})
		secondDone <- result{updated, err}
	}()
	waitRefreshBlock(t, adapter, ctx, firstPID)
	close(release)
	first, second := <-firstDone, <-secondDone
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	assert.Equal(t, evidence{"reports-v2", 1, "winning-callback"}, <-seen)
	assert.Equal(t, new(id.ClientID("reports-v2")), second.session.UpstreamClientID)
	assert.Equal(t, int64(1), second.session.UpstreamClientRevision)
	*first.session.UpstreamClientID = "mutated callback result"
	second.session.EncryptedAccessToken[0] ^= 0xff
	loser := *session
	loser.ID = id.NewSessionID()
	loser.CallbackID = "losing-callback"
	loser.UpstreamClientID = new(id.ClientID("obsolete"))
	loser.UpstreamClientRevision = 100
	_, err = firstRepo.CreateIfAbsent(ctx, &loser)
	var sourceErr *model.CredentialSourceError
	require.ErrorAs(t, err, &sourceErr)
	require.Equal(t, model.CredentialSourceReasonIdentitySuperseded, sourceErr.Reason)
	loser.UpstreamClientID = new(provider.ClientID)
	loser.UpstreamClientRevision = provider.ClientIdentityRevision
	winner, err := firstRepo.CreateIfAbsent(ctx, &loser)
	require.NoError(t, err)
	assert.Equal(t, session.ID, winner.ID)
	assert.Equal(t, "winning-callback", winner.CallbackID)
	assert.Equal(t, new(id.ClientID("reports-v2")), winner.UpstreamClientID)
	assert.Equal(t, int64(1), winner.UpstreamClientRevision)
	assert.Equal(t, []byte("refreshed-access-ciphertext"), winner.EncryptedAccessToken)
}
