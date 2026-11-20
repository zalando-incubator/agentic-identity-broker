//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The AFTER trigger holds an already-mutated row until the test releases the advisory lock.
// This exposes PostgreSQL's blocked-reader ordering without sleeps or production test hooks.
func blockClientIdentityWriteCommit(t *testing.T, adapter *Adapter, ctx context.Context) (*sqlx.Tx, int) {
	t.Helper()
	gate, err := adapter.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = gate.Rollback() })
	var pid int
	require.NoError(t, gate.GetContext(ctx, &pid, `SELECT pg_backend_pid()`))
	_, err = gate.ExecContext(ctx, `SELECT pg_advisory_xact_lock(56024)`)
	require.NoError(t, err)
	_, err = adapter.db.ExecContext(ctx, `
		CREATE FUNCTION pause_client_identity_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			PERFORM pg_advisory_xact_lock(56024);
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER pause_client_identity_commit AFTER UPDATE ON thirdparty_oauth2_services
		FOR EACH ROW EXECUTE FUNCTION pause_client_identity_commit();`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = gate.Rollback()
		_, err := adapter.db.ExecContext(context.Background(), `DROP TRIGGER pause_client_identity_commit ON thirdparty_oauth2_services; DROP FUNCTION pause_client_identity_commit();`)
		require.NoError(t, err)
	})
	return gate, pid
}

func TestPostgresClientIdentityUpdateUsesLockedObservation(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	t.Cleanup(cleanup)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := createClientIdentityProvider(t, repo, true)
	_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "reports-v1", clientIdentityObservation(provider))
	require.NoError(t, err)
	require.True(t, applied)
	require.False(t, changed)
	stale, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	stale.DisplayName = "Metadata from an earlier read"
	gate, gatePID := blockClientIdentityWriteCommit(t, adapter, ctx)
	type observationResult struct {
		state            ports.ClientIdentityState
		applied, changed bool
		err              error
	}
	observed := make(chan observationResult, 1)
	go func() {
		state, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "reports-v2", clientIdentityObservation(provider))
		observed <- observationResult{state, applied, changed, err}
	}()
	observerPID := waitRefreshBlock(t, adapter, ctx, gatePID)
	type updateResult struct {
		changed bool
		err     error
	}
	updated := make(chan updateResult, 1)
	go func() {
		changed, err := repo.Update(ctx, stale, &stale.Version)
		updated <- updateResult{changed, err}
	}()
	waitRefreshBlock(t, adapter, ctx, observerPID)
	require.NoError(t, gate.Commit())
	observation := <-observed
	require.NoError(t, observation.err)
	require.True(t, observation.applied)
	require.True(t, observation.changed)
	require.Equal(t, int64(1), observation.state.Revision)
	update := <-updated
	require.NoError(t, update.err)
	assert.False(t, update.changed)
	assert.Equal(t, observation.state.Identity, stale.ClientIdentity)
	assert.Equal(t, observation.state.Revision, stale.ClientIdentityRevision)
	assert.Equal(t, provider.Version+1, stale.Version)
	found, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, stale, found)
}

func TestPostgresClientIdentityObservationRechecksConcurrentRevision(t *testing.T) {
	for _, secondIdentity := range []id.ClientID{"reports-v2", "reports-v3"} {
		t.Run(secondIdentity.String(), func(t *testing.T) {
			adapter, cleanup := setupMigratedAdapter(t)
			t.Cleanup(cleanup)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
			provider := createClientIdentityProvider(t, repo, true)
			_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "reports-v1", clientIdentityObservation(provider))
			require.NoError(t, err)
			require.True(t, applied)
			require.False(t, changed)
			before, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			gate, gatePID := blockClientIdentityWriteCommit(t, adapter, ctx)
			type result struct {
				state            ports.ClientIdentityState
				applied, changed bool
				err              error
			}
			first, second := make(chan result, 1), make(chan result, 1)
			go func() {
				state, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "reports-v2", clientIdentityObservation(provider))
				first <- result{state, applied, changed, err}
			}()
			firstPID := waitRefreshBlock(t, adapter, ctx, gatePID)
			go func() {
				state, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, secondIdentity, clientIdentityObservation(provider))
				second <- result{state, applied, changed, err}
			}()
			waitRefreshBlock(t, adapter, ctx, firstPID)
			require.NoError(t, gate.Commit())
			winner, loser := <-first, <-second
			require.NoError(t, winner.err)
			require.True(t, winner.applied)
			require.True(t, winner.changed)
			require.Equal(t, int64(1), winner.state.Revision)
			require.NoError(t, loser.err)
			assert.False(t, loser.applied, "a blocked observation must recheck the winning revision")
			assert.False(t, loser.changed)
			persisted, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, new(id.ClientID("reports-v2")), persisted.ClientIdentity)
			assert.Equal(t, int64(1), persisted.ClientIdentityRevision)
			assert.Equal(t, before.Version, persisted.Version)
			assert.Equal(t, before.UpdatedAt, persisted.UpdatedAt)
		})
	}
}

func TestPostgresClientIdentityObservationRechecksConcurrentConfiguration(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	t.Cleanup(cleanup)
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	for _, observation := range []string{"initial", "same", "different"} {
		for _, mutation := range []string{"source", "canonical binding", "metadata version", "source round trip"} {
			t.Run(observation+"/"+mutation, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				provider := createClientIdentityProvider(t, repo, true)
				if observation != "initial" {
					_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "reports-v1", clientIdentityObservation(provider))
					require.NoError(t, err)
					require.True(t, applied)
					require.False(t, changed)
				}
				before, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				candidate := before.Copy()
				switch mutation {
				case "source", "source round trip":
					candidate.CredentialSource = model.CredentialSourceStored
					candidate.ClientID = "reports-v1"
					candidate.Secret = model.NewEncryptedSecret([]byte("stored-secret-ciphertext"))
					if mutation == "source round trip" {
						changed, err := repo.Update(ctx, candidate, &candidate.Version)
						require.NoError(t, err)
						require.False(t, changed)
						candidate.CredentialSource = model.CredentialSourceFilesystem
						candidate.ClientID = ""
						candidate.Secret = model.NewAbsentSecret()
					}
				case "canonical binding":
					candidate.CanonicalID = new("replacement-" + provider.ID.String())
				case "metadata version":
					candidate.DisplayName = "Replacement metadata"
				}
				gate, gatePID := blockClientIdentityWriteCommit(t, adapter, ctx)
				type updateResult struct {
					changed bool
					err     error
				}
				updated := make(chan updateResult, 1)
				go func() {
					changed, err := repo.Update(ctx, candidate, &candidate.Version)
					updated <- updateResult{changed, err}
				}()
				writerPID := waitRefreshBlock(t, adapter, ctx, gatePID)
				type observationResult struct {
					applied, changed bool
					err              error
				}
				observed := make(chan observationResult, 1)
				value := id.ClientID("reports-v1")
				if observation == "different" {
					value = "obsolete-file-value"
				}
				go func() {
					_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, value, clientIdentityObservation(before))
					observed <- observationResult{applied, changed, err}
				}()
				if mutation != "source round trip" {
					waitRefreshBlock(t, adapter, ctx, writerPID)
				}
				require.NoError(t, gate.Commit())
				update := <-updated
				require.NoError(t, update.err)
				require.False(t, update.changed)
				result := <-observed
				require.NoError(t, result.err)
				assert.False(t, result.applied)
				assert.False(t, result.changed)
				found, err := repo.Get(ctx, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, candidate, found)
				assert.Zero(t, found.ClientIdentityRevision)
				found.CredentialSource = model.CredentialSourceStored
				found.ClientID = "reports-v1"
				found.Secret = model.NewEncryptedSecret([]byte("rotated-secret-ciphertext"))
				changed, err := repo.Update(ctx, found, &found.Version)
				require.NoError(t, err)
				assert.False(t, changed, "discarded file values cannot invalidate a later same-ID update")
				assert.Zero(t, found.ClientIdentityRevision)
			})
		}
	}
}

func TestPostgresClientIdentityConcurrentUpdatePreconditions(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	t.Cleanup(cleanup)
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	for _, precondition := range []string{"version", "discovery attempt"} {
		t.Run(precondition, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var provider *model.ThirdpartyOAuth2ProviderEntity
			if precondition == "version" {
				provider = createClientIdentityProvider(t, repo, false)
			} else {
				provider = discoveredCIMDProvider(id.NewServiceID(), "https://reports.example.com/"+id.NewServiceID().String(), time.Now().UTC().Truncate(time.Microsecond))
				require.NoError(t, repo.Create(ctx, provider))
			}
			first, second := provider.Copy(), provider.Copy()
			first.ClientID = id.ClientID("https://newer.example.test/.well-known/oauth-client/" + provider.ID.String())
			second.ClientID = id.ClientID("https://older.example.test/.well-known/oauth-client/" + provider.ID.String())
			expectedVersion := &provider.Version
			if precondition == "discovery attempt" {
				expectedVersion = nil
				first.DiscoveryStatus.LastAttemptAt = new(provider.DiscoveryStatus.LastAttemptAt.Add(2 * time.Second))
				second.DiscoveryStatus.LastAttemptAt = new(provider.DiscoveryStatus.LastAttemptAt.Add(time.Second))
				first.DiscoveryStatus.LastSuccessAt = first.DiscoveryStatus.LastAttemptAt
				second.DiscoveryStatus.LastSuccessAt = second.DiscoveryStatus.LastAttemptAt
			}
			gate, gatePID := blockClientIdentityWriteCommit(t, adapter, ctx)
			type result struct {
				changed bool
				err     error
			}
			firstDone, secondDone := make(chan result, 1), make(chan result, 1)
			go func() {
				changed, err := repo.Update(ctx, first, expectedVersion)
				firstDone <- result{changed, err}
			}()
			firstPID := waitRefreshBlock(t, adapter, ctx, gatePID)
			go func() {
				changed, err := repo.Update(ctx, second, expectedVersion)
				secondDone <- result{changed, err}
			}()
			waitRefreshBlock(t, adapter, ctx, firstPID)
			require.NoError(t, gate.Commit())
			winner, loser := <-firstDone, <-secondDone
			require.NoError(t, winner.err)
			assert.True(t, winner.changed)
			require.Error(t, loser.err)
			assert.False(t, loser.changed)
			var storageErr *storage.StorageError
			require.ErrorAs(t, loser.err, &storageErr)
			assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
			assert.Equal(t, provider.ClientIdentity, second.ClientIdentity)
			assert.Zero(t, second.ClientIdentityRevision)
			found, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, first.ClientIdentity, found.ClientIdentity)
			assert.Equal(t, int64(1), found.ClientIdentityRevision)
			assert.Equal(t, first.Version, found.Version)
			if first.DiscoveryStatus.LastAttemptAt != nil {
				assertDiscoveryInstant(t, *first.DiscoveryStatus.LastAttemptAt, found.DiscoveryStatus.LastAttemptAt)
			} else {
				assert.Nil(t, found.DiscoveryStatus.LastAttemptAt)
			}
		})
	}
}
