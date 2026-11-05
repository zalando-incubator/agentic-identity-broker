package extproc_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

// The mock broker's token exchange returns alice@example.com and agent
// 7c9e6679-7425-40de-944b-e07fc1f90ae7 unless a test overrides them.
var _ = Describe("ExtProc OPA Identity Context", func() {
	Context("when a policy requires approval for one user", func() {
		var (
			env    *bootstrap.TestEnvironment
			broker *bootstrap.MockApprovalBroker
			stop   func()
		)

		BeforeEach(func() {
			env, broker, stop = startApprovalEnvironmentWithPolicy(nil, "approval_for_principal.rego", time.Minute, "Mcp-Session-Id")
			waitForBootstrap(broker)
		})

		AfterEach(func() {
			stop()
		})

		// Scenario US1.1 from specs/055-extproc-opa-identity-context/spec.md
		It("elicits approval when that user's agent calls the tool", func() {
			_, response := sendApprovalToolCall(env, "get_user", "agent-session")

			Expect(response).NotTo(BeNil())
			Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusOK)))
			Expect(elicitation(response)).To(Equal("https://broker.example/approvals/approval-1"))
			Expect(broker.CreateCount()).To(Equal(1))
		})

		// Scenario US1.2 from specs/055-extproc-opa-identity-context/spec.md
		It("forwards the same call for another user without creating an approval", func() {
			env.MockTokenExchange.WithApprovalIdentity("bob@example.com", "7c9e6679-7425-40de-944b-e07fc1f90ae7")

			_, response := sendApprovalToolCall(env, "get_user", "agent-session")

			Expect(response).To(helpers.BeForwardedRequestBody())
			Expect(broker.CreateCount()).To(Equal(0))
		})
	})

	Context("when a policy denies a tool for one agent", func() {
		// Scenario US2.1 from specs/055-extproc-opa-identity-context/spec.md
		It("denies that agent's call with the policy's reason", func() {
			env := bootstrap.NewTestEnvironment(opaEnabledConfig(policyPath("deny_for_agent.rego")), opaLogger)
			env.Start()
			defer env.Stop()

			_, response := sendApprovalToolCall(env, "get_user", "agent-session")

			Expect(response).To(helpers.HaveImmediateResponseWithStatus(http.StatusForbidden))
			Expect(response).To(helpers.HaveImmediateResponseWithBody("this agent may not look up users"))
		})
	})

	Context("when a policy denies any request that carries an identity", func() {
		var env *bootstrap.TestEnvironment

		BeforeEach(func() {
			env = bootstrap.NewTestEnvironment(opaEnabledConfig(policyPath("deny_when_identity_visible.rego")), opaLogger)
			env.Start()
			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
		})

		AfterEach(func() {
			env.Stop()
		})

		// Scenario US3.1 from specs/055-extproc-opa-identity-context/spec.md
		It("allows a header-only request, which is evaluated before token exchange", func() {
			client, conn := env.NewExtProcClient()
			defer conn.Close() //nolint:errcheck
			headers := helpers.NewRequestHeaders().
				WithHeader(":method", "GET").
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				WithAgentgatewayProtocol("mcp").
				WithEndOfStream(true).
				BuildWithMetadata()

			response := helpers.SendRequestHeaders(context.Background(), client, headers)

			Expect(response).To(helpers.HaveReplacedAuthorizationHeader("Bearer " + fixtures.FreshExchangedToken))
		})

		// Scenario US3.2 from specs/055-extproc-opa-identity-context/spec.md
		It("allows a tool call whose token exchange returns no principal or agent ID", func() {
			env.MockTokenExchange.WithApprovalIdentity("", "")

			_, response := sendApprovalToolCall(env, "get_user", "agent-session")

			Expect(response).To(helpers.BeForwardedRequestBody())
		})
	})
})
