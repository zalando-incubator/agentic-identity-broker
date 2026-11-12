package urivalidation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidPrivateUseRedirectURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"GitHub Desktop authority", "github-app://oauth/callback", true},
		{"RFC 8252 single-slash form", "com.example.app:/oauth2redirect/example-provider", true},
		{"authority without path", "myapp://callback", true},
		{"authority with valid port", "myapp://oauth:8080/callback", true},
		{"query parameter", "vscode://vscode.github-authentication/did-authenticate?windowId=1", true},
		{"query on GitHub callback", "github-app://oauth/callback?window=1", true},
		{"allowed URI punctuation", "myapp://oauth/a!$&'()+,;=:@b", true},
		{"empty", "", false},
		{"HTTPS", "https://agent.example.com/callback", false},
		{"loopback HTTP", "http://127.0.0.1/callback", false},
		{"javascript authority", "javascript://oauth/callback", false},
		{"javascript opaque", "javascript:alert(1)", false},
		{"vbscript", "vbscript://oauth/callback", false},
		{"data opaque", "data:text/html,callback", false},
		{"data authority", "data://text/callback", false},
		{"file empty authority", "file:///etc/passwd", false},
		{"file authority", "file://localhost/etc/passwd", false},
		{"blob", "blob://oauth/callback", false},
		{"filesystem", "filesystem://oauth/callback", false},
		{"about", "about://oauth/blank", false},
		{"view-source", "view-source://oauth/callback", false},
		{"ftp", "ftp://client.example.com/callback", false},
		{"ws", "ws://oauth/callback", false},
		{"wss", "wss://oauth/callback", false},
		{"Windows drive", "c:/Windows/System32", false},
		{"mixed-case scheme", "GitHub-App://oauth/callback", false},
		{"mixed-case host", "github-app://OAuth/callback", false},
		{"fragment", "github-app://oauth/callback#frag", false},
		{"empty fragment", "github-app://oauth/callback#", false},
		{"percent-encoded callback", "github-app://oauth/%63allback", false},
		{"percent-encoded dot segment", "github-app://oauth/%2e%2e/callback", false},
		{"parent segment", "github-app://oauth/../callback", false},
		{"current segment", "github-app://oauth/./callback", false},
		{"wildcard", "github-app://*/callback", false},
		{"backslash", "github-app://oauth\\callback", false},
		{"space", "github-app://oauth/call back", false},
		{"newline", "github-app://oauth/callback\n", false},
		{"non-ASCII", "github-app://oauth/cälback", false},
		{"opaque rootless path", "github-app:oauth/callback", false},
		{"empty authority", "github-app:///callback", false},
		{"scheme only", "github-app:", false},
		{"userinfo", "github-app://user@oauth/callback", false},
		{"empty port", "github-app://oauth:/callback", false},
		{"zero port", "github-app://oauth:0/callback", false},
		{"out-of-range port", "github-app://oauth:65536/callback", false},
		{"missing hostname", "github-app://:8080/callback", false},
		{"empty query delimiter", "github-app://oauth/callback?", false},
		{"relative path", "/callback", false},
		{"network path", "//oauth/callback", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidPrivateUseRedirectURI(tt.uri), "%q", tt.uri)
		})
	}
}
