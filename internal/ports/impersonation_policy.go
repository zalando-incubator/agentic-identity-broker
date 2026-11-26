package ports

import (
	"context"
	"errors"
	"time"
)

// ImpersonationPolicyAction is a valid impersonation policy decision.
type ImpersonationPolicyAction string

// Impersonation policy decisions. Every other decision document is invalid.
const (
	ImpersonationPolicyAllow ImpersonationPolicyAction = "allow"
	ImpersonationPolicyDeny  ImpersonationPolicyAction = "deny"
)

// ImpersonationPolicyInput is the OPA input document of one impersonation decision. It excludes
// raw credentials but contains sensitive identity claims, so it renders only as a constant.
type ImpersonationPolicyInput map[string]any

// ImpersonationPolicyDecisionRef names one rule's decision in the shared engine.
type ImpersonationPolicyDecisionRef struct {
	Rule              string
	Package           string
	Decision          string
	EvaluationTimeout time.Duration
}

// ImpersonationPolicyRevision is an immutable admitted snapshot holding one prepared query per
// decision reference. It is safe for concurrent use.
type ImpersonationPolicyRevision interface {
	// Label is the operator manifest label, for logs and audit only.
	Label() string
	// AdmissionID identifies this snapshot within the process instance, for logs and audit only.
	AdmissionID() string
	// Evaluate runs the rule's decision and returns allow or deny, or an error wrapping
	// ErrPolicyDecisionInvalid or ErrPolicyEvaluationFailed. It honors ctx and the rule's
	// evaluation timeout.
	Evaluate(ctx context.Context, rule string, input ImpersonationPolicyInput) (ImpersonationPolicyAction, error)
}

// ImpersonationPolicySource owns the one shared impersonation OPA engine.
type ImpersonationPolicySource interface {
	// AwaitReady blocks until an admitted revision containing every configured bundle is active,
	// or returns when ctx ends or the engine fails.
	AwaitReady(ctx context.Context) error
	// Current returns the active revision; it is non-nil after AwaitReady succeeds.
	Current() ImpersonationPolicyRevision
	Stop(ctx context.Context)
}

// ImpersonationPolicyAdmission decides whether a candidate revision may become active. It
// receives, per rule name, whether that rule's decision rule exists in the candidate. A non-nil
// error rejects the whole candidate.
type ImpersonationPolicyAdmission func(decisionPresent map[string]bool) error

// ImpersonationPolicyAdmissionError names the first rule that rejected a candidate.
type ImpersonationPolicyAdmissionError struct {
	Rule string
	Err  error
}

func (e *ImpersonationPolicyAdmissionError) Error() string {
	return "rule " + e.Rule + ": " + e.Err.Error()
}

func (e *ImpersonationPolicyAdmissionError) Unwrap() error { return e.Err }

// Impersonation policy evaluation failures.
var (
	ErrPolicyDecisionInvalid  = errors.New("impersonation policy decision invalid")
	ErrPolicyEvaluationFailed = errors.New("impersonation policy evaluation failed")
)
