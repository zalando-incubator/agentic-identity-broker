package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func postgresSpanBusinessEvent() *model.BusinessEvent {
	principal := "rendered:owner"
	return &model.BusinessEvent{
		ID:         id.NewBusinessEventID(),
		Event:      "test.ledger.recorded",
		OccurredAt: time.Date(2025, time.January, 2, 3, 4, 5, 123456789, time.UTC),
		Principal:  &principal,
	}
}

func TestBusinessEventAppendStorageSpans(t *testing.T) {
	recordedAt := time.Date(2025, time.March, 4, 5, 6, 7, 123456000, time.UTC)
	for _, test := range []struct {
		name      string
		config    signingKeyRepoTestConfig
		invalid   bool
		wantError bool
		status    codes.Code
		errorType string
		outcome   string
	}{
		{name: "success", config: signingKeyRepoTestConfig{queryColumns: []string{"recorded_at"}, queryRows: [][]driver.Value{{recordedAt}}}},
		{name: "duplicate", config: signingKeyRepoTestConfig{queryErr: &pgconn.PgError{Code: "23505"}}, wantError: true, outcome: "conflict"},
		{name: "deadline", config: signingKeyRepoTestConfig{queryErr: context.DeadlineExceeded}, wantError: true, status: codes.Error, errorType: "timeout"},
		{name: "connection", config: signingKeyRepoTestConfig{queryErr: driver.ErrBadConn}, wantError: true, status: codes.Error, errorType: "connection"},
		{name: "trigger rejection", config: signingKeyRepoTestConfig{queryErr: &pgconn.PgError{Code: "P0001"}}, wantError: true, status: codes.Error, errorType: "unknown"},
		{name: "validation", invalid: true, wantError: true, outcome: "validation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := recordStorageSpans(t)
			adapter := newUnitTestSigningKeyRepo(t, test.config).adapter
			event := postgresSpanBusinessEvent()
			if test.invalid {
				event.Event = ""
			}
			parent := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
			})
			err := adapter.Append(trace.ContextWithSpanContext(context.Background(), parent), event)
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, recordedAt.Equal(event.RecordedAt))
			}
			span := requireOneStorageSpan(t, recorder, "storage.append.business_event")
			assertStorageSpanResult(t, span, test.status, test.errorType, test.outcome)
			assert.Equal(t, "Append", spanAttributes(span)["db.operation"])
			assert.Equal(t, parent.SpanID(), span.Parent().SpanID())
			for _, attribute := range span.Attributes() {
				assert.NotContains(t, string(attribute.Key), "principal")
				assert.NotEqual(t, "rendered:owner", attribute.Value.AsString())
			}
		})
	}
}

func TestBusinessEventStorageTimeUsesDatabaseClock(t *testing.T) {
	recorder := recordStorageSpans(t)
	databaseTime := time.Date(2025, time.March, 4, 5, 6, 7, 123456000, time.UTC)
	adapter := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
		queryColumns: []string{"statement_timestamp"}, queryRows: [][]driver.Value{{databaseTime}},
	}).adapter
	actual, err := adapter.StorageTime(t.Context())
	require.NoError(t, err)
	assert.True(t, databaseTime.Equal(actual))
	span := requireOneStorageSpan(t, recorder, "storage.current_time.business_event")
	assert.Equal(t, "StorageTime", spanAttributes(span)["db.operation"])
	assertStorageSpanResult(t, span, codes.Unset, "", "")
}

func TestBusinessEventDeleteStorageSpans(t *testing.T) {
	for _, test := range []struct {
		name      string
		config    signingKeyRepoTestConfig
		wantError bool
		status    codes.Code
		errorType string
	}{
		{name: "success", config: signingKeyRepoTestConfig{rowsAffected: 7}},
		{name: "deadline", config: signingKeyRepoTestConfig{execErr: context.DeadlineExceeded}, wantError: true, status: codes.Error, errorType: "timeout"},
		{name: "connection", config: signingKeyRepoTestConfig{execErr: driver.ErrBadConn}, wantError: true, status: codes.Error, errorType: "connection"},
		{name: "trigger rejection", config: signingKeyRepoTestConfig{execErr: &pgconn.PgError{Code: "P0001"}}, wantError: true, status: codes.Error, errorType: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := recordStorageSpans(t)
			adapter := newUnitTestSigningKeyRepo(t, test.config).adapter
			deleted, err := adapter.DeleteRecordedBefore(context.Background(), time.Now(), 1000)
			if test.wantError {
				require.Error(t, err)
				assert.Zero(t, deleted)
			} else {
				require.NoError(t, err)
				assert.EqualValues(t, 7, deleted)
			}
			span := requireOneStorageSpan(t, recorder, "storage.delete_recorded_before.business_event")
			assertStorageSpanResult(t, span, test.status, test.errorType, "")
			assert.Equal(t, "DeleteRecordedBefore", spanAttributes(span)["db.operation"])
			assert.False(t, span.Parent().IsValid(), "background retention has no request parent")
			for _, attribute := range span.Attributes() {
				assert.NotContains(t, string(attribute.Key), "principal")
			}
		})
	}
}

func TestBusinessEventStorageErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		kind  storage.ErrorKind
	}{
		{name: "trigger rejection", cause: &pgconn.PgError{Code: "P0001"}, kind: storage.ErrorKindUnknown},
		{name: "check violation", cause: &pgconn.PgError{Code: "23514"}, kind: storage.ErrorKindUnknown},
		{name: "unexpected error", cause: errors.New("unexpected driver result"), kind: storage.ErrorKindUnknown},
		{name: "duplicate ID", cause: &pgconn.PgError{Code: "23505"}, kind: storage.ErrorKindConflict},
		{name: "deadline", cause: context.DeadlineExceeded, kind: storage.ErrorKindTimeout},
		{name: "canceled", cause: context.Canceled, kind: storage.ErrorKindTimeout},
		{name: "statement timeout", cause: &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}, kind: storage.ErrorKindTimeout},
		{name: "bad connection", cause: driver.ErrBadConn, kind: storage.ErrorKindConnection},
		{name: "closed connection", cause: pgconn.ErrConnClosed, kind: storage.ErrorKindConnection},
		{name: "EOF", cause: io.EOF, kind: storage.ErrorKindConnection},
		{name: "unexpected EOF", cause: io.ErrUnexpectedEOF, kind: storage.ErrorKindConnection},
		{name: "network failure", cause: &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}, kind: storage.ErrorKindConnection},
		{name: "connection exception", cause: &pgconn.PgError{Code: "08006"}, kind: storage.ErrorKindConnection},
		{name: "admin shutdown", cause: &pgconn.PgError{Code: "57P01"}, kind: storage.ErrorKindConnection},
		{name: "crash shutdown", cause: &pgconn.PgError{Code: "57P02"}, kind: storage.ErrorKindConnection},
		{name: "cannot connect", cause: &pgconn.PgError{Code: "57P03"}, kind: storage.ErrorKindConnection},
	} {
		t.Run(test.name, func(t *testing.T) {
			cause := fmt.Errorf("wrapped: %w", test.cause)
			err := businessEventStorageError("Append", cause, "append failed")
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, test.kind, storageErr.Kind)
			assert.Equal(t, "Append", storageErr.Operation)
			assert.ErrorIs(t, err, test.cause)
		})
	}
}
