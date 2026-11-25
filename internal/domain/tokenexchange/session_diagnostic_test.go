package tokenexchange

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/stretchr/testify/assert"
)

func TestSessionDiagnosticIdentityRecovery(t *testing.T) {
	for _, operation := range []oauth2session.Operation{oauth2session.OperationSessionLookup, oauth2session.OperationRefresh} {
		for _, reason := range []model.CredentialSourceReason{
			model.CredentialSourceReasonIdentityMissing,
			model.CredentialSourceReasonIdentityMismatch,
			model.CredentialSourceReasonIdentitySuperseded,
			model.CredentialSourceReasonNotFound,
			model.CredentialSourceReasonReadFailed,
			model.CredentialSourceReasonGenerationChanged,
		} {
			t.Run(string(operation)+"/"+string(reason), func(t *testing.T) {
				err := oauth2session.NewOperationError(oauth2session.NewErrorMetadata(operation, oauth2session.DetailCredentialSourceUnavailable), model.NewCredentialSourceError(reason))
				diagnostic := sessionDiagnostic(err)
				if reason == model.CredentialSourceReasonIdentityMissing || reason == model.CredentialSourceReasonIdentityMismatch || reason == model.CredentialSourceReasonIdentitySuperseded {
					assert.Equal(t, DetailCredentialIdentityChanged, diagnostic.Detail())
					assert.Equal(t, OutcomeReauthRequired, diagnostic.Outcome())
					assert.Equal(t, RecoveryReauthenticate, diagnostic.RecoveryAction())
					assert.Equal(t, TargetProviderSession, diagnostic.RecoveryTarget())
				} else {
					assert.Equal(t, DetailCredentialSourceUnavailable, diagnostic.Detail())
					assert.Equal(t, OutcomeInfrastructureError, diagnostic.Outcome())
					assert.Equal(t, RecoveryRetry, diagnostic.RecoveryAction())
					assert.Equal(t, TargetNone, diagnostic.RecoveryTarget())
				}
			})
		}
	}
}
