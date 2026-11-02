package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReconcileHandlerCreate(t *testing.T) {
	repo := new(MockAgentRepository)
	handler := newAgentsHandlerForTest(repo, new(MockProviderRepository), slog.Default())
	repo.On("ListByExternalID", mock.Anything, id.ExternalID("registry")).Return([]*storage.Agent{}, nil).Once()
	repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	body := `{"display_name":"Research","description":"Definition","permission_sets":[{"permission_set_id":"` + id.NewPermissionSetID().String() + `","requirement_type":"optional"}],"client_uris":["https://agent.example/z.json","https://agent.example/a.json"]}`
	w := httptest.NewRecorder()
	handler.ReconcileAgent(w, httptest.NewRequest(http.MethodPut, "/api/agents?external_id=registry", strings.NewReader(body)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var response AgentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "/api/agents/"+response.ID, w.Header().Get("Location"))
	assert.Contains(t, w.Header().Values("Vary"), "Prefer")
	assert.Equal(t, []string{"https://agent.example/a.json", "https://agent.example/z.json"}, response.ClientURIs)
	assert.Equal(t, response.CreatedAt, response.UpdatedAt)
	repo.AssertExpectations(t)
}

func TestReconcileHandlerRejectsBeforeWrites(t *testing.T) {
	valid := `{"display_name":"Research","description":"Definition","permission_sets":[{"permission_set_id":"` + id.NewPermissionSetID().String() + `","requirement_type":"optional"}]}`
	for _, tc := range []struct{ name, path, body, errorCode string }{
		{"missing selector", "/api/agents", valid, "invalid external_id"},
		{"repeated", "/api/agents?external_id=a&external_id=a", valid, "invalid external_id"},
		{"escape", "/api/agents?external_id=%ZZ", valid, "invalid external_id"},
		{"semicolon", "/api/agents?external_id=a;b", valid, "invalid external_id"},
		{"null client", "/api/agents?external_id=a", strings.TrimSuffix(valid, "}") + `,"client_id":null}`, "validation failed"},
		{"missing name", "/api/agents?external_id=a", `{"description":"Definition","permission_sets":[]}`, "validation failed"},
		{"multiple JSON", "/api/agents?external_id=a", valid + valid, "invalid request body"},
		{"limit", "/api/agents?external_id=a", `{"padding":"` + strings.Repeat("a", 65536) + `"}`, "invalid request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockAgentRepository)
			handler := newAgentsHandlerForTest(repo, new(MockProviderRepository), slog.Default())
			w := httptest.NewRecorder()
			handler.ReconcileAgent(w, httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body)))
			require.Equal(t, http.StatusBadRequest, w.Code)
			var response ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, tc.errorCode, response.Error)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		})
	}
}

func TestAgentResponseStablePresentation(t *testing.T) {
	h := newAgentsHandlerForTest(new(MockAgentRepository), new(MockProviderRepository), slog.Default())
	instant := time.Date(2026, 1, 1, 1, 2, 3, 123456789, time.FixedZone("offset", 3600))
	a := &storage.Agent{ID: id.NewAgentID(), ClientURIs: []string{"https://example/z", "https://example/A"}, CreatedAt: instant, UpdatedAt: instant}
	before := a.Copy()
	response, err := h.toResponseWithServiceMap(a, map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://example/A", "https://example/z"}, response.ClientURIs)
	assert.Equal(t, "2026-01-01T00:02:03.123456Z", response.CreatedAt)
	assert.Equal(t, response.CreatedAt, response.UpdatedAt)
	assert.Equal(t, before, a)
}
