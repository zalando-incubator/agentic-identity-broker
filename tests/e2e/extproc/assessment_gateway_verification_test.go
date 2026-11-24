// Package extproc_test contains design verification cases for
// specs/20261123-194542-decision-model-tool-approvals (research.md D-02 and the time budget).
//
// These cases are not acceptance scenarios. They prove gateway behavior that the design of the
// decision model assessment depends on. They use the real ExtProc server behind a real
// agentgateway container. A gRPC interceptor records each ExtProc message without changing it.
//
// Set AGENTGATEWAY_IMAGE to run the cases against a different agentgateway release.
//
// Focused run:
//
//	ginkgo -v --focus "Assessment gateway verification" ./tests/e2e/extproc/
package extproc_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

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
)

const (
	verifySingleTargetPort = "4000"
	verifyTwoTargetPort    = "4001"
	verifyPagedTargetPort  = "4002"

	verifySlowProbeTool = "verify.slow_probe"
	verifyPagedToolSize = 300
	verifyPageLimit     = 100
	verifyGzipMarker    = "verify-list-gzip-"
)

// --- ExtProc message recorder ---

type verifyExtProcMessage struct {
	kind        string
	headers     map[string]string
	body        []byte
	endOfStream bool
}

type verifyExtProcStream struct {
	mu       sync.Mutex
	messages []verifyExtProcMessage
}

func (s *verifyExtProcStream) add(m verifyExtProcMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
}

func (s *verifyExtProcStream) snapshot() []verifyExtProcMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]verifyExtProcMessage(nil), s.messages...)
}

func (s *verifyExtProcStream) requestBody() []byte {
	var body []byte
	for _, m := range s.snapshot() {
		if m.kind == "request_body" {
			body = append(body, m.body...)
		}
	}
	return body
}

func (s *verifyExtProcStream) responseBodyChunks() [][]byte {
	var chunks [][]byte
	for _, m := range s.snapshot() {
		if m.kind == "response_body" {
			chunks = append(chunks, m.body)
		}
	}
	return chunks
}

func (s *verifyExtProcStream) firstHeaders(kind string) map[string]string {
	for _, m := range s.snapshot() {
		if m.kind == kind {
			return m.headers
		}
	}
	return nil
}

func (s *verifyExtProcStream) kinds() []string {
	var kinds []string
	for _, m := range s.snapshot() {
		kinds = append(kinds, m.kind)
	}
	return kinds
}

// verifyExtProcRecorder records each ExtProc message per gRPC stream. It can delay the
// response to a request body that names the slow probe tool.
type verifyExtProcRecorder struct {
	mu        sync.Mutex
	streams   []*verifyExtProcStream
	slowDelay time.Duration
}

func (r *verifyExtProcRecorder) setSlowDelay(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slowDelay = d
}

func (r *verifyExtProcRecorder) delay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.slowDelay
}

// streamWithRequestBody returns the stream whose request body contains marker.
func (r *verifyExtProcRecorder) streamWithRequestBody(marker string) *verifyExtProcStream {
	r.mu.Lock()
	streams := append([]*verifyExtProcStream(nil), r.streams...)
	r.mu.Unlock()
	for _, s := range streams {
		if bytes.Contains(s.requestBody(), []byte(marker)) {
			return s
		}
	}
	return nil
}

func (r *verifyExtProcRecorder) interceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	stream := &verifyExtProcStream{}
	r.mu.Lock()
	r.streams = append(r.streams, stream)
	r.mu.Unlock()
	return handler(srv, &verifyRecordingServerStream{ServerStream: ss, stream: stream, recorder: r})
}

type verifyRecordingServerStream struct {
	grpc.ServerStream
	stream   *verifyExtProcStream
	recorder *verifyExtProcRecorder
	slowCall bool
}

func (s *verifyRecordingServerStream) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	req, ok := m.(*extprocv3.ProcessingRequest)
	if !ok {
		return nil
	}
	switch msg := req.Request.(type) {
	case *extprocv3.ProcessingRequest_RequestHeaders:
		s.stream.add(verifyExtProcMessage{kind: "request_headers", headers: verifyHeaderMap(msg.RequestHeaders), endOfStream: msg.RequestHeaders.EndOfStream})
	case *extprocv3.ProcessingRequest_RequestBody:
		body := append([]byte(nil), msg.RequestBody.Body...)
		if bytes.Contains(body, []byte(verifySlowProbeTool)) {
			s.slowCall = true
		}
		s.stream.add(verifyExtProcMessage{kind: "request_body", body: body, endOfStream: msg.RequestBody.EndOfStream})
	case *extprocv3.ProcessingRequest_ResponseHeaders:
		s.stream.add(verifyExtProcMessage{kind: "response_headers", headers: verifyHeaderMap(msg.ResponseHeaders), endOfStream: msg.ResponseHeaders.EndOfStream})
	case *extprocv3.ProcessingRequest_ResponseBody:
		s.stream.add(verifyExtProcMessage{kind: "response_body", body: append([]byte(nil), msg.ResponseBody.Body...), endOfStream: msg.ResponseBody.EndOfStream})
	case *extprocv3.ProcessingRequest_RequestTrailers:
		s.stream.add(verifyExtProcMessage{kind: "request_trailers"})
	case *extprocv3.ProcessingRequest_ResponseTrailers:
		s.stream.add(verifyExtProcMessage{kind: "response_trailers"})
	}
	return nil
}

func (s *verifyRecordingServerStream) SendMsg(m any) error {
	if resp, ok := m.(*extprocv3.ProcessingResponse); ok && s.slowCall {
		if _, isBody := resp.Response.(*extprocv3.ProcessingResponse_RequestBody); isBody {
			time.Sleep(s.recorder.delay())
		}
	}
	return s.ServerStream.SendMsg(m)
}

func verifyHeaderMap(headers *extprocv3.HttpHeaders) map[string]string {
	result := map[string]string{}
	if headers == nil || headers.Headers == nil {
		return result
	}
	for _, h := range headers.Headers.Headers {
		value := h.Value
		if len(h.RawValue) > 0 {
			value = string(h.RawValue)
		}
		result[strings.ToLower(h.Key)] = value
	}
	return result
}

// --- Upstream MCP recorder ---

type verifyUpstreamExchange struct {
	method          string
	requestHeaders  http.Header
	requestBody     []byte
	responseHeaders http.Header
	responseBody    []byte
}

type verifyUpstreamRecorder struct {
	mu        sync.Mutex
	exchanges []verifyUpstreamExchange
}

func (u *verifyUpstreamRecorder) withMethod(method string) []verifyUpstreamExchange {
	u.mu.Lock()
	defer u.mu.Unlock()
	var result []verifyUpstreamExchange
	for _, e := range u.exchanges {
		if e.method == method {
			result = append(result, e)
		}
	}
	return result
}

type verifyCapturingWriter struct {
	http.ResponseWriter
	body bytes.Buffer
}

func (w *verifyCapturingWriter) Write(p []byte) (int, error) {
	w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

func (w *verifyCapturingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (u *verifyUpstreamRecorder) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody []byte
		if r.Body != nil {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(requestBody))
		}
		var envelope struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(requestBody, &envelope)
		cw := &verifyCapturingWriter{ResponseWriter: w}
		if bytes.Contains(requestBody, []byte(verifyGzipMarker)) && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			// The gzip probe compresses the upstream response to show what the gateway gives ExtProc.
			plain := httptest.NewRecorder()
			next.ServeHTTP(plain, r)
			for k, v := range plain.Header() {
				w.Header()[k] = v
			}
			w.Header().Del("Content-Length")
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(plain.Code)
			zw := gzip.NewWriter(cw)
			_, _ = zw.Write(plain.Body.Bytes())
			_ = zw.Close()
		} else {
			next.ServeHTTP(cw, r)
		}
		u.mu.Lock()
		u.exchanges = append(u.exchanges, verifyUpstreamExchange{
			method:          envelope.Method,
			requestHeaders:  r.Header.Clone(),
			requestBody:     requestBody,
			responseHeaders: w.Header().Clone(),
			responseBody:    cw.body.Bytes(),
		})
		u.mu.Unlock()
	})
}

// --- MCP servers ---

func verifyShareFileTool() mcp.Tool {
	tool := mcp.NewToolWithRawSchema("drive.share_file",
		"Share a file with a user, group, or domain. The grantee gets access at once and receives a notification email.",
		json.RawMessage(`{"type":"object","properties":{"file_id":{"type":"string","description":"ID of the file to share."},"role":{"type":"string","enum":["reader","commenter","editor"],"description":"Access level to grant."},"grantee_domain":{"type":"string","description":"Email domain of the grantee."},"transfer_ownership":{"type":"boolean","description":"Make the grantee the owner. The previous owner cannot revert this."}},"required":["file_id","role"]}`),
	)
	tool.Annotations = mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(false),
		DestructiveHint: mcp.ToBoolPtr(false),
		IdempotentHint:  mcp.ToBoolPtr(true),
		OpenWorldHint:   mcp.ToBoolPtr(true),
	}
	return tool
}

func verifyStartMCPServer(build func(*mcpserver.MCPServer), options ...mcpserver.ServerOption) (int, *verifyUpstreamRecorder, *http.Server) {
	options = append([]mcpserver.ServerOption{mcpserver.WithToolCapabilities(false)}, options...)
	srv := mcpserver.NewMCPServer("verify-mcp-server", "1.0.0", options...)
	build(srv)
	recorder := &verifyUpstreamRecorder{}
	handler := mcpserver.NewStreamableHTTPServer(srv,
		mcpserver.WithEndpointPath("/mcp"),
		mcpserver.WithDisableLocalhostProtection(true),
	)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred())
	httpServer := &http.Server{Handler: recorder.wrap(handler)}
	go func() { _ = httpServer.Serve(listener) }()
	return listener.Addr().(*net.TCPAddr).Port, recorder, httpServer
}

func verifyTextResult(text string) mcpserver.ToolHandlerFunc {
	return func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(text), nil
	}
}

// --- ExtProc ---

func verifyStartExtProc(oauth2URL, tokenExchangeURL string, recorder *verifyExtProcRecorder) (int, *grpc.Server, *extprocserver.TokenExchanger, authorization.Authorizer) {
	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{Bind: "0.0.0.0", MaxConcurrentStreams: 100},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             tokenExchangeURL + "/oauth2/token",
			Issuer:                    oauth2URL,
			ClientID:                  "verify-extproc-client",
			ClientSecret:              "verify-client-secret",
			ClientCredentialsEndpoint: oauth2URL + "/oauth/token",
			ClientAssertionType:       "access_token",
			ExchangeTimeout:           10 * time.Second,
			TLS:                       extprocconfig.TLSConfig{AllowHTTP: true},
		},
		Cache: extprocconfig.CacheConfig{DefaultTTL: 5 * time.Minute, MaxTTL: time.Hour},
		Log:   extprocconfig.LogConfig{Level: "info", Format: "text"},
		Authorization: extprocconfig.AuthorizationConfig{
			Enabled: true,
			Policy: extprocconfig.PolicyConfig{
				Path:     policyPath("allow_all.rego"),
				Package:  "aib.extproc.authz",
				Decision: "result",
			},
			DefaultDecision:   "deny",
			EvaluationTimeout: 500 * time.Millisecond,
			MaxBodySize:       1024 * 1024,
		},
	}
	logger := bootstrap.NewTestLogger()
	exchanger, err := extprocserver.NewTokenExchanger(cfg, logger)
	Expect(err).NotTo(HaveOccurred())
	authorizer, err := bootstrap.NewOPAAuthorizer(cfg, logger)
	Expect(err).NotTo(HaveOccurred())
	svc := extprocserver.NewServerWithAuthorizer(cfg, exchanger, authorizer, logger)

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred())
	grpcSrv := grpc.NewServer(grpc.StreamInterceptor(recorder.interceptor))
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)
	go func() { _ = grpcSrv.Serve(listener) }()
	return listener.Addr().(*net.TCPAddr).Port, grpcSrv, exchanger, authorizer
}

// --- agentgateway ---

func verifyListenerYAML(port, serverName, targets string, extprocPort int, jwtFixture *fixtures.RS256JWTFixture) string {
	return fmt.Sprintf(`- port: %s
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
              resource_uri: "'%s'"
            agentgateway:
              protocol: "'mcp'"
              mcp_server: "'%s'"
      backends:
      - mcp:
          targets:
%s`, port, jwtFixture.Issuer(), jwtFixture.Audience(), extprocPort, fixtures.ValidResourceURI, serverName, targets)
}

func verifyTargetYAML(name string, port int) string {
	return fmt.Sprintf("          - name: %s\n            mcp:\n              host: http://host.testcontainers.internal:%d/mcp\n", name, port)
}

func verifyStartGateway(ctx context.Context, extprocPort int, mcpPorts []int, configYAML, jwks string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:           agentgatewayTestImage(),
		ExposedPorts:    []string{verifySingleTargetPort + "/tcp", verifyTwoTargetPort + "/tcp", verifyPagedTargetPort + "/tcp"},
		Cmd:             []string{"-f", "/config.yaml"},
		HostAccessPorts: append([]int{extprocPort}, mcpPorts...),
		Files: []testcontainers.ContainerFile{
			{Reader: strings.NewReader(configYAML), ContainerFilePath: "/config.yaml", FileMode: 0o644},
			{Reader: strings.NewReader(jwks), ContainerFilePath: "/jwks.json", FileMode: 0o644},
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(verifySingleTargetPort+"/tcp"),
			wait.ForListeningPort(verifyTwoTargetPort+"/tcp"),
			wait.ForListeningPort(verifyPagedTargetPort+"/tcp"),
		).WithDeadline(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	Expect(err).NotTo(HaveOccurred(), "failed to start agentgateway container")
	return container
}

// --- MCP client over raw HTTP, to keep the exact response bytes ---

type verifyHTTPResult struct {
	status   int
	headers  http.Header
	body     []byte
	duration time.Duration
}

type verifyMCPClient struct {
	baseURL         string
	token           string
	sessionID       string
	protocolVersion string
}

func (c *verifyMCPClient) post(ctx context.Context, body string, extraHeaders map[string]string) verifyHTTPResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/mcp", strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.protocolVersion != "" {
		req.Header.Set("Mcp-Protocol-Version", c.protocolVersion)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	// DisableCompression keeps the bytes on the wire when a case sets Accept-Encoding itself.
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}, Timeout: 60 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return verifyHTTPResult{status: resp.StatusCode, headers: resp.Header, body: data, duration: time.Since(start)}
}

// The MCP revision 2025-11-25 uses initialize and a session. The revision 2026-07-28 has no
// initialize and no session. Each request carries the protocol version in _meta and headers.
const (
	verifyLegacyProtocol = "2025-11-25"
	verifyModernProtocol = "2026-07-28"
)

func verifyConnect(ctx context.Context, port, token, protocol string) *verifyMCPClient {
	c := &verifyMCPClient{baseURL: "http://localhost:" + port, token: token}
	if protocol == verifyModernProtocol {
		c.protocolVersion = protocol
		return c
	}
	init := c.post(ctx, fmt.Sprintf(`{"jsonrpc":"2.0","id":"verify-init","method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"verify-client","version":"1.0.0"}}}`, protocol), nil)
	Expect(init.status).To(Equal(http.StatusOK), "initialize must succeed: %s", init.body)
	c.sessionID = init.headers.Get("Mcp-Session-Id")
	initResult := verifyJSONRPCResult(init.headers.Get("Content-Type"), init.body, "verify-init")
	Expect(initResult).NotTo(BeNil(), "initialize must return a result: %s", init.body)
	c.protocolVersion = fmt.Sprint(initResult["protocolVersion"])
	notified := c.post(ctx, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	Expect(notified.status).To(BeNumerically("<", 300), "initialized notification must succeed: %s", notified.body)
	return c
}

// call sends one JSON-RPC request. For the stateless revision, it adds _meta and Mcp-Method.
func (c *verifyMCPClient) call(ctx context.Context, id, method string, params map[string]any, extraHeaders map[string]string) verifyHTTPResult {
	if params == nil {
		params = map[string]any{}
	}
	headers := map[string]string{}
	if c.sessionID == "" && c.protocolVersion == verifyModernProtocol {
		params["_meta"] = map[string]any{
			"io.modelcontextprotocol/protocolVersion":    verifyModernProtocol,
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "verify-client", "version": "1.0.0"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}
		headers["Mcp-Method"] = method
		if name, ok := params["name"].(string); ok {
			headers["Mcp-Name"] = name
		}
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	Expect(err).NotTo(HaveOccurred())
	return c.post(ctx, string(body), headers)
}

// verifyJSONRPCResult returns the JSON-RPC result with the given id from a JSON or SSE body.
func verifyJSONRPCResult(contentType string, body []byte, id string) map[string]any {
	var payloads [][]byte
	if strings.HasPrefix(contentType, "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data:"); ok {
				payloads = append(payloads, []byte(strings.TrimSpace(data)))
			}
		}
	} else {
		payloads = append(payloads, body)
	}
	for _, p := range payloads {
		var msg map[string]any
		if json.Unmarshal(p, &msg) != nil {
			continue
		}
		if fmt.Sprint(msg["id"]) == id {
			result, _ := msg["result"].(map[string]any)
			return result
		}
	}
	return nil
}

func verifyToolNames(result map[string]any) []string {
	tools, _ := result["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if tool, ok := t.(map[string]any); ok {
			names = append(names, fmt.Sprint(tool["name"]))
		}
	}
	return names
}

func verifyChunkSizes(chunks [][]byte) []int {
	sizes := make([]int, len(chunks))
	for i, c := range chunks {
		sizes[i] = len(c)
	}
	return sizes
}

var _ = Describe("Assessment gateway verification", Ordered, ContinueOnFailure, Label("docker", "verification"), func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		recorder       *verifyExtProcRecorder
		token          string
		driveUpstream  *verifyUpstreamRecorder
		wikiUpstream   *verifyUpstreamRecorder
		pagedUpstream  *verifyUpstreamRecorder
		gatewayPorts   map[string]string
		mockOAuth2Srv  *httptest.Server
		mockExchange   *opaAgentgwMockTokenExchangeSrv
		image          string
		gatewayLogsFor func() string
	)

	BeforeAll(func() {
		if _, dockerErr := testcontainers.ProviderDocker.GetProvider(); dockerErr != nil {
			Skip("Skipping assessment gateway verification: Docker not available")
		}
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(func() { cancel() })
		image = agentgatewayTestImage()
		AddReportEntry("agentgateway image", image)

		jwtFixture, err := fixtures.NewRS256JWTFixture(opaAgentgwJWTIssuer, opaAgentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred())
		token, err = jwtFixture.MintToken("verify-subject", time.Now().Add(30*time.Minute))
		Expect(err).NotTo(HaveOccurred())

		mockOAuth2Srv = opaAgentgwNewMockOAuth2Srv()
		mockExchange = opaAgentgwNewMockTokenExchangeSrv()
		DeferCleanup(mockOAuth2Srv.Close)
		DeferCleanup(mockExchange.Close)

		drivePort, driveRec, driveSrv := verifyStartMCPServer(func(s *mcpserver.MCPServer) {
			s.AddTool(verifyShareFileTool(), verifyTextResult("shared"))
			s.AddTool(mcp.NewTool(verifySlowProbeTool, mcp.WithDescription("Probe for the ExtProc time budget.")), verifyTextResult("slow probe"))
		})
		wikiPort, wikiRec, wikiSrv := verifyStartMCPServer(func(s *mcpserver.MCPServer) {
			s.AddTool(mcp.NewTool("wiki.read_page", mcp.WithDescription("Read one wiki page."), mcp.WithString("page_id", mcp.Required(), mcp.Description("ID of the page."))), verifyTextResult("page"))
		})
		pagedPort, pagedRec, pagedSrv := verifyStartMCPServer(func(s *mcpserver.MCPServer) {
			filler := strings.Repeat("This sentence makes the tool description longer for the chunk size check. ", 6)
			for i := range verifyPagedToolSize {
				s.AddTool(mcp.NewTool(fmt.Sprintf("bulk.tool_%03d", i), mcp.WithDescription(filler), mcp.WithString("value", mcp.Description("A value."))), verifyTextResult("ok"))
			}
		}, mcpserver.WithPaginationLimit(verifyPageLimit))
		driveUpstream, wikiUpstream, pagedUpstream = driveRec, wikiRec, pagedRec
		DeferCleanup(func() { _ = driveSrv.Close(); _ = wikiSrv.Close(); _ = pagedSrv.Close() })

		recorder = &verifyExtProcRecorder{}
		extprocPort, grpcSrv, exchanger, authorizer := verifyStartExtProc(mockOAuth2Srv.URL, mockExchange.URL, recorder)
		DeferCleanup(func() {
			grpcSrv.Stop()
			exchanger.Shutdown()
			authorizer.Stop(context.Background())
		})

		configYAML := "binds:\n" +
			verifyListenerYAML(verifySingleTargetPort, "google-drive-mcp", verifyTargetYAML("drive", drivePort), extprocPort, jwtFixture) +
			verifyListenerYAML(verifyTwoTargetPort, "workspace-mcp", verifyTargetYAML("drive", drivePort)+verifyTargetYAML("wiki", wikiPort), extprocPort, jwtFixture) +
			verifyListenerYAML(verifyPagedTargetPort, "bulk-mcp", verifyTargetYAML("bulk", pagedPort), extprocPort, jwtFixture)

		setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer setupCancel()
		container := verifyStartGateway(setupCtx, extprocPort, []int{drivePort, wikiPort, pagedPort}, configYAML, jwtFixture.JWKSJSON())
		DeferCleanup(func() {
			cleanupCtx, c := context.WithTimeout(context.Background(), 30*time.Second)
			defer c()
			_ = container.Terminate(cleanupCtx)
		})
		gatewayLogsFor = func() string {
			logs, err := container.Logs(context.Background())
			if err != nil {
				return err.Error()
			}
			defer logs.Close() //nolint:errcheck
			data, _ := io.ReadAll(logs)
			return string(data)
		}
		gatewayPorts = map[string]string{}
		for _, p := range []string{verifySingleTargetPort, verifyTwoTargetPort, verifyPagedTargetPort} {
			mapped, err := container.MappedPort(setupCtx, p)
			Expect(err).NotTo(HaveOccurred())
			gatewayPorts[p] = mapped.Port()
		}
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			GinkgoWriter.Println(gatewayLogsFor())
		}
	})

	for _, protocol := range []string{verifyLegacyProtocol, verifyModernProtocol} {
		It("delivers the tools/list response body to ExtProc on the stream of the request, and the client gets the same bytes (MCP "+protocol+")", NodeTimeout(2*time.Minute), func(ctx SpecContext) {
			id := "verify-list-single-" + protocol
			client := verifyConnect(ctx, gatewayPorts[verifySingleTargetPort], token, protocol)
			listed := client.call(ctx, id, "tools/list", nil, nil)
			Expect(listed.status).To(Equal(http.StatusOK), "tools/list must succeed: %s", listed.body)

			stream := recorder.streamWithRequestBody(id)
			Expect(stream).NotTo(BeNil(), "ExtProc must see the tools/list request body")
			Eventually(func() []string { return stream.kinds() }).Should(ContainElement("response_body"),
				"the response phase of tools/list must arrive on the same ExtProc stream as the request phase")

			chunks := stream.responseBodyChunks()
			Expect(bytes.Join(chunks, nil)).To(Equal(listed.body), "the client must get the bytes that ExtProc echoed")

			responseHeaders := stream.firstHeaders("response_headers")
			contentType := responseHeaders["content-type"]
			AddReportEntry(protocol+" single target: message kinds", stream.kinds())
			AddReportEntry(protocol+" single target: response headers at ExtProc", responseHeaders)
			AddReportEntry(protocol+" single target: response chunk sizes", verifyChunkSizes(chunks))
			AddReportEntry(protocol+" single target: client body", string(listed.body))
			Expect(contentType).To(Or(HavePrefix("application/json"), HavePrefix("text/event-stream")))

			result := verifyJSONRPCResult(contentType, listed.body, id)
			Expect(result).NotTo(BeNil(), "the response must contain the JSON-RPC result")
			Expect(verifyToolNames(result)).To(ContainElement("drive.share_file"))
			expected, err := json.Marshal(verifyShareFileTool())
			Expect(err).NotTo(HaveOccurred())
			var expectedDefinition map[string]any
			Expect(json.Unmarshal(expected, &expectedDefinition)).To(Succeed())
			for _, t := range result["tools"].([]any) {
				if tool := t.(map[string]any); tool["name"] == "drive.share_file" {
					Expect(tool["description"]).To(Equal(expectedDefinition["description"]))
					Expect(tool["inputSchema"]).To(Equal(expectedDefinition["inputSchema"]))
					Expect(tool["annotations"]).To(Equal(expectedDefinition["annotations"]))
				}
			}

			upstream := driveUpstream.withMethod("tools/list")
			Expect(upstream).NotTo(BeEmpty())
			last := upstream[len(upstream)-1]
			AddReportEntry(protocol+" single target: upstream response content-type", last.responseHeaders.Get("Content-Type"))
			AddReportEntry(protocol+" single target: upstream response body", string(last.responseBody))
		})

		It("keeps the response bytes unchanged when the client accepts gzip (MCP "+protocol+")", NodeTimeout(2*time.Minute), func(ctx SpecContext) {
			id := verifyGzipMarker + protocol
			client := verifyConnect(ctx, gatewayPorts[verifySingleTargetPort], token, protocol)
			listed := client.call(ctx, id, "tools/list", nil, map[string]string{"Accept-Encoding": "gzip"})
			Expect(listed.status).To(Equal(http.StatusOK))

			stream := recorder.streamWithRequestBody(id)
			Expect(stream).NotTo(BeNil())
			Eventually(func() []string { return stream.kinds() }).Should(ContainElement("response_body"))
			Expect(bytes.Join(stream.responseBodyChunks(), nil)).To(Equal(listed.body))

			upstream := driveUpstream.withMethod("tools/list")
			AddReportEntry(protocol+" gzip: request accept-encoding at ExtProc", stream.firstHeaders("request_headers")["accept-encoding"])
			AddReportEntry(protocol+" gzip: response content-encoding at ExtProc", stream.firstHeaders("response_headers")["content-encoding"])
			AddReportEntry(protocol+" gzip: response content-encoding at client", listed.headers.Get("Content-Encoding"))
			last := upstream[len(upstream)-1]
			AddReportEntry(protocol+" gzip: upstream request accept-encoding", last.requestHeaders.Get("Accept-Encoding"))
			AddReportEntry(protocol+" gzip: upstream response content-encoding", last.responseHeaders.Get("Content-Encoding"))
			Expect(last.responseHeaders.Get("Content-Encoding")).To(Equal("gzip"), "the upstream must compress the probe response")
			result := verifyJSONRPCResult(stream.firstHeaders("response_headers")["content-type"], stream.responseBodyChunks()[0], id)
			Expect(result).NotTo(BeNil(), "ExtProc must get a readable listing when the upstream compresses its response")
		})

		It("delivers large listings in chunks and passes nextCursor pages through (MCP "+protocol+")", NodeTimeout(2*time.Minute), func(ctx SpecContext) {
			id := "verify-list-page-1-" + protocol
			client := verifyConnect(ctx, gatewayPorts[verifyPagedTargetPort], token, protocol)
			first := client.call(ctx, id, "tools/list", nil, nil)
			Expect(first.status).To(Equal(http.StatusOK))
			stream := recorder.streamWithRequestBody(id)
			Expect(stream).NotTo(BeNil())
			Eventually(func() []string { return stream.kinds() }).Should(ContainElement("response_body"))
			chunks := stream.responseBodyChunks()
			Expect(bytes.Join(chunks, nil)).To(Equal(first.body))

			contentType := stream.firstHeaders("response_headers")["content-type"]
			result := verifyJSONRPCResult(contentType, first.body, id)
			Expect(result).NotTo(BeNil())
			AddReportEntry(protocol+" paged: first page tool count", len(verifyToolNames(result)))
			AddReportEntry(protocol+" paged: first page nextCursor", fmt.Sprint(result["nextCursor"]))
			AddReportEntry(protocol+" paged: first page bytes", len(first.body))
			AddReportEntry(protocol+" paged: first page chunk sizes", verifyChunkSizes(chunks))
			pagedLists := pagedUpstream.withMethod("tools/list")
			AddReportEntry(protocol+" paged: upstream tools/list requests so far", len(pagedLists))
			AddReportEntry(protocol+" paged: upstream first page has nextCursor", bytes.Contains(pagedLists[len(pagedLists)-1].responseBody, []byte(`"nextCursor"`)))

			cursor, hasCursor := result["nextCursor"].(string)
			if hasCursor && cursor != "" {
				secondID := "verify-list-page-2-" + protocol
				second := client.call(ctx, secondID, "tools/list", map[string]any{"cursor": cursor}, nil)
				Expect(second.status).To(Equal(http.StatusOK))
				secondStream := recorder.streamWithRequestBody(secondID)
				Expect(secondStream).NotTo(BeNil())
				Eventually(func() []string { return secondStream.kinds() }).Should(ContainElement("response_body"))
				secondResult := verifyJSONRPCResult(secondStream.firstHeaders("response_headers")["content-type"], second.body, secondID)
				Expect(secondResult).NotTo(BeNil())
				AddReportEntry(protocol+" paged: second page tool count", len(verifyToolNames(secondResult)))
				AddReportEntry(protocol+" paged: second page names", verifyToolNames(secondResult))
			}
		})

		It("shows ExtProc the client-side tool names when one route has two MCP targets (MCP "+protocol+")", NodeTimeout(2*time.Minute), func(ctx SpecContext) {
			id := "verify-list-multi-" + protocol
			client := verifyConnect(ctx, gatewayPorts[verifyTwoTargetPort], token, protocol)
			listed := client.call(ctx, id, "tools/list", nil, nil)
			Expect(listed.status).To(Equal(http.StatusOK))
			stream := recorder.streamWithRequestBody(id)
			Expect(stream).NotTo(BeNil())
			Eventually(func() []string { return stream.kinds() }).Should(ContainElement("response_body"))
			Expect(bytes.Join(stream.responseBodyChunks(), nil)).To(Equal(listed.body))

			contentType := stream.firstHeaders("response_headers")["content-type"]
			names := verifyToolNames(verifyJSONRPCResult(contentType, listed.body, id))
			AddReportEntry(protocol+" two targets: tool names at ExtProc and client", names)
			AddReportEntry(protocol+" two targets: response content-type at ExtProc", contentType)
			AddReportEntry(protocol+" two targets: response chunk sizes", verifyChunkSizes(stream.responseBodyChunks()))
			AddReportEntry(protocol+" two targets: wiki upstream tools/list requests so far", len(wikiUpstream.withMethod("tools/list")))

			var shareName string
			for _, n := range names {
				if strings.HasSuffix(n, "drive.share_file") {
					shareName = n
				}
			}
			Expect(shareName).NotTo(BeEmpty())
			callID := "verify-call-multi-" + protocol
			called := client.call(ctx, callID, "tools/call", map[string]any{"name": shareName, "arguments": map[string]any{"file_id": "1Zx9QpL0aB3", "role": "reader", "grantee_domain": "example.com"}}, nil)
			Expect(called.status).To(Equal(http.StatusOK), "tools/call must succeed: %s", called.body)
			callStream := recorder.streamWithRequestBody(callID)
			Expect(callStream).NotTo(BeNil())
			Expect(string(callStream.requestBody())).To(ContainSubstring(shareName), "ExtProc must see the client-side tool name in tools/call")
			upstreamCalls := driveUpstream.withMethod("tools/call")
			Expect(upstreamCalls).NotTo(BeEmpty())
			AddReportEntry(protocol+" two targets: tools/call name at ExtProc", shareName)
			AddReportEntry(protocol+" two targets: tools/call body at upstream", string(upstreamCalls[len(upstreamCalls)-1].requestBody))
		})

		It("waits for a slow ExtProc answer in the request-body phase for the time budget of the assessment (MCP "+protocol+")", NodeTimeout(2*time.Minute), func(ctx SpecContext) {
			// The assessment budget is the 5 s gate timeout, the 500 ms model request, and two
			// 100 ms evaluations. The case uses 6 s to include all of them.
			recorder.setSlowDelay(6 * time.Second)
			DeferCleanup(func() { recorder.setSlowDelay(0) })
			client := verifyConnect(ctx, gatewayPorts[verifySingleTargetPort], token, protocol)
			called := client.call(ctx, "verify-slow-"+protocol, "tools/call", map[string]any{"name": verifySlowProbeTool, "arguments": map[string]any{}}, nil)
			AddReportEntry(protocol+" slow body phase: status", called.status)
			AddReportEntry(protocol+" slow body phase: duration", called.duration.String())
			AddReportEntry(protocol+" slow body phase: body", string(called.body))
			Expect(called.status).To(Equal(http.StatusOK))
			Expect(called.duration).To(BeNumerically(">=", 6*time.Second))
			Expect(string(called.body)).To(ContainSubstring("slow probe"))
		})
	}
})
