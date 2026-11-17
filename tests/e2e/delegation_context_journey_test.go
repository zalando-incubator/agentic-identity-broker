package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	extprochelpers "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

// delegationJourney runs the real broker and the production ExtProc composition with
// delegation context enabled and the canonical-ID policy.
type delegationJourney struct {
	broker *authzenFixture
	env    *bootstrap.ExtProcEnvironment
	client extprocv3.ExternalProcessorClient
}

func newDelegationJourney(maxStaleness time.Duration, seedSession bool, mutate ...func(*authzenFixture)) *delegationJourney {
	j := &delegationJourney{broker: newAuthZENData(nil)}
	for _, m := range mutate {
		m(j.broker)
	}
	j.broker.BootAligned()
	if seedSession {
		j.broker.SeedSession(j.broker.Repo, []string{"read:repo"})
	}
	policyPath, err := filepath.Abs("fixtures/policies/delegation_canonical.rego")
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
		Expect(j.broker.Storage.UserGrants().Delete(context.Background(), j.broker.Grant.ID)).To(Succeed())
		Eventually(func() uint32 {
			headersResp, _ := j.BodyCall(authzenRepoResource)
			return extprochelpers.ExtractImmediateResponseStatus(headersResp)
		}, 5*time.Second, 200*time.Millisecond).Should(Equal(uint32(http.StatusForbidden)))
	})

	// Scenario US3.5 from specs/054-authzen-delegation-pip/spec.md
	It("does not cache denials and permits right after consent", func() {
		j := newDelegationJourney(time.Second, true)
		consentURL := j.broker.Enduser.BaseURL() + "/agents/" + j.broker.Agent.ID.String()
		Expect(j.broker.Storage.UserGrants().Delete(context.Background(), j.broker.Grant.ID)).To(Succeed())
		expectDelegationDenied(j.HeaderOnly(authzenRepoResource, nil), consentURL)
		Expect(j.broker.Storage.UserGrants().Create(context.Background(), j.broker.Grant)).To(Succeed())
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(j.HeaderOnly(authzenRepoResource, nil))).To(HavePrefix("Bearer "),
			"consent takes effect immediately: the denial was not cached")
		Expect(j.broker.EvaluationCount()).To(Equal(2))

		Expect(j.broker.Storage.UserGrants().Delete(context.Background(), j.broker.Grant.ID)).To(Succeed())
		Eventually(func() *extprocv3.ProcessingResponse {
			headersResp, _ := j.BodyCall(authzenRepoResource)
			return headersResp
		}, 5*time.Second, 250*time.Millisecond).Should(extprochelpers.HaveImmediateResponseWithStatus(http.StatusForbidden))
		evaluations := j.broker.EvaluationCount()
		Expect(j.broker.Storage.UserGrants().Create(context.Background(), j.broker.Grant)).To(Succeed())
		// The body path's concurrent exchange recorded the existing 5 s exchange re-consent
		// cooldown (ADR 012) during the denied request; delegation itself holds no denial
		// and consults the broker again on the very next request.
		_, _ = j.BodyCall(authzenRepoResource)
		Expect(j.broker.EvaluationCount()).To(Equal(evaluations + 1))
		Eventually(func() bool { return forwarded(j.BodyCall(authzenRepoResource)) }, 8*time.Second, 250*time.Millisecond).Should(BeTrue())
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

	// Scenario US4.5 from specs/054-authzen-delegation-pip/spec.md
	It("discovers from the token endpoint origin by default and uses the advertised endpoint", func() {
		j := newDelegationJourney(time.Minute, true)
		Expect(j.env.PolicyDecisionPoint).To(Equal(j.broker.Enduser.BaseURL()))
		Expect(strings.HasSuffix(j.broker.Config.Server.EndUser.PublicURL, "/")).To(BeFalse())
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		Expect(j.broker.EvaluationCount()).To(Equal(1))
	})
})
