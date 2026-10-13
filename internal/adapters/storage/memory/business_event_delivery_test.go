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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestMemoryBusinessEventQueuesOnlyRequestedReferences(t *testing.T) {
	ctx := context.Background()
	store := memoryLedgerStore(t)
	unqueued := memoryLedgerExample(t, "grant-created")
	unqueuedKey := memoryLedgerAppend(t, store, ctx, unqueued, false)
	queued := memoryLedgerExample(t, "grant-updated")
	queuedKey := memoryLedgerAppend(t, store, ctx, queued, true)

	unqueuedRow, err := store.BusinessEvents().Get(ctx, unqueuedKey)
	require.NoError(t, err, "the copy switch must not disable retained history")
	require.Equal(t, unqueued.ID, unqueuedRow.ID)
	queuedRow, err := store.BusinessEvents().Get(ctx, queuedKey)
	require.NoError(t, err)
	require.Equal(t, queued.ID, queuedRow.ID)

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
	rolledBackKey := memoryLedgerAppend(t, store, txCtx, rolledBack, true)
	require.NoError(t, store.Rollback(txCtx))
	settled = true

	_, err = store.BusinessEvents().Get(ctx, rolledBackKey)
	require.True(t, ports.IsNotFoundErr(err), "rolled-back occurrence must not remain addressable")
	retained, err := store.BusinessEvents().Get(ctx, committedKey)
	require.NoError(t, err)
	require.Equal(t, committedKey.ID, retained.ID, "rollback must preserve a different committed occurrence")
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
	for _, key := range []model.BusinessEventKey{primaryKey, controlKey} {
		retained, err := store.BusinessEvents().Get(ctx, key)
		require.NoError(t, err)
		require.Equal(t, key.ID, retained.ID, "delivery may acknowledge references but must retain the occurrence")
	}
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
	retained, err := store.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	want, err := json.Marshal(retained)
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
	stored, err := store.BusinessEvents().Get(ctx, key)
	require.NoError(t, err)
	got, err := json.Marshal(stored)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "failed exports cannot rewrite the immutable event used by later retries")
	due, err := store.BusinessEventDelivery().ListDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{controlKey}, due, "the other queued reference remains immediately deliverable")
}
