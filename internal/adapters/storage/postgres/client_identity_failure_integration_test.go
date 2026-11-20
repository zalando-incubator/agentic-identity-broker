//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresClientIdentityLegacyInitialization(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	for _, next := range []id.ClientID{"client-123", "replacement"} {
		t.Run(next.String(), func(t *testing.T) {
			provider := createClientIdentityProvider(t, repo, false)
			_, err := adapter.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET client_identity = NULL WHERE id = $1`, provider.ID)
			require.NoError(t, err)
			provider.ClientID = next
			changed, err := repo.Update(ctx, provider, &provider.Version)
			require.NoError(t, err)
			assert.Equal(t, next != "client-123", changed)
			expectedRevision := int64(0)
			if changed {
				expectedRevision = 1
			}
			assert.Equal(t, new(next), provider.ClientIdentity)
			assert.Equal(t, expectedRevision, provider.ClientIdentityRevision)
			found, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, provider.ClientIdentity, found.ClientIdentity)
			assert.Equal(t, expectedRevision, found.ClientIdentityRevision)
		})
	}
}

func TestPostgresClientIdentityUpdateFailuresAreAtomic(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	for _, failure := range []string{"resource ownership", "deferred commit", "revision exhaustion", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			provider := createClientIdentityProvider(t, repo, false)
			sessionRepo := NewUserSessionRepository(adapter)
			session := createClientIdentitySession(t, sessionRepo, provider)
			if failure == "revision exhaustion" {
				_, err := adapter.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET client_identity_revision = $1 WHERE id = $2`, int64(math.MaxInt64), provider.ID)
				require.NoError(t, err)
			}
			before, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			candidate := before.Copy()
			candidate.ClientID = "replacement"
			candidate.Secret = model.NewEncryptedSecret([]byte("discarded-secret-ciphertext"))
			candidate.ProtectedResources = []string{before.ProtectedResources[0] + "/replacement"}
			switch failure {
			case "resource ownership":
				owner := createClientIdentityProvider(t, repo, false)
				candidate.ProtectedResources = owner.ProtectedResources
			case "deferred commit":
				defer rejectClientIdentityCommit(t, adapter, "thirdparty_oauth2_services")()
			}
			input := candidate.Copy()
			writeCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			changed, err := repo.Update(writeCtx, candidate, &before.Version)
			require.Error(t, err)
			assert.False(t, changed, "rolled-back writes cannot attribute an identity-change event")
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, input, candidate, "failed updates must not publish committed identity or version")
			after, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after, "identity, secret, metadata and child resources must roll back together")
			persisted, err := sessionRepo.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, session.UpstreamClientID, persisted.UpstreamClientID)
			assert.Equal(t, session.UpstreamClientRevision, persisted.UpstreamClientRevision)
			assert.Equal(t, session.EncryptedAccessToken, persisted.EncryptedAccessToken)
			assert.Equal(t, session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
		})
	}
}

func TestPostgresClientIdentityDiscoveryPrecondition(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	for _, attempt := range []string{"older", "equal", "missing", "newer"} {
		t.Run(attempt, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Microsecond)
			provider := discoveredCIMDProvider(id.NewServiceID(), "https://reports.example.com/"+id.NewServiceID().String(), now)
			require.NoError(t, repo.Create(ctx, provider))
			before, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			candidate := before.Copy()
			candidate.ClientID = id.ClientID("https://replacement.example.test/.well-known/oauth-client/" + candidate.ID.String())
			switch attempt {
			case "older":
				candidate.DiscoveryStatus.LastAttemptAt = new(now.Add(-time.Second))
				candidate.DiscoveryStatus.LastSuccessAt = candidate.DiscoveryStatus.LastAttemptAt
			case "missing":
				candidate.DiscoveryStatus.LastAttemptAt = nil
			case "newer":
				candidate.DiscoveryStatus.LastAttemptAt = new(now.Add(time.Second))
				candidate.DiscoveryStatus.LastSuccessAt = candidate.DiscoveryStatus.LastAttemptAt
			}
			changed, err := repo.Update(ctx, candidate, nil)
			if attempt == "newer" {
				require.NoError(t, err)
				assert.True(t, changed)
				assert.Equal(t, int64(1), candidate.ClientIdentityRevision)
			} else {
				require.Error(t, err)
				assert.False(t, changed)
				var storageErr *storage.StorageError
				require.ErrorAs(t, err, &storageErr)
				kind := storage.ErrorKindConflict
				if attempt == "missing" {
					kind = storage.ErrorKindValidation
				}
				assert.Equal(t, kind, storageErr.Kind)
			}
			after, getErr := repo.Get(ctx, provider.ID)
			require.NoError(t, getErr)
			if attempt == "newer" {
				assert.Equal(t, candidate.ClientIdentity, after.ClientIdentity)
				assert.Equal(t, candidate.ClientIdentityRevision, after.ClientIdentityRevision)
			} else {
				assert.Equal(t, before, after)
			}
		})
	}
}

func TestPostgresClientIdentityObservationErrorsAreAtomic(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	for _, failure := range []string{"revision exhaustion", "canceled", "lock timeout", "deferred commit"} {
		t.Run(failure, func(t *testing.T) {
			provider := createClientIdentityProvider(t, repo, true)
			_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "reports-v1", clientIdentityObservation(provider))
			require.NoError(t, err)
			require.True(t, applied)
			require.False(t, changed)
			if failure == "revision exhaustion" {
				_, err := adapter.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET client_identity_revision = $1 WHERE id = $2`, int64(math.MaxInt64), provider.ID)
				require.NoError(t, err)
			}
			before, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			writeCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			if failure == "deferred commit" {
				defer rejectClientIdentityCommit(t, adapter, "thirdparty_oauth2_services")()
			}
			if failure == "lock timeout" {
				tx, err := adapter.db.BeginTxx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = tx.Rollback() }()
				_, err = tx.ExecContext(ctx, `SELECT id FROM thirdparty_oauth2_services WHERE id = $1 FOR UPDATE`, provider.ID)
				require.NoError(t, err)
				oldTimeout := adapter.timeouts.Write
				adapter.timeouts.Write = 50 * time.Millisecond
				defer func() { adapter.timeouts.Write = oldTimeout }()
			}
			state, applied, changed, err := repo.RecordObservedClientIdentity(writeCtx, provider.ID, "reports-v2", clientIdentityObservation(before))
			require.Error(t, err)
			assert.False(t, applied)
			assert.False(t, changed)
			assert.Equal(t, ports.ClientIdentityState{}, state)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			if failure == "lock timeout" {
				assert.Equal(t, storage.ErrorKindTimeout, storageErr.Kind)
			}
			if failure == "canceled" {
				assert.ErrorIs(t, err, context.Canceled)
			}
			after, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestPostgresSessionClientIdentityCallbackFailures(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	providers := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	for _, failure := range []string{"callback error", "validation", "no write", "deferred commit"} {
		t.Run(failure, func(t *testing.T) {
			provider := createClientIdentityProvider(t, providers, false)
			session := createClientIdentitySession(t, repo, provider)
			before, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			rejected := errors.New("provider callback rejected")
			if failure == "deferred commit" {
				defer rejectClientIdentityCommit(t, adapter, "user_sessions")()
			}
			result, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				current.CallbackID = "discarded"
				current.UpstreamClientID = new(id.ClientID("replacement"))
				current.UpstreamClientRevision = 1
				current.EncryptedAccessToken = []byte("discarded-access-ciphertext")
				switch failure {
				case "callback error":
					return true, rejected
				case "validation":
					current.UpstreamClientRevision = -1
					return true, nil
				case "deferred commit":
					return true, nil
				default:
					return false, nil
				}
			})
			if failure == "no write" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
				if failure == "callback error" {
					assert.ErrorIs(t, err, rejected)
				}
			}
			after, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func rejectClientIdentityCommit(t *testing.T, adapter *Adapter, table string) func() {
	t.Helper()
	ctx := context.Background()
	_, err := adapter.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION reject_client_identity_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'injected identity commit failure' USING ERRCODE = '23514';
		END;
		$$;
		CREATE CONSTRAINT TRIGGER reject_client_identity_commit
		AFTER UPDATE ON %s DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION reject_client_identity_commit();`, table))
	require.NoError(t, err)
	return func() {
		_, err := adapter.db.ExecContext(ctx, fmt.Sprintf(`DROP TRIGGER reject_client_identity_commit ON %s; DROP FUNCTION reject_client_identity_commit();`, table))
		require.NoError(t, err)
	}
}
