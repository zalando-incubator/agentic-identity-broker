package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubOAuth2Service implements ports.OAuth2Service for integration tests.
// ResolveForTokenGrant returns a pre-configured agent.
type stubOAuth2Service struct {
	agent *domainstorage.Agent
}

func (s *stubOAuth2Service) HandleAuthorization(_ context.Context, _ *ports.AuthorizationRequest, _ id.Principal) (*ports.AuthorizationDecision, error) {
	return nil, nil
}

func (s *stubOAuth2Service) ResolveForTokenGrant(_ context.Context, _ id.ClientID) (*ports.TokenGrantResolution, error) {
	return &ports.TokenGrantResolution{AgentID: s.agent.ID, ClientID: s.agent.ClientID, ClientType: s.agent.ClientType()}, nil
}

func (s *stubOAuth2Service) GenerateMetadata(_ context.Context) (*ports.MetadataResponse, error) {
	return nil, nil
}

func newTestTokenHandler(upstreamURL string, agentID id.AgentID) *enduser.OAuth2TokenHandler {
	agent := &domainstorage.Agent{
		ID:       agentID,
		ClientID: ptr.To(id.ClientID("test-upstream-client-id")),
	}
	transport := enduser.NewOAuth2TokenProxy(upstreamURL, nil)
	outcomes := oauth2.NewTokenOutcomeService(transport, nil)
	return &enduser.OAuth2TokenHandler{
		OAuth2Service: &stubOAuth2Service{agent: agent},
		GrantHandler:  enduser.NewProxyTokenGrantStrategy(upstreamURL, outcomes, nil),
	}
}

// TestOAuth2TokenEndpoint_SuccessfulTokenExchange tests complete token exchange flow
func TestOAuth2TokenEndpoint_SuccessfulTokenExchange(t *testing.T) {
	agentID := id.NewAgentID()

	// Mock upstream OAuth2 server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		// Verify required token request parameters
		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, "authorization_code", r.FormValue("grant_type"))
		assert.Equal(t, "auth_code_123", r.FormValue("code"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "access_token_xyz",
			"token_type": "Bearer",
			"expires_in": 3600,
			"scope": "openid profile"
		}`))
	}))
	defer mockUpstream.Close()

	handler := newTestTokenHandler(mockUpstream.URL, agentID)

	reqBody := strings.NewReader("grant_type=authorization_code&code=auth_code_123&client_id=" + agentID.String() + "&client_secret=secret&redirect_uri=https://client.example.com/callback")
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify broker proxies response correctly
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "access_token_xyz")
	assert.Contains(t, string(respBody), "Bearer")
}

// TestOAuth2TokenEndpoint_RefreshTokenGrant tests refresh token grant exchange
func TestOAuth2TokenEndpoint_RefreshTokenGrant(t *testing.T) {
	agentID := id.NewAgentID()

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))
		assert.Equal(t, "refresh_token_abc", r.FormValue("refresh_token"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "new_access_token",
			"token_type": "Bearer",
			"expires_in": 3600
		}`))
	}))
	defer mockUpstream.Close()

	handler := newTestTokenHandler(mockUpstream.URL, agentID)

	reqBody := strings.NewReader("grant_type=refresh_token&refresh_token=refresh_token_abc&client_id=" + agentID.String() + "&client_secret=secret")
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "new_access_token")
}

// TestOAuth2TokenEndpoint_InvalidGrantError tests upstream error responses are proxied
func TestOAuth2TokenEndpoint_InvalidGrantError(t *testing.T) {
	agentID := id.NewAgentID()

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": "invalid_grant",
			"error_description": "Authorization code has expired or been revoked"
		}`))
	}))
	defer mockUpstream.Close()

	handler := newTestTokenHandler(mockUpstream.URL, agentID)

	reqBody := strings.NewReader("grant_type=authorization_code&code=expired_code&client_id=" + agentID.String())
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Upstream error status and response preserved
	assert.Equal(t, http.StatusBadRequest, w.Code)
	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "invalid_grant")
	assert.Contains(t, string(respBody), "expired")
}

// TestOAuth2TokenEndpoint_HeadersFiltered verifies that proxy token grants do not disclose inbound credentials upstream.
func TestOAuth2TokenEndpoint_HeadersFiltered(t *testing.T) {
	agentID := id.NewAgentID()
	var upstreamHeaders http.Header

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "token123", "token_type": "Bearer"}`))
	}))
	defer mockUpstream.Close()

	handler := newTestTokenHandler(mockUpstream.URL, agentID)

	body := strings.NewReader("grant_type=authorization_code&code=abc123&client_id=" + agentID.String())
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic aW5ib3VuZC1jcmVkZW50aWFs")
	req.Header.Set("Cookie", "broker_session=inbound-credential")
	req.Header.Set("X-Remote-User", "user@example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-www-form-urlencoded", upstreamHeaders.Get("Content-Type"))
	for _, header := range []string{"Authorization", "Cookie", "X-Remote-User"} {
		assert.Empty(t, upstreamHeaders.Get(header), "inbound %s must not be forwarded upstream", header)
	}
}

// TestOAuth2TokenEndpoint_StandardHeadersPreserved verifies only OAuth2 response metadata is relayed.
func TestOAuth2TokenEndpoint_StandardHeadersPreserved(t *testing.T) {
	agentID := id.NewAgentID()

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Set-Cookie", "upstream_session=credential; Secure; HttpOnly")
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "token123", "token_type": "Bearer"}`))
	}))
	defer mockUpstream.Close()

	handler := newTestTokenHandler(mockUpstream.URL, agentID)

	body := strings.NewReader("grant_type=authorization_code&code=abc123&client_id=" + agentID.String())
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Empty(t, w.Header().Values("Set-Cookie"))
	assert.Empty(t, w.Header().Get("X-Custom-Header"))
}

// TestOAuth2TokenEndpoint_StatusCodePreserved tests various status codes are preserved
func TestOAuth2TokenEndpoint_StatusCodePreserved(t *testing.T) {
	agentID := id.NewAgentID()

	tests := []struct {
		name            string
		upstreamStatus  int
		upstreamBody    string
		wwwAuthenticate string
	}{
		{
			name:           "200 OK",
			upstreamStatus: http.StatusOK,
			upstreamBody:   `{"access_token": "token", "token_type": "Bearer"}`,
		},
		{
			name:           "400 Bad Request",
			upstreamStatus: http.StatusBadRequest,
			upstreamBody:   `{"error": "invalid_request"}`,
		},
		{
			name:            "401 Unauthorized",
			upstreamStatus:  http.StatusUnauthorized,
			upstreamBody:    `{"error": "invalid_client"}`,
			wwwAuthenticate: `Basic realm="upstream"`,
		},
		{
			name:           "500 Internal Server Error",
			upstreamStatus: http.StatusInternalServerError,
			upstreamBody:   `{"error": "server_error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tt.wwwAuthenticate != "" {
					w.Header().Set("WWW-Authenticate", tt.wwwAuthenticate)
				}
				w.WriteHeader(tt.upstreamStatus)
				_, _ = w.Write([]byte(tt.upstreamBody))
			}))
			defer mockUpstream.Close()

			handler := newTestTokenHandler(mockUpstream.URL, agentID)

			body := strings.NewReader("grant_type=authorization_code&code=abc123&client_id=" + agentID.String())
			req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", body)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Verify status code preserved
			assert.Equal(t, tt.upstreamStatus, w.Code)
			assert.Equal(t, tt.wwwAuthenticate, w.Header().Get("WWW-Authenticate"))
		})
	}
}

func TestOAuth2TokenEndpoint_CIMDFetchFailuresAreDirect(t *testing.T) {
	const clientURL = "https://client.example/metadata"
	for _, test := range []struct {
		name       string
		fetcher    ports.CIMDFetcher
		wantStatus int
		wantError  string
	}{
		{name: "temporarily unavailable", fetcher: integrationCIMDFetcher{err: &ports.CIMDUnavailableError{Cause: context.DeadlineExceeded}}, wantStatus: http.StatusInternalServerError, wantError: "server_error"},
		{name: "SSRF blocked", fetcher: integrationCIMDFetcher{err: &ports.SSRFBlockedError{IP: "127.0.0.1"}}, wantStatus: http.StatusUnauthorized, wantError: "invalid_client"},
		{name: "malformed document", fetcher: integrationCIMDFetcher{result: &ports.CIMDFetchResult{Body: []byte("{")}}, wantStatus: http.StatusUnauthorized, wantError: "invalid_client"},
		{name: "policy violation", fetcher: integrationCIMDFetcher{result: &ports.CIMDFetchResult{Body: []byte(`{"client_id":"https://other.example/client","redirect_uris":["https://client.example/callback"]}`)}}, wantStatus: http.StatusUnauthorized, wantError: "invalid_client"},
		{name: "client error response", fetcher: integrationCIMDFetcher{err: errors.New("CIMD endpoint returned status 404")}, wantStatus: http.StatusUnauthorized, wantError: "invalid_client"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, agents := newCIMDIntegrationOAuth2Service(t, test.fetcher)
			agent := &domainstorage.Agent{ID: id.NewAgentID(), ClientURIs: []string{clientURL}, DisplayName: "CIMD Integration Agent"}
			require.NoError(t, agents.Create(context.Background(), agent))
			handler := &enduser.OAuth2TokenHandler{
				OAuth2Service: service,
				GrantHandler:  enduser.NewLocalGrantStrategy(nil, nil),
			}
			form := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientURL}, "code": {"unused-code"}}
			request := httptest.NewRequest(http.MethodPost, "https://broker.example.com/oauth2/token", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, test.wantStatus, response.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, test.wantError, body["error"])
		})
	}
}
