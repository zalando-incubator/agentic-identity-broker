package telemetry

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

type capturedContextRecord struct {
	message string
	attrs   map[string]any
}

type captureContextHandler struct {
	records *[]capturedContextRecord
	attrs   []slog.Attr
	groups  []string
}

func newCaptureContextHandler() *captureContextHandler {
	records := make([]capturedContextRecord, 0, 1)
	return &captureContextHandler{records: &records}
}

func (h *captureContextHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureContextHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any, record.NumAttrs()+len(h.attrs))
	for _, attr := range h.attrs {
		h.storeAttr(attrs, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		h.storeAttr(attrs, attr)
		return true
	})
	*h.records = append(*h.records, capturedContextRecord{message: record.Message, attrs: attrs})
	return nil
}

func (h *captureContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *captureContextHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *captureContextHandler) storeAttr(dst map[string]any, attr slog.Attr) {
	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string{}, h.groups...), attr.Key), ".")
	}
	dst[key] = attr.Value.Any()
}

func TestContextHandler_UsesSecurityContextFieldsWhenPresent(t *testing.T) {
	t.Parallel()

	ctx := trace.ContextWithSpanContext(context.Background(), mustSpanContext(t,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbb",
	))
	ctx = security.WithSecurityContext(ctx, security.SecurityContext{
		TraceID:     "0123456789abcdef0123456789abcdef",
		Actor:       "alice@example.com",
		CallingPeer: "gateway-client-1",
	})

	base := newCaptureContextHandler()
	logger := slog.New(NewContextHandler(base))
	logger.InfoContext(ctx, "request handled", "component", "telemetry-test")

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "request handled", records[0].message)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", records[0].attrs["trace_id"], "security context must be the trace_id authority when present")
	assert.Equal(t, "alice@example.com", records[0].attrs["actor"])
	assert.Equal(t, "gateway-client-1", records[0].attrs["calling_peer"])
}

func TestContextHandler_OmitsEmptyCallingPeer(t *testing.T) {
	t.Parallel()

	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		TraceID:     "11111111111111111111111111111111",
		Actor:       "anonymous",
		CallingPeer: "",
	})

	base := newCaptureContextHandler()
	logger := slog.New(NewContextHandler(base))
	logger.InfoContext(ctx, "health check")

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "11111111111111111111111111111111", records[0].attrs["trace_id"])
	assert.Equal(t, "anonymous", records[0].attrs["actor"])
	_, hasCallingPeer := records[0].attrs["calling_peer"]
	assert.False(t, hasCallingPeer, "calling_peer must be omitted when the security context does not carry a distinct peer")
}

func TestContextHandler_UsesCapturedTraceBeforeDeferredFinalization(t *testing.T) {
	holder := security.NewCaptureHolder(security.TransportCapture{
		TraceID: "33333333333333333333333333333333",
	})
	ctx := security.WithCaptureHolder(context.Background(), holder)
	base := newCaptureContextHandler()
	logger := slog.New(NewContextHandler(base))

	logger.WarnContext(ctx, "token exchange rejected")

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "33333333333333333333333333333333", records[0].attrs["trace_id"])
	assert.Equal(t, security.AnonymousActor, records[0].attrs["actor"])
	_, finalized := holder.Finalized()
	assert.False(t, finalized, "logging must not finalize the deferred capture holder")
}

func TestContextHandler_FallsBackToSpanTraceIDOnlyWhenSecurityContextAbsent(t *testing.T) {
	t.Parallel()

	ctx := trace.ContextWithSpanContext(context.Background(), mustSpanContext(t,
		"22222222222222222222222222222222",
		"3333333333333333",
	))

	base := newCaptureContextHandler()
	logger := slog.New(NewContextHandler(base))
	logger.InfoContext(ctx, "request handled")

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "22222222222222222222222222222222", records[0].attrs["trace_id"])
	_, hasActor := records[0].attrs["actor"]
	_, hasCallingPeer := records[0].attrs["calling_peer"]
	assert.False(t, hasActor, "actor must come from SecurityContext, not span state")
	assert.False(t, hasCallingPeer, "calling_peer must come from SecurityContext, not span state")
}

func mustSpanContext(t *testing.T, traceIDHex, spanIDHex string) trace.SpanContext {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(traceIDHex)
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex(spanIDHex)
	require.NoError(t, err)

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
}

func TestContextHandler_EnrichesEveryMultiHandlerDestination(t *testing.T) {
	t.Parallel()

	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		TraceID:     "0123456789abcdef0123456789abcdef",
		Actor:       "alice@example.com",
		CallingPeer: "gateway-client-1",
	})
	first := newCaptureContextHandler()
	second := newCaptureContextHandler()
	logger := slog.New(NewContextHandler(NewMultiHandler(first, second)))

	logger.InfoContext(ctx, "request handled")

	for _, destination := range []*captureContextHandler{first, second} {
		records := *destination.records
		require.Len(t, records, 1)
		assert.Equal(t, "0123456789abcdef0123456789abcdef", records[0].attrs["trace_id"])
		assert.Equal(t, "alice@example.com", records[0].attrs["actor"])
		assert.Equal(t, "gateway-client-1", records[0].attrs["calling_peer"])
	}
}
