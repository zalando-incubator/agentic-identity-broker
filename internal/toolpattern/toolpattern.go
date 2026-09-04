// Package toolpattern provides the single shared implementation of approval-pattern
// grammar, canonicalization, matching, and precedence for the broker and ExtProc.
package toolpattern

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToolPattern   = errors.New("invalid tool pattern")
	ErrInvalidParamsPattern = errors.New("invalid params pattern")
)

// Match reports whether glob matches s. '*' matches any run of characters, possibly empty.
// '\' escapes the next character. Every other character is a literal.
func Match(glob, s string) bool {
	globIndex, stringIndex := 0, 0
	star, retryIndex := -1, 0

	for stringIndex < len(s) {
		if globIndex < len(glob) {
			switch glob[globIndex] {
			case '*':
				star = globIndex
				globIndex++
				retryIndex = stringIndex
				continue
			case '\\':
				if globIndex+1 < len(glob) && glob[globIndex+1] == s[stringIndex] {
					globIndex += 2
					stringIndex++
					continue
				}
			default:
				if glob[globIndex] == s[stringIndex] {
					globIndex++
					stringIndex++
					continue
				}
			}
		}
		if star < 0 {
			return false
		}
		globIndex = star + 1
		retryIndex++
		stringIndex = retryIndex
	}

	for globIndex < len(glob) && glob[globIndex] == '*' {
		globIndex++
	}
	return globIndex == len(glob)
}

// EscapeLiteral escapes '\' and '*' so that Match(EscapeLiteral(s), s) is true and no other
// string matches.
func EscapeLiteral(s string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*").Replace(s)
}

// Canonical renders a decoded JSON argument value as the string globs are matched against.
func Canonical(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 64)
	case json.Number:
		if number, err := value.Float64(); err == nil {
			return strconv.FormatFloat(number, 'f', -1, 64)
		}
		return string(value)
	case int:
		return strconv.FormatInt(int64(value), 10)
	case int8:
		return strconv.FormatInt(int64(value), 10)
	case int16:
		return strconv.FormatInt(int64(value), 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case int64:
		return strconv.FormatInt(value, 10)
	case uint:
		return strconv.FormatUint(uint64(value), 10)
	case uint8:
		return strconv.FormatUint(uint64(value), 10)
	case uint16:
		return strconv.FormatUint(uint64(value), 10)
	case uint32:
		return strconv.FormatUint(uint64(value), 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	case bool:
		return strconv.FormatBool(value)
	case nil:
		return "null"
	default:
		return canonicalJSON(v)
	}
}

// ExactParams returns the fully constrained params pattern for a concrete argument map.
// The result is never nil.
func ExactParams(args map[string]any) map[string]string {
	params := make(map[string]string, len(args))
	for key, value := range args {
		params[key] = EscapeLiteral(Canonical(value))
	}
	return params
}

// ValidateToolPattern reports whether p is a legal tool-name glob.
func ValidateToolPattern(p string) error {
	if p == "" || len(p) > 255 {
		return fmt.Errorf("%w: must contain 1 to 255 bytes", ErrInvalidToolPattern)
	}
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' {
			if i+1 == len(p) {
				return fmt.Errorf("%w: trailing escape", ErrInvalidToolPattern)
			}
			i++
			continue
		}
		if p[i] == '*' || (p[i] >= 'A' && p[i] <= 'Z') || (p[i] >= 'a' && p[i] <= 'z') || (p[i] >= '0' && p[i] <= '9') || strings.ContainsRune("_.:/-", rune(p[i])) {
			continue
		}
		return fmt.Errorf("%w: invalid character %q", ErrInvalidToolPattern, p[i])
	}
	return nil
}

// ValidateParamsPattern reports whether every entry of m is a legal constraint.
func ValidateParamsPattern(m map[string]string) error {
	if len(m) > 64 {
		return fmt.Errorf("%w: at most 64 entries", ErrInvalidParamsPattern)
	}
	for key, value := range m {
		if key == "" || len(key) > 255 {
			return fmt.Errorf("%w: invalid key %q", ErrInvalidParamsPattern, key)
		}
		if len(value) > 1024 {
			return fmt.Errorf("%w: value for %q exceeds 1024 bytes", ErrInvalidParamsPattern, key)
		}
		if strings.HasSuffix(value, "\\") && !hasEscapedTrailingBackslash(value) {
			return fmt.Errorf("%w: value for %q has trailing escape", ErrInvalidParamsPattern, key)
		}
	}
	return nil
}

func hasEscapedTrailingBackslash(s string) bool {
	backslashes := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 0
}

// Matches reports whether the concrete invocation satisfies the pattern.
func Matches(toolPattern string, paramsPattern map[string]string, toolName string, args map[string]any) bool {
	if !Match(toolPattern, toolName) {
		return false
	}
	for key, pattern := range paramsPattern {
		value, ok := args[key]
		if !ok || !Match(pattern, Canonical(value)) {
			return false
		}
	}
	return true
}

// Format renders the human-readable combined pattern, for example
// create_pull_request(repo=acme/*). Keys are sorted; the separator is "," with no space.
func Format(toolPattern string, paramsPattern map[string]string) string {
	if len(paramsPattern) == 0 {
		return toolPattern
	}
	keys := make([]string, 0, len(paramsPattern))
	for key := range paramsPattern {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+paramsPattern[key])
	}
	return toolPattern + "(" + strings.Join(parts, ",") + ")"
}

// Rank orders candidate patterns from most to least specific.
type Rank struct {
	ExactTool         bool
	ConstrainedParams int
	Wildcards         int
}

// Specificity computes the rank of a pattern.
func Specificity(toolPattern string, paramsPattern map[string]string) Rank {
	rank := Rank{ExactTool: countWildcards(toolPattern) == 0, Wildcards: countWildcards(toolPattern)}
	for _, pattern := range paramsPattern {
		if pattern != "*" {
			rank.ConstrainedParams++
		}
		rank.Wildcards += countWildcards(pattern)
	}
	return rank
}

func countWildcards(pattern string) int {
	count := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
			continue
		}
		if pattern[i] == '*' {
			count++
		}
	}
	return count
}

// Compare returns +1 when a is more specific than b, -1 when b is more specific, 0 when equal.
func Compare(a, b Rank) int {
	if a.ExactTool != b.ExactTool {
		if a.ExactTool {
			return 1
		}
		return -1
	}
	if a.ConstrainedParams != b.ConstrainedParams {
		if a.ConstrainedParams > b.ConstrainedParams {
			return 1
		}
		return -1
	}
	if a.Wildcards != b.Wildcards {
		if a.Wildcards < b.Wildcards {
			return 1
		}
		return -1
	}
	return 0
}

// Candidate is one stored approval considered during selection.
type Candidate struct {
	ID            string
	ToolPattern   string
	ParamsPattern map[string]string
	DecidedAt     time.Time
}

// SelectBest returns the index of the most specific matching candidate, or -1 when none match.
func SelectBest(candidates []Candidate, toolName string, args map[string]any) int {
	best := -1
	for i, candidate := range candidates {
		if !Matches(candidate.ToolPattern, candidate.ParamsPattern, toolName, args) {
			continue
		}
		if best == -1 {
			best = i
			continue
		}
		comparison := Compare(Specificity(candidate.ToolPattern, candidate.ParamsPattern), Specificity(candidates[best].ToolPattern, candidates[best].ParamsPattern))
		if comparison > 0 || (comparison == 0 && (candidate.DecidedAt.After(candidates[best].DecidedAt) || (candidate.DecidedAt.Equal(candidates[best].DecidedAt) && candidate.ID < candidates[best].ID))) {
			best = i
		}
	}
	return best
}

func canonicalJSON(v any) string {
	if v == nil {
		return "null"
	}
	switch value := v.(type) {
	case string:
		return quoteJSON(value)
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 64)
	case json.Number:
		if number, err := value.Float64(); err == nil {
			return strconv.FormatFloat(number, 'f', -1, 64)
		}
		return string(value)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return Canonical(value)
	}

	value := reflect.ValueOf(v)
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "null"
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return "null"
		}
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, quoteJSON(key.String())+":"+canonicalJSON(value.MapIndex(key).Interface()))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case reflect.Array, reflect.Slice:
		parts := make([]string, value.Len())
		for i := range parts {
			parts[i] = canonicalJSON(value.Index(i).Interface())
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return "null"
	}
}

func quoteJSON(s string) string {
	var builder strings.Builder
	builder.Grow(len(s) + 2)
	builder.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		case '\b':
			builder.WriteString(`\b`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\f':
			builder.WriteString(`\f`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if s[i] < 0x20 {
				builder.WriteString(`\u00`)
				builder.WriteByte("0123456789abcdef"[s[i]>>4])
				builder.WriteByte("0123456789abcdef"[s[i]&0x0f])
			} else {
				builder.WriteByte(s[i])
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}
