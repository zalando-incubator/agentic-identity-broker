package middleware

import (
	"strconv"
	"strings"
)

// SanitizeLogValue escapes backslashes and line breaks, including the Unicode line and
// paragraph separators, so an untrusted value cannot forge or split a log line.
func SanitizeLogValue(value string) string {
	if !strings.ContainsAny(value, "\\\r\n\v\f\u0085  ") {
		return value
	}
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}
