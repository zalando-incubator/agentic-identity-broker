// Package metric is a minimal stub of go.opentelemetry.io/otel/metric for analyzer tests.
package metric

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

type MeterOption interface{ applyMeter() }

type Int64CounterOption interface{ applyInt64Counter() }

type Float64HistogramOption interface{ applyFloat64Histogram() }

type InstrumentOption interface {
	Int64CounterOption
	Float64HistogramOption
}

type MeasurementOption interface {
	AddOption
}

type AddOption interface{ applyAdd() }

type option struct{}

func (option) applyInt64Counter()     {}
func (option) applyFloat64Histogram() {}
func (option) applyAdd()              {}

func WithDescription(desc string) InstrumentOption { return option{} }

func WithUnit(unit string) InstrumentOption { return option{} }

func WithAttributes(attributes ...attribute.KeyValue) MeasurementOption { return option{} }

func WithAttributeSet(attributes attribute.Set) MeasurementOption { return option{} }

type Int64Counter interface {
	Add(ctx context.Context, incr int64, options ...AddOption)
}

type Float64Histogram interface{}

type Meter interface {
	Int64Counter(name string, options ...Int64CounterOption) (Int64Counter, error)
	Float64Histogram(name string, options ...Float64HistogramOption) (Float64Histogram, error)
}

type MeterProvider interface {
	Meter(name string, opts ...MeterOption) Meter
}
