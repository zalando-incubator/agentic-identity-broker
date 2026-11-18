package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

const header = `---
title: "Telemetry"
description: Spans, metrics and attributes that the broker and ExtProc emit through OpenTelemetry. Generated from the source code.
---

# Telemetry

This page lists every span, metric and attribute that the broker and ExtProc create through
the OpenTelemetry API. ` + "`tools/telemetrydoc`" + ` generates it from the source code, and a test
fails when the page is out of date. Do not edit it by hand. After you change a span or a
metric, run ` + "`just telemetry-doc`" + `.

The page does not list:

- the HTTP server spans and metrics that ` + "`otelchi`" + ` creates for each inbound request;
- metrics that the OpenTelemetry SDK, the Go runtime or a library creates;
- span events and resource attributes;
- log records. ` + "`coding-standards/LOGGING.md`" + ` covers them.

A value list can show more values than one span or metric sets. An attribute with an
enumerated Go type lists every constant of the type. A variable or a parameter lists every
constant that the code can assign or pass to it. An attribute that the code sets on the span
in a context is listed under each span of this module that the context can hold. Rules for
adding telemetry are in ` + "`coding-standards/OPENTELEMETRY.md`" + `.
`

// Write renders ref as the telemetry reference page.
func Write(w io.Writer, ref *Reference) error {
	b := &strings.Builder{}
	b.WriteString(header)
	writeSpans(b, ref)
	writeMetrics(b, ref)
	writeSpanAttributes(b, ref)
	writeMetricAttributes(b, ref)
	writeEnums(b, ref)
	_, err := io.WriteString(w, strings.TrimRight(b.String(), "\n")+"\n")
	return err
}

func writeSpans(b *strings.Builder, ref *Reference) {
	b.WriteString("\n## Spans\n\n")
	b.WriteString("Error status: whether the span can end with status `Error`.\n\n")
	b.WriteString("| Span | Kind | Scope | Error status | Started in |\n|---|---|---|---|---|\n")
	for _, n := range sortedKeys(ref.Spans) {
		s := ref.Spans[n]
		started := codeList(sortedKeys(s.Files))
		if s.RenamedFrom != "" {
			started = fmt.Sprintf("Renamed from `%s`", s.RenamedFrom)
		}
		fmt.Fprintf(b, "| [`%s`](#%s) | %s | `%s` | %s | %s |\n",
			n, anchor(n), kindName(s.Kind), s.Scope, yesNo(s.ErrorStatus), started)
	}
}

func writeMetrics(b *strings.Builder, ref *Reference) {
	b.WriteString("\n## Metrics\n\n")
	b.WriteString("| Metric | Instrument | Unit | Scope | Description |\n|---|---|---|---|---|\n")
	for _, n := range sortedKeys(ref.Metrics) {
		m := ref.Metrics[n]
		fmt.Fprintf(b, "| [`%s`](#%s) | %s | %s | `%s` | %s |\n",
			n, anchor(n), m.Instrument, code(m.Unit), m.Scope, text(m.Desc))
	}
}

func writeSpanAttributes(b *strings.Builder, ref *Reference) {
	b.WriteString("\n## Span attributes\n")
	for _, n := range sortedKeys(ref.Spans) {
		s := ref.Spans[n]
		fmt.Fprintf(b, "\n### %s\n\n", n)
		if s.RenamedFrom != "" {
			fmt.Fprintf(b, "The `%s` span, renamed while it runs. It has the same attributes.\n\n", s.RenamedFrom)
		}
		writeAttrs(b, s.Attrs)
	}
}

func writeMetricAttributes(b *strings.Builder, ref *Reference) {
	b.WriteString("\n## Metric attributes\n")
	for _, n := range sortedKeys(ref.Metrics) {
		m := ref.Metrics[n]
		fmt.Fprintf(b, "\n### %s\n\n", n)
		fmt.Fprintf(b, "Created in `%s`.\n\n", m.File)
		writeAttrs(b, m.Attrs)
	}
}

func writeAttrs(b *strings.Builder, attrs map[string]*Attr) {
	if len(attrs) == 0 {
		b.WriteString("No attributes.\n")
		return
	}
	b.WriteString("| Attribute | Values |\n|---|---|\n")
	for _, k := range sortedKeys(attrs) {
		a := attrs[k]
		var vals []string
		for _, v := range sortedKeys(a.Values) {
			vals = append(vals, code(v))
		}
		if a.EnumType != "" {
			vals = append(vals, fmt.Sprintf("[`%s`](#%s)", a.EnumType, anchor(a.EnumType)))
		}
		for _, t := range sortedKeys(a.Types) {
			vals = append(vals, "any "+code(t))
		}
		fmt.Fprintf(b, "| `%s` | %s |\n", a.Key, strings.Join(vals, ", "))
	}
}

func writeEnums(b *strings.Builder, ref *Reference) {
	enums := map[string][]EnumValue{}
	collect := func(attrs map[string]*Attr) {
		for _, a := range attrs {
			if a.EnumType != "" {
				enums[a.EnumType] = a.Enum
			}
		}
	}
	for _, s := range ref.Spans {
		collect(s.Attrs)
	}
	for _, m := range ref.Metrics {
		collect(m.Attrs)
	}
	b.WriteString("\n## Attribute values\n")
	for _, n := range sortedKeys(enums) {
		fmt.Fprintf(b, "\n### %s\n\n", n)
		documented := false
		for _, e := range enums[n] {
			documented = documented || e.Doc != ""
		}
		if !documented {
			vals := make([]string, 0, len(enums[n]))
			for _, e := range enums[n] {
				vals = append(vals, code(e.Value))
			}
			b.WriteString(strings.Join(vals, ", ") + "\n")
			continue
		}
		b.WriteString("| Value | Meaning |\n|---|---|\n")
		for _, e := range enums[n] {
			fmt.Fprintf(b, "| %s | %s |\n", code(e.Value), text(e.Doc))
		}
	}
}

func kindName(k string) string {
	if k == "" {
		return "Internal"
	}
	return k
}

func yesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func code(s string) string {
	if s == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(s, "|", `\|`) + "`"
}

func codeList(list []string) string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = code(s)
	}
	return strings.Join(out, ", ")
}

// text escapes characters that break a table cell or MDX.
func text(s string) string {
	return strings.NewReplacer("|", `\|`, "{", `\{`, "}", `\}`, "<", "&lt;", ">", "&gt;").Replace(s)
}

// anchor returns the heading ID that GitHub and Docusaurus generate.
func anchor(s string) string {
	var out []rune
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			out = append(out, r)
		case r == ' ':
			out = append(out, '-')
		}
	}
	return string(out)
}

// sortedGaps returns the gaps without duplicates, in a stable order.
func sortedGaps(gaps []Gap) []Gap {
	seen := map[Gap]bool{}
	var out []Gap
	for _, g := range gaps {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Where != out[j].Where {
			return out[i].Where < out[j].Where
		}
		return out[i].What < out[j].What
	})
	return out
}
