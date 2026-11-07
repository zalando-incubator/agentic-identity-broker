package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestInsufficientScopeChallenge(t *testing.T) {
	const recoveryURI = "https://broker.example.com/sessions"
	tests := []struct {
		name        string
		description string
		errorURI    string
		want        string
	}{
		{
			name:        "all attributes",
			description: "User session is unavailable. Please re-authenticate.",
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:     "empty description",
			errorURI: recoveryURI,
			want:     `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "quote in description",
			description: `Session "expired"`,
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "backslash in description",
			description: `Session \expired`,
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "CRLF in description",
			description: "Session expired\r\nInjected: value",
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "non-ASCII description",
			description: "Sitzung abgelaufen – bitte neu anmelden",
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "space in URI",
			description: "Session expired",
			errorURI:    "https://broker.example.com/sess ions",
			want:        `Bearer error="insufficient_scope", error_description="Session expired"`,
		},
		{
			name:        "quote in URI",
			description: "Session expired",
			errorURI:    `https://broker.example.com/"sessions`,
			want:        `Bearer error="insufficient_scope", error_description="Session expired"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, insufficientScopeChallenge(tt.description, tt.errorURI))
		})
	}
}

// Challenge attributes from specs/055-incremental-service-authorization/contracts/extproc-challenge.md.
func TestServiceScopeChallengeAttributes(t *testing.T) {
	const (
		description = "User session is unavailable. Please re-authenticate."
		errorURI    = "https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub"
		scope       = "https://broker.example.com/sessions/github"
		metadata    = "https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp"
		base        = `Bearer error="insufficient_scope", error_description="` + description + `", error_uri="` + errorURI + `"`
	)
	tests := []struct {
		name     string
		scope    string
		metadata string
		want     string
	}{
		{"scope and metadata in fixed order", scope, metadata, base + `, scope="` + scope + `", resource_metadata="` + metadata + `"`},
		{"metadata absent", scope, "", base + `, scope="` + scope + `"`},
		{"scope absent", "", metadata, base + `, resource_metadata="` + metadata + `"`},
		{"two scope tokens", scope + " repo", "", base},
		{"relative scope", "/sessions/github", "", base},
		{"non-http scope", "urn:example:github", "", base},
		{"quote in scope", `https://broker.example.com/sessions/"github`, "", base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, serviceScopeChallenge(description, errorURI, tt.scope, tt.metadata))
		})
	}
}

func TestResourceMetadataURL(t *testing.T) {
	valid := "https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp"
	tests := []struct {
		name    string
		value   *structpb.Value
		want    string
		present bool
	}{
		{"absent", nil, "", false},
		{"valid", structpb.NewStringValue(valid), valid, true},
		{"at the 2048-byte limit", structpb.NewStringValue("https://a.example/" + strings.Repeat("a", 2048-len("https://a.example/"))), "https://a.example/" + strings.Repeat("a", 2048-len("https://a.example/")), true},
		{"beyond 2048 bytes", structpb.NewStringValue("https://a.example/" + strings.Repeat("a", 2049-len("https://a.example/"))), "", true},
		{"not a string", structpb.NewNumberValue(42), "", true},
		{"http scheme", structpb.NewStringValue("http://agentgateway.example.com/metadata"), "", true},
		{"relative", structpb.NewStringValue("/metadata"), "", true},
		{"empty host", structpb.NewStringValue("https:///metadata"), "", true},
		{"userinfo", structpb.NewStringValue("https://user@agentgateway.example.com/metadata"), "", true},
		{"fragment", structpb.NewStringValue("https://agentgateway.example.com/metadata#x"), "", true},
		{"double quote", structpb.NewStringValue(`https://agentgateway.example.com/meta"data`), "", true},
		{"backslash", structpb.NewStringValue(`https://agentgateway.example.com/meta\data`), "", true},
		{"space", structpb.NewStringValue("https://agentgateway.example.com/meta data"), "", true},
		{"header injection", structpb.NewStringValue("https://agentgateway.example.com/\r\nX-Injected: 1"), "", true},
		{"non-ASCII", structpb.NewStringValue("https://agentgateway.example.com/métadata"), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, present := resourceMetadataURL(tt.value)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.present, present)
		})
	}
}
