package memory

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.AgentDependentRepository = (*AgentRepository)(nil)

func (r *AgentRepository) ListGrantsByAgent(ctx context.Context, agentID id.AgentID) ([]*storage.UserGrant, error) {
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	if r.grants == nil {
		return nil, nil
	}
	r.grants.mu.RLock()
	defer r.grants.mu.RUnlock()
	ids := r.grants.grantIDsByAgent[agentID]
	result := make([]*storage.UserGrant, 0, len(ids))
	for _, grantID := range ids {
		if grant := r.grants.grants[grantID]; grant != nil {
			result = append(result, grant.Copy())
		}
	}
	return result, nil
}

func (r *AgentRepository) ListApprovalsByAgent(ctx context.Context, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	if r.approvals == nil {
		return nil, nil
	}
	r.approvals.mu.RLock()
	defer r.approvals.mu.RUnlock()
	var result []*storage.ToolApproval
	for _, approval := range r.approvals.approvals {
		if approval.AgentID == agentID {
			result = append(result, copyApproval(approval))
		}
	}
	return result, nil
}

func (r *ToolApprovalRepository) deleteByAgent(ctx context.Context, agentID id.AgentID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for approvalID, approval := range r.approvals {
		if approval.AgentID == agentID {
			journalEntry(ctx, r.approvals, approvalID)
			delete(r.approvals, approvalID)
			journalEntry(ctx, r.expirationRecordedFor, approvalID)
			delete(r.expirationRecordedFor, approvalID)
		}
	}
}

func (r *AgentRepository) deleteDependents(ctx context.Context, agentID id.AgentID) error {
	if r.grants != nil {
		if err := r.grants.DeleteByAgent(ctx, agentID); err != nil {
			return err
		}
	}
	if r.approvals != nil {
		r.approvals.deleteByAgent(ctx, agentID)
	}
	if r.credentials != nil {
		if err := r.credentials.Delete(ctx, agentID); err != nil && !ports.IsNotFoundErr(err) {
			return err
		}
	}
	return nil
}
