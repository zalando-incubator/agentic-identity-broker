package telemetry

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

type contextHandler struct {
	next         slog.Handler
	omitIdentity bool
}

func NewContextHandler(next slog.Handler) slog.Handler {
	return &contextHandler{next: next}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	omitIdentity := h.omitIdentity
	if !omitIdentity {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "event" {
				omitIdentity = strings.HasPrefix(attr.Value.String(), "session.oauth2.")
				return false
			}
			return true
		})
	}
	if sc, ok := security.FromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", sc.TraceID))
		if !omitIdentity {
			record.AddAttrs(slog.String("actor", sc.Actor))
			if sc.CallingPeer != "" {
				record.AddAttrs(slog.String("calling_peer", sc.CallingPeer))
			}
		}
	} else if holder, ok := security.CaptureHolderFromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", holder.Capture().TraceID))
		if !omitIdentity {
			record.AddAttrs(slog.String("actor", security.AnonymousActor))
		}
	} else {
		spanContext := trace.SpanContextFromContext(ctx)
		if spanContext.IsValid() {
			record.AddAttrs(slog.String("trace_id", spanContext.TraceID().String()))
		}
	}

	return h.next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	omitIdentity := h.omitIdentity
	for _, attr := range attrs {
		if attr.Key == "component" && attr.Value.String() == "oauth2session" {
			omitIdentity = true
		}
	}
	return &contextHandler{next: h.next.WithAttrs(attrs), omitIdentity: omitIdentity}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name), omitIdentity: h.omitIdentity}
}
