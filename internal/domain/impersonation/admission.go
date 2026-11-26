package impersonation

import (
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ErrPolicyDecisionAbsent rejects a candidate revision that has no rule for an OPA rule's
// decision (decision_absent).
var ErrPolicyDecisionAbsent = errors.New("decision_absent")

// newOPAAdmission admits a candidate revision only when every OPA rule's decision rule exists.
// A rejection applies to all rules and names the first failing rule in rule order. OPA rules have
// no reference or field-binding requirement.
func newOPAAdmission(rules []string) ports.ImpersonationPolicyAdmission {
	ordered := append([]string(nil), rules...)
	return func(decisionPresent map[string]bool) error {
		for _, rule := range ordered {
			if !decisionPresent[rule] {
				return &ports.ImpersonationPolicyAdmissionError{Rule: rule, Err: ErrPolicyDecisionAbsent}
			}
		}
		return nil
	}
}
