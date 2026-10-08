package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

type cascadeDiscoveryTransactions struct {
	*ledgerfixture.Store
	begins        int
	rollbacks     int
	hints         []ports.StorageTransactionHints
	beforeBegin   func(context.Context)
	afterRollback func()
	beginError    error
	rollbackError error
}

func (tx *cascadeDiscoveryTransactions) BeginTX(ctx context.Context) (context.Context, error) {
	tx.begins++
	if _, joined := ports.StorageTransactionEffectsFromContext(ctx); !joined {
		tx.hints = append(tx.hints, ports.StorageTransactionHintsFromContext(ctx))
		if tx.beginError != nil {
			return nil, tx.beginError
		}
		if tx.beforeBegin != nil {
			tx.beforeBegin(ctx)
		}
	}
	return tx.Store.BeginTX(ctx)
}

func (tx *cascadeDiscoveryTransactions) Rollback(ctx context.Context) error {
	tx.rollbacks++
	if err := tx.Store.Rollback(ctx); err != nil {
		return err
	}
	if tx.afterRollback != nil {
		tx.afterRollback()
	}
	return tx.rollbackError
}

func observeCascadeTransactions(t *testing.T, svc *Service, store *ledgerfixture.Store) *cascadeDiscoveryTransactions {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	tx := &cascadeDiscoveryTransactions{Store: store}
	svc.ledger = ledger.NewService(registry, store, nil, tx, false)
	return tx
}

func TestAgentCascadeNewSubjectRestartsWithCompleteSortedLocks(t *testing.T) {
	for _, dependent := range []string{"grant", "approval"} {
		t.Run(dependent, func(t *testing.T) {
			svc, repo, store, dependents, _ := newLedgerAgent(t)
			agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent"}
			repo.agents[agent.ID] = agent
			original := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: "z-original"}
			dependents.grants[original.ID] = original
			tx := observeCascadeTransactions(t, svc, store)
			tx.beforeBegin = func(context.Context) {
				attempt := len(tx.hints)
				if attempt > 2 {
					return
				}
				require.Equal(t, attempt-1, tx.rollbacks, "rediscovery must follow completed rollback")
				require.Contains(t, repo.agents, agent.ID)
				require.Empty(t, store.Events)
				principal := []id.Principal{"b-first", "a-second"}[attempt-1]
				if dependent == "grant" {
					grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: principal}
					dependents.grants[grant.ID] = grant
				} else {
					permanent := storage.ApprovalPersistencePermanent
					approval := &storage.ToolApproval{ID: id.NewApprovalID(), AgentID: agent.ID, Principal: principal, Status: storage.ApprovalStatusApproved, Persistence: &permanent, ExpiresAt: time.Now().Add(time.Hour)}
					dependents.approvals[approval.ID] = approval
				}
			}
			tx.afterRollback = func() {
				if tx.rollbacks == 1 {
					grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: "c-after-rollback"}
					dependents.grants[grant.ID] = grant
				}
			}
			inherited := ports.StorageSubjectGate{Principal: "m-inherited", Exclusive: true}
			ctx := ports.WithStorageTransactionHints(context.Background(), ports.StorageTransactionHints{
				Isolation: ports.StorageSerializable,
				Subjects:  []ports.StorageSubjectGate{inherited},
			})
			require.NoError(t, svc.Delete(ctx, agent.ID))
			require.Equal(t, 2, tx.rollbacks)
			require.Equal(t, []ports.StorageTransactionHints{
				{Isolation: ports.StorageSerializable, Subjects: []ports.StorageSubjectGate{inherited, {Principal: "z-original"}}},
				{Isolation: ports.StorageSerializable, Subjects: []ports.StorageSubjectGate{{Principal: "b-first"}, {Principal: "c-after-rollback"}, inherited, {Principal: "z-original"}}},
				{Isolation: ports.StorageSerializable, Subjects: []ports.StorageSubjectGate{{Principal: "a-second"}, {Principal: "b-first"}, {Principal: "c-after-rollback"}, inherited, {Principal: "z-original"}}},
			}, tx.hints)
			require.Equal(t, []ports.StorageSubjectGate{inherited}, ports.StorageTransactionHintsFromContext(ctx).Subjects)
			require.NotContains(t, repo.agents, agent.ID)
			require.Empty(t, dependents.grants)
			require.Empty(t, dependents.approvals)
			require.Len(t, store.Events, 5)
			counts := make(map[string]int)
			subjects := make(map[id.Principal]int)
			for _, event := range store.Events {
				counts[event.Type]++
				require.Equal(t, agent.ID, event.AgentID)
				if event.Subject != nil {
					subjects[*event.Subject]++
				}
			}
			require.Equal(t, map[id.Principal]int{"a-second": 1, "b-first": 1, "c-after-rollback": 1, "z-original": 1}, subjects)
			require.Equal(t, 1, counts[model.BusinessEventTypePrefix+"agent-deleted"])
			if dependent == "grant" {
				require.Equal(t, 4, counts[model.BusinessEventTypePrefix+"grant-revoked"])
			} else {
				require.Equal(t, 2, counts[model.BusinessEventTypePrefix+"grant-revoked"])
				require.Equal(t, 2, counts[model.BusinessEventTypePrefix+"approval-revoked"])
			}
		})
	}
}

func TestAgentCascadeRediscoveryRespectsCancellation(t *testing.T) {
	for _, cancelAt := range []string{"before discovery", "before locked reload", "after rollback"} {
		t.Run(cancelAt, func(t *testing.T) {
			svc, repo, store, dependents, _ := newLedgerAgent(t)
			agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent"}
			repo.agents[agent.ID] = agent
			tx := observeCascadeTransactions(t, svc, store)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelAt == "before discovery" {
				cancel()
			} else {
				tx.beforeBegin = func(context.Context) {
					require.Len(t, tx.hints, 1, "a cancelled deletion must not retry")
					grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: "new-subject"}
					dependents.grants[grant.ID] = grant
					if cancelAt == "before locked reload" {
						cancel()
					}
				}
				tx.afterRollback = cancel
			}
			require.ErrorIs(t, svc.Delete(ctx, agent.ID), context.Canceled)
			require.Equal(t, agent, repo.agents[agent.ID])
			require.Empty(t, store.Events)
			if cancelAt == "before discovery" {
				require.Zero(t, tx.begins)
			} else {
				require.Equal(t, 1, tx.begins)
				require.Equal(t, 1, tx.rollbacks)
			}
		})
	}
}

func TestAgentCascadeRollbackFailureStopsRediscovery(t *testing.T) {
	svc, repo, store, dependents, _ := newLedgerAgent(t)
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent"}
	repo.agents[agent.ID] = agent
	tx := observeCascadeTransactions(t, svc, store)
	tx.rollbackError = errors.New("rollback failed")
	tx.beforeBegin = func(context.Context) {
		require.Len(t, tx.hints, 1, "a failed rollback must not retry")
		grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: "new-subject"}
		dependents.grants[grant.ID] = grant
	}
	err := svc.Delete(context.Background(), agent.ID)
	require.ErrorIs(t, err, tx.rollbackError)
	require.Equal(t, 1, tx.begins)
	require.Equal(t, 1, tx.rollbacks)
	require.Equal(t, agent, repo.agents[agent.ID])
	require.Empty(t, store.Events)
}

func TestAgentCascadeJoinedScopeCannotRestartItsOwner(t *testing.T) {
	svc, repo, store, dependents, _ := newLedgerAgent(t)
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent"}
	repo.agents[agent.ID] = agent
	tx := observeCascadeTransactions(t, svc, store)
	ownerCtx, err := store.BeginTX(context.Background())
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Rollback(ownerCtx)) }()
	repo.getFn = func(context.Context, id.AgentID) (*storage.Agent, error) {
		grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: "new-subject"}
		dependents.grants[grant.ID] = grant
		return agent.Copy(), nil
	}
	require.Error(t, svc.Delete(ownerCtx, agent.ID))
	require.Equal(t, 1, tx.begins, "a joined rollback cannot release the owner's subject and business locks")
	require.Equal(t, 1, tx.rollbacks)
	require.Error(t, store.Commit(ownerCtx), "the owning caller must not commit the failed joined deletion")
	require.Equal(t, agent, repo.agents[agent.ID])
	require.Empty(t, store.Events)
}

func TestAgentCascadeDoesNotRetryOtherTransactionFailures(t *testing.T) {
	for _, failAt := range []string{"begin", "reload", "delete", "append", "commit"} {
		t.Run(failAt, func(t *testing.T) {
			svc, repo, store, _, _ := newLedgerAgent(t)
			agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent"}
			repo.agents[agent.ID] = agent
			tx := observeCascadeTransactions(t, svc, store)
			failure := storage.NewStorageError("DeleteAgent", storage.ErrorKindConflict, nil, "unrelated storage conflict")
			switch failAt {
			case "begin":
				tx.beginError = failure
			case "reload":
				repo.getFn = func(context.Context, id.AgentID) (*storage.Agent, error) { return nil, failure }
			case "delete":
				repo.deleteFn = func(context.Context, id.AgentID) error { return failure }
			case "append":
				store.AppendError = failure
			case "commit":
				store.CommitError = failure
			}
			require.ErrorIs(t, svc.Delete(context.Background(), agent.ID), failure)
			require.Len(t, tx.hints, 1, "only changed cascade discovery permits a fresh local deletion transaction")
			require.Equal(t, agent, repo.agents[agent.ID])
			require.Empty(t, store.Events)
		})
	}
}
