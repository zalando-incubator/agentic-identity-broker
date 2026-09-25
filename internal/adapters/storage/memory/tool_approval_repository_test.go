package memory

import (
	"context"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestToolApprovalRepository_CreateReplacesExpiredDuplicate(t *testing.T) {
	repo := NewToolApprovalRepository()
	now := time.Now()
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()
	first := &storage.ToolApproval{
		ID:            id.NewApprovalID(),
		Principal:     principal,
		AgentID:       agentID,
		ToolName:      "read_file",
		ToolPattern:   "read_file",
		ArgumentsHash: "same",
		Status:        storage.ApprovalStatusPending,
		ApprovalURL:   "https://broker.example.com/consent/approvals/old",
		CreatedAt:     now.Add(-2 * time.Minute),
		ExpiresAt:     now.Add(-time.Minute),
	}
	second := &storage.ToolApproval{
		ID:            id.NewApprovalID(),
		Principal:     principal,
		AgentID:       agentID,
		ToolName:      "read_file",
		ToolPattern:   "read_file",
		ArgumentsHash: "same",
		Status:        storage.ApprovalStatusPending,
		ApprovalURL:   "https://broker.example.com/consent/approvals/new",
		CreatedAt:     now,
		ExpiresAt:     now.Add(time.Minute),
	}

	_, err := repo.Create(context.Background(), first)
	require.NoError(t, err)
	created, err := repo.Create(context.Background(), second)
	require.NoError(t, err)

	require.Equal(t, second.ID, created.ID)
	require.Equal(t, second.ApprovalURL, created.ApprovalURL)
	old, err := repo.Get(context.Background(), first.ID)
	require.NoError(t, err)
	require.True(t, old.Consumed)
}

func TestToolApprovalRepository_RevokePermanentClearsPermanentDenial(t *testing.T) {
	repo := NewToolApprovalRepository()
	now := time.Now()
	permanent := storage.ApprovalPersistencePermanent
	approval := &storage.ToolApproval{
		ID:            id.NewApprovalID(),
		Principal:     id.Principal("user@example.com"),
		AgentID:       id.NewAgentID(),
		ToolName:      "read_file",
		ArgumentsHash: "same",
		Status:        storage.ApprovalStatusDenied,
		Persistence:   &permanent,
		ApprovalURL:   "https://broker.example.com/consent/approvals/test",
		CreatedAt:     now,
		DeniedAt:      &now,
		ExpiresAt:     now.Add(time.Minute),
	}
	repo.approvals[approval.ID] = approval

	result, err := repo.RevokePermanent(context.Background(), approval.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, storage.ApprovalStatusDenied, result.Status)
	require.Nil(t, result.Persistence)

	_, err = repo.RevokePermanent(context.Background(), approval.ID, now.Add(2*time.Second))
	require.ErrorIs(t, err, ports.ErrNotFound)
}

func TestToolApprovalRepository_RejectsExpiredPendingResolution(t *testing.T) {
	repo := NewToolApprovalRepository()
	now := time.Now()
	approval := &storage.ToolApproval{
		ID:            id.NewApprovalID(),
		Principal:     id.Principal("user@example.com"),
		AgentID:       id.NewAgentID(),
		ToolName:      "read_file",
		ArgumentsHash: "same",
		Status:        storage.ApprovalStatusPending,
		ApprovalURL:   "https://broker.example.com/consent/approvals/test",
		CreatedAt:     now.Add(-2 * time.Minute),
		ExpiresAt:     now.Add(-time.Minute),
	}
	repo.approvals[approval.ID] = approval

	_, err := repo.Approve(context.Background(), approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce, ToolPattern: "read_file", ParamsPattern: map[string]string{}}, now)
	require.ErrorIs(t, err, ports.ErrNotFound)

	_, err = repo.Deny(context.Background(), approval.ID, nil, now)
	require.ErrorIs(t, err, ports.ErrNotFound)
}

func TestToolApprovalRepository_ListAllActiveOmitsInactiveSessionApproval(t *testing.T) {
	repo := NewToolApprovalRepository()
	now := time.Now()
	session := storage.ApprovalPersistenceSession
	sessionID := "session-1"
	approval := &storage.ToolApproval{
		ID:             id.NewApprovalID(),
		Principal:      id.Principal("user@example.com"),
		AgentID:        id.NewAgentID(),
		ToolName:       "read_file",
		ArgumentsHash:  "same",
		Status:         storage.ApprovalStatusApproved,
		Persistence:    &session,
		AgentSessionID: &sessionID,
		ApprovalURL:    "https://broker.example.com/consent/approvals/test",
		CreatedAt:      now,
		ExpiresAt:      now.Add(-time.Minute),
	}
	repo.approvals[approval.ID] = approval

	inactive, err := repo.ListAllActive(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Empty(t, inactive)

	active, err := repo.ListAllActive(context.Background(), nil, []string{sessionID})
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, approval.ID, active[0].ID)
}

func TestToolApprovalRepositoryPatterns(t *testing.T) {
	repo := NewToolApprovalRepository()
	approval := &storage.ToolApproval{ID: id.NewApprovalID(), ToolName: "read_file", Status: storage.ApprovalStatusPending}
	_, err := repo.Create(context.Background(), approval)
	require.ErrorIs(t, err, storage.ErrApprovalPatternMissing)

	require.NoError(t, domainapproval.ApplyExactPatterns(approval))
	approval.ExpiresAt = time.Now().Add(time.Minute)
	created, err := repo.Create(context.Background(), approval)
	require.NoError(t, err)
	_, err = repo.Approve(context.Background(), created.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce}, time.Now())
	require.ErrorIs(t, err, storage.ErrApprovalPatternMissing)
	decision := storage.ApprovalDecision{Persistence: storage.ApprovalPersistencePermanent, ToolPattern: "read_*", ParamsPattern: map[string]string{"path": "acme/*"}}
	updated, err := repo.Approve(context.Background(), created.ID, decision, time.Now())
	require.NoError(t, err)
	require.Equal(t, decision.ToolPattern, updated.ToolPattern)
	require.Equal(t, decision.ParamsPattern, updated.ParamsPattern)
	updated.ParamsPattern["path"] = "changed"
	stored, err := repo.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "acme/*", stored.ParamsPattern["path"])
}
