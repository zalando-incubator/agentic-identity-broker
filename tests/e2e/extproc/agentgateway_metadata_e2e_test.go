package extproc_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"

	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
)

const (
	metadataGatewayJWTIssuer    = "https://metadata-input.issuer.test"
	metadataGatewayJWTAudience  = "metadata-input-mcp"
	metadataGatewayResourceURI  = "https://metadata-input.mcp.test/mcp"
	metadataGatewayExchangedJWT = "metadata-input-exchanged-token"
)

var metadataGatewayLogger = bootstrap.NewTestLogger()

// metadataTokenExchangeInput records only a digest of the credential so a failing
// assertion cannot expose the suite-minted JWT in test output.
type metadataTokenExchangeInput struct {
	calls              int
	hasSubjectToken    bool
	subjectTokenDigest [sha256.Size]byte
	resourceURI        string
}

type metadataTokenExchangeServer struct {
	*httptest.Server
	mu    sync.RWMutex
	input metadataTokenExchangeInput
}

func newMetadataTokenExchangeServer() *metadataTokenExchangeServer {
	server := &metadataTokenExchangeServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		subjectToken := r.Form.Get("subject_token")
		server.mu.Lock()
		server.input.calls++
		server.input.hasSubjectToken = subjectToken != ""
		server.input.subjectTokenDigest = metadataTokenDigest(subjectToken)
		server.input.resourceURI = r.Form.Get("resource")
		server.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, //nolint:errcheck
			`{"access_token":%q,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":3600}`,
			metadataGatewayExchangedJWT,
		)
	})
	server.Server = httptest.NewServer(mux)
	return server
}

func (s *metadataTokenExchangeServer) latestInput() metadataTokenExchangeInput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.input
}

func (s *metadataTokenExchangeServer) reset() {
	s.mu.Lock()
	s.input = metadataTokenExchangeInput{}
	s.mu.Unlock()
}

type metadataMCPAuthKey struct{}

type metadataMCPAuthorization struct {
	hasCredential    bool
	calls            int
	scheme           string
	credentialDigest [sha256.Size]byte
}

type metadataMCPAuthorizationObserver struct {
	mu            sync.RWMutex
	authorization metadataMCPAuthorization
}

func (o *metadataMCPAuthorizationObserver) observe(header string) {
	scheme, credential, hasSeparator := strings.Cut(header, " ")
	observation := metadataMCPAuthorization{
		hasCredential: hasSeparator && credential != "",
		scheme:        scheme,
	}
	if observation.hasCredential {
		observation.credentialDigest = metadataTokenDigest(credential)
	}

	o.mu.Lock()
	o.authorization.calls++
	observation.calls = o.authorization.calls
	o.authorization = observation
	o.mu.Unlock()
}

func (o *metadataMCPAuthorizationObserver) reset() {
	o.mu.Lock()
	o.authorization = metadataMCPAuthorization{}
	o.mu.Unlock()
}

func (o *metadataMCPAuthorizationObserver) latest() metadataMCPAuthorization {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.authorization
}

func metadataTokenDigest(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

func startMetadataGatewayMCPServer(observer *metadataMCPAuthorizationObserver) (net.Listener, *http.Server) {
	mcpService := mcpserver.NewMCPServer(
		"metadata-input-mcp-server", "1.0.0",
		mcpserver.WithToolCapabilities(false),
	)
	whoamiTool := mcp.NewTool(
		"whoami",
		mcp.WithDescription("Confirms that the MCP server processed the request."),
	)
	mcpService.AddTool(whoamiTool, func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		authHeader, _ := ctx.Value(metadataMCPAuthKey{}).(string)
		observer.observe(authHeader)
		return mcp.NewToolResultText("authorization observed"), nil
	})

	httpServer := &http.Server{Handler: mcpserver.NewStreamableHTTPServer(
		mcpService,
		mcpserver.WithEndpointPath("/mcp"),
		// Agentgateway reaches this host fixture through a loopback proxy but preserves
		// host.testcontainers.internal. This fixture is not browser-facing.
		mcpserver.WithDisableLocalhostProtection(true),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, metadataMCPAuthKey{}, r.Header.Get("Authorization"))
		}),
	)}

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create metadata MCP server listener")

	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			metadataGatewayLogger.Error("metadata MCP server error", "err", serveErr)
		}
	}()

	return listener, httpServer
}

func startMetadataGatewayContainer(
	ctx context.Context,
	extprocPort, mcpPort int,
	issuer, audience, jwksJSON string,
) string {
	resourceExpression := fmt.Sprintf("'%s'", metadataGatewayResourceURI)
	configYAML := fmt.Sprintf(`binds:
- port: 4000
  listeners:
  - routes:
    - policies:
        jwtAuth:
          mode: strict
          preserveToken: false
          providers:
          - issuer: %q
            audiences:
            - %q
            jwks:
              file: /jwks.json
        extProc:
          host: "host.testcontainers.internal:%d"
          failureMode: failClosed
          metadataContext:
            aib.tokenexchange:
              subject_token: "jwt.rawToken.unredacted()"
              resource_uri: %q
      backends:
      - mcp:
          targets:
          - name: tools
            mcp:
              host: http://host.testcontainers.internal:%d/mcp
`, issuer, audience, extprocPort, resourceExpression, mcpPort)

	request := testcontainers.ContainerRequest{
		Image:           agentgatewayTestImage(),
		ExposedPorts:    []string{"4000/tcp"},
		Cmd:             []string{"-f", "/config.yaml"},
		HostAccessPorts: []int{extprocPort, mcpPort},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(configYAML),
				ContainerFilePath: "/config.yaml",
				FileMode:          0644,
			},
			{
				Reader:            strings.NewReader(jwksJSON),
				ContainerFilePath: "/jwks.json",
				FileMode:          0644,
			},
		},
		WaitingFor: wait.ForListeningPort("4000/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to start metadata Agentgateway container")

	DeferCleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		Expect(container.Terminate(cleanupCtx)).To(Succeed(), "failed to terminate metadata Agentgateway container")
	})

	mappedPort, err := container.MappedPort(ctx, "4000")
	Expect(err).NotTo(HaveOccurred(), "failed to get metadata Agentgateway port")
	return mappedPort.Port()
}

func connectMetadataGatewayMCPClient(ctx context.Context, agentgatewayURL, token string) *client.Client {
	mcpClient, err := client.NewStreamableHttpClient(
		agentgatewayURL+"/mcp",
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to create metadata MCP client")

	err = mcpClient.Start(ctx)
	Expect(err).NotTo(HaveOccurred(), "failed to start metadata MCP client")

	initializeRequest := mcp.InitializeRequest{}
	initializeRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initializeRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "metadata-input-e2e-client",
		Version: "1.0.0",
	}
	_, err = mcpClient.Initialize(ctx, initializeRequest)
	Expect(err).NotTo(HaveOccurred(), "metadata MCP Initialize should succeed")

	return mcpClient
}

func expectMetadataMCPReceivesExchangedToken(observer *metadataMCPAuthorizationObserver) {
	observedAuthorization := observer.latest()
	Expect(observedAuthorization.calls).To(BeNumerically(">=", 1), "whoami must reach the MCP handler")
	Expect(observedAuthorization.hasCredential).To(BeTrue(), "MCP server should receive an authorization credential")
	Expect(observedAuthorization.scheme).To(Equal("Bearer"), "MCP server should receive a Bearer credential")
	Expect(observedAuthorization.credentialDigest).To(Equal(metadataTokenDigest(metadataGatewayExchangedJWT)),
		"MCP server should receive only the exchanged credential")
}

var _ = Describe("Metadata Input via Agentgateway", Ordered, func() {
	var (
		ctx                 context.Context
		cancel              context.CancelFunc
		jwtFixture          *fixtures.RS256JWTFixture
		agentgatewayURL     string
		mockOAuth2Server    *httptest.Server
		tokenExchangeServer *metadataTokenExchangeServer
		mcpAuthorization    *metadataMCPAuthorizationObserver
		mcpHTTPServer       *http.Server
		extprocGRPC         *grpc.Server
		exchanger           *extprocserver.TokenExchanger
	)

	BeforeAll(func() {
		// testcontainers-go v0.41.0 can panic rather than return an error when Docker is unavailable.
		var dockerErr error
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					dockerErr = fmt.Errorf("Docker availability check panicked: %v", recovered)
				}
			}()
			_, dockerErr = testcontainers.ProviderDocker.GetProvider()
		}()
		if dockerErr != nil {
			Skip(fmt.Sprintf("Skipping metadata Agentgateway tests: Docker unavailable: %v", dockerErr))
		}

		var err error
		jwtFixture, err = fixtures.NewRS256JWTFixture(metadataGatewayJWTIssuer, metadataGatewayJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create metadata gateway JWT fixture")

		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(func() {
			if extprocGRPC != nil {
				extprocGRPC.GracefulStop()
			}
			if exchanger != nil {
				exchanger.Shutdown()
			}
			if mcpHTTPServer != nil {
				mcpHTTPServer.Close() //nolint:errcheck
			}
			if tokenExchangeServer != nil {
				tokenExchangeServer.Close()
			}
			if mockOAuth2Server != nil {
				mockOAuth2Server.Close()
			}
			cancel()
		})
		mockOAuth2Server = newAgentgwMockOAuth2Srv()
		tokenExchangeServer = newMetadataTokenExchangeServer()
		mcpAuthorization = &metadataMCPAuthorizationObserver{}
		mcpListener, mcpServer := startMetadataGatewayMCPServer(mcpAuthorization)
		mcpHTTPServer = mcpServer
		mcpPort := mcpListener.Addr().(*net.TCPAddr).Port

		var extprocListener net.Listener
		extprocListener, extprocGRPC, exchanger = startAgentgwExtProc(
			mockOAuth2Server.URL, tokenExchangeServer.URL,
		)
		extprocPort := extprocListener.Addr().(*net.TCPAddr).Port

		setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer setupCancel()
		agentgatewayPort := startMetadataGatewayContainer(
			setupCtx,
			extprocPort,
			mcpPort,
			jwtFixture.Issuer(),
			jwtFixture.Audience(),
			jwtFixture.JWKSJSON(),
		)
		agentgatewayURL = fmt.Sprintf("http://localhost:%s", agentgatewayPort)
	})

	BeforeEach(func() {
		tokenExchangeServer.reset()
		mcpAuthorization.reset()
	})

	Context("US1: Exchange From Dynamic Metadata", func() {
		// 043-extproc-metadata-input: US1-S1 — specs/043-extproc-metadata-input/spec.md
		It("exchanges the JWT-validated raw token for the configured MCP resource", NodeTimeout(time.Minute), func(ctx SpecContext) {
			mintedJWT, err := jwtFixture.MintToken("metadata-input-us1-s1", time.Now().Add(10*time.Minute))
			Expect(err).NotTo(HaveOccurred(), "failed to mint US1-S1 metadata gateway JWT")
			mcpClient := connectMetadataGatewayMCPClient(ctx, agentgatewayURL, mintedJWT)
			DeferCleanup(func() {
				mcpClient.Close() //nolint:errcheck
			})
			mcpAuthorization.reset()

			result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: "whoami"},
			})
			Expect(err).NotTo(HaveOccurred(), "whoami tool call should succeed")
			Expect(result).NotTo(BeNil(), "whoami tool result should not be nil")
			Expect(result.IsError).To(BeFalse(), "whoami should not return an MCP error")

			input := tokenExchangeServer.latestInput()
			Expect(input.calls).To(Equal(1), "the scenario should perform exactly one token exchange")
			Expect(input.hasSubjectToken).To(BeTrue(), "token exchange should receive a subject token")
			Expect(input.subjectTokenDigest).To(Equal(metadataTokenDigest(mintedJWT)),
				"token exchange should receive the JWT raw token projected from metadata")
			Expect(input.resourceURI).To(Equal(metadataGatewayResourceURI),
				"token exchange should use the resource URI from Agentgateway metadata")
			expectMetadataMCPReceivesExchangedToken(mcpAuthorization)
		})
	})

	Context("US2: Configure Instance Resource", func() {
		// 043-extproc-metadata-input: US2-S1 — specs/043-extproc-metadata-input/spec.md
		It("exchanges for the resource URL configured in the instance ExtProc policy", NodeTimeout(time.Minute), func(ctx SpecContext) {
			mintedJWT, err := jwtFixture.MintToken("metadata-input-us2-s1", time.Now().Add(10*time.Minute))
			Expect(err).NotTo(HaveOccurred(), "failed to mint US2-S1 metadata gateway JWT")
			mcpClient := connectMetadataGatewayMCPClient(ctx, agentgatewayURL, mintedJWT)
			DeferCleanup(func() {
				mcpClient.Close() //nolint:errcheck
			})
			mcpAuthorization.reset()

			result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
				Params: mcp.CallToolParams{Name: "whoami"},
			})
			Expect(err).NotTo(HaveOccurred(), "whoami tool call should succeed")
			Expect(result).NotTo(BeNil(), "whoami tool result should not be nil")
			Expect(result.IsError).To(BeFalse(), "whoami should not return an MCP error")

			input := tokenExchangeServer.latestInput()
			Expect(input.calls).To(Equal(1), "the scenario should perform exactly one token exchange")
			Expect(input.hasSubjectToken).To(BeTrue(), "token exchange should receive a subject token")
			Expect(input.subjectTokenDigest).To(Equal(metadataTokenDigest(mintedJWT)),
				"token exchange should receive the JWT raw token projected from metadata")
			Expect(input.resourceURI).To(Equal(metadataGatewayResourceURI),
				"token exchange should use the resource URI from Agentgateway metadata")
			expectMetadataMCPReceivesExchangedToken(mcpAuthorization)
		})
	})
})
