package servicescope

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

const testPublicURL = "https://broker.example.com"

func TestParse(t *testing.T) {
	uuid := "880e8400-e29b-41d4-a716-446655440010"
	tests := []struct {
		name       string
		raw        string
		publicURL  string
		kind       Kind
		identifier string
		form       Form
	}{
		{"canonical", testPublicURL + "/sessions/github", testPublicURL, KindVirtual, "github", FormCanonical},
		{"canonical with dots, dashes and underscores", testPublicURL + "/sessions/git_hub.prod-1", testPublicURL, KindVirtual, "git_hub.prod-1", FormCanonical},
		{"lowercase UUID", testPublicURL + "/sessions/" + uuid, testPublicURL, KindVirtual, uuid, FormUUID},
		{"configured trailing slash is removed", testPublicURL + "/sessions/github", testPublicURL + "//", KindVirtual, "github", FormCanonical},
		{"configured path prefix is kept", testPublicURL + "/broker/sessions/github", testPublicURL + "/broker/", KindVirtual, "github", FormCanonical},
		{"identifier at the 128-character limit", testPublicURL + "/sessions/" + repeat("a", 128), testPublicURL, KindVirtual, repeat("a", 128), FormCanonical},
		{"uppercase UUID is never canonical", testPublicURL + "/sessions/880E8400-E29B-41D4-A716-446655440010", testPublicURL, KindMalformed, "", ""},
		{"trailing slash", testPublicURL + "/sessions/github/", testPublicURL, KindMalformed, "", ""},
		{"empty remainder", testPublicURL + "/sessions/", testPublicURL, KindMalformed, "", ""},
		{"nested path", testPublicURL + "/sessions/github/admin", testPublicURL, KindMalformed, "", ""},
		{"query", testPublicURL + "/sessions/github?x=1", testPublicURL, KindMalformed, "", ""},
		{"fragment", testPublicURL + "/sessions/github#x", testPublicURL, KindMalformed, "", ""},
		{"percent encoding", testPublicURL + "/sessions/github%2Fadmin", testPublicURL, KindMalformed, "", ""},
		{"identifier beyond 128 characters", testPublicURL + "/sessions/" + repeat("a", 129), testPublicURL, KindMalformed, "", ""},
		{"other origin lookalike", "https://other.example.com/sessions/github", testPublicURL, KindOrdinary, "", ""},
		{"host case differs", "https://BROKER.example.com/sessions/github", testPublicURL, KindOrdinary, "", ""},
		{"scheme differs", "http://broker.example.com/sessions/github", testPublicURL, KindOrdinary, "", ""},
		{"ordinary scope", "repo", testPublicURL, KindOrdinary, "", ""},
		{"sessions page without trailing slash", testPublicURL + "/sessions", testPublicURL, KindOrdinary, "", ""},
		{"empty public URL never matches", "/sessions/github", "", KindOrdinary, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, kind := Parse(tt.raw, tt.publicURL)
			assert.Equal(t, tt.kind, kind)
			if tt.kind != KindVirtual {
				return
			}
			assert.Equal(t, tt.raw, scope.Raw)
			assert.Equal(t, tt.identifier, scope.Identifier)
			assert.Equal(t, tt.form, scope.Form)
		})
	}
}

func TestFormat(t *testing.T) {
	serviceID := id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	canonicalID := "github"
	assert.Equal(t, testPublicURL+"/sessions/github", Format(testPublicURL+"/", serviceID, &canonicalID))
	assert.Equal(t, testPublicURL+"/sessions/"+serviceID.String(), Format(testPublicURL, serviceID, nil))
	empty := ""
	assert.Equal(t, testPublicURL+"/sessions/"+serviceID.String(), Format(testPublicURL, serviceID, &empty))
}

func TestStripVirtual(t *testing.T) {
	assert.Equal(t, "openid profile", StripVirtual("openid "+testPublicURL+"/sessions/github profile "+testPublicURL+"/sessions/bad/", testPublicURL))
	assert.Equal(t, "", StripVirtual(testPublicURL+"/sessions/github", testPublicURL))
	assert.Equal(t, "openid https://other.example.com/sessions/github", StripVirtual("openid https://other.example.com/sessions/github", testPublicURL))
}

func repeat(value string, count int) string {
	result := ""
	for range count {
		result += value
	}
	return result
}
