//go:build integration

package storage_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sync"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

var lifecycleFixtureRegistry = sync.OnceValues(func() (*ledger.Registry, error) {
	return ledger.NewRegistry(eventschemas.Schemas)
})

// Historical rows are inserted only by the migration owner, through the same
// published registry and partition-pair function used by the production schema.
func seedLifecycleHistory(t *testing.T, db *sqlx.DB, event *model.BusinessEvent, pending bool) {
	t.Helper()
	registry, err := lifecycleFixtureRegistry()
	require.NoError(t, err)
	event.RecordedAt = event.RecordedAt.UTC().Truncate(time.Microsecond)
	event.OccurredAt = event.OccurredAt.UTC().Truncate(time.Microsecond)
	wire, err := registry.Validate(event)
	require.NoError(t, err, "historical fixture must pass the published registry")
	envelope, err := json.Marshal(wire)
	require.NoError(t, err)
	ctx := context.Background()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `SELECT public.business_event_create_partition_pair($1)`, event.RecordedAt.Truncate(6*time.Hour))
	require.NoError(t, err)
	var subject any
	if event.Subject != nil {
		subject = event.Subject.String()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, event.RecordedAt, event.ID, event.OccurredAt,
		event.Type, event.Outcome, subject, envelope)
	require.NoError(t, err)
	if pending {
		_, err = tx.ExecContext(ctx, `INSERT INTO public.business_event_delivery_pending(recorded_at,event_id,next_attempt_at)
			VALUES ($1,$2,clock_timestamp())`, event.RecordedAt, event.ID)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
}

func lifecyclePair(lower time.Time) (string, string) {
	suffix := lower.UTC().Format("20060102_15")
	return "public.business_events_" + suffix, "public.business_event_delivery_pending_" + suffix
}

func lifecyclePairExists(t *testing.T, db *sqlx.DB, lower time.Time) bool {
	t.Helper()
	eventPartition, referencePartition := lifecyclePair(lower)
	var exists bool
	require.NoError(t, db.Get(&exists, `SELECT to_regclass($1) IS NOT NULL AND to_regclass($2) IS NOT NULL
		AND EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid=to_regclass($1) AND inhparent='public.business_events'::regclass)
		AND EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid=to_regclass($2) AND inhparent='public.business_event_delivery_pending'::regclass)`, eventPartition, referencePartition))
	return exists
}

func lifecycleCounts(t *testing.T, db *sqlx.DB, event *model.BusinessEvent) (int, int) {
	t.Helper()
	var events, references int
	require.NoError(t, db.Get(&events, `SELECT count(*) FROM public.business_events WHERE recorded_at=$1 AND id=$2`, event.RecordedAt, event.ID))
	require.NoError(t, db.Get(&references, `SELECT count(*) FROM public.business_event_delivery_pending WHERE recorded_at=$1 AND event_id=$2`, event.RecordedAt, event.ID))
	return events, references
}

func waitForLifecycleAdvisoryBlock[T any](t *testing.T, db *sqlx.DB, pid int, class int32, minimum int, completed <-chan T) {
	t.Helper()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-completed:
			t.Fatalf("operation completed instead of waiting on advisory class %d: %v", class, result)
		case <-timer.C:
			t.Fatalf("operation did not reach advisory class %d before deadline", class)
		case <-ticker.C:
			var waiting int
			err := db.Get(&waiting, `SELECT count(*) FROM pg_locks
				WHERE ($1=0 OR pid=$1) AND locktype='advisory' AND classid::text=$2 AND NOT granted`, pid, fmt.Sprint(class))
			require.NoError(t, err)
			if waiting >= minimum {
				return
			}
		}
	}
}

func TestBusinessEventLifecycle_EraseExactSubjectAcrossPartitionsAndReferences(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	target := id.NewPrincipal("erase-target' OR true --")
	other := id.NewPrincipal("erase-target' OR true -- suffix")
	current := ledgerExample(t, "token-issued", target)
	otherFamily := ledgerExample(t, "token-request-failed", target)
	unaffected := ledgerExample(t, "token-issued", other)
	actorID := target.String()
	unaffected.Actor.ID = &actorID // Actor attribution must not turn another subject into the erased subject.
	nullSubject := ledgerExample(t, "agent-registered", "")
	for _, event := range []*model.BusinessEvent{current, otherFamily, unaffected, nullSubject} {
		require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
	}
	old := ledgerExample(t, "token-issued", target)
	old.RecordedAt = time.Now().UTC().Truncate(6 * time.Hour).Add(-48 * time.Hour).Add(time.Minute)
	old.OccurredAt = old.RecordedAt
	seedLifecycleHistory(t, db, old, true)

	for _, selection := range []any{nil, "", "  "} {
		var count int64
		err := db.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1::text)`, selection).Scan(&count)
		require.Error(t, err, "an absent or blank selector may not erase every subject")
	}
	var deleted int64
	require.NoError(t, db.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1)`, target.String()).Scan(&deleted))
	require.Equal(t, int64(3), deleted, "erase by exact subject over every retained partition and event family")
	for _, event := range []*model.BusinessEvent{current, otherFamily, old} {
		events, references := lifecycleCounts(t, db, event)
		require.Zero(t, events)
		require.Zero(t, references, "a deleted event must not leave a replayable delivery reference")
	}
	for _, event := range []*model.BusinessEvent{unaffected, nullSubject} {
		events, references := lifecycleCounts(t, db, event)
		require.Equal(t, 1, events, "other subjects, including null, must remain")
		require.Equal(t, 1, references)
	}
	require.NoError(t, db.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1)`, target.String()).Scan(&deleted))
	require.Zero(t, deleted, "a repeated erase is successful and has no replacement subject event")
	after := ledgerExample(t, "token-issued", target)
	require.NoError(t, adapter.BusinessEvents().Append(ctx, after, false))
	events, _ := lifecycleCounts(t, db, after)
	require.Equal(t, 1, events, "erasure is not a permanent identity tombstone")
	deleted, err := adapter.BusinessEventLifecycle().EraseSubject(ctx, target)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted, "repository erasure uses the same committed exact-subject operation")
	events, _ = lifecycleCounts(t, db, after)
	require.Zero(t, events)
	deleted, err = adapter.BusinessEventLifecycle().EraseSubject(ctx, target)
	require.NoError(t, err)
	require.Zero(t, deleted)
}

func TestBusinessEventLifecycle_ErasureRejectsSnapshotIsolation(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	for _, isolation := range []struct {
		name string
		sql  sql.IsolationLevel
		hint ports.StorageTransactionIsolation
	}{
		{name: "repeatable read", sql: sql.LevelRepeatableRead, hint: ports.StorageRepeatableRead},
		{name: "serializable", sql: sql.LevelSerializable, hint: ports.StorageSerializable},
	} {
		t.Run(isolation.name, func(t *testing.T) {
			subject := id.NewPrincipal("isolation-" + isolation.name)
			operational, err := db.BeginTxx(ctx, &sql.TxOptions{Isolation: isolation.sql})
			require.NoError(t, err)
			defer func() { _ = operational.Rollback() }()
			var previous int
			require.NoError(t, operational.Get(&previous, `SELECT count(*) FROM public.business_events WHERE subject=$1`, subject.String()))
			event := ledgerExample(t, "token-issued", subject)
			require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
			var deleted int64
			err = operational.Get(&deleted, `SELECT public.business_event_erase_subject($1)`, subject.String())
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr, "an operational snapshot must not silently miss a committed predecessor")
			require.Equal(t, "25001", pgErr.Code)
			require.NoError(t, operational.Rollback())

			strong := ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{Isolation: isolation.hint})
			deleted, err = adapter.BusinessEventLifecycle().EraseSubject(strong, subject)
			require.Error(t, err, "erasure cannot start a transaction with a fixed snapshot")
			require.Zero(t, deleted)
			owner, err := adapter.BeginTX(strong)
			require.NoError(t, err)
			defer func() { _ = adapter.Rollback(owner) }()
			// Weaker hints do not weaken the isolation of the existing owner.
			joined := ports.WithStorageTransactionHints(owner, ports.StorageTransactionHints{})
			deleted, err = adapter.BusinessEventLifecycle().EraseSubject(joined, subject)
			require.Error(t, err, "the actual ambient owner, not only its current hints, must be checked")
			require.Zero(t, deleted)
			require.NoError(t, adapter.Rollback(owner))
			events, references := lifecycleCounts(t, db, event)
			require.Equal(t, 1, events)
			require.Equal(t, 1, references)
		})
	}
}

func TestBusinessEventLifecycle_OperationalFunctionsRequireArmedDeadline(t *testing.T) {
	_, db := openLedgerFoundation(t)
	for _, query := range []string{
		`SELECT public.business_event_erase_subject('deadline-precondition')`,
		`SELECT public.business_event_maintain_partitions()`,
	} {
		for _, timeout := range []string{"0", "31s"} {
			t.Run(query+"/"+timeout, func(t *testing.T) {
				tx, err := db.BeginTxx(context.Background(), nil)
				require.NoError(t, err)
				defer func() { _ = tx.Rollback() }()
				_, err = tx.Exec(`SELECT set_config('statement_timeout',$1,true)`, timeout)
				require.NoError(t, err)
				_, err = tx.Exec(query)
				var pgErr *pgconn.PgError
				require.ErrorAs(t, err, &pgErr)
				require.Equal(t, "55000", pgErr.Code, "unsafe operational sessions must fail before taking gates")
			})
		}
	}
}

func TestBusinessEventLifecycle_OperationalStatementDeadlineBoundsNonLockWork(t *testing.T) {
	for _, operation := range []string{"erase", "maintenance"} {
		t.Run(operation, func(t *testing.T) {
			adapter, db := openLedgerFoundation(t)
			subject := id.NewPrincipal("non-lock-deadline")
			event := ledgerExample(t, "token-issued", subject)
			query := `SELECT public.business_event_erase_subject('non-lock-deadline')`
			if operation == "erase" {
				require.NoError(t, adapter.BusinessEvents().Append(context.Background(), event, true))
				_, err := db.Exec(`CREATE FUNCTION public.delay_ledger_delete() RETURNS trigger LANGUAGE plpgsql AS $$
					BEGIN PERFORM pg_sleep(2); RETURN OLD; END; $$;
					CREATE TRIGGER delay_ledger_delete BEFORE DELETE ON public.business_events
					FOR EACH ROW EXECUTE FUNCTION public.delay_ledger_delete()`)
				require.NoError(t, err)
			} else {
				event.RecordedAt = time.Now().UTC().Truncate(6 * time.Hour).Add(-100*24*time.Hour + time.Minute)
				event.OccurredAt = event.RecordedAt
				seedLifecycleHistory(t, db, event, true)
				_, err := db.Exec(`CREATE FUNCTION public.delay_ledger_drop() RETURNS event_trigger LANGUAGE plpgsql AS $$
					BEGIN PERFORM pg_sleep(2); END; $$;
					CREATE EVENT TRIGGER delay_ledger_drop ON ddl_command_start WHEN TAG IN ('DROP TABLE')
					EXECUTE FUNCTION public.delay_ledger_drop()`)
				require.NoError(t, err)
				query = `SELECT public.business_event_maintain_partitions()`
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tx, err := db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, `SET LOCAL statement_timeout = '100ms'`)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, query)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, "57014", pgErr.Code, "server-side statement deadline must interrupt non-lock work")
			require.NoError(t, ctx.Err(), "the server deadline must fire before the client context")
			require.NoError(t, tx.Rollback())
			events, references := lifecycleCounts(t, db, event)
			require.Equal(t, 1, events, "deadline rollback preserves event history")
			require.Equal(t, 1, references, "deadline rollback restores delivery references deleted first")
		})
	}
}

func TestBusinessEventLifecycle_EraseWaitsForPredecessorAndProtectsLaterWriter(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	subject := id.NewPrincipal("erase-order-subject")
	before := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(ctx, before, true))
	owner, err := adapter.BeginTX(ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{
		Subjects: []ports.StorageSubjectGate{{Principal: subject}},
	}))
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	predecessor := ledgerExample(t, "token-request-failed", subject)
	require.NoError(t, adapter.BusinessEvents().Append(owner, predecessor, true))
	events, _ := lifecycleCounts(t, db, predecessor)
	require.Zero(t, events, "predecessor is still uncommitted while holding the subject shared gate")

	eraser, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = eraser.Rollback() }()
	var eraserPID int
	require.NoError(t, eraser.QueryRowxContext(ctx, `SELECT pg_backend_pid()`).Scan(&eraserPID))
	type eraseResult struct {
		count int64
		err   error
	}
	erased := make(chan eraseResult, 1)
	go func() {
		var result eraseResult
		result.err = eraser.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1)`, subject.String()).Scan(&result.count)
		if result.err == nil {
			result.err = eraser.Commit()
		}
		erased <- result
	}()
	waitForLifecycleAdvisoryBlock(t, db, eraserPID, 1095320147, 1, erased)
	subjectHash := fnv.New32a()
	_, err = subjectHash.Write([]byte(subject.String()))
	require.NoError(t, err)
	var sharedHolder, exclusiveWaiter int
	require.NoError(t, db.Get(&sharedHolder, `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND objsubid=2
		AND classid::text=$1 AND objid::text=$2 AND mode='ShareLock' AND granted`, fmt.Sprint(1095320147), fmt.Sprint(subjectHash.Sum32())))
	require.NoError(t, db.Get(&exclusiveWaiter, `SELECT count(*) FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND objsubid=2
		AND classid::text=$2 AND objid::text=$3 AND mode='ExclusiveLock' AND NOT granted`, eraserPID, fmt.Sprint(1095320147), fmt.Sprint(subjectHash.Sum32())))
	require.Equal(t, 1, sharedHolder, "recording holds the FNV-1a subject gate")
	require.Equal(t, 1, exclusiveWaiter, "erasure must wait on that same two-int FNV-1a advisory key")
	var lifecycleHeld bool
	require.NoError(t, db.Get(&lifecycleHeld, `SELECT EXISTS (SELECT 1 FROM pg_locks
		WHERE pid=$1 AND locktype='advisory' AND classid::text=$2 AND granted AND mode='ShareLock')`, eraserPID, fmt.Sprint(1095320140)))
	require.True(t, lifecycleHeld, "erasure takes lifecycle shared before subject exclusive")
	require.NoError(t, adapter.Commit(owner))
	select {
	case outcome := <-erased:
		require.NoError(t, outcome.err)
		require.Equal(t, int64(2), outcome.count, "a fresh post-lock snapshot must see the committed predecessor")
	case <-ctx.Done():
		t.Fatal("erasure did not complete after the predecessor committed")
	}
	for _, event := range []*model.BusinessEvent{before, predecessor} {
		events, references := lifecycleCounts(t, db, event)
		require.Zero(t, events)
		require.Zero(t, references)
	}

	boundary, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = boundary.Rollback() }()
	var count int64
	require.NoError(t, boundary.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1)`, subject.String()).Scan(&count))
	require.Zero(t, count)
	later := make([]*model.BusinessEvent, 4)
	inserted := make(chan error, len(later))
	for i := range later {
		later[i] = ledgerExample(t, "token-issued", subject)
		event := later[i]
		go func() { inserted <- adapter.BusinessEvents().Append(ctx, event, true) }()
	}
	waitForLifecycleAdvisoryBlock(t, db, 0, 1095320147, len(later), inserted)
	require.NoError(t, boundary.Commit())
	for range later {
		select {
		case err := <-inserted:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("the new occurrences did not resume after the erasure commit")
		}
	}
	for _, event := range later {
		events, references := lifecycleCounts(t, db, event)
		require.Equal(t, 1, events)
		require.Equal(t, 1, references, "post-boundary recording remains legitimate")
	}
}

func TestBusinessEventLifecycle_ErasureHoldsGatesBeforeDeliveryRowLock(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	subject := id.NewPrincipal("erase-row-lock-subject")
	event := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
	rowOwner, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = rowOwner.Rollback() }()
	var eventID id.BusinessEventID
	require.NoError(t, rowOwner.QueryRowxContext(ctx, `SELECT event_id FROM public.business_event_delivery_pending
		WHERE recorded_at=$1 AND event_id=$2 FOR UPDATE`, event.RecordedAt, event.ID).Scan(&eventID))
	require.Equal(t, event.ID, eventID)
	eraser, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = eraser.Rollback() }()
	var eraserPID int
	require.NoError(t, eraser.QueryRowxContext(ctx, `SELECT pg_backend_pid()`).Scan(&eraserPID))
	type result struct {
		count int64
		err   error
	}
	completed := make(chan result, 1)
	go func() {
		var outcome result
		outcome.err = eraser.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1)`, subject.String()).Scan(&outcome.count)
		if outcome.err == nil {
			outcome.err = eraser.Commit()
		}
		completed <- outcome
	}()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waitingForRow := false
	for !waitingForRow {
		select {
		case outcome := <-completed:
			t.Fatalf("erasure bypassed held delivery row: %v", outcome)
		case <-timer.C:
			t.Fatal("erasure did not reach the held delivery row")
		case <-ticker.C:
			require.NoError(t, db.Get(&waitingForRow, `SELECT EXISTS(SELECT 1 FROM pg_locks
				WHERE pid=$1 AND locktype IN ('tuple','transactionid') AND NOT granted)`, eraserPID))
		}
	}
	var held int
	require.NoError(t, db.Get(&held, `SELECT count(*) FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND granted
		AND ((classid::text=$2 AND mode='ShareLock') OR (classid::text=$3 AND mode='ExclusiveLock'))`, eraserPID,
		fmt.Sprint(1095320140), fmt.Sprint(1095320147)))
	require.Equal(t, 2, held, "lifecycle and subject gates are acquired before reference-row deletion")
	require.NoError(t, rowOwner.Commit())
	select {
	case outcome := <-completed:
		require.NoError(t, outcome.err)
		require.Equal(t, int64(1), outcome.count)
	case <-ctx.Done():
		t.Fatal("erasure did not complete when the delivery row lock released")
	}
	events, references := lifecycleCounts(t, db, event)
	require.Zero(t, events)
	require.Zero(t, references)
}

func TestBusinessEventLifecycle_RetentionDropsOnlyWholeSixHourPairs(t *testing.T) {
	for _, test := range []struct {
		name      string
		retention time.Duration
		initial   bool
	}{
		{name: "default ninety days", retention: 2160 * time.Hour, initial: true},
		{name: "thirty-six hours", retention: 36 * time.Hour},
		{name: "short positive two hours", retention: 2 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter, db := openLedgerFoundation(t)
			ctx := context.Background()
			if !test.initial {
				require.NoError(t, adapter.BusinessEventLifecycle().SetRetentionPolicy(ctx, test.retention))
			}
			var stored int64
			require.NoError(t, db.Get(&stored, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
			require.Equal(t, test.retention.Microseconds(), stored, "maintenance must use the stored common policy")
			var now time.Time
			require.NoError(t, db.Get(&now, `SELECT clock_timestamp()`))
			cutoff := now.UTC().Add(-test.retention)
			mixedLower := cutoff.Truncate(6 * time.Hour)
			require.True(t, mixedLower.Before(cutoff), "fixture needs an old and a young record in the same window")
			mixedUpper := mixedLower.Add(6 * time.Hour)
			previousLower := mixedLower.Add(-6 * time.Hour)
			subject := id.NewPrincipal("retention-" + test.name)
			makeHistory := func(recorded, occurred time.Time) *model.BusinessEvent {
				t.Helper()
				event := ledgerExample(t, "token-issued", subject)
				event.RecordedAt, event.OccurredAt = recorded, occurred
				seedLifecycleHistory(t, db, event, true)
				return event
			}
			expired := makeHistory(mixedLower.Add(-time.Microsecond), mixedLower.Add(-time.Microsecond))
			mixedOld := makeHistory(mixedLower, mixedLower)
			mixedYoungAt := cutoff.Add(mixedUpper.Sub(cutoff) / 2).Truncate(time.Microsecond)
			require.True(t, mixedYoungAt.After(cutoff))
			mixedYoung := makeHistory(mixedYoungAt, mixedYoungAt)
			youngRecording := makeHistory(now.UTC().Add(-test.retention/2).Truncate(time.Microsecond), previousLower.Add(-48*time.Hour))
			require.True(t, youngRecording.OccurredAt.Before(cutoff))
			require.True(t, youngRecording.RecordedAt.After(cutoff), "age uses recorded_at, not occurred_at")
			require.True(t, lifecyclePairExists(t, db, previousLower))
			require.True(t, lifecyclePairExists(t, db, mixedLower))
			_, err := db.ExecContext(ctx, `SELECT public.business_event_maintain_partitions()`)
			require.NoError(t, err)
			require.False(t, lifecyclePairExists(t, db, previousLower), "both old physical partitions must drop")
			require.True(t, lifecyclePairExists(t, db, mixedLower), "no event may disappear early within a mixed window")
			for _, row := range []struct {
				event *model.BusinessEvent
				want  int
			}{{expired, 0}, {mixedOld, 1}, {mixedYoung, 1}, {youngRecording, 1}} {
				events, references := lifecycleCounts(t, db, row.event)
				require.Equal(t, row.want, events)
				require.Equal(t, row.want, references, "event/reference partitions stay paired")
			}
		})
	}
}

func TestBusinessEventLifecycle_ProvisionBeforePolicyIncreaseKeepsHistory(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	require.NoError(t, adapter.BusinessEventLifecycle().SetRetentionPolicy(ctx, 720*time.Hour))
	old := ledgerExample(t, "token-issued", id.NewPrincipal("increase-preserves-history"))
	old.RecordedAt = time.Now().UTC().Truncate(6 * time.Hour).Add(-45*24*time.Hour + time.Minute)
	old.OccurredAt = old.RecordedAt
	seedLifecycleHistory(t, db, old, true)
	partitionLower := old.RecordedAt.Truncate(6 * time.Hour)
	var originalPolicy int64
	require.NoError(t, db.Get(&originalPolicy, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
	require.Equal(t, (720 * time.Hour).Microseconds(), originalPolicy)
	_, err := db.ExecContext(ctx, `SELECT public.business_event_provision_partitions()`)
	require.NoError(t, err, "pre-upgrade provisioning runs before the new broker stores its longer duration")
	require.NoError(t, db.Get(&originalPolicy, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
	require.Equal(t, (720 * time.Hour).Microseconds(), originalPolicy, "provisioning must not substitute the incoming retention policy")
	require.True(t, lifecyclePairExists(t, db, partitionLower))
	events, references := lifecycleCounts(t, db, old)
	require.Equal(t, 1, events)
	require.Equal(t, 1, references)
	require.NoError(t, adapter.BusinessEventLifecycle().SetRetentionPolicy(ctx, 2160*time.Hour))
	_, err = db.ExecContext(ctx, `SELECT public.business_event_maintain_partitions()`)
	require.NoError(t, err)
	require.True(t, lifecyclePairExists(t, db, partitionLower))
	events, references = lifecycleCounts(t, db, old)
	require.Equal(t, 1, events, "history aged 30-90 days must survive the coordinated increase")
	require.Equal(t, 1, references)
}

func TestBusinessEventLifecycle_SubMicrosecondPolicyRoundsUp(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	require.NoError(t, adapter.BusinessEventLifecycle().SetRetentionPolicy(context.Background(), time.Nanosecond))
	var stored int64
	require.NoError(t, db.Get(&stored, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
	require.Equal(t, int64(1), stored, "a positive duration may never normalize to immediate deletion")
	current := ledgerExample(t, "token-issued", id.NewPrincipal("one-microsecond-window"))
	require.NoError(t, adapter.BusinessEvents().Append(context.Background(), current, true))
	_, err := db.Exec(`SELECT public.business_event_maintain_partitions()`)
	require.NoError(t, err)
	events, references := lifecycleCounts(t, db, current)
	require.Equal(t, 1, events, "the current six-hour window is not wholly expired")
	require.Equal(t, 1, references)
}

func TestBusinessEventLifecycle_MaintenanceAndPolicySerializeWithRecording(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	owner, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	pending := ledgerExample(t, "token-issued", id.NewPrincipal("policy-lock-subject"))
	require.NoError(t, adapter.BusinessEvents().Append(owner, pending, false))
	changed := make(chan error, 1)
	go func() { changed <- adapter.BusinessEventLifecycle().SetRetentionPolicy(ctx, 36*time.Hour) }()
	waitForLifecycleAdvisoryBlock(t, db, 0, 1095320140, 1, changed)
	var stored int64
	require.NoError(t, db.Get(&stored, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
	require.Equal(t, (2160 * time.Hour).Microseconds(), stored, "policy read cannot precede the lifecycle-exclusive barrier")
	require.NoError(t, adapter.Commit(owner))
	select {
	case err := <-changed:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("policy update did not resume after recording commit")
	}
	require.NoError(t, db.Get(&stored, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
	require.Equal(t, (36 * time.Hour).Microseconds(), stored)

	shared, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(shared) }()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var maintenancePID int
	require.NoError(t, tx.QueryRowxContext(ctx, `SELECT pg_backend_pid()`).Scan(&maintenancePID))
	maintained := make(chan error, 1)
	go func() {
		_, err := tx.ExecContext(ctx, `SELECT public.business_event_maintain_partitions()`)
		if err == nil {
			err = tx.Commit()
		}
		maintained <- err
	}()
	waitForLifecycleAdvisoryBlock(t, db, maintenancePID, 1095320140, 1, maintained)
	require.NoError(t, adapter.Commit(shared))
	select {
	case err := <-maintained:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("maintenance did not resume after the shared lifecycle gate released")
	}
}

func TestBusinessEventLifecycle_MaintenanceRejectsPolicyAndMetadataCorruption(t *testing.T) {
	for _, corruption := range []string{"missing policy", "zero policy", "unpaired partition"} {
		t.Run(corruption, func(t *testing.T) {
			_, db := openLedgerFoundation(t)
			old := ledgerExample(t, "token-issued", id.NewPrincipal("corrupt-maintenance-"+corruption))
			old.RecordedAt = time.Now().UTC().Truncate(6 * time.Hour).Add(-100*24*time.Hour + time.Minute)
			old.OccurredAt = old.RecordedAt
			seedLifecycleHistory(t, db, old, true)
			lower := old.RecordedAt.Truncate(6 * time.Hour)
			switch corruption {
			case "missing policy":
				_, err := db.Exec(`DELETE FROM public.business_event_policy WHERE singleton`)
				require.NoError(t, err)
			case "zero policy":
				_, err := db.Exec(`ALTER TABLE public.business_event_policy DROP CONSTRAINT business_event_policy_retention_microseconds_check`)
				require.NoError(t, err)
				_, err = db.Exec(`UPDATE public.business_event_policy SET retention_microseconds=0 WHERE singleton`)
				require.NoError(t, err)
			case "unpaired partition":
				_, reference := lifecyclePair(lower)
				_, err := db.Exec(`ALTER TABLE public.business_event_delivery_pending DETACH PARTITION ` + reference)
				require.NoError(t, err)
			}
			_, err := db.Exec(`SELECT public.business_event_maintain_partitions()`)
			require.Error(t, err, "maintenance must fail closed rather than drop history with corrupt metadata or policy")
			eventPartition, referencePartition := lifecyclePair(lower)
			var stillExists bool
			require.NoError(t, db.Get(&stillExists, `SELECT to_regclass($1) IS NOT NULL AND to_regclass($2) IS NOT NULL`, eventPartition, referencePartition))
			require.True(t, stillExists, "a failed sweep cannot partially drop a physical pair")
			var retained int
			require.NoError(t, db.Get(&retained, `SELECT count(*) FROM public.business_events WHERE recorded_at=$1 AND id=$2`, old.RecordedAt, old.ID))
			require.Equal(t, 1, retained)
		})
	}
}

func TestBusinessEventLifecycle_MaintenanceFailsAtomicallyWithoutOwnerPrivileges(t *testing.T) {
	_, db := openLedgerFoundation(t)
	old := ledgerExample(t, "token-issued", id.NewPrincipal("maintenance-permission"))
	old.RecordedAt = time.Now().UTC().Truncate(6 * time.Hour).Add(-100*24*time.Hour + time.Minute)
	old.OccurredAt = old.RecordedAt
	seedLifecycleHistory(t, db, old, true)
	_, err := db.Exec(`CREATE ROLE ledger_limited_maintainer NOLOGIN`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.Exec(`DROP OWNED BY ledger_limited_maintainer`)
		require.NoError(t, err)
		_, err = db.Exec(`DROP ROLE ledger_limited_maintainer`)
		require.NoError(t, err)
	})
	_, err = db.Exec(`GRANT USAGE ON SCHEMA public TO ledger_limited_maintainer`)
	require.NoError(t, err)
	_, err = db.Exec(`GRANT EXECUTE ON FUNCTION public.business_event_create_partition_pair(timestamptz),
		public.business_event_provision_partitions(), public.business_event_maintain_partitions() TO ledger_limited_maintainer`)
	require.NoError(t, err)
	tx, err := db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`SET LOCAL ROLE ledger_limited_maintainer`)
	require.NoError(t, err)
	_, err = tx.Exec(`SELECT public.business_event_maintain_partitions()`)
	require.Error(t, err, "SECURITY INVOKER must not let a function grant confer migration ownership")
	require.NoError(t, tx.Rollback())
	require.True(t, lifecyclePairExists(t, db, old.RecordedAt.Truncate(6*time.Hour)))
	events, references := lifecycleCounts(t, db, old)
	require.Equal(t, 1, events)
	require.Equal(t, 1, references)
}

func TestBusinessEventLifecycle_CurrentPartitionReadinessAndSchedulerCatchUp(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	require.NoError(t, adapter.HealthCheck(ctx))
	var now time.Time
	require.NoError(t, db.Get(&now, `SELECT clock_timestamp()`))
	lower := now.UTC().Truncate(6 * time.Hour)
	eventPartition, _ := lifecyclePair(lower)
	futureLower := now.UTC().Truncate(24 * time.Hour).Add(7*24*time.Hour + 18*time.Hour)
	futureEventPartition, futureReferencePartition := lifecyclePair(futureLower)
	_, err := db.Exec(`DROP TABLE ` + eventPartition)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE ` + futureEventPartition)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE ` + futureReferencePartition)
	require.NoError(t, err)
	require.False(t, lifecyclePairExists(t, db, futureLower))
	require.Error(t, adapter.HealthCheck(ctx), "readiness must detect a missing current partition even when PostgreSQL still pings")
	failed := ledgerExample(t, "token-issued", id.NewPrincipal("missing-partition"))
	require.Error(t, adapter.BusinessEvents().Append(ctx, failed, false), "runtime recording may not create or route around absent partitions")
	_, err = db.Exec(`SELECT public.business_event_maintain_partitions()`)
	require.NoError(t, err, "migration-owner scheduler restores current and future partitions")
	require.True(t, lifecyclePairExists(t, db, lower))
	require.True(t, lifecyclePairExists(t, db, futureLower), "scheduler also repairs missing future windows")
	future := ledgerExample(t, "token-issued", id.NewPrincipal("repaired-future"))
	future.RecordedAt = futureLower.Add(time.Minute)
	seedLifecycleHistory(t, db, future, true)
	futureEvents, futureReferences := lifecycleCounts(t, db, future)
	require.Equal(t, 1, futureEvents)
	require.Equal(t, 1, futureReferences)
	require.NoError(t, adapter.HealthCheck(ctx), "readiness recovers after scheduler catch-up")
	after := ledgerExample(t, "token-issued", id.NewPrincipal("after-maintenance"))
	require.NoError(t, adapter.BusinessEvents().Append(ctx, after, true))
	events, references := lifecycleCounts(t, db, after)
	require.Equal(t, 1, events)
	require.Equal(t, 1, references)
	_, currentReferencePartition := lifecyclePair(lower)
	_, err = db.Exec(`DROP TABLE ` + currentReferencePartition)
	require.NoError(t, err)
	require.Error(t, adapter.HealthCheck(ctx), "missing current delivery partition also makes storage unready")
	_, err = db.Exec(`SELECT public.business_event_provision_partitions()`)
	require.NoError(t, err)
	require.True(t, lifecyclePairExists(t, db, lower))
	require.NoError(t, adapter.HealthCheck(ctx))
}

func TestBusinessEventLifecycle_OperationalPrivileges(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	subject := id.NewPrincipal("role-erasure-target")
	recorded := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(ctx, recorded, true))
	for _, role := range []string{"ledger_runtime", "ledger_reader", "ledger_eraser"} {
		_, err := db.Exec(`CREATE ROLE ` + role + ` NOLOGIN`)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := db.Exec(`DROP OWNED BY ` + role)
			require.NoError(t, err)
			_, err = db.Exec(`DROP ROLE ` + role)
			require.NoError(t, err)
		})
		_, err = db.Exec(`GRANT USAGE ON SCHEMA public TO ` + role)
		require.NoError(t, err)
	}
	_, err := db.Exec(`GRANT SELECT,INSERT ON public.business_events TO ledger_runtime`)
	require.NoError(t, err)
	_, err = db.Exec(`GRANT SELECT,INSERT,UPDATE,DELETE ON public.business_event_delivery_pending TO ledger_runtime`)
	require.NoError(t, err)
	_, err = db.Exec(`GRANT SELECT,UPDATE ON public.business_event_policy TO ledger_runtime`)
	require.NoError(t, err)
	_, err = db.Exec(`GRANT SELECT ON public.business_events, public.business_event_delivery_pending TO ledger_reader`)
	require.NoError(t, err)
	_, err = db.Exec(`GRANT EXECUTE ON FUNCTION public.business_event_erase_subject(text) TO ledger_eraser`)
	require.NoError(t, err)

	asRole := func(role string, check func(*sqlx.Tx)) {
		t.Helper()
		tx, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.Exec(`SET LOCAL ROLE ` + role)
		require.NoError(t, err)
		check(tx)
		require.NoError(t, tx.Commit())
	}
	asRole("ledger_runtime", func(tx *sqlx.Tx) {
		var count int
		require.NoError(t, tx.Get(&count, `SELECT count(*) FROM public.business_events WHERE id=$1`, recorded.ID))
		require.Equal(t, 1, count)
	})
	copied := *recorded
	copied.ID = id.NewBusinessEventID()
	asRole("ledger_runtime", func(tx *sqlx.Tx) {
		_, err := tx.Exec(`INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
			SELECT recorded_at,$1::uuid,occurred_at,type,outcome,subject,
				jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
			FROM public.business_events WHERE recorded_at=$2 AND id=$3`, copied.ID, recorded.RecordedAt, recorded.ID)
		require.NoError(t, err, "runtime INSERT through a partition parent remains possible")
		_, err = tx.Exec(`INSERT INTO public.business_event_delivery_pending(recorded_at,event_id,next_attempt_at)
			VALUES($1,$2,clock_timestamp())`, copied.RecordedAt, copied.ID)
		require.NoError(t, err, "runtime can queue a payload-free delivery reference")
		_, err = tx.Exec(`UPDATE public.business_event_policy SET retention_microseconds=$1 WHERE singleton`, (36 * time.Hour).Microseconds())
		require.NoError(t, err, "startup may persist the validated policy under the lifecycle gate")
	})
	events, references := lifecycleCounts(t, db, &copied)
	require.Equal(t, 1, events)
	require.Equal(t, 1, references)
	// Rejected SQL aborts a transaction, so each denied operation needs a new role scope.
	for _, query := range []string{
		`DELETE FROM public.business_events WHERE id='00000000-0000-0000-0000-000000000000'`,
		`UPDATE public.business_events SET outcome=outcome WHERE false`,
		`SELECT public.business_event_erase_subject('role-erasure-target')`,
		`SELECT public.business_event_maintain_partitions()`,
		`SELECT public.business_event_provision_partitions()`,
		`SELECT public.business_event_create_partition_pair(date_bin(interval '6 hours',clock_timestamp(),timestamptz '2000-01-01 00:00:00+00'))`,
	} {
		tx, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		_, err = tx.Exec(`SET LOCAL ROLE ledger_runtime`)
		require.NoError(t, err)
		_, err = tx.Exec(query)
		require.Error(t, err, "runtime may not mutate ledger history or obtain partition DDL capabilities")
		require.NoError(t, tx.Rollback())
	}
	eventChild, referenceChild := lifecyclePair(recorded.RecordedAt.Truncate(6 * time.Hour))
	asRole("ledger_runtime", func(tx *sqlx.Tx) {
		var canInsertChild bool
		require.NoError(t, tx.Get(&canInsertChild, `SELECT has_table_privilege(current_user,$1,'INSERT')`, eventChild))
		require.False(t, canInsertChild, "runtime parent INSERT must not grant direct writes into child partitions")
		require.NoError(t, tx.Get(&canInsertChild, `SELECT has_table_privilege(current_user,$1,'INSERT')`, referenceChild))
		require.False(t, canInsertChild, "runtime must use the delivery parent, not direct child writes")
	})
	asRole("ledger_reader", func(tx *sqlx.Tx) {
		var count int
		require.NoError(t, tx.Get(&count, `SELECT count(*) FROM public.business_events WHERE id=$1`, recorded.ID))
		require.Equal(t, 1, count)
	})
	for _, query := range []string{
		`DELETE FROM public.business_events WHERE false`,
		`SELECT public.business_event_erase_subject('role-erasure-target')`,
		`SELECT public.business_event_maintain_partitions()`,
	} {
		tx, err := db.BeginTxx(ctx, nil)
		require.NoError(t, err)
		_, err = tx.Exec(`SET LOCAL ROLE ledger_reader`)
		require.NoError(t, err)
		_, err = tx.Exec(query)
		require.Error(t, err, "operational reader is SELECT only")
		require.NoError(t, tx.Rollback())
	}
	asRole("ledger_eraser", func(tx *sqlx.Tx) {
		var deleted int64
		require.NoError(t, tx.Get(&deleted, `SELECT public.business_event_erase_subject($1)`, subject.String()))
		require.Equal(t, int64(2), deleted, "the authorized function can delete without direct table DELETE privileges")
	})
	events, references = lifecycleCounts(t, db, recorded)
	require.Zero(t, events)
	require.Zero(t, references)
	events, references = lifecycleCounts(t, db, &copied)
	require.Zero(t, events)
	require.Zero(t, references)
	asRole("ledger_eraser", func(tx *sqlx.Tx) {
		var canDelete bool
		require.NoError(t, tx.Get(&canDelete, `SELECT has_table_privilege(current_user,'public.business_events','DELETE')`))
		require.False(t, canDelete)
	})
}

func TestBusinessEventLifecycle_ExpiryRecognitionMarkerSurvivesDeletion(t *testing.T) {
	for _, deletion := range []string{"erase", "retention"} {
		for _, family := range []string{"grant", "approval"} {
			t.Run(deletion+"/"+family, func(t *testing.T) {
				adapter, db := openLedgerFoundation(t)
				ctx := context.Background()
				principal := id.NewPrincipal("marker-" + deletion + "-" + family)
				agentID := id.NewAgentID()
				_, err := db.Exec(`INSERT INTO public.agents(id,display_name,description) VALUES($1,'Lifecycle agent','Fixture')`, agentID)
				require.NoError(t, err)
				expiry := time.Now().UTC().Add(-100 * 24 * time.Hour).Truncate(time.Microsecond)
				event := ledgerExample(t, family+"-expired", principal)
				event.OccurredAt = expiry
				var recognize func() (bool, error)
				var marker func() time.Time
				var referenceField, referenceID string
				if family == "grant" {
					grantID := id.NewGrantID()
					_, err = db.Exec(`INSERT INTO public.user_grants(id,principal,agent_id,valid_until,granted_permission_sets)
						VALUES($1,$2,$3,$4,'[]'::jsonb)`, grantID, principal.String(), agentID, expiry)
					require.NoError(t, err)
					event.GrantID = grantID
					referenceField, referenceID = "grant_id", grantID.String()
					recognize = func() (bool, error) {
						return adapter.UserGrants().(ports.UserGrantExpirationRepository).RecordExpiration(ctx, grantID, expiry)
					}
					marker = func() time.Time {
						var at time.Time
						require.NoError(t, db.Get(&at, `SELECT expiration_recorded_for FROM public.user_grants WHERE id=$1`, grantID))
						return at.UTC()
					}
				} else {
					approvalID := id.NewApprovalID()
					_, err = db.Exec(`INSERT INTO public.tool_approvals(id,principal,agent_id,gateway_client_id,
						tool_name,tool_pattern,arguments_hash,status,approval_url,expires_at)
						VALUES($1,$2,$3,'lifecycle-gateway','read_file','read_file',$5,'pending','https://broker.example/approval',$4)`, approvalID, principal.String(), agentID, expiry, approvalID.String())
					require.NoError(t, err)
					event.ApprovalID = approvalID
					referenceField, referenceID = "approval_id", approvalID.String()
					recognize = func() (bool, error) {
						return adapter.ToolApprovals().(ports.ToolApprovalExpirationRepository).RecordExpiration(ctx, approvalID, expiry)
					}
					marker = func() time.Time {
						var at time.Time
						require.NoError(t, db.Get(&at, `SELECT expiration_recorded_for FROM public.tool_approvals WHERE id=$1`, approvalID))
						return at.UTC()
					}
				}
				won, err := recognize()
				require.NoError(t, err)
				require.True(t, won)
				if deletion == "erase" {
					require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
					var erased int64
					require.NoError(t, db.Get(&erased, `SELECT public.business_event_erase_subject($1)`, principal.String()))
					require.Equal(t, int64(1), erased)
				} else {
					event.RecordedAt = expiry.Add(time.Hour)
					seedLifecycleHistory(t, db, event, true)
					_, err = db.Exec(`SELECT public.business_event_maintain_partitions()`)
					require.NoError(t, err)
				}
				events, references := lifecycleCounts(t, db, event)
				require.Zero(t, events)
				require.Zero(t, references)
				require.Equal(t, expiry, marker(), "recognition state belongs to the source object, not ledger history")
				won, err = recognize()
				require.NoError(t, err)
				require.False(t, won, "repeated lazy expiry must not resurrect deleted history")
				var total int
				require.NoError(t, db.Get(&total, `SELECT count(*) FROM public.business_events WHERE envelope->>$1=$2`, referenceField, referenceID))
				require.Zero(t, total)
			})
		}
	}
}

func TestBusinessEventLifecycle_ErasureIgnoresCallerTemporaryTypes(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	subject := id.NewPrincipal("temporary-type-isolation")
	event := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(ctx, event, true))
	_, err := db.ExecContext(ctx, `CREATE ROLE ledger_temp_eraser NOLOGIN;
		GRANT EXECUTE ON FUNCTION public.business_event_erase_subject(text) TO ledger_temp_eraser`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.Exec(`DROP OWNED BY ledger_temp_eraser; DROP ROLE ledger_temp_eraser`)
		require.NoError(t, err)
	})
	connection, err := db.Connx(ctx)
	require.NoError(t, err)
	defer func() {
		_, err := connection.ExecContext(context.Background(), `RESET ROLE; DROP DOMAIN IF EXISTS pg_temp.bytea; DROP TABLE IF EXISTS pg_temp.ledger_type_control`)
		require.NoError(t, err)
		require.NoError(t, connection.Close())
	}()
	_, err = connection.ExecContext(ctx, `SET ROLE ledger_temp_eraser;
		CREATE TEMP TABLE ledger_type_control (id integer);
		CREATE DOMAIN pg_temp.bytea AS pg_catalog.bytea CHECK (VALUE IS NULL)`)
	require.NoError(t, err)
	var deleted int64
	err = connection.QueryRowxContext(ctx, `SELECT public.business_event_erase_subject($1)`, subject.String()).Scan(&deleted)
	require.NoError(t, err, "caller temporary types must not alter the privileged erasure body")
	require.EqualValues(t, 1, deleted)
	events, references := lifecycleCounts(t, db, event)
	require.Zero(t, events)
	require.Zero(t, references)
}
