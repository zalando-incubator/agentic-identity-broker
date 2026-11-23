// Package telemetry holds deliberate violations of the coding-standards/OPENTELEMETRY.md lint
// rules.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp" // want depguard `OPENTELEMETRY\.md section 1`
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func Spans(ctx context.Context, p id.Principal, agentID string) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.get.agent", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetStatus(codes.Error, "agent lookup failed")
	span.RecordError(errors.New("agent lookup failed"))                                                 // want forbidigo `OPENTELEMETRY\.md section 3\.4`
	otel.Tracer("billing")                                                                              // want telemetrylint `scope: instrumentation scope "billing" is not in the allowlist`
	otel.Tracer("storage").Start(ctx, "storage.get."+agentID, trace.WithSpanKind(trace.SpanKindClient)) // want telemetrylint `span-name: the span name must be a constant string`
	otel.Tracer("storage").Start(ctx, "storage.get.agent")                                              // want telemetrylint `span-kind: the span start does not set trace.WithSpanKind`
	span.SetAttributes(attribute.String("approval.id", agentID))
	span.SetAttributes(attribute.String(agentID, "a1"))                   // want telemetrylint `attribute-key: the attribute key must be a constant string`
	span.SetAttributes(attribute.String("approval.principal", string(p))) // want gocritic `principal rendered as a string attribute` telemetrylint `principal-key: key "approval\.principal" is reserved`
}

func Metrics(ctx context.Context, logger *slog.Logger, p id.Principal) {
	meter := otel.Meter("approval")
	decisions, err := meter.Int64Counter("approval.decisions",
		metric.WithDescription("Approval decisions."), metric.WithUnit("{decision}"))
	if err != nil {
		logger.WarnContext(ctx, "approval decision counter unavailable", "error", err)
	}
	decisions.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "approved")))
	decisions.Add(ctx, 1, metric.WithAttributes(attribute.String("owner", p.String()))) // want gocritic `principal rendered as a string attribute` telemetrylint `principal-sink: a principal must not be a metric attribute`
	pending, _ := meter.Int64UpDownCounter("approval.pending",                          // want gocritic `instrument constructor error discarded`
		metric.WithDescription("Pending approvals."), metric.WithUnit("{approval}"))
	pending.Add(ctx, 1)
	_, _ = meter.Int64Counter("approvals_pending_total", metric.WithDescription("Pending approvals."), metric.WithUnit("{approval}")) // want gocritic `instrument constructor error discarded` telemetrylint `metric-name: metric name "approvals_pending_total" does not match`
	_, _ = meter.Int64Counter("approval.expired")                                                                                     // want gocritic `instrument constructor error discarded` telemetrylint `metric-metadata: instrument "approval\.expired" does not set metric\.WithDescription` telemetrylint `metric-metadata: instrument "approval\.expired" does not set metric\.WithUnit`
}

func Outbound() *http.Client {
	return &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)} // want forbidigo `telemetryhttp\.NewTransport`
}
