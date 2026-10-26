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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
	e2ehelpers "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// agentgwLogger writes structured test output to GinkgoWriter for test visibility.
var agentgwLogger = bootstrap.NewTestLogger()

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
		mcpHTTPServer        *http.Server
	)

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(func() {
			if mcpClient != nil {
				mcpClient.Close() //nolint:errcheck
			}
			if mcpHTTPServer != nil {
				mcpHTTPServer.Close() //nolint:errcheck
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

		var err error
		jwtFixture, err = fixtures.NewRS256JWTFixture(agentgwJWTIssuer, agentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create agentgateway JWT fixture")
		mintedJWT, err = jwtFixture.MintToken("agentgateway-e2e-subject", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred(), "failed to mint agentgateway JWT")

		// --- 1. Start mock identity broker (client_credentials + token exchange) ---
		mockOAuth2Srv = newAgentgwMockOAuth2Srv()
		mockTokenExchangeSvr = newAgentgwMockTokenExchangeSrv()

		// --- 2. Start mock MCP server using mcp-go (captures Authorization header) ---
		var mcpListener net.Listener
		mcpListener, mcpHTTPServer = startAgentgwMCPServer()
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
		setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer setupCancel()
		agentgatewayPort := startAgentgwContainer(setupCtx, extprocPort, mcpPort, 0, jwtFixture)
		agentgwLogger.Info("agentgateway accessible on host port", "port", agentgatewayPort)

		// --- 5. Create MCP client and connect through agentgateway ---
		agentgatewayURL := fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		initCtx, initCancel := context.WithTimeout(ctx, time.Minute)
		defer initCancel()
		mcpClient = connectAgentgwMCPClient(initCtx, agentgatewayURL, mintedJWT)
	})

	// US1-agentgw: Token exchange through agentgateway
	// Given an MCP client sends a Bearer token to agentgateway,
	// When agentgateway routes the request through ExtProc,
	// Then the MCP server receives the exchanged token (not the original).
	It("should exchange the Bearer token through agentgateway and deliver exchanged token to MCP server", NodeTimeout(time.Minute), func(ctx SpecContext) {
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
	It("should route the echo tool call through agentgateway to the MCP server", NodeTimeout(time.Minute), func(ctx SpecContext) {
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
func startAgentgwMCPServer() (net.Listener, *http.Server) {
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

	httpServer := &http.Server{Handler: httpSrv}
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			agentgwLogger.Error("MCP server error", "err", serveErr)
		}
	}()

	return listener, httpServer
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

func startAgentgwContainer(ctx context.Context, extprocPort, mcpPort, prmPort int, jwtFixture *fixtures.RS256JWTFixture) string {
	image := agentgatewayTestImage()
	resourceExpression := fmt.Sprintf("'%s'", fixtures.ValidResourceURI)

	routePrefix := ""
	policyHeader := "    - policies:"
	hostAccessPorts := []int{extprocPort, mcpPort}
	if prmPort > 0 {
		routePrefix = fmt.Sprintf(`    - name: protected-resource-metadata
      matches:
      - path:
          exact: /.well-known/oauth-protected-resource/mcp
      backends:
      - host: host.testcontainers.internal:%d
    - matches:
      - path:
          exact: /mcp
`, prmPort)
		policyHeader = "      policies:"
		hostAccessPorts = append(hostAccessPorts, prmPort)
	}

	// Generate agentgateway config pointing to host services
	configYAML := fmt.Sprintf(`binds:
- port: 4000
  listeners:
  - routes:
%s%s
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
`, routePrefix, policyHeader, jwtFixture.Issuer(), jwtFixture.Audience(), extprocPort, resourceExpression, mcpPort)

	agentgwLogger.Info("agentgateway config", "yaml", configYAML)

	req := testcontainers.ContainerRequest{
		Image:           image,
		ExposedPorts:    []string{"4000/tcp"},
		Cmd:             []string{"-f", "/config.yaml"},
		HostAccessPorts: hostAccessPorts,
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
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		Expect(container.Terminate(cleanupCtx)).To(Succeed(), "failed to terminate agentgateway container")
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

// --- Agentgateway Re-authentication Challenge Integration ---

const (
	agentgwRecoveryURL         = "https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"
	agentgwRecoveryDescription = "User session is unavailable. Please re-authenticate."
	agentgwRecoveryChallenge   = `Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"`
	agentgwUpstreamToken       = "agentgateway-reauth-upstream-token"
	agentgwStepUpClientID      = "e2e-stepup-client"
	agentgwStepUpRedirectURL   = "http://localhost/callback"
	agentgwStepUpCode          = "e2e-stepup-code"
)

type agentgwReAuthBroker struct {
	*httptest.Server
	mu       sync.Mutex
	failing  bool
	subjects []string
}

func newAgentgwReAuthBroker() *agentgwReAuthBroker {
	b := &agentgwReAuthBroker{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"id_token":"mock-id-token","token_type":"Bearer","expires_in":3600}`, agentgwMockAccessToken) //nolint:errcheck
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		b.subjects = append(b.subjects, r.PostForm.Get("subject_token"))
		failing := b.failing
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if failing {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q,"error_uri":%q}`, agentgwRecoveryDescription, agentgwRecoveryURL) //nolint:errcheck
			return
		}
		fmt.Fprintf(w, `{"access_token":%q,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":0}`, agentgwUpstreamToken) //nolint:errcheck
	})
	b.Server = httptest.NewServer(mux)
	return b
}

func (b *agentgwReAuthBroker) setFailing(failing bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failing = failing
}

func (b *agentgwReAuthBroker) exchangedSubjects() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.subjects...)
}

type agentgwStepUpAuthServer struct {
	listener    net.Listener
	server      *http.Server
	broker      *agentgwReAuthBroker
	jwtFixture  *fixtures.RS256JWTFixture
	mu          sync.Mutex
	resource    string
	authorize   []url.Values
	tokenForms  []url.Values
	issuedToken string
}

func newAgentgwStepUpAuthServer(broker *agentgwReAuthBroker, fixture *fixtures.RS256JWTFixture) *agentgwStepUpAuthServer {
	a := &agentgwStepUpAuthServer{broker: broker, jwtFixture: fixture}
	var err error
	a.listener, err = net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create step-up authorization server listener")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		resource := a.resource
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, resource, a.URL()) //nolint:errcheck
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"response_types_supported":["code"],"grant_types_supported":["authorization_code"],"code_challenge_methods_supported":["S256"],"token_endpoint_auth_methods_supported":["none"]}`, a.URL(), a.URL()+"/authorize", a.URL()+"/token") //nolint:errcheck
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		a.mu.Lock()
		a.authorize = append(a.authorize, q)
		a.mu.Unlock()
		a.broker.setFailing(false) // The user repaired the session during consent.
		redirect, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || redirect.String() != agentgwStepUpRedirectURL {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		values := redirect.Query()
		values.Set("code", agentgwStepUpCode)
		values.Set("state", q.Get("state"))
		redirect.RawQuery = values.Encode()
		http.Redirect(w, r, redirect.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		a.mu.Lock()
		a.tokenForms = append(a.tokenForms, r.PostForm)
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("code") != agentgwStepUpCode || r.PostForm.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_grant"}`) //nolint:errcheck
			return
		}
		token, err := a.jwtFixture.MintToken("agentgateway-stepup-user", time.Now().Add(20*time.Minute))
		if err != nil {
			http.Error(w, "failed to mint token", http.StatusInternalServerError)
			return
		}
		a.mu.Lock()
		a.issuedToken = token
		a.mu.Unlock()
		fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":600}`, token) //nolint:errcheck
	})
	a.server = &http.Server{Handler: mux}
	go func() {
		if serveErr := a.server.Serve(a.listener); serveErr != nil && serveErr != http.ErrServerClosed {
			agentgwLogger.Error("step-up authorization server error", "err", serveErr)
		}
	}()
	return a
}

func (a *agentgwStepUpAuthServer) Port() int { return a.listener.Addr().(*net.TCPAddr).Port }

func (a *agentgwStepUpAuthServer) URL() string { return fmt.Sprintf("http://127.0.0.1:%d", a.Port()) }

func (a *agentgwStepUpAuthServer) setResource(resource string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resource = resource
}

func (a *agentgwStepUpAuthServer) recorded() ([]url.Values, []url.Values, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]url.Values(nil), a.authorize...), append([]url.Values(nil), a.tokenForms...), a.issuedToken
}

var _ = Describe("Agentgateway Re-authentication Challenge", Ordered, func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		jwtFixture      *fixtures.RS256JWTFixture
		broker          *agentgwReAuthBroker
		authServer      *agentgwStepUpAuthServer
		extprocGRPC     *grpc.Server
		exchanger       *extprocserver.TokenExchanger
		mcpHTTPServer   *http.Server
		agentgatewayURL string
	)

	BeforeAll(func() {
		_, dockerErr := testcontainers.ProviderDocker.GetProvider()
		if dockerErr != nil {
			Skip("Skipping agentgateway elicitation tests: Docker not available")
		}

		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(func() {
			if authServer != nil {
				authServer.server.Close() //nolint:errcheck
			}
			if extprocGRPC != nil {
				extprocGRPC.GracefulStop()
			}
			if mcpHTTPServer != nil {
				mcpHTTPServer.Close() //nolint:errcheck
			}
			if exchanger != nil {
				exchanger.Shutdown()
			}
			if broker != nil {
				broker.Close()
			}
			cancel()
		})

		var err error
		jwtFixture, err = fixtures.NewRS256JWTFixture(agentgwJWTIssuer, agentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create challenge Agentgateway JWT fixture")
		broker = newAgentgwReAuthBroker()
		var mcpListener net.Listener
		mcpListener, mcpHTTPServer = startAgentgwMCPServer()
		var extprocListener net.Listener
		extprocListener, extprocGRPC, exchanger = startAgentgwExtProc(broker.URL, broker.URL)
		authServer = newAgentgwStepUpAuthServer(broker, jwtFixture)
		setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer setupCancel()
		port := startAgentgwContainer(setupCtx, extprocListener.Addr().(*net.TCPAddr).Port,
			mcpListener.Addr().(*net.TCPAddr).Port, authServer.Port(), jwtFixture)
		agentgatewayURL = "http://localhost:" + port
		authServer.setResource(agentgatewayURL + "/mcp")

		// The gateway must serve PRM at the exact URL go-sdk discovers after a 403.
		prmReq, err := http.NewRequestWithContext(setupCtx, http.MethodGet, agentgatewayURL+"/.well-known/oauth-protected-resource/mcp", nil)
		Expect(err).NotTo(HaveOccurred())
		prmResp, err := e2ehelpers.HTTPClient().Do(prmReq)
		Expect(err).NotTo(HaveOccurred())
		defer prmResp.Body.Close() //nolint:errcheck
		Expect(prmResp.StatusCode).To(Equal(http.StatusOK))
		var prm struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
		}
		Expect(json.NewDecoder(prmResp.Body).Decode(&prm)).To(Succeed())
		Expect(prm.Resource).To(Equal(agentgatewayURL + "/mcp"))
		Expect(prm.AuthorizationServers).To(Equal([]string{authServer.URL()}))
	})

	BeforeEach(func() { broker.setFailing(false) })

	// Feature 023 User Story 1 Amendment, Scenario 1 (specs/023-extproc-mcp-elicitation/spec.md): Challenge through agentgateway
	It("returns HTTP 403 with an insufficient_scope challenge and the URL elicitation body when the broker requires re-authentication", NodeTimeout(time.Minute), func(ctx SpecContext) {
		token, err := jwtFixture.MintToken("agentgateway-reauth-scenario-1", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		broker.setFailing(true)
		initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` +
			mcp.LATEST_PROTOCOL_VERSION + `","clientInfo":{"name":"e2e-challenge-client","version":"1.0.0"},"capabilities":{}}}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentgatewayURL+"/mcp", bytes.NewBufferString(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := e2ehelpers.HTTPClient().Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close() //nolint:errcheck
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(agentgwRecoveryChallenge))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		helpers.ExpectReauthElicitationBody(body, agentgwRecoveryURL, agentgwRecoveryDescription)
	})

	// Feature 023 User Story 1 Amendment, Scenario 2 (specs/023-extproc-mcp-elicitation/spec.md): mcp-go after initialize
	It("surfaces the challenge to an initialized mcp-go client as a typed URL elicitation error", NodeTimeout(time.Minute), func(ctx SpecContext) {
		token, err := jwtFixture.MintToken("agentgateway-reauth-scenario-2", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		mcpClient := connectAgentgwMCPClient(ctx, agentgatewayURL, token)
		DeferCleanup(func() { _ = mcpClient.Close() })
		broker.setFailing(true)
		_, err = mcpClient.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "whoami"}})
		var elicitation mcp.URLElicitationRequiredError
		Expect(errors.As(err, &elicitation)).To(BeTrue(), "CallTool should expose the typed URL elicitation error")
		Expect(elicitation.Elicitations).To(HaveLen(1))
		Expect(elicitation.Elicitations[0].URL).To(Equal(agentgwRecoveryURL))
	})

	// Feature 023 2026-10-26 Amendment, User Story 3 Scenario 1 (specs/023-extproc-mcp-elicitation/spec.md): Step-up authorization and retry
	It("lets a go-sdk client step up through the advertised authorization server and retry successfully", NodeTimeout(2*time.Minute), func(ctx SpecContext) {
		initial, err := jwtFixture.MintToken("agentgateway-stepup-user", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred())
		browser := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		fetcher := func(ctx context.Context, args *sdkauth.AuthorizationArgs) (*sdkauth.AuthorizationResult, error) {
			authURL, err := url.Parse(args.URL)
			if err != nil || authURL.Scheme+"://"+authURL.Host != authServer.URL() || authURL.Path != "/authorize" {
				return nil, fmt.Errorf("unexpected authorization endpoint: %q", args.URL)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := browser.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close() //nolint:errcheck
			if resp.StatusCode != http.StatusFound {
				return nil, fmt.Errorf("authorization endpoint returned HTTP %d", resp.StatusCode)
			}
			redirect, err := url.Parse(resp.Header.Get("Location"))
			if err != nil || redirect.Scheme+"://"+redirect.Host+redirect.Path != agentgwStepUpRedirectURL {
				return nil, fmt.Errorf("unexpected authorization redirect: %q", resp.Header.Get("Location"))
			}
			return &sdkauth.AuthorizationResult{Code: redirect.Query().Get("code"), State: redirect.Query().Get("state")}, nil
		}
		handler, err := sdkauth.NewAuthorizationCodeHandler(&sdkauth.AuthorizationCodeHandlerConfig{
			PreregisteredClient:      &oauthex.ClientCredentials{ClientID: agentgwStepUpClientID},
			RedirectURL:              agentgwStepUpRedirectURL,
			InitialTokenSource:       oauth2.StaticTokenSource(&oauth2.Token{AccessToken: initial, TokenType: "Bearer"}),
			AuthorizationCodeFetcher: fetcher,
		})
		Expect(err).NotTo(HaveOccurred())
		sdkClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "e2e-go-sdk-client", Version: "1.0.0"}, nil)
		session, err := sdkClient.Connect(ctx, &sdkmcp.StreamableClientTransport{
			Endpoint: agentgatewayURL + "/mcp", OAuthHandler: handler, DisableStandaloneSSE: true,
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(session.Close()).To(Succeed()) })

		broker.setFailing(true)
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		result, err := session.CallTool(callCtx, &sdkmcp.CallToolParams{Name: "whoami"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse())
		var text strings.Builder
		for _, content := range result.Content {
			if c, ok := content.(*sdkmcp.TextContent); ok {
				text.WriteString(c.Text)
			}
		}
		Expect(text.String()).To(ContainSubstring("auth_scheme=Bearer"))
		Expect(text.String()).To(ContainSubstring("token_digest=" + agentgwTokenDigest(agentgwUpstreamToken)))

		requests, forms, issued := authServer.recorded()
		Expect(requests).To(HaveLen(1), "step-up must issue exactly one authorization request")
		Expect(requests[0].Get("response_type")).To(Equal("code"))
		Expect(requests[0].Get("client_id")).To(Equal(agentgwStepUpClientID))
		Expect(requests[0].Get("code_challenge_method")).To(Equal("S256"))
		Expect(requests[0].Get("code_challenge")).NotTo(BeEmpty())
		Expect(requests[0].Get("resource")).To(Equal(agentgatewayURL + "/mcp"))
		Expect(forms).To(HaveLen(1), "step-up must exchange exactly one authorization code")
		Expect(forms[0].Get("grant_type")).To(Equal("authorization_code"))
		Expect(forms[0].Get("code_verifier")).NotTo(BeEmpty())
		Expect(issued).NotTo(BeEmpty())
		subjects := broker.exchangedSubjects()
		Expect(subjects).To(ContainElement(initial))
		Expect(subjects).NotTo(BeEmpty())
		Expect(subjects[len(subjects)-1]).To(Equal(issued), "the retried tool call must exchange the newly issued token")
	})
})
