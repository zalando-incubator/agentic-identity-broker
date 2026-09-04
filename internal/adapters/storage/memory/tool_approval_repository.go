package memory

import (
	"context"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ToolApprovalRepository implements the three tool approval repository interfaces in-memory.
type ToolApprovalRepository struct {
	mu        sync.RWMutex
	approvals map[id.ApprovalID]*storage.ToolApproval
}

// NewToolApprovalRepository creates a new in-memory ToolApprovalRepository.
func NewToolApprovalRepository() *ToolApprovalRepository {
	return &ToolApprovalRepository{
		approvals: make(map[id.ApprovalID]*storage.ToolApproval),
	}
}

// Compile-time interface checks.
var (
	_ ports.ToolApprovalRepository        = (*ToolApprovalRepository)(nil)
	_ ports.ToolApprovalQueryRepository   = (*ToolApprovalRepository)(nil)
	_ ports.ToolApprovalMetricsRepository = (*ToolApprovalRepository)(nil)
)

var _ ports.ApprovalMutationSyncRepository = (*ToolApprovalRepository)(nil)

func (*ToolApprovalRepository) ApprovalMutationsSyncAtomically() bool {
	return false
}

func (r *ToolApprovalRepository) Create(_ context.Context, approval *storage.ToolApproval) (*storage.ToolApproval, error) {
	if approval.ToolPattern == "" {
		return nil, storage.NewStorageError("Create", storage.ErrorKindValidation, storage.ErrApprovalPatternMissing, "approval tool pattern is required")
	}
	if approval.ParamsPattern == nil {
		approval.ParamsPattern = map[string]string{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.approvals {
		if existing.Principal == approval.Principal &&
			existing.AgentID == approval.AgentID &&
			existing.ToolName == approval.ToolName &&
			existing.ArgumentsHash == approval.ArgumentsHash &&
			existing.Status == storage.ApprovalStatusPending &&
			!existing.Consumed {
			if existing.IsExpired(time.Now()) {
				existing.Consumed = true
				continue
			}
			return copyApproval(existing), nil
		}
	}

	r.approvals[approval.ID] = copyApproval(approval)
	return copyApproval(approval), nil
}

func (r *ToolApprovalRepository) Get(_ context.Context, approvalID id.ApprovalID) (*storage.ToolApproval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found")
	}
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) Approve(_ context.Context, approvalID id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error) {
	if decision.ToolPattern == "" {
		return nil, storage.NewStorageError("Approve", storage.ErrorKindValidation, storage.ErrApprovalPatternMissing, "approval tool pattern is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok || a.Status != storage.ApprovalStatusPending || a.IsExpired(time.Now()) {
		return nil, storage.NewStorageError("Approve", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found or not pending")
	}

	a.Status = storage.ApprovalStatusApproved
	a.Persistence = &decision.Persistence
	a.ToolPattern = decision.ToolPattern
	a.ParamsPattern = copyParamsPattern(decision.ParamsPattern)
	a.ApprovedAt = &approvedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) Deny(_ context.Context, approvalID id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok || a.Status != storage.ApprovalStatusPending || a.IsExpired(time.Now()) {
		return nil, storage.NewStorageError("Deny", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found or not pending")
	}

	a.Status = storage.ApprovalStatusDenied
	a.Persistence = persistence
	a.DeniedAt = &deniedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) RevokePermanent(_ context.Context, approvalID id.ApprovalID, revokedAt time.Time) (*storage.ToolApproval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok || a.Persistence == nil || *a.Persistence != storage.ApprovalPersistencePermanent ||
		(a.Status != storage.ApprovalStatusApproved && a.Status != storage.ApprovalStatusDenied) {
		return nil, storage.NewStorageError("RevokePermanent", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found or not permanently approved or denied")
	}

	a.Status = storage.ApprovalStatusDenied
	a.Persistence = nil
	a.DeniedAt = &revokedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) Consume(_ context.Context, approvalID id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Consume", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found")
	}

	// Only approved + once-persistence approvals are consumable
	if a.Status != storage.ApprovalStatusApproved || a.Persistence == nil || *a.Persistence != storage.ApprovalPersistenceOnce {
		return nil, storage.NewStorageError("Consume", storage.ErrorKindValidation, nil, "only once-persistence approved approvals can be consumed")
	}

	if a.Consumed {
		return copyApproval(a), nil // Idempotent
	}

	a.Consumed = true
	a.ConsumedAt = &consumedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) ListAllActive(_ context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) ([]*storage.ToolApproval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	activeSessions := make(map[string]struct{}, len(activeAgentSessionIDs))
	for _, sessionID := range activeAgentSessionIDs {
		activeSessions[sessionID] = struct{}{}
	}

	var result []*storage.ToolApproval
	now := time.Now()
	for _, a := range r.approvals {
		if principalFilter != nil && a.Principal != *principalFilter {
			continue
		}
		if a.Status == storage.ApprovalStatusPending && a.IsExpired(now) {
			continue
		}
		if a.Consumed {
			continue
		}
		if a.Persistence != nil && *a.Persistence == storage.ApprovalPersistenceSession &&
			(a.AgentSessionID == nil || *a.AgentSessionID == "") {
			continue
		}
		if a.Persistence != nil && *a.Persistence == storage.ApprovalPersistenceSession {
			if _, ok := activeSessions[*a.AgentSessionID]; !ok {
				continue
			}
		}
		result = append(result, copyApproval(a))
	}
	return result, nil
}

func (r *ToolApprovalRepository) ListActiveByPrincipalAndAgent(_ context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.ToolApproval
	now := time.Now()
	for _, a := range r.approvals {
		if a.Principal != principal || a.AgentID != agentID {
			continue
		}
		if a.Status == storage.ApprovalStatusPending && a.IsExpired(now) {
			continue
		}
		if a.Consumed {
			continue
		}
		result = append(result, copyApproval(a))
	}
	return result, nil
}

func (r *ToolApprovalRepository) ListPermanentByPrincipal(_ context.Context, principal id.Principal) ([]*storage.ToolApproval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.ToolApproval
	for _, a := range r.approvals {
		if a.Principal != principal {
			continue
		}
		if a.Persistence != nil && *a.Persistence == storage.ApprovalPersistencePermanent {
			result = append(result, copyApproval(a))
		}
	}
	return result, nil
}

func (r *ToolApprovalRepository) CountPendingByPrincipalAndAgent(_ context.Context, principal id.Principal, agentID id.AgentID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, a := range r.approvals {
		if a.Principal == principal && a.AgentID == agentID &&
			a.Status == storage.ApprovalStatusPending && !a.IsExpired(now) {
			count++
		}
	}
	return count, nil
}

func copyApproval(a *storage.ToolApproval) *storage.ToolApproval {
	cp := *a
	if a.Arguments != nil {
		cp.Arguments = make(map[string]any, len(a.Arguments))
		for k, v := range a.Arguments {
			cp.Arguments[k] = v
		}
	}
	if a.ParamsPattern != nil {
		cp.ParamsPattern = copyParamsPattern(a.ParamsPattern)
	}
	if a.Persistence != nil {
		p := *a.Persistence
		cp.Persistence = &p
	}
	if a.ApprovedAt != nil {
		t := *a.ApprovedAt
		cp.ApprovedAt = &t
	}
	if a.DeniedAt != nil {
		t := *a.DeniedAt
		cp.DeniedAt = &t
	}
	if a.ConsumedAt != nil {
		t := *a.ConsumedAt
		cp.ConsumedAt = &t
	}
	return &cp
}

func copyParamsPattern(params map[string]string) map[string]string {
	if params == nil {
		return nil
	}
	cp := make(map[string]string, len(params))
	for key, value := range params {
		cp[key] = value
	}
	return cp
}
