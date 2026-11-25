// Command livingdoc checks living-documentation pages against the evidence
// that acceptance suites record for their examples.
//
// A page cites an example either inline as `example:<id>` or as a fenced
// http block with example="<id>" that shows one request and its response.
// Every citation needs a passing scenario carrying that example, and every
// shown exchange must equal one that the scenario performed. Timestamps
// compare by shape and UUIDs by identity pattern, not by value.
//
// Usage:
//
//	livingdoc -evidence <dir> [-report <file>] [-status <file>] [-strict] <page.md>...
//
// The status file maps each cited example to its worst verdict. The
// documentation site reads it to render the example badges.
//
// Exit codes:
//
//	0  — every citation is backed by evidence
//	1  — a citation is unbound, failed, or stale (with -strict: or not run)
//	2  — usage / I/O error
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
)

func main() {
	evidenceDir := flag.String("evidence", "", "directory with recorded evidence")
	reportPath := flag.String("report", "", "also write the Markdown report to this file")
	statusPath := flag.String("status", "", "also write the verdict of each example to this JSON file")
	strict := flag.Bool("strict", false, "fail when a cited example did not run")
	flag.Parse()
	if *evidenceDir == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Usage: livingdoc -evidence <dir> [-report <file>] [-status <file>] [-strict] <page.md>...")
		os.Exit(2)
	}
	records, err := evidence.ReadDir(*evidenceDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var claims []Claim
	for _, path := range flag.Args() {
		content, err := os.ReadFile(path) // #nosec G304 -- operator-supplied page path
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		pageClaims, err := parsePage(path, string(content))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		claims = append(claims, pageClaims...)
	}
	results := check(claims, records)
	report := render(results, undocumented(claims, records))
	fmt.Print(report)
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, []byte(report), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	if *statusPath != "" {
		encoded, err := json.MarshalIndent(status(results), "", "  ")
		if err == nil {
			err = os.WriteFile(*statusPath, encoded, 0o600)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	if failing(results, *strict) {
		os.Exit(1)
	}
}

func render(results []Result, uncited []string) string {
	var out strings.Builder
	counts := map[string]int{}
	out.WriteString("# Living documentation check\n\n| Page | Line | Example | Shows | Verdict |\n|---|---|---|---|---|\n")
	for _, result := range results {
		counts[result.Verdict]++
		shows := "claim"
		if result.Claim.Exchange != nil {
			shows = fmt.Sprintf("`%s %s` → %d", result.Claim.Exchange.Method, result.Claim.Exchange.Target, result.Claim.Exchange.Status)
		}
		fmt.Fprintf(&out, "| %s | %d | `%s` | %s | %s |\n", result.Claim.Page, result.Claim.Line, result.Claim.Example, shows, result.Verdict)
	}
	fmt.Fprintf(&out, "\n%d passed, %d stale, %d failed, %d unbound, %d not run.\n",
		counts[verdictPassed], counts[verdictStale], counts[verdictFailed], counts[verdictUnbound], counts[verdictNotRun])
	for _, result := range results {
		if result.Verdict == verdictPassed || result.Detail == "" {
			continue
		}
		fmt.Fprintf(&out, "\n## %s:%d `%s` — %s\n\n", result.Claim.Page, result.Claim.Line, result.Claim.Example, result.Verdict)
		if result.Verdict == verdictStale && strings.Contains(result.Detail, "\n") {
			fmt.Fprintf(&out, "`-` is the page, `+` is what the scenario recorded.\n\n```diff\n%s```\n", result.Detail)
		} else {
			fmt.Fprintf(&out, "%s\n", result.Detail)
		}
	}
	if len(uncited) > 0 {
		out.WriteString("\n## Examples no page cites\n\n")
		for _, example := range uncited {
			fmt.Fprintf(&out, "- `%s`\n", example)
		}
	}
	return out.String()
}
