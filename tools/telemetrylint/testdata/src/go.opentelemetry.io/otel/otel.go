// Package otel is a minimal stub of go.opentelemetry.io/otel for analyzer tests.
package otel

import (
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func Tracer(name string, opts ...trace.TracerOption) trace.Tracer { return nil }

func Meter(name string, opts ...metric.MeterOption) metric.Meter { return nil }

func GetTracerProvider() trace.TracerProvider { return nil }

func GetMeterProvider() metric.MeterProvider { return nil }
