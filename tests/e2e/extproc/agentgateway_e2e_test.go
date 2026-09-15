// Package extproc_test contains an E2E integration test that validates the ExtProc
// Token Exchange service works correctly with the real agentgateway Docker image.
// This test is part of the ExtProc E2E suite (TestExtProcTokenExchange) and uses Ginkgo.
//
// Test architecture:
//
//	MCP Client (mcp-go) → agentgateway (Docker) → ExtProc (in-process gRPC, SUT) → Mock Identity Broker (httptest)
//	                       agentgateway (Docker) → Mock MCP Server (mcp-go, host)
//
// The test verifies that:
//  1. An MCP client sends a request with a Bearer token to agentgateway
//  2. agentgateway passes the request through ExtProc for token exchange
//  3. ExtProc exchanges the token via the mock identity broker
//  4. The exchanged token replaces the original in the Authorization header
//  5. The MCP server receives the exchanged token (not the original)
package extproc_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
)

// agentgwLogger writes structured test output to GinkgoWriter for test visibility.
var agentgwLogger = bootstrap.NewTestLogger()

func init() {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
			panic(err)
		}
	}
}

// agentgwContextKey is used for storing request-scoped values in context.
type agentgwContextKey string

const agentgwAuthHeaderKey agentgwContextKey = "authorization"

const (
	defaultAgentgatewayImage = "cr.agentgateway.dev/agentgateway:v1.5.0"

	agentgwJWTIssuer   = "https://agentgateway.e2e.test"
	agentgwJWTAudience = "agentgateway-mcp"

	agentgwExchangedToken  = "exchanged-downstream-token-e2e"
	agentgwMockAccessToken = "mock-client-assertion-access-token"
)

func agentgatewayTestImage() string {
	if image := os.Getenv("AGENTGATEWAY_IMAGE"); image != "" {
		return image
	}

	return defaultAgentgatewayImage
}

// Agentgateway Integration describes the end-to-end flow through a real agentgateway
// Docker container: MCP client → agentgateway → ExtProc (SUT) → mock identity broker.
// The container setup is shared across all specs (Ordered + BeforeAll) since starting
// agentgateway is expensive.
var _ = Describe("Agentgateway Integration", Ordered, func() {
	var (
		ctx                  context.Context
		cancel               context.CancelFunc
		jwtFixture           *fixtures.RS256JWTFixture
		mintedJWT            string
		mockOAuth2Srv        *httptest.Server
		mockTokenExchangeSvr *agentgwMockTokenExchangeSrv
		extprocGRPC          *grpc.Server
		exchanger            *extprocserver.TokenExchanger
		mcpClient            *client.Client
	)

	BeforeAll(func() {

		ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)

		var err error
		jwtFixture, err = fixtures.NewRS256JWTFixture(agentgwJWTIssuer, agentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create agentgateway JWT fixture")
		mintedJWT, err = jwtFixture.MintToken("agentgateway-e2e-subject", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred(), "failed to mint agentgateway JWT")

		// --- 1. Start mock identity broker (client_credentials + token exchange) ---
		mockOAuth2Srv = newAgentgwMockOAuth2Srv()
		mockTokenExchangeSvr = newAgentgwMockTokenExchangeSrv()

		// --- 2. Start mock MCP server using mcp-go (captures Authorization header) ---
		mcpListener := startAgentgwMCPServer()
		mcpPort := mcpListener.Addr().(*net.TCPAddr).Port
		agentgwLogger.Info("Mock MCP server listening", "port", mcpPort)

		// --- 3. Start ExtProc gRPC server (system under test) ---
		var extprocListener net.Listener
		extprocListener, extprocGRPC, exchanger = startAgentgwExtProc(
			mockOAuth2Srv.URL, mockTokenExchangeSvr.URL,
		)
		extprocPort := extprocListener.Addr().(*net.TCPAddr).Port
		agentgwLogger.Info("ExtProc gRPC server listening", "port", extprocPort)

		// --- 4. Start agentgateway Docker container ---
		agentgatewayPort := startAgentgwContainer(ctx, extprocPort, mcpPort, jwtFixture)
		agentgwLogger.Info("agentgateway accessible on host port", "port", agentgatewayPort)

		// --- 5. Create MCP client and connect through agentgateway ---
		agentgatewayURL := fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		mcpClient = connectAgentgwMCPClient(ctx, agentgatewayURL, mintedJWT)

		DeferCleanup(func() {
			if mcpClient != nil {
				mcpClient.Close() //nolint:errcheck
			}
			if extprocGRPC != nil {
				extprocGRPC.GracefulStop()
			}
			if exchanger != nil {
				exchanger.Shutdown()
			}
			if mockTokenExchangeSvr != nil {
				mockTokenExchangeSvr.Close()
			}
			if mockOAuth2Srv != nil {
				mockOAuth2Srv.Close()
			}
			cancel()
		})
	})

	// US1-agentgw: Token exchange through agentgateway
	// Given an MCP client sends a Bearer token to agentgateway,
	// When agentgateway routes the request through ExtProc,
	// Then the MCP server receives the exchanged token (not the original).
	It("should exchange the Bearer token through agentgateway and deliver exchanged token to MCP server", func() {
		result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "whoami",
			},
		})
		Expect(err).NotTo(HaveOccurred(), "CallTool should succeed")
		Expect(result).NotTo(BeNil(), "CallTool result should not be nil")
		Expect(result.IsError).To(BeFalse(), "CallTool should not return an error result")
		Expect(result.Content).NotTo(BeEmpty(), "CallTool result should have content")

		// The MCP server's whoami tool returns a credential digest so a failed assertion
		// cannot expose the suite-minted JWT.
		text := agentgwExtractTextContent(result)

		Expect(text).To(ContainSubstring("auth_scheme=Bearer"),
			"MCP server should have received a Bearer token")
		Expect(text).To(ContainSubstring("token_digest="+agentgwTokenDigest(agentgwExchangedToken)),
			"MCP server should have received the exchanged token")

		Expect(mockTokenExchangeSvr.callCount()).To(BeNumerically(">=", 1),
			"Token exchange endpoint should have been called at least once")
	})

	// Echo tool: basic functional test through agentgateway
	It("should route the echo tool call through agentgateway to the MCP server", func() {
		result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "echo",
				Arguments: map[string]any{"message": "hello from e2e"},
			},
		})
		Expect(err).NotTo(HaveOccurred(), "CallTool echo should succeed")
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse())

		text := agentgwExtractTextContent(result)
		Expect(text).To(ContainSubstring("hello from e2e"),
			"echo tool should return the sent message")
	})
})

// --- Mock OAuth2 Server (client_credentials) ---

func newAgentgwMockOAuth2Srv() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, //nolint:errcheck
			`{"access_token":%q,"id_token":"mock-id-token","token_type":"Bearer","expires_in":3600}`,
			agentgwMockAccessToken,
		)
	})
	return httptest.NewServer(mux)
}

// --- Mock Token Exchange Server (identity broker) ---

type agentgwMockTokenExchangeSrv struct {
	*httptest.Server
	mu    sync.Mutex
	calls int
}

func newAgentgwMockTokenExchangeSrv() *agentgwMockTokenExchangeSrv {
	m := &agentgwMockTokenExchangeSrv{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.calls++
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, //nolint:errcheck
			`{"access_token":%q,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":3600}`,
			agentgwExchangedToken,
		)
	})
	m.Server = httptest.NewServer(mux)
	return m
}

func (m *agentgwMockTokenExchangeSrv) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// --- Mock MCP Server (using mcp-go) ---

// startAgentgwMCPServer creates an MCP server with "echo" and "whoami" tools using the mcp-go library.
// The "whoami" tool returns a digest of the Authorization credential so the test can
// verify token exchange without exposing a suite-minted JWT.
func startAgentgwMCPServer() net.Listener {
	mcpSvr := mcpserver.NewMCPServer(
		"e2e-mock-mcp-server", "1.0.0",
		mcpserver.WithToolCapabilities(false),
	)

	// Register "echo" tool
	echoTool := mcp.NewTool("echo",
		mcp.WithDescription("Echoes the provided message back to the caller."),
		mcp.WithString("message", mcp.Description("The message to echo"), mcp.Required()),
	)
	mcpSvr.AddTool(echoTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		msg := req.GetString("message", "")
		return mcp.NewToolResultText(fmt.Sprintf("echo: %s", msg)), nil
	})

	// Register "whoami" tool — returns a digest of the Authorization credential.
	whoamiTool := mcp.NewTool("whoami",
		mcp.WithDescription("Returns a digest of the Authorization token visible to the MCP server."),
	)
	mcpSvr.AddTool(whoamiTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		authHeader, _ := ctx.Value(agentgwAuthHeaderKey).(string)
		scheme := "none"
		token := ""
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			scheme = parts[0]
			if len(parts) == 2 {
				token = parts[1]
			}
		}
		return mcp.NewToolResultText(fmt.Sprintf("auth_scheme=%s\ntoken_digest=%s", scheme, agentgwTokenDigest(token))), nil
	})

	// Create the streamable HTTP server with context injection for Authorization header
	httpSrv := mcpserver.NewStreamableHTTPServer(mcpSvr,
		mcpserver.WithEndpointPath("/mcp"),
		// Agentgateway reaches this host fixture through a loopback proxy but preserves
		// host.testcontainers.internal. This fixture is not browser-facing.
		mcpserver.WithDisableLocalhostProtection(true),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, agentgwAuthHeaderKey, r.Header.Get("Authorization"))
		}),
	)

	// Listen on a random port (0.0.0.0 so Docker container can reach us)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create MCP server listener")

	go func() {
		srv := &http.Server{Handler: httpSrv}
		if serveErr := srv.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			agentgwLogger.Error("MCP server error", "err", serveErr)
		}
	}()

	return listener
}

// --- ExtProc gRPC Server (System Under Test) ---

func startAgentgwExtProc(
	oauth2URL, tokenExchangeURL string,
) (net.Listener, *grpc.Server, *extprocserver.TokenExchanger) {
	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{
			Bind:                 "0.0.0.0",
			Port:                 0, // will use listener port
			MaxConcurrentStreams: 100,
		},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             tokenExchangeURL + "/oauth2/token",
			Issuer:                    oauth2URL,
			ClientID:                  "e2e-extproc-client",
			ClientSecret:              "e2e-client-secret",
			ClientCredentialsEndpoint: oauth2URL + "/oauth/token",
			ClientAssertionType:       "access_token",
			ExchangeTimeout:           10 * time.Second,
			TLS: extprocconfig.TLSConfig{
				AllowHTTP: true,
			},
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		Log: extprocconfig.LogConfig{
			Level:  "debug",
			Format: "text",
		},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
	}

	slogLogger := bootstrap.NewTestLogger()
	exchanger, err := extprocserver.NewTokenExchanger(cfg, slogLogger)
	Expect(err).NotTo(HaveOccurred(), "failed to create token exchanger")

	svc := extprocserver.NewServer(cfg, exchanger, slogLogger)

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create ExtProc listener")

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	go func() {
		if serveErr := grpcSrv.Serve(listener); serveErr != nil {
			agentgwLogger.Error("ExtProc gRPC server error", "err", serveErr)
		}
	}()

	return listener, grpcSrv, exchanger
}

// --- agentgateway Docker Container ---

func startAgentgwContainer(ctx context.Context, extprocPort, mcpPort int, jwtFixture *fixtures.RS256JWTFixture) string {
	image := agentgatewayTestImage()
	resourceExpression := fmt.Sprintf("'%s'", fixtures.ValidResourceURI)

	// Generate agentgateway config pointing to host services
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
`, jwtFixture.Issuer(), jwtFixture.Audience(), extprocPort, resourceExpression, mcpPort)

	agentgwLogger.Info("agentgateway config", "yaml", configYAML)

	req := testcontainers.ContainerRequest{
		Image:           image,
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
				Reader:            strings.NewReader(jwtFixture.JWKSJSON()),
				ContainerFilePath: "/jwks.json",
				FileMode:          0644,
			},
		},
		WaitingFor: wait.ForListeningPort("4000/tcp").WithStartupTimeout(30 * time.Second),
	}

	agentgwLogger.Info("starting agentgateway container", "image", image)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to start agentgateway container")

	DeferCleanup(func() {
		if termErr := container.Terminate(ctx); termErr != nil {
			agentgwLogger.Error("failed to terminate agentgateway container", "err", termErr)
		}
	})

	mappedPort, err := container.MappedPort(ctx, "4000")
	Expect(err).NotTo(HaveOccurred(), "failed to get mapped port")

	return mappedPort.Port()
}

// --- MCP Client ---

func connectAgentgwMCPClient(ctx context.Context, agentgatewayURL, token string) *client.Client {
	mcpClient, err := client.NewStreamableHttpClient(
		agentgatewayURL+"/mcp",
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to create MCP client")

	err = mcpClient.Start(ctx)
	Expect(err).NotTo(HaveOccurred(), "failed to start MCP client")

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	Expect(err).NotTo(HaveOccurred(), "MCP Initialize should succeed")

	return mcpClient
}

// --- Helpers ---

func agentgwExtractTextContent(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func agentgwTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", digest)
}

// --- Agentgateway Elicitation Integration ---
// Validates the URLElicitationRequiredError path: when the identity broker returns
// an RFC 8693 error with error_uri, ExtProc returns JSON-RPC code -32042 with an
// elicitations array directly from the headers phase (id: null, per JSON-RPC 2.0 §5).

const agentgwElicitationReAuthURL = "https://broker.example.com/api/third-party/svc-elicitation/oauth2/authorize"

// newAgentgwElicitationBroker returns a mock broker that accepts client_credentials at
// /oauth/token (200) but rejects token exchange at /oauth2/token (401 + error_uri).
func newAgentgwElicitationBroker() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, //nolint:errcheck
			`{"access_token":%q,"token_type":"Bearer","expires_in":3600}`,
			agentgwMockAccessToken,
		)
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, //nolint:errcheck
			`{"error":"invalid_grant","error_description":"Session expired, please re-authenticate","error_uri":%q}`,
			agentgwElicitationReAuthURL,
		)
	})
	return httptest.NewServer(mux)
}

var _ = Describe("Agentgateway Elicitation Integration", Ordered, func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		jwtFixture        *fixtures.RS256JWTFixture
		mintedJWT         string
		elicitationBroker *httptest.Server
		extprocGRPC       *grpc.Server
		exchanger         *extprocserver.TokenExchanger
		agentgatewayURL   string
	)

	BeforeAll(func() {
		_, dockerErr := testcontainers.ProviderDocker.GetProvider()
		if dockerErr != nil {
			Skip("Skipping agentgateway elicitation tests: Docker not available")
		}

		ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)

		var err error
		jwtFixture, err = fixtures.NewRS256JWTFixture(agentgwJWTIssuer, agentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create elicitation Agentgateway JWT fixture")
		mintedJWT, err = jwtFixture.MintToken("agentgateway-elicitation-subject", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred(), "failed to mint elicitation Agentgateway JWT")

		// 1. Broker: client_credentials succeeds, token exchange returns 401 + error_uri.
		elicitationBroker = newAgentgwElicitationBroker()

		// 2. MCP server: needed by agentgateway config; the elicitation response is
		//    returned before agentgateway forwards to the backend, so it is never called.
		mcpListener := startAgentgwMCPServer()
		mcpPort := mcpListener.Addr().(*net.TCPAddr).Port

		// 3. ExtProc (system under test).
		var extprocListener net.Listener
		extprocListener, extprocGRPC, exchanger = startAgentgwExtProc(
			elicitationBroker.URL, elicitationBroker.URL,
		)
		extprocPort := extprocListener.Addr().(*net.TCPAddr).Port
		agentgwLogger.Info("Elicitation ExtProc listening", "port", extprocPort)

		// 4. agentgateway Docker container.
		agentgatewayPort := startAgentgwContainer(ctx, extprocPort, mcpPort, jwtFixture)
		agentgatewayURL = fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		agentgwLogger.Info("Elicitation agentgateway accessible", "url", agentgatewayURL)

		DeferCleanup(func() {
			if extprocGRPC != nil {
				extprocGRPC.GracefulStop()
			}
			if exchanger != nil {
				exchanger.Shutdown()
			}
			if elicitationBroker != nil {
				elicitationBroker.Close()
			}
			cancel()
		})
	})

	// Scenario 1.1 from specs/023-extproc-mcp-elicitation/spec.md:
	// When the broker returns error_uri on token exchange failure, ExtProc returns
	// HTTP 200 with a JSON-RPC -32042 URLElicitationRequiredError body.
	//
	// We use a raw HTTP client here because the mcp-go StreamableHTTP transport
	// rejects responses with "id": null (treating them as orphan notifications).
	// Per JSON-RPC 2.0 §5, null id is correct when the request id is indeterminate
	// — which is the case here since ExtProc short-circuits from the headers phase
	// before the request body (containing the id) is read.
	It("should return URLElicitationRequiredError with re-auth URL when token exchange fails with error_uri", func() {
		// Build a JSON-RPC Initialize request (id=1) — the same request mcp-go would send.
		initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` +
			mcp.LATEST_PROTOCOL_VERSION + `","clientInfo":{"name":"e2e-elicitation-client","version":"1.0.0"},"capabilities":{}}}`

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentgatewayURL+"/mcp",
			bytes.NewBufferString(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+mintedJWT)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close() //nolint:errcheck

		// JSON-RPC errors travel over HTTP 200 (per JSON-RPC 2.0).
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"elicitation response must be HTTP 200")

		// Decode the JSON-RPC error response.
		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			ID      any    `json:"id"`
			Error   struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    struct {
					Elicitations []struct {
						Mode          string `json:"mode"`
						ElicitationID string `json:"elicitationId"`
						URL           string `json:"url"`
						Message       string `json:"message"`
					} `json:"elicitations"`
				} `json:"data"`
			} `json:"error"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&envelope)).To(Succeed())

		Expect(envelope.JSONRPC).To(Equal("2.0"))
		// id is null because ExtProc responds from the headers phase, before the
		// request body (which contains the JSON-RPC id) is available (JSON-RPC 2.0 §5).
		Expect(envelope.ID).To(BeNil(), "id must be JSON null")
		Expect(envelope.Error.Code).To(Equal(mcp.URL_ELICITATION_REQUIRED),
			"must use JSON-RPC error code -32042")

		Expect(envelope.Error.Data.Elicitations).To(HaveLen(1))
		Expect(envelope.Error.Data.Elicitations[0].Mode).To(Equal(mcp.ElicitationModeURL))
		Expect(envelope.Error.Data.Elicitations[0].URL).To(Equal(agentgwElicitationReAuthURL),
			"elicitation URL must match the error_uri from the broker")
		Expect(envelope.Error.Data.Elicitations[0].ElicitationID).NotTo(BeEmpty(),
			"elicitationId must be set")
	})
})
