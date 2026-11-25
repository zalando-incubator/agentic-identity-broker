// Package fixture holds deliberate non-API OpenTelemetry imports in a domain package.
package fixture

import (
	_ "go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/metric"
	_ "go.opentelemetry.io/otel/semconv/v1.26.0" // want depguard `Domain packages import only the OpenTelemetry API`

	_ "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry" // want depguard `Domain packages must not import the telemetry adapter`
)
