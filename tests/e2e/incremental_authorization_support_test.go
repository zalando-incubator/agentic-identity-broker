package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const incrementalRedirectURI = "https://client.example.com/cb"

// incrementalJourney drives the specs/055-incremental-service-authorization journeys through
// the production broker: authorization, consent, provider sign-in, token and exchange endpoints.
type incrementalJourney struct {
	store     *storageadapter.Adapter
	enduser   *bootstrap.TestServer
	admin     *bootstrap.TestServer
	provider  *helpers.MockUpstreamOAuth2Server
	upstream  *helpers.MockUpstreamOAuth2Server
	config    *ports.Config
	cast      *fixtures.IncrementalCast
	principal string
	tokens    *TokenFixtures
}

// newIncrementalJourney builds a fresh broker in local, hybrid or proxy mode. The mock
// provider serves every cast service; proxy and hybrid modes get a separate mock upstream IdP.
// configure may adjust the cast before the application is built.
func newIncrementalJourney(mode string, configure func(*incrementalJourney)) *incrementalJourney {
	journey := &incrementalJourney{principal: fixtures.DefaultPrincipal().String()}
	logger := bootstrap.TestLogger(slog.LevelWarn)
	journey.provider = helpers.NewMockUpstreamOAuth2Server().WithAccessToken("synthetic-github-access").WithTokenScope("repo user:email")
	DeferCleanup(journey.provider.Close)

	switch mode {
	case "local":
		journey.config = fixtures.LocalConfig()
	case "hybrid", "proxy":
		journey.upstream = helpers.NewMockUpstreamOAuth2Server().WithRefreshToken("synthetic-upstream-rotated-refresh")
		DeferCleanup(journey.upstream.Close)
		journey.config = fixtures.HybridConfig(journey.upstream.URL())
		if mode == "proxy" {
			journey.config = fixtures.OAuth2ConfigWithUpstream(journey.upstream.URL())
		}
	default:
		Fail("unknown OAuth2 mode " + mode)
	}
	issuer := journey.provider
	if journey.upstream != nil {
		issuer = journey.upstream
	}
	if mode != "proxy" {
		journey.config.OAuth2AuthServer.Local.TokenClaimsExpression = `{"azp": agent.id, "aud": "token-exchange-broker"}`
	}
	journey.config.TokenExchange = ports.TokenExchangeConfig{
		ClientAssertion: ports.ClientAssertionTrustConfig{IssuerURI: issuer.URL()},
		ClaimExtraction: ports.ClaimExtractionConfig{
			PrincipalExpression: "subject_token.sub",
			AgentIDExpression:   "subject_token.azp",
		},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL:  ports.CELAuthorizationConfig{Expression: "true", EvaluationTimeout: 2 * time.Second},
		},
	}

	var err error
	journey.store, err = bootstrap.NewStorageFactory(logger).NewTestStorage()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(journey.store.Close(context.Background())).To(Succeed()) })
	ctx := context.Background()
	journey.cast, err = fixtures.SeedIncrementalCast(ctx, journey.store, journey.principal, journey.provider.URL())
	Expect(err).NotTo(HaveOccurred())
	if configure != nil {
		configure(journey)
	}
	Expect(journey.store.Agents().Update(ctx, journey.cast.Agent)).To(Succeed())

	application, err := bootstrap.NewServerFactory(journey.config, logger).BuildApp(journey.store)
	Expect(err).NotTo(HaveOccurred())
	journey.admin, err = bootstrap.NewAdminTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(journey.admin.Close)
	journey.enduser, err = bootstrap.NewEndUserTestServer(application, logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(journey.enduser.Close)
	if mode != "proxy" {
		Expect(helpers.ProvisionSigningKey(journey.admin.BaseURL())).To(Succeed())
	}
	journey.tokens = generateTokenFixtures(issuer, journey.principal, journey.cast.Agent)
	return journey
}

func (j *incrementalJourney) scopeURL(identifier string) string {
	return j.config.Server.EndUser.PublicURL + "/sessions/" + identifier
}

func (j *incrementalJourney) githubScope() string {
	return j.scopeURL("github")
}

func (j *incrementalJourney) clientID() string {
	return j.cast.Agent.ID.String()
}

// putSession replaces the principal's session for the session's service.
func (j *incrementalJourney) putSession(session *storagedomain.UserSession) {
	Expect(j.store.UserSessions().Create(context.Background(), session)).To(Succeed())
}

func (j *incrementalJourney) usableGitHub() {
	j.putSession(fixtures.SessionForServiceWithScopes(j.principal, j.cast.GitHub.ID.String(), []string{"repo", "user:email"}))
}

func (j *incrementalJourney) rejectedGitHub() {
	j.putSession(fixtures.RejectedRefreshSession(j.principal, j.cast.GitHub.ID.String(), []string{"repo", "user:email"}))
}

func (j *incrementalJourney) currentGrant() *storagedomain.UserGrant {
	grant, err := j.store.UserGrants().FindByPrincipalAndAgent(context.Background(), id.Principal(j.principal), j.cast.Agent.ID)
	Expect(err).NotTo(HaveOccurred())
	return grant
}

func (j *incrementalJourney) updateAgent(change func(*storagedomain.Agent)) {
	ctx := context.Background()
	agent, err := j.store.Agents().Get(ctx, j.cast.Agent.ID)
	Expect(err).NotTo(HaveOccurred())
	change(agent)
	Expect(j.store.Agents().Update(ctx, agent)).To(Succeed())
	j.cast.Agent = agent
}

func (j *incrementalJourney) renameGitHub(canonicalID *string) {
	ctx := context.Background()
	service, err := j.store.Services().Get(ctx, j.cast.GitHub.ID)
	Expect(err).NotTo(HaveOccurred())
	service.CanonicalID = canonicalID
	service.ClearCanonicalID = canonicalID == nil
	Expect(j.store.Services().Update(ctx, service, nil)).To(Succeed())
}

// authorize sends an authorization request and returns the redirect Location and PKCE verifier.
func (j *incrementalJourney) authorize(scope string) (*url.URL, string) {
	verifier := helpers.PKCEVerifier()
	query := url.Values{
		"response_type": {"code"}, "client_id": {j.clientID()},
		"redirect_uri": {incrementalRedirectURI}, "state": {"incremental-state"},
		"code_challenge": {helpers.GenerateCodeChallenge(verifier)}, "code_challenge_method": {"S256"},
	}
	if scope != "" {
		query.Set("scope", scope)
	}
	return j.follow("/oauth2/authorize?" + query.Encode()), verifier
}

// follow issues an authenticated GET and returns the redirect Location.
func (j *incrementalJourney) follow(requestURI string) *url.URL {
	response, err := j.enduser.AuthenticatedGET(requestURI, j.principal)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	location, err := helpers.ExtractRedirectURL(response)
	Expect(err).NotTo(HaveOccurred())
	return location
}

func (j *incrementalJourney) expectConsentRedirect(location *url.URL) string {
	Expect(location.Path).To(Equal("/agents/" + j.clientID()))
	Expect(location.Query().Get("code")).To(BeEmpty())
	token := location.Query().Get("session_token")
	Expect(token).NotTo(BeEmpty())
	return token
}

func (j *incrementalJourney) expectClientRedirect(location *url.URL) url.Values {
	Expect(location.Scheme + "://" + location.Host + location.Path).To(Equal(incrementalRedirectURI))
	Expect(location.Query().Get("state")).To(Equal("incremental-state"))
	Expect(location.Query().Get("session_token")).To(BeEmpty())
	return location.Query()
}

func (j *incrementalJourney) expectAuthorizationError(location *url.URL, code, description string) {
	query := j.expectClientRedirect(location)
	Expect(query.Get("code")).To(BeEmpty())
	Expect(query.Get("error")).To(Equal(code))
	Expect(query.Get("error_description")).To(Equal(description))
}

func (j *incrementalJourney) consentDetail(sessionToken string) map[string]any {
	response, err := j.enduser.AuthenticatedGET("/api/consent/agents/"+j.clientID()+"?session_token="+url.QueryEscape(sessionToken), j.principal)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK))
	body := decodeJSON[map[string]any](response)
	data, ok := body["data"].(map[string]any)
	Expect(ok).To(BeTrue())
	return data
}

// coverage returns the requested_service_coverage entry for the service.
func coverageFor(data map[string]any, serviceID id.ServiceID) map[string]any {
	Expect(data).To(HaveKey("requested_service_coverage"))
	for _, entry := range data["requested_service_coverage"].([]any) {
		coverage := entry.(map[string]any)
		if coverage["service_id"] == serviceID.String() {
			return coverage
		}
	}
	Fail("no requested_service_coverage entry for " + serviceID.String())
	return nil
}

func candidateFor(coverage map[string]any, setID id.PermissionSetID) map[string]any {
	for _, entry := range coverage["covering_permission_sets"].([]any) {
		candidate := entry.(map[string]any)
		if candidate["permission_set_id"] == setID.String() {
			return candidate
		}
	}
	Fail("no covering_permission_sets entry for " + setID.String())
	return nil
}

// submit posts a grant selection with the sealed session token.
func (j *incrementalJourney) submit(sessionToken string, selection map[id.PermissionSetID][]id.ServiceID, validUntil *time.Time) (int, map[string]any) {
	sets := map[string][]string{}
	for setID, services := range selection {
		sets[setID.String()] = []string{}
		for _, serviceID := range services {
			sets[setID.String()] = append(sets[setID.String()], serviceID.String())
		}
	}
	payload := map[string]any{"granted_permission_sets": sets}
	if validUntil != nil {
		payload["valid_until"] = validUntil.UTC().Format(time.RFC3339)
	}
	encoded, err := json.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())
	response, err := j.enduser.AuthenticatedPOST("/api/consent/agents/"+j.clientID()+"/grants?session_token="+url.QueryEscape(sessionToken), j.principal, "application/json", bytes.NewReader(encoded))
	Expect(err).NotTo(HaveOccurred())
	return response.StatusCode, decodeJSON[map[string]any](response)
}

func (j *incrementalJourney) grantSelection(sets ...*storagedomain.PermissionSet) map[id.PermissionSetID][]id.ServiceID {
	selection := map[id.PermissionSetID][]id.ServiceID{}
	for _, set := range sets {
		for _, scope := range set.ServiceScopes {
			selection[set.ID] = append(selection[set.ID], scope.ServiceID)
		}
	}
	return selection
}

// expectSaved asserts a successful Save and returns the protected continuation path.
func (j *incrementalJourney) expectSaved(status int, body map[string]any, sessionToken string) string {
	Expect(status).To(Equal(http.StatusCreated), "consent error: %v", body)
	continuation, err := url.Parse(body["redirect_url"].(string))
	Expect(err).NotTo(HaveOccurred())
	Expect(continuation.Path).To(Equal("/oauth2/authorize"))
	Expect(continuation.Query().Get("session_token")).To(Equal(sessionToken))
	return continuation.RequestURI()
}

// connect completes provider sign-in for the service and returns the callback redirect.
func (j *incrementalJourney) connect(serviceID id.ServiceID, returnURL string) *url.URL {
	authorizationURL := j.follow("/api/third-party/" + serviceID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(returnURL))
	callback, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
	Expect(err).NotTo(HaveOccurred())
	return j.follow(callback.RequestURI())
}

func (j *incrementalJourney) postToken(form url.Values) (int, map[string]any) {
	form.Set("client_id", j.clientID())
	return j.postForm(form)
}

func (j *incrementalJourney) postForm(form url.Values) (int, map[string]any) {
	response, err := j.enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	Expect(err).NotTo(HaveOccurred())
	return response.StatusCode, decodeJSON[map[string]any](response)
}

func (j *incrementalJourney) redeem(code, verifier string) (int, map[string]any) {
	return j.postToken(url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {incrementalRedirectURI}, "code_verifier": {verifier},
	})
}

func (j *incrementalJourney) refresh(refreshToken, scope string) (int, map[string]any) {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	if scope != "" {
		form.Set("scope", scope)
	}
	return j.postToken(form)
}

// issue completes a satisfied authorization and code exchange, returning the token response.
func (j *incrementalJourney) issue(scope string) map[string]any {
	location, verifier := j.authorize(scope)
	code := j.expectClientRedirect(location).Get("code")
	Expect(code).NotTo(BeEmpty(), "authorization must issue a code, got %s", location)
	status, body := j.redeem(code, verifier)
	Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body)
	return body
}

// exchange performs the RFC 8693 exchange for GitHub with the broker-issued subject token.
func (j *incrementalJourney) exchange(subjectToken string) (int, map[string]any) {
	return j.postForm(url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {subjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {j.tokens.ClientAssertion},
		"resource":              {"https://api.github.com"},
	})
}

func scopeSet(value any) []string {
	text, ok := value.(string)
	Expect(ok).To(BeTrue(), "scope must be a string, got %v", value)
	return strings.Fields(text)
}
