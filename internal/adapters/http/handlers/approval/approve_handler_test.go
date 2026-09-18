package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

func newApprovePatternHandler(t *testing.T) (*ApproveHandler, *memory.ToolApprovalRepository, *storage.ToolApproval) {
	t.Helper()
	repo := memory.NewToolApprovalRepository()
	svc := domainapproval.NewService(repo, repo, repo, memory.NewApprovalSyncStateRepository(), nil, domainapproval.NewApprovalRateLimiter(50, 10), domainapproval.NewApprovalSyncBroadcaster(0), time.Minute, "https://broker.example", slog.New(slog.NewTextHandler(io.Discard, nil)))
	approval := &storage.ToolApproval{ID: id.NewApprovalID(), Principal: id.Principal("user@example.com"), AgentID: id.NewAgentID(), ToolName: "create_pull_request", Arguments: map[string]any{"repo": "acme/app", "title": "Fix bug"}, ArgumentsHash: "hash", Status: storage.ApprovalStatusPending, ApprovalURL: "https://broker.example/approval", ExpiresAt: time.Now().Add(time.Minute)}
	if err := domainapproval.ApplyExactPatterns(approval); err != nil {
		t.Fatal(err)
	}
	_, err := repo.Create(context.Background(), approval)
	if err != nil {
		t.Fatal(err)
	}
	return NewApproveHandler(svc), repo, approval
}

func TestApproveHandlerPatterns(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		params     map[string]string
		code       string
		message    string
	}{
		{"omitted defaults exact", `{"persistence":"once"}`, http.StatusOK, map[string]string{"repo": "acme/app", "title": "Fix bug"}, "", ""},
		{"empty allows any arguments", `{"persistence":"session","params_pattern":{}}`, http.StatusOK, map[string]string{}, "", ""},
		{"edited pattern persists", `{"persistence":"permanent","params_pattern":{"repo":"acme/*"}}`, http.StatusOK, map[string]string{"repo": "acme/*"}, "", ""},
		{"tool pattern is not allowed", `{"persistence":"permanent","tool_pattern":"*"}`, http.StatusBadRequest, nil, "invalid_request", "tool_pattern is not allowed"},
		{"explicit null rejects", `{"persistence":"permanent","params_pattern":null}`, http.StatusUnprocessableEntity, nil, "invalid_pattern", ""},
		{"non-object params reject", `{"persistence":"permanent","params_pattern":"*"}`, http.StatusBadRequest, nil, "invalid_request", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, repo, approval := newApprovePatternHandler(t)
			req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+approval.ID.String()+"/approve", bytes.NewBufferString(test.body))
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("id", approval.ID.String())
			req = req.WithContext(context.WithValue(principal.WithPrincipal(req.Context(), "user@example.com"), chi.RouteCtxKey, routeContext))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, test.status, rec.Body.String())
			}
			if test.code != "" {
				var response map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if response["error"] != test.code {
					t.Fatalf("error = %q", response["error"])
				}
				if test.message != "" && response["message"] != test.message {
					t.Fatalf("message = %q, want %q", response["message"], test.message)
				}
				stored, err := repo.Get(context.Background(), approval.ID)
				if err != nil || stored.Status != storage.ApprovalStatusPending {
					t.Fatalf("rejected approval mutated: %#v, %v", stored, err)
				}
				return
			}
			stored, err := repo.Get(context.Background(), approval.ID)
			if err != nil || stored.ParamsPattern == nil {
				t.Fatalf("approval not persisted: %#v, %v", stored, err)
			}
			if len(stored.ParamsPattern) != len(test.params) {
				t.Fatalf("params = %#v, want %#v", stored.ParamsPattern, test.params)
			}
			for key, value := range test.params {
				if stored.ParamsPattern[key] != value {
					t.Fatalf("params[%q] = %q", key, stored.ParamsPattern[key])
				}
			}
		})
	}
}
