// Package extproc_test contains an E2E integration test that validates three-hop
// trace propagation through a real agentgateway Docker container with OTel enabled.
//
// Test architecture:
//
//	MCP Client (traceparent: 00-TTTT-SSSS-01)
//	  → agentgateway (Docker, no OTel config — passes headers through)
//	  → ExtProc (in-process gRPC, SpanRecorder + TraceContext propagator)
//	  → Mock Identity Broker (httptest, captures Traceparent header)
//
// This validates the spec's US1 Independent Test: "send a request through
// agentgateway with trace context headers, and verify that a trace span from
// ExtProc appears linked to the upstream span via the same trace ID."
//
// The MCP Initialize handshake (BeforeAll) carries the injected traceparent and
// triggers the first token exchange — this is the exchange whose trace
// propagation the test asserts on. The whoami call verifies functional
// correctness (token exchange still works with OTel enabled).
package extproc_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
)

const (
	// Well-known W3C trace context values injected by the MCP client.
	// Used to verify end-to-end propagation through agentgateway and ExtProc.
	otelAgentgwTraceID = "0af7651916cd43dd8448eb211c80319c"
	otelAgentgwSpanID  = "b7ad6b7169203331"
)

// startOTelAgentgwExtProc creates an ExtProc gRPC server with OTel globals
// already installed. The caller must set up TracerProvider and TextMapPropagator
// before calling. NewServer binds meter instruments from the global MeterProvider
// at construction time; tracing uses globals at request time.
func startOTelAgentgwExtProc(
	oauth2URL, tokenExchangeURL string,
) (net.Listener, *grpc.Server, *extprocserver.TokenExchanger) {
	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{
			Bind:                 "0.0.0.0",
			Port:                 0,
			MaxConcurrentStreams: 100,
		},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             tokenExchangeURL + "/oauth2/token",
			Issuer:                    oauth2URL,
			ClientID:                  "e2e-otel-extproc",
			ClientSecret:              "e2e-otel-secret",
			ClientCredentialsEndpoint: oauth2URL + "/oauth/token",
			ClientAssertionType:       "access_token",
			ExchangeTimeout:           10 * time.Second,
			TLS:                       extprocconfig.TLSConfig{AllowHTTP: true},
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		Log: extprocconfig.LogConfig{Level: "debug", Format: "text"},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
		Telemetry: extprocconfig.TelemetryConfig{
			Enabled: true,
			Traces:  extprocconfig.TracesConfig{Enabled: true},
		},
	}

	slogLogger := bootstrap.NewTestLogger()
	exchanger, err := extprocserver.NewTokenExchanger(cfg, slogLogger)
	Expect(err).NotTo(HaveOccurred(), "failed to create OTel token exchanger")

	svc := extprocserver.NewServer(cfg, exchanger, slogLogger)

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	Expect(err).NotTo(HaveOccurred(), "failed to create OTel ExtProc listener")

	grpcSrv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	go func() {
		if serveErr := grpcSrv.Serve(listener); serveErr != nil {
			agentgwLogger.Error("OTel ExtProc gRPC error", "err", serveErr)
		}
	}()

	return listener, grpcSrv, exchanger
}

var _ = Describe("Agentgateway Telemetry Integration", Ordered, func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		spanRecorder    *tracetest.SpanRecorder
		mockOAuth2      *httptest.Server
		broker          *bootstrap.MockTokenExchangeServer
		mcpHTTPServer   *http.Server
		extprocGRPC     *grpc.Server
		exchanger       *extprocserver.TokenExchanger
		jwtFixture      *fixtures.RS256JWTFixture
		mintedJWT       string
		agentgatewayURL string
		prevTP          trace.TracerProvider
		prevProp        propagation.TextMapPropagator
		prevMP          metric.MeterProvider
	)

	BeforeAll(func() {
		// Save current OTel globals inside BeforeAll so we capture the actual
		// state at test execution time, not at package init (which could be
		// stale if another suite modified globals first in the same process).
		prevTP = otel.GetTracerProvider()
		prevProp = otel.GetTextMapPropagator()
		prevMP = otel.GetMeterProvider()
		// testcontainers-go v0.41.0 may panic (instead of returning an error)
		// when Docker is not available. Wrap the check with recover.
		var dockerErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					dockerErr = fmt.Errorf("Docker check panicked: %v", r)
				}
			}()
			_, dockerErr = testcontainers.ProviderDocker.GetProvider()
		}()
		if dockerErr != nil {
			Skip(fmt.Sprintf("Skipping agentgateway telemetry tests: %v", dockerErr))
		}

		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(func() {
			if extprocGRPC != nil {
				extprocGRPC.GracefulStop()
			}
			if exchanger != nil {
				exchanger.Shutdown()
			}
			if broker != nil {
				broker.Stop()
			}
			if mockOAuth2 != nil {
				mockOAuth2.Close()
			}
			if mcpHTTPServer != nil {
				mcpHTTPServer.Close() //nolint:errcheck
			}
			otel.SetTracerProvider(prevTP)
			otel.SetTextMapPropagator(prevProp)
			otel.SetMeterProvider(prevMP)
			cancel()
		})

		var err error
		jwtFixture, err = fixtures.NewRS256JWTFixture(agentgwJWTIssuer, agentgwJWTAudience)
		Expect(err).NotTo(HaveOccurred(), "failed to create telemetry Agentgateway JWT fixture")
		mintedJWT, err = jwtFixture.MintToken("agentgateway-telemetry-subject", time.Now().Add(10*time.Minute))
		Expect(err).NotTo(HaveOccurred(), "failed to mint telemetry Agentgateway JWT")

		// Install test OTel globals BEFORE creating any servers.
		// SpanRecorder captures all spans; TraceContext propagator enables W3C traceparent.
		spanRecorder = tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.TraceContext{})

		// 1. Mock OAuth2 (client_credentials) — reuse from agentgateway_e2e_test.go.
		mockOAuth2 = newAgentgwMockOAuth2Srv()

		// 2. Header-capturing mock broker (identity broker token exchange endpoint).
		// Uses bootstrap.MockTokenExchangeServer which captures LastRequestHeaders().
		broker = bootstrap.NewMockTokenExchangeServer()
		broker.WithExchangedToken(agentgwExchangedToken)
		broker.Start()

		// 3. Mock MCP server — reuse from agentgateway_e2e_test.go.
		var mcpListener net.Listener
		mcpListener, mcpHTTPServer = startAgentgwMCPServer()
		mcpPort := mcpListener.Addr().(*net.TCPAddr).Port

		// 4. ExtProc gRPC server (system under test).
		// OTel globals are already installed, so NewServer binds instruments from them.
		var extprocListener net.Listener
		extprocListener, extprocGRPC, exchanger = startOTelAgentgwExtProc(
			mockOAuth2.URL, broker.URL(),
		)
		extprocPort := extprocListener.Addr().(*net.TCPAddr).Port
		agentgwLogger.Info("OTel ExtProc listening", "port", extprocPort)

		// 5. agentgateway Docker container — reuse from agentgateway_e2e_test.go.
		setupCtx, setupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer setupCancel()
		agentgatewayPort := startAgentgwContainer(setupCtx, extprocPort, mcpPort, jwtFixture)
		agentgatewayURL = fmt.Sprintf("http://localhost:%s", agentgatewayPort)
		agentgwLogger.Info("OTel agentgateway accessible", "url", agentgatewayURL)
	})

	// US1-agentgw-otel: Three-hop trace propagation through agentgateway.
	//
	// Validates the spec's US1 Independent Test:
	//   "Configure the ExtProc service with a valid OTLP endpoint, send a request
	//    through agentgateway with trace context headers, and verify that a trace
	//    span from ExtProc appears in the collector linked to the upstream
	//    agentgateway span via the same trace ID."
	//
	// Trace chain: MCP Client → agentgateway → ExtProc → Mock Broker
	//
	// The MCP client is created within this It block with a known traceparent,
	// so all trace assertions are scoped to this specific request flow.
	It("should propagate trace context end-to-end: client → agentgateway → ExtProc → broker", NodeTimeout(time.Minute), func(ctx SpecContext) {
		// Create MCP client with a known traceparent injected on every HTTP request.
		traceparent := fmt.Sprintf("00-%s-%s-01", otelAgentgwTraceID, otelAgentgwSpanID)
		mcpCl, err := client.NewStreamableHttpClient(
			agentgatewayURL+"/mcp",
			transport.WithHTTPHeaders(map[string]string{
				"Authorization": "Bearer " + mintedJWT,
				"traceparent":   traceparent,
			}),
		)
		Expect(err).NotTo(HaveOccurred())
		defer mcpCl.Close() //nolint:errcheck

		// Start opens the SSE transport — no token exchange expected here.
		err = mcpCl.Start(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Snapshot AFTER Start, BEFORE Initialize — so we can prove Initialize
		// specifically triggers the token exchange (not Start or CallTool).
		spansBefore := len(spanRecorder.Ended())
		brokerCallsBefore := broker.CallCount()

		// Initialize triggers the first token exchange (cache miss).
		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcp.Implementation{Name: "otel-e2e", Version: "1.0.0"}
		_, err = mcpCl.Initialize(ctx, initReq)
		Expect(err).NotTo(HaveOccurred())

		// Assert that Initialize specifically triggered the token exchange.
		Expect(broker.CallCount()).To(BeNumerically(">", brokerCallsBefore),
			"Initialize must trigger the token exchange (cache miss)")

		// Capture broker headers from the Initialize-triggered exchange before
		// any subsequent requests could overwrite them.
		brokerHeaders := broker.LastRequestHeaders()
		Expect(brokerHeaders).NotTo(BeNil(), "Broker must have received request headers")
		brokerTraceparent := brokerHeaders.Get("Traceparent")
		Expect(brokerTraceparent).NotTo(BeEmpty(),
			"Broker must receive a Traceparent header (otelhttp.NewTransport outbound propagation)")

		// Functional baseline: token exchange must still work with OTel enabled.
		// CallTool uses the cached token — no new exchange expected.
		result, err := mcpCl.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: "whoami"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.IsError).To(BeFalse())
		text := agentgwExtractTextContent(result)
		Expect(text).To(ContainSubstring("token_digest="+agentgwTokenDigest(agentgwExchangedToken)),
			"Token exchange must still work with OTel enabled")

		// --- Three-hop trace propagation assertions ---

		// Parse broker's traceparent (captured right after Initialize): 00-<trace_id>-<span_id>-<flags>
		parts := strings.Split(brokerTraceparent, "-")
		Expect(parts).To(HaveLen(4), "traceparent must have 4 dash-separated parts")
		brokerTraceID := parts[1]

		// Find new ExtProc spans created during Initialize (delta from post-Start snapshot).
		var extprocSpan sdktrace.ReadOnlySpan
		allEnded := spanRecorder.Ended()
		for _, s := range allEnded[spansBefore:] {
			if s.Name() == "extproc.token_exchange" &&
				s.SpanContext().TraceID().String() == otelAgentgwTraceID {
				extprocSpan = s
				break
			}
		}
		Expect(extprocSpan).NotTo(BeNil(),
			"SpanRecorder must contain an extproc.token_exchange span from Initialize with the injected trace ID")

		// Two-hop continuity: ExtProc span and broker share the same trace ID.
		extprocTraceID := extprocSpan.SpanContext().TraceID().String()
		Expect(brokerTraceID).To(Equal(extprocTraceID),
			"Broker Traceparent trace_id must match ExtProc span trace_id (ExtProc → broker)")

		// Three-hop continuity: trace ID matches the one injected by the MCP client.
		// This proves agentgateway forwarded the traceparent in the ExtProc RequestHeaders.
		Expect(extprocTraceID).To(Equal(otelAgentgwTraceID),
			"ExtProc span trace_id must match client's injected trace ID "+
				"(client → agentgateway → ExtProc)")

		// Parent-child linkage: ExtProc span's parent is a valid remote span context.
		// agentgateway v1.1.0 creates its own span as an intermediary, so the parent
		// span ID will be agentgateway's span — not the original client span ID.
		// The trace ID continuity (asserted above) already proves three-hop propagation;
		// this assertion proves the ExtProc span is a genuine child, not a root span.
		Expect(extprocSpan.Parent().IsValid()).To(BeTrue(),
			"ExtProc span must have a valid parent (not a root span)")
		Expect(extprocSpan.Parent().SpanID().String()).NotTo(Equal(extprocSpan.SpanContext().SpanID().String()),
			"Parent span ID must differ from ExtProc span ID (genuine parent-child relationship)")
	})
})
