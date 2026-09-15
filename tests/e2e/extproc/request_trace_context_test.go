package extproc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

type traceLogCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *traceLogCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *traceLogCapture) records() []map[string]any {
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
		Expect(json.Unmarshal(line, &record)).To(Succeed(), "expected JSON log line: %s", string(line))
		records = append(records, record)
	}

	return records
}

func newTraceLogger() (*slog.Logger, *traceLogCapture) {
	capture := &traceLogCapture{}
	logger := slog.New(slog.NewJSONHandler(capture, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, capture
}

func installTraceTestTelemetry() {
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()

	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	DeferCleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func findTraceRecord(records []map[string]any, message string) map[string]any {
	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}
	return nil
}

func fieldString(record map[string]any, key string) string {
	value, ok := record[key]
	Expect(ok).To(BeTrue(), "expected %q in log record %v", key, record)
	str, ok := value.(string)
	Expect(ok).To(BeTrue(), "expected %q to be a string in log record %v", key, record)
	return str
}

func startDirectTraceEnvironment() (*bootstrap.TestEnvironment, extprocv3.ExternalProcessorClient, *grpc.ClientConn, *traceLogCapture) {
	logger, capture := newTraceLogger()
	cfg := fixtures.DefaultConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Traces.Enabled = true

	env := bootstrap.NewTestEnvironment(cfg, logger)
	env.Start()
	client, conn := env.NewExtProcClient()
	return env, client, conn, capture
}

func startOPATraceEnvironment() (*bootstrap.TestEnvironment, extprocv3.ExternalProcessorClient, *grpc.ClientConn, *traceLogCapture) {
	logger, capture := newTraceLogger()
	cfg := opaEnabledConfig(policyPath("allow_readonly.rego"))
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Traces.Enabled = true

	env := bootstrap.NewTestEnvironment(cfg, logger)
	env.Start()
	client, conn := env.NewExtProcClient()
	return env, client, conn, capture
}

func sendDirectTraceRequest(client extprocv3.ExternalProcessorClient, traceparent string) {
	headersReq := helpers.NewRequestHeaders().
		WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI)
	if traceparent != "" {
		headersReq = headersReq.WithHeader("traceparent", traceparent)
	}

	resp := helpers.SendRequestHeaders(context.Background(), client, headersReq.BuildWithMetadata())
	Expect(resp).NotTo(BeNil())
}

func sendOPATraceRequest(client extprocv3.ExternalProcessorClient, traceparent string) {
	headersReq := helpers.NewRequestHeaders().
		WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
		WithHeader(":method", "POST")
	if traceparent != "" {
		headersReq = headersReq.WithHeader("traceparent", traceparent)
	}

	headersResp, bodyResp := helpers.SendHeadersAndBody(
		context.Background(),
		client,
		headersReq.WithAgentgatewayProtocol("mcp").BuildWithMetadata(),
		toolCallBody("list_files"),
	)
	Expect(headersResp).NotTo(BeNil())
	Expect(bodyResp).NotTo(BeNil())
}

var _ = Describe("Request Trace Context", func() {
	const directSuccessMessage = "token exchanged successfully"
	const opaSuccessMessage = "OPA: token exchanged in headers phase, buffering body for OPA evaluation"

	BeforeEach(func() {
		installTraceTestTelemetry()
	})

	Context("US4 S1 — propagated trace_id", func() {
		// Scenario 4.1 from specs/033-request-security-context/spec.md
		It("reuses an inbound traceparent across the OPA-disabled and OPA-enabled paths", func() {
			const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
			const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

			directEnv, directClient, directConn, directLogs := startDirectTraceEnvironment()
			defer directConn.Close() //nolint:errcheck
			defer directEnv.Stop()
			sendDirectTraceRequest(directClient, traceparent)

			Eventually(func() map[string]any {
				return findTraceRecord(directLogs.records(), directSuccessMessage)
			}).ShouldNot(BeNil())
			Expect(fieldString(findTraceRecord(directLogs.records(), directSuccessMessage), "trace_id")).To(Equal(traceID))

			opaEnv, opaClient, opaConn, opaLogs := startOPATraceEnvironment()
			defer opaConn.Close() //nolint:errcheck
			defer opaEnv.Stop()
			sendOPATraceRequest(opaClient, traceparent)

			Eventually(func() map[string]any {
				return findTraceRecord(opaLogs.records(), opaSuccessMessage)
			}).ShouldNot(BeNil())
			Expect(fieldString(findTraceRecord(opaLogs.records(), opaSuccessMessage), "trace_id")).To(Equal(traceID))
		})
	})

	Context("US4 S2 — generated trace_id", func() {
		// Scenario 4.2 from specs/033-request-security-context/spec.md
		It("generates a trace_id across the OPA-disabled and OPA-enabled paths when none is supplied", func() {
			const traceIDPattern = "^[0-9a-f]{32}$"

			directEnv, directClient, directConn, directLogs := startDirectTraceEnvironment()
			defer directConn.Close() //nolint:errcheck
			defer directEnv.Stop()
			sendDirectTraceRequest(directClient, "")

			Eventually(func() map[string]any {
				return findTraceRecord(directLogs.records(), directSuccessMessage)
			}).ShouldNot(BeNil())
			directTraceID := fieldString(findTraceRecord(directLogs.records(), directSuccessMessage), "trace_id")
			Expect(directTraceID).To(MatchRegexp(traceIDPattern))

			opaEnv, opaClient, opaConn, opaLogs := startOPATraceEnvironment()
			defer opaConn.Close() //nolint:errcheck
			defer opaEnv.Stop()
			sendOPATraceRequest(opaClient, "")

			Eventually(func() map[string]any {
				return findTraceRecord(opaLogs.records(), opaSuccessMessage)
			}).ShouldNot(BeNil())
			opaTraceID := fieldString(findTraceRecord(opaLogs.records(), opaSuccessMessage), "trace_id")
			Expect(opaTraceID).To(MatchRegexp(traceIDPattern))
			Expect(opaTraceID).NotTo(Equal(directTraceID))
		})
	})

	Context("US4 S3 — actor and optional calling_peer semantics", func() {
		// Scenario 4.3 from specs/033-request-security-context/spec.md
		It("records actor=anonymous and omits calling_peer consistently across the OPA-disabled and OPA-enabled paths", func() {
			directEnv, directClient, directConn, directLogs := startDirectTraceEnvironment()
			defer directConn.Close() //nolint:errcheck
			defer directEnv.Stop()
			sendDirectTraceRequest(directClient, "")

			Eventually(func() map[string]any {
				return findTraceRecord(directLogs.records(), directSuccessMessage)
			}).ShouldNot(BeNil())
			directRecord := findTraceRecord(directLogs.records(), directSuccessMessage)
			Expect(fieldString(directRecord, "actor")).To(Equal("anonymous"))
			_, directHasCallingPeer := directRecord["calling_peer"]
			Expect(directHasCallingPeer).To(BeFalse())

			opaEnv, opaClient, opaConn, opaLogs := startOPATraceEnvironment()
			defer opaConn.Close() //nolint:errcheck
			defer opaEnv.Stop()
			sendOPATraceRequest(opaClient, "")

			Eventually(func() map[string]any {
				return findTraceRecord(opaLogs.records(), opaSuccessMessage)
			}).ShouldNot(BeNil())
			opaRecord := findTraceRecord(opaLogs.records(), opaSuccessMessage)
			Expect(fieldString(opaRecord, "actor")).To(Equal("anonymous"))
			_, opaHasCallingPeer := opaRecord["calling_peer"]
			Expect(opaHasCallingPeer).To(BeFalse())
		})
	})
})
