package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAgentWriteErrorEnvelopes(t *testing.T) {
	for _, operation := range []string{"POST", "PUT ID", "PUT create", "PUT update"} {
		for _, failure := range []string{"empty external", "external conflict", "canonical conflict", "client URI conflict"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				repo := new(MockAgentRepository)
				handler := newAgentsHandlerForTest(repo, new(MockProviderRepository), slog.Default())
				existing := &storage.Agent{ID: id.NewAgentID(), ExternalID: ptr.To(id.ExternalID("registry")), DisplayName: "Original", Description: "Definition", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}}}
				body := `{"display_name":"Changed","description":"Definition","permission_sets":[{"permission_set_id":"` + existing.PermissionSets[0].PermissionSetID.String() + `","requirement_type":"optional"}],"external_id":"registry"}`
				status, code, message := http.StatusConflict, "conflict", "external_id is already held by another agent"
				cause := error(ports.ErrExternalIDConflict)
				switch failure {
				case "empty external":
					status = http.StatusBadRequest
					code = "validation failed"
					body = strings.Replace(body, `"external_id":"registry"`, `"external_id":""`, 1)
				case "canonical conflict":
					message = "agent canonical_id already exists"
					cause = nil
				case "client URI conflict":
					message = "client_uri already registered to another agent"
					cause = nil
				}
				failureErr := storage.NewStorageError("Write", storage.ErrorKindConflict, cause, message)
				method, path := http.MethodPost, "/api/agents"
				call := handler.CreateAgent
				if operation != "POST" {
					method = http.MethodPut
				}
				switch operation {
				case "POST":
					if failure != "empty external" {
						repo.On("Create", mock.Anything, mock.Anything).Return(failureErr).Once()
					}
				case "PUT ID":
					path += "/" + existing.ID.String()
					call = handler.UpdateAgent
					repo.On("Get", mock.Anything, existing.ID).Return(existing.Copy(), nil)
					if failure != "empty external" {
						repo.On("Update", mock.Anything, mock.Anything).Return(failureErr).Once()
					}
				case "PUT create", "PUT update":
					path += "?external_id=registry"
					call = handler.ReconcileAgent
					if failure == "empty external" {
						code = "invalid external_id"
					} else if operation == "PUT create" {
						repo.On("ListByExternalID", mock.Anything, id.ExternalID("registry")).Return([]*storage.Agent{}, nil).Twice()
						repo.On("Create", mock.Anything, mock.Anything).Return(failureErr).Once()
					} else {
						repo.On("ListByExternalID", mock.Anything, id.ExternalID("registry")).Return([]*storage.Agent{existing.Copy()}, nil).Once()
						repo.On("Get", mock.Anything, existing.ID).Return(existing.Copy(), nil)
						repo.On("Update", mock.Anything, mock.Anything).Return(failureErr).Once()
					}
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				route := chi.NewRouteContext()
				route.URLParams.Add("agent-id", existing.ID.String())
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
				w := httptest.NewRecorder()
				call(w, req)
				require.Equal(t, status, w.Code, w.Body.String())
				var result map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				assert.Len(t, result, 2)
				assert.Equal(t, code, result["error"])
				if failure != "empty external" {
					assert.Equal(t, message, result["message"])
				}
				repo.AssertExpectations(t)
			})
		}
	}
}
