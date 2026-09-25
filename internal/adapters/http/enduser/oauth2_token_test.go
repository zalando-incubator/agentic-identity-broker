package enduser

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTokenMintingStrategy is a configurable test double for ports.TokenMintingStrategy.
type mockTokenMintingStrategy struct {
	clientCredentialsFn         func(context.Context, id.ClientID, string, string) (*ports.TokenResponse, error)
	authorizationCodeExchangeFn func(context.Context, id.ClientID, string, string, string, string) (*ports.TokenResponse, error)
	refreshTokenFn              func(context.Context, id.ClientID, string, string, string) (*ports.TokenResponse, error)
}

func (m *mockTokenMintingStrategy) HandleClientCredentials(ctx context.Context, clientID id.ClientID, clientSecret, scope string) (*ports.TokenResponse, error) {
	return m.clientCredentialsFn(ctx, clientID, clientSecret, scope)
}

func (m *mockTokenMintingStrategy) HandleAuthorizationCodeExchange(ctx context.Context, clientID id.ClientID, clientSecret, code, redirectURI, codeVerifier string) (*ports.TokenResponse, error) {
	return m.authorizationCodeExchangeFn(ctx, clientID, clientSecret, code, redirectURI, codeVerifier)
}

func (m *mockTokenMintingStrategy) HandleRefreshToken(ctx context.Context, clientID id.ClientID, clientSecret, refreshToken, scope string) (*ports.TokenResponse, error) {
	return m.refreshTokenFn(ctx, clientID, clientSecret, refreshToken, scope)
}

// fixedMinting returns a mock strategy that always returns the given response/error for all local grant types.
func fixedMinting(resp *ports.TokenResponse, err error) *mockTokenMintingStrategy {
	return &mockTokenMintingStrategy{
		clientCredentialsFn: func(_ context.Context, _ id.ClientID, _, _ string) (*ports.TokenResponse, error) {
			return resp, err
		},
		authorizationCodeExchangeFn: func(_ context.Context, _ id.ClientID, _, _, _, _ string) (*ports.TokenResponse, error) {
			return resp, err
		},
		refreshTokenFn: func(_ context.Context, _ id.ClientID, _, _, _ string) (*ports.TokenResponse, error) {
			return resp, err
		},
	}
}

// mockMultiAgentVerifier is a test double for MultiAgentVerifier
type mockMultiAgentVerifier struct {
	verifyFn func(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error
}

func (m *mockMultiAgentVerifier) VerifyAgentIDClaim(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error {
	return m.verifyFn(ctx, responseBody, expectedAgentID)
}

// stubAgentRepo is a minimal agent repository stub for unit testing.
// Get always returns the configured agent (or error), ignoring the agentID argument.
// This is intentional: unit tests in this package focus on HTTP handler behaviour
// (client_id replacement, error propagation) rather than repository routing. The
// correct agent is selected by configuring the stub with the expected agent.
// Storage-layer routing (fetching the correct agent by ID) is tested in the storage adapter tests.
type stubAgentRepo struct {
	agent *storage.Agent
	err   error
}

func newStubAgentRepo(agentID id.AgentID, upstreamClientID string) *stubAgentRepo {
	return &stubAgentRepo{
		agent: &storage.Agent{
			ID:       agentID,
			ClientID: ptr.To(id.ClientID(upstreamClientID)),
		},
	}
}

func (r *stubAgentRepo) Get(_ context.Context, _ id.AgentID) (*storage.Agent, error) {
	return r.agent, r.err
}

func (r *stubAgentRepo) Create(_ context.Context, _ *storage.Agent) error { return nil }
func (r *stubAgentRepo) Update(_ context.Context, _ *storage.Agent) error { return nil }
func (r *stubAgentRepo) Delete(_ context.Context, _ id.AgentID) error     { return nil }
func (r *stubAgentRepo) List(_ context.Context) ([]*storage.Agent, error) { return nil, nil }
func (r *stubAgentRepo) GetByClientID(_ context.Context, _ id.ClientID) (*storage.Agent, error) {
	return nil, nil
}

func (r *stubAgentRepo) GetByClientURI(_ context.Context, _ string) (*storage.Agent, error) {
	return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindNotFound, nil, "not found")
}

func (r *stubAgentRepo) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

// mockOAuth2ServiceForToken implements ports.OAuth2Service with configurable ResolveForTokenGrant.
type mockOAuth2ServiceForToken struct {
	resolveFn func(ctx context.Context, clientID id.ClientID) (*ports.TokenGrantResolution, error)
}

func (m *mockOAuth2ServiceForToken) HandleAuthorization(_ context.Context, _ *ports.AuthorizationRequest, _ id.Principal) (*ports.AuthorizationDecision, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOAuth2ServiceForToken) GenerateMetadata(_ context.Context) (*ports.MetadataResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockOAuth2ServiceForToken) ResolveForTokenGrant(ctx context.Context, clientID id.ClientID) (*ports.TokenGrantResolution, error) {
	return m.resolveFn(ctx, clientID)
}

func newResolvingOAuth2Service(agent *storage.Agent) *mockOAuth2ServiceForToken {
	return &mockOAuth2ServiceForToken{
		resolveFn: func(_ context.Context, _ id.ClientID) (*ports.TokenGrantResolution, error) {
			return ports.NewTokenGrantResolution(agent.ID, agent.ClientID, agent.ClientType())
		},
	}
}

func newFailingOAuth2Service(err error) *mockOAuth2ServiceForToken {
	return &mockOAuth2ServiceForToken{
		resolveFn: func(_ context.Context, _ id.ClientID) (*ports.TokenGrantResolution, error) {
			return nil, err
		},
	}
}

// newLocalModeOAuth2Service returns an OAuth2Service mock that resolves valid UUIDs to a
// dummy local agent (no ClientID) and rejects non-UUIDs with invalid_client.
func newLocalModeOAuth2Service() *mockOAuth2ServiceForToken {
	return &mockOAuth2ServiceForToken{
		resolveFn: func(_ context.Context, clientID id.ClientID) (*ports.TokenGrantResolution, error) {
			_, err := id.ParseAgentID(clientID.String())
			if err != nil {
				return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "client authentication failed"}
			}
			return &ports.TokenGrantResolution{
				AgentID:    id.NewAgentID(),
				ClientType: storage.LocalClient,
			}, nil
		},
	}
}

// TestOAuth2TokenHandler_ServeHTTP_ContentTypeValidation tests Content-Type validation
func TestOAuth2TokenHandler_ServeHTTP_ContentTypeValidation(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := newStubAgentRepo(agentID, "test-upstream-client")

	// Mock upstream server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "token123", "token_type": "Bearer"}`))
	}))
	defer mockUpstream.Close()

	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
	}

	tests := []struct {
		name           string
		contentType    string
		wantStatusCode int
	}{
		{
			name:           "valid content-type application/x-www-form-urlencoded",
			contentType:    "application/x-www-form-urlencoded",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "valid parameterized form content-type",
			contentType:    "application/x-www-form-urlencoded; charset=UTF-8",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "invalid content-type application/json",
			contentType:    "application/json",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "invalid content-type with form prefix",
			contentType:    "application/x-www-form-urlencoded-malicious",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "invalid content-type text/plain",
			contentType:    "text/plain",
			wantStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "grant_type=authorization_code&code=abc123&client_id=" + agentID.String()
			req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", strings.NewReader(body))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatusCode, w.Code)
		})
	}
}

// TestOAuth2TokenHandler_PreFlightErrorsReturnJSON verifies that all pre-flight error
// paths return application/json with a valid RFC 6749 error body, not text/plain.
func TestOAuth2TokenHandler_PreFlightErrorsReturnJSON(t *testing.T) {
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewLocalGrantStrategy(fixedMinting(nil, nil), nil),
		OAuth2Service: newLocalModeOAuth2Service(),
	}

	tests := []struct {
		name       string
		method     string
		ct         string
		body       io.Reader
		wantStatus int
		wantError  string
	}{
		{
			name:       "wrong HTTP method returns JSON error",
			method:     "GET",
			ct:         "application/x-www-form-urlencoded",
			body:       strings.NewReader("grant_type=client_credentials"),
			wantStatus: http.StatusMethodNotAllowed,
			wantError:  "invalid_request",
		},
		{
			name:       "wrong Content-Type returns JSON error",
			method:     "POST",
			ct:         "application/json",
			body:       strings.NewReader(`{"grant_type":"client_credentials"}`),
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
		{
			name:       "empty body returns JSON error",
			method:     "POST",
			ct:         "application/x-www-form-urlencoded",
			body:       strings.NewReader(""),
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
		{
			name:       "body read failure returns JSON error",
			method:     "POST",
			ct:         "application/x-www-form-urlencoded",
			body:       &failingReadCloser{err: errors.New("disk failure")},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/oauth2/token", tt.body)
			req.Header.Set("Content-Type", tt.ct)
			if fr, ok := tt.body.(*failingReadCloser); ok {
				req.Body = fr
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"),
				"token endpoint must return application/json for errors per RFC 6749 §5.2")

			var body map[string]string
			err := json.NewDecoder(w.Body).Decode(&body)
			assert.NoError(t, err, "response body must be valid JSON")
			assert.Equal(t, tt.wantError, body["error"])
			assert.NotEmpty(t, body["error_description"])
		})
	}
}

func TestOAuth2TokenHandler_ServeHTTP_RejectsOversizedBody(t *testing.T) {
	handler := &OAuth2TokenHandler{}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(strings.Repeat("x", 256*1024+1)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "invalid_request", body["error"])
	assert.Equal(t, "failed to read request body", body["error_description"])
}

// TestOAuth2TokenHandler_ServeHTTP_HeaderFiltering verifies allowlisted proxy request and response headers.
func TestOAuth2TokenHandler_ServeHTTP_HeaderFiltering(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := newStubAgentRepo(agentID, "test-upstream-client")
	var upstreamHeaders http.Header

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Set-Cookie", "upstream_session=credential; Secure; HttpOnly")
		w.Header().Set("X-Upstream-Credential", "must-not-leave-upstream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "token123", "token_type": "Bearer"}`))
	}))
	defer mockUpstream.Close()

	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
	}

	reqBody := strings.NewReader("grant_type=authorization_code&code=abc123&client_id=" + agentID.String())
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Authorization", "Basic aW5ib3VuZC1jcmVkZW50aWFs")
	req.Header.Set("Cookie", "broker_session=inbound-credential")
	req.Header.Set("X-Remote-User", "user@example.com")
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.Header.Set("X-Custom-Credential", "inbound-credential")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-www-form-urlencoded; charset=UTF-8", upstreamHeaders.Get("Content-Type"))
	for _, header := range []string{"Authorization", "Cookie", "X-Remote-User", "X-Forwarded-For", "X-Custom-Credential"} {
		assert.Empty(t, upstreamHeaders.Get(header), "inbound %s must not be forwarded upstream", header)
	}
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Empty(t, w.Header().Values("Set-Cookie"))
	assert.Empty(t, w.Header().Get("X-Upstream-Credential"))
}

// TestOAuth2TokenHandler_ServeHTTP_SuccessfulProxy tests successful token request proxy
func TestOAuth2TokenHandler_ServeHTTP_SuccessfulProxy(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := newStubAgentRepo(agentID, "test-upstream-client")

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token": "token123", "token_type": "Bearer", "expires_in": 3600}`))
	}))
	defer mockUpstream.Close()

	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
	}

	reqBody := strings.NewReader("grant_type=authorization_code&code=abc123&client_id=" + agentID.String() + "&redirect_uri=https://client.example.com/callback")
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))

	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "access_token")
	assert.Contains(t, string(respBody), "token123")
}

// TestOAuth2TokenHandler_ServeHTTP_UpstreamError tests upstream errors are proxied
func TestOAuth2TokenHandler_ServeHTTP_UpstreamError(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := newStubAgentRepo(agentID, "test-upstream-client")

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant", "error_description": "Authorization code expired"}`))
	}))
	defer mockUpstream.Close()

	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
	}

	reqBody := strings.NewReader("grant_type=authorization_code&code=expired&client_id=" + agentID.String())
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Upstream error status is preserved
	assert.Equal(t, http.StatusBadRequest, w.Code)
	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "invalid_grant")
}

// TestProxyGrantStrategy_InfraErrorsReturnJSON verifies that infrastructure failures
// in the proxy strategy return application/json per RFC 6749 §5.2.
func TestProxyGrantStrategy_InfraErrorsReturnJSON(t *testing.T) {
	agentID := id.NewAgentID()

	t.Run("unreachable upstream returns JSON server_error", func(t *testing.T) {
		// Use an invalid URL that will fail to connect
		strategy := NewProxyTokenGrantStrategy("http://127.0.0.1:1/token", nil, nil, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/oauth2/token", nil)

		resolution := &ports.TokenGrantResolution{
			AgentID:    agentID,
			ClientID:   ptr.To(id.ClientID("upstream-client")),
			ClientType: storage.ProxyClient,
		}
		strategy.HandleTokenGrant(w, req, "authorization_code", url.Values{
			"grant_type": {"authorization_code"},
			"client_id":  {agentID.String()},
			"code":       {"abc"},
		}, resolution)

		assert.Equal(t, "application/json", w.Header().Get("Content-Type"),
			"upstream contact failure must return application/json")
		var body map[string]string
		_ = json.NewDecoder(w.Body).Decode(&body)
		assert.Equal(t, "server_error", body["error"])
	})
}

// TestOAuth2TokenHandler_ProxyToUpstream_MultiAgentVerifier tests claim verification
// for multi-agent client sharing (Feature 021 US1).
func TestOAuth2TokenHandler_ProxyToUpstream_MultiAgentVerifier(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := newStubAgentRepo(agentID, "test-upstream-client")
	upstreamResponseBody := `{"access_token":"tok123","token_type":"Bearer"}`

	tests := []struct {
		name             string
		verifier         ports.MultiAgentVerifier
		clientID         string // form body client_id
		wantStatusCode   int
		wantBodyContains string
	}{
		{
			name:             "nil verifier (feature disabled) passes response through unchanged",
			verifier:         nil,
			clientID:         agentID.String(),
			wantStatusCode:   http.StatusOK,
			wantBodyContains: "access_token",
		},
		{
			name: "verifier returns nil (claim matches) passes response through",
			verifier: &mockMultiAgentVerifier{verifyFn: func(_ context.Context, _ []byte, _ id.AgentID) error {
				return nil
			}},
			clientID:         agentID.String(),
			wantStatusCode:   http.StatusOK,
			wantBodyContains: "access_token",
		},
		{
			name: "verifier returns claim-absent error returns 500 server_error",
			verifier: &mockMultiAgentVerifier{verifyFn: func(_ context.Context, _ []byte, _ id.AgentID) error {
				return errors.New("agent ID claim absent from upstream token")
			}},
			clientID:         agentID.String(),
			wantStatusCode:   http.StatusInternalServerError,
			wantBodyContains: "server_error",
		},
		{
			name: "verifier returns claim-mismatch error returns 500 server_error",
			verifier: &mockMultiAgentVerifier{verifyFn: func(_ context.Context, _ []byte, _ id.AgentID) error {
				return errors.New("agent ID claim mismatch")
			}},
			clientID:         agentID.String(),
			wantStatusCode:   http.StatusInternalServerError,
			wantBodyContains: "server_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(upstreamResponseBody))
			}))
			defer mockUpstream.Close()

			handler := &OAuth2TokenHandler{
				GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, tt.verifier, nil),
				OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
			}

			body := "grant_type=authorization_code&code=abc123&client_id=" + tt.clientID
			req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatusCode, w.Code)
			respBody, _ := io.ReadAll(w.Body)
			assert.Contains(t, string(respBody), tt.wantBodyContains)
		})
	}
}

// TestOAuth2TokenHandler_ServeHTTP_ResponseStreaming tests response is streamed properly
func TestOAuth2TokenHandler_ServeHTTP_ResponseStreaming(t *testing.T) {
	agentID := id.NewAgentID()
	agentRepo := newStubAgentRepo(agentID, "test-upstream-client")

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("traceresponse", "00-upstream-trace-id-upstream-span-id-01")
		w.WriteHeader(http.StatusOK)
		// Return large response to test streaming
		_, _ = w.Write([]byte(`{"access_token": "verylongtoken123456789", "token_type": "Bearer", "expires_in": 3600, "scope": "openid profile email"}`))
	}))
	defer mockUpstream.Close()

	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
	}

	reqBody := strings.NewReader("grant_type=authorization_code&code=abc123&client_id=" + agentID.String())
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	w.Header().Set("traceresponse", "00-broker-trace-id-broker-span-id-01")

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"00-broker-trace-id-broker-span-id-01"}, w.Header().Values("traceresponse"))

	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "access_token")
	assert.Contains(t, string(respBody), "verylongtoken123456789")
}

// TestOAuth2TokenHandler_ClientIDValidation tests that client_id is always validated as an
// agent UUID, regardless of whether MultiAgentVerifier is set.  The client_id is the
// broker-internal agent UUID from the perspective of every OAuth2 client (spec: Feature 021).
func TestOAuth2TokenHandler_ClientIDValidation(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer"}`))
	}))
	defer mockUpstream.Close()

	verifiers := []struct {
		name     string
		verifier ports.MultiAgentVerifier
	}{
		{"without verifier", nil},
		{"with verifier", &mockMultiAgentVerifier{verifyFn: func(_ context.Context, _ []byte, _ id.AgentID) error { return nil }}},
	}

	tests := []struct {
		name           string
		body           string
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "missing client_id returns 400 invalid_request",
			body:           "grant_type=authorization_code&code=abc123",
			wantStatusCode: http.StatusBadRequest,
			wantError:      "invalid_request",
		},
		{
			name:           "non-UUID client_id returns 401 invalid_client",
			body:           "grant_type=authorization_code&code=abc123&client_id=not-a-uuid",
			wantStatusCode: http.StatusUnauthorized,
			wantError:      "invalid_client",
		},
		{
			name:           "legacy non-UUID client_id returns 401 invalid_client",
			body:           "grant_type=authorization_code&code=abc123&client_id=legacy-client-id",
			wantStatusCode: http.StatusUnauthorized,
			wantError:      "invalid_client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Behaviour must be identical regardless of whether MultiAgentVerifier is set.
			for _, v := range verifiers {
				t.Run(v.name, func(t *testing.T) {
					handler := &OAuth2TokenHandler{
						GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, v.verifier, nil),
						OAuth2Service: newFailingOAuth2Service(&ports.ClientIDError{Code: "invalid_client", Desc: "client authentication failed"}),
					}

					req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", strings.NewReader(tt.body))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					w := httptest.NewRecorder()

					handler.ServeHTTP(w, req)

					assert.Equal(t, tt.wantStatusCode, w.Code)
					respBody, _ := io.ReadAll(w.Body)
					assert.Contains(t, string(respBody), tt.wantError)
				})
			}
		})
	}
}

// TestOAuth2TokenHandler_ProxyToUpstream_ClientIDReplacement verifies that the broker
// replaces the agent UUID in the token request body with the agent's upstream client_id
// before forwarding to the upstream OAuth2 server (review comment r2995280734).
func TestOAuth2TokenHandler_ProxyToUpstream_ClientIDReplacement(t *testing.T) {
	agentID := id.NewAgentID()
	const upstreamClientID = "shared-upstream-oauth2-client"

	var receivedClientID string
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the client_id that upstream received
		if err := r.ParseForm(); err == nil {
			receivedClientID = r.FormValue("client_id")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"Bearer"}`))
	}))
	defer mockUpstream.Close()

	agentRepo := newStubAgentRepo(agentID, upstreamClientID)
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newResolvingOAuth2Service(agentRepo.agent),
	}

	body := "grant_type=authorization_code&code=abc&client_id=" + agentID.String()
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Upstream must receive the configured upstream client_id, NOT the broker agent UUID.
	assert.Equal(t, upstreamClientID, receivedClientID)
	assert.NotEqual(t, agentID.String(), receivedClientID)
}

// TestOAuth2TokenHandler_ProxyToUpstream_AgentNotFound verifies that when the agent UUID
// is valid but the agent does not exist in the repository, the handler fails closed with
// 401 invalid_client without forwarding the request upstream (per SR-001).
func TestOAuth2TokenHandler_ProxyToUpstream_AgentNotFound(t *testing.T) {
	agentID := id.NewAgentID()

	upstreamCalled := false
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	// OAuth2Service returns an error for agent not found
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil),
		OAuth2Service: newFailingOAuth2Service(&ports.ClientIDError{Code: "invalid_client", Desc: "agent not found"}),
	}

	body := "grant_type=authorization_code&code=abc&client_id=" + agentID.String()
	req := httptest.NewRequest("POST", "https://broker.example.com/oauth2/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	respBody, _ := io.ReadAll(w.Body)
	assert.Contains(t, string(respBody), "invalid_client")
	assert.False(t, upstreamCalled, "upstream must not be called when agent is not found")
}

// TestProxyGrantStrategy_NilClientID_ReturnsServerError verifies that a nil ClientID
// (server misconfiguration) returns 500 server_error, not 400 invalid_client.
// This is a server-side configuration error, not a client authentication failure.
func TestProxyGrantStrategy_NilClientID_ReturnsServerError(t *testing.T) {
	upstreamCalled := false
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	strategy := NewProxyTokenGrantStrategy(mockUpstream.URL, nil, nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader("grant_type=authorization_code&code=abc"))

	// resolution with nil ClientID = misconfigured proxy agent
	resolution := &ports.TokenGrantResolution{
		AgentID:    id.NewAgentID(),
		ClientType: storage.ProxyClient,
		ClientID:   nil,
	}

	strategy.HandleTokenGrant(w, req, "authorization_code", url.Values{"grant_type": {"authorization_code"}}, resolution)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "nil ClientID is server misconfiguration, not client error")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "server_error", body["error"])
	assert.False(t, upstreamCalled, "upstream must not be called when ClientID is nil")
}

func TestWriteTokenResponse(t *testing.T) {
	t.Run("success returns 200 with complete JSON body", func(t *testing.T) {
		s := &localGrantStrategy{}
		w := httptest.NewRecorder()

		s.writeTokenResponse(w, &ports.TokenResponse{
			AccessToken:  "tok123",
			RefreshToken: "refresh123",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
		})

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "tok123", body["access_token"])
		assert.Equal(t, "refresh123", body["refresh_token"])
		assert.Equal(t, "Bearer", body["token_type"])
		assert.EqualValues(t, 3600, body["expires_in"])
		assert.NotContains(t, body, "scope")
	})

	t.Run("scope included when non-empty", func(t *testing.T) {
		s := &localGrantStrategy{}
		w := httptest.NewRecorder()

		s.writeTokenResponse(w, &ports.TokenResponse{
			AccessToken: "tok456",
			TokenType:   "Bearer",
			ExpiresIn:   900,
			Scope:       "read write",
		})

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		assert.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "read write", body["scope"])
	})
}

// TestHandleLocalMinting_ClientCredentials covers the client_credentials path through
// HandleTokenGrant: success, input validation failures, and strategy error mapping.
func TestHandleLocalMinting_ClientCredentials(t *testing.T) {
	successResp := &ports.TokenResponse{AccessToken: "tok123", TokenType: "Bearer", ExpiresIn: 3600, Scope: "read"}

	tests := []struct {
		name          string
		body          string
		mintingErr    error // nil = return successResp
		wantStatus    int
		wantErrorCode string // empty = expect success
	}{
		{
			name:       "success returns 200 with all token fields",
			body:       "grant_type=client_credentials&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=secret&scope=read",
			wantStatus: http.StatusOK,
		},
		{
			name:          "missing client_id returns 400 invalid_request",
			body:          "grant_type=client_credentials&client_secret=secret",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "non-UUID client_id returns 401 invalid_client",
			body:          "grant_type=client_credentials&client_id=broker_abc&client_secret=secret",
			mintingErr:    oauth2server.NewRFC6749Error("invalid_client", "client authentication failed", http.StatusUnauthorized, oauth2server.ErrInvalidClient),
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "invalid_client",
		},
		{
			name:          "missing client_secret returns 400 invalid_request",
			body:          "grant_type=client_credentials&client_id=550e8400-e29b-41d4-a716-446655440000",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "strategy ErrInvalidClient returns 401 invalid_client",
			body:          "grant_type=client_credentials&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=wrong",
			mintingErr:    oauth2server.NewRFC6749Error("invalid_client", "client authentication failed", http.StatusUnauthorized, oauth2server.ErrInvalidClient),
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "invalid_client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minting := &mockTokenMintingStrategy{
				clientCredentialsFn: func(_ context.Context, _ id.ClientID, _, _ string) (*ports.TokenResponse, error) {
					if tt.mintingErr != nil {
						return nil, tt.mintingErr
					}
					return successResp, nil
				},
			}
			handler := &OAuth2TokenHandler{GrantHandler: NewLocalGrantStrategy(minting, nil), OAuth2Service: newLocalModeOAuth2Service()}
			req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			var body map[string]interface{}
			_ = json.NewDecoder(w.Body).Decode(&body)
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, body["error"])
			} else {
				assert.Equal(t, "tok123", body["access_token"])
				assert.Equal(t, "Bearer", body["token_type"])
				assert.EqualValues(t, 3600, body["expires_in"])
				assert.Equal(t, "read", body["scope"])
			}
		})
	}
}

// TestHandleLocalMinting_AuthorizationCode covers the authorization_code path through
// HandleTokenGrant: success, input validation failures, and strategy error mapping.
func TestHandleLocalMinting_AuthorizationCode(t *testing.T) {
	successResp := &ports.TokenResponse{AccessToken: "tok456", TokenType: "Bearer", ExpiresIn: 900}

	tests := []struct {
		name          string
		body          string
		mintingErr    error
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:       "success returns 200 with all token fields",
			body:       "grant_type=authorization_code&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=secret&code=authcode123&redirect_uri=https://example.com/cb&code_verifier=verifier",
			wantStatus: http.StatusOK,
		},
		{
			name:          "missing client_id returns 400 invalid_request",
			body:          "grant_type=authorization_code&client_secret=secret&code=abc",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "non-UUID client_id returns 401 invalid_client",
			body:          "grant_type=authorization_code&client_id=broker_abc&client_secret=secret&code=abc",
			mintingErr:    oauth2server.NewRFC6749Error("invalid_client", "client authentication failed", http.StatusUnauthorized, oauth2server.ErrInvalidClient),
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "invalid_client",
		},
		{
			name:       "missing client_secret succeeds for public clients",
			body:       "grant_type=authorization_code&client_id=550e8400-e29b-41d4-a716-446655440000&code=abc",
			wantStatus: http.StatusOK,
		},
		{
			name:          "missing code returns 400 invalid_request",
			body:          "grant_type=authorization_code&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=secret",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "strategy ErrInvalidGrant returns 400 invalid_grant",
			body:          "grant_type=authorization_code&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=secret&code=expired",
			mintingErr:    oauth2server.NewRFC6749Error("invalid_grant", "invalid grant", http.StatusBadRequest, oauth2server.ErrInvalidGrant),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_grant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minting := &mockTokenMintingStrategy{
				authorizationCodeExchangeFn: func(_ context.Context, _ id.ClientID, _, _, _, _ string) (*ports.TokenResponse, error) {
					if tt.mintingErr != nil {
						return nil, tt.mintingErr
					}
					return successResp, nil
				},
			}
			handler := &OAuth2TokenHandler{GrantHandler: NewLocalGrantStrategy(minting, nil), OAuth2Service: newLocalModeOAuth2Service()}
			req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			var body map[string]interface{}
			_ = json.NewDecoder(w.Body).Decode(&body)
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, body["error"])
			} else {
				assert.Equal(t, "tok456", body["access_token"])
				assert.Equal(t, "Bearer", body["token_type"])
				assert.EqualValues(t, 900, body["expires_in"])
			}
		})
	}
}

func TestHandleLocalMinting_RefreshToken(t *testing.T) {
	successResp := &ports.TokenResponse{AccessToken: "tok789", RefreshToken: "refresh789", TokenType: "Bearer", ExpiresIn: 1800, Scope: "read offline_access"}

	tests := []struct {
		name          string
		body          string
		mintingErr    error
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:       "success returns 200 with refresh token body",
			body:       "grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=secret&refresh_token=rt-123&scope=read+offline_access",
			wantStatus: http.StatusOK,
		},
		{
			name:          "missing client_id returns 400 invalid_request",
			body:          "grant_type=refresh_token&refresh_token=rt-123",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "missing refresh_token returns 400 invalid_request",
			body:          "grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000",
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_request",
		},
		{
			name:          "invalid grant surfaces as invalid_grant",
			body:          "grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&refresh_token=stale",
			mintingErr:    oauth2server.NewRFC6749Error("invalid_grant", "invalid grant", http.StatusBadRequest, oauth2server.ErrInvalidGrant),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: "invalid_grant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minting := &mockTokenMintingStrategy{
				refreshTokenFn: func(_ context.Context, _ id.ClientID, _, _, _ string) (*ports.TokenResponse, error) {
					if tt.mintingErr != nil {
						return nil, tt.mintingErr
					}
					return successResp, nil
				},
			}
			handler := &OAuth2TokenHandler{GrantHandler: NewLocalGrantStrategy(minting, nil), OAuth2Service: newLocalModeOAuth2Service()}
			req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			var body map[string]interface{}
			_ = json.NewDecoder(w.Body).Decode(&body)
			if tt.wantErrorCode != "" {
				assert.Equal(t, tt.wantErrorCode, body["error"])
			} else {
				assert.Equal(t, "tok789", body["access_token"])
				assert.Equal(t, "refresh789", body["refresh_token"])
				assert.Equal(t, "Bearer", body["token_type"])
				assert.EqualValues(t, 1800, body["expires_in"])
				assert.Equal(t, "read offline_access", body["scope"])
			}
		})
	}
}

// TestHandleLocalMinting_UnsupportedGrantType verifies the default branch returns
// 400 unsupported_grant_type for any grant type other than client_credentials,
// authorization_code, or refresh_token (e.g. password, implicit, device_code).
func TestHandleLocalMinting_UnsupportedGrantType(t *testing.T) {
	handler := &OAuth2TokenHandler{GrantHandler: NewLocalGrantStrategy(fixedMinting(nil, nil), nil), OAuth2Service: newLocalModeOAuth2Service()}
	req := httptest.NewRequest("POST", "/oauth2/token",
		strings.NewReader("grant_type=password&client_id=550e8400-e29b-41d4-a716-446655440000&username=user&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "unsupported_grant_type", body["error"])
}

// TestHandleMintingError_RFC6749StatusCodes verifies the complete RFC 6749 error code →
// HTTP status mapping in handleMintingError.
func TestHandleMintingError_RFC6749StatusCodes(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantStatus    int
		wantErrorCode string
	}{
		{"ErrInvalidClient → 401 invalid_client", oauth2server.NewRFC6749Error("invalid_client", "client auth failed", http.StatusUnauthorized, oauth2server.ErrInvalidClient), http.StatusUnauthorized, "invalid_client"},
		{"ErrInvalidScope → 400 invalid_scope", oauth2server.NewRFC6749Error("invalid_scope", "invalid scope", http.StatusBadRequest, oauth2server.ErrInvalidScope), http.StatusBadRequest, "invalid_scope"},
		{"ErrInvalidGrant → 400 invalid_grant", oauth2server.NewRFC6749Error("invalid_grant", "invalid grant", http.StatusBadRequest, oauth2server.ErrInvalidGrant), http.StatusBadRequest, "invalid_grant"},
		{"unknown error → 500 server_error", errors.New("unexpected db failure"), http.StatusInternalServerError, "server_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &localGrantStrategy{}
			w := httptest.NewRecorder()

			s.handleMintingError(w, context.Background(), tt.err, "client_credentials", "broker_test")

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			var body map[string]interface{}
			_ = json.NewDecoder(w.Body).Decode(&body)
			assert.Equal(t, tt.wantErrorCode, body["error"])
		})
	}
}

func TestHandleMintingError_OpaqueDescriptions(t *testing.T) {
	internalDetail := "scope \"read:admin\" not allowed for this agent"

	t.Run("invalid_scope does not leak internal detail", func(t *testing.T) {
		s := &localGrantStrategy{}
		w := httptest.NewRecorder()

		s.handleMintingError(w, context.Background(), fmt.Errorf("%s: %w", internalDetail, oauth2server.NewRFC6749Error("invalid_scope", "scope not allowed", http.StatusBadRequest, oauth2server.ErrInvalidScope)), "client_credentials", "broker_test")

		assert.Equal(t, http.StatusBadRequest, w.Code)
		body, _ := io.ReadAll(w.Body)
		assert.Contains(t, string(body), "invalid_scope")
		assert.NotContains(t, string(body), internalDetail)
		assert.NotContains(t, string(body), "read:admin")
	})

	t.Run("invalid_grant does not leak internal detail", func(t *testing.T) {
		s := &localGrantStrategy{}
		w := httptest.NewRecorder()

		s.handleMintingError(w, context.Background(), fmt.Errorf("%s: %w", internalDetail, oauth2server.NewRFC6749Error("invalid_grant", "invalid grant", http.StatusBadRequest, oauth2server.ErrInvalidGrant)), "authorization_code", "broker_test")

		assert.Equal(t, http.StatusBadRequest, w.Code)
		body, _ := io.ReadAll(w.Body)
		assert.Contains(t, string(body), "invalid_grant")
		assert.NotContains(t, string(body), internalDetail)
		assert.NotContains(t, string(body), "read:admin")
	})
}

func TestHandleMintingError_LogDoesNotLeakErrorChain(t *testing.T) {
	internalDetail := "postgres: connection refused to db-host-internal"

	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	s := &localGrantStrategy{logger: logger}
	w := httptest.NewRecorder()

	wrapped := fmt.Errorf("%s: %w", internalDetail, oauth2server.NewRFC6749Error("invalid_client", "client auth failed", http.StatusUnauthorized, oauth2server.ErrInvalidClient))
	s.handleMintingError(w, context.Background(), wrapped, "client_credentials", "broker_test")

	logLine := buf.String()
	assert.NotContains(t, logLine, internalDetail)
	assert.Contains(t, logLine, "invalid_client")
	assert.Contains(t, logLine, "client auth failed")
}

func TestLocalGrantStrategy_MintingFailureLogCarriesRequestContext(t *testing.T) {
	base := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	logger := slog.New(telemetry.NewContextHandler(base))
	strategy := NewLocalGrantStrategy(fixedMinting(nil,
		oauth2server.NewRFC6749Error("invalid_client", "client authentication failed", http.StatusUnauthorized, oauth2server.ErrInvalidClient),
	), logger)
	want := security.SecurityContext{
		TraceID: "0123456789abcdef0123456789abcdef",
		Actor:   "service-account@example.com",
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", nil).WithContext(
		security.WithSecurityContext(context.Background(), want),
	)

	strategy.HandleTokenGrant(httptest.NewRecorder(), req, "client_credentials", url.Values{
		"client_id":     {"broker_test"},
		"client_secret": {"secret"},
	}, nil)

	record, ok := findTokenEndpointLogRecord(*base.records, "TokenRequestFailed")
	require.True(t, ok)
	assert.Equal(t, want.TraceID, record.attrs["trace_id"])
	assert.Equal(t, want.Actor, record.attrs["actor"])
}

func TestLocalGrantStrategy_RefreshTokenLogsCarryRequestContext(t *testing.T) {
	want := security.SecurityContext{
		TraceID:     "0123456789abcdef0123456789abcdef",
		Actor:       "service-account@example.com",
		CallingPeer: "gateway-client-1",
	}
	tests := []struct {
		name     string
		response *ports.TokenResponse
		err      error
		message  string
	}{
		{
			name:     "success",
			response: &ports.TokenResponse{AccessToken: "access-token", TokenType: "Bearer", ExpiresIn: 3600},
			message:  "TokenIssued",
		},
		{
			name:    "failure",
			err:     oauth2server.NewRFC6749Error("invalid_grant", "invalid grant", http.StatusBadRequest, oauth2server.ErrInvalidGrant),
			message: "refresh_token grant failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
			logger := slog.New(telemetry.NewContextHandler(base))
			strategy := NewLocalGrantStrategy(fixedMinting(tt.response, tt.err), logger)
			req := httptest.NewRequest(http.MethodPost, "/oauth2/token", nil).WithContext(
				security.WithSecurityContext(context.Background(), want),
			)

			strategy.HandleTokenGrant(httptest.NewRecorder(), req, "refresh_token", url.Values{
				"client_id":     {"broker_test"},
				"client_secret": {"secret"},
				"refresh_token": {"refresh-token"},
			}, nil)

			record, ok := findTokenEndpointLogRecord(*base.records, tt.message)
			require.True(t, ok)
			assert.Equal(t, want.TraceID, record.attrs["trace_id"])
			assert.Equal(t, want.Actor, record.attrs["actor"])
			assert.Equal(t, want.CallingPeer, record.attrs["calling_peer"])
		})
	}
}

// mockTokenGrantStrategy captures HandleTokenGrant arguments for dispatch assertion.
type mockTokenGrantStrategy struct {
	capturedFormData url.Values
}

func (m *mockTokenGrantStrategy) HandleTokenGrant(w http.ResponseWriter, _ *http.Request, _ string, formData url.Values, _ *ports.TokenGrantResolution) {
	m.capturedFormData = formData
	w.WriteHeader(http.StatusOK)
}

// securityContextRecordingGrantStrategy records the SecurityContext visible to the
// downstream grant handler at dispatch time, proving the /oauth2/token seam
// finalized it before HandleTokenGrant ran.
type securityContextRecordingGrantStrategy struct {
	observedSC security.SecurityContext
	observedOK bool
}

func (s *securityContextRecordingGrantStrategy) HandleTokenGrant(w http.ResponseWriter, r *http.Request, _ string, _ url.Values, _ *ports.TokenGrantResolution) {
	s.observedSC, s.observedOK = security.FromContext(r.Context())
	w.WriteHeader(http.StatusOK)
}

// T048b: localGrantStrategy accepts LocalClient agents (no upstream ClientID, no ClientURIs).
// The minting strategy is reached and returns a token response.
func TestLocalGrantStrategy_AcceptsLocalClient(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000012")

	expected := &ports.TokenResponse{AccessToken: "local-tok", TokenType: "Bearer", ExpiresIn: 3600}
	strategy := NewLocalGrantStrategy(fixedMinting(expected, nil), nil)

	body := strings.NewReader("grant_type=client_credentials&client_id=" + agentID.String() + "&client_secret=secret")
	req := httptest.NewRequest("POST", "/oauth2/token", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	strategy.HandleTokenGrant(w, req, "client_credentials", parseForm(req), &ports.TokenGrantResolution{
		AgentID:    agentID,
		ClientType: storage.LocalClient,
	})

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "local-tok", resp["access_token"])
}

// parseForm parses the form body from a request (re-reads body, for test helpers only).
func parseForm(r *http.Request) url.Values {
	_ = r.ParseForm()
	return r.Form
}

// TestHybridTokenGrant_EmptyClientIDReturns400 ensures missing client_id is rejected with
// 400 invalid_request before any agent lookup, matching RFC 6749 §5.2.
func TestHybridTokenGrant_EmptyClientIDReturns400(t *testing.T) {
	strategy := NewHybridTokenGrantStrategy(
		&mockTokenGrantStrategy{},
		&mockTokenGrantStrategy{},
		nil,
	)
	handler := &OAuth2TokenHandler{GrantHandler: strategy}

	req := httptest.NewRequest("POST", "/oauth2/token",
		strings.NewReader("grant_type=client_credentials&client_secret=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "invalid_request", body["error"])
}

// TestHybridTokenGrantStrategy_DispatchByClientType verifies that hybridTokenGrantStrategy
// routes ProxyClient agents to the proxy sub-strategy and local modes (LocalClient,
// CIMDClient) to the local sub-strategy, while rejecting ambiguous/unknown modes.
func TestHybridTokenGrantStrategy_DispatchByClientType(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000099")
	cases := []struct {
		name         string
		resolution   *ports.TokenGrantResolution
		expectsProxy bool
		expectsError bool
	}{
		{
			name:         "ProxyClient routes to proxy sub-strategy",
			resolution:   &ports.TokenGrantResolution{AgentID: agentID, ClientID: ptr.To(id.ClientID("upstream-client")), ClientType: storage.ProxyClient},
			expectsProxy: true,
		},
		{
			name:       "LocalClient routes to local sub-strategy",
			resolution: &ports.TokenGrantResolution{AgentID: agentID, ClientType: storage.LocalClient},
		},
		{
			name:       "CIMDClient routes to local sub-strategy",
			resolution: &ports.TokenGrantResolution{AgentID: agentID, ClientType: storage.CIMDClient},
		},
		{
			name:         "AmbiguousClient returns server_error",
			resolution:   &ports.TokenGrantResolution{AgentID: agentID, ClientType: storage.AmbiguousClient},
			expectsError: true,
		},
		{
			name:         "UnknownClient returns server_error",
			resolution:   &ports.TokenGrantResolution{AgentID: agentID, ClientType: storage.UnknownClient},
			expectsError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxyMock := &mockTokenGrantStrategy{}
			localMock := &mockTokenGrantStrategy{}
			strategy := NewHybridTokenGrantStrategy(proxyMock, localMock, nil)

			req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader("grant_type=client_credentials"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			strategy.HandleTokenGrant(w, req, "client_credentials", url.Values{"grant_type": {"client_credentials"}}, tc.resolution)

			if tc.expectsError {
				assert.Equal(t, http.StatusInternalServerError, w.Code)
				assert.Nil(t, proxyMock.capturedFormData, "proxy sub-strategy must not be called")
				assert.Nil(t, localMock.capturedFormData, "local sub-strategy must not be called")
				return
			}
			assert.Equal(t, tc.expectsProxy, proxyMock.capturedFormData != nil, "proxy sub-strategy called")
			assert.Equal(t, !tc.expectsProxy, localMock.capturedFormData != nil, "local sub-strategy called")
		})
	}
}

// TestHandleTokenExchange_NilService verifies that a token-exchange request returns
// 400 unsupported_grant_type (not 500 server_error) when TokenExchangeService is nil.
// This covers the local-mode deployment where token exchange is not wired.
func TestHandleTokenExchange_NilService(t *testing.T) {
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	handler := &OAuth2TokenHandler{
		GrantHandler:  NewLocalGrantStrategy(fixedMinting(nil, nil), nil),
		OAuth2Service: newLocalModeOAuth2Service(),
		TokenExchange: nil,
		Logger:        logger,
	}
	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange" +
		"&subject_token=sometoken&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token"
	req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "unsupported_grant_type", body["error"])
	assert.Contains(t, logs.String(), "WARN", "nil TokenExchange must log at Warn level")
}

// TestOAuth2TokenHandler_UnauthorizedClient_Returns400 verifies that an unauthorized_client
// error code maps to HTTP 400, not 401 (RFC 6749 §5.2 + tokenEndpointStatus mapping).
func TestOAuth2TokenHandler_UnauthorizedClient_Returns400(t *testing.T) {
	handler := &OAuth2TokenHandler{
		GrantHandler: NewLocalGrantStrategy(fixedMinting(nil, nil), nil),
		OAuth2Service: newFailingOAuth2Service(
			&ports.ClientIDError{Code: "unauthorized_client", Desc: "client mode not permitted"},
		),
	}
	form := "grant_type=client_credentials&client_id=" + id.NewAgentID().String()
	req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "unauthorized_client must return 400, not 401")
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "unauthorized_client", body["error"])
}

// TestOAuth2TokenHandler_NilOAuth2Service_Returns500 verifies that when OAuth2Service
// is nil (misconfigured deployment), the token endpoint returns 500 (server_error) and
// NOT 503 (ServiceUnavailable). RFC 6749 §5.2 constrains the token endpoint to
// 400 / 401 / 500 status codes.
func TestOAuth2TokenHandler_NilOAuth2Service_Returns500(t *testing.T) {
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewLocalGrantStrategy(fixedMinting(nil, nil), nil),
		OAuth2Service: nil,
	}
	form := "grant_type=client_credentials&client_id=" + id.NewAgentID().String()
	req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "nil OAuth2Service must yield 500, not 503")
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "server_error", body["error"])
}

// TestOAuth2TokenHandler_MissingGrantType verifies that an absent grant_type returns
// 400 invalid_request without performing client resolution (RFC 6749 §5.2).
func TestOAuth2TokenHandler_MissingGrantType(t *testing.T) {
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewLocalGrantStrategy(fixedMinting(nil, nil), nil),
		OAuth2Service: newLocalModeOAuth2Service(),
	}
	form := "client_id=" + id.NewAgentID().String()
	req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "invalid_request", body["error"])
}

// TestNewHybridTokenGrantStrategy_PanicsOnNilSubStrategies verifies that construction
// panics when either sub-strategy is nil, matching NewHybridProceedStrategy's behavior.
func TestNewHybridTokenGrantStrategy_PanicsOnNilSubStrategies(t *testing.T) {
	dummy := &mockTokenGrantStrategy{}

	t.Run("nil proxy panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewHybridTokenGrantStrategy(nil, dummy, nil)
		})
	})

	t.Run("nil local panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewHybridTokenGrantStrategy(dummy, nil, nil)
		})
	})

	t.Run("both non-nil does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			NewHybridTokenGrantStrategy(dummy, dummy, nil)
		})
	})
}

// TestHybridTokenGrantStrategy_DefaultBranchLogsError verifies that reaching the default
// (unknown/ambiguous client type) branch emits an Error-level log with agent_id.
func TestHybridTokenGrantStrategy_DefaultBranchLogsError(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000077")

	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	strategy := NewHybridTokenGrantStrategy(&mockTokenGrantStrategy{}, &mockTokenGrantStrategy{}, logger)

	req := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader("grant_type=client_credentials"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	resolution := &ports.TokenGrantResolution{
		AgentID:    agentID,
		ClientType: storage.AmbiguousClient,
	}
	strategy.HandleTokenGrant(w, req, "client_credentials", url.Values{}, resolution)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	logLine := buf.String()
	assert.Contains(t, logLine, "ERROR")
	assert.Contains(t, logLine, agentID.String())
}

// TestOAuth2TokenHandler_BodyClosedOnReadError verifies that r.Body is closed even
// when io.ReadAll returns an error (i.e. the defer fires on all exit paths).
func TestOAuth2TokenHandler_BodyClosedOnReadError(t *testing.T) {
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewLocalGrantStrategy(fixedMinting(nil, nil), nil),
		OAuth2Service: newLocalModeOAuth2Service(),
	}

	closed := false
	body := &failingReadCloser{
		err:     errors.New("simulated read failure"),
		onClose: func() { closed = true },
	}

	req := httptest.NewRequest("POST", "/oauth2/token", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = body // override the body with our tracking reader
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.True(t, closed, "r.Body must be closed even when ReadAll fails")
}

// failingReadCloser is a test double that fails on Read and tracks Close calls.
type failingReadCloser struct {
	err     error
	onClose func()
}

func (f *failingReadCloser) Read([]byte) (int, error) { return 0, f.err }
func (f *failingReadCloser) Close() error {
	if f.onClose != nil {
		f.onClose()
	}
	return nil
}

// TestHandleTokenExchangeError_WrappedError verifies that handleTokenExchangeError
// correctly handles wrapped TokenExchangeErrors using errors.As rather than a direct
// type assertion. A wrapped error must return the domain-specified HTTP status and
// error code, not a 500 server_error.
func TestHandleTokenExchangeError_WrappedError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "direct TokenExchangeError returns correct status",
			err:        tokenexchange.NewInvalidRequestError("bad token"),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name:       "wrapped TokenExchangeError returns correct status (not 500)",
			err:        fmt.Errorf("context: %w", tokenexchange.NewInvalidRequestError("bad token")),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name:       "wrapped invalid_client returns 401",
			err:        fmt.Errorf("context: %w", tokenexchange.NewInvalidClientError("client rejected")),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_client",
		},
		{
			name:       "non-TokenExchangeError returns 500",
			err:        errors.New("unexpected failure"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "server_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &OAuth2TokenHandler{}
			w := httptest.NewRecorder()
			h.handleTokenExchangeError(w, tt.err)

			assert.Equal(t, tt.wantStatus, w.Code)
			var body map[string]string
			_ = json.NewDecoder(w.Body).Decode(&body)
			assert.Equal(t, tt.wantCode, body["error"])
		})
	}
}

// TestHandleTokenExchangeError_UnrecognizedErrorLogged verifies that when a non-TokenExchangeError
// reaches handleTokenExchangeError, an Error-level log is emitted so it isn't silently swallowed.
func TestHandleTokenExchangeError_UnrecognizedErrorLogged(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	h := &OAuth2TokenHandler{Logger: logger}
	w := httptest.NewRecorder()
	h.handleTokenExchangeError(w, errors.New("unexpected db failure"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	logLine := buf.String()
	assert.Contains(t, logLine, "ERROR", "unexpected error must be logged at Error level")
}

// TestHandleTokenExchange_WrappedTokenExchangeErrorMapsCorrectly verifies that
// handleTokenExchangeError maps wrapped TokenExchangeErrors to the correct HTTP status.
func TestHandleTokenExchange_WrappedTokenExchangeErrorMapsCorrectly(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	wrapped := fmt.Errorf("context: %w",
		tokenexchange.NewInvalidRequestErrorWithDetails("bad token", "details about the failure"))

	h := &OAuth2TokenHandler{Logger: logger}
	w := httptest.NewRecorder()
	h.handleTokenExchangeError(w, wrapped)

	assert.Equal(t, http.StatusBadRequest, w.Code, "wrapped error must map to 400, not 500")
}

// TestTokenEndpointStatus_RFC6749Mapping verifies the HTTP status code mapping for
// OAuth2 error codes on the token endpoint per RFC 6749 §5.2.
// invalid_client → 401, server_error → 500, all others (including unknown codes) → 400.
func TestTokenEndpointStatus_RFC6749Mapping(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{"invalid_client", http.StatusUnauthorized},
		{"server_error", http.StatusInternalServerError},
		{"invalid_request", http.StatusBadRequest},
		{"invalid_grant", http.StatusBadRequest},
		{"unauthorized_client", http.StatusBadRequest},
		{"unsupported_grant_type", http.StatusBadRequest},
		{"invalid_scope", http.StatusBadRequest},
		{"some_future_code", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := tokenEndpointStatus(tt.code)
			assert.Equal(t, tt.want, got, "tokenEndpointStatus(%q)", tt.code)
		})
	}
}

type tokenEndpointLogRecord struct {
	level   slog.Level
	message string
	attrs   map[string]any
}

type tokenEndpointLogCaptureHandler struct {
	minLevel slog.Level
	records  *[]tokenEndpointLogRecord
	attrs    []slog.Attr
	groups   []string
}

func newTokenEndpointLogCaptureHandler(minLevel slog.Level) *tokenEndpointLogCaptureHandler {
	records := make([]tokenEndpointLogRecord, 0, 4)
	return &tokenEndpointLogCaptureHandler{minLevel: minLevel, records: &records}
}

func (h *tokenEndpointLogCaptureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *tokenEndpointLogCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any, record.NumAttrs()+len(h.attrs))
	for _, attr := range h.attrs {
		h.storeAttr(attrs, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		h.storeAttr(attrs, attr)
		return true
	})
	*h.records = append(*h.records, tokenEndpointLogRecord{
		level:   record.Level,
		message: record.Message,
		attrs:   attrs,
	})
	return nil
}

func (h *tokenEndpointLogCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *tokenEndpointLogCaptureHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *tokenEndpointLogCaptureHandler) storeAttr(dst map[string]any, attr slog.Attr) {
	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string{}, h.groups...), attr.Key), ".")
	}
	dst[key] = attr.Value.Any()
}

func findTokenEndpointLogRecord(records []tokenEndpointLogRecord, message string) (tokenEndpointLogRecord, bool) {
	for _, record := range records {
		if record.message == message {
			return record, true
		}
	}
	return tokenEndpointLogRecord{}, false
}

type oauth2TokenJWKSProvider struct {
	keySet jwk.Set
}

func (p *oauth2TokenJWKSProvider) GetKeySet(context.Context) (jwk.Set, error) {
	return p.keySet, nil
}

func (p *oauth2TokenJWKSProvider) GetKey(_ context.Context, kid string) (jwk.Key, error) {
	key, ok := p.keySet.LookupKeyID(kid)
	if !ok {
		return nil, errors.New("key not found")
	}
	return key, nil
}

type oauth2TokenPassthroughEncryption struct{}

func (e *oauth2TokenPassthroughEncryption) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (e *oauth2TokenPassthroughEncryption) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

type oauth2TokenNoopBranchKeyManager struct{}

func (m *oauth2TokenNoopBranchKeyManager) Create(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

type oauth2TokenProviderRepo struct {
	seenSC security.SecurityContext
	seenOK bool
}

func (r *oauth2TokenProviderRepo) Create(context.Context, *model.ThirdpartyOAuth2ProviderEntity) error {
	return nil
}

func (r *oauth2TokenProviderRepo) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, ports.ErrNotFound
}

func (r *oauth2TokenProviderRepo) Update(context.Context, *model.ThirdpartyOAuth2ProviderEntity, *int64) error {
	return nil
}

func (r *oauth2TokenProviderRepo) Delete(context.Context, id.ServiceID) error {
	return nil
}

func (r *oauth2TokenProviderRepo) List(context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, nil
}

func (r *oauth2TokenProviderRepo) FindByProtectedResource(ctx context.Context, _ string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.seenSC, r.seenOK = security.FromContext(ctx)
	return nil, tokenexchange.NewInvalidTargetError("no service configured for the requested resource")
}

func (*oauth2TokenProviderRepo) AddProtectedResource(context.Context, id.ServiceID, string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, nil
}

func (*oauth2TokenProviderRepo) RemoveProtectedResource(context.Context, id.ServiceID, string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, nil
}

func (*oauth2TokenProviderRepo) RenameProtectedResource(context.Context, id.ServiceID, string, string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, nil
}

func (*oauth2TokenProviderRepo) ListProtectedResources(context.Context, id.ServiceID) ([]string, int64, error) {
	return nil, 0, nil
}

func generateOAuth2TokenExchangeKeySet(t *testing.T) (*rsa.PrivateKey, jwk.Set) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	publicKey, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, publicKey.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, publicKey.Set(jwk.AlgorithmKey, jwa.RS256()))

	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(publicKey))
	return privateKey, keySet
}

func signOAuth2TokenExchangeJWT(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()

	tok := jwt.New()
	for key, value := range claims {
		require.NoError(t, tok.Set(key, value))
	}

	privateJWK, err := jwk.Import[jwk.Key](privateKey)
	require.NoError(t, err)
	require.NoError(t, privateJWK.Set(jwk.KeyIDKey, "test-key"))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), privateJWK))
	require.NoError(t, err)
	return string(signed)
}

func newTokenExchangeServiceForContextPropagationTest(t *testing.T, keySet jwk.Set, repo ports.ThirdpartyOAuth2ProviderRepository) *tokenexchange.TokenExchangeService {
	t.Helper()

	validator, err := tokenexchange.NewJWTValidator(
		&oauth2TokenJWKSProvider{keySet: keySet},
		"https://auth.example.com",
		"agentic-identity-broker",
		60,
	)
	require.NoError(t, err)

	celEvaluator, err := tokenexchange.NewCELEvaluator(tokenexchange.CELEvaluatorConfig{
		PrincipalExpression:     "subject_token.sub",
		AgentIDExpression:       "subject_token.azp",
		AuthorizationExpression: "true",
		EvaluationTimeout:       100 * time.Millisecond,
	})
	require.NoError(t, err)

	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		repo,
		&oauth2TokenPassthroughEncryption{},
		&oauth2TokenNoopBranchKeyManager{},
		nil,
		false,
		nil,
	)
	permissionSetService := permissionset.NewPermissionSetService(nil, nil, slog.Default())
	t.Cleanup(permissionSetService.Close)

	service, err := tokenexchange.NewTokenExchangeService(
		validator,
		celEvaluator,
		providerService,
		&oauth2session.OAuth2SessionService{},
		&consent.Service{},
		permissionSetService,
		newStubAgentRepo(id.NewAgentID(), "upstream-client-id"),
		&ports.TokenExchangeConfig{
			ClaimExtraction: ports.ClaimExtractionConfig{
				PrincipalExpression: "subject_token.sub",
				AgentIDExpression:   "subject_token.azp",
			},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: "true"},
			},
		},
	)
	require.NoError(t, err)
	return service
}

func TestOAuth2TokenHandler_TokenExchangeErrorLogCarriesFinalSecurityContext(t *testing.T) {
	t.Parallel()

	privateKey, keySet := generateOAuth2TokenExchangeKeySet(t)
	providerRepo := &oauth2TokenProviderRepo{}
	logCapture := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	logger := slog.New(telemetry.NewContextHandler(logCapture))
	handler := &OAuth2TokenHandler{
		TokenExchange: newTokenExchangeServiceForContextPropagationTest(t, keySet, providerRepo),
		Logger:        logger,
	}

	want := security.SecurityContext{
		TraceID:     "0123456789abcdef0123456789abcdef",
		Actor:       "user@example.com",
		CallingPeer: "privileged-client-1",
	}
	now := time.Now()
	subjectToken := signOAuth2TokenExchangeJWT(t, privateKey, map[string]any{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": want.Actor,
		"azp": id.NewAgentID().String(),
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	})
	clientAssertion := signOAuth2TokenExchangeJWT(t, privateKey, map[string]any{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": want.CallingPeer,
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	})

	form := url.Values{
		"grant_type":            {tokenexchange.TokenExchangeGrantType},
		"subject_token":         {subjectToken},
		"subject_token_type":    {tokenexchange.AccessTokenType},
		"client_assertion":      {clientAssertion},
		"client_assertion_type": {tokenexchange.JWTBearerType},
		"resource":              {"https://api.example.com/resource"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(security.WithSecurityContext(req.Context(), want))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "invalid_target", body["error"])

	require.True(t, providerRepo.seenOK, "handler must preserve the final SecurityContext into downstream token-exchange service calls")
	assert.Equal(t, want.TraceID, providerRepo.seenSC.TraceID)
	assert.Equal(t, want.Actor, providerRepo.seenSC.Actor)
	assert.Equal(t, want.CallingPeer, providerRepo.seenSC.CallingPeer)

	record, ok := findTokenEndpointLogRecord(*logCapture.records, "Token exchange failed")
	require.True(t, ok, "token-exchange failures must be logged")
	assert.Equal(t, want.TraceID, record.attrs["trace_id"], "handler logs must remain bound to the request trace")
	assert.Equal(t, want.Actor, record.attrs["actor"], "handler logs must include the finalized actor")
	assert.Equal(t, want.CallingPeer, record.attrs["calling_peer"], "handler logs must include the finalized calling_peer")
}

// TestOAuth2TokenHandler_FinalizesSecurityContextBeforeGrantHandler is a regression
// guard for architecture-review Finding 4: on POST /oauth2/token with a non-RFC8693
// grant, the request SecurityContext MUST be finalized at the handler seam before
// control reaches the downstream grant handler. That grant handler emits
// context-aware TokenIssued audit logs (slog InfoContext) that must carry actor and
// trace_id, and the LoggingMiddleware perimeter safety net only finalizes AFTER
// ServeHTTP returns — too late for in-handler logs. SecurityContextMiddleware defers
// finalization for every POST /oauth2/token, so the handler must finalize the still-open
// deferred capture holder itself before dispatch.
func TestOAuth2TokenHandler_FinalizesSecurityContextBeforeGrantHandler(t *testing.T) {
	t.Parallel()

	stub := &securityContextRecordingGrantStrategy{}
	handler := &OAuth2TokenHandler{
		OAuth2Service: newLocalModeOAuth2Service(),
		GrantHandler:  stub,
	}

	holder := security.NewCaptureHolder(security.TransportCapture{
		TraceID:       "0123456789abcdef0123456789abcdef",
		ClientIP:      "203.0.113.9",
		UserAgent:     "cc-client/1.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    time.Date(2026, time.July, 4, 12, 0, 0, 0, time.UTC),
	})
	ctx := principal.WithPrincipal(security.WithCaptureHolder(context.Background(), holder), "svc-account@example.com")

	form := "grant_type=client_credentials&client_id=" + id.NewAgentID().String() + "&client_secret=secret"
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(ctx)

	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.True(t, stub.observedOK, "grant handler must observe a finalized SecurityContext — non-exchange grants finalize at the /oauth2/token seam before dispatch so in-handler TokenIssued audit logs carry actor/trace_id")
	assert.Equal(t, "svc-account@example.com", stub.observedSC.Actor)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", stub.observedSC.TraceID)
	assert.Empty(t, stub.observedSC.CallingPeer, "non-delegated grants carry no calling peer")

	sc, ok := holder.Finalized()
	require.True(t, ok)
	assert.Equal(t, "svc-account@example.com", sc.Actor)
}

func TestSanitizeResourceURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain uri unchanged", "https://api.example.com/resource", "https://api.example.com/resource"},
		{"query stripped", "https://api.example.com/resource?access_token=SECRET", "https://api.example.com/resource"},
		{"fragment stripped", "https://api.example.com/resource#SECRET", "https://api.example.com/resource"},
		{"query and fragment stripped", "https://api.example.com/r?token=SECRET#frag", "https://api.example.com/r"},
		{"unparseable with query redacted", "not a uri?token=SECRET", "[invalid resource URI]"},
		{"unparseable with fragment redacted", "not a uri#SECRET", "[invalid resource URI]"},
		{"userinfo stripped", "https://user:secret@example.com/resource", "https://example.com/resource"},
		{"unparseable userinfo redacted", "https://user:secret@example.com/%zz", "[invalid resource URI]"},
		{"empty redacted", "", "[invalid resource URI]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeResourceURI(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "SECRET", "sanitized resource must never retain query/fragment secrets")
			assert.NotContains(t, got, "user:secret", "sanitized resource must never retain URI credentials")
		})
	}
}

// TestOAuth2TokenHandler_TokenExchangeResourceSanitizedInLogsAndSpan guards the
// secret-scrubbing contract: the RFC 8693 resource is caller-controlled and its
// query string / fragment can carry tokens. The handler MUST NOT emit the raw
// resource into span attributes or structured logs.
func TestOAuth2TokenHandler_TokenExchangeResourceSanitizedInLogsAndSpan(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	privateKey, keySet := generateOAuth2TokenExchangeKeySet(t)
	providerRepo := &oauth2TokenProviderRepo{}
	logCapture := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	logger := slog.New(telemetry.NewContextHandler(logCapture))
	handler := &OAuth2TokenHandler{
		TokenExchange: newTokenExchangeServiceForContextPropagationTest(t, keySet, providerRepo),
		Logger:        logger,
	}

	const rawResource = "https://api.example.com/resource?access_token=SUPERSECRET#frag"
	const sanitized = "https://api.example.com/resource"
	now := time.Now()
	subjectToken := signOAuth2TokenExchangeJWT(t, privateKey, map[string]any{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "user@example.com",
		"azp": id.NewAgentID().String(),
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	})
	clientAssertion := signOAuth2TokenExchangeJWT(t, privateKey, map[string]any{
		"iss": "https://auth.example.com",
		"aud": "agentic-identity-broker",
		"sub": "privileged-client-1",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	})

	form := url.Values{
		"grant_type":            {tokenexchange.TokenExchangeGrantType},
		"subject_token":         {subjectToken},
		"subject_token_type":    {tokenexchange.AccessTokenType},
		"client_assertion":      {clientAssertion},
		"client_assertion_type": {tokenexchange.JWTBearerType},
		"resource":              {rawResource},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	record, ok := findTokenEndpointLogRecord(*logCapture.records, "Token exchange failed")
	require.True(t, ok, "token-exchange failures must be logged")
	assert.Equal(t, sanitized, record.attrs["resource"], "error log resource must be sanitized")
	assert.NotContains(t, fmt.Sprintf("%v", record.attrs["resource"]), "SUPERSECRET")

	spans := spanRecorder.Ended()
	var resourceAttr string
	var sawSpan bool
	for _, s := range spans {
		if s.Name() != "tokenexchange.exchange" {
			continue
		}
		sawSpan = true
		for _, kv := range s.Attributes() {
			if string(kv.Key) == "token_exchange.resource" {
				resourceAttr = kv.Value.AsString()
			}
		}
	}
	require.True(t, sawSpan, "tokenexchange.exchange span must be recorded")
	assert.Equal(t, sanitized, resourceAttr, "span resource attribute must be sanitized")
	assert.NotContains(t, resourceAttr, "SUPERSECRET")
}
