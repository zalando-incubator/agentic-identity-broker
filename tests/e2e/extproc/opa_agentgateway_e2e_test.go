// Package extproc_test contains E2E acceptance tests for OPA authorization via agentgateway.
//
// This file covers US1 acceptance scenarios from specs/020-extproc-opa-authorization/spec.md.
// It requires Docker (agentgateway container) and is skipped when Docker is unavailable.
//
// Test architecture:
//
//	MCP Client (mcp-go) → agentgateway (Docker) → ExtProc (in-process, OPA enabled) → Mock Identity Broker
//	                       agentgateway (Docker) → Mock MCP Server (mcp-go, host)
//
// Agentgateway passes the protocol type ("mcp") in MetadataContext to ExtProc.
// ExtProc evaluates the policy and either allows (→ token exchange) or denies (→ 403).
//
// Scenario mapping:
//   - US1 Scenario 1: Allow read-only tool call + token exchange
//   - US1 Scenario 2: Deny destructive tool call with 403 + reasons
//   - US1 Scenario 3: Default deny for unmatched tool
//   - US1 Scenario 4: Non-tool-call MCP methods evaluated by policy
//   - Edge: permission-gated tool call allowed via granted permission sets (FR-024)
//   - Edge: mcp_headers_only GET /mcp allowed + exchanged token forwarded (FR-018)
//   - Edge: missing protocol metadata → 403
//
// Ordered suite shares the expensive agentgateway container across all It() blocks.
//
// Focused run:
//
//	cd tests/e2e/extproc && go test -list "OPA.*Agentgateway" ./...
package extproc_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

const (
	opaAgentgwJWTIssuer   = "https://opa-agentgateway.e2e.test"
	opaAgentgwJWTAudience = "opa-agentgateway-mcp"

	opaAgentgwExchangedToken  = "opa-agentgw-exchanged-token"
	opaAgentgwMockAccessToken = "opa-agentgw-mock-access-token"

	opaAgentgwGrantedPermissionSetID = "11111111-1111-1111-1111-111111111111"
	opaAgentgwRequiredServiceID      = "22222222-2222-2222-2222-222222222222"
)

// opaAgentgwLogger writes structured test output to GinkgoWriter.
var opaAgentgwLogger = bootstrap.NewTestLogger()

// opaAgentgwAuthHeaderKey is used for storing Authorization header in MCP context.
type opaAgentgwContextKey string

const opaAgentgwAuthKey opaAgentgwContextKey = "authorization"

type opaAgentgwToolObservation struct {
	calls      int
	authDigest string
}

type opaAgentgwToolObserver struct {
	mu    sync.Mutex
	calls map[string]opaAgentgwToolObservation
}

func (o *opaAgentgwToolObserver) record(name, authHeader string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	observation := o.calls[name]
	observation.calls++
	observation.authDigest = opaAgentgwAuthorizationDigest(authHeader)
	o.calls[name] = observation
}

func (o *opaAgentgwToolObserver) snapshot(name string) opaAgentgwToolObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls[name]
}

func (o *opaAgentgwToolObserver) reset() {
	o.mu.Lock()
	clear(o.calls)
	o.mu.Unlock()
}

// OPA Authorization via Agentgateway describes the end-to-end flow through a real
// agentgateway Docker container with OPA authorization enabled in ExtProc.
// The container setup is shared across scenarios (Ordered + BeforeAll).
var _ = Describe("OPA Authorization via Agentgateway", Ordered, func() {
	var (
		ctx                      context.Context
		cancel                   context.CancelFunc
		jwtFixture               *fixtures.RS256JWTFixture
		mcpJWT                   string
		reAuthJWT                string
		permissionedJWT          string
		permissionUnavailableJWT string
		headersOnlyJWT           string
		mockOAuth2Srv            *httptest.Server
		mockTokenExchangeSvr     *opaAgentgwMockTokenExchangeSrv
		extprocGRPC              *grpc.Server
		extprocAddr              string // direct gRPC address of the ExtProc server
		exchanger                *extprocserver.TokenExchanger
		authorizer               authorization.Authorizer
		mcpClient                *client.Client
		mcpHTTPServer            *http.Server
		agentgatewayPort         string // host port for the agentgateway container

		toolCalls *opaAgentgwToolObserver
	)

	BeforeAll(func() {
		_, dockerErr := testcontainers.ProviderDocker.GetProvider()
		if dockerErr != nil {
			Skip("Skipping OPA agentgateway integration tests: Docker not available")
		}

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
			if authorizer != nil {
				authorizer.Stop(context.Background())
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
		jwtFixture, err = fixtures.NewRS256JWTFixture(opaAgentgwJWTIssuer, opaAgentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create OPA Agentgateway JWT fixture")

		expiresAt := time.Now().Add(10 * time.Minute)
		mcpJWT, err = jwtFixture.MintToken("opa-agentgateway-mcp-subject", expiresAt)
		Expect(err).NotTo(HaveOccurred(), "failed to mint OPA MCP JWT")
		reAuthJWT, err = jwtFixture.MintToken("opa-agentgateway-reauth-subject", expiresAt)
		Expect(err).NotTo(HaveOccurred(), "failed to mint OPA re-auth JWT")
		permissionedJWT, err = jwtFixture.MintToken("opa-agentgateway-permissioned-subject", expiresAt)
		Expect(err).NotTo(HaveOccurred(), "failed to mint OPA permissioned JWT")
		permissionUnavailableJWT, err = jwtFixture.MintToken("opa-agentgateway-permission-unavailable-subject", expiresAt)
		Expect(err).NotTo(HaveOccurred(), "failed to mint OPA unavailable-permission JWT")
		headersOnlyJWT, err = jwtFixture.MintToken("opa-agentgateway-headers-only-subject", expiresAt)
		Expect(err).NotTo(HaveOccurred(), "failed to mint OPA headers-only JWT")

		// --- 1. Start mock identity broker ---
		mockOAuth2Srv = opaAgentgwNewMockOAuth2Srv()
		mockTokenExchangeSvr = opaAgentgwNewMockTokenExchangeSrv()

		// --- 2. Start mock MCP server that captures Authorization digests ---
		toolCalls = &opaAgentgwToolObserver{calls: make(map[string]opaAgentgwToolObservation)}
		var mcpListener net.Listener
		mcpListener, mcpHTTPServer = opaAgentgwStartMCPServer(toolCalls)
		mcpPort := mcpListener.Addr().(*net.TCPAddr).Port
		opaAgentgwLogger.Info("Mock MCP server listening", "port", mcpPort)

		// --- 3. Start ExtProc with OPA enabled (allow_readonly policy) ---
		var extprocListener net.Listener
		extprocListener, extprocGRPC, exchanger, authorizer = opaAgentgwStartExtProc(
			mockOAuth2Srv.URL, mockTokenExchangeSvr.URL,
		)
		extprocPort := extprocListener.Addr().(*net.TCPAddr).Port
		extprocAddr = fmt.Sprintf("127.0.0.1:%d", extprocPort)
		opaAgentgwLogger.Info("ExtProc gRPC server with OPA listening", "port", extprocPort)

		// --- 4. Start agentgateway Docker container ---
		setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer setupCancel()
		agentgatewayPort = opaAgentgwStartContainer(setupCtx, extprocPort, mcpPort, jwtFixture)
		opaAgentgwLogger.Info("agentgateway accessible on host port", "port", agentgatewayPort)

		// --- 5. Create MCP client connected through agentgateway ---
		agentgatewayURL := fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		initCtx, initCancel := context.WithTimeout(ctx, time.Minute)
		defer initCancel()
		mcpClient = opaAgentgwConnectMCPClient(initCtx, agentgatewayURL, mcpJWT)
	})

	BeforeEach(func() {
		toolCalls.reset()
		mockTokenExchangeSvr.reset()
	})

	// US1 Scenario 1 from specs/020-extproc-opa-authorization/spec.md
	// Given OPA is enabled with allow_readonly policy and list_repositories is a read-only tool,
	// When an MCP tools/call request for list_repositories arrives with a Bearer token,
	// Then the request proceeds to token exchange and the MCP server receives the exchanged token.
	It("should allow read-only tool call and exchange token", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// US1 Scenario 1 from specs/020-extproc-opa-authorization/spec.md

		mockTokenExchangeSvr.withExchangedToken(opaAgentgwExchangedToken)

		result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "list_repositories",
			},
		})
		Expect(err).NotTo(HaveOccurred(), "list_repositories tool call should succeed (allowed by policy)")
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse(), "tool call should not return an MCP error")

		// Verify the tool handler received the exchanged credential without retaining a raw token.
		observed := toolCalls.snapshot("list_repositories")
		Expect(observed.calls).To(BeNumerically(">=", 1), "allowed tool must reach the MCP handler")
		Expect(observed.authDigest).To(Equal(opaAgentgwAuthorizationDigest("Bearer "+opaAgentgwExchangedToken)),
			"MCP tool handler should receive the exchanged token after OPA allow decision")
	})

	// US1 Scenario 2 from specs/020-extproc-opa-authorization/spec.md
	// Given OPA is enabled with allow_readonly policy and delete_repository is destructive,
	// When an MCP tools/call request for delete_repository arrives,
	// Then ExtProc returns 403 with denial reasons.
	It("should deny destructive tool call with 403 and reasons", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// US1 Scenario 2 from specs/020-extproc-opa-authorization/spec.md

		resp := opaAgentgwPostMCP(ctx, agentgatewayPort, mcpJWT,
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"delete_repository","arguments":{"repo":"acme/app"}}}`)
		defer resp.Body.Close() //nolint:errcheck
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden), "destructive tool must be denied by OPA")

		Expect(toolCalls.snapshot("delete_repository").calls).To(BeZero(),
			"denied tool must not reach the MCP handler")

		directClient, conn := helpers.ConnectToExtProc(extprocAddr)
		defer conn.Close() //nolint:errcheck

		headersReq := helpers.NewRequestHeaders().
			WithPath("/mcp").
			WithHeader(":method", "POST").
			WithHeader(":authority", "mcp-server:9003").
			WithHeader(":scheme", "http").
			WithTokenExchangeMetadata(mcpJWT, fixtures.ValidResourceURI).
			WithAgentgatewayProtocol("mcp").
			BuildWithMetadata()

		_, bodyResp := helpers.SendHeadersAndBody(ctx, directClient, headersReq,
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"delete_repository","arguments":{"repo":"acme/app"}}}`))

		Expect(bodyResp).NotTo(BeNil())
		Expect(bodyResp).To(helpers.HaveImmediateResponseWithStatus(403),
			"destructive tool calls must be denied with 403 in the body phase")
		Expect(bodyResp).To(helpers.HaveImmediateResponseWithBody("access_denied"),
			"deny response should contain the access_denied error code")
		Expect(bodyResp).To(helpers.HaveImmediateResponseWithBody("tool is destructive"),
			"deny response should include the human-readable reason from the policy")
	})

	// US1 Scenario 3 from specs/020-extproc-opa-authorization/spec.md
	// Given OPA is enabled and the policy does not match any rule for the incoming tool,
	// When the request is evaluated,
	// Then the default deny decision applies and the request is rejected with 403.
	It("should deny unmatched tool call by default", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// US1 Scenario 3 from specs/020-extproc-opa-authorization/spec.md

		// "unknown_tool" is not in allow or deny sets — default deny applies
		resp := opaAgentgwPostMCP(ctx, agentgatewayPort, mcpJWT,
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unknown_tool_xyz","arguments":{}}}`)
		defer resp.Body.Close() //nolint:errcheck
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden), "unmatched tool must be denied by default")
		Expect(toolCalls.snapshot("unknown_tool_xyz").calls).To(BeZero(),
			"default-denied tool must not reach the MCP handler")
	})

	// US1 Scenario 4 from specs/020-extproc-opa-authorization/spec.md
	// Given OPA is enabled with a Rego policy,
	// When a non-tools/call MCP method (initialize) arrives,
	// Then the full parsed MCP message is available to the policy.
	// The allow_readonly policy explicitly allows non-tools/call methods.
	It("should evaluate non-tool-call MCP methods against policy", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// US1 Scenario 4 from specs/020-extproc-opa-authorization/spec.md
		// The MCP client already sent an initialize during BeforeAll connection setup.
		// This test verifies that the policy evaluated initialize (mcp_method) and allowed it
		// by making a fresh connection through agentgateway (which triggers a new initialize).

		Expect(agentgatewayPort).NotTo(BeEmpty(),
			"agentgateway port must be available from BeforeAll setup")

		agentgatewayURL := fmt.Sprintf("http://localhost:%s", agentgatewayPort)

		// Attempt a fresh initialize through agentgateway
		newClient, err := client.NewStreamableHttpClient(
			agentgatewayURL+"/mcp",
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + mcpJWT,
			}),
		)
		Expect(err).NotTo(HaveOccurred())

		initErr := newClient.Start(ctx)
		Expect(initErr).NotTo(HaveOccurred(),
			"MCP client Start (initialize) should succeed — allow_readonly allows mcp_method")
		newClient.Close() //nolint:errcheck
	})

	// FR-024 from specs/020-extproc-opa-authorization/spec.md
	// Given the token exchange response carries an authoritative granted_permission_sets snapshot for the required service,
	// When a permission-gated MCP tool call arrives,
	// Then the policy allows the request and the MCP server receives the exchanged token.
	It("should allow permission-gated tool calls when granted permission sets authorize the service", NodeTimeout(time.Minute), func(ctx SpecContext) {
		mockTokenExchangeSvr.withExchangedToken(opaAgentgwExchangedToken)
		mockTokenExchangeSvr.withGrantedPermissionSets(map[string][]string{
			opaAgentgwGrantedPermissionSetID: {opaAgentgwRequiredServiceID},
		})

		agentgatewayURL := fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		newClient, err := client.NewStreamableHttpClient(
			agentgatewayURL+"/mcp",
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + permissionedJWT,
			}),
		)
		Expect(err).NotTo(HaveOccurred())
		defer newClient.Close() //nolint:errcheck

		Expect(newClient.Start(ctx)).To(Succeed(),
			"initialize transport should succeed so the permission-scoped token exchange snapshot is cached")

		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "opa-permission-test-client", Version: "1.0.0"}
		_, err = newClient.Initialize(ctx, initReq)
		Expect(err).NotTo(HaveOccurred(),
			"initialize should succeed so the permission-scoped token exchange snapshot is cached")

		result, err := newClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "permissioned_read",
			},
		})
		Expect(err).NotTo(HaveOccurred(),
			"permissioned_read should be allowed only when granted_permission_sets_available is true and the required service is present")
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse())

		observed := toolCalls.snapshot("permissioned_read")
		Expect(observed.calls).To(BeNumerically(">=", 1), "allowed permissioned tool must reach the MCP handler")
		Expect(observed.authDigest).To(Equal(opaAgentgwAuthorizationDigest("Bearer "+opaAgentgwExchangedToken)),
			"permission-gated tool should receive the exchanged token after OPA allow")
		Expect(mockTokenExchangeSvr.callCount()).To(BeNumerically(">=", 1),
			"the broker must be reached so an authoritative granted_permission_sets snapshot enters the token-bound cache")
	})

	// FR-024 fail-closed path: when the broker omits granted_permission_sets,
	// permission-dependent policies must see unavailable context and deny.
	It("should deny permission-gated tool calls when permission-set context is unavailable", NodeTimeout(time.Minute), func(ctx SpecContext) {
		mockTokenExchangeSvr.withExchangedToken(opaAgentgwExchangedToken)
		mockTokenExchangeSvr.withGrantedPermissionSets(nil)

		agentgatewayURL := fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		newClient, err := client.NewStreamableHttpClient(
			agentgatewayURL+"/mcp",
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + permissionUnavailableJWT,
			}),
		)
		Expect(err).NotTo(HaveOccurred())
		defer newClient.Close() //nolint:errcheck

		Expect(newClient.Start(ctx)).To(Succeed(),
			"initialize transport should still succeed because allow_readonly explicitly allows mcp_headers_only")

		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "opa-permission-unavailable-client", Version: "1.0.0"}
		_, err = newClient.Initialize(ctx, initReq)
		Expect(err).NotTo(HaveOccurred(),
			"initialize should succeed so the cached token snapshot carries unavailable permission context into the tool call")

		resp := opaAgentgwPostMCP(ctx, agentgatewayPort, permissionUnavailableJWT,
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"permissioned_read","arguments":{}}}`)
		defer resp.Body.Close() //nolint:errcheck
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden), "missing permission context must deny the tool")

		Expect(toolCalls.snapshot("permissioned_read").calls).To(BeZero(),
			"permission-set-denied tool must not reach the MCP handler")
		Expect(mockTokenExchangeSvr.callCount()).To(BeNumerically(">=", 1),
			"the broker must be reached so unavailable permission-set context is cached with the exchanged token")

		directClient, conn := helpers.ConnectToExtProc(extprocAddr)
		defer conn.Close() //nolint:errcheck

		headersReq := helpers.NewRequestHeaders().
			WithPath("/mcp").
			WithHeader(":method", "POST").
			WithHeader(":authority", "mcp-server:9003").
			WithHeader(":scheme", "http").
			WithTokenExchangeMetadata(permissionUnavailableJWT, fixtures.ValidResourceURI).
			WithAgentgatewayProtocol("mcp").
			BuildWithMetadata()

		_, bodyResp := helpers.SendHeadersAndBody(ctx, directClient, headersReq,
			[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"permissioned_read","arguments":{}}}`))

		Expect(bodyResp).NotTo(BeNil())
		Expect(bodyResp).To(helpers.HaveImmediateResponseWithStatus(403),
			"permission-set-unavailable tool calls must be denied with 403 in the body phase")
		Expect(bodyResp).To(helpers.HaveImmediateResponseWithBody("access_denied"),
			"deny response should contain the access_denied error code")
		Expect(bodyResp).To(helpers.HaveImmediateResponseWithBody("permission set context unavailable"),
			"deny response should explain that permission-set context was unavailable")
	})

	// When OPA is enabled, token exchange runs eagerly in the headers phase before the body
	// is read. If the broker returns error_uri (re-auth required), the URLElicitationRequiredError
	// is returned as an ImmediateResponse from the headers phase. Because the body has not been
	// read at that point, the JSON-RPC request id is unknown and the error carries id=null.
	It("should return URLElicitationRequiredError from headers phase with null id when exchange requires re-auth", NodeTimeout(time.Minute), func(ctx SpecContext) {
		const opaAgentgwReAuthURL = "https://broker.example.com/api/third-party/svc-opa/oauth2/authorize"
		mockTokenExchangeSvr.withReAuthErrorURI(opaAgentgwReAuthURL)
		defer mockTokenExchangeSvr.reset()

		// Send a tools/call for list_repositories (OPA allows) via raw HTTP so we can
		// inspect the JSON-RPC response body directly (mcp-go may reject custom error codes).
		// Use a distinct minted JWT so the exchanger cache (populated during BeforeAll
		// Initialize()) does not return a cached successful exchange for this scenario.
		const requestID = 99
		reqBody := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"list_repositories","arguments":{}}}`,
			requestID,
		)
		resp := opaAgentgwPostMCP(ctx, agentgatewayPort, reAuthJWT, reqBody)
		defer resp.Body.Close() //nolint:errcheck

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"URLElicitationRequiredError must be HTTP 200 (JSON-RPC error over HTTP)")

		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			ID      any    `json:"id"`
			Error   struct {
				Code int `json:"code"`
				Data struct {
					Elicitations []struct {
						URL string `json:"url"`
					} `json:"elicitations"`
				} `json:"data"`
			} `json:"error"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&envelope)).To(Succeed())

		Expect(envelope.JSONRPC).To(Equal("2.0"))
		Expect(envelope.Error.Code).To(Equal(mcp.URL_ELICITATION_REQUIRED),
			"must use JSON-RPC error code -32042")
		// id is null because exchange fails in the headers phase before the body is read.
		Expect(envelope.ID).To(BeNil(),
			"headers-phase re-auth: JSON-RPC id is unknown (body not yet read)")
		Expect(envelope.Error.Data.Elicitations).To(HaveLen(1))
		Expect(envelope.Error.Data.Elicitations[0].URL).To(Equal(opaAgentgwReAuthURL),
			"elicitation URL must match the error_uri returned by the broker")
		Expect(mockTokenExchangeSvr.callCount()).To(BeNumerically(">=", 1),
			"broker endpoint must be reached — a cache hit would silently bypass the re-auth path")
	})

	// mcp_headers_only scenario from specs/020-extproc-opa-authorization/spec.md (FR-018)
	// When agentgateway sends a header-only GET /mcp (end_of_stream=true, no body),
	// ExtProc evaluates type=mcp_headers_only through OPA. The allow_readonly policy
	// allows mcp_headers_only GET requests, so token exchange proceeds and the
	// exchanged token is in the headers-phase response.
	//
	// NOTE: Tested via direct gRPC rather than through agentgateway because agentgateway
	// manages MCP sessions internally and returns 422 for GET /mcp without an established
	// session (agentgateway does route the request to ExtProc correctly, but the final
	// HTTP response to a session-less GET is 422). Direct gRPC verifies the ExtProc
	// mcp_headers_only path (OPA evaluation + token exchange + auth mutation) exactly.
	It("should allow mcp_headers_only GET /mcp and forward exchanged token to MCP server", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// mcp_headers_only scenario from specs/020-extproc-opa-authorization/spec.md (FR-018)
		//
		// Uses a distinct minted JWT so the exchange endpoint is actually called and no
		// cached token produces a false pass.
		mockTokenExchangeSvr.withExchangedToken(opaAgentgwExchangedToken)
		callsBefore := mockTokenExchangeSvr.callCount()

		// Connect directly to the ExtProc gRPC server (bypasses agentgateway session management).
		directClient, conn := helpers.ConnectToExtProc(extprocAddr)
		defer conn.Close() //nolint:errcheck

		// Build a headers-only GET request with agentgateway protocol metadata.
		// EndOfStream=true signals to ExtProc that no body phase follows, triggering the
		// mcp_headers_only evaluation path (BuildOPAInputHeadersOnly → type="mcp_headers_only").
		headersReq := helpers.NewRequestHeaders().
			WithPath("/mcp").
			WithHeader(":method", "GET").
			WithHeader(":authority", "mcp-server:9003").
			WithHeader(":scheme", "http").
			WithTokenExchangeMetadata(headersOnlyJWT, fixtures.ValidResourceURI).
			WithEndOfStream(true).
			WithAgentgatewayProtocol("mcp").
			BuildWithMetadata()

		resp := helpers.SendRequestHeaders(ctx, directClient, headersReq)

		Expect(helpers.ExtractMutatedAuthorizationHeader(resp)).To(ContainSubstring(opaAgentgwExchangedToken),
			"mcp_headers_only GET allowed by allow_readonly policy: auth header must contain exchanged token")

		Expect(mockTokenExchangeSvr.callCount()).To(BeNumerically(">", callsBefore),
			"exchange endpoint must be called — cache must not produce a false-positive pass")
	})

	// Edge case from specs/020-extproc-opa-authorization/spec.md
	// When a request has no protocol metadata AND the body is not recognizable as MCP
	// JSON-RPC 2.0 (type="unknown"), OPA denies with the default deny rule.
	//
	// NOTE: This scenario is tested directly via gRPC (not through agentgateway MCP client)
	// because we need to send a raw ExtProc request without agentgateway's metadata injection.
	It("should reject with 403 when protocol metadata missing and body is non-MCP", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// Edge case from specs/020-extproc-opa-authorization/spec.md

		// Connect directly to the ExtProc gRPC server (bypass agentgateway).
		directClient, conn := helpers.ConnectToExtProc(extprocAddr)
		defer conn.Close() //nolint:errcheck

		// Send a request with token-exchange metadata but a non-JSON-RPC body so that
		// auto-detection returns type="unknown" and the allow_readonly policy denies it.
		headersReq := helpers.NewRequestHeaders().
			WithPath("/mcp").
			WithTokenExchangeMetadata(mcpJWT, fixtures.ValidResourceURI).
			WithHeader(":authority", "mcp-server:9003").
			WithHeader(":scheme", "http").
			BuildWithMetadata()

		nonMCPBody := []byte(`plain text — not a JSON-RPC message`)

		// No protocol metadata → OPA rejects in headers phase with 403.
		headersResp, _ := helpers.SendHeadersAndBody(ctx, directClient, headersReq, nonMCPBody)
		Expect(headersResp).NotTo(BeNil(), "expected a response from ExtProc")

		status := helpers.ExtractImmediateResponseStatus(headersResp)
		Expect(status).To(Equal(uint32(403)),
			"non-MCP body with no protocol metadata must be denied with 403")

		body := helpers.ExtractImmediateResponseBody(headersResp)
		Expect(body).To(ContainSubstring("access_denied"),
			"deny response must contain access_denied error")
	})
})

// --- Helpers ---

// --- Mock OAuth2 Server ---

func opaAgentgwNewMockOAuth2Srv() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, //nolint:errcheck
			`{"access_token":%q,"id_token":"mock-id-token","token_type":"Bearer","expires_in":3600}`,
			opaAgentgwMockAccessToken,
		)
	})
	return httptest.NewServer(mux)
}

// --- Mock Token Exchange Server ---

type opaAgentgwMockTokenExchangeSrv struct {
	*httptest.Server
	mu                    sync.Mutex
	calls                 int
	exchangedToken        string
	grantedPermissionSets map[string][]string
	reAuthErrorURI        string // when non-empty, returns 401 + error_uri instead of a token
}

func opaAgentgwNewMockTokenExchangeSrv() *opaAgentgwMockTokenExchangeSrv {
	m := &opaAgentgwMockTokenExchangeSrv{
		exchangedToken: opaAgentgwExchangedToken,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.calls++
		token := m.exchangedToken
		grantedPermissionSets := m.grantedPermissionSets
		errURI := m.reAuthErrorURI
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if errURI != "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, //nolint:errcheck
				`{"error":"invalid_grant","error_description":"Session expired, please re-authenticate","error_uri":%q}`,
				errURI,
			)
			return
		}

		response := map[string]any{
			"access_token":      token,
			"issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
			"token_type":        "Bearer",
			"expires_in":        3600,
		}
		if grantedPermissionSets != nil {
			response["granted_permission_sets"] = grantedPermissionSets
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	m.Server = httptest.NewServer(mux)
	return m
}

func (m *opaAgentgwMockTokenExchangeSrv) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *opaAgentgwMockTokenExchangeSrv) withExchangedToken(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exchangedToken = token
}

func (m *opaAgentgwMockTokenExchangeSrv) withGrantedPermissionSets(gps map[string][]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grantedPermissionSets = gps
}

func (m *opaAgentgwMockTokenExchangeSrv) withReAuthErrorURI(uri string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reAuthErrorURI = uri
}

func (m *opaAgentgwMockTokenExchangeSrv) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = 0
	m.exchangedToken = opaAgentgwExchangedToken
	m.grantedPermissionSets = nil
	m.reAuthErrorURI = ""
}

// --- Mock MCP Server ---

func opaAgentgwAuthorizationDigest(header string) string {
	digest := sha256.Sum256([]byte(header))
	return fmt.Sprintf("%x", digest)
}

func opaAgentgwStartMCPServer(toolCalls *opaAgentgwToolObserver) (net.Listener, *http.Server) {
	mcpSvr := mcpserver.NewMCPServer(
		"opa-e2e-mock-mcp-server", "1.0.0",
		mcpserver.WithToolCapabilities(false),
	)

	// Register "list_repositories" (read-only, allowed by allow_readonly policy)
	listTool := mcp.NewTool("list_repositories",
		mcp.WithDescription("Lists repositories (read-only)."),
	)
	mcpSvr.AddTool(listTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		authHeader, _ := ctx.Value(opaAgentgwAuthKey).(string)
		toolCalls.record("list_repositories", authHeader)
		return mcp.NewToolResultText("repositories: [acme/app]"), nil
	})

	// Register "delete_repository" (destructive, denied by allow_readonly policy)
	deleteTool := mcp.NewTool("delete_repository",
		mcp.WithDescription("Deletes a repository (destructive)."),
		mcp.WithString("repo", mcp.Required()),
	)
	mcpSvr.AddTool(deleteTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		authHeader, _ := ctx.Value(opaAgentgwAuthKey).(string)
		toolCalls.record("delete_repository", authHeader)
		return mcp.NewToolResultText("deleted"), nil
	})

	// Register "permissioned_read" (allowed only when permission-set context is available
	// and contains the required service)
	permissionedTool := mcp.NewTool("permissioned_read",
		mcp.WithDescription("Reads permission-gated data when the exchanged token grants the required service."),
	)
	mcpSvr.AddTool(permissionedTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		authHeader, _ := ctx.Value(opaAgentgwAuthKey).(string)
		toolCalls.record("permissioned_read", authHeader)
		return mcp.NewToolResultText("permissioned result"), nil
	})

	// Register "unknown_tool_xyz" (not in any rule, triggers default deny)
	unknownTool := mcp.NewTool("unknown_tool_xyz",
		mcp.WithDescription("An unknown tool not covered by any policy rule."),
	)
	mcpSvr.AddTool(unknownTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		authHeader, _ := ctx.Value(opaAgentgwAuthKey).(string)
		toolCalls.record("unknown_tool_xyz", authHeader)
		return mcp.NewToolResultText("unknown result"), nil
	})

	httpSrv := mcpserver.NewStreamableHTTPServer(mcpSvr,
		mcpserver.WithEndpointPath("/mcp"),
		// Agentgateway reaches this host fixture through a loopback proxy but preserves
		// host.testcontainers.internal. This fixture is not browser-facing.
		mcpserver.WithDisableLocalhostProtection(true),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(ctx, opaAgentgwAuthKey, r.Header.Get("Authorization"))
		}),
	)

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create OPA MCP server listener")

	httpServer := &http.Server{Handler: httpSrv}
	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			opaAgentgwLogger.Error("OPA MCP server error", "err", serveErr)
		}
	}()

	return listener, httpServer
}

// --- ExtProc gRPC Server with OPA ---

func opaAgentgwStartExtProc(
	oauth2URL, tokenExchangeURL string,
) (net.Listener, *grpc.Server, *extprocserver.TokenExchanger, authorization.Authorizer) {
	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{
			Bind:                 "0.0.0.0",
			Port:                 0,
			MaxConcurrentStreams: 100,
		},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             tokenExchangeURL + "/oauth2/token",
			Issuer:                    oauth2URL,
			ClientID:                  "opa-e2e-extproc-client",
			ClientSecret:              "opa-e2e-client-secret",
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
		Authorization: extprocconfig.AuthorizationConfig{
			Enabled: true,
			Policy: extprocconfig.PolicyConfig{
				Path:     policyPath("allow_readonly.rego"),
				Package:  "aib.extproc.authz",
				Decision: "result",
			},
			DefaultDecision:   "deny",
			EvaluationTimeout: 500 * time.Millisecond,
			MaxBodySize:       1024 * 1024,
		},
	}

	slogLogger := bootstrap.NewTestLogger()
	exchanger, err := extprocserver.NewTokenExchanger(cfg, slogLogger)
	Expect(err).NotTo(HaveOccurred(), "failed to create token exchanger for OPA test")

	authorizer, err := bootstrap.NewOPAAuthorizer(cfg, slogLogger)
	Expect(err).NotTo(HaveOccurred(), "failed to create OPA authorizer for test")

	svc := extprocserver.NewServerWithAuthorizer(cfg, exchanger, authorizer, slogLogger)

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create ExtProc listener")

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	go func() {
		if serveErr := grpcSrv.Serve(listener); serveErr != nil {
			opaAgentgwLogger.Error("ExtProc OPA gRPC server error", "err", serveErr)
		}
	}()

	return listener, grpcSrv, exchanger, authorizer
}

// --- agentgateway Docker Container ---

func opaAgentgwStartContainer(ctx context.Context, extprocPort, mcpPort int, jwtFixture *fixtures.RS256JWTFixture) string {
	resourceExpression := fmt.Sprintf("'%s'", fixtures.ValidResourceURI)
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
            agentgateway:
              protocol: "'mcp'"
              mcp_server: "'mcp-server-mock'"
      backends:
      - mcp:
          targets:
          - name: tools
            mcp:
              host: http://host.testcontainers.internal:%d/mcp
`, jwtFixture.Issuer(), jwtFixture.Audience(), extprocPort, resourceExpression, mcpPort)

	req := testcontainers.ContainerRequest{
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
				Reader:            strings.NewReader(jwtFixture.JWKSJSON()),
				ContainerFilePath: "/jwks.json",
				FileMode:          0644,
			},
		},
		WaitingFor: wait.ForListeningPort("4000/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to start agentgateway container for OPA tests")

	DeferCleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		Expect(container.Terminate(cleanupCtx)).To(Succeed(), "failed to terminate agentgateway OPA container")
	})

	mappedPort, err := container.MappedPort(ctx, "4000")
	Expect(err).NotTo(HaveOccurred(), "failed to get mapped port")

	return mappedPort.Port()
}

// --- MCP Client ---

func opaAgentgwConnectMCPClient(ctx context.Context, agentgatewayURL, token string) *client.Client {
	mcpClient, err := client.NewStreamableHttpClient(
		agentgatewayURL+"/mcp",
		transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to create OPA MCP client")

	err = mcpClient.Start(ctx)
	Expect(err).NotTo(HaveOccurred(), "failed to start OPA MCP client (check allow_readonly policy allows initialize)")

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "opa-e2e-test-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initReq)
	Expect(err).NotTo(HaveOccurred(), "MCP Initialize should succeed (allow_readonly allows mcp_method)")

	return mcpClient
}

func opaAgentgwPostMCP(ctx context.Context, port, token, body string) *http.Response {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("http://localhost:%s/mcp", port), strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

// opaAgentgwExtractText extracts text content from a CallToolResult.
func opaAgentgwExtractText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// Ensure opaAgentgwExtractText is used to avoid unused variable lint errors.
var _ = opaAgentgwExtractText
