package consent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type consentRenewalScopeKey struct{}

type consentRenewalScope struct {
	agentID id.AgentID
	grants  map[id.GrantID]*storage.UserGrant
	roots   map[id.RefreshSessionID]*storage.RefreshSession
}

type consentRenewalGrants struct{ *mockGrantRepo }

func (r *consentRenewalGrants) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	if scope, ok := ctx.Value(consentRenewalScopeKey{}).(*consentRenewalScope); ok {
		for grantID, grant := range scope.grants {
			if grant == nil {
				original := r.grants[grantID]
				if original != nil && original.Principal == principal && original.AgentID == agentID {
					return nil, ports.ErrNotFound
				}
				continue
			}
			if grant.Principal == principal && grant.AgentID == agentID {
				return grant.Copy(), nil
			}
		}
	}
	return r.mockGrantRepo.FindByPrincipalAndAgent(ctx, principal, agentID)
}

func (r *consentRenewalGrants) Update(ctx context.Context, grant *storage.UserGrant) error {
	if scope, ok := ctx.Value(consentRenewalScopeKey{}).(*consentRenewalScope); ok {
		if grant.AgentID != scope.agentID {
			return errors.New("grant update crossed its authorization scope")
		}
		scope.grants[grant.ID] = grant.Copy()
		return nil
	}
	return r.mockGrantRepo.Update(ctx, grant)
}

type consentRenewalRoots struct {
	roots  map[id.RefreshSessionID]*storage.RefreshSession
	tokens map[string]*storage.RefreshToken
	fault  error
}

func (r *consentRenewalRoots) revoke(ctx context.Context, at time.Time, reason storage.RefreshRevocationReason, match func(*storage.RefreshSession) bool) error {
	if r.fault != nil {
		return r.fault
	}
	scope, ok := ctx.Value(consentRenewalScopeKey{}).(*consentRenewalScope)
	if !ok {
		return errors.New("native refresh revocation requires the authorization scope")
	}
	for _, root := range r.roots {
		if !match(root) || root.TerminalReason != nil {
			continue
		}
		if root.AgentID != scope.agentID {
			return errors.New("refresh revocation crossed its authorization scope")
		}
		changed := *root
		changed.RevokedAt = &at
		changed.TerminalReason = &reason
		changed.RetryCiphertext = nil
		if err := changed.ValidateTransition(root, at); err != nil {
			return err
		}
		scope.roots[root.ID] = &changed
	}
	return nil
}

func (r *consentRenewalRoots) RevokeByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	return r.revoke(ctx, at, reason, func(root *storage.RefreshSession) bool {
		return root.Principal == principal && root.AgentID == agentID
	})
}

func (r *consentRenewalRoots) RevokeByAgent(ctx context.Context, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	return r.revoke(ctx, at, reason, func(root *storage.RefreshSession) bool { return root.AgentID == agentID })
}

func (r *consentRenewalRoots) RevokeByID(ctx context.Context, sessionID id.RefreshSessionID, at time.Time, reason storage.RefreshRevocationReason) error {
	return r.revoke(ctx, at, reason, func(root *storage.RefreshSession) bool { return root.ID == sessionID })
}

type consentRenewalCoordinator struct {
	clock           ports.AuthorizationClock
	grants          *consentRenewalGrants
	roots           *consentRenewalRoots
	beforeCommitErr error
}

func (c *consentRenewalCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	at, err := c.clock.Now(ctx)
	if err != nil {
		return err
	}
	scope := &consentRenewalScope{
		agentID: agentID,
		grants:  make(map[id.GrantID]*storage.UserGrant),
		roots:   make(map[id.RefreshSessionID]*storage.RefreshSession),
	}
	if err := operation(context.WithValue(ctx, consentRenewalScopeKey{}, scope), at); err != nil {
		return err
	}
	if c.beforeCommitErr != nil {
		return c.beforeCommitErr
	}
	for grantID, grant := range scope.grants {
		if grant == nil {
			delete(c.grants.grants, grantID)
		} else {
			c.grants.grants[grantID] = grant.Copy()
		}
	}
	for sessionID, root := range scope.roots {
		c.roots.roots[sessionID] = root
	}
	return nil
}

type consentRenewalFixture struct {
	service      *Service
	grants       *consentRenewalGrants
	roots        *consentRenewalRoots
	coordinator  *consentRenewalCoordinator
	userSessions *mockUserSessionRepo
	grant        *storage.UserGrant
	request      *GrantRequest
	principal    id.Principal
	agentID      id.AgentID
	serviceID    id.ServiceID
	otherService id.ServiceID
	decisionTime time.Time
}

func newConsentRenewalFixture(decisionTime, oldUntil time.Time) *consentRenewalFixture {
	principal := id.Principal("renewal-owner@example.test")
	agentID, serviceID, otherService := id.NewAgentID(), id.NewServiceID(), id.NewServiceID()
	psID, grantID := id.NewPermissionSetID(), id.NewGrantID()
	created := decisionTime.Add(-48 * time.Hour)
	initial := &storage.UserGrant{
		ID: grantID, Principal: principal, AgentID: agentID, ValidUntil: &oldUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID}}},
		CreatedAt:             created, UpdatedAt: created.Add(time.Hour),
	}
	grants := &consentRenewalGrants{mockGrantRepo: &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{grantID: initial.Copy()}}}
	roots := &consentRenewalRoots{roots: make(map[id.RefreshSessionID]*storage.RefreshSession), tokens: make(map[string]*storage.RefreshToken)}
	clock := testAuthorizationClock{now: decisionTime}
	coordinator := &consentRenewalCoordinator{clock: clock, grants: grants, roots: roots}
	connectedUntil := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	sessions := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{}}
	for _, svcID := range []id.ServiceID{serviceID, otherService} {
		sessionID := id.NewSessionID()
		sessions.sessions[sessionID] = &storage.UserSession{
			ID: sessionID, Principal: principal, ServiceID: svcID, RefreshTokenExpiresAt: &connectedUntil,
		}
	}
	agent := &storage.Agent{ID: agentID, DisplayName: "Consent renewal agent", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeOptional}}}
	ps := &storage.PermissionSet{ID: psID, Name: "Consent renewal", ServiceScopes: []storage.ServiceScope{
		{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional},
		{ServiceID: otherService, RequirementType: storage.RequirementTypeOptional},
	}}
	service := NewService(&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}}, nil, grants, sessions,
		&mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{psID: ps}}, slog.Default(), clock, coordinator, roots)
	newUntil := decisionTime.Add(48 * time.Hour)
	return &consentRenewalFixture{
		service: service, grants: grants, roots: roots, coordinator: coordinator, userSessions: sessions,
		grant: initial, principal: principal, agentID: agentID, serviceID: serviceID, otherService: otherService,
		decisionTime: decisionTime,
		request: &GrantRequest{Principal: principal, AgentID: agentID, ValidUntil: &newUntil,
			GrantedPermissionSets: initial.Copy().GrantedPermissionSets},
	}
}

func consentRenewalSignature(label string) string {
	digest := sha256.Sum256([]byte(label))
	return hex.EncodeToString(digest[:])
}

func (f *consentRenewalFixture) addRoot(t *testing.T, principal id.Principal, agentID id.AgentID, grantID id.GrantID, withPredecessor bool) *storage.RefreshSession {
	t.Helper()
	rootID := id.NewRefreshSessionID()
	started := f.decisionTime.Add(-2 * time.Hour)
	originalSignature := consentRenewalSignature("original-" + rootID.String())
	currentSignature := originalSignature
	lastFresh := started
	if withPredecessor {
		lastFresh = f.decisionTime.Add(-15 * time.Second)
		currentSignature = consentRenewalSignature("current-" + rootID.String())
	}
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: grantID, OriginalTokenSignature: originalSignature,
		AgentID: agentID, Principal: principal, ClientID: id.ClientID("renewal-client"), Scope: "read",
		StartedAt: started, LastFreshAt: lastFresh, InactivityExpiresAt: f.decisionTime.Add(24 * time.Hour),
		RetainUntil: f.decisionTime.Add(48 * time.Hour), BranchKeyID: "refresh_" + rootID.String() + "_branch_key",
		CurrentSignature: currentSignature,
	}
	originalToken := &storage.RefreshToken{Signature: originalSignature, SessionID: rootID, IssuedAt: started, ExpiresAt: f.decisionTime.Add(24 * time.Hour)}
	if withPredecessor {
		deadline := f.decisionTime.Add(15 * time.Second)
		accessExpiry := f.decisionTime.Add(time.Hour)
		requestScope := "read"
		fingerprint := consentRenewalSignature("request-" + rootID.String())
		root.PreviousSignature = &originalSignature
		root.PreviousConsumedAt = &lastFresh
		root.ReuseUntil = &deadline
		root.OriginalRequestedScope = &requestScope
		root.OriginalRequestContextFingerprint = &fingerprint
		root.RetryAccessExpiresAt = &accessExpiry
		root.RetryExpiresAt = &deadline
		root.RetryCiphertext = []byte("sealed-result")
		originalToken.UsedAt = &lastFresh
		currentToken := &storage.RefreshToken{Signature: currentSignature, SessionID: rootID, IssuedAt: lastFresh, ExpiresAt: originalToken.ExpiresAt}
		require.NoError(t, currentToken.Validate())
		f.roots.tokens[currentSignature] = currentToken
	}
	require.NoError(t, root.Validate())
	require.NoError(t, originalToken.Validate())
	f.roots.roots[rootID] = root
	f.roots.tokens[originalSignature] = originalToken
	return root
}

func TestService_GrantConsent_ExpiredRenewalInvalidatesOnlyPreviousPrincipalAgentRoots(t *testing.T) {
	decisionTime := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		oldUntil time.Time
	}{
		{name: "deadline passed", oldUntil: decisionTime.Add(-time.Minute)},
		{name: "deadline equality", oldUntil: decisionTime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newConsentRenewalFixture(decisionTime, tc.oldUntil)
			ctx := context.Background()
			old, err := f.service.VerifyAgentAccess(ctx, f.principal, f.agentID, decisionTime)
			require.ErrorIs(t, err, ErrGrantExpired)
			require.Nil(t, old)

			first := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			second := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)
			otherPrincipalGrant := f.grant.Copy()
			otherPrincipalGrant.ID = id.NewGrantID()
			otherPrincipalGrant.Principal = id.Principal("other@example.test")
			otherPrincipalGrant.ValidUntil = f.request.ValidUntil
			f.grants.grants[otherPrincipalGrant.ID] = otherPrincipalGrant
			otherAgentGrant := f.grant.Copy()
			otherAgentGrant.ID = id.NewGrantID()
			otherAgentGrant.AgentID = id.NewAgentID()
			otherAgentGrant.ValidUntil = f.request.ValidUntil
			f.grants.grants[otherAgentGrant.ID] = otherAgentGrant
			otherPrincipal := f.addRoot(t, otherPrincipalGrant.Principal, f.agentID, otherPrincipalGrant.ID, false)
			otherAgent := f.addRoot(t, f.principal, otherAgentGrant.AgentID, otherAgentGrant.ID, false)
			beforeOtherPrincipal := *otherPrincipal
			beforeOtherAgent := *otherAgent

			updated, err := f.service.GrantConsent(ctx, f.request)
			require.NoError(t, err)
			require.NotNil(t, updated)
			persisted, err := f.grants.Get(ctx, f.grant.ID)
			require.NoError(t, err)
			assert.Equal(t, f.grant.ID, updated.ID, "renewal must not rely on changing the grant ID to end older roots")
			assert.Equal(t, f.request.ValidUntil, persisted.ValidUntil)
			assert.Equal(t, updated, persisted)
			verified, err := f.service.VerifyAgentAccess(ctx, f.principal, f.agentID, decisionTime)
			require.NoError(t, err)
			assert.Equal(t, persisted.ID, verified.ID, "the renewed grant remains available for fresh authorization")

			for _, previous := range []*storage.RefreshSession{first, second} {
				root := f.roots.roots[previous.ID]
				require.NotNil(t, root)
				require.NotNil(t, root.TerminalReason)
				assert.Equal(t, storage.RefreshReasonExpiredGrantRenewal, *root.TerminalReason)
				require.NotNil(t, root.RevokedAt)
				assert.Equal(t, decisionTime, *root.RevokedAt)
				assert.Empty(t, root.RetryCiphertext)
				assert.Equal(t, previous.OriginalGrantID, root.OriginalGrantID)
				assert.Equal(t, previous.StartedAt, root.StartedAt)
				assert.Equal(t, previous.CurrentSignature, root.CurrentSignature)
			}
			assert.Equal(t, &beforeOtherPrincipal, f.roots.roots[otherPrincipal.ID])
			assert.Equal(t, &beforeOtherAgent, f.roots.roots[otherAgent.ID])
			assert.Nil(t, f.roots.tokens[otherPrincipal.CurrentSignature].UsedAt)
			assert.Nil(t, f.roots.tokens[otherAgent.CurrentSignature].UsedAt)
			verifiedOtherPrincipal, err := f.service.VerifyAgentAccess(ctx, otherPrincipalGrant.Principal, f.agentID, decisionTime)
			require.NoError(t, err)
			assert.Equal(t, otherPrincipalGrant.ID, verifiedOtherPrincipal.ID)
			verifiedOtherAgent, err := f.service.VerifyAgentAccess(ctx, f.principal, otherAgentGrant.AgentID, decisionTime)
			require.NoError(t, err)
			assert.Equal(t, otherAgentGrant.ID, verifiedOtherAgent.ID)
		})
	}
}

func TestService_GrantConsent_ActiveEditAndExtensionPreserveRootOrigin(t *testing.T) {
	decisionTime := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name            string
		editPermissions bool
	}{
		{name: "extend active grant"},
		{name: "edit active permission set", editPermissions: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newConsentRenewalFixture(decisionTime, decisionTime.Add(time.Nanosecond))
			if tc.editPermissions {
				oldUntil := *f.grant.ValidUntil
				f.request.ValidUntil = &oldUntil
				f.request.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{f.serviceID, f.otherService}
			}
			root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			before := *root
			beforeCurrent := *f.roots.tokens[root.CurrentSignature]
			ctx := context.Background()

			updated, err := f.service.GrantConsent(ctx, f.request)
			require.NoError(t, err)
			require.NotNil(t, updated)
			persisted, err := f.grants.Get(ctx, f.grant.ID)
			require.NoError(t, err)
			assert.Equal(t, f.grant.ID, persisted.ID)
			assert.Equal(t, f.grant.CreatedAt, persisted.CreatedAt)
			assert.Equal(t, f.request.ValidUntil, persisted.ValidUntil)
			assert.Equal(t, f.request.GrantedPermissionSets, persisted.GrantedPermissionSets)
			verified, err := f.service.VerifyAgentAccess(ctx, f.principal, f.agentID, decisionTime)
			require.NoError(t, err)
			assert.Equal(t, root.OriginalGrantID, verified.ID)
			assert.Equal(t, &before, f.roots.roots[root.ID], "an active edit must not reset original issuance or activity clocks")
			assert.Equal(t, &beforeCurrent, f.roots.tokens[root.CurrentSignature])
		})
	}
}

func TestService_GrantConsent_UsesSharedTimeForRenewalAndNewDeadline(t *testing.T) {
	// The decision is intentionally before wall time. A renewal valid at the shared
	// decision time must not be rejected by a node-local time.Now call.
	decisionTime := time.Date(2020, time.June, 10, 12, 0, 0, 0, time.UTC)
	f := newConsentRenewalFixture(decisionTime, decisionTime.Add(-time.Minute))
	root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)

	updated, err := f.service.GrantConsent(context.Background(), f.request)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, f.request.ValidUntil, updated.ValidUntil)
	persisted, err := f.grants.Get(context.Background(), f.grant.ID)
	require.NoError(t, err)
	assert.Equal(t, updated, persisted)
	renewedRoot := f.roots.roots[root.ID]
	require.NotNil(t, renewedRoot)
	require.NotNil(t, renewedRoot.RevokedAt)
	assert.Equal(t, decisionTime, *renewedRoot.RevokedAt)
}

func TestService_GrantConsent_ActiveAtSharedTimeDoesNotRevoke(t *testing.T) {
	// The stored deadline is past wall time but still ahead of the coordinator's
	// decision. Extending it is an active edit, not an expired-grant renewal.
	decisionTime := time.Date(2020, time.June, 10, 12, 0, 0, 0, time.UTC)
	f := newConsentRenewalFixture(decisionTime, decisionTime.Add(time.Minute))
	root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)
	before := *root

	updated, err := f.service.GrantConsent(context.Background(), f.request)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, f.grant.ID, updated.ID)
	assert.Equal(t, f.request.ValidUntil, updated.ValidUntil)
	persisted, err := f.grants.Get(context.Background(), f.grant.ID)
	require.NoError(t, err)
	assert.Equal(t, updated, persisted)
	assert.Equal(t, &before, f.roots.roots[root.ID])
}

func TestService_GrantConsent_DeadlineAtSharedTimeDoesNotReplaceGrant(t *testing.T) {
	decisionTime := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
	f := newConsentRenewalFixture(decisionTime, decisionTime.Add(time.Hour))
	root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)
	beforeRoot := *root
	f.request.ValidUntil = &decisionTime

	updated, err := f.service.GrantConsent(context.Background(), f.request)
	require.ErrorIs(t, err, ErrGrantValidation)
	assert.Nil(t, updated)
	persisted, err := f.grants.Get(context.Background(), f.grant.ID)
	require.NoError(t, err)
	assert.Equal(t, f.grant, persisted)
	assert.Equal(t, &beforeRoot, f.roots.roots[root.ID])
}

func TestService_GrantConsent_RenewalFaultsLeaveGrantAndRootsUnchanged(t *testing.T) {
	decisionTime := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		fault func(*consentRenewalFixture, error)
	}{
		{name: "revocation fails", fault: func(f *consentRenewalFixture, err error) { f.roots.fault = err }},
		{name: "commit fails before publishing", fault: func(f *consentRenewalFixture, err error) { f.coordinator.beforeCommitErr = err }},
		{name: "shared clock fails", fault: func(f *consentRenewalFixture, err error) {
			f.service.clock = testAuthorizationClock{err: err}
			f.coordinator.clock = f.service.clock
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newConsentRenewalFixture(decisionTime, decisionTime.Add(-time.Minute))
			root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			beforeRoot := *root
			beforeToken := *f.roots.tokens[root.CurrentSignature]
			beforeSession := *f.userSessions.sessions[f.userSessionIDForService(t, f.serviceID)]
			fault := errors.New("authorization operation failed")
			tc.fault(f, fault)

			updated, err := f.service.GrantConsent(context.Background(), f.request)
			require.ErrorIs(t, err, fault)
			assert.Nil(t, updated)
			persisted, err := f.grants.Get(context.Background(), f.grant.ID)
			require.NoError(t, err)
			assert.Equal(t, f.grant, persisted)
			assert.Equal(t, &beforeRoot, f.roots.roots[root.ID])
			assert.Equal(t, &beforeToken, f.roots.tokens[root.CurrentSignature])
			assert.Equal(t, &beforeSession, f.userSessions.sessions[beforeSession.ID])
		})
	}
}

func (f *consentRenewalFixture) userSessionIDForService(t *testing.T, serviceID id.ServiceID) id.SessionID {
	t.Helper()
	for sessionID, session := range f.userSessions.sessions {
		if session.ServiceID == serviceID {
			return sessionID
		}
	}
	t.Fatalf("missing connected service %s", serviceID)
	return id.SessionID{}
}
