package opa

import (
	"context"
	"fmt"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// revisionReason is the reason attribute of a candidate revision outcome.
type revisionReason string

const (
	// reasonNone is set on an admitted revision.
	reasonNone revisionReason = "none"
	// reasonCompileFailed is set when a module does not parse or compile; fix the policy and
	// publish a new revision.
	reasonCompileFailed revisionReason = "compile_failed"
	// reasonProhibitedBuiltin is set when a module calls a removed nondeterministic built-in such
	// as http.send, even in a rule no decision reaches.
	reasonProhibitedBuiltin revisionReason = "prohibited_builtin"
	// reasonDecisionAbsent is set when an OPA rule's decision rule does not exist in the
	// candidate; the log names the first failing rule.
	reasonDecisionAbsent revisionReason = "decision_absent"
	// reasonSnapshotFailed is set when the candidate's policy or data cannot be read or prepared.
	reasonSnapshotFailed revisionReason = "snapshot_failed"
	// reasonBundlesIncomplete is set when the candidate does not contain every configured bundle.
	reasonBundlesIncomplete revisionReason = "bundles_incomplete"
)

// rejection is a candidate revision that did not become active. It carries only closed values
// and operator-defined names, never policy text, data, or native OPA errors.
type rejection struct {
	reason  revisionReason
	builtin string // removed built-in, for prohibited_builtin
	module  string // bundle path of the failing module, for compile failures
	rule    string // first failing rule, for decision_absent
}

// decisionQuery is one rule's prepared decision query.
type decisionQuery struct {
	query   rego.PreparedEvalQuery
	timeout time.Duration
}

// revision is an immutable admitted snapshot. Every query was prepared from the same compiled
// modules and the same data copy, so all OPA rules of one request see one policy and data version.
type revision struct {
	label       string
	admissionID string
	admittedAt  time.Time
	queries     map[string]decisionQuery
	telemetry   *telemetry
}

var _ ports.ImpersonationPolicyRevision = (*revision)(nil)

func (r *revision) Label() string       { return r.label }
func (r *revision) AdmissionID() string { return r.admissionID }

// Evaluate runs the rule's decision within its evaluation timeout. The decision must be exactly
// one object whose action is allow or deny; anything else is invalid. Errors, timeouts, and
// cancellation are evaluation failures.
func (r *revision) Evaluate(ctx context.Context, rule string, input ports.ImpersonationPolicyInput) (ports.ImpersonationPolicyAction, error) {
	q, ok := r.queries[rule]
	if !ok {
		return "", fmt.Errorf("rule has no prepared decision: %w", ports.ErrPolicyEvaluationFailed)
	}
	ctx, span := otel.Tracer(telemetryScope).Start(ctx, evaluateSpan, trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrRule, rule)))
	if ctx.Err() != nil {
		endEvaluation(span, actionCanceled, "")
		return "", fmt.Errorf("policy evaluation canceled: %w: %w", ports.ErrPolicyEvaluationFailed, ctx.Err())
	}
	evalCtx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()

	value, err := ast.InterfaceToValue(map[string]any(input))
	if err != nil {
		endEvaluation(span, actionError, errorTypeEvaluationFailed)
		return "", fmt.Errorf("policy input conversion failed: %w", ports.ErrPolicyEvaluationFailed)
	}
	results, err := q.query.Eval(evalCtx, rego.EvalParsedInput(value))
	if err != nil {
		if ctx.Err() != nil {
			endEvaluation(span, actionCanceled, "")
			return "", fmt.Errorf("policy evaluation canceled: %w: %w", ports.ErrPolicyEvaluationFailed, ctx.Err())
		}
		endEvaluation(span, actionError, errorTypeEvaluationFailed)
		return "", fmt.Errorf("policy evaluation failed: %w", ports.ErrPolicyEvaluationFailed)
	}
	action, ok := parseDecision(results)
	if !ok {
		endEvaluation(span, actionInvalid, errorTypeDecisionInvalid)
		return "", fmt.Errorf("policy decision is not allow or deny: %w", ports.ErrPolicyDecisionInvalid)
	}
	if action == ports.ImpersonationPolicyAllow {
		endEvaluation(span, actionAllow, "")
	} else {
		endEvaluation(span, actionDeny, "")
	}
	return action, nil
}

// parseDecision accepts exactly one result whose value is an object with a string action of allow
// or deny. Other members are ignored.
func parseDecision(results rego.ResultSet) (ports.ImpersonationPolicyAction, bool) {
	if len(results) != 1 || len(results[0].Expressions) != 1 {
		return "", false
	}
	decision, ok := results[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return "", false
	}
	action, ok := decision["action"].(string)
	if !ok {
		return "", false
	}
	switch ports.ImpersonationPolicyAction(action) {
	case ports.ImpersonationPolicyAllow, ports.ImpersonationPolicyDeny:
		return ports.ImpersonationPolicyAction(action), true
	default:
		return "", false
	}
}
