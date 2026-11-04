package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/mock-oauth2-service/internal/config"
)

func TestTokenEndpointAcceptsBasicAndPostClientAuth(t *testing.T) {
	cfg := &config.Config{OAuth2: config.OAuth2Config{ClientID: "client", ClientSecret: "secret"}}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name  string
		basic bool
	}{
		{name: "client_secret_basic", basic: true},
		{name: "client_secret_post", basic: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"unknown"}}
			if !tt.basic {
				form.Set("client_id", "client")
				form.Set("client_secret", "secret")
			}
			req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tt.basic {
				req.SetBasicAuth("client", "secret")
			}
			w := httptest.NewRecorder()

			s.router.ServeHTTP(w, req)

			// The client authenticates, so only the unknown refresh token is rejected.
			var body struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if body.Error != "invalid_grant" {
				t.Errorf("expected invalid_grant, got status %d body %s", w.Code, w.Body.String())
			}
		})
	}
}
