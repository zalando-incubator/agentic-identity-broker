package tokenexchange

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingClassificationAgentRepo struct {
	ports.AgentRepository
	cause     error
	beforeGet func()
}

func (r failingClassificationAgentRepo) Get(context.Context, id.AgentID) (*storage.Agent, error) {
	if r.beforeGet != nil {
		r.beforeGet()
	}
	return nil, r.cause
}

func TestExchangeIdentityFailurePreservesClassificationAndCause(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cause   error
		detail  FailureDetail
		outcome Outcome
	}{
		{"agent absent", ports.ErrNotFound, DetailAgentMissing, OutcomeConfigurationError},
		{"agent repository unavailable", storage.NewStorageError("Get", storage.ErrorKindConnection, errors.New("SENTINEL_DATABASE_SECRET"), "unavailable"), DetailAgentRepositoryUnavailable, OutcomeInfrastructureError},
		{"dependency deadline", context.DeadlineExceeded, DetailAgentRepositoryUnavailable, OutcomeInfrastructureError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			future := time.Now().Add(time.Hour)
			fixture := newExchangeFixture(t, exchangeFixtureConfig{coverage: grantCoversRequested, sessionRepo: &MockSessionRepository{session: &storage.UserSession{
				ID: id.NewSessionID(), Principal: "user@example.com", EncryptedAccessToken: []byte("access"), AccessTokenExpiresAt: &future,
			}}, encryption: &MockEncryption{}})
			fixture.svc.agentRepository = failingClassificationAgentRepo{AgentRepository: fixture.svc.agentRepository, cause: tc.cause}
			response, err := fixture.svc.Exchange(context.Background(), fixture.req)
			assert.Nil(t, response)
			var failure *TokenExchangeError
			require.ErrorAs(t, err, &failure)
			assert.ErrorIs(t, err, tc.cause)
			assert.Equal(t, "server_error", failure.Code())
			assert.Equal(t, tc.detail, failure.Diagnostic().Detail())
			assert.Equal(t, StageIdentityResolution, failure.Diagnostic().Stage())
			assert.Equal(t, tc.outcome, failure.Diagnostic().Outcome())
		})
	}
}

func TestExchangeCallerCancellationRetainsCurrentStage(t *testing.T) {
	fixture := newExchangeFixture(t, exchangeFixtureConfig{coverage: grantCoversRequested, sessionRepo: &MockSessionRepository{}, encryption: &MockEncryption{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fixture.svc.Exchange(ctx, fixture.req)
	var failure *TokenExchangeError
	require.ErrorAs(t, err, &failure)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, OutcomeCanceled, failure.Diagnostic().Outcome())
	assert.Equal(t, StageRequestValidation, failure.Diagnostic().Stage())
	assert.Equal(t, DetailCallerCanceled, failure.Diagnostic().Detail())
}

func TestExchangeRepositoryCancellationUsesMatchingCallerCause(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cause   error
		outcome Outcome
		detail  FailureDetail
	}{
		{"caller canceled during agent lookup", context.Canceled, OutcomeCanceled, DetailCallerCanceled},
		{"dependency timed out before caller canceled", context.DeadlineExceeded, OutcomeInfrastructureError, DetailAgentRepositoryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newExchangeFixture(t, exchangeFixtureConfig{coverage: grantCoversRequested, sessionRepo: &MockSessionRepository{}, encryption: &MockEncryption{}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture.svc.agentRepository = failingClassificationAgentRepo{AgentRepository: fixture.svc.agentRepository, cause: tc.cause, beforeGet: cancel}
			_, err := fixture.svc.Exchange(ctx, fixture.req)
			var failure *TokenExchangeError
			require.ErrorAs(t, err, &failure)
			assert.ErrorIs(t, err, tc.cause)
			assert.Equal(t, tc.outcome, failure.Diagnostic().Outcome())
			assert.Equal(t, StageIdentityResolution, failure.Diagnostic().Stage())
			assert.Equal(t, tc.detail, failure.Diagnostic().Detail())
		})
	}
}

type cancelingClassificationGrantRepo struct {
	ports.UserGrantRepository
	cancel context.CancelFunc
}

func (r cancelingClassificationGrantRepo) FindByPrincipalAndAgent(ctx context.Context, _ id.Principal, _ id.AgentID) (*storage.UserGrant, error) {
	r.cancel()
	return nil, ctx.Err()
}

func TestExchangeCallerCancellationDuringGrantLookupPreservesStage(t *testing.T) {
	fixture := newExchangeFixture(t, exchangeFixtureConfig{coverage: grantCoversRequested, sessionRepo: &MockSessionRepository{}, encryption: &MockEncryption{}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture.svc.consentService = consent.NewService(fixture.svc.agentRepository, fixture.svc.providerService, cancelingClassificationGrantRepo{cancel: cancel}, nil, nil, nil, slog.Default())
	_, err := fixture.svc.Exchange(ctx, fixture.req)
	var failure *TokenExchangeError
	require.ErrorAs(t, err, &failure)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, OutcomeCanceled, failure.Diagnostic().Outcome())
	assert.Equal(t, StageGrantAuthorization, failure.Diagnostic().Stage())
	assert.Equal(t, DetailCallerCanceled, failure.Diagnostic().Detail())
}
