package approval

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type testQueryRepo struct {
	approvals      []*storage.ToolApproval
	activeSessions []string
}

func (m *testQueryRepo) ListAllActive(_ context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) ([]*storage.ToolApproval, error) {
	m.activeSessions = append([]string(nil), activeAgentSessionIDs...)
	if principalFilter == nil {
		return m.approvals, nil
	}
	var result []*storage.ToolApproval
	for _, a := range m.approvals {
		if a.Principal == *principalFilter {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *testQueryRepo) ListActiveByPrincipalAndAgent(_ context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	var result []*storage.ToolApproval
	for _, a := range m.approvals {
		if a.Principal == principal && a.AgentID == agentID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *testQueryRepo) ListPermanentByPrincipal(_ context.Context, _ id.Principal) ([]*storage.ToolApproval, error) {
	return nil, nil
}

type testSyncStateRepo struct {
	version atomic.Int64
	onGet   func()
}

func newTestSyncStateRepo(v int64) *testSyncStateRepo {
	r := &testSyncStateRepo{}
	r.version.Store(v)
	return r
}

func (m *testSyncStateRepo) GetVersion(_ context.Context) (int64, error) {
	if m.onGet != nil {
		m.onGet()
		m.onGet = nil
	}
	return m.version.Load(), nil
}

func (m *testSyncStateRepo) IncrementVersion(_ context.Context) (int64, error) {
	return m.version.Add(1), nil
}

type testApprovalRepo struct{}

func (m *testApprovalRepo) Create(_ context.Context, a *storage.ToolApproval) (*storage.ToolApproval, error) {
	return a, nil
}

func (m *testApprovalRepo) Get(_ context.Context, _ id.ApprovalID) (*storage.ToolApproval, error) {
	return nil, nil
}

func (m *testApprovalRepo) Approve(_ context.Context, _ id.ApprovalID, _ storage.ApprovalDecision, _ time.Time) (*storage.ToolApproval, error) {
	return nil, nil
}

func (m *testApprovalRepo) Deny(_ context.Context, _ id.ApprovalID, _ *storage.ApprovalPersistence, _ time.Time) (*storage.ToolApproval, error) {
	return nil, nil
}

func (m *testApprovalRepo) RevokePermanent(_ context.Context, _ id.ApprovalID, _ time.Time) (*storage.ToolApproval, error) {
	return nil, nil
}

func (m *testApprovalRepo) Consume(_ context.Context, _ id.ApprovalID, _ time.Time) (*storage.ToolApproval, error) {
	return nil, nil
}

func newTestSyncService(queries *testQueryRepo, syncState *testSyncStateRepo, broadcaster *domainapproval.ApprovalSyncBroadcaster) *domainapproval.Service {
	return domainapproval.NewService(
		&testApprovalRepo{},
		queries,
		nil,
		syncState,
		nil,
		domainapproval.NewApprovalRateLimiter(50, 10),
		broadcaster,
		10*time.Minute,
		"https://broker.example.com",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

func TestSyncHandler_ImmediateReturn(t *testing.T) {
	t.Run("returns 200 with ETag when no If-None-Match header", func(t *testing.T) {
		syncState := newTestSyncStateRepo(7)
		queries := &testQueryRepo{}
		broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
		svc := newTestSyncService(queries, syncState, broadcaster)
		handler := NewSyncHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if etag := rec.Header().Get("ETag"); etag != `"v7"` {
			t.Fatalf("expected ETag \"v7\", got %s", etag)
		}

		var resp syncResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Data.Pairs) != 0 {
			t.Fatalf("expected 0 pairs, got %d", len(resp.Data.Pairs))
		}
	})

	t.Run("forwards active agent sessions", func(t *testing.T) {
		queries := &testQueryRepo{}
		handler := NewSyncHandler(newTestSyncService(queries, newTestSyncStateRepo(1), nil))
		req := httptest.NewRequest(http.MethodGet, "/api/approvals?agent_session_id=session-1&agent_session_id=session-2", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if len(queries.activeSessions) != 2 || queries.activeSessions[0] != "session-1" || queries.activeSessions[1] != "session-2" {
			t.Fatalf("active sessions = %v, want [session-1 session-2]", queries.activeSessions)
		}
	})

	t.Run("returns 200 immediately when server version is newer than client", func(t *testing.T) {
		syncState := newTestSyncStateRepo(10)
		queries := &testQueryRepo{}
		broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
		svc := newTestSyncService(queries, syncState, broadcaster)
		handler := NewSyncHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("If-None-Match", `"v5"`)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if etag := rec.Header().Get("ETag"); etag != `"v10"` {
			t.Fatalf("expected ETag \"v10\", got %s", etag)
		}
	})

	t.Run("returns current state when client version is ahead", func(t *testing.T) {
		syncState := newTestSyncStateRepo(5)
		queries := &testQueryRepo{}
		broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
		handler := NewSyncHandler(newTestSyncService(queries, syncState, broadcaster))

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("If-None-Match", `"v10"`)
		req.Header.Set("X-Long-Poll-Timeout", "1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if etag := rec.Header().Get("ETag"); etag != `"v5"` {
			t.Fatalf("expected ETag \"v5\", got %s", etag)
		}
	})
}

func TestSyncHandler_ChangeDuringVersionCheck(t *testing.T) {
	syncState := newTestSyncStateRepo(5)
	queries := &testQueryRepo{}
	broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
	syncState.onGet = func() {
		syncState.version.Store(6)
		broadcaster.Broadcast()
	}
	handler := NewSyncHandler(newTestSyncService(queries, syncState, broadcaster))

	req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
	req.Header.Set("If-None-Match", `"v5"`)
	req.Header.Set("X-Long-Poll-Timeout", "1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after a change during version check, got %d", rec.Code)
	}
	if etag := rec.Header().Get("ETag"); etag != `"v6"` {
		t.Fatalf("expected updated ETag, got %s", etag)
	}
}

func TestSyncHandler_LongPollTimeout(t *testing.T) {
	t.Run("returns 304 when long-poll times out with no changes", func(t *testing.T) {
		syncState := newTestSyncStateRepo(5)
		queries := &testQueryRepo{}
		broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
		svc := newTestSyncService(queries, syncState, broadcaster)
		handler := NewSyncHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("If-None-Match", `"v5"`)
		req.Header.Set("X-Long-Poll-Timeout", "1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotModified {
			t.Fatalf("expected 304, got %d", rec.Code)
		}
	})

	t.Run("returns 304 when the initial version matches v0", func(t *testing.T) {
		syncState := newTestSyncStateRepo(0)
		handler := NewSyncHandler(newTestSyncService(&testQueryRepo{}, syncState, domainapproval.NewApprovalSyncBroadcaster(0)))

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("If-None-Match", `"v0"`)
		req.Header.Set("X-Long-Poll-Timeout", "1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotModified {
			t.Fatalf("expected 304, got %d", rec.Code)
		}
	})
}

func TestSyncHandler_WakeOnChange(t *testing.T) {
	t.Run("returns 200 when broadcaster wakes handler", func(t *testing.T) {
		syncState := newTestSyncStateRepo(5)
		principal := id.Principal("user@example.com")
		agentID := id.NewAgentID()
		approval := &storage.ToolApproval{
			ID:            id.NewApprovalID(),
			Principal:     principal,
			AgentID:       agentID,
			ToolName:      "test-tool",
			ArgumentsHash: "abc123",
			Status:        storage.ApprovalStatusPending,
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(10 * time.Minute),
		}
		queries := &testQueryRepo{approvals: []*storage.ToolApproval{approval}}
		broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
		svc := newTestSyncService(queries, syncState, broadcaster)
		handler := NewSyncHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("If-None-Match", `"v5"`)
		req.Header.Set("X-Long-Poll-Timeout", "30")
		rec := httptest.NewRecorder()

		go func() {
			time.Sleep(100 * time.Millisecond)
			syncState.version.Store(6)
			broadcaster.Broadcast()
		}()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp syncResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Data.Pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(resp.Data.Pairs))
		}
	})
}

func TestSyncHandler_ClientDisconnect(t *testing.T) {
	t.Run("exits cleanly when client context is cancelled", func(t *testing.T) {
		syncState := newTestSyncStateRepo(5)
		queries := &testQueryRepo{}
		broadcaster := domainapproval.NewApprovalSyncBroadcaster(0)
		svc := newTestSyncService(queries, syncState, broadcaster)
		handler := NewSyncHandler(svc)

		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil).WithContext(ctx)
		req.Header.Set("If-None-Match", `"v5"`)
		req.Header.Set("X-Long-Poll-Timeout", "30")
		rec := httptest.NewRecorder()

		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		handler.ServeHTTP(rec, req)
	})
}

func TestToSummaryIncludesApprovalPatterns(t *testing.T) {
	approval := &storage.ToolApproval{ToolName: "create_pull_request", ToolPattern: "issues.*", ParamsPattern: map[string]string{"repo": "acme/*"}}
	summary := toSummary(approval)
	if summary.ToolPattern != "issues.*" || summary.ParamsPattern["repo"] != "acme/*" {
		t.Fatalf("summary omitted pattern: %#v", summary)
	}

	summary = toSummary(&storage.ToolApproval{ToolName: "create_pull_request", ToolPattern: "create_pull_request"})
	if summary.ParamsPattern == nil || len(summary.ParamsPattern) != 0 {
		t.Fatalf("nil pattern must serialize as empty object: %#v", summary.ParamsPattern)
	}
}
