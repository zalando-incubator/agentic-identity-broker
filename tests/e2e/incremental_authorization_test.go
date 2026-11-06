package e2e_test

import (
	"context"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// Scenarios reference specs/055-incremental-service-authorization/spec.md.
var _ = Describe("Incremental Service Authorization", func() {
	var journey *incrementalJourney

	ordinary := "openid offline_access"
	notPermitted := "requested service scope is not permitted"

	Context("when the agent already holds an ordinary local refresh token and GitHub becomes unusable", func() {
		var refreshToken string

		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
			journey.usableGitHub()
			refreshToken = journey.issue(ordinary)["refresh_token"].(string)
			journey.rejectedGitHub()
		})

		// US1 S2 from specs/055-incremental-service-authorization/spec.md.
		It("refuses refresh widening with invalid_scope, keeps the refresh token valid, and sends interactive step-up to consent", func() {
			union := ordinary + " " + journey.githubScope()
			status, body := journey.refresh(refreshToken, union)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body).To(Equal(map[string]any{
				"error":             "invalid_scope",
				"error_description": "scope " + journey.githubScope() + " requires interactive authorization",
			}))

			status, body = journey.refresh(refreshToken, "")
			Expect(status).To(Equal(http.StatusOK), "the refused widening must not consume the token: %v", body)
			Expect(scopeSet(body["scope"])).To(ConsistOf("openid", "offline_access"))

			location, _ := journey.authorize(union)
			sessionToken := journey.expectConsentRedirect(location)
			data := journey.consentDetail(sessionToken)
			Expect(data["requested_service_ids"]).To(Equal([]any{journey.cast.GitHub.ID.String()}))
			Expect(coverageFor(data, journey.cast.GitHub.ID)).To(HaveKeyWithValue("usability", "unusable"))
		})
	})

	Context("when the grant covers GitHub but Alice has no GitHub session", func() {
		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
		})

		// US1 S3 from specs/055-incremental-service-authorization/spec.md.
		It("identifies the requested service and every covering set while preserving selections and end date", func() {
			before := journey.currentGrant()
			location, _ := journey.authorize(ordinary + " " + journey.githubScope())
			sessionToken := journey.expectConsentRedirect(location)

			data := journey.consentDetail(sessionToken)
			Expect(data["requested_service_ids"]).To(Equal([]any{journey.cast.GitHub.ID.String()}))
			coverage := coverageFor(data, journey.cast.GitHub.ID)
			Expect(coverage["required_scopes"]).To(ConsistOf("repo", "user:email"))
			Expect(coverage["usability"]).To(Equal("missing"))
			Expect(coverage["covering_permission_sets"]).To(HaveLen(2))
			read := candidateFor(coverage, journey.cast.GitHubRead.ID)
			Expect(read["supplied_required_scopes"]).To(ConsistOf("repo", "user:email"))
			Expect(read["usability"]).To(Equal("missing"))
			triage := candidateFor(coverage, journey.cast.IssueTriage.ID)
			Expect(triage["supplied_required_scopes"]).To(ConsistOf("repo"))
			Expect(triage["usability"]).To(Equal("missing"))

			status, body := journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead), before.ValidUntil)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("requested service not satisfied"))
			Expect(body["message"]).To(ContainSubstring(journey.cast.GitHub.ID.String()))
			Expect(body).NotTo(HaveKey("redirect_url"))
			after := journey.currentGrant()
			Expect(after.GrantedPermissionSets).To(Equal(before.GrantedPermissionSets))
			Expect(after.ValidUntil.Equal(*before.ValidUntil)).To(BeTrue())
		})
	})

	Context("when the grant does not include GitHub but a usable GitHub session exists", func() {
		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
			grant := journey.currentGrant()
			grant.GrantedPermissionSets = grant.GrantedPermissionSets[:1]
			Expect(journey.store.UserGrants().Create(context.Background(), grant)).To(Succeed())
			journey.usableGitHub()
		})

		// US1 S5 from specs/055-incremental-service-authorization/spec.md.
		It("rejects a grant submission that does not cover the requested service even with a usable session", func() {
			before := journey.currentGrant()
			location, verifier := journey.authorize(ordinary + " " + journey.githubScope())
			sessionToken := journey.expectConsentRedirect(location)
			coverage := coverageFor(journey.consentDetail(sessionToken), journey.cast.GitHub.ID)
			Expect(coverage["usability"]).To(Equal("usable"))

			status, body := journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess), before.ValidUntil)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("requested service not satisfied"))
			Expect(body).NotTo(HaveKey("redirect_url"))
			Expect(journey.currentGrant().GrantedPermissionSets).To(Equal(before.GrantedPermissionSets))

			status, body = journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead), before.ValidUntil)
			continuation := journey.expectSaved(status, body, sessionToken)
			code := journey.expectClientRedirect(journey.follow(continuation)).Get("code")
			Expect(code).NotTo(BeEmpty())
			status, body = journey.redeem(code, verifier)
			Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body)
			Expect(scopeSet(body["scope"])).To(ConsistOf("openid", "offline_access", journey.githubScope()))
		})
	})

	Context("when two services are requested and only GitHub is satisfied", func() {
		BeforeEach(func() {
			journey = newIncrementalJourney("local", func(j *incrementalJourney) { j.cast.AddJiraRequirement() })
			journey.usableGitHub()
		})

		// US1 S6 from specs/055-incremental-service-authorization/spec.md.
		It("requires every requested service and explains each unsatisfied one without changing agent requirements", func() {
			requirements := journey.cast.Agent.ServiceRequirements
			location, verifier := journey.authorize(ordinary + " " + journey.githubScope() + " " + journey.scopeURL("jira"))
			sessionToken := journey.expectConsentRedirect(location)
			data := journey.consentDetail(sessionToken)
			Expect(data["requested_service_ids"]).To(ConsistOf(journey.cast.GitHub.ID.String(), journey.cast.Jira.ID.String()))
			Expect(coverageFor(data, journey.cast.GitHub.ID)["usability"]).To(Equal("usable"))
			jira := coverageFor(data, journey.cast.Jira.ID)
			Expect(jira["usability"]).To(Equal("missing"))
			Expect(candidateFor(jira, journey.cast.JiraRead.ID)["supplied_required_scopes"]).To(ConsistOf("read:jira-work"))

			grant := journey.currentGrant()
			status, body := journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead), grant.ValidUntil)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["message"]).To(ContainSubstring(journey.cast.Jira.ID.String()))
			Expect(body["message"]).NotTo(ContainSubstring(journey.cast.GitHub.ID.String()))

			journey.provider.WithTokenScope("read:jira-work")
			journey.connect(journey.cast.Jira.ID, journey.config.Server.EndUser.PublicURL+"/agents/"+journey.clientID())
			status, body = journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead, journey.cast.JiraRead), grant.ValidUntil)
			code := journey.expectClientRedirect(journey.follow(journey.expectSaved(status, body, sessionToken))).Get("code")
			status, body = journey.redeem(code, verifier)
			Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body)
			Expect(scopeSet(body["scope"])).To(ContainElements(journey.githubScope(), journey.scopeURL("jira")))

			agent, err := journey.store.Agents().Get(context.Background(), journey.cast.Agent.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(agent.ServiceRequirements).To(Equal(requirements))
		})
	})

	Context("when GitHub access expired but the provider refresh succeeds", func() {
		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
			journey.putSession(fixtures.ExpiredAccessWithRefresh(journey.principal, journey.cast.GitHub.ID.String(), []string{"repo", "user:email"}))
		})

		// US1 S7 from specs/055-incremental-service-authorization/spec.md.
		It("uses a successfully refreshed session and continues without login", func() {
			body := journey.issue(ordinary + " " + journey.githubScope())
			Expect(scopeSet(body["scope"])).To(ContainElement(journey.githubScope()))
			Expect(journey.provider.GetTokenRequestsForGrant("refresh_token")).To(HaveLen(1))
			Expect(journey.provider.GetAuthorizationRequestCount()).To(BeZero())

			By("treating a valid token missing a selected permission-set scope as unusable under require_all_scopes")
			journey.updateAgent(func(agent *storagedomain.Agent) {
				agent.ServiceRequirements[0].RequiredScopes = nil
				agent.ServiceRequirements[0].RequireAllScopes = true
			})
			journey.putSession(fixtures.SessionForServiceWithScopes(journey.principal, journey.cast.GitHub.ID.String(), []string{"repo"}))
			location, _ := journey.authorize(ordinary + " " + journey.githubScope())
			coverage := coverageFor(journey.consentDetail(journey.expectConsentRedirect(location)), journey.cast.GitHub.ID)
			Expect(coverage["required_scopes"]).To(BeEmpty())
			Expect(candidateFor(coverage, journey.cast.GitHubRead.ID)["usability"]).To(Equal("unusable"))
			Expect(candidateFor(coverage, journey.cast.IssueTriage.ID)["usability"]).To(Equal("usable"))

			By("applying the same rule to agents without explicit requirements")
			journey.updateAgent(func(agent *storagedomain.Agent) { agent.ServiceRequirements = nil })
			location, _ = journey.authorize(ordinary + " " + journey.githubScope())
			coverage = coverageFor(journey.consentDetail(journey.expectConsentRedirect(location)), journey.cast.GitHub.ID)
			Expect(candidateFor(coverage, journey.cast.GitHubRead.ID)["usability"]).To(Equal("unusable"))
		})
	})

	Context("when a local refresh token carries the granted GitHub scope", func() {
		var tokenResponse map[string]any

		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
			journey.usableGitHub()
			tokenResponse = journey.issue(ordinary + " " + journey.githubScope())
		})

		// US3 S1 from specs/055-incremental-service-authorization/spec.md.
		It("rechecks granted service scopes on local refresh and returns invalid_scope once the service is unusable", func() {
			status, body := journey.refresh(tokenResponse["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body)
			Expect(scopeSet(body["scope"])).To(ConsistOf("openid", "offline_access", journey.githubScope()))

			status, body = journey.refresh(body["refresh_token"].(string), ordinary)
			Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body)
			Expect(scopeSet(body["scope"])).To(ConsistOf("openid", "offline_access"))
			Expect(scopeSet(decodeJWTClaims(body["access_token"].(string))["scope"])).To(ConsistOf("openid", "offline_access"))
			narrowed := body["refresh_token"].(string)

			journey.rejectedGitHub()
			unsatisfied := map[string]any{
				"error":             "invalid_scope",
				"error_description": "scope " + journey.githubScope() + " is no longer satisfied; interactive authorization required",
			}
			status, body = journey.refresh(narrowed, "")
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body).To(Equal(unsatisfied))
			status, body = journey.refresh(narrowed, ordinary+" "+journey.githubScope())
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error_description"]).To(Equal("scope " + journey.githubScope() + " requires interactive authorization"))

			By("treating a valid token missing a selected permission-set scope as unusable under require_all_scopes")
			journey.updateAgent(func(agent *storagedomain.Agent) {
				agent.ServiceRequirements[0].RequiredScopes = nil
				agent.ServiceRequirements[0].RequireAllScopes = true
			})
			journey.putSession(fixtures.SessionForServiceWithScopes(journey.principal, journey.cast.GitHub.ID.String(), []string{"repo"}))
			status, body = journey.refresh(narrowed, "")
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body).To(Equal(unsatisfied))

			journey.usableGitHub()
			status, body = journey.refresh(narrowed, "")
			Expect(status).To(Equal(http.StatusOK), "refused refreshes must not consume the token: %v", body)
			Expect(scopeSet(body["scope"])).To(ConsistOf("openid", "offline_access"))
		})

		// US3 S4 from specs/055-incremental-service-authorization/spec.md.
		It("includes the granted virtual scope in the token response and JWT scope and rechecks it on scope-less refresh", func() {
			granted := []string{"openid", "offline_access", journey.githubScope()}
			Expect(scopeSet(tokenResponse["scope"])).To(ConsistOf(granted))
			Expect(scopeSet(decodeJWTClaims(tokenResponse["access_token"].(string))["scope"])).To(ConsistOf(granted))

			status, body := journey.refresh(tokenResponse["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusOK), "OAuth2 error: %v", body)
			Expect(scopeSet(body["scope"])).To(ConsistOf(granted))
			Expect(scopeSet(decodeJWTClaims(body["access_token"].(string))["scope"])).To(ConsistOf(granted))
			journey.rejectedGitHub()
			status, refused := journey.refresh(body["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(refused["error"]).To(Equal("invalid_scope"))
			journey.usableGitHub()

			location, verifier := journey.authorize(strings.Join(granted, " "))
			code := journey.expectClientRedirect(location).Get("code")
			status, body = journey.redeem(code, "wrong-"+verifier)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body).NotTo(HaveKey("access_token"))
			location, verifier = journey.authorize(strings.Join(granted, " "))
			code = journey.expectClientRedirect(location).Get("code")
			status, _ = journey.redeem(code, verifier)
			Expect(status).To(Equal(http.StatusOK))
			status, body = journey.redeem(code, verifier)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("invalid_grant"))

			response, err := journey.enduser.DirectRequest(http.MethodDelete, "/api/consent/agents/"+journey.clientID()+"/grants", journey.principal, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusNoContent))
			Expect(response.Body.Close()).To(Succeed())
			status, body = journey.refresh(tokenResponse["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("invalid_grant"))
		})
	})

	Context("when GitHub is identified by canonical ID and UUID", func() {
		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
			journey.usableGitHub()
		})

		// US4 S2 from specs/055-incremental-service-authorization/spec.md.
		It("resolves both identifier forms, keeps UUID scopes across rename, and fails old canonical refresh without consuming it", func() {
			uuidScope := journey.scopeURL(journey.cast.GitHub.ID.String())
			grant := journey.currentGrant()
			grant.GrantedPermissionSets = grant.GrantedPermissionSets[:1]
			Expect(journey.store.UserGrants().Create(context.Background(), grant)).To(Succeed())
			location, _ := journey.authorize(ordinary + " " + journey.githubScope() + " " + uuidScope)
			sessionToken := journey.expectConsentRedirect(location)
			Expect(journey.consentDetail(sessionToken)["requested_service_ids"]).To(Equal([]any{journey.cast.GitHub.ID.String()}))
			status, body := journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead), grant.ValidUntil)
			continuation := journey.expectSaved(status, body, sessionToken)
			journey.renameGitHub(ptr.To("github-production"))
			journey.expectAuthorizationError(journey.follow(continuation), "invalid_scope", notPermitted)

			journey.renameGitHub(ptr.To("github"))
			canonical := journey.issue(ordinary + " " + journey.githubScope())
			durable := journey.issue(ordinary + " " + uuidScope)
			Expect(scopeSet(durable["scope"])).To(ContainElement(uuidScope))
			journey.renameGitHub(ptr.To("github-production"))

			status, body = journey.refresh(canonical["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("invalid_scope"))
			status, body = journey.refresh(durable["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusOK), "UUID scopes survive renames: %v", body)

			journey.rejectedGitHub()
			status, body = journey.exchange(durable["access_token"].(string))
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["scope"]).To(Equal(journey.scopeURL("github-production")))
			journey.renameGitHub(nil)
			status, body = journey.exchange(durable["access_token"].(string))
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["scope"]).To(Equal(uuidScope))

			journey.renameGitHub(ptr.To("github"))
			journey.usableGitHub()
			status, body = journey.refresh(canonical["refresh_token"].(string), "")
			Expect(status).To(Equal(http.StatusOK), "the failed old-canonical refresh must not consume the token: %v", body)
			fresh := journey.issue(ordinary + " " + journey.githubScope())
			Expect(scopeSet(fresh["scope"])).To(ContainElement(journey.githubScope()))
		})
	})

	Context("when crafted service scopes are requested", func() {
		BeforeEach(func() {
			journey = newIncrementalJourney("local", nil)
			journey.usableGitHub()
			unconfigured := fixtures.CorporateIdentityService(journey.provider.URL())
			unconfigured.CanonicalID = ptr.To("salesforce")
			Expect(journey.store.Services().Create(context.Background(), unconfigured)).To(Succeed())
		})

		// US4 S3 from specs/055-incremental-service-authorization/spec.md.
		It("rejects unknown, unconfigured, and malformed service scopes and treats external lookalikes as ordinary scopes", func() {
			before := journey.currentGrant()
			for _, crafted := range []string{
				journey.scopeURL("unknown-service"),
				journey.scopeURL(id.NewServiceID().String()),
				journey.scopeURL("salesforce"),
				journey.scopeURL("github") + "/",
				journey.scopeURL(strings.ToUpper(journey.cast.GitHub.ID.String())),
				journey.config.Server.EndUser.PublicURL + "/sessions/",
				journey.scopeURL("github%2Fadmin"),
			} {
				location, _ := journey.authorize(ordinary + " " + crafted)
				journey.expectAuthorizationError(location, "invalid_scope", notPermitted)
			}
			Expect(journey.currentGrant()).To(Equal(before))

			journey.updateAgent(func(agent *storagedomain.Agent) { agent.AllowedScopes = []string{"openid"} })
			for _, lookalike := range []string{"https://other.example.com/sessions/github", "http://LOCALHOST:8000/sessions/github"} {
				location, _ := journey.authorize("openid " + lookalike)
				journey.expectAuthorizationError(location, "invalid_scope", "requested scope is not permitted")
			}
			body := journey.issue("openid " + journey.githubScope())
			Expect(scopeSet(body["scope"])).To(ConsistOf("openid", journey.githubScope()))
		})
	})
})
