package tokenexchange

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

func TestDiagnosticClassificationContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		detail  FailureDetail
		stage   FailureStage
		outcome Outcome
		action  RecoveryAction
		target  RecoveryTarget
	}{
		{DetailResourceMissing, StageRequestValidation, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{DetailRequestMalformed, StageRequestValidation, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{DetailSubjectInvalid, StageSubjectValidation, OutcomeAuthenticationFailed, RecoveryReauthenticate, TargetSubjectIdentity},
		{DetailSubjectExpired, StageSubjectValidation, OutcomeAuthenticationFailed, RecoveryReauthenticate, TargetSubjectIdentity},
		{DetailClientInvalid, StageClientValidation, OutcomeAuthenticationFailed, RecoveryReauthenticate, TargetCallingClient},
		{DetailClientExpired, StageClientValidation, OutcomeAuthenticationFailed, RecoveryReauthenticate, TargetCallingClient},
		{DetailJWKSUnavailable, StageSubjectValidation, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailJWKSUnavailable, StageClientValidation, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailJWTConfiguration, StageSubjectValidation, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailJWTConfiguration, StageClientValidation, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailClientPolicyDenied, StageClientAuthorization, OutcomeAuthorizationDenied, RecoveryNone, TargetNone},
		{DetailCELConfiguration, StageClientAuthorization, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailCELConfiguration, StageIdentityResolution, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailCELEvaluationFailed, StageClientAuthorization, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailAgentMissing, StageIdentityResolution, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailAgentInvalid, StageIdentityResolution, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailAgentRepositoryUnavailable, StageIdentityResolution, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailResourceUnregistered, StageResourceResolution, OutcomeInvalidRequest, RecoveryNone, TargetNone},
		{DetailResourceAmbiguous, StageResourceResolution, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailResourceRepositoryUnavailable, StageResourceResolution, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailGrantMissing, StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{DetailGrantExpired, StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_empty"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_permission_set_undeclared"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_permission_set_missing"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_service_omitted"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_service_definition_missing"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_service_requirement_excluded"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{FailureDetail("grant_scope_intersection_empty"), StageGrantAuthorization, OutcomeAuthorizationDenied, RecoveryReconsent, TargetConsent},
		{DetailGrantRepositoryUnavailable, StageGrantAuthorization, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionMissing, StageSessionLookup, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailAccessTokenExpired, StageSessionLookup, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailRefreshTokenExpired, StageSessionLookup, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailRefreshUnavailable, StageSessionLookup, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailAccessTokenExpired, StageRefresh, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailRefreshTokenExpired, StageRefresh, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailRefreshUnavailable, StageRefresh, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailRefreshRejected, StageRefresh, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailProviderClientRejected, StageRefresh, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailProviderRejected, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailProviderUnavailable, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailProviderResponseInvalid, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionRepositoryUnavailable, StageSessionLookup, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionDecryptionFailed, StageSessionLookup, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionDecryptionFailed, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionEncryptionFailed, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionPersistenceFailed, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionConfiguration, StageRefresh, OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration},
		{DetailCredentialSourceUnavailable, StageSessionLookup, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailCredentialSourceUnavailable, StageRefresh, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailSessionScopeInsufficient, StageScopeValidation, OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession},
		{DetailResponseWriteFailed, StageResponseWrite, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailInternalUnclassified, StageIdentityResolution, OutcomeInfrastructureError, RecoveryRetry, TargetNone},
		{DetailCallerCanceled, StageRefresh, OutcomeCanceled, RecoveryNone, TargetNone},
	} {
		t.Run(string(tc.detail)+"/"+string(tc.stage), func(t *testing.T) {
			d := NewDiagnostic(tc.stage, tc.detail)
			assert.Equal(t, tc.detail, d.Detail())
			assert.Equal(t, tc.stage, d.Stage())
			assert.Equal(t, tc.outcome, d.Outcome())
			assert.Equal(t, tc.action, d.RecoveryAction())
			assert.Equal(t, tc.target, d.RecoveryTarget())
		})
	}
	unknown := NewDiagnostic(FailureStage("SECRET"), FailureDetail("SECRET"))
	assert.Equal(t, DetailInternalUnclassified, unknown.Detail())
	assert.Equal(t, StageExchangeRouting, unknown.Stage())
	success := SuccessDiagnostic(ExchangeThirdParty)
	assert.Equal(t, OutcomeSuccess, success.Outcome())
	assert.Equal(t, StageNone, success.Stage())
	assert.Equal(t, DetailNone, success.Detail())
	assert.Equal(t, RecoveryNone, success.RecoveryAction())
	assert.Equal(t, TargetNone, success.RecoveryTarget())
}

func TestSessionDiagnosticPreservesOriginAndCause(t *testing.T) {
	for _, tc := range []struct {
		detail oauth2session.ErrorDetail
		stage  FailureStage
		want   FailureDetail
	}{
		{oauth2session.DetailSessionMissing, StageSessionLookup, DetailSessionMissing},
		{oauth2session.DetailAccessTokenExpired, StageSessionLookup, DetailAccessTokenExpired},
		{oauth2session.DetailRefreshTokenExpired, StageSessionLookup, DetailRefreshTokenExpired},
		{oauth2session.DetailRefreshUnavailable, StageSessionLookup, DetailRefreshUnavailable},
		{oauth2session.DetailRefreshRejected, StageRefresh, DetailRefreshRejected},
		{oauth2session.DetailProviderClientRejected, StageRefresh, DetailProviderClientRejected},
		{oauth2session.DetailProviderRejected, StageRefresh, DetailProviderRejected},
		{oauth2session.DetailProviderUnavailable, StageRefresh, DetailProviderUnavailable},
		{oauth2session.DetailProviderResponseInvalid, StageRefresh, DetailProviderResponseInvalid},
		{oauth2session.DetailRepositoryUnavailable, StageRefresh, DetailSessionRepositoryUnavailable},
		{oauth2session.DetailDecryptionFailed, StageRefresh, DetailSessionDecryptionFailed},
		{oauth2session.DetailEncryptionFailed, StageRefresh, DetailSessionEncryptionFailed},
		{oauth2session.DetailPersistenceFailed, StageRefresh, DetailSessionPersistenceFailed},
		{oauth2session.DetailConfiguration, StageRefresh, DetailSessionConfiguration},
		{oauth2session.DetailCredentialSourceUnavailable, StageSessionLookup, DetailCredentialSourceUnavailable},
		{oauth2session.DetailCredentialSourceUnavailable, StageRefresh, DetailCredentialSourceUnavailable},
		{oauth2session.DetailCallerCanceled, StageRefresh, DetailCallerCanceled},
		{oauth2session.DetailInternalUnclassified, StageRefresh, DetailInternalUnclassified},
	} {
		t.Run(string(tc.detail), func(t *testing.T) {
			op := oauth2session.OperationRefresh
			if tc.stage == StageSessionLookup {
				op = oauth2session.OperationSessionLookup
			}
			cause := errors.New("nested credential SECRET")
			sessionErr := oauth2session.NewOperationError(oauth2session.NewErrorMetadata(op, tc.detail), cause)
			exchangeErr := NewServerErrorWithCause("exchange failed", sessionErr).WithDiagnostic(sessionDiagnostic(sessionErr))
			assert.Equal(t, tc.want, exchangeErr.Diagnostic().Detail())
			assert.Equal(t, tc.stage, exchangeErr.Diagnostic().Stage())
			assert.ErrorIs(t, exchangeErr, cause)
			var recovered *oauth2session.OperationError
			require.ErrorAs(t, exchangeErr, &recovered)
			assert.Equal(t, sessionErr, recovered)
		})
	}
}

func TestSharedExchangeErrorEnrichmentIsImmutable(t *testing.T) {
	const callers = 32
	cause := errors.New("dependency error")
	baseService := ServiceRef{ID: id.NewServiceID()}
	baseAuthorization := AuthorizationRef{
		AgentID:            id.NewAgentID(),
		GrantID:            id.NewGrantID(),
		GrantUpdatedAt:     time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC),
		GrantValidUntil:    time.Date(2026, time.January, 3, 3, 4, 5, 987654321, time.UTC),
		GrantHasValidUntil: true,
	}
	base := NewServerErrorWithCause("exchange failed", cause).
		WithErrorURI("https://broker.example/base").
		WithObservation(NewDiagnostic(StageRefresh, DetailProviderUnavailable), baseService, baseAuthorization)
	var group singleflight.Group
	started, release := make(chan struct{}), make(chan struct{})
	results := make([]<-chan singleflight.Result, callers)
	results[0] = group.DoChan("same-session", func() (any, error) { close(started); <-release; return nil, base })
	<-started
	for i := 1; i < callers; i++ {
		results[i] = group.DoChan("same-session", func() (any, error) { return nil, base })
	}
	var workers sync.WaitGroup
	for i, result := range results {
		workers.Add(1)
		go func() {
			defer workers.Done()
			shared := <-result
			original := shared.Err.(*TokenExchangeError)
			service := ServiceRef{ID: id.NewServiceID()}
			uri := fmt.Sprintf("https://broker.example/agents/%d", i)
			validUntil := time.Date(2026, time.February, i+1, 3, 4, 5, i, time.UTC)
			grant := &storage.UserGrant{ID: id.NewGrantID(), UpdatedAt: validUntil.Add(-time.Hour), ValidUntil: &validUntil}
			authorization := authorizationRef(id.NewAgentID(), grant)
			diagnostic := NewDiagnostic(StageRefresh, DetailCallerCanceled)
			observed := original.WithObservation(diagnostic, service, authorization)
			*grant.ValidUntil = grant.ValidUntil.Add(time.Hour)
			assert.NotEqual(t, *grant.ValidUntil, observed.Authorization().GrantValidUntil)
			assert.Equal(t, authorization, observed.Authorization(), "input timestamp mutation must not alter observation context")
			assert.NotSame(t, original, observed)
			assert.Equal(t, original.Code(), observed.Code())
			assert.Equal(t, original.Description(), observed.Description())
			assert.Equal(t, original.HTTPStatus(), observed.HTTPStatus())
			assert.Equal(t, original.ErrorURI(), observed.ErrorURI())
			assert.ErrorIs(t, observed, cause)
			enriched := observed.WithErrorURI(uri).WithCause(context.Canceled)
			assert.Equal(t, service, enriched.Service())
			assert.Equal(t, authorization, enriched.Authorization())
			assert.Equal(t, uri, enriched.ErrorURI())
			assert.Equal(t, OutcomeCanceled, enriched.Diagnostic().Outcome())
			assert.ErrorIs(t, enriched, context.Canceled)
			assert.Equal(t, baseService, original.Service())
			assert.Equal(t, baseAuthorization, original.Authorization())
			assert.Equal(t, "https://broker.example/base", original.ErrorURI())
			assert.Equal(t, OutcomeInfrastructureError, original.Diagnostic().Outcome())
			assert.ErrorIs(t, original, cause)
		}()
	}
	close(release)
	workers.Wait()
	assert.Equal(t, "https://broker.example/base", base.ErrorURI())
	assert.Equal(t, baseService, base.Service())
	assert.Equal(t, baseAuthorization, base.Authorization())
	assert.Equal(t, OutcomeInfrastructureError, base.Diagnostic().Outcome())
	assert.ErrorIs(t, base, cause)
}

func TestResourceCauseDoesNotDependOnDescription(t *testing.T) {
	missing := NewResourceUnregisteredError()
	ambiguous := NewResourceAmbiguousError()
	assert.ErrorIs(t, missing, ErrResourceUnregistered)
	assert.ErrorIs(t, ambiguous, ErrResourceAmbiguous)
	assert.True(t, IsResourceNotConfigured(fmt.Errorf("wrapped: %w", missing)))
	assert.True(t, IsResourceAmbiguous(fmt.Errorf("wrapped: %w", ambiguous)))
	assert.False(t, IsResourceNotConfigured(NewInvalidTargetError(missing.Description())))
	assert.False(t, IsResourceAmbiguous(NewInvalidTargetError(ambiguous.Description())))
}
