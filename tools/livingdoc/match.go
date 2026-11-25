package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
)

const (
	verdictPassed  = "passed"
	verdictFailed  = "failed"
	verdictStale   = "stale"
	verdictNotRun  = "not run"
	verdictUnbound = "unbound"
)

// Result is the verdict for one claim.
type Result struct {
	Claim   Claim
	Verdict string
	Detail  string
}

func check(claims []Claim, records []evidence.Record) []Result {
	byExample := map[string][]evidence.Record{}
	for _, record := range records {
		byExample[record.Example] = append(byExample[record.Example], record)
	}
	results := make([]Result, 0, len(claims))
	for _, claim := range claims {
		results = append(results, checkClaim(claim, byExample[claim.Example]))
	}
	return results
}

func checkClaim(claim Claim, records []evidence.Record) Result {
	if len(records) == 0 {
		return Result{Claim: claim, Verdict: verdictUnbound, Detail: "no acceptance scenario carries this example"}
	}
	var exchanges []evidence.Exchange
	passed := false
	for _, record := range records {
		switch record.State {
		case evidence.StatePassed:
			passed = true
			exchanges = append(exchanges, record.Exchanges...)
		case evidence.StateSkipped:
		default:
			return Result{Claim: claim, Verdict: verdictFailed, Detail: fmt.Sprintf("%s (%s)", record.Spec, record.Location)}
		}
	}
	if !passed {
		return Result{Claim: claim, Verdict: verdictNotRun, Detail: fmt.Sprintf("%s (%s)", records[0].Spec, records[0].Location)}
	}
	if claim.Exchange == nil {
		return Result{Claim: claim, Verdict: verdictPassed}
	}
	want := canonicalDoc(claim.Exchange)
	closest := ""
	for _, exchange := range exchanges {
		got := canonicalRecorded(exchange, claim.Exchange)
		wantText, gotText := align(want, got)
		if wantText == gotText {
			return Result{Claim: claim, Verdict: verdictPassed}
		}
		if closest == "" && firstLine(gotText) == firstLine(wantText) {
			closest = diff(wantText, gotText)
		}
	}
	if closest == "" {
		return Result{Claim: claim, Verdict: verdictStale,
			Detail: fmt.Sprintf("no recorded exchange for %s %s", claim.Exchange.Method, claim.Exchange.Target)}
	}
	return Result{Claim: claim, Verdict: verdictStale, Detail: closest}
}

func undocumented(claims []Claim, records []evidence.Record) []string {
	cited := map[string]bool{}
	for _, claim := range claims {
		cited[claim.Example] = true
	}
	seen := map[string]bool{}
	var missing []string
	for _, record := range records {
		if !cited[record.Example] && !seen[record.Example] {
			seen[record.Example] = true
			missing = append(missing, record.Example)
		}
	}
	sort.Strings(missing)
	return missing
}

func failing(results []Result, strict bool) bool {
	for _, result := range results {
		switch result.Verdict {
		case verdictPassed:
		case verdictNotRun:
			if strict {
				return true
			}
		default:
			return true
		}
	}
	return false
}

var severity = map[string]int{verdictPassed: 0, verdictNotRun: 1, verdictUnbound: 2, verdictStale: 3, verdictFailed: 4}

// status returns the worst verdict among the citations of each example.
func status(results []Result) map[string]string {
	worst := map[string]string{}
	for _, result := range results {
		current, seen := worst[result.Claim.Example]
		if !seen || severity[result.Verdict] > severity[current] {
			worst[result.Claim.Example] = result.Verdict
		}
	}
	return worst
}

const ellipsis = "…"

func canonicalDoc(doc *DocExchange) string {
	return canonical(doc.Method, doc.Target, doc.RequestHeader, canonicalBody(doc.RequestBody, ""),
		doc.Status, doc.ResponseHeader, canonicalBody(doc.ResponseBody, ""))
}

// canonicalRecorded renders a recorded exchange with only the headers and,
// where the page elides object members, only the members that the page shows.
func canonicalRecorded(recorded evidence.Exchange, doc *DocExchange) string {
	pick := func(shown []Header, actual http.Header) []Header {
		picked := make([]Header, 0, len(shown))
		for _, header := range shown {
			picked = append(picked, Header{Name: header.Name, Value: strings.Join(actual.Values(header.Name), ", ")})
		}
		return picked
	}
	return canonical(recorded.Method, recorded.Target, pick(doc.RequestHeader, recorded.RequestHeader),
		canonicalBody(recorded.RequestBody, doc.RequestBody), recorded.Status,
		pick(doc.ResponseHeader, recorded.ResponseHeader), canonicalBody(recorded.ResponseBody, doc.ResponseBody))
}

func canonical(method, target string, requestHeader []Header, requestBody string, status int, responseHeader []Header, responseBody string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s %s\n", method, target)
	writeHeaders(&out, requestHeader)
	fmt.Fprintf(&out, "\n%s\n\nHTTP %d\n", requestBody, status)
	writeHeaders(&out, responseHeader)
	fmt.Fprintf(&out, "\n%s\n", responseBody)
	return out.String()
}

// align applies the page's ellipses and then replaces volatile values on both
// sides. A page line with an ellipsis matches any recorded line that agrees
// with its literal text; such a recorded line is replaced by the page line.
// UUIDs on a line with an ellipsis compare by shape only.
func align(want, got string) (string, string) {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if strings.Contains(wantLines[i], ellipsis) && linePattern(wantLines[i]).MatchString(gotLines[i]) {
			wantLines[i] = volatile.ReplaceAllString(wantLines[i], "<volatile>")
			gotLines[i] = wantLines[i]
		}
	}
	return placeholders(strings.Join(wantLines, "\n")), placeholders(strings.Join(gotLines, "\n"))
}

func linePattern(line string) *regexp.Regexp {
	var pattern strings.Builder
	pattern.WriteString("^")
	for i, part := range strings.Split(line, ellipsis) {
		if i > 0 {
			pattern.WriteString(".+")
		}
		rest := part
		for {
			loc := volatile.FindStringIndex(rest)
			if loc == nil {
				break
			}
			pattern.WriteString(regexp.QuoteMeta(rest[:loc[0]]))
			pattern.WriteString("(?:" + volatile.String() + ")")
			rest = rest[loc[1]:]
		}
		pattern.WriteString(regexp.QuoteMeta(rest))
	}
	pattern.WriteString("$")
	return regexp.MustCompile(pattern.String())
}

func writeHeaders(out *strings.Builder, headers []Header) {
	for _, header := range headers {
		if strings.EqualFold(header.Name, "Host") {
			continue
		}
		fmt.Fprintf(out, "%s: %s\n", http.CanonicalHeaderKey(header.Name), header.Value)
	}
}

var formBody = regexp.MustCompile(`^[^\s=&{}\[\]"]+=[^\s&]*(?:[&\s]+[^\s=&]+=[^\s&]*)*$`)

// canonicalBody renders a body for comparison. JSON is re-encoded with sorted
// keys, and a form body becomes one sorted parameter per line. When shown is
// the page's version of a recorded JSON body, members that the page elides
// with an ellipsis member are removed.
func canonicalBody(body, shown string) string {
	body = strings.TrimSpace(body)
	var decoded any
	if json.Unmarshal([]byte(body), &decoded) != nil {
		if formBody.MatchString(body) {
			parameters := strings.FieldsFunc(body, func(r rune) bool { return r == '&' || r == '\n' || r == ' ' || r == '\t' || r == '\r' })
			sort.Strings(parameters)
			return strings.Join(parameters, "\n")
		}
		return body
	}
	var shownDecoded any
	if shown != "" && json.Unmarshal([]byte(shown), &shownDecoded) == nil {
		decoded = elide(shownDecoded, decoded)
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if encoder.Encode(decoded) != nil {
		return body
	}
	return strings.TrimSpace(out.String())
}

func elide(shown, recorded any) any {
	switch shownValue := shown.(type) {
	case map[string]any:
		recordedValue, ok := recorded.(map[string]any)
		if !ok {
			return recorded
		}
		marker, elided := shownValue[ellipsis]
		reduced := map[string]any{}
		for key, value := range recordedValue {
			shownMember, isShown := shownValue[key]
			if elided && !isShown {
				continue
			}
			reduced[key] = elide(shownMember, value)
		}
		if elided {
			reduced[ellipsis] = marker
		}
		return reduced
	case []any:
		recordedValue, ok := recorded.([]any)
		if !ok {
			return recorded
		}
		reduced := make([]any, len(recordedValue))
		for i, value := range recordedValue {
			if i < len(shownValue) {
				reduced[i] = elide(shownValue[i], value)
			} else {
				reduced[i] = value
			}
		}
		return reduced
	default:
		return recorded
	}
}

var volatile = regexp.MustCompile(
	`(?P<uuid>[0-9a-fA-F]{8}-(?:[0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12})` +
		`|(?P<time>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))`)

// placeholders replaces UUIDs and timestamps with placeholders. Equal UUIDs
// share a numbered placeholder, in order of first appearance, so equalities
// and differences between identities survive while the values do not.
// Timestamps carry no identity: two of them can coincide or not depending on
// when a scenario crosses a clock tick.
func placeholders(text string) string {
	seen := map[string]string{}
	return volatile.ReplaceAllStringFunc(text, func(value string) string {
		if strings.Contains(value, "T") {
			return "<time>"
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; !ok {
			seen[key] = fmt.Sprintf("<uuid-%d>", len(seen)+1)
		}
		return seen[key]
	})
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// diff returns a line diff: "-" lines are what the page shows, "+" lines what was recorded.
func diff(want, got string) string {
	a, b := strings.Split(strings.TrimRight(want, "\n"), "\n"), strings.Split(strings.TrimRight(got, "\n"), "\n")
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			fmt.Fprintf(&out, "  %s\n", a[i])
			i, j = i+1, j+1
		case j == len(b) || (i < len(a) && lcs[i+1][j] >= lcs[i][j+1]):
			fmt.Fprintf(&out, "- %s\n", a[i])
			i++
		default:
			fmt.Fprintf(&out, "+ %s\n", b[j])
			j++
		}
	}
	return out.String()
}
