package oauth2server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

func newLedgerSigningService(t *testing.T) (*SigningKeyService, *testSigningKeyStore, *ledgerfixture.Store) {
	t.Helper()
	svc, repo := newTestSigningKeyService()
	store := &ledgerfixture.Store{Snapshot: func() func() {
		repo.mu.RLock()
		keys := make(map[id.SigningKeyID]*storage.SigningKey, len(repo.byID))
		for key, record := range repo.byID {
			keys[key] = cloneStrategySigningKey(record)
		}
		repo.mu.RUnlock()
		return func() {
			repo.mu.Lock()
			defer repo.mu.Unlock()
			repo.byID = keys
			repo.byKID = make(map[id.KeyID]*storage.SigningKey, len(keys))
			for _, key := range keys {
				repo.byKID[key.KID] = key
			}
		}
	}}
	svc.ledger = store.Recorder(t)
	return svc, repo, store
}

func TestSigningLedgerBootstrapAndActualCurrentSelectionOnly(t *testing.T) {
	svc, _, store := newLedgerSigningService(t)
	ctx := context.Background()
	start := time.Now().UTC()
	initial, created, err := svc.EnsureInitialKey(ctx, "ES256")
	require.NoError(t, err)
	require.True(t, created)
	_, created, err = svc.EnsureInitialKey(ctx, "ES256")
	require.NoError(t, err)
	require.False(t, created)
	_, err = svc.PromoteKey(ctx, initial.KID)
	require.NoError(t, err)
	standby, err := svc.GenerateAndStoreKey(ctx, "ES256", false)
	require.NoError(t, err)
	_, err = svc.PromoteKey(ctx, standby.KID)
	require.NoError(t, err)
	_, err = svc.PromoteKey(ctx, standby.KID)
	require.NoError(t, err)
	require.Len(t, store.Events, 2, "bootstrap and changed selection are facts; generation alone and same selection are not")
	for i, key := range []*storage.SigningKey{initial, standby} {
		event := store.Events[i]
		require.Equal(t, model.BusinessEventTypePrefix+"signing-key-promoted", event.Type)
		require.Equal(t, key.ID.String(), event.Data["signing_key_id"])
		require.Nil(t, event.Subject)
		require.False(t, event.OccurredAt.Before(start), "selection time must not be backdated to bootstrap eligibility")
		require.False(t, event.OccurredAt.After(time.Now().UTC()))
	}
}

func TestSigningLedgerSelectionTimeIsNotFutureEligibilityTime(t *testing.T) {
	svc, _, store := newLedgerSigningService(t)
	start := time.Now().UTC()
	key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
	require.NoError(t, err)
	require.Len(t, store.Events, 1)
	event := store.Events[0]
	require.Equal(t, model.BusinessEventTypePrefix+"signing-key-promoted", event.Type)
	require.False(t, event.OccurredAt.Before(start))
	require.True(t, event.OccurredAt.Before(key.ActivatesAt))
	require.Equal(t, key.ActivatesAt.Format(time.RFC3339Nano), event.Data["activates_at"])
}

func TestSigningLedgerFailedSelectionRestoresKeys(t *testing.T) {
	for _, action := range []string{"bootstrap", "generate-current", "promote"} {
		for _, failure := range []string{"append", "commit"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				svc, repo, store := newLedgerSigningService(t)
				ctx := context.Background()
				var previous, standby *storage.SigningKey
				if action == "promote" {
					var err error
					previous, err = svc.generateAndStore(ctx, "ES256", true, time.Now().UTC().Add(-time.Hour))
					require.NoError(t, err)
					standby, err = svc.GenerateAndStoreKey(ctx, "ES256", false)
					require.NoError(t, err)
					store.Events = nil
				}
				failed := errors.New("ledger unavailable")
				if failure == "append" {
					store.AppendError = failed
				} else {
					store.CommitError = failed
				}
				var err error
				switch action {
				case "bootstrap":
					key, created, failure := svc.EnsureInitialKey(ctx, "ES256")
					err = failure
					require.Nil(t, key)
					require.False(t, created)
				case "generate-current":
					key, failure := svc.GenerateAndStoreKey(ctx, "ES256", true)
					err = failure
					require.Nil(t, key)
				case "promote":
					_, err = svc.PromoteKey(ctx, standby.KID)
				}
				require.Error(t, err)
				require.Empty(t, store.Events)
				if action == "promote" {
					require.Equal(t, previous, repo.byKID[previous.KID])
					require.Equal(t, standby, repo.byKID[standby.KID])
				} else {
					require.Empty(t, repo.byID)
				}
			})
		}
	}
}

func TestSigningLedgerCompetingBootstrapProducesOneSelection(t *testing.T) {
	svc, repo, store := newLedgerSigningService(t)
	second := *svc
	type outcome struct {
		created bool
		err     error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, service := range []*SigningKeyService{svc, &second} {
		go func() {
			<-start
			_, created, err := service.EnsureInitialKey(context.Background(), "ES256")
			results <- outcome{created, err}
		}()
	}
	close(start)
	winners := 0
	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		if result.created {
			winners++
		}
	}
	require.Equal(t, 1, winners)
	keys, err := repo.ListActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Len(t, store.Events, 1)
	require.Equal(t, keys[0].ID.String(), store.Events[0].Data["signing_key_id"])
}
