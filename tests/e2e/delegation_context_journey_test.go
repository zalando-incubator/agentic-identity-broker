package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	extprochelpers "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// delegationJourney runs the real broker and the production ExtProc composition with
// delegation context enabled and the canonical-ID policy.
type delegationJourney struct {
	broker *authzenFixture
	env    *bootstrap.ExtProcEnvironment
	client extprocv3.ExternalProcessorClient
}

func newDelegationJourney(maxStaleness time.Duration, seedSession bool, mutate ...func(*authzenFixture)) *delegationJourney {
	return newDelegationJourneyWithPolicy("delegation_canonical.rego", maxStaleness, seedSession, mutate...)
}

func newDelegationJourneyWithPolicy(policy string, maxStaleness time.Duration, seedSession bool, mutate ...func(*authzenFixture)) *delegationJourney {
	j := &delegationJourney{broker: newAuthZENData(nil)}
	for _, m := range mutate {
		m(j.broker)
	}
	j.broker.BootAligned()
	if seedSession {
		j.broker.SeedSession(j.broker.Repo, []string{"read:repo"})
	}
	policyPath, err := filepath.Abs(filepath.Join("fixtures/policies", policy))
	Expect(err).NotTo(HaveOccurred())
	j.env, err = bootstrap.NewExtProcEnvironmentWithOptions(j.broker.Enduser.BaseURL(), j.broker.ClientAssertion, policyPath,
		bootstrap.TestLogger(0), bootstrap.ExtProcOptions{
			DisableToolApprovals: true,
			DelegationContext: extprocconfig.DelegationContextConfig{
				Enabled: true, RequestTimeout: 2 * time.Second, MaxStaleness: maxStaleness,
			},
		})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(j.env.Close)
	var conn *grpc.ClientConn
	j.client, conn = extprochelpers.ConnectToExtProc(j.env.Address)
	DeferCleanup(func() { _ = conn.Close() })
	return j
}

func (j *delegationJourney) request(method string, endOfStream bool, resource string, extra map[string]string) *extprocv3.ProcessingRequest {
	builder := extprochelpers.NewRequestHeaders().
		WithHeader(":method", method).
		WithPath(resource).
		WithTokenExchangeMetadata(j.broker.SubjectToken, resource).
		WithHeader("Mcp-Session-Id", "delegation-session").
		WithAgentgatewayProtocol("mcp").
		WithEndOfStream(endOfStream)
	for k, v := range extra {
		builder = builder.WithHeader(k, v)
	}
	return builder.BuildWithMetadata()
}

func (j *delegationJourney) BodyCall(resource string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repositories"}}`)
	return extprochelpers.SendHeadersAndBody(context.Background(), j.client, j.request(http.MethodPost, false, resource, nil), body)
}

func (j *delegationJourney) HeaderOnly(resource string, extra map[string]string) *extprocv3.ProcessingResponse {
	return extprochelpers.SendRequestHeaders(context.Background(), j.client, j.request(http.MethodGet, true, resource, extra))
}

func (j *delegationJourney) bindingCall(headerOnly bool, scopes, updatedAt string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	extra := map[string]string{"x-expected-scopes": scopes, "x-expected-grant-update": updatedAt}
	if headerOnly {
		return j.HeaderOnly(authzenRepoResource, extra), nil
	}
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repositories"}}`)
	return extprochelpers.SendHeadersAndBody(context.Background(), j.client,
		j.request(http.MethodPost, false, authzenRepoResource, extra), body)
}

func (j *delegationJourney) narrowConsent() *storage.UserGrant {
	body, err := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{
		j.broker.Reader.ID.String(): {j.broker.Repo.ID.String()},
	}})
	Expect(err).NotTo(HaveOccurred())
	resp, err := j.broker.Enduser.AuthenticatedPOST("/api/consent/agents/"+j.broker.Agent.ID.String()+"/grants",
		j.broker.Principal.String(), "application/json", strings.NewReader(string(body)))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(resp.Body.Close()).To(Succeed())
	grant, err := j.broker.Storage.UserGrants().FindByPrincipalAndAgent(context.Background(), j.broker.Principal, j.broker.Agent.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(grant.ID).To(Equal(j.broker.Grant.ID))
	Expect(grant.UpdatedAt.After(j.broker.Grant.UpdatedAt)).To(BeTrue())
	return grant
}

func (j *delegationJourney) reauthorizeReader() {
	j.broker.Repo.Scopes = []model.OAuthScope{{ScopeValue: "read:repo", Description: "Read repositories"}}
	Expect(j.broker.Storage.Services().Update(context.Background(), j.broker.Repo, nil)).To(Succeed())
	j.broker.Upstream.WithAccessToken("SYNTHETIC_PROVIDER_TOKEN_R")
	start, err := j.broker.Enduser.AuthenticatedGET("/api/third-party/"+j.broker.Repo.ID.String()+
		"/oauth2/authorize?redirect_uri="+url.QueryEscape(j.broker.Enduser.BaseURL()+"/sessions"), j.broker.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	Expect(start.StatusCode).To(Equal(http.StatusFound))
	providerURL, err := url.Parse(start.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.Fields(providerURL.Query().Get("scope"))).To(ConsistOf("read:repo"))
	Expect(start.Body.Close()).To(Succeed())
	browser := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	provider, err := browser.Get(providerURL.String())
	Expect(err).NotTo(HaveOccurred())
	Expect(provider.StatusCode).To(Equal(http.StatusFound))
	callbackURL, err := url.Parse(provider.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	Expect(provider.Body.Close()).To(Succeed())
	callback, err := j.broker.Enduser.AuthenticatedGET(callbackURL.RequestURI(), j.broker.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	Expect(callback.StatusCode).To(Equal(http.StatusFound))
	Expect(callback.Body.Close()).To(Succeed())
	session, err := j.broker.Storage.UserSessions().FindByPrincipalAndService(context.Background(), j.broker.Principal, j.broker.Repo.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(session.Scope).To(ConsistOf("read:repo"))
}

func broadDelegation(f *authzenFixture) {
	f.Agent.ServiceRequirements[0].RequiredScopes = []string{"read:repo", "write:repo"}
	f.SaveAgent()
	session := fixtures.SessionForServiceWithScopes(f.Principal.String(), f.Repo.ID.String(), []string{"read:repo", "write:repo"})
	session.EncryptedAccessToken = fixtures.EncryptedToken(f.Repo.ID.String(), "SYNTHETIC_PROVIDER_TOKEN_RW")
	Expect(f.Storage.UserSessions().Create(context.Background(), session)).To(Succeed())
}

func awaitBindingPermitExpiry(warmedAt time.Time) {
	Eventually(func() time.Duration { return time.Since(warmedAt) }, 4*time.Second, 25*time.Millisecond).
		Should(BeNumerically(">=", 2*time.Second), "wait without issuing extra target requests")
}

// forwarded reports whether the body phase allowed and forwarded the request.
func forwarded(headersResp, bodyResp *extprocv3.ProcessingResponse) bool {
	return bodyResp != nil && extprochelpers.ExtractMutatedAuthorizationHeader(headersResp) != "" && extprochelpers.ExtractImmediateResponseStatus(bodyResp) == 0
}

func immediateJSON(resp *extprocv3.ProcessingResponse) (uint32, map[string]string) {
	Expect(resp).NotTo(BeNil())
	var body map[string]string
	_ = json.Unmarshal([]byte(extprochelpers.ExtractImmediateResponseBody(resp)), &body)
	return extprochelpers.ExtractImmediateResponseStatus(resp), body
}

func expectDelegationDenied(resp *extprocv3.ProcessingResponse, consentURL string) {
	status, body := immediateJSON(resp)
	Expect(status).To(Equal(uint32(http.StatusForbidden)))
	Expect(body["error"]).To(Equal("access_denied"))
	Expect(body["error_description"]).To(HavePrefix("delegation denied: "))
	Expect(body["error_uri"]).To(Equal(consentURL))
}

func expectDelegationUnavailable(resp *extprocv3.ProcessingResponse) {
	status, body := immediateJSON(resp)
	Expect(status).To(Equal(uint32(http.StatusServiceUnavailable)))
	Expect(body).To(Equal(map[string]string{"error": "service_unavailable", "error_description": "delegation context unavailable"}))
}

var _ = Describe("ExtProc delegation context journeys", Label("delegation-context"), func() {
	// Scenario US3.1 from specs/054-authzen-delegation-pip/spec.md
	It("resolves concurrently with exchange and gives body policy the documented direct context members", func() {
		j := newDelegationJourney(time.Minute, true)
		headersResp, bodyResp := j.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(extprochelpers.BeForwardedRequestBody(), "the canonical policy allows only with the direct members and permit-derived permission sets")
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headersResp)).To(HavePrefix("Bearer "))
		Expect(j.broker.EvaluationCount()).To(Equal(1))
	})

	// Scenario US3.2 from specs/054-authzen-delegation-pip/spec.md
	It("resolves before header-only policy with the same direct members and exchanges only after allow", func() {
		j := newDelegationJourney(time.Minute, true)
		resp := j.HeaderOnly(authzenRepoResource, nil)
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(resp)).To(HavePrefix("Bearer "), "header-only policy allowed, then exchange ran")
		Expect(j.broker.EvaluationCount()).To(Equal(1))
	})

	// Scenario US3.3 from specs/054-authzen-delegation-pip/spec.md
	It("returns 403 access_denied with the consent URL before policy", func() {
		j := newDelegationJourney(time.Minute, true)
		Expect(j.broker.Storage.UserGrants().Delete(context.Background(), j.broker.Grant.ID)).To(Succeed())
		headersResp, bodyResp := j.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(BeNil())
		expectDelegationDenied(headersResp, j.broker.Enduser.BaseURL()+"/agents/"+j.broker.Agent.ID.String())
		expectDelegationDenied(j.HeaderOnly(authzenRepoResource, nil), j.broker.Enduser.BaseURL()+"/agents/"+j.broker.Agent.ID.String())
	})

	// Scenario US3.4 from specs/054-authzen-delegation-pip/spec.md
	It("re-resolves after permit freshness lapses and denies a revoked grant despite a cached token", func() {
		j := newDelegationJourney(2*time.Second, true)
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		j.broker.RevokeGrant()
		Eventually(func() uint32 {
			headersResp, _ := j.BodyCall(authzenRepoResource)
			return extprochelpers.ExtractImmediateResponseStatus(headersResp)
		}, 5*time.Second, 200*time.Millisecond).Should(Equal(uint32(http.StatusForbidden)))
	})

	// Scenario US3.5 from specs/054-authzen-delegation-pip/spec.md
	It("re-consults after consent immediately and preserves the body exchange cooldown", func() {
		j := newDelegationJourney(time.Second, true)
		consentURL := j.broker.Enduser.BaseURL() + "/agents/" + j.broker.Agent.ID.String()
		j.broker.RevokeGrant()
		expectDelegationDenied(j.HeaderOnly(authzenRepoResource, nil), consentURL)
		j.broker.ConsentToRepo()
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(j.HeaderOnly(authzenRepoResource, nil))).To(HavePrefix("Bearer "),
			"consent takes effect immediately: the denial was not cached")
		Expect(j.broker.EvaluationCount()).To(Equal(2))

		// A separate consumer has no exchanged token from the header-only permit above.
		bodyJourney := newDelegationJourney(time.Second, true)
		bodyConsentURL := bodyJourney.broker.Enduser.BaseURL() + "/agents/" + bodyJourney.broker.Agent.ID.String()
		bodyJourney.broker.RevokeGrant()
		headersResp, bodyResp := bodyJourney.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(BeNil())
		expectDelegationDenied(headersResp, bodyConsentURL)
		Expect(bodyJourney.broker.EvaluationCount()).To(Equal(1))
		bodyJourney.broker.ConsentToRepo()
		headersResp, bodyResp = bodyJourney.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(BeNil(), "the independent exchange cooldown still blocks forwarding")
		Expect(extprochelpers.ExtractImmediateResponseStatus(headersResp)).To(Equal(uint32(http.StatusForbidden)))
		challenge := extprochelpers.ExtractImmediateResponseHeader(headersResp, "www-authenticate")
		Expect(challenge).To(HavePrefix(`Bearer error="insufficient_scope"`))
		Expect(challenge).To(ContainSubstring(`error_uri="` + bodyConsentURL + `"`))
		Expect(bodyJourney.broker.EvaluationCount()).To(Equal(2), "the delegation denial was not cached")
		Eventually(func() bool { return forwarded(bodyJourney.BodyCall(authzenRepoResource)) }, 8*time.Second, 250*time.Millisecond).Should(BeTrue())
	})

	// Scenario US3.6 from specs/054-authzen-delegation-pip/spec.md
	It("fails closed when the broker is unavailable after permit expiry", func() {
		j := newDelegationJourney(time.Second, true)
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		j.broker.Enduser.CloseListener()
		Eventually(func() uint32 {
			headersResp, _ := j.BodyCall(authzenRepoResource)
			return extprochelpers.ExtractImmediateResponseStatus(headersResp)
		}, 5*time.Second, 200*time.Millisecond).Should(Equal(uint32(http.StatusServiceUnavailable)))
		headersResp, bodyResp := j.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(BeNil())
		expectDelegationUnavailable(headersResp)
		expectDelegationUnavailable(j.HeaderOnly(authzenRepoResource, nil))
	})

	// Scenario US3.7 from specs/054-authzen-delegation-pip/spec.md
	It("isolates the cache per subject and resource and honors max_staleness", func() {
		j := newDelegationJourney(2*time.Second, true)
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		Expect(j.broker.EvaluationCount()).To(Equal(1), "the second request reuses the fresh permit")

		_, _ = j.BodyCall(authzenOtherResource)
		Expect(j.broker.EvaluationCount()).To(Equal(2), "another resource never uses the cached permit")
		j.broker.SubjectToken = j.broker.SignSubject(time.Now().Add(time.Hour).Add(time.Second))
		_, _ = j.BodyCall(authzenRepoResource)
		Expect(j.broker.EvaluationCount()).To(Equal(3), "another subject token never uses the cached permit")

		Eventually(func() int {
			_, _ = j.BodyCall(authzenRepoResource)
			return j.broker.EvaluationCount()
		}, 5*time.Second, 250*time.Millisecond).Should(BeNumerically(">=", 4), "the local cap forces revalidation")
	})

	// Scenario US3.8 from specs/054-authzen-delegation-pip/spec.md
	It("makes the same canonical-ID policy decision across different UUIDs on both paths with permit-derived permission-set inputs", func() {
		first := newDelegationJourney(time.Minute, true)
		second := newDelegationJourney(time.Minute, true)
		Expect(second.broker.Agent.ID).NotTo(Equal(first.broker.Agent.ID))
		Expect(second.broker.Repo.ID).NotTo(Equal(first.broker.Repo.ID))
		Expect(second.broker.Reader.ID).NotTo(Equal(first.broker.Reader.ID))
		for _, j := range []*delegationJourney{first, second} {
			Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(j.HeaderOnly(authzenRepoResource, nil))).To(HavePrefix("Bearer "))
		}

		withoutReaderCanonicalID := newDelegationJourney(time.Minute, true, func(f *authzenFixture) {
			f.Reader.CanonicalID = nil
			Expect(f.Storage.PermissionSets().Update(context.Background(), f.Reader)).To(Succeed())
		})
		_, bodyResp := withoutReaderCanonicalID.BodyCall(authzenRepoResource)
		status, body := immediateJSON(bodyResp)
		Expect(status).To(Equal(uint32(http.StatusForbidden)), "the policy keys on canonical IDs, not UUIDs")
		Expect(body["error_description"]).To(ContainSubstring("delegation does not cover repo-api"))
	})

	// Scenario US3.9 from specs/054-authzen-delegation-pip/spec.md
	It("denies a header-only request by policy without calling exchange", func() {
		j := newDelegationJourney(time.Minute, true)
		resp := j.HeaderOnly(authzenRepoResource, map[string]string{"x-policy-deny": "true"})
		status, body := immediateJSON(resp)
		Expect(status).To(Equal(uint32(http.StatusForbidden)))
		Expect(body["error_description"]).To(ContainSubstring("header requested a policy denial"))
		Expect(j.broker.EvaluationCount()).To(Equal(1))
		Expect(j.broker.Logs.Raw()).NotTo(ContainSubstring("token_exchange_succeeded"), "no exchange after a policy denial")
	})

	// Scenario US3.10 from specs/054-authzen-delegation-pip/spec.md
	It("blocks forwarding when exchange fails despite a permitted delegation", func() {
		j := newDelegationJourney(time.Minute, false)
		headersResp, bodyResp := j.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(BeNil())
		status, body := immediateJSON(headersResp)
		Expect(status).NotTo(BeZero())
		Expect(body["error"]).NotTo(Equal("service_unavailable"))
		Expect(body["error_description"]).NotTo(HavePrefix("delegation denied"))
		Expect(j.broker.EvaluationCount()).To(Equal(1), "the delegation permitted; exchange failure decided")
	})

	// Scenario US3.11 from specs/054-authzen-delegation-pip/spec.md
	It("forwards the reauthorized reader token on the first request after permit expiry", func() {
		for _, headerOnly := range []bool{false, true} {
			for _, failExchange := range []bool{false, true} {
				j := newDelegationJourneyWithPolicy("delegation_binding.rego", 2*time.Second, false, broadDelegation)
				headers, body := j.bindingCall(headerOnly, "read:repo write:repo", "")
				Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headers)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_RW"))
				if !headerOnly {
					Expect(body).To(extprochelpers.BeForwardedRequestBody())
				}
				warmedAt := time.Now()
				grant := j.narrowConsent()
				j.reauthorizeReader()
				if failExchange {
					session, err := j.broker.Storage.UserSessions().FindByPrincipalAndService(context.Background(), j.broker.Principal, j.broker.Repo.ID)
					Expect(err).NotTo(HaveOccurred())
					Expect(j.broker.Storage.UserSessions().Delete(context.Background(), session.ID)).To(Succeed())
				}
				awaitBindingPermitExpiry(warmedAt)
				before := j.broker.ExchangeCount()
				headers, body = j.bindingCall(headerOnly, "read:repo", grant.UpdatedAt.Format(time.RFC3339Nano))
				Expect(j.broker.ExchangeCount()).To(Equal(before+1), "one target request makes exactly one additional broker exchange")
				if failExchange {
					Expect(extprochelpers.ExtractImmediateResponseStatus(headers)).NotTo(BeZero())
					Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headers)).To(BeEmpty())
					Expect(body).To(BeNil())
				} else {
					Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headers)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_R"))
					if !headerOnly {
						Expect(body).To(extprochelpers.BeForwardedRequestBody(), "policy must see reader scopes and the new grant instant")
					}
				}
			}
		}
	})

	// Scenario US3.12 from specs/054-authzen-delegation-pip/spec.md
	It("re-exchanges after narrowing without claiming provider downscoping", func() {
		for _, headerOnly := range []bool{false, true} {
			j := newDelegationJourneyWithPolicy("delegation_binding.rego", 2*time.Second, false, broadDelegation)
			headers, body := j.bindingCall(headerOnly, "read:repo write:repo", "")
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headers)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_RW"))
			if !headerOnly {
				Expect(body).To(extprochelpers.BeForwardedRequestBody())
			}
			warmedAt := time.Now()
			grant := j.narrowConsent()
			awaitBindingPermitExpiry(warmedAt)
			before := j.broker.ExchangeCount()
			headers, body = j.bindingCall(headerOnly, "read:repo", grant.UpdatedAt.Format(time.RFC3339Nano))
			Expect(j.broker.ExchangeCount()).To(Equal(before+1), "the old binding is not reused even when token bytes repeat")
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headers)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_RW"), "an unchanged provider session remains broad")
			if !headerOnly {
				Expect(body).To(extprochelpers.BeForwardedRequestBody())
			}
			session, err := j.broker.Storage.UserSessions().FindByPrincipalAndService(context.Background(), j.broker.Principal, j.broker.Repo.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session.Scope).To(ConsistOf("read:repo", "write:repo"))
		}
	})

	// Scenario US4.5 from specs/054-authzen-delegation-pip/spec.md
	It("discovers from the token endpoint origin by default and uses the advertised endpoint", func() {
		j := newDelegationJourney(time.Minute, true)
		Expect(j.env.PolicyDecisionPoint).To(Equal(j.broker.Enduser.BaseURL()))
		Expect(strings.HasSuffix(j.broker.Config.Server.EndUser.PublicURL, "/")).To(BeFalse())
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		Expect(j.broker.EvaluationCount()).To(Equal(1))
	})
})
