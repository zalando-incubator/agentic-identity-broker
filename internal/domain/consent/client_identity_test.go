package consent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionUsabilityEvaluatorFunc func(context.Context, []*storage.UserSession, ports.UsabilityMode) (map[id.ServiceID]bool, error)

func (f sessionUsabilityEvaluatorFunc) RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
	return f(ctx, sessions, mode)
}

type consentIdentityFixture struct {
	service  *Service
	agent    *storage.Agent
	grant    *storage.UserGrant
	grants   *mockGrantRepo
	sessions *mockUserSessionRepo
	reports  *storage.UserSession
	calendar *storage.UserSession
}

func newConsentIdentityFixture() *consentIdentityFixture {
	return newConsentIdentityFixtureWithLogger(slog.New(slog.DiscardHandler))
}

func newConsentIdentityFixtureWithLogger(logger *slog.Logger) *consentIdentityFixture {
	principal := id.Principal("alice@example.com")
	reportsID, calendarID, psID := id.NewServiceID(), id.NewServiceID(), id.NewPermissionSetID()
	agent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "Reports Reader", Description: "Read reports with optional calendar access",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeMandatory}},
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: reportsID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"reports:read"}},
			{ServiceID: calendarID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"calendar:read"}},
		},
	}
	permissionSet := &storage.PermissionSet{
		ID: psID, Name: "Reader", Description: "Read reports and optionally calendar",
		ServiceScopes: []storage.ServiceScope{
			{ServiceID: reportsID, Scopes: []string{"reports:read"}, RequirementType: storage.RequirementTypeMandatory},
			{ServiceID: calendarID, Scopes: []string{"calendar:read"}, RequirementType: storage.RequirementTypeOptional},
		},
	}
	expiresAt := time.Now().Add(time.Hour)
	reports := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: reportsID, TokenType: "Bearer", Scope: []string{"reports:read"},
		AccessTokenExpiresAt: &expiresAt, UpstreamClientID: ptr.To(id.ClientID("reports-client-v1")), UpstreamClientRevision: 1,
		EncryptedAccessToken: []byte("encrypted-reports-access"), EncryptedRefreshToken: []byte("encrypted-reports-refresh"),
	}
	calendar := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: calendarID, TokenType: "Bearer", Scope: []string{"calendar:read"},
		AccessTokenExpiresAt: &expiresAt, UpstreamClientID: ptr.To(id.ClientID("calendar-client")), UpstreamClientRevision: 0,
	}
	sessions := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{reports.ID: reports, calendar.ID: calendar}}
	validUntil := time.Now().Add(30 * time.Minute)
	grant := &storage.UserGrant{
		ID: id.NewGrantID(), Principal: principal, AgentID: agent.ID, ValidUntil: &validUntil,
		CreatedAt: time.Now().Add(-24 * time.Hour), UpdatedAt: time.Now().Add(-2 * time.Hour),
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{reportsID}}},
	}
	grants := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{grant.ID: grant.Copy()}}
	providers := &mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{
		reportsID:  {ID: reportsID, DisplayName: "Reports Production", Scopes: []model.OAuthScope{{ScopeValue: "reports:read", Description: "Read reports"}}},
		calendarID: {ID: calendarID, DisplayName: "Calendar Production", Scopes: []model.OAuthScope{{ScopeValue: "calendar:read", Description: "Read calendar"}}},
	}}
	service := NewService(
		&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agent.ID: agent}}, newTestProviderService(providers), grants, sessions,
		&mockConsentRefreshRepo{}, &mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{psID: permissionSet}},
		logger,
	)
	return &consentIdentityFixture{service: service, agent: agent, grant: grant, grants: grants, sessions: sessions, reports: reports, calendar: calendar}
}

func TestService_GrantConsent_ClientIdentityUsability(t *testing.T) {
	cases := []struct {
		name             string
		unusableReports  bool
		unusableCalendar bool
		includeCalendar  bool
		expiredAccess    bool
		rejected         bool
		missingReports   bool
		gateError        bool
		wantDenied       bool
	}{
		{name: "mandatory superseded session denies unchanged grant replay", unusableReports: true, wantDenied: true},
		{name: "refreshable access expiry does not rescue superseded identity", unusableReports: true, expiredAccess: true, wantDenied: true},
		{name: "mandatory missing session still denies submission", missingReports: true, wantDenied: true},
		{name: "included optional superseded session also denies submission", unusableCalendar: true, includeCalendar: true, wantDenied: true},
		{name: "excluded optional superseded session does not block submission", unusableCalendar: true},
		{name: "usable session preserves existing selections and expiry"},
		{name: "expired access with usable identity remains refreshable", expiredAccess: true},
		{name: "observed rejection still denies despite valid access", rejected: true, wantDenied: true},
		{name: "Gate read error cannot fall back to apparently usable session", gateError: true, wantDenied: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var logs bytes.Buffer
			f := newConsentIdentityFixtureWithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
			if tc.expiredAccess {
				f.reports.AccessTokenExpiresAt = ptr.To(time.Now().Add(-time.Hour))
			}
			if tc.rejected {
				f.reports.RefreshRejectedAt = ptr.To(time.Now().Add(-time.Minute))
			}
			if tc.missingReports {
				delete(f.sessions.sessions, f.reports.ID)
			}
			beforeGrant := f.grant.Copy()
			req := &GrantRequest{
				Principal: f.grant.Principal, AgentID: f.agent.ID, ValidUntil: ptr.To(*f.grant.ValidUntil),
				GrantedPermissionSets: f.grant.Copy().GrantedPermissionSets,
			}
			if tc.includeCalendar {
				req.GrantedPermissionSets[0].IncludedServiceIDs = append(req.GrantedPermissionSets[0].IncludedServiceIDs, f.calendar.ServiceID)
			}
			seen := make(map[id.ServiceID]bool)
			sourceErr := errors.New("identity state unavailable")
			f.service.WithSessionUsabilityEvaluator(sessionUsabilityEvaluatorFunc(func(_ context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
				assert.Equal(t, ports.UsabilityGate, mode)
				result := make(map[id.ServiceID]bool, len(sessions))
				for _, session := range sessions {
					assert.Equal(t, req.Principal, session.Principal)
					require.Contains(t, req.GrantedPermissionSets[0].IncludedServiceIDs, session.ServiceID, "excluded optional sessions must not trigger Gate identity reads")
					seen[session.ServiceID] = true
					result[session.ServiceID] = session.RequiresReauthentication() ||
						(session.ServiceID == f.reports.ServiceID && tc.unusableReports) ||
						(session.ServiceID == f.calendar.ServiceID && tc.unusableCalendar)
				}
				if tc.gateError {
					return result, sourceErr
				}
				return result, nil
			}))

			grant, err := f.service.GrantConsent(ctx, req)
			if tc.wantDenied {
				if tc.gateError {
					require.ErrorIs(t, err, sourceErr)
					assert.NotErrorIs(t, err, ErrUnconnectedServices, "source failure is not confirmed reauthentication")
				} else {
					require.ErrorIs(t, err, ErrUnconnectedServices)
				}
				assert.Nil(t, grant)
			} else {
				require.NoError(t, err)
				require.NotNil(t, grant)
				assert.Equal(t, beforeGrant, grant, "reconnection must not broaden selections or extend validity")
			}
			if !tc.missingReports && !tc.rejected {
				assert.True(t, seen[f.reports.ServiceID], "mandatory sessions must reach the Gate evaluator even for unchanged grant submissions")
			}
			if tc.includeCalendar {
				assert.True(t, seen[f.calendar.ServiceID], "included optional sessions participate in validation")
			}
			assert.NotContains(t, logs.String(), "session.oauth2.credential_source_failed", "an ordinary unusable verdict is not a source failure")
			stored, err := f.grants.Get(ctx, f.grant.ID)
			require.NoError(t, err)
			assert.Equal(t, beforeGrant, stored)
			assert.Zero(t, f.grants.createCalls)
			assert.Zero(t, f.grants.updateCalls, "denied submissions and identical valid selections must never rewrite grants")
			assert.Equal(t, []string{"reports:read"}, f.reports.Scope)
			assert.Equal(t, int64(1), f.reports.UpstreamClientRevision)
			assert.Equal(t, id.ClientID("reports-client-v1"), *f.reports.UpstreamClientID)
			assert.Equal(t, []byte("encrypted-reports-access"), f.reports.EncryptedAccessToken)
		})
	}
}

func TestService_GetAgentConsentDetail_ClientIdentityDisplay(t *testing.T) {
	cases := []struct {
		name              string
		unusableReports   bool
		expiredAccess     bool
		rejected          bool
		missingReports    bool
		sourceUnavailable bool
		wantConnected     bool
	}{
		{name: "superseded identity is not_connected despite unexpired access", unusableReports: true},
		{name: "superseded identity is not_connected despite refresh token", unusableReports: true, expiredAccess: true},
		{name: "usable identity remains connected", wantConnected: true},
		{name: "expired access with usable refresh token remains connected", expiredAccess: true, wantConnected: true},
		{name: "observed rejection remains not_connected", rejected: true},
		{name: "missing session remains not_connected", missingReports: true},
		{name: "Display accepts evaluator persisted usable fallback on source failure", sourceUnavailable: true, wantConnected: true},
		{name: "Display accepts evaluator persisted superseded fallback on source failure", sourceUnavailable: true, unusableReports: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newConsentIdentityFixture()
			if tc.expiredAccess {
				f.reports.AccessTokenExpiresAt = ptr.To(time.Now().Add(-time.Hour))
			}
			if tc.rejected {
				f.reports.RefreshRejectedAt = ptr.To(time.Now().Add(-time.Minute))
			}
			if tc.missingReports {
				delete(f.sessions.sessions, f.reports.ID)
			}
			beforeGrant := f.grant.Copy()
			seen := make(map[id.ServiceID]bool)
			f.service.WithSessionUsabilityEvaluator(sessionUsabilityEvaluatorFunc(func(_ context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
				assert.Equal(t, ports.UsabilityDisplay, mode)
				if tc.sourceUnavailable && mode != ports.UsabilityDisplay {
					return nil, errors.New("live client identity unavailable")
				}
				// Persisted fallback is the evaluator's contract; consent consumes its verdict.
				result := make(map[id.ServiceID]bool, len(sessions))
				for _, session := range sessions {
					assert.Equal(t, f.grant.Principal, session.Principal)
					seen[session.ServiceID] = true
					if session.ServiceID == f.reports.ServiceID {
						require.NotNil(t, session.UpstreamClientID, "Display evaluation needs establishment metadata, not only service IDs")
						assert.Equal(t, id.ClientID("reports-client-v1"), *session.UpstreamClientID)
						assert.Equal(t, int64(1), session.UpstreamClientRevision)
					}
					result[session.ServiceID] = session.RequiresReauthentication() || (session.ServiceID == f.reports.ServiceID && tc.unusableReports)
				}
				return result, nil
			}))

			detail, err := f.service.GetAgentConsentDetail(ctx, f.agent.ID, f.grant.Principal)
			require.NoError(t, err, "Display fallback must not become a Gate error")
			require.NotNil(t, detail)
			require.Len(t, detail.ServiceRequirements, 2)
			connected := make(map[id.ServiceID]bool)
			for _, requirement := range detail.ServiceRequirements {
				connected[requirement.ServiceID] = requirement.IsConnected
			}
			assert.Equal(t, tc.wantConnected, connected[f.reports.ServiceID], "IsConnected maps to the existing connected/not_connected API enum")
			assert.True(t, connected[f.calendar.ServiceID])
			wantActive := []id.ServiceID{f.calendar.ServiceID}
			if tc.wantConnected {
				wantActive = append(wantActive, f.reports.ServiceID)
			}
			assert.ElementsMatch(t, wantActive, detail.ActiveSessionServiceIDs, "unusable sessions must not be selectable as connected")
			assert.True(t, seen[f.calendar.ServiceID], "consent detail must use the evaluator rather than ID-only presence")
			if !tc.missingReports && !tc.rejected {
				assert.True(t, seen[f.reports.ServiceID], "identity establishment metadata must reach Display evaluation")
			}
			assert.Equal(t, f.agent, detail.Agent)
			require.Len(t, detail.ResolvedPermissionSets, 1)
			assert.Equal(t, f.grant.GrantedPermissionSets[0].PermissionSetID, detail.ResolvedPermissionSets[0].PermissionSet.ID)
			assert.Equal(t, []ServiceScopeInfo{{Name: "reports:read", Description: "Read reports"}}, detail.ServiceRequirements[0].RequiredScopes)
			stored, err := f.grants.Get(ctx, f.grant.ID)
			require.NoError(t, err)
			assert.Equal(t, beforeGrant, stored)
			assert.Zero(t, f.grants.createCalls)
			assert.Zero(t, f.grants.updateCalls)
			assert.Equal(t, []byte("encrypted-reports-access"), f.reports.EncryptedAccessToken)
			assert.Equal(t, int64(1), f.reports.UpstreamClientRevision)
		})
	}
}

func TestService_GetAgentConsentDetail_EvaluatesOnlyRelevantMetadata(t *testing.T) {
	f := newConsentIdentityFixture()
	f.agent.ServiceRequirements = f.agent.ServiceRequirements[:1]
	unrelated := &storage.UserSession{ID: id.NewSessionID(), Principal: f.grant.Principal, ServiceID: id.NewServiceID()}
	f.sessions.sessions[unrelated.ID] = unrelated
	var evaluated []id.ServiceID
	f.service.WithSessionUsabilityEvaluator(sessionUsabilityEvaluatorFunc(func(_ context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
		assert.Equal(t, ports.UsabilityDisplay, mode)
		for _, session := range sessions {
			evaluated = append(evaluated, session.ServiceID)
			assert.Empty(t, session.EncryptedAccessToken)
			assert.Empty(t, session.EncryptedRefreshToken)
		}
		return map[id.ServiceID]bool{}, nil
	}))
	detail, err := f.service.GetAgentConsentDetail(context.Background(), f.agent.ID, f.grant.Principal)
	require.NoError(t, err)
	want := []id.ServiceID{f.reports.ServiceID, f.calendar.ServiceID}
	assert.ElementsMatch(t, want, evaluated, "permission-set-only services remain relevant, unrelated connections do not")
	assert.ElementsMatch(t, want, detail.ActiveSessionServiceIDs)
}

func TestService_ClientIdentityReconnect_PreservesGrantSelectionsAndExpiry(t *testing.T) {
	for _, expiredGrant := range []bool{false, true} {
		name := "active delegation"
		if expiredGrant {
			name = "expired delegation"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f := newConsentIdentityFixture()
			if expiredGrant {
				f.grant.ValidUntil = ptr.To(time.Now().Add(-time.Hour))
				f.grants.grants[f.grant.ID] = f.grant.Copy()
			}
			beforeGrant := f.grant.Copy()
			unusable := true
			f.service.WithSessionUsabilityEvaluator(sessionUsabilityEvaluatorFunc(func(_ context.Context, sessions []*storage.UserSession, _ ports.UsabilityMode) (map[id.ServiceID]bool, error) {
				result := make(map[id.ServiceID]bool, len(sessions))
				for _, session := range sessions {
					result[session.ServiceID] = session.ServiceID == f.reports.ServiceID && unusable
				}
				return result, nil
			}))
			detail, err := f.service.GetAgentConsentDetail(ctx, f.agent.ID, f.grant.Principal)
			require.NoError(t, err)
			require.NotEmpty(t, detail.ServiceRequirements)
			assert.False(t, detail.ServiceRequirements[0].IsConnected)

			// The repository now supplies the winning callback's current-identity session.
			f.reports.UpstreamClientID = ptr.To(id.ClientID("reports-client-v2"))
			f.reports.UpstreamClientRevision = 2
			unusable = false
			detail, err = f.service.GetAgentConsentDetail(ctx, f.agent.ID, f.grant.Principal)
			require.NoError(t, err)
			assert.True(t, detail.ServiceRequirements[0].IsConnected)
			grant, err := f.service.VerifyAgentAccess(ctx, f.grant.Principal, f.agent.ID)
			if expiredGrant {
				require.ErrorIs(t, err, ErrGrantExpired, "a successful provider reconnect cannot renew delegation")
			} else {
				require.NoError(t, err)
				grant, err = f.service.GrantConsent(ctx, &GrantRequest{
					Principal: f.grant.Principal, AgentID: f.agent.ID, ValidUntil: beforeGrant.ValidUntil,
					GrantedPermissionSets: beforeGrant.Copy().GrantedPermissionSets,
				})
				require.NoError(t, err)
			}
			assert.Equal(t, beforeGrant, grant)
			stored, err := f.grants.Get(ctx, f.grant.ID)
			require.NoError(t, err)
			assert.Equal(t, beforeGrant, stored)
			assert.Zero(t, f.grants.createCalls)
			assert.Zero(t, f.grants.updateCalls)
		})
	}
}

func TestService_RequiresExplicitSessionUsabilityEvaluator(t *testing.T) {
	f := newConsentIdentityFixture()
	grant, err := f.service.GrantConsent(context.Background(), &GrantRequest{
		Principal: f.grant.Principal, AgentID: f.agent.ID, ValidUntil: f.grant.ValidUntil,
		GrantedPermissionSets: f.grant.GrantedPermissionSets,
	})
	assert.Nil(t, grant)
	var failure *ports.SessionUsabilityError
	require.ErrorAs(t, err, &failure)
	detail, err := f.service.GetAgentConsentDetail(context.Background(), f.agent.ID, f.grant.Principal)
	assert.Nil(t, detail)
	require.ErrorAs(t, err, &failure)
	assert.Zero(t, f.grants.updateCalls)
}
