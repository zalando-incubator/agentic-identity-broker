package extproc_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

func grantContextStartupConfig(broker *bootstrap.MockAuthZENBroker) *extprocconfig.Config {
	cfg := opaEnabledConfig(policyPath("grant_context_disabled_input.rego"))
	cfg.GrantContext = extprocconfig.GrantContextConfig{
		Enabled: true, PolicyDecisionPoint: broker.URL(), RequestTimeout: time.Second, MaxStaleness: time.Minute,
	}
	return cfg
}

var _ = Describe("ExtProc grant context startup", Label("grant-context-startup"), func() {
	var broker *bootstrap.MockAuthZENBroker

	BeforeEach(func() {
		broker = bootstrap.NewMockAuthZENBroker()
		DeferCleanup(broker.Stop)
	})

	// Scenario US4.1 from specs/054-authzen-delegation-pip/spec.md
	It("makes no discovery or evaluation requests when disabled by default", func() {
		cfg := opaEnabledConfig(policyPath("grant_context_disabled_input.rego"))
		Expect(cfg.GrantContext.Enabled).To(BeFalse())
		env := bootstrap.NewTestEnvironment(cfg, opaLogger)
		DeferCleanup(env.Stop)
		Expect(env.StartWithGrantContext()).To(Succeed())
		env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken).
			WithGrantedPermissionSets(map[string][]string{"repo-reader": {"11111111-1111-1111-1111-111111111111"}})
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
		Expect(bodyResp).To(helpers.BeForwardedRequestBody(), "disabled-mode policy input keeps the exchange snapshot and omits every delegation member")
		Expect(env.MockTokenExchange.UnroutedRequestCount()).To(BeZero(), "no AuthZEN request reaches the default PDP origin")
	})

	// Scenario US4.2 from specs/054-authzen-delegation-pip/spec.md
	It("fails startup when grant context is enabled without authorization", func() {
		cfg := grantContextStartupConfig(broker)
		cfg.Authorization.Enabled = false
		env := bootstrap.NewTestEnvironment(cfg, opaLogger)
		DeferCleanup(env.Stop)

		err := env.StartWithGrantContext()

		Expect(err).To(MatchError(ContainSubstring("grant_context.enabled requires authorization.enabled")))
		Expect(err.Error()).NotTo(ContainSubstring(cfg.OAuth2.ClientSecret))
		Expect(env.GRPCAddress()).To(BeEmpty())
		Expect(broker.TotalRequestCount()).To(BeZero())
	})

	// Scenario US4.3 from specs/054-authzen-delegation-pip/spec.md
	It("retries unavailable or invalid discovery without listening, then serves once discovery succeeds", func() {
		broker.TransientDiscoveryFailures = 3
		env := bootstrap.NewTestEnvironment(grantContextStartupConfig(broker), opaLogger)
		DeferCleanup(env.Stop)

		Expect(env.StartWithGrantContext()).To(Succeed(), "unavailable, invalid and incomplete metadata are retried, not fatal")

		Expect(broker.DiscoveryCount()).To(Equal(4), "one attempt per failure, then the successful discovery")
		Expect(env.GRPCAddress()).NotTo(BeEmpty(), "the listener opens only after discovery succeeds")
		Expect(broker.EvaluationCount()).To(BeZero())
		Expect(broker.CredentialCallCount()).To(BeZero(), "discovery retries send no credential")
	})

	// Scenario US4.4 from specs/054-authzen-delegation-pip/spec.md
	It("fails startup on a PDP identifier mismatch without sending evaluation credentials", func() {
		broker.AdvertisedPDP = "https://other.example.com"
		env := bootstrap.NewTestEnvironment(grantContextStartupConfig(broker), opaLogger)
		DeferCleanup(env.Stop)

		err := env.StartWithGrantContext()

		Expect(err).To(MatchError(ContainSubstring("policy_decision_point mismatch (expected " + broker.URL() + ", advertised https://other.example.com)")))
		Expect(env.GRPCAddress()).To(BeEmpty())
		Expect(broker.DiscoveryCount()).To(Equal(1))
		Expect(broker.EvaluationCount()).To(BeZero())
		Expect(broker.CredentialCallCount()).To(BeZero(), "no credential reaches the mismatched PDP")
	})
})
