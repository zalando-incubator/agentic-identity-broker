//go:build integration

package storage_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type agentLedgerRequirementValidator struct{}

func (agentLedgerRequirementValidator) ValidateServiceRequirements(context.Context, []storage.ServiceRequirement) error {
	return nil
}

func TestAgentLedgerConcurrentIdenticalUpdateRecordsOnlyWinner(t *testing.T) {
	adapter, owner := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	recorder := ledger.NewService(registry, adapter.BusinessEvents(), adapter.BusinessEventLifecycle(), adapter, false)
	service := agents.NewService(adapter.Agents(), agentLedgerRequirementValidator{}, slog.Default(), true, recorder, adapter.Agents().(ports.AgentDependentRepository), adapter.BrokerCredentials())
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Original", Description: "Description", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}}}
	_, err = owner.ExecContext(ctx, `INSERT INTO permission_sets (id, name, description, created_at, updated_at) VALUES ($1, 'Ledger test', 'Ledger test', now(), now())`, agent.PermissionSets[0].PermissionSetID)
	require.NoError(t, err)
	require.NoError(t, adapter.Agents().Create(ctx, agent))
	changed := agent.Copy()
	changed.DisplayName = "Changed"
	first, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = adapter.Rollback(first) }()
	require.NoError(t, service.Update(first, agent.ID, changed.Copy(), false))
	second := make(chan error, 1)
	go func() { second <- service.Update(ctx, agent.ID, changed.Copy(), false) }()
	require.Eventually(t, func() bool {
		var waiting int
		err := owner.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`)
		return err == nil && waiting > 0
	}, 3*time.Second, 10*time.Millisecond, "the competing request must reach a business-row lock before the owner commits")
	require.NoError(t, adapter.Commit(first))
	require.NoError(t, <-second)
	events, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{NoSubject: true}, Type: model.BusinessEventTypePrefix + "agent-updated", Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour), Limit: 100})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, agent.ID, events[0].AgentID)
	stored, err := adapter.Agents().Get(ctx, agent.ID)
	require.NoError(t, err)
	require.Equal(t, "Changed", stored.DisplayName)
}
