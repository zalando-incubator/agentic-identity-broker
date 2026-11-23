// Package id is a minimal stub of the broker's typed principal for analyzer tests.
package id

import "go.opentelemetry.io/otel/attribute"

type Principal string

func (p Principal) String() string { return string(p) }

func (p Principal) SpanAttribute(key string) attribute.KeyValue {
	return attribute.String(key, string(p))
}
