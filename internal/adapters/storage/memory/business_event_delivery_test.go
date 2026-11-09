package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestMemoryBusinessEventQueuesOnlyRequestedReferences(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	unqueued := memoryLedgerExample(t, "grant-created")
	unqueuedKey := memoryLedgerAppend(t, store, ctx, unqueued, false)
	queued := memoryLedgerExample(t, "grant-updated")
	queuedKey := memoryLedgerAppend(t, store, ctx, queued, true)

	rows, err := store.BusinessEvents().Query(ctx, model.BusinessEventQuery{
		Subject: memoryLedgerSubject(*unqueued.Subject),
		Start:   unqueued.OccurredAt.Add(-time.Second), End: queued.OccurredAt.Add(time.Second),
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []model.BusinessEventKey{unqueuedKey, queuedKey}, memoryLedgerKeys(rows), "the copy switch must not disable retained history")

	// A candidate is the exact payload-free (recorded_at, event_id) reference;
	// the event envelope is obtainable only through the ledger repository.
	keys, err := store.BusinessEventDelivery().ListDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{queuedKey}, keys)
	require.NotEqual(t, unqueuedKey.ID, keys[0].ID)
}

func TestMemoryBusinessEventRollbackRemovesEventAndReferenceTogether(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	committed := memoryLedgerExample(t, "grant-created")
	committedKey := memoryLedgerAppend(t, store, ctx, committed, true)

	txCtx, err := store.BeginTX(ctx)
	require.NoError(t, err)
	settled := false
	defer func() {
		if !settled {
			_ = store.Rollback(txCtx)
		}
	}()
	rolledBack := memoryLedgerExample(t, "grant-updated")
	memoryLedgerAppend(t, store, txCtx, rolledBack, true)
	require.NoError(t, store.Rollback(txCtx))
	settled = true

	rows, err := store.BusinessEvents().Query(ctx, model.BusinessEventQuery{
		Subject: memoryLedgerSubject(*committed.Subject),
		Start:   committed.OccurredAt.Add(-time.Second),
		End:     committed.OccurredAt.Add(time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{committedKey}, memoryLedgerKeys(rows), "rollback must not leave an event even when both facts reference the same grant")
	keys, err := store.BusinessEventDelivery().ListDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{committedKey}, keys, "rollback must remove only the failed occurrence's reference")
}

func memoryLedgerKeys(rows []*model.BusinessEvent) []model.BusinessEventKey {
	keys := make([]model.BusinessEventKey, 0, len(rows))
	for _, event := range rows {
		keys = append(keys, model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID})
	}
	return keys
}

func TestMemoryBusinessEventDeliveryClaimsAreIndependent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	store := memoryLedgerStore(t)
	primary := memoryLedgerExample(t, "grant-created")
	primaryKey := memoryLedgerAppend(t, store, ctx, primary, true)
	control := memoryLedgerExample(t, "grant-updated")
	controlKey := memoryLedgerAppend(t, store, ctx, control, true)

	received := make(chan model.BusinessEventKey, 1)
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	failure := errors.New("collector unavailable")
	type result struct {
		sent bool
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		sent, err := store.BusinessEventDelivery().DispatchOne(ctx, primaryKey, func(callbackCtx context.Context, retained *model.BusinessEvent) error {
			received <- model.BusinessEventKey{RecordedAt: retained.RecordedAt, ID: retained.ID}
			select {
			case <-gate:
				return failure
			case <-callbackCtx.Done():
				return callbackCtx.Err()
			}
		})
		finished <- result{sent: sent, err: err}
	}()
	select {
	case got := <-received:
		require.Equal(t, primaryKey, got)
	case got := <-finished:
		t.Fatalf("first claim never reached the collector: %+v", got)
	case <-ctx.Done():
		t.Fatal("first claim timed out before export")
	}

	controlCtx, stopControl := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stopControl()
	var delivered model.BusinessEventKey
	sent, err := store.BusinessEventDelivery().DispatchOne(controlCtx, controlKey, func(_ context.Context, retained *model.BusinessEvent) error {
		delivered = model.BusinessEventKey{RecordedAt: retained.RecordedAt, ID: retained.ID}
		return nil
	})
	require.NoError(t, err, "a blocked collector for one reference cannot serialize another claim")
	require.True(t, sent)
	require.Equal(t, controlKey, delivered)
	select {
	case got := <-finished:
		t.Fatalf("primary collector unexpectedly completed before release: %+v", got)
	default:
	}
	release()
	select {
	case got := <-finished:
		require.False(t, got.sent)
		require.ErrorIs(t, got.err, failure)
	case <-ctx.Done():
		t.Fatal("failed claim did not release after collector returned")
	}
	rows, err := store.BusinessEvents().Query(ctx, model.BusinessEventQuery{
		Subject: memoryLedgerSubject(*primary.Subject),
		Start:   primary.OccurredAt.Add(-time.Second), End: control.OccurredAt.Add(time.Second),
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []model.BusinessEventKey{primaryKey, controlKey}, memoryLedgerKeys(rows), "delivery retains both occurrences")
	due, err := store.BusinessEventDelivery().ListDue(ctx, 10)
	require.NoError(t, err)
	require.NotContains(t, due, controlKey, "successful control delivery is acknowledged independently")
	require.NotContains(t, due, primaryKey, "failed primary claim has a delayed retry, not an immediate loop")
}

func TestMemoryBusinessEventDeliveryFailedCallbackCannotMutateRetainedHistory(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	event := memoryLedgerExample(t, "grant-created")
	key := memoryLedgerAppend(t, store, ctx, event, true)
	control := memoryLedgerExample(t, "grant-updated")
	controlKey := memoryLedgerAppend(t, store, ctx, control, true)
	query := model.BusinessEventQuery{Subject: memoryLedgerSubject(*event.Subject), Start: event.OccurredAt.Add(-time.Second), End: event.OccurredAt.Add(time.Second)}
	rows, err := store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	want, err := json.Marshal(rows)
	require.NoError(t, err)
	failure := errors.New("destination rejected export")
	sent, err := store.BusinessEventDelivery().DispatchOne(ctx, key, func(_ context.Context, copy *model.BusinessEvent) error {
		require.Equal(t, key.ID, copy.ID)
		require.Equal(t, key.RecordedAt, copy.RecordedAt)
		copy.ID = id.NewBusinessEventID()
		*copy.Subject = id.NewPrincipal("collector-mutated-subject")
		copy.Data["reason_code"] = "collector-mutated-data"
		return failure
	})
	require.False(t, sent)
	require.ErrorIs(t, err, failure)
	rows, err = store.BusinessEvents().Query(ctx, query)
	require.NoError(t, err)
	got, err := json.Marshal(rows)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "failed exports cannot rewrite the immutable event used by later retries")
	due, err := store.BusinessEventDelivery().ListDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{controlKey}, due, "the other queued reference remains immediately deliverable")
}
