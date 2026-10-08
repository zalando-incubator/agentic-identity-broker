package impersonation

import (
	"context"
	"errors"
	"fmt"
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
	mintErr := errors.New("mint-error-canary")
	issuer := ledgerImpersonationIssuer{mint: func(context.Context, ports.ImpersonationMintInput) (string, error) {
		return "", mintErr
	}}
	svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
	outcome, err := svc.Impersonate(context.Background(), request, target)
	var failure *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &failure)
	require.ErrorIs(t, err, mintErr)
	require.Equal(t, tokenexchange.StageExchangeRouting, failure.Diagnostic().Stage())
	require.Equal(t, tokenexchange.DetailInternalUnclassified, failure.Diagnostic().Detail())
	require.Equal(t, tokenexchange.OutcomeInfrastructureError, failure.Diagnostic().Outcome())
	require.Equal(t, tokenexchange.ExchangeImpersonation, failure.Diagnostic().ExchangeKind())
	require.NotContains(t, failure.Error(), "mint-error-canary")
	require.Nil(t, outcome.Response)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"impersonation-granted", store.Events[0].Type)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[1].Type)
	require.Equal(t, "internal_failure", store.Events[1].Data["reason_code"])
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

func TestImpersonationLedgerMissingOrExpiredDelegationPreservesDecision(t *testing.T) {
	for _, test := range []struct {
		name   string
		status ports.UserDelegationStatus
		detail tokenexchange.FailureDetail
	}{
		{"missing", ports.UserDelegationMissing, tokenexchange.DetailGrantMissing},
		{"expired", ports.UserDelegationExpired, tokenexchange.DetailGrantExpired},
	} {
		t.Run(test.name, func(t *testing.T) {
			issuer := &stubIssuer{}
			svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
			svc.delegationVerifier = &recordingDelegationVerifier{status: test.status}
			outcome, err := svc.Impersonate(context.Background(), request, target)
			var failure *tokenexchange.TokenExchangeError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, tokenexchange.AccessDeniedError, failure.Code())
			require.Equal(t, test.detail, failure.Diagnostic().Detail())
			require.Equal(t, tokenexchange.StageGrantAuthorization, failure.Diagnostic().Stage())
			require.Equal(t, tokenexchange.ExchangeImpersonation, failure.Diagnostic().ExchangeKind())
			require.Nil(t, outcome.Response)
			require.False(t, issuer.called)
			require.Len(t, store.Events, 2)
			require.Equal(t, model.BusinessEventTypePrefix+"impersonation-denied", store.Events[0].Type)
			require.Equal(t, "delegation_missing", store.Events[0].Data["reason_code"])
			require.Equal(t, model.BusinessEventTypePrefix+"token-exchange-denied", store.Events[1].Type)
			require.Equal(t, "authorization_failed", store.Events[1].Data["reason_code"])
			require.Nil(t, store.Events[0].Actor.OnBehalfOf)
		})
	}
}

func TestImpersonationLedgerExchangeRecordingFailureWithholdsMintedToken(t *testing.T) {
	var store *ledgerfixture.Store
	minted := false
	recordingErr := errors.New("exchange recording unavailable")
	issuer := ledgerImpersonationIssuer{mint: func(context.Context, ports.ImpersonationMintInput) (string, error) {
		require.Len(t, store.Events, 1)
		require.Equal(t, model.BusinessEventTypePrefix+"impersonation-granted", store.Events[0].Type)
		minted = true
		store.AppendError = recordingErr
		return "issued-token-canary", nil
	}}
	svc, records, request, target := newLedgerImpersonation(t, "true", issuer)
	store = records
	outcome, err := svc.Impersonate(context.Background(), request, target)
	var failure *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "server_error", failure.Code())
	require.ErrorIs(t, err, recordingErr)
	require.Equal(t, "failed to record impersonation outcome", failure.Description())
	require.Equal(t, tokenexchange.StageExchangeRouting, failure.Diagnostic().Stage())
	require.Equal(t, tokenexchange.DetailInternalUnclassified, failure.Diagnostic().Detail())
	require.Equal(t, tokenexchange.OutcomeInfrastructureError, failure.Diagnostic().Outcome())
	require.Equal(t, tokenexchange.ExchangeImpersonation, failure.Diagnostic().ExchangeKind())
	require.True(t, minted)
	require.Nil(t, outcome.Response)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"impersonation-granted", store.Events[0].Type)
}

func TestImpersonationLedgerScopeRejectionBypassesRecording(t *testing.T) {
	for _, ledgerDown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ledger-available", true: "ledger-unavailable"}[ledgerDown], func(t *testing.T) {
			issuer := &stubIssuer{}
			svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
			target.Agent.AllowedScopes = []string{"read"}
			request.Scope, request.Scopes = "admin", []string{"admin"}
			request.ClientAssertion, request.ActorToken, request.SubjectToken = "invalid-client", "invalid-actor", "invalid-subject"
			if ledgerDown {
				store.AppendError = errors.New("ledger unavailable")
			}
			outcome, err := svc.Impersonate(context.Background(), request, target)
			var failure *tokenexchange.TokenExchangeError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, "invalid_scope", failure.Code())
			require.Equal(t, tokenexchange.StageRequestValidation, failure.Diagnostic().Stage())
			require.Equal(t, tokenexchange.DetailRequestMalformed, failure.Diagnostic().Detail())
			require.Equal(t, tokenexchange.ExchangeImpersonation, failure.Diagnostic().ExchangeKind())
			require.Nil(t, outcome.Response)
			require.False(t, issuer.called)
			require.Empty(t, outcome.Audit.SelectedRule)
			require.Empty(t, store.Events)
		})
	}
}

func TestImpersonationLedgerJWKSFailureIsNotPermissionDenial(t *testing.T) {
	issuer := &stubIssuer{}
	svc, store, request, target := newLedgerImpersonation(t, "true", issuer)
	cause := errors.New("jwks-secret-canary")
	for _, rule := range svc.rules {
		for _, trusted := range rule.issuers {
			trusted.validator.jwksProvider = configurableJWKSProvider{err: cause}
		}
	}
	outcome, err := svc.Impersonate(context.Background(), request, target)
	var failure *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &failure)
	require.ErrorIs(t, err, cause)
	require.Equal(t, tokenexchange.InvalidClientError, failure.Code())
	require.Equal(t, tokenexchange.DetailJWKSUnavailable, failure.Diagnostic().Detail())
	require.Equal(t, tokenexchange.OutcomeInfrastructureError, failure.Diagnostic().Outcome())
	require.NotContains(t, failure.Error(), cause.Error())
	require.Nil(t, outcome.Response)
	require.False(t, issuer.called)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	require.Equal(t, "internal_failure", store.Events[0].Data["reason_code"])
	require.Nil(t, store.Events[0].Subject)
	require.Nil(t, store.Events[0].Actor.ID)
	require.Nil(t, store.Events[0].Actor.OnBehalfOf)
}

func TestImpersonationLedgerMalformedAudienceBypassesRecording(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		for _, suffix := range []string{"", "/", "/not a canonical id", "/00000000-0000-4000-8000-00000000000A", "/target/extra"} {
			t.Run(fmt.Sprintf("unavailable=%t/suffix=%s", unavailable, suffix), func(t *testing.T) {
				svc, store, _, _ := newLedgerImpersonation(t, "true", &stubIssuer{})
				if unavailable {
					store.AppendError = errors.New("ledger unavailable")
				}
				target, activated, err := svc.ResolveTarget(context.Background(), []string{svc.audiencePrefix + suffix})
				var failure *tokenexchange.TokenExchangeError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, tokenexchange.InvalidRequestError, failure.Code())
				require.Equal(t, 400, failure.HTTPStatus())
				require.Equal(t, tokenexchange.DetailRequestMalformed, failure.Diagnostic().Detail())
				require.True(t, activated)
				require.Nil(t, target)
				require.Empty(t, store.Events)
			})
		}
	}
}
