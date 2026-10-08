package tokenexchange

// Diagnostic is a credential-free classification, not an automatic retry policy.
// Its scalar fields are private so errors shared by singleflight callers stay immutable.
type Diagnostic struct {
	outcome Outcome
	stage   FailureStage
	detail  FailureDetail
	action  RecoveryAction
	target  RecoveryTarget
	kind    ExchangeKind
}

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
	StageSubjectValidation   FailureStage = "subject_validation"
	StageClientValidation    FailureStage = "client_validation"
	StageIdentityResolution  FailureStage = "identity_resolution"
	StageClientAuthorization FailureStage = "client_authorization"
	StageResourceResolution  FailureStage = "resource_resolution"
	StageGrantAuthorization  FailureStage = "grant_authorization"
	StageSessionLookup       FailureStage = "session_lookup"
	StageRefresh             FailureStage = "refresh"
	StageScopeValidation     FailureStage = "scope_validation"
	StageResponseWrite       FailureStage = "response_write"

	DetailNone                          FailureDetail = "none"
	DetailResourceMissing               FailureDetail = "resource_missing"
	DetailRequestMalformed              FailureDetail = "request_malformed"
	DetailSubjectInvalid                FailureDetail = "subject_invalid"
	DetailSubjectExpired                FailureDetail = "subject_expired"
	DetailClientInvalid                 FailureDetail = "client_invalid"
	DetailClientExpired                 FailureDetail = "client_expired"
	DetailJWKSUnavailable               FailureDetail = "jwks_unavailable"
	DetailJWTConfiguration              FailureDetail = "jwt_configuration"
	DetailClientPolicyDenied            FailureDetail = "client_policy_denied"
	DetailCELConfiguration              FailureDetail = "cel_configuration"
	DetailCELEvaluationFailed           FailureDetail = "cel_evaluation_failed"
	DetailAgentMissing                  FailureDetail = "agent_missing"
	DetailAgentInvalid                  FailureDetail = "agent_invalid"
	DetailAgentRepositoryUnavailable    FailureDetail = "agent_repository_unavailable"
	DetailResourceUnregistered          FailureDetail = "resource_unregistered"
	DetailResourceAmbiguous             FailureDetail = "resource_ambiguous"
	DetailResourceRepositoryUnavailable FailureDetail = "resource_repository_unavailable"
	DetailGrantMissing                  FailureDetail = "grant_missing"
	DetailGrantExpired                  FailureDetail = "grant_expired"
	DetailGrantInsufficient             FailureDetail = "grant_insufficient"
	DetailGrantRepositoryUnavailable    FailureDetail = "grant_repository_unavailable"
	DetailSessionMissing                FailureDetail = "session_missing"
	DetailAccessTokenExpired            FailureDetail = "access_token_expired"
	DetailRefreshTokenExpired           FailureDetail = "refresh_token_expired"
	DetailRefreshUnavailable            FailureDetail = "refresh_unavailable"
	DetailRefreshRejected               FailureDetail = "refresh_rejected"
	DetailProviderClientRejected        FailureDetail = "provider_client_rejected"
	DetailProviderRejected              FailureDetail = "provider_rejected"
	DetailProviderUnavailable           FailureDetail = "provider_unavailable"
	DetailProviderResponseInvalid       FailureDetail = "provider_response_invalid"
	DetailSessionRepositoryUnavailable  FailureDetail = "session_repository_unavailable"
	DetailSessionDecryptionFailed       FailureDetail = "session_decryption_failed"
	DetailSessionEncryptionFailed       FailureDetail = "session_encryption_failed"
	DetailSessionPersistenceFailed      FailureDetail = "session_persistence_failed"
	DetailSessionConfiguration          FailureDetail = "session_configuration"
	DetailSessionScopeInsufficient      FailureDetail = "session_scope_insufficient"
	DetailResponseWriteFailed           FailureDetail = "response_write_failed"
	DetailInternalUnclassified          FailureDetail = "internal_unclassified"
	DetailCallerCanceled                FailureDetail = "caller_canceled"

	RecoveryNone             RecoveryAction = "none"
	RecoveryReconsent        RecoveryAction = "reconsent"
	RecoveryReauthenticate   RecoveryAction = "reauthenticate"
	RecoveryFixConfiguration RecoveryAction = "fix_configuration"
	RecoveryRetry            RecoveryAction = "retry"

	TargetNone                RecoveryTarget = "none"
	TargetConsent             RecoveryTarget = "consent"
	TargetSubjectIdentity     RecoveryTarget = "subject_identity"
	TargetCallingClient       RecoveryTarget = "calling_client"
	TargetProviderSession     RecoveryTarget = "provider_session"
	TargetBrokerConfiguration RecoveryTarget = "broker_configuration"

	ExchangeThirdParty    ExchangeKind = "third_party"
	ExchangeImpersonation ExchangeKind = "impersonation"
)

func NewDiagnostic(stage FailureStage, detail FailureDetail) Diagnostic {
	d := Diagnostic{outcome: OutcomeInfrastructureError, stage: boundedStage(stage), detail: detail,
		action: RecoveryRetry, target: TargetNone, kind: ExchangeThirdParty}
	switch detail {
	case DetailResourceMissing, DetailRequestMalformed:
		d.stage, d.outcome, d.action = StageRequestValidation, OutcomeInvalidRequest, RecoveryNone
	case DetailSubjectInvalid, DetailSubjectExpired:
		d.stage, d.outcome, d.action, d.target = StageSubjectValidation, OutcomeAuthenticationFailed, RecoveryReauthenticate, TargetSubjectIdentity
	case DetailClientInvalid, DetailClientExpired:
		d.stage, d.outcome, d.action, d.target = StageClientValidation, OutcomeAuthenticationFailed, RecoveryReauthenticate, TargetCallingClient
	case DetailClientPolicyDenied:
		d.stage, d.outcome, d.action = StageClientAuthorization, OutcomeAuthorizationDenied, RecoveryNone
	case DetailCELConfiguration, DetailJWTConfiguration:
		d.outcome, d.action, d.target = OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration
	case DetailAgentMissing, DetailAgentInvalid:
		d.stage, d.outcome, d.action, d.target = StageIdentityResolution, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration
	case DetailAgentRepositoryUnavailable:
		d.stage = StageIdentityResolution
	case DetailResourceUnregistered:
		d.stage, d.outcome, d.action = StageResourceResolution, OutcomeInvalidRequest, RecoveryNone
	case DetailResourceAmbiguous:
		d.stage, d.outcome, d.action, d.target = StageResourceResolution, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration
	case DetailResourceRepositoryUnavailable:
		d.stage = StageResourceResolution
	case DetailGrantMissing, DetailGrantExpired, DetailGrantInsufficient:
		d.stage, d.outcome, d.action, d.target = StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent
	case DetailGrantRepositoryUnavailable:
		d.stage = StageGrantAuthorization
	case DetailSessionMissing:
		d.stage, d.outcome, d.action, d.target = StageSessionLookup, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailAccessTokenExpired, DetailRefreshTokenExpired, DetailRefreshUnavailable:
		d.outcome, d.action, d.target = OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailRefreshRejected:
		d.stage, d.outcome, d.action, d.target = StageRefresh, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailProviderClientRejected, DetailSessionConfiguration:
		d.outcome, d.action, d.target = OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration
	case DetailProviderRejected, DetailProviderUnavailable, DetailProviderResponseInvalid:
		d.stage = StageRefresh
	case DetailSessionRepositoryUnavailable, DetailSessionDecryptionFailed, DetailSessionEncryptionFailed, DetailSessionPersistenceFailed,
		DetailJWKSUnavailable, DetailCELEvaluationFailed, DetailInternalUnclassified:
		// These failures can occur at more than one stage; retain the origin's stage.
	case DetailSessionScopeInsufficient:
		d.stage, d.outcome, d.action, d.target = StageScopeValidation, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailResponseWriteFailed:
		d.stage = StageResponseWrite
	case DetailCallerCanceled:
		d.outcome, d.action = OutcomeCanceled, RecoveryNone
	default:
		d.detail = DetailInternalUnclassified
	}
	return d
}

func boundedStage(stage FailureStage) FailureStage {
	switch stage {
	case StageRequestValidation, StageExchangeRouting, StageSubjectValidation, StageClientValidation, StageIdentityResolution,
		StageClientAuthorization, StageResourceResolution, StageGrantAuthorization, StageSessionLookup, StageRefresh,
		StageScopeValidation, StageResponseWrite:
		return stage
	default:
		return StageExchangeRouting
	}
}

func SuccessDiagnostic(kind ExchangeKind) Diagnostic {
	return Diagnostic{outcome: OutcomeSuccess, stage: StageNone, detail: DetailNone, action: RecoveryNone,
		target: TargetNone, kind: boundedExchangeKind(kind)}
}

func boundedExchangeKind(kind ExchangeKind) ExchangeKind {
	if kind == ExchangeImpersonation {
		return kind
	}
	return ExchangeThirdParty
}

func (d Diagnostic) Outcome() Outcome               { return d.outcome }
func (d Diagnostic) Stage() FailureStage            { return d.stage }
func (d Diagnostic) Detail() FailureDetail          { return d.detail }
func (d Diagnostic) RecoveryAction() RecoveryAction { return d.action }
func (d Diagnostic) RecoveryTarget() RecoveryTarget { return d.target }
func (d Diagnostic) ExchangeKind() ExchangeKind     { return d.kind }

func (d Diagnostic) WithExchangeKind(kind ExchangeKind) Diagnostic {
	d.kind = boundedExchangeKind(kind)
	return d
}
