// Command telemetrydoc writes docs/reference/telemetry.md: the spans,
// metrics and attributes that the module creates through the OpenTelemetry
// API. It reads the code with go/packages and never runs it.
//
// It writes the page to standard output. `just telemetry-doc` runs it from
// the repository root and replaces the page.
//
// It exits with status 1 when it cannot follow a span name, a metric name,
// an attribute, or a call that changes a span, passes a span or records a
// measurement, and lists each place on standard error. It also exits with
// status 1 when a span or an attribute key has no description in
// docs/reference/telemetry-descriptions.yaml, or the file describes one that
// the code does not have.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// patterns are the packages that the reference covers.
var patterns = []string{"./internal/...", "./cmd/..."}

func main() {
	dir := flag.String("dir", ".", "repository root")
	flag.Parse()
	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "telemetrydoc:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	ref, err := Extract(dir, patterns...)
	if err != nil {
		return err
	}
	if gaps := sortedGaps(ref.Gaps); len(gaps) > 0 {
		for _, g := range gaps {
			fmt.Fprintf(os.Stderr, "%s: %s\n", g.Where, g.What)
		}
		return fmt.Errorf("cannot follow %d places (coding-standards/OPENTELEMETRY.md section 10)", len(gaps))
	}
	d, err := LoadDescriptions(filepath.Join(dir, descriptionsPath))
	if err != nil {
		return err
	}
	if problems := checkDescriptions(ref, d); len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "%s: %s\n", descriptionsPath, p)
		}
		return fmt.Errorf("%d problems with the descriptions (coding-standards/OPENTELEMETRY.md section 10)", len(problems))
	}
	var buf bytes.Buffer
	if err := Write(&buf, ref, d); err != nil {
		return err
	}
	_, err = os.Stdout.Write(buf.Bytes())
	return err
}
