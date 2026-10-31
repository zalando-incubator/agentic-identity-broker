package tokenexchange

import "errors"

var (
	// ErrAgentSubjectToken marks the denial of a detected agent-subject token.
	ErrAgentSubjectToken = errors.New("agent-subject token")
	// ErrAgentSubjectDetection marks a failed agent_subject_expression evaluation.
	ErrAgentSubjectDetection = errors.New("agent-subject detection failed")
)

// ResolveHumanSubject resolves the human principal and agent ID a subject token speaks for.
// Agent-subject tokens are denied before agent extraction (ADR 045 decisions 6 and 6a), so
// no caller can treat one as a human principal. The principal is returned whenever it was
// extracted, including on failure, so callers can still attribute the request.
func (e *CELEvaluator) ResolveHumanSubject(subjectTokenClaims map[string]interface{}) (principal, agentID string, err error) {
	principal, err = e.ExtractPrincipal(subjectTokenClaims)
	if err != nil {
		return "", "", err
	}
	isAgent, err := e.IsAgentSubject(subjectTokenClaims)
	if err != nil {
		var exchangeErr *TokenExchangeError
		if errors.As(err, &exchangeErr) {
			return principal, "", exchangeErr.WithCause(errors.Join(ErrAgentSubjectDetection, exchangeErr.Unwrap()))
		}
		return principal, "", errors.Join(ErrAgentSubjectDetection, err)
	}
	if isAgent {
		return principal, "", NewAccessDeniedError("agent-subject tokens cannot be exchanged for third-party service tokens").
			WithDiagnostic(NewDiagnostic(StageIdentityResolution, DetailSubjectCategoryUnsupported)).
			WithCause(ErrAgentSubjectToken)
	}
	agentID, err = e.ExtractAgentID(subjectTokenClaims)
	if err != nil {
		return principal, "", err
	}
	return principal, agentID, nil
}
