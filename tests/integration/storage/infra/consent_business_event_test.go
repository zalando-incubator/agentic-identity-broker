//go:build integration

package storage_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestConsentLedgerConcurrentIdenticalFirstGrantRecordsOnlyWinner(t *testing.T) {
	adapter, owner := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	recorder := ledger.NewService(registry, adapter.BusinessEvents(), adapter.BusinessEventLifecycle(), adapter, false)
	sets := permissionset.NewPermissionSetService(adapter.PermissionSets(), adapter.UserGrants(), slog.Default())
	defer sets.Close()
	service := consent.NewService(adapter.Agents(), nil, adapter.UserGrants(), adapter.UserSessions(), sets, slog.Default(), recorder)
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Consent agent", Description: "Concurrent first grant", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}}}
	provider := createTestService("first-grant-provider", "First grant provider", nil)
	provider.Secret, provider.TokenEndpointAuthMethod = model.NewAbsentSecret(), model.TokenEndpointAuthMethodNone
	require.NoError(t, adapter.Services().Create(ctx, provider))
	now := time.Now().UTC()
	set := &storage.PermissionSet{ID: agent.PermissionSets[0].PermissionSetID, Name: "Ledger test", Description: "Ledger test", ServiceScopes: []storage.ServiceScope{{ServiceID: provider.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, adapter.PermissionSets().Create(ctx, set))
	require.NoError(t, adapter.Agents().Create(ctx, agent))
	request := &consent.GrantRequest{Principal: id.Principal("first-grant-user"), AgentID: agent.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: set.ID, IncludedServiceIDs: []id.ServiceID{provider.ID}}}}
	require.NoError(t, adapter.UserSessions().Create(ctx, &storage.UserSession{ID: id.NewSessionID(), Principal: request.Principal, ServiceID: provider.ID, EncryptedAccessToken: []byte("opaque-session-fixture"), TokenType: "Bearer", Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID}, InitiatedAt: now, CreatedAt: now, UpdatedAt: now}))
	_, err = adapter.UserGrants().FindByPrincipalAndAgent(ctx, request.Principal, request.AgentID)
	require.True(t, ports.IsNotFoundErr(err))

	gate, err := owner.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = gate.Rollback() }()
	const pairLockClass int32 = 1095320135
	_, err = gate.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2 || '/' || $3))`, pairLockClass, request.Principal.String(), request.AgentID.String())
	require.NoError(t, err)
	type outcome struct {
		grant *storage.UserGrant
		err   error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			grant, err := service.GrantConsent(ctx, request)
			results <- outcome{grant, err}
		}()
	}
	waitForLifecycleAdvisoryBlock(t, owner, 0, pairLockClass, 2, results)
	require.NoError(t, gate.Commit())
	var grantID id.GrantID
	for range 2 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.NotNil(t, result.grant)
			require.False(t, result.grant.ID.IsZero())
			if grantID.IsZero() {
				grantID = result.grant.ID
			}
			require.Equal(t, grantID, result.grant.ID)
		case <-ctx.Done():
			t.Fatal("competing consent requests did not complete")
		}
	}
	stored, err := adapter.UserGrants().FindByPrincipalAndAgent(ctx, request.Principal, request.AgentID)
	require.NoError(t, err)
	require.Equal(t, grantID, stored.ID)
	events, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: request.Principal}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour), Limit: 100})
	require.NoError(t, err)
	require.Len(t, events, 1, "identical competing grants must not record an update or another creation")
	require.Equal(t, model.BusinessEventTypePrefix+"grant-created", events[0].Type)
	require.Equal(t, grantID, events[0].GrantID)
	require.Equal(t, agent.ID, events[0].AgentID)
}
