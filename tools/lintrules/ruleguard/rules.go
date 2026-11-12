//go:build ruleguard

// Package gorules holds the ruleguard rules that gocritic loads from .golangci.yml. The build
// constraint keeps them out of the broker build.
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// principalSpanString rejects a principal rendered as a plain string attribute value
// (OPENTELEMETRY.md section 4.3, rule 1).
func principalSpanString(m dsl.Matcher) {
	m.Import("go.opentelemetry.io/otel/attribute")
	m.Import("github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id")
	m.Match(`attribute.String($_, $p.String())`, `attribute.String($_, string($p))`).
		Where(m["p"].Type.Is("id.Principal") || m["p"].Type.Is("*id.Principal")).
		Report(`principal rendered as a string attribute; use id.Principal.SpanAttribute (coding-standards/OPENTELEMETRY.md section 4.3)`)
}

// principalLogString rejects a principal logged through slog.String (LOGGING.md section 5.1,
// rules 2 and 5). TestLoggedPrincipalsAreTyped also covers key-value arguments and slog.Attr
// literals.
func principalLogString(m dsl.Matcher) {
	m.Import("github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id")
	m.Match(`slog.String($k, $_)`).
		Where(m["k"].Text.Matches(`^"(principal|operator_principal|actor)"$`)).
		Report(`slog.String under a reserved principal key; log the id.Principal value (coding-standards/LOGGING.md section 5.1)`)
	m.Match(`slog.String($_, $p.String())`, `slog.String($_, string($p))`).
		Where(m["p"].Type.Is("id.Principal") || m["p"].Type.Is("*id.Principal")).
		Report(`principal logged as a string; log the id.Principal value (coding-standards/LOGGING.md section 5.1)`)
}

// discardedInstrumentError rejects an instrument constructor whose error is assigned to _
// (OPENTELEMETRY.md section 5.1).
func discardedInstrumentError(m dsl.Matcher) {
	m.Import("go.opentelemetry.io/otel/metric")
	m.Match(`$_, _ := $meter.$ctor($*_)`, `$_, _ = $meter.$ctor($*_)`, `var $_, _ = $meter.$ctor($*_)`).
		Where(m["meter"].Type.Implements("metric.Meter") &&
			m["ctor"].Text.Matches(`^(Int64|Float64)(Counter|UpDownCounter|Histogram|Gauge|ObservableCounter|ObservableUpDownCounter|ObservableGauge)$`)).
		Report(`instrument constructor error discarded; log it once at Warn and keep the returned instrument (coding-standards/OPENTELEMETRY.md section 5.1)`)
}
