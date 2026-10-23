package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/lestrrat-go/jwx/v4/jwa"
)

const cimdPrivateKeyJWTFakeHost = "private-key-jwt-shared.test.invalid"

type cimdPrivateKeyJWTFixture struct {
	logger         *slog.Logger
	logBuffer      synchronizedLogBuffer
	storageFactory *bootstrap.StorageFactory
	storage        *storageadapter.Adapter
	server         *bootstrap.TestServer
	metadataServer *httptest.Server
	agent          *domainstorage.Agent
	clientID       string
	redirectURI    string
	jwksURI        string
	assertionKey   *helpers.ClientAssertionKey

	mu          sync.RWMutex
	documents   map[string][]byte
	jwks        []byte
	jwksCalls   int
	jwksHandler func(http.ResponseWriter, *http.Request)
}

type synchronizedLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedLogBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func newCIMDPrivateKeyJWTFixture() (*cimdPrivateKeyJWTFixture, error) {
	return newCIMDPrivateKeyJWTFixtureWithProductionKeySetFetcher(false)
}

func newCIMDPrivateKeyJWTFixtureWithProductionKeySetFetcher(useProduction bool) (*cimdPrivateKeyJWTFixture, error) {
	return newCIMDPrivateKeyJWTFixtureWithKeySetSettings(useProduction, 0, 0, 0)
}

func newCIMDPrivateKeyJWTFixtureWithKeySetSettings(useProduction bool, minRefetch, fetchTimeout, httpTimeout time.Duration) (*cimdPrivateKeyJWTFixture, error) {
	fixture := &cimdPrivateKeyJWTFixture{
		clientID:    "https://" + cimdPrivateKeyJWTFakeHost + "/client",
		redirectURI: "https://" + cimdPrivateKeyJWTFakeHost + "/callback",
		jwksURI:     "https://" + cimdPrivateKeyJWTFakeHost + "/jwks.json",
		documents:   make(map[string][]byte),
	}
	fixture.logger = slog.New(slog.NewTextHandler(&fixture.logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			fixture.Close()
		}
	}()

	var err error
	fixture.storageFactory = bootstrap.NewStorageFactory(fixture.logger)
	fixture.storage, err = fixture.storageFactory.NewTestStorage()
	if err != nil {
		return nil, fmt.Errorf("create CIMD test storage: %w", err)
	}
	fixture.assertionKey, err = helpers.GenerateClientAssertionKey(jwa.RS256(), "client-key-1")
	if err != nil {
		return nil, fmt.Errorf("create CIMD assertion key: %w", err)
	}
	fixture.jwks, err = fixture.assertionKey.JWKSJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal CIMD key set: %w", err)
	}
	if err := fixture.SetDocument("none", nil); err != nil {
		return nil, err
	}

	fixture.metadataServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=300")
		if r.URL.Path == "/jwks.json" {
			fixture.mu.Lock()
			fixture.jwksCalls++
			handler := fixture.jwksHandler
			jwks := append([]byte(nil), fixture.jwks...)
			fixture.mu.Unlock()
			if handler != nil {
				handler(w, r)
				return
			}
			_, _ = w.Write(jwks)
			return
		}
		fixture.mu.RLock()
		defer fixture.mu.RUnlock()
		document, ok := fixture.documents[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(document)
	}))

	now := time.Now()
	fixture.agent = &domainstorage.Agent{
		ID:             id.NewAgentID(),
		ClientURIs:     []string{fixture.clientID},
		DisplayName:    "Private Key JWT E2E Agent",
		Description:    "E2E fixture for CIMD client assertion tests",
		CreatedAt:      now,
		UpdatedAt:      now,
		PermissionSets: fixtures.DefaultPermissionSets(),
	}
	if err := fixture.storage.Agents().Create(context.Background(), fixture.agent); err != nil {
		return nil, fmt.Errorf("create CIMD test agent: %w", err)
	}
	if err := fixtures.SeedDefaultConsentData(context.Background(), fixture.storage, id.Principal(fixtures.DefaultPrincipal().String())); err != nil {
		return nil, fmt.Errorf("seed consent data: %w", err)
	}

	config := fixtures.OAuth2ConfigWithCIMD("")
	if minRefetch > 0 {
		config.OAuth2AuthServer.CIMD.ClientAuthentication.KeySetMinRefetchInterval = minRefetch
	}
	if fetchTimeout > 0 {
		config.OAuth2AuthServer.CIMD.ClientAuthentication.KeySetFetchTimeout = fetchTimeout
	}
	serverFactory := bootstrap.NewServerFactory(config, fixture.logger)
	documentFetcher, err := bootstrap.NewCIMDTestFetcher(fixture.metadataServer, cimdPrivateKeyJWTFakeHost, 5120)
	if err != nil {
		return nil, fmt.Errorf("create document fetcher: %w", err)
	}
	var keySetFetcher ports.CIMDFetcher
	if !useProduction {
		keySetClient := bootstrap.CIMDTestHTTPClient(fixture.metadataServer, cimdPrivateKeyJWTFakeHost)
		if httpTimeout > 0 {
			keySetClient.Timeout = httpTimeout
		}
		keySetFetcher, err = bootstrap.NewCIMDTestFetcherFromClient(keySetClient, 65536)
		if err != nil {
			return nil, fmt.Errorf("create key-set fetcher: %w", err)
		}
	}
	fixture.server, err = bootstrap.NewCIMDEndUserTestServerWithFetchers(fixture.storage, serverFactory, documentFetcher, keySetFetcher, fixture.logger)
	if err != nil {
		return nil, fmt.Errorf("create CIMD test server: %w", err)
	}
	cleanupOnError = false
	return fixture, nil
}

func (f *cimdPrivateKeyJWTFixture) SetDocument(authMethod string, extra map[string]any) error {
	return f.SetDocumentFor(f.clientID, authMethod, extra)
}

func (f *cimdPrivateKeyJWTFixture) SetDocumentFor(clientID, authMethod string, extra map[string]any) error {
	return f.SetClientDocument(clientID, f.redirectURI, authMethod, extra)
}

func (f *cimdPrivateKeyJWTFixture) SetClientDocument(clientID, redirectURI, authMethod string, extra map[string]any) error {
	metadata := map[string]any{
		"client_id":      clientID,
		"client_name":    "Private Key JWT E2E Agent",
		"redirect_uris":  []string{redirectURI},
		"grant_types":    []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"},
	}
	if authMethod != "" {
		metadata["token_endpoint_auth_method"] = authMethod
	}
	if authMethod == "private_key_jwt" {
		metadata["token_endpoint_auth_signing_alg"] = "RS256"
		metadata["jwks_uri"] = f.jwksURI
	}
	for name, value := range extra {
		metadata[name] = value
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal CIMD document: %w", err)
	}
	f.mu.Lock()
	f.documents[path.Clean(mustClientPath(clientID))] = encoded
	f.mu.Unlock()
	return nil
}

func mustClientPath(clientID string) string {
	parsed, err := url.Parse(clientID)
	if err != nil || parsed.Path == "" {
		return "/"
	}
	return parsed.Path
}

func (f *cimdPrivateKeyJWTFixture) AddClient(clientID, redirectURI string) (*domainstorage.Agent, error) {
	now := time.Now()
	agent := &domainstorage.Agent{
		ID:             id.NewAgentID(),
		ClientURIs:     []string{clientID},
		DisplayName:    "Additional Private Key JWT E2E Agent",
		Description:    "Additional CIMD client for isolation scenarios",
		CreatedAt:      now,
		UpdatedAt:      now,
		PermissionSets: fixtures.DefaultPermissionSets(),
	}
	if err := f.storage.Agents().Create(context.Background(), agent); err != nil {
		return nil, fmt.Errorf("create additional CIMD agent: %w", err)
	}
	if err := f.SetClientDocument(clientID, redirectURI, "none", nil); err != nil {
		return nil, err
	}
	return agent, nil
}

func (f *cimdPrivateKeyJWTFixture) SetJWKS(encoded []byte) {
	f.mu.Lock()
	f.jwks = append([]byte(nil), encoded...)
	f.mu.Unlock()
}

func (f *cimdPrivateKeyJWTFixture) SetJWKSHandler(handler func(http.ResponseWriter, *http.Request)) {
	f.mu.Lock()
	f.jwksHandler = handler
	f.mu.Unlock()
}

func (f *cimdPrivateKeyJWTFixture) JWKSRequestCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.jwksCalls
}

func (f *cimdPrivateKeyJWTFixture) SignAssertion(options helpers.ClientAssertionOptions) (string, string, error) {
	return f.SignAssertionFor(f.clientID, f.assertionKey, options)
}

func (f *cimdPrivateKeyJWTFixture) SignAssertionFor(clientID string, key *helpers.ClientAssertionKey, options helpers.ClientAssertionOptions) (string, string, error) {
	issuer := strings.TrimRight(f.server.App().Config.Server.EndUser.PublicURL, "/")
	return helpers.SignClientAssertion(clientID, issuer+"/oauth2/token", key, options)
}

func (f *cimdPrivateKeyJWTFixture) AuthorizeCode(scope string) (string, string, error) {
	return f.AuthorizeCodeFor(f.clientID, f.redirectURI, f.agent.ID, scope)
}

func (f *cimdPrivateKeyJWTFixture) AuthorizeCodeFor(clientID, redirectURI string, agentID id.AgentID, scope string) (string, string, error) {
	principal := fixtures.DefaultPrincipal().String()
	verifier := helpers.PKCEVerifier()
	challenge := helpers.GenerateCodeChallenge(verifier)
	authorizeURL := "/oauth2/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {"private-key-jwt-state"},
		"scope":                 {scope},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	authorizeResp, err := f.server.AuthenticatedGET(authorizeURL, principal)
	if err != nil {
		return "", "", fmt.Errorf("authorize CIMD client: %w", err)
	}
	defer func() { _ = authorizeResp.Body.Close() }()
	if authorizeResp.StatusCode != http.StatusFound {
		return "", "", fmt.Errorf("authorize CIMD client returned HTTP %d", authorizeResp.StatusCode)
	}
	consentLocation, err := url.Parse(authorizeResp.Header.Get("Location"))
	if err != nil {
		return "", "", fmt.Errorf("parse consent redirect: %w", err)
	}
	sessionToken := consentLocation.Query().Get("session_token")
	if sessionToken == "" {
		return "", "", fmt.Errorf("consent redirect has no session token")
	}

	consentDetailPath := fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agentID, url.QueryEscape(sessionToken))
	consentDetail, err := f.server.AuthenticatedGET(consentDetailPath, principal)
	if err != nil {
		return "", "", fmt.Errorf("load CIMD consent context: %w", err)
	}
	defer func() { _ = consentDetail.Body.Close() }()
	if consentDetail.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("load CIMD consent context returned HTTP %d", consentDetail.StatusCode)
	}

	grantBody, err := json.Marshal(map[string]any{
		"granted_permission_sets": map[string][]string{
			fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("marshal consent grant: %w", err)
	}
	grantResp, err := f.server.AuthenticatedPOST(
		fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agentID, url.QueryEscape(sessionToken)),
		principal,
		"application/json",
		bytes.NewReader(grantBody),
	)
	if err != nil {
		return "", "", fmt.Errorf("submit consent grant: %w", err)
	}
	defer func() { _ = grantResp.Body.Close() }()
	if grantResp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("consent grant returned HTTP %d", grantResp.StatusCode)
	}
	var grant map[string]any
	if err := json.NewDecoder(grantResp.Body).Decode(&grant); err != nil {
		return "", "", fmt.Errorf("decode consent grant: %w", err)
	}
	resumeURL, ok := grant["redirect_url"].(string)
	if !ok || resumeURL == "" {
		return "", "", fmt.Errorf("consent grant has no redirect URL")
	}
	codeResp, err := f.server.AuthenticatedGET(resumeURL, principal)
	if err != nil {
		return "", "", fmt.Errorf("resume CIMD authorization: %w", err)
	}
	defer func() { _ = codeResp.Body.Close() }()
	if codeResp.StatusCode != http.StatusFound {
		return "", "", fmt.Errorf("resume authorization returned HTTP %d", codeResp.StatusCode)
	}
	callback, err := url.Parse(codeResp.Header.Get("Location"))
	if err != nil {
		return "", "", fmt.Errorf("parse callback redirect: %w", err)
	}
	code := callback.Query().Get("code")
	if code == "" {
		return "", "", fmt.Errorf("callback redirect has no authorization code")
	}
	return code, verifier, nil
}

func (f *cimdPrivateKeyJWTFixture) CodeForm(code, verifier string) url.Values {
	return f.CodeFormFor(f.clientID, f.redirectURI, code, verifier)
}

func (f *cimdPrivateKeyJWTFixture) CodeFormFor(clientID, redirectURI, code, verifier string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
}

func (f *cimdPrivateKeyJWTFixture) RefreshForm(refreshToken string) url.Values {
	return f.RefreshFormFor(f.clientID, refreshToken)
}

func (f *cimdPrivateKeyJWTFixture) RefreshFormFor(clientID, refreshToken string) url.Values {
	return url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}
}

func (f *cimdPrivateKeyJWTFixture) PostToken(form url.Values, authorization string) (*http.Response, error) {
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	if authorization != "" {
		headers["Authorization"] = authorization
	}
	return f.server.DirectRequest("POST", "/oauth2/token", "", headers, strings.NewReader(form.Encode()))
}

func (f *cimdPrivateKeyJWTFixture) Close() {
	if f.server != nil {
		f.server.Close()
	}
	if f.metadataServer != nil {
		f.metadataServer.Close()
	}
	if f.storage != nil && f.storageFactory != nil {
		_ = f.storageFactory.CloseStorage(f.storage)
	}
}
