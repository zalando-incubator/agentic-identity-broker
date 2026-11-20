package server

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
)

// Diagnostic is the standalone, immutable exchange classification contract.
// Broker stages and exchange kinds are never inferred from provider descriptions.
type Outcome string
type FailureStage string
type FailureDetail string
type RecoveryAction string
type RecoveryTarget string
type ExchangeKind string

const (
	// OutcomeSuccess means the checks and the exchange passed, or the client assertion was refreshed; a later
	// policy decision on a request body does not change it.
	OutcomeSuccess Outcome = "success"
	// OutcomeAuthorizationDenied means the OPA policy, the grant context, the broker or the OAuth provider refused
	// access; the failure detail tells which, and whether ExtProc returned a consent URL.
	OutcomeAuthorizationDenied Outcome = "authorization_denied"
	// OutcomeAuthenticationFailed means the broker or the OAuth provider rejected a credential, or ExtProc's own
	// client assertion had expired; the failure detail tells which.
	OutcomeAuthenticationFailed Outcome = "authentication_failed"
	// OutcomeReauthRequired means the broker answered `invalid_grant` with an `error_uri` for a new sign-in, or an
	// OAuth error had the code `reauth_required`; the failure detail tells whether the client got the URL.
	OutcomeReauthRequired Outcome = "reauth_required"
	// OutcomeInvalidRequest means ExtProc found the request's metadata, method or body malformed, or the broker or
	// the OAuth provider rejected the request it received as invalid.
	OutcomeInvalidRequest Outcome = "invalid_request"
	// OutcomeConfigurationError means ExtProc's configuration, or the metadata that the gateway sends for policy
	// evaluation, is unusable; fix it, because a retry fails the same way.
	OutcomeConfigurationError Outcome = "configuration_error"
	// OutcomeInfrastructureError means a dependency or ExtProc itself failed, such as an unreachable broker, an open
	// circuit breaker or an unavailable grant context; a retry may succeed.
	OutcomeInfrastructureError Outcome = "infrastructure_error"
	// OutcomeCanceled means the request was canceled, or its deadline passed, while ExtProc waited for the exchange or
	// sent its response; ExtProc does not set the error status on the span.
	OutcomeCanceled Outcome = "canceled"

	// StageNone means nothing failed; metrics of successful requests carry it, while successful spans and logs leave
	// out the failure stage.
	StageNone FailureStage = "none"
	// StageRequestValidation means ExtProc rejected the request itself: its token-exchange metadata, its HTTP method
	// or its body was missing or malformed.
	StageRequestValidation FailureStage = "request_validation"
	// StageExchangeRouting means the token exchange with the broker failed, including an open circuit breaker and a
	// canceled wait; ExtProc also uses it for an unknown stage.
	StageExchangeRouting FailureStage = "exchange_routing"
	// StageClientAuthorization means ExtProc's own authorization step failed or denied the request: the OPA policy,
	// the gateway metadata that it needs, or the grant context.
	StageClientAuthorization FailureStage = "client_authorization"
	// StageRefresh means ExtProc's own client assertion refresh failed, or an exchange found the assertion expired;
	// it is not the broker's refresh of a provider session.
	StageRefresh FailureStage = "refresh"
	// StageResponseWrite means ExtProc could not send its response to the gateway; only logs carry it, since the
	// span has already ended.
	StageResponseWrite FailureStage = "response_write"

	// DetailNone means nothing failed; metrics of successful requests and successful `extproc.grant_context.*` spans
	// carry it, while other successful spans and logs leave out the failure detail.
	DetailNone FailureDetail = "none"
	// DetailResourceMissing means the request's token-exchange metadata has no resource URI, or a blank one; check
	// that the gateway sets it, because ExtProc answers 503.
	DetailResourceMissing FailureDetail = "resource_missing"
	// DetailRequestMalformed means ExtProc found the request's metadata, method or body malformed, or the broker or
	// the OAuth provider answered `invalid_request`, `invalid_target`, `invalid_scope` or `unsupported_grant_type`.
	DetailRequestMalformed FailureDetail = "request_malformed"
	// DetailSubjectInvalid means the broker, or the OAuth provider during refresh, answered `invalid_token`; the
	// broker reports a rejected subject token as `invalid_grant`, which is `credential_rejected`.
	DetailSubjectInvalid FailureDetail = "subject_invalid"
	// DetailClientInvalid means the broker or the OAuth provider answered `invalid_client` or `unauthorized_client`:
	// it rejected ExtProc's client authentication, or ExtProc may not use that grant.
	DetailClientInvalid FailureDetail = "client_invalid"
	// DetailClientExpired means ExtProc's own client assertion had expired, so ExtProc answered 503 without calling
	// the broker; look for failed `extproc.client_assertion.refresh` spans.
	DetailClientExpired FailureDetail = "client_expired"
	// DetailCredentialRejected means the broker answered `invalid_grant` without an `error_uri`, which it does only
	// when the subject token fails validation, or the OAuth provider did so during refresh. The client gets no sign-in
	// URL.
	DetailCredentialRejected FailureDetail = "credential_rejected" // #nosec G101 -- Diagnostic classification identifier, not a credential.
	// DetailAuthorizationDenied means the broker, or the OAuth provider during refresh, answered `access_denied`
	// without an `error_uri`, so ExtProc had no consent URL to return.
	DetailAuthorizationDenied FailureDetail = "authorization_denied"
	// DetailConsentRequired means the broker answered `access_denied` with an `error_uri`, and ExtProc returned that
	// URL to the client in a 403 `insufficient_scope` challenge.
	DetailConsentRequired FailureDetail = "consent_required"
	// DetailReauthRequired means an OAuth error had the code `reauth_required` and no `error_uri`; the broker never
	// sends this code, so it can only come from the OAuth provider during refresh.
	DetailReauthRequired FailureDetail = "reauth_required"
	// DetailSessionReauthRequired means the broker answered `invalid_grant` with an `error_uri` because the user must
	// reconnect the provider session, and ExtProc returned that URL to the client in a 403 `insufficient_scope`
	// challenge.
	DetailSessionReauthRequired FailureDetail = "session_reauth_required"
	// DetailBrokerUnavailable means ExtProc could not reach the broker in time, or the broker answered with a 5xx or
	// 429 status or with `server_error` or `temporarily_unavailable`.
	DetailBrokerUnavailable FailureDetail = "broker_unavailable"
	// DetailBrokerResponseInvalid means the broker answered 200, but ExtProc could not read or parse the body, or the
	// body had no access token.
	DetailBrokerResponseInvalid FailureDetail = "broker_response_invalid"
	// DetailProviderUnavailable means ExtProc could not reach the OAuth provider in time to refresh its client
	// assertion, or the provider answered 5xx, 429, `server_error` or `temporarily_unavailable`.
	DetailProviderUnavailable FailureDetail = "provider_unavailable"
	// DetailProviderResponseInvalid means the OAuth provider answered the client assertion refresh with a response
	// that ExtProc could not read or parse, or without the configured `id_token` or `access_token`.
	DetailProviderResponseInvalid FailureDetail = "provider_response_invalid"
	// DetailCircuitOpen means ExtProc's circuit breaker for the broker was open, or half-open with no free probe, so
	// ExtProc answered 503 without calling the broker; failures inside ExtProc, such as an expired client assertion,
	// also count toward opening it.
	DetailCircuitOpen FailureDetail = "circuit_open"
	// DetailConfiguration means ExtProc's own configuration is unusable: it could not build the request to the
	// broker, or the client assertion type is neither `id_token` nor `access_token`.
	DetailConfiguration FailureDetail = "configuration"
	// DetailPolicyConfiguration means authorization is enabled but the gateway sent no protocol metadata, or no
	// `mcp_server` metadata for an MCP request; ExtProc answered 403.
	DetailPolicyConfiguration FailureDetail = "policy_configuration"
	// DetailPolicyEvaluationFailed means the authorizer returned an error instead of a decision; the OPA authorizer
	// never does, because it reports errors, timeouts and undefined results as `policy_denied`.
	DetailPolicyEvaluationFailed FailureDetail = "policy_evaluation_failed"
	// DetailPolicyDenied means the OPA decision did not allow the request, also after a policy error, a timeout or an
	// undefined result, so ExtProc answered 403 `access_denied`; `authorization.result_code` on `extproc.opa.evaluate`
	// tells which. For a request with a body, only logs carry this detail.
	DetailPolicyDenied FailureDetail = "policy_denied"
	// DetailResponseWriteFailed means ExtProc could not send its response to the gateway on the processing stream;
	// only logs carry it, since the span has already ended.
	DetailResponseWriteFailed FailureDetail = "response_write_failed"
	// DetailInternalUnclassified means ExtProc could not classify the failure, such as an unexpected error or an
	// OAuth error code that it does not map to a detail.
	DetailInternalUnclassified FailureDetail = "internal_unclassified"
	// DetailCallerCanceled means the request was canceled, or its deadline passed, while ExtProc waited for the
	// exchange or was sending its response.
	DetailCallerCanceled FailureDetail = "caller_canceled"
	// DetailGrantContextDenied means the broker's grant context evaluation denied the delegation, and ExtProc
	// answered 403 `access_denied` with the consent URL as `error_uri`.
	DetailGrantContextDenied FailureDetail = "grant_context_denied"
	// DetailGrantContextUnavailable means ExtProc had no usable delegation permit and answered 503: the broker
	// evaluation failed or could not run, the request was canceled meanwhile, the permit expired or was invalid, or the
	// exchange result did not match it.
	DetailGrantContextUnavailable FailureDetail = "grant_context_unavailable"

	// RecoveryNone means ExtProc suggests no recovery: the request succeeded or was canceled, or no listed action
	// fits, as for rejected credentials and policy denials.
	RecoveryNone RecoveryAction = "none"
	// RecoveryReconsent means the user should grant consent again at the URL that ExtProc returned; ExtProc suggests
	// it for `consent_required` and `grant_context_denied`.
	RecoveryReconsent RecoveryAction = "reconsent"
	// RecoveryReauthenticate means the user should sign in again at the `error_uri` that ExtProc returned; ExtProc
	// suggests it only for `session_reauth_required`.
	RecoveryReauthenticate RecoveryAction = "reauthenticate"
	// RecoveryFixConfiguration means an operator should fix ExtProc's configuration or the gateway's metadata;
	// ExtProc suggests it for `configuration` and `policy_configuration`.
	RecoveryFixConfiguration RecoveryAction = "fix_configuration"
	// RecoveryRetry means the failure may be temporary and a later request may succeed; ExtProc suggests it for
	// every `infrastructure_error` outcome.
	RecoveryRetry RecoveryAction = "retry"

	// TargetNone means the recovery action has no target, which holds for every action except `reconsent` and
	// `reauthenticate`.
	TargetNone RecoveryTarget = "none"
	// TargetConsent means the user's consent for the agent, which the user renews at the consent URL that ExtProc
	// returned; it goes with `reconsent`.
	TargetConsent RecoveryTarget = "consent"
	// TargetProviderSession means the user's provider session that the broker holds, which the user renews at the
	// returned `error_uri`; it goes with `reauthenticate`.
	TargetProviderSession RecoveryTarget = "provider_session"

	// ExchangeUnknown means ExtProc cannot observe which kind of exchange the broker performed; ExtProc always sets
	// this value.
	ExchangeUnknown ExchangeKind = "unknown"
	// ExchangeThirdParty means a third-party service token exchange; ExtProc never sets it, because it cannot
	// observe the broker's exchange kind.
	ExchangeThirdParty ExchangeKind = "third_party"
	// ExchangeImpersonation means a user impersonation exchange; ExtProc never sets it, because it cannot observe
	// the broker's exchange kind.
	ExchangeImpersonation ExchangeKind = "impersonation"
)

type Diagnostic struct {
	outcome Outcome
	stage   FailureStage
	detail  FailureDetail
	action  RecoveryAction
	target  RecoveryTarget
	kind    ExchangeKind
	// serverError is the error.type of a failure that the client receives as a 5xx response.
	serverError ErrorKind
}

// asServerError marks a failure that the client receives as a 5xx response. Only these failures
// set the error status on the extproc.token_exchange Server span.
func (d Diagnostic) asServerError(kind ErrorKind) Diagnostic {
	d.serverError = kind
	return d
}

func NewDiagnostic(stage FailureStage, detail FailureDetail) Diagnostic {
	switch stage {
	case StageRequestValidation, StageExchangeRouting, StageClientAuthorization, StageRefresh, StageResponseWrite:
	default:
		stage = StageExchangeRouting
	}
	d := Diagnostic{outcome: OutcomeInfrastructureError, stage: stage, detail: detail, action: RecoveryRetry, target: TargetNone, kind: ExchangeUnknown}
	switch detail {
	case DetailResourceMissing, DetailRequestMalformed:
		d.outcome, d.action = OutcomeInvalidRequest, RecoveryNone
	case DetailSubjectInvalid, DetailClientInvalid, DetailClientExpired, DetailCredentialRejected:
		d.outcome, d.action = OutcomeAuthenticationFailed, RecoveryNone
	case DetailAuthorizationDenied, DetailPolicyDenied:
		d.outcome, d.action = OutcomeAuthorizationDenied, RecoveryNone
	case DetailConsentRequired, DetailGrantContextDenied:
		d.outcome, d.action, d.target = OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent
	case DetailReauthRequired:
		d.outcome, d.action = OutcomeReauthRequired, RecoveryNone
	case DetailSessionReauthRequired:
		d.outcome, d.action, d.target = OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailConfiguration, DetailPolicyConfiguration:
		d.outcome, d.action = OutcomeConfigurationError, RecoveryFixConfiguration
	case DetailCallerCanceled:
		d.outcome, d.action = OutcomeCanceled, RecoveryNone
	case DetailBrokerUnavailable, DetailBrokerResponseInvalid, DetailProviderUnavailable, DetailProviderResponseInvalid, DetailCircuitOpen, DetailPolicyEvaluationFailed, DetailResponseWriteFailed, DetailInternalUnclassified, DetailGrantContextUnavailable:
	default:
		d.detail = DetailInternalUnclassified
	}
	return d
}

func SuccessDiagnostic(kind ExchangeKind) Diagnostic {
	return Diagnostic{outcome: OutcomeSuccess, stage: StageNone, detail: DetailNone, action: RecoveryNone, target: TargetNone}.WithExchangeKind(kind)
}

func (d Diagnostic) Outcome() Outcome               { return d.outcome }
func (d Diagnostic) Stage() FailureStage            { return d.stage }
func (d Diagnostic) Detail() FailureDetail          { return d.detail }
func (d Diagnostic) RecoveryAction() RecoveryAction { return d.action }
func (d Diagnostic) RecoveryTarget() RecoveryTarget { return d.target }
func (d Diagnostic) ExchangeKind() ExchangeKind     { return d.kind }
func (d Diagnostic) WithExchangeKind(kind ExchangeKind) Diagnostic {
	switch kind {
	case ExchangeThirdParty, ExchangeImpersonation:
	default:
		kind = ExchangeUnknown
	}
	d.kind = kind
	return d
}

// metricAttributes always includes stage/detail, with none on success.
func (d Diagnostic) metricAttributes() [6]attribute.KeyValue {
	return [6]attribute.KeyValue{
		attribute.String("token_exchange.outcome", string(d.Outcome())),
		attribute.String("token_exchange.recovery_action", string(d.RecoveryAction())),
		attribute.String("token_exchange.recovery_target", string(d.RecoveryTarget())),
		attribute.String("token_exchange.exchange_kind", string(d.ExchangeKind())),
		attribute.String("token_exchange.failure_stage", string(d.Stage())),
		attribute.String("token_exchange.failure_detail", string(d.Detail())),
	}
}

func logDiagnostic(ctx context.Context, logger *slog.Logger, level slog.Level, message string, d Diagnostic) {
	attrs := [6]slog.Attr{
		slog.String("token_exchange.outcome", string(d.Outcome())),
		slog.String("token_exchange.recovery_action", string(d.RecoveryAction())),
		slog.String("token_exchange.recovery_target", string(d.RecoveryTarget())),
		slog.String("token_exchange.exchange_kind", string(d.ExchangeKind())),
		slog.String("token_exchange.failure_stage", string(d.Stage())),
		slog.String("token_exchange.failure_detail", string(d.Detail())),
	}
	n := len(attrs)
	if d.Outcome() == OutcomeSuccess {
		n = 4
	}
	logger.LogAttrs(ctx, level, message, attrs[:n]...) //nolint:sloglint // Each caller passes a constant message.
}

func diagnosticForMetadata(metadata ErrorMetadata, cause error) Diagnostic {
	stage := StageExchangeRouting
	if metadata.Operation() == OperationAssertionRefresh {
		stage = StageRefresh
	}
	switch metadata.Kind() {
	case KindNone:
		return SuccessDiagnostic(ExchangeUnknown)
	case KindConfiguration:
		return NewDiagnostic(stage, DetailConfiguration)
	case KindCallerCanceled:
		return NewDiagnostic(stage, DetailCallerCanceled)
	case KindAssertionExpired:
		return NewDiagnostic(stage, DetailClientExpired)
	case KindCircuitOpen:
		return NewDiagnostic(stage, DetailCircuitOpen)
	case KindUnavailable:
		if metadata.Dependency() == DependencyOAuthProvider {
			return NewDiagnostic(stage, DetailProviderUnavailable)
		}
		return NewDiagnostic(stage, DetailBrokerUnavailable)
	case KindInvalidResponse:
		if metadata.Dependency() == DependencyOAuthProvider {
			return NewDiagnostic(stage, DetailProviderResponseInvalid)
		}
		return NewDiagnostic(stage, DetailBrokerResponseInvalid)
	case KindRejected:
		var brokerErr *BrokerExchangeError
		hasRecovery := errors.As(cause, &brokerErr) && brokerErr.ErrorURI != ""
		switch metadata.OAuthCode() {
		case "invalid_request", "invalid_target", "invalid_scope", "unsupported_grant_type":
			return NewDiagnostic(stage, DetailRequestMalformed)
		case "invalid_client", "unauthorized_client":
			return NewDiagnostic(stage, DetailClientInvalid)
		case "invalid_token":
			return NewDiagnostic(stage, DetailSubjectInvalid)
		case "invalid_grant":
			if hasRecovery {
				return NewDiagnostic(stage, DetailSessionReauthRequired)
			}
			return NewDiagnostic(stage, DetailCredentialRejected)
		case "reauth_required":
			if hasRecovery {
				return NewDiagnostic(stage, DetailSessionReauthRequired)
			}
			return NewDiagnostic(stage, DetailReauthRequired)
		case "access_denied":
			if hasRecovery {
				return NewDiagnostic(stage, DetailConsentRequired)
			}
			return NewDiagnostic(stage, DetailAuthorizationDenied)
		case "server_error", "temporarily_unavailable":
			if metadata.Dependency() == DependencyOAuthProvider {
				return NewDiagnostic(stage, DetailProviderUnavailable)
			}
			return NewDiagnostic(stage, DetailBrokerUnavailable)
		}
	}
	return NewDiagnostic(stage, DetailInternalUnclassified)
}

func diagnosticFromError(ctx context.Context, err error) Diagnostic {
	metadata := metadataFromError(ctx, err)
	var operationErr *OperationError
	if errors.As(err, &operationErr) {
		if metadata.Kind() == KindCallerCanceled {
			return NewDiagnostic(operationErr.Diagnostic().Stage(), DetailCallerCanceled)
		}
		return operationErr.Diagnostic()
	}
	return diagnosticForMetadata(metadata, err)
}
