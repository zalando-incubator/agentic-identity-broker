package impersonation

import (
	"context"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

// Impersonation reuses the RFC 8693 TokenExchangeError envelope so the HTTP layer maps codes and
// statuses uniformly. Token values, key material, and trust internals are never placed in errors
// (FR-011). The constructors below centralize the impersonation error taxonomy.

// invalidRequest builds an invalid_request (400) error for malformed/missing/repeated parameters,
// wrong type parameters, present resource, requested_token_type mismatch, or invalid actor/subject
// credentials (FR-011).
func invalidRequest(description, details string) *tokenexchange.TokenExchangeError {
	return tokenexchange.NewInvalidRequestErrorWithDetails(description, details).
		WithDiagnostic(impersonationDiagnostic(tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed))
}

// invalidScope builds an invalid_scope (400) error without exposing the requested scope value.
func invalidScope(description, details string) *tokenexchange.TokenExchangeError {
	return tokenexchange.NewInvalidScopeErrorWithDetails(description, details).
		WithDiagnostic(impersonationDiagnostic(tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed))
}

// invalidClient builds an invalid_client (401) error for a client assertion that fails
// validation or that no rule trusts (FR-011).
func invalidClient(description, details string) *tokenexchange.TokenExchangeError {
	return tokenexchange.NewInvalidClientErrorWithDetails(description, details).
		WithDiagnostic(impersonationDiagnostic(tokenexchange.StageClientValidation, tokenexchange.DetailClientInvalid))
}

// accessDenied builds an access_denied (403) error for a request that validated all credentials
// but that no rule's authorization predicate permitted (FR-011).
func accessDenied(description, details string) *tokenexchange.TokenExchangeError {
	return tokenexchange.NewAccessDeniedErrorWithDetails(description, details).
		WithDiagnostic(impersonationDiagnostic(tokenexchange.StageClientAuthorization, tokenexchange.DetailClientPolicyDenied))
}

const consentRequiredDescription = "user delegation is required before impersonation"

// consentRequired builds a credential-free access_denied error directing the user to the
// existing consent-management page for the target agent.
func consentRequired(consentURL, details string) *tokenexchange.TokenExchangeError {
	return accessDenied(consentRequiredDescription, details).WithErrorURI(consentURL)
}

// serverError builds a server_error (500) for fail-closed internal failures (FR-011).
func serverError(description, details string) *tokenexchange.TokenExchangeError {
	return tokenexchange.NewServerErrorWithDetails(description, details).
		WithDiagnostic(impersonationDiagnostic(tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified))
}

func impersonationDiagnostic(stage tokenexchange.FailureStage, detail tokenexchange.FailureDetail) tokenexchange.Diagnostic {
	return tokenexchange.NewDiagnostic(stage, detail).WithExchangeKind(tokenexchange.ExchangeImpersonation)
}

func originError(ctx context.Context, envelope *tokenexchange.TokenExchangeError, cause error, stage tokenexchange.FailureStage, detail tokenexchange.FailureDetail) *tokenexchange.TokenExchangeError {
	if ctx.Err() != nil {
		detail = tokenexchange.DetailCallerCanceled
		cause = errors.Join(cause, ctx.Err())
	}
	return envelope.WithCause(cause).WithDiagnostic(impersonationDiagnostic(stage, detail))
}

// noMatchPrecedence is the deterministic, order-independent precedence applied when no rule
// matches (FR-004a): access_denied > invalid_request > invalid_client.
var noMatchPrecedence = map[string]int{
	tokenexchange.AccessDeniedError:   3,
	tokenexchange.InvalidRequestError: 2,
	tokenexchange.InvalidClientError:  1,
}

// selectNoMatchError chooses the highest-precedence error among the per-rule failures. When the
// list is empty or carries no recognized code, it fails closed with server_error.
func selectNoMatchError(failures []*tokenexchange.TokenExchangeError) *tokenexchange.TokenExchangeError {
	var best *tokenexchange.TokenExchangeError
	bestRank := 0
	for _, failure := range failures {
		if failure == nil {
			continue
		}
		if rank := noMatchPrecedence[failure.Code()]; rank > bestRank {
			best = failure
			bestRank = rank
		}
	}
	if best == nil {
		return serverError("impersonation request could not be authorized", "no_rule_matched")
	}
	return best
}
