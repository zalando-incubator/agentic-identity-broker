package app

import (
	"context"
	"testing"
	"time"

	memorystorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAuthorizationClock struct{ now time.Time }

func (c testAuthorizationClock) Now(context.Context) (time.Time, error) { return c.now, nil }

func (c testAuthorizationClock) Run(ctx context.Context, _ id.AgentID, operation func(context.Context, time.Time) error) error {
	return operation(ctx, c.now)
}

type unexpectedRefreshRevocations struct{}

func (unexpectedRefreshRevocations) ListActiveByAgent(context.Context, id.AgentID, *id.Principal) ([]storage.RefreshSessionAuditIdentity, error) {
	panic("unexpected refresh revocation in delegation verifier test")
}

func (unexpectedRefreshRevocations) RevokeByID(context.Context, id.RefreshSessionID, time.Time, storage.RefreshRevocationReason) error {
	panic("unexpected refresh revocation in delegation verifier test")
}

func (unexpectedRefreshRevocations) RevokeByPrincipalAndAgent(context.Context, id.Principal, id.AgentID, time.Time, storage.RefreshRevocationReason) error {
	panic("unexpected refresh revocation in delegation verifier test")
}

func (unexpectedRefreshRevocations) RevokeByAgent(context.Context, id.AgentID, time.Time, storage.RefreshRevocationReason) error {
	panic("unexpected refresh revocation in delegation verifier test")
}

func TestUserDelegationVerifier(t *testing.T) {
	principal := id.NewPrincipal("subject-1")
	agentID := id.NewAgentID()
	decisionTime := time.Now().UTC()

	newVerifier := func(repo ports.UserGrantRepository) ports.UserDelegationVerifier {
		clock := testAuthorizationClock{now: decisionTime}
		return newUserDelegationVerifier(consent.NewService(nil, nil, repo, nil, nil, nil, clock, clock, unexpectedRefreshRevocations{}))
	}
	createGrant := func(t *testing.T, repo ports.UserGrantRepository, validUntil time.Time) id.GrantID {
		t.Helper()
		grantID := id.NewGrantID()
		require.NoError(t, repo.Create(context.Background(), &storage.UserGrant{
			ID:         grantID,
			Principal:  principal,
			AgentID:    agentID,
			ValidUntil: &validUntil,
		}))
		return grantID
	}

	t.Run("classifies active delegation", func(t *testing.T) {
		repo := memorystorage.NewUserGrantRepository()
		validUntil := decisionTime.Add(time.Hour)
		grantID := createGrant(t, repo, validUntil)

		decision, err := newVerifier(repo).VerifyUserDelegation(context.Background(), principal, agentID, decisionTime)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationActive, decision.Status)
		assert.Equal(t, grantID, decision.GrantID)
		require.NotNil(t, decision.ValidUntil)
		assert.Equal(t, validUntil, *decision.ValidUntil)
		*decision.ValidUntil = decisionTime.Add(-time.Hour)
		second, err := newVerifier(repo).VerifyUserDelegation(context.Background(), principal, agentID, decisionTime)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationActive, second.Status)
		assert.Equal(t, grantID, second.GrantID)
		require.NotNil(t, second.ValidUntil)
		assert.Equal(t, validUntil, *second.ValidUntil)
	})

	t.Run("classifies missing delegation", func(t *testing.T) {
		decision, err := newVerifier(memorystorage.NewUserGrantRepository()).VerifyUserDelegation(context.Background(), principal, agentID, decisionTime)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationMissing, decision.Status)
		assert.True(t, decision.GrantID.IsZero())
		assert.Nil(t, decision.ValidUntil)
	})

	t.Run("classifies expired delegation", func(t *testing.T) {
		repo := memorystorage.NewUserGrantRepository()
		createGrant(t, repo, decisionTime.Add(-time.Hour))

		decision, err := newVerifier(repo).VerifyUserDelegation(context.Background(), principal, agentID, decisionTime)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationExpired, decision.Status)
		assert.True(t, decision.GrantID.IsZero())
		assert.Nil(t, decision.ValidUntil)
	})

	t.Run("expiry at decision time denies", func(t *testing.T) {
		repo := memorystorage.NewUserGrantRepository()
		createGrant(t, repo, decisionTime)
		decision, err := newVerifier(repo).VerifyUserDelegation(context.Background(), principal, agentID, decisionTime)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationExpired, decision.Status)
		assert.True(t, decision.GrantID.IsZero())
	})

	t.Run("fails closed when no consent service is available", func(t *testing.T) {
		decision, err := userDelegationVerifier{}.VerifyUserDelegation(context.Background(), principal, agentID, decisionTime)
		require.Error(t, err)
		assert.Equal(t, ports.UserDelegationDecision{}, decision)
	})
}
