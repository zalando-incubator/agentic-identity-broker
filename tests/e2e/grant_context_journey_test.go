package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	extprochelpers "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// grantContextJourney runs the real broker and the production ExtProc composition with
// grant context enabled and the canonical-ID policy.
type grantContextJourney struct {
	broker *authzenFixture
	env    *bootstrap.ExtProcEnvironment
	client extprocv3.ExternalProcessorClient
}

func newGrantContextJourney(maxStaleness time.Duration, seedSession bool, mutate ...func(*authzenFixture)) *grantContextJourney {
	return newGrantContextJourneyWithPolicy("grant_context_canonical.rego", maxStaleness, seedSession, mutate...)
}

func newGrantContextJourneyWithPolicy(policy string, maxStaleness time.Duration, seedSession bool, mutate ...func(*authzenFixture)) *grantContextJourney {
	return newGrantContextJourneyConfigured(policy, maxStaleness, seedSession, nil, mutate...)
}

func newGrantContextJourneyConfigured(policy string, maxStaleness time.Duration, seedSession bool, configure func(*authzenFixture, *bootstrap.ExtProcOptions), mutate ...func(*authzenFixture)) *grantContextJourney {
	j := &grantContextJourney{broker: newAuthZENData(nil)}
	for _, m := range mutate {
		m(j.broker)
	}
	j.broker.BootAligned()
	if seedSession {
		j.broker.SeedSession(j.broker.Repo, []string{"read:repo"})
	}
	policyPath, err := filepath.Abs(filepath.Join("fixtures/policies", policy))
	Expect(err).NotTo(HaveOccurred())
	options := bootstrap.ExtProcOptions{DisableToolApprovals: true, GrantContext: extprocconfig.GrantContextConfig{Enabled: true, RequestTimeout: 2 * time.Second, MaxStaleness: maxStaleness}}
	if configure != nil {
		configure(j.broker, &options)
	}
	j.env, err = bootstrap.NewExtProcEnvironmentWithOptions(j.broker.Enduser.BaseURL(), j.broker.ClientAssertion, policyPath, bootstrap.TestLogger(0), options)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(j.env.Close)
	var conn *grpc.ClientConn
	j.client, conn = extprochelpers.ConnectToExtProc(j.env.Address)
	DeferCleanup(func() { _ = conn.Close() })
	return j
}

func (j *grantContextJourney) request(method string, endOfStream bool, resource string, extra map[string]string) *extprocv3.ProcessingRequest {
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

func (j *grantContextJourney) BodyCall(resource string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	return j.BodyCallWith(resource, nil)
}

func (j *grantContextJourney) BodyCallWith(resource string, extra map[string]string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repositories"}}`)
	return extprochelpers.SendHeadersAndBody(context.Background(), j.client, j.request(http.MethodPost, false, resource, extra), body)
}

func (j *grantContextJourney) HeaderOnly(resource string, extra map[string]string) *extprocv3.ProcessingResponse {
	return extprochelpers.SendRequestHeaders(context.Background(), j.client, j.request(http.MethodGet, true, resource, extra))
}

func (j *grantContextJourney) bindingCall(headerOnly bool, scopes, updatedAt string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	extra := map[string]string{"x-expected-scopes": scopes, "x-expected-grant-update": updatedAt}
	if headerOnly {
		return j.HeaderOnly(authzenRepoResource, extra), nil
	}
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repositories"}}`)
	return extprochelpers.SendHeadersAndBody(context.Background(), j.client,
		j.request(http.MethodPost, false, authzenRepoResource, extra), body)
}

func (j *grantContextJourney) narrowConsent() *storage.UserGrant {
	body, err := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{
		j.broker.Reader.ID.String(): {j.broker.Repo.ID.String()},
	}})
	Expect(err).NotTo(HaveOccurred())
	resp, err := j.broker.Enduser.AuthenticatedPOST("/api/consent/agents/"+j.broker.Agent.ID.String()+"/grants",
		j.broker.Principal.String(), "application/json", strings.NewReader(string(body)))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = resp.Body.Close() })
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(resp.Body.Close()).To(Succeed())
	grant, err := j.broker.Storage.UserGrants().FindByPrincipalAndAgent(context.Background(), j.broker.Principal, j.broker.Agent.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(grant.ID).To(Equal(j.broker.Grant.ID))
	Expect(grant.UpdatedAt.After(j.broker.Grant.UpdatedAt)).To(BeTrue())
	return grant
}

func (j *grantContextJourney) reauthorizeReader() {
	j.broker.Repo.Scopes = []model.OAuthScope{{ScopeValue: "read:repo", Description: "Read repositories"}}
	Expect(j.broker.Storage.Services().Update(context.Background(), j.broker.Repo, nil)).To(Succeed())
	j.broker.Upstream.WithAccessToken("SYNTHETIC_PROVIDER_TOKEN_R")
	start, err := j.broker.Enduser.AuthenticatedGET("/api/third-party/"+j.broker.Repo.ID.String()+
		"/oauth2/authorize?redirect_uri="+url.QueryEscape(j.broker.Enduser.BaseURL()+"/sessions"), j.broker.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = start.Body.Close() })
	Expect(start.StatusCode).To(Equal(http.StatusFound))
	providerURL, err := url.Parse(start.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.Fields(providerURL.Query().Get("scope"))).To(ConsistOf("read:repo"))
	Expect(start.Body.Close()).To(Succeed())
	browser := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	provider, err := browser.Get(providerURL.String())
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = provider.Body.Close() })
	Expect(provider.StatusCode).To(Equal(http.StatusFound))
	callbackURL, err := url.Parse(provider.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	Expect(provider.Body.Close()).To(Succeed())
	callback, err := j.broker.Enduser.AuthenticatedGET(callbackURL.RequestURI(), j.broker.Principal.String())
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = callback.Body.Close() })
	Expect(callback.StatusCode).To(Equal(http.StatusFound))
	Expect(callback.Body.Close()).To(Succeed())
	session, err := j.broker.Storage.UserSessions().FindByPrincipalAndService(context.Background(), j.broker.Principal, j.broker.Repo.ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(session.Scope).To(ConsistOf("read:repo"))
}

func broadGrantContext(f *authzenFixture) {
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

// forwarded reports whether the body phase returned a request-body response that forwards
// the request with the exchanged Authorization header.
func forwarded(headersResp, bodyResp *extprocv3.ProcessingResponse) bool {
	return bodyResp.GetRequestBody() != nil && extprochelpers.ExtractMutatedAuthorizationHeader(headersResp) != ""
}

// expectSessionRecovery asserts the MCP URL elicitation that sends the user to
// re-authorize the repository provider session, without forwarding.
func expectSessionRecovery(j *grantContextJourney, headersResp *extprocv3.ProcessingResponse) {
	Expect(extprochelpers.ExtractImmediateResponseStatus(headersResp)).To(Equal(uint32(http.StatusForbidden)))
	raw := extprochelpers.ExtractImmediateResponseBody(headersResp)
	var envelope struct {
		Error struct {
			Data struct {
				Elicitations []struct {
					ID string `json:"elicitationId"`
				} `json:"elicitations"`
			} `json:"data"`
		} `json:"error"`
	}
	Expect(json.Unmarshal([]byte(raw), &envelope)).To(Succeed())
	Expect(envelope.Error.Data.Elicitations).To(HaveLen(1))
	elicitationID := envelope.Error.Data.Elicitations[0].ID
	_, err := uuid.Parse(elicitationID)
	Expect(err).NotTo(HaveOccurred())
	const message = "User session is unavailable. Please re-authenticate."
	recoveryURI := j.broker.Enduser.BaseURL() + "/api/third-party/" + j.broker.Repo.ID.String() +
		"/oauth2/authorize?redirect_uri=" + url.QueryEscape(j.broker.Enduser.BaseURL()+"/sessions")
	expected, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": nil,
		"error": map[string]any{"code": -32042, "message": message,
			"data": map[string]any{"elicitations": []map[string]any{{
				"mode": "url", "elicitationId": elicitationID, "url": recoveryURI, "message": message,
			}}}},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(raw).To(MatchJSON(expected))
}

// overlapBarrier fronts the broker as PDP and token endpoint. It holds the first
// evaluation and the first exchange until both have arrived, or until a timeout, and
// records whether both were in flight together.
type overlapBarrier struct {
	server     *httptest.Server
	mu         sync.Mutex
	arrived    map[string]bool
	both       chan struct{}
	overlapped atomic.Bool
}

func newOverlapBarrier(brokerURL string) *overlapBarrier {
	target, err := url.Parse(brokerURL)
	Expect(err).NotTo(HaveOccurred())
	proxy := httputil.NewSingleHostReverseProxy(target)
	b := &overlapBarrier{arrived: map[string]bool{}, both: make(chan struct{})}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/authzen-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"policy_decision_point":      b.server.URL,
				"access_evaluation_endpoint": b.server.URL + "/access/v1/evaluation",
			})
			return
		case "/access/v1/evaluation", "/oauth2/token":
			b.hold(r.URL.Path)
		}
		proxy.ServeHTTP(w, r)
	}))
	DeferCleanup(b.server.Close)
	return b
}

func (b *overlapBarrier) hold(kind string) {
	b.mu.Lock()
	first := !b.arrived[kind]
	b.arrived[kind] = true
	completes := first && len(b.arrived) == 2
	if completes {
		close(b.both)
	}
	b.mu.Unlock()
	if !first || completes {
		return
	}
	select {
	case <-b.both:
		b.overlapped.Store(true)
	case <-time.After(time.Second):
	}
}

// advertisedEndpointPDP fronts the broker as token endpoint origin and PDP. It advertises an
// evaluation endpoint that differs from the default path and counts requests to each.
type advertisedEndpointPDP struct {
	server      *httptest.Server
	advertised  atomic.Int32
	defaultPath atomic.Int32
}

const advertisedEvaluationPath = "/pdp/v1/evaluate"

func newAdvertisedEndpointPDP(brokerURL string) *advertisedEndpointPDP {
	target, err := url.Parse(brokerURL)
	Expect(err).NotTo(HaveOccurred())
	proxy := httputil.NewSingleHostReverseProxy(target)
	p := &advertisedEndpointPDP{}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/authzen-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"policy_decision_point":      p.server.URL,
				"access_evaluation_endpoint": p.server.URL + advertisedEvaluationPath,
			})
		case advertisedEvaluationPath:
			p.advertised.Add(1)
			r.URL.Path = "/access/v1/evaluation"
			proxy.ServeHTTP(w, r)
		case "/access/v1/evaluation":
			p.defaultPath.Add(1)
			w.WriteHeader(http.StatusNotFound)
		default:
			proxy.ServeHTTP(w, r)
		}
	}))
	DeferCleanup(p.server.Close)
	return p
}

func immediateJSON(resp *extprocv3.ProcessingResponse) (uint32, map[string]string) {
	Expect(resp).NotTo(BeNil())
	var body map[string]string
	_ = json.Unmarshal([]byte(extprochelpers.ExtractImmediateResponseBody(resp)), &body)
	return extprochelpers.ExtractImmediateResponseStatus(resp), body
}

func expectGrantContextDenied(resp *extprocv3.ProcessingResponse, consentURL string) {
	status, body := immediateJSON(resp)
	Expect(status).To(Equal(uint32(http.StatusForbidden)))
	Expect(body["error"]).To(Equal("access_denied"))
	Expect(body["error_description"]).To(HavePrefix("delegation denied: "))
	Expect(body["error_uri"]).To(Equal(consentURL))
}

func expectGrantContextUnavailable(resp *extprocv3.ProcessingResponse) {
	status, body := immediateJSON(resp)
	Expect(status).To(Equal(uint32(http.StatusServiceUnavailable)))
	Expect(body).To(Equal(map[string]string{"error": "service_unavailable", "error_description": "grant context unavailable"}))
}

var _ = Describe("ExtProc grant context journeys", Label("grant-context"), func() {
	// Scenario US3.1 from specs/054-authzen-delegation-pip/spec.md
	It("resolves concurrently with exchange and gives body policy the documented direct context members", func() {
		var barrier *overlapBarrier
		j := newGrantContextJourneyConfigured("grant_context_canonical.rego", time.Minute, true, func(f *authzenFixture, options *bootstrap.ExtProcOptions) {
			barrier = newOverlapBarrier(f.Enduser.BaseURL())
			options.ExchangeEndpoint = barrier.server.URL + "/oauth2/token"
		})
		grant, err := j.broker.Storage.UserGrants().FindByPrincipalAndAgent(context.Background(), j.broker.Principal, j.broker.Agent.ID)
		Expect(err).NotTo(HaveOccurred())
		headersResp, bodyResp := j.BodyCallWith(authzenRepoResource, map[string]string{
			"x-expected-grant-id":     grant.ID.String(),
			"x-expected-grant-update": grant.UpdatedAt.Format(time.RFC3339Nano),
		})
		Expect(bodyResp).To(extprochelpers.BeForwardedRequestBody(), "the canonical policy allows only with the direct members, the expected grant and permit-derived permission sets")
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headersResp)).To(HavePrefix("Bearer "))
		Expect(barrier.overlapped.Load()).To(BeTrue(), "evaluation and exchange were in flight at the broker together")
		Expect(j.broker.EvaluationCount()).To(Equal(1))

		By("creating and approving a pending tool call owned by the matching exchange identity")
		matching := newGrantContextJourneyConfigured("approval_journey.rego", time.Minute, true, func(_ *authzenFixture, options *bootstrap.ExtProcOptions) {
			options.DisableToolApprovals = false
		})
		exchangeStatus, exchange := matching.broker.Exchange(authzenRepoResource)
		Expect(exchangeStatus).To(Equal(http.StatusOK))
		Expect(exchange["principal"]).To(Equal(matching.broker.Principal.String()))
		Expect(exchange["agent_id"]).To(Equal(matching.broker.Agent.ID.String()))
		toolBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"repo":"acme/app"}}}`)
		headersResp, bodyResp = extprochelpers.SendHeadersAndBody(context.Background(), matching.client,
			matching.request(http.MethodPost, false, authzenRepoResource, nil), toolBody)
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headersResp)).To(Equal("Bearer " + exchange["access_token"].(string)))
		approvalURL := approvalJourneyElicitationURL(bodyResp)
		Expect(bodyResp).NotTo(extprochelpers.BeForwardedRequestBody())
		pendingSync := approvalJourneySync(matching.broker.Enduser, matching.broker.ClientAssertion)
		Expect(pendingSync.Data.Pairs).To(HaveLen(1))
		pair := pendingSync.Data.Pairs[0]
		Expect(pair.Principal).To(Equal(exchange["principal"]))
		Expect(pair.AgentID).To(Equal(exchange["agent_id"]))
		Expect(pair.Approvals).To(HaveLen(1))
		pending := pair.Approvals[0]
		Expect(pending.Status).To(Equal("pending"))
		detail := approvalJourneyDetail(matching.broker.Enduser, pending.ID, pair.Principal)
		Expect(detail.Data.ApprovalURL).To(Equal(approvalURL))
		response, err := postJSON(matching.broker.Enduser, "/api/approvals/"+pending.ID+"/approve",
			pair.Principal, helpers.ApproveRequest{Persistence: "once"})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		approved := decodeJSON[helpers.ApproveResponse](response)
		Expect(approved.Data.Persistence).To(Equal("once"))

		By("forwarding the retry only after the approval is synchronized and consumed")
		Eventually(func() *extprocv3.ProcessingResponse {
			headersResp, bodyResp = extprochelpers.SendHeadersAndBody(context.Background(), matching.client,
				matching.request(http.MethodPost, false, authzenRepoResource, nil), toolBody)
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headersResp)).To(Equal("Bearer " + exchange["access_token"].(string)))
			return bodyResp
		}, 5*time.Second, 50*time.Millisecond).Should(extprochelpers.BeForwardedRequestBody())
		detail = approvalJourneyDetail(matching.broker.Enduser, pending.ID, pair.Principal)
		Expect(detail.Data.Consumed).To(BeTrue())

		By("rejecting an exchange identity that differs from the permit before approval or forwarding")
		var exchangeCalls atomic.Int32
		withApproval := newGrantContextJourneyConfigured("approval_journey.rego", time.Minute, true, func(f *authzenFixture, options *bootstrap.ExtProcOptions) {
			options.DisableToolApprovals = false
			target, err := url.Parse(f.Enduser.BaseURL())
			Expect(err).NotTo(HaveOccurred())
			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.ModifyResponse = func(response *http.Response) error {
				if response.StatusCode != http.StatusOK {
					return nil
				}
				defer func(body io.ReadCloser) { _ = body.Close() }(response.Body)
				var result map[string]any
				if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
					return err
				}
				result["principal"] = "different-exchange-principal"
				result["agent_id"] = "00000000-0000-0000-0000-000000000001"
				raw, err := json.Marshal(result)
				if err != nil {
					return err
				}
				response.Body = io.NopCloser(bytes.NewReader(raw))
				response.ContentLength = int64(len(raw))
				response.Header.Del("Content-Length")
				exchangeCalls.Add(1)
				return nil
			}
			endpoint := httptest.NewServer(proxy)
			DeferCleanup(endpoint.Close)
			options.ExchangeEndpoint = endpoint.URL + "/oauth2/token"
			options.GrantContext.PolicyDecisionPoint = f.Enduser.BaseURL()
		})
		body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"repo":"acme/app"}}}`)
		headersResp, bodyResp = extprochelpers.SendHeadersAndBody(context.Background(), withApproval.client,
			withApproval.request(http.MethodPost, false, authzenRepoResource, nil), body)
		status, responseBody := immediateJSON(headersResp)
		Expect(status).To(Equal(uint32(http.StatusServiceUnavailable)))
		Expect(responseBody).To(Equal(map[string]string{"error": "service_unavailable", "error_description": "exchange result binding unavailable"}))
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(headersResp)).To(BeEmpty())
		Expect(bodyResp).To(BeNil(), "an identity mismatch must fail before body policy or approval")
		snapshot := approvalJourneySync(withApproval.broker.Enduser, withApproval.broker.ClientAssertion)
		Expect(snapshot.Data.Pairs).To(BeEmpty(), "no approval is created for either identity")
		Expect(exchangeCalls.Load()).To(Equal(int32(1)), "an identity mismatch is terminal")
	})

	Context("with a one-minute permit cap", func() {
		var j *grantContextJourney

		BeforeEach(func() {
			j = newGrantContextJourney(time.Minute, true)
		})

		// Scenario US3.2 from specs/054-authzen-delegation-pip/spec.md
		It("resolves before header-only policy with the same direct members and exchanges only after allow", func() {
			resp := j.HeaderOnly(authzenRepoResource, nil)
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(resp)).To(HavePrefix("Bearer "), "header-only policy allowed, then exchange ran")
			Expect(j.broker.EvaluationCount()).To(Equal(1))
		})

		// Scenario US3.3 from specs/054-authzen-delegation-pip/spec.md
		It("returns 403 access_denied with the consent URL before policy", func() {
			Expect(j.broker.Storage.UserGrants().Delete(context.Background(), j.broker.Grant.ID)).To(Succeed())
			headersResp, bodyResp := j.BodyCall(authzenRepoResource)
			Expect(bodyResp).To(BeNil())
			expectGrantContextDenied(headersResp, j.broker.Enduser.BaseURL()+"/agents/"+j.broker.Agent.ID.String())
			expectGrantContextDenied(j.HeaderOnly(authzenRepoResource, nil), j.broker.Enduser.BaseURL()+"/agents/"+j.broker.Agent.ID.String())
		})

		Context("with canonical-ID variants", func() {
			var second, withoutReaderCanonicalID *grantContextJourney

			BeforeEach(func() {
				second = newGrantContextJourney(time.Minute, true)
				withoutReaderCanonicalID = newGrantContextJourney(time.Minute, true, func(f *authzenFixture) {
					f.Reader.CanonicalID = nil
					Expect(f.Storage.PermissionSets().Update(context.Background(), f.Reader)).To(Succeed())
				})
			})

			// Scenario US3.8 from specs/054-authzen-delegation-pip/spec.md
			It("makes the same canonical-ID policy decision across different UUIDs on both paths with permit-derived permission-set inputs", func() {
				Expect(second.broker.Agent.ID).NotTo(Equal(j.broker.Agent.ID))
				Expect(second.broker.Repo.ID).NotTo(Equal(j.broker.Repo.ID))
				Expect(second.broker.Reader.ID).NotTo(Equal(j.broker.Reader.ID))
				for _, journey := range []*grantContextJourney{j, second} {
					Expect(forwarded(journey.BodyCall(authzenRepoResource))).To(BeTrue())
					Expect(extprochelpers.ExtractMutatedAuthorizationHeader(journey.HeaderOnly(authzenRepoResource, nil))).To(HavePrefix("Bearer "))
				}

				_, bodyResp := withoutReaderCanonicalID.BodyCall(authzenRepoResource)
				status, body := immediateJSON(bodyResp)
				Expect(status).To(Equal(uint32(http.StatusForbidden)), "the policy keys on canonical IDs, not UUIDs")
				Expect(body["error_description"]).To(ContainSubstring("delegation does not cover repo-api"))
			})
		})

		// Scenario US3.9 from specs/054-authzen-delegation-pip/spec.md
		It("denies a header-only request by policy without calling exchange", func() {
			resp := j.HeaderOnly(authzenRepoResource, map[string]string{"x-policy-deny": "true"})
			status, body := immediateJSON(resp)
			Expect(status).To(Equal(uint32(http.StatusForbidden)))
			Expect(body["error_description"]).To(ContainSubstring("header requested a policy denial"))
			Expect(j.broker.EvaluationCount()).To(Equal(1))
			Expect(j.broker.ExchangeCount()).To(BeZero(), "no exchange after a policy denial")
		})
	})

	Context("with a two-second permit cap", func() {
		var j *grantContextJourney

		BeforeEach(func() {
			j = newGrantContextJourney(2*time.Second, true)
		})

		// Scenario US3.4 from specs/054-authzen-delegation-pip/spec.md
		It("re-resolves after permit freshness lapses and denies a revoked grant despite a cached token", func() {
			Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
			j.broker.RevokeGrant()
			Eventually(func() uint32 {
				headersResp, _ := j.BodyCall(authzenRepoResource)
				return extprochelpers.ExtractImmediateResponseStatus(headersResp)
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(uint32(http.StatusForbidden)))
		})

		// Scenario US3.7 from specs/054-authzen-delegation-pip/spec.md
		It("isolates the cache per subject and resource and honors max_staleness", func() {
			original := j.broker.SubjectToken
			var warmedAt, lastPairRequestDone time.Time
			reusedInsideCap := false
			// The permit deadline starts between warmStart and warmedAt. A reuse that completes
			// within 2 s of warmStart must hit the cache; a slower one may legitimately
			// revalidate and cache a new permit, so the sequence is repeated only after every
			// permit for the pair, including one created by a late reuse, has expired.
			for attempt := 0; attempt < 3 && !reusedInsideCap; attempt++ {
				if attempt > 0 {
					awaitBindingPermitExpiry(lastPairRequestDone)
				}
				before := j.broker.EvaluationCount()
				warmStart := time.Now()
				Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
				warmedAt = time.Now()
				Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
				lastPairRequestDone = time.Now()
				if time.Since(warmStart) >= 2*time.Second {
					continue
				}
				Expect(j.broker.EvaluationCount()).To(Equal(before+1), "the second request reuses the fresh permit")

				_, _ = j.BodyCall(authzenOtherResource)
				Expect(j.broker.EvaluationCount()).To(Equal(before+2), "another resource never uses the cached permit")
				j.broker.SubjectToken = j.broker.SignSubject(time.Now().Add(time.Hour).Add(time.Second))
				_, _ = j.BodyCall(authzenRepoResource)
				Expect(j.broker.EvaluationCount()).To(Equal(before+3), "another subject token never uses the cached permit")

				j.broker.SubjectToken = original
				Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
				lastPairRequestDone = time.Now()
				if time.Since(warmStart) >= 2*time.Second {
					continue
				}
				Expect(j.broker.EvaluationCount()).To(Equal(before+3), "the original pair reuses its permit before the cap")
				reusedInsideCap = true
			}
			Expect(reusedInsideCap).To(BeTrue(), "the warm-and-reuse sequence completed within the 2 s cap in one of three attempts")

			revalidations := j.broker.EvaluationCount()
			awaitBindingPermitExpiry(warmedAt)
			Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
			Expect(j.broker.EvaluationCount()).To(Equal(revalidations+1), "the 2 s local cap forces revalidation of the original pair despite the 60 s permit")
		})
	})

	Context("with a one-second permit cap", func() {
		var j *grantContextJourney

		BeforeEach(func() {
			j = newGrantContextJourney(time.Second, true)
		})

		// Scenario US3.5 from specs/054-authzen-delegation-pip/spec.md
		It("re-consults after consent immediately and preserves the body exchange cooldown", func() {
			consentURL := j.broker.Enduser.BaseURL() + "/agents/" + j.broker.Agent.ID.String()
			j.broker.RevokeGrant()
			expectGrantContextDenied(j.HeaderOnly(authzenRepoResource, nil), consentURL)
			j.broker.ConsentToRepo()
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(j.HeaderOnly(authzenRepoResource, nil))).To(HavePrefix("Bearer "),
				"consent takes effect immediately: the denial was not cached")
			Expect(j.broker.EvaluationCount()).To(Equal(2))

			// A separate consumer has no exchanged token from the header-only permit above.
			bodyJourney := newGrantContextJourney(time.Second, true)
			bodyConsentURL := bodyJourney.broker.Enduser.BaseURL() + "/agents/" + bodyJourney.broker.Agent.ID.String()
			bodyJourney.broker.RevokeGrant()
			headersResp, bodyResp := bodyJourney.BodyCall(authzenRepoResource)
			Expect(bodyResp).To(BeNil())
			expectGrantContextDenied(headersResp, bodyConsentURL)
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
			Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
			j.broker.Enduser.CloseListener()
			Eventually(func() uint32 {
				headersResp, _ := j.BodyCall(authzenRepoResource)
				return extprochelpers.ExtractImmediateResponseStatus(headersResp)
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(uint32(http.StatusServiceUnavailable)))
			headersResp, bodyResp := j.BodyCall(authzenRepoResource)
			Expect(bodyResp).To(BeNil())
			expectGrantContextUnavailable(headersResp)
			expectGrantContextUnavailable(j.HeaderOnly(authzenRepoResource, nil))
		})
	})

	// Scenario US3.10 from specs/054-authzen-delegation-pip/spec.md
	It("blocks forwarding when exchange fails despite a permitted delegation", func() {
		j := newGrantContextJourney(time.Minute, false)
		headersResp, bodyResp := j.BodyCall(authzenRepoResource)
		Expect(bodyResp).To(BeNil())
		expectSessionRecovery(j, headersResp)
		Expect(j.broker.EvaluationCount()).To(Equal(1), "the delegation permitted; exchange failure decided")
	})

	// Scenario US3.11 from specs/054-authzen-delegation-pip/spec.md
	It("forwards the reauthorized reader token on the first request after permit expiry", func() {
		for _, headerOnly := range []bool{false, true} {
			for _, failExchange := range []bool{false, true} {
				j := newGrantContextJourneyWithPolicy("grant_context_binding.rego", 2*time.Second, false, broadGrantContext)
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
					expectSessionRecovery(j, headers)
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
			j := newGrantContextJourneyWithPolicy("grant_context_binding.rego", 2*time.Second, false, broadGrantContext)
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

	// Scenario US3.13 from specs/054-authzen-delegation-pip/spec.md
	It("fails closed on a shared retry claimed by another live permit", func() {
		for _, headerOnly := range []bool{false, true} {
			var barrier *bootstrap.BindingConcurrencyBroker
			j := newGrantContextJourneyConfigured("grant_context_canonical.rego", time.Minute, false, func(f *authzenFixture, options *bootstrap.ExtProcOptions) {
				barrier = bootstrap.NewBindingConcurrencyBroker(f.Enduser.BaseURL())
				DeferCleanup(barrier.Close)
				options.ExchangeEndpoint = barrier.Server.URL
				options.GrantContext.PolicyDecisionPoint = f.Enduser.BaseURL()
			}, broadGrantContext)
			seed := j.HeaderOnly(authzenRepoResource, nil)
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(seed)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_RW"))
			shared := bootstrap.NewBindingConcurrencyExchanger(j.env)
			DeferCleanup(shared.Release)
			policy, err := filepath.Abs("fixtures/policies/grant_context_canonical.rego")
			Expect(err).NotTo(HaveOccurred())
			peers := make([]*grantContextJourney, 2)
			for i := range peers {
				env, err := bootstrap.NewExtProcEnvironmentWithOptions(j.broker.Enduser.BaseURL(), j.broker.ClientAssertion, policy, bootstrap.TestLogger(0), bootstrap.ExtProcOptions{DisableToolApprovals: true, Exchanger: shared, GrantContext: extprocconfig.GrantContextConfig{Enabled: true, RequestTimeout: 2 * time.Second, MaxStaleness: time.Minute}})
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(env.Close)
				client, conn := extprochelpers.ConnectToExtProc(env.Address)
				DeferCleanup(func() { _ = conn.Close() })
				peers[i] = &grantContextJourney{broker: j.broker, env: env, client: client}
			}
			j.narrowConsent()
			type result struct {
				index         int
				headers, body *extprocv3.ProcessingResponse
			}
			results := make(chan result, 2)
			call := func(index int) {
				defer GinkgoRecover()
				headers, body := peers[index].bindingCall(headerOnly, "", "")
				results <- result{index, headers, body}
			}
			go call(0)
			Eventually(shared.Ready, 5*time.Second).Should(Receive())
			grantBody, err := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{j.broker.Reader.ID.String(): {j.broker.Repo.ID.String()}, j.broker.Agent.PermissionSets[1].PermissionSetID.String(): {j.broker.Repo.ID.String()}}})
			Expect(err).NotTo(HaveOccurred())
			resp, err := j.broker.Enduser.AuthenticatedPOST("/api/consent/agents/"+j.broker.Agent.ID.String()+"/grants", j.broker.Principal.String(), "application/json", strings.NewReader(string(grantBody)))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = resp.Body.Close() })
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			Expect(resp.Body.Close()).To(Succeed())
			go call(1)
			Eventually(shared.Ready, 5*time.Second).Should(Receive())
			shared.Release()
			Eventually(shared.RetryJoined, 5*time.Second).Should(Receive())
			Eventually(shared.RetryJoined, 5*time.Second).Should(Receive())
			barrier.Release()
			var winner int
			statuses := make([]uint32, 0, 2)
			for range 2 {
				var r result
				Eventually(results, 5*time.Second).Should(Receive(&r))
				status := extprochelpers.ExtractImmediateResponseStatus(r.headers)
				statuses = append(statuses, status)
				if status == 0 {
					winner = r.index
					Expect(extprochelpers.ExtractMutatedAuthorizationHeader(r.headers)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_RW"))
					if !headerOnly {
						Expect(r.body).To(extprochelpers.BeForwardedRequestBody())
					}
				} else {
					_, body := immediateJSON(r.headers)
					Expect(body).To(Equal(map[string]string{"error": "service_unavailable", "error_description": "exchange result binding unavailable"}))
					Expect(extprochelpers.ExtractMutatedAuthorizationHeader(r.headers)).To(BeEmpty())
					Expect(r.body).To(BeNil())
				}
			}
			Expect(statuses).To(ConsistOf(uint32(0), uint32(http.StatusServiceUnavailable)))
			Expect(barrier.Calls.Load()).To(Equal(int32(2)), "one shared additional flight and no third exchange")
			winnerResponse := peers[winner].HeaderOnly(authzenRepoResource, nil)
			Expect(extprochelpers.ExtractMutatedAuthorizationHeader(winnerResponse)).To(Equal("Bearer SYNTHETIC_PROVIDER_TOKEN_RW"))
			Expect(barrier.Calls.Load()).To(Equal(int32(2)), "the compatible same-token replacement survives")
		}
	})

	// Scenario US4.5 from specs/054-authzen-delegation-pip/spec.md
	It("discovers from the token endpoint origin by default and uses the advertised endpoint", func() {
		var pdp *advertisedEndpointPDP
		j := newGrantContextJourneyConfigured("grant_context_canonical.rego", time.Minute, true, func(f *authzenFixture, options *bootstrap.ExtProcOptions) {
			pdp = newAdvertisedEndpointPDP(f.Enduser.BaseURL())
			options.ExchangeEndpoint = pdp.server.URL + "/oauth2/token"
		})
		Expect(j.env.PolicyDecisionPoint).To(Equal(pdp.server.URL), "the default PDP is the token endpoint origin")
		Expect(strings.HasSuffix(j.broker.Config.Server.EndUser.PublicURL, "/")).To(BeFalse())
		Expect(forwarded(j.BodyCall(authzenRepoResource))).To(BeTrue())
		Expect(j.broker.EvaluationCount()).To(Equal(1))
		Expect(pdp.advertised.Load()).To(Equal(int32(1)), "evaluation uses the advertised endpoint")
		Expect(pdp.defaultPath.Load()).To(BeZero(), "evaluation never falls back to the default path")
	})
})
