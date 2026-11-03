package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLogValue(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "/oauth2/authorize", "/oauth2/authorize"},
		{"empty", "", ""},
		{"backslash", `a\b`, `a\\b`},
		{"forged escape", `a\nb`, `a\\nb`},
		{"carriage return", "a\rb", `a\rb`},
		{"line feed", "a\nb", `a\nb`},
		{"vertical tab", "a\vb", `a\vb`},
		{"form feed", "a\fb", `a\fb`},
		{"next line", "a\u0085b", `a\u0085b`},
		{"line separator", "a\u2028b", `a\u2028b`},
		{"paragraph separator", "a\u2029b", `a\u2029b`},
		{"mixed", "GET\r\n/x\u2028y", `GET\r\n/x\u2028y`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SanitizeLogValue(tt.value))
		})
	}
}
