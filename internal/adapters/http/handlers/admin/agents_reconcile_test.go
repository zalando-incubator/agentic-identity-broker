package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
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
	repo.On("GetByExternalID", mock.Anything, id.ExternalID("registry")).Return(nil, storage.NewStorageError("lookup", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")).Once()
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
		{"misspelled selector", "/api/agents?externalId=a", valid, "invalid external_id"},
		{"empty selector", "/api/agents?external_id=", valid, "invalid external_id"},
		{"null external", "/api/agents?external_id=a", strings.TrimSuffix(valid, "}") + `,"external_id":null}`, "invalid external_id"},
		{"empty external", "/api/agents?external_id=a", strings.TrimSuffix(valid, "}") + `,"external_id":""}`, "invalid external_id"},
		{"different external", "/api/agents?external_id=a", strings.TrimSuffix(valid, "}") + `,"external_id":"b"}`, "invalid external_id"},
		{"empty client", "/api/agents?external_id=a", strings.TrimSuffix(valid, "}") + `,"client_id":""}`, "validation failed"},
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

func TestReconcileHandlerLogsValidationFailures(t *testing.T) {
	permissionSetID := id.NewPermissionSetID()
	serviceID := id.NewServiceID()
	valid := `{"display_name":"Research","description":"Definition","permission_sets":[{"permission_set_id":"` + permissionSetID.String() + `","requirement_type":"optional"}]}`
	for _, tc := range []struct {
		name, path, body            string
		readError, coverage, lookup bool
	}{
		{name: "selector", path: "/api/agents", body: valid},
		{name: "read failure", body: valid, readError: true},
		{name: "body limit", body: strings.Repeat("a", 65537)},
		{name: "JSON syntax", body: `{"display_name":`},
		{name: "JSON type", body: `{"display_name":123}`},
		{name: "null body", body: `null`},
		{name: "client ID", body: strings.TrimSuffix(valid, "}") + `,"client_id":null}`},
		{name: "permission sets", body: `{"permission_sets":[]}`},
		{name: "service requirements", body: strings.TrimSuffix(valid, "}") + `,"service_requirements":[{"service_id":"` + serviceID.String() + `","requirement_type":"invalid"}]}`},
		{name: "coverage", body: strings.TrimSuffix(valid, "}") + `,"service_requirements":[{"service_id":"` + serviceID.String() + `","requirement_type":"optional"}]}`, coverage: true},
		{name: "client URIs", body: strings.TrimSuffix(valid, "}") + `,"client_uris":["https://agent.example/client?token=do-not-log-this","https://agent.example/client?token=do-not-log-this"]}`},
		{name: "domain external ID", body: strings.TrimSuffix(valid, "}") + `,"external_id":null}`},
		{name: "domain validation", body: strings.Replace(valid, `"display_name":"Research",`, "", 1), lookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			repo := new(MockAgentRepository)
			handler := newAgentsHandlerForTest(repo, new(MockProviderRepository), logger)
			if tc.coverage {
				ps := new(MockPermissionSetValidator)
				ps.On("ValidateIDs", mock.Anything, []id.PermissionSetID{permissionSetID}).Return(nil).Once()
				ps.On("GetByIDs", mock.Anything, []id.PermissionSetID{permissionSetID}).Return([]*storage.PermissionSet{{ID: permissionSetID}}, nil).Once()
				handler.psService = ps
				handler.agentService = agents.NewService(repo, handler.providerService, logger, true, ps)
				repo.On("GetByExternalID", mock.Anything, id.ExternalID("registry")).Return(nil, storage.NewStorageError("lookup", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")).Once()
				t.Cleanup(func() { ps.AssertExpectations(t) })
			}
			if tc.lookup {
				repo.On("GetByExternalID", mock.Anything, id.ExternalID("registry")).Return(nil, storage.NewStorageError("lookup", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")).Once()
			}
			path := tc.path
			if path == "" {
				path = "/api/agents?external_id=registry"
			}
			var body io.Reader = strings.NewReader(tc.body)
			if tc.readError {
				body = iotest.ErrReader(errors.New("unreadable request"))
			}
			w := httptest.NewRecorder()
			handler.ReconcileAgent(w, httptest.NewRequest(http.MethodPut, path, body))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.NotContains(t, logs.String(), "do-not-log-this")
			var record map[string]any
			decoder := json.NewDecoder(&logs)
			require.NoError(t, decoder.Decode(&record), "validation failure must be logged")
			assert.Equal(t, "WARN", record["level"])
			assert.NotEmpty(t, record["error"])
			assert.ErrorIs(t, decoder.Decode(&record), io.EOF, "failure must be logged once")
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
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
	assert.Equal(t, "2026-01-01T00:02:03Z", response.CreatedAt)
	assert.Equal(t, response.CreatedAt, response.UpdatedAt)
	assert.Equal(t, before, a)
}
