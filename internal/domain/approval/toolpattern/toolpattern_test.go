package toolpattern

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMatchesVectors(t *testing.T) {
	for _, vector := range MatchVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			if got := Matches(vector.ToolPattern, vector.ParamsPattern, vector.ToolName, vector.Arguments); got != vector.Match {
				t.Fatalf("Matches() = %v, want %v", got, vector.Match)
			}
		})
	}
}

func TestCanonicalVectors(t *testing.T) {
	for _, vector := range CanonicalVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			if got := Canonical(vector.Value); got != vector.Canonical {
				t.Fatalf("Canonical() = %q, want %q", got, vector.Canonical)
			}
		})
	}
}

func TestCanonicalPreservesExactJSONNumbersForApprovalMatching(t *testing.T) {
	approved := map[string]any{"count": json.Number("9007199254740993"), "nested": []any{json.Number("9007199254740993")}}
	pattern := ExactParams(approved)
	if pattern["count"] != "9007199254740993" || pattern["nested"] != `[9007199254740993]` {
		t.Fatalf("approval pattern rounded argument numbers: %#v", pattern)
	}
	if !Matches("deploy", pattern, "deploy", approved) {
		t.Fatal("exact numeric approval must match its original invocation")
	}
	for _, value := range []json.Number{"9007199254740992", "9007199254740994"} {
		if Matches("deploy", pattern, "deploy", map[string]any{"count": value, "nested": approved["nested"]}) {
			t.Fatalf("approval for count=9007199254740993 matched %s", value)
		}
	}
}

func TestNumericApprovalIsNotationIndependent(t *testing.T) {
	for _, tc := range []struct {
		approved string
		retry    string
	}{
		{"1.0", "1e0"},
		{"0.1e6", "100000.00"},
		{"9.007199254740993e15", "9007199254740993"},
		{"-0.00e99", "0"},
		{"1e1000000000", "10e999999999"},
		{"1e-1000000000", "10e-1000000001"},
		{"1e92233720368547758080", "10e92233720368547758079"},
		{"1E+0003", "1000.000"},
		{"1e1024", "1" + strings.Repeat("0", 1024)},
		{"-1e1023", "-1" + strings.Repeat("0", 1023)},
	} {
		t.Run(tc.approved, func(t *testing.T) {
			args := func(number string) map[string]any {
				return map[string]any{"amount": json.Number(number), "nested": map[string]any{"values": []any{json.Number(number)}}}
			}
			if !Matches("pay", ExactParams(args(tc.approved)), "pay", args(tc.retry)) {
				t.Fatal("numerically identical arguments must match, including nested values")
			}
		})
	}
}

func TestNumericApprovalScopeRejectsExponentBypass(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		value   func(json.Number) any
	}{
		{"0.*", func(n json.Number) any { return n }},
		{`{"amount":0.*}`, func(n json.Number) any { return map[string]any{"amount": n} }},
		{"[0.*]", func(n json.Number) any { return []any{n} }},
	} {
		pattern := map[string]string{"value": tc.pattern}
		if !Matches("pay", pattern, "pay", map[string]any{"value": tc.value("0.1")}) {
			t.Fatal("scope must cover the approved fractional amount")
		}
		if Matches("pay", pattern, "pay", map[string]any{"value": tc.value("0.1e6")}) {
			t.Fatal("fractional approval must not cover 100000 via exponent notation")
		}
	}
}

func TestEscapeLiteralRoundTrip(t *testing.T) {
	value := `a*b\\c`
	if !match(EscapeLiteral(value), value) || match(EscapeLiteral(value), value+"x") {
		t.Fatal("escaped literal must match only its original value")
	}
}

func TestValidationRejectsInvalidPatterns(t *testing.T) {
	for _, pattern := range []string{"", "bad?", "trailing\\"} {
		if !errors.Is(ValidateToolPattern(pattern), ErrInvalidToolPattern) {
			t.Fatalf("ValidateToolPattern(%q) must reject", pattern)
		}
	}
	tooMany := make(map[string]string, 65)
	for i := range 65 {
		tooMany[string(rune(i))] = "*"
	}
	for _, pattern := range []map[string]string{{"": "*"}, {"key": strings.Repeat("x", 1025)}, {"key": "trailing\\"}, tooMany} {
		if !errors.Is(ValidateParamsPattern(pattern), ErrInvalidParamsPattern) {
			t.Fatalf("ValidateParamsPattern(%v) must reject", pattern)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format("create_pull_request", map[string]string{"title": "Fix", "repo": "acme/*"}); got != "create_pull_request(repo=acme/*,title=Fix)" {
		t.Fatalf("Format() = %q", got)
	}
}
