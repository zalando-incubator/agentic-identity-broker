package authorization_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
)

func TestOPAAuthorizer_RealPolicyExportsOnlyAuditFields(t *testing.T) {
	evidence.ForTest(t, "gateway-policy-authorization.audit.no-request-content")
	{
		t.Run("deny", func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previous)
				require.NoError(t, provider.Shutdown(context.Background()))
			})
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			policy := `package aib.extproc.authz
import rego.v1
result := {"action": "deny", "reasons": [input.mcp.tool_name, input.mcp.target_server_name]}
`
			authorizer, err := authorization.NewOPAAuthorizer(authzConfig(writePolicy(t, policy)), logger)
			require.NoError(t, err)
			defer authorizer.Stop(context.Background())
			input := authorization.OPAInput{
				"type":    "mcp_type-secret-sentinel",
				"headers": map[string]any{"authorization": "header-secret-sentinel"},
				"body":    map[string]any{"secret": "body-secret-sentinel"},
				"mcp":     map[string]any{"tool_name": "list_files", "method": "method-secret-sentinel", "target_server_name": "github-mcp"},
			}
			decision, err := authorizer.Evaluate(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, authorization.ActionDeny, decision.Action)
			require.NotEmpty(t, decision.Reasons, "policy protocol behavior remains unchanged")
			assert.Contains(t, fmt.Sprint(decision.Reasons), "list_files")
			require.NotEmpty(t, logs.String())
			telemetry := logs.String()
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			for _, span := range spans {
				encoded, err := json.Marshal(tracetest.SpanStubFromReadOnlySpan(span))
				require.NoError(t, err)
				telemetry += string(encoded) + fmt.Sprint(span.Resource().Attributes())
				assert.Empty(t, span.Events())
				attrs := attributeMap(span.Attributes())
				assert.Equal(t, "deny", attrs["authorization.action"])
				assert.Equal(t, "unknown", attrs["authorization.method"])
				assert.Equal(t, "mcp", attrs["authorization.protocol"])
				assert.Equal(t, "list_files", attrs["authorization.tool_name"])
				assert.Equal(t, "github-mcp", attrs["authorization.target_server_name"])
			}
			for _, secret := range []string{"type-secret-sentinel", "header-secret-sentinel", "body-secret-sentinel", "method-secret-sentinel"} {
				assert.NotContains(t, telemetry, secret)
			}
		})
	}
}

func TestOPAAuthorizer_MCPMethodTelemetry(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		want   string
	}{
		{name: "tool call", method: "tools/call", want: "tools/call"},
		{name: "initialize", method: "initialize", want: "initialize"},
		{name: "unknown", method: "method-secret-sentinel", want: "unknown"},
		{name: "absent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previous)
				require.NoError(t, provider.Shutdown(context.Background()))
			})
			var logs bytes.Buffer
			authorizer, err := authorization.NewOPAAuthorizer(authzConfig(writePolicy(t, allowAllPolicy)), slog.New(slog.NewJSONHandler(&logs, nil)))
			require.NoError(t, err)
			defer authorizer.Stop(context.Background())
			mcp := map[string]any{"tool_name": "list_files", "target_server_name": "github-mcp"}
			if test.method != "" {
				mcp["method"] = test.method
			}
			inputType := "mcp_method"
			if test.method == "tools/call" {
				inputType = "mcp_tool_call"
			}
			decision, err := authorizer.Evaluate(context.Background(), authorization.OPAInput{"type": inputType, "mcp": mcp})
			require.NoError(t, err)
			assert.Equal(t, authorization.ActionAllow, decision.Action)
			var audit map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &audit))
			assert.Equal(t, test.want, audit["method"])
			assert.Equal(t, "mcp", audit["protocol"])
			assert.Equal(t, "allow", audit["action"])
			assert.Equal(t, "list_files", audit["tool_name"])
			assert.Equal(t, "github-mcp", audit["target_server_name"])
			assert.Equal(t, "ok", audit["result_code"])
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			attrs := attributeMap(spans[0].Attributes())
			assert.Equal(t, "mcp", attrs["authorization.protocol"])
			assert.Equal(t, "allow", attrs["authorization.action"])
			assert.Equal(t, "ok", attrs["authorization.result_code"])
			if test.want == "" {
				assert.NotContains(t, attrs, "authorization.method")
			} else {
				assert.Equal(t, test.want, attrs["authorization.method"])
			}
			for _, secret := range []string{"method-secret-sentinel"} {
				assert.NotContains(t, logs.String(), secret)
				assert.NotContains(t, fmt.Sprint(attrs), secret)
			}
		})
	}
}
