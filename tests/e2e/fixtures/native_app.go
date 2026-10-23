package fixtures

import "encoding/json"

const (
	GitHubAppCallback = "github-app://oauth/callback"
	NativeAppCallback = "com.example.app:/oauth2redirect/example-provider"
)

// GitHubCopilotDesktopDocument returns the published client metadata with caller-supplied redirects.
func GitHubCopilotDesktopDocument(clientID string, redirectURIs ...string) []byte {
	document, _ := json.Marshal(struct {
		ClientID        string   `json:"client_id"`
		ClientName      string   `json:"client_name"`
		ClientURI       string   `json:"client_uri"`
		ApplicationType string   `json:"application_type"`
		GrantTypes      []string `json:"grant_types"`
		ResponseTypes   []string `json:"response_types"`
		AuthMethod      string   `json:"token_endpoint_auth_method"`
		RedirectURIs    []string `json:"redirect_uris"`
	}{
		ClientID:        clientID,
		ClientName:      "GitHub Copilot",
		ClientURI:       "https://github.com/github/app",
		ApplicationType: "native",
		GrantTypes:      []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		ResponseTypes:   []string{"code"},
		AuthMethod:      "none",
		RedirectURIs:    redirectURIs,
	})
	return document
}
