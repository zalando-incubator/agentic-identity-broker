package scopes

import "go.opentelemetry.io/otel"

var dynamic = "storage"

func scopes() {
	otel.Tracer("storage")
	otel.Meter("approval")
	otel.GetMeterProvider().Meter("extproc")
	otel.Tracer("billing")                  // want `scope: instrumentation scope "billing" is not in the allowlist of OPENTELEMETRY.md section 2`
	otel.Meter(dynamic)                     // want `scope: the instrumentation scope must be a constant string`
	otel.GetTracerProvider().Tracer("Jwks") // want `scope: instrumentation scope "Jwks" is not in the allowlist`
}
