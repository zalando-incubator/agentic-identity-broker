package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func recordStorageSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return recorder
}

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]string {
	attributes := make(map[string]string, len(span.Attributes()))
	for _, keyValue := range span.Attributes() {
		attributes[string(keyValue.Key)] = keyValue.Value.AsString()
	}
	return attributes
}

// assertStorageSpanResult checks the status, error.type and storage.outcome of a finished span.
// An empty wantErrorType or wantOutcome means the attribute must be absent.
func assertStorageSpanResult(t *testing.T, span sdktrace.ReadOnlySpan, wantStatus codes.Code, wantErrorType, wantOutcome string) {
	t.Helper()
	assert.Equal(t, trace.SpanKindClient, span.SpanKind())
	assert.Equal(t, wantStatus, span.Status().Code)
	assert.Empty(t, span.Events(), "storage errors must not become span events")

	attributes := spanAttributes(span)
	assert.Equal(t, "postgresql", attributes["db.system"])
	assertOptionalAttribute(t, attributes, "error.type", wantErrorType)
	assertOptionalAttribute(t, attributes, "storage.outcome", wantOutcome)
}

// requireOneStorageSpan returns the only span the recorder has ended, after checking its name.
func requireOneStorageSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, name, spans[0].Name())
	return spans[0]
}

func assertOptionalAttribute(t *testing.T, attributes map[string]string, key, want string) {
	t.Helper()
	if want == "" {
		assert.NotContains(t, attributes, key)
		return
	}
	assert.Equal(t, want, attributes[key], key)
}

func TestStartStorageSpan_RecordsResult(t *testing.T) {
	const credential = "postgres://broker:s3cret@db.internal/broker"
	cause := errors.New("dial " + credential)

	tests := []struct {
		name           string
		err            error
		callerCanceled bool
		wantStatus     codes.Code
		wantErrorType  string
		wantOutcome    string
	}{
		{name: "success", wantStatus: codes.Unset},
		{
			name:          "connection failure",
			err:           storage.NewStorageError("Op", storage.ErrorKindConnection, cause, "query failed"),
			wantStatus:    codes.Error,
			wantErrorType: "connection",
		},
		{
			name:          "timeout",
			err:           storage.NewStorageError("Op", storage.ErrorKindTimeout, context.DeadlineExceeded, "timed out"),
			wantStatus:    codes.Error,
			wantErrorType: "timeout",
		},
		{
			name:          "unknown storage error",
			err:           storage.NewStorageError("Op", storage.ErrorKindUnknown, cause, "unexpected"),
			wantStatus:    codes.Error,
			wantErrorType: "unknown",
		},
		{
			name:          "error without a storage kind",
			err:           cause,
			wantStatus:    codes.Error,
			wantErrorType: "unknown",
		},
		{
			name:        "row not found",
			err:         storage.NewStorageError("Op", storage.ErrorKindNotFound, ports.ErrNotFound, "not found"),
			wantStatus:  codes.Unset,
			wantOutcome: "not_found",
		},
		{
			name:        "conflict",
			err:         storage.NewStorageError("Op", storage.ErrorKindConflict, cause, "conflict"),
			wantStatus:  codes.Unset,
			wantOutcome: "conflict",
		},
		{
			name:        "rejected input",
			err:         storage.NewStorageError("Op", storage.ErrorKindValidation, nil, "ID cannot be empty"),
			wantStatus:  codes.Unset,
			wantOutcome: "validation",
		},
		{
			name:           "canceled by the caller",
			err:            storage.NewStorageError("Op", storage.ErrorKindConnection, fmt.Errorf("query: %w", context.Canceled), "query failed"),
			callerCanceled: true,
			wantStatus:     codes.Unset,
			wantOutcome:    "canceled",
		},
		{
			name:           "bad connection after the caller canceled",
			err:            storage.NewStorageError("Op", storage.ErrorKindConnection, driver.ErrBadConn, "query failed"),
			callerCanceled: true,
			wantStatus:     codes.Unset,
			wantOutcome:    "canceled",
		},
		{
			name:           "undecodable value after the caller canceled",
			err:            storage.NewStorageError("Op", storage.ErrorKindUnknown, cause, "failed to unmarshal"),
			callerCanceled: true,
			wantStatus:     codes.Error,
			wantErrorType:  "unknown",
		},
		{
			name:           "timeout after the caller canceled",
			err:            storage.NewStorageError("Op", storage.ErrorKindTimeout, context.DeadlineExceeded, "timed out"),
			callerCanceled: true,
			wantStatus:     codes.Error,
			wantErrorType:  "timeout",
		},
		{
			name:           "row not found after the caller canceled",
			err:            storage.NewStorageError("Op", storage.ErrorKindNotFound, ports.ErrNotFound, "not found"),
			callerCanceled: true,
			wantStatus:     codes.Unset,
			wantOutcome:    "not_found",
		},
		{
			name:          "statement timeout",
			err:           storage.NewStorageError("Op", storage.ErrorKindConnection, &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}, "query failed"),
			wantStatus:    codes.Error,
			wantErrorType: "timeout",
		},
		{
			name:          "query canceled by the server",
			err:           storage.NewStorageError("Op", storage.ErrorKindConnection, &pgconn.PgError{Code: "57014", Message: "canceling statement due to user request"}, "query failed"),
			wantStatus:    codes.Error,
			wantErrorType: "connection",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := recordStorageSpans(t)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, endSpan := startStorageSpan(ctx, "storage.get.thing", "GetThing")
			if test.callerCanceled {
				cancel()
			}
			endSpan(test.err)

			spans := recorder.Ended()
			require.Len(t, spans, 1, "the returned function must end the span")
			span := spans[0]
			assert.Equal(t, "storage.get.thing", span.Name())
			assert.Equal(t, "GetThing", spanAttributes(span)["db.operation"])
			assertStorageSpanResult(t, span, test.wantStatus, test.wantErrorType, test.wantOutcome)

			assert.NotContains(t, span.Status().Description, credential)
			for key, value := range spanAttributes(span) {
				assert.NotContains(t, value, credential, key)
			}
		})
	}
}

func TestStorageSpans_FailedRepositoryCalls(t *testing.T) {
	adapter := &Adapter{}
	agents := NewAgentRepository(adapter)
	approvals := NewToolApprovalRepository(adapter)
	syncState := NewApprovalSyncStateRepository(adapter)
	grants := NewUserGrantRepository(adapter)
	now := time.Now()

	var (
		agentID    id.AgentID
		approvalID id.ApprovalID
		principal  id.Principal
	)

	calls := []struct {
		span string
		call func(ctx context.Context) error
	}{
		{"storage.get.agent", func(ctx context.Context) error {
			_, err := agents.Get(ctx, agentID)
			return err
		}},
		{"storage.get_version.approval_sync", func(ctx context.Context) error {
			_, err := syncState.GetVersion(ctx)
			return err
		}},
		{"storage.increment_version.approval_sync", func(ctx context.Context) error {
			_, err := syncState.IncrementVersion(ctx)
			return err
		}},
		{"storage.create.tool_approval", func(ctx context.Context) error {
			_, err := approvals.Create(ctx, nil)
			return err
		}},
		{"storage.get.tool_approval", func(ctx context.Context) error {
			_, err := approvals.Get(ctx, approvalID)
			return err
		}},
		{"storage.approve.tool_approval", func(ctx context.Context) error {
			_, err := approvals.Approve(ctx, approvalID, storage.ApprovalDecision{}, now)
			return err
		}},
		{"storage.deny.tool_approval", func(ctx context.Context) error {
			_, err := approvals.Deny(ctx, approvalID, nil, now)
			return err
		}},
		{"storage.consume.tool_approval", func(ctx context.Context) error {
			_, err := approvals.Consume(ctx, approvalID, now)
			return err
		}},
		{"storage.revoke_permanent.tool_approval", func(ctx context.Context) error {
			_, err := approvals.RevokePermanent(ctx, approvalID, now)
			return err
		}},
		{"storage.list_active.tool_approval", func(ctx context.Context) error {
			_, err := approvals.ListAllActive(ctx, nil, nil)
			return err
		}},
		{"storage.list_permanent.tool_approval", func(ctx context.Context) error {
			_, err := approvals.ListPermanentByPrincipal(ctx, principal)
			return err
		}},
		{"storage.count_pending.tool_approval", func(ctx context.Context) error {
			_, err := approvals.CountPendingByPrincipalAndAgent(ctx, principal, agentID)
			return err
		}},
		{"storage.create.user_grant", func(ctx context.Context) error {
			return grants.Create(ctx, nil)
		}},
		{"storage.list.user_grants_by_principal", func(ctx context.Context) error {
			_, err := grants.ListByPrincipal(ctx, principal)
			return err
		}},
	}

	for _, test := range calls {
		t.Run(test.span, func(t *testing.T) {
			recorder := recordStorageSpans(t)

			require.Error(t, test.call(context.Background()), "an adapter without a database must fail")

			assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, test.span), codes.Error, "connection", "")
		})
	}
}
