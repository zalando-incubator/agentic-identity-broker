package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

const protectedResourceRequestLimit = 64 << 10

// ProtectedResourceRequest is a safe snapshot of an inbound provider request.
// Credentials, assertions, authorization codes, PKCE values, and tokens are
// deliberately excluded from Query and Form.
type ProtectedResourceRequest struct {
	Host       string
	Method     string
	Path       string
	Query      url.Values
	Form       url.Values
	AuthMethod string

	PKCE                   bool
	PKCEChallengePresent   bool
	PKCEVerifierPresent    bool
	BasicAuthPresent       bool
	ClientSecretPresent    bool
	ClientAssertionPresent bool
}

type protectedResourceRoute struct {
	host   string
	method string
	path   string
}

type protectedResourceAuthorization struct {
	clientID    string
	redirectURI string
	resource    string
	challenge   string
}

type protectedResourceRegistration struct {
	clientID     string
	clientSecret string
	method       string
	redirectURIs []string
}

type protectedResourceRegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	RedirectURIs            []string `json:"redirect_uris"`
}

// MockProtectedResourceProvider serves configurable responses from an isolated
// TLS server. Route changes and observations never cross scenario boundaries.
type MockProtectedResourceProvider struct {
	Server *httptest.Server

	mu            sync.RWMutex
	replies       map[protectedResourceRoute]fixtures.ProtectedResourceReply
	registration  protectedResourceRoute
	authorization map[protectedResourceRoute]struct{}
	token         map[protectedResourceRoute]struct{}
	calls         []ProtectedResourceRequest
	registrations int
	codes         map[[sha256.Size]byte]protectedResourceAuthorization
	clients       map[string]protectedResourceRegistration
	refreshTokens map[[sha256.Size]byte]string
}

// NewMockProtectedResourceProvider starts one TLS server for one scenario.
// Close the provider after the scenario finishes.
func NewMockProtectedResourceProvider(s fixtures.ProtectedResourceScenario) *MockProtectedResourceProvider {
	provider := &MockProtectedResourceProvider{
		replies:       make(map[protectedResourceRoute]fixtures.ProtectedResourceReply, len(s.Replies)),
		authorization: make(map[protectedResourceRoute]struct{}),
		token:         make(map[protectedResourceRoute]struct{}),
		codes:         make(map[[sha256.Size]byte]protectedResourceAuthorization),
		clients:       make(map[string]protectedResourceRegistration),
		refreshTokens: make(map[[sha256.Size]byte]string),
	}
	if u, err := url.Parse(s.RegistrationURL); err == nil && u.Host != "" {
		provider.registration = protectedResourceRoute{host: strings.ToLower(u.Host), method: http.MethodPost, path: u.Path}
	}
	for _, reply := range s.Replies {
		provider.SetReply(reply)
	}
	provider.Server = httptest.NewTLSServer(http.HandlerFunc(provider.serveHTTP))
	return provider
}

// Close shuts down this scenario's TLS server.
func (m *MockProtectedResourceProvider) Close() {
	if m != nil && m.Server != nil {
		m.Server.Close()
	}
}

// Calls returns the request sequence without exposing mutable provider state.
func (m *MockProtectedResourceProvider) Calls() []ProtectedResourceRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]ProtectedResourceRequest, len(m.calls))
	for i, call := range m.calls {
		calls[i] = call
		calls[i].Query = cloneProtectedResourceValues(call.Query)
		calls[i].Form = cloneProtectedResourceValues(call.Form)
	}
	return calls
}

// RegistrationCount reports how many DCR POSTs reached this provider.
func (m *MockProtectedResourceProvider) RegistrationCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.registrations
}

// SetReply replaces a host/method/path response for subsequent requests.
// In-flight responses retain the snapshot selected when their request arrived.
func (m *MockProtectedResourceProvider) SetReply(reply fixtures.ProtectedResourceReply) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := protectedResourceRoute{host: strings.ToLower(reply.Host), method: reply.Method, path: reply.Path}
	reply.Headers = reply.Headers.Clone()
	m.replies[key] = reply
	if reply.Status != 0 && reply.Status != http.StatusOK {
		return
	}
	if !strings.Contains(reply.Path, "/.well-known/oauth-authorization-server") &&
		!strings.Contains(reply.Path, "/.well-known/openid-configuration") {
		return
	}
	var metadata struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if json.Unmarshal([]byte(reply.Body), &metadata) != nil {
		return
	}
	if endpoint, ok := protectedResourceEndpoint(metadata.AuthorizationEndpoint, http.MethodGet); ok {
		m.authorization[endpoint] = struct{}{}
	}
	if endpoint, ok := protectedResourceEndpoint(metadata.TokenEndpoint, http.MethodPost); ok {
		m.token[endpoint] = struct{}{}
	}
}

func protectedResourceEndpoint(rawURL, method string) (protectedResourceRoute, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.Path == "" {
		return protectedResourceRoute{}, false
	}
	return protectedResourceRoute{host: strings.ToLower(u.Host), method: method, path: u.Path}, true
}

func (m *MockProtectedResourceProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	key := protectedResourceRoute{host: strings.ToLower(r.Host), method: r.Method, path: r.URL.Path}
	query := r.URL.Query()
	call := ProtectedResourceRequest{
		Host:   r.Host,
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  observedProtectedResourceValues(query, "resource", "response_type", "client_id", "code_challenge_method"),
	}
	call.PKCEChallengePresent = len(query["code_challenge"]) != 0
	m.mu.Lock()
	reply, matched := m.replies[key]
	_, authorize := m.authorization[key]
	_, token := m.token[key]
	registration := key == m.registration
	index := len(m.calls)
	m.calls = append(m.calls, call)
	if registration {
		m.registrations++
	}
	m.mu.Unlock()

	if !matched {
		http.NotFound(w, r)
		return
	}

	var registrationRequest protectedResourceRegistrationRequest
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, protectedResourceRequestLimit)
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err == nil && mediaType == "application/x-www-form-urlencoded" {
			if err = r.ParseForm(); err == nil {
				call.Form = observedProtectedResourceValues(r.PostForm, "resource", "grant_type", "client_id", "client_assertion_type")
			}
		} else if registration && err == nil && mediaType == "application/json" {
			decoder := json.NewDecoder(r.Body)
			if err = decoder.Decode(&registrationRequest); err == nil {
				var remaining any
				if decoder.Decode(&remaining) != io.EOF {
					err = io.ErrUnexpectedEOF
				}
			}
			call.Form = url.Values{
				"client_name":                {registrationRequest.ClientName},
				"token_endpoint_auth_method": {registrationRequest.TokenEndpointAuthMethod},
			}
		} else if registration || token {
			err = io.ErrUnexpectedEOF
		} else {
			err = nil
		}
		call.AuthMethod, call.BasicAuthPresent, call.ClientSecretPresent, call.ClientAssertionPresent = protectedResourceAuthForm(r)
		call.PKCEVerifierPresent = len(r.PostForm["code_verifier"]) != 0
		m.mu.Lock()
		m.calls[index] = call
		m.mu.Unlock()
		if err != nil {
			writeProtectedResourceError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}

	if reply.Delay > 0 {
		timer := time.NewTimer(reply.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}

	status := reply.Status
	if status == 0 {
		status = http.StatusOK
	}
	for name, values := range reply.Headers {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	if authorize && status >= 300 && status < 400 && w.Header().Get("Location") == "" {
		m.authorize(w, r, status, index)
		return
	}
	if registration && status == http.StatusCreated {
		if r.Header.Get("Authorization") != "" || registrationRequest.ClientName == "" ||
			registrationRequest.TokenEndpointAuthMethod == "" || len(registrationRequest.RedirectURIs) == 0 {
			writeProtectedResourceError(w, http.StatusBadRequest, "invalid_client_metadata")
			return
		}
		m.register(w, reply, registrationRequest, status)
		return
	}
	if token && status >= 200 && status < 300 {
		if !m.validateToken(r, index) {
			writeProtectedResourceError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		m.rememberRefreshToken(reply.Body, r.PostForm.Get("resource"))
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, reply.Body)
}

func (m *MockProtectedResourceProvider) authorize(w http.ResponseWriter, r *http.Request, status, index int) {
	query := r.URL.Query()
	redirectURI, err := url.Parse(query.Get("redirect_uri"))
	challenge := query.Get("code_challenge")
	decoded, challengeErr := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil || redirectURI.Host == "" || (redirectURI.Scheme != "https" && redirectURI.Scheme != "http") ||
		len(query["resource"]) != 1 || query.Get("resource") == "" ||
		len(query["client_id"]) != 1 || query.Get("client_id") == "" ||
		len(query["state"]) != 1 || query.Get("state") == "" ||
		len(query["code_challenge"]) != 1 || challengeErr != nil || len(decoded) != sha256.Size ||
		len(query["code_challenge_method"]) != 1 || query.Get("code_challenge_method") != "S256" {
		writeProtectedResourceError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	m.mu.RLock()
	registered, hasClient := m.clients[query.Get("client_id")]
	m.mu.RUnlock()
	if hasClient {
		callbackAllowed := false
		for _, callback := range registered.redirectURIs {
			if callback == redirectURI.String() {
				callbackAllowed = true
				break
			}
		}
		if !callbackAllowed {
			writeProtectedResourceError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	codeBytes := make([]byte, 32)
	if _, err := rand.Read(codeBytes); err != nil {
		writeProtectedResourceError(w, http.StatusInternalServerError, "server_error")
		return
	}
	code := base64.RawURLEncoding.EncodeToString(codeBytes)
	m.mu.Lock()
	m.codes[sha256.Sum256([]byte(code))] = protectedResourceAuthorization{
		clientID: query.Get("client_id"), redirectURI: redirectURI.String(),
		resource: query.Get("resource"), challenge: challenge,
	}
	m.calls[index].PKCE = true
	m.mu.Unlock()

	params := redirectURI.Query()
	params.Set("code", code)
	params.Set("state", query.Get("state"))
	redirectURI.RawQuery = params.Encode()
	w.Header().Set("Location", redirectURI.String())
	w.WriteHeader(status)
}

func (m *MockProtectedResourceProvider) register(w http.ResponseWriter, reply fixtures.ProtectedResourceReply, request protectedResourceRegistrationRequest, status int) {
	var response map[string]json.RawMessage
	if json.Unmarshal([]byte(reply.Body), &response) != nil {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply.Body)
		return
	}
	var redirectURIs []string
	if raw, ok := response["redirect_uris"]; ok && json.Unmarshal(raw, &redirectURIs) == nil && len(redirectURIs) == 0 {
		response["redirect_uris"], _ = json.Marshal(request.RedirectURIs)
	}
	body, err := json.Marshal(response)
	if err != nil {
		writeProtectedResourceError(w, http.StatusInternalServerError, "server_error")
		return
	}
	var client struct {
		ID         string `json:"client_id"`
		Secret     string `json:"client_secret"`
		AuthMethod string `json:"token_endpoint_auth_method"`
	}
	if json.Unmarshal(body, &client) == nil && client.ID != "" &&
		(client.AuthMethod == "" || client.AuthMethod == request.TokenEndpointAuthMethod) {
		m.mu.Lock()
		m.clients[client.ID] = protectedResourceRegistration{
			clientID: client.ID, clientSecret: client.Secret,
			method: request.TokenEndpointAuthMethod, redirectURIs: append([]string(nil), request.RedirectURIs...),
		}
		m.mu.Unlock()
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (m *MockProtectedResourceProvider) validateToken(r *http.Request, index int) bool {
	form := r.PostForm
	if len(form["resource"]) != 1 || form.Get("resource") == "" || len(r.URL.Query()["resource"]) != 0 ||
		len(form["grant_type"]) != 1 {
		return false
	}
	method, _, hasSecret, hasAssertion := protectedResourceAuthForm(r)
	clientID := form.Get("client_id")
	if method == "client_secret_basic" {
		clientID, _, _ = r.BasicAuth()
	}
	m.mu.RLock()
	registered, hasRegistration := m.clients[clientID]
	m.mu.RUnlock()
	if hasRegistration {
		if method != registered.method || hasAssertion || len(form["client_id"]) > 1 {
			return false
		}
		switch method {
		case "client_secret_basic":
			user, password, ok := r.BasicAuth()
			if !ok || user != registered.clientID || hasSecret || len(form["client_id"]) != 0 ||
				subtle.ConstantTimeCompare([]byte(password), []byte(registered.clientSecret)) != 1 {
				return false
			}
		case "client_secret_post":
			if len(form["client_id"]) != 1 || len(form["client_secret"]) != 1 ||
				subtle.ConstantTimeCompare([]byte(form.Get("client_secret")), []byte(registered.clientSecret)) != 1 {
				return false
			}
		case "none":
			if len(form["client_id"]) != 1 || hasSecret || r.Header.Get("Authorization") != "" {
				return false
			}
		default:
			return false
		}
	} else if method != "private_key_jwt" || hasSecret || r.Header.Get("Authorization") != "" ||
		len(form["client_id"]) != 1 || len(form["client_assertion"]) != 1 || form.Get("client_assertion") == "" ||
		len(form["client_assertion_type"]) != 1 || form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		return false
	}

	switch form.Get("grant_type") {
	case "authorization_code":
		if len(form["code"]) != 1 || len(form["code_verifier"]) != 1 ||
			len(form["redirect_uri"]) != 1 || form.Get("code") == "" {
			return false
		}
		m.mu.Lock()
		authorization, ok := m.codes[sha256.Sum256([]byte(form.Get("code")))]
		if !ok || authorization.clientID != clientID || authorization.redirectURI != form.Get("redirect_uri") ||
			authorization.resource != form.Get("resource") {
			m.mu.Unlock()
			return false
		}
		verifierHash := sha256.Sum256([]byte(form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(verifierHash[:]) != authorization.challenge {
			m.mu.Unlock()
			return false
		}
		delete(m.codes, sha256.Sum256([]byte(form.Get("code"))))
		m.calls[index].PKCE = true
		m.mu.Unlock()
		return true
	case "refresh_token":
		if len(form["refresh_token"]) != 1 || form.Get("refresh_token") == "" {
			return false
		}
		m.mu.RLock()
		resource, ok := m.refreshTokens[sha256.Sum256([]byte(form.Get("refresh_token")))]
		m.mu.RUnlock()
		return ok && resource == form.Get("resource")
	default:
		return false
	}
}

func (m *MockProtectedResourceProvider) rememberRefreshToken(body, resource string) {
	var response struct {
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal([]byte(body), &response) != nil || response.RefreshToken == "" {
		return
	}
	m.mu.Lock()
	m.refreshTokens[sha256.Sum256([]byte(response.RefreshToken))] = resource
	m.mu.Unlock()
}

func protectedResourceAuthForm(r *http.Request) (method string, basic, secret, assertion bool) {
	basic = r.Header.Get("Authorization") != ""
	secret = len(r.PostForm["client_secret"]) != 0
	assertion = len(r.PostForm["client_assertion"]) != 0
	switch {
	case basic:
		if _, _, ok := r.BasicAuth(); ok {
			return "client_secret_basic", true, secret, assertion
		}
		return "invalid", true, secret, assertion
	case assertion:
		return "private_key_jwt", false, secret, true
	case secret:
		return "client_secret_post", false, true, false
	default:
		return "none", false, false, false
	}
}

func observedProtectedResourceValues(values url.Values, names ...string) url.Values {
	observed := make(url.Values)
	for _, name := range names {
		if entries, present := values[name]; present {
			observed[name] = append([]string(nil), entries...)
		}
	}
	return observed
}

func cloneProtectedResourceValues(values url.Values) url.Values {
	copy := make(url.Values, len(values))
	for key, entries := range values {
		copy[key] = append([]string(nil), entries...)
	}
	return copy
}

func writeProtectedResourceError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
