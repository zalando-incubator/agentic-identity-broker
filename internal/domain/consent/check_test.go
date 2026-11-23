package consent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type consentGrantStore struct {
	ports.UserGrantRepository
	grant *storage.UserGrant
	err   error
}

func (r consentGrantStore) FindByPrincipalAndAgent(context.Context, id.Principal, id.AgentID) (*storage.UserGrant, error) {
	return r.grant, r.err
}

type consentSessionStore struct {
	ports.UserSessionRepository
	sessions map[id.ServiceID]*storage.UserSession
	calls    map[id.ServiceID]int
	err      error
}

func (r *consentSessionStore) FindByPrincipalAndService(_ context.Context, _ id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	r.calls[serviceID]++
	return r.sessions[serviceID], r.err
}

func TestCheckConsent(t *testing.T) {
	principal := id.NewPrincipal("user@example.com")
	serviceID, optionalID := id.NewServiceID(), id.NewServiceID()
	permissionSetID, secondSetID := id.NewPermissionSetID(), id.NewPermissionSetID()
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	for _, tc := range []struct {
		name         string
		grantMissing bool
		grantExpiry  *time.Time
		grantError   error
		secondSet    bool
		declared     bool
		included     bool
		mandatory    bool
		optional     bool
		session      *storage.UserSession
		sessionError error
		want         ports.ConsentReason
		calls        int
		wantError    error
	}{
		{name: "missing grant", grantMissing: true, want: ports.ConsentGrantMissing},
		{name: "missing grant reported as not found", grantError: ports.ErrNotFound, want: ports.ConsentGrantMissing},
		{name: "storage not-found grant means consent", grantError: storage.NewStorageError("find grant", storage.ErrorKindNotFound, nil, "not found"), want: ports.ConsentGrantMissing},
		{name: "expired grant", grantExpiry: &past, want: ports.ConsentGrantExpired},
		{name: "undeclared granted permission set", included: true, want: ports.ConsentPermissionSetUndeclared},
		{name: "selected service needs a connection", declared: true, included: true, want: ports.ConsentIncludedSessionMissing, calls: 1},
		{name: "selected service has no usable token", declared: true, included: true, session: &storage.UserSession{AccessTokenExpiresAt: &past}, want: ports.ConsentIncludedSessionUnusable, calls: 1},
		{name: "provider rejected refresh while access token still valid", declared: true, included: true, session: &storage.UserSession{AccessTokenExpiresAt: &future, EncryptedRefreshToken: []byte("encrypted"), RefreshRejectedAt: &past}, want: ports.ConsentIncludedSessionUnusable, calls: 1},
		{name: "mandatory service needs a connection", mandatory: true, want: ports.ConsentMandatorySessionMissing, calls: 1},
		{name: "mandatory session needs reauthentication", mandatory: true, session: &storage.UserSession{AccessTokenExpiresAt: &past}, want: ports.ConsentMandatorySessionUnusable, calls: 1},
		{name: "mandatory service lacks required scope", mandatory: true, session: &storage.UserSession{Scope: []string{"read"}}, want: ports.ConsentMandatoryScopeInsufficient, calls: 1},
		{name: "refreshable selected mandatory session is ready", declared: true, included: true, mandatory: true, secondSet: true, session: &storage.UserSession{Scope: []string{"read", "write"}, AccessTokenExpiresAt: &past, EncryptedRefreshToken: []byte("encrypted")}, calls: 1},
		{name: "unselected optional service does not block", optional: true},
		{name: "repository failure is not consent", grantError: assert.AnError, want: ports.ConsentGrantLookupFailed, wantError: assert.AnError},
		{name: "session repository failure is not consent", declared: true, included: true, sessionError: assert.AnError, want: ports.ConsentSessionLookupFailed, calls: 1, wantError: assert.AnError},
		{name: "mandatory session repository failure is not consent", mandatory: true, sessionError: assert.AnError, want: ports.ConsentSessionLookupFailed, calls: 1, wantError: assert.AnError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := &storage.Agent{ID: id.NewAgentID()}
			if tc.declared {
				agent.PermissionSets = []storage.AgentPermissionSetEntry{{PermissionSetID: permissionSetID}}
				if tc.secondSet {
					agent.PermissionSets = append(agent.PermissionSets, storage.AgentPermissionSetEntry{PermissionSetID: secondSetID})
				}
			}
			if tc.mandatory {
				agent.ServiceRequirements = append(agent.ServiceRequirements, storage.ServiceRequirement{ServiceID: serviceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"write"}})
			}
			if tc.optional {
				agent.ServiceRequirements = append(agent.ServiceRequirements, storage.ServiceRequirement{ServiceID: optionalID, RequirementType: storage.RequirementTypeOptional})
			}
			grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: principal}
			if tc.grantExpiry != nil {
				grant.ValidUntil = tc.grantExpiry
			}
			if tc.included {
				grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: permissionSetID, IncludedServiceIDs: []id.ServiceID{serviceID}}}
				if tc.secondSet {
					grant.GrantedPermissionSets = append(grant.GrantedPermissionSets, storage.GrantedPermissionSetEntry{PermissionSetID: secondSetID, IncludedServiceIDs: []id.ServiceID{serviceID}})
				}
			}
			if tc.grantMissing {
				grant = nil
			}
			store := &consentSessionStore{sessions: map[id.ServiceID]*storage.UserSession{serviceID: tc.session}, calls: make(map[id.ServiceID]int), err: tc.sessionError}
			got, err := CheckConsent(context.Background(), consentGrantStore{grant: grant, err: tc.grantError}, store, principal, agent)
			if tc.wantError == nil {
				require.NoError(t, err)
			} else {
				require.True(t, errors.Is(err, tc.wantError), "expected repository cause, got %v", err)
			}
			assert.Equal(t, tc.want, got.Reason)
			assert.Equal(t, tc.calls, store.calls[serviceID])
			assert.Zero(t, store.calls[optionalID])
			if tc.want == ports.ConsentPermissionSetUndeclared {
				assert.Equal(t, permissionSetID, got.PermissionSetID)
			}
			if tc.calls > 0 && tc.want != "" {
				assert.Equal(t, serviceID, got.ServiceID)
			}
		})
	}
}
