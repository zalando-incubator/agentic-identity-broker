package approval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

func TestGetHandler_RequiresAuthenticatedOwner(t *testing.T) {
	approveHandler, _, approval := newApprovePatternHandler(t)
	handler := NewGetHandler(approveHandler.service)

	for _, test := range []struct {
		name, principal, errorCode string
		status                     int
	}{
		{name: "rejects unauthenticated callers", status: http.StatusUnauthorized, errorCode: "unauthorized"},
		{name: "rejects a different principal", principal: "other@example.com", status: http.StatusForbidden, errorCode: "forbidden"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/approvals/"+approval.ID.String(), nil)
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("id", approval.ID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
			if test.principal != "" {
				req = req.WithContext(principal.WithPrincipal(req.Context(), test.principal))
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, test.status, rec.Body.String())
			}
			var response errorResponse
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if response.Error != test.errorCode {
				t.Fatalf("error = %q, want %q", response.Error, test.errorCode)
			}
		})
	}
}

func TestGetHandler_ReturnsOwnerApprovalDetail(t *testing.T) {
	approveHandler, _, approval := newApprovePatternHandler(t)
	handler := NewGetHandler(approveHandler.service)

	req := httptest.NewRequest(http.MethodGet, "/api/approvals/"+approval.ID.String(), nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", approval.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	req = req.WithContext(principal.WithPrincipal(req.Context(), string(approval.Principal)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Data domainapproval.ApprovalDetail `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Data.ID != approval.ID || response.Data.Principal != approval.Principal || response.Data.ToolName != approval.ToolName || response.Data.Status != storage.ApprovalStatusPending {
		t.Fatalf("approval detail = %#v", response.Data)
	}
	if response.Data.PatternPreview != "create_pull_request(repo=acme/app,title=Fix bug)" {
		t.Fatalf("pattern_preview = %q, want fixture's exact scope", response.Data.PatternPreview)
	}
}
