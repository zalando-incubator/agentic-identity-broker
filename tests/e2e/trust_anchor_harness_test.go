package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const (
	// trustAnchorLabel selects the 29 scenario It() blocks of feature 052.
	trustAnchorLabel = "inline-jwks-trust-anchors"
	// trustAnchorParityLabel selects the SC-003 parity table.
	trustAnchorParityLabel = "inline-jwks-trust-anchors-parity"

	gatewayKID        = "agentgateway-2026-10"
	gatewaySubject    = "agentgateway"
	brokerAudience    = "token-exchange-broker"
	githubResourceURI = "https://api.github.com"
	brokerOwnIssuer   = "http://localhost:8000"

	// Short bounds let Eventually observe file re-reads and fail-closed expiry.
	fastFileMinRefresh = 200 * time.Millisecond
	fastFileMaxRefresh = 2 * time.Second
)

// trustAnchorEnv is a per-test broker built through production bootstrap, with a gateway that
// signs locally and an issuer host that counts every request made to it.
type trustAnchorEnv struct {
	logger          *slog.Logger
	logs            *bootstrap.BufferedLogCapture
	storageFactory  *bootstrap.StorageFactory
	storage         *storageadapter.Adapter
	upstream        *helpers.MockUpstreamOAuth2Server
	issuer          *helpers.CountingServer
	gateway         *helpers.LocalKey
	trust           *helpers.TrustDir
	principal       string
	agent           *domainstorage.Agent
	githubServiceID string
	clientSecret    string
	app             *app.App
	adminServer     *bootstrap.TestServer
	enduserServer   *bootstrap.TestServer
}

const localAgentCallback = "http://localhost:9999/callback"

// newTrustAnchorEnv seeds an agent, a GitHub service, a grant, and a session. Call close in AfterEach.
func newTrustAnchorEnv() *trustAnchorEnv {
	e := &trustAnchorEnv{}
	e.logger, e.logs = bootstrap.NewBufferedJSONLogger(slog.LevelDebug)
	e.storageFactory = bootstrap.NewStorageFactory(e.logger)
	e.upstream = helpers.NewMockUpstreamOAuth2Server()
	e.issuer = helpers.NewCountingServer()
	e.gateway = helpers.NewLocalES256Key(gatewayKID)
	e.trust = helpers.NewTrustDir(GinkgoT().TempDir())
	e.principal = fixtures.DefaultPrincipal().String()

	var err error
	e.storage, err = e.storageFactory.NewTestStorage()
	Expect(err).ToNot(HaveOccurred())

	ctx := context.Background()
	e.agent = fixtures.LocalAgent()
	e.agent.RedirectURIs = []string{localAgentCallback}
	Expect(e.storage.Agents().Create(ctx, e.agent)).To(Succeed())

	github := fixtures.GitHubService()
	github.Endpoints.TokenEndpoint = e.upstream.URL() + "/oauth/token"
	github.Endpoints.AuthorizeEndpoint = e.upstream.URL() + "/oauth/authorize"
	Expect(e.storage.Services().Create(ctx, github)).To(Succeed())
	e.githubServiceID = github.ID.String()
	Expect(fixtures.SeedPlaceholderGrantData(ctx, e.storage, github.ID)).To(Succeed())
	grant := fixtures.ActiveGrant(e.principal, e.agent.ID.String(), github.ID.String(), []string{"repo", "user"})
	Expect(e.storage.UserGrants().Create(ctx, grant)).To(Succeed())
	Expect(e.storage.UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(e.principal))).To(Succeed())
	return e
}

func (e *trustAnchorEnv) close() {
	if e.app != nil && e.app.Shutdown != nil {
		_ = e.app.Shutdown(context.Background())
	}
	for _, server := range []*bootstrap.TestServer{e.adminServer, e.enduserServer} {
		if server != nil {
			server.Close()
		}
	}
	if e.issuer != nil {
		e.issuer.Close()
	}
	if e.upstream != nil {
		e.upstream.Close()
	}
	if e.storage != nil {
		_ = e.storageFactory.CloseStorage(e.storage)
	}
}

func tokenExchangeDefaults() ports.TokenExchangeConfig {
	return ports.TokenExchangeConfig{
		ClaimExtraction: ports.ClaimExtractionConfig{
			PrincipalExpression: "subject_token.sub",
			AgentIDExpression:   "subject_token.azp",
		},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL:  ports.CELAuthorizationConfig{Expression: "true", EvaluationTimeout: 2 * time.Second},
		},
	}
}

// localConfig builds a local-mode config whose client-assertion trust anchor is clientAssertion.
func (e *trustAnchorEnv) localConfig(clientAssertion ports.ClientAssertionTrustConfig) *ports.Config {
	config := fixtures.LocalConfig()
	config.OAuth2AuthServer.Local.TokenClaimsExpression = `{"azp": agent.id, "aud": "token-exchange-broker"}`
	config.TokenExchange = tokenExchangeDefaults()
	config.TokenExchange.ClientAssertion = clientAssertion
	config.Security.TrustMaterialDir = e.trust.Path
	return config
}

// hybridConfig builds a hybrid-mode config against the mock upstream.
func (e *trustAnchorEnv) hybridConfig(clientAssertion ports.ClientAssertionTrustConfig) *ports.Config {
	config := fixtures.HybridConfig(e.upstream.Server.URL)
	config.OAuth2AuthServer.Local.TokenClaimsExpression = `{"azp": agent.id, "aud": "token-exchange-broker"}`
	config.TokenExchange = tokenExchangeDefaults()
	config.TokenExchange.ClientAssertion = clientAssertion
	config.Security.TrustMaterialDir = e.trust.Path
	return config
}

// proxyConfig builds a proxy-mode config with approval authentication against the mock upstream.
func (e *trustAnchorEnv) proxyConfig(clientAssertion ports.ClientAssertionTrustConfig) *ports.Config {
	config := fixtures.OAuth2ConfigWithTokenExchange(e.upstream.URL())
	config.Approvals.SyncCoalesceWindow = 0
	config.TokenExchange.ClientAssertion = clientAssertion
	config.Security.TrustMaterialDir = e.trust.Path
	return config
}

// build runs production bootstrap and returns the BuildApp error. It does not start servers.
func (e *trustAnchorEnv) build(config *ports.Config) error {
	built, err := bootstrap.NewServerFactory(config, e.logger).BuildApp(e.storage)
	if err != nil {
		return err
	}
	e.app = built
	return nil
}

// start builds the app and starts the admin and end-user test servers. Modes that issue tokens
// locally also get a signing key and agent credentials.
func (e *trustAnchorEnv) start(config *ports.Config) {
	Expect(e.build(config)).To(Succeed())
	var err error
	e.adminServer, err = bootstrap.NewAdminTestServer(e.app, e.logger)
	Expect(err).ToNot(HaveOccurred())
	e.enduserServer, err = bootstrap.NewEndUserTestServer(e.app, e.logger)
	Expect(err).ToNot(HaveOccurred())
	if config.OAuth2AuthServer.Mode == "proxy" {
		return
	}
	Expect(helpers.ProvisionSigningKey(e.adminServer.BaseURL())).To(Succeed())

	resp, err := helpers.HTTPClient().Post(
		e.adminServer.BaseURL()+"/api/agents/"+e.agent.ID.String()+"/client-credentials", "application/json", nil)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	var creds map[string]any
	Expect(json.NewDecoder(resp.Body).Decode(&creds)).To(Succeed())
	e.clientSecret, _ = creds["client_secret"].(string)
	Expect(e.clientSecret).ToNot(BeEmpty())
}

// expectStartupFailure builds config and asserts BuildApp fails with every substring in want.
func (e *trustAnchorEnv) expectStartupFailure(config *ports.Config, want ...string) error {
	err := e.build(config)
	Expect(err).To(HaveOccurred(), "BuildApp unexpectedly succeeded")
	for _, fragment := range want {
		Expect(err.Error()).To(ContainSubstring(fragment))
	}
	return err
}

// expectNoKeyMaterial asserts that neither the error text nor the captured logs contain any
// base64url x, y, n, or d member value of the keys.
func (e *trustAnchorEnv) expectNoKeyMaterial(err error, keys ...*helpers.LocalKey) {
	logs := e.logs.Raw()
	for _, key := range keys {
		for _, value := range key.MemberValues() {
			if err != nil {
				Expect(err.Error()).ToNot(ContainSubstring(value))
			}
			Expect(logs).ToNot(ContainSubstring(value))
		}
	}
}

// assertion signs a client assertion with key for issuer.
func (e *trustAnchorEnv) assertion(key *helpers.LocalKey, issuer string, mutate ...func(map[string]any)) string {
	now := time.Now()
	claims := map[string]any{
		"iss": issuer,
		"sub": gatewaySubject,
		"aud": brokerAudience,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
	for _, change := range mutate {
		change(claims)
	}
	token, err := key.Sign(claims)
	Expect(err).ToNot(HaveOccurred())
	return token
}

// upstreamSubjectToken signs a subject token with the mock upstream's key (proxy and hybrid modes).
func (e *trustAnchorEnv) upstreamSubjectToken() string {
	now := time.Now()
	token, err := helpers.SignTestJWT(map[string]any{
		"sub": e.principal,
		"azp": e.agent.ID.String(),
		"iss": e.upstream.URL(),
		"aud": brokerAudience,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}, e.upstream.GetPrivateKeyPEM())
	Expect(err).ToNot(HaveOccurred())
	return token
}

// mintLocalSubjectToken runs the authorization-code flow and returns a broker-minted access token.
func (e *trustAnchorEnv) mintLocalSubjectToken() string {
	verifier := helpers.PKCEVerifier()
	authorizeURL := "/oauth2/authorize?" + url.Values{
		"client_id":             {e.agent.ID.String()},
		"redirect_uri":          {localAgentCallback},
		"response_type":         {"code"},
		"state":                 {"trust-anchor-state"},
		"code_challenge":        {helpers.GenerateCodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode()

	resp, err := e.enduserServer.AuthenticatedGET(authorizeURL, e.principal)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusFound))
	location := resp.Header.Get("Location")

	var code string
	if strings.HasPrefix(location, localAgentCallback) {
		redirect, err := url.Parse(location)
		Expect(err).ToNot(HaveOccurred())
		code = redirect.Query().Get("code")
	} else {
		code = e.consentAndAuthorize(location)
	}
	Expect(code).ToNot(BeEmpty())

	tokenResp, err := helpers.HTTPClient().Post(
		e.enduserServer.BaseURL()+"/oauth2/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {e.agent.ID.String()},
			"client_secret": {e.clientSecret},
			"code":          {code},
			"redirect_uri":  {localAgentCallback},
			"code_verifier": {verifier},
		}.Encode()),
	)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = tokenResp.Body.Close() }()
	Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))
	var body map[string]any
	Expect(json.NewDecoder(tokenResp.Body).Decode(&body)).To(Succeed())
	accessToken, _ := body["access_token"].(string)
	Expect(accessToken).ToNot(BeEmpty())
	return accessToken
}

func (e *trustAnchorEnv) consentAndAuthorize(consentLocation string) string {
	consentURL, err := url.Parse(consentLocation)
	Expect(err).ToNot(HaveOccurred())
	sessionToken := consentURL.Query().Get("session_token")
	Expect(sessionToken).ToNot(BeEmpty())

	grantBody, err := json.Marshal(map[string]any{
		"granted_permission_sets": map[string][]string{
			fixtures.PlaceholderPermissionSetID.String(): {e.githubServiceID},
		},
	})
	Expect(err).ToNot(HaveOccurred())
	grantResp, err := e.enduserServer.AuthenticatedPOST(
		fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", e.agent.ID, url.QueryEscape(sessionToken)),
		e.principal, "application/json", bytes.NewReader(grantBody))
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = grantResp.Body.Close() }()
	Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
	var grantBodyResp map[string]any
	Expect(json.NewDecoder(grantResp.Body).Decode(&grantBodyResp)).To(Succeed())
	reAuthorize, _ := grantBodyResp["redirect_url"].(string)
	Expect(reAuthorize).ToNot(BeEmpty())

	codeResp, err := e.enduserServer.AuthenticatedGET(reAuthorize, e.principal)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = codeResp.Body.Close() }()
	Expect(codeResp.StatusCode).To(Equal(http.StatusFound))
	codeLocation, err := url.Parse(codeResp.Header.Get("Location"))
	Expect(err).ToNot(HaveOccurred())
	return codeLocation.Query().Get("code")
}

// exchange posts an RFC 8693 token exchange for the GitHub resource.
func (e *trustAnchorEnv) exchange(subjectToken, clientAssertion string) *http.Response {
	resp, err := e.enduserServer.PublicPOST(
		"/oauth2/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(url.Values{
			"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
			"subject_token":         {subjectToken},
			"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {clientAssertion},
			"resource":              {githubResourceURI},
		}.Encode()),
	)
	Expect(err).ToNot(HaveOccurred())
	return resp
}

// decodeBody reads and closes a JSON response body.
func decodeResponseBody(resp *http.Response) map[string]any {
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return body
}

// healthComponents returns the end-user /health components map.
func (e *trustAnchorEnv) healthComponents() map[string]any {
	resp, err := e.enduserServer.PublicGET("/health")
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	components, _ := decodeResponseBody(resp)["components"].(map[string]any)
	return components
}

// logRecords returns the captured structured log records whose msg equals message.
func (e *trustAnchorEnv) logRecords(message string) []map[string]any {
	records, err := e.logs.Records()
	Expect(err).ToNot(HaveOccurred())
	var matched []map[string]any
	for _, record := range records {
		if record["msg"] == message {
			matched = append(matched, record)
		}
	}
	return matched
}
