package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// Impersonation mints local broker tokens, so the Virtual Service Scope rules of
// specs/055-incremental-service-authorization (FR-002, FR-003, FR-020) apply to it as they do to
// authorization and local refresh (ADR 050 decision 3).
var _ = Describe("Impersonation with Virtual Service Scopes", func() {
	var (
		journey *incrementalJourney
		issuer  *helpers.MockJWKSServer
	)

	BeforeEach(func() {
		issuer = helpers.NewMockJWKSServer()
		DeferCleanup(issuer.Close)
		journey = newIncrementalJourney("local", func(j *incrementalJourney) {
			j.cast.AddJiraRequirement()
			rule := imperSignedRule("internal-gateway", issuer.URL(), issuer.JWKSURL(), `client_assertion.sub == "gateway-prod"`)
			rule.AllowedSubjectCategories = []string{"human", "agent"}
			j.config.Security.SkipThirdpartyHTTPSValidation = true
			j.config.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{
				AudiencePrefix: imperAudience,
				Rules:          []ports.ImpersonationRuleConfig{rule},
			}
		})
	})

	sign := func(sub string) string {
		now := time.Now()
		token, err := issuer.SignJWT(map[string]any{
			"iss": issuer.URL(), "sub": sub, "aud": imperAudience,
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
		Expect(err).ToNot(HaveOccurred())
		return token
	}

	impersonate := func(subject, scope string) (int, map[string]any) {
		form := url.Values{
			"grant_type":            {imperGrant},
			"audience":              {imperAudience + "/" + journey.cast.Agent.ID.String()},
			"client_assertion_type": {imperBearerTyp},
			"client_assertion":      {sign("gateway-prod")},
			"actor_token_type":      {imperJWTType},
			"actor_token":           {sign("actor-1")},
			"subject_token_type":    {imperJWTType},
			"subject_token":         {sign(subject)},
			"scope":                 {scope},
		}
		resp, err := journey.enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		return resp.StatusCode, body
	}

	expectInvalidScope := func(status int, body map[string]any) {
		Expect(status).To(Equal(http.StatusBadRequest))
		Expect(body["error"]).To(Equal("invalid_scope"))
		Expect(body).ToNot(HaveKey("access_token"))
	}

	It("mints a covered, usable service scope in its requested form", func() {
		journey.usableGitHub()
		for _, scope := range []string{journey.githubScope(), journey.scopeURL(journey.cast.GitHub.ID.String())} {
			status, body := impersonate(journey.principal, scope)
			Expect(status).To(Equal(http.StatusOK), "scope %s: %v", scope, body)
			Expect(body["scope"]).To(Equal(scope))
			Expect(decodeJWTClaims(body["access_token"].(string))["scope"]).To(Equal(scope))
		}
	})

	DescribeTable("rejects a service scope that authorization would reject, minting nothing",
		func(identifier func() string) {
			journey.usableGitHub()
			expectInvalidScope(impersonate(journey.principal, journey.scopeURL(identifier())))
		},
		Entry("malformed identifier", func() string { return "github/" }),
		Entry("unknown service", func() string { return "unknown-service" }),
		Entry("eligible service the user's grant does not cover", func() string { return "jira" }),
	)

	It("rejects a covered service without a usable session", func() {
		expectInvalidScope(impersonate(journey.principal, journey.githubScope()))
	})

	It("never grants a service scope to an agent subject", func() {
		journey.usableGitHub()
		expectInvalidScope(impersonate(journey.cast.Agent.ID.String(), journey.githubScope()))
	})
})
