// Command telemetrydoc writes docs/reference/telemetry.md: the spans,
// metrics and attributes that the module creates through the OpenTelemetry
// API. It reads the code with go/packages and never runs it.
//
// It writes the page to standard output. `just telemetry-doc` runs it from
// the repository root and replaces the page.
//
// It exits with status 1 when it cannot follow a span name, a metric name,
// an attribute, or a call that changes a span or records a measurement, and
// lists each place on standard error.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
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
		return fmt.Errorf("cannot follow %d places (coding-standards/OPENTELEMETRY.md section 9)", len(gaps))
	}
	var buf bytes.Buffer
	if err := Write(&buf, ref); err != nil {
		return err
	}
	_, err = os.Stdout.Write(buf.Bytes())
	return err
}
