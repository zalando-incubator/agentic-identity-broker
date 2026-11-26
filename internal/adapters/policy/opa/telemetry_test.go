package opa

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const telemetryMarker = "SYNTHETIC-CLAIM-4d1e"

type capturedTelemetry struct {
	spans  *tracetest.SpanRecorder
	reader *sdkmetric.ManualReader
	logs   *lockedBuffer
}

func captureTelemetry(t *testing.T) *capturedTelemetry {
	t.Helper()
	c := &capturedTelemetry{spans: tracetest.NewSpanRecorder(), reader: sdkmetric.NewManualReader(), logs: &lockedBuffer{}}
	previousTracer, previousMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(c.spans)))
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(c.reader)))
	t.Cleanup(func() {
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
	})
	return c
}

func (c *capturedTelemetry) logger() *slog.Logger { return slog.New(slog.NewJSONHandler(c.logs, nil)) }

func (c *capturedTelemetry) metrics(t *testing.T) map[string]metricdata.Aggregation {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, c.reader.Collect(context.Background(), &rm))
	out := map[string]metricdata.Aggregation{}
	for _, scope := range rm.ScopeMetrics {
		assert.Equal(t, "tokenexchange", scope.Scope.Name)
		for _, m := range scope.Metrics {
			out[m.Name] = m.Data
		}
	}
	return out
}

func attributeMap(set attribute.Set) map[string]string {
	out := map[string]string{}
	for _, kv := range set.ToSlice() {
		out[string(kv.Key)] = kv.Value.String()
	}
	return out
}

func TestEvaluationSpan(t *testing.T) {
	c := captureTelemetry(t)
	module := func(body string) map[string]string {
		return map[string]string{"/p.rego": "package company.impersonation\n\nimport rego.v1\n\n" + body + "\n"}
	}
	input := ports.ImpersonationPolicyInput{"context": map[string]any{"subject_token": map[string]any{"sub": telemetryMarker}}}
	cases := map[string]struct {
		body      string
		ctx       func() context.Context
		action    string
		errorType string
	}{
		"allow":    {body: `result := {"action": "allow", "reasons": ["` + telemetryMarker + `"]}`, action: "allow"},
		"deny":     {body: `result := {"action": "deny"}`, action: "deny"},
		"invalid":  {body: `result := {"action": "approval_required"}`, action: "invalid", errorType: "opa_decision_invalid"},
		"error":    {body: `result := {"action": "allow"} if to_number(input.context.subject_token.sub) > 0`, action: "error", errorType: "opa_evaluation_failed"},
		"canceled": {body: `default result := {"action": "deny"}`, action: "canceled", ctx: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rev := localRevision(t, module(tc.body), nil, decision("governed-inventory", "company.impersonation", "result"))
			ctx := context.Background()
			if tc.ctx != nil {
				ctx = tc.ctx()
			}
			before := len(c.spans.Ended())
			_, _ = rev.Evaluate(ctx, "governed-inventory", input)
			spans := c.spans.Ended()[before:]
			require.Len(t, spans, 1)
			span := spans[0]
			assert.Equal(t, "tokenexchange.impersonation.opa_evaluate", span.Name())
			assert.Equal(t, trace.SpanKindInternal, span.SpanKind())
			attrs := attributeMap(attribute.NewSet(span.Attributes()...))
			assert.Equal(t, "governed-inventory", attrs["impersonation.rule"])
			assert.Equal(t, tc.action, attrs["impersonation.authorization.action"])
			if tc.errorType != "" {
				assert.Equal(t, tc.errorType, attrs["error.type"])
				assert.Equal(t, codes.Error, span.Status().Code)
			} else {
				assert.NotContains(t, attrs, "error.type")
				assert.NotEqual(t, codes.Error, span.Status().Code)
			}
			for _, value := range attrs {
				assert.NotContains(t, value, telemetryMarker)
				assert.NotContains(t, value, "local=")
			}
			assert.Empty(t, span.Events())
		})
	}
}

func TestRevisionMetricsAndLogs(t *testing.T) {
	c := captureTelemetry(t)
	server := newBundleServer(t)
	server.publish(t, "impersonation", "r1", map[string]string{"/p.rego": allowPolicy}, governance(true))
	src, err := NewImpersonationPolicySource(ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{
		ConfigFile: server.bootstrap(t, "    credentials:\n      bearer: {token: "+telemetryMarker+"}\n", "impersonation"),
	}}, []ports.ImpersonationPolicyDecisionRef{governedDecision}, requireDecisions("governed"), c.logger())
	require.NoError(t, err)
	defer src.Stop(context.Background())
	require.NoError(t, awaitReady(t, src, 10*time.Second))

	server.publish(t, "impersonation", "r2", map[string]string{"/p.rego": "package company.impersonation\n\nimport rego.v1\n\nx := http.send({\"method\": \"GET\", \"url\": \"http://127.0.0.1:1\"})\n"}, nil)
	require.Eventually(t, func() bool { return strings.Contains(c.logs.String(), "revision_rejected") }, 10*time.Second, 50*time.Millisecond)
	server.set(func(s *bundleServer) { s.failing = true })
	require.Eventually(t, func() bool { return strings.Contains(c.logs.String(), "bundle_download_failed") }, 10*time.Second, 50*time.Millisecond)

	metrics := c.metrics(t)
	revisions := metrics["tokenexchange.impersonation_opa.revisions"].(metricdata.Sum[int64])
	seen := map[string]bool{}
	for _, point := range revisions.DataPoints {
		attrs := attributeMap(point.Attributes)
		assert.Len(t, attrs, 2, "outcome and reason only")
		seen[attrs["outcome"]+"/"+attrs["reason"]] = true
	}
	assert.True(t, seen["admitted/none"])
	assert.True(t, seen["rejected/prohibited_builtin"])

	age := metrics["tokenexchange.impersonation_opa.revision_age"].(metricdata.Gauge[float64])
	require.Len(t, age.DataPoints, 1)
	assert.Empty(t, age.DataPoints[0].Attributes.ToSlice())
	assert.GreaterOrEqual(t, age.DataPoints[0].Value, 0.0)

	downloads := metrics["tokenexchange.impersonation_opa.bundle_downloads"].(metricdata.Sum[int64])
	outcomes := map[string]bool{}
	for _, point := range downloads.DataPoints {
		attrs := attributeMap(point.Attributes)
		assert.Equal(t, "impersonation", attrs["bundle"])
		assert.Len(t, attrs, 2, "bundle and outcome only")
		outcomes[attrs["outcome"]] = true
	}
	assert.True(t, outcomes["success"])
	assert.True(t, outcomes["failure"])

	first := metrics["tokenexchange.impersonation_opa.bundle_download_age"].(metricdata.Gauge[float64]).DataPoints[0].Value
	time.Sleep(1200 * time.Millisecond)
	second := c.metrics(t)["tokenexchange.impersonation_opa.bundle_download_age"].(metricdata.Gauge[float64]).DataPoints[0].Value
	assert.Greater(t, second, first, "the download age grows while downloads fail")

	logs := c.logs.String()
	assert.NotContains(t, logs, telemetryMarker)
	assert.NotContains(t, logs, server.URL)
	assert.Equal(t, 1, strings.Count(logs, "bundle_download_failed"), "rate-limited per bundle")
	for _, point := range revisions.DataPoints {
		for _, kv := range point.Attributes.ToSlice() {
			assert.NotContains(t, kv.Value.String(), "impersonation=r")
			assert.NotContains(t, kv.Value.String(), src.Current().AdmissionID())
		}
	}

	src.Stop(context.Background())
	assert.NotContains(t, c.metrics(t), "tokenexchange.impersonation_opa.revision_age", "gauge callbacks are unregistered on stop")
}
