package telemetry

import (
	"context"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// reservedPrincipalKeys carry principals as plain strings, for callers such as
// ExtProc that cannot use id.Principal.
var reservedPrincipalKeys = map[string]struct{}{
	"principal":          {},
	"operator_principal": {},
	"actor":              {},
}

// principalPolicyHandler is the single place where principals are rendered
// into log records. Wire it before any fan-out so every sink sees the same
// output.
type principalPolicyHandler struct {
	next   slog.Handler
	policy func(string) string
}

// NewPrincipalPolicyHandler renders every id.Principal attribute and every
// string attribute under a reserved principal key through the principal
// policy. The current policy is identity.
func NewPrincipalPolicyHandler(next slog.Handler) slog.Handler {
	return newPrincipalPolicyHandler(next, identityPrincipalPolicy)
}

func newPrincipalPolicyHandler(next slog.Handler, policy func(string) string) slog.Handler {
	return &principalPolicyHandler{next: next, policy: policy}
}

func identityPrincipalPolicy(principal string) string {
	return principal
}

func (h *principalPolicyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *principalPolicyHandler) Handle(ctx context.Context, record slog.Record) error {
	rendered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		rendered.AddAttrs(h.render(attr))
		return true
	})
	return h.next.Handle(ctx, rendered)
}

func (h *principalPolicyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &principalPolicyHandler{next: h.next.WithAttrs(h.renderAll(attrs)), policy: h.policy}
}

func (h *principalPolicyHandler) WithGroup(name string) slog.Handler {
	return &principalPolicyHandler{next: h.next.WithGroup(name), policy: h.policy}
}

func (h *principalPolicyHandler) renderAll(attrs []slog.Attr) []slog.Attr {
	rendered := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		rendered[i] = h.render(attr)
	}
	return rendered
}

func (h *principalPolicyHandler) render(attr slog.Attr) slog.Attr {
	if principal, ok := attr.Value.Any().(id.Principal); ok {
		return slog.String(attr.Key, h.policy(principal.String()))
	}

	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindGroup:
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(h.renderAll(value.Group())...)}
	case slog.KindString:
		if _, reserved := reservedPrincipalKeys[attr.Key]; reserved {
			return slog.String(attr.Key, h.policy(value.String()))
		}
	}
	return slog.Attr{Key: attr.Key, Value: value}
}
