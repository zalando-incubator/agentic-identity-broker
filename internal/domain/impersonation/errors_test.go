package impersonation

import (
	"context"
	"errors"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

func TestSelectNoMatchError_Precedence(t *testing.T) {
	ad := accessDenied("denied")
	ic := invalidClient("bad client")
	ir := invalidRequest("bad actor")

	// access_denied outranks invalid_client and invalid_request, order-independently.
	assert.Equal(t, tokenexchange.AccessDeniedError, selectNoMatchError([]*tokenexchange.TokenExchangeError{ir, ic, ad}).Code())
	assert.Equal(t, tokenexchange.AccessDeniedError, selectNoMatchError([]*tokenexchange.TokenExchangeError{ad, ir, ic}).Code())

	// invalid_request outranks invalid_client.
	assert.Equal(t, tokenexchange.InvalidRequestError, selectNoMatchError([]*tokenexchange.TokenExchangeError{ir, ic}).Code())

	// invalid_request alone.
	assert.Equal(t, tokenexchange.InvalidRequestError, selectNoMatchError([]*tokenexchange.TokenExchangeError{ir}).Code())

	// empty → server_error fallback (fail closed).
	assert.Equal(t, tokenexchange.ServerErrorCode, selectNoMatchError(nil).Code())
}

func TestImpersonationErrorDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    *tokenexchange.TokenExchangeError
		stage  tokenexchange.FailureStage
		detail tokenexchange.FailureDetail
	}{
		{"request", invalidRequest("description"), tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed},
		{"scope", invalidScope("description"), tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed},
		{"client", invalidClient("description"), tokenexchange.StageClientValidation, tokenexchange.DetailClientInvalid},
		{"policy", accessDenied("description"), tokenexchange.StageClientAuthorization, tokenexchange.DetailClientPolicyDenied},
		{"unclassified", serverError("description"), tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := test.err.Diagnostic()
			assert.Equal(t, test.stage, diagnostic.Stage())
			assert.Equal(t, test.detail, diagnostic.Detail())
			assert.Equal(t, tokenexchange.ExchangeImpersonation, diagnostic.ExchangeKind())
		})
	}
}

func TestImpersonationOriginErrorPreservesCauseAndCallerCancellation(t *testing.T) {
	cause := errors.New("SECRET_CAUSE_SENTINEL")
	base := serverError("description")
	err := originError(context.Background(), base, cause, tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantRepositoryUnavailable)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, tokenexchange.DetailGrantRepositoryUnavailable, err.Diagnostic().Detail())
	assert.Equal(t, tokenexchange.DetailInternalUnclassified, base.Diagnostic().Detail(), "enrichment must not mutate the base error")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := originError(ctx, base, cause, tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantRepositoryUnavailable)
	assert.ErrorIs(t, canceled, cause)
	assert.ErrorIs(t, canceled, context.Canceled)
	assert.Equal(t, tokenexchange.OutcomeCanceled, canceled.Diagnostic().Outcome())
	assert.Equal(t, tokenexchange.DetailCallerCanceled, canceled.Diagnostic().Detail())
	assert.Equal(t, tokenexchange.ExchangeImpersonation, canceled.Diagnostic().ExchangeKind())
}

func TestImpersonationCredentialDiagnosticsPreserveOrigins(t *testing.T) {
	for _, test := range []struct {
		name   string
		stage  tokenexchange.FailureStage
		cause  error
		detail tokenexchange.FailureDetail
	}{
		{"invalid client", tokenexchange.StageClientValidation, errors.New("invalid signature"), tokenexchange.DetailClientInvalid},
		{"expired client", tokenexchange.StageClientValidation, jwt.TokenExpiredError{}, tokenexchange.DetailClientExpired},
		{"invalid subject", tokenexchange.StageSubjectValidation, errors.New("invalid signature"), tokenexchange.DetailSubjectInvalid},
		{"expired subject", tokenexchange.StageSubjectValidation, jwt.TokenExpiredError{}, tokenexchange.DetailSubjectExpired},
		{"client keys unavailable", tokenexchange.StageClientValidation, tokenexchange.NewServerError("keys unavailable").WithDiagnostic(impersonationDiagnostic(tokenexchange.StageSubjectValidation, tokenexchange.DetailJWKSUnavailable)), tokenexchange.DetailJWKSUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			envelope := invalidRequest("description")
			if test.stage == tokenexchange.StageClientValidation {
				envelope = invalidClient("description")
			}
			err := credentialError(context.Background(), envelope, test.cause, test.stage)
			assert.ErrorIs(t, err, test.cause)
			assert.Equal(t, test.stage, err.Diagnostic().Stage())
			assert.Equal(t, test.detail, err.Diagnostic().Detail())
			assert.Equal(t, tokenexchange.ExchangeImpersonation, err.Diagnostic().ExchangeKind())
			assert.Equal(t, envelope.Code(), err.Code(), "origin enrichment must preserve OAuth protocol semantics")
		})
	}
}
