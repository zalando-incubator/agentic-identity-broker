package authorization_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sdktest "github.com/open-policy-agent/opa/v1/sdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

type capturedLogContext struct {
	message string
	traceID string
	spanID  string
}

type contextCaptureHandler struct {
	mu      sync.Mutex
	records []capturedLogContext
}

func (h *contextCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *contextCaptureHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	sc := trace.SpanContextFromContext(ctx)
	h.records = append(h.records, capturedLogContext{
		message: record.Message,
		traceID: sc.TraceID().String(),
		spanID:  sc.SpanID().String(),
	})
	return nil
}

func (h *contextCaptureHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *contextCaptureHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *contextCaptureHandler) find(message string) (capturedLogContext, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, record := range h.records {
		if record.message == message {
			return record, true
		}
	}

	return capturedLogContext{}, false
}

// writePolicy writes a Rego policy file to a temp dir and returns the path.
func writePolicy(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "authz.rego")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// allowAllPolicy follows the documented helper-set pattern: policy-local allow/deny
// sets compose into a single result document that ExtProc reads.
const allowAllPolicy = `package aib.extproc.authz
import rego.v1

deny contains {"reason": "unused deny helper"} if {
	false
}

allow contains {"reason": "all requests allowed"} if {
	true
}

result := {"action": "deny", "reasons": _deny_reasons} if {
	count(deny) > 0
	_deny_reasons := [r | some entry in deny; r := entry.reason]
} else := {"action": "allow"} if {
	count(allow) > 0
} else := {"action": "deny", "reasons": ["no policy rule matched"]}
`

// denyAllPolicy follows the same aggregation shape but drives the deny path.
const denyAllPolicy = `package aib.extproc.authz
import rego.v1


allow contains {"reason": "unused allow helper"} if {
	false
}
deny contains {"reason": "all requests denied"} if {
	true
}

result := {"action": "deny", "reasons": _deny_reasons} if {
	count(deny) > 0
	_deny_reasons := [r | some entry in deny; r := entry.reason]
} else := {"action": "allow"} if {
	count(allow) > 0
} else := {"action": "deny", "reasons": ["no policy rule matched"]}
`

// undefinedPolicy is a Rego policy where result is never defined.
const undefinedPolicy = `package aib.extproc.authz
import rego.v1
`

// approvalRequiredPolicy emits the active Tier 2 action.
const approvalRequiredPolicy = `package aib.extproc.authz
import rego.v1

approval_required contains {"reason": "user approval required"} if {
	true
}

result := {"action": "approval_required", "reasons": _approval_reasons} if {
	count(approval_required) > 0
	_approval_reasons := [r | some entry in approval_required; r := entry.reason]
} else := {"action": "deny", "reasons": ["no policy rule matched"]}
`

const unknownActionPolicy = `package aib.extproc.authz
import rego.v1

result := {"action": "future_action"}
`

const cibaRequiredPolicy = `package aib.extproc.authz
import rego.v1

result := {"action": "ciba_required"}
`

const permissionSetPolicy = `package aib.extproc.authz
import rego.v1

required_service_id := "svc-required"

permission_set_context_available if {
	input.context.granted_permission_sets_available
}

permission_set_grants_service(service_id) if {
	permission_set_context_available
	some permission_set_id
	service_id in input.context.granted_permission_sets[permission_set_id]
}

deny contains {"reason": "permission-set context unavailable"} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "permissioned_read"
	not permission_set_context_available
}

allow contains {"reason": "permission set grants service access"} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "permissioned_read"
	permission_set_grants_service(required_service_id)
}

deny contains {"reason": "required permission set missing"} if {
	input.type == "mcp_tool_call"
	input.mcp.tool_name == "permissioned_read"
	permission_set_context_available
	not permission_set_grants_service(required_service_id)
}

result := {"action": "deny", "reasons": _deny_reasons} if {
	count(deny) > 0
	_deny_reasons := [r | some entry in deny; r := entry.reason]
} else := {"action": "allow"} if {
	count(allow) > 0
} else := {"action": "deny", "reasons": ["no policy rule matched"]}
`

func authzConfig(policyPath string) *config.AuthorizationConfig {
	return &config.AuthorizationConfig{
		Enabled: true,
		Policy: config.PolicyConfig{
			Path:     policyPath,
			Package:  "aib.extproc.authz",
			Decision: "result",
		},
		DefaultDecision:   "deny",
		EvaluationTimeout: 5 * time.Second,
		MaxBodySize:       1048576,
	}
}

func authzSDKConfig(configFile string) *config.AuthorizationConfig {
	return &config.AuthorizationConfig{
		Enabled: true,
		Policy: config.PolicyConfig{
			ConfigFile: configFile,
			Package:    "aib.extproc.authz",
			Decision:   "result",
		},
		DefaultDecision:   "deny",
		EvaluationTimeout: 5 * time.Second,
		MaxBodySize:       1048576,
	}
}

func writeOPAConfigFile(t *testing.T, bundleURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "opa-config.yaml")
	content := fmt.Sprintf(`services:
  bundle-server:
    url: %s
bundles:
  app:
    service: bundle-server
    resource: /bundles/bundle.tar.gz
    polling:
      min_delay_seconds: 1
      max_delay_seconds: 2
`, bundleURL)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// testInput creates a minimal OPAInput for unit tests with the given type discriminator.
func testInput(inputType string) authorization.OPAInput {
	return authorization.OPAInput{
		"type": inputType,
	}
}

func attributeMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.String()
	}
	return m
}

func TestOPAAuthorizer_Evaluate_CreatesTraceSpanWithDecisionAttributes(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})

	path := writePolicy(t, allowAllPolicy)
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	ctx, parentSpan := tp.Tracer("test").Start(context.Background(), "headers-phase")
	expectedTraceID := parentSpan.SpanContext().TraceID().String()
	expectedParentSpanID := parentSpan.SpanContext().SpanID().String()
	parentSpan.End()

	input := authorization.OPAInput{
		"type": "mcp_tool_call",
		"mcp":  &authorization.MCPInput{ToolName: "list_files"},
	}
	decision, err := auth.Evaluate(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "allow", decision.Action)

	require.NoError(t, tp.ForceFlush(context.Background()))
	spans := spanRecorder.Ended()

	var found bool
	for _, s := range spans {
		if s.Name() != "extproc.opa.evaluate" {
			continue
		}
		found = true
		attrs := attributeMap(s.Attributes())
		assert.Equal(t, expectedTraceID, s.SpanContext().TraceID().String())
		assert.Equal(t, expectedParentSpanID, s.Parent().SpanID().String())
		assert.Equal(t, "allow", attrs["authorization.action"])
		assert.Equal(t, "ok", attrs["authorization.result_code"])
		assert.Equal(t, "mcp", attrs["authorization.protocol"])
		assert.Equal(t, "list_files", attrs["authorization.tool_name"])
		break
	}
	assert.True(t, found, "span 'extproc.opa.evaluate' must exist")
}

// TestOPAAuthorizer_Evaluate_TraceSpanIncludesTargetServerName verifies that a
// non-empty MCPInput.TargetServerName is included in the trace span and audit log
// as authorization.target_server_name / target_server_name (SR-004).
func TestOPAAuthorizer_Evaluate_TraceSpanIncludesTargetServerName(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})

	path := writePolicy(t, allowAllPolicy)
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	input := authorization.OPAInput{
		"type": "mcp_tool_call",
		"mcp":  &authorization.MCPInput{ToolName: "list_files", TargetServerName: "github-mcp"},
	}
	decision, err := auth.Evaluate(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "allow", decision.Action)

	require.NoError(t, tp.ForceFlush(context.Background()))
	spans := spanRecorder.Ended()

	var found bool
	for _, s := range spans {
		if s.Name() != "extproc.opa.evaluate" {
			continue
		}
		found = true
		attrs := attributeMap(s.Attributes())
		assert.Equal(t, "github-mcp", attrs["authorization.target_server_name"])
		break
	}
	assert.True(t, found, "span 'extproc.opa.evaluate' must exist")
}

// TestOPAAuthorizer_Evaluate_TraceSpanOmitsTargetServerNameWhenAbsent verifies
// that the target_server_name span attribute is omitted (not set to "") when
// MCPInput.TargetServerName is empty.
func TestOPAAuthorizer_Evaluate_TraceSpanOmitsTargetServerNameWhenAbsent(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})

	path := writePolicy(t, allowAllPolicy)
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	input := authorization.OPAInput{
		"type": "mcp_tool_call",
		"mcp":  &authorization.MCPInput{ToolName: "list_files"},
	}
	decision, err := auth.Evaluate(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "allow", decision.Action)

	require.NoError(t, tp.ForceFlush(context.Background()))
	spans := spanRecorder.Ended()

	var found bool
	for _, s := range spans {
		if s.Name() != "extproc.opa.evaluate" {
			continue
		}
		found = true
		attrs := attributeMap(s.Attributes())
		_, hasTargetServerName := attrs["authorization.target_server_name"]
		assert.False(t, hasTargetServerName, "target_server_name attribute must be omitted when absent")
		break
	}
	assert.True(t, found, "span 'extproc.opa.evaluate' must exist")
}

func TestOPAAuthorizer_Allow(t *testing.T) {
	// Scenario: policy returns allow → decision is allow
	path := writePolicy(t, allowAllPolicy)
	cfg := authzConfig(path)

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, "allow", decision.Action)
	assert.Empty(t, decision.Reasons)
}

func TestOPAAuthorizer_Deny(t *testing.T) {
	// Scenario: policy returns deny with reasons → decision is deny with reasons
	path := writePolicy(t, denyAllPolicy)
	cfg := authzConfig(path)

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, "deny", decision.Action)
	assert.Contains(t, decision.Reasons, "all requests denied")
}

func TestOPAAuthorizer_PermissionSetContextAvailability(t *testing.T) {
	path := writePolicy(t, permissionSetPolicy)
	cfg := authzConfig(path)

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	tests := []struct {
		name       string
		context    authorization.ContextInput
		wantAction string
		wantReason string
	}{
		{
			name: "available snapshot grants service",
			context: authorization.ContextInput{
				GrantedPermissionSetsAvailable: true,
				GrantedPermissionSets: map[string][]string{
					"perm-set-1": {"svc-required"},
				},
			},
			wantAction: "allow",
		},
		{
			name: "available empty snapshot denies missing service",
			context: authorization.ContextInput{
				GrantedPermissionSetsAvailable: true,
				GrantedPermissionSets:          map[string][]string{},
			},
			wantAction: "deny",
			wantReason: "required permission set missing",
		},
		{
			name:       "unavailable snapshot denies before service check",
			context:    authorization.ContextInput{GrantedPermissionSetsAvailable: false},
			wantAction: "deny",
			wantReason: "permission-set context unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := auth.Evaluate(context.Background(), authorization.OPAInput{
				"type":    "mcp_tool_call",
				"mcp":     &authorization.MCPInput{ToolName: "permissioned_read"},
				"context": tt.context,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantAction, decision.Action)
			if tt.wantReason == "" {
				assert.Empty(t, decision.Reasons)
			} else {
				assert.Contains(t, decision.Reasons, tt.wantReason)
			}
		})
	}
}

func TestOPAAuthorizer_UndefinedResult_Deny(t *testing.T) {
	// Scenario: policy does not define result → default_decision (deny) is returned
	path := writePolicy(t, undefinedPolicy)
	cfg := authzConfig(path)

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, "deny", decision.Action)
}

func TestOPAAuthorizer_UndefinedResult_DeniesEvenWhenConfiguredAllowDefaultInPathMode(t *testing.T) {
	path := writePolicy(t, undefinedPolicy)
	cfg := authzConfig(path)
	cfg.DefaultDecision = "allow"

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, "deny", decision.Action)
	assert.Contains(t, decision.Reasons, "policy result is undefined")
}

func TestOPAAuthorizer_EvaluationTimeout_Deny(t *testing.T) {
	// Scenario: context cancelled before evaluation → deny
	path := writePolicy(t, allowAllPolicy)
	cfg := authzConfig(path)
	cfg.EvaluationTimeout = 1 * time.Millisecond

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	// Use an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	decision, err := auth.Evaluate(ctx, testInput("unknown"))
	// Either returns error or returns deny — both acceptable outcomes
	if err != nil {
		// error path: caller should treat as deny
		return
	}
	assert.Equal(t, "deny", decision.Action)
}

func TestOPAAuthorizer_Stop_GracefulShutdown(t *testing.T) {
	// Scenario: Stop() is idempotent and does not panic
	path := writePolicy(t, allowAllPolicy)
	cfg := authzConfig(path)

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)

	// First Stop
	auth.Stop(context.Background())
	// Second Stop must not panic
	assert.NotPanics(t, func() {
		auth.Stop(context.Background())
	})
}

func TestOPAAuthorizer_ApprovalRequired_IsPreserved(t *testing.T) {
	path := writePolicy(t, approvalRequiredPolicy)
	cfg := authzConfig(path)
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	auth, err := authorization.NewOPAAuthorizer(cfg, logger)
	require.NoError(t, err)
	defer auth.Stop(context.Background())
	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, authorization.ActionApprovalRequired, decision.Action)
	assert.Contains(t, logBuf.String(), `"action":"approval_required"`)
}

func TestOPAAuthorizer_CIBARequiredIsDeniedAndWarned(t *testing.T) {
	path := writePolicy(t, cibaRequiredPolicy)
	var logBuf bytes.Buffer
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), slog.New(slog.NewJSONHandler(&logBuf, nil)))
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, authorization.ActionDeny, decision.Action)
	assert.Equal(t, []string{"ciba_required is not yet supported"}, decision.Reasons)
	assert.Contains(t, logBuf.String(), "unsupported OPA action")
	assert.Contains(t, logBuf.String(), authorization.ActionCIBARequired)
}

func TestOPAAuthorizer_UndefinedActionWarnsAndDenies(t *testing.T) {
	path := writePolicy(t, unknownActionPolicy)
	var logBuf bytes.Buffer
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), slog.New(slog.NewJSONHandler(&logBuf, nil)))
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, authorization.ActionDeny, decision.Action)
	assert.Equal(t, []string{"unknown action: future_action"}, decision.Reasons)
	assert.Contains(t, logBuf.String(), "undefined OPA action")
	assert.Contains(t, logBuf.String(), "future_action")
}

func TestOPAAuthorizer_AuditLog_UsesRequestContext(t *testing.T) {
	handler := &contextCaptureHandler{}
	logger := slog.New(handler)

	path := writePolicy(t, allowAllPolicy)
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), logger)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	tp := sdktrace.NewTracerProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})

	ctx, parentSpan := tp.Tracer("test").Start(context.Background(), "headers-phase")
	expectedTraceID := parentSpan.SpanContext().TraceID().String()

	decision, err := auth.Evaluate(ctx, authorization.OPAInput{
		"type": "mcp_tool_call",
		"mcp":  &authorization.MCPInput{ToolName: "list_files"},
	})
	parentSpan.End()
	require.NoError(t, err)
	assert.Equal(t, "allow", decision.Action)

	record, ok := handler.find("opa authorization decision")
	require.True(t, ok, "audit log must be emitted")
	assert.Equal(t, expectedTraceID, record.traceID)
	assert.NotEqual(t, trace.TraceID{}.String(), record.traceID)
	assert.NotEqual(t, trace.SpanID{}.String(), record.spanID)
}

func TestOPAAuthorizer_ApprovalRequiredLog_UsesRequestContext(t *testing.T) {
	handler := &contextCaptureHandler{}
	logger := slog.New(handler)
	path := writePolicy(t, approvalRequiredPolicy)
	auth, err := authorization.NewOPAAuthorizer(authzConfig(path), logger)
	require.NoError(t, err)
	defer auth.Stop(context.Background())
	tp := sdktrace.NewTracerProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP); _ = tp.Shutdown(context.Background()) })
	ctx, parentSpan := tp.Tracer("test").Start(context.Background(), "headers-phase")
	expectedTraceID := parentSpan.SpanContext().TraceID().String()
	decision, err := auth.Evaluate(ctx, testInput("unknown"))
	parentSpan.End()
	require.NoError(t, err)
	assert.Equal(t, authorization.ActionApprovalRequired, decision.Action)
	record, ok := handler.find("opa authorization decision")
	require.True(t, ok)
	assert.Equal(t, expectedTraceID, record.traceID)
}

func TestOPAAuthorizer_SDKUndefined_DeniesEvenWhenDefaultDecisionAllow(t *testing.T) {
	bundleReadyCh := make(chan struct{})
	bundleServer := sdktest.MustNewServer(
		sdktest.Ready(bundleReadyCh),
		sdktest.MockBundle("/bundles/bundle.tar.gz", map[string]string{
			"allow_all.rego": allowAllPolicy,
		}),
	)
	defer bundleServer.Stop()

	cfg := authzSDKConfig(writeOPAConfigFile(t, bundleServer.URL()))
	cfg.DefaultDecision = "allow"

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
	require.NoError(t, err)
	assert.Equal(t, "deny", decision.Action)
	assert.Contains(t, decision.Reasons, "policy result is undefined")
}

func TestOPAAuthorizer_SDKConfigWithEnvoyPlugin_StartsSuccessfully(t *testing.T) {
	bundleServer := sdktest.MustNewServer(
		sdktest.MockBundle("/bundles/bundle.tar.gz", map[string]string{
			"allow_all.rego": allowAllPolicy,
		}),
	)
	defer bundleServer.Stop()

	cfg := authzSDKConfig(writeOPAConfigFileWithEnvoyPlugin(t, bundleServer.URL()))

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	// bundle activation is asynchronous, poll until it loads
	require.Eventually(t, func() bool {
		decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
		return err == nil && decision.Action == "allow"
	}, 5*time.Second, 10*time.Millisecond, "expected policy decision to eventually become allow once the bundle is loaded")
}

// writeOPAConfigFileWithEnvoyPlugin writes an OPA SDK config declaring an
// envoy_ext_authz_grpc plugin block alongside a bundle service.
func writeOPAConfigFileWithEnvoyPlugin(t *testing.T, bundleURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "opa-config.yaml")
	content := fmt.Sprintf(`
services:
  bundle-server:
    url: %s
bundles:
  app:
    service: bundle-server
    resource: /bundles/bundle.tar.gz
plugins:
  envoy_ext_authz_grpc:
    addr: "127.0.0.1:0"
    path: aib/extproc/authz
    dry-run: false
`, bundleURL)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestOPAAuthorizer_DiscoveryConfigWithEnvoyPlugin_StartsSuccessfully(t *testing.T) {
	bundleServer := sdktest.MustNewServer(
		sdktest.MockBundle("/bundles/discovery.tar.gz", map[string]string{
			"discovery.rego": `package discovery

bundles := {"app": {"service": "bundle-server", "resource": "/bundles/bundle.tar.gz"}}

plugins := {"envoy_ext_authz_grpc": {"addr": "127.0.0.1:0", "path": "aib/extproc/authz", "dry-run": false}}
`,
		}),
		sdktest.MockBundle("/bundles/bundle.tar.gz", map[string]string{
			"allow_all.rego": allowAllPolicy,
		}),
	)
	defer bundleServer.Stop()

	cfg := authzSDKConfig(writeOPAConfigFileWithDiscovery(t, bundleServer.URL()))

	auth, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.NoError(t, err)
	defer auth.Stop(context.Background())

	// discovery + bundle activation is asynchronous, poll until it loads
	require.Eventually(t, func() bool {
		decision, err := auth.Evaluate(context.Background(), testInput("unknown"))
		return err == nil && decision.Action == "allow"
	}, 5*time.Second, 10*time.Millisecond, "expected policy decision to eventually become allow once the discovery and bundle are loaded")
}

// writeOPAConfigFileWithDiscovery writes an OPA SDK config that fetches its
// bundles and plugins (including envoy_ext_authz_grpc) via a discovery document.
func writeOPAConfigFileWithDiscovery(t *testing.T, bundleURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "opa-config.yaml")
	content := fmt.Sprintf(`
services:
  bundle-server:
    url: %s
discovery:
  name: discovery
  resource: /bundles/discovery.tar.gz
  service: bundle-server
`, bundleURL)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNewOPAAuthorizer_InvalidPolicyPath(t *testing.T) {
	// Scenario: policy file does not exist → NewOPAAuthorizer returns error
	cfg := authzConfig("/does/not/exist/policy.rego")

	_, err := authorization.NewOPAAuthorizer(cfg, nil)
	require.Error(t, err)
}
