package authorization

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	envoyplugin "github.com/open-policy-agent/opa-envoy-plugin/plugin"
	"github.com/open-policy-agent/opa/v1/plugins"
	oparego "github.com/open-policy-agent/opa/v1/rego"
	opasdk "github.com/open-policy-agent/opa/v1/sdk"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// Authorizer evaluates authorization policies against a request.
// Implementations must be safe for concurrent use.
type Authorizer interface {
	// Evaluate evaluates the authorization policy with the given input document.
	// Returns the decision or an error if evaluation fails.
	Evaluate(ctx context.Context, input OPAInput) (*OPADecision, error)
	// Stop releases any resources held by the authorizer (e.g., background goroutines).
	Stop(ctx context.Context)
}

// OPAAuthorizer is the production Authorizer implementation backed by embedded OPA.
// Supports two modes determined by configuration:
//   - Path mode (cfg.Policy.Path set): uses rego.PreparedEvalQuery (static compile-once)
//   - SDK mode (cfg.Policy.ConfigFile set): uses sdk.OPA (bundle-aware, dynamic config)
//
// Safe for concurrent use — PreparedEvalQuery.Eval and sdk.OPA.Decision are goroutine-safe.
type OPAAuthorizer struct {
	// path mode (Policy.Path set)
	query *oparego.PreparedEvalQuery
	// sdk mode (Policy.ConfigFile set)
	sdkOPA   *opasdk.OPA
	stopOnce sync.Once
	stopCh   chan struct{} // closed on Stop() to terminate background goroutines
	cfg      *config.AuthorizationConfig
	logger   *slog.Logger
}

// NewOPAAuthorizer compiles the Rego policy from cfg and returns an OPAAuthorizer
// ready for concurrent use. Returns an error if the policy cannot be loaded or compiled.
//
// Policy source: exactly one of cfg.Policy.Path or cfg.Policy.ConfigFile must be set.
func NewOPAAuthorizer(cfg *config.AuthorizationConfig, logger *slog.Logger) (*OPAAuthorizer, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.Policy.Path == "" && cfg.Policy.ConfigFile == "" {
		return nil, fmt.Errorf("opa authorizer: no policy source configured (set authorization.policy.path or authorization.policy.config_file)")
	}

	if cfg.Policy.Path != "" && cfg.Policy.ConfigFile != "" {
		return nil, fmt.Errorf("opa authorizer: authorization.policy.path and authorization.policy.config_file are mutually exclusive — set only one")
	}

	if cfg.Policy.ConfigFile != "" {
		return newSDKAuthorizer(cfg, logger)
	}

	return newPreparedQueryAuthorizer(cfg, logger)
}

// newPreparedQueryAuthorizer creates an OPAAuthorizer using rego.PreparedEvalQuery.
// Used when cfg.Policy.Path is set (US3 — local Rego file mode).
func newPreparedQueryAuthorizer(cfg *config.AuthorizationConfig, logger *slog.Logger) (*OPAAuthorizer, error) {
	queryStr := "data." + cfg.Policy.Package + "." + cfg.Policy.Decision

	query, err := oparego.New(
		oparego.Query(queryStr),
		oparego.Load([]string{cfg.Policy.Path}, nil),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, fmt.Errorf("opa authorizer: failed to compile policy %q: %w", cfg.Policy.Path, err)
	}

	return &OPAAuthorizer{
		query:  &query,
		stopCh: make(chan struct{}),
		cfg:    cfg,
		logger: logger,
	}, nil
}

// newSDKAuthorizer creates an OPAAuthorizer using the OPA SDK with config file.
// Used when cfg.Policy.ConfigFile is set (US4 — OPA config file / bundle mode).
//
// Uses the non-blocking Ready channel approach: sdk.New returns immediately and
// OPA retries bundle loading in the background. Until the bundle is loaded, all
// policy evaluations return "undefined" which is treated as deny (fail-closed).
// This allows the ExtProc service to start and become healthy even when the
// bundle server is temporarily unreachable.
func newSDKAuthorizer(cfg *config.AuthorizationConfig, logger *slog.Logger) (*OPAAuthorizer, error) {
	configBytes, err := os.ReadFile(cfg.Policy.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("opa authorizer: cannot read config file %q: %w", cfg.Policy.ConfigFile, err)
	}

	readyCh := make(chan struct{})
	stopCh := make(chan struct{})
	opaSDK, err := opasdk.New(context.Background(), opasdk.Options{
		Config:       bytes.NewReader(configBytes),
		V1Compatible: true,
		Ready:        readyCh,
		// Support configs declaring an envoy_ext_authz_grpc plugin block.
		Plugins: map[string]plugins.Factory{
			envoyplugin.PluginName: envoyplugin.Factory{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opa authorizer: failed to initialize OPA from config %q: %w", cfg.Policy.ConfigFile, err)
	}

	// Log when OPA becomes ready (bundle loaded) in the background.
	// The goroutine exits when either readyCh is closed (bundle loaded) or
	// stopCh is closed (authorizer stopped before bundle loaded).
	go func() {
		select {
		case <-readyCh:
			logger.Info("opa authorizer: OPA SDK ready (bundle loaded)")
		case <-stopCh:
			// Authorizer stopped before bundle loaded — exit silently.
		}
	}()

	return &OPAAuthorizer{
		sdkOPA: opaSDK,
		stopCh: stopCh,
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Evaluate evaluates the OPA policy against input, applying the evaluation timeout
// from cfg. On undefined result or evaluation error, deny is returned (fail-closed).
//
// Audit log fields (SR-004): action, reasons, tool_name, protocol, duration_ms.
func (a *OPAAuthorizer) Evaluate(ctx context.Context, input OPAInput) (*OPADecision, error) {
	ctx, span := otel.Tracer("extproc").Start(ctx, "extproc.opa.evaluate")
	defer span.End()
	if a.sdkOPA != nil {
		return a.evaluateSDK(ctx, input)
	}
	return a.evaluateQuery(ctx, input)
}

// evaluateQuery evaluates using rego.PreparedEvalQuery (path mode).
func (a *OPAAuthorizer) evaluateQuery(ctx context.Context, input OPAInput) (*OPADecision, error) {
	start := time.Now()

	evalCtx, cancel := context.WithTimeout(ctx, a.cfg.EvaluationTimeout)
	defer cancel()

	// Fast-path: if the context is already done (pre-cancelled or deadline already passed),
	// deny immediately without calling OPA. OPA's Eval does not check the context for fast
	// policies, so we must enforce the deadline explicitly here.
	if evalCtx.Err() != nil {
		durationMS := time.Since(start).Milliseconds()
		traceDecision(ctx, input, "deny", "timeout", durationMS)
		a.logAudit(ctx, "deny", nil, durationMS, input, "timeout")
		return &OPADecision{Action: "deny", Reasons: []string{"evaluation timeout"}}, nil
	}

	rs, err := a.query.Eval(evalCtx, oparego.EvalInput(input))
	durationMS := time.Since(start).Milliseconds()

	if err != nil {
		traceDecision(ctx, input, "deny", "error", durationMS)
		a.logAudit(ctx, "deny", nil, durationMS, input, "error")
		a.logger.ErrorContext(ctx, "opa evaluation error — denying", "error", err, "duration_ms", durationMS)
		return &OPADecision{Action: "deny", Reasons: []string{"policy evaluation error"}}, nil
	}

	// Empty ResultSet means the rule is undefined — deny unconditionally to fail closed.
	if len(rs) == 0 || len(rs[0].Expressions) == 0 || rs[0].Expressions[0].Value == nil {
		traceDecision(ctx, input, "deny", "undefined", durationMS)
		a.logAudit(ctx, "deny", nil, durationMS, input, "undefined")
		return &OPADecision{Action: "deny", Reasons: []string{"policy result is undefined"}}, nil
	}

	resultMap, _ := rs[0].Expressions[0].Value.(map[string]any)
	return a.finalizeDecision(ctx, resultMap, durationMS, input)
}

// evaluateSDK evaluates using the OPA SDK (config file mode).
func (a *OPAAuthorizer) evaluateSDK(ctx context.Context, input OPAInput) (*OPADecision, error) {
	start := time.Now()

	evalCtx, cancel := context.WithTimeout(ctx, a.cfg.EvaluationTimeout)
	defer cancel()

	// Fast-path: if the context is already done (pre-cancelled or deadline already passed),
	// deny immediately without calling OPA.
	if evalCtx.Err() != nil {
		durationMS := time.Since(start).Milliseconds()
		traceDecision(ctx, input, "deny", "timeout", durationMS)
		a.logAudit(ctx, "deny", nil, durationMS, input, "timeout")
		return &OPADecision{Action: "deny", Reasons: []string{"evaluation timeout"}}, nil
	}

	// Convert package+decision to slash-separated SDK path format with leading slash.
	// e.g., "aib.extproc.authz" + "result" → "/aib/extproc/authz/result"
	decisionPath := "/" + strings.ReplaceAll(a.cfg.Policy.Package, ".", "/") + "/" + a.cfg.Policy.Decision

	dr, err := a.sdkOPA.Decision(evalCtx, opasdk.DecisionOptions{
		Path:  decisionPath,
		Input: input,
	})
	durationMS := time.Since(start).Milliseconds()

	if err != nil {
		if opasdk.IsUndefinedErr(err) {
			traceDecision(ctx, input, "deny", "undefined", durationMS)
			a.logAudit(ctx, "deny", nil, durationMS, input, "undefined")
			return &OPADecision{Action: "deny", Reasons: []string{"policy result is undefined"}}, nil
		}
		traceDecision(ctx, input, "deny", "error", durationMS)
		a.logAudit(ctx, "deny", nil, durationMS, input, "error")
		a.logger.ErrorContext(ctx, "opa sdk evaluation error — denying", "error", err, "duration_ms", durationMS)
		return &OPADecision{Action: "deny", Reasons: []string{"policy evaluation error"}}, nil
	}

	resultMap, _ := dr.Result.(map[string]any)
	return a.finalizeDecision(ctx, resultMap, durationMS, input)
}

// finalizeDecision parses the raw policy result, preserves future-action observability,
// emits the audit log, and returns the effective enforcement decision.
func (a *OPAAuthorizer) finalizeDecision(ctx context.Context, resultMap map[string]any, durationMS int64, input OPAInput) (*OPADecision, error) {
	decision := ParseDecision(resultMap)

	auditAction := decision.Action
	resultCode := "ok"
	rawAction, _ := resultMap["action"].(string)
	if rawAction == "approval_required" || rawAction == "ciba_required" {
		auditAction = rawAction
		resultCode = "unsupported_action"
		a.logger.WarnContext(ctx, "unsupported OPA action — treating as deny",
			"action", rawAction, "duration_ms", durationMS)
	}

	traceDecision(ctx, input, auditAction, resultCode, durationMS)
	a.logAudit(ctx, auditAction, decision.Reasons, durationMS, input, resultCode)
	return decision, nil
}

func traceDecision(ctx context.Context, input OPAInput, action, resultCode string, durationMS int64) {
	fields := auditDimensions(input)
	attrs := []attribute.KeyValue{
		attribute.String("authorization.action", action),
		attribute.String("authorization.result_code", resultCode),
		attribute.String("authorization.protocol", fields.protocol),
		attribute.Int64("authorization.duration_ms", durationMS),
	}
	if fields.toolName != "" {
		attrs = append(attrs, attribute.String("authorization.tool_name", fields.toolName))
	}
	if fields.method != "" {
		attrs = append(attrs, attribute.String("authorization.method", fields.method))
	}
	if fields.targetServerName != "" {
		attrs = append(attrs, attribute.String("authorization.target_server_name", fields.targetServerName))
	}
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// auditFields holds the request dimensions extracted from an OPAInput for audit
// logs and trace spans (SR-004).
type auditFields struct {
	protocol         string
	toolName         string
	method           string
	targetServerName string
}

func auditDimensions(input OPAInput) auditFields {
	fields := auditFields{protocol: "unknown"}
	if input == nil {
		return fields
	}
	if t, ok := input["type"].(string); ok {
		switch {
		case strings.HasPrefix(t, "mcp_"):
			fields.protocol = "mcp"
		default:
			fields.protocol = "unknown"
		}
	}
	if mcp, ok := input["mcp"].(*MCPInput); ok && mcp != nil {
		fields.toolName = mcp.ToolName
		fields.method = mcp.Method
		fields.targetServerName = mcp.TargetServerName
	}
	return fields
}

// logAudit emits a structured audit log entry per SR-004.
func (a *OPAAuthorizer) logAudit(ctx context.Context, action string, reasons []string, durationMS int64, input OPAInput, resultCode string) {
	fields := auditDimensions(input)
	a.logger.InfoContext(ctx, "opa authorization decision",
		"action", action,
		"reasons", reasons,
		"tool_name", fields.toolName,
		"method", fields.method,
		"protocol", fields.protocol,
		"target_server_name", fields.targetServerName,
		"duration_ms", durationMS,
		"result_code", resultCode,
	)
}

// Stop releases resources held by the authorizer.
// For PreparedEvalQuery mode: closes stopCh (no background goroutines to stop).
// For SDK mode: stops the OPA SDK instance and its background goroutines.
// Safe to call multiple times.
func (a *OPAAuthorizer) Stop(ctx context.Context) {
	a.stopOnce.Do(func() {
		close(a.stopCh)
	})
	if a.sdkOPA != nil {
		a.sdkOPA.Stop(ctx)
	}
}
