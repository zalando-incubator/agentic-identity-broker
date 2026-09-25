package authorization

const (
	ActionAllow            = "allow"
	ActionDeny             = "deny"
	ActionApprovalRequired = "approval_required"
	ActionCIBARequired     = "ciba_required"
)

type ApprovalContext struct {
	Description string `json:"description,omitempty"`
	RiskLevel   string `json:"risk_level,omitempty"`
}

// OPADecision represents the structured result of OPA policy evaluation.
type OPADecision struct {
	Action          string           `json:"action"`
	Reasons         []string         `json:"reasons,omitempty"`
	ApprovalContext *ApprovalContext `json:"approval_context,omitempty"`
}

// ParseDecision extracts a fail-closed decision from an OPA result object.
func ParseDecision(result map[string]any) *OPADecision {
	if result == nil {
		return &OPADecision{Action: ActionDeny, Reasons: []string{"undefined result"}}
	}
	action, ok := result["action"].(string)
	if !ok {
		return &OPADecision{Action: ActionDeny, Reasons: []string{"undefined result"}}
	}
	switch action {
	case ActionAllow:
		return &OPADecision{Action: ActionAllow}
	case ActionApprovalRequired:
		decision := &OPADecision{Action: ActionApprovalRequired}
		if raw, ok := result["approval_context"].(map[string]any); ok {
			decision.ApprovalContext = &ApprovalContext{}
			decision.ApprovalContext.Description, _ = raw["description"].(string)
			decision.ApprovalContext.RiskLevel, _ = raw["risk_level"].(string)
		}
		return decision
	case ActionDeny, ActionCIBARequired:
		decision := &OPADecision{Action: ActionDeny}
		if action == ActionCIBARequired {
			decision.Reasons = []string{ActionCIBARequired + " is not yet supported"}
			return decision
		}
		if raw, ok := result["reasons"].([]any); ok {
			for _, reason := range raw {
				if reason, ok := reason.(string); ok {
					decision.Reasons = append(decision.Reasons, reason)
				}
			}
		}
		return decision
	default:
		return &OPADecision{Action: ActionDeny, Reasons: []string{"unknown action: " + action}}
	}
}
