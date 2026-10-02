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
	mu                    sync.RWMutex
	transactions          *TransactionManager
	approvals             map[id.ApprovalID]*storage.ToolApproval
	expirationRecordedFor map[id.ApprovalID]time.Time
}

// NewToolApprovalRepository creates a new in-memory ToolApprovalRepository.
func NewToolApprovalRepository(transactions *TransactionManager) *ToolApprovalRepository {
	return &ToolApprovalRepository{
		transactions:          transactions,
		approvals:             make(map[id.ApprovalID]*storage.ToolApproval),
		expirationRecordedFor: make(map[id.ApprovalID]time.Time),
	}
}

func (r *ToolApprovalRepository) ListUnrecordedExpired(ctx context.Context, at time.Time, limit int) ([]*storage.ToolApproval, error) {
	if at.IsZero() || limit <= 0 || limit > 1000 {
		return nil, storage.NewStorageError("ListExpiredToolApprovals", storage.ErrorKindValidation, nil, "invalid expiration query")
	}
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*storage.ToolApproval
	for _, approval := range r.approvals {
		if approval.Status == storage.ApprovalStatusPending && approval.IsExpired(at) && !r.expirationRecordedFor[approval.ID].Equal(approval.ExpiresAt) {
			result = append(result, copyApproval(approval))
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (r *ToolApprovalRepository) RecordExpiration(ctx context.Context, approvalID id.ApprovalID, effectiveExpiry time.Time) (bool, error) {
	if approvalID.IsZero() || effectiveExpiry.IsZero() {
		return false, storage.NewStorageError("RecordToolApprovalExpiration", storage.ErrorKindValidation, nil, "invalid expiration recognition")
	}
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return false, err
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()
	approval := r.approvals[approvalID]
	if approval == nil || approval.Status != storage.ApprovalStatusPending || !approval.IsExpired(time.Now()) || !approval.ExpiresAt.Equal(effectiveExpiry) || r.expirationRecordedFor[approvalID].Equal(effectiveExpiry) {
		return false, nil
	}
	journalEntry(ctx, r.expirationRecordedFor, approvalID)
	r.expirationRecordedFor[approvalID] = effectiveExpiry
	return true, nil
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

func (r *ToolApprovalRepository) Create(ctx context.Context, approval *storage.ToolApproval) (*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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
				if !r.expirationRecordedFor[existing.ID].Equal(existing.ExpiresAt) {
					return copyApproval(existing), nil
				}
				updated := *existing
				updated.Consumed = true
				journalEntry(ctx, r.approvals, updated.ID)
				r.approvals[updated.ID] = &updated
				continue
			}
			return copyApproval(existing), nil
		}
	}

	journalEntry(ctx, r.approvals, approval.ID)
	r.approvals[approval.ID] = copyApproval(approval)
	return copyApproval(approval), nil
}

func (r *ToolApprovalRepository) Get(ctx context.Context, approvalID id.ApprovalID) (*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found")
	}
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) Approve(ctx context.Context, approvalID id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	if decision.ToolPattern == "" {
		return nil, storage.NewStorageError("Approve", storage.ErrorKindValidation, storage.ErrApprovalPatternMissing, "approval tool pattern is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok || a.Status != storage.ApprovalStatusPending || a.IsExpired(time.Now()) {
		return nil, storage.NewStorageError("Approve", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found or not pending")
	}

	updated := *a
	a = &updated
	journalEntry(ctx, r.approvals, approvalID)
	r.approvals[approvalID] = a
	a.Status = storage.ApprovalStatusApproved
	a.Persistence = &decision.Persistence
	a.ToolPattern = decision.ToolPattern
	a.ParamsPattern = copyParamsPattern(decision.ParamsPattern)
	a.ApprovedAt = &approvedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) Deny(ctx context.Context, approvalID id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok || a.Status != storage.ApprovalStatusPending || a.IsExpired(time.Now()) {
		return nil, storage.NewStorageError("Deny", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found or not pending")
	}

	updated := *a
	a = &updated
	journalEntry(ctx, r.approvals, approvalID)
	r.approvals[approvalID] = a
	a.Status = storage.ApprovalStatusDenied
	a.Persistence = copyPointer(persistence)
	a.DeniedAt = &deniedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) RevokePermanent(ctx context.Context, approvalID id.ApprovalID, revokedAt time.Time) (*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[approvalID]
	if !ok || a.Persistence == nil || *a.Persistence != storage.ApprovalPersistencePermanent ||
		(a.Status != storage.ApprovalStatusApproved && a.Status != storage.ApprovalStatusDenied) {
		return nil, storage.NewStorageError("RevokePermanent", storage.ErrorKindNotFound, ports.ErrNotFound, "approval not found or not permanently approved or denied")
	}

	updated := *a
	a = &updated
	journalEntry(ctx, r.approvals, approvalID)
	r.approvals[approvalID] = a
	a.Status = storage.ApprovalStatusDenied
	a.Persistence = nil
	a.DeniedAt = &revokedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) Consume(ctx context.Context, approvalID id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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

	updated := *a
	a = &updated
	journalEntry(ctx, r.approvals, approvalID)
	r.approvals[approvalID] = a
	a.Consumed = true
	a.ConsumedAt = &consumedAt
	return copyApproval(a), nil
}

func (r *ToolApprovalRepository) ListAllActive(ctx context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) ([]*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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

func (r *ToolApprovalRepository) ListActiveByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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

func (r *ToolApprovalRepository) ListPermanentByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.ToolApproval, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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

func (r *ToolApprovalRepository) CountPendingByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
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
	cp.MCPSessionID = copyPointer(a.MCPSessionID)
	cp.AgentSessionID = copyPointer(a.AgentSessionID)
	cp.ToolInvocationID = copyPointer(a.ToolInvocationID)
	cp.OpenTelemetryTraceparent = copyPointer(a.OpenTelemetryTraceparent)
	if a.Arguments != nil {
		cp.Arguments = make(map[string]any, len(a.Arguments))
		for k, v := range a.Arguments {
			cp.Arguments[k] = copyJSONValue(v)
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
		return map[string]string{}
	}
	cp := make(map[string]string, len(params))
	for key, value := range params {
		cp[key] = value
	}
	return cp
}
