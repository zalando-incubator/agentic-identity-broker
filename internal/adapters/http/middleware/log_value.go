package middleware

import "strings"

var logValueEscaper = strings.NewReplacer(
	"\\", `\\`,
	"\r", `\r`,
	"\n", `\n`,
	"\v", `\v`,
	"\f", `\f`,
	"\u0085", `\u0085`,
	"\u2028", `\u2028`,
	"\u2029", `\u2029`,
)

// SanitizeLogValue escapes backslashes and line breaks, including the Unicode line and
// paragraph separators, so an untrusted value cannot forge or split a log line.
func SanitizeLogValue(value string) string {
	return logValueEscaper.Replace(value)
}
