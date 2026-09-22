// Package helpers provides gRPC client helpers and request builders for ExtProc E2E tests.
// These utilities simplify construction of ProcessingRequest messages and assertion of
// ProcessingResponse messages.
package helpers

import (
	"context"
	"fmt"
	"io"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/gomega" //nolint:staticcheck
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// ProcessingRequestBuilder builds Envoy ProcessingRequest messages for testing.
// Implements the builder pattern for fluent test construction.
type ProcessingRequestBuilder struct {
	headers                  map[string]string
	protocol                 string  // agentgateway protocol metadata (e.g., "mcp", "a2a")
	mcpServer                *string // agentgateway mcp_server metadata; nil means the field is omitted entirely
	subjectToken             string
	resourceURI              string
	hasTokenExchangeMetadata bool
	endOfStream              bool // set EndOfStream=true on HttpHeaders (simulates header-only GET)
}

// NewRequestHeaders creates a builder initialized with common pseudo-headers for ExtProc testing.
// The method path, authority, and scheme are set to sensible defaults.
func NewRequestHeaders() *ProcessingRequestBuilder {
	return &ProcessingRequestBuilder{
		headers: map[string]string{
			":method":    "GET",
			":scheme":    "http",
			":authority": "mcp-server:9003",
		},
	}
}

// WithPath sets a raw :path pseudo-header that cannot supply token-exchange input.
func (b *ProcessingRequestBuilder) WithPath(path string) *ProcessingRequestBuilder {
	b.headers[":path"] = path
	return b
}

// WithBearerToken sets the Authorization header with the given Bearer token.
func (b *ProcessingRequestBuilder) WithBearerToken(token string) *ProcessingRequestBuilder {
	b.headers["authorization"] = fmt.Sprintf("Bearer %s", token)
	return b
}

// WithHeader sets an arbitrary header value.
func (b *ProcessingRequestBuilder) WithHeader(key, value string) *ProcessingRequestBuilder {
	b.headers[key] = value
	return b
}

// WithoutAuthorizationHeader removes the Authorization header (simulates requests without Bearer).
func (b *ProcessingRequestBuilder) WithoutAuthorizationHeader() *ProcessingRequestBuilder {
	delete(b.headers, "authorization")
	return b
}

// BuildRequestBody constructs a ProcessingRequest for the RequestBody phase
// containing the provided body bytes. Used in OPA authorization tests to send
// MCP request bodies through ExtProc after the RequestHeaders phase.
func BuildRequestBody(body []byte) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{
				Body:        body,
				EndOfStream: true,
			},
		},
	}
}

// Build constructs the ProcessingRequest with request headers phase.
func (b *ProcessingRequestBuilder) Build() *extprocv3.ProcessingRequest {
	headers := make([]*corev3.HeaderValue, 0, len(b.headers))
	for k, v := range b.headers {
		headers = append(headers, &corev3.HeaderValue{
			Key:      k,
			RawValue: []byte(v),
		})
	}

	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: headers,
				},
			},
		},
	}
}

// SendRequestHeaders sends a RequestHeaders phase message to ExtProc and returns the response.
// This is the primary processing phase where token exchange occurs.
// Handles the streaming gRPC protocol: sends one message, receives one response.
func SendRequestHeaders(
	ctx context.Context,
	client extprocv3.ExternalProcessorClient,
	req *extprocv3.ProcessingRequest,
) *extprocv3.ProcessingResponse {
	// Create streaming context with timeout
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	stream, err := client.Process(streamCtx)
	Expect(err).NotTo(HaveOccurred(), "failed to open gRPC stream")

	// Send the request headers
	err = stream.Send(req)
	Expect(err).NotTo(HaveOccurred(), "failed to send ProcessingRequest")

	// Close the send side (we're done sending headers for this request)
	err = stream.CloseSend()
	Expect(err).NotTo(HaveOccurred(), "failed to close send")

	// Receive the response
	resp, err := stream.Recv()
	Expect(err).NotTo(HaveOccurred(), "failed to receive ProcessingResponse")

	return resp
}

// SendRequestHeadersWithError sends a request headers message and expects to receive an error
// or an ImmediateResponse (failure path). Returns the response without failing the test.
func SendRequestHeadersWithError(
	ctx context.Context,
	client extprocv3.ExternalProcessorClient,
	req *extprocv3.ProcessingRequest,
) (*extprocv3.ProcessingResponse, error) {
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	stream, err := client.Process(streamCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to open gRPC stream: %w", err)
	}

	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if err := stream.CloseSend(); err != nil {
		return nil, fmt.Errorf("failed to close send: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("stream closed without response (EOF)")
		}
		return nil, fmt.Errorf("stream recv error: %w", err)
	}

	return resp, nil
}

// ConnectToExtProc creates a gRPC connection to the ExtProc server at addr.
// Returns the client and connection. Caller must call conn.Close() when done.
func ConnectToExtProc(addr string) (extprocv3.ExternalProcessorClient, *grpc.ClientConn) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	Expect(err).NotTo(HaveOccurred(), "failed to connect to ExtProc server at "+addr)
	return extprocv3.NewExternalProcessorClient(conn), conn
}

// ExtractMutatedAuthorizationHeader extracts the Authorization header value from a
// ProcessingResponse that mutates headers. The Authorization mutation is always in the
// RequestHeaders phase response — both non-OPA (direct exchange) and OPA (eager exchange
// in headers phase with requestBodyBufferingResponseWithAuth) set it there.
// Returns empty string if not found.
func ExtractMutatedAuthorizationHeader(resp *extprocv3.ProcessingResponse) string {
	headersResp, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	if !ok {
		return ""
	}
	if headersResp.RequestHeaders == nil ||
		headersResp.RequestHeaders.Response == nil ||
		headersResp.RequestHeaders.Response.HeaderMutation == nil {
		return ""
	}
	return findAuthorizationHeader(headersResp.RequestHeaders.Response.HeaderMutation.SetHeaders)
}

// findAuthorizationHeader returns the RawValue of the "authorization" header from a slice.
func findAuthorizationHeader(headers []*corev3.HeaderValueOption) string {
	for _, hvo := range headers {
		if hvo.Header != nil && hvo.Header.Key == "authorization" {
			return string(hvo.Header.RawValue)
		}
	}
	return ""
}

// ExtractImmediateResponseStatus extracts the HTTP status code from an ImmediateResponse.
// Returns 0 if the response is not an ImmediateResponse.
func ExtractImmediateResponseStatus(resp *extprocv3.ProcessingResponse) uint32 {
	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	if !ok {
		return 0
	}

	if immResp.ImmediateResponse == nil || immResp.ImmediateResponse.Status == nil {
		return 0
	}

	statusCode := immResp.ImmediateResponse.Status.Code
	if statusCode < 0 {
		return 0
	}
	return uint32(statusCode) // #nosec G115 -- statusCode is non-negative and int32 cannot exceed uint32.
}

// ExtractImmediateResponseBody extracts the body from an ImmediateResponse.
// Returns empty string if not an ImmediateResponse or no body.
func ExtractImmediateResponseBody(resp *extprocv3.ProcessingResponse) string {
	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	if !ok {
		return ""
	}

	if immResp.ImmediateResponse == nil {
		return ""
	}

	return string(immResp.ImmediateResponse.Body)
}

// WithEndOfStream sets the EndOfStream flag on the HttpHeaders message.
// Use this to simulate a header-only request (e.g. GET /mcp for SSE stream setup)
// where no body phase follows. ExtProc uses EndOfStream=true to trigger the
// mcp_headers_only evaluation path.
func (b *ProcessingRequestBuilder) WithEndOfStream(v bool) *ProcessingRequestBuilder {
	b.endOfStream = v
	return b
}

// WithAgentgatewayProtocol adds the protocol metadata emitted by Agentgateway.
func (b *ProcessingRequestBuilder) WithAgentgatewayProtocol(protocol string) *ProcessingRequestBuilder {
	b.protocol = protocol
	return b
}

// WithTokenExchangeMetadata adds the metadata required for ExtProc token exchange.
func (b *ProcessingRequestBuilder) WithTokenExchangeMetadata(subjectToken, resourceURI string) *ProcessingRequestBuilder {
	b.subjectToken = subjectToken
	b.resourceURI = resourceURI
	b.hasTokenExchangeMetadata = true
	return b
}

// WithAgentgatewayMCPServer adds an mcp_server field to the agentgateway filter metadata,
// alongside protocol. Calling this method always includes the field in the built metadata
// (even with an empty string), which BuildWithMetadata's default-injection (below) treats as
// "explicitly set" and does not override. This simulates the metadata agentgateway sends when
// it knows which MCP server a request targets (spec 044).
func (b *ProcessingRequestBuilder) WithAgentgatewayMCPServer(server string) *ProcessingRequestBuilder {
	b.mcpServer = &server
	return b
}

// WithoutAgentgatewayMCPServer explicitly opts a request out of BuildWithMetadata's
// mcp_server default-injection, producing metadata equivalent to agentgateway never having
// sent the field (present-but-empty and entirely-absent are handled identically by ExtProc
// per FR-004). Named readably for tests exercising the mandatory-metadata rejection path,
// rather than requiring callers to know that WithAgentgatewayMCPServer("") has the same effect.
func (b *ProcessingRequestBuilder) WithoutAgentgatewayMCPServer() *ProcessingRequestBuilder {
	return b.WithAgentgatewayMCPServer("")
}

// BuildWithMetadata constructs the ProcessingRequest with all configured Agentgateway metadata.
func (b *ProcessingRequestBuilder) BuildWithMetadata() *extprocv3.ProcessingRequest {
	headers := make([]*corev3.HeaderValue, 0, len(b.headers))
	for k, v := range b.headers {
		headers = append(headers, &corev3.HeaderValue{
			Key:      k,
			RawValue: []byte(v),
		})
	}

	req := &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: headers,
				},
				EndOfStream: b.endOfStream,
			},
		},
	}

	// mcp_server is mandatory for MCP requests once OPA authorization is enabled
	// (spec 044 FR-004). Default-inject a value for protocol=="mcp" builds so existing
	// tests unrelated to mcp_server itself don't need to set it explicitly. Tests that
	// exercise the true-absence rejection path call WithAgentgatewayMCPServer("") to opt
	// out: an explicit (even empty) value is never overridden by the default below.
	mcpServer := b.mcpServer
	if b.protocol == "mcp" && mcpServer == nil {
		def := "test-mcp-server"
		mcpServer = &def
	}

	filterMetadata := make(map[string]*structpb.Struct, 2)
	if b.protocol != "" || mcpServer != nil {
		fields := map[string]*structpb.Value{}
		if b.protocol != "" {
			fields["protocol"] = structpb.NewStringValue(b.protocol)
		}
		if mcpServer != nil {
			fields["mcp_server"] = structpb.NewStringValue(*mcpServer)
		}
		filterMetadata["agentgateway"] = &structpb.Struct{
			Fields: fields,
		}
	}
	if b.hasTokenExchangeMetadata {
		filterMetadata["aib.tokenexchange"] = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"subject_token": structpb.NewStringValue(b.subjectToken),
				"resource_uri":  structpb.NewStringValue(b.resourceURI),
			},
		}
	}
	if len(filterMetadata) != 0 {
		req.MetadataContext = &corev3.Metadata{FilterMetadata: filterMetadata}
	}

	return req
}

// SendHeadersAndBody sends a two-phase stream: RequestHeaders then RequestBody.
// Returns (headersResp, bodyResp). The Authorization header mutation is always in
// headersResp — OPA mode sets it in the headers phase (requestBodyBufferingResponseWithAuth)
// and non-OPA mode also sets it in the headers phase.
// If the headers phase returns an ImmediateResponse (e.g. exchange error in headers phase),
// (headersResp, nil) is returned and no body phase is sent.
func SendHeadersAndBody(
	ctx context.Context,
	client extprocv3.ExternalProcessorClient,
	headersReq *extprocv3.ProcessingRequest,
	bodyBytes []byte,
) (headersResp, bodyResp *extprocv3.ProcessingResponse) {
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	stream, err := client.Process(streamCtx)
	Expect(err).NotTo(HaveOccurred(), "failed to open gRPC stream")

	// Phase 1: send request headers
	err = stream.Send(headersReq)
	Expect(err).NotTo(HaveOccurred(), "failed to send RequestHeaders")

	headersResp, err = stream.Recv()
	Expect(err).NotTo(HaveOccurred(), "failed to receive response to RequestHeaders")

	// If headers phase returned ImmediateResponse, stream is complete — no body phase.
	if _, isImmediate := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse); isImmediate {
		return headersResp, nil
	}

	// Phase 2: send request body
	bodyReq := BuildRequestBody(bodyBytes)
	err = stream.Send(bodyReq)
	Expect(err).NotTo(HaveOccurred(), "failed to send RequestBody")

	err = stream.CloseSend()
	Expect(err).NotTo(HaveOccurred(), "failed to close send")

	bodyResp, err = stream.Recv()
	Expect(err).NotTo(HaveOccurred(), "failed to receive response to RequestBody")

	return headersResp, bodyResp
}
