package impersonation

import (
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type SubjectCategory string

const (
	SubjectCategoryHuman SubjectCategory = "human"
	SubjectCategoryAgent SubjectCategory = "agent"
)

var ErrInvalidAllowedSubjectCategories = errors.New("allowed_subject_categories must contain 1–2 distinct human/agent strings")

// ParseAllowedSubjectCategories is the single eligibility rule for allowed_subject_categories;
// omission (nil) means human only.
func ParseAllowedSubjectCategories(categories []string) (allowsHuman, allowsAgent bool, err error) {
	if categories == nil {
		return true, false, nil
	}
	if len(categories) < 1 || len(categories) > 2 {
		return false, false, ErrInvalidAllowedSubjectCategories
	}
	for _, category := range categories {
		switch SubjectCategory(category) {
		case SubjectCategoryHuman:
			if allowsHuman {
				return false, false, ErrInvalidAllowedSubjectCategories
			}
			allowsHuman = true
		case SubjectCategoryAgent:
			if allowsAgent {
				return false, false, ErrInvalidAllowedSubjectCategories
			}
			allowsAgent = true
		default:
			return false, false, ErrInvalidAllowedSubjectCategories
		}
	}
	return allowsHuman, allowsAgent, nil
}

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
