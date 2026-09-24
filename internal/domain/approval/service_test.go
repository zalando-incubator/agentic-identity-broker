package approval

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval/toolpattern"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// mockApprovalRepo implements ports.ToolApprovalRepository for testing.
type mockApprovalRepo struct {
	approvals   map[id.ApprovalID]*storage.ToolApproval
	approveFunc func(context.Context, id.ApprovalID, storage.ApprovalDecision, time.Time) (*storage.ToolApproval, error)
	denyFunc    func(context.Context, id.ApprovalID, *storage.ApprovalPersistence, time.Time) (*storage.ToolApproval, error)
	consumeFunc func(context.Context, id.ApprovalID, time.Time) (*storage.ToolApproval, error)
}

func newMockApprovalRepo() *mockApprovalRepo {
	return &mockApprovalRepo{approvals: make(map[id.ApprovalID]*storage.ToolApproval)}
}

func (m *mockApprovalRepo) Create(_ context.Context, a *storage.ToolApproval) (*storage.ToolApproval, error) {
	// Check for existing pending duplicate (idempotency, like in-memory repo)
	for _, existing := range m.approvals {
		if existing.Principal == a.Principal &&
			existing.AgentID == a.AgentID &&
			existing.ToolName == a.ToolName &&
			existing.ArgumentsHash == a.ArgumentsHash &&
			existing.Status == storage.ApprovalStatusPending &&
			!existing.Consumed {
			copy := *existing
			return &copy, nil
		}
	}
	copy := *a
	m.approvals[a.ID] = &copy
	return a, nil
}

func (m *mockApprovalRepo) Get(_ context.Context, approvalID id.ApprovalID) (*storage.ToolApproval, error) {
	a, ok := m.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "approval not found")
	}
	// Return a copy to simulate real repo behavior
	copy := *a
	return &copy, nil
}

func (m *mockApprovalRepo) Approve(_ context.Context, approvalID id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error) {
	if m.approveFunc != nil {
		return m.approveFunc(context.Background(), approvalID, decision, approvedAt)
	}
	a, ok := m.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Approve", storage.ErrorKindNotFound, nil, "approval not found")
	}
	a.Status = storage.ApprovalStatusApproved
	a.Persistence = &decision.Persistence
	a.ToolPattern = decision.ToolPattern
	a.ParamsPattern = decision.ParamsPattern
	a.ApprovedAt = &approvedAt
	copy := *a
	return &copy, nil
}

func (m *mockApprovalRepo) Deny(_ context.Context, approvalID id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error) {
	if m.denyFunc != nil {
		return m.denyFunc(context.Background(), approvalID, persistence, deniedAt)
	}
	a, ok := m.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Deny", storage.ErrorKindNotFound, nil, "approval not found")
	}
	a.Status = storage.ApprovalStatusDenied
	a.Persistence = persistence
	a.DeniedAt = &deniedAt
	copy := *a
	return &copy, nil
}

func (m *mockApprovalRepo) RevokePermanent(_ context.Context, approvalID id.ApprovalID, revokedAt time.Time) (*storage.ToolApproval, error) {
	a, ok := m.approvals[approvalID]
	if !ok || a.Persistence == nil || *a.Persistence != storage.ApprovalPersistencePermanent ||
		(a.Status != storage.ApprovalStatusApproved && a.Status != storage.ApprovalStatusDenied) {
		return nil, storage.NewStorageError("RevokePermanent", storage.ErrorKindNotFound, nil, "approval not found or not permanent")
	}
	a.Status = storage.ApprovalStatusDenied
	a.Persistence = nil
	a.DeniedAt = &revokedAt
	copy := *a
	return &copy, nil
}

func (m *mockApprovalRepo) Consume(ctx context.Context, approvalID id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error) {
	if m.consumeFunc != nil {
		return m.consumeFunc(ctx, approvalID, consumedAt)
	}
	a, ok := m.approvals[approvalID]
	if !ok {
		return nil, storage.NewStorageError("Consume", storage.ErrorKindNotFound, nil, "approval not found")
	}
	a.Consumed = true
	a.ConsumedAt = &consumedAt
	copy := *a
	return &copy, nil
}

func (m *mockApprovalRepo) ListAllActive(_ context.Context, principalFilter *id.Principal, _ []string) ([]*storage.ToolApproval, error) {
	var approvals []*storage.ToolApproval
	for _, approval := range m.approvals {
		if principalFilter == nil || approval.Principal == *principalFilter {
			approvals = append(approvals, approval)
		}
	}
	return approvals, nil
}

func (m *mockApprovalRepo) ListActiveByPrincipalAndAgent(_ context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	var approvals []*storage.ToolApproval
	for _, approval := range m.approvals {
		if approval.Principal == principal && approval.AgentID == agentID {
			approvals = append(approvals, approval)
		}
	}
	return approvals, nil
}

func (m *mockApprovalRepo) ListPermanentByPrincipal(_ context.Context, principal id.Principal) ([]*storage.ToolApproval, error) {
	var approvals []*storage.ToolApproval
	for _, approval := range m.approvals {
		if approval.Principal == principal && approval.Persistence != nil && *approval.Persistence == storage.ApprovalPersistencePermanent {
			approvals = append(approvals, approval)
		}
	}
	return approvals, nil
}

// mockSyncStateRepo implements ports.ApprovalSyncStateRepository for testing.
type mockSyncStateRepo struct {
	version int64
}

func (m *mockSyncStateRepo) GetVersion(_ context.Context) (int64, error) {
	return m.version, nil
}

func (m *mockSyncStateRepo) IncrementVersion(_ context.Context) (int64, error) {
	m.version++
	return m.version, nil
}

func newTestService(repo *mockApprovalRepo) *Service {
	return NewService(
		repo,
		nil, // queries not needed for these tests
		nil, // metrics not needed for these tests
		&mockSyncStateRepo{},
		nil, // agents not needed for these tests
		NewApprovalRateLimiter(50, 10),
		NewApprovalSyncBroadcaster(0),
		10*time.Minute,
		"https://broker.example.com",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

func makePendingApproval(principal id.Principal, agentID id.AgentID) *storage.ToolApproval {
	approval := &storage.ToolApproval{
		ID:            id.NewApprovalID(),
		Principal:     principal,
		AgentID:       agentID,
		ToolName:      "test-tool",
		Arguments:     map[string]any{"key": "value"},
		ArgumentsHash: storage.ComputeArgumentsHash(map[string]any{"key": "value"}),
		Status:        storage.ApprovalStatusPending,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	if err := ApplyExactPatterns(approval); err != nil {
		panic(err)
	}
	return approval
}

func TestService_GetApproval(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	t.Run("returns approval for matching principal", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		a.RiskLevel = "medium"
		mcpSessionID := "mcp-123"
		agentSessionID := "sess-xyz"
		toolInvocationID := "inv-abc"
		a.MCPSessionID = &mcpSessionID
		a.AgentSessionID = &agentSessionID
		a.ToolInvocationID = &toolInvocationID
		repo.approvals[a.ID] = a

		result, err := svc.GetApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ID != a.ID {
			t.Fatal("expected matching approval ID")
		}
		if result.RiskLevel != "medium" {
			t.Fatalf("expected risk level medium, got %s", result.RiskLevel)
		}
		if result.MCPSessionID == nil || *result.MCPSessionID != mcpSessionID {
			t.Fatal("expected mcp_session_id to be exposed")
		}
		if result.AgentSessionID == nil || *result.AgentSessionID != agentSessionID {
			t.Fatal("expected agent_session_id to be exposed")
		}
		if result.ToolInvocationID == nil || *result.ToolInvocationID != toolInvocationID {
			t.Fatal("expected tool_invocation_id to be exposed")
		}
	})

	t.Run("returns ErrApprovalNotFound for missing approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		_, err := svc.GetApproval(context.Background(), id.NewApprovalID(), principal)
		if err != ErrApprovalNotFound {
			t.Fatalf("expected ErrApprovalNotFound, got %v", err)
		}
	})

	t.Run("returns ErrApprovalForbidden for wrong principal", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		_, err := svc.GetApproval(context.Background(), a.ID, id.Principal("other@example.com"))
		if err != ErrApprovalForbidden {
			t.Fatalf("expected ErrApprovalForbidden, got %v", err)
		}
	})

	t.Run("returns ErrApprovalGone for expired pending approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		a.ExpiresAt = time.Now().Add(-1 * time.Minute)
		repo.approvals[a.ID] = a

		_, err := svc.GetApproval(context.Background(), a.ID, principal)
		if err != ErrApprovalGone {
			t.Fatalf("expected ErrApprovalGone, got %v", err)
		}
	})

	t.Run("returns expired approved approval (not gone)", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		a.Status = storage.ApprovalStatusApproved
		a.ExpiresAt = time.Now().Add(-1 * time.Minute)
		repo.approvals[a.ID] = a

		result, err := svc.GetApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != storage.ApprovalStatusApproved {
			t.Fatal("expected approved status for expired but already-resolved approval")
		}
	})
}

func TestService_ApproveApproval(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	t.Run("approves pending approval with once persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		result, err := svc.ApproveApproval(context.Background(), a.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != storage.ApprovalStatusApproved {
			t.Fatalf("expected approved, got %s", result.Status)
		}
		if result.Persistence == nil || *result.Persistence != storage.ApprovalPersistenceOnce {
			t.Fatal("expected once persistence")
		}
	})

	t.Run("approves with session persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		result, err := svc.ApproveApproval(context.Background(), a.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceSession})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Persistence == nil || *result.Persistence != storage.ApprovalPersistenceSession {
			t.Fatal("expected session persistence")
		}
	})

	t.Run("approves with permanent persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		result, err := svc.ApproveApproval(context.Background(), a.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistencePermanent})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Persistence == nil || *result.Persistence != storage.ApprovalPersistencePermanent {
			t.Fatal("expected permanent persistence")
		}
	})

	t.Run("returns ErrApprovalNotFound for missing approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		_, err := svc.ApproveApproval(context.Background(), id.NewApprovalID(), principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
		if err != ErrApprovalNotFound {
			t.Fatalf("expected ErrApprovalNotFound, got %v", err)
		}
	})

	t.Run("returns ErrApprovalForbidden for wrong principal", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		_, err := svc.ApproveApproval(context.Background(), a.ID, id.Principal("other@example.com"), ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
		if err != ErrApprovalForbidden {
			t.Fatalf("expected ErrApprovalForbidden, got %v", err)
		}
	})

	t.Run("returns ErrApprovalGone for expired approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		a.ExpiresAt = time.Now().Add(-1 * time.Minute)
		repo.approvals[a.ID] = a

		_, err := svc.ApproveApproval(context.Background(), a.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
		if err != ErrApprovalGone {
			t.Fatalf("expected ErrApprovalGone, got %v", err)
		}
	})

	t.Run("returns ErrApprovalGone when approval expires during approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)
		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a
		repo.approveFunc = func(_ context.Context, _ id.ApprovalID, _ storage.ApprovalDecision, _ time.Time) (*storage.ToolApproval, error) {
			a.ExpiresAt = time.Now().Add(-time.Minute)
			return nil, storage.NewStorageError("Approve", storage.ErrorKindNotFound, ports.ErrNotFound, "approval no longer actionable")
		}

		_, err := svc.ApproveApproval(context.Background(), a.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
		if err != ErrApprovalGone {
			t.Fatalf("expected ErrApprovalGone, got %v", err)
		}
	})

	t.Run("increments sync version after approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		syncRepo := &mockSyncStateRepo{}
		svc := NewService(repo, nil, nil, syncRepo, nil, NewApprovalRateLimiter(50, 10), nil, 10*time.Minute, "", slog.New(slog.NewTextHandler(io.Discard, nil)))

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		_, err := svc.ApproveApproval(context.Background(), a.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if syncRepo.version != 1 {
			t.Fatalf("expected sync version 1, got %d", syncRepo.version)
		}
	})
}

func TestService_ResolveApprovalDecision(t *testing.T) {
	approval := makePendingApproval(id.Principal("user@example.com"), id.NewAgentID())
	for _, test := range []struct {
		name   string
		req    ApproveRequest
		params map[string]string
	}{
		{"exact defaults", ApproveRequest{Persistence: storage.ApprovalPersistenceOnce}, map[string]string{"key": "value"}},
		{"unconstrained params", ApproveRequest{Persistence: storage.ApprovalPersistenceSession, ParamsPattern: map[string]string{}}, map[string]string{}},
		{"edited params", ApproveRequest{Persistence: storage.ApprovalPersistencePermanent, ParamsPattern: map[string]string{"key": "val*"}}, map[string]string{"key": "val*"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, err := resolveApprovalDecision(approval, test.req)
			if err != nil || decision.ToolPattern != toolpattern.EscapeLiteral(approval.ToolName) || !reflect.DeepEqual(decision.ParamsPattern, test.params) {
				t.Fatalf("decision=%#v err=%v", decision, err)
			}
		})
	}
	t.Run("rejects an explicit parameter pattern for once", func(t *testing.T) {
		_, err := resolveApprovalDecision(approval, ApproveRequest{
			Persistence:   storage.ApprovalPersistenceOnce,
			ParamsPattern: map[string]string{},
		})
		if !errors.Is(err, ErrApprovalInvalidPattern) {
			t.Fatalf("expected invalid pattern, got %v", err)
		}
	})
	for _, req := range []ApproveRequest{{Persistence: storage.ApprovalPersistencePermanent, ParamsPattern: map[string]string{"branch": "main"}}} {
		if _, err := resolveApprovalDecision(approval, req); !errors.Is(err, ErrApprovalInvalidPattern) {
			t.Fatalf("expected invalid pattern, got %v", err)
		}
	}
}

func TestService_DenyApproval(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	t.Run("denies pending approval without persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		result, err := svc.DenyApproval(context.Background(), a.ID, principal, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != storage.ApprovalStatusDenied {
			t.Fatalf("expected denied, got %s", result.Status)
		}
	})

	t.Run("denies with permanent persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		p := storage.ApprovalPersistencePermanent
		result, err := svc.DenyApproval(context.Background(), a.ID, principal, &p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Persistence == nil || *result.Persistence != storage.ApprovalPersistencePermanent {
			t.Fatal("expected permanent persistence")
		}
	})

	t.Run("returns ErrApprovalNotFound for missing approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		_, err := svc.DenyApproval(context.Background(), id.NewApprovalID(), principal, nil)
		if err != ErrApprovalNotFound {
			t.Fatalf("expected ErrApprovalNotFound, got %v", err)
		}
	})

	t.Run("returns ErrApprovalForbidden for wrong principal", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		_, err := svc.DenyApproval(context.Background(), a.ID, id.Principal("other@example.com"), nil)
		if err != ErrApprovalForbidden {
			t.Fatalf("expected ErrApprovalForbidden, got %v", err)
		}
	})

	t.Run("returns ErrApprovalGone for expired approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		a.ExpiresAt = time.Now().Add(-1 * time.Minute)
		repo.approvals[a.ID] = a

		_, err := svc.DenyApproval(context.Background(), a.ID, principal, nil)
		if err != ErrApprovalGone {
			t.Fatalf("expected ErrApprovalGone, got %v", err)
		}
	})

	t.Run("returns ErrApprovalGone when approval expires during denial", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)
		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a
		repo.denyFunc = func(_ context.Context, _ id.ApprovalID, _ *storage.ApprovalPersistence, _ time.Time) (*storage.ToolApproval, error) {
			a.ExpiresAt = time.Now().Add(-time.Minute)
			return nil, storage.NewStorageError("Deny", storage.ErrorKindNotFound, ports.ErrNotFound, "approval no longer actionable")
		}

		_, err := svc.DenyApproval(context.Background(), a.ID, principal, nil)
		if err != ErrApprovalGone {
			t.Fatalf("expected ErrApprovalGone, got %v", err)
		}
	})

	t.Run("increments sync version after deny", func(t *testing.T) {
		repo := newMockApprovalRepo()
		syncRepo := &mockSyncStateRepo{}
		svc := NewService(repo, nil, nil, syncRepo, nil, NewApprovalRateLimiter(50, 10), nil, 10*time.Minute, "", slog.New(slog.NewTextHandler(io.Discard, nil)))

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		_, err := svc.DenyApproval(context.Background(), a.ID, principal, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if syncRepo.version != 1 {
			t.Fatalf("expected sync version 1, got %d", syncRepo.version)
		}
	})
}

func TestService_CreatePendingApproval(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	makeRequest := func() CreateApprovalRequest {
		return CreateApprovalRequest{
			Principal:       principal,
			AgentID:         agentID,
			GatewayClientID: "gw-client-123",
			ToolName:        "create_pull_request",
			Arguments:       map[string]any{"repo": "acme/app", "title": "Fix bug"},
			Description:     "Create a PR in acme/app",
			RiskLevel:       "medium",
		}
	}

	t.Run("creates new pending approval with correct fields", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		result, err := svc.CreatePendingApproval(context.Background(), makeRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsNew {
			t.Fatal("expected IsNew to be true")
		}
		a := result.Approval
		if a.Principal != principal {
			t.Fatalf("expected principal %s, got %s", principal, a.Principal)
		}
		if a.AgentID != agentID {
			t.Fatalf("expected agent ID %s, got %s", agentID, a.AgentID)
		}
		if a.ToolName != "create_pull_request" {
			t.Fatalf("expected tool_name create_pull_request, got %s", a.ToolName)
		}
		if a.Status != storage.ApprovalStatusPending {
			t.Fatalf("expected status pending, got %s", a.Status)
		}
		if a.ArgumentsHash == "" {
			t.Fatal("expected non-empty arguments hash")
		}
		if a.ApprovalURL == "" {
			t.Fatal("expected non-empty approval_url")
		}
		if a.ExpiresAt.Before(a.CreatedAt) {
			t.Fatal("expected expires_at to be after created_at")
		}
	})

	t.Run("constructs approval_url from publicURL + approval ID", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		result, err := svc.CreatePendingApproval(context.Background(), makeRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "https://broker.example.com/approvals/" + result.Approval.ID.String()
		if result.Approval.ApprovalURL != expected {
			t.Fatalf("expected approval_url %s, got %s", expected, result.Approval.ApprovalURL)
		}
	})

	t.Run("sets TTL from pendingTTL config", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		before := time.Now()
		result, err := svc.CreatePendingApproval(context.Background(), makeRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// pendingTTL is 10 minutes in test service
		expectedMin := before.Add(10 * time.Minute)
		if result.Approval.ExpiresAt.Before(expectedMin.Add(-1 * time.Second)) {
			t.Fatalf("expected expires_at >= %v, got %v", expectedMin, result.Approval.ExpiresAt)
		}
	})

	t.Run("returns existing pending approval (idempotent dedup)", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		req := makeRequest()
		result1, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result2, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result2.Approval.ID != result1.Approval.ID {
			t.Fatal("expected same approval ID for idempotent request")
		}
		if result2.IsNew {
			t.Fatal("expected IsNew to be false for idempotent hit")
		}
	})

	t.Run("returns existing approval without charging rate limit", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := NewService(
			repo, repo, nil,
			&mockSyncStateRepo{},
			nil,
			NewApprovalRateLimiter(10, 1),
			NewApprovalSyncBroadcaster(0),
			10*time.Minute,
			"https://broker.example.com",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := makeRequest()
		first, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error on first request: %v", err)
		}
		second, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("duplicate request should not be rate limited: %v", err)
		}
		if second.IsNew || second.Approval.ID != first.Approval.ID {
			t.Fatal("expected existing approval for duplicate request")
		}
	})

	t.Run("returns a duplicate found after rate limit exhaustion", func(t *testing.T) {
		repo := newMockApprovalRepo()
		queries := &mockQueryRepo{}
		svc := NewService(
			repo, queries, nil,
			&mockSyncStateRepo{},
			nil,
			NewApprovalRateLimiter(10, 1),
			NewApprovalSyncBroadcaster(0),
			10*time.Minute,
			"https://broker.example.com",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := makeRequest()
		first, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error on first request: %v", err)
		}
		calls := 0
		queries.listActiveByPairFunc = func(_ context.Context, _ id.Principal, _ id.AgentID) ([]*storage.ToolApproval, error) {
			calls++
			if calls == 1 {
				return nil, nil
			}
			return []*storage.ToolApproval{first.Approval}, nil
		}
		second, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("duplicate request should not be rate limited: %v", err)
		}
		if second.IsNew || second.Approval.ID != first.Approval.ID {
			t.Fatal("expected existing approval for duplicate request")
		}
	})

	t.Run("computes deterministic arguments hash", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		result, err := svc.CreatePendingApproval(context.Background(), makeRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedHash := storage.ComputeArgumentsHash(map[string]any{"repo": "acme/app", "title": "Fix bug"})
		if result.Approval.ArgumentsHash != expectedHash {
			t.Fatalf("expected hash %s, got %s", expectedHash, result.Approval.ArgumentsHash)
		}
	})

	t.Run("rate limit blocks when exceeded", func(t *testing.T) {
		repo := newMockApprovalRepo()
		// Create service with very restrictive rate limit
		svc := NewService(
			repo, nil, nil,
			&mockSyncStateRepo{},
			nil,
			NewApprovalRateLimiter(1, 1), // max 1 pending, 1 req/min
			NewApprovalSyncBroadcaster(0),
			10*time.Minute,
			"https://broker.example.com",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		// First request should succeed
		req := makeRequest()
		_, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error on first request: %v", err)
		}

		// Second request with different tool (different dedup key) should be rate limited
		req2 := makeRequest()
		req2.ToolName = "different_tool"
		_, err = svc.CreatePendingApproval(context.Background(), req2)
		if err != ErrApprovalRateLimit {
			t.Fatalf("expected ErrApprovalRateLimit, got %v", err)
		}
	})

	t.Run("increments sync version for new approval only", func(t *testing.T) {
		repo := newMockApprovalRepo()
		syncRepo := &mockSyncStateRepo{}
		svc := NewService(
			repo, nil, nil,
			syncRepo,
			nil,
			NewApprovalRateLimiter(50, 10),
			NewApprovalSyncBroadcaster(0),
			10*time.Minute,
			"https://broker.example.com",
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := makeRequest()
		_, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if syncRepo.version != 1 {
			t.Fatalf("expected sync version 1 after new approval, got %d", syncRepo.version)
		}

		// Idempotent hit should NOT increment
		_, err = svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if syncRepo.version != 1 {
			t.Fatalf("expected sync version still 1 after idempotent hit, got %d", syncRepo.version)
		}
	})

	t.Run("stores optional metadata fields", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		mcpSessID := "mcp-123"
		agentSessID := "sess-xyz"
		toolInvID := "inv-abc"
		traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

		req := makeRequest()
		req.MCPSessionID = &mcpSessID
		req.AgentSessionID = &agentSessID
		req.ToolInvocationID = &toolInvID
		req.OpenTelemetryTraceparent = &traceparent

		result, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Approval.MCPSessionID == nil || *result.Approval.MCPSessionID != mcpSessID {
			t.Fatal("expected mcp_session_id")
		}
		if result.Approval.AgentSessionID == nil || *result.Approval.AgentSessionID != agentSessID {
			t.Fatal("expected agent_session_id")
		}
		if result.Approval.ToolInvocationID == nil || *result.Approval.ToolInvocationID != toolInvID {
			t.Fatal("expected tool_invocation_id")
		}
		if result.Approval.OpenTelemetryTraceparent == nil || *result.Approval.OpenTelemetryTraceparent != traceparent {
			t.Fatal("expected opentelemetry_traceparent")
		}
	})

	t.Run("defaults unknown risk levels to critical", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		req := makeRequest()
		req.RiskLevel = "severe"

		result, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Approval.RiskLevel != "critical" {
			t.Fatalf("expected unknown risk level to default to critical, got %s", result.Approval.RiskLevel)
		}
	})

	t.Run("normalizes known risk levels to canonical lowercase", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		req := makeRequest()
		req.RiskLevel = " Medium "

		result, err := svc.CreatePendingApproval(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Approval.RiskLevel != "medium" {
			t.Fatalf("expected normalized risk level medium, got %s", result.Approval.RiskLevel)
		}
	})
}

// mockQueryRepo implements ports.ToolApprovalQueryRepository for testing.
type mockQueryRepo struct {
	approvals            []*storage.ToolApproval
	listActiveByPairFunc func(context.Context, id.Principal, id.AgentID) ([]*storage.ToolApproval, error)
}

func (m *mockQueryRepo) ListAllActive(_ context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) ([]*storage.ToolApproval, error) {
	activeSessions := make(map[string]struct{}, len(activeAgentSessionIDs))
	for _, sessionID := range activeAgentSessionIDs {
		activeSessions[sessionID] = struct{}{}
	}

	var result []*storage.ToolApproval
	for _, a := range m.approvals {
		if principalFilter != nil && a.Principal != *principalFilter {
			continue
		}
		if a.Persistence != nil && *a.Persistence == storage.ApprovalPersistenceSession {
			if a.AgentSessionID == nil {
				continue
			}
			if _, ok := activeSessions[*a.AgentSessionID]; !ok {
				continue
			}
		}
		result = append(result, a)
	}
	return result, nil
}
func (m *mockQueryRepo) ListActiveByPrincipalAndAgent(_ context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	if m.listActiveByPairFunc != nil {
		return m.listActiveByPairFunc(context.Background(), principal, agentID)
	}
	var result []*storage.ToolApproval
	for _, a := range m.approvals {
		if a.Principal == principal && a.AgentID == agentID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockQueryRepo) ListPermanentByPrincipal(_ context.Context, principal id.Principal) ([]*storage.ToolApproval, error) {
	var result []*storage.ToolApproval
	for _, a := range m.approvals {
		if a.Principal == principal && a.Persistence != nil && *a.Persistence == storage.ApprovalPersistencePermanent {
			result = append(result, a)
		}
	}
	return result, nil
}

func newTestServiceWithQueries(repo *mockApprovalRepo, queries *mockQueryRepo, syncState *mockSyncStateRepo) *Service {
	return NewService(
		repo,
		queries,
		nil, // metrics not needed
		syncState,
		nil, // agents not needed
		NewApprovalRateLimiter(50, 10),
		NewApprovalSyncBroadcaster(0),
		10*time.Minute,
		"https://broker.example.com",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

func TestService_ApprovalLifecycleSpansLinkToStoredTraceparent(t *testing.T) {
	traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	spanID := "00f067aa0ba902b7"

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	principal := id.Principal("user@example.com")
	repo := newMockApprovalRepo()
	svc := newTestService(repo)
	request := CreateApprovalRequest{
		Principal:                principal,
		AgentID:                  id.NewAgentID(),
		GatewayClientID:          "gw-client-123",
		ToolName:                 "create_pull_request",
		Arguments:                map[string]any{"repo": "acme/app"},
		Description:              "Create a PR in acme/app",
		OpenTelemetryTraceparent: &traceparent,
	}

	created, err := svc.CreatePendingApproval(context.Background(), request)
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if _, err := svc.GetApproval(context.Background(), created.Approval.ID, principal); err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if _, err := svc.ApproveApproval(context.Background(), created.Approval.ID, principal, ApproveRequest{Persistence: storage.ApprovalPersistenceOnce}); err != nil {
		t.Fatalf("approve approval: %v", err)
	}

	request.ToolName = "deny_pull_request"
	created, err = svc.CreatePendingApproval(context.Background(), request)
	if err != nil {
		t.Fatalf("create approval for denial: %v", err)
	}
	if _, err := svc.DenyApproval(context.Background(), created.Approval.ID, principal, nil); err != nil {
		t.Fatalf("deny approval: %v", err)
	}

	expectedCounts := map[string]int{
		"approval.created": 2,
		"approval.review":  1,
		"approval.approve": 1,
		"approval.deny":    1,
	}
	for _, span := range recorder.Ended() {
		if _, expected := expectedCounts[span.Name()]; !expected {
			continue
		}
		if span.Parent().IsValid() {
			t.Fatalf("%s unexpectedly has a parent span", span.Name())
		}
		links := span.Links()
		if len(links) != 1 {
			t.Fatalf("%s links = %d, want 1", span.Name(), len(links))
		}
		if links[0].SpanContext.TraceID().String() != traceID || links[0].SpanContext.SpanID().String() != spanID {
			t.Fatalf("%s link = %s/%s, want %s/%s", span.Name(), links[0].SpanContext.TraceID(), links[0].SpanContext.SpanID(), traceID, spanID)
		}
		expectedCounts[span.Name()]--
	}
	for name, count := range expectedCounts {
		if count != 0 {
			t.Fatalf("expected %d additional %s spans", count, name)
		}
	}
}

func TestService_GetSyncState(t *testing.T) {
	principal1 := id.Principal("user1@example.com")
	principal2 := id.Principal("user2@example.com")
	agent1 := id.NewAgentID()
	agent2 := id.NewAgentID()

	t.Run("returns empty pairs when no active approvals", func(t *testing.T) {
		syncState := &mockSyncStateRepo{version: 5}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), &mockQueryRepo{}, syncState)

		state, err := svc.GetSyncState(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state.Version != 5 {
			t.Fatalf("expected version 5, got %d", state.Version)
		}
		if len(state.Pairs) != 0 {
			t.Fatalf("expected 0 pairs, got %d", len(state.Pairs))
		}
	})

	t.Run("groups approvals by principal and agent pair", func(t *testing.T) {
		a1 := makePendingApproval(principal1, agent1)
		a1.ToolName = "tool-a"
		a2 := makePendingApproval(principal1, agent1)
		a2.ToolName = "tool-b"
		a3 := makePendingApproval(principal1, agent2)
		a3.ToolName = "tool-c"
		a4 := makePendingApproval(principal2, agent1)
		a4.ToolName = "tool-d"

		queries := &mockQueryRepo{approvals: []*storage.ToolApproval{a1, a2, a3, a4}}
		syncState := &mockSyncStateRepo{version: 10}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), queries, syncState)

		state, err := svc.GetSyncState(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state.Version != 10 {
			t.Fatalf("expected version 10, got %d", state.Version)
		}
		if len(state.Pairs) != 3 {
			t.Fatalf("expected 3 pairs, got %d", len(state.Pairs))
		}

		// Find the (principal1, agent1) pair and verify it has 2 approvals
		var found bool
		for _, pair := range state.Pairs {
			if pair.Principal == principal1 && pair.AgentID == agent1 {
				found = true
				if len(pair.Approvals) != 2 {
					t.Fatalf("expected 2 approvals for (principal1, agent1), got %d", len(pair.Approvals))
				}
			}
		}
		if !found {
			t.Fatal("expected to find (principal1, agent1) pair")
		}
	})

	t.Run("filters by principal when principalFilter is set", func(t *testing.T) {
		a1 := makePendingApproval(principal1, agent1)
		a2 := makePendingApproval(principal2, agent1)

		queries := &mockQueryRepo{approvals: []*storage.ToolApproval{a1, a2}}
		syncState := &mockSyncStateRepo{version: 3}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), queries, syncState)

		filter := principal1
		state, err := svc.GetSyncState(context.Background(), &filter, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(state.Pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(state.Pairs))
		}
		if state.Pairs[0].Principal != principal1 {
			t.Fatalf("expected principal1, got %s", state.Pairs[0].Principal)
		}
	})

	t.Run("excludes session approvals without an active matching session", func(t *testing.T) {
		session := storage.ApprovalPersistenceSession
		sessionID := "session-1"
		a := makePendingApproval(principal1, agent1)
		a.Status = storage.ApprovalStatusApproved
		a.Persistence = &session
		a.AgentSessionID = &sessionID
		queries := &mockQueryRepo{approvals: []*storage.ToolApproval{a}}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), queries, &mockSyncStateRepo{})

		state, err := svc.GetSyncState(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(state.Pairs) != 0 {
			t.Fatalf("expected no session approvals without an active session, got %d pairs", len(state.Pairs))
		}

		state, err = svc.GetSyncState(context.Background(), nil, []string{sessionID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(state.Pairs) != 1 || len(state.Pairs[0].Approvals) != 1 {
			t.Fatalf("expected the matching session approval, got %+v", state.Pairs)
		}
	})

	t.Run("version comes from sync state repository", func(t *testing.T) {
		syncState := &mockSyncStateRepo{version: 42}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), &mockQueryRepo{}, syncState)

		state, err := svc.GetSyncState(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state.Version != 42 {
			t.Fatalf("expected version 42, got %d", state.Version)
		}
	})
}

func TestService_ConsumeApproval(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	makeApprovedOnce := func() *storage.ToolApproval {
		a := makePendingApproval(principal, agentID)
		p := storage.ApprovalPersistenceOnce
		now := time.Now()
		a.Status = storage.ApprovalStatusApproved
		a.Persistence = &p
		a.ApprovedAt = &now
		return a
	}

	t.Run("consumes once-persistence approved approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makeApprovedOnce()
		repo.approvals[a.ID] = a

		result, err := svc.ConsumeApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Consumed {
			t.Fatal("expected consumed=true")
		}
		if result.ConsumedAt == nil {
			t.Fatal("expected consumed_at to be set")
		}
	})

	t.Run("idempotent consume returns already-consumed approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makeApprovedOnce()
		consumedAt := time.Now().Add(-5 * time.Minute)
		a.Consumed = true
		a.ConsumedAt = &consumedAt
		repo.approvals[a.ID] = a

		result, err := svc.ConsumeApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Consumed {
			t.Fatal("expected consumed=true")
		}
	})

	t.Run("returns consumed approval when consume loses a race", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makeApprovedOnce()
		repo.approvals[a.ID] = a
		repo.consumeFunc = func(_ context.Context, approvalID id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error) {
			requireSameApprovalID := approvalID == a.ID
			if !requireSameApprovalID {
				t.Fatalf("unexpected approval ID: %s", approvalID)
			}
			consumedAtCopy := consumedAt
			repo.approvals[a.ID].Consumed = true
			repo.approvals[a.ID].ConsumedAt = &consumedAtCopy
			return nil, storage.NewStorageError("Consume", storage.ErrorKindNotFound, nil, "approval not found")
		}

		result, err := svc.ConsumeApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Consumed {
			t.Fatal("expected consumed=true")
		}
		if result.ConsumedAt == nil {
			t.Fatal("expected consumed_at to be set")
		}
	})

	t.Run("returns ErrApprovalNotFound for missing approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		_, err := svc.ConsumeApproval(context.Background(), id.NewApprovalID(), principal)
		if err != ErrApprovalNotFound {
			t.Fatalf("expected ErrApprovalNotFound, got %v", err)
		}
	})

	t.Run("returns ErrApprovalForbidden for wrong principal", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makeApprovedOnce()
		repo.approvals[a.ID] = a

		_, err := svc.ConsumeApproval(context.Background(), a.ID, id.Principal("other@example.com"))
		if err != ErrApprovalForbidden {
			t.Fatalf("expected ErrApprovalForbidden, got %v", err)
		}
	})

	t.Run("returns ErrApprovalNotConsumable for session persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		p := storage.ApprovalPersistenceSession
		now := time.Now()
		a.Status = storage.ApprovalStatusApproved
		a.Persistence = &p
		a.ApprovedAt = &now
		repo.approvals[a.ID] = a

		_, err := svc.ConsumeApproval(context.Background(), a.ID, principal)
		if err != ErrApprovalNotConsumable {
			t.Fatalf("expected ErrApprovalNotConsumable, got %v", err)
		}
	})

	t.Run("returns ErrApprovalNotConsumable for permanent persistence", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		p := storage.ApprovalPersistencePermanent
		now := time.Now()
		a.Status = storage.ApprovalStatusApproved
		a.Persistence = &p
		a.ApprovedAt = &now
		repo.approvals[a.ID] = a

		_, err := svc.ConsumeApproval(context.Background(), a.ID, principal)
		if err != ErrApprovalNotConsumable {
			t.Fatalf("expected ErrApprovalNotConsumable, got %v", err)
		}
	})

	t.Run("returns ErrApprovalNotConsumable for pending approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		repo.approvals[a.ID] = a

		_, err := svc.ConsumeApproval(context.Background(), a.ID, principal)
		if err != ErrApprovalNotConsumable {
			t.Fatalf("expected ErrApprovalNotConsumable, got %v", err)
		}
	})
}

func TestService_ListPermanentApprovals(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	t.Run("returns permanent approvals for principal", func(t *testing.T) {
		perm := storage.ApprovalPersistencePermanent
		a1 := makePendingApproval(principal, agentID)
		a1.Status = storage.ApprovalStatusApproved
		a1.Persistence = &perm
		a2 := makePendingApproval(principal, agentID)
		a2.Status = storage.ApprovalStatusDenied
		a2.Persistence = &perm

		queries := &mockQueryRepo{approvals: []*storage.ToolApproval{a1, a2}}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), queries, &mockSyncStateRepo{})

		result, err := svc.ListPermanentApprovals(context.Background(), principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 permanent approvals, got %d", len(result))
		}
	})

	t.Run("returns empty list when no permanent approvals", func(t *testing.T) {
		queries := &mockQueryRepo{}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), queries, &mockSyncStateRepo{})

		result, err := svc.ListPermanentApprovals(context.Background(), principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected 0 permanent approvals, got %d", len(result))
		}
	})

}

func TestService_ListPendingApprovals(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	t.Run("returns pending approvals sorted by created_at descending", func(t *testing.T) {
		base := time.Now()

		older := makePendingApproval(principal, agentID)
		older.CreatedAt = base.Add(-2 * time.Minute)

		approved := makePendingApproval(principal, agentID)
		approved.Status = storage.ApprovalStatusApproved
		approved.CreatedAt = base.Add(-time.Minute)

		newer := makePendingApproval(principal, agentID)
		newer.CreatedAt = base

		queries := &mockQueryRepo{approvals: []*storage.ToolApproval{older, approved, newer}}
		svc := newTestServiceWithQueries(newMockApprovalRepo(), queries, &mockSyncStateRepo{})

		result, err := svc.ListPendingApprovals(context.Background(), principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 pending approvals, got %d", len(result))
		}
		if result[0].ID != newer.ID {
			t.Fatalf("expected newest approval first, got %s", result[0].ID)
		}
		if result[1].ID != older.ID {
			t.Fatalf("expected oldest approval last, got %s", result[1].ID)
		}
	})
}

func TestService_RevokePermanentApproval(t *testing.T) {
	principal := id.Principal("user@example.com")
	agentID := id.NewAgentID()

	makePermanentApproved := func() *storage.ToolApproval {
		a := makePendingApproval(principal, agentID)
		p := storage.ApprovalPersistencePermanent
		now := time.Now()
		a.Status = storage.ApprovalStatusApproved
		a.Persistence = &p
		a.ApprovedAt = &now
		return a
	}

	t.Run("revokes permanent approval successfully", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePermanentApproved()
		repo.approvals[a.ID] = a

		result, err := svc.RevokePermanentApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != storage.ApprovalStatusDenied {
			t.Fatalf("expected denied status, got %s", result.Status)
		}
		if result.Persistence != nil {
			t.Fatalf("expected permanent persistence to be cleared, got %s", *result.Persistence)
		}
		_, err = svc.RevokePermanentApproval(context.Background(), a.ID, principal)
		if err != ErrApprovalNotRevocable {
			t.Fatalf("expected ErrApprovalNotRevocable after revoke, got %v", err)
		}
	})

	t.Run("revokes permanent denial successfully", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePermanentApproved()
		a.Status = storage.ApprovalStatusDenied
		a.DeniedAt = a.ApprovedAt
		repo.approvals[a.ID] = a

		result, err := svc.RevokePermanentApproval(context.Background(), a.ID, principal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Status != storage.ApprovalStatusDenied || result.Persistence != nil {
			t.Fatal("expected permanent denial to remain denied with persistence cleared")
		}
	})

	t.Run("returns ErrApprovalNotFound for missing approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		_, err := svc.RevokePermanentApproval(context.Background(), id.NewApprovalID(), principal)
		if err != ErrApprovalNotFound {
			t.Fatalf("expected ErrApprovalNotFound, got %v", err)
		}
	})

	t.Run("returns ErrApprovalForbidden for wrong principal", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePermanentApproved()
		repo.approvals[a.ID] = a

		_, err := svc.RevokePermanentApproval(context.Background(), a.ID, id.Principal("other@example.com"))
		if err != ErrApprovalForbidden {
			t.Fatalf("expected ErrApprovalForbidden, got %v", err)
		}
	})

	t.Run("returns ErrApprovalNotRevocable for non-permanent approval", func(t *testing.T) {
		repo := newMockApprovalRepo()
		svc := newTestService(repo)

		a := makePendingApproval(principal, agentID)
		once := storage.ApprovalPersistenceOnce
		now := time.Now()
		a.Status = storage.ApprovalStatusApproved
		a.Persistence = &once
		a.ApprovedAt = &now
		repo.approvals[a.ID] = a

		_, err := svc.RevokePermanentApproval(context.Background(), a.ID, principal)
		if err != ErrApprovalNotRevocable {
			t.Fatalf("expected ErrApprovalNotRevocable, got %v", err)
		}
	})
}
