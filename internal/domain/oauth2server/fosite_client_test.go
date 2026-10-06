package oauth2server

import (
	"context"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestBrokerClientIdentityRejectsForeignAuthority(t *testing.T) {
	for _, client := range []fosite.Client{
		&fosite.DefaultClient{ID: id.NewAgentID().String()},
		&publicClient{clientID: id.NewAgentID().String()},
		&confidentialClient{agent: &storage.Agent{}},
	} {
		agentID, err := extractAgentID(client)
		require.Error(t, err)
		require.True(t, agentID.IsZero())
	}
}

func TestPreparedClientCannotRetainReplacedCredential(t *testing.T) {
	ctx := context.Background()
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	prepared, err := provider.authenticateProviderClient(ctx, agent.ID.String(), secret)
	require.NoError(t, err)
	replacement, replacementSecret, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, provider.fositeStorage.credRepo.Rotate(ctx, agent.ID, replacement))
	err = provider.refreshDeps.Coordinator.Run(ctx, agent.ID, func(owner context.Context, at time.Time) error {
		owner = withRefreshOperation(owner, at, prepared, nil)
		current, err := provider.currentProviderClient(owner, agent.ID.String(), prepared)
		require.Nil(t, current)
		require.ErrorIs(t, err, fosite.ErrInvalidClient)
		return nil
	})
	require.NoError(t, err)
	current, err := provider.authenticateProviderClient(ctx, agent.ID.String(), replacementSecret)
	require.NoError(t, err)
	require.Equal(t, replacement.ID, current.(*confidentialClient).credential.ID)
}

func TestPreparedPublicClientCannotBypassNewCredential(t *testing.T) {
	ctx := context.Background()
	provider, agents, _ := newTestProvider(t)
	agent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "Public client", Description: "Credential promotion regression",
		PermissionSets: testAgentPermissionSets(),
	}
	require.NoError(t, agents.Create(ctx, agent))
	prepared, err := provider.authenticateProviderClient(ctx, agent.ID.String(), "")
	require.NoError(t, err)
	require.True(t, prepared.IsPublic())
	credential, secret, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, provider.fositeStorage.credRepo.Create(ctx, credential))
	err = provider.refreshDeps.Coordinator.Run(ctx, agent.ID, func(owner context.Context, at time.Time) error {
		owner = withRefreshOperation(owner, at, prepared, nil)
		current, err := provider.currentProviderClient(owner, agent.ID.String(), prepared)
		require.Nil(t, current)
		require.ErrorIs(t, err, fosite.ErrInvalidClient)
		return nil
	})
	require.NoError(t, err)
	current, err := provider.authenticateProviderClient(ctx, agent.ID.String(), secret)
	require.NoError(t, err)
	require.False(t, current.IsPublic())
}

func TestPreparedCIMDClientCannotReuseRemovedRegistration(t *testing.T) {
	ctx := context.Background()
	provider, agents, _ := newTestProvider(t)
	clientID := "https://client.example.com/metadata.json"
	agent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "CIMD client", Description: "Registration removal regression",
		ClientURIs: []string{clientID}, PermissionSets: testAgentPermissionSets(),
	}
	require.NoError(t, agents.Create(ctx, agent))
	prepared := &publicClient{clientID: clientID, agent: agent.Copy(), grantTypes: fosite.Arguments{"authorization_code", "refresh_token"}}
	agent.ClientURIs = []string{"https://client.example.com/replacement.json"}
	require.NoError(t, agents.Update(ctx, agent))
	err := provider.refreshDeps.Coordinator.Run(ctx, agent.ID, func(owner context.Context, at time.Time) error {
		owner = withRefreshOperation(owner, at, prepared, nil)
		current, err := provider.currentProviderClient(owner, clientID, prepared)
		require.Nil(t, current)
		require.ErrorIs(t, err, fosite.ErrInvalidClient)
		return nil
	})
	require.NoError(t, err)
}
