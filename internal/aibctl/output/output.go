// Package output renders command results as tables, JSON, or YAML. Tables
// show canonical IDs and fall back to UUIDs; JSON and YAML print the API
// representation verbatim.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"go.yaml.in/yaml/v3"
)

type Format string

const (
	FormatTable Format = "table"
	FormatWide  Format = "wide"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

type Printer struct {
	Out       io.Writer
	Format    Format
	ShowUUID  bool
	NoHeaders bool
}

// Wide reports whether the table should include the extra columns.
func (p *Printer) Wide() bool { return p.Format == FormatWide }

// IDColumns reports whether a UUID column should accompany the ID column.
func (p *Printer) IDColumns() bool { return p.ShowUUID || p.Wide() }

// Table is a rendered list; Rows must align with Header.
type Table struct {
	Header []string
	Rows   [][]string
}

// PrintList renders v as JSON/YAML, or the table produced by build.
func (p *Printer) PrintList(v any, build func() Table) error {
	if p.structured() {
		return p.printStructured(v)
	}
	return p.printTable(build())
}

// PrintObject renders v as JSON/YAML, or the key/value pairs produced by build.
func (p *Printer) PrintObject(v any, build func() [][2]string) error {
	if p.structured() {
		return p.printStructured(v)
	}
	tw := tabwriter.NewWriter(p.Out, 0, 8, 2, ' ', 0)
	for _, kv := range build() {
		_, _ = fmt.Fprintf(tw, "%s:\t%s\n", kv[0], kv[1])
	}
	return tw.Flush()
}

func (p *Printer) structured() bool {
	return p.Format == FormatJSON || p.Format == FormatYAML
}

func (p *Printer) printStructured(v any) error {
	if p.Format == FormatYAML {
		return printYAML(p.Out, v)
	}
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) printTable(t Table) error {
	tw := tabwriter.NewWriter(p.Out, 0, 8, 2, ' ', 0)
	if !p.NoHeaders {
		_, _ = fmt.Fprintln(tw, strings.Join(t.Header, "\t"))
	}
	for _, row := range t.Rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}

// printYAML goes through JSON so the keys match the API field names.
func printYAML(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return err
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(generic); err != nil {
		return err
	}
	return enc.Close()
}

// DisplayID prefers the canonical ID and falls back to the UUID.
func DisplayID(canonical *string, uuid string) string {
	if canonical != nil && *canonical != "" {
		return *canonical
	}
	return uuid
}

// Deref returns the string behind a pointer or "" for nil.
func Deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Join renders a list for a table cell.
func Join(items []string) string {
	return strings.Join(items, ",")
}

// Bool renders a boolean for a table cell.
func Bool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
