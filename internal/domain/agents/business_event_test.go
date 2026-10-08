package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

type ledgerAgentDependents struct {
	grants    map[id.GrantID]*storage.UserGrant
	approvals map[id.ApprovalID]*storage.ToolApproval
}

func (d *ledgerAgentDependents) ListGrantsByAgent(_ context.Context, agentID id.AgentID) ([]*storage.UserGrant, error) {
	var result []*storage.UserGrant
	for _, grant := range d.grants {
		if grant.AgentID == agentID {
			result = append(result, grant.Copy())
		}
	}
	return result, nil
}
func (d *ledgerAgentDependents) ListApprovalsByAgent(_ context.Context, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	var result []*storage.ToolApproval
	for _, approval := range d.approvals {
		if approval.AgentID == agentID {
			copy := *approval
			result = append(result, &copy)
		}
	}
	return result, nil
}

type ledgerAgentCredentials struct {
	ports.ClientCredentialRepository
	records map[id.AgentID]*storage.ClientCredential
}

func (r *ledgerAgentCredentials) GetByAgentID(_ context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	credential := r.records[agentID]
	if credential == nil {
		return nil, ports.ErrNotFound
	}
	copy := *credential
	return &copy, nil
}
func (r *ledgerAgentCredentials) Delete(_ context.Context, agentID id.AgentID) error {
	delete(r.records, agentID)
	return nil
}

func newLedgerAgent(t *testing.T) (*Service, *mockAgentRepo, *ledgerfixture.Store, *ledgerAgentDependents, *ledgerAgentCredentials) {
	t.Helper()
	repo := newMockAgentRepo()
	dependents := &ledgerAgentDependents{grants: make(map[id.GrantID]*storage.UserGrant), approvals: make(map[id.ApprovalID]*storage.ToolApproval)}
	credentials := &ledgerAgentCredentials{records: make(map[id.AgentID]*storage.ClientCredential)}
	repo.deleteFn = func(_ context.Context, agentID id.AgentID) error {
		delete(repo.agents, agentID)
		for key, grant := range dependents.grants {
			if grant.AgentID == agentID {
				delete(dependents.grants, key)
			}
		}
		for key, approval := range dependents.approvals {
			if approval.AgentID == agentID {
				delete(dependents.approvals, key)
			}
		}
		delete(credentials.records, agentID)
		return nil
	}
	store := &ledgerfixture.Store{Snapshot: func() func() {
		agents := make(map[id.AgentID]*storage.Agent, len(repo.agents))
		for key, agent := range repo.agents {
			agents[key] = agent.Copy()
		}
		grants := make(map[id.GrantID]*storage.UserGrant, len(dependents.grants))
		for key, grant := range dependents.grants {
			grants[key] = grant.Copy()
		}
		approvals := make(map[id.ApprovalID]*storage.ToolApproval, len(dependents.approvals))
		for key, approval := range dependents.approvals {
			copy := *approval
			approvals[key] = &copy
		}
		credentialRows := make(map[id.AgentID]*storage.ClientCredential, len(credentials.records))
		for key, credential := range credentials.records {
			copy := *credential
			credentialRows[key] = &copy
		}
		return func() {
			repo.agents, dependents.grants, dependents.approvals, credentials.records = agents, grants, approvals, credentialRows
		}
	}}
	svc := newTestService(repo, false)
	svc.ledger, svc.dependents, svc.credentials = store.Recorder(t), dependents, credentials
	return svc, repo, store, dependents, credentials
}

func TestAgentLedgerChangedConfigurationAndActualDeletionOnly(t *testing.T) {
	svc, _, store, _, _ := newLedgerAgent(t)
	ctx := context.Background()
	agent := &storage.Agent{DisplayName: "Agent", Description: "credential-canary-description", PermissionSets: testPermissionSets()}
	require.NoError(t, svc.Create(ctx, agent))
	require.NoError(t, svc.Update(ctx, agent.ID, agent.Copy(), false))
	changed := agent.Copy()
	changed.DisplayName = "Renamed"
	require.NoError(t, svc.Update(ctx, agent.ID, changed, false))
	require.NoError(t, svc.Delete(ctx, agent.ID))
	require.NoError(t, svc.Delete(ctx, agent.ID))
	require.Len(t, store.Events, 3)
	for i, name := range []string{"agent-registered", "agent-updated", "agent-deleted"} {
		event := store.Events[i]
		require.Equal(t, model.BusinessEventTypePrefix+name, event.Type)
		require.Equal(t, agent.ID, event.AgentID)
		require.Nil(t, event.Subject)
		require.Empty(t, event.Data, "agent configuration is not ledger data")
	}
}

func TestAgentLedgerFailureRollsBackEveryMutation(t *testing.T) {
	for _, action := range []string{"create", "update", "delete"} {
		for _, failure := range []string{"append", "commit"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				svc, repo, store, _, _ := newLedgerAgent(t)
				agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent", Description: "Description", PermissionSets: testPermissionSets()}
				if action != "create" {
					repo.agents[agent.ID] = agent.Copy()
				}
				failed := errors.New("ledger unavailable")
				if failure == "append" {
					store.AppendError = failed
				} else {
					store.CommitError = failed
				}
				ctx := context.Background()
				var err error
				switch action {
				case "create":
					err = svc.Create(ctx, agent)
				case "update":
					changed := agent.Copy()
					changed.DisplayName = "Renamed"
					err = svc.Update(ctx, agent.ID, changed, false)
				case "delete":
					err = svc.Delete(ctx, agent.ID)
				}
				require.Error(t, err)
				require.Empty(t, store.Events)
				if action == "create" {
					require.Empty(t, repo.agents)
				} else {
					require.Equal(t, agent, repo.agents[agent.ID])
				}
			})
		}
	}
}

func TestAgentLedgerDeletionCapturesCompleteActualCascade(t *testing.T) {
	svc, repo, store, dependents, credentials := newLedgerAgent(t)
	agentID, otherID := id.NewAgentID(), id.NewAgentID()
	repo.agents[agentID] = &storage.Agent{ID: agentID, DisplayName: "Agent"}
	grantIDs := []id.GrantID{id.NewGrantID(), id.NewGrantID()}
	for i, principal := range []id.Principal{"first-user", "second-user"} {
		dependents.grants[grantIDs[i]] = &storage.UserGrant{ID: grantIDs[i], AgentID: agentID, Principal: principal}
	}
	permanent := storage.ApprovalPersistencePermanent
	approvedID, deniedID := id.NewApprovalID(), id.NewApprovalID()
	dependents.approvals[approvedID] = &storage.ToolApproval{ID: approvedID, AgentID: agentID, Principal: "first-user", Status: storage.ApprovalStatusApproved, Persistence: &permanent, ExpiresAt: time.Now().Add(time.Hour)}
	dependents.approvals[deniedID] = &storage.ToolApproval{ID: deniedID, AgentID: agentID, Principal: "second-user", Status: storage.ApprovalStatusDenied, Persistence: &permanent, ExpiresAt: time.Now().Add(time.Hour)}
	credentialID := id.NewCredentialID()
	credentials.records[agentID] = &storage.ClientCredential{ID: credentialID, AgentID: agentID, SecretHash: "credential-canary-hash"}
	otherGrant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: otherID, Principal: "other-user"}
	dependents.grants[otherGrant.ID] = otherGrant
	require.NoError(t, svc.Delete(context.Background(), agentID))
	require.NotContains(t, repo.agents, agentID)
	require.Equal(t, map[id.GrantID]*storage.UserGrant{otherGrant.ID: otherGrant}, dependents.grants)
	require.Empty(t, dependents.approvals)
	require.Empty(t, credentials.records)
	require.Len(t, store.Events, 5)
	counts := make(map[string]int)
	for _, event := range store.Events {
		counts[event.Type]++
		require.Equal(t, agentID, event.AgentID)
		switch event.Type {
		case model.BusinessEventTypePrefix + "grant-revoked":
			require.Contains(t, grantIDs, event.GrantID)
			require.Contains(t, []id.Principal{"first-user", "second-user"}, *event.Subject)
		case model.BusinessEventTypePrefix + "approval-revoked":
			require.Equal(t, approvedID, event.ApprovalID)
		case model.BusinessEventTypePrefix + "credential-revoked":
			require.Equal(t, credentialID.String(), event.Data["credential_id"])
		}
	}
	require.Equal(t, map[string]int{model.BusinessEventTypePrefix + "agent-deleted": 1, model.BusinessEventTypePrefix + "grant-revoked": 2, model.BusinessEventTypePrefix + "approval-revoked": 1, model.BusinessEventTypePrefix + "credential-revoked": 1}, counts)
}

func TestAgentLedgerCascadeRecordingFailureRestoresAllDependents(t *testing.T) {
	svc, repo, store, dependents, credentials := newLedgerAgent(t)
	agentID := id.NewAgentID()
	agent := &storage.Agent{ID: agentID, DisplayName: "Agent"}
	repo.agents[agentID] = agent
	grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: "ledger-user"}
	dependents.grants[grant.ID] = grant
	persistence := storage.ApprovalPersistencePermanent
	approval := &storage.ToolApproval{ID: id.NewApprovalID(), AgentID: agentID, Principal: "ledger-user", Status: storage.ApprovalStatusApproved, Persistence: &persistence, ExpiresAt: time.Now().Add(time.Hour)}
	dependents.approvals[approval.ID] = approval
	credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agentID, SecretHash: "credential-canary-hash"}
	credentials.records[agentID] = credential
	store.AppendError = errors.New("ledger unavailable")
	require.Error(t, svc.Delete(context.Background(), agentID))
	require.Equal(t, agent, repo.agents[agentID])
	require.Equal(t, grant, dependents.grants[grant.ID])
	require.Equal(t, approval, dependents.approvals[approval.ID])
	require.Equal(t, credential, credentials.records[agentID])
	require.Empty(t, store.Events)
}
