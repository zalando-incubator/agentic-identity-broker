package urivalidation

import "testing"

func TestValidateAgentRedirectURIRegistration(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"accepts valid wildcard pattern", "https://preview*.app-*.example.com/api/oidc/callback", false},
		{"preserves exact HTTPS IP registration", "https://192.0.2.1/callback", false},
		{"preserves exact single-label HTTPS registration", "https://internal/callback", false},
		{"preserves loopback HTTP registration", "http://localhost/callback", false},
		{"preserves private-use registration", "com.example.app:/oauth2redirect/callback", false},
		{"rejects wildcard in public suffix", "https://preview*.app-*.example.*/api/oidc/callback", true},
		{"rejects wildcard in effective registrable domain", "https://preview*.app-*.*.com/api/oidc/callback", true},
		{"rejects wildcard replacing entire host", "https://*/api/oidc/callback", true},
			{"rejects wildcard in IP-shaped hostname", "https://192.0.2.*/api/oidc/callback", true},
			{"rejects empty hostname label", "https://preview*.app-*.example..com/api/oidc/callback", true},
			{"rejects invalid IDNA label", "https://preview*.app-*.xn--/api/oidc/callback", true},
			{"rejects invalid DNS label character", "https://preview*.app-*.exa_mple.com/api/oidc/callback", true},
			{"rejects trailing dot in wildcard hostname", "https://preview*.app-*.example.com./api/oidc/callback", true},
		{"rejects wildcard outside hostname", "https://preview.example.com/*", true},
		{"rejects wildcard in query", "https://preview.example.com/callback?target=*", true},
		{"accepts wildcard in one subdomain label", "https://*.example.com/callback", false},
		{"rejects non-loopback HTTP registration", "http://preview*.example.com/callback", true},
		{"rejects fragment", "https://preview*.example.com/callback#fragment", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAgentRedirectURIRegistration(test.uri)
			if test.wantErr && err == nil {
				t.Fatalf("ValidateAgentRedirectURIRegistration(%q) = nil, want error", test.uri)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("ValidateAgentRedirectURIRegistration(%q) = %v, want nil", test.uri, err)
			}
		})
	}
}