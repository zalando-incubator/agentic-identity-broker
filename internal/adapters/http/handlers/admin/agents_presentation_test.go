package admin

import (
	"context"
	"encoding/json"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAgentPresentationEveryHTTPPath(t *testing.T) {
	for _, prefer := range []string{"", "reference-id=canonical"} {
		for _, operation := range []string{"POST", "PUT ID", "PUT create", "PUT update", "PUT replay", "GET ID", "GET list", "GET filter"} {
			t.Run(operation+"/"+prefer, func(t *testing.T) {
				repo := new(MockAgentRepository)
				providers := new(MockProviderRepository)
				if prefer != "" && operation != "POST" && operation != "PUT ID" {
					providers.On("GetCanonicalIDs", mock.Anything, mock.Anything).Return(map[id.ServiceID]string{}, nil).Once()
				}
				h := newAgentsHandlerForTest(repo, providers, slog.Default())
				a := &storage.Agent{ID: id.NewAgentID(), ExternalID: ptr.To(id.ExternalID("registry")), DisplayName: "Research", Description: "Definition", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}}, ClientURIs: []string{"https://example.com/z.json", "https://example.com/A.json"}, CreatedAt: time.Date(2026, 1, 1, 1, 2, 3, 123456789, time.FixedZone("offset", 3600)), UpdatedAt: time.Date(2026, 1, 1, 1, 2, 3, 123456789, time.FixedZone("offset", 3600))}
				original := a.Copy()
				body := `{"display_name":"Research","description":"Definition","permission_sets":[{"permission_set_id":"` + a.PermissionSets[0].PermissionSetID.String() + `","requirement_type":"optional"}],"client_uris":["https://example.com/z.json","https://example.com/A.json"]}`
				method, path := http.MethodGet, "/api/agents"
				call := h.ListAgents
				status := http.StatusOK
				switch operation {
				case "POST":
					method = http.MethodPost
					call = h.CreateAgent
					status = http.StatusCreated
					repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
				case "PUT ID":
					method = http.MethodPut
					path += "/" + a.ID.String()
					call = h.UpdateAgent
					repo.On("Get", mock.Anything, a.ID).Return(a.Copy(), nil)
					repo.On("Update", mock.Anything, mock.Anything).Return(nil).Once()
				case "PUT create":
					method = http.MethodPut
					path += "?external_id=registry"
					call = h.ReconcileAgent
					status = http.StatusCreated
					repo.On("GetByExternalID", mock.Anything, id.ExternalID("registry")).Return(nil, storage.NewStorageError("lookup", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")).Once()
					repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
				case "PUT update", "PUT replay":
					method = http.MethodPut
					path += "?external_id=registry"
					call = h.ReconcileAgent
					if operation == "PUT update" {
						body = strings.Replace(body, "Research", "Changed", 1)
						repo.On("Update", mock.Anything, mock.Anything).Return(nil).Once()
					}
					repo.On("GetByExternalID", mock.Anything, id.ExternalID("registry")).Return(a.Copy(), nil).Once()
				case "GET ID":
					path += "/" + a.ID.String()
					call = h.GetAgent
					repo.On("Get", mock.Anything, a.ID).Return(a.Copy(), nil)
				case "GET list":
					repo.On("List", mock.Anything).Return([]*storage.Agent{a.Copy()}, nil).Once()
				case "GET filter":
					path += "?external_id=registry"
					repo.On("GetByExternalID", mock.Anything, id.ExternalID("registry")).Return(a.Copy(), nil).Once()
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Prefer", prefer)
				route := chi.NewRouteContext()
				route.URLParams.Add("agent-id", a.ID.String())
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
				w := httptest.NewRecorder()
				call(w, req)
				require.Equal(t, status, w.Code, w.Body.String())
				var response AgentResponse
				if operation == "GET list" || operation == "GET filter" {
					var list []AgentResponse
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
					require.Len(t, list, 1)
					response = list[0]
				} else {
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				}
				assert.Equal(t, []string{"https://example.com/A.json", "https://example.com/z.json"}, response.ClientURIs)
				for _, instant := range []string{response.CreatedAt, response.UpdatedAt} {
					parsed, err := time.Parse(time.RFC3339Nano, instant)
					require.NoError(t, err)
					assert.Equal(t, parsed.UTC().Format(time.RFC3339), instant)
				}
				if strings.HasPrefix(operation, "GET") || operation == "PUT replay" {
					assert.Equal(t, "2026-01-01T00:02:03Z", response.CreatedAt)
					assert.Equal(t, response.CreatedAt, response.UpdatedAt)
				}
				if strings.HasPrefix(operation, "GET") || strings.HasPrefix(operation, "PUT ") && operation != "PUT ID" {
					assert.Contains(t, w.Header().Values("Vary"), "Prefer")
					if prefer != "" {
						assert.Equal(t, prefer, w.Header().Get("Preference-Applied"))
					} else {
						assert.Empty(t, w.Header().Get("Preference-Applied"))
					}
				}
				if operation == "PUT create" {
					assert.Equal(t, "/api/agents/"+response.ID, w.Header().Get("Location"))
				}
				assert.Equal(t, a.PermissionSets[0].PermissionSetID.String(), response.PermissionSets[0].PermissionSetID)
				assert.Equal(t, original, a)
				repo.AssertExpectations(t)
				if operation == "PUT replay" {
					repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
					repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
				}
			})
		}
	}
}

func TestAgentTimestampWholeSecondPresentation(t *testing.T) {
	h := newAgentsHandlerForTest(new(MockAgentRepository), new(MockProviderRepository), slog.Default())
	for _, tc := range []struct {
		nano int
		want string
	}{{0, "2026-01-01T00:00:00Z"}, {100000000, "2026-01-01T00:00:00Z"}, {123456789, "2026-01-01T00:00:00Z"}} {
		a := &storage.Agent{CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, tc.nano, time.UTC)}
		resp, err := h.toResponseWithServiceMap(a, nil)
		require.NoError(t, err)
		assert.Equal(t, tc.want, resp.CreatedAt)
	}
}
