package e2e_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const (
	defaultAgentgatewayImage = "cr.agentgateway.dev/agentgateway:v1.5.0"

	agentgatewayClientID   = "https://gateway.example.com"
	brokerTokenEndpoint    = "https://broker.example.com/oauth2/token"
	agentgatewaySigningKID = "gateway-2026-09"
	agentgatewayKeyPath    = "/etc/agentgateway/keys/gateway-signing.key"
)

type mcpAuthorizationKey struct{}

// The agentgateway oauthTokenExchange policy (issue #26) authenticates to the broker with
// private_key_jwt, and the broker trusts its public key through an inline key set.
var _ = Describe("agentgateway oauthTokenExchange with an inline client-assertion key set", Label("docker"), func() {
	BeforeEach(func() {
		if err := bootstrap.CanAccessContainerRuntime(); err != nil {
			Expect(os.Getenv("CI")).To(BeEmpty(), "agentgateway E2E scenarios need Docker or Podman in CI")
			Skip("Skipping agentgateway E2E scenario: " + err.Error())
		}
	})

	Context("when the broker trusts agentgateway's inline key and agentgateway fronts an MCP server", func() {
		var (
			env             *trustAnchorEnv
			agentgatewayURL string
		)

		BeforeEach(func(ctx SpecContext) {
			env = newTrustAnchorEnv()
			DeferCleanup(env.close)
			gatewayKey := helpers.NewLocalRS256Key(agentgatewaySigningKID)
			config := env.localConfig(ports.ClientAssertionTrustConfig{
				IssuerURI: agentgatewayClientID,
				JWKS:      helpers.PublicKeySet(gatewayKey),
			})
			config.TokenExchange.Authorization.CEL.Expression = fmt.Sprintf("client_assertion.sub == %q", agentgatewayClientID)
			config.TokenExchange.ExpectedAudience = brokerTokenEndpoint
			config.OAuth2AuthServer.Local.TokenClaimsExpression = fmt.Sprintf(`{"agent_id": agent.id, "azp": agent.id, "aud": %q}`, brokerTokenEndpoint)
			config.TokenExchange.ClaimExtraction.AgentIDExpression = "has(subject_token.agent_id) ? subject_token.agent_id : resolveAgentIdByClientId(subject_token.azp)"
			env.start(config)

			mcpPort := startAuthorizationDigestMCPServer()
			agentgatewayURL = startAgentgatewayWithTokenExchange(ctx, hostPort(env.enduserServer.BaseURL()), mcpPort, gatewayKey)
		}, NodeTimeout(5*time.Minute))

		// US1-S2 from specs/052-inline-jwks-trust-anchors/spec.md
		It("exchanges the agent's broker token for the stored GitHub token before agentgateway calls the MCP server", func(ctx SpecContext) {
			mcpClient := connectMCPClient(ctx, agentgatewayURL, env.mintLocalSubjectToken())

			result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "whoami"}})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			Expect(result.Content).To(ConsistOf(HaveField("Text", authorizationDigest("Bearer github-token-xyz"))))
		}, NodeTimeout(time.Minute))
	})
})

func agentgatewayImage() string {
	if image := os.Getenv("AGENTGATEWAY_IMAGE"); image != "" {
		return image
	}
	return defaultAgentgatewayImage
}

// startAuthorizationDigestMCPServer serves a whoami tool that returns a digest of the received
// Authorization header, so a failed assertion cannot print a credential.
func startAuthorizationDigestMCPServer() int {
	server := mcpserver.NewMCPServer("agentgateway-e2e-tools", "1.0.0", mcpserver.WithToolCapabilities(false))
	server.AddTool(
		mcp.NewTool("whoami", mcp.WithDescription("Returns a digest of the Authorization header the MCP server received.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			authorization, _ := ctx.Value(mcpAuthorizationKey{}).(string)
			return mcp.NewToolResultText(authorizationDigest(authorization)), nil
		},
	)
	httpServer := httptest.NewServer(mcpserver.NewStreamableHTTPServer(server,
		mcpserver.WithEndpointPath("/mcp"),
		// agentgateway reaches this server through the testcontainers host tunnel with Host
		// host.testcontainers.internal.
		mcpserver.WithDisableLocalhostProtection(true),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, mcpAuthorizationKey{}, r.Header.Get("Authorization"))
		}),
	))
	DeferCleanup(httpServer.Close)
	return hostPort(httpServer.URL)
}

// startAgentgatewayWithTokenExchange runs the documented agentgateway configuration against the
// broker and MCP server on the host and returns agentgateway's base URL.
func startAgentgatewayWithTokenExchange(ctx context.Context, brokerPort, mcpPort int, signingKey *helpers.LocalKey) string {
	configYAML := fmt.Sprintf(`binds:
- port: 4000
  listeners:
  - routes:
    - backends:
      - mcp:
          targets:
          - name: tools
            mcp:
              host: http://host.testcontainers.internal:%d/mcp
        policies:
          backendAuth:
            oauthTokenExchange:
              host: http://host.testcontainers.internal:%d
              path: /oauth2/token
              resources:
              - %s
              clientAuth:
                method: privateKeyJwt
                clientId: %s
                assertionAudience: %s
                signingKey:
                  file: %s
                alg: RS256
                kid: %s
`, mcpPort, brokerPort, githubResourceURI, agentgatewayClientID, brokerTokenEndpoint, agentgatewayKeyPath, agentgatewaySigningKID)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:           agentgatewayImage(),
			ExposedPorts:    []string{"4000/tcp"},
			Cmd:             []string{"-f", "/etc/agentgateway/config.yaml"},
			HostAccessPorts: []int{brokerPort, mcpPort},
			Files: []testcontainers.ContainerFile{
				{Reader: strings.NewReader(configYAML), ContainerFilePath: "/etc/agentgateway/config.yaml", FileMode: 0o644},
				{Reader: strings.NewReader(signingKey.PrivatePEM()), ContainerFilePath: agentgatewayKeyPath, FileMode: 0o644},
			},
			WaitingFor: wait.ForListeningPort("4000/tcp").WithStartupTimeout(time.Minute),
		},
		Started: true,
	})
	DeferCleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		Expect(testcontainers.TerminateContainer(container, testcontainers.StopContext(cleanupCtx))).To(Succeed())
	})
	Expect(err).ToNot(HaveOccurred(), "failed to start agentgateway container")

	endpoint, err := container.PortEndpoint(ctx, "4000/tcp", "http")
	Expect(err).ToNot(HaveOccurred())
	return endpoint
}

func connectMCPClient(ctx context.Context, baseURL, bearerToken string) *client.Client {
	mcpClient, err := client.NewStreamableHttpClient(baseURL+"/mcp",
		transport.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + bearerToken}))
	Expect(err).ToNot(HaveOccurred())
	DeferCleanup(func() { _ = mcpClient.Close() })
	Expect(mcpClient.Start(ctx)).To(Succeed())

	initialize := mcp.InitializeRequest{}
	initialize.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initialize.Params.ClientInfo = mcp.Implementation{Name: "agentgateway-e2e-agent", Version: "1.0.0"}
	_, err = mcpClient.Initialize(ctx, initialize)
	Expect(err).ToNot(HaveOccurred())
	return mcpClient
}

func authorizationDigest(authorization string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(authorization)))
}

func hostPort(rawURL string) int {
	parsed, err := url.Parse(rawURL)
	Expect(err).ToNot(HaveOccurred())
	_, port, err := net.SplitHostPort(parsed.Host)
	Expect(err).ToNot(HaveOccurred())
	number, err := strconv.Atoi(port)
	Expect(err).ToNot(HaveOccurred())
	return number
}
