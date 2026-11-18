package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	repoRoot      = "../.."
	referencePath = "docs/reference/telemetry.md"
)

func TestReferenceIsCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the module")
	}
	ref, err := Extract(repoRoot, patterns...)
	if err != nil {
		t.Fatal(err)
	}
	if gaps := sortedGaps(ref.Gaps); len(gaps) > 0 {
		var lines []string
		for _, g := range gaps {
			lines = append(lines, g.Where+": "+g.What)
		}
		t.Fatalf("telemetrydoc cannot follow these places (coding-standards/OPENTELEMETRY.md section 9):\n%s",
			strings.Join(lines, "\n"))
	}
	var got bytes.Buffer
	if err := Write(&got, ref); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(repoRoot, referencePath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("%s is out of date. Run `just telemetry-doc` and commit the result.", referencePath)
	}
}

func extractFixture(t *testing.T, dir string) *Reference {
	t.Helper()
	if testing.Short() {
		t.Skip("type-checks packages")
	}
	ref, err := Extract(repoRoot, "./tools/telemetrydoc/testdata/"+dir)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func attrKeys(attrs map[string]*Attr) []string { return sortedKeys(attrs) }

func TestExtractSpans(t *testing.T) {
	ref := extractFixture(t, "fixture")
	if len(ref.Gaps) > 0 {
		t.Fatalf("unexpected gaps: %v", ref.Gaps)
	}
	if got, want := sortedKeys(ref.Spans), []string{
		"fixture.fetch", "fixture.generic", "fixture.held", "fixture.labeled", "fixture.reopened", "fixture.special",
		"fixture.wrapped_op",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %v, want %v", got, want)
	}

	fetch := ref.Spans["fixture.fetch"]
	if fetch.Scope != "fixture" || fetch.Kind != "Client" || !fetch.ErrorStatus {
		t.Errorf("fixture.fetch scope=%q kind=%q error=%v", fetch.Scope, fetch.Kind, fetch.ErrorStatus)
	}
	if got, want := attrKeys(fetch.Attrs), []string{
		"db.system", "fixture.cache", "fixture.count", "fixture.outcome", "fixture.result",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("fixture.fetch attributes = %v, want %v", got, want)
	}
	if !fetch.Attrs["db.system"].Values["postgresql"] {
		t.Errorf("db.system values = %v, want postgresql", fetch.Attrs["db.system"].Values)
	}
	if !fetch.Attrs["fixture.count"].Types["int"] {
		t.Errorf("fixture.count types = %v, want int", fetch.Attrs["fixture.count"].Types)
	}
	// A local variable that is only assigned constants.
	if cache := fetch.Attrs["fixture.cache"]; !cache.Values["cold"] || !cache.Values["warm"] || len(cache.Types) > 0 {
		t.Errorf("fixture.cache = %+v, want the values cold and warm", cache)
	}
	// A key that a helper receives as a parameter.
	if r := fetch.Attrs["fixture.result"]; r.EnumType != "tools/telemetrydoc/testdata/fixture.Outcome" {
		t.Errorf("fixture.result enum = %q", r.EnumType)
	}
	// Set on the span in a context that passes through context.WithoutCancel.
	outcome := fetch.Attrs["fixture.outcome"]
	if outcome.EnumType != "tools/telemetrydoc/testdata/fixture.Outcome" {
		t.Errorf("fixture.outcome enum = %q", outcome.EnumType)
	}
	if want := []EnumValue{
		{"failure", "The operation failed."},
		{"success", "The operation worked."},
	}; !reflect.DeepEqual(outcome.Enum, want) {
		t.Errorf("fixture.outcome values = %v, want %v", outcome.Enum, want)
	}
	if got, want := sortedKeys(fetch.Files), []string{"tools/telemetrydoc/testdata/fixture/fixture.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("fixture.fetch files = %v, want %v", got, want)
	}

	// One caller passes a constant, but an interface call can pass anything.
	if label := ref.Spans["fixture.labeled"].Attrs["fixture.label"]; label == nil || len(label.Values) > 0 || !label.Types["string"] {
		t.Errorf("fixture.label = %+v, want any string", label)
	}

	wrapped := ref.Spans["fixture.wrapped_op"]
	if wrapped.Scope != "fixture" || wrapped.Kind != "" {
		t.Errorf("fixture.wrapped_op scope=%q kind=%q", wrapped.Scope, wrapped.Kind)
	}
	if got, want := attrKeys(wrapped.Attrs), []string{"fixture.mode", "fixture.op", "fixture.wrapped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture.wrapped_op attributes = %v, want %v", got, want)
	}
	// The wrapper receives the value as an argument.
	if op := wrapped.Attrs["fixture.op"]; !op.Values["WrappedOp"] || len(op.Types) > 0 {
		t.Errorf("fixture.op = %+v, want the value WrappedOp", op)
	}

	// The span is finished through a struct field.
	held := ref.Spans["fixture.held"]
	if !held.ErrorStatus {
		t.Errorf("fixture.held error status = false, want true")
	}
	if et := held.Attrs["error.type"]; et == nil || !et.Values["close_failed"] {
		t.Errorf("fixture.held error.type = %+v, want the value close_failed", et)
	}
	// The span is started straight into the same field.
	if reopened := ref.Spans["fixture.reopened"]; !reopened.ErrorStatus || reopened.Attrs["error.type"] == nil {
		t.Errorf("fixture.reopened = %+v, want error status and error.type", reopened)
	}

	if special := ref.Spans["fixture.special"]; special.RenamedFrom != "fixture.generic" || special.Scope != "fixture" {
		t.Errorf("fixture.special renamed from %q, scope %q", special.RenamedFrom, special.Scope)
	}
}

func TestExtractMetrics(t *testing.T) {
	ref := extractFixture(t, "fixture")
	if got, want := sortedKeys(ref.Metrics), []string{"fixture.queue_depth", "fixture.requests"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("metrics = %v, want %v", got, want)
	}
	req := ref.Metrics["fixture.requests"]
	if req.Instrument != "Int64Counter" || req.Unit != "{request}" || req.Desc != "Requests handled" || req.Scope != "fixture" {
		t.Errorf("fixture.requests = %+v", req)
	}
	if got, want := attrKeys(req.Attrs), []string{"outcome", "source"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture.requests attributes = %v, want %v", got, want)
	}
	// An attribute held in a struct field.
	if src := req.Attrs["source"]; !src.Values["fixture"] {
		t.Errorf("source = %+v, want the value fixture", src)
	}
	// An attribute that a closure returns.
	if got, want := attrKeys(ref.Metrics["fixture.queue_depth"].Attrs), []string{"queue"}; !reflect.DeepEqual(got, want) {
		t.Errorf("fixture.queue_depth attributes = %v, want %v", got, want)
	}
}

func TestExtractReportsGaps(t *testing.T) {
	ref := extractFixture(t, "gaps")
	var got []string
	for _, g := range sortedGaps(ref.Gaps) {
		got = append(got, g.What)
	}
	want := []string{
		"span name is not a constant",
		"attribute key is not a constant",
		"builds an attribute that the generator cannot trace to a span or a metric",
		"changes a span that the generator cannot trace to its Start call",
		"builds an attribute that the generator cannot trace to a span or a metric",
		"span gaps.twice is started in two places with a different scope or kind",
		"span kind is not a trace.SpanKind constant",
		"the reference does not list span events, so it cannot list these event attributes",
		"span gaps.passed is passed to a function that the generator does not follow",
		"attributes reach the parameter attrs from a call that the generator does not see",
		"span name is not a constant",
		"span renamed to gaps.static, which is the name of another span",
		"tracer scope is not resolved",
		"metric gaps.dup is created in more than one place, also in tools/telemetrydoc/testdata/gaps/gaps.go",
		"records a measurement on an instrument that the generator cannot trace to where it is created",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gaps =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAnchor(t *testing.T) {
	for in, want := range map[string]string{
		"storage.get.agent":                     "storagegetagent",
		"internal/extproc/server.FailureDetail": "internalextprocserverfailuredetail",
		"extproc.token_exchange":                "extproctoken_exchange",
	} {
		if got := anchor(in); got != want {
			t.Errorf("anchor(%q) = %q, want %q", in, got, want)
		}
	}
}
