package impersonation

import (
	"context"
	"net/url"

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
	http                                     RequestAttributes
	agent                                    *AgentContext
}

// Authorization engine values, recorded as the audit authorization_engine.
const (
	engineCEL = "cel"
	engineOPA = "opa"
)

// opaAuthorization is an OPA rule's decision on the revision the request read before its first
// rule. The operator's policy is the only control on caller-asserted subject values, so the rule
// may use the unverified subject's sub and email.
type opaAuthorization struct {
	rule      string
	revision  ports.ImpersonationPolicyRevision
	publicURL *url.URL
}

func (a opaAuthorization) label() string { return a.rule }

func (a opaAuthorization) engine() string { return engineOPA }

func (a opaAuthorization) bindsSubjectField(string) bool { return true }

func (a opaAuthorization) evaluate(ctx context.Context, in policyInputs) (bool, error) {
	action, err := a.revision.Evaluate(ctx, a.rule, buildPolicyInput(in, a.publicURL))
	if err != nil {
		return false, err
	}
	return action == ports.ImpersonationPolicyAllow, nil
}
