// Package codes is a minimal stub of go.opentelemetry.io/otel/codes for analyzer tests.
package codes

type Code uint32

const (
	Unset Code = 0
	Error Code = 1
	Ok    Code = 2
)
