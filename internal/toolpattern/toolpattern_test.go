package toolpattern

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestPrecedenceVectors(t *testing.T) {
	for _, vector := range PrecedenceVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			candidates := make([]Candidate, len(vector.Candidates))
			for i, candidate := range vector.Candidates {
				decidedAt, err := time.Parse(time.RFC3339, candidate.DecidedAt)
				if err != nil {
					t.Fatal(err)
				}
				candidates[i] = Candidate{ID: candidate.ID, ToolPattern: candidate.ToolPattern, ParamsPattern: candidate.ParamsPattern, DecidedAt: decidedAt}
			}
			index := SelectBest(candidates, vector.ToolName, vector.Arguments)
			if vector.WinnerID == "" {
				if index != -1 {
					t.Fatalf("SelectBest() = %d, want -1", index)
				}
				return
			}
			if index < 0 || candidates[index].ID != vector.WinnerID {
				t.Fatalf("SelectBest() = %d (%v), want %q", index, candidates, vector.WinnerID)
			}
		})
	}
}

func TestEscapeLiteralRoundTrip(t *testing.T) {
	value := `a*b\\c`
	if !Match(EscapeLiteral(value), value) || Match(EscapeLiteral(value), value+"x") {
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
