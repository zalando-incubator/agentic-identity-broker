package servicescope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutcomeReason(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome Outcome
		want    OutcomeReason
	}{
		{"not selected precedes usability", Outcome{Usability: UsabilityUnverifiableTransient}, OutcomeReasonNotSelected},
		{"coverage precedes usability", Outcome{Selected: true, Usability: UsabilityMissing}, OutcomeReasonRequiredScopesUncovered},
		{"missing", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityMissing}, OutcomeReasonMissing},
		{"definitively unusable", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityUnusableDefinitive}, OutcomeReasonUnusableDefinitive},
		{"unverifiable", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityUnverifiableTransient}, OutcomeReasonUnverifiableTransient},
		{"usable", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityUsable}, OutcomeReasonUsable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.outcome.Reason())
		})
	}
}
