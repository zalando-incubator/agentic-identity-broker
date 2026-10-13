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
	OutcomeSuccess              Outcome = "success"
	OutcomeAuthorizationDenied  Outcome = "authorization_denied"
	OutcomeAuthenticationFailed Outcome = "authentication_failed"
	OutcomeReauthRequired       Outcome = "reauth_required"
	OutcomeInvalidRequest       Outcome = "invalid_request"
	OutcomeConfigurationError   Outcome = "configuration_error"
	OutcomeInfrastructureError  Outcome = "infrastructure_error"
	OutcomeCanceled             Outcome = "canceled"

	StageNone                FailureStage = "none"
	StageRequestValidation   FailureStage = "request_validation"
	StageExchangeRouting     FailureStage = "exchange_routing"
	StageClientAuthorization FailureStage = "client_authorization"
	StageRefresh             FailureStage = "refresh"
	StageResponseWrite       FailureStage = "response_write"

	DetailNone                    FailureDetail = "none"
	DetailResourceMissing         FailureDetail = "resource_missing"
	DetailRequestMalformed        FailureDetail = "request_malformed"
	DetailSubjectInvalid          FailureDetail = "subject_invalid"
	DetailClientInvalid           FailureDetail = "client_invalid"
	DetailClientExpired           FailureDetail = "client_expired"
	DetailCredentialRejected      FailureDetail = "credential_rejected"
	DetailAuthorizationDenied     FailureDetail = "authorization_denied"
	DetailConsentRequired         FailureDetail = "consent_required"
	DetailReauthRequired          FailureDetail = "reauth_required"
	DetailSessionReauthRequired   FailureDetail = "session_reauth_required"
	DetailBrokerUnavailable       FailureDetail = "broker_unavailable"
	DetailBrokerResponseInvalid   FailureDetail = "broker_response_invalid"
	DetailProviderUnavailable     FailureDetail = "provider_unavailable"
	DetailProviderResponseInvalid FailureDetail = "provider_response_invalid"
	DetailCircuitOpen             FailureDetail = "circuit_open"
	DetailConfiguration           FailureDetail = "configuration"
	DetailPolicyConfiguration     FailureDetail = "policy_configuration"
	DetailPolicyEvaluationFailed  FailureDetail = "policy_evaluation_failed"
	DetailPolicyDenied            FailureDetail = "policy_denied"
	DetailResponseWriteFailed     FailureDetail = "response_write_failed"
	DetailInternalUnclassified    FailureDetail = "internal_unclassified"
	DetailCallerCanceled          FailureDetail = "caller_canceled"

	RecoveryNone             RecoveryAction = "none"
	RecoveryReconsent        RecoveryAction = "reconsent"
	RecoveryReauthenticate   RecoveryAction = "reauthenticate"
	RecoveryFixConfiguration RecoveryAction = "fix_configuration"
	RecoveryRetry            RecoveryAction = "retry"

	TargetNone            RecoveryTarget = "none"
	TargetConsent         RecoveryTarget = "consent"
	TargetProviderSession RecoveryTarget = "provider_session"

	ExchangeUnknown       ExchangeKind = "unknown"
	ExchangeThirdParty    ExchangeKind = "third_party"
	ExchangeImpersonation ExchangeKind = "impersonation"
)

type Diagnostic struct {
	outcome Outcome
	stage   FailureStage
	detail  FailureDetail
	action  RecoveryAction
	target  RecoveryTarget
	kind    ExchangeKind
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
	case DetailConsentRequired:
		d.outcome, d.action, d.target = OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent
	case DetailReauthRequired:
		d.outcome, d.action = OutcomeReauthRequired, RecoveryNone
	case DetailSessionReauthRequired:
		d.outcome, d.action, d.target = OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailConfiguration, DetailPolicyConfiguration:
		d.outcome, d.action = OutcomeConfigurationError, RecoveryFixConfiguration
	case DetailCallerCanceled:
		d.outcome, d.action = OutcomeCanceled, RecoveryNone
	case DetailBrokerUnavailable, DetailBrokerResponseInvalid, DetailProviderUnavailable, DetailProviderResponseInvalid, DetailCircuitOpen, DetailPolicyEvaluationFailed, DetailResponseWriteFailed, DetailInternalUnclassified:
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
	logger.LogAttrs(ctx, level, message, attrs[:n]...)
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
