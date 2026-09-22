// Package helpers provides test utilities for E2E testing.
package helpers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

const (
	cimdClientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	cimdAssertionLifetime   = 5 * time.Minute
	cimdClockSkew           = 30 * time.Second
	cimdCodeLifetime        = 5 * time.Minute
	cimdMaxDocumentSize     = 1 << 20
	cimdMaxTokenFormSize    = 64 << 10
)

// CIMDUpstreamOption configures a MockCIMDUpstream instance.
type CIMDUpstreamOption func(*MockCIMDUpstream)

// WithCIMDUpstreamHTTPClient supplies the client used to retrieve broker-hosted
// client metadata and its JWKS. This lets E2E tests route the stable HTTPS
// public URL to their production-bootstrap test server.
func WithCIMDUpstreamHTTPClient(client *http.Client) CIMDUpstreamOption {
	return func(upstream *MockCIMDUpstream) {
		if client != nil {
			upstream.brokerHTTPClient = client
		}
	}
}

// CIMDAuthorizationRequestObservation is the safe subset of an authorization
// request that a test may inspect. It never contains a code or PKCE value.
type CIMDAuthorizationRequestObservation struct {
	MetadataValidated bool
	PKCEValidated     bool
}

// CIMDTokenRequestObservation is the safe subset of a token request that a
// test may inspect. It never contains an assertion, secret, code, verifier,
// refresh token, or access token.
type CIMDTokenRequestObservation struct {
	GrantType                  string
	ClientIDValidated          bool
	ClientAssertionCount       int
	ClientAssertionTypeCount   int
	JWTBearerAssertionType     bool
	AssertionKeyID             string
	AssertionValidated         bool
	PKCEValidated              bool
	SecretAuthenticationReject bool
}

// CIMDUpstreamObservations is an immutable snapshot of safe provider state.
type CIMDUpstreamObservations struct {
	AuthorizationRequests []CIMDAuthorizationRequestObservation
	TokenRequests         []CIMDTokenRequestObservation
	MetadataFetchCount    int
	JWKSFetchCount        int
	CachedKeyIDs          []string
}

type cimdClientMetadataDocument struct {
	ClientID                     string   `json:"client_id"`
	RedirectURIs                 []string `json:"redirect_uris"`
	GrantTypes                   []string `json:"grant_types"`
	ResponseTypes                []string `json:"response_types"`
	TokenEndpointAuthMethod      string   `json:"token_endpoint_auth_method"`
	TokenEndpointAuthSigningAlgo string   `json:"token_endpoint_auth_signing_alg"`
	JWKSURI                      string   `json:"jwks_uri"`
}

type cimdPublicJWKSet struct {
	Keys []struct {
		KeyID      string  `json:"kid"`
		KeyType    string  `json:"kty"`
		Algorithm  string  `json:"alg"`
		Use        string  `json:"use"`
		Curve      string  `json:"crv"`
		PrivateKey *string `json:"d"`
	} `json:"keys"`
}

type cachedCIMDClient struct {
	clientID     string
	redirectURIs map[string]struct{}
	keys         jwk.Set
	keyIDs       []string
}

type cimdPKCESession struct {
	challenge   string
	clientID    string
	redirectURI string
	expiresAt   time.Time
}

// MockCIMDUpstream is a per-test OAuth2 authorization server double for a
// broker that is a private_key_jwt CIMD client. It retrieves the broker's
// public metadata and JWKS over HTTP, then retains only the current public
// verification keys until RefreshClientMetadata is called.
type MockCIMDUpstream struct {
	Server *httptest.Server

	brokerHTTPClient *http.Client

	mu                    sync.RWMutex
	clients               map[string]*cachedCIMDClient
	codes                 map[[sha256.Size]byte]cimdPKCESession
	usedAssertionIDs      map[[sha256.Size]byte]time.Time
	authorizationRequests []CIMDAuthorizationRequestObservation
	tokenRequests         []CIMDTokenRequestObservation
	metadataFetchCount    int
	jwksFetchCount        int
	tokenError            string
}

// NewMockCIMDUpstream starts a conformant, isolated private_key_jwt provider
// double. Call Close when the test finishes.
func NewMockCIMDUpstream(options ...CIMDUpstreamOption) *MockCIMDUpstream {
	upstream := &MockCIMDUpstream{
		brokerHTTPClient: &http.Client{Timeout: 5 * time.Second},
		clients:          make(map[string]*cachedCIMDClient),
		codes:            make(map[[sha256.Size]byte]cimdPKCESession),
		usedAssertionIDs: make(map[[sha256.Size]byte]time.Time),
	}
	for _, option := range options {
		if option != nil {
			option(upstream)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", upstream.handleAuthorize)
	mux.HandleFunc("/oauth/authorize", upstream.handleAuthorize)
	mux.HandleFunc("/token", upstream.handleToken)
	mux.HandleFunc("/oauth/token", upstream.handleToken)
	upstream.Server = httptest.NewServer(mux)
	return upstream
}

// Close shuts down the provider double.
func (m *MockCIMDUpstream) Close() {
	if m != nil && m.Server != nil {
		m.Server.Close()
	}
}

// URL returns the provider base URL.
func (m *MockCIMDUpstream) URL() string {
	if m == nil || m.Server == nil {
		return ""
	}
	return m.Server.URL
}

// AuthorizationEndpoint returns the primary authorization endpoint URL.
func (m *MockCIMDUpstream) AuthorizationEndpoint() string {
	return m.URL() + "/authorize"
}

// TokenEndpoint returns the primary token endpoint URL.
func (m *MockCIMDUpstream) TokenEndpoint() string {
	return m.URL() + "/token"
}

// WithTokenError makes otherwise-valid token requests receive a safe OAuth
// error code. It is useful for proving that the broker fails closed after an
// upstream rejection. An empty code restores successful responses.
func (m *MockCIMDUpstream) WithTokenError(code string) *MockCIMDUpstream {
	switch code {
	case "", "invalid_client", "invalid_grant", "server_error":
	default:
		code = "server_error"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenError = code
	return m
}

// RefreshClientMetadata fetches a fresh broker metadata document and its JWKS.
// It atomically replaces the cached public keys only after both documents pass
// their protocol checks, mirroring a provider's explicit key-cache refresh.
func (m *MockCIMDUpstream) RefreshClientMetadata(ctx context.Context, clientID string) error {
	return m.refreshClientMetadata(ctx, clientID)
}

// Observations returns a copy of the safe state available to E2E assertions.
func (m *MockCIMDUpstream) Observations() CIMDUpstreamObservations {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keyIDs := make(map[string]struct{})
	for _, client := range m.clients {
		for _, keyID := range client.keyIDs {
			keyIDs[keyID] = struct{}{}
		}
	}
	cachedKeyIDs := make([]string, 0, len(keyIDs))
	for keyID := range keyIDs {
		cachedKeyIDs = append(cachedKeyIDs, keyID)
	}
	sort.Strings(cachedKeyIDs)

	return CIMDUpstreamObservations{
		AuthorizationRequests: append([]CIMDAuthorizationRequestObservation(nil), m.authorizationRequests...),
		TokenRequests:         append([]CIMDTokenRequestObservation(nil), m.tokenRequests...),
		MetadataFetchCount:    m.metadataFetchCount,
		JWKSFetchCount:        m.jwksFetchCount,
		CachedKeyIDs:          cachedKeyIDs,
	}
}

func (m *MockCIMDUpstream) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	observation := CIMDAuthorizationRequestObservation{}
	defer func() {
		m.mu.Lock()
		m.authorizationRequests = append(m.authorizationRequests, observation)
		m.mu.Unlock()
	}()

	if r.Method != http.MethodGet {
		writeCIMDOAuthError(w, http.StatusMethodNotAllowed, "invalid_request")
		return
	}

	query := r.URL.Query()
	clientID, ok := exactlyOneNonEmpty(query, "client_id")
	if !ok {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	redirectURI, ok := exactlyOneNonEmpty(query, "redirect_uri")
	if !ok || !hasExactly(query, "response_type", "code") {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	state, ok := exactlyOneNonEmpty(query, "state")
	if !ok {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	challenge, ok := exactlyOneNonEmpty(query, "code_challenge")
	if !ok || !hasExactly(query, "code_challenge_method", "S256") {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	client, err := m.cachedClient(r.Context(), clientID)
	if err != nil {
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}
	observation.MetadataValidated = true
	if _, found := client.redirectURIs[redirectURI]; !found {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	code, err := randomCIMDOpaqueValue()
	if err != nil {
		writeCIMDOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	m.storeAuthorizationCode(code, cimdPKCESession{
		challenge:   challenge,
		clientID:    clientID,
		redirectURI: redirectURI,
		expiresAt:   time.Now().UTC().Add(cimdCodeLifetime),
	})
	observation.PKCEValidated = true

	callback, err := url.Parse(redirectURI)
	if err != nil {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	params := callback.Query()
	params.Set("code", code)
	params.Set("state", state)
	callback.RawQuery = params.Encode()
	http.Redirect(w, r, callback.String(), http.StatusFound)
}

func (m *MockCIMDUpstream) handleToken(w http.ResponseWriter, r *http.Request) {
	observation := CIMDTokenRequestObservation{}
	defer func() {
		m.mu.Lock()
		m.tokenRequests = append(m.tokenRequests, observation)
		m.mu.Unlock()
	}()

	if r.Method != http.MethodPost || !isFormPost(r) {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if hasSecretAuthentication(r) {
		observation.SecretAuthenticationReject = true
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, cimdMaxTokenFormSize)
	if err := r.ParseForm(); err != nil {
		writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if hasAuthenticationQueryParameter(r.URL.Query()) || hasSecretFormParameter(r.PostForm) {
		observation.SecretAuthenticationReject = true
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}

	grantType, ok := exactlyOneNonEmpty(r.PostForm, "grant_type")
	if !ok || (grantType != "authorization_code" && grantType != "refresh_token") {
		writeCIMDOAuthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	observation.GrantType = grantType

	clientID, ok := exactlyOneNonEmpty(r.PostForm, "client_id")
	if !ok {
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}
	client, err := m.cachedClient(r.Context(), clientID)
	if err != nil {
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}
	observation.ClientIDValidated = true

	assertionTypes := r.PostForm["client_assertion_type"]
	assertions := r.PostForm["client_assertion"]
	observation.ClientAssertionTypeCount = len(assertionTypes)
	observation.ClientAssertionCount = len(assertions)
	if len(assertionTypes) != 1 || len(assertions) != 1 || assertions[0] == "" || assertionTypes[0] != cimdClientAssertionType {
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}
	observation.JWTBearerAssertionType = true

	keyID, err := m.validateClientAssertion(assertions[0], client, m.tokenAudience(r))
	if err != nil {
		writeCIMDOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}
	observation.AssertionKeyID = keyID
	observation.AssertionValidated = true

	switch grantType {
	case "authorization_code":
		code, codeOK := exactlyOneNonEmpty(r.PostForm, "code")
		verifier, verifierOK := exactlyOneNonEmpty(r.PostForm, "code_verifier")
		redirectURI, redirectOK := exactlyOneNonEmpty(r.PostForm, "redirect_uri")
		if !codeOK || !verifierOK || !redirectOK || !m.consumeAuthorizationCode(code, verifier, clientID, redirectURI) {
			writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		observation.PKCEValidated = true
	case "refresh_token":
		if _, ok := exactlyOneNonEmpty(r.PostForm, "refresh_token"); !ok {
			writeCIMDOAuthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
	}

	m.mu.RLock()
	tokenError := m.tokenError
	m.mu.RUnlock()
	if tokenError != "" {
		writeCIMDOAuthError(w, http.StatusBadRequest, tokenError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  "cimd-upstream-access-token",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": "cimd-upstream-refresh-token",
	})
}

func (m *MockCIMDUpstream) cachedClient(ctx context.Context, clientID string) (*cachedCIMDClient, error) {
	m.mu.RLock()
	client := m.clients[clientID]
	m.mu.RUnlock()
	if client != nil {
		return client, nil
	}
	if err := m.refreshClientMetadata(ctx, clientID); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[clientID], nil
}

func (m *MockCIMDUpstream) refreshClientMetadata(ctx context.Context, clientID string) error {
	metadataURL, err := url.Parse(clientID)
	if err != nil || metadataURL.Scheme != "https" || metadataURL.Host == "" || metadataURL.User != nil || metadataURL.RawQuery != "" || metadataURL.Fragment != "" {
		return fmt.Errorf("client metadata URL is not HTTPS")
	}

	m.mu.Lock()
	m.metadataFetchCount++
	m.mu.Unlock()
	metadataBody, err := m.getJSON(ctx, clientID)
	if err != nil {
		return fmt.Errorf("client metadata request failed: %w", err)
	}

	var metadata cimdClientMetadataDocument
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		return fmt.Errorf("client metadata is not valid JSON")
	}
	if err := validateCIMDClientMetadata(metadata, clientID); err != nil {
		return err
	}

	m.mu.Lock()
	m.jwksFetchCount++
	m.mu.Unlock()
	jwksBody, err := m.getJSON(ctx, metadata.JWKSURI)
	if err != nil {
		return fmt.Errorf("client JWKS request failed: %w", err)
	}
	keySet, keyIDs, err := parseCIMDPublicJWKS(jwksBody)
	if err != nil {
		return err
	}

	redirectURIs := make(map[string]struct{}, len(metadata.RedirectURIs))
	for _, redirectURI := range metadata.RedirectURIs {
		redirectURIs[redirectURI] = struct{}{}
	}
	client := &cachedCIMDClient{
		clientID:     metadata.ClientID,
		redirectURIs: redirectURIs,
		keys:         keySet,
		keyIDs:       keyIDs,
	}

	m.mu.Lock()
	m.clients[clientID] = client
	m.mu.Unlock()
	return nil
}

func (m *MockCIMDUpstream) getJSON(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := m.brokerHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status")
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return nil, fmt.Errorf("response is not JSON")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, cimdMaxDocumentSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > cimdMaxDocumentSize {
		return nil, fmt.Errorf("response is too large")
	}
	return body, nil
}

func validateCIMDClientMetadata(metadata cimdClientMetadataDocument, requestedClientID string) error {
	if metadata.ClientID != requestedClientID || metadata.TokenEndpointAuthMethod != "private_key_jwt" || metadata.TokenEndpointAuthSigningAlgo != "ES256" {
		return fmt.Errorf("client metadata does not declare ES256 private_key_jwt authentication")
	}
	jwksURL, err := url.Parse(metadata.JWKSURI)
	if err != nil || jwksURL.Scheme != "https" || jwksURL.Host == "" || jwksURL.User != nil || jwksURL.RawQuery != "" || jwksURL.Fragment != "" {
		return fmt.Errorf("client metadata JWKS URI is not HTTPS")
	}
	if len(metadata.RedirectURIs) == 0 || !containsExact(metadata.GrantTypes, "authorization_code") || !containsExact(metadata.GrantTypes, "refresh_token") || !containsExact(metadata.ResponseTypes, "code") {
		return fmt.Errorf("client metadata is incomplete")
	}
	return nil
}

func parseCIMDPublicJWKS(raw []byte) (jwk.Set, []string, error) {
	var wire cimdPublicJWKSet
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, nil, fmt.Errorf("client JWKS is not valid JSON")
	}
	if len(wire.Keys) == 0 {
		return nil, nil, fmt.Errorf("client JWKS has no keys")
	}

	seen := make(map[string]struct{}, len(wire.Keys))
	keyIDs := make([]string, 0, len(wire.Keys))
	for _, key := range wire.Keys {
		if key.KeyID == "" || key.KeyType != "EC" || key.Curve != "P-256" || key.Algorithm != "ES256" || key.Use != "sig" || key.PrivateKey != nil {
			return nil, nil, fmt.Errorf("client JWKS contains a non-public ES256 signing key")
		}
		if _, duplicate := seen[key.KeyID]; duplicate {
			return nil, nil, fmt.Errorf("client JWKS contains duplicate key IDs")
		}
		seen[key.KeyID] = struct{}{}
		keyIDs = append(keyIDs, key.KeyID)
	}

	keySet, err := jwk.Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("client JWKS cannot be parsed")
	}
	sort.Strings(keyIDs)
	return keySet, keyIDs, nil
}

func (m *MockCIMDUpstream) validateClientAssertion(assertion string, client *cachedCIMDClient, audience string) (string, error) {
	signature, err := soleCIMDSignature(assertion)
	if err != nil {
		return "", err
	}
	algorithm, ok := signature.ProtectedHeaders().Algorithm()
	if !ok || algorithm != jwa.ES256() {
		return "", fmt.Errorf("client assertion is not ES256")
	}
	keyID, ok := signature.ProtectedHeaders().KeyID()
	if !ok || keyID == "" {
		return "", fmt.Errorf("client assertion has no key ID")
	}
	key, found := client.keys.LookupKeyID(keyID)
	if !found {
		return "", fmt.Errorf("client assertion key is not cached")
	}

	token, err := jwt.ParseString(assertion, jwt.WithKey(jwa.ES256(), key), jwt.WithValidate(false))
	if err != nil {
		return "", fmt.Errorf("client assertion signature is invalid")
	}
	issuer, issuerOK := token.Issuer()
	subject, subjectOK := token.Subject()
	audiences, audienceOK := token.Audience()
	issuedAt, issuedAtOK := token.IssuedAt()
	expiresAt, expiresAtOK := token.Expiration()
	assertionID, assertionIDOK := token.JwtID()
	now := time.Now().UTC()
	if !issuerOK || issuer != client.clientID || !subjectOK || subject != client.clientID || !audienceOK || len(audiences) != 1 || audiences[0] != audience || !issuedAtOK || !expiresAtOK || !assertionIDOK || assertionID == "" {
		return "", fmt.Errorf("client assertion claims are invalid")
	}
	if issuedAt.After(now.Add(cimdClockSkew)) || expiresAt.Before(now.Add(-cimdClockSkew)) || !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > cimdAssertionLifetime {
		return "", fmt.Errorf("client assertion lifetime is invalid")
	}
	if !m.claimAssertionID(assertionID, expiresAt) {
		return "", fmt.Errorf("client assertion was replayed")
	}
	return keyID, nil
}

func soleCIMDSignature(assertion string) (*jws.Signature, error) {
	message, err := jws.Parse([]byte(assertion))
	if err != nil {
		return nil, fmt.Errorf("client assertion is not a compact JWS")
	}
	signatures := message.Signatures()
	if len(signatures) != 1 {
		return nil, fmt.Errorf("client assertion must have one signature")
	}
	return signatures[0], nil
}

func (m *MockCIMDUpstream) claimAssertionID(assertionID string, expiresAt time.Time) bool {
	hash := sha256.Sum256([]byte(assertionID))
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for seenHash, seenExpiry := range m.usedAssertionIDs {
		if !seenExpiry.After(now) {
			delete(m.usedAssertionIDs, seenHash)
		}
	}
	if _, used := m.usedAssertionIDs[hash]; used {
		return false
	}
	m.usedAssertionIDs[hash] = expiresAt
	return true
}

func (m *MockCIMDUpstream) storeAuthorizationCode(code string, session cimdPKCESession) {
	hash := sha256.Sum256([]byte(code))
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for codeHash, existing := range m.codes {
		if !existing.expiresAt.After(now) {
			delete(m.codes, codeHash)
		}
	}
	m.codes[hash] = session
}

func (m *MockCIMDUpstream) consumeAuthorizationCode(code, verifier, clientID, redirectURI string) bool {
	hash := sha256.Sum256([]byte(code))
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	session, found := m.codes[hash]
	if !found || !session.expiresAt.After(now) {
		return false
	}
	delete(m.codes, hash)
	return session.clientID == clientID && session.redirectURI == redirectURI && session.challenge == GenerateCodeChallenge(verifier)
}

func (m *MockCIMDUpstream) tokenAudience(r *http.Request) string {
	return m.URL() + r.URL.EscapedPath()
}

func randomCIMDOpaqueValue() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func exactlyOneNonEmpty(values url.Values, name string) (string, bool) {
	entries, found := values[name]
	if !found || len(entries) != 1 || entries[0] == "" {
		return "", false
	}
	return entries[0], true
}

func hasExactly(values url.Values, name, expected string) bool {
	actual, ok := exactlyOneNonEmpty(values, name)
	return ok && actual == expected
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func isFormPost(r *http.Request) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && contentType == "application/x-www-form-urlencoded"
}

func hasSecretAuthentication(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" || r.Header.Get("Proxy-Authorization") != "" {
		return true
	}
	for name := range r.Header {
		if strings.Contains(strings.ToLower(name), "secret") {
			return true
		}
	}
	return false
}

func hasAuthenticationQueryParameter(values url.Values) bool {
	for name := range values {
		switch name {
		case "client_id", "client_assertion", "client_assertion_type", "client_secret":
			return true
		}
	}
	return false
}

func hasSecretFormParameter(values url.Values) bool {
	for name := range values {
		if strings.Contains(strings.ToLower(name), "secret") {
			return true
		}
	}
	return false
}

func writeCIMDOAuthError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
