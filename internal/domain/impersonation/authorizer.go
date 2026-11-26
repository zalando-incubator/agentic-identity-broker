package impersonation

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// PolicySourceFactory starts the shared impersonation OPA engine without blocking on readiness.
// It is supplied by the app builder so the domain starts no adapter itself (Principle VI).
type PolicySourceFactory func(cfg ports.ImpersonationOPAEngineConfig,
	decisions []ports.ImpersonationPolicyDecisionRef,
	admit ports.ImpersonationPolicyAdmission) (ports.ImpersonationPolicySource, error)

// authorization is one rule's authorization decision. CEL rules hold a static implementation
// compiled at startup.
type authorization interface {
	// label is the rule name, for diagnostics and audit.
	label() string
	// engine is the audit authorization_engine value.
	engine() string
	evaluate(ctx context.Context, in policyInputs) (bool, error)
	// bindsSubjectField reports whether the rule may use the unverified subject field ("sub" or
	// "email").
	bindsSubjectField(field string) bool
}

// policyInputs carries the validated request facts an authorization decision may use.
type policyInputs struct {
	clientClaims, actorClaims, subjectClaims map[string]any
	subjectUnverified                        bool
	request                                  map[string]any
}
