package oauth2session

import (
	"context"
	"net/http"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientIdentitySourceFailureEventsCoverInitiationCallbackAndDisplayFallback(t *testing.T) {
	for _, operation := range []string{"authorization_initiation", "code_exchange", "usability_display"} {
		for _, reason := range []model.CredentialSourceReason{model.CredentialSourceReasonMissingBinding, model.CredentialSourceReasonGenerationChanged, model.CredentialSourceReasonNotFound, model.CredentialSourceReasonPermissionDenied, model.CredentialSourceReasonNotRegular, model.CredentialSourceReasonTooLarge, model.CredentialSourceReasonEmpty, model.CredentialSourceReasonReadFailed} {
			t.Run(operation+"/"+string(reason), func(t *testing.T) {
				service, provider, session, store, logs := newIdentityCallbackFixture(t, "filesystem")
				service.credentialFileReader = enforcementCredentialReader{err: model.NewCredentialSourceError(reason)}
				calls := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
				})
				before, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				switch operation {
				case "authorization_initiation":
					flow, flowErr := service.InitiateOAuth2Flow(context.Background(), session.Principal, session.ServiceID, "https://broker.example.com/sessions")
					assert.Nil(t, flow)
					err = flowErr
				case "code_exchange":
					revision := provider.ClientIdentityRevision
					state := identityCallbackState(t, service, provider, session, session.UpstreamClientID, &revision)
					result, callbackErr := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: session.ServiceID, Code: "identity-code-sentinel", State: state})
					assert.Nil(t, result)
					err = callbackErr
				case "usability_display":
					result, displayErr := service.RequiresReauthentication(context.Background(), []*storage.UserSession{session, session}, ports.UsabilityDisplay)
					require.NoError(t, displayErr)
					assert.False(t, result[session.ServiceID], "matching persisted evidence remains usable for display only")
				}
				if operation != "usability_display" {
					var source *model.CredentialSourceError
					require.ErrorAs(t, err, &source)
					assert.Equal(t, reason, source.Reason)
				}
				assert.Zero(t, calls)
				after, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				assertIdentityFailureEvent(t, logs, provider.ID, operation, reason)
				assert.NotContains(t, logs.text(), "identity-code-sentinel")
			})
		}
	}
}

func TestClientIdentityDisconnectedEvaluationDoesNotLogAnExpectedResult(t *testing.T) {
	for _, mode := range []ports.UsabilityMode{ports.UsabilityGate, ports.UsabilityDisplay} {
		for _, reason := range []model.CredentialSourceReason{model.CredentialSourceReasonIdentityMissing, model.CredentialSourceReasonIdentityMismatch, model.CredentialSourceReasonIdentitySuperseded} {
			t.Run(string(reason)+"/"+map[ports.UsabilityMode]string{ports.UsabilityGate: "gate", ports.UsabilityDisplay: "display"}[mode], func(t *testing.T) {
				service, _, session, _, logs := newIdentityEnforcementFixture(t, "public")
				switch reason {
				case model.CredentialSourceReasonIdentityMissing:
					session.UpstreamClientID = nil
				case model.CredentialSourceReasonIdentityMismatch:
					session.UpstreamClientID = identityPointer("identity-previous-sentinel")
				case model.CredentialSourceReasonIdentitySuperseded:
					session.UpstreamClientRevision--
				}
				result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{session}, mode)
				require.NoError(t, err)
				assert.True(t, result[session.ServiceID])
				assert.Empty(t, logs.text(), "a disconnected boolean is not a failed token use or source fallback")
			})
		}
	}
}

func TestClientIdentityGateReturnsSourceFailureWithoutHelperLogging(t *testing.T) {
	service, _, session, _, logs := newIdentityEnforcementFixture(t, "filesystem")
	service.credentialFileReader = enforcementCredentialReader{err: model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)}
	result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{session}, ports.UsabilityGate)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
	assert.Empty(t, logs.text(), "the authorization or consent boundary owns the usability_gate denial")
}
