package consent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/requestbody"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestCreateGrantRejectsInvalidBodiesBeforeService(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   string
		status int
	}{
		{"unknown field", `{"granted_permission_sets":{},"unexpected":true}`, http.StatusBadRequest},
		{"trailing value", `{"granted_permission_sets":{}} {}`, http.StatusBadRequest},
		{"trailing junk", `{"granted_permission_sets":{}} x`, http.StatusBadRequest},
		{"oversize whitespace", `{"granted_permission_sets":{}}` + strings.Repeat(" ", 64<<10), http.StatusRequestEntityTooLarge},
		{"oversize malformed", strings.Repeat("x", (64<<10)+1), http.StatusRequestEntityTooLarge},
		{"read failure", "", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewGrantsHandler(nil, nil, newTestSessionTokenValidator())
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			req.ContentLength = -1
			if test.name == "read failure" {
				req.Body = io.NopCloser(iotest.ErrReader(errors.New("request read failed")))
			}
			route := chi.NewRouteContext()
			route.URLParams.Add("agent-id", id.NewAgentID().String())
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, route)
			req = req.WithContext(principal.WithPrincipal(ctx, "user@example.com"))
			rec := httptest.NewRecorder()
			handler.CreateGrant(rec, req)
			require.Equal(t, test.status, rec.Code, rec.Body.String())
		})
	}
}

func TestGrantRequestPermissionSetKeysStayOpen(t *testing.T) {
	var req GrantRequest
	err := requestbody.Unmarshal([]byte(`{"granted_permission_sets":{"custom-service":["custom-permission"]}}`), &req)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"custom-service": {"custom-permission"}}, req.GrantedPermissionSets)
}
