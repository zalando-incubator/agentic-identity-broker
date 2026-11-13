package e2e_test

import (
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// Scenarios reference specs/055-incremental-service-authorization/spec.md.
var _ = Describe("Incremental Service Authorization Proxy Mode", func() {
	var (
		journey             *incrementalJourney
		upstreamAccessToken string
		upstreamClaims      map[string]any
	)

	BeforeEach(func() {
		journey = newIncrementalJourney("proxy", func(j *incrementalJourney) {
			j.cast.Agent.ClientID = ptr.To(id.ClientID("data-analysis-assistant"))
		})
		claims := helpers.NewJWTClaims().
			WithIssuer(journey.upstream.URL()).
			WithSubject(journey.principal).
			WithAudience("upstream-resource").
			WithClaim("scope", "openid profile").
			WithClaim("roles", []string{"reader", "triager"}).
			WithClaim("tenant", map[string]any{"name": "engineering", "active": true}).Build()
		var err error
		upstreamAccessToken, err = helpers.SignTestJWT(claims, journey.upstream.GetPrivateKeyPEM())
		Expect(err).NotTo(HaveOccurred())
		upstreamClaims = decodeJWTClaims(upstreamAccessToken)
		journey.upstream.WithAccessToken(upstreamAccessToken)
		mux := journey.upstream.Server.Config.Handler.(*http.ServeMux)
		for _, endpoint := range []string{"/token", "/oauth/token"} {
			request, err := http.NewRequest(http.MethodPost, journey.upstream.URL()+endpoint, nil)
			Expect(err).NotTo(HaveOccurred())
			handler, _ := mux.Handler(request)
			mux.Handle("POST "+endpoint, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Pragma", "no-cache")
				handler.ServeHTTP(w, r)
			}))
		}
	})

	postProxyToken := func(form url.Values) (int, map[string]any) {
		form.Set("client_id", journey.clientID())
		response, err := journey.enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Header.Get("Content-Type")).To(Equal("application/json"))
		Expect(response.Header.Get("Cache-Control")).To(Equal("no-store"))
		Expect(response.Header.Get("Pragma")).To(Equal("no-cache"))
		return response.StatusCode, decodeJSON[map[string]any](response)
	}

	expectUpstreamToken := func(body map[string]any) {
		Expect(body).To(Equal(map[string]any{
			"access_token": upstreamAccessToken, "token_type": "Bearer", "expires_in": float64(3600),
			"refresh_token": "synthetic-upstream-rotated-refresh",
		}))
		Expect(decodeJWTClaims(body["access_token"].(string))).To(Equal(upstreamClaims))
		Expect(upstreamClaims["scope"]).To(Equal("openid profile"))
		Expect(upstreamClaims).NotTo(HaveKey("agent_id"))
	}

	upstreamScope := func(location *url.URL) (string, bool) {
		Expect(location.Scheme + "://" + location.Host).To(Equal(journey.upstream.URL()))
		Expect(location.Query().Get("client_id")).To(Equal("data-analysis-assistant"))
		Expect(location.Query().Get("session_token")).To(BeEmpty())
		Expect(location.Query().Get("state")).To(Equal("incremental-state"))
		return location.Query().Get("scope"), location.Query().Has("scope")
	}

	Context("when interactive authorization requires a service", func() {
		// US3 S2 from specs/055-incremental-service-authorization/spec.md.
		It("verifies requested services then redirects upstream with only ordinary scopes", func() {
			journey.rejectedGitHub()
			location, verifier := journey.authorize("openid profile " + journey.githubScope())
			sessionToken := journey.expectConsentRedirect(location)
			Expect(coverageFor(journey.consentDetail(sessionToken), journey.cast.GitHub.ID)["usability"]).To(Equal("unusable"))

			journey.connect(journey.cast.GitHub.ID, journey.config.Server.EndUser.PublicURL+"/agents/"+journey.clientID())
			grant := journey.currentGrant()
			status, body := journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead), grant.ValidUntil)
			upstreamAuthorization := journey.follow(journey.expectSaved(status, body, sessionToken))
			scope, present := upstreamScope(upstreamAuthorization)
			Expect(present).To(BeTrue())
			Expect(scope).To(Equal("openid profile"))
			Expect(journey.upstream.GetAuthorizeCalled()).To(BeFalse(), "the broker only redirects; the client agent visits the IdP")

			By("redeeming the upstream code without rewriting its signed token or response")
			callback, err := helpers.ProviderAuthorizationCallback(upstreamAuthorization.String())
			Expect(err).NotTo(HaveOccurred())
			query := journey.expectClientRedirect(callback)
			Expect(query.Get("code")).NotTo(BeEmpty())
			status, body = postProxyToken(url.Values{
				"grant_type": {"authorization_code"}, "code": {query.Get("code")},
				"redirect_uri": {incrementalRedirectURI}, "code_verifier": {verifier},
			})
			Expect(status).To(Equal(http.StatusOK))
			expectUpstreamToken(body)
			Expect(journey.upstream.GetAuthorizeCalled()).To(BeTrue())

			location, _ = journey.authorize(journey.githubScope())
			_, present = upstreamScope(location)
			Expect(present).To(BeFalse(), "an empty upstream scope must be omitted")

			location, _ = journey.authorize("openid " + journey.scopeURL(journey.cast.GitHub.ID.String()))
			scope, _ = upstreamScope(location)
			Expect(scope).To(Equal("openid"))
		})
	})

	Context("when upstream refresh cannot attest to service usability", func() {
		// US3 S3 from specs/055-incremental-service-authorization/spec.md.
		It("strips virtual scopes from proxy refresh, forwards the rotated upstream token, and the next exchange still challenges", func() {
			journey.rejectedGitHub()
			forward := func(scope string) (int, map[string]any, url.Values) {
				form := url.Values{
					"grant_type": {"refresh_token"}, "refresh_token": {"synthetic-upstream-refresh"},
					"extension": {"kept as sent"},
				}
				if scope != "" {
					form.Set("scope", scope)
				}
				status, body := postProxyToken(form)
				requests := journey.upstream.GetTokenRequests()
				Expect(requests).NotTo(BeEmpty())
				received, err := url.ParseQuery(requests[len(requests)-1].Body)
				Expect(err).NotTo(HaveOccurred())
				return status, body, received
			}

			for _, scope := range []string{
				journey.githubScope() + "/",
				journey.scopeURL("unknown"),
				journey.scopeURL(journey.cast.Jira.ID.String()),
			} {
				By("rejecting " + scope + " before forwarding")
				before := len(journey.upstream.GetTokenRequests())
				status, body := journey.postToken(url.Values{
					"grant_type": {"refresh_token"}, "refresh_token": {"synthetic-upstream-refresh"}, "scope": {"openid " + scope},
				})
				Expect(status).To(Equal(http.StatusBadRequest), "invalid scope %s", scope)
				Expect(body["error"]).To(Equal("invalid_scope"))
				Expect(body["error_description"]).To(Equal("requested service scope is not permitted"))
				Expect(journey.upstream.GetTokenRequests()).To(HaveLen(before), "refusal leaves the upstream credential unconsumed")
				Expect(journey.provider.GetTokenRequests()).To(BeEmpty())

				By("retrying a valid scope with the same upstream refresh token")
				status, body, received := forward("openid " + journey.githubScope())
				Expect(status).To(Equal(http.StatusOK))
				expectUpstreamToken(body)
				Expect(body["refresh_token"]).To(Equal("synthetic-upstream-rotated-refresh"))
				Expect(received.Get("refresh_token")).To(Equal("synthetic-upstream-refresh"))
				Expect(received.Get("scope")).To(Equal("openid"))
				Expect(journey.upstream.GetTokenRequests()).To(HaveLen(before + 1))
				Expect(journey.provider.GetTokenRequests()).To(BeEmpty())
			}

			status, body, received := forward("openid profile " + journey.githubScope())
			Expect(status).To(Equal(http.StatusOK))
			Expect(body["refresh_token"]).To(Equal("synthetic-upstream-rotated-refresh"))
			expectUpstreamToken(body)
			Expect(body).NotTo(HaveKey("scope"), "the broker must not fabricate an upstream scope")
			Expect(received.Get("scope")).To(Equal("openid profile"))
			Expect(received.Get("extension")).To(Equal("kept as sent"))
			Expect(received.Get("refresh_token")).To(Equal("synthetic-upstream-refresh"))

			By("preserving the upstream error without a service-usability veto")
			journey.upstream.WithErrorResponseAndDescription("invalid_grant", "The upstream refresh credential was rejected.")
			status, body, received = forward("openid " + journey.githubScope())
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body).To(Equal(map[string]any{
				"error": "invalid_grant", "error_description": "The upstream refresh credential was rejected.",
			}))
			Expect(received.Get("scope")).To(Equal("openid"))
			Expect(received.Get("refresh_token")).To(Equal("synthetic-upstream-refresh"))
			Expect(journey.provider.GetTokenRequests()).To(BeEmpty())

			By("retrying the same credential and preserving successful rotation")
			journey.upstream.WithSuccessfulTokenResponse()
			status, body, received = forward(journey.githubScope())
			Expect(status).To(Equal(http.StatusOK))
			expectUpstreamToken(body)
			Expect(body["refresh_token"]).To(Equal("synthetic-upstream-rotated-refresh"))
			Expect(received.Get("refresh_token")).To(Equal("synthetic-upstream-refresh"))
			Expect(received.Has("scope")).To(BeFalse())
			for _, request := range journey.upstream.GetTokenRequests() {
				Expect(request.Body).NotTo(ContainSubstring(url.QueryEscape(journey.config.Server.EndUser.PublicURL + "/sessions/")))
			}
			Expect(journey.provider.GetTokenRequests()).To(BeEmpty(), "a proxy refresh performs no broker service usability check")

			status, exchanged := journey.exchange(journey.tokens.SubjectToken)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(exchanged["scope"]).To(Equal(journey.githubScope()))
			Expect(exchanged["error_uri"]).To(ContainSubstring(url.QueryEscape(journey.githubScope())))

			journey.usableGitHub()
			status, exchanged = journey.exchange(journey.tokens.SubjectToken)
			Expect(status).To(Equal(http.StatusOK), "repairing GitHub restores the exchange: %v", exchanged)
			Expect(strings.Count(journey.upstream.GetLastBody(), "sessions")).To(BeZero())
		})
	})
})
