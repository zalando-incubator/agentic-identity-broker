//go:build integration

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	testbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

func newBusinessEventStorage(t *testing.T) (*storageadapter.Adapter, *sqlx.DB) {
	t.Helper()
	pg := testbootstrap.RequireSharedPostgres(t)
	_, connectionString, cleanupDB := pg.SetupMigratedDatabase(t)
	t.Cleanup(cleanupDB)
	store, cleanupStore := newCIMDKeyDomainStorage(t, connectionString)
	t.Cleanup(cleanupStore)
	db, err := sqlx.Connect("pgx", connectionString)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return store, db
}

func postgresBusinessEvent() *model.BusinessEvent {
	return &model.BusinessEvent{
		ID:         id.NewBusinessEventID(),
		Event:      "test.ledger.recorded",
		OccurredAt: time.Date(2025, time.January, 2, 3, 4, 5, 123456789, time.UTC),
	}
}

type businessEventSQLRow struct {
	ID         string         `db:"id"`
	Event      string         `db:"event"`
	OccurredAt time.Time      `db:"occurred_at"`
	RecordedAt time.Time      `db:"recorded_at"`
	Principal  sql.NullString `db:"principal"`
	Actor      sql.NullString `db:"actor"`
	TraceID    sql.NullString `db:"trace_id"`
	Attributes []byte         `db:"attributes"`
}

func readBusinessEventSQL(t *testing.T, db *sqlx.DB, eventID id.BusinessEventID) businessEventSQLRow {
	t.Helper()
	var row businessEventSQLRow
	require.NoError(t, db.Get(&row, `SELECT id, event, occurred_at, recorded_at, principal, actor, trace_id, attributes
		FROM business_events WHERE id = $1`, eventID))
	return row
}

func TestBusinessEventPostgresAppend(t *testing.T) {
	store, db := newBusinessEventStorage(t)
	ctx := context.Background()

	t.Run("JSONB and caller isolation", func(t *testing.T) {
		event := postgresBusinessEvent()
		principal, actor := "policy:owner", "policy:operator"
		event.Principal, event.Actor = &principal, &actor
		event.TraceID = "0123456789abcdef0123456789abcdef"
		event.Attributes = map[string]any{
			"nested":  map[string]any{"items": []any{"original", map[string]any{"count": float64(2)}}},
			"enabled": true,
		}
		event.RecordedAt = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
		var before, after time.Time
		require.NoError(t, db.Get(&before, `SELECT clock_timestamp()`))
		require.NoError(t, store.BusinessEvents().Append(ctx, event))
		require.NoError(t, db.Get(&after, `SELECT clock_timestamp()`))
		row := readBusinessEventSQL(t, db, event.ID)
		assert.Equal(t, event.ID.String(), row.ID)
		assert.Equal(t, event.Event, row.Event)
		assert.True(t, event.RecordedAt.Equal(row.RecordedAt))
		assert.False(t, row.RecordedAt.Before(before))
		assert.False(t, row.RecordedAt.After(after))
		assert.Equal(t, sql.NullString{String: principal, Valid: true}, row.Principal)
		assert.Equal(t, sql.NullString{String: actor, Valid: true}, row.Actor)
		assert.Equal(t, sql.NullString{String: event.TraceID, Valid: true}, row.TraceID)
		var attributes map[string]any
		require.NoError(t, json.Unmarshal(row.Attributes, &attributes))
		assert.Equal(t, event.Attributes, attributes)
		principal, actor = "changed owner", "changed actor"
		event.Attributes["nested"].(map[string]any)["items"].([]any)[0] = "changed"
		row = readBusinessEventSQL(t, db, event.ID)
		assert.Equal(t, "policy:owner", row.Principal.String)
		assert.Equal(t, "policy:operator", row.Actor.String)
		assert.JSONEq(t, `{"nested":{"items":["original",{"count":2}]},"enabled":true}`, string(row.Attributes))
	})

	t.Run("nil attributes and nullable principals", func(t *testing.T) {
		event := postgresBusinessEvent()
		require.NoError(t, store.BusinessEvents().Append(ctx, event))
		row := readBusinessEventSQL(t, db, event.ID)
		assert.JSONEq(t, `{}`, string(row.Attributes))
		assert.False(t, row.Principal.Valid)
		assert.False(t, row.Actor.Valid)
		assert.Nil(t, event.Attributes)
	})

	t.Run("JSONB rejects escaped NUL", func(t *testing.T) {
		event := postgresBusinessEvent()
		event.Attributes = map[string]any{"nested": []any{"a\x00b"}}
		var storageErr *storage.StorageError
		require.ErrorAs(t, store.BusinessEvents().Append(ctx, event), &storageErr)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, storageErr, &pgErr)
		assert.Equal(t, "22P05", pgErr.Code)
		assert.True(t, event.RecordedAt.IsZero())
		var exists bool
		require.NoError(t, db.Get(&exists, `SELECT EXISTS (SELECT 1 FROM business_events WHERE id = $1)`, event.ID))
		assert.False(t, exists)
	})

	t.Run("duplicate ID is conflict and preserves original", func(t *testing.T) {
		event := postgresBusinessEvent()
		require.NoError(t, store.BusinessEvents().Append(ctx, event))
		original := readBusinessEventSQL(t, db, event.ID)
		event.Event = "test.ledger.duplicate"
		var storageErr *storage.StorageError
		require.ErrorAs(t, store.BusinessEvents().Append(ctx, event), &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
		row := readBusinessEventSQL(t, db, event.ID)
		assert.Equal(t, original.Event, row.Event)
		assert.True(t, original.RecordedAt.Equal(row.RecordedAt))
	})

	t.Run("committed context transaction is ignored by ledger operations", func(t *testing.T) {
		txCtx, err := store.BeginTX(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Rollback(txCtx) })
		require.NoError(t, store.Commit(txCtx))
		event := postgresBusinessEvent()
		require.NoError(t, store.BusinessEvents().Append(txCtx, event))
		_, err = store.BusinessEventRetention().StorageTime(txCtx)
		require.NoError(t, err)
		row := readBusinessEventSQL(t, db, event.ID)
		assert.Equal(t, event.Event, row.Event)
		deleted, err := store.BusinessEventRetention().DeleteRecordedBefore(txCtx, event.OccurredAt, 1)
		require.NoError(t, err)
		assert.Zero(t, deleted)
	})
	t.Run("empty event validation", func(t *testing.T) {
		event := postgresBusinessEvent()
		event.Event = ""
		var storageErr *storage.StorageError
		require.ErrorAs(t, store.BusinessEvents().Append(ctx, event), &storageErr)
		assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		var exists bool
		require.NoError(t, db.Get(&exists, `SELECT EXISTS (SELECT 1 FROM business_events WHERE id = $1)`, event.ID))
		assert.False(t, exists)
	})

	t.Run("expired contexts are timeout", func(t *testing.T) {
		expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()
		var storageErr *storage.StorageError
		require.ErrorAs(t, store.BusinessEvents().Append(expired, postgresBusinessEvent()), &storageErr)
		assert.Equal(t, storage.ErrorKindTimeout, storageErr.Kind)
		deleted, err := store.BusinessEventRetention().DeleteRecordedBefore(expired, time.Now(), 1000)
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindTimeout, storageErr.Kind)
		assert.Zero(t, deleted)
	})

	for _, occurredAt := range []time.Time{
		time.Date(2025, time.January, 2, 3, 4, 5, 123456789, time.UTC),
		time.Date(2025, time.January, 2, 3, 4, 5, 999999999, time.UTC),
		time.Date(2025, time.January, 2, 3, 4, 5, 123456789, time.FixedZone("offset", 5*3600+1800)),
		time.Date(2025, time.January, 2, 3, 4, 5, 999999999, time.FixedZone("offset", -7*3600)),
	} {
		t.Run("precision "+occurredAt.Format(time.RFC3339Nano), func(t *testing.T) {
			event := postgresBusinessEvent()
			event.OccurredAt = occurredAt
			require.NoError(t, store.BusinessEvents().Append(ctx, event))
			row := readBusinessEventSQL(t, db, event.ID)
			assert.True(t, occurredAt.UTC().Truncate(time.Microsecond).Equal(row.OccurredAt), "PostgreSQL must truncate, not round, occurrence timestamps")
			assert.Equal(t, occurredAt, event.OccurredAt, "only RecordedAt may be assigned back")
			assert.True(t, event.RecordedAt.Equal(row.RecordedAt))
		})
	}

	for _, test := range []struct {
		name       string
		event      string
		attributes string
		constraint string
	}{
		{"empty event CHECK", "", `{}`, "chk_business_events_event_not_empty"},
		{"object attributes CHECK", "test.ledger.recorded", `[]`, "chk_business_events_attributes_object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := db.Exec(`INSERT INTO business_events (id, event, occurred_at, attributes) VALUES ($1, $2, $3, $4::jsonb)`,
				id.NewBusinessEventID(), test.event, time.Now(), test.attributes)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, "23514", pgErr.Code)
			assert.Equal(t, test.constraint, pgErr.ConstraintName)
		})
	}
}

func TestBusinessEventPostgresRetentionOrderAndLimit(t *testing.T) {
	store, db := newBusinessEventStorage(t)
	const period = 24 * time.Hour
	var before, after time.Time
	require.NoError(t, db.Get(&before, `SELECT statement_timestamp()`))
	storageNow, err := store.BusinessEventRetention().StorageTime(t.Context())
	require.NoError(t, err)
	require.NoError(t, db.Get(&after, `SELECT statement_timestamp()`))
	require.False(t, storageNow.Before(before))
	require.False(t, storageNow.After(after))
	cutoff := storageNow.Add(-period)
	ids := []id.BusinessEventID{
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000001")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000002")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000003")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000004")),
		id.BusinessEventID(uuid.MustParse("00000000-0000-4000-8000-000000000005")),
	}
	for index, recordedAt := range []time.Time{cutoff.Add(-time.Microsecond), cutoff.Add(-time.Microsecond), cutoff, cutoff.Add(time.Microsecond), cutoff.Add(-time.Hour)} {
		_, err := db.Exec(`INSERT INTO business_events (id, event, occurred_at, recorded_at) VALUES ($1, 'test.ledger.recorded', $2, $2)`, ids[index], recordedAt)
		require.NoError(t, err)
	}
	for _, expectedID := range []id.BusinessEventID{ids[4], ids[0], ids[1]} {
		deleted, err := store.BusinessEventRetention().DeleteRecordedBefore(context.Background(), cutoff, 1)
		require.NoError(t, err)
		assert.EqualValues(t, 1, deleted)
		var exists bool
		require.NoError(t, db.Get(&exists, `SELECT EXISTS (SELECT 1 FROM business_events WHERE id = $1)`, expectedID))
		assert.False(t, exists, "oldest row, then lowest ID, must be deleted")
	}
	deleted, err := store.BusinessEventRetention().DeleteRecordedBefore(t.Context(), cutoff, 1000)
	require.NoError(t, err)
	assert.Zero(t, deleted)
	var remaining []string
	require.NoError(t, db.Select(&remaining, `SELECT id FROM business_events ORDER BY id`))
	assert.Equal(t, []string{ids[2].String(), ids[3].String()}, remaining)
}

func TestBusinessEventPostgresConcurrentRetentionShortBatch(t *testing.T) {
	store, db := newBusinessEventStorage(t)
	cutoff := time.Date(2025, time.March, 4, 5, 6, 7, 0, time.UTC)
	_, err := db.Exec(`INSERT INTO business_events (id, event, occurred_at, recorded_at)
		SELECT gen_random_uuid(), 'test.ledger.expired', $1::timestamptz, $1::timestamptz - interval '2 hours' + n * interval '1 microsecond'
		FROM generate_series(1, 2500) AS n`, cutoff)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO business_events (id, event, occurred_at, recorded_at)
		SELECT gen_random_uuid(), 'test.ledger.newer', $1::timestamptz, $1::timestamptz + n * interval '1 second'
		FROM generate_series(0, 2) AS n`, cutoff)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var blockerPID int
	require.NoError(t, tx.GetContext(ctx, &blockerPID, `SELECT pg_backend_pid()`))
	var lockedIDs []string
	require.NoError(t, tx.SelectContext(ctx, &lockedIDs, `SELECT id FROM business_events WHERE recorded_at < $1 ORDER BY recorded_at, id LIMIT 1000 FOR UPDATE`, cutoff))
	require.Len(t, lockedIDs, 1000)

	type deleteResult struct {
		count int64
		err   error
	}
	finished := make(chan deleteResult, 1)
	joined := false
	go func() {
		count, deleteErr := store.BusinessEventRetention().DeleteRecordedBefore(ctx, cutoff, 1000)
		finished <- deleteResult{count: count, err: deleteErr}
	}()
	t.Cleanup(func() {
		cancel()
		_ = tx.Rollback()
		if !joined {
			select {
			case <-finished:
			case <-time.After(6 * time.Second):
				t.Error("retention goroutine did not stop after cancellation and rollback")
			}
		}
	})

	var pollErr error
	blocked := false
	require.Eventually(t, func() bool {
		pollErr = db.GetContext(ctx, &blocked, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND $1 = ANY(pg_blocking_pids(pid))
			AND query ILIKE '%DELETE FROM business_events%' AND wait_event_type = 'Lock'
		)`, blockerPID)
		return pollErr == nil && blocked
	}, 3*time.Second, 10*time.Millisecond, "delete must wait on the transaction holding the first 1000 rows; last poll error: %v", pollErr)

	query, args, err := sqlx.In(`DELETE FROM business_events WHERE id IN (?)`, lockedIDs)
	require.NoError(t, err)
	result, err := tx.ExecContext(ctx, tx.Rebind(query), args...)
	require.NoError(t, err)
	deleted, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1000, deleted)
	require.NoError(t, tx.Commit())

	select {
	case result := <-finished:
		joined = true
		require.NoError(t, result.err)
		require.Zero(t, result.count, "a delete whose selected rows were concurrently removed returns a short batch")
	case <-ctx.Done():
		t.Fatal("retention delete did not finish after the blocking transaction committed")
	}
	var expired, newer int64
	require.NoError(t, db.Get(&expired, `SELECT COUNT(*) FROM business_events WHERE recorded_at < $1`, cutoff))
	require.NoError(t, db.Get(&newer, `SELECT COUNT(*) FROM business_events WHERE recorded_at >= $1`, cutoff))
	assert.EqualValues(t, 1500, expired, "a short concurrent batch does not establish an empty backlog")
	assert.EqualValues(t, 3, newer)

	for _, want := range []int64{1000, 500, 0} {
		deleted, err := store.BusinessEventRetention().DeleteRecordedBefore(ctx, cutoff, 1000)
		require.NoError(t, err)
		assert.Equal(t, want, deleted)
	}
	require.NoError(t, db.Get(&expired, `SELECT COUNT(*) FROM business_events WHERE recorded_at < $1`, cutoff))
	require.NoError(t, db.Get(&newer, `SELECT COUNT(*) FROM business_events WHERE recorded_at >= $1`, cutoff))
	assert.Zero(t, expired)
	assert.EqualValues(t, 3, newer, "recovery must preserve cutoff-equal and newer rows")
}
