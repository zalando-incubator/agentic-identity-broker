package consent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func (r *consentRenewalGrants) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	if r.err != nil {
		return r.err
	}
	scope, owned := ctx.Value(consentRenewalScopeKey{}).(*consentRenewalScope)
	if !owned {
		return r.mockGrantRepo.DeleteByPrincipalAndAgentID(ctx, principal, agentID)
	}
	if agentID != scope.agentID {
		return errors.New("grant deletion crossed its authorization scope")
	}
	grant, err := r.FindByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		return err
	}
	scope.grants[grant.ID] = nil
	return nil
}

func TestService_ConsentRevocationEndsAllMatchingRoots(t *testing.T) {
	for _, action := range []struct {
		name   string
		revoke func(*Service, context.Context, id.Principal, id.AgentID) error
	}{
		{"idempotent revoke", (*Service).RevokeConsent},
		{"principal DELETE", (*Service).RevokeConsentForPrincipal},
		{"empty submission", func(service *Service, ctx context.Context, principal id.Principal, agentID id.AgentID) error {
			grant, err := service.GrantConsent(ctx, &GrantRequest{Principal: principal, AgentID: agentID})
			require.Nil(t, grant, "empty submission must not return a created grant")
			return err
		}},
	} {
		t.Run(action.name, func(t *testing.T) {
			at := time.Date(2034, 6, 10, 12, 0, 0, 0, time.UTC)
			f := newConsentRenewalFixture(at, at.Add(time.Hour))
			first := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			second := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)
			otherGrant := f.grant.Copy()
			otherGrant.ID = id.NewGrantID()
			otherGrant.Principal = id.Principal("unrelated@example.test")
			f.grants.grants[otherGrant.ID] = otherGrant
			other := f.addRoot(t, otherGrant.Principal, f.agentID, otherGrant.ID, false)
			beforeOther := *other
			beforeUpstream := *f.userSessions.sessions[f.userSessionIDForService(t, f.serviceID)]
			require.NoError(t, action.revoke(f.service, context.Background(), f.principal, f.agentID))
			_, err := f.grants.Get(context.Background(), f.grant.ID)
			require.ErrorIs(t, err, ports.ErrNotFound)
			for _, original := range []*storage.RefreshSession{first, second} {
				ended := f.roots.roots[original.ID]
				require.NotNil(t, ended.TerminalReason)
				require.Equal(t, storage.RefreshReasonGrantDeleted, *ended.TerminalReason)
				require.Equal(t, &at, ended.RevokedAt)
				require.Empty(t, ended.RetryCiphertext)
				require.Equal(t, original.OriginalGrantID, ended.OriginalGrantID)
				require.Equal(t, original.StartedAt, ended.StartedAt)
			}
			require.Equal(t, &beforeOther, f.roots.roots[other.ID])
			retained, err := f.grants.Get(context.Background(), otherGrant.ID)
			require.NoError(t, err)
			require.Equal(t, otherGrant, retained)
			require.Equal(t, &beforeUpstream, f.userSessions.sessions[beforeUpstream.ID])
			f.grants.grants[f.grant.ID] = f.grant.Copy()
			current, err := f.service.VerifyAgentAccess(context.Background(), f.principal, f.agentID, at)
			require.NoError(t, err)
			require.Equal(t, f.grant.ID, current.ID)
			require.NotNil(t, f.roots.roots[first.ID].TerminalReason, "recreated grant cannot restore ended authority")
		})
	}
}

func TestService_AbsentGrantRevocationStillEndsLeftoverRoots(t *testing.T) {
	for _, tc := range []struct {
		name   string
		revoke func(*Service, context.Context, id.Principal, id.AgentID) error
		want   error
	}{
		{"idempotent POST", (*Service).RevokeConsent, nil},
		{"DELETE retains not found", (*Service).RevokeConsentForPrincipal, ErrGrantNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Date(2034, 6, 10, 12, 0, 0, 0, time.UTC)
			f := newConsentRenewalFixture(at, at.Add(time.Hour))
			root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			delete(f.grants.grants, f.grant.ID)
			err := tc.revoke(f.service, context.Background(), f.principal, f.agentID)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			ended := f.roots.roots[root.ID]
			require.NotNil(t, ended.TerminalReason)
			require.Equal(t, storage.RefreshReasonGrantDeleted, *ended.TerminalReason)
			require.Empty(t, ended.RetryCiphertext)
			before := *ended
			err = tc.revoke(f.service, context.Background(), f.principal, f.agentID)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			require.Equal(t, &before, f.roots.roots[root.ID], "idempotent repeat preserves original terminal evidence")
		})
	}
}

func TestService_ConsentRevocationFaultsRollBackGrantAndRoots(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fault func(*consentRenewalFixture, error)
	}{
		{"revocation failure", func(f *consentRenewalFixture, err error) { f.roots.fault = err }},
		{"grant deletion failure", func(f *consentRenewalFixture, err error) { f.grants.err = err }},
		{"commit failure before publish", func(f *consentRenewalFixture, err error) { f.coordinator.beforeCommitErr = err }},
		{"clock failure", func(f *consentRenewalFixture, err error) { f.coordinator.clock = testAuthorizationClock{err: err} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Date(2034, 6, 10, 12, 0, 0, 0, time.UTC)
			f := newConsentRenewalFixture(at, at.Add(time.Hour))
			root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			before := *root
			fault := errors.New("lifecycle write failed")
			tc.fault(f, fault)
			require.ErrorIs(t, f.service.RevokeConsent(context.Background(), f.principal, f.agentID), fault)
			f.grants.err = nil
			grant, err := f.grants.Get(context.Background(), f.grant.ID)
			require.NoError(t, err)
			require.Equal(t, f.grant, grant)
			require.Equal(t, &before, f.roots.roots[root.ID])
		})
	}
}
