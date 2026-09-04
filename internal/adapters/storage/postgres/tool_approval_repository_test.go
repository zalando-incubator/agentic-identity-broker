//go:build integration
// +build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAgent inserts a minimal agent row into the test database to satisfy the
// tool_approvals.agent_id foreign-key constraint.
func seedAgent(t *testing.T, adapter *Adapter, agentID id.AgentID) {
	t.Helper()
	ctx := context.Background()
	repo := NewAgentRepository(adapter)
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("test-client-" + agentID.String()[:8])),
		DisplayName: "Test Agent",
		Description: "seeded for FK constraint",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	attachTestPermissionSet(t, adapter, agent)
	err := repo.Create(ctx, agent)
	require.NoError(t, err, "seedAgent: failed to create agent %s", agentID)
}

func setupApprovalTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

func TestToolApprovalRepository_CRUD(t *testing.T) {
	adapter, cleanup := setupApprovalTestDB(t)
	defer cleanup()

	repo := NewToolApprovalRepository(adapter)
	ctx := context.Background()

	t.Run("Create and Get", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		agentID := id.NewAgentID()
		seedAgent(t, adapter, agentID)
		approval := &storage.ToolApproval{
			ID:                       id.NewApprovalID(),
			Principal:                id.Principal("user@example.com"),
			AgentID:                  agentID,
			GatewayClientID:          "test-gateway",
			ToolName:                 "read_file",
			Arguments:                map[string]any{"path": "/etc/passwd"},
			ArgumentsHash:            "abc123",
			Description:              "Read a system file",
			RiskLevel:                "critical",
			Status:                   storage.ApprovalStatusPending,
			ApprovalURL:              "https://broker.example.com/approvals/" + id.NewApprovalID().String(),
			OpenTelemetryTraceparent: &traceparent,
			CreatedAt:                now,
			ExpiresAt:                now.Add(10 * time.Minute),
		}

		created, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)
		assert.Equal(t, approval.ID, created.ID)
		assert.Equal(t, storage.ApprovalStatusPending, created.Status)

		got, err := repo.Get(ctx, approval.ID)
		require.NoError(t, err)
		assert.Equal(t, approval.ToolName, got.ToolName)
		assert.Equal(t, approval.Principal, got.Principal)
		assert.Equal(t, "critical", got.RiskLevel)
		assert.Equal(t, &traceparent, got.OpenTelemetryTraceparent)
	})

	t.Run("Create idempotent", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		approvalID := id.NewApprovalID()
		agentID2 := id.NewAgentID()
		seedAgent(t, adapter, agentID2)
		approval := &storage.ToolApproval{
			ID:              approvalID,
			Principal:       id.Principal("user@example.com"),
			AgentID:         agentID2,
			GatewayClientID: "test-gateway",
			ToolName:        "write_file",
			Arguments:       map[string]any{"path": "/tmp/test"},
			ArgumentsHash:   "def456",
			Description:     "Write a temp file",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/" + approvalID.String(),
			CreatedAt:       now,
			ExpiresAt:       now.Add(10 * time.Minute),
		}

		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)

		// Second create should be idempotent
		_, err = createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)
	})

	t.Run("Create replaces expired duplicate", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		agentID := id.NewAgentID()
		seedAgent(t, adapter, agentID)
		first := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       id.Principal("expired@example.com"),
			AgentID:         agentID,
			GatewayClientID: "test-gateway",
			ToolName:        "read_file",
			ArgumentsHash:   "expired-hash",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/old",
			CreatedAt:       now.Add(-2 * time.Minute),
			ExpiresAt:       now.Add(-time.Minute),
		}
		second := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       first.Principal,
			AgentID:         agentID,
			GatewayClientID: "test-gateway",
			ToolName:        first.ToolName,
			ArgumentsHash:   first.ArgumentsHash,
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/new",
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Minute),
		}

		_, err := createPatternedApproval(ctx, repo, first)
		require.NoError(t, err)
		created, err := createPatternedApproval(ctx, repo, second)
		require.NoError(t, err)
		assert.Equal(t, second.ID, created.ID)
		assert.Equal(t, second.ApprovalURL, created.ApprovalURL)

		old, err := repo.Get(ctx, first.ID)
		require.NoError(t, err)
		assert.True(t, old.Consumed)
	})

	t.Run("Approve", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		agentID3 := id.NewAgentID()
		seedAgent(t, adapter, agentID3)
		approval := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       id.Principal("approver@example.com"),
			AgentID:         agentID3,
			GatewayClientID: "test-gateway",
			ToolName:        "run_command",
			Arguments:       map[string]any{"cmd": "ls"},
			ArgumentsHash:   "ghi789",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/test",
			CreatedAt:       now,
			ExpiresAt:       now.Add(10 * time.Minute),
		}
		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)

		approvedAt := now.Add(30 * time.Second)
		result, err := repo.Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce, ToolPattern: approval.ToolName, ParamsPattern: storageExactParams(approval)}, approvedAt)
		require.NoError(t, err)
		assert.Equal(t, storage.ApprovalStatusApproved, result.Status)
		assert.NotNil(t, result.Persistence)
		assert.Equal(t, storage.ApprovalPersistenceOnce, *result.Persistence)
	})

	t.Run("Deny and revoke permanent denial", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		agentID4 := id.NewAgentID()
		seedAgent(t, adapter, agentID4)
		approval := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       id.Principal("denier@example.com"),
			AgentID:         agentID4,
			GatewayClientID: "test-gateway",
			ToolName:        "delete_file",
			Arguments:       map[string]any{"path": "/important"},
			ArgumentsHash:   "jkl012",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/test",
			CreatedAt:       now,
			ExpiresAt:       now.Add(10 * time.Minute),
		}
		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)

		deniedAt := now.Add(30 * time.Second)
		permanent := storage.ApprovalPersistencePermanent
		result, err := repo.Deny(ctx, approval.ID, &permanent, deniedAt)
		require.NoError(t, err)
		assert.Equal(t, storage.ApprovalStatusDenied, result.Status)
		assert.NotNil(t, result.Persistence)
		assert.Equal(t, storage.ApprovalPersistencePermanent, *result.Persistence)

		result, err = repo.RevokePermanent(ctx, approval.ID, deniedAt.Add(time.Second))
		require.NoError(t, err)
		assert.Equal(t, storage.ApprovalStatusDenied, result.Status)
		assert.Nil(t, result.Persistence)
		_, err = repo.RevokePermanent(ctx, approval.ID, deniedAt.Add(2*time.Second))
		require.ErrorIs(t, err, ports.ErrNotFound)
	})

	t.Run("rejects expired pending approvals", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		agentID := id.NewAgentID()
		seedAgent(t, adapter, agentID)
		approval := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       id.Principal("expired-action@example.com"),
			AgentID:         agentID,
			GatewayClientID: "test-gateway",
			ToolName:        "write_file",
			ArgumentsHash:   "expired-action",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/test",
			CreatedAt:       now.Add(-2 * time.Minute),
			ExpiresAt:       now.Add(-time.Minute),
		}
		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)

		_, err = repo.Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce, ToolPattern: approval.ToolName, ParamsPattern: storageExactParams(approval)}, now)
		require.ErrorIs(t, err, ports.ErrNotFound)

		_, err = repo.Deny(ctx, approval.ID, nil, now)
		require.ErrorIs(t, err, ports.ErrNotFound)
	})

	t.Run("Consume", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		agentID5 := id.NewAgentID()
		seedAgent(t, adapter, agentID5)
		approval := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       id.Principal("consumer@example.com"),
			AgentID:         agentID5,
			GatewayClientID: "test-gateway",
			ToolName:        "send_email",
			Arguments:       map[string]any{"to": "admin@example.com"},
			ArgumentsHash:   "mno345",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/test",
			CreatedAt:       now,
			ExpiresAt:       now.Add(10 * time.Minute),
		}
		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)

		// First approve it
		_, err = repo.Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce, ToolPattern: approval.ToolName, ParamsPattern: storageExactParams(approval)}, now.Add(10*time.Second))
		require.NoError(t, err)

		// Then consume it
		consumedAt := now.Add(20 * time.Second)
		result, err := repo.Consume(ctx, approval.ID, consumedAt)
		require.NoError(t, err)
		assert.True(t, result.Consumed)
		assert.NotNil(t, result.ConsumedAt)
	})

	t.Run("ListAllActive", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		principal := id.Principal("lister@example.com")
		agentID := id.NewAgentID()
		seedAgent(t, adapter, agentID)

		for i := 0; i < 3; i++ {
			a := &storage.ToolApproval{
				ID:              id.NewApprovalID(),
				Principal:       principal,
				AgentID:         agentID,
				GatewayClientID: "test-gateway",
				ToolName:        "tool_" + string(rune('a'+i)),
				Arguments:       map[string]any{},
				ArgumentsHash:   "hash_" + string(rune('a'+i)),
				Status:          storage.ApprovalStatusPending,
				ApprovalURL:     "https://broker.example.com/approvals/test",
				CreatedAt:       now,
				ExpiresAt:       now.Add(10 * time.Minute),
			}
			_, err := createPatternedApproval(ctx, repo, a)
			require.NoError(t, err)
		}

		results, err := repo.ListAllActive(ctx, &principal, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	t.Run("omits inactive session approvals", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		principal := id.Principal("session@example.com")
		agentID := id.NewAgentID()
		seedAgent(t, adapter, agentID)
		sessionID := "session-1"
		approval := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       principal,
			AgentID:         agentID,
			GatewayClientID: "test-gateway",
			ToolName:        "session_tool",
			ArgumentsHash:   "session-hash",
			AgentSessionID:  &sessionID,
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/session",
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Minute),
		}
		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)
		_, err = repo.Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceSession, ToolPattern: approval.ToolName, ParamsPattern: storageExactParams(approval)}, now)
		require.NoError(t, err)

		inactive, err := repo.ListAllActive(ctx, &principal, nil)
		require.NoError(t, err)
		assert.Empty(t, inactive)

		active, err := repo.ListAllActive(ctx, &principal, []string{sessionID})
		require.NoError(t, err)
		require.Len(t, active, 1)
		assert.Equal(t, approval.ID, active[0].ID)
	})

	t.Run("ListPermanentByPrincipal", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		principal := id.Principal("permanent@example.com")
		agentID7 := id.NewAgentID()
		seedAgent(t, adapter, agentID7)
		approval := &storage.ToolApproval{
			ID:              id.NewApprovalID(),
			Principal:       principal,
			AgentID:         agentID7,
			GatewayClientID: "test-gateway",
			ToolName:        "permanent_tool",
			Arguments:       map[string]any{},
			ArgumentsHash:   "perm_hash",
			Status:          storage.ApprovalStatusPending,
			ApprovalURL:     "https://broker.example.com/approvals/test",
			CreatedAt:       now,
			ExpiresAt:       now.Add(10 * time.Minute),
		}
		_, err := createPatternedApproval(ctx, repo, approval)
		require.NoError(t, err)

		_, err = repo.Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistencePermanent, ToolPattern: approval.ToolName, ParamsPattern: storageExactParams(approval)}, now.Add(10*time.Second))
		require.NoError(t, err)

		results, err := repo.ListPermanentByPrincipal(ctx, principal)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)
	})
}

func TestToolApprovalRepository_MutationsAdvanceSyncVersion(t *testing.T) {
	adapter, cleanup := setupApprovalTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewToolApprovalRepository(adapter)
	syncRepo := NewApprovalSyncStateRepository(adapter)
	now := time.Now().UTC().Truncate(time.Microsecond)
	agentID := id.NewAgentID()
	seedAgent(t, adapter, agentID)
	approval := &storage.ToolApproval{
		ID:              id.NewApprovalID(),
		Principal:       id.Principal("sync@example.com"),
		AgentID:         agentID,
		GatewayClientID: "test-gateway",
		ToolName:        "read_file",
		ArgumentsHash:   "sync-version",
		Status:          storage.ApprovalStatusPending,
		ApprovalURL:     "https://broker.example.com/approvals/test",
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Minute),
	}

	_, err := createPatternedApproval(ctx, repo, approval)
	require.NoError(t, err)
	version, err := syncRepo.GetVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), version)

	_, err = repo.Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce, ToolPattern: approval.ToolName, ParamsPattern: storageExactParams(approval)}, now)
	require.NoError(t, err)
	version, err = syncRepo.GetVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), version)
}

func TestApprovalSyncStateRepository(t *testing.T) {
	adapter, cleanup := setupApprovalTestDB(t)
	defer cleanup()

	repo := NewApprovalSyncStateRepository(adapter)
	ctx := context.Background()

	t.Run("GetVersion returns initial version", func(t *testing.T) {
		version, err := repo.GetVersion(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), version)
	})

	t.Run("IncrementVersion increments and returns new version", func(t *testing.T) {
		v1, err := repo.IncrementVersion(ctx)
		require.NoError(t, err)

		v2, err := repo.IncrementVersion(ctx)
		require.NoError(t, err)

		assert.Equal(t, v1+1, v2)
	})
}

func storageExactParams(approval *storage.ToolApproval) map[string]string {
	approval.ApplyExactPatterns()
	return approval.ParamsPattern
}

func createPatternedApproval(ctx context.Context, repo *ToolApprovalRepository, approval *storage.ToolApproval) (*storage.ToolApproval, error) {
	approval.ApplyExactPatterns()
	return repo.Create(ctx, approval)
}
