//go:build integration

package storage_test

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type ledgerDispatchResult struct {
	sent bool
	err  error
}

func awaitLedgerDispatch(t *testing.T, ctx context.Context, completed <-chan ledgerDispatchResult) ledgerDispatchResult {
	t.Helper()
	select {
	case result := <-completed:
		return result
	case <-ctx.Done():
		t.Fatal("delivery did not finish before the bounded deadline")
		return ledgerDispatchResult{}
	}
}

func TestBusinessEventDelivery_DueCandidatesAreBoundedReferences(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	first := ledgerExample(t, "token-issued", id.NewPrincipal("delivery-first"))
	second := ledgerExample(t, "token-issued", id.NewPrincipal("delivery-second"))
	future := ledgerExample(t, "token-issued", id.NewPrincipal("delivery-future"))
	unrequested := ledgerExample(t, "token-issued", id.NewPrincipal("delivery-uncopied"))
	for _, event := range []*model.BusinessEvent{first, second, future} {
		require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
	}
	require.NoError(t, adapter.BusinessEvents().Append(ctx, unrequested, false))
	_, err := db.ExecContext(ctx, `UPDATE public.business_event_delivery_pending
		SET next_attempt_at=clock_timestamp() + interval '1 hour' WHERE recorded_at=$1 AND event_id=$2`, second.RecordedAt, second.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE public.business_event_delivery_pending
		SET next_attempt_at=clock_timestamp() + interval '1 hour' WHERE recorded_at=$1 AND event_id=$2`, future.RecordedAt, future.ID)
	require.NoError(t, err)

	keys, err := adapter.BusinessEventDelivery().ListDue(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{ledgerKey(first)}, keys, "only due queued references may be selected")
	_, err = db.ExecContext(ctx, `UPDATE public.business_event_delivery_pending
		SET next_attempt_at=clock_timestamp() - interval '1 minute' WHERE recorded_at=$1 AND event_id=$2`, second.RecordedAt, second.ID)
	require.NoError(t, err)
	keys, err = adapter.BusinessEventDelivery().ListDue(ctx, 1)
	require.NoError(t, err)
	require.Len(t, keys, 1, "one scan cannot overrun its candidate limit")
	require.Contains(t, []model.BusinessEventKey{ledgerKey(first), ledgerKey(second)}, keys[0])
	keys, err = adapter.BusinessEventDelivery().ListDue(ctx, 2)
	require.NoError(t, err)
	require.ElementsMatch(t, []model.BusinessEventKey{ledgerKey(first), ledgerKey(second)}, keys)
	for _, event := range []*model.BusinessEvent{first, second, future, unrequested} {
		got, err := adapter.BusinessEvents().Get(ctx, ledgerKey(event))
		require.NoError(t, err)
		require.Equal(t, event.ID, got.ID, "candidate scans must not consume or replace retained history")
	}
}

func TestBusinessEventDelivery_CompetingWorkersSkipClaimedReference(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	primary := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-primary"))
	control := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-control"))
	for _, event := range []*model.BusinessEvent{primary, control} {
		require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
	}

	received := make(chan model.BusinessEventKey, 1)
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	finished := make(chan ledgerDispatchResult, 1)
	go func() {
		sent, err := adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(primary), func(callbackCtx context.Context, retained *model.BusinessEvent) error {
			received <- ledgerKey(retained)
			select {
			case <-gate:
				return nil
			case <-callbackCtx.Done():
				return callbackCtx.Err()
			}
		})
		finished <- ledgerDispatchResult{sent: sent, err: err}
	}()
	select {
	case got := <-received:
		require.Equal(t, ledgerKey(primary), got, "the receiver must see the retained event with its original ID")
	case result := <-finished:
		t.Fatalf("claim finished without delivering the retained event: %+v", result)
	case <-ctx.Done():
		t.Fatal("primary worker never reached the synchronous receiver")
	}

	otherCtx, stopOther := context.WithTimeout(ctx, 2*time.Second)
	defer stopOther()
	unexpected := make(chan model.BusinessEventKey, 1)
	sent, err := adapter.BusinessEventDelivery().DispatchOne(otherCtx, ledgerKey(primary), func(_ context.Context, event *model.BusinessEvent) error {
		unexpected <- ledgerKey(event)
		return nil
	})
	require.NoError(t, err, "a competing worker must skip a row locked by the active export")
	require.False(t, sent)
	require.Empty(t, unexpected, "SKIP LOCKED must prevent two simultaneous exports of one reference")

	sent, err = adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(control), func(_ context.Context, event *model.BusinessEvent) error {
		require.Equal(t, ledgerKey(control), ledgerKey(event))
		return nil
	})
	require.NoError(t, err, "another subject must be deliverable while the first receiver is blocked")
	require.True(t, sent)
	firstEvents, firstRefs := lifecycleCounts(t, db, primary)
	require.Equal(t, 1, firstEvents)
	require.Equal(t, 1, firstRefs, "a callback still in flight cannot be acknowledged")
	release()
	result := awaitLedgerDispatch(t, ctx, finished)
	require.NoError(t, result.err)
	require.True(t, result.sent)
	for _, event := range []*model.BusinessEvent{primary, control} {
		events, references := lifecycleCounts(t, db, event)
		require.Equal(t, 1, events, "delivery does not erase the ledger")
		require.Zero(t, references, "only a committed export removes its own reference")
	}
}

func TestBusinessEventDelivery_ReexportsStableIDWhenAcknowledgementRollsBack(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	event := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-ack-rollback"))
	control := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-ack-control"))
	for _, row := range []*model.BusinessEvent{event, control} {
		require.NoError(t, adapter.BusinessEvents().Append(ctx, row, true))
	}

	owner, err := adapter.BeginTX(ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{
		Subjects: []ports.StorageSubjectGate{{Principal: *event.Subject}},
	}))
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	var receiver []model.BusinessEventKey
	emit := func(_ context.Context, retained *model.BusinessEvent) error {
		require.Equal(t, event.Type, retained.Type)
		require.Equal(t, event.Subject, retained.Subject)
		require.Equal(t, event.ReasonUser, retained.ReasonUser)
		receiver = append(receiver, ledgerKey(retained))
		return nil
	}
	sent, err := adapter.BusinessEventDelivery().DispatchOne(owner, ledgerKey(event), emit)
	require.NoError(t, err)
	require.True(t, sent)
	require.Equal(t, []model.BusinessEventKey{ledgerKey(event)}, receiver, "the first export actually reached the receiver")
	_, references := lifecycleCounts(t, db, event)
	require.Equal(t, 1, references, "acknowledgement is invisible until its owning transaction commits")
	require.NoError(t, adapter.Rollback(owner))
	_, references = lifecycleCounts(t, db, event)
	require.Equal(t, 1, references, "rollback restores the already-exported event's reference")

	sent, err = adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(event), emit)
	require.NoError(t, err)
	require.True(t, sent)
	require.Equal(t, []model.BusinessEventKey{ledgerKey(event), ledgerKey(event)}, receiver,
		"retry after an uncommitted acknowledgement may duplicate only the same original ID")
	events, references := lifecycleCounts(t, db, event)
	require.Equal(t, 1, events)
	require.Zero(t, references, "the committed acknowledgement removes only the completed reference")
	controlEvents, controlRefs := lifecycleCounts(t, db, control)
	require.Equal(t, 1, controlEvents)
	require.Equal(t, 1, controlRefs)
}

func TestBusinessEventDelivery_RechecksEventAndReferenceAfterSubjectBarrier(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	control := ledgerExample(t, "token-issued", id.NewPrincipal("fresh-dispatch-control"))
	require.NoError(t, adapter.BusinessEvents().Append(ctx, control, true))

	for _, disappearance := range []string{"erasure deletes event and reference", "reference deleted before claim"} {
		t.Run(disappearance, func(t *testing.T) {
			subject := id.NewPrincipal("fresh-dispatch-" + disappearance)
			event := ledgerExample(t, "token-issued", subject)
			require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
			owner, err := db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = owner.Rollback() }()
			if disappearance == "erasure deletes event and reference" {
				var erased int64
				require.NoError(t, owner.GetContext(ctx, &erased, `SELECT public.business_event_erase_subject($1)`, subject.String()))
				require.EqualValues(t, 1, erased)
			} else {
				hash := fnv.New32a()
				_, err = hash.Write([]byte(subject.String()))
				require.NoError(t, err)
				_, err = owner.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1,$2)`, 1095320147, int32(hash.Sum32()))
				require.NoError(t, err)
				_, err = owner.ExecContext(ctx, `DELETE FROM public.business_event_delivery_pending
					WHERE recorded_at=$1 AND event_id=$2`, event.RecordedAt, event.ID)
				require.NoError(t, err)
			}
			callback := make(chan model.BusinessEventKey, 1)
			finished := make(chan ledgerDispatchResult, 1)
			go func() {
				sent, err := adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(event), func(_ context.Context, retained *model.BusinessEvent) error {
					callback <- ledgerKey(retained)
					return nil
				})
				finished <- ledgerDispatchResult{sent: sent, err: err}
			}()
			waitForLifecycleAdvisoryBlock(t, db, 0, 1095320147, 1, finished)
			require.NoError(t, owner.Commit())
			result := awaitLedgerDispatch(t, ctx, finished)
			require.NoError(t, result.err)
			require.False(t, result.sent, "a candidate found before deletion must be discarded after acquiring the subject gate")
			require.Empty(t, callback, "no payload may escape after its retained reference disappears")
			events, references := lifecycleCounts(t, db, event)
			require.Zero(t, references)
			if disappearance == "erasure deletes event and reference" {
				require.Zero(t, events)
			} else {
				require.Equal(t, 1, events, "loss of a reference does not erase an unrelated retained fact")
			}
		})
	}
	controlEvents, controlRefs := lifecycleCounts(t, db, control)
	require.Equal(t, 1, controlEvents)
	require.Equal(t, 1, controlRefs)
}

func TestBusinessEventDelivery_DeletionWaitsForExportAndCannotReplayFailedCopy(t *testing.T) {
	for _, deletion := range []string{"erasure", "retention"} {
		t.Run(deletion, func(t *testing.T) {
			adapter, db := openLedgerFoundation(t)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			subject := id.NewPrincipal("dispatch-delete-" + deletion)
			event := ledgerExample(t, "token-issued", subject)
			if deletion == "retention" {
				event.RecordedAt = time.Now().UTC().Truncate(6 * time.Hour).Add(-100*24*time.Hour + time.Minute)
				event.OccurredAt = event.RecordedAt
				seedLifecycleHistory(t, db, event, true)
			} else {
				require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
			}
			control := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-delete-control"))
			require.NoError(t, adapter.BusinessEvents().Append(ctx, control, true))

			received := make(chan model.BusinessEventKey, 1)
			gate := make(chan struct{})
			release := sync.OnceFunc(func() { close(gate) })
			defer release()
			collectorFailure := errors.New("collector unavailable")
			finished := make(chan ledgerDispatchResult, 1)
			go func() {
				sent, err := adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(event), func(callbackCtx context.Context, retained *model.BusinessEvent) error {
					received <- ledgerKey(retained)
					select {
					case <-gate:
						return collectorFailure
					case <-callbackCtx.Done():
						return callbackCtx.Err()
					}
				})
				finished <- ledgerDispatchResult{sent: sent, err: err}
			}()
			select {
			case got := <-received:
				require.Equal(t, ledgerKey(event), got)
			case result := <-finished:
				t.Fatalf("delivery ended without reaching the receiver: %+v", result)
			case <-ctx.Done():
				t.Fatal("delivery did not reach the receiver")
			}

			deleted := make(chan error, 1)
			if deletion == "erasure" {
				go func() {
					count, err := adapter.BusinessEventLifecycle().EraseSubject(ctx, subject)
					if err == nil && count != 1 {
						err = errors.New("erasure did not remove the dispatched occurrence")
					}
					deleted <- err
				}()
				waitForLifecycleAdvisoryBlock(t, db, 0, 1095320147, 1, deleted)
			} else {
				go func() {
					_, err := db.ExecContext(ctx, `SELECT public.business_event_maintain_partitions()`)
					deleted <- err
				}()
				waitForLifecycleAdvisoryBlock(t, db, 0, 1095320140, 1, deleted)
			}
			// A failed export leaves work pending while the barrier remains held.
			_, refs := lifecycleCounts(t, db, event)
			require.Equal(t, 1, refs)
			release()
			attempt := awaitLedgerDispatch(t, ctx, finished)
			require.False(t, attempt.sent)
			require.Error(t, attempt.err)
			select {
			case err := <-deleted:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("deletion did not finish after the receiver released the barrier")
			}
			events, references := lifecycleCounts(t, db, event)
			require.Zero(t, events)
			require.Zero(t, references, "deletion must remove even a deferred retry")
			_, err := adapter.BusinessEvents().Get(ctx, ledgerKey(event))
			require.True(t, ports.IsNotFoundErr(err))
			var replayed bool
			sent, err := adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(event), func(context.Context, *model.BusinessEvent) error {
				replayed = true
				return nil
			})
			require.NoError(t, err)
			require.False(t, sent)
			require.False(t, replayed, "no copied payload may outlive the deletion barrier")
			controlEvents, controlRefs := lifecycleCounts(t, db, control)
			require.Equal(t, 1, controlEvents)
			require.Equal(t, 1, controlRefs, "deletion may not consume a different subject's pending copy")
		})
	}
}

func TestBusinessEventDelivery_FailedExportDefersOnlyItsOwnReference(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	event := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-retry-target"))
	control := ledgerExample(t, "token-issued", id.NewPrincipal("dispatch-retry-control"))
	for _, row := range []*model.BusinessEvent{event, control} {
		require.NoError(t, adapter.BusinessEvents().Append(ctx, row, true))
	}
	var before, after, nextAttempt time.Time
	require.NoError(t, db.GetContext(ctx, &before, `SELECT clock_timestamp()`))
	var firstID id.BusinessEventID
	sent, err := adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(event), func(_ context.Context, retained *model.BusinessEvent) error {
		firstID = retained.ID
		return errors.New("collector unavailable")
	})
	require.False(t, sent)
	require.Error(t, err)
	require.Equal(t, event.ID, firstID, "the failed export must have actually reached the receiver")
	require.NoError(t, db.GetContext(ctx, &after, `SELECT clock_timestamp()`))
	require.NoError(t, db.GetContext(ctx, &nextAttempt, `SELECT next_attempt_at FROM public.business_event_delivery_pending
		WHERE recorded_at=$1 AND event_id=$2`, event.RecordedAt, event.ID))
	require.False(t, nextAttempt.Before(before.Add(30*time.Second)), "retry cannot happen before the 30-second failure interval")
	require.False(t, nextAttempt.After(after.Add(30*time.Second)), "retry cannot be postponed indefinitely")
	keys, err := adapter.BusinessEventDelivery().ListDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []model.BusinessEventKey{ledgerKey(control)}, keys, "an unrelated pending copy remains due")
	_, err = db.ExecContext(ctx, `UPDATE public.business_event_delivery_pending SET next_attempt_at=clock_timestamp() - interval '1 second'
		WHERE recorded_at=$1 AND event_id=$2`, event.RecordedAt, event.ID)
	require.NoError(t, err)
	sent, err = adapter.BusinessEventDelivery().DispatchOne(ctx, ledgerKey(event), func(_ context.Context, retained *model.BusinessEvent) error {
		require.Equal(t, firstID, retained.ID, "retry must export the original retained ID")
		return nil
	})
	require.NoError(t, err)
	require.True(t, sent)
	events, references := lifecycleCounts(t, db, event)
	require.Equal(t, 1, events)
	require.Zero(t, references)
	controlEvents, controlRefs := lifecycleCounts(t, db, control)
	require.Equal(t, 1, controlEvents)
	require.Equal(t, 1, controlRefs)
}
