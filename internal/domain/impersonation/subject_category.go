package impersonation

import "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"

type SubjectCategory string

const (
	SubjectCategoryHuman SubjectCategory = "human"
	SubjectCategoryAgent SubjectCategory = "agent"
)

func classifySubject(unverified bool, claims map[string]interface{}, target id.AgentID) SubjectCategory {
	if subject, ok := claims["sub"].(string); !unverified && ok && subject == target.String() {
		return SubjectCategoryAgent
	}
	return SubjectCategoryHuman
}

func (r *compiledRule) allows(category SubjectCategory) bool {
	switch category {
	case SubjectCategoryHuman:
		return r.allowsHuman
	case SubjectCategoryAgent:
		return r.allowsAgent
	default:
		return false
	}
}
