package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// testLogger returns a discard logger for unit tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// testConfig returns a minimal valid config for unit tests.
func testConfig() *extprocconfig.Config {
	return &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{Bind: "127.0.0.1", Port: 50051},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:       "https://idp.example.com/oauth2/token",
			Issuer:              "https://idp.example.com",
			ClientID:            "test-client",
			ClientSecret:        "test-secret",
			ClientAssertionType: "id_token",
			ExchangeTimeout:     5 * time.Second,
			TLS:                 extprocconfig.TLSConfig{AllowHTTP: true},
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			Enabled:      true,
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
	}
}

// testConfigWithTelemetry returns a config with full telemetry enabled for observability tests.
func testConfigWithTelemetry() *extprocconfig.Config {
	cfg := testConfig()
	cfg.Telemetry = extprocconfig.TelemetryConfig{
		Enabled: true,
		Traces:  extprocconfig.TracesConfig{Enabled: true},
		Metrics: extprocconfig.MetricsConfig{Enabled: true},
	}
	return cfg
}

// mockExchanger is a controllable Exchanger for unit tests.
type mockExchanger struct {
	exchangeFunc   func(ctx context.Context, subjectToken, resourceURI string) (server.ExchangeResult, error)
	shutdownCalled bool
}

func (m *mockExchanger) Exchange(ctx context.Context, subjectToken, resourceURI string) (server.ExchangeResult, error) {
	return m.exchangeFunc(ctx, subjectToken, resourceURI)
}

func (m *mockExchanger) Shutdown() {
	m.shutdownCalled = true
}

type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) records(t *testing.T) []map[string]any {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	raw := bytes.TrimSpace(c.buf.Bytes())
	if len(raw) == 0 {
		return nil
	}

	lines := bytes.Split(raw, []byte("\n"))
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		record := map[string]any{}
		require.NoError(t, json.Unmarshal(line, &record), "expected JSON log line: %s", string(line))
		records = append(records, record)
	}

	return records
}

func newJSONTestLogger() (*slog.Logger, *logCapture) {
	capture := &logCapture{}
	logger := slog.New(slog.NewJSONHandler(capture, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, capture
}

func startTestServerWithConfigAndLogger(t *testing.T, cfg *extprocconfig.Config, exchanger server.Exchanger, logger *slog.Logger) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()

	svc := server.NewServer(cfg, exchanger, logger)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	go func() {
		_ = grpcSrv.Serve(listener)
	}()

	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := extprocv3.NewExternalProcessorClient(conn)

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	}
	return client, cleanup
}

func startTestServerWithAuthorizerConfigAndLogger(t *testing.T, cfg *extprocconfig.Config, exchanger server.Exchanger, auth authorization.Authorizer, logger *slog.Logger) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()

	svc := server.NewServerWithAuthorizer(cfg, exchanger, auth, logger)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	go func() {
		_ = grpcSrv.Serve(listener)
	}()

	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := extprocv3.NewExternalProcessorClient(conn)

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	}
	return client, cleanup
}

func requireLogRecord(t *testing.T, capture *logCapture, message string) map[string]any {
	t.Helper()

	records := capture.records(t)
	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}

	require.Failf(t, "missing log message", "expected log message %q in records %#v", message, records)
	return nil
}

// startTestServer registers the Server on a random in-process port and returns
// a connected client + cleanup function.
func startTestServer(t *testing.T, exchanger server.Exchanger) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()
	return startTestServerWithConfig(t, testConfig(), exchanger)
}

// startTestServerWithConfig registers the Server with the given config on a random in-process
// port and returns a connected client + cleanup function.
func startTestServerWithConfig(t *testing.T, cfg *extprocconfig.Config, exchanger server.Exchanger) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()

	svc := server.NewServer(cfg, exchanger, testLogger())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	go func() {
		_ = grpcSrv.Serve(listener)
	}()

	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := extprocv3.NewExternalProcessorClient(conn)

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	}
	return client, cleanup
}

// sendRequestHeaders opens a Process stream, sends a RequestHeaders message with
// valid token-exchange metadata, and returns the first ProcessingResponse.
func sendRequestHeaders(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string) (*extprocv3.ProcessingResponse, error) {
	t.Helper()
	return sendRequestHeadersWithProtocol(t, client, headers, "")
}

// sendRequestHeadersWithProtocol is like sendRequestHeaders but also attaches
// agentgateway protocol metadata when protocol is non-empty.
func sendRequestHeadersWithProtocol(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string, protocol string) (*extprocv3.ProcessingResponse, error) {
	t.Helper()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	headerList := make([]*corev3.HeaderValue, 0, len(headers))
	for k, v := range headers {
		headerList = append(headerList, &corev3.HeaderValue{Key: k, RawValue: []byte(v)})
	}

	req := &extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata(protocol),
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{Headers: headerList},
			},
		},
	}

	if err = stream.Send(req); err != nil {
		return nil, err
	}
	_ = stream.CloseSend()

	resp, err := stream.Recv()
	return resp, err
}

// sendRequestHeadersWithProtocolEOS sends a RequestHeaders message with EndOfStream=true
// (no body phase will follow), valid token-exchange metadata, and agentgateway protocol
// metadata when protocol is non-empty.
func sendRequestHeadersWithProtocolEOS(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string, protocol string) (*extprocv3.ProcessingResponse, error) {
	t.Helper()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	headerList := make([]*corev3.HeaderValue, 0, len(headers))
	for k, v := range headers {
		headerList = append(headerList, &corev3.HeaderValue{Key: k, RawValue: []byte(v)})
	}

	req := &extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata(protocol),
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers:     &corev3.HeaderMap{Headers: headerList},
				EndOfStream: true,
			},
		},
	}

	if err = stream.Send(req); err != nil {
		return nil, err
	}
	_ = stream.CloseSend()

	resp, err := stream.Recv()
	return resp, err
}

// ---------------------------------------------------------------------------
// T013: Token-exchange metadata input boundary
// ---------------------------------------------------------------------------

const (
	tokenExchangeMetadataNamespace = "aib.tokenexchange"
	subjectTokenMetadataField      = "subject_token"
	resourceURIMetadataField       = "resource_uri"

	invalidSubjectTokenMetadataBody = `{"error":"invalid_subject_token","error_description":"subject token metadata is missing or invalid"}`
	invalidResourceMetadataBody     = `{"error":"invalid_resource","error_description":"resource metadata is missing or invalid"}`
)

type exchangeCall struct {
	subjectToken string
	resourceURI  string
}

type exchangeCallRecorder struct {
	mu    sync.Mutex
	calls []exchangeCall
}

func (r *exchangeCallRecorder) record(subjectToken, resourceURI string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, exchangeCall{subjectToken: subjectToken, resourceURI: resourceURI})
}

func (r *exchangeCallRecorder) snapshot() []exchangeCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]exchangeCall(nil), r.calls...)
}

type callCounter struct {
	mu    sync.Mutex
	count int
}

func (c *callCounter) increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
}

func (c *callCounter) value() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.count
}

func recordingExchanger(recorder *exchangeCallRecorder, result server.ExchangeResult) *mockExchanger {
	return &mockExchanger{
		exchangeFunc: func(_ context.Context, subjectToken, resourceURI string) (server.ExchangeResult, error) {
			recorder.record(subjectToken, resourceURI)
			return result, nil
		},
	}
}

func tokenExchangeMetadata(fields map[string]*structpb.Value) *corev3.Metadata {
	return &corev3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{
			tokenExchangeMetadataNamespace: {Fields: fields},
		},
	}
}

// tokenExchangeMetadataWithProtocol builds token-exchange metadata with an
// agentgateway protocol field. When protocol == "mcp", it also sets a default
// mcp_server field, since mcp_server is mandatory for MCP requests once OPA
// authorization is enabled (spec 044 FR-004). Tests that need to exercise the
// absent-mcp_server rejection path must build metadata without this helper's
// default (see TestServer_OPA_AbsentMCPServerMetadata_Returns403).
func tokenExchangeMetadataWithProtocol(fields map[string]*structpb.Value, protocol string) *corev3.Metadata {
	metadata := tokenExchangeMetadata(fields)
	agwFields := map[string]*structpb.Value{
		"protocol": structpb.NewStringValue(protocol),
	}
	if protocol == "mcp" {
		agwFields["mcp_server"] = structpb.NewStringValue("test-mcp-server")
	}
	metadata.FilterMetadata["agentgateway"] = &structpb.Struct{
		Fields: agwFields,
	}
	return metadata
}

func tokenExchangeMetadataFields(subjectToken, resourceURI *structpb.Value) map[string]*structpb.Value {
	return map[string]*structpb.Value{
		subjectTokenMetadataField: subjectToken,
		resourceURIMetadataField:  resourceURI,
	}
}

const (
	testSubjectToken = "test-metadata-subject-token"
	testResourceURI  = "https://metadata.example.test/resource"
)

func validTokenExchangeMetadata(protocol string) *corev3.Metadata {
	fields := tokenExchangeMetadataFields(
		structpb.NewStringValue(testSubjectToken),
		structpb.NewStringValue(testResourceURI),
	)
	if protocol == "" {
		return tokenExchangeMetadata(fields)
	}
	return tokenExchangeMetadataWithProtocol(fields, protocol)
}

func extProcHeaders(headers map[string]string, endOfStream bool) *extprocv3.HttpHeaders {
	headerList := make([]*corev3.HeaderValue, 0, len(headers))
	for key, value := range headers {
		headerList = append(headerList, &corev3.HeaderValue{Key: key, RawValue: []byte(value)})
	}

	return &extprocv3.HttpHeaders{
		Headers:     &corev3.HeaderMap{Headers: headerList},
		EndOfStream: endOfStream,
	}
}

func sendRequestHeadersWithMetadata(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string, metadata *corev3.Metadata, endOfStream bool) (*extprocv3.ProcessingResponse, error) {
	t.Helper()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: metadata,
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(headers, endOfStream),
		},
	})
	if err != nil {
		return nil, err
	}
	_ = stream.CloseSend()

	return stream.Recv()
}

func sendHeadersThenBodyWithMetadata(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string, metadata *corev3.Metadata, body []byte) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	t.Helper()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: metadata,
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(headers, false),
		},
	})
	require.NoError(t, err)

	headersResp, err := stream.Recv()
	require.NoError(t, err)
	if _, isImmediate := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse); isImmediate {
		_ = stream.CloseSend()
		return headersResp, nil
	}

	err = stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{Body: body, EndOfStream: true},
		},
	})
	require.NoError(t, err)
	_ = stream.CloseSend()

	bodyResp, err := stream.Recv()
	require.NoError(t, err)
	return headersResp, bodyResp
}

func headerMutationValue(mutation *extprocv3.HeaderMutation, key string) string {
	if mutation == nil {
		return ""
	}
	for _, option := range mutation.SetHeaders {
		if option != nil && option.Header != nil && option.Header.Key == key {
			return string(option.Header.RawValue)
		}
	}
	return ""
}

func requireTokenExchangeMetadataRejection(t *testing.T, response *extprocv3.ProcessingResponse, wantBody string) {
	t.Helper()

	require.NotNil(t, response)
	immediate, ok := response.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "metadata validation must return an ImmediateResponse")
	require.NotNil(t, immediate.ImmediateResponse)
	require.NotNil(t, immediate.ImmediateResponse.Status)
	assert.Equal(t, int32(httpv3.StatusCode_ServiceUnavailable), int32(immediate.ImmediateResponse.Status.Code))
	assert.Equal(t, wantBody, string(immediate.ImmediateResponse.Body))
	assert.Equal(t, "application/json", headerMutationValue(immediate.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immediate.ImmediateResponse.Headers, "authorization"), "rejection must not mutate authorization")
}

func requireAuthorizationMutation(t *testing.T, response *extprocv3.ProcessingResponse) string {
	t.Helper()

	require.NotNil(t, response)
	headersResp, ok := response.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	require.True(t, ok, "successful token exchange must return a RequestHeaders response")
	require.NotNil(t, headersResp.RequestHeaders)
	require.NotNil(t, headersResp.RequestHeaders.Response)
	return headerMutationValue(headersResp.RequestHeaders.Response.HeaderMutation, "authorization")
}

func conflictingRawRequestHeaders(method string) map[string]string {
	return map[string]string{
		":method":       method,
		":path":         "https://raw-input.example.test/should-not-be-used",
		"authorization": "Bearer raw-input-subject-token",
	}
}

func TestServer_Process_TokenExchangeMetadataRejectsInvalidInputs(t *testing.T) {
	const (
		validSubjectToken = "metadata-subject-token"
		validResourceURI  = "https://metadata.example.test/mcp"
	)

	type testCase struct {
		name     string
		metadata func() *corev3.Metadata
		wantBody string
	}

	tests := []testCase{
		{
			name:     "metadata context is absent",
			metadata: func() *corev3.Metadata { return nil },
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "filter metadata is nil",
			metadata: func() *corev3.Metadata {
				return &corev3.Metadata{FilterMetadata: nil}
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name:     "token exchange namespace is absent",
			metadata: func() *corev3.Metadata { return &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{}} },
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "token exchange input is in another namespace",
			metadata: func() *corev3.Metadata {
				return &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
					"aib.other": {Fields: tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue(validResourceURI))},
				}}
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "token exchange namespace is nil",
			metadata: func() *corev3.Metadata {
				return &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{tokenExchangeMetadataNamespace: nil}}
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name:     "token exchange namespace has no fields",
			metadata: func() *corev3.Metadata { return tokenExchangeMetadata(nil) },
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token field is absent",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(map[string]*structpb.Value{resourceURIMetadataField: structpb.NewStringValue(validResourceURI)})
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token field is nil",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(nil, structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token is empty",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(""), structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token is whitespace only",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(" \t\r\n"), structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token has Bearer prefix",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue("Bearer metadata-subject-token"), structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token has lowercase bearer prefix",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue("bearer metadata-subject-token"), structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token has uppercase BEARER prefix",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue("BEARER metadata-subject-token"), structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token has mixed-case bearer prefix",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue("bEaReR metadata-subject-token"), structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "subject token has no protobuf value kind",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(&structpb.Value{}, structpb.NewStringValue(validResourceURI)))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "resource URI field is absent",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(map[string]*structpb.Value{subjectTokenMetadataField: structpb.NewStringValue(validSubjectToken)})
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI field is nil",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), nil))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI is empty",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue("")))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI is whitespace only",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue(" \t\r\n")))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI is relative",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue("/mcp")))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI has a non HTTP scheme",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue("grpc://metadata.example.test/mcp")))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI has no host",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue("https:///mcp")))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "resource URI is malformed",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), structpb.NewStringValue("https://metadata.example.test/%zz")))
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "subject token validation takes precedence over resource URI validation",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(" "), structpb.NewStringValue("/mcp")))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
	}

	nonStringValues := []struct {
		name  string
		value func() *structpb.Value
	}{
		{name: "null", value: func() *structpb.Value { return structpb.NewNullValue() }},
		{name: "number", value: func() *structpb.Value { return structpb.NewNumberValue(1) }},
		{name: "boolean", value: func() *structpb.Value { return structpb.NewBoolValue(true) }},
		{name: "struct", value: func() *structpb.Value { return structpb.NewStructValue(&structpb.Struct{}) }},
		{name: "list", value: func() *structpb.Value { return structpb.NewListValue(&structpb.ListValue{}) }},
	}
	for _, nonStringValue := range nonStringValues {
		nonStringValue := nonStringValue
		tests = append(tests,
			testCase{
				name: "subject token has " + nonStringValue.name + " protobuf value",
				metadata: func() *corev3.Metadata {
					return tokenExchangeMetadata(tokenExchangeMetadataFields(nonStringValue.value(), structpb.NewStringValue(validResourceURI)))
				},
				wantBody: invalidSubjectTokenMetadataBody,
			},
			testCase{
				name: "resource URI has " + nonStringValue.name + " protobuf value",
				metadata: func() *corev3.Metadata {
					return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(validSubjectToken), nonStringValue.value()))
				},
				wantBody: invalidResourceMetadataBody,
			},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &exchangeCallRecorder{}
			client, cleanup := startTestServer(t, recordingExchanger(recorder, server.ExchangeResult{Token: "must-not-be-returned"}))
			t.Cleanup(cleanup)

			response, err := sendRequestHeadersWithMetadata(t, client, conflictingRawRequestHeaders("POST"), tt.metadata(), false)
			require.NoError(t, err)
			requireTokenExchangeMetadataRejection(t, response, tt.wantBody)
			assert.Empty(t, recorder.snapshot(), "invalid metadata must not invoke token exchange")
		})
	}
}

func TestServer_Process_TokenExchangeMetadataOverridesRawInputsAndPropagatesSubjectVerbatim(t *testing.T) {
	const (
		subjectToken = " \tmetadata subject token\n"
		exchanged    = "exchanged-metadata-token"
	)

	tests := []struct {
		name        string
		resourceURI string
	}{
		{name: "HTTPS resource", resourceURI: "https://metadata.example.test/opaque-resource"},
		{name: "HTTP resource", resourceURI: "http://metadata.example.test/opaque-resource"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			recorder := &exchangeCallRecorder{}
			client, cleanup := startTestServer(t, recordingExchanger(recorder, server.ExchangeResult{Token: exchanged}))
			t.Cleanup(cleanup)

			response, err := sendRequestHeadersWithMetadata(t, client, conflictingRawRequestHeaders("POST"), tokenExchangeMetadata(map[string]*structpb.Value{
				subjectTokenMetadataField: structpb.NewStringValue(subjectToken),
				resourceURIMetadataField:  structpb.NewStringValue(tt.resourceURI),
				"unrelated":               structpb.NewStringValue("ignored"),
			}), false)
			require.NoError(t, err)

			assert.Equal(t, []exchangeCall{{subjectToken: subjectToken, resourceURI: tt.resourceURI}}, recorder.snapshot())
			assert.Equal(t, "Bearer "+exchanged, requireAuthorizationMutation(t, response))
		})
	}
}

func TestServer_OPA_TokenExchangeMetadataRejectsBeforeProtocolPolicyOrExchange(t *testing.T) {
	const validResourceURI = "https://metadata.example.test/mcp"

	type testCase struct {
		name     string
		metadata func() *corev3.Metadata
		wantBody string
	}
	tests := []testCase{
		{
			name: "subject token rejection takes precedence over resource URI and protocol metadata",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadata(tokenExchangeMetadataFields(structpb.NewStringValue(" "), structpb.NewStringValue("/mcp")))
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
		{
			name: "resource URI rejection precedes policy evaluation",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadataWithProtocol(tokenExchangeMetadataFields(structpb.NewStringValue("metadata-subject-token"), structpb.NewStringValue("/mcp")), "mcp")
			},
			wantBody: invalidResourceMetadataBody,
		},
		{
			name: "subject token rejection with protocol metadata",
			metadata: func() *corev3.Metadata {
				return tokenExchangeMetadataWithProtocol(tokenExchangeMetadataFields(structpb.NewStringValue("bearer metadata-subject-token"), structpb.NewStringValue(validResourceURI)), "mcp")
			},
			wantBody: invalidSubjectTokenMetadataBody,
		},
	}

	type requestShape struct {
		name string
		send func(t *testing.T, client extprocv3.ExternalProcessorClient, metadata *corev3.Metadata) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse)
	}
	shapes := []requestShape{
		{
			name: "body bearing request",
			send: func(t *testing.T, client extprocv3.ExternalProcessorClient, metadata *corev3.Metadata) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
				return sendHeadersThenBodyWithMetadata(t, client, conflictingRawRequestHeaders("POST"), metadata, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`))
			},
		},
		{
			name: "header only request",
			send: func(t *testing.T, client extprocv3.ExternalProcessorClient, metadata *corev3.Metadata) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
				response, err := sendRequestHeadersWithMetadata(t, client, conflictingRawRequestHeaders("GET"), metadata, true)
				require.NoError(t, err)
				return response, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, shape := range shapes {
				shape := shape
				t.Run(shape.name, func(t *testing.T) {
					recorder := &exchangeCallRecorder{}
					authorizerCalls := &callCounter{}
					authorizer := &mockAuthorizer{
						evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
							authorizerCalls.increment()
							return &authorization.OPADecision{Action: "allow"}, nil
						},
					}
					client, cleanup := startTestServerWithAuthorizer(t, recordingExchanger(recorder, server.ExchangeResult{Token: "must-not-be-returned"}), authorizer)
					t.Cleanup(cleanup)

					headersResp, bodyResp := shape.send(t, client, tt.metadata())
					assert.Nil(t, bodyResp, "invalid metadata must terminate the stream in the headers phase")
					requireTokenExchangeMetadataRejection(t, headersResp, tt.wantBody)
					assert.Empty(t, recorder.snapshot(), "invalid metadata must not invoke token exchange")
					assert.Equal(t, 0, authorizerCalls.value(), "metadata validation must run before policy evaluation")
				})
			}
		})
	}
}

func TestServer_OPA_TokenExchangeMetadataCarriesValuesThroughBodyPhase(t *testing.T) {
	const (
		subjectToken = " \tbody metadata subject\n"
		resourceURI  = "https://metadata.example.test/body-phase"
		exchanged    = "body-phase-exchanged-token"
	)

	recorder := &exchangeCallRecorder{}
	authorizerCalls := &callCounter{}
	authorizer := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			authorizerCalls.increment()
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, recordingExchanger(recorder, server.ExchangeResult{Token: exchanged}), authorizer)
	defer cleanup()

	headersResp, bodyResp := sendHeadersThenBodyWithMetadata(t, client, conflictingRawRequestHeaders("POST"), tokenExchangeMetadataWithProtocol(map[string]*structpb.Value{
		subjectTokenMetadataField: structpb.NewStringValue(subjectToken),
		resourceURIMetadataField:  structpb.NewStringValue(resourceURI),
		"unrelated":               structpb.NewStringValue("ignored"),
	}, "mcp"), []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`))

	assert.Equal(t, []exchangeCall{{subjectToken: subjectToken, resourceURI: resourceURI}}, recorder.snapshot())
	assert.Equal(t, "Bearer "+exchanged, requireAuthorizationMutation(t, headersResp))
	require.NotNil(t, bodyResp, "the OPA body phase must receive the request after validated metadata is stored")
	_, isRequestBody := bodyResp.Response.(*extprocv3.ProcessingResponse_RequestBody)
	assert.True(t, isRequestBody, "an allowed OPA body phase must return a RequestBody response")
	assert.Equal(t, 1, authorizerCalls.value(), "the body phase must evaluate policy once")
}

func TestServer_OPA_TokenExchangeMetadataCarriesValuesThroughHeadersOnly(t *testing.T) {
	const (
		subjectToken = " \theader only metadata subject\n"
		resourceURI  = "https://metadata.example.test/headers-only"
		exchanged    = "headers-only-exchanged-token"
	)

	recorder := &exchangeCallRecorder{}
	authorizerCalls := &callCounter{}
	authorizer := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			authorizerCalls.increment()
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, recordingExchanger(recorder, server.ExchangeResult{Token: exchanged}), authorizer)
	defer cleanup()

	response, err := sendRequestHeadersWithMetadata(t, client, conflictingRawRequestHeaders("GET"), tokenExchangeMetadataWithProtocol(tokenExchangeMetadataFields(
		structpb.NewStringValue(subjectToken),
		structpb.NewStringValue(resourceURI),
	), "mcp"), true)
	require.NoError(t, err)

	assert.Equal(t, []exchangeCall{{subjectToken: subjectToken, resourceURI: resourceURI}}, recorder.snapshot())
	assert.Equal(t, "Bearer "+exchanged, requireAuthorizationMutation(t, response))
	assert.Equal(t, 1, authorizerCalls.value(), "header-only OPA requests must evaluate policy before exchange")
}

// ---------------------------------------------------------------------------
// T015: ExtProc gRPC streaming logic — RequestHeaders processing
// ---------------------------------------------------------------------------

// Valid token-exchange metadata replaces the downstream Authorization header.
func TestServer_Process_TokenExchangeMetadata_ReplacesAuthorizationHeader(t *testing.T) {
	const subjectToken = "metadata-subject-token"
	const exchangedToken = "exchanged-downstream-token"
	const resourceURI = "https://metadata.example.test/mcp"

	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, st, ru string) (server.ExchangeResult, error) {
			assert.Equal(t, subjectToken, st, "Exchange must receive the metadata subject token")
			assert.Equal(t, resourceURI, ru, "Exchange must receive the metadata resource URI")
			return server.ExchangeResult{Token: exchangedToken}, nil
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeadersWithMetadata(t, client, nil, tokenExchangeMetadata(tokenExchangeMetadataFields(
		structpb.NewStringValue(subjectToken),
		structpb.NewStringValue(resourceURI),
	)), false)
	require.NoError(t, err)

	headersResp, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	require.True(t, ok, "successful exchange should produce a RequestHeaders response")
	require.NotNil(t, headersResp.RequestHeaders)
	require.NotNil(t, headersResp.RequestHeaders.Response)
	require.NotNil(t, headersResp.RequestHeaders.Response.HeaderMutation)

	setHeaders := headersResp.RequestHeaders.Response.HeaderMutation.SetHeaders
	require.NotEmpty(t, setHeaders, "exchanged token must be set in headers")

	var authValue string
	for _, h := range setHeaders {
		if h.Header.Key == "authorization" {
			authValue = string(h.Header.RawValue)
		}
	}
	assert.Equal(t, "Bearer "+exchangedToken, authValue,
		"Authorization header must be replaced with exchanged token")
}

// Spec: FR-008, FR-010 — Token exchange failure → 500 ImmediateResponse
func TestServer_Process_ExchangeFailure_Returns500(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, errors.New("exchange endpoint returned 403")
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "exchange failure must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_InternalServerError),
		int32(immResp.ImmediateResponse.Status.Code),
		"exchange failure must return HTTP 500")
	assert.Equal(t, `{"error":"token_exchange_failed","error_description":"token exchange request failed"}`, string(immResp.ImmediateResponse.Body))
	assert.Equal(t, "application/json", headerMutationValue(immResp.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immResp.ImmediateResponse.Headers, "authorization"), "exchange failure must not mutate authorization")
}

// SR-003 — Exchanged token details must not appear in error responses (information disclosure)
func TestServer_Process_ErrorResponse_DoesNotLeakTokenDetails(t *testing.T) {
	const internalError = "upstream returned 403: access_denied for client xyz"
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, errors.New(internalError)
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeadersWithMetadata(t, client, nil, tokenExchangeMetadata(tokenExchangeMetadataFields(
		structpb.NewStringValue("secret-token"),
		structpb.NewStringValue(testResourceURI),
	)), false)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok)
	assert.NotContains(t, string(immResp.ImmediateResponse.Body), internalError,
		"internal error details must not be exposed in response body")
	assert.NotContains(t, string(immResp.ImmediateResponse.Body), "secret-token",
		"token values must not appear in error response body")
}

// Spec: FR-006 — expired client assertion → 503 ImmediateResponse
func TestServer_Process_ExpiredAssertion_Returns503(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, server.ErrAssertionExpired
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "expired assertion must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_ServiceUnavailable),
		int32(immResp.ImmediateResponse.Status.Code),
		"expired assertion must return HTTP 503")
	assert.Equal(t, `{"error":"service_unavailable","error_description":"client assertion expired"}`, string(immResp.ImmediateResponse.Body))
	assert.Equal(t, "application/json", headerMutationValue(immResp.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immResp.ImmediateResponse.Headers, "authorization"), "expired assertion must not mutate authorization")

}

// Spec: FR-001 — Server must implement ExternalProcessorServer interface
func TestServer_ImplementsExternalProcessorServer(t *testing.T) {
	cfg := testConfig()
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, nil
		},
	}
	svc := server.NewServer(cfg, exchanger, testLogger())

	// Compile-time interface check
	var _ extprocv3.ExternalProcessorServer = svc
}

// ---------------------------------------------------------------------------
// T023: OPA authorization flow — server integration tests
// ---------------------------------------------------------------------------

// mockAuthorizer is a controllable authorization.Authorizer for unit tests.
type mockAuthorizer struct {
	evaluateFunc func(ctx context.Context, input authorization.OPAInput) (*authorization.OPADecision, error)
	stopCalled   bool
}

func (m *mockAuthorizer) Evaluate(ctx context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
	return m.evaluateFunc(ctx, input)
}

func (m *mockAuthorizer) Stop(_ context.Context) {
	m.stopCalled = true
}

type mockApprovalBroker struct {
	readFunc    func(context.Context, string, []string) ([]approval.Pair, string, error)
	createFunc  func(context.Context, string, approval.CreateRequest) (string, error)
	consumeFunc func(context.Context, string, string) error
}

func (m *mockApprovalBroker) Read(ctx context.Context, principal string, sessionIDs []string) ([]approval.Pair, string, error) {
	return m.readFunc(ctx, principal, sessionIDs)
}

func (m *mockApprovalBroker) Create(ctx context.Context, subjectToken string, request approval.CreateRequest) (string, error) {
	return m.createFunc(ctx, subjectToken, request)
}

func (m *mockApprovalBroker) Consume(ctx context.Context, subjectToken, approvalID string) error {
	return m.consumeFunc(ctx, subjectToken, approvalID)
}

// startTestServerWithAuthorizer registers a Server (with OPA) on a random port.
func startTestServerWithAuthorizer(t *testing.T, exchanger server.Exchanger, auth authorization.Authorizer) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()

	cfg := testConfig()
	cfg.Authorization = extprocconfig.AuthorizationConfig{
		Enabled:           true,
		Policy:            extprocconfig.PolicyConfig{Package: "aib.extproc.authz", Decision: "result"},
		DefaultDecision:   "deny",
		EvaluationTimeout: 5 * time.Second,
		MaxBodySize:       1048576,
	}
	svc := server.NewServerWithAuthorizer(cfg, exchanger, auth, testLogger())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)
	go func() { _ = grpcSrv.Serve(listener) }()

	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	}
	return extprocv3.NewExternalProcessorClient(conn), cleanup
}

func startTestServerWithAuthorizerConfig(t *testing.T, cfg *extprocconfig.Config, exchanger server.Exchanger, auth authorization.Authorizer) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()
	return startTestServerWithAuthorizerConfigAndGate(t, cfg, exchanger, auth, nil)
}

func startTestServerWithAuthorizerConfigAndGate(t *testing.T, cfg *extprocconfig.Config, exchanger server.Exchanger, auth authorization.Authorizer, gate server.ApprovalGate) (extprocv3.ExternalProcessorClient, func()) {
	t.Helper()

	if !cfg.Authorization.Enabled {
		cfg.Authorization.Enabled = true
	}
	if cfg.Authorization.Policy.Package == "" {
		cfg.Authorization.Policy.Package = "aib.extproc.authz"
	}
	if cfg.Authorization.Policy.Decision == "" {
		cfg.Authorization.Policy.Decision = "result"
	}
	if cfg.Authorization.DefaultDecision == "" {
		cfg.Authorization.DefaultDecision = "deny"
	}
	if cfg.Authorization.EvaluationTimeout == 0 {
		cfg.Authorization.EvaluationTimeout = 5 * time.Second
	}
	if cfg.Authorization.MaxBodySize == 0 {
		cfg.Authorization.MaxBodySize = 1048576
	}
	var svc *server.Server
	if gate != nil {
		svc = server.NewServerWithApprovalGate(cfg, exchanger, auth, gate, testLogger())
	} else {
		svc = server.NewServerWithAuthorizer(cfg, exchanger, auth, testLogger())
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)
	go func() { _ = grpcSrv.Serve(listener) }()

	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
	}
	return extprocv3.NewExternalProcessorClient(conn), cleanup
}

// sendHeadersThenBody sends RequestHeaders with valid token-exchange and default MCP
// protocol metadata followed by RequestBody on the same stream.
func sendHeadersThenBody(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string, body []byte) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	t.Helper()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	headerList := make([]*corev3.HeaderValue, 0, len(headers))
	for k, v := range headers {
		headerList = append(headerList, &corev3.HeaderValue{Key: k, RawValue: []byte(v)})
	}

	metadata := validTokenExchangeMetadata("mcp")

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: metadata,
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{Headers: headerList},
			},
		},
	})
	require.NoError(t, err)

	headersResp, err := stream.Recv()
	require.NoError(t, err)

	// If headers response is an ImmediateResponse, stop here
	if _, isImm := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse); isImm {
		_ = stream.CloseSend()
		return headersResp, nil
	}

	// Send body
	err = stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{
				Body:        body,
				EndOfStream: true,
			},
		},
	})
	require.NoError(t, err)
	_ = stream.CloseSend()

	bodyResp, err := stream.Recv()
	require.NoError(t, err)

	return headersResp, bodyResp
}

// sendHeadersThenBodyWithProtocol is like sendHeadersThenBody but uses the given
// agentgateway protocol metadata instead of the default "mcp".
func sendHeadersThenBodyWithProtocol(t *testing.T, client extprocv3.ExternalProcessorClient, headers map[string]string, body []byte, protocol string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	t.Helper()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	headerList := make([]*corev3.HeaderValue, 0, len(headers))
	for k, v := range headers {
		headerList = append(headerList, &corev3.HeaderValue{Key: k, RawValue: []byte(v)})
	}

	metadata := validTokenExchangeMetadata(protocol)

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: metadata,
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{Headers: headerList},
			},
		},
	})
	require.NoError(t, err)

	headersResp, err := stream.Recv()
	require.NoError(t, err)

	if _, isImm := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse); isImm {
		_ = stream.CloseSend()
		return headersResp, nil
	}

	err = stream.Send(&extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{Body: body, EndOfStream: true},
		},
	})
	require.NoError(t, err)
	_ = stream.CloseSend()

	bodyResp, err := stream.Recv()
	require.NoError(t, err)

	return headersResp, bodyResp
}

// Spec: OPA disabled (nil authorizer) → direct token exchange in headers phase (no body buffering)
func TestServer_OPA_Disabled_DirectExchange(t *testing.T) {
	const exchangedToken = "direct-exchanged-token"
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: exchangedToken}, nil
		},
	}
	// NewServer (no authorizer) = OPA disabled
	cfg := testConfig()
	svc := server.NewServer(cfg, exchanger, testLogger())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)
	go func() { _ = grpcSrv.Serve(listener) }()
	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close(); grpcSrv.GracefulStop() }()
	client := extprocv3.NewExternalProcessorClient(conn)

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	headersResp, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	require.True(t, ok, "OPA disabled: must get RequestHeaders response (not body buffering)")
	require.NotNil(t, headersResp.RequestHeaders.Response)
	assert.Nil(t, resp.ModeOverride,
		"OPA disabled: headers response must not request body buffering")

	var authValue string
	for _, h := range headersResp.RequestHeaders.Response.HeaderMutation.SetHeaders {
		if h.Header.Key == "authorization" {
			authValue = string(h.Header.RawValue)
		}
	}
	assert.Equal(t, "Bearer "+exchangedToken, authValue)
}

// Spec: OPA enabled + allow → token exchange completes, Authorization header replaced
func TestServer_OPA_Enabled_Allow_ExchangeCompletes(t *testing.T) {
	const exchangedToken = "opa-allowed-token"
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: exchangedToken}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`))

	require.NotNil(t, bodyResp, "expected body response after OPA allow")
	// Must NOT be an ImmediateResponse (deny)
	_, isImmediate := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	assert.False(t, isImmediate, "OPA allow must not produce ImmediateResponse")
}

// Spec: OPA enabled + deny → 403 ImmediateResponse with JSON access_denied body
// Note: token exchange now occurs eagerly in the RequestHeaders phase (before OPA evaluation)
// to support proxies that only apply header mutations from HeadersResponse. When OPA denies,
// the 403 ImmediateResponse from the body phase prevents the already-mutated header from
// reaching the backend.
func TestServer_OPA_Enabled_Deny_Returns403(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{
				Action:  "deny",
				Reasons: []string{"tool is destructive"},
			}, nil
		},
	}
	exchanger := &mockExchanger{
		// Exchange IS called in the headers phase (eager exchange); the subsequent OPA
		// denial in the body phase returns 403 and prevents the token from reaching the backend.
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "pre-exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"delete_file"}}`))

	require.NotNil(t, bodyResp, "expected response in body phase after OPA deny")
	immResp, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "OPA deny must produce ImmediateResponse")
	assert.Equal(t, int32(403), int32(immResp.ImmediateResponse.Status.Code))
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "access_denied")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "tool is destructive")
}

// Spec: OPA enabled + header-only request (end_of_stream=true) + allow →
// OPA is evaluated with type="mcp_headers_only" and token exchange completes in the headers
// phase, replacing downstream Authorization with the exchanged token.
func TestServer_OPA_HeadersOnly_EndOfStream_Allow_ExchangeCompletes(t *testing.T) {
	var capturedInput authorization.OPAInput
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			capturedInput = input
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method": "GET",
	}, "mcp")
	require.NoError(t, err)

	headersResp, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	require.True(t, ok, "OPA allow on header-only request must return RequestHeaders response with token mutation")
	require.NotNil(t, headersResp.RequestHeaders)
	var authHeader string
	for _, h := range headersResp.RequestHeaders.Response.HeaderMutation.SetHeaders {
		if h.Header.Key == "authorization" {
			authHeader = string(h.Header.RawValue)
		}
	}
	assert.Equal(t, "Bearer exchanged-token", authHeader)
	require.NotNil(t, capturedInput, "OPA authorizer must be called for header-only request")
	assert.Equal(t, "mcp_headers_only", capturedInput["type"],
		"MCP header-only request must produce type=mcp_headers_only in OPA input, not type=unknown")
}

// Spec: OPA enabled + header-only request (end_of_stream=true) + deny →
// OPA is evaluated and the request is rejected with 403.
func TestServer_OPA_HeadersOnly_EndOfStream_Deny_Returns403(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "deny", Reasons: []string{"not authorized"}}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			t.Fatal("Exchange must not be called when OPA denies")
			return server.ExchangeResult{}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method": "GET",
	}, "mcp")
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "OPA deny on header-only request must return ImmediateResponse")
	assert.Equal(t, int32(403), int32(immResp.ImmediateResponse.Status.Code))
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "access_denied")
}

// Spec: OPA allow + header-only + broker returns 401 + error_uri →
// URLElicitationRequiredError must be returned (same as body-phase path).
func TestServer_OPA_HeadersOnly_EndOfStream_ReAuth_Returns401WithElicitation(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	reAuthURL := "https://idp.example.com/reauth"
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode: 401,
				Code:       "reauth_required",
				ErrorURI:   reAuthURL,
			}
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method": "GET",
	}, "mcp")
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "re-auth on header-only MCP must produce ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"MCP re-auth must return HTTP 200 with JSON-RPC -32042")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), reAuthURL)
}

// Spec: OPA allow + header-only + broker returns 500 + error_uri →
// transient 5xx must NOT return elicitation; returns normal exchange failure (500).
func TestServer_OPA_HeadersOnly_EndOfStream_5xxWithErrorURI_Returns500NotElicitation(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode: 500,
				Code:       "server_error",
				ErrorURI:   "https://idp.example.com/reauth",
			}
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method": "GET",
	}, "mcp")
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "5xx with error_uri on header-only must produce ImmediateResponse")
	assert.NotEqual(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"transient 5xx must not return elicitation (HTTP 200)")
	assert.Equal(t, int32(httpv3.StatusCode_InternalServerError), int32(immResp.ImmediateResponse.Status.Code),
		"transient 5xx with error_uri on header-only must return 500")
}

// Spec: Only GET is a valid MCP header-only transport (SSE streams, upgrade handshakes).
// Body-bearing methods (POST/PUT/PATCH) without a body return 400 (malformed JSON-RPC).
// POST with no body returns 400. All other non-GET methods return 405 with Allow: GET, POST.
func TestServer_OPA_HeadersOnly_MCP_InvalidMethods(t *testing.T) {
	type testCase struct {
		method    string
		wantCode  httpv3.StatusCode
		wantAllow bool
	}
	cases := []testCase{
		// POST without a body is malformed JSON-RPC → 400
		{"POST", httpv3.StatusCode_BadRequest, false},
		// Unsupported methods → 405 with Allow: GET, POST
		{"PUT", httpv3.StatusCode_MethodNotAllowed, true},
		{"PATCH", httpv3.StatusCode_MethodNotAllowed, true},
		{"DELETE", httpv3.StatusCode_MethodNotAllowed, true},
		{"HEAD", httpv3.StatusCode_MethodNotAllowed, true},
		{"OPTIONS", httpv3.StatusCode_MethodNotAllowed, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			auth := &mockAuthorizer{
				evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
					return &authorization.OPADecision{Action: "allow"}, nil
				},
			}
			exchanger := &mockExchanger{
				exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
					return server.ExchangeResult{Token: "exchanged-token"}, nil
				},
			}
			client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
			defer cleanup()

			resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
				":method": tc.method,
			}, "mcp")
			require.NoError(t, err)

			immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
			require.True(t, ok, "%s with no body must produce an ImmediateResponse", tc.method)
			assert.Equal(t, int32(tc.wantCode), int32(immResp.ImmediateResponse.Status.Code),
				"%s: unexpected status code", tc.method)
			if tc.wantAllow {
				var allowHeader string
				for _, h := range immResp.ImmediateResponse.Headers.GetSetHeaders() {
					if h.Header.Key == "allow" {
						allowHeader = string(h.Header.RawValue)
					}
				}
				assert.Equal(t, "GET, POST", allowHeader, "%s 405 response must include Allow: GET, POST", tc.method)
			}
		})
	}
}

// Spec: GET is the valid MCP header-only method for SSE streams. An EOS GET must
// succeed through the full OPA+exchange path and return mcp_headers_only input type.
func TestServer_OPA_HeadersOnly_MCP_GET_Succeeds(t *testing.T) {
	var capturedMethod string
	var hasRequestWrapper bool
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			// Verify input.attributes.request.http.method (envoy-plugin path)
			if attrs, ok := input["attributes"].(map[string]any); ok {
				if req, ok := attrs["request"].(map[string]any); ok {
					if http, ok := req["http"].(map[string]any); ok {
						capturedMethod, _ = http["method"].(string)
					}
				}
			}
			_, hasRequestWrapper = input["request"]
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method": "GET",
	}, "mcp")
	require.NoError(t, err)

	headersResp, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	require.True(t, ok, "MCP GET EOS must return RequestHeaders (token mutated)")
	var authHeader string
	for _, h := range headersResp.RequestHeaders.Response.HeaderMutation.SetHeaders {
		if h.Header.Key == "authorization" {
			authHeader = string(h.Header.RawValue)
		}
	}
	assert.Equal(t, "Bearer exchanged-token", authHeader)
	assert.Equal(t, "GET", capturedMethod,
		"mcp_headers_only OPA input must expose GET method via input.attributes.request.http.method")
	assert.False(t, hasRequestWrapper,
		"mcp_headers_only OPA input must not duplicate HTTP request fields at top level")
}

// GET is a header-only MCP transport; a body-bearing GET must be rejected in the body phase.
func TestServer_OPA_MCP_GET_WithBody_Returns405(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
		":method": "GET",
	}, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`), "mcp")

	require.NotNil(t, bodyResp)
	immResp, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "body-bearing MCP GET must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_MethodNotAllowed), int32(immResp.ImmediateResponse.Status.Code))
	var allowHeader string
	for _, h := range immResp.ImmediateResponse.Headers.GetSetHeaders() {
		if h.Header.Key == "allow" {
			allowHeader = string(h.Header.RawValue)
		}
	}
	assert.Equal(t, "GET, POST", allowHeader)
}

// PUT and PATCH are not valid MCP transports even when a body is present — they
// must be rejected with 405 before buffering, not processed as JSON-RPC.
func TestServer_OPA_MCP_InvalidBodyMethods_Return405(t *testing.T) {
	for _, method := range []string{"PUT", "PATCH"} {
		method := method
		t.Run(method, func(t *testing.T) {
			auth := &mockAuthorizer{
				evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
					return &authorization.OPADecision{Action: "allow"}, nil
				},
			}
			exchanger := &mockExchanger{
				exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
					return server.ExchangeResult{Token: "exchanged-token"}, nil
				},
			}
			client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
			defer cleanup()

			headersResp, _ := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
				":method": method,
			}, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`), "mcp")

			immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
			require.True(t, ok, "%s MCP request must be rejected with ImmediateResponse", method)
			assert.Equal(t, int32(httpv3.StatusCode_MethodNotAllowed), int32(immResp.ImmediateResponse.Status.Code))
			var allowHeader string
			for _, h := range immResp.ImmediateResponse.Headers.GetSetHeaders() {
				if h.Header.Key == "allow" {
					allowHeader = string(h.Header.RawValue)
				}
			}
			assert.Equal(t, "GET, POST", allowHeader)
		})
	}
}

// Spec: Process must handle stream correctly — CloseSend triggers clean completion
func TestServer_Process_StreamHandledCleanly(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "tok"}, nil
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	// Close without sending anything — server must handle gracefully
	err = stream.CloseSend()
	require.NoError(t, err)

	_, err = stream.Recv()
	// EOF or a non-error close is acceptable
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			assert.NotEqual(t, codes.Internal, st.Code(),
				"unclean close should not produce Internal error")
		}
	}
}

// ---------------------------------------------------------------------------
// MCP URL Elicitation — BrokerExchangeError with ErrorURI
// ---------------------------------------------------------------------------

// Spec: When broker returns error_uri and protocol is "mcp", headers phase immediately
// returns HTTP 200 with JSON-RPC -32042 URLElicitationRequiredError. id is null because
// the request body has not been read yet; this is correct per JSON-RPC 2.0 §5.
func TestServer_Process_BrokerErrorWithURI_ReturnsElicitationFromHeadersPhase(t *testing.T) {
	reAuthURL := "https://broker.example.com/api/third-party/svc-123/oauth2/authorize"
	const description = "User session has expired. Please re-authenticate."
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode:  401,
				Code:        "invalid_grant",
				Description: description,
				ErrorURI:    reAuthURL,
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocol(t, client, nil, "mcp")
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "headers phase must return ImmediateResponse for elicitation")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"elicitation must use HTTP 200 (JSON-RPC errors always travel over HTTP 200)")
	assert.Equal(t, "application/json", headerMutationValue(immResp.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immResp.ImmediateResponse.Headers, "authorization"), "elicitation must not mutate authorization")

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
	require.NoError(t, json.Unmarshal(immResp.ImmediateResponse.Body, &envelope))

	assert.Equal(t, "2.0", envelope.JSONRPC)
	assert.Nil(t, envelope.ID, "id must be JSON null (body not yet available in headers phase)")
	assert.Equal(t, -32042, envelope.Error.Code, "must use JSON-RPC error code -32042")
	assert.Equal(t, description, envelope.Error.Message)
	require.Len(t, envelope.Error.Data.Elicitations, 1)
	assert.Equal(t, "url", envelope.Error.Data.Elicitations[0].Mode)
	assert.Equal(t, reAuthURL, envelope.Error.Data.Elicitations[0].URL)
	assert.NotEmpty(t, envelope.Error.Data.Elicitations[0].ElicitationID, "elicitationId must be set")
}

// Spec: When broker returns error_uri and protocol metadata is absent, non-OPA mode
// returns URLElicitationRequiredError (same as protocol="mcp"). The absent protocol
// defaults to MCP for token-exchange-only deployments.
func TestServer_Process_BrokerErrorWithURI_AbsentProtocolMetadata_ReturnsElicitationFromHeadersPhase(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode:  401,
				Code:        "invalid_grant",
				Description: "session expired",
				ErrorURI:    "https://broker.example.com/api/third-party/svc-123/oauth2/authorize",
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "absent protocol metadata must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"absent protocol metadata defaults to MCP — elicitation uses HTTP 200")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "-32042",
		"absent protocol metadata must return URLElicitationRequiredError (JSON-RPC -32042), not 503")
}

// Spec: Absent protocol metadata defaults to MCP in non-OPA mode.
func TestServer_Process_BrokerErrorWithURI_AbsentProtocolMetadata_DefaultsToMCP(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode:  401,
				Code:        "invalid_grant",
				Description: "session expired",
				ErrorURI:    "https://broker.example.com/api/third-party/svc-999/oauth2/authorize",
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "absent protocol metadata must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"absent protocol metadata defaults to MCP — elicitation uses HTTP 200")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "-32042",
		"absent protocol metadata must return URLElicitationRequiredError (JSON-RPC -32042)")
}

// Spec: Absent protocol metadata defaults to MCP for direct ExtProc clients.
func TestServer_Process_BrokerErrorWithURI_AbsentProtocolMetadata_DirectClientDefaultsToMCP(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode:  401,
				Code:        "invalid_grant",
				Description: "session expired",
				ErrorURI:    "https://broker.example.com/api/third-party/svc-123/oauth2/authorize",
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "absent protocol metadata must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"absent protocol metadata defaults to MCP — elicitation uses HTTP 200")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "-32042",
		"absent protocol metadata returns JSON-RPC -32042")
}

// Spec: When broker returns error_uri and protocol is explicitly non-MCP (e.g. "a2a"),
// headers phase must return 503 — URLElicitationRequiredError is MCP-specific.
func TestServer_Process_BrokerErrorWithURI_NonMCP_Returns503FromHeadersPhase(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode:  401,
				Code:        "invalid_grant",
				Description: "session expired",
				ErrorURI:    "https://idp.example.com/reauth",
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocol(t, client, nil, "a2a")
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "non-MCP re-auth must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_ServiceUnavailable), int32(immResp.ImmediateResponse.Status.Code),
		"non-MCP re-auth in headers phase must return 503")
	assert.Equal(t, `{"error":"service_unavailable","error_description":"token exchange requires re-authentication"}`, string(immResp.ImmediateResponse.Body))
	assert.Equal(t, "application/json", headerMutationValue(immResp.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immResp.ImmediateResponse.Headers, "authorization"), "non-MCP re-auth must not mutate authorization")

}

// Spec: A transient 5xx broker error carrying error_uri must NOT trigger re-auth
// elicitation — it must be treated as a normal exchange failure (500), even when
// the exchange cache is empty (cache miss). Transient errors must never surface
// a spurious re-auth URL.
func TestServer_Process_5xxBrokerErrorWithURI_Returns500NotElicitation(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode: 500,
				Code:       "server_error",
				ErrorURI:   "https://idp.example.com/reauth",
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocol(t, client, nil, "mcp")
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "5xx with error_uri must produce an ImmediateResponse")
	assert.NotEqual(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"transient 5xx with error_uri must not return elicitation (HTTP 200)")
	assert.Equal(t, int32(httpv3.StatusCode_InternalServerError), int32(immResp.ImmediateResponse.Status.Code),
		"transient 5xx with error_uri must return 500, not elicitation")
	assert.Equal(t, `{"error":"token_exchange_failed","error_description":"token exchange request failed"}`, string(immResp.ImmediateResponse.Body))
	assert.Equal(t, "application/json", headerMutationValue(immResp.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immResp.ImmediateResponse.Headers, "authorization"), "transient failure must not mutate authorization")
}

// ---------------------------------------------------------------------------
// FR-003: OPA mode — absent protocol metadata
// ---------------------------------------------------------------------------

// Spec: FR-003 — when OPA is enabled and agentgateway protocol metadata is absent,
// the request MUST be rejected with 403 without calling the exchanger or authorizer.
func TestServer_OPA_AbsentProtocolMetadata_Returns403(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			t.Fatal("Evaluate must not be called when protocol metadata is absent")
			return nil, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			t.Fatal("Exchange must not be called when protocol metadata is absent")
			return server.ExchangeResult{}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata(""),
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(nil, false),
		},
	})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "absent protocol metadata must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immResp.ImmediateResponse.Status.Code))
}

// Spec: Exchange fails with re-auth → elicitation is returned in the headers-phase response.
// Token exchange now occurs eagerly in the headers phase (before the body is seen), so the
// JSON-RPC id cannot be extracted from the request body; the elicitation carries id=null.
func TestServer_OPA_BodyPhaseReauth_ReturnsElicitationInHeadersPhase(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	reAuthErr := &server.BrokerExchangeError{
		StatusCode:  401,
		Code:        "reauth_required",
		Description: "session expired",
		ErrorURI:    "https://idp.example.com/reauth",
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, reAuthErr
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	headersResp, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","id":42,"params":{"name":"list_files"}}`))

	// Exchange failed in headers phase → elicitation is in headersResp, no body phase.
	assert.Nil(t, bodyResp, "re-auth in headers phase must not produce a body-phase response")
	immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "re-auth must produce an ImmediateResponse in the headers phase")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code))
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "idp.example.com/reauth")

	var rpcErr map[string]any
	require.NoError(t, json.Unmarshal(immResp.ImmediateResponse.Body, &rpcErr))
	assert.Nil(t, rpcErr["id"], "headers-phase re-auth elicitation carries id=null (body not yet seen)")
}

// Spec: An MCP body-phase request with an empty body must be rejected with 403 (fail-closed).
// The empty-body path in buildMCPInput is reserved for EOS header-only requests via
// BuildOPAInputHeadersOnly; a zero-length body reaching the body phase is malformed.
// Token exchange occurs eagerly in the headers phase (before the body is seen), so
// exchange IS called. The 403 is returned from the body phase after OPA rejects the
// empty body.
func TestServer_OPA_BodyPhase_EmptyMCPBody_Denied(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			t.Fatal("OPA authorizer must not be called for malformed (empty-body) MCP requests")
			return nil, nil
		},
	}
	exchanger := &mockExchanger{
		// Exchange IS called eagerly in the headers phase before the body arrives.
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "pre-exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, []byte{})

	require.NotNil(t, bodyResp, "empty-body MCP request must produce a body-phase response")
	immResp, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "empty-body MCP request must produce ImmediateResponse")
	assert.Equal(t, int32(403), int32(immResp.ImmediateResponse.Status.Code),
		"empty-body MCP request must be denied with 403")
}

// Spec: Bodies larger than authorization.max_body_size must be rejected before OPA evaluation.
func TestServer_OPA_BodyPhase_RequestTooLarge_DeniesWithoutAuthorizerCall(t *testing.T) {
	cfg := testConfig()
	cfg.Authorization.Enabled = true
	cfg.Authorization.MaxBodySize = 10

	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			t.Fatal("OPA authorizer must not be called when the body exceeds max_body_size")
			return nil, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "pre-exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfig(t, cfg, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, []byte("01234567890"))

	require.NotNil(t, bodyResp, "oversized request must produce a body-phase response")
	immResp, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "oversized request must produce ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immResp.ImmediateResponse.Status.Code))
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "request_too_large")
}

// Spec: A batch where every element is allowed must evaluate each element and echo the full body.
func TestServer_OPA_BatchBodyPhase_Allow_EchoesBody(t *testing.T) {
	var seenToolNames []string
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			mcp, ok := input["mcp"].(*authorization.MCPInput)
			require.True(t, ok, "batch element must expose parsed MCP input")
			seenToolNames = append(seenToolNames, mcp.ToolName)
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "batch-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	batch := []byte(`[{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"list_files","arguments":{}}},{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"read_config","arguments":{}}}]`)
	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, batch)

	require.NotNil(t, bodyResp)
	requestBodyResp, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_RequestBody)
	require.True(t, ok, "allowed batch must echo a RequestBody response")
	streamed, ok := requestBodyResp.RequestBody.Response.BodyMutation.Mutation.(*extprocv3.BodyMutation_StreamedResponse)
	require.True(t, ok, "allowed batch must use streamed body echo")
	assert.Equal(t, batch, streamed.StreamedResponse.Body)
	assert.Equal(t, []string{"list_files", "read_config"}, seenToolNames)
}

// Spec: A denied batch must aggregate reasons from every denying element into one 403.
func TestServer_OPA_BatchBodyPhase_Deny_AggregatesReasons(t *testing.T) {
	var evaluateCalls int
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			evaluateCalls++
			mcp, ok := input["mcp"].(*authorization.MCPInput)
			require.True(t, ok, "batch element must expose parsed MCP input")
			return &authorization.OPADecision{Action: "deny", Reasons: []string{"denied tool: " + mcp.ToolName}}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "batch-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	batch := []byte(`[{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"delete_repo","arguments":{}}},{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"drop_db","arguments":{}}}]`)
	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, batch)

	require.NotNil(t, bodyResp)
	immResp, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "denied batch must produce ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immResp.ImmediateResponse.Status.Code))
	assert.Equal(t, 2, evaluateCalls)
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "denied tool: delete_repo")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "denied tool: drop_db")
}

func TestApprovalInvocationIDIsSemantic(t *testing.T) {
	for _, tc := range []struct {
		name         string
		id           json.RawMessage
		invocationID string
	}{
		{name: "numeric id", id: json.RawMessage(`42`), invocationID: "42"},
		{name: "string id", id: json.RawMessage(`"call-42"`), invocationID: "call-42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotPrincipal, gotSubjectToken string
			var gotSessionIDs []string
			var gotCreate approval.CreateRequest
			var gotAgentSession, gotMCPSession string
			broker := &mockApprovalBroker{
				readFunc: func(_ context.Context, principal string, sessionIDs []string) ([]approval.Pair, string, error) {
					mu.Lock()
					defer mu.Unlock()
					gotPrincipal = principal
					gotSessionIDs = append([]string(nil), sessionIDs...)
					return nil, "etag-1", nil
				},
				createFunc: func(_ context.Context, subjectToken string, request approval.CreateRequest) (string, error) {
					mu.Lock()
					defer mu.Unlock()
					gotSubjectToken = subjectToken
					gotCreate = request
					return "https://broker.example.com/approvals/approval-1", nil
				},
				consumeFunc: func(context.Context, string, string) error {
					return errors.New("consume must not be called without a matching approval")
				},
			}
			auth := &mockAuthorizer{
				evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
					mu.Lock()
					if contextInput, ok := input["context"].(authorization.ContextInput); ok {
						gotAgentSession = contextInput.AgentSessionID
					}
					if mcpInput, ok := input["mcp"].(*authorization.MCPInput); ok {
						gotMCPSession = mcpInput.SessionID
					}
					mu.Unlock()
					return &authorization.OPADecision{Action: authorization.ActionApprovalRequired, ApprovalContext: &authorization.ApprovalContext{Description: "Review deployment", RiskLevel: "medium"}}, nil
				},
			}
			exchanger := &mockExchanger{
				exchangeFunc: func(_ context.Context, subjectToken, _ string) (server.ExchangeResult, error) {
					assert.Equal(t, "test-metadata-subject-token", subjectToken)
					return server.ExchangeResult{Token: "exchanged-token", Principal: "verified@example.com", AgentID: "canonical-agent-id", GrantedPermissionSets: map[string][]string{}}, nil
				},
			}
			cfg := testConfig()
			cfg.Sessions.Extraction.HTTPHeader = "X-Agent-Session"
			gate := approval.NewGate(approval.NewCache(time.Minute, time.Minute), broker)
			client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, cfg, exchanger, auth, gate)
			defer cleanup()

			body := []byte(`{"jsonrpc":"2.0","method":"tools/call","id":` + string(tc.id) + `,"params":{"name":"deploy","arguments":{"environment":"production"}}}`)
			_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
				":method":         "POST",
				":path":           "http://mcp-server:9003/mcp",
				":authority":      "mcp-server:9003",
				":scheme":         "http",
				"authorization":   "Bearer subject-token",
				"Mcp-Session-Id":  "mcp-session",
				"X-Agent-Session": "agent-session",
			}, body)
			require.NotNil(t, bodyResp)
			immediate, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
			require.True(t, ok)
			assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immediate.ImmediateResponse.Status.Code))

			var response struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Error   struct {
					Code int `json:"code"`
					Data struct {
						Elicitations []struct {
							Mode string `json:"mode"`
							URL  string `json:"url"`
						} `json:"elicitations"`
					} `json:"data"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(immediate.ImmediateResponse.Body, &response))
			assert.Equal(t, "2.0", response.JSONRPC)
			assert.JSONEq(t, string(tc.id), string(response.ID))
			assert.Equal(t, -32042, response.Error.Code)
			require.Len(t, response.Error.Data.Elicitations, 1)
			assert.Equal(t, "url", response.Error.Data.Elicitations[0].Mode)
			assert.Equal(t, "https://broker.example.com/approvals/approval-1", response.Error.Data.Elicitations[0].URL)

			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, "verified@example.com", gotPrincipal)
			assert.Equal(t, []string{"agent-session"}, gotSessionIDs)
			assert.Equal(t, "test-metadata-subject-token", gotSubjectToken)
			assert.Equal(t, "deploy", gotCreate.ToolName)
			assert.Equal(t, map[string]any{"environment": "production"}, gotCreate.Arguments)
			assert.Equal(t, "mcp-session", gotCreate.Metadata.MCPSessionID)
			assert.Equal(t, "agent-session", gotCreate.Metadata.AgentSessionID)
			assert.Equal(t, tc.invocationID, gotCreate.Metadata.InvocationID)
			assert.Equal(t, "Review deployment", gotCreate.Metadata.Description)
			assert.Equal(t, "medium", gotCreate.RiskLevel)
			assert.Equal(t, "agent-session", gotAgentSession)
			assert.Equal(t, "mcp-session", gotMCPSession)
		})
	}
}

func TestServer_OPA_ApprovalRequired_UsesBrokerAuthoritativeIdentityForCacheKey(t *testing.T) {
	permanent := "permanent"
	approvedAt := time.Now()
	cache := approval.NewCache(time.Minute, time.Minute)
	cache.Replace([]approval.Pair{{
		Identity: approval.Identity{Principal: "verified@example.com", AgentID: "canonical-agent-id"},
		Approvals: []approval.Record{{
			ID:            "approval-1",
			ToolPattern:   "deploy",
			ParamsPattern: map[string]string{},
			Status:        "approved",
			Persistence:   &permanent,
			ApprovedAt:    &approvedAt,
		}},
	}}, "etag-1")

	var mu sync.Mutex
	brokerCalls := 0
	broker := &mockApprovalBroker{
		readFunc: func(context.Context, string, []string) ([]approval.Pair, string, error) {
			mu.Lock()
			brokerCalls++
			mu.Unlock()
			return nil, "", errors.New("cache match must not read the broker")
		},
		createFunc: func(context.Context, string, approval.CreateRequest) (string, error) {
			mu.Lock()
			brokerCalls++
			mu.Unlock()
			return "", errors.New("cache match must not create approval")
		},
		consumeFunc: func(context.Context, string, string) error {
			mu.Lock()
			brokerCalls++
			mu.Unlock()
			return errors.New("permanent approval must not be consumed")
		},
	}
	auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
		return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
	}}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "exchanged-token", Principal: "verified@example.com", AgentID: "canonical-agent-id", GrantedPermissionSets: map[string][]string{}}, nil
	}}
	client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, testConfig(), exchanger, auth, approval.NewGate(cache, broker))
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method":       "POST",
		":path":         "http://mcp-server:9003/mcp",
		":authority":    "mcp-server:9003",
		":scheme":       "http",
		"authorization": "Bearer subject-token",
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{}}}`))
	require.NotNil(t, bodyResp)
	_, immediate := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	assert.False(t, immediate, "a cache match keyed by broker identity must proceed")
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, brokerCalls)
}

func TestServer_OPA_ApprovalRequired_WithoutGateDenies(t *testing.T) {
	auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
		return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
	}}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "exchanged-token", GrantedPermissionSets: map[string][]string{}}, nil
	}}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method":       "POST",
		":path":         "http://mcp-server:9003/mcp",
		":authority":    "mcp-server:9003",
		":scheme":       "http",
		"authorization": "Bearer subject-token",
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{}}}`))
	require.NotNil(t, bodyResp)
	immediate, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok)
	assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immediate.ImmediateResponse.Status.Code))
	assert.Contains(t, string(immediate.ImmediateResponse.Body), "approval_required is not yet supported")
}

func TestServer_OPA_ApprovalGateIsolatedFromNonApprovalActionsAndBatches(t *testing.T) {
	for _, tc := range []struct {
		name            string
		action          string
		body            []byte
		expectImmediate bool
		wantReason      string
	}{
		{name: "allow", action: authorization.ActionAllow, body: []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"list","arguments":{}}}`)},
		{name: "deny", action: authorization.ActionDeny, body: []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"delete","arguments":{}}}`), expectImmediate: true, wantReason: "denied by policy"},
		{name: "ciba", action: authorization.ActionCIBARequired, body: []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"ciba","arguments":{}}}`), expectImmediate: true, wantReason: "ciba is deferred"},
		{name: "batch", action: authorization.ActionApprovalRequired, body: []byte(`[{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{}}}]`), expectImmediate: true, wantReason: "re-issued as standalone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			brokerCalls := 0
			broker := &mockApprovalBroker{
				readFunc: func(context.Context, string, []string) ([]approval.Pair, string, error) {
					mu.Lock()
					brokerCalls++
					mu.Unlock()
					return nil, "", errors.New("approval gate must not read the broker")
				},
				createFunc: func(context.Context, string, approval.CreateRequest) (string, error) {
					mu.Lock()
					brokerCalls++
					mu.Unlock()
					return "", errors.New("approval gate must not create approval")
				},
				consumeFunc: func(context.Context, string, string) error {
					mu.Lock()
					brokerCalls++
					mu.Unlock()
					return errors.New("approval gate must not consume approval")
				},
			}
			auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
				return &authorization.OPADecision{Action: tc.action, Reasons: []string{tc.wantReason}}, nil
			}}
			exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
				return server.ExchangeResult{Token: "exchanged-token", Principal: "verified@example.com", AgentID: "canonical-agent-id", GrantedPermissionSets: map[string][]string{}}, nil
			}}
			client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, testConfig(), exchanger, auth, approval.NewGate(approval.NewCache(time.Minute, time.Minute), broker))
			defer cleanup()

			_, bodyResp := sendHeadersThenBody(t, client, map[string]string{
				":method":       "POST",
				":path":         "http://mcp-server:9003/mcp",
				":authority":    "mcp-server:9003",
				":scheme":       "http",
				"authorization": "Bearer subject-token",
			}, tc.body)
			require.NotNil(t, bodyResp)
			_, immediate := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
			assert.Equal(t, tc.expectImmediate, immediate)
			if tc.wantReason != "" {
				assert.Contains(t, string(bodyResp.GetImmediateResponse().GetBody()), tc.wantReason)
			}
			mu.Lock()
			defer mu.Unlock()
			assert.Zero(t, brokerCalls)
		})
	}
}

func TestServer_OPA_ApprovalGateOnlyHandlesStandaloneMCPToolCalls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol string
		body     []byte
	}{
		{name: "MCP method", protocol: "mcp", body: []byte(`{"jsonrpc":"2.0","method":"initialize","id":1,"params":{}}`)},
		{name: "non MCP tool call", protocol: "a2a", body: []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{}}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			brokerCalls := 0
			broker := &mockApprovalBroker{
				readFunc: func(context.Context, string, []string) ([]approval.Pair, string, error) {
					mu.Lock()
					brokerCalls++
					mu.Unlock()
					return nil, "", nil
				},
				createFunc: func(context.Context, string, approval.CreateRequest) (string, error) {
					mu.Lock()
					brokerCalls++
					mu.Unlock()
					return "https://broker.example.com/approvals/approval-1", nil
				},
				consumeFunc: func(context.Context, string, string) error { return nil },
			}
			auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
				return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
			}}
			exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
				return server.ExchangeResult{Token: "exchanged-token", Principal: "verified@example.com", AgentID: "canonical-agent-id", GrantedPermissionSets: map[string][]string{}}, nil
			}}
			client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, testConfig(), exchanger, auth, approval.NewGate(approval.NewCache(time.Minute, time.Minute), broker))
			defer cleanup()

			_, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
				":method":       "POST",
				":path":         "http://mcp-server:9003/mcp",
				":authority":    "mcp-server:9003",
				":scheme":       "http",
				"authorization": "Bearer subject-token",
			}, tc.body, tc.protocol)
			require.NotNil(t, bodyResp)
			immediate, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
			require.True(t, ok)
			assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immediate.ImmediateResponse.Status.Code))
			mu.Lock()
			defer mu.Unlock()
			assert.Zero(t, brokerCalls)
		})
	}
}

func TestServer_OPA_ApprovalGateDoesNotHandleHeaderOnlyRequests(t *testing.T) {
	var mu sync.Mutex
	brokerCalls := 0
	broker := &mockApprovalBroker{
		readFunc: func(context.Context, string, []string) ([]approval.Pair, string, error) {
			mu.Lock()
			brokerCalls++
			mu.Unlock()
			return nil, "", nil
		},
		createFunc: func(context.Context, string, approval.CreateRequest) (string, error) {
			mu.Lock()
			brokerCalls++
			mu.Unlock()
			return "https://broker.example.com/approvals/approval-1", nil
		},
		consumeFunc: func(context.Context, string, string) error { return nil },
	}
	auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
		return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
	}}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "exchanged-token"}, nil
	}}
	client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, testConfig(), exchanger, auth, approval.NewGate(approval.NewCache(time.Minute, time.Minute), broker))
	defer cleanup()

	response, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method":       "GET",
		":path":         "http://mcp-server:9003/mcp",
		":authority":    "mcp-server:9003",
		":scheme":       "http",
		"authorization": "Bearer subject-token",
	}, "mcp")
	require.NoError(t, err)
	immediate, ok := response.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok)
	assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immediate.ImmediateResponse.Status.Code))
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, brokerCalls)
}

// Spec: Exchange fails with re-auth for a batch request → elicitation is returned in the
// headers-phase response. Token exchange occurs eagerly in the headers phase (before the
// batch body is seen), so the elicitation carries id=null regardless of batch element ids.
func TestServer_OPA_BatchBodyPhaseReauth_ReturnsElicitationInHeadersPhase(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	reAuthErr := &server.BrokerExchangeError{
		StatusCode:  401,
		Code:        "reauth_required",
		Description: "session expired",
		ErrorURI:    "https://idp.example.com/reauth",
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, reAuthErr
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	batch := []byte(`[{"jsonrpc":"2.0","method":"tools/call","id":7,"params":{"name":"a"}},{"jsonrpc":"2.0","method":"tools/call","id":8,"params":{"name":"b"}}]`)
	headersResp, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, batch)

	// Exchange failed in headers phase → elicitation is in headersResp, no body phase.
	assert.Nil(t, bodyResp, "re-auth in headers phase must not produce a body-phase response")
	immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "batch re-auth must produce an ImmediateResponse in the headers phase")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code))
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "idp.example.com/reauth")

	var rpcErr map[string]any
	require.NoError(t, json.Unmarshal(immResp.ImmediateResponse.Body, &rpcErr))
	assert.Nil(t, rpcErr["id"], "headers-phase re-auth elicitation carries id=null (batch not yet seen)")
}

// Spec: Exchange fails with re-auth for a batch starting with a notification → elicitation
// is returned in the headers-phase response with id=null. Token exchange occurs eagerly in
// the headers phase, so the batch body (and its element ids) is never seen.
func TestServer_OPA_BatchBodyPhaseReauth_NotificationFirstUsesNextID(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	reAuthErr := &server.BrokerExchangeError{
		StatusCode:  401,
		Code:        "reauth_required",
		Description: "session expired",
		ErrorURI:    "https://idp.example.com/reauth",
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, reAuthErr
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	// First element is a notification (no id field); second carries id=42.
	batch := []byte(`[{"jsonrpc":"2.0","method":"notifications/progress"},{"jsonrpc":"2.0","method":"tools/call","id":42,"params":{"name":"a"}}]`)
	headersResp, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, batch)

	// Exchange failed in headers phase → elicitation is in headersResp, no body phase.
	assert.Nil(t, bodyResp, "re-auth in headers phase must not produce a body-phase response")
	immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "batch re-auth must produce an ImmediateResponse in the headers phase")
	assert.Equal(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code))

	var rpcErr map[string]any
	require.NoError(t, json.Unmarshal(immResp.ImmediateResponse.Body, &rpcErr))
	assert.Nil(t, rpcErr["id"], "headers-phase re-auth elicitation carries id=null (batch not yet seen)")
}

// Spec: A transient 5xx broker error with error_uri on an MCP batch request must NOT
// trigger re-auth elicitation — it must return a normal exchange failure (500).
// Token exchange occurs eagerly in the headers phase, so the 500 is returned in the
// headers-phase response (not the body phase).
func TestServer_OPA_BatchBodyPhase_5xxWithErrorURI_Returns500NotElicitation(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode: 500,
				Code:       "server_error",
				ErrorURI:   "https://idp.example.com/reauth",
			}
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	batch := []byte(`[{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"a"}}]`)
	headersResp, bodyResp := sendHeadersThenBody(t, client, map[string]string{
		":method": "POST",
	}, batch)

	// Exchange failed in headers phase → error is in headersResp, no body phase.
	assert.Nil(t, bodyResp, "5xx exchange error in headers phase must not produce a body-phase response")
	immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "5xx with error_uri on batch must produce an ImmediateResponse in the headers phase")
	assert.NotEqual(t, int32(httpv3.StatusCode_OK), int32(immResp.ImmediateResponse.Status.Code),
		"transient 5xx with error_uri must not return elicitation (HTTP 200)")
	assert.Equal(t, int32(httpv3.StatusCode_InternalServerError), int32(immResp.ImmediateResponse.Status.Code),
		"transient 5xx with error_uri must return 500, not elicitation")
}

// Spec: re-auth returns elicitation in the headers phase with id=null. Token exchange
// occurs eagerly before the body is seen, so the request body (including its id field)
// is never inspected; id is always null in headers-phase elicitation responses.
func TestServer_OPA_BodyPhaseReauth_InvalidIDTypesFallBackToNull(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	reAuthErr := &server.BrokerExchangeError{
		StatusCode:  401,
		Code:        "reauth_required",
		Description: "session expired",
		ErrorURI:    "https://idp.example.com/reauth",
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, reAuthErr
		},
	}

	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"object id", []byte(`{"jsonrpc":"2.0","method":"tools/call","id":{},"params":{"name":"a"}}`)},
		{"array id", []byte(`{"jsonrpc":"2.0","method":"tools/call","id":[],"params":{"name":"a"}}`)},
		{"boolean true id", []byte(`{"jsonrpc":"2.0","method":"tools/call","id":true,"params":{"name":"a"}}`)},
		{"boolean false id", []byte(`{"jsonrpc":"2.0","method":"tools/call","id":false,"params":{"name":"a"}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
			defer cleanup()

			headersResp, bodyResp := sendHeadersThenBody(t, client, map[string]string{
				":method": "POST",
			}, tc.body)

			// Exchange failed in headers phase → elicitation is in headersResp, no body phase.
			assert.Nil(t, bodyResp)
			immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
			require.True(t, ok)

			var rpcErr map[string]any
			require.NoError(t, json.Unmarshal(immResp.ImmediateResponse.Body, &rpcErr))
			assert.Nil(t, rpcErr["id"], "headers-phase re-auth elicitation carries id=null")
		})
	}
}

// Spec: When OPA allows a non-MCP request and Exchange returns error_uri,
// the response must NOT be a URLElicitationRequiredError (MCP-specific).
// It should fall through to the generic token-exchange error path.
func TestServer_OPA_NonMCPBodyPhaseReauth_DoesNotReturnElicitation(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	reAuthErr := &server.BrokerExchangeError{
		StatusCode:  401,
		Code:        "reauth_required",
		Description: "session expired",
		ErrorURI:    "https://idp.example.com/reauth",
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, reAuthErr
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	// Send as "a2a" protocol (non-MCP) — Exchange returns error_uri in headers phase.
	headersResp, bodyResp := sendHeadersThenBodyWithProtocol(t, client, nil, []byte(`{"some":"a2a-body"}`), "a2a")

	// Exchange failed in headers phase → error is in headersResp, no body phase.
	assert.Nil(t, bodyResp, "non-MCP re-auth in headers phase must not produce a body-phase response")
	immResp, ok := headersResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "non-MCP re-auth must produce an ImmediateResponse in the headers phase")
	assert.Equal(t, int32(httpv3.StatusCode_ServiceUnavailable), int32(immResp.ImmediateResponse.Status.Code),
		"non-MCP re-auth must return 503")
	assert.Contains(t, string(immResp.ImmediateResponse.Body), "service_unavailable",
		"non-MCP re-auth body must contain service_unavailable")
}

// Spec: BrokerExchangeError without ErrorURI still returns 500 (no elicitation).
func TestServer_Process_BrokerErrorWithoutURI_Returns500(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, &server.BrokerExchangeError{
				StatusCode:  403,
				Code:        "access_denied",
				Description: "agent does not have a grant",
				// ErrorURI intentionally empty
			}
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "broker error without ErrorURI must produce an ImmediateResponse directly")
	assert.Equal(t, int32(httpv3.StatusCode_InternalServerError),
		int32(immResp.ImmediateResponse.Status.Code))
}

// ---------------------------------------------------------------------------
// T021: Span creation in processRequestHeaders
// ---------------------------------------------------------------------------

// Spec: US1 S2 — When traces are enabled, processRequestHeaders creates a span
// with resource.uri and outcome attributes set correctly
func TestServer_ProcessRequestHeaders_CreatesSpanWithAttributes(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
	defer cleanup()

	_, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	tp.ForceFlush(context.Background()) //nolint:errcheck
	spans := spanRecorder.Ended()
	require.NotEmpty(t, spans, "at least one span must be recorded")

	var found bool
	for _, s := range spans {
		if s.Name() == "extproc.token_exchange" {
			found = true
			attrs := attributeMap(s.Attributes())
			assert.Equal(t, testResourceURI, attrs["resource.uri"], "resource.uri must be set")
			assert.Equal(t, "success", attrs["outcome"], "outcome must be 'success'")
			break
		}
	}
	assert.True(t, found, "span 'extproc.token_exchange' must exist")
}

// Spec: US1 S2 — Span outcome attribute must be set based on exchange result
func TestServer_ProcessRequestHeaders_SpanOutcomeOnSuccess(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "success-token"}, nil
		},
	}
	client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
	defer cleanup()

	_, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	tp.ForceFlush(context.Background()) //nolint:errcheck
	spans := spanRecorder.Ended()

	var outcomeAttr string
	for _, s := range spans {
		if s.Name() == "extproc.token_exchange" {
			outcomeAttr = attributeMap(s.Attributes())["outcome"]
		}
	}
	assert.Equal(t, "success", outcomeAttr, "span outcome must be 'success' on successful exchange")
}

// Spec: US1 S2 — Span outcome attribute must be set on exchange_failure
func TestServer_ProcessRequestHeaders_SpanOutcomeOnFailure(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, errors.New("token exchange failed")
		},
	}
	client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
	defer cleanup()

	_, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	tp.ForceFlush(context.Background()) //nolint:errcheck
	spans := spanRecorder.Ended()

	var outcomeAttr string
	for _, s := range spans {
		if s.Name() == "extproc.token_exchange" {
			outcomeAttr = attributeMap(s.Attributes())["outcome"]
		}
	}
	assert.Equal(t, "exchange_failure", outcomeAttr, "span outcome must be 'exchange_failure' on error")
}

// Spec: US3 S1 — Metrics are recorded with outcome attribute
func TestServer_ProcessRequestHeaders_MetricsRecordOutcome(t *testing.T) {
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prevMP) })

	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
	defer cleanup()

	_, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &rm))

	var foundCounterWithOutcome, foundHistogramWithOutcome bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "extproc.token_exchange.requests":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "success" {
								foundCounterWithOutcome = true
							}
						}
					}
				}
			case "extproc.token_exchange.duration":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					for _, dp := range hist.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "success" {
								foundHistogramWithOutcome = true
							}
						}
					}
				}
			}
		}
	}
	assert.True(t, foundCounterWithOutcome, "counter must have outcome=success attribute")
	assert.True(t, foundHistogramWithOutcome, "histogram must have outcome=success attribute")
}

// Regression test: metrics must still be recorded when the gRPC stream context
// is cancelled (client disconnect) because the deferred closure uses
// context.WithoutCancel. Without that guard, the OTel SDK receives a cancelled
// context which could silently drop metric data points.
func TestServer_ProcessRequestHeaders_MetricsRecordedWhenStreamCancelled(t *testing.T) {
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prevMP) })

	exchangeStarted := make(chan struct{})
	exchangeContinue := make(chan struct{})

	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			close(exchangeStarted)
			<-exchangeContinue
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithConfig(t, testConfigWithTelemetry(), exchanger)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Process(ctx)
	require.NoError(t, err)

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata(""),
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(nil, false),
		},
	})
	require.NoError(t, err)
	_ = stream.CloseSend()

	// Wait for exchange to start, cancel stream context (simulate client disconnect),
	// then let the exchange complete.
	<-exchangeStarted
	cancel()
	close(exchangeContinue)

	// Poll until metrics appear — avoids flaky time.Sleep on slow CI runners.
	var foundCounter, foundHistogram bool
	require.Eventually(t, func() bool {
		var rm metricdata.ResourceMetrics
		if err := metricReader.Collect(context.Background(), &rm); err != nil {
			return false
		}
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				switch m.Name {
				case "extproc.token_exchange.requests":
					if sum, ok := m.Data.(metricdata.Sum[int64]); ok && len(sum.DataPoints) > 0 {
						foundCounter = true
					}
				case "extproc.token_exchange.duration":
					if hist, ok := m.Data.(metricdata.Histogram[float64]); ok && len(hist.DataPoints) > 0 {
						foundHistogram = true
					}
				}
			}
		}
		return foundCounter && foundHistogram
	}, 2*time.Second, 10*time.Millisecond,
		"counter and histogram must be recorded even when stream context is cancelled")
}

func TestServer_OPA_BodyBearing_EmitsTelemetryAndPropagatesTrace(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })
	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prevMP) })

	const traceparentHeader = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	const expectedTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	var seenTraceID string

	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(ctx context.Context, _, _ string) (server.ExchangeResult, error) {
			seenTraceID = trace.SpanContextFromContext(ctx).TraceID().String()
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfig(t, testConfigWithTelemetry(), exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
		":method":     "POST",
		"traceparent": traceparentHeader,
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`), "mcp")
	require.NotNil(t, bodyResp)
	_, isImmediate := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	assert.False(t, isImmediate, "allow path should not return ImmediateResponse")

	tp.ForceFlush(context.Background()) //nolint:errcheck
	spans := spanRecorder.Ended()
	require.NotEmpty(t, spans, "OPA exchange path must emit a span")
	assert.Equal(t, expectedTraceID, seenTraceID, "exchange call should receive trace context extracted from request headers")

	var foundSpan bool
	for _, s := range spans {
		if s.Name() == "extproc.token_exchange" {
			foundSpan = true
			attrs := attributeMap(s.Attributes())
			assert.Equal(t, testResourceURI, attrs["resource.uri"])
			assert.Equal(t, "success", attrs["outcome"])
		}
	}
	assert.True(t, foundSpan, "OPA exchange path must emit extproc.token_exchange span")

	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &rm))
	var foundCounterWithOutcome, foundHistogramWithOutcome bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "extproc.token_exchange.requests":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "success" {
								foundCounterWithOutcome = true
							}
						}
					}
				}
			case "extproc.token_exchange.duration":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					for _, dp := range hist.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "success" {
								foundHistogramWithOutcome = true
							}
						}
					}
				}
			}
		}
	}
	assert.True(t, foundCounterWithOutcome, "OPA exchange path must record counter with outcome=success")
	assert.True(t, foundHistogramWithOutcome, "OPA exchange path must record histogram with outcome=success")
}

func TestServer_OPA_HeadersOnly_EmitsTelemetryAndPropagatesTrace(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prevMP) })

	const traceparentHeader = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	const expectedTraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var seenTraceID string

	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(ctx context.Context, _, _ string) (server.ExchangeResult, error) {
			seenTraceID = trace.SpanContextFromContext(ctx).TraceID().String()
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfig(t, testConfigWithTelemetry(), exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method":     "GET",
		"traceparent": traceparentHeader,
	}, "mcp")
	require.NoError(t, err)
	require.NotNil(t, resp)

	tp.ForceFlush(context.Background()) //nolint:errcheck
	spans := spanRecorder.Ended()
	require.NotEmpty(t, spans, "OPA header-only exchange path must emit a span")
	assert.Equal(t, expectedTraceID, seenTraceID, "header-only exchange should receive trace context extracted from request headers")

	var foundSpan bool
	for _, s := range spans {
		if s.Name() == "extproc.token_exchange" {
			foundSpan = true
			attrs := attributeMap(s.Attributes())
			assert.Equal(t, testResourceURI, attrs["resource.uri"])
			assert.Equal(t, "success", attrs["outcome"])
		}
	}
	assert.True(t, foundSpan, "OPA header-only exchange path must emit extproc.token_exchange span")

	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &rm))
	var foundCounterWithOutcome, foundHistogramWithOutcome bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "extproc.token_exchange.requests":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "success" {
								foundCounterWithOutcome = true
							}
						}
					}
				}
			case "extproc.token_exchange.duration":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					for _, dp := range hist.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "success" {
								foundHistogramWithOutcome = true
							}
						}
					}
				}
			}
		}
	}
	assert.True(t, foundCounterWithOutcome, "OPA header-only path must record counter with outcome=success")
	assert.True(t, foundHistogramWithOutcome, "OPA header-only path must record histogram with outcome=success")
}

func TestServer_OPA_HeadersOnly_Deny_RecordsAuthorizationOutcome(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prevMP) })

	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "deny", Reasons: []string{"tool denied"}}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			t.Fatal("Exchange must not be called when header-only OPA denies")
			return server.ExchangeResult{}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfig(t, testConfigWithTelemetry(), exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method": "GET",
	}, "mcp")
	require.NoError(t, err)
	require.NotNil(t, resp)

	tp.ForceFlush(context.Background()) //nolint:errcheck
	spans := spanRecorder.Ended()
	require.NotEmpty(t, spans, "OPA header-only deny path must emit a span")

	var foundSpan bool
	for _, s := range spans {
		if s.Name() == "extproc.token_exchange" {
			foundSpan = true
			attrs := attributeMap(s.Attributes())
			assert.Equal(t, testResourceURI, attrs["resource.uri"])
			assert.Equal(t, "authorization_denied", attrs["outcome"])
			assert.Equal(t, "access_denied", attrs["error.type"])
		}
	}
	assert.True(t, foundSpan, "OPA header-only deny path must emit extproc.token_exchange span")

	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &rm))
	var foundCounterWithOutcome, foundHistogramWithOutcome bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "extproc.token_exchange.requests":
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "authorization_denied" {
								foundCounterWithOutcome = true
							}
						}
					}
				}
			case "extproc.token_exchange.duration":
				if hist, ok := m.Data.(metricdata.Histogram[float64]); ok {
					for _, dp := range hist.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if string(attr.Key) == "outcome" && attr.Value.AsString() == "authorization_denied" {
								foundHistogramWithOutcome = true
							}
						}
					}
				}
			}
		}
	}
	assert.True(t, foundCounterWithOutcome, "OPA header-only deny path must record counter with outcome=authorization_denied")
	assert.True(t, foundHistogramWithOutcome, "OPA header-only deny path must record histogram with outcome=authorization_denied")
}

// Spec: FR-014 — OPA-disabled requests must log the propagated trace_id with anonymous actor semantics.
func TestServer_RequestContext_DirectPath_LogsPropagatedTraceIDAndAnonymousActor(t *testing.T) {
	const traceparentHeader = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	const expectedTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	logger, capture := newJSONTestLogger()
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}

	client, cleanup := startTestServerWithConfigAndLogger(t, testConfigWithTelemetry(), exchanger, logger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, map[string]string{
		"traceparent": traceparentHeader,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	record := requireLogRecord(t, capture, "token exchanged successfully")
	traceID, ok := record["trace_id"].(string)
	require.True(t, ok, "expected trace_id field in log record: %#v", record)
	assert.Equal(t, expectedTraceID, traceID)

	actor, ok := record["actor"].(string)
	require.True(t, ok, "expected actor field in log record: %#v", record)
	assert.Equal(t, "anonymous", actor)

	_, hasCallingPeer := record["calling_peer"]
	assert.False(t, hasCallingPeer, "calling_peer should be omitted when unavailable")
}

func TestServer_RequestContext_DirectPath_ReusesInboundTraceWhenTracingDisabled(t *testing.T) {
	const inboundTraceID = "55555555555555555555555555555555"

	previousPropagator := otel.GetTextMapPropagator()
	previousProvider := otel.GetTracerProvider()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	cfg := testConfigWithTelemetry()
	cfg.Telemetry.Traces.Enabled = false
	logger, capture := newJSONTestLogger()
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithConfigAndLogger(t, cfg, exchanger, logger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, map[string]string{
		"traceparent": "00-" + inboundTraceID + "-bbbbbbbbbbbbbbbb-01",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	record := requireLogRecord(t, capture, "token exchanged successfully")
	traceID, ok := record["trace_id"].(string)
	require.True(t, ok)
	assert.Equal(t, inboundTraceID, traceID)
	assert.Regexp(t, regexp.MustCompile("^[0-9a-f]{32}$"), traceID)
}

// Spec: FR-014 — OPA body-bearing requests must generate a trace_id when none is supplied.
func TestServer_RequestContext_OPABodyBearingPath_LogsGeneratedTraceIDAndAnonymousActor(t *testing.T) {
	logger, capture := newJSONTestLogger()
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}

	client, cleanup := startTestServerWithAuthorizerConfigAndLogger(t, testConfig(), exchanger, auth, logger)
	defer cleanup()

	headersResp, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
		":method": "POST",
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`), "mcp")
	require.NotNil(t, headersResp)
	require.NotNil(t, bodyResp)

	record := requireLogRecord(t, capture, "OPA: token exchanged in headers phase, buffering body for OPA evaluation")
	traceID, ok := record["trace_id"].(string)
	require.True(t, ok, "expected trace_id field in log record: %#v", record)
	assert.Regexp(t, regexp.MustCompile("^[0-9a-f]{32}$"), traceID)

	actor, ok := record["actor"].(string)
	require.True(t, ok, "expected actor field in log record: %#v", record)
	assert.Equal(t, "anonymous", actor)

	_, hasCallingPeer := record["calling_peer"]
	assert.False(t, hasCallingPeer, "calling_peer should be omitted when unavailable")
}

// Spec: FR-014 — OPA header-only requests must preserve propagated trace_id and omit calling_peer.
func TestServer_RequestContext_OPAHeadersOnlyPath_LogsPropagatedTraceIDAndAnonymousActor(t *testing.T) {
	const traceparentHeader = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	const expectedTraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	logger, capture := newJSONTestLogger()
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}

	client, cleanup := startTestServerWithAuthorizerConfigAndLogger(t, testConfigWithTelemetry(), exchanger, auth, logger)
	defer cleanup()

	resp, err := sendRequestHeadersWithProtocolEOS(t, client, map[string]string{
		":method":     "GET",
		"traceparent": traceparentHeader,
	}, "mcp")
	require.NoError(t, err)
	require.NotNil(t, resp)

	record := requireLogRecord(t, capture, "OPA allowed header-only request, token exchanged successfully")
	traceID, ok := record["trace_id"].(string)
	require.True(t, ok, "expected trace_id field in log record: %#v", record)
	assert.Equal(t, expectedTraceID, traceID)

	actor, ok := record["actor"].(string)
	require.True(t, ok, "expected actor field in log record: %#v", record)
	assert.Equal(t, "anonymous", actor)

	_, hasCallingPeer := record["calling_peer"]
	assert.False(t, hasCallingPeer, "calling_peer should be omitted when unavailable")
}

// Spec: FR-014 — a direct-path exchange failure must still log the propagated trace_id.
func TestServer_RequestContext_DirectPath_ExchangeFailureLogsTraceID(t *testing.T) {
	const traceparentHeader = "00-99999999999999999999999999999999-1111111111111111-01"
	const expectedTraceID = "99999999999999999999999999999999"

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	logger, capture := newJSONTestLogger()
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, errors.New("broker unreachable")
		},
	}
	client, cleanup := startTestServerWithConfigAndLogger(t, testConfigWithTelemetry(), exchanger, logger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, map[string]string{
		"traceparent": traceparentHeader,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	record := requireLogRecord(t, capture, "token exchange failed")
	assert.Equal(t, expectedTraceID, record["trace_id"], "exchange-failure log must retain the propagated trace_id")
	assert.Equal(t, "anonymous", record["actor"])
}

// Spec: FR-014 — an OPA deny must still log the propagated trace_id.
func TestServer_RequestContext_OPADeny_LogsTraceID(t *testing.T) {
	const traceparentHeader = "00-88888888888888888888888888888888-2222222222222222-01"
	const expectedTraceID = "88888888888888888888888888888888"

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	logger, capture := newJSONTestLogger()
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "deny", Reasons: []string{"policy forbids tool"}}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfigAndLogger(t, testConfigWithTelemetry(), exchanger, auth, logger)
	defer cleanup()

	_, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
		":method":     "POST",
		"traceparent": traceparentHeader,
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`), "mcp")
	require.NotNil(t, bodyResp)

	record := requireLogRecord(t, capture, "OPA denied request")
	assert.Equal(t, expectedTraceID, record["trace_id"], "OPA-deny log must retain the propagated trace_id")
	assert.Equal(t, "anonymous", record["actor"])
}

// Spec: FR-014 — an OPA evaluation error must still log the propagated trace_id.
func TestServer_RequestContext_OPAEvaluationError_LogsTraceID(t *testing.T) {
	const traceparentHeader = "00-77777777777777777777777777777777-3333333333333333-01"
	const expectedTraceID = "77777777777777777777777777777777"

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	logger, capture := newJSONTestLogger()
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return nil, errors.New("rego runtime error")
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfigAndLogger(t, testConfigWithTelemetry(), exchanger, auth, logger)
	defer cleanup()

	_, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
		":method":     "POST",
		"traceparent": traceparentHeader,
	}, []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_files"}}`), "mcp")
	require.NotNil(t, bodyResp)

	record := requireLogRecord(t, capture, "OPA evaluation error — denying")
	assert.Equal(t, expectedTraceID, record["trace_id"], "OPA-evaluation-error log must retain the propagated trace_id")
	assert.Equal(t, "anonymous", record["actor"])
}

// Spec: FR-014 — a request whose body cannot be parsed for OPA must still log the propagated trace_id.
func TestServer_RequestContext_OPAInvalidInput_LogsTraceID(t *testing.T) {
	const traceparentHeader = "00-66666666666666666666666666666666-4444444444444444-01"
	const expectedTraceID = "66666666666666666666666666666666"

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	logger, capture := newJSONTestLogger()
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizerConfigAndLogger(t, testConfigWithTelemetry(), exchanger, auth, logger)
	defer cleanup()

	_, bodyResp := sendHeadersThenBodyWithProtocol(t, client, map[string]string{
		":method":     "POST",
		"traceparent": traceparentHeader,
	}, []byte(`this is not valid json-rpc`), "mcp")
	require.NotNil(t, bodyResp)

	record := requireLogRecord(t, capture, "OPA: failed to parse request body — denying")
	assert.Equal(t, expectedTraceID, record["trace_id"], "invalid-input deny log must retain the propagated trace_id")
	assert.Equal(t, "anonymous", record["actor"])
}

// attributeMap converts a slice of key-value attributes to a map for easy assertions.
func attributeMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.String()
	}
	return m
}

type failingProcessStream struct {
	ctx      context.Context
	requests []*extprocv3.ProcessingRequest
	recvErr  error
	sendErr  error
	next     int
}

func (s *failingProcessStream) Send(*extprocv3.ProcessingResponse) error {
	return s.sendErr
}

func (s *failingProcessStream) Recv() (*extprocv3.ProcessingRequest, error) {
	if s.next < len(s.requests) {
		req := s.requests[s.next]
		s.next++
		return req, nil
	}
	return nil, s.recvErr
}

func (s *failingProcessStream) SetHeader(metadata.MD) error { return nil }

func (s *failingProcessStream) SendHeader(metadata.MD) error { return nil }

func (s *failingProcessStream) SetTrailer(metadata.MD) {}

func (s *failingProcessStream) Context() context.Context { return s.ctx }

func (s *failingProcessStream) SendMsg(any) error { return nil }

func (s *failingProcessStream) RecvMsg(any) error { return nil }

func TestServer_Process_TransportFailuresRetainRequestContext(t *testing.T) {
	request := &extprocv3.ProcessingRequest{
		MetadataContext: validTokenExchangeMetadata(""),
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(nil, false),
		},
	}
	tests := []struct {
		name    string
		message string
		stream  *failingProcessStream
	}{
		{
			name:    "send failure",
			message: "stream send error",
			stream: &failingProcessStream{
				ctx:      context.Background(),
				requests: []*extprocv3.ProcessingRequest{request},
				sendErr:  errors.New("send failed"),
			},
		},
		{
			name:    "receive failure after response",
			message: "stream recv error",
			stream: &failingProcessStream{
				ctx:      context.Background(),
				requests: []*extprocv3.ProcessingRequest{request},
				recvErr:  errors.New("receive failed"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, capture := newJSONTestLogger()
			srv := server.NewServer(testConfig(), &mockExchanger{
				exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
					return server.ExchangeResult{Token: "exchanged-token"}, nil
				},
			}, logger)

			err := srv.Process(tt.stream)
			require.Error(t, err)

			record := requireLogRecord(t, capture, tt.message)
			assert.Regexp(t, regexp.MustCompile("^[0-9a-f]{32}$"), record["trace_id"])
			assert.Equal(t, "anonymous", record["actor"])
		})
	}
}

// tokenExchangeMetadataWithMCPServer builds valid token-exchange metadata for the "mcp"
// protocol, additionally setting agentgateway's mcp_server field. This is agentgateway
// routing metadata, not an MCP protocol field (spec 044).
func tokenExchangeMetadataWithMCPServer(mcpServer string) *corev3.Metadata {
	metadata := tokenExchangeMetadataWithProtocol(tokenExchangeMetadataFields(
		structpb.NewStringValue(testSubjectToken),
		structpb.NewStringValue(testResourceURI),
	), "mcp")
	metadata.FilterMetadata["agentgateway"].Fields["mcp_server"] = structpb.NewStringValue(mcpServer)
	return metadata
}

// Spec 044: agentgateway's mcp_server metadata propagates to input.mcp.target_server_name
// in the body phase.
func TestServer_OPA_BodyPhase_MCPServerMetadata_PropagatedToOPAInput(t *testing.T) {
	var seenTargetServerName string
	var sawMCPInput bool
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			mcp, ok := input["mcp"].(*authorization.MCPInput)
			require.True(t, ok, "body-phase input must expose parsed MCP input")
			sawMCPInput = true
			seenTargetServerName = mcp.TargetServerName
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBodyWithMetadata(t, client, map[string]string{
		":method": "POST",
	}, tokenExchangeMetadataWithMCPServer("github-mcp"),
		[]byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"list_files","arguments":{}}}`))

	require.NotNil(t, bodyResp)
	_, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_RequestBody)
	require.True(t, ok, "allowed request must echo a RequestBody response")
	assert.True(t, sawMCPInput)
	assert.Equal(t, "github-mcp", seenTargetServerName)
}

// Spec 044 FR-004: absent mcp_server metadata for an MCP-protocol request MUST be
// rejected with 403, mirroring how absent protocol metadata is rejected (spec 020
// FR-003). Neither the authorizer nor the exchanger must be invoked.
func TestServer_OPA_AbsentMCPServerMetadata_Returns403(t *testing.T) {
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, _ authorization.OPAInput) (*authorization.OPADecision, error) {
			t.Fatal("Evaluate must not be called when mcp_server metadata is absent")
			return nil, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			t.Fatal("Exchange must not be called when mcp_server metadata is absent")
			return server.ExchangeResult{}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	stream, err := client.Process(context.Background())
	require.NoError(t, err)

	// Deliberately built without tokenExchangeMetadataWithProtocol's default
	// mcp_server value, to exercise the true-absent case.
	metadata := tokenExchangeMetadata(tokenExchangeMetadataFields(
		structpb.NewStringValue(testSubjectToken),
		structpb.NewStringValue(testResourceURI),
	))
	metadata.FilterMetadata["agentgateway"] = &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"protocol": structpb.NewStringValue("mcp"),
		},
	}

	err = stream.Send(&extprocv3.ProcessingRequest{
		MetadataContext: metadata,
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: extProcHeaders(map[string]string{":method": "POST"}, false),
		},
	})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "absent mcp_server metadata must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immResp.ImmediateResponse.Status.Code))
}

// Spec 044: agentgateway's mcp_server metadata propagates to input.mcp.target_server_name
// for header-only requests too.
func TestServer_OPA_HeadersOnly_MCPServerMetadata_PropagatedToOPAInput(t *testing.T) {
	var seenTargetServerName string
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			mcp, ok := input["mcp"].(*authorization.MCPInput)
			require.True(t, ok, "header-only input must expose parsed MCP input")
			seenTargetServerName = mcp.TargetServerName
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "exchanged-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	resp, err := sendRequestHeadersWithMetadata(t, client, map[string]string{
		":method": "GET",
	}, tokenExchangeMetadataWithMCPServer("github-mcp"), true)
	require.NoError(t, err)

	_, ok := resp.Response.(*extprocv3.ProcessingResponse_RequestHeaders)
	require.True(t, ok, "allowed header-only request must produce a RequestHeaders response")
	assert.Equal(t, "github-mcp", seenTargetServerName)
}

// Spec 044 Edge Case (batch requests): every element in a JSON-RPC batch must receive
// the identical mcp.target_server_name value, sourced once from the agentgateway
// metadata at the headers phase.
func TestServer_OPA_BatchBodyPhase_SameTargetServerNameAcrossElements(t *testing.T) {
	var seenServers []string
	auth := &mockAuthorizer{
		evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
			mcp, ok := input["mcp"].(*authorization.MCPInput)
			require.True(t, ok, "batch element must expose parsed MCP input")
			seenServers = append(seenServers, mcp.TargetServerName)
			return &authorization.OPADecision{Action: "allow"}, nil
		},
	}
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{Token: "batch-token"}, nil
		},
	}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	batch := []byte(`[{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"list_files","arguments":{}}},{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"read_config","arguments":{}}}]`)
	_, bodyResp := sendHeadersThenBodyWithMetadata(t, client, map[string]string{
		":method": "POST",
	}, tokenExchangeMetadataWithMCPServer("github-mcp"), batch)

	require.NotNil(t, bodyResp)
	_, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_RequestBody)
	require.True(t, ok, "allowed batch must echo a RequestBody response")
	assert.Equal(t, []string{"github-mcp", "github-mcp"}, seenServers,
		"every batch element must receive the identical mcp.target_server_name value")
}
