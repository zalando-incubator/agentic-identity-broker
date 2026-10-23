package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/requestbody"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval/toolpattern"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type failingRequestBody struct {
	data string
}

func (b *failingRequestBody) Read(p []byte) (int, error) {
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, errors.New("request read failed")
}

func (*failingRequestBody) Close() error { return nil }

func TestApprovalHandlersRejectInvalidBodiesWithoutMutation(t *testing.T) {
	for _, operation := range []struct {
		name  string
		limit int
		valid string
	}{
		{"create", approvalCreateBodyLimit, `{"metadata":{"description":"review"},"tool_name":"read","arguments":{}}`},
		{"approve", approvalDecisionBodyLimit, `{"persistence":"once"}`},
		{"scope-preview", approvalDecisionBodyLimit, `{}`},
		{"deny", 4 << 10, `{}`},
	} {
		t.Run(operation.name, func(t *testing.T) {
			for _, test := range []struct {
				name      string
				body      string
				readError bool
				status    int
			}{
				{"unknown field", `{"unexpected":true}`, false, http.StatusBadRequest},
				{"trailing value", operation.valid + `{}`, false, http.StatusBadRequest},
				{"trailing junk", operation.valid + `x`, false, http.StatusBadRequest},
				{"oversize whitespace", operation.valid + strings.Repeat(" ", operation.limit), false, http.StatusRequestEntityTooLarge},
				{"oversize malformed", strings.Repeat("x", operation.limit+1), false, http.StatusRequestEntityTooLarge},
				{"read failure", "", true, http.StatusBadRequest},
				{"read failure after valid JSON", operation.valid, true, http.StatusBadRequest},
			} {
				t.Run(test.name, func(t *testing.T) {
					approve, repo, record := newApprovePatternHandler(t)
					var handler http.Handler
					switch operation.name {
					case "create":
						handler = NewCreateHandler(approve.service)
					case "approve":
						handler = approve
					case "scope-preview":
						handler = NewScopePreviewHandler(approve.service)
					case "deny":
						handler = NewDenyHandler(approve.service)
					}
					req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
					req.ContentLength = -1
					if test.readError {
						req.Body = &failingRequestBody{data: test.body}
					}
					route := chi.NewRouteContext()
					route.URLParams.Add("id", record.ID.String())
					ctx := context.WithValue(req.Context(), chi.RouteCtxKey, route)
					req = req.WithContext(principal.WithPrincipal(ctx, string(record.Principal)))
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, req)
					require.Equal(t, test.status, rec.Code, rec.Body.String())
					stored, err := repo.Get(context.Background(), record.ID)
					require.NoError(t, err)
					require.Equal(t, storage.ApprovalStatusPending, stored.Status)
				})
			}
		})
	}
}

func TestDenyHandlerAcceptsEmptyAndExactLimitBodies(t *testing.T) {
	for _, body := range []string{"", `{}` + strings.Repeat(" ", (4<<10)-2)} {
		approve, repo, record := newApprovePatternHandler(t)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		route := chi.NewRouteContext()
		route.URLParams.Add("id", record.ID.String())
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, route)
		req = req.WithContext(principal.WithPrincipal(ctx, string(record.Principal)))
		rec := httptest.NewRecorder()
		NewDenyHandler(approve.service).ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		stored, err := repo.Get(context.Background(), record.ID)
		require.NoError(t, err)
		require.Equal(t, storage.ApprovalStatusDenied, stored.Status)
	}
}

var _ io.ReadCloser = (*failingRequestBody)(nil)

func TestApprovalRequestFieldsAreClosedButMapsStayOpen(t *testing.T) {
	t.Run("nested metadata rejects unknown fields", func(t *testing.T) {
		var req createRequest
		err := requestbody.Unmarshal([]byte(`{"metadata":{"description":"review","unexpected":true},"tool_name":"read","arguments":{}}`), &req)
		require.Error(t, err)
	})
	t.Run("arguments preserve arbitrary nested values", func(t *testing.T) {
		var req createRequest
		err := requestbody.Unmarshal([]byte(`{"metadata":{"description":"review"},"tool_name":"read","arguments":{"custom":{"nested":[true,1,"value"]}}}`), &req)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"nested": []any{true, float64(1), "value"}}, req.Arguments["custom"])
	})
	t.Run("parameter pattern names stay open", func(t *testing.T) {
		var req approveRequest
		require.NoError(t, requestbody.Unmarshal([]byte(`{"persistence":"session","params_pattern":{"custom.path":"*"}}`), &req))
		pattern, err := decodeParamsPattern(req.ParamsPattern)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"custom.path": "*"}, pattern)
	})
}

func TestApprovalCreateBodyLimitAllowsReencodedMCPArguments(t *testing.T) {
	mcpBody := `{"arguments":{"text":"` + strings.Repeat("<", (1<<20)-64) + `"}}`
	require.LessOrEqual(t, len(mcpBody), 1<<20)
	var invocation struct {
		Arguments map[string]any `json:"arguments"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpBody), &invocation))
	body, err := json.Marshal(map[string]any{
		"arguments": invocation.Arguments,
		"metadata":  map[string]string{"description": strings.Repeat("d", 1024)},
	})
	require.NoError(t, err)
	require.Greater(t, len(body), 6<<20, "re-encoded arguments plus the envelope need headroom above 6 MiB")

	approve, _, _ := newApprovePatternHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/approvals", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	NewCreateHandler(approve.service).ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	var response map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "tool_name is required", response["message"])
}

// The consent UI posts the stored params_pattern back verbatim, so the decision body
// limit must accept the largest pattern the domain itself accepts.
func TestApprovalDecisionBodyLimitAcceptsLargestDomainValidParamsPattern(t *testing.T) {
	arguments := make(map[string]any, toolpattern.MaxParamsPatternEntries)
	for i := range toolpattern.MaxParamsPatternEntries {
		key := strings.Repeat("\x01", toolpattern.MaxParamsPatternKeyBytes-2) + fmt.Sprintf("%02d", i)
		arguments[key] = strings.Repeat("\x01", toolpattern.MaxParamsPatternValueBytes)
	}
	pattern := toolpattern.ExactParams(arguments)
	require.NoError(t, toolpattern.ValidateParamsPattern(pattern))
	encoded, err := json.Marshal(pattern)
	require.NoError(t, err)
	require.Greater(t, len(encoded), 256<<10, "escaped patterns must exceed the former decision limit")

	for _, operation := range []struct {
		name string
		body []byte
		new  func(*ApproveHandler) http.Handler
	}{
		{"approve", fmt.Appendf(nil, `{"persistence":"permanent","params_pattern":%s}`, encoded), func(handler *ApproveHandler) http.Handler { return handler }},
		{"scope-preview", fmt.Appendf(nil, `{"params_pattern":%s}`, encoded), func(handler *ApproveHandler) http.Handler { return NewScopePreviewHandler(handler.service) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			approve, repo, record := newApprovePatternHandlerWithArguments(t, arguments)
			require.Equal(t, pattern, record.ParamsPattern)
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(operation.body))
			route := chi.NewRouteContext()
			route.URLParams.Add("id", record.ID.String())
			ctx := principal.WithPrincipal(context.WithValue(req.Context(), chi.RouteCtxKey, route), string(record.Principal))
			rec := httptest.NewRecorder()
			operation.new(approve).ServeHTTP(rec, req.WithContext(ctx))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			stored, err := repo.Get(context.Background(), record.ID)
			require.NoError(t, err)
			require.Equal(t, pattern, stored.ParamsPattern)
			if operation.name == "approve" {
				require.Equal(t, storage.ApprovalStatusApproved, stored.Status)
			} else {
				require.Equal(t, storage.ApprovalStatusPending, stored.Status)
			}
		})
	}
}
