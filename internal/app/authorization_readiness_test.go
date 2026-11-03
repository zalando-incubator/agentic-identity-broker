package app

import (
	"context"
	"testing"
	"time"

	memorystorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type readinessAgentRepository struct {
	ports.AgentRepository
	agent *storage.Agent
}

func (r readinessAgentRepository) Get(context.Context, id.AgentID) (*storage.Agent, error) {
	return r.agent, nil
}

type readinessSessionRepository struct {
	ports.UserSessionRepository
	session *storage.UserSession
	err     error
	calls   int
}

func (r *readinessSessionRepository) FindByPrincipalAndService(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
	r.calls++
	return r.session, r.err
}

func TestUserDelegationVerifierRequiresBrowserReadiness(t *testing.T) {
	for _, name := range []string{"undeclared permission set", "missing delegated session", "expired delegated session", "missing mandatory session", "mandatory scopes missing", "refreshable delegated session", "optional unselected service", "session repository failure", "mandatory repository failure", "shared mandatory and delegated session"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			principal := id.NewPrincipal("subject-1")
			agent := &storage.Agent{ID: id.NewAgentID()}
			serviceID := id.NewServiceID()
			psID := id.NewPermissionSetID()
			grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: principal}
			sessions := &readinessSessionRepository{}
			wantActive, wantError := false, false
			switch name {
			case "undeclared permission set":
				grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: psID}}
			case "missing delegated session", "expired delegated session", "refreshable delegated session", "session repository failure":
				agent.PermissionSets = []storage.AgentPermissionSetEntry{{PermissionSetID: psID}}
				grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID}}}
				if name == "expired delegated session" || name == "refreshable delegated session" {
					expired := time.Now().Add(-time.Hour)
					sessions.session = &storage.UserSession{AccessTokenExpiresAt: &expired}
					if name == "refreshable delegated session" {
						sessions.session.EncryptedRefreshToken = []byte("encrypted-refresh")
						wantActive = true
					}
				}
				if name == "session repository failure" {
					sessions.err, wantError = assert.AnError, true
				}
			case "missing mandatory session", "mandatory scopes missing", "mandatory repository failure", "shared mandatory and delegated session":
				agent.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read"}}}
				if name == "mandatory scopes missing" {
					sessions.session = &storage.UserSession{Scope: []string{"write"}}
				}
				if name == "mandatory repository failure" {
					sessions.err, wantError = assert.AnError, true
				}
				if name == "shared mandatory and delegated session" {
					agent.PermissionSets = []storage.AgentPermissionSetEntry{{PermissionSetID: psID}}
					grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID, serviceID}}}
					sessions.session = &storage.UserSession{Scope: []string{"read", "write"}}
					wantActive = true
				}
			case "optional unselected service":
				agent.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional}}
				wantActive = true
			}
			grants := memorystorage.NewUserGrantRepository()
			require.NoError(t, grants.Create(ctx, grant))
			verifier := newUserDelegationVerifier(consent.NewService(readinessAgentRepository{agent: agent}, nil, grants, sessions, nil, nil, nil))
			status, err := verifier.VerifyUserDelegation(ctx, principal, agent.ID)
			if wantError {
				require.ErrorIs(t, err, assert.AnError)
				assert.Empty(t, status)
			} else {
				require.NoError(t, err)
				if wantActive {
					assert.Equal(t, ports.UserDelegationActive, status)
				} else {
					assert.Equal(t, ports.UserDelegationConsentRequired, status)
				}
			}
			if name == "shared mandatory and delegated session" {
				assert.Equal(t, 1, sessions.calls)
			}
			if name == "undeclared permission set" || name == "optional unselected service" {
				assert.Zero(t, sessions.calls)
			}
		})
	}
}
