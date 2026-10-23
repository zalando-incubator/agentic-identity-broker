package oauth2

import (
	"net/url"
	"strings"
	"testing"
)

// TestBuildErrorRedirectURL tests the buildErrorRedirectURL function with table-driven tests.
func TestBuildErrorRedirectURL(t *testing.T) {
	tests := []struct {
		name             string
		redirectURI      string
		state            string
		errorCode        string
		errorDescription string
		wantErr          bool
		wantErrMsg       string
		validateURL      func(t *testing.T, redirectURL string)
	}{
		{
			name:             "valid error redirect with all parameters",
			redirectURI:      "https://client.example.com/callback",
			state:            "xyz123",
			errorCode:        "invalid_client",
			errorDescription: "Client not registered",
			wantErr:          false,
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatalf("Failed to parse result URL: %v", err)
				}

				q := u.Query()
				if q.Get("error") != "invalid_client" {
					t.Errorf("error parameter = %q, want 'invalid_client'", q.Get("error"))
				}
				if q.Get("error_description") != "Client not registered" {
					t.Errorf("error_description = %q, want 'Client not registered'", q.Get("error_description"))
				}
				if q.Get("state") != "xyz123" {
					t.Errorf("state parameter = %q, want 'xyz123'", q.Get("state"))
				}
			},
		},
		{
			name:             "error redirect without state",
			redirectURI:      "https://client.example.com/callback",
			state:            "",
			errorCode:        "server_error",
			errorDescription: "Internal server error",
			wantErr:          false,
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatalf("Failed to parse result URL: %v", err)
				}

				q := u.Query()
				if q.Get("error") != "server_error" {
					t.Errorf("error parameter = %q, want 'server_error'", q.Get("error"))
				}
				if q.Get("state") != "" {
					t.Errorf("state parameter should be empty, got %q", q.Get("state"))
				}
			},
		},
		{
			name:             "error redirect without description",
			redirectURI:      "https://client.example.com/callback",
			state:            "state123",
			errorCode:        "temporarily_unavailable",
			errorDescription: "",
			wantErr:          false,
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatalf("Failed to parse result URL: %v", err)
				}

				q := u.Query()
				if q.Get("error") != "temporarily_unavailable" {
					t.Errorf("error parameter = %q, want 'temporarily_unavailable'", q.Get("error"))
				}
				if q.Get("error_description") != "" {
					t.Errorf("error_description should be empty, got %q", q.Get("error_description"))
				}
			},
		},
		{
			name:             "error redirect with special characters in description",
			redirectURI:      "https://client.example.com/callback",
			state:            "abc",
			errorCode:        "invalid_request",
			errorDescription: "Invalid request: missing client_id & scope parameters",
			wantErr:          false,
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatalf("Failed to parse result URL: %v", err)
				}

				q := u.Query()
				// Special characters should be properly encoded
				if q.Get("error_description") != "Invalid request: missing client_id & scope parameters" {
					t.Errorf("error_description not properly preserved after URL encoding/decoding")
				}
			},
		},
		{
			name:             "error redirect preserves existing query parameters",
			redirectURI:      "https://client.example.com/callback?existing=param",
			state:            "xyz",
			errorCode:        "invalid_scope",
			errorDescription: "Scope not granted",
			wantErr:          false,
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatalf("Failed to parse result URL: %v", err)
				}

				q := u.Query()
				if q.Get("error") != "invalid_scope" {
					t.Errorf("error parameter = %q, want 'invalid_scope'", q.Get("error"))
				}
				if q.Get("existing") != "param" {
					t.Errorf("existing parameter = %q, want 'param'", q.Get("existing"))
				}
			},
		},
		{
			name:             "empty redirect URI returns error",
			redirectURI:      "",
			state:            "xyz",
			errorCode:        "invalid_client",
			errorDescription: "Client not found",
			wantErr:          true,
			wantErrMsg:       "redirect_uri is required",
		},
		{
			name:             "HTTPS redirect URI with port",
			redirectURI:      "https://client.example.com:8443/callback",
			state:            "state456",
			errorCode:        "access_denied",
			errorDescription: "User denied access",
			wantErr:          false,
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatalf("Failed to parse result URL: %v", err)
				}

				q := u.Query()
				if q.Get("error") != "access_denied" {
					t.Errorf("error parameter = %q, want 'access_denied'", q.Get("error"))
				}
			},
		},
		{
			name:        "native authority retains callback and state",
			redirectURI: "github-app://oauth/callback",
			state:       "xyz",
			errorCode:   "invalid_scope",
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatal(err)
				}
				if u.Scheme != "github-app" || u.Host != "oauth" || u.Path != "/callback" || u.Query().Get("error") != "invalid_scope" || u.Query().Get("state") != "xyz" {
					t.Errorf("native callback = %q; want registered authority/path and error/state", redirectURL)
				}
			},
		},
		{
			name:        "single-slash native callback stays single-slash",
			redirectURI: "com.example.app:/oauth2redirect/example-provider",
			errorCode:   "invalid_scope",
			validateURL: func(t *testing.T, redirectURL string) {
				if !strings.HasPrefix(redirectURL, "com.example.app:/oauth2redirect/example-provider?") {
					t.Errorf("native callback = %q; want single-slash form", redirectURL)
				}
			},
		},
		{
			name:        "native query is preserved when errors are appended",
			redirectURI: "github-app://oauth/callback?window=1",
			state:       "xyz",
			errorCode:   "invalid_scope",
			validateURL: func(t *testing.T, redirectURL string) {
				u, err := url.Parse(redirectURL)
				if err != nil {
					t.Fatal(err)
				}
				if u.Query().Get("window") != "1" || u.Query().Get("error") != "invalid_scope" || u.Query().Get("state") != "xyz" {
					t.Errorf("native callback query = %q; want original window, error, and state", u.RawQuery)
				}
			},
		},
		{
			name:        "unsafe script scheme cannot receive an error",
			redirectURI: "javascript://oauth/callback",
			state:       "xyz",
			errorCode:   "invalid_scope",
			wantErr:     true,
			wantErrMsg:  "invalid redirect_uri",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectURL, err := BuildErrorRedirectURL(tt.redirectURI, tt.state, tt.errorCode, tt.errorDescription)

			if (err != nil) != tt.wantErr {
				t.Errorf("BuildErrorRedirectURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.wantErrMsg != "" && err.Error() != tt.wantErrMsg {
					t.Errorf("BuildErrorRedirectURL() error message = %q, want %q", err.Error(), tt.wantErrMsg)
				}
				return
			}

			if !tt.wantErr && tt.validateURL != nil {
				tt.validateURL(t, redirectURL)
			}
		})
	}
}

// TestOAuth2Error tests OAuth2Error implementations.
func TestOAuth2Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *OAuth2Error
		wantMsg string
	}{
		{
			name:    "invalid_client error",
			err:     InvalidClientError("Client not registered"),
			wantMsg: "oauth2_error: invalid_client",
		},
		{
			name:    "server_error",
			err:     ServerError("Internal server error"),
			wantMsg: "oauth2_error: server_error",
		},
		{
			name:    "temporarily_unavailable error",
			err:     TemporarilyUnavailableError("Service unavailable"),
			wantMsg: "oauth2_error: temporarily_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.wantMsg)
			}

			// Verify error code is accessible
			if tt.err.Code == "" {
				t.Error("Error code should not be empty")
			}
		})
	}
}
