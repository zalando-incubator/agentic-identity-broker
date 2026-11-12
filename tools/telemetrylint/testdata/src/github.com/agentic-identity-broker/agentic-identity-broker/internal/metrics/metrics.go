package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

var (
	desc = metric.WithDescription("Approval decisions.")
	unit = metric.WithUnit("{decision}")
)

func instruments(dynamic string) {
	meter := otel.Meter("approval")
	meter.Int64Counter("approval.decisions", metric.WithDescription("Approval decisions."), metric.WithUnit("{decision}"))
	meter.Int64Counter("approval.decisions.denied", desc, unit)
	meter.Int64Counter("approval_decisions_total", desc, unit)       // want `metric-name: metric name "approval_decisions_total" does not match <scope>.<noun>\[.<qualifier>\]`
	meter.Int64Counter("approval.gate.decisions.denied", desc, unit) // want `metric-name: metric name "approval.gate.decisions.denied" does not match`
	meter.Int64Counter("extproc.decisions", desc, unit)              // want `metric-name: metric name "extproc.decisions" does not start with the meter scope "approval"`
	meter.Int64Counter(dynamic, desc, unit)                          // want `metric-name: the metric name must be a constant string`
	meter.Int64Counter("approval.decisions")                         // want `metric-metadata: instrument "approval.decisions" does not set metric.WithDescription` `metric-metadata: instrument "approval.decisions" does not set metric.WithUnit`
	options := []metric.Int64CounterOption{desc, unit}
	meter.Int64Counter("approval.decisions", options...)
	otel.GetMeterProvider().Meter("extproc").Float64Histogram("extproc.duration", metric.WithUnit("s")) // want `metric-metadata: instrument "extproc.duration" does not set metric.WithDescription`
}

func attributes(ctx context.Context, counter metric.Int64Counter, p id.Principal) {
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "approved")))
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("owner", string(p))))                          // want `principal-sink: a principal must not be a metric attribute`
	counter.Add(ctx, 1, metric.WithAttributes(p.SpanAttribute("approval.principal")))                         // want `principal-sink: a principal must not be a metric attribute`
	counter.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(attribute.Key("owner").String(p.String())))) // want `principal-sink: a principal must not be a metric attribute`
}
