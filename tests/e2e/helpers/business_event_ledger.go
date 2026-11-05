package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// QueryLedgerEvents uses an authorized reader connection, not broker HTTP access.
func QueryLedgerEvents(ctx context.Context, db *sqlx.DB, query model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	if db == nil {
		return nil, errors.New("query ledger events: database is required")
	}
	hasPrincipal := !query.Subject.Principal.IsZero()
	if query.Subject.NoSubject == hasPrincipal {
		return nil, errors.New("query ledger events: select one exact principal or no-subject")
	}
	if query.Start.IsZero() || query.End.IsZero() || !query.Start.Before(query.End) {
		return nil, errors.New("query ledger events: a nonempty occurrence interval is required")
	}
	limit := query.Limit
	if limit == 0 {
		limit = 200
	}
	if limit < 1 || limit > 1000 {
		return nil, errors.New("query ledger events: limit must be between 1 and 1000")
	}
	var afterTime, afterID any
	if query.After != nil {
		if query.After.OccurredAt.IsZero() || query.After.ID.IsZero() {
			return nil, errors.New("query ledger events: cursor requires occurrence time and event ID")
		}
		afterTime = query.After.OccurredAt.UTC()
		afterID = query.After.ID.String()
	}

	rows, err := db.QueryContext(ctx, `
		SELECT envelope FROM public.business_events
		WHERE (($1::boolean AND subject IS NULL) OR (NOT $1::boolean AND subject = $2::text))
		  AND occurred_at >= $3::timestamptz AND occurred_at < $4::timestamptz
		  AND ($5::text = '' OR type = $5::text)
		  AND ($6::text = '' OR outcome = $6::text)
		  AND ($7::timestamptz IS NULL OR (occurred_at, id) > ($7::timestamptz, $8::uuid))
		ORDER BY occurred_at ASC, id ASC
		LIMIT $9`, query.Subject.NoSubject, query.Subject.Principal.String(),
		query.Start.UTC(), query.End.UTC(), query.Type, string(query.Outcome), afterTime, afterID, limit)
	if err != nil {
		return nil, ledgerSQLError("query ledger events", err)
	}
	defer func() { _ = rows.Close() }()

	events := make([]*model.BusinessEvent, 0)
	for rows.Next() {
		var envelope []byte
		if err := rows.Scan(&envelope); err != nil {
			return nil, ledgerSQLError("scan ledger envelope", err)
		}
		var event model.BusinessEvent
		if err := json.Unmarshal(envelope, &event); err != nil {
			return nil, errors.New("decode ledger envelope: invalid stored event")
		}
		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, ledgerSQLError("read ledger envelopes", err)
	}
	if err := rows.Close(); err != nil {
		return nil, ledgerSQLError("close ledger query", err)
	}
	return events, nil
}

// EraseLedgerSubject requires an erasure-operator connection and returns after commit.
func EraseLedgerSubject(ctx context.Context, db *sqlx.DB, subject id.Principal) (int64, error) {
	if db == nil || subject.IsZero() {
		return 0, errors.New("erase ledger subject: database and exact principal are required")
	}
	var count int64
	if err := db.QueryRowContext(ctx, `SELECT public.business_event_erase_subject($1::text)`, subject.String()).Scan(&count); err != nil {
		return 0, ledgerSQLError("erase ledger subject", err)
	}
	return count, nil
}

// MaintainLedgerPartitions requires a migration-owner connection.
func MaintainLedgerPartitions(ctx context.Context, db *sqlx.DB) error {
	if db == nil {
		return errors.New("maintain ledger partitions: database is required")
	}
	_, err := db.ExecContext(ctx, `SELECT public.business_event_maintain_partitions()`)
	return ledgerSQLError("maintain ledger partitions", err)
}

// SeedLedgerHistory is migration-owner test setup, never a runtime recording path.
// Validation is supplied by the production recorder; timestamps are normalized to SQL precision.
func SeedLedgerHistory(ctx context.Context, db *sqlx.DB, event *model.BusinessEvent, validate func(*model.BusinessEvent) error) error {
	if db == nil || event == nil || validate == nil {
		return errors.New("seed ledger history: database, event, and production validator are required")
	}
	if event.RecordedAt.IsZero() {
		return errors.New("seed ledger history: explicit recorded time is required")
	}
	stored := *event
	stored.RecordedAt = stored.RecordedAt.UTC().Truncate(time.Microsecond)
	stored.OccurredAt = stored.OccurredAt.UTC().Truncate(time.Microsecond)
	if err := validate(&stored); err != nil {
		return errors.New("seed ledger history: production validation failed")
	}
	envelope, err := json.Marshal(&stored)
	if err != nil {
		return errors.New("seed ledger history: event serialization failed")
	}
	var subject any
	if stored.Subject != nil {
		subject = stored.Subject.String()
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return ledgerSQLError("begin ledger history seed", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT public.business_event_create_partition_pair($1::timestamptz)`, stored.RecordedAt.Truncate(6*time.Hour)); err != nil {
		return ledgerSQLError("create historical ledger partition pair", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO public.business_events (recorded_at, id, occurred_at, type, outcome, subject, envelope)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		stored.RecordedAt, stored.ID, stored.OccurredAt, stored.Type, string(stored.Outcome), subject, string(envelope)); err != nil {
		return ledgerSQLError("insert historical ledger event", err)
	}
	return ledgerSQLError("commit ledger history seed", tx.Commit())
}

// PostgreSQL errors can contain the full rejected row, principal, or connection credentials.
func ledgerSQLError(operation string, err error) error {
	if err == nil {
		return nil
	}
	for _, cancellation := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cancellation) {
			return fmt.Errorf("%s: %w", operation, cancellation)
		}
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return fmt.Errorf("%s failed (SQLSTATE %s)", operation, postgresError.Code)
	}
	return fmt.Errorf("%s failed", operation)
}
