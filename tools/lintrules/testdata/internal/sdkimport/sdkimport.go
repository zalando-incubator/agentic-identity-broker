// Package sdkimport holds deliberate OpenTelemetry SDK imports outside the telemetry adapter.
package sdkimport

import (
	_ "go.opentelemetry.io/contrib/propagators/b3"                      // want depguard `OPENTELEMETRY\.md section 1`
	_ "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp" // want depguard `OPENTELEMETRY\.md section 1`
	_ "go.opentelemetry.io/otel/sdk/trace"                              // want depguard `OPENTELEMETRY\.md section 1`
	_ "go.opentelemetry.io/otel/trace"
)
