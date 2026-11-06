package e2e_test

import (
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// Scenarios reference specs/055-incremental-service-authorization/spec.md.
var _ = Describe("Incremental Service Authorization Proxy Mode", func() {
	var journey *incrementalJourney

	BeforeEach(func() {
		journey = newIncrementalJourney("proxy", func(j *incrementalJourney) {
			j.cast.Agent.ClientID = ptr.To(id.ClientID("data-analysis-assistant"))
		})
	})

	upstreamScope := func(location *url.URL) (string, bool) {
		Expect(location.Scheme + "://" + location.Host).To(Equal(journey.upstream.URL()))
		Expect(location.Query().Get("client_id")).To(Equal("data-analysis-assistant"))
		Expect(location.Query().Get("session_token")).To(BeEmpty())
		Expect(location.Query().Get("state")).To(Equal("incremental-state"))
		return location.Query().Get("scope"), location.Query().Has("scope")
	}

	// US3 S2 from specs/055-incremental-service-authorization/spec.md.
	It("verifies requested services then redirects upstream with only ordinary scopes", func() {
		journey.rejectedGitHub()
		location, _ := journey.authorize("openid profile " + journey.githubScope())
		sessionToken := journey.expectConsentRedirect(location)
		Expect(coverageFor(journey.consentDetail(sessionToken), journey.cast.GitHub.ID)["usability"]).To(Equal("unusable"))

		journey.connect(journey.cast.GitHub.ID, journey.config.Server.EndUser.PublicURL+"/agents/"+journey.clientID())
		grant := journey.currentGrant()
		status, body := journey.submit(sessionToken, journey.grantSelection(journey.cast.IdentityAccess, journey.cast.GitHubRead), grant.ValidUntil)
		scope, present := upstreamScope(journey.follow(journey.expectSaved(status, body, sessionToken)))
		Expect(present).To(BeTrue())
		Expect(scope).To(Equal("openid profile"))

		location, _ = journey.authorize(journey.githubScope())
		_, present = upstreamScope(location)
		Expect(present).To(BeFalse(), "an empty upstream scope must be omitted")

		location, _ = journey.authorize("openid " + journey.scopeURL(journey.cast.GitHub.ID.String()))
		scope, _ = upstreamScope(location)
		Expect(scope).To(Equal("openid"))
		Expect(journey.upstream.GetAuthorizeCalled()).To(BeFalse(), "the broker only redirects; the client agent visits the IdP")
	})

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
			status, body := journey.postToken(form)
			requests := journey.upstream.GetTokenRequests()
			Expect(requests).NotTo(BeEmpty())
			received, err := url.ParseQuery(requests[len(requests)-1].Body)
			Expect(err).NotTo(HaveOccurred())
			return status, body, received
		}

		status, body, received := forward("openid profile " + journey.githubScope())
		Expect(status).To(Equal(http.StatusOK))
		Expect(body["refresh_token"]).To(Equal("synthetic-upstream-rotated-refresh"))
		Expect(body["access_token"]).To(Equal("mock-access-token"))
		Expect(body).NotTo(HaveKey("scope"), "the broker must not fabricate an upstream scope")
		Expect(received.Get("scope")).To(Equal("openid profile"))
		Expect(received.Get("extension")).To(Equal("kept as sent"))
		Expect(received.Get("refresh_token")).To(Equal("synthetic-upstream-refresh"))

		_, _, received = forward(journey.githubScope())
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
