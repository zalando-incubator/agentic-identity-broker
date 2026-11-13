package extproc_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

func delegationStartupConfig(broker *bootstrap.MockAuthZENBroker) *extprocconfig.Config {
	cfg := opaEnabledConfig(policyPath("delegation_disabled_input.rego"))
	cfg.DelegationContext = extprocconfig.DelegationContextConfig{
		Enabled: true, PolicyDecisionPoint: broker.URL(), RequestTimeout: time.Second, MaxStaleness: time.Minute,
	}
	return cfg
}

var _ = Describe("ExtProc delegation context startup", Label("delegation-context-startup"), func() {
	var broker *bootstrap.MockAuthZENBroker

	BeforeEach(func() {
		broker = bootstrap.NewMockAuthZENBroker()
		DeferCleanup(broker.Stop)
	})

	// Scenario US4.1 from specs/054-authzen-delegation-pip/spec.md
	It("makes no discovery or evaluation requests when disabled by default", func() {
		cfg := opaEnabledConfig(policyPath("delegation_disabled_input.rego"))
		Expect(cfg.DelegationContext.Enabled).To(BeFalse())
		env := bootstrap.NewTestEnvironment(cfg, opaLogger)
		Expect(env.StartWithDelegationContext()).To(Succeed())
		DeferCleanup(env.Stop)
		env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
		client, conn := env.NewExtProcClient()
		DeferCleanup(conn.Close)

		headersReq := helpers.NewRequestHeaders().
			WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
			WithHeader(":method", "POST").
			WithAgentgatewayProtocol("mcp").
			BuildWithMetadata()
		headersResp, bodyResp := helpers.SendHeadersAndBody(context.Background(), client, headersReq,
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_files"}}`))

		Expect(helpers.ExtractMutatedAuthorizationHeader(headersResp)).To(Equal("Bearer " + fixtures.FreshExchangedToken))
		Expect(bodyResp).To(helpers.BeForwardedRequestBody(), "disabled-mode policy input omits every delegation member")
		Expect(broker.TotalRequestCount()).To(BeZero())
	})

	// Scenario US4.2 from specs/054-authzen-delegation-pip/spec.md
	It("fails startup when delegation context is enabled without authorization", func() {
		cfg := delegationStartupConfig(broker)
		cfg.Authorization.Enabled = false
		env := bootstrap.NewTestEnvironment(cfg, opaLogger)
		DeferCleanup(env.Stop)

		err := env.StartWithDelegationContext()

		Expect(err).To(MatchError(ContainSubstring("delegation_context.enabled requires authorization.enabled")))
		Expect(err.Error()).NotTo(ContainSubstring(cfg.OAuth2.ClientSecret))
		Expect(env.GRPCAddress()).To(BeEmpty())
		Expect(broker.TotalRequestCount()).To(BeZero())
	})

	// Scenario US4.3 from specs/054-authzen-delegation-pip/spec.md
	It("fails startup when discovery is unavailable or invalid", func() {
		for _, configure := range []func(){
			func() { broker.DiscoveryStatus = http.StatusServiceUnavailable },
			func() { broker.DiscoveryStatus = 0; broker.DiscoveryBody = `{"policy_decision_point":` },
			func() { broker.DiscoveryBody = `{"policy_decision_point":"` + broker.URL() + `"}` },
		} {
			configure()
			env := bootstrap.NewTestEnvironment(delegationStartupConfig(broker), opaLogger)
			err := env.StartWithDelegationContext()
			env.Stop()
			Expect(err).To(MatchError(ContainSubstring("delegation context discovery failed")))
			Expect(env.GRPCAddress()).To(BeEmpty(), "no listener after a failed discovery")
		}
		Expect(broker.DiscoveryCount()).To(Equal(3))
		Expect(broker.EvaluationCount()).To(BeZero())
	})

	// Scenario US4.4 from specs/054-authzen-delegation-pip/spec.md
	It("fails startup on a PDP identifier mismatch without sending evaluation credentials", func() {
		broker.AdvertisedPDP = "https://other.example.com"
		env := bootstrap.NewTestEnvironment(delegationStartupConfig(broker), opaLogger)
		DeferCleanup(env.Stop)

		err := env.StartWithDelegationContext()

		Expect(err).To(MatchError(ContainSubstring("policy_decision_point mismatch (expected " + broker.URL() + ", advertised https://other.example.com)")))
		Expect(env.GRPCAddress()).To(BeEmpty())
		Expect(broker.DiscoveryCount()).To(Equal(1))
		Expect(broker.EvaluationCount()).To(BeZero())
		Expect(broker.CredentialCallCount()).To(BeZero(), "no credential reaches the mismatched PDP")
	})
})
