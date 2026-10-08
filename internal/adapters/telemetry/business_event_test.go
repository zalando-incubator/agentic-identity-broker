package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

const ledgerTestScope = "agentic-identity-broker.ledger"

// This exporter observes records built by the actual SDK provider and processor.
// Clones are required because Export's record slice is valid only for the call.
type ledgerRecordExporter struct {
	records  []sdklog.Record
	exportFn func(context.Context) error
}

func (e *ledgerRecordExporter) Export(ctx context.Context, batch []sdklog.Record) error {
	for i := range batch {
		e.records = append(e.records, batch[i].Clone())
	}
	if e.exportFn != nil {
		return e.exportFn(ctx)
	}
	return nil
}

func (*ledgerRecordExporter) ForceFlush(context.Context) error { return nil }
func (*ledgerRecordExporter) Shutdown(context.Context) error   { return nil }

func ledgerSDKProvider(t *testing.T, exporter *ledgerRecordExporter) *BusinessEventProvider {
	t.Helper()
	provider := newBusinessEventProvider(exporter, resource.NewSchemaless(attribute.String("service.name", "ledger-test")))
	require.NotNil(t, provider)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	return provider
}

func ledgerTestEvent() *model.BusinessEvent {
	subject := id.NewPrincipal("affected-principal")
	caller := "verified-caller"
	delegate := id.NewPrincipal("represented-principal")
	occurred := time.Date(2026, 9, 26, 12, 34, 56, 123456000, time.UTC)
	return &model.BusinessEvent{
		ID:              id.MustParseBusinessEventID("01997100-0000-7000-8000-000000000001"),
		Type:            model.BusinessEventTypePrefix + "token-exchanged",
		Source:          model.BusinessEventSource,
		OccurredAt:      occurred,
		RecordedAt:      occurred.Add(2 * time.Second),
		Subject:         &subject,
		Actor:           model.BusinessEventActor{Kind: "agent", ID: &caller, OnBehalfOf: &delegate},
		AgentID:         id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		GatewayClientID: id.NewClientID("trusted-gateway"),
		ServiceID:       id.MustParseServiceID("22222222-2222-4222-8222-222222222222"),
		PermissionSetIDs: []id.PermissionSetID{
			id.MustParsePermissionSetID("33333333-3333-4333-8333-333333333333"),
			id.MustParsePermissionSetID("44444444-4444-4444-8444-444444444444"),
		},
		GrantID:     id.MustParseGrantID("55555555-5555-4555-8555-555555555555"),
		SessionID:   id.MustParseSessionID("66666666-6666-4666-8666-666666666666"),
		ApprovalID:  id.MustParseApprovalID("77777777-7777-4777-8777-777777777777"),
		Outcome:     model.BusinessEventSuccess,
		ReasonUser:  "The broker completes a token exchange for a receiving agent.",
		ReasonAdmin: "The broker completes a token exchange for a receiving agent.",
		Client:      &model.BusinessEventClient{IP: "192.0.2.1", UserAgent: "curl"},
		Data:        map[string]any{},
	}
}

func ledgerSDKAttributes(t *testing.T, record sdklog.Record) map[string]attribute.Value {
	t.Helper()
	attrs := make(map[string]attribute.Value, record.AttributesLen())
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		_, duplicate := attrs[string(kv.Key)]
		require.False(t, duplicate, "SDK must not overwrite colliding envelope paths")
		require.NotEqual(t, attribute.MAP, kv.Value.Type(), "ledger attributes must be flat")
		attrs[string(kv.Key)] = kv.Value
		return true
	})
	require.Len(t, attrs, record.AttributesLen())
	return attrs
}

func TestBusinessEventExportsFullFlatEnvelopeAndNativeCorrelation(t *testing.T) {
	// The encoder receives the primitive view after schema validation. Extra
	// data shapes exercise typed arrays, which the initial catalogue rarely uses.
	event := ledgerTestEvent()
	event.TraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	event.SpanID = "00f067aa0ba902b7"
	event.SetTrustedSessionCorrelations("mcp-trusted", "agent-trusted")
	event.Data = map[string]any{
		"active": true, "attempts": int64(7), "ratio": 0.75,
		"labels": []string{"first", "second"}, "counts": []int64{2, 3},
		"rates": []float64{0.5, 1.5}, "flags": []bool{false, true},
		"review": map[string]any{"decision": "allowed"},
	}
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	workerTrace, err := trace.TraceIDFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	workerSpan, err := trace.SpanIDFromHex("bbbbbbbbbbbbbbbb")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: workerTrace, SpanID: workerSpan, TraceFlags: trace.FlagsSampled,
	}))
	started := time.Now()
	require.NoError(t, provider.Emit(ctx, event))
	finished := time.Now()
	require.Len(t, exporter.records, 1, "success requires a synchronous SDK export")
	record := exporter.records[0]
	require.Equal(t, ledgerTestScope, record.InstrumentationScope().Name)
	require.Equal(t, event.Type, record.EventName())
	require.Equal(t, event.OccurredAt, record.Timestamp())
	require.False(t, record.ObservedTimestamp().Before(started))
	require.False(t, record.ObservedTimestamp().After(finished))
	require.NotEqual(t, event.RecordedAt, record.ObservedTimestamp(), "observed time is export time, not persistence time")
	require.Equal(t, attribute.EMPTY, record.Body().Type(), "no unsanitized second channel")
	nativeTrace, err := trace.TraceIDFromHex(event.TraceID)
	require.NoError(t, err)
	nativeSpan, err := trace.SpanIDFromHex(event.SpanID)
	require.NoError(t, err)
	require.Equal(t, nativeTrace, record.TraceID(), "native correlation comes from the retained envelope")
	require.Equal(t, nativeSpan, record.SpanID())
	require.NotEqual(t, workerTrace, record.TraceID())
	require.NotEqual(t, workerSpan, record.SpanID())
	require.Zero(t, record.DroppedAttributes())

	want := map[string]attribute.Value{
		"id":                 attribute.StringValue(event.ID.String()),
		"type":               attribute.StringValue(event.Type),
		"source":             attribute.StringValue(model.BusinessEventSource),
		"occurred_at":        attribute.StringValue("2026-09-26T12:34:56.123456Z"),
		"recorded_at":        attribute.StringValue("2026-09-26T12:34:58.123456Z"),
		"subject":            attribute.StringValue("affected-principal"),
		"actor.kind":         attribute.StringValue("agent"),
		"actor.id":           attribute.StringValue("verified-caller"),
		"actor.on_behalf_of": attribute.StringValue("represented-principal"),
		"agent_id":           attribute.StringValue(event.AgentID.String()),
		"gateway_client_id":  attribute.StringValue("trusted-gateway"),
		"service_id":         attribute.StringValue(event.ServiceID.String()),
		"permission_set_ids": attribute.StringSliceValue([]string{
			"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444",
		}),
		"grant_id":             attribute.StringValue(event.GrantID.String()),
		"session_id":           attribute.StringValue(event.SessionID.String()),
		"approval_id":          attribute.StringValue(event.ApprovalID.String()),
		"mcp_session_id":       attribute.StringValue("mcp-trusted"),
		"agent_session_id":     attribute.StringValue("agent-trusted"),
		"outcome":              attribute.StringValue("success"),
		"reason_user":          attribute.StringValue(event.ReasonUser),
		"reason_admin":         attribute.StringValue(event.ReasonAdmin),
		"trace_id":             attribute.StringValue(event.TraceID),
		"span_id":              attribute.StringValue(event.SpanID),
		"client.ip":            attribute.StringValue("192.0.2.1"),
		"client.user_agent":    attribute.StringValue("curl"),
		"data.active":          attribute.BoolValue(true),
		"data.attempts":        attribute.Int64Value(7),
		"data.ratio":           attribute.Float64Value(0.75),
		"data.labels":          attribute.StringSliceValue([]string{"first", "second"}),
		"data.counts":          attribute.Int64SliceValue([]int64{2, 3}),
		"data.rates":           attribute.Float64SliceValue([]float64{0.5, 1.5}),
		"data.flags":           attribute.BoolSliceValue([]bool{false, true}),
		"data.review.decision": attribute.StringValue("allowed"),
	}
	require.Equal(t, want, ledgerSDKAttributes(t, record), "every literal path is exported exactly once")
	require.Equal(t, len(want), record.AttributesLen())
	var paths []string
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		paths = append(paths, string(kv.Key))
		return true
	})
	wantPaths := slices.Clone(paths)
	slices.Sort(wantPaths)
	require.Equal(t, wantPaths, paths, "field traversal must be deterministic and sorted")
}

func TestBusinessEventExportsEverySupportedNumericWidthAndArray(t *testing.T) {
	event := ledgerTestEvent()
	event.Data = map[string]any{
		"int": int(-7), "int8": int8(-8), "int16": int16(-16), "int32": int32(7),
		"int64": int64(9007199254740993), "uint": uint(7), "uint8": uint8(8),
		"uint16": uint16(16), "uint32": uint32(32), "uint64": uint64(math.MaxInt64),
		"uint64_float": uint64(1) << 63,
		"float32":      float32(0.1), "float64": float64(0.1),
		"json_integer": json.Number("9007199254740993"), "json_fraction": json.Number("0.1"),
		"json_large_exact": json.Number("9223372036854775808"),
		"ints":             []int{-7, 7}, "int8s": []int8{-8, 8}, "int16s": []int16{-16, 16},
		"int32s": []int32{7, -7}, "int64s": []int64{math.MinInt64, 9007199254740993},
		"uints": []uint{7, 9}, "uint16s": []uint16{16, 17},
		"uint32s": []uint32{32, 33}, "uint64s": []uint64{9007199254740993, math.MaxInt64},
		"uint64_floats": []uint64{uint64(1) << 63, (uint64(1) << 63) + 2048},
		"float32s":      []float32{0.1, 0.25}, "float64s": []float64{0.1, 0.25},
		"json_integers":  []any{json.Number("-9007199254740993"), json.Number("9007199254740993")},
		"json_fractions": []any{json.Number("0.1"), json.Number("0.125")},
		"mixed_integers": []any{int32(7), json.Number("9007199254740993")},
		"mixed_floats":   []any{int32(7), float32(0.1)},
	}
	wantData := map[string]attribute.Value{
		"data.int": attribute.Int64Value(-7), "data.int8": attribute.Int64Value(-8),
		"data.int16": attribute.Int64Value(-16), "data.int32": attribute.Int64Value(7),
		"data.int64": attribute.Int64Value(9007199254740993), "data.uint": attribute.Int64Value(7),
		"data.uint8": attribute.Int64Value(8), "data.uint16": attribute.Int64Value(16),
		"data.uint32": attribute.Int64Value(32), "data.uint64": attribute.Int64Value(math.MaxInt64),
		"data.uint64_float":     attribute.Float64Value(float64(uint64(1) << 63)),
		"data.float32":          attribute.Float64Value(0.1),
		"data.float64":          attribute.Float64Value(0.1),
		"data.json_integer":     attribute.Int64Value(9007199254740993),
		"data.json_fraction":    attribute.Float64Value(0.1),
		"data.json_large_exact": attribute.Float64Value(float64(uint64(1) << 63)),
		"data.ints":             attribute.Int64SliceValue([]int64{-7, 7}),
		"data.int8s":            attribute.Int64SliceValue([]int64{-8, 8}),
		"data.int16s":           attribute.Int64SliceValue([]int64{-16, 16}),
		"data.int32s":           attribute.Int64SliceValue([]int64{7, -7}),
		"data.int64s":           attribute.Int64SliceValue([]int64{math.MinInt64, 9007199254740993}),
		"data.uints":            attribute.Int64SliceValue([]int64{7, 9}),
		"data.uint16s":          attribute.Int64SliceValue([]int64{16, 17}),
		"data.uint32s":          attribute.Int64SliceValue([]int64{32, 33}),
		"data.uint64s":          attribute.Int64SliceValue([]int64{9007199254740993, math.MaxInt64}),
		"data.uint64_floats":    attribute.Float64SliceValue([]float64{float64(uint64(1) << 63), float64((uint64(1) << 63) + 2048)}),
		"data.float32s":         attribute.Float64SliceValue([]float64{0.1, 0.25}),
		"data.float64s":         attribute.Float64SliceValue([]float64{0.1, 0.25}),
		"data.json_integers":    attribute.SliceValue(attribute.Int64Value(-9007199254740993), attribute.Int64Value(9007199254740993)),
		"data.json_fractions":   attribute.SliceValue(attribute.Float64Value(0.1), attribute.Float64Value(0.125)),
		"data.mixed_integers":   attribute.SliceValue(attribute.Int64Value(7), attribute.Int64Value(9007199254740993)),
		"data.mixed_floats":     attribute.SliceValue(attribute.Float64Value(7), attribute.Float64Value(0.1)),
	}
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	require.NoError(t, provider.Emit(context.Background(), event))
	require.Len(t, exporter.records, 1, "success requires the real synchronous SDK export")
	record := exporter.records[0]
	require.Zero(t, record.DroppedAttributes())
	attrs := ledgerSDKAttributes(t, record)
	require.Equal(t, attribute.StringValue(event.ID.String()), attrs["id"], "the original event ID must survive export")
	gotData := make(map[string]attribute.Value, len(wantData))
	for key, value := range attrs {
		if strings.HasPrefix(key, "data.") {
			gotData[key] = value
		}
	}
	require.Len(t, gotData, len(wantData), "no numeric field may be lost")
	for key, want := range wantData {
		require.Contains(t, gotData, key)
		require.Equal(t, ledgerNumericAttribute(want), ledgerNumericAttribute(gotData[key]), "numeric OTLP value %s must not change", key)
	}
}

func ledgerNumericAttribute(value attribute.Value) any {
	if value.Type() == attribute.SLICE {
		items := value.AsSlice()
		values := make([]any, len(items))
		for i, item := range items {
			values[i] = ledgerNumericAttribute(item)
		}
		return values
	}
	items := reflect.ValueOf(value.AsInterface())
	if items.Kind() == reflect.Slice {
		values := make([]any, items.Len())
		for i := range items.Len() {
			values[i] = items.Index(i).Interface()
		}
		return values
	}
	return value.AsInterface()
}

func TestBusinessEventRejectsIntegersOTLPCannotRepresentWithoutRounding(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"unsigned scalar", uint64(math.MaxUint64)},
		{"unsigned typed array", []uint64{7, math.MaxUint64}},
		{"JSON scalar", json.Number("18446744073709551615")},
		{"JSON array", []any{json.Number("7"), json.Number("18446744073709551615")}},
		{"mixed array would round integer", []any{json.Number("9007199254740993"), json.Number("0.5")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := ledgerTestEvent()
			event.Data = map[string]any{"value": tc.value}
			exporter := &ledgerRecordExporter{}
			provider := ledgerSDKProvider(t, exporter)
			require.Error(t, provider.Emit(context.Background(), event), "a rounded integer cannot be acknowledged")
			require.Empty(t, exporter.records, "a rejected numeric attribute must not export a partial envelope")
		})
	}
}

func TestBusinessEventExportsExactNumbersThroughOTLPHTTP(t *testing.T) {
	requests := make(chan []byte, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/logs" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	t.Cleanup(collector.Close)
	cfg := minimalEnabledConfig("http")
	cfg.Exporter.Endpoint = collector.URL
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider, err := NewBusinessEventProvider(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	event := ledgerTestEvent()
	event.Data = map[string]any{
		"count": int32(7), "ratio": float32(0.1),
		"large":  json.Number("9007199254740993"),
		"counts": []any{json.Number("7"), json.Number("9007199254740993")},
	}
	require.NoError(t, provider.Emit(ctx, event))
	require.Len(t, requests, 1, "Emit must complete the HTTP collector call synchronously")
	logs := new(collogpb.ExportLogsServiceRequest)
	require.NoError(t, proto.Unmarshal(<-requests, logs))
	require.Len(t, logs.GetResourceLogs(), 1)
	require.Len(t, logs.GetResourceLogs()[0].GetScopeLogs(), 1)
	require.Len(t, logs.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords(), 1)
	record := logs.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()[0]
	require.Zero(t, record.GetDroppedAttributesCount())
	attrs := make(map[string]*commonpb.AnyValue, len(record.GetAttributes()))
	for _, kv := range record.GetAttributes() {
		require.NotContains(t, attrs, kv.GetKey())
		attrs[kv.GetKey()] = kv.GetValue()
	}
	require.Equal(t, event.ID.String(), attrs["id"].GetStringValue())
	require.Equal(t, int64(7), attrs["data.count"].GetIntValue())
	require.Equal(t, 0.1, attrs["data.ratio"].GetDoubleValue())
	require.Equal(t, int64(9007199254740993), attrs["data.large"].GetIntValue())
	require.NotNil(t, attrs["data.counts"].GetArrayValue())
	counts := attrs["data.counts"].GetArrayValue().GetValues()
	require.Len(t, counts, 2)
	require.Equal(t, int64(7), counts[0].GetIntValue())
	require.Equal(t, int64(9007199254740993), counts[1].GetIntValue())
}

func TestBusinessEventDistinguishesNullAbsentEmptyAndSortedMarkers(t *testing.T) {
	event := ledgerTestEvent()
	event.Subject = nil
	event.Actor.ID = nil
	event.Actor.OnBehalfOf = nil
	event.Client = nil
	event.PermissionSetIDs = []id.PermissionSetID{}
	event.Data = map[string]any{
		"z_null": nil, "a_null": nil,
		"z_object": map[string]any{}, "a_object": map[string]any{},
		"empty_text": "", "empty_array": []string{},
		"review": map[string]any{"not_set": nil, "empty_object": map[string]any{}},
	}
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	require.NoError(t, provider.Emit(context.Background(), event))
	require.Len(t, exporter.records, 1)
	record := exporter.records[0]
	attrs := ledgerSDKAttributes(t, record)
	require.Equal(t, attribute.StringSliceValue([]string{
		"actor.id", "actor.on_behalf_of", "data.a_null", "data.review.not_set", "data.z_null", "subject",
	}), attrs["_ledger.null_fields"])
	require.Equal(t, attribute.StringSliceValue([]string{"data.a_object", "data.review.empty_object", "data.z_object"}), attrs["_ledger.empty_objects"])
	require.Equal(t, attribute.StringValue(""), attrs["data.empty_text"])
	require.Contains(t, attrs, "data.empty_array")
	require.Equal(t, attribute.STRINGSLICE, attrs["data.empty_array"].Type())
	require.Empty(t, attrs["data.empty_array"].AsStringSlice())
	require.Contains(t, attrs, "permission_set_ids", "present empty ID set must differ from absent optional set")
	require.Equal(t, attribute.STRINGSLICE, attrs["permission_set_ids"].Type())
	require.Empty(t, attrs["permission_set_ids"].AsStringSlice())
	for _, absent := range []string{"subject", "actor.id", "actor.on_behalf_of", "data.a_null", "data.review.not_set", "data.z_null", "data.a_object", "data.review.empty_object", "data.z_object", "client.ip", "client.user_agent", "trace_id", "span_id"} {
		require.NotContains(t, attrs, absent)
	}
	require.Zero(t, record.TraceID())
	require.Zero(t, record.SpanID())
	require.Zero(t, record.DroppedAttributes())

	event.PermissionSetIDs = nil
	event.Data = map[string]any{}
	exporter.records = nil
	require.NoError(t, provider.Emit(context.Background(), event))
	require.Len(t, exporter.records, 1)
	attrs = ledgerSDKAttributes(t, exporter.records[0])
	require.NotContains(t, attrs, "permission_set_ids")
	require.Equal(t, attribute.StringSliceValue([]string{"data"}), attrs["_ledger.empty_objects"])
}

func TestBusinessEventDoesNotInheritWorkerTraceWhenEnvelopeHasNoCorrelation(t *testing.T) {
	event := ledgerTestEvent()
	workerTrace, err := trace.TraceIDFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	workerSpan, err := trace.SpanIDFromHex("bbbbbbbbbbbbbbbb")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: workerTrace, SpanID: workerSpan, TraceFlags: trace.FlagsSampled,
	}))
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	require.NoError(t, provider.Emit(ctx, event))
	require.Len(t, exporter.records, 1)
	record := exporter.records[0]
	require.Zero(t, record.TraceID())
	require.Zero(t, record.SpanID())
	require.NotContains(t, ledgerSDKAttributes(t, record), "trace_id")
	require.NotContains(t, ledgerSDKAttributes(t, record), "span_id")
}

func TestBusinessEventTraceWithoutEnvelopeSpanCannotAcquireWorkerSpan(t *testing.T) {
	event := ledgerTestEvent()
	event.TraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	workerTrace, err := trace.TraceIDFromHex(event.TraceID)
	require.NoError(t, err)
	workerSpan, err := trace.SpanIDFromHex("bbbbbbbbbbbbbbbb")
	require.NoError(t, err)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: workerTrace, SpanID: workerSpan, TraceFlags: trace.FlagsSampled,
	}))
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	require.NoError(t, provider.Emit(ctx, event))
	require.Len(t, exporter.records, 1)
	record := exporter.records[0]
	require.Equal(t, workerTrace, record.TraceID(), "envelope trace may have no request span")
	require.Zero(t, record.SpanID(), "worker span must never fill an absent envelope span")
	require.Equal(t, attribute.StringValue(event.TraceID), ledgerSDKAttributes(t, record)["trace_id"])
	require.NotContains(t, ledgerSDKAttributes(t, record), "span_id")
}

func TestBusinessEventRejectsUnsafeAndCollidingPathsBeforeExport(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{"dotted property", map[string]any{"review.decision": "allowed"}},
		{"nested dotted property", map[string]any{"review": map[string]any{"action.name": "allowed"}}},
		{"reserved marker prefix", map[string]any{"_ledger.null_fields": "forged"}},
		{"nested reserved marker prefix", map[string]any{"review": map[string]any{"_ledger_fake": true}}},
		{"empty property name", map[string]any{"": true}},
		{"colliding flattened path", map[string]any{"review.decision": "no", "review": map[string]any{"decision": "yes"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := ledgerTestEvent()
			event.Data = tc.data
			_, _, err := encodeBusinessEvent(event)
			require.Error(t, err)
			exporter := &ledgerRecordExporter{}
			provider := ledgerSDKProvider(t, exporter)
			require.Error(t, provider.Emit(context.Background(), event))
			require.Empty(t, exporter.records, "an unsafe envelope must not reach the exporter")
		})
	}
}

func TestBusinessEventOverridesEnvironmentAttributeTruncation(t *testing.T) {
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_COUNT_LIMIT", "1")
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "3")
	event := ledgerTestEvent()
	event.ReasonUser = "A long, fixed credential-free reason must remain whole."
	event.Data = map[string]any{"tags": []string{"first-long-tag", "second-long-tag"}}
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	require.NoError(t, provider.Emit(context.Background(), event))
	require.Len(t, exporter.records, 1)
	record := exporter.records[0]
	attrs := ledgerSDKAttributes(t, record)
	require.Greater(t, record.AttributesLen(), 1)
	require.Zero(t, record.DroppedAttributes())
	require.Equal(t, attribute.StringValue(event.ReasonUser), attrs["reason_user"])
	require.Equal(t, attribute.StringSliceValue([]string{"first-long-tag", "second-long-tag"}), attrs["data.tags"])
	require.Contains(t, attrs, "recorded_at")
}

func TestBusinessEventRejectsDroppedSDKAttributesBeforeExport(t *testing.T) {
	exporter := &ledgerRecordExporter{}
	processor := &businessEventProcessor{Processor: sdklog.NewSimpleProcessor(exporter)}
	capped := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithAttributeCountLimit(1),
	)
	provider := &BusinessEventProvider{logger: capped.Logger(ledgerTestScope), sdk: capped}
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	require.Error(t, provider.Emit(context.Background(), ledgerTestEvent()), "a returned SDK Emit cannot acknowledge a dropped envelope")
	require.Empty(t, exporter.records, "incomplete SDK records must not be sent")
}

func TestBusinessEventMissingExportCallbackNeverAcknowledges(t *testing.T) {
	// The OTel API's noop Logger.Emit returns no error and produces no callback.
	// An Emit return alone cannot prove that a collector received the event.
	provider := &BusinessEventProvider{logger: noop.NewLoggerProvider().Logger(ledgerTestScope)}
	require.Error(t, provider.Emit(context.Background(), ledgerTestEvent()))
}

func TestBusinessEventFailedExportNeverAcknowledgesAndCanRetrySameID(t *testing.T) {
	failure := errors.New("collector rejected ledger log")
	exporter := &ledgerRecordExporter{exportFn: func(context.Context) error { return failure }}
	provider := ledgerSDKProvider(t, exporter)
	event := ledgerTestEvent()
	require.ErrorIs(t, provider.Emit(context.Background(), event), failure)
	require.Len(t, exporter.records, 1, "an attempted export is not an acknowledged export")
	firstID := ledgerSDKAttributes(t, exporter.records[0])["id"]
	exporter.exportFn = nil
	require.NoError(t, provider.Emit(context.Background(), event))
	require.Len(t, exporter.records, 2)
	require.Equal(t, firstID, ledgerSDKAttributes(t, exporter.records[1])["id"])
}

func TestBusinessEventCancellationDuringSynchronousExportNeverAcknowledges(t *testing.T) {
	entered := make(chan struct{})
	exporter := &ledgerRecordExporter{exportFn: func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}
	provider := ledgerSDKProvider(t, exporter)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- provider.Emit(ctx, ledgerTestEvent()) }()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("emission returned before synchronous export: %v", err)
	case <-ctx.Done():
		t.Fatal("export did not start before deadline")
	}
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("canceled export did not finish")
	}
	require.Len(t, exporter.records, 1, "export attempt did not deliver the event")
}

func TestBusinessEventCanceledBeforeExportNeverAcknowledges(t *testing.T) {
	exporter := &ledgerRecordExporter{}
	provider := ledgerSDKProvider(t, exporter)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, provider.Emit(ctx, ledgerTestEvent()), context.Canceled)
	require.Empty(t, exporter.records)
}

func TestBusinessEventCanceledAfterExporterReportsSuccessNeverAcknowledges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exporter := &ledgerRecordExporter{exportFn: func(context.Context) error {
		cancel()
		return nil
	}}
	provider := ledgerSDKProvider(t, exporter)
	require.ErrorIs(t, provider.Emit(ctx, ledgerTestEvent()), context.Canceled)
	require.Len(t, exporter.records, 1, "an ambiguous post-cancel export can be retried, never acknowledged")
}
