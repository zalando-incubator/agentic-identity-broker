package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/go-chi/chi/v5"
)

func TestScopePreviewHandlerPatternDecoding(t *testing.T) {
	approveHandler, _, approval := newApprovePatternHandler(t)
	handler := NewScopePreviewHandler(approveHandler.service)
	for _, test := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"whitespace null", `{"params_pattern":  null  }`, http.StatusUnprocessableEntity, "invalid_pattern"},
		{"scalar params", `{"params_pattern":"*"}`, http.StatusBadRequest, "invalid_request"},
		{"array params", `{"params_pattern":[]}`, http.StatusBadRequest, "invalid_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+approval.ID.String()+"/scope-preview", bytes.NewBufferString(test.body))
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("id", approval.ID.String())
			req = req.WithContext(context.WithValue(principal.WithPrincipal(req.Context(), "user@example.com"), chi.RouteCtxKey, routeContext))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, test.status, rec.Body.String())
			}
			var response map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if response["error"] != test.code {
				t.Fatalf("error = %q, want %q", response["error"], test.code)
			}
		})
	}
}
