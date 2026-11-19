package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

type contextHandler struct {
	next    slog.Handler
	grouped bool
}

type groupedIdentityKey struct{}

type groupedIdentity uint8

const (
	groupedTraceOnly groupedIdentity = iota + 1
	groupedActorAndTrace
)

func NewContextHandler(next slog.Handler) slog.Handler {
	return &contextHandler{next: next}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	var identity groupedIdentity
	if sc, ok := security.FromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", sc.TraceID), slog.Any("actor", sc.Actor))
		identity = groupedActorAndTrace
		if sc.CallingPeer != "" {
			record.AddAttrs(slog.String("calling_peer", sc.CallingPeer))
		}
	} else if holder, ok := security.CaptureHolderFromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", holder.Capture().TraceID), slog.Any("actor", id.NewPrincipal(security.AnonymousActor)))
		identity = groupedActorAndTrace
	} else {
		spanContext := trace.SpanContextFromContext(ctx)
		if spanContext.IsValid() {
			record.AddAttrs(slog.String("trace_id", spanContext.TraceID().String()))
			identity = groupedTraceOnly
		}
	}
	if h.grouped && identity != 0 {
		// Pass only provenance. The principal policy still renders the record before any sink.
		ctx = context.WithValue(ctx, groupedIdentityKey{}, identity)
	}
	return h.next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.next = h.next.WithAttrs(attrs)
	return &clone
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &contextHandler{next: h.next.WithGroup(name), grouped: true}
}
