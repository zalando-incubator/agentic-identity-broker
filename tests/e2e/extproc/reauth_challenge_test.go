package extproc_test

import (
	"context"
	"net/http"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

var _ = Describe("ExtProc Re-authentication Challenge", func() {
	var (
		env    *bootstrap.TestEnvironment
		client extprocv3.ExternalProcessorClient
		conn   *grpc.ClientConn
		logger = bootstrap.NewTestLogger()
	)

	BeforeEach(func() {
		env = bootstrap.NewTestEnvironment(fixtures.DefaultConfig(), logger)
		env.Start()
		client, conn = env.NewExtProcClient()
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close() //nolint:errcheck
		}
		if env != nil {
			env.Stop()
		}
	})

	// Feature 20260326-120000 User Story 1 Amendment, Scenario 4 (specs/20260326-120000-extproc-mcp-elicitation/spec.md): Non-MCP protocol
	It("returns the 403 insufficient_scope challenge instead of 503 for non-MCP protocol metadata", func() {
		const recoveryURL = "https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"
		env.MockTokenExchange.WithError(http.StatusBadRequest, "invalid_grant").WithErrorURI(recoveryURL)
		req := helpers.NewRequestHeaders().
			WithoutAuthorizationHeader().
			WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
			WithAgentgatewayProtocol("a2a").
			BuildWithMetadata()

		resp := helpers.SendRequestHeaders(context.Background(), client, req)

		Expect(resp).To(helpers.HaveImmediateResponseWithStatus(http.StatusForbidden))
		Expect(helpers.ExtractImmediateResponseHeader(resp, "www-authenticate")).To(Equal(
			`Bearer error="insufficient_scope", error_description="mock exchange error", error_uri="https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"`,
		))
		helpers.ExpectReauthElicitationBody([]byte(helpers.ExtractImmediateResponseBody(resp)), recoveryURL, "mock exchange error")
		Expect(helpers.ExtractMutatedAuthorizationHeader(resp)).To(BeEmpty())
	})
})
