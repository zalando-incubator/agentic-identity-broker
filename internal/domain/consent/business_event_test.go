package consent

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

type ledgerGrantExpirations struct {
	repo    *mockGrantRepo
	markers map[id.GrantID]time.Time
}

func (e *ledgerGrantExpirations) ListUnrecordedExpiredForPrincipal(_ context.Context, principal id.Principal, at time.Time, limit int) ([]*storage.UserGrant, error) {
	var candidates []*storage.UserGrant
	for _, grant := range e.repo.grants {
		if grant.Principal == principal && grant.ValidUntil != nil && !grant.ValidUntil.After(at) && !e.markers[grant.ID].Equal(*grant.ValidUntil) {
			candidates = append(candidates, grant.Copy())
			if len(candidates) == limit {
				break
			}
		}
	}
	return candidates, nil
}
func (e *ledgerGrantExpirations) RecordExpiration(_ context.Context, key id.GrantID, expiry time.Time) (bool, error) {
	grant := e.repo.grants[key]
	if grant == nil || grant.ValidUntil == nil || !grant.ValidUntil.Equal(expiry) || e.markers[key].Equal(expiry) {
		return false, nil
	}
	e.markers[key] = expiry
	return true, nil
}

func newLedgerConsent(t *testing.T) (*Service, *mockGrantRepo, *ledgerfixture.Store, *GrantRequest, *ledgerGrantExpirations) {
	t.Helper()
	principal, agentID, serviceID, psID := id.Principal("ledger-user"), id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
	agents := &mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: {ID: agentID, DisplayName: "Agent", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeOptional}}}}}
	sets := &mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{psID: {ID: psID, Name: "read", ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}}}}}
	sessionID := id.NewSessionID()
	sessions := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{sessionID: {ID: sessionID, Principal: principal, ServiceID: serviceID}}}
	repo := &mockGrantRepo{grants: make(map[id.GrantID]*storage.UserGrant)}
	expirations := &ledgerGrantExpirations{repo: repo, markers: make(map[id.GrantID]time.Time)}
	store := &ledgerfixture.Store{Snapshot: func() func() {
		grants := make(map[id.GrantID]*storage.UserGrant, len(repo.grants))
		for key, grant := range repo.grants {
			grants[key] = grant.Copy()
		}
		markers := make(map[id.GrantID]time.Time, len(expirations.markers))
		for key, at := range expirations.markers {
			markers[key] = at
		}
		return func() { repo.grants, expirations.markers = grants, markers }
	}}
	svc := NewService(agents, newTestProviderService(&mockServiceRepo{services: make(map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity)}), repo, sessions, sets, slog.Default(), store.Recorder(t))
	svc.expirations = expirations
	request := &GrantRequest{Principal: principal, AgentID: agentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID}}}}
	return svc, repo, store, request, expirations
}

func TestConsentLedgerChangedStateAndIdempotentRevocation(t *testing.T) {
	svc, _, store, request, _ := newLedgerConsent(t)
	ctx := context.Background()
	grant, err := svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	unchanged, err := svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	require.Equal(t, grant.ID, unchanged.ID)
	expiry := time.Now().UTC().Add(time.Hour)
	request.ValidUntil = &expiry
	_, err = svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	require.NoError(t, svc.RevokeConsentForPrincipal(ctx, request.Principal, request.AgentID))
	require.ErrorIs(t, svc.RevokeConsentForPrincipal(ctx, request.Principal, request.AgentID), ErrGrantNotFound)
	require.NoError(t, svc.RevokeConsent(ctx, request.Principal, request.AgentID))
	require.Len(t, store.Events, 3, "equal state and absent revocations must not create facts")
	for i, name := range []string{"grant-created", "grant-updated", "grant-revoked"} {
		event := store.Events[i]
		require.Equal(t, model.BusinessEventTypePrefix+name, event.Type)
		require.Equal(t, grant.ID, event.GrantID)
		require.Equal(t, request.AgentID, event.AgentID)
		require.Equal(t, request.Principal, *event.Subject)
		require.Equal(t, []id.PermissionSetID{request.GrantedPermissionSets[0].PermissionSetID}, event.PermissionSetIDs)
	}
}

func TestConsentLedgerRecordingFailureRollsBackEveryMutation(t *testing.T) {
	for _, operation := range []string{"create", "update", "revoke"} {
		for _, failure := range []string{"append", "commit"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				svc, repo, store, request, _ := newLedgerConsent(t)
				ctx := context.Background()
				var before *storage.UserGrant
				if operation != "create" {
					var err error
					before, err = svc.GrantConsent(ctx, request)
					require.NoError(t, err)
					store.Events = nil
				}
				failed := errors.New("ledger unavailable")
				if failure == "append" {
					store.AppendError = failed
				} else {
					store.CommitError = failed
				}
				if operation == "revoke" {
					require.Error(t, svc.RevokeConsentForPrincipal(ctx, request.Principal, request.AgentID))
				} else {
					if operation == "update" {
						expiry := time.Now().Add(time.Hour)
						request.ValidUntil = &expiry
					}
					result, err := svc.GrantConsent(ctx, request)
					require.Error(t, err)
					require.Nil(t, result, "failed recording must not release a success result")
				}
				require.Empty(t, store.Events)
				if before == nil {
					require.Empty(t, repo.grants)
				} else {
					require.Equal(t, before, repo.grants[before.ID])
				}
			})
		}
	}
}

func TestConsentLedgerExpiryRecognitionAndRenewedValidity(t *testing.T) {
	svc, repo, store, request, markers := newLedgerConsent(t)
	ctx := context.Background()
	grant, err := svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	store.Events = nil
	first := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	repo.grants[grant.ID].ValidUntil = &first
	for range 2 {
		active, err := svc.GetActiveGrants(ctx, request.Principal, request.AgentID)
		require.NoError(t, err)
		require.Empty(t, active)
	}
	_, err = svc.VerifyAgentAccess(ctx, request.Principal, request.AgentID)
	require.ErrorIs(t, err, ErrGrantExpired)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"grant-expired", store.Events[0].Type)
	require.Equal(t, first, store.Events[0].OccurredAt)
	require.Equal(t, first, markers.markers[grant.ID])
	second := first.Add(time.Minute)
	repo.grants[grant.ID].ValidUntil = &second
	_, err = svc.GetActiveGrants(ctx, request.Principal, request.AgentID)
	require.NoError(t, err)
	require.Len(t, store.Events, 2)
	require.Equal(t, second, store.Events[1].OccurredAt)
}

func TestConsentLedgerExpiryAppendFailureDoesNotConsumeMarker(t *testing.T) {
	svc, repo, store, request, markers := newLedgerConsent(t)
	grant, err := svc.GrantConsent(context.Background(), request)
	require.NoError(t, err)
	store.Events = nil
	expiry := time.Now().UTC().Add(-time.Hour)
	repo.grants[grant.ID].ValidUntil = &expiry
	store.AppendError = errors.New("ledger unavailable")
	_, err = svc.GetActiveGrants(context.Background(), request.Principal, request.AgentID)
	require.Error(t, err)
	require.Empty(t, markers.markers)
	require.Empty(t, store.Events)
}

func TestConsentLedgerPermissionOrderingDoesNotCreateUpdateFact(t *testing.T) {
	svc, repo, store, request, _ := newLedgerConsent(t)
	entry := &request.GrantedPermissionSets[0]
	secondService := id.NewServiceID()
	set := svc.psService.(*mockPermissionSetService).permissionSets[entry.PermissionSetID]
	set.ServiceScopes = append(set.ServiceScopes, storage.ServiceScope{ServiceID: secondService, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional})
	session := &storage.UserSession{ID: id.NewSessionID(), Principal: request.Principal, ServiceID: secondService}
	svc.sessionRepo.(*mockUserSessionRepo).sessions[session.ID] = session
	entry.IncludedServiceIDs = append(entry.IncludedServiceIDs, secondService)
	grant, err := svc.GrantConsent(context.Background(), request)
	require.NoError(t, err)
	entry.IncludedServiceIDs[0], entry.IncludedServiceIDs[1] = entry.IncludedServiceIDs[1], entry.IncludedServiceIDs[0]
	_, err = svc.GrantConsent(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, store.Events, 1, "permission set order is not an effective permission change")
	require.Equal(t, grant, repo.grants[grant.ID])
}

func TestConsentLedgerReplacesIncludedServiceWithoutChangingValidity(t *testing.T) {
	svc, repo, store, request, _ := newLedgerConsent(t)
	ctx := context.Background()
	expiry := time.Now().UTC().Add(time.Hour)
	request.ValidUntil = &expiry
	grant, err := svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	secondService := id.NewServiceID()
	psID := request.GrantedPermissionSets[0].PermissionSetID
	set := svc.psService.(*mockPermissionSetService).permissionSets[psID]
	set.ServiceScopes = append(set.ServiceScopes, storage.ServiceScope{ServiceID: secondService, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional})
	session := &storage.UserSession{ID: id.NewSessionID(), Principal: request.Principal, ServiceID: secondService}
	svc.sessionRepo.(*mockUserSessionRepo).sessions[session.ID] = session
	request.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{secondService}}}
	updated, err := svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	require.Equal(t, grant.ID, updated.ID)
	require.Equal(t, grant.ValidUntil, updated.ValidUntil)
	require.Equal(t, request.GrantedPermissionSets, updated.GrantedPermissionSets)
	stored, err := repo.Get(ctx, grant.ID)
	require.NoError(t, err)
	require.Equal(t, updated, stored)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"grant-created", store.Events[0].Type)
	require.Equal(t, model.BusinessEventTypePrefix+"grant-updated", store.Events[1].Type)
	require.Equal(t, grant.ID, store.Events[1].GrantID)
	_, err = svc.GrantConsent(ctx, request)
	require.NoError(t, err)
	require.Len(t, store.Events, 2, "repeating the new membership must remain silent")
}
