package impersonation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/stretchr/testify/require"
)

type ledgerImpersonationIssuer struct {
	mint func(context.Context, ports.ImpersonationMintInput) (string, error)
}

func (i ledgerImpersonationIssuer) IssueImpersonationToken(ctx context.Context, input ports.ImpersonationMintInput) (string, error) {
	return i.mint(ctx, input)
}

func newLedgerImpersonation(t *testing.T, expression string, issuer ports.ImpersonationTokenIssuer) (*Service, *ledgerfixture.Store, *Request, *Target) {
	t.Helper()
	key, set := signedValidationKey(t, "ledger")
	config := testImpersonationConfig("first", "second")
	for i := range config.Rules {
		config.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
		config.Rules[i].Authorization.CEL.Expression = expression
	}
	svc, err := NewService(config, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return configurableJWKSProvider{set: set}, nil
	}, stubAgentRepository{}, issuer, 0, nil, allowDelegationVerifier{}, "https://broker.example", ledgerfixture.NewRecorder())
	require.NoError(t, err)
	store := &ledgerfixture.Store{}
	svc.ledger = store.Recorder(t)
	token := signedValidationTokenWithSubject(t, jwa.ES256(), key, "https://idp.example.com", "aud", "ledger-user", time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	return svc, store, &Request{ClientAssertion: token, ActorToken: token, SubjectToken: token, SubjectTokenType: JWTTokenType}, testTarget()
}

func TestImpersonationLedgerGrantedDecisionCommitsBeforeMint(t *testing.T) {
	var store *ledgerfixture.Store
	issuer := ledgerImpersonationIssuer{mint: func(context.Context, ports.ImpersonationMintInput) (string, error) {
		require.Len(t, store.Events, 1, "minting must start after the permission fact commits")
		require.Equal(t, model.BusinessEventTypePrefix+"impersonation-granted", store.Events[0].Type)
		return "issued-token-canary", nil
	}}
	svc, records, request, target := newLedgerImpersonation(t, "true", issuer)
	store = records
	outcome, err := svc.Impersonate(context.Background(), request, target)
	require.NoError(t, err)
	require.Equal(t, "issued-token-canary", outcome.Response.AccessToken)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"token-exchanged", store.Events[1].Type)
	require.Equal(t, target.Agent.ID, store.Events[1].AgentID)
}

func TestImpersonationLedgerMintFailureDoesNotUndoGrantedDecision(t *testing.T) {
	issuer := ledgerImpersonationIssuer{mint: func(context.Context, ports.ImpersonationMintInput) (string, error) {
		return "", errors.New("mint-error-canary")
	}}
	svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
	outcome, err := svc.Impersonate(context.Background(), request, target)
	require.Error(t, err)
	require.Nil(t, outcome.Response)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"impersonation-granted", store.Events[0].Type)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[1].Type)
}

func TestImpersonationLedgerFinalPolicyDenialIsNotPerCandidate(t *testing.T) {
	issuer := &stubIssuer{}
	svc, store, request, target := newLedgerImpersonation(t, "false", issuer)
	_, err := svc.Impersonate(context.Background(), request, target)
	require.Error(t, err)
	require.False(t, issuer.called)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"impersonation-denied", store.Events[0].Type)
	require.Equal(t, model.BusinessEventTypePrefix+"token-exchange-denied", store.Events[1].Type)
}

func TestImpersonationLedgerInternalErrorIsNotPermissionDenial(t *testing.T) {
	issuer := &stubIssuer{}
	svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
	svc.delegationVerifier = &recordingDelegationVerifier{err: errors.New("lookup-error-canary")}
	_, err := svc.Impersonate(context.Background(), request, target)
	require.Error(t, err)
	require.False(t, issuer.called)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
}

func TestImpersonationLedgerGrantRecordingFailurePreventsMint(t *testing.T) {
	issuer := &stubIssuer{}
	svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
	store.AppendError = errors.New("ledger unavailable")
	outcome, err := svc.Impersonate(context.Background(), request, target)
	require.Error(t, err)
	require.Nil(t, outcome.Response)
	require.False(t, issuer.called)
	require.Empty(t, store.Events)
}
