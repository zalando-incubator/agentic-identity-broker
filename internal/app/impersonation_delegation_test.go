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

func TestUserDelegationVerifier(t *testing.T) {
	principal := id.NewPrincipal("subject-1")
	agentID := id.NewAgentID()

	newVerifier := func(repo ports.UserGrantRepository) ports.UserDelegationVerifier {
		return newUserDelegationVerifier(consent.NewService(nil, nil, repo, nil, memorystorage.NewRefreshTokenSessionStore(), nil, nil))
	}
	selection := []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}}
	createGrant := func(t *testing.T, repo ports.UserGrantRepository, validUntil time.Time) {
		t.Helper()
		require.NoError(t, repo.Create(context.Background(), &storage.UserGrant{
			ID:                    id.NewGrantID(),
			Principal:             principal,
			AgentID:               agentID,
			ValidUntil:            &validUntil,
			GrantedPermissionSets: selection,
		}))
	}

	t.Run("classifies active delegation", func(t *testing.T) {
		repo := memorystorage.NewUserGrantRepository()
		createGrant(t, repo, time.Now().Add(time.Hour))

		delegation, err := newVerifier(repo).VerifyUserDelegation(context.Background(), principal, agentID)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationActive, delegation.Status)
		assert.Equal(t, selection, delegation.GrantedPermissionSets)
	})

	t.Run("classifies missing delegation", func(t *testing.T) {
		delegation, err := newVerifier(memorystorage.NewUserGrantRepository()).VerifyUserDelegation(context.Background(), principal, agentID)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationMissing, delegation.Status)
		assert.Empty(t, delegation.GrantedPermissionSets)
	})

	t.Run("classifies expired delegation", func(t *testing.T) {
		repo := memorystorage.NewUserGrantRepository()
		createGrant(t, repo, time.Now().Add(-time.Hour))

		delegation, err := newVerifier(repo).VerifyUserDelegation(context.Background(), principal, agentID)
		require.NoError(t, err)
		assert.Equal(t, ports.UserDelegationExpired, delegation.Status)
		assert.Empty(t, delegation.GrantedPermissionSets)
	})

	t.Run("fails closed when no consent service is available", func(t *testing.T) {
		delegation, err := userDelegationVerifier{}.VerifyUserDelegation(context.Background(), principal, agentID)
		require.Error(t, err)
		assert.Empty(t, delegation.Status)
	})
}
