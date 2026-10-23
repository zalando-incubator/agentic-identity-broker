package urivalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidRedirectURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid https", "https://client.example.com/callback", true},
		{"valid https with port", "https://client.example.com:8080/cb", true},
		{"valid https with query", "https://client.example.com/cb?foo=bar", true},
		{"http localhost allowed", "http://localhost/callback", true},
		{"http 127.0.0.1 allowed", "http://127.0.0.1/callback", true},
		{"http ::1 allowed", "http://[::1]/callback", true},
		{"empty port rejected", "http://localhost:/callback", false},
		{"ipv6 empty port rejected", "http://[::1]:/callback", false},
		{"port zero rejected", "http://localhost:0/callback", false},
		{"port above range rejected", "https://client.example.com:65536/cb", false},
		{"http non-local rejected", "http://client.example.com/callback", false},
		{"http non-local with port rejected", "http://client.example.com:8080/cb", false},
		{"private-use scheme is not a web redirect URI", "github-app://oauth/callback", false},
		{"empty string", "", false},
		{"no scheme", "client.example.com/callback", false},
		{"non-http scheme", "ftp://client.example.com/callback", false},
		{"no host", "https:///callback", false},
		{"fragment present", "https://client.example.com/callback#section", false},
		{"empty fragment", "https://client.example.com/callback#", false},
		{"space in path", "https://client.example.com/call back", false},
		{"relative path", "/callback", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidRedirectURI(tt.input))
		})
	}
}

func TestMatchesRedirectURI(t *testing.T) {
	cases := []struct {
		name       string
		registered string
		incoming   string
		want       bool
	}{
		// Loopback — port ignored
		{"loopback: portless reg, port in request", "http://localhost/cb", "http://localhost:52341/cb", true},
		{"loopback: explicit port reg, different port", "http://localhost:3000/cb", "http://localhost:9999/cb", true},
		{"loopback: explicit port reg, portless request", "http://localhost:3000/cb", "http://localhost/cb", true},
		{"loopback: both portless", "http://localhost/cb", "http://localhost/cb", true},
		{"loopback: 127.0.0.1 different ports", "http://127.0.0.1:8080/cb", "http://127.0.0.1:51234/cb", true},
		{"loopback: [::1] different ports", "http://[::1]:8080/cb", "http://[::1]:51234/cb", true},
		// Loopback — other components must still match
		{"loopback: path mismatch", "http://localhost/cb", "http://localhost:3000/other", false},
		{"loopback: scheme mismatch", "http://localhost/cb", "https://localhost/cb", false},
		{"loopback: host mismatch (localhost vs 127.0.0.1)", "http://localhost/cb", "http://127.0.0.1/cb", false},
		// Non-loopback — exact match required
		{"non-loopback: exact match", "https://app.example.com/cb", "https://app.example.com/cb", true},
		{"non-loopback: port differs", "https://app.example.com/cb", "https://app.example.com:9999/cb", false},
		{"non-loopback: explicit port differs", "https://app.example.com:8443/cb", "https://app.example.com:9000/cb", false},
		{"non-loopback: portless reg, port in request", "https://app.example.com/cb", "https://app.example.com:443/cb", false},
		{"private-use authority exact", "github-app://oauth/callback", "github-app://oauth/callback", true},
		{"private-use scheme case differs", "github-app://oauth/callback", "GITHUB-APP://oauth/callback", false},
		{"private-use host case differs", "github-app://oauth/callback", "github-app://OAUTH/callback", false},
		{"private-use path differs", "github-app://oauth/callback", "github-app://oauth/callback/", false},
		{"private-use query differs", "github-app://oauth/callback", "github-app://oauth/callback?x=1", false},
		{"private-use single-slash exact", "com.example.app:/oauth2redirect/example-provider", "com.example.app:/oauth2redirect/example-provider", true},
		// Defense-in-depth — reject malformed values even if legacy data bypassed write validation
		{"invalid: registered empty port rejected", "http://localhost:/cb", "http://localhost:52341/cb", false},
		{"invalid: incoming empty port rejected", "http://localhost/cb", "http://localhost:/cb", false},
		{"invalid: registered ipv6 empty port rejected", "http://[::1]:/cb", "http://[::1]:52341/cb", false},
		{"invalid: registered port zero rejected", "http://localhost:0/cb", "http://localhost:52341/cb", false},
		{"invalid: registered out-of-range port rejected", "http://localhost:65536/cb", "http://localhost:52341/cb", false},
		{"invalid: raw fragment rejected", "https://app.example.com/cb#frag", "https://app.example.com/cb#frag", false},
		{"invalid: raw whitespace rejected", "https://app.example.com/call back", "https://app.example.com/call back", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesRedirectURI(tc.registered, tc.incoming)
			if got != tc.want {
				t.Errorf("MatchesRedirectURI(%q, %q) = %v, want %v", tc.registered, tc.incoming, got, tc.want)
			}
		})
	}
}

func TestIsRegisteredRedirectURI(t *testing.T) {
	callback := "github-app://oauth/callback"
	tests := []struct {
		name     string
		client   []string
		agent    []string
		incoming string
		want     bool
	}{
		{"web URI comes from client list", []string{"https://agent.example.com/callback"}, nil, "https://agent.example.com/callback", true},
		{"loopback port may vary", []string{"http://127.0.0.1/callback"}, nil, "http://127.0.0.1:53124/callback", true},
		{"private-use in both lists", []string{callback}, []string{callback}, callback, true},
		{"private-use only in client list", []string{callback}, nil, callback, false},
		{"private-use only in Agent list", nil, []string{callback}, callback, false},
		{"scheme alone cannot authorize", []string{callback}, []string{callback}, "github-app://evil/steal", false},
		{"host alone cannot authorize", []string{callback}, []string{callback}, "github-app://oauth", false},
		{"prefix cannot authorize", []string{callback}, []string{callback}, "github-app://oauth/callback/extra", false},
		{"wildcard cannot authorize", []string{callback}, []string{callback}, "github-app://*/callback", false},
		{"case variant cannot authorize", []string{callback}, []string{callback}, "GITHUB-APP://oauth/callback", false},
		{"port cannot vary", []string{"myapp://oauth:8080/callback"}, []string{"myapp://oauth:8080/callback"}, "myapp://oauth:9090/callback", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsRegisteredRedirectURI(tt.client, tt.agent, tt.incoming))
		})
	}
}
