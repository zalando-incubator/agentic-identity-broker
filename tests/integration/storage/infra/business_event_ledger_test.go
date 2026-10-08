//go:build integration
// +build integration

package storage_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func ledgerFoundationMigrations(t *testing.T) []bootstrap.SQLMigration {
	t.Helper()
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(root, "migrations", "*.up.sql"))
	require.NoError(t, err)
	migrations := make([]bootstrap.SQLMigration, 0, len(files))
	found036 := false
	for _, file := range files {
		name := filepath.Base(file)
		version, err := strconv.ParseInt(strings.SplitN(name, "_", 2)[0], 10, 64)
		require.NoError(t, err)
		if version > 36 {
			continue
		}
		found036 = found036 || name == "036_business_event_ledger.up.sql"
		migrations = append(migrations, bootstrap.SQLMigration{File: name, Version: version})
	}
	require.True(t, found036, "the ledger must have a runnable migration 036")
	return migrations
}

func openLedgerFoundation(t *testing.T) (*storageadapter.Adapter, *sqlx.DB) {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	migrations := ledgerFoundationMigrations(t)
	_, connectionURL, drop := pg.SetupDatabaseFromTemplate(t, "business_event_foundation_036", func(name string) {
		pg.ApplyMigrationsUpTo(t, name, migrations, 36)
	})
	t.Cleanup(drop)

	owner, err := sqlx.Connect("pgx", connectionURL+"&statement_timeout=30000")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })

	adapter, err := storageadapter.NewAdapter(&ports.StorageConfig{
		Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: connectionURL},
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close(context.Background())) })
	return adapter, owner
}

// Fixtures come from the published catalogue; individual occurrences get distinct IDs and fact times.
func ledgerExample(t *testing.T, eventName string, subject id.Principal) *model.BusinessEvent {
	t.Helper()
	data, err := fs.ReadFile(eventschemas.Schemas, "examples.json")
	require.NoError(t, err)
	var examples []model.BusinessEvent
	require.NoError(t, json.Unmarshal(data, &examples))
	for i := range examples {
		if examples[i].Type != model.BusinessEventTypePrefix+eventName {
			continue
		}
		event := examples[i]
		event.ID = id.NewBusinessEventID()
		event.OccurredAt = time.Now().UTC().Truncate(time.Microsecond)
		if !subject.IsZero() {
			event.Subject = &subject
			if event.Actor.Kind == "user" {
				actorID := subject.String()
				event.Actor.ID = &actorID
			}
		}
		return &event
	}
	t.Fatalf("published example for %q is missing", eventName)
	return nil
}

func ledgerKey(event *model.BusinessEvent) model.BusinessEventKey {
	return model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
}

func TestBusinessEventMigration_UpDownUpPreservesBusinessData(t *testing.T) {
	pg := bootstrap.RequireSharedPostgres(t)
	migrations := ledgerFoundationMigrations(t)
	agentID := id.NewAgentID()
	dbName, connectionURL, drop := pg.SetupDatabaseFromTemplate(t, "business_event_before_036", func(name string) {
		pg.ApplyMigrationsUpTo(t, name, migrations, 35)
	})
	t.Cleanup(drop)
	db, err := sqlx.Connect("pgx", connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`INSERT INTO public.agents(id, client_id, display_name, description)
		VALUES ($1, $2, 'Existing agent', 'Preserved across ledger migration')`, agentID, "existing-"+agentID.String())
	require.NoError(t, err)
	grantID := id.NewGrantID()
	_, err = db.Exec(`INSERT INTO public.user_grants(id,principal,agent_id,granted_permission_sets)
		VALUES ($1,'existing-ledger-subject',$2,'[]'::jsonb)`, grantID, agentID)
	require.NoError(t, err)

	for cycle := range 2 {
		pg.ApplyMigration(t, dbName, "036_business_event_ledger.up.sql")
		var displayName string
		require.NoError(t, db.Get(&displayName, `SELECT display_name FROM public.agents WHERE id=$1`, agentID))
		require.Equal(t, "Existing agent", displayName)
		var grantSubject string
		require.NoError(t, db.Get(&grantSubject, `SELECT principal FROM public.user_grants WHERE id=$1`, grantID))
		require.Equal(t, "existing-ledger-subject", grantSubject)
		var hasLedger bool
		require.NoError(t, db.Get(&hasLedger, `SELECT to_regclass('public.business_events') IS NOT NULL
			AND to_regclass('public.business_event_delivery_pending') IS NOT NULL
			AND to_regclass('public.business_event_policy') IS NOT NULL`))
		require.True(t, hasLedger, "migration must create the event, reference and policy tables")
		if cycle == 0 {
			pg.ApplyMigration(t, dbName, "036_business_event_ledger.down.sql")
			require.NoError(t, db.Get(&hasLedger, `SELECT to_regclass('public.business_events') IS NOT NULL
				OR to_regclass('public.business_event_delivery_pending') IS NOT NULL
				OR to_regclass('public.business_event_policy') IS NOT NULL`))
			require.False(t, hasLedger, "rollback must remove only the feature's tables")
			require.NoError(t, db.Get(&displayName, `SELECT display_name FROM public.agents WHERE id=$1`, agentID))
			require.Equal(t, "Existing agent", displayName)
			require.NoError(t, db.Get(&grantSubject, `SELECT principal FROM public.user_grants WHERE id=$1`, grantID))
			require.Equal(t, "existing-ledger-subject", grantSubject)
		}
	}
}

func TestBusinessEventLedger_ProjectedColumnsAndImmutableEnvelope(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	subject := id.NewPrincipal("ledger-projection-subject")
	event := ledgerExample(t, "token-issued", subject)
	before := event.RecordedAt
	require.NoError(t, adapter.BusinessEvents().Append(ctx, event, false))
	require.False(t, event.RecordedAt.IsZero())
	require.NotEqual(t, before, event.RecordedAt, "PostgreSQL must assign the recording timestamp")
	require.Zero(t, event.RecordedAt.Nanosecond()%1000, "stored recording time must have microsecond precision")
	var queued int
	require.NoError(t, db.GetContext(ctx, &queued, `SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1`, event.ID))
	require.Zero(t, queued, "append without copying retains no delivery reference")

	var row struct {
		RecordedAt time.Time `db:"recorded_at"`
		OccurredAt time.Time `db:"occurred_at"`
		Type       string    `db:"type"`
		Outcome    string    `db:"outcome"`
		Subject    string    `db:"subject"`
		Envelope   []byte    `db:"envelope"`
	}
	require.NoError(t, db.GetContext(ctx, &row, `SELECT recorded_at, occurred_at, type, outcome, subject, envelope
		FROM public.business_events WHERE id=$1`, event.ID))
	require.Equal(t, event.RecordedAt.UTC(), row.RecordedAt.UTC())
	require.Equal(t, event.OccurredAt.UTC(), row.OccurredAt.UTC())
	require.Equal(t, event.Type, row.Type)
	require.Equal(t, string(event.Outcome), row.Outcome)
	require.Equal(t, subject.String(), row.Subject)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(row.Envelope, &envelope))
	require.Equal(t, event.ID.String(), envelope["id"])
	require.Equal(t, row.Type, envelope["type"])
	require.Equal(t, row.Outcome, envelope["outcome"])
	require.Equal(t, row.Subject, envelope["subject"])
	require.Equal(t, row.RecordedAt.UTC(), mustLedgerTimestamp(t, envelope["recorded_at"]).UTC())
	require.Equal(t, row.OccurredAt.UTC(), mustLedgerTimestamp(t, envelope["occurred_at"]).UTC())
	unaligned := ledgerExample(t, "token-issued", subject)
	unaligned.OccurredAt = unaligned.OccurredAt.Add(789 * time.Nanosecond)
	require.NoError(t, adapter.BusinessEvents().Append(ctx, unaligned, false))
	var projected, serialized time.Time
	require.NoError(t, db.QueryRowxContext(ctx, `SELECT occurred_at, (envelope->>'occurred_at')::timestamptz
		FROM public.business_events WHERE recorded_at=$1 AND id=$2`, unaligned.RecordedAt, unaligned.ID).Scan(&projected, &serialized))
	require.Zero(t, projected.Nanosecond()%1000)
	require.Equal(t, projected, serialized, "SQL and JSON must use one normalized microsecond occurrence")
	require.WithinDuration(t, unaligned.OccurredAt, projected, time.Microsecond)

	_, err := db.ExecContext(ctx, `UPDATE public.business_events SET type=type WHERE recorded_at=$1 AND id=$2`, row.RecordedAt, event.ID)
	require.Error(t, err, "even a no-op UPDATE by the migration owner must be rejected")
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		SELECT recorded_at,$1::uuid,occurred_at,type,outcome,subject,
			jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
		FROM public.business_events WHERE recorded_at=$2 AND id=$3`, id.NewBusinessEventID(), row.RecordedAt, event.ID)
	require.NoError(t, err, "an otherwise identical valid envelope must pass all projection checks")
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		SELECT recorded_at,$1::uuid,occurred_at,type||'-changed',outcome,subject,
			jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
		FROM public.business_events WHERE recorded_at=$2 AND id=$3`, id.NewBusinessEventID(), row.RecordedAt, event.ID)
	require.Error(t, err, "projected type must equal the immutable envelope type")
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		SELECT recorded_at,$1::uuid,occurred_at,type,outcome,NULL,
			jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
		FROM public.business_events WHERE recorded_at=$2 AND id=$3`, id.NewBusinessEventID(), row.RecordedAt, event.ID)
	require.Error(t, err, "SQL NULL cannot disagree with non-null JSON subject")
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		SELECT recorded_at,$1::uuid,occurred_at + interval '1 microsecond',type,outcome,subject,
			jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
		FROM public.business_events WHERE recorded_at=$2 AND id=$3`, id.NewBusinessEventID(), row.RecordedAt, event.ID)
	require.Error(t, err, "SQL occurrence time must equal the time in the envelope")

	anonymous := ledgerExample(t, "agent-registered", "")
	require.NoError(t, adapter.BusinessEvents().Append(ctx, anonymous, false))
	var subjectPresent, subjectNull bool
	require.NoError(t, db.QueryRowxContext(ctx, `SELECT envelope ? 'subject', envelope->'subject' = 'null'::jsonb
		FROM public.business_events WHERE recorded_at=$1 AND id=$2`, anonymous.RecordedAt, anonymous.ID).Scan(&subjectPresent, &subjectNull))
	require.True(t, subjectPresent, "subject must be present even when null")
	require.True(t, subjectNull, "no-subject uses JSON null rather than a missing field")
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		SELECT recorded_at,$1::uuid,occurred_at,type,outcome,'not-null',
			jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
		FROM public.business_events WHERE recorded_at=$2 AND id=$3`, id.NewBusinessEventID(), anonymous.RecordedAt, anonymous.ID)
	require.Error(t, err, "SQL non-null cannot disagree with JSON null")
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		SELECT recorded_at,$1::uuid,occurred_at,type,outcome,subject,
			jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text)) - 'subject'
		FROM public.business_events WHERE recorded_at=$2 AND id=$3`, id.NewBusinessEventID(), anonymous.RecordedAt, anonymous.ID)
	require.Error(t, err, "the envelope cannot omit its explicit nullable subject")
}

func mustLedgerTimestamp(t *testing.T, value any) time.Time {
	t.Helper()
	text, ok := value.(string)
	require.True(t, ok)
	parsed, err := time.Parse(time.RFC3339Nano, text)
	require.NoError(t, err)
	return parsed
}

func TestBusinessEventLedger_AmbientNestedOwnershipAndRollback(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	subject := id.NewPrincipal("ledger-transaction-subject")
	owner, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	outer := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(owner, outer, true))
	var outsideEvents int
	require.NoError(t, db.Get(&outsideEvents, `SELECT count(*) FROM public.business_events WHERE id=$1`, outer.ID))
	require.Zero(t, outsideEvents, "another connection cannot see an uncommitted event")

	joined, err := adapter.BeginTX(owner)
	require.NoError(t, err)
	inner := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(joined, inner, true))
	require.NoError(t, adapter.Commit(joined))
	require.NoError(t, db.Get(&outsideEvents, `SELECT count(*) FROM public.business_events WHERE id=$1`, inner.ID))
	require.Zero(t, outsideEvents, "joined commit must not physically commit the owner's event")
	require.NoError(t, adapter.Rollback(owner))
	var events, references int
	require.NoError(t, db.QueryRowx(`SELECT count(*) FROM public.business_events WHERE id IN ($1,$2)`, outer.ID, inner.ID).Scan(&events))
	require.NoError(t, db.QueryRowx(`SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id IN ($1,$2)`, outer.ID, inner.ID).Scan(&references))
	require.Zero(t, events, "owner rollback must erase both nested event writes")
	require.Zero(t, references, "reference insertion must roll back with its event")

	owner, err = adapter.BeginTX(ctx)
	require.NoError(t, err)
	joined, err = adapter.BeginTX(owner)
	require.NoError(t, err)
	poisoned := ledgerExample(t, "token-issued", subject)
	require.NoError(t, adapter.BusinessEvents().Append(joined, poisoned, true))
	require.NoError(t, adapter.Rollback(joined))
	require.Error(t, adapter.Commit(owner), "a joined rollback must poison its owning transaction")
	require.NoError(t, db.Get(&events, `SELECT count(*) FROM public.business_events WHERE id=$1`, poisoned.ID))
	require.Zero(t, events)
	require.NoError(t, db.Get(&references, `SELECT count(*) FROM public.business_event_delivery_pending WHERE event_id=$1`, poisoned.ID))
	require.Zero(t, references)
}

func TestBusinessEventLedger_RejectsStrongerJoinedIsolation(t *testing.T) {
	adapter, _ := openLedgerFoundation(t)
	ctx := context.Background()
	owner, err := adapter.BeginTX(ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{Isolation: ports.StorageReadCommitted}))
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	_, err = adapter.BeginTX(ports.WithStorageTransactionHints(owner, ports.StorageTransactionHints{Isolation: ports.StorageSerializable}))
	require.Error(t, err, "joining a weaker owner may not silently weaken serializable isolation")

	serializable, err := adapter.BeginTX(ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{Isolation: ports.StorageSerializable}))
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(serializable) }()
	joined, err := adapter.BeginTX(serializable)
	require.NoError(t, err, "a weaker request can join a serializable owner")
	require.NoError(t, adapter.Commit(joined))
}

func TestBusinessEventLedger_ProvisionCurrentAndSevenDaysWithoutDropping(t *testing.T) {
	_, db := openLedgerFoundation(t)
	ctx := context.Background()
	var day time.Time
	require.NoError(t, db.GetContext(ctx, &day, `SELECT date_trunc('day', clock_timestamp() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`))
	var initialEvents, initialReferences int
	require.NoError(t, db.GetContext(ctx, &initialEvents, `SELECT count(*) FROM pg_partition_tree('public.business_events') WHERE level=1`))
	require.NoError(t, db.GetContext(ctx, &initialReferences, `SELECT count(*) FROM pg_partition_tree('public.business_event_delivery_pending') WHERE level=1`))
	require.Equal(t, 32, initialEvents, "six-hour windows must cover the current and next seven complete UTC days")
	require.Equal(t, initialEvents, initialReferences, "each event partition needs a matching reference partition")

	probe := id.NewPrincipal("ledger-partition-probe")
	sample := ledgerExample(t, "token-issued", probe)
	for window := range 32 {
		event := *sample
		event.ID = id.NewBusinessEventID()
		event.OccurredAt = day.Add(-time.Hour)
		event.RecordedAt = day.Add(time.Duration(window)*6*time.Hour + time.Minute)
		wire, err := json.Marshal(&event)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
			VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, event.RecordedAt, event.ID, event.OccurredAt,
			event.Type, event.Outcome, probe.String(), string(wire))
		require.NoError(t, err, "a provisioned window must accept a complete event envelope")
	}
	outside := *sample
	outside.ID = id.NewBusinessEventID()
	outside.OccurredAt = day.Add(-time.Hour)
	outside.RecordedAt = day.Add(8 * 24 * time.Hour)
	wire, err := json.Marshal(&outside)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, outside.RecordedAt, outside.ID, outside.OccurredAt,
		outside.Type, outside.Outcome, probe.String(), string(wire))
	require.Error(t, err, "no default partition may conceal an unprovisioned future window")

	pair := day.Add(-24 * time.Hour)
	_, err = db.ExecContext(ctx, `SELECT public.business_event_create_partition_pair($1)`, pair)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT public.business_event_create_partition_pair($1)`, pair)
	require.NoError(t, err, "creating an existing pair is idempotent")
	_, err = db.ExecContext(ctx, `SELECT public.business_event_create_partition_pair($1)`, pair.Add(time.Minute))
	require.Error(t, err, "the pair function must reject boundaries not aligned to six UTC hours")
	historical := *sample
	historical.ID = id.NewBusinessEventID()
	historical.OccurredAt = pair.Add(-time.Hour)
	historical.RecordedAt = pair.Add(time.Minute)
	wire, err = json.Marshal(&historical)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, historical.RecordedAt, historical.ID, historical.OccurredAt,
		historical.Type, historical.Outcome, probe.String(), string(wire))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE public.business_event_policy SET retention_microseconds=1 WHERE singleton=TRUE`)
	require.NoError(t, err)
	for range 2 {
		_, err = db.ExecContext(ctx, `SELECT public.business_event_provision_partitions()`)
		require.NoError(t, err)
	}
	var events, references int
	require.NoError(t, db.GetContext(ctx, &events, `SELECT count(*) FROM pg_partition_tree('public.business_events') WHERE level=1`))
	require.NoError(t, db.GetContext(ctx, &references, `SELECT count(*) FROM pg_partition_tree('public.business_event_delivery_pending') WHERE level=1`))
	require.Equal(t, initialEvents+1, events, "provisioning must not drop a historical event partition under a short policy")
	require.Equal(t, initialReferences+1, references, "paired references must not be dropped by provisioning")
	var retained int
	require.NoError(t, db.GetContext(ctx, &retained, `SELECT count(*) FROM public.business_events WHERE recorded_at=$1 AND id=$2`, historical.RecordedAt, historical.ID))
	require.Equal(t, 1, retained, "provisioning must not erase history")
}

func TestBusinessEventLedger_AppendLocksLifecycleThenSubject(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	ctx := context.Background()
	subject := id.NewPrincipal("ledger-locked-subject")
	owner, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(owner) }()
	require.NoError(t, adapter.BusinessEvents().Append(owner, ledgerExample(t, "agent-registered", ""), false))
	lifecycle := ledgerAdvisoryLocks(t, db)
	require.Len(t, lifecycle, 1, "a null-subject append must hold the lifecycle gate, not a subject gate")
	require.NoError(t, adapter.BusinessEvents().Append(owner, ledgerExample(t, "token-issued", subject), false))
	withSubject := ledgerAdvisoryLocks(t, db)
	require.Len(t, withSubject, 2, "a subject append must add its subject gate under the lifecycle gate")
	var subjectLock ledgerAdvisoryLock
	for _, lock := range withSubject {
		if !slices.Contains(lifecycle, lock) {
			subjectLock = lock
		}
	}
	require.NotEmpty(t, subjectLock.ClassID)
	require.NoError(t, adapter.Rollback(owner))

	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lockOwner, err := db.BeginTxx(lockCtx, nil)
	require.NoError(t, err)
	defer func() { _ = lockOwner.Rollback() }()
	lockExclusive := func(tx *sqlx.Tx, lock ledgerAdvisoryLock) {
		t.Helper()
		classID, parseErr := strconv.ParseUint(lock.ClassID, 10, 32)
		require.NoError(t, parseErr)
		objectID, parseErr := strconv.ParseUint(lock.ObjectID, 10, 32)
		require.NoError(t, parseErr)
		_, lockErr := tx.ExecContext(lockCtx, `SELECT pg_advisory_xact_lock($1,$2)`, int32(classID), int32(objectID))
		require.NoError(t, lockErr)
	}
	lockExclusive(lockOwner, subjectLock)
	finished := make(chan error, 1)
	blocked := ledgerExample(t, "token-issued", subject)
	go func() { finished <- adapter.BusinessEvents().Append(ctx, blocked, false) }()
	select {
	case err := <-finished:
		t.Fatalf("append bypassed the subject gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, lockOwner.Commit())
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(4 * time.Second):
		t.Fatal("append did not resume when the subject gate was released")
	}

	lifecycleOwner, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lifecycleOwner.Rollback() }()
	lockExclusive(lifecycleOwner, lifecycle[0])
	pending := ledgerExample(t, "token-issued", subject)
	go func() { finished <- adapter.BusinessEvents().Append(ctx, pending, false) }()
	select {
	case err := <-finished:
		t.Fatalf("append bypassed the lifecycle-exclusive provisioning gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, lifecycleOwner.Commit())
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(4 * time.Second):
		t.Fatal("append did not resume when the lifecycle gate was released")
	}
}

type ledgerAdvisoryLock struct {
	ClassID  string `db:"classid"`
	ObjectID string `db:"objid"`
}

func ledgerAdvisoryLocks(t *testing.T, db *sqlx.DB) []ledgerAdvisoryLock {
	t.Helper()
	var locks []ledgerAdvisoryLock
	require.NoError(t, db.Select(&locks, `SELECT l.classid::text AS classid, l.objid::text AS objid
		FROM pg_locks l JOIN pg_stat_activity a ON a.pid=l.pid
		WHERE l.locktype='advisory' AND l.objsubid=2 AND l.granted
			AND l.mode='ShareLock' AND a.datname=current_database() AND a.state='idle in transaction'`))
	return locks
}
