package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

func TestDenyHandler_PersistsOwnedDenialAndRejectsOtherPrincipal(t *testing.T) {
	approveHandler, repo, approval := newApprovePatternHandler(t)
	handler := NewDenyHandler(approveHandler.service)

	t.Run("rejects a different principal without mutating approval", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+approval.ID.String()+"/deny", bytes.NewBufferString(`{"persistence":"permanent"}`))
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("id", approval.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
		req = req.WithContext(principal.WithPrincipal(req.Context(), "other@example.com"))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
		var response errorResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Error != "forbidden" {
			t.Fatalf("error = %q, want forbidden", response.Error)
		}
		stored, err := repo.Get(context.Background(), approval.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != storage.ApprovalStatusPending || stored.DeniedAt != nil {
			t.Fatalf("rejected denial mutated approval: %#v", stored)
		}
	})

	t.Run("denies the owner's approval permanently", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+approval.ID.String()+"/deny", bytes.NewBufferString(`{"persistence":"permanent"}`))
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("id", approval.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
		req = req.WithContext(principal.WithPrincipal(req.Context(), string(approval.Principal)))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var response denyResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Data.ID != approval.ID.String() || response.Data.Status != string(storage.ApprovalStatusDenied) || response.Data.Persistence == nil || *response.Data.Persistence != string(storage.ApprovalPersistencePermanent) {
			t.Fatalf("deny response = %#v", response.Data)
		}
		stored, err := repo.Get(context.Background(), approval.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != storage.ApprovalStatusDenied || stored.Persistence == nil || *stored.Persistence != storage.ApprovalPersistencePermanent || stored.DeniedAt == nil {
			t.Fatalf("denial was not persisted: %#v", stored)
		}
		deniedAt, err := time.Parse(time.RFC3339, response.Data.DeniedAt)
		if err != nil || !deniedAt.Equal(stored.DeniedAt.Truncate(time.Second)) {
			t.Fatalf("denied_at = %q, want %q (parse error: %v)", response.Data.DeniedAt, stored.DeniedAt.Format(time.RFC3339), err)
		}
	})
}
