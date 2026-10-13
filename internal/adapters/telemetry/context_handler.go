package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

type contextHandler struct {
	next slog.Handler
}

func NewContextHandler(next slog.Handler) slog.Handler {
	return &contextHandler{next: next}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc, ok := security.FromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", sc.TraceID), slog.String("actor", sc.Actor))
		if sc.CallingPeer != "" {
			record.AddAttrs(slog.String("calling_peer", sc.CallingPeer))
		}
	} else if holder, ok := security.CaptureHolderFromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", holder.Capture().TraceID), slog.String("actor", security.AnonymousActor))
	} else {
		spanContext := trace.SpanContextFromContext(ctx)
		if spanContext.IsValid() {
			record.AddAttrs(slog.String("trace_id", spanContext.TraceID().String()))
		}
	}
	return h.next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}
