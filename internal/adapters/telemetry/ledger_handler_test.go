package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/domainevent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

var (
	ledgerTestEvent  = domainevent.MustDefine("telemetry.ledger_test.recorded")
	ledgerOtherEvent = domainevent.MustDefine("telemetry.ledger_test.replaced")
)

type ledgerCaptureAppender struct {
	events   []*model.DomainEvent
	appendFn func(context.Context, *model.DomainEvent) error
}

func (a *ledgerCaptureAppender) Append(ctx context.Context, event *model.DomainEvent) error {
	copy := *event
	a.events = append(a.events, &copy)
	if a.appendFn != nil {
		return a.appendFn(ctx, event)
	}
	return nil
}

func ledgerRecord(attrs ...slog.Attr) slog.Record {
	record := slog.NewRecord(time.Date(2001, 2, 3, 4, 5, 6, 123456789, time.FixedZone("offset", -7*3600)), slog.LevelInfo, "domain event recorded", 0)
	record.AddAttrs(attrs...)
	return record
}

func TestLedgerHandlerRecognitionAndOccurrence(t *testing.T) {
	cases := []struct {
		name      string
		attrs     []slog.Attr
		inherited []slog.Attr
		group     string
		want      string
	}{
		{name: "typed", attrs: []slog.Attr{slog.Any("event", ledgerTestEvent)}, want: ledgerTestEvent.Name()},
		{name: "string", attrs: []slog.Attr{slog.String("event", ledgerTestEvent.Name())}},
		{name: "absent"},
		{name: "zero", attrs: []slog.Attr{slog.Any("event", domainevent.Definition{})}},
		{name: "explicit group", attrs: []slog.Attr{slog.Group("details", slog.Any("event", ledgerTestEvent))}},
		{name: "logger group", attrs: []slog.Attr{slog.Any("event", ledgerTestEvent)}, group: "details"},
		{name: "inherited", inherited: []slog.Attr{slog.Any("event", ledgerTestEvent)}, want: ledgerTestEvent.Name()},
		{name: "string overrides inherited", inherited: []slog.Attr{slog.Any("event", ledgerTestEvent)}, attrs: []slog.Attr{slog.String("event", "operational.event.recorded")}},
		{name: "zero overrides inherited", inherited: []slog.Attr{slog.Any("event", ledgerTestEvent)}, attrs: []slog.Attr{slog.Any("event", domainevent.Definition{})}},
		{name: "last typed wins", inherited: []slog.Attr{slog.Any("event", ledgerTestEvent)}, attrs: []slog.Attr{slog.Any("event", ledgerOtherEvent)}, want: ledgerOtherEvent.Name()},
		{name: "record duplicates", attrs: []slog.Attr{slog.Any("event", ledgerOtherEvent), slog.Any("event", ledgerTestEvent)}, want: ledgerTestEvent.Name()},
		{name: "group cannot override", inherited: []slog.Attr{slog.Any("event", ledgerTestEvent)}, attrs: []slog.Attr{slog.Group("details", slog.Any("event", ledgerOtherEvent))}, want: ledgerTestEvent.Name()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appender := &ledgerCaptureAppender{}
			handler := NewPrincipalPolicyHandler(NewLedgerHandler(appender, enabledNoopHandler{}, time.Second))
			handler = handler.WithAttrs(tc.inherited)
			if tc.group != "" {
				handler = handler.WithGroup(tc.group)
			}
			record := ledgerRecord(tc.attrs...)
			require.NoError(t, handler.Handle(context.Background(), record))
			if tc.want == "" {
				assert.Empty(t, appender.events)
				return
			}
			require.Len(t, appender.events, 1)
			event := appender.events[0]
			assert.Equal(t, tc.want, event.Event)
			assert.Equal(t, uuid.Version(7), uuid.UUID(event.ID).Version())
			assert.True(t, event.OccurredAt.Equal(record.Time))
			assert.Equal(t, time.UTC, event.OccurredAt.Location())
			assert.Equal(t, 123456789, event.OccurredAt.Nanosecond())
			assert.True(t, event.RecordedAt.IsZero(), "storage assigns recorded_at")
			assert.Nil(t, event.Principal)
			assert.Nil(t, event.Actor)
			assert.Empty(t, event.TraceID)
			for _, key := range []string{"event", "principal", "actor", "trace_id"} {
				assert.NotContains(t, event.Attributes, key)
			}
		})
	}
}

func TestLedgerHandlerEventGroupOverridesDefinition(t *testing.T) {
	cases := []struct {
		name       string
		inherited  []slog.Attr
		attrs      []slog.Attr
		wantEvent  any
		wantStored bool
	}{
		{name: "record attributes", attrs: []slog.Attr{slog.String("detail", "record")}, wantEvent: map[string]any{"detail": "record"}},
		{name: "inherited attributes", inherited: []slog.Attr{slog.String("detail", "inherited")}, wantEvent: map[string]any{"detail": "inherited"}},
		{name: "empty logger group", wantEvent: ledgerTestEvent.Name(), wantStored: true},
		{name: "omitted attributes", inherited: []slog.Attr{slog.Group("empty")}, attrs: []slog.Attr{slog.Group("empty")}, wantEvent: ledgerTestEvent.Name(), wantStored: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appender := &ledgerCaptureAppender{}
			var operational bytes.Buffer
			handler := NewPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, nil), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second)))
			handler = handler.WithAttrs([]slog.Attr{slog.Any("event", ledgerTestEvent)}).WithGroup("event").WithAttrs(tc.inherited)
			require.NoError(t, handler.Handle(context.Background(), ledgerRecord(tc.attrs...)))
			decoded := decodeLedgerJSON(t, operational.Bytes())
			assert.Equal(t, tc.wantEvent, decoded["event"])
			if !tc.wantStored {
				assert.Empty(t, appender.events, "a group replacing the effective event must suppress persistence")
				return
			}
			require.Len(t, appender.events, 1)
			assert.Equal(t, decoded["event"], appender.events[0].Event)
			delete(decoded, "event")
			assert.Equal(t, decoded, normalizeLedgerJSON(t, appender.events[0].Attributes))
		})
	}
}

type ledgerLogValue struct{}

func (ledgerLogValue) LogValue() slog.Value { return slog.GroupValue(slog.String("resolved", "yes")) }

type ledgerJSONError struct{}

func (ledgerJSONError) Error() string                { return "error must lose" }
func (ledgerJSONError) MarshalJSON() ([]byte, error) { return []byte(`{"encoded":"json wins"}`), nil }

type ledgerBrokenJSON struct{}

func (ledgerBrokenJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("encoding rejected") }

func TestLedgerHandlerEncodingMatchesOperationalJSON(t *testing.T) {
	values := []slog.Attr{
		slog.String("text", "value"), slog.Bool("boolean", true), slog.Int64("signed", -42),
		slog.Uint64("unsigned", 42), slog.Float64("float", 1.25),
		slog.Time("time_at", time.Date(2002, 3, 4, 5, 6, 7, 987654321, time.FixedZone("east", 5*3600+1800))),
		slog.Duration("elapsed", 123456789*time.Nanosecond), slog.Any("resolved", ledgerLogValue{}),
		slog.Any("agent_id", id.NewAgentID()), slog.Any("error", errors.New("denied")),
		slog.Any("json_error", ledgerJSONError{}), slog.Any("unsupported", make(chan int)),
		slog.Any("broken_json", ledgerBrokenJSON{}), slog.Float64("infinite", math.Inf(1)),
		slog.Float64("nan", math.NaN()), slog.Any("arbitrary", map[string]any{"error": errors.New("not recursively resolved")}),
		slog.Any("secret", model.NewPlaintextSecret("credential-must-never-persist")),
		slog.Any("encrypted_secret", model.NewEncryptedSecret([]byte("ciphertext-must-never-persist"))),
		slog.Group("empty"), {}, slog.Group("", slog.String("inline", "flattened")),
	}
	for _, mode := range []string{"record", "inherited", "group", "inherited group"} {
		t.Run(mode, func(t *testing.T) {
			appender := &ledgerCaptureAppender{}
			var operational bytes.Buffer
			handler := NewPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, nil), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second)))
			handler = handler.WithAttrs([]slog.Attr{slog.Any("event", ledgerTestEvent)})
			attrs := values
			switch mode {
			case "inherited":
				handler = handler.WithAttrs(values)
				attrs = nil
			case "group":
				attrs = []slog.Attr{{Key: "details", Value: slog.GroupValue(values...)}}
			case "inherited group":
				handler = handler.WithGroup("details").WithAttrs(values)
				attrs = nil
			}
			require.NoError(t, handler.Handle(context.Background(), ledgerRecord(attrs...)))
			require.Len(t, appender.events, 1)
			decoded := decodeLedgerJSON(t, operational.Bytes())
			delete(decoded, "event")
			assert.Equal(t, decoded, normalizeLedgerJSON(t, appender.events[0].Attributes))
			document := normalizeLedgerJSON(t, appender.events[0].Attributes)
			if mode == "group" || mode == "inherited group" {
				document = document["details"].(map[string]any)
			}
			assert.Equal(t, "denied", document["error"])
			assert.Equal(t, map[string]any{"encoded": "json wins"}, document["json_error"])
			assert.Equal(t, "2002-03-04T05:06:07.987654321+05:30", document["time_at"])
			for _, key := range []string{"unsupported", "broken_json", "infinite", "nan"} {
				value, ok := document[key].(string)
				require.True(t, ok, key)
				assert.Contains(t, value, "!ERROR:", key)
			}
			assert.Equal(t, "REDACTED", document["secret"])
			assert.Equal(t, "REDACTED", document["encrypted_secret"])
			encoded, err := json.Marshal(appender.events[0])
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "credential-must-never-persist")
			assert.NotContains(t, string(encoded), "ciphertext-must-never-persist")
		})
	}
}

func decodeLedgerJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	delete(decoded, "time")
	delete(decoded, "level")
	delete(decoded, "msg")
	return decoded
}

func normalizeLedgerJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return decoded
}

func TestLedgerHandlerPrincipalPolicyAndGroupPrecedence(t *testing.T) {
	appender := &ledgerCaptureAppender{}
	var operational bytes.Buffer
	policy := func(value string) string { return fmt.Sprintf("principal-length:%d", len(value)) }
	handler := newPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, nil), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second)), policy)
	handler = handler.WithAttrs([]slog.Attr{slog.Any("event", ledgerTestEvent), slog.Any("principal", id.NewPrincipal("old-owner")), slog.String("ordinary", "old")})
	handler = handler.WithAttrs([]slog.Attr{slog.Any("operator_principal", id.NewPrincipal("operator-subject")), slog.Group("details", slog.Any("principal", id.NewPrincipal("group-owner")))})
	record := ledgerRecord(slog.Any("principal", id.NewPrincipal("resource-owner")), slog.Any("actor", id.NewPrincipal("request-actor")), slog.String("trace_id", "trace-value"), slog.String("ordinary", "last"), slog.String("ordinary", "final"), slog.Group("nested", slog.Any("event", ledgerOtherEvent), slog.Any("actor", id.NewPrincipal("nested-actor")), slog.String("trace_id", "nested-trace")))
	require.NoError(t, handler.Handle(context.Background(), record))
	require.Len(t, appender.events, 1)
	event := appender.events[0]
	require.NotNil(t, event.Principal)
	require.NotNil(t, event.Actor)
	assert.Equal(t, policy("resource-owner"), *event.Principal)
	assert.Equal(t, policy("request-actor"), *event.Actor)
	assert.Equal(t, "trace-value", event.TraceID)
	decoded := decodeLedgerJSON(t, operational.Bytes())
	assert.Equal(t, decoded["event"], event.Event)
	assert.Equal(t, decoded["principal"], *event.Principal)
	assert.Equal(t, decoded["actor"], *event.Actor)
	for _, key := range []string{"event", "principal", "actor", "trace_id"} {
		delete(decoded, key)
	}
	assert.Equal(t, decoded, normalizeLedgerJSON(t, event.Attributes))
	assert.Equal(t, "final", event.Attributes["ordinary"])
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	for _, raw := range []string{"old-owner", "resource-owner", "request-actor", "operator-subject", "group-owner", "nested-actor"} {
		assert.NotContains(t, string(encoded), raw)
	}
	assert.Equal(t, policy("operator-subject"), event.Attributes["operator_principal"])
}

func TestLedgerHandlerWithGroupScopesOnlyLaterAttributes(t *testing.T) {
	appender := &ledgerCaptureAppender{}
	var operational bytes.Buffer
	handler := NewPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, nil), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second)))
	handler = handler.WithAttrs([]slog.Attr{slog.Any("event", ledgerTestEvent), slog.String("scope", "root")})
	handler = slog.New(handler).WithGroup("").Handler()
	handler = handler.WithGroup("details").WithAttrs([]slog.Attr{slog.String("scope", "inherited"), slog.Any("principal", id.NewPrincipal("nested-owner"))})
	handler = handler.WithGroup("inner")
	require.NoError(t, handler.Handle(context.Background(), ledgerRecord(slog.String("scope", "record"), slog.Any("event", ledgerOtherEvent))))
	require.Len(t, appender.events, 1)
	decoded := decodeLedgerJSON(t, operational.Bytes())
	delete(decoded, "event")
	assert.Equal(t, decoded, normalizeLedgerJSON(t, appender.events[0].Attributes))
	assert.Equal(t, ledgerTestEvent.Name(), appender.events[0].Event)
	assert.Nil(t, appender.events[0].Principal)
}

func TestLedgerHandlerPromotesGroupedRequestIdentity(t *testing.T) {
	const actor = "request-actor"
	const traceID = "context-trace"
	policy := func(value string) string { return "policy:" + value }
	for _, test := range []struct {
		name  string
		ctx   context.Context
		actor bool
		trace string
	}{
		{name: "request", ctx: security.WithSecurityContext(t.Context(), security.SecurityContext{Actor: id.NewPrincipal(actor), TraceID: traceID}), actor: true, trace: traceID},
		{name: "span only", ctx: trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})), trace: trace.TraceID{1}.String()},
		{name: "no context identity", ctx: t.Context()},
	} {
		t.Run(test.name, func(t *testing.T) {
			appender := &ledgerCaptureAppender{}
			var operational bytes.Buffer
			fanout := NewMultiHandler(slog.NewJSONHandler(&operational, nil), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second))
			logger := slog.New(NewContextHandler(newPrincipalPolicyHandler(fanout, policy))).With("event", ledgerTestEvent).WithGroup("outer").WithGroup("details")
			logger.InfoContext(test.ctx, "domain event recorded", "actor", id.NewPrincipal("group-actor"), "trace_id", "group-trace")

			require.Len(t, appender.events, 1)
			event := appender.events[0]
			if test.actor {
				require.NotNil(t, event.Actor)
				assert.Equal(t, policy(actor), *event.Actor)
			} else {
				assert.Nil(t, event.Actor)
			}
			assert.Equal(t, test.trace, event.TraceID)
			decoded := decodeLedgerJSON(t, operational.Bytes())
			delete(decoded, "event")
			assert.Equal(t, decoded, normalizeLedgerJSON(t, event.Attributes), "keep the operational group intact")
		})
	}
}

func TestLedgerHandlerLevelIndependenceAndCancellation(t *testing.T) {
	appender := &ledgerCaptureAppender{}
	var operational bytes.Buffer
	ledger := NewLedgerHandler(appender, enabledNoopHandler{}, time.Second)
	assert.False(t, ledger.Enabled(context.Background(), slog.LevelDebug))
	for _, level := range []slog.Level{slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		assert.True(t, ledger.Enabled(context.Background(), level))
	}
	logger := slog.New(NewContextHandler(NewPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, &slog.HandlerOptions{Level: slog.LevelError}), ledger))))
	type contextKey struct{}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx, cancel := context.WithCancel(context.WithValue(trace.ContextWithSpanContext(context.Background(), spanContext), contextKey{}, "preserved"))
	cancel()
	var appendContext context.Context
	appender.appendFn = func(ctx context.Context, _ *model.DomainEvent) error {
		appendContext = ctx
		assert.NoError(t, ctx.Err())
		assert.Equal(t, "preserved", ctx.Value(contextKey{}))
		assert.Equal(t, spanContext, trace.SpanContextFromContext(ctx))
		deadline, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(time.Second), deadline, 100*time.Millisecond)
		return nil
	}
	logger.InfoContext(ctx, "domain event recorded", "event", ledgerTestEvent)
	require.Len(t, appender.events, 1)
	assert.Empty(t, operational.String())
	require.NotNil(t, appendContext)
	assert.ErrorIs(t, appendContext.Err(), context.Canceled, "append timeout resources are released")
	logger.DebugContext(ctx, "debug ignored", "event", ledgerTestEvent)
	assert.Len(t, appender.events, 1)
}

func ledgerMetricReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); assert.NoError(t, provider.Shutdown(context.Background())) })
	return reader
}

func TestLedgerHandlerWriteMetricsAndSafeFallback(t *testing.T) {
	reader := ledgerMetricReader(t)
	appender := &ledgerCaptureAppender{}
	var fallback bytes.Buffer
	fallbackHandler := NewContextHandler(newPrincipalPolicyHandler(slog.NewJSONHandler(&fallback, nil), func(string) string { return "policy-actor" }))
	handler := NewLedgerHandler(appender, fallbackHandler, time.Second)
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{TraceID: "request-trace", Actor: id.NewPrincipal("raw-actor"), CallingPeer: "gateway"})
	require.NoError(t, handler.Handle(ctx, ledgerRecord(slog.Any("event", ledgerTestEvent))))
	kinds := []storage.ErrorKind{storage.ErrorKindConnection, storage.ErrorKindTimeout, storage.ErrorKindValidation, storage.ErrorKindNotFound, storage.ErrorKindConflict, storage.ErrorKindUnknown}
	for _, kind := range kinds {
		appender.appendFn = func(context.Context, *model.DomainEvent) error {
			return fmt.Errorf("wrapped: %w", &storage.StorageError{Kind: kind, Cause: errors.New("raw-cause-must-not-leak")})
		}
		require.NoError(t, handler.Handle(ctx, ledgerRecord(slog.Any("event", ledgerTestEvent), slog.Any("principal", id.NewPrincipal("failed-owner")), slog.String("sensitive_attribute", "must-not-copy"))))
	}
	appender.appendFn = func(context.Context, *model.DomainEvent) error { return errors.New("raw-unclassified-error") }
	require.NoError(t, handler.Handle(ctx, ledgerRecord(slog.Any("event", ledgerTestEvent))))
	lines := bytes.Split(bytes.TrimSpace(fallback.Bytes()), []byte("\n"))
	require.Len(t, lines, len(kinds)+1)
	for i, line := range lines {
		attrs := decodeLedgerJSON(t, line)
		assert.Equal(t, "domain_event.ledger.write_failed", attrs["event"])
		assert.Equal(t, ledgerTestEvent.Name(), attrs["failed_event"])
		assert.Equal(t, "request-trace", attrs["trace_id"])
		assert.Equal(t, "policy-actor", attrs["actor"])
		assert.Equal(t, "gateway", attrs["calling_peer"])
		if i < len(kinds) {
			assert.Equal(t, string(kinds[i]), attrs["error_kind"])
		} else {
			assert.Equal(t, "unknown", attrs["error_kind"])
		}
		assert.Contains(t, attrs, "duration_ms")
		assert.Len(t, attrs, 7)
		assert.NotContains(t, string(line), "raw-")
		assert.NotContains(t, string(line), "failed-owner")
		assert.NotContains(t, string(line), "must-not-copy")
		var raw map[string]any
		require.NoError(t, json.Unmarshal(line, &raw))
		assert.Equal(t, "ERROR", raw["level"])
	}
	assert.Len(t, appender.events, len(kinds)+2, "failure diagnostics never append")
	assertLedgerMetrics(t, reader, map[string]uint64{"stored": 1, "failed": uint64(len(kinds) + 1)}, map[string]int64{"connection": 1, "timeout": 1, "validation": 1, "not_found": 1, "conflict": 1, "unknown": 2})
}

func TestLedgerHandlerAppendTimeout(t *testing.T) {
	reader := ledgerMetricReader(t)
	appender := &ledgerCaptureAppender{appendFn: func(ctx context.Context, _ *model.DomainEvent) error {
		<-ctx.Done()
		return fmt.Errorf("append: %w", ctx.Err())
	}}
	var fallback bytes.Buffer
	handler := NewLedgerHandler(appender, NewContextHandler(NewPrincipalPolicyHandler(slog.NewJSONHandler(&fallback, nil))), 20*time.Millisecond)
	started := time.Now()
	require.NoError(t, handler.Handle(context.Background(), ledgerRecord(slog.Any("event", ledgerTestEvent))))
	assert.Less(t, time.Since(started), time.Second)
	assert.Equal(t, "timeout", decodeLedgerJSON(t, fallback.Bytes())["error_kind"])
	assertLedgerMetrics(t, reader, map[string]uint64{"failed": 1}, map[string]int64{"timeout": 1})
}

func assertLedgerMetrics(t *testing.T, reader *sdkmetric.ManualReader, durations map[string]uint64, failures map[string]int64) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	gotDurations := map[string]uint64{}
	gotFailures := map[string]int64{}
	for _, scope := range collected.ScopeMetrics {
		if scope.Scope.Name != "domain_events" {
			continue
		}
		for _, instrument := range scope.Metrics {
			assert.NotEmpty(t, instrument.Description)
			switch instrument.Name {
			case "domain_events.write.duration":
				assert.Equal(t, "s", instrument.Unit)
				data, ok := instrument.Data.(metricdata.Histogram[float64])
				require.True(t, ok)
				for _, point := range data.DataPoints {
					assert.Equal(t, 1, point.Attributes.Len())
					value, ok := point.Attributes.Value("outcome")
					require.True(t, ok)
					gotDurations[value.AsString()] += point.Count
					assert.GreaterOrEqual(t, point.Sum, float64(0))
				}
			case "domain_events.write_failures":
				assert.Equal(t, "{event}", instrument.Unit)
				data, ok := instrument.Data.(metricdata.Sum[int64])
				require.True(t, ok)
				assert.True(t, data.IsMonotonic)
				for _, point := range data.DataPoints {
					assert.Equal(t, 1, point.Attributes.Len())
					value, ok := point.Attributes.Value("error_kind")
					require.True(t, ok)
					gotFailures[value.AsString()] += point.Value
				}
			default:
				t.Errorf("unexpected ledger instrument %s", instrument.Name)
			}
		}
	}
	assert.Equal(t, durations, gotDurations)
	assert.Equal(t, failures, gotFailures)
}

type ledgerFailingMeterProvider struct{ metric.MeterProvider }

func (p ledgerFailingMeterProvider) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return ledgerFailingMeter{Meter: p.MeterProvider.Meter(name, opts...)}
}

type ledgerFailingMeter struct{ metric.Meter }

func (m ledgerFailingMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	instrument, err := m.Meter.Float64Histogram(name, opts...)
	if err != nil {
		return instrument, err
	}
	return instrument, errors.New("raw-instrument-error-and-principal")
}
func (m ledgerFailingMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	instrument, err := m.Meter.Int64Counter(name, opts...)
	if err != nil {
		return instrument, err
	}
	return instrument, errors.New("raw-instrument-error-and-principal")
}

func TestLedgerHandlerInstrumentFailuresWarnAndContinue(t *testing.T) {
	reader := ledgerMetricReader(t)
	otel.SetMeterProvider(ledgerFailingMeterProvider{MeterProvider: otel.GetMeterProvider()})
	appender := &ledgerCaptureAppender{appendFn: func(context.Context, *model.DomainEvent) error {
		return &storage.StorageError{Kind: storage.ErrorKindConnection}
	}}
	var fallback bytes.Buffer
	handler := NewLedgerHandler(appender, NewContextHandler(NewPrincipalPolicyHandler(slog.NewJSONHandler(&fallback, nil))), time.Second)
	lines := bytes.Split(bytes.TrimSpace(fallback.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)
	instruments := []string{}
	for _, line := range lines {
		attrs := decodeLedgerJSON(t, line)
		assert.Len(t, attrs, 2)
		assert.Equal(t, "domain_event.instrument.initialization_failed", attrs["event"])
		instruments = append(instruments, attrs["instrument"].(string))
		assert.NotContains(t, string(line), "raw-instrument")
		var raw map[string]any
		require.NoError(t, json.Unmarshal(line, &raw))
		assert.Equal(t, "WARN", raw["level"])
	}
	assert.ElementsMatch(t, []string{"domain_events.write.duration", "domain_events.write_failures"}, instruments)
	assert.Empty(t, appender.events)
	require.NoError(t, handler.Handle(context.Background(), ledgerRecord(slog.Any("event", ledgerTestEvent))))
	require.NoError(t, handler.WithAttrs(nil).WithGroup("").Handle(context.Background(), ledgerRecord(slog.Any("event", ledgerTestEvent))))
	assert.Len(t, bytes.Split(bytes.TrimSpace(fallback.Bytes()), []byte("\n")), 4, "constructor warnings are not repeated by writes or clones")
	assert.Len(t, appender.events, 2)
	assertLedgerMetrics(t, reader, map[string]uint64{"failed": 2}, map[string]int64{"connection": 2})
}

type ledgerLogExporter struct{ records []sdklog.Record }

func (e *ledgerLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}
func (*ledgerLogExporter) Shutdown(context.Context) error   { return nil }
func (*ledgerLogExporter) ForceFlush(context.Context) error { return nil }

func TestLedgerHandlerIdentifierRendersInJSONAndOTel(t *testing.T) {
	exporter := &ledgerLogExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	t.Cleanup(func() { assert.NoError(t, provider.Shutdown(context.Background())) })
	var operational bytes.Buffer
	appender := &ledgerCaptureAppender{}
	logger := slog.New(NewPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, nil), otelslog.NewHandler("ledger-test", otelslog.WithLoggerProvider(provider)), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second))))
	logger.InfoContext(context.Background(), "domain event recorded", "event", ledgerTestEvent)
	require.Len(t, appender.events, 1)
	assert.Equal(t, ledgerTestEvent.Name(), decodeLedgerJSON(t, operational.Bytes())["event"])
	require.Len(t, exporter.records, 1)
	found := false
	exporter.records[0].WalkAttributes(func(value attribute.KeyValue) bool {
		if value.Key == "event" {
			found = true
			assert.Equal(t, ledgerTestEvent.Name(), value.Value.AsString())
		}
		return true
	})
	assert.True(t, found)
}

func TestLedgerHandlerDuplicateObjectsAndImmutableDerivation(t *testing.T) {
	cases := []struct {
		name   string
		derive func(slog.Handler) slog.Handler
		attrs  []slog.Attr
	}{
		{"inherited ordinary duplicates", func(h slog.Handler) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.String("value", "first")}).WithAttrs([]slog.Attr{slog.String("value", "second")})
		}, []slog.Attr{slog.String("value", "last")}},
		{"explicit group replacement", func(h slog.Handler) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.Group("details", slog.Int("old", 1))})
		}, []slog.Attr{slog.Group("details", slog.Int("new", 2))}},
		{"duplicate inside group", func(h slog.Handler) slog.Handler {
			return h.WithGroup("details").WithAttrs([]slog.Attr{slog.String("value", "first")})
		}, []slog.Attr{slog.String("value", "last")}},
		{"scalar replaced by logger group", func(h slog.Handler) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.String("details", "old")}).WithGroup("details")
		}, []slog.Attr{slog.Int("new", 2)}},
		{"empty group does not override", func(h slog.Handler) slog.Handler { return h.WithAttrs([]slog.Attr{slog.String("value", "kept")}) }, []slog.Attr{slog.Group("value")}},
		{"inline duplicate", func(h slog.Handler) slog.Handler { return h.WithAttrs([]slog.Attr{slog.String("value", "first")}) }, []slog.Attr{slog.Group("", slog.String("value", "last"))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var operational bytes.Buffer
			appender := &ledgerCaptureAppender{}
			handler := NewPrincipalPolicyHandler(NewMultiHandler(slog.NewJSONHandler(&operational, nil), NewLedgerHandler(appender, enabledNoopHandler{}, time.Second)))
			handler = handler.WithAttrs([]slog.Attr{slog.Any("event", ledgerTestEvent)})
			handler = tc.derive(handler)
			require.NoError(t, handler.Handle(context.Background(), ledgerRecord(tc.attrs...)))
			require.Len(t, appender.events, 1)
			decoded := decodeLedgerJSON(t, operational.Bytes())
			delete(decoded, "event")
			assert.Equal(t, decoded, normalizeLedgerJSON(t, appender.events[0].Attributes))
		})
	}
	appender := &ledgerCaptureAppender{}
	root := NewLedgerHandler(appender, enabledNoopHandler{}, time.Second).WithAttrs([]slog.Attr{slog.Any("event", ledgerTestEvent)})
	left := root.WithAttrs([]slog.Attr{slog.String("branch", "left")})
	right := root.WithAttrs([]slog.Attr{slog.String("branch", "right")}).WithGroup("details")
	require.NoError(t, left.Handle(context.Background(), ledgerRecord()))
	require.NoError(t, right.Handle(context.Background(), ledgerRecord(slog.Int("count", 1))))
	require.NoError(t, root.Handle(context.Background(), ledgerRecord()))
	require.Len(t, appender.events, 3)
	assert.Equal(t, map[string]any{"branch": "left"}, normalizeLedgerJSON(t, appender.events[0].Attributes))
	assert.Equal(t, map[string]any{"branch": "right", "details": map[string]any{"count": float64(1)}}, normalizeLedgerJSON(t, appender.events[1].Attributes))
	assert.Empty(t, appender.events[2].Attributes)
	assert.NotEqual(t, appender.events[0].ID, appender.events[1].ID)
}

func TestLedgerHandlerFallbackOwnsContextEnrichment(t *testing.T) {
	appender := &ledgerCaptureAppender{appendFn: func(context.Context, *model.DomainEvent) error {
		return &storage.StorageError{Kind: storage.ErrorKindConnection}
	}}
	var fallback bytes.Buffer
	handler := NewLedgerHandler(appender, slog.NewJSONHandler(&fallback, nil), time.Second)
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{TraceID: "must-not-be-added-here", Actor: id.NewPrincipal("actor-not-added-here"), CallingPeer: "peer-not-added-here"})
	require.NoError(t, handler.Handle(ctx, ledgerRecord(slog.Any("event", ledgerTestEvent))))
	attrs := decodeLedgerJSON(t, fallback.Bytes())
	assert.Len(t, attrs, 4)
	for _, key := range []string{"principal", "actor", "trace_id", "calling_peer", "error"} {
		assert.NotContains(t, attrs, key)
	}
}
