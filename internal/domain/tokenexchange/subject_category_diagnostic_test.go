package tokenexchange

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubjectCategoryDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		detail FailureDetail
		stage  FailureStage
		kind   ExchangeKind
	}{
		{"subject_category_ineligible", StageClientAuthorization, ExchangeImpersonation},
		{"subject_category_unsupported", StageIdentityResolution, ExchangeThirdParty},
	} {
		t.Run(string(tc.detail), func(t *testing.T) {
			d := NewDiagnostic(tc.stage, tc.detail).WithExchangeKind(tc.kind)
			require.Equal(t, tc.detail, d.Detail())
			require.Equal(t, tc.stage, d.Stage())
			require.Equal(t, OutcomeAuthorizationDenied, d.Outcome())
			require.Equal(t, RecoveryNone, d.RecoveryAction())
			require.Equal(t, TargetNone, d.RecoveryTarget())
			require.Equal(t, tc.kind, d.ExchangeKind())
		})
	}
}
