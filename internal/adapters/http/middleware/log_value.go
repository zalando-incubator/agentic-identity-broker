package middleware

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

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

// SanitizeLogPath bounds the escaped path to 2048 bytes, including a truncation marker.
func SanitizeLogPath(path string) string {
	const maxBytes = 2048
	const marker = "...[truncated]"

	// Bound the input before escaping so allocation does not scale with the full path.
	prefix := logPathPrefix(path, maxBytes)
	escaped := SanitizeLogValue(prefix)
	if len(prefix) == len(path) && len(escaped) <= maxBytes {
		return escaped
	}
	return logPathPrefix(escaped, maxBytes-len(marker)) + marker
}

func logPathPrefix(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

// LoggedRequestLine returns the request method and escaped target captured by
// SecurityContextMiddleware, including a still-open deferred capture. A field the
// capture does not carry falls back to the request's own value.
func LoggedRequestLine(r *http.Request) (method, target string) {
	method, target = r.Method, r.URL.Path
	var capture security.TransportCapture
	if sc, ok := security.FromContext(r.Context()); ok {
		capture = security.TransportCapture{RequestMethod: sc.RequestMethod, RequestTarget: sc.RequestTarget}
	} else if holder, ok := security.CaptureHolderFromContext(r.Context()); ok {
		capture = holder.Capture()
	}
	if capture.RequestMethod != "" {
		method = capture.RequestMethod
	}
	if capture.RequestTarget != "" {
		target = capture.RequestTarget
	}
	return method, target
}
