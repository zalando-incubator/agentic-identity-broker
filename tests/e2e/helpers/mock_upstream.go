// Package helpers provides test utilities for E2E testing.
package helpers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"
)

// sharedRSAKeyPair holds the single RSA key pair generated once per test process.
// Reusing it avoids ~2-5ms key-generation overhead per MockUpstreamOAuth2Server instantiation.
// This is safe because the key is used only for signature verification, not for isolation.
var sharedRSAKeyPair struct {
	sync.Once
	privateKeyPEM string
	publicKeyPEM  string
	jwksSet       map[string]interface{}
}

func sharedKeyPair() (string, string, map[string]interface{}) {
	sharedRSAKeyPair.Do(func() {
		privateKeyPEM, publicKeyPEM, err := GenerateTestRSAKeyPair()
		if err != nil {
			panic(fmt.Sprintf("failed to generate shared test RSA key pair: %v", err))
		}
		jwksSet, err := GenerateJWKSFromPublicKey(publicKeyPEM)
		if err != nil {
			panic(fmt.Sprintf("failed to generate JWKS from shared public key: %v", err))
		}
		sharedRSAKeyPair.privateKeyPEM = privateKeyPEM
		sharedRSAKeyPair.publicKeyPEM = publicKeyPEM
		sharedRSAKeyPair.jwksSet = jwksSet
	})
	return sharedRSAKeyPair.privateKeyPEM, sharedRSAKeyPair.publicKeyPEM, sharedRSAKeyPair.jwksSet
}

type pkceChallenge struct {
	value  string
	method string
}

// CapturedTokenRequest is an immutable snapshot of a token request received by the mock upstream.
type CapturedTokenRequest struct {
	Header http.Header
	Body   string
}

// MockUpstreamOAuth2Server provides a mock upstream OAuth2 server using httptest.Server.
// It captures requests for assertion and supports both successful and error responses.
// This is stable because it only depends on HTTP contract, not internal implementation.
type MockUpstreamOAuth2Server struct {
	Server        *httptest.Server
	LastRequest   *http.Request
	LastBody      string
	requestMutex  sync.RWMutex
	tokenRequests []CapturedTokenRequest

	// Response configuration
	authorizeCalled         bool
	tokenCalled             bool
	metadataCalled          bool
	jwksCalled              bool
	successfulTokenResp     bool
	errorCode               string
	errorDescription        string
	responseDelay           time.Duration
	blockTokenUntilCanceled bool
	accessToken             string
	refreshToken            string
	tokenType               string
	expiresIn               int
	strictPublicClientMode  bool
	codeChallengesByCode    map[string]pkceChallenge

	// RSA key pair for JWT signing (generated on init)
	privateKeyPEM string
	publicKeyPEM  string
	jwksSet       map[string]interface{}
}

// NewMockUpstreamOAuth2Server creates a new mock upstream OAuth2 server.
// The server handles /oauth/authorize, /oauth/token, /.well-known/openid-configuration,
// and /.well-known/jwks.json endpoints.
// Generates RSA key pair on initialization for JWT signing in tests.
func NewMockUpstreamOAuth2Server() *MockUpstreamOAuth2Server {
	m := &MockUpstreamOAuth2Server{
		accessToken:  "mock-access-token",
		refreshToken: "mock-refresh-token",
		tokenType:    "Bearer",
		expiresIn:    3600,
	}

	// Reuse the package-level RSA key pair (generated once per test process).
	m.privateKeyPEM, m.publicKeyPEM, m.jwksSet = sharedKeyPair()

	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", m.handleAuthorize)
	mux.HandleFunc("/oauth/authorize", m.handleAuthorize)
	mux.HandleFunc("/token", m.handleToken)
	mux.HandleFunc("/oauth/token", m.handleToken)
	mux.HandleFunc("/.well-known/openid-configuration", m.handleMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", m.handleMetadata)
	mux.HandleFunc("/.well-known/jwks.json", m.handleJWKS)

	m.Server = httptest.NewServer(mux)
	return m
}

// Close shuts down the mock server.
func (m *MockUpstreamOAuth2Server) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

// URL returns the base URL of the mock server.
func (m *MockUpstreamOAuth2Server) URL() string {
	if m.Server != nil {
		return m.Server.URL
	}
	return ""
}

// WithSuccessfulTokenResponse configures the server to return a successful token response.
func (m *MockUpstreamOAuth2Server) WithSuccessfulTokenResponse() *MockUpstreamOAuth2Server {
	m.successfulTokenResp = true
	m.errorCode = ""
	m.errorDescription = ""
	return m
}

// WithErrorResponse configures the server to return an error response.
func (m *MockUpstreamOAuth2Server) WithErrorResponse(errorCode string) *MockUpstreamOAuth2Server {
	m.successfulTokenResp = false
	m.errorCode = errorCode
	m.errorDescription = fmt.Sprintf("Error from upstream: %s", errorCode)
	return m
}

// WithErrorResponseAndDescription configures error response with custom description.
func (m *MockUpstreamOAuth2Server) WithErrorResponseAndDescription(errorCode, description string) *MockUpstreamOAuth2Server {
	m.successfulTokenResp = false
	m.errorCode = errorCode
	m.errorDescription = description
	return m
}

// WithAccessToken sets the access token in successful responses.
func (m *MockUpstreamOAuth2Server) WithAccessToken(token string) *MockUpstreamOAuth2Server {
	m.accessToken = token
	return m
}

// WithRefreshToken sets the refresh token in successful responses.
func (m *MockUpstreamOAuth2Server) WithRefreshToken(token string) *MockUpstreamOAuth2Server {
	m.refreshToken = token
	return m
}

// WithResponseDelay adds a delay to token endpoint responses (simulates network latency).
func (m *MockUpstreamOAuth2Server) WithResponseDelay(delay time.Duration) *MockUpstreamOAuth2Server {
	m.responseDelay = delay
	return m
}

// WithTokenHangUntilCanceled makes the token endpoint block until the caller's
// request context is canceled. This is useful for provoking client-side
// timeouts without sleeping longer than necessary in tests.
func (m *MockUpstreamOAuth2Server) WithTokenHangUntilCanceled() *MockUpstreamOAuth2Server {
	m.blockTokenUntilCanceled = true
	m.responseDelay = 0
	return m
}

// WithStrictPublicClientMode makes the token endpoint reject client credentials
// and require a verifier matching an authorization request's PKCE challenge.
func (m *MockUpstreamOAuth2Server) WithStrictPublicClientMode() *MockUpstreamOAuth2Server {
	m.requestMutex.Lock()
	defer m.requestMutex.Unlock()

	m.strictPublicClientMode = true
	m.codeChallengesByCode = make(map[string]pkceChallenge)
	return m
}

// WithExpiresIn sets the token expiration time in seconds.
func (m *MockUpstreamOAuth2Server) WithExpiresIn(seconds int) *MockUpstreamOAuth2Server {
	m.expiresIn = seconds
	return m
}

// ReturnTokenWithClaim configures the server to return a signed JWT access token
// that includes the specified claim name and value.
// The JWT is signed with the server's RSA private key (same key as the JWKS endpoint).
// This is used for multi-agent scenarios where the upstream embeds the agent ID as a claim.
func (m *MockUpstreamOAuth2Server) ReturnTokenWithClaim(claimName, value string) *MockUpstreamOAuth2Server {
	claims := NewJWTClaims().
		WithClaim(claimName, value).
		Build()

	signed, err := SignTestJWT(claims, m.privateKeyPEM)
	if err != nil {
		panic(fmt.Sprintf("ReturnTokenWithClaim: failed to sign JWT with claim %q: %v", claimName, err))
	}

	m.successfulTokenResp = true
	m.accessToken = signed
	return m
}

// GetLastRequest returns the last captured request (thread-safe).
func (m *MockUpstreamOAuth2Server) GetLastRequest() *http.Request {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.LastRequest
}

// GetLastBody returns the last captured request body (thread-safe).
func (m *MockUpstreamOAuth2Server) GetLastBody() string {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.LastBody
}

// GetTokenRequests returns the token requests received by the mock upstream in order.
func (m *MockUpstreamOAuth2Server) GetTokenRequests() []CapturedTokenRequest {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()

	requests := make([]CapturedTokenRequest, len(m.tokenRequests))
	for i, request := range m.tokenRequests {
		requests[i] = CapturedTokenRequest{
			Header: request.Header.Clone(),
			Body:   request.Body,
		}
	}
	return requests
}

// GetAuthorizeCalled returns whether authorize endpoint was called.
func (m *MockUpstreamOAuth2Server) GetAuthorizeCalled() bool {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.authorizeCalled
}

// GetTokenCalled returns whether token endpoint was called.
func (m *MockUpstreamOAuth2Server) GetTokenCalled() bool {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.tokenCalled
}

// GetMetadataCalled returns whether metadata endpoint was called.
func (m *MockUpstreamOAuth2Server) GetMetadataCalled() bool {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.metadataCalled
}

// GetJWKSCalled returns whether JWKS endpoint was called.
func (m *MockUpstreamOAuth2Server) GetJWKSCalled() bool {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.jwksCalled
}

// GetPrivateKeyPEM returns the private key in PEM format for test JWT signing.
func (m *MockUpstreamOAuth2Server) GetPrivateKeyPEM() string {
	return m.privateKeyPEM
}

// GetPublicKeyPEM returns the public key in PEM format for test verification.
func (m *MockUpstreamOAuth2Server) GetPublicKeyPEM() string {
	return m.publicKeyPEM
}

// Reset clears captured request state for reuse in tests.
func (m *MockUpstreamOAuth2Server) Reset() {
	m.requestMutex.Lock()
	defer m.requestMutex.Unlock()

	m.LastRequest = nil
	m.LastBody = ""
	m.tokenRequests = nil
	m.authorizeCalled = false
	m.tokenCalled = false
	m.metadataCalled = false
	m.jwksCalled = false
	clear(m.codeChallengesByCode)
}

// handleAuthorize handles the /oauth/authorize endpoint.
// Simulates upstream OAuth2 authorization endpoint behavior.
func (m *MockUpstreamOAuth2Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	m.requestMutex.Lock()
	m.LastRequest = r
	m.authorizeCalled = true
	// Read body if present
	if r.Body != nil {
		defer func() { _ = r.Body.Close() }()
		bodyBytes, _ := readRequestBody(r)
		m.LastBody = string(bodyBytes)
	}
	m.requestMutex.Unlock()

	// Parse query parameters
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Extract authorization code request parameters
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")

	// Validate parameters
	if clientID == "" || redirectURI == "" || state == "" {
		// Return error back to redirect URI
		params := url.Values{}
		params.Set("error", "invalid_request")
		params.Set("error_description", "Missing required parameters")
		if state != "" {
			params.Set("state", state)
		}

		w.Header().Set("Location", redirectURI+"?"+params.Encode())
		w.WriteHeader(http.StatusFound)
		return
	}

	authorizationCode := "mock-auth-code-123"
	m.requestMutex.Lock()
	if m.strictPublicClientMode {
		authorizationCode = fmt.Sprintf("mock-auth-code-%d", len(m.codeChallengesByCode)+1)
		m.codeChallengesByCode[authorizationCode] = pkceChallenge{
			value:  r.FormValue("code_challenge"),
			method: r.FormValue("code_challenge_method"),
		}
	}
	m.requestMutex.Unlock()

	// Return authorization code
	params := url.Values{}
	params.Set("code", authorizationCode)
	params.Set("state", state)

	w.Header().Set("Location", redirectURI+"?"+params.Encode())
	w.WriteHeader(http.StatusFound)
}

// handleToken handles the /oauth/token endpoint.
// Simulates upstream OAuth2 token exchange endpoint.
func (m *MockUpstreamOAuth2Server) handleToken(w http.ResponseWriter, r *http.Request) {
	// Parse form data so it's available for testing via FormValue
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	body := r.Form.Encode()
	m.requestMutex.Lock()
	m.LastRequest = r
	m.LastBody = body
	m.tokenRequests = append(m.tokenRequests, CapturedTokenRequest{
		Header: r.Header.Clone(),
		Body:   body,
	})
	m.tokenCalled = true
	m.requestMutex.Unlock()

	if status, errorCode := m.strictPublicClientTokenError(r); errorCode != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": errorCode})
		return
	}

	if m.blockTokenUntilCanceled {
		<-r.Context().Done()
		return
	}

	// Apply response delay if configured
	if m.responseDelay > 0 {
		timer := time.NewTimer(m.responseDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}

	// Check for error condition
	if !m.successfulTokenResp && m.errorCode != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		errResp := map[string]string{
			"error":             m.errorCode,
			"error_description": m.errorDescription,
		}
		_ = json.NewEncoder(w).Encode(errResp)
		return
	}

	// Return successful token response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	tokenResp := map[string]interface{}{
		"access_token":  m.accessToken,
		"token_type":    m.tokenType,
		"expires_in":    m.expiresIn,
		"refresh_token": m.refreshToken,
	}

	_ = json.NewEncoder(w).Encode(tokenResp)
}

func (m *MockUpstreamOAuth2Server) strictPublicClientTokenError(r *http.Request) (int, string) {
	m.requestMutex.RLock()
	strictPublicClientMode := m.strictPublicClientMode
	m.requestMutex.RUnlock()
	if !strictPublicClientMode {
		return 0, ""
	}
	if len(r.Header.Values("Authorization")) > 0 {
		return http.StatusUnauthorized, "invalid_client"
	}
	if _, hasClientSecret := r.Form["client_secret"]; hasClientSecret {
		return http.StatusUnauthorized, "invalid_client"
	}
	if r.FormValue("grant_type") != "authorization_code" {
		return 0, ""
	}
	if m.matchesPKCEVerifier(r.FormValue("code"), r.FormValue("code_verifier")) {
		return 0, ""
	}
	return http.StatusBadRequest, "invalid_grant"
}

func (m *MockUpstreamOAuth2Server) matchesPKCEVerifier(code, verifier string) bool {
	verifierChallenge := GenerateCodeChallenge(verifier)
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	challenge, ok := m.codeChallengesByCode[code]
	return ok && challenge.method == "S256" && verifierChallenge == challenge.value
}

// handleMetadata handles the /.well-known/openid-configuration endpoint.
// Simulates OpenID Connect metadata discovery.
func (m *MockUpstreamOAuth2Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	m.requestMutex.Lock()
	m.metadataCalled = true
	m.requestMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	baseURL := m.URL()
	metadata := map[string]interface{}{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/oauth/authorize",
		"token_endpoint":                        baseURL + "/oauth/token",
		"userinfo_endpoint":                     baseURL + "/oauth/userinfo",
		"jwks_uri":                              baseURL + "/.well-known/jwks.json",
		"response_types_supported":              []string{"code", "token"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}

	_ = json.NewEncoder(w).Encode(metadata)
}

// handleJWKS handles the /.well-known/jwks.json endpoint.
// Returns the JWKS Set containing the public key used for JWT validation.
// This endpoint is called by the JWT validator to fetch keys for JWT signature verification.
func (m *MockUpstreamOAuth2Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	m.requestMutex.Lock()
	m.jwksCalled = true
	m.requestMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(m.jwksSet)
}

// readRequestBody is a helper to read request body content.
// It's safe to call multiple times (doesn't consume the reader).
func readRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return []byte{}, nil
	}

	defer func() { _ = r.Body.Close() }()

	var buf [32 * 1024]byte
	n, err := r.Body.Read(buf[:])
	if err != nil && err.Error() != "EOF" {
		return nil, err
	}

	return buf[:n], nil
}
