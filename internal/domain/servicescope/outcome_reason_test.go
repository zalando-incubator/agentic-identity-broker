package servicescope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutcomeReason(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome Outcome
		want    Reason
	}{
		{"not selected precedes usability", Outcome{Usability: UsabilityUnverifiableTransient}, ReasonNotSelected},
		{"coverage precedes usability", Outcome{Selected: true, Usability: UsabilityMissing}, ReasonRequiredScopesUncovered},
		{"missing", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityMissing}, "missing"},
		{"definitively unusable", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityUnusableDefinitive}, "unusable_definitive"},
		{"unverifiable", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityUnverifiableTransient}, ReasonUnverifiableTransient},
		{"usable", Outcome{Selected: true, RequiredScopesCovered: true, Usability: UsabilityUsable}, "usable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.outcome.Reason())
		})
	}
}
