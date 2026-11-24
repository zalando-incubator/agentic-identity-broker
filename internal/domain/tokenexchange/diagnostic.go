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
	// OutcomeSuccess means the broker issued the requested token, or an access evaluation permitted the request.
	OutcomeSuccess Outcome = "success"
	// OutcomeAuthorizationDenied means the credentials were valid, but the authorization policy, the subject
	// category or the user's grant did not allow the exchange.
	OutcomeAuthorizationDenied Outcome = "authorization_denied"
	// OutcomeAuthenticationFailed means the subject token, an impersonation actor token or the client assertion was
	// invalid or expired.
	OutcomeAuthenticationFailed Outcome = "authentication_failed"
	// OutcomeReauthRequired means the user's stored session with the service is missing or unusable, so the broker
	// answered `invalid_grant` with an `error_uri` to reconnect it.
	OutcomeReauthRequired Outcome = "reauth_required"
	// OutcomeInvalidRequest means the request was malformed, had no resource, or named a resource that no service
	// registers.
	OutcomeInvalidRequest Outcome = "invalid_request"
	// OutcomeConfigurationError means the broker configuration or an agent or service registration prevents the
	// exchange, so a retry fails the same way until an operator fixes it.
	OutcomeConfigurationError Outcome = "configuration_error"
	// OutcomeInfrastructureError means storage, the signing key set, encryption, the provider, a CEL evaluation or
	// the response write failed, or the failure is unclassified.
	OutcomeInfrastructureError Outcome = "infrastructure_error"
	// OutcomeCanceled means the request was canceled or its deadline passed before the broker finished it, usually
	// because the caller disconnected.
	OutcomeCanceled Outcome = "canceled"

	// StageNone means the exchange or access evaluation succeeded, and broker spans and logs then leave out the
	// failure stage.
	StageNone FailureStage = "none"
	// StageRequestValidation means the broker was checking the request's parameters or body, such as the grant type,
	// token types, resource or audience.
	StageRequestValidation FailureStage = "request_validation"
	// StageExchangeRouting means the failure is not tied to one exchange step, such as when token exchange is not
	// enabled or an impersonation token cannot be minted.
	// The broker also uses it for an error that carries no stage.
	StageExchangeRouting FailureStage = "exchange_routing"
	// StageSubjectValidation means the broker was validating the subject token, or an impersonation actor token,
	// including fetching its signing keys.
	StageSubjectValidation FailureStage = "subject_validation"
	// StageClientValidation means the broker was validating the client assertion that authenticates the calling
	// client, including fetching its signing keys.
	StageClientValidation FailureStage = "client_validation"
	// StageIdentityResolution means the broker was deriving identities and the subject category from token claims
	// with CEL, or looking up the agent.
	StageIdentityResolution FailureStage = "identity_resolution"
	// StageClientAuthorization means the broker was evaluating the authorization policy for the calling client, or
	// the subject categories that an impersonation rule allows.
	StageClientAuthorization FailureStage = "client_authorization"
	// StageResourceResolution means the broker was looking up the registered service whose protected resources
	// include the requested resource.
	// An access evaluation also uses it while loading canonical service IDs.
	StageResourceResolution FailureStage = "resource_resolution"
	// StageGrantAuthorization means the broker was checking that the user's grant for the agent exists, has not
	// expired and covers the requested service.
	StageGrantAuthorization FailureStage = "grant_authorization"
	// StageSessionLookup means the broker was reading and decrypting the user's stored session for the service.
	StageSessionLookup FailureStage = "session_lookup"
	// StageRefresh means the broker was renewing the session's access token with its refresh token, or found that
	// it cannot.
	StageRefresh FailureStage = "refresh"
	// StageScopeValidation means the broker was checking that the session's scopes cover the scopes that the grant
	// gives the agent for the service.
	StageScopeValidation FailureStage = "scope_validation"
	// StageResponseWrite means the broker was encoding or writing its response to the client.
	StageResponseWrite FailureStage = "response_write"

	// DetailNone means the exchange or access evaluation succeeded, and broker spans and logs then leave out the
	// failure detail.
	DetailNone FailureDetail = "none"
	// DetailResourceMissing means the third-party exchange request had no `resource` parameter.
	DetailResourceMissing FailureDetail = "resource_missing"
	// DetailRequestMalformed means a parameter or the request body was missing, repeated or invalid, repeated
	// `resource` values differ, an impersonation asked for a scope that the target agent does not allow, or the
	// `context.agent` of an access evaluation does not match the subject token.
	DetailRequestMalformed FailureDetail = "request_malformed"
	// DetailSubjectInvalid means the subject token, or an impersonation actor token, failed parsing or its
	// signature, issuer, audience or claim checks.
	DetailSubjectInvalid FailureDetail = "subject_invalid"
	// DetailSubjectExpired means the subject token, or an impersonation actor token, has expired, so the caller
	// needs a new one.
	DetailSubjectExpired FailureDetail = "subject_expired"
	// DetailClientInvalid means the client assertion failed validation, is not trusted or lacks a required subject,
	// or an access evaluation request carried no bearer assertion.
	DetailClientInvalid FailureDetail = "client_invalid"
	// DetailClientExpired means the client assertion has expired, so the calling client must create a new one.
	DetailClientExpired FailureDetail = "client_expired"
	// DetailJWKSUnavailable means the broker could not fetch the signing keys that it needs to verify the subject
	// token, an actor token or the client assertion.
	DetailJWKSUnavailable FailureDetail = "jwks_unavailable"
	// DetailJWTConfiguration means the token validation settings, such as issuers, audience or clock skew, are
	// invalid, which stops the broker at startup instead of failing a request.
	DetailJWTConfiguration FailureDetail = "jwt_configuration"
	// DetailClientPolicyDenied means the CEL authorization policy, or an impersonation rule's authorization
	// predicate, denied the calling client.
	DetailClientPolicyDenied FailureDetail = "client_policy_denied"
	// DetailCELConfiguration means a configured CEL expression returned a value of the wrong type or an empty value, so
	// check the expression and the token claims that it reads; impersonation reports such values as
	// `cel_evaluation_failed` instead. A CEL expression that fails to compile stops the broker at startup.
	DetailCELConfiguration FailureDetail = "cel_configuration"
	// DetailCELEvaluationFailed means a CEL expression failed or timed out while it ran, such as when it reads a
	// claim that the token lacks, or an impersonation expression returned a value that is not a string or an empty
	// identity.
	DetailCELEvaluationFailed FailureDetail = "cel_evaluation_failed"
	// DetailAgentMissing means no registered agent matches the agent ID that the token claims resolve to, or that the
	// impersonation `audience` names, in which case the caller may have named the wrong agent.
	DetailAgentMissing FailureDetail = "agent_missing"
	// DetailAgentInvalid means the agent ID from the token claims is not a valid agent UUID, or the client ID that
	// it is resolved from matches several registered agents.
	DetailAgentInvalid FailureDetail = "agent_invalid"
	// DetailAgentRepositoryUnavailable means looking up the agent in storage failed for a reason other than the
	// agent not existing.
	DetailAgentRepositoryUnavailable FailureDetail = "agent_repository_unavailable"
	// DetailResourceUnregistered means no registered service lists the requested resource among its protected
	// resources.
	DetailResourceUnregistered FailureDetail = "resource_unregistered"
	// DetailResourceAmbiguous means more than one registered service claims the requested resource; storage keeps
	// each resource unique to one service, so the broker does not currently set it.
	DetailResourceAmbiguous FailureDetail = "resource_ambiguous"
	// DetailResourceRepositoryUnavailable means looking up the service for the resource failed in storage, or an
	// access evaluation could not load canonical service IDs.
	DetailResourceRepositoryUnavailable FailureDetail = "resource_repository_unavailable"
	// DetailGrantMissing means the user has not granted the agent access or has revoked the grant, so the user must
	// consent on the agent's consent page.
	DetailGrantMissing FailureDetail = "grant_missing"
	// DetailGrantExpired means the user's grant for the agent has passed its expiry time, so the user must renew it
	// on the agent's consent page.
	DetailGrantExpired FailureDetail = "grant_expired"
	// DetailGrantEmpty means the user's grant for the agent has no permission set entries.
	DetailGrantEmpty FailureDetail = "grant_empty"
	// DetailGrantPermissionSetUndeclared means the grant references a permission set that the agent no longer
	// declares.
	DetailGrantPermissionSetUndeclared FailureDetail = "grant_permission_set_undeclared"
	// DetailGrantPermissionSetMissing means the grant references a permission set that no longer exists, even one
	// unrelated to the requested service.
	DetailGrantPermissionSetMissing FailureDetail = "grant_permission_set_missing"
	// DetailGrantServiceOmitted means none of the grant's permission set entries includes the requested service.
	DetailGrantServiceOmitted FailureDetail = "grant_service_omitted"
	// DetailGrantServiceDefinitionMissing means the grant includes the requested service, but none of the permission
	// sets that include it defines that service.
	DetailGrantServiceDefinitionMissing FailureDetail = "grant_service_definition_missing"
	// DetailGrantServiceRequirementExcluded means the grant covers the requested service, but the agent declares
	// service requirements that do not list it, so check those requirements.
	DetailGrantServiceRequirementExcluded FailureDetail = "grant_service_requirement_excluded"
	// DetailGrantScopeIntersectionEmpty means none of the scopes granted for the service is in the agent's required
	// scopes for it, so check those required scopes.
	DetailGrantScopeIntersectionEmpty FailureDetail = "grant_scope_intersection_empty"
	// DetailGrantRepositoryUnavailable means the broker could not read the user's grant, or the permission sets that
	// it references, from storage.
	DetailGrantRepositoryUnavailable FailureDetail = "grant_repository_unavailable"
	// DetailConsentIncludedSessionMissing means a service selected in the user's grant has no stored session,
	// so the user must connect it from the agent's consent page.
	DetailConsentIncludedSessionMissing FailureDetail = "consent_included_session_missing"
	// DetailConsentIncludedSessionUnusable means a selected service's session needs reconnection before consent is ready.
	DetailConsentIncludedSessionUnusable FailureDetail = "consent_included_session_unusable"
	// DetailConsentMandatorySessionMissing means an agent-required service has no stored session,
	// even when the grant did not select that service.
	DetailConsentMandatorySessionMissing FailureDetail = "consent_mandatory_session_missing"
	// DetailConsentMandatorySessionUnusable means an agent-required service's session needs reconnection.
	DetailConsentMandatorySessionUnusable FailureDetail = "consent_mandatory_session_unusable"
	// DetailConsentMandatoryScopeInsufficient means an agent-required service's session lacks a required scope.
	DetailConsentMandatoryScopeInsufficient FailureDetail = "consent_mandatory_scope_insufficient"
	// DetailSessionMissing means the user has no stored session for the service, so the user must connect it
	// through the `error_uri` of the `invalid_grant` response.
	DetailSessionMissing FailureDetail = "session_missing"
	// DetailAccessTokenExpired means the stored access token has expired and the session has no refresh token, so
	// the user must reconnect the service.
	DetailAccessTokenExpired FailureDetail = "access_token_expired"
	// DetailRefreshUnavailable means the broker had to renew the access token but found no usable refresh token, so
	// the user must reconnect the service.
	DetailRefreshUnavailable FailureDetail = "refresh_unavailable"
	// DetailRefreshRejected means the provider rejected the session's refresh token with `invalid_grant`, now or on
	// an earlier refresh, so the user must reconnect the service.
	DetailRefreshRejected FailureDetail = "refresh_rejected"
	// DetailProviderClientRejected means the provider answered the refresh with `invalid_client` or
	// `unauthorized_client`, so fix the client credentials or registration of the service.
	DetailProviderClientRejected FailureDetail = "provider_client_rejected"
	// DetailProviderRejected means the provider refused the refresh with a 4xx error that is neither a client
	// rejection nor `invalid_grant`.
	DetailProviderRejected FailureDetail = "provider_rejected"
	// DetailProviderUnavailable means the provider's token endpoint was unreachable, timed out or answered 429 or
	// 5xx during refresh, or signing the client assertion failed.
	DetailProviderUnavailable FailureDetail = "provider_unavailable"
	// DetailProviderResponseInvalid means the provider answered the refresh with a success status but a malformed or
	// oversized body, or one without an access token.
	DetailProviderResponseInvalid FailureDetail = "provider_response_invalid"
	// DetailSessionRepositoryUnavailable means the broker could not read or lock the stored session, or read the
	// service registration for a refresh.
	DetailSessionRepositoryUnavailable FailureDetail = "session_repository_unavailable"
	// DetailSessionDecryptionFailed means the broker could not decrypt the session's stored tokens or the service's
	// client secret, so check the encryption keys.
	DetailSessionDecryptionFailed FailureDetail = "session_decryption_failed"
	// DetailSessionEncryptionFailed means the broker could not encrypt the tokens from a refresh before storing
	// them, so check the encryption keys.
	DetailSessionEncryptionFailed FailureDetail = "session_encryption_failed"
	// DetailSessionPersistenceFailed means the broker could not save refreshed tokens, or a recorded refresh
	// rejection, to the session store.
	DetailSessionPersistenceFailed FailureDetail = "session_persistence_failed"
	// DetailSessionConfiguration means the service's OAuth2 registration is missing, invalid or incomplete for a
	// refresh, or no key is configured to sign client assertions.
	DetailSessionConfiguration FailureDetail = "session_configuration"
	// DetailCredentialSourceUnavailable means no credential files are configured for the service, or the broker could
	// not read them during refresh.
	DetailCredentialSourceUnavailable FailureDetail = "credential_source_unavailable" // #nosec G101 -- diagnostic detail code, not a credential.
	// DetailCredentialIdentityChanged means the service's configured client ID differs from the one that the session
	// recorded, or the session recorded none, so the user must reconnect.
	DetailCredentialIdentityChanged FailureDetail = "credential_identity_changed" // #nosec G101 -- diagnostic detail code, not a credential.
	// DetailSessionScopeInsufficient means the user's stored session lacks scopes that the grant gives the agent for
	// the service, so the user must reconnect the service, which helps only if the service registration requests those
	// scopes.
	DetailSessionScopeInsufficient FailureDetail = "session_scope_insufficient"
	// DetailResponseWriteFailed means the broker could not encode or fully write its response to the client.
	DetailResponseWriteFailed FailureDetail = "response_write_failed"
	// DetailInternalUnclassified means the broker has no specific classification for the failure, and the failure
	// stage shows where it happened.
	DetailInternalUnclassified FailureDetail = "internal_unclassified"
	// DetailCallerCanceled means the request was canceled or its deadline passed before the broker finished it,
	// usually because the caller disconnected.
	DetailCallerCanceled FailureDetail = "caller_canceled"

	// RecoveryNone means the broker suggests no repair, as for a success, an invalid request, a policy or subject
	// category denial, or a cancellation.
	RecoveryNone RecoveryAction = "none"
	// RecoveryReconsent means a grant check denied the request, so the user should review the grant on the consent page
	// that the response links to, unless the failure detail points to the agent's service requirements.
	RecoveryReconsent RecoveryAction = "reconsent"
	// RecoveryReauthenticate means a fresh credential is needed, which the recovery target names: a subject token, a
	// client assertion or the user's session with the service.
	RecoveryReauthenticate RecoveryAction = "reauthenticate"
	// RecoveryFixConfiguration means an operator must correct the broker configuration or an agent or service
	// registration before the exchange can succeed.
	RecoveryFixConfiguration RecoveryAction = "fix_configuration"
	// RecoveryRetry means the failure is an infrastructure failure or unclassified, so a later request may succeed if
	// its cause was temporary.
	RecoveryRetry RecoveryAction = "retry"

	// TargetNone means the recovery action has no target, which holds for `none` and `retry`.
	TargetNone RecoveryTarget = "none"
	// TargetConsent means the user's grant for the agent, which the user manages on the agent's consent page; it
	// goes with `reconsent`.
	TargetConsent RecoveryTarget = "consent"
	// TargetSubjectIdentity means the subject token, or an impersonation actor token, which the caller must replace
	// with a new, valid one.
	TargetSubjectIdentity RecoveryTarget = "subject_identity"
	// TargetCallingClient means the calling client's client assertion, which the client must replace with a new,
	// valid one.
	TargetCallingClient RecoveryTarget = "calling_client"
	// TargetProviderSession means the user's stored session with the service, which the user reconnects through the
	// `error_uri` of the `invalid_grant` response.
	TargetProviderSession RecoveryTarget = "provider_session"
	// TargetBrokerConfiguration means the broker configuration or an agent or service registration, which an
	// operator must fix.
	TargetBrokerConfiguration RecoveryTarget = "broker_configuration"

	// ExchangeThirdParty means the caller exchanged a subject token for the user's stored access token to a
	// registered service.
	// The broker also uses it when token exchange is not enabled.
	ExchangeThirdParty ExchangeKind = "third_party"
	// ExchangeImpersonation means the `audience` parameter selected a target agent, so the broker mints its own token
	// for the subject instead of returning a provider token.
	ExchangeImpersonation ExchangeKind = "impersonation"
)

const (
	// DetailSubjectCategoryIneligible means an impersonation rule does not allow the subject's category, human or
	// agent, or the subject is a different registered agent.
	DetailSubjectCategoryIneligible FailureDetail = "subject_category_ineligible"
	// DetailSubjectCategoryUnsupported means the agent-subject expression matched, so the subject token is an
	// agent's own token, which third-party exchange and access evaluation reject.
	DetailSubjectCategoryUnsupported FailureDetail = "subject_category_unsupported"
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
	case DetailSubjectCategoryUnsupported:
		d.stage, d.outcome, d.action = StageIdentityResolution, OutcomeAuthorizationDenied, RecoveryNone
	case DetailClientPolicyDenied, DetailSubjectCategoryIneligible:
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
	case DetailGrantMissing, DetailGrantExpired, DetailGrantEmpty, DetailGrantPermissionSetUndeclared,
		DetailGrantPermissionSetMissing, DetailGrantServiceOmitted, DetailGrantServiceDefinitionMissing,
		DetailGrantServiceRequirementExcluded, DetailGrantScopeIntersectionEmpty:
		d.stage, d.outcome, d.action, d.target = StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent
	case DetailConsentIncludedSessionMissing, DetailConsentIncludedSessionUnusable,
		DetailConsentMandatorySessionMissing, DetailConsentMandatorySessionUnusable:
		d.stage, d.outcome, d.action, d.target = StageSessionLookup, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent
	case DetailConsentMandatoryScopeInsufficient:
		d.stage, d.outcome, d.action, d.target = StageScopeValidation, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent
	case DetailGrantRepositoryUnavailable:
		d.stage = StageGrantAuthorization
	case DetailSessionMissing:
		d.stage, d.outcome, d.action, d.target = StageSessionLookup, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailAccessTokenExpired, DetailRefreshUnavailable, DetailCredentialIdentityChanged:
		d.outcome, d.action, d.target = OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailRefreshRejected:
		d.stage, d.outcome, d.action, d.target = StageRefresh, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
	case DetailProviderClientRejected, DetailSessionConfiguration:
		d.outcome, d.action, d.target = OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration
	case DetailProviderRejected, DetailProviderUnavailable, DetailProviderResponseInvalid:
		d.stage = StageRefresh
	case DetailSessionRepositoryUnavailable, DetailSessionDecryptionFailed, DetailSessionEncryptionFailed, DetailSessionPersistenceFailed,
		DetailCredentialSourceUnavailable, DetailJWKSUnavailable, DetailCELEvaluationFailed, DetailInternalUnclassified:
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
