package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
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

func TestLoggedRequestLine(t *testing.T) {
	capture := security.TransportCapture{RequestMethod: http.MethodPost, RequestTarget: "/oauth2/a%2Fb"}
	for _, tt := range []struct {
		name       string
		context    func(*http.Request) *http.Request
		wantMethod string
		wantTarget string
	}{
		{"no security context", func(r *http.Request) *http.Request { return r }, http.MethodGet, "/oauth2/a/b"},
		{"security context", func(r *http.Request) *http.Request {
			return r.WithContext(security.WithSecurityContext(r.Context(), security.NewSecurityContext(capture, id.NewPrincipal(""), "")))
		}, http.MethodPost, "/oauth2/a%2Fb"},
		{"unfinalized capture holder", func(r *http.Request) *http.Request {
			return r.WithContext(security.WithCaptureHolder(r.Context(), security.NewCaptureHolder(capture)))
		}, http.MethodPost, "/oauth2/a%2Fb"},
		{"security context without request line", func(r *http.Request) *http.Request {
			return r.WithContext(security.WithSecurityContext(r.Context(), security.SecurityContext{ClientIP: "192.0.2.1"}))
		}, http.MethodGet, "/oauth2/a/b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.context(httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/a%2Fb", nil))

			method, target := LoggedRequestLine(req)

			assert.Equal(t, tt.wantMethod, method)
			assert.Equal(t, tt.wantTarget, target)
		})
	}
}
