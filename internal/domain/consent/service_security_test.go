package consent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestGrantConsent_RejectsUndeclaredPermissionSets(t *testing.T) {
	t.Parallel()

	declaredSetID := id.NewPermissionSetID()
	undeclaredSetID := id.NewPermissionSetID()
	declaredServiceID := id.NewServiceID()
	additionalServiceID := id.NewServiceID()

	tests := []struct {
		name                  string
		grantedPermissionSets []storage.GrantedPermissionSetEntry
		serviceRequirements   []storage.ServiceRequirement
		seedExistingGrant     bool
	}{
		{
			name: "unrelated set without service requirements",
			grantedPermissionSets: []storage.GrantedPermissionSetEntry{{
				PermissionSetID:    undeclaredSetID,
				IncludedServiceIDs: []id.ServiceID{additionalServiceID},
			}},
		},
		{
			name: "mixed upsert with service requirements",
			grantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{
					PermissionSetID:    declaredSetID,
					IncludedServiceIDs: []id.ServiceID{declaredServiceID},
				},
				{
					PermissionSetID:    undeclaredSetID,
					IncludedServiceIDs: []id.ServiceID{declaredServiceID},
				},
			},
			serviceRequirements: []storage.ServiceRequirement{{
				ServiceID:       declaredServiceID,
				RequirementType: storage.RequirementTypeMandatory,
				RequiredScopes:  []string{"read"},
			}},
			seedExistingGrant: true,
		},
		{
			name: "undeclared empty entry alongside valid declared entry",
			grantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{
					PermissionSetID:    declaredSetID,
					IncludedServiceIDs: []id.ServiceID{declaredServiceID},
				},
				{
					PermissionSetID:    undeclaredSetID,
					IncludedServiceIDs: []id.ServiceID{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			agentID := id.NewAgentID()
			principal := id.Principal("user@example.com")
			sessionID := id.NewSessionID()
			future := time.Now().Add(time.Hour)
			agent := &storage.Agent{
				ID:          agentID,
				DisplayName: "Declared permission set agent",
				PermissionSets: []storage.AgentPermissionSetEntry{{
					PermissionSetID: declaredSetID,
					RequirementType: storage.RequirementTypeOptional,
				}},
			}
			agent.ServiceRequirements = tt.serviceRequirements
			psService := &mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
				declaredSetID: {
					ID:          declaredSetID,
					Name:        "Declared read access",
					Description: "Read access to the declared service",
					ServiceScopes: []storage.ServiceScope{{
						ServiceID:       declaredServiceID,
						Scopes:          []string{"read"},
						RequirementType: storage.RequirementTypeOptional,
					}},
				},
				undeclaredSetID: {
					ID:          undeclaredSetID,
					Name:        "Unrelated administrative access",
					Description: "Broader access including an additional service",
					ServiceScopes: []storage.ServiceScope{
						{
							ServiceID:       declaredServiceID,
							Scopes:          []string{"read", "write", "admin"},
							RequirementType: storage.RequirementTypeOptional,
						},
						{
							ServiceID:       additionalServiceID,
							Scopes:          []string{"read", "write", "admin"},
							RequirementType: storage.RequirementTypeOptional,
						},
					},
				},
			}}
			sessionRepo := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{
				sessionID: {
					ID:                    sessionID,
					Principal:             principal,
					ServiceID:             additionalServiceID,
					RefreshTokenExpiresAt: &future,
					Scope:                 []string{"read", "write", "admin"},
				},
			}}
			declaredSessionID := id.NewSessionID()
			sessionRepo.sessions[declaredSessionID] = &storage.UserSession{
				ID:                    declaredSessionID,
				Principal:             principal,
				ServiceID:             declaredServiceID,
				RefreshTokenExpiresAt: &future,
				Scope:                 []string{"read", "write", "admin"},
			}
			grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}
			if tt.seedExistingGrant {
				grantID := id.NewGrantID()
				createdAt := time.Now().Add(-time.Hour)
				grantRepo.grants[grantID] = &storage.UserGrant{
					ID:         grantID,
					Principal:  principal,
					AgentID:    agentID,
					ValidUntil: &future,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
						PermissionSetID:    declaredSetID,
						IncludedServiceIDs: []id.ServiceID{declaredServiceID},
					}},
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				}
			}
			originalGrants := make(map[id.GrantID]*storage.UserGrant, len(grantRepo.grants))
			for grantID, stored := range grantRepo.grants {
				originalGrants[grantID] = stored.Copy()
			}
			svc := NewService(
				&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
				nil,
				grantRepo,
				sessionRepo,
				psService,
				slog.Default(),
			)

			grant, err := svc.GrantConsent(context.Background(), &GrantRequest{
				Principal:             principal,
				AgentID:               agentID,
				ValidUntil:            &future,
				GrantedPermissionSets: tt.grantedPermissionSets,
			})

			assert.ErrorIs(t, err, ErrGrantValidation)
			assert.True(t, grant == nil, "rejected consent must not return a grant")
			if !tt.seedExistingGrant {
				assert.Zero(t, len(grantRepo.grants), "rejected consent must not save a grant")
			}
			assert.Equal(t, originalGrants, grantRepo.grants, "rejected consent must leave storage unchanged")
			assert.Zero(t, grantRepo.createCalls, "rejected consent must not create a grant")
			assert.Zero(t, grantRepo.updateCalls, "rejected consent must not update a grant")
		})
	}
}
