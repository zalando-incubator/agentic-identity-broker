package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReconcileCanonicalNestedReferencesOnBothBranchesAndReplay(t *testing.T) {
	for _, operation := range []string{"create", "update", "replay"} {
		t.Run(operation, func(t *testing.T) {
			repo := new(MockAgentRepository)
			providers := new(MockProviderRepository)
			serviceID, fallbackServiceID := id.NewServiceID(), id.NewServiceID()
			permissionID, fallbackPermissionID := id.NewPermissionSetID(), id.NewPermissionSetID()
			services := []id.ServiceID{serviceID, fallbackServiceID}
			permissionIDs := []id.PermissionSetID{permissionID, fallbackPermissionID}
			a := &storage.Agent{ID: id.NewAgentID(), ExternalID: ptr.To(id.ExternalID("registry")), DisplayName: "Canonical Agent", Description: "Definition", CreatedAt: time.Date(2026, 1, 1, 1, 2, 3, 123456789, time.FixedZone("offset", 3600)), UpdatedAt: time.Date(2026, 1, 1, 2, 3, 4, 987654321, time.FixedZone("offset", 3600))}
			validator := canonicalPermissionSetValidator{ids: map[string]id.PermissionSetID{"canonical-permission": permissionID}, canonicalIDs: map[id.PermissionSetID]string{permissionID: "canonical-permission"}, sets: map[id.PermissionSetID]*storage.PermissionSet{}}
			for i, service := range services {
				a.ServiceRequirements = append(a.ServiceRequirements, storage.ServiceRequirement{ServiceID: service, RequirementType: storage.RequirementTypeOptional})
				a.PermissionSets = append(a.PermissionSets, storage.AgentPermissionSetEntry{PermissionSetID: permissionIDs[i], RequirementType: storage.RequirementTypeOptional})
				validator.sets[permissionIDs[i]] = &storage.PermissionSet{ID: permissionIDs[i], ServiceScopes: []storage.ServiceScope{{ServiceID: service}}}
				entity := &model.ThirdpartyOAuth2ProviderEntity{ID: service, DisplayName: "Service"}
				providers.On("Get", mock.Anything, service).Return(entity, nil)
				if i == 0 {
					providers.On("GetByCanonicalID", mock.Anything, "canonical-service").Return(entity, nil)
				}
			}
			providers.On("GetCanonicalIDs", mock.Anything, mock.Anything).Return(map[id.ServiceID]string{serviceID: "canonical-service"}, nil)
			handler := newAgentsHandlerWithCanonicalPermissionSets(repo, providers, validator)
			body := `{"display_name":"Canonical Agent","description":"Definition","service_requirements":[{"service_id":"canonical-service","requirement_type":"optional"},{"service_id":"` + fallbackServiceID.String() + `","requirement_type":"optional"}],"permission_sets":[{"permission_set_id":"canonical-permission","requirement_type":"optional"},{"permission_set_id":"` + fallbackPermissionID.String() + `","requirement_type":"optional"}]}`
			original := a.Copy()
			var written *storage.Agent
			status := http.StatusOK
			if operation == "create" {
				status = http.StatusCreated
				repo.On("ListByExternalID", mock.Anything, id.ExternalID("registry")).Return([]*storage.Agent{}, nil).Once()
				repo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) { written = args.Get(1).(*storage.Agent).Copy() }).Return(nil).Once()
			} else {
				repo.On("ListByExternalID", mock.Anything, id.ExternalID("registry")).Return([]*storage.Agent{a.Copy()}, nil).Once()
				if operation == "update" {
					body = strings.Replace(body, "Canonical Agent", "Changed Agent", 1)
					repo.On("Get", mock.Anything, a.ID).Return(a.Copy(), nil)
					repo.On("Update", mock.Anything, mock.Anything).Run(func(args mock.Arguments) { written = args.Get(1).(*storage.Agent).Copy() }).Return(nil).Once()
				}
			}
			req := httptest.NewRequest(http.MethodPut, "/api/agents?external_id=registry", strings.NewReader(body))
			req.Header.Set("Prefer", "reference-id=canonical")
			w := httptest.NewRecorder()
			handler.ReconcileAgent(w, req)
			require.Equal(t, status, w.Code, w.Body.String())
			var response AgentResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "canonical-service", response.ServiceRequirements[0].ServiceID)
			assert.Equal(t, fallbackServiceID.String(), response.ServiceRequirements[1].ServiceID)
			assert.Equal(t, "canonical-permission", response.PermissionSets[0].PermissionSetID)
			assert.Equal(t, fallbackPermissionID.String(), response.PermissionSets[1].PermissionSetID)
			assert.Equal(t, "reference-id=canonical", w.Header().Get("Preference-Applied"))
			assert.Contains(t, w.Header().Values("Vary"), "Prefer")
			if operation == "replay" {
				assert.Nil(t, written)
				assert.Equal(t, a.UpdatedAt.UTC().Format(time.RFC3339), response.UpdatedAt)
			} else {
				require.NotNil(t, written)
				assert.Equal(t, a.PermissionSets, written.PermissionSets)
				assert.Equal(t, a.ServiceRequirements, written.ServiceRequirements)
				if operation == "create" {
					assert.Equal(t, "/api/agents/"+written.ID.String(), w.Header().Get("Location"))
				} else {
					assert.Equal(t, a.ID, written.ID)
				}
			}
			assert.Equal(t, original, a)
			repo.AssertExpectations(t)
			providers.AssertExpectations(t)
		})
	}
}
