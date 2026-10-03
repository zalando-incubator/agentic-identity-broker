package memory

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type memoryLifecycleFixture struct {
	transactions *TransactionManager
	events       *BusinessEventRepository
	clock        time.Time
	examples     []model.BusinessEvent
}

func newMemoryLifecycleFixture(t *testing.T, at time.Time) *memoryLifecycleFixture {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	contents, err := fs.ReadFile(eventschemas.Schemas, "examples.json")
	require.NoError(t, err)
	fixture := &memoryLifecycleFixture{transactions: NewTransactionManager(), clock: at}
	require.NoError(t, json.Unmarshal(contents, &fixture.examples))
	fixture.events = NewBusinessEventRepository(fixture.transactions, registry)
	fixture.events.now = func() time.Time { return fixture.clock }
	return fixture
}

func (f *memoryLifecycleFixture) append(t *testing.T, ctx context.Context, name string, subject *id.Principal, recordedAt time.Time, queue bool, occurrence ...time.Time) model.BusinessEventKey {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, example := range f.examples {
		if example.Type != model.BusinessEventTypePrefix+name {
			continue
		}
		event := example
		event.ID = id.NewBusinessEventID()
		event.Subject = subject
		event.OccurredAt = recordedAt
		if len(occurrence) != 0 {
			event.OccurredAt = occurrence[0]
		}
		f.clock = recordedAt
		require.NoError(t, f.events.Append(ctx, &event, queue))
		require.Equal(t, recordedAt.UTC().Truncate(time.Microsecond), event.RecordedAt)
		return model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
	}
	t.Fatalf("no published business-event example for %q", name)
	return model.BusinessEventKey{}
}

func (f *memoryLifecycleFixture) query(t *testing.T, subject model.BusinessEventSubject) []model.BusinessEventKey {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := f.events.Query(ctx, model.BusinessEventQuery{
		Subject: subject,
		Start:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		End:     time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	keys := make([]model.BusinessEventKey, 0, len(rows))
	for _, event := range rows {
		keys = append(keys, model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID})
	}
	return keys
}

func (f *memoryLifecycleFixture) assertDeleted(t *testing.T, key model.BusinessEventKey) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := f.events.Get(ctx, key)
	require.True(t, ports.IsNotFoundErr(err), "deleted event must not remain addressable: %v", err)
	_, pending := f.events.pending[key]
	require.False(t, pending, "deleted event must not retain a delivery reference")
}

func (f *memoryLifecycleFixture) assertRetained(t *testing.T, key model.BusinessEventKey, queued bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	row, err := f.events.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, key.ID, row.ID)
	_, pending := f.events.pending[key]
	require.Equal(t, queued, pending, "retention/erasure must not change another event's pending state")
}

func TestMemoryBusinessEventErasureIsExactCommittedAndRepeatable(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, at)
	target, other := id.Principal("principal-example"), id.Principal("principal-example-extra")
	first := f.append(t, ctx, "grant-created", &target, at.Add(-100*24*time.Hour), true)
	second := f.append(t, ctx, "approval-denied", &target, at.Add(-time.Hour), false)
	third := f.append(t, ctx, "grant-updated", &target, at, true)
	// These controls have an actor that names the erased principal, but not its subject.
	otherKey := f.append(t, ctx, "grant-created", &other, at.Add(-time.Hour), true)
	nullKey := f.append(t, ctx, "agent-registered", nil, at, true)

	count, err := f.events.EraseSubject(ctx, target)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
	for _, key := range []model.BusinessEventKey{first, second, third} {
		f.assertDeleted(t, key)
	}
	require.Empty(t, f.query(t, model.BusinessEventSubject{Principal: target}))
	f.assertRetained(t, otherKey, true)
	f.assertRetained(t, nullKey, true)
	require.Equal(t, []model.BusinessEventKey{otherKey}, f.query(t, model.BusinessEventSubject{Principal: other}))
	require.Equal(t, []model.BusinessEventKey{nullKey}, f.query(t, model.BusinessEventSubject{NoSubject: true}))
	require.Len(t, f.events.events, 2, "erasure must not insert a replacement occurrence")

	count, err = f.events.EraseSubject(ctx, target)
	require.NoError(t, err)
	require.Zero(t, count, "a repeated erasure must report only newly committed deletions")
	f.assertRetained(t, otherKey, true)
	f.assertRetained(t, nullKey, true)

	later := f.append(t, ctx, "grant-created", &target, at.Add(time.Minute), true)
	f.assertRetained(t, later, true)
	require.Equal(t, []model.BusinessEventKey{later}, f.query(t, model.BusinessEventSubject{Principal: target}), "erasure is not a permanent identity tombstone")
}

func TestMemoryBusinessEventErasureRejectsInvalidSelectorsWithoutDeletingHistory(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, at)
	target := id.Principal("principal-example")
	key := f.append(t, ctx, "grant-created", &target, at, true)
	for _, selector := range []id.Principal{"", "principal\x00example"} {
		count, err := f.events.EraseSubject(ctx, selector)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		require.Zero(t, count)
		f.assertRetained(t, key, true)
	}
}

func TestMemoryBusinessEventRetentionDefaultSixHourWindow(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, now)
	target, other := id.Principal("principal-example"), id.Principal("other-principal")
	cutoff := now.Add(-90 * 24 * time.Hour)
	older := f.append(t, ctx, "grant-created", &target, cutoff.Add(-time.Microsecond), true)
	boundary := f.append(t, ctx, "grant-updated", &target, cutoff, true, cutoff.Add(-365*24*time.Hour))
	otherKey := f.append(t, ctx, "grant-created", &other, now.Add(-time.Hour), true)
	f.clock = now.Add(-time.Microsecond)
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertRetained(t, older, true)
	f.assertRetained(t, boundary, true)

	f.clock = now
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertDeleted(t, older)
	f.assertRetained(t, boundary, true)
	f.assertRetained(t, otherKey, true)
	require.Equal(t, []model.BusinessEventKey{boundary}, f.query(t, model.BusinessEventSubject{Principal: target}))
	require.Equal(t, []model.BusinessEventKey{otherKey}, f.query(t, model.BusinessEventSubject{Principal: other}))
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertRetained(t, boundary, true)
}

func TestMemoryBusinessEventRetentionShortPolicyNeverRemovesCurrentWindowEarly(t *testing.T) {
	ctx := context.Background()
	noon := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, noon)
	target := id.Principal("principal-example")
	old := f.append(t, ctx, "grant-created", &target, noon.Add(-time.Microsecond), true)
	inWindow := f.append(t, ctx, "grant-updated", &target, noon, false)
	young := f.append(t, ctx, "grant-revoked", &target, noon.Add(25*time.Minute), true)
	require.NoError(t, f.events.SetRetentionPolicy(ctx, 30*time.Minute))

	f.clock = noon.Add(30*time.Minute - time.Microsecond)
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertRetained(t, old, true)
	f.clock = noon.Add(30 * time.Minute)
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertDeleted(t, old)
	f.assertRetained(t, inWindow, false)
	f.assertRetained(t, young, true)

	f.clock = noon.Add(6*time.Hour + 30*time.Minute - time.Microsecond)
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertRetained(t, inWindow, false)
	f.assertRetained(t, young, true)
	f.clock = noon.Add(6*time.Hour + 30*time.Minute)
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertDeleted(t, inWindow)
	f.assertDeleted(t, young)
	require.Empty(t, f.query(t, model.BusinessEventSubject{Principal: target}))
}

func TestMemoryBusinessEventSubMicrosecondPolicyNeverRemovesEarly(t *testing.T) {
	ctx := context.Background()
	boundary := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, boundary)
	target := id.Principal("principal-example")
	key := f.append(t, ctx, "grant-created", &target, boundary.Add(-6*time.Hour), true)
	require.NoError(t, f.events.SetRetentionPolicy(ctx, time.Nanosecond))
	f.clock = boundary
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertRetained(t, key, true)
	f.clock = boundary.Add(time.Microsecond)
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertDeleted(t, key)
}

func TestMemoryBusinessEventRetentionIncreaseTakesEffectBeforeSweep(t *testing.T) {
	ctx := context.Background()
	noon := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, noon)
	target := id.Principal("principal-example")
	key := f.append(t, ctx, "grant-created", &target, noon.Add(-time.Hour), true)
	require.NoError(t, f.events.SetRetentionPolicy(ctx, 30*time.Minute))
	f.clock = noon.Add(30 * time.Minute)
	require.NoError(t, f.events.SetRetentionPolicy(ctx, 90*24*time.Hour))
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertRetained(t, key, true)
	require.Equal(t, []model.BusinessEventKey{key}, f.query(t, model.BusinessEventSubject{Principal: target}))
}

func TestMemoryBusinessEventLifecycleWaitsForPredecessorsAndHonorsCancellation(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, at)
	target := id.Principal("principal-example")
	owner, err := f.transactions.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.transactions.Rollback(owner)) }()
	key := f.append(t, owner, "grant-created", &target, at, true)

	blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	count, err := f.events.EraseSubject(blocked, target)
	require.ErrorIs(t, err, context.DeadlineExceeded, "erasure must not complete while an earlier writer owns the lifecycle gate")
	require.Zero(t, count)
	inside, err := f.events.Get(owner, key)
	require.NoError(t, err)
	require.Equal(t, key.ID, inside.ID)
	require.NoError(t, f.transactions.Commit(owner))

	finalCtx, stopFinal := context.WithTimeout(ctx, 2*time.Second)
	defer stopFinal()
	count, err = f.events.EraseSubject(finalCtx, target)
	require.NoError(t, err)
	require.EqualValues(t, 1, count, "erasure must count the predecessor's committed occurrence")
	f.assertDeleted(t, key)
	later := f.append(t, ctx, "grant-created", &target, at.Add(time.Minute), true)
	f.assertRetained(t, later, true)

	cancelled, stop := context.WithCancel(ctx)
	stop()
	count, err = f.events.EraseSubject(cancelled, target)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, count)
	f.assertRetained(t, later, true)
}

func TestMemoryBusinessEventErasureSeesOnlyCommittedPredecessors(t *testing.T) {
	for _, commit := range []bool{true, false} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
			f := newMemoryLifecycleFixture(t, at)
			target := id.Principal("principal-example")
			owner, err := f.transactions.BeginTX(context.Background())
			require.NoError(t, err)
			defer func() { require.NoError(t, f.transactions.Rollback(owner)) }()
			key := f.append(t, owner, "grant-created", &target, at, true)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := make(chan struct{})
			type result struct {
				count int64
				err   error
			}
			finished := make(chan result, 1)
			go func() {
				close(started)
				count, err := f.events.EraseSubject(ctx, target)
				finished <- result{count, err}
			}()
			<-started
			if commit {
				require.NoError(t, f.transactions.Commit(owner))
			} else {
				require.NoError(t, f.transactions.Rollback(owner))
			}
			select {
			case got := <-finished:
				require.NoError(t, got.err)
				if commit {
					require.EqualValues(t, 1, got.count)
				} else {
					require.Zero(t, got.count, "a rolled-back event cannot contribute to the committed deletion count")
				}
			case <-ctx.Done():
				t.Fatal("erasure did not complete after the predecessor released its transaction")
			}
			f.assertDeleted(t, key)
			require.Empty(t, f.query(t, model.BusinessEventSubject{Principal: target}))
		})
	}
}

func TestMemoryBusinessEventMaintenanceCannotRunInsideUncommittedBusinessTransaction(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, at)
	target := id.Principal("principal-example")
	owner, err := f.transactions.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.transactions.Rollback(owner)) }()
	key := f.append(t, owner, "grant-created", &target, at.Add(-91*24*time.Hour), true)
	young := f.append(t, owner, "grant-updated", &target, at.Add(-7*time.Hour), true)

	for _, operation := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"retention", f.events.ApplyRetention},
		{"policy update", func(ctx context.Context) error { return f.events.SetRetentionPolicy(ctx, time.Hour) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			err := operation.run(blocked)
			require.ErrorIs(t, err, context.DeadlineExceeded, "exclusive maintenance must wait for active recording")
		})
	}
	require.NoError(t, f.transactions.Commit(owner))
	f.clock = at
	finalCtx, stopFinal := context.WithTimeout(ctx, 2*time.Second)
	defer stopFinal()
	require.NoError(t, f.events.ApplyRetention(finalCtx))
	f.assertDeleted(t, key)
	f.assertRetained(t, young, true)
	require.Equal(t, []model.BusinessEventKey{young}, f.query(t, model.BusinessEventSubject{Principal: target}))
}

func TestMemoryBusinessEventErasurePreservesGrantRecognition(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := newMemoryLifecycleFixture(t, now)
	target := id.Principal("principal-example")
	expiry := now.Add(-time.Hour)
	grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: target, AgentID: id.NewAgentID(), ValidUntil: &expiry}
	grants := NewUserGrantRepository(f.transactions)
	require.NoError(t, grants.Create(ctx, grant))
	owner, err := f.transactions.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.transactions.Rollback(owner)) }()
	won, err := grants.RecordExpiration(owner, grant.ID, expiry)
	require.NoError(t, err)
	require.True(t, won)
	key := f.append(t, owner, "grant-expired", &target, now, true, expiry)
	require.NoError(t, f.transactions.Commit(owner))

	count, err := f.events.EraseSubject(ctx, target)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	f.assertDeleted(t, key)
	candidates, err := grants.ListUnrecordedExpired(ctx, now, 10)
	require.NoError(t, err)
	require.Empty(t, candidates, "deleting event history must not reset the business object's recognition marker")
	won, err = grants.RecordExpiration(ctx, grant.ID, expiry)
	require.NoError(t, err)
	require.False(t, won)

	laterExpiry := now.Add(-time.Minute)
	grant.ValidUntil = &laterExpiry
	require.NoError(t, grants.Update(ctx, grant))
	candidates, err = grants.ListUnrecordedExpired(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "a changed effective expiry is a new business occurrence")
	won, err = grants.RecordExpiration(ctx, grant.ID, laterExpiry)
	require.NoError(t, err)
	require.True(t, won)
	later := f.append(t, ctx, "grant-expired", &target, now.Add(time.Minute), true, laterExpiry)
	f.assertRetained(t, later, true)
	require.Equal(t, []model.BusinessEventKey{later}, f.query(t, model.BusinessEventSubject{Principal: target}))
}

func TestMemoryBusinessEventRetentionPreservesApprovalRecognition(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := newMemoryLifecycleFixture(t, now)
	target := id.Principal("principal-example")
	expiry := now.Add(-91 * 24 * time.Hour)
	approval := &storage.ToolApproval{
		ID: id.NewApprovalID(), Principal: target, AgentID: id.NewAgentID(), ToolName: "read_file",
		ToolPattern: "read_file", ArgumentsHash: "expired-approval", Status: storage.ApprovalStatusPending,
		CreatedAt: expiry.Add(-time.Minute), ExpiresAt: expiry,
	}
	approvals := NewToolApprovalRepository(f.transactions)
	_, err := approvals.Create(ctx, approval)
	require.NoError(t, err)
	owner, err := f.transactions.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.transactions.Rollback(owner)) }()
	won, err := approvals.RecordExpiration(owner, approval.ID, expiry)
	require.NoError(t, err)
	require.True(t, won)
	key := f.append(t, owner, "approval-expired", &target, expiry, true)
	require.NoError(t, f.transactions.Commit(owner))

	f.clock = now
	require.NoError(t, f.events.ApplyRetention(ctx))
	f.assertDeleted(t, key)
	candidates, err := approvals.ListUnrecordedExpiredForPrincipal(ctx, target, now, 10)
	require.NoError(t, err)
	require.Empty(t, candidates, "logical retention must not reset approval recognition")
	won, err = approvals.RecordExpiration(ctx, approval.ID, expiry)
	require.NoError(t, err)
	require.False(t, won)
}

func TestMemoryBusinessEventInvalidPolicyLeavesHistoryUntouched(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, at)
	target := id.Principal("principal-example")
	key := f.append(t, ctx, "grant-created", &target, at, true)
	for _, retention := range []time.Duration{0, -time.Second} {
		err := f.events.SetRetentionPolicy(ctx, retention)
		var storageErr *storage.StorageError
		require.True(t, errors.As(err, &storageErr), "invalid policy must report a storage validation error: %v", err)
		require.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		f.assertRetained(t, key, true)
	}
}

func TestMemoryBusinessEventCollectorReleasesVisibilityButProtectsErasure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	f := newMemoryLifecycleFixture(t, at)
	target, other := id.Principal("principal-example"), id.Principal("other-principal")
	otherKey := f.append(t, context.Background(), "grant-created", &other, at, true)
	key := f.append(t, context.Background(), "grant-updated", &target, at, true)
	observed := make(chan model.BusinessEventKey, 1)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	type dispatchResult struct {
		sent bool
		err  error
	}
	finished := make(chan dispatchResult, 1)
	go func() {
		sent, err := f.events.DispatchOne(ctx, key, func(_ context.Context, event *model.BusinessEvent) error {
			observed <- model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
			<-release
			return nil
		})
		finished <- dispatchResult{sent, err}
	}()
	select {
	case got := <-observed:
		require.Equal(t, key, got, "collector must receive the retained occurrence while the deletion barrier is held")
	case got := <-finished:
		t.Fatalf("dispatch ended before synchronously invoking the collector: %+v", got)
	case <-ctx.Done():
		t.Fatal("dispatch never reached the synchronous collector")
	}

	readCtx, stopRead := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stopRead()
	owner, err := f.transactions.BeginTX(readCtx)
	require.NoError(t, err, "collector I/O must not hold the exclusive visibility gate")
	row, err := f.events.Get(owner, otherKey)
	require.NoError(t, err)
	require.Equal(t, otherKey.ID, row.ID)
	require.NoError(t, f.transactions.Commit(owner))

	blockedErase, stopErase := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stopErase()
	count, err := f.events.EraseSubject(blockedErase, target)
	require.ErrorIs(t, err, context.DeadlineExceeded, "erasure cannot complete during an active collector attempt")
	require.Zero(t, count)
	blockedRetention, stopRetention := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stopRetention()
	err = f.events.ApplyRetention(blockedRetention)
	require.ErrorIs(t, err, context.DeadlineExceeded, "retention cannot finish during a collector attempt")

	secondCallbacks := make(chan struct{}, 1)
	secondCtx, stopSecond := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stopSecond()
	sent, err := f.events.DispatchOne(secondCtx, key, func(context.Context, *model.BusinessEvent) error {
		secondCallbacks <- struct{}{}
		return nil
	})
	require.NoError(t, err)
	require.False(t, sent, "the reference is already claimed by the first dispatcher")
	require.Empty(t, secondCallbacks, "a claimed reference must not invoke another collector")

	close(release)
	released = true
	select {
	case got := <-finished:
		require.NoError(t, got.err)
		require.True(t, got.sent)
	case <-ctx.Done():
		t.Fatal("dispatch did not acknowledge after the collector returned")
	}
	f.assertRetained(t, key, false)
	f.assertRetained(t, otherKey, true)
	finalCtx, stopFinal := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopFinal()
	count, err = f.events.EraseSubject(finalCtx, target)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	f.assertDeleted(t, key)
	f.assertRetained(t, otherKey, true)
}

func TestMemoryBusinessEventCollectorFailureOrCancellationReleasesClaim(t *testing.T) {
	for _, cancelAttempt := range []bool{false, true} {
		name := "collector failure"
		if cancelAttempt {
			name = "cancellation"
		}
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
			f := newMemoryLifecycleFixture(t, at)
			target := id.Principal("principal-example")
			key := f.append(t, context.Background(), "grant-created", &target, at, true)
			ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			attempt, cancel := context.WithCancel(ctx)
			defer cancel()
			failure := errors.New("collector unavailable")
			observed := make(chan id.BusinessEventID, 1)
			release := make(chan struct{})
			released := false
			defer func() {
				if !released {
					close(release)
				}
			}()
			type dispatchResult struct {
				sent bool
				err  error
			}
			finished := make(chan dispatchResult, 1)
			go func() {
				sent, err := f.events.DispatchOne(attempt, key, func(callbackCtx context.Context, event *model.BusinessEvent) error {
					observed <- event.ID
					select {
					case <-release:
						if err := callbackCtx.Err(); err != nil {
							return err
						}
						return failure
					case <-callbackCtx.Done():
						return callbackCtx.Err()
					}
				})
				finished <- dispatchResult{sent, err}
			}()
			select {
			case got := <-observed:
				require.Equal(t, key.ID, got)
			case got := <-finished:
				t.Fatalf("dispatch ended without an actual collector attempt: %+v", got)
			case <-ctx.Done():
				t.Fatal("dispatch never reached the collector")
			}

			secondCallbacks := make(chan struct{}, 1)
			secondCtx, stopSecond := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer stopSecond()
			sent, err := f.events.DispatchOne(secondCtx, key, func(context.Context, *model.BusinessEvent) error {
				secondCallbacks <- struct{}{}
				return nil
			})
			require.NoError(t, err)
			require.False(t, sent)
			require.Empty(t, secondCallbacks)
			if cancelAttempt {
				cancel()
			} else {
				close(release)
				released = true
			}
			select {
			case got := <-finished:
				require.False(t, got.sent)
				if cancelAttempt {
					require.ErrorIs(t, got.err, context.Canceled)
				} else {
					require.ErrorIs(t, got.err, failure)
				}
			case <-ctx.Done():
				t.Fatal("failed or canceled dispatch did not release its claim")
			}
			f.assertRetained(t, key, true)
			retryCtx, stopRetry := context.WithTimeout(context.Background(), 2*time.Second)
			defer stopRetry()
			f.clock = at.Add(30*time.Second - time.Microsecond)
			due, err := f.events.ListDue(retryCtx, 10)
			require.NoError(t, err)
			require.NotContains(t, due, key, "a failed attempt waits for its thirty-second retry deadline")
			f.clock = at.Add(30 * time.Second)
			due, err = f.events.ListDue(retryCtx, 10)
			require.NoError(t, err)
			require.Contains(t, due, key, "cancellation and failure both release the in-process claim")

			var retryID id.BusinessEventID
			sent, err = f.events.DispatchOne(retryCtx, key, func(_ context.Context, event *model.BusinessEvent) error {
				retryID = event.ID
				return nil
			})
			require.NoError(t, err)
			require.True(t, sent)
			require.Equal(t, key.ID, retryID, "retry must use the retained event ID, not record a new occurrence")
			f.assertRetained(t, key, false)
		})
	}
}

func TestMemoryBusinessEventDeletedReferenceCannotDispatch(t *testing.T) {
	for _, erase := range []bool{true, false} {
		name := "retention"
		if erase {
			name = "erasure"
		}
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
			f := newMemoryLifecycleFixture(t, at)
			target := id.Principal("principal-example")
			recorded := at.Add(-91 * 24 * time.Hour)
			if erase {
				recorded = at
			}
			key := f.append(t, context.Background(), "grant-created", &target, recorded, true)
			f.clock = at
			if erase {
				count, err := f.events.EraseSubject(context.Background(), target)
				require.NoError(t, err)
				require.EqualValues(t, 1, count)
			} else {
				require.NoError(t, f.events.ApplyRetention(context.Background()))
			}
			f.assertDeleted(t, key)
			due, err := f.events.ListDue(context.Background(), 10)
			require.NoError(t, err)
			require.NotContains(t, due, key)
			called := false
			sent, err := f.events.DispatchOne(context.Background(), key, func(context.Context, *model.BusinessEvent) error {
				called = true
				return nil
			})
			require.NoError(t, err)
			require.False(t, sent, "deleted history is never re-exported")
			require.False(t, called, "a deleted event cannot reach a collector")
			require.Empty(t, f.query(t, model.BusinessEventSubject{Principal: target}))
		})
	}
}
