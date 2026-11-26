package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const (
	opaPackage            = "company.impersonation"
	opaExceptionalPackage = "company.impersonation.exceptional"
	opaBundleName         = "impersonation"
	opaEvaluateSpan       = "tokenexchange.impersonation.opa.evaluate"
	opaInternalError      = "internal server error during token exchange"
	opaDeniedDescription  = "token exchange is not authorized"
	opaHuman              = "alice@example.com"
	opaClient             = "gateway-1"
	opaActor              = "automation-worker"
)

// opaImpersonation is the per-test harness of the OPA impersonation journeys: fresh storage,
// issuer, bundle server, temporary files, and broker for every test.
type opaImpersonation struct {
	logger         *slog.Logger
	logs           *bootstrap.BufferedLogCapture
	storageFactory *bootstrap.StorageFactory
	storage        *storageadapter.Adapter
	issuer         *helpers.MockJWKSServer
	bundles        *helpers.OPABundleServer
	dir            string
	agent          *storage.Agent
	admin          *bootstrap.TestServer
	enduser        *bootstrap.TestServer
	now            time.Time
}

func newOPAImpersonation() *opaImpersonation {
	j := &opaImpersonation{now: time.Now(), dir: GinkgoT().TempDir()}
	j.logger, j.logs = bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
	j.storageFactory = bootstrap.NewStorageFactory(j.logger)
	var err error
	j.storage, err = j.storageFactory.NewTestStorage()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(j.storageFactory.CloseStorage(j.storage)).To(Succeed()) })
	j.issuer = helpers.NewMockJWKSServer()
	DeferCleanup(j.issuer.Close)
	j.bundles = helpers.NewOPABundleServer()
	DeferCleanup(j.bundles.Close)

	j.agent = j.registerAgent("inventory-assistant", ptrExternalID("inventory-agent"))
	j.seedDelegation(opaHuman)
	return j
}

func ptrExternalID(value string) *id.ExternalID {
	external := id.NewExternalID(value)
	return &external
}

func (j *opaImpersonation) registerAgent(canonicalID string, externalID *id.ExternalID) *storage.Agent {
	agent := fixtures.ImpersonationTarget(canonicalID)
	agent.AllowedScopes = []string{"read"}
	agent.ExternalID = externalID
	Expect(j.storage.Agents().Create(context.Background(), agent)).To(Succeed())
	return agent
}

func (j *opaImpersonation) seedDelegation(principal string) {
	Expect(j.storage.UserGrants().Create(context.Background(), fixtures.ActiveGrant(
		principal, j.agent.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"},
	))).To(Succeed())
}

func (j *opaImpersonation) rule(name string, categories ...string) fixtures.ImpersonationRuleSpec {
	return fixtures.ImpersonationRuleSpec{Name: name, IssuerURI: j.issuer.URL(), JWKSURI: j.issuer.JWKSURL(), Categories: categories, Email: true}
}

// config is a local-mode configuration with the given rules and no OPA engine.
func (j *opaImpersonation) config(rules ...ports.ImpersonationRuleConfig) *ports.Config {
	config := fixtures.LocalConfig()
	config.Security.SkipThirdpartyHTTPSValidation = true
	config.OAuth2AuthServer.Local.TokenClaimsExpression = `principal.email == "" ? {"aud": "https://inventory.example.com"} : {"aud": "https://inventory.example.com", "email": principal.email}`
	config.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{AudiencePrefix: fixtures.ImpersonationAudience, Rules: rules}
	return config
}

// managed adds the shared engine in managed mode with a bootstrap for the named bundles.
func (j *opaImpersonation) managed(config *ports.Config, startupTimeout time.Duration, bundleNames ...string) *ports.Config {
	config.OAuth2AuthServer.Impersonation.OPA = ports.ImpersonationOPAEngineConfig{
		Policy:         ports.ImpersonationOPAPolicySource{ConfigFile: j.writeBootstrap("", bundleNames...)},
		StartupTimeout: startupTimeout,
	}
	return config
}

// writeBootstrap writes a bootstrap for the bundle server. credentials replaces the service's
// credentials block when non-empty.
func (j *opaImpersonation) writeBootstrap(credentials string, bundleNames ...string) string {
	if len(bundleNames) == 0 {
		bundleNames = []string{opaBundleName}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "services:\n  governance:\n    url: %s\n", j.bundles.URL())
	if credentials != "" {
		b.WriteString(credentials)
	}
	b.WriteString("bundles:\n")
	for _, name := range bundleNames {
		fmt.Fprintf(&b, "  %s:\n    service: governance\n    resource: %s\n    polling:\n      min_delay_seconds: 1\n      max_delay_seconds: 1\n", name, j.bundles.Resource(name))
	}
	return j.writeFile("bootstrap.yaml", b.String())
}

func (j *opaImpersonation) writeFile(name, content string) string {
	path := filepath.Join(j.dir, name)
	Expect(os.MkdirAll(filepath.Dir(path), 0o700)).To(Succeed())
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	return path
}

func opaFixture(name string) string {
	raw, err := os.ReadFile(filepath.Join("fixtures", "opa", "impersonation", name))
	Expect(err).NotTo(HaveOccurred())
	return string(raw)
}

func opaFixtureData(name string) map[string]any {
	var data map[string]any
	Expect(json.Unmarshal([]byte(opaFixture(name)), &data)).To(Succeed())
	return data
}

// publish publishes the named fixture modules and data as a new revision of the bundle.
func (j *opaImpersonation) publish(bundleName, revision string, data map[string]any, modules ...string) {
	files := map[string]string{}
	for _, module := range modules {
		files["/"+module] = opaFixture(module)
	}
	Expect(j.bundles.Publish(bundleName, helpers.OPABundle{Modules: files, Data: data, Revision: revision})).To(Succeed())
}

// build starts the broker. It returns the BuildApp error so startup scenarios can assert it.
func (j *opaImpersonation) build(config *ports.Config) error {
	return j.buildWith(config, nil)
}

func (j *opaImpersonation) buildWith(config *ports.Config, tp *sdktrace.TracerProvider) error {
	factory := bootstrap.NewServerFactory(config, j.logger)
	var err error
	var application *app.App
	if tp != nil {
		application, err = factory.BuildAppWithTracerProvider(j.storage, tp)
	} else {
		application, err = factory.BuildApp(j.storage)
	}
	if err != nil {
		return err
	}
	j.admin, err = bootstrap.NewAdminTestServer(application, j.logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(j.admin.Close)
	j.enduser, err = bootstrap.NewEndUserTestServer(application, j.logger)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(j.enduser.Close)
	Expect(helpers.ProvisionSigningKey(j.admin.BaseURL())).To(Succeed())
	return nil
}

func (j *opaImpersonation) boot(config *ports.Config) {
	Expect(j.build(config)).To(Succeed())
}

func (j *opaImpersonation) targetAudience(agent *storage.Agent) string {
	return fixtures.ImpersonationAudience + "/" + agent.ID.String()
}

// sign issues a signed credential. A nil audience omits aud.
func (j *opaImpersonation) sign(sub string, audience *string, extra map[string]any) string {
	claims := map[string]any{"iss": j.issuer.URL(), "sub": sub, "iat": j.now.Unix(), "exp": j.now.Add(time.Hour).Unix()}
	if audience != nil {
		claims["aud"] = *audience
	}
	for key, value := range extra {
		claims[key] = value
	}
	token, err := j.issuer.SignJWT(claims)
	Expect(err).NotTo(HaveOccurred())
	return token
}

func (j *opaImpersonation) signed(sub string, extra map[string]any) string {
	audience := fixtures.ImpersonationAudience
	return j.sign(sub, &audience, extra)
}

// request is Journey 1's exchange for the target agent.
func (j *opaImpersonation) request(agent *storage.Agent) url.Values {
	return url.Values{
		"grant_type":            {imperGrant},
		"audience":              {j.targetAudience(agent)},
		"scope":                 {"read"},
		"client_assertion_type": {imperBearerTyp},
		"client_assertion":      {j.signed(opaClient, nil)},
		"actor_token_type":      {imperJWTType},
		"actor_token":           {j.signed(opaActor, nil)},
		"subject_token_type":    {imperJWTType},
		"subject_token":         {j.signed(opaHuman, nil)},
	}
}

// exchange posts the form and returns the status and decoded body.
func (j *opaImpersonation) exchange(form url.Values) (int, map[string]any) {
	return j.exchangeWithHeaders(form, nil, "")
}

func (j *opaImpersonation) exchangeWithHeaders(form url.Values, headers http.Header, rawQuery string) (int, map[string]any) {
	target := j.enduser.BaseURL() + "/oauth2/token"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
	return resp.StatusCode, body
}

// status returns only the status of an exchange, for Eventually.
func (j *opaImpersonation) status(form func() url.Values) func() int {
	return func() int {
		status, _ := j.exchange(form())
		return status
	}
}

func (j *opaImpersonation) expectToken(status int, body map[string]any, sub string) map[string]any {
	Expect(status).To(Equal(http.StatusOK), "body: %v", body)
	Expect(body["token_type"]).To(Equal("Bearer"))
	Expect(body["issued_token_type"]).To(Equal(imperAccessTyp))
	claims := decodeJWTClaims(body["access_token"].(string))
	Expect(claims["sub"]).To(Equal(sub))
	Expect(claims["agent_id"]).To(Equal(j.agent.ID.String()))
	Expect(claims["act"]).To(Equal(map[string]any{"iss": j.issuer.URL(), "sub": opaActor}))
	Expect(claims["scope"]).To(Equal("read"))
	return claims
}

func expectOAuthError(status int, body map[string]any, wantStatus int, code, description string) {
	Expect(status).To(Equal(wantStatus), "body: %v", body)
	Expect(body["error"]).To(Equal(code))
	if description != "" {
		Expect(body["error_description"]).To(Equal(description))
	}
	Expect(body).NotTo(HaveKey("access_token"))
}

// audits returns the impersonation.decision.recorded records.
func (j *opaImpersonation) audits() []map[string]any {
	return j.events("impersonation.decision.recorded")
}

func (j *opaImpersonation) events(event string) []map[string]any {
	records, err := j.logs.Records()
	Expect(err).NotTo(HaveOccurred())
	var matched []map[string]any
	for _, record := range records {
		if record["event"] == event {
			matched = append(matched, record)
		}
	}
	return matched
}

func (j *opaImpersonation) lastAudit() map[string]any {
	audits := j.audits()
	Expect(audits).NotTo(BeEmpty())
	return audits[len(audits)-1]
}

// agentHumanState counts the grants and sessions whose principal is the target agent's UUID.
func (j *opaImpersonation) agentHumanState() int {
	principal := id.Principal(j.agent.ID.String())
	grants, err := j.storage.UserGrants().ListByPrincipalAndAgent(context.Background(), principal, j.agent.ID)
	Expect(err).NotTo(HaveOccurred())
	sessions, err := j.storage.UserSessions().ListByPrincipal(context.Background(), principal)
	Expect(err).NotTo(HaveOccurred())
	return len(grants) + len(sessions)
}

func (j *opaImpersonation) governanceData() map[string]any {
	return opaFixtureData("data.json")
}

func unsignedJWT(claims map[string]any) string {
	token, err := helpers.CreateUnsignedJWT(claims)
	Expect(err).NotTo(HaveOccurred())
	return token
}

// tamperSignature flips the last signature character so the JWT no longer verifies.
func tamperSignature(token string) string {
	last := token[len(token)-1]
	replacement := byte('A')
	if last == 'A' {
		replacement = 'B'
	}
	return token[:len(token)-1] + string(replacement)
}

func joinRoles(roles []string) string {
	return strings.Join(roles, ", ")
}

func (j *opaImpersonation) expectHumanToken(status int, body map[string]any) map[string]any {
	return j.expectToken(status, body, opaHuman)
}

func spanAttribute(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.String()
		}
	}
	return ""
}
