package main

import (
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"
)

//go:embed page.md.tmpl
var pageTemplate string

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"anchor":     anchor,
	"code":       code,
	"codes":      codes,
	"documented": documented,
	"text":       text,
	"values":     values,
}).Parse(pageTemplate))

// Write renders ref as the telemetry reference page.
func Write(w io.Writer, ref *Reference) error {
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
	var b strings.Builder
	err := page.Execute(&b, struct {
		Spans   map[string]*Span
		Metrics map[string]*Metric
		Enums   map[string][]EnumValue
	}{ref.Spans, ref.Metrics, enums})
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, strings.TrimRight(b.String(), "\n")+"\n")
	return err
}

// values lists what the code can put in the attribute a.
func values(a *Attr) string {
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
	return strings.Join(vals, ", ")
}

// documented reports whether any of the values has a doc comment.
func documented(values []EnumValue) bool {
	for _, e := range values {
		if e.Doc != "" {
			return true
		}
	}
	return false
}

func code(s string) string {
	if s == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(s, "|", `\|`) + "`"
}

func codes(set map[string]bool) string {
	list := sortedKeys(set)
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
