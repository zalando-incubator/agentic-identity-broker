//go:build integration

package storage_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestConsentLedgerConcurrentIdenticalFirstGrantRecordsOnlyWinner(t *testing.T) {
	adapter, owner := openLedgerFoundation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	recorder := ledger.NewService(registry, adapter.BusinessEvents(), adapter.BusinessEventLifecycle(), adapter, false)
	sets := permissionset.NewPermissionSetService(adapter.PermissionSets(), adapter.UserGrants(), slog.Default())
	defer sets.Close()
	service := consent.NewService(adapter.Agents(), nil, adapter.UserGrants(), adapter.UserSessions(), adapter.RefreshTokenSessions(), sets, slog.Default(), recorder)
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Consent agent", Description: "Concurrent first grant", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}}}
	provider := createTestService("first-grant-provider", "First grant provider", nil)
	provider.Secret, provider.TokenEndpointAuthMethod = model.NewAbsentSecret(), model.TokenEndpointAuthMethodNone
	require.NoError(t, adapter.Services().Create(ctx, provider))
	now := time.Now().UTC()
	set := &storage.PermissionSet{ID: agent.PermissionSets[0].PermissionSetID, Name: "Ledger test", Description: "Ledger test", ServiceScopes: []storage.ServiceScope{{ServiceID: provider.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, adapter.PermissionSets().Create(ctx, set))
	require.NoError(t, adapter.Agents().Create(ctx, agent))
	request := &consent.GrantRequest{Principal: id.Principal("first-grant-user"), AgentID: agent.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: set.ID, IncludedServiceIDs: []id.ServiceID{provider.ID}}}}
	require.NoError(t, adapter.UserSessions().Create(ctx, &storage.UserSession{ID: id.NewSessionID(), Principal: request.Principal, ServiceID: provider.ID, EncryptedAccessToken: []byte("opaque-session-fixture"), TokenType: "Bearer", Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID}, InitiatedAt: now, CreatedAt: now, UpdatedAt: now}))
	_, err = adapter.UserGrants().FindByPrincipalAndAgent(ctx, request.Principal, request.AgentID)
	require.True(t, ports.IsNotFoundErr(err))

	gate, err := owner.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = gate.Rollback() }()
	var locked int
	require.NoError(t, gate.GetContext(ctx, &locked, `SELECT 1 FROM agents WHERE id=$1 FOR UPDATE`, agent.ID))
	type outcome struct {
		grant *storage.UserGrant
		err   error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			grant, err := service.GrantConsent(ctx, request)
			results <- outcome{grant, err}
		}()
	}
	require.Eventually(t, func() bool {
		var waiting int
		err := owner.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND cardinality(pg_blocking_pids(pid)) > 0`)
		return err == nil && waiting == 2
	}, 3*time.Second, 10*time.Millisecond, "both consent requests must wait on the agent before creating a grant")
	require.NoError(t, gate.Commit())
	var grantID id.GrantID
	for range 2 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.NotNil(t, result.grant)
			require.False(t, result.grant.ID.IsZero())
			if grantID.IsZero() {
				grantID = result.grant.ID
			}
			require.Equal(t, grantID, result.grant.ID)
		case <-ctx.Done():
			t.Fatal("competing consent requests did not complete")
		}
	}
	stored, err := adapter.UserGrants().FindByPrincipalAndAgent(ctx, request.Principal, request.AgentID)
	require.NoError(t, err)
	require.Equal(t, grantID, stored.ID)
	events, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: request.Principal}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour), Limit: 100})
	require.NoError(t, err)
	require.Len(t, events, 1, "identical competing grants must not record an update or another creation")
	require.Equal(t, model.BusinessEventTypePrefix+"grant-created", events[0].Type)
	require.Equal(t, grantID, events[0].GrantID)
	require.Equal(t, agent.ID, events[0].AgentID)
}

func TestConsentLedgerRevocationTransactions(t *testing.T) {
	adapter, db := openLedgerFoundation(t)
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	recorder := ledger.NewService(registry, adapter.BusinessEvents(), adapter.BusinessEventLifecycle(), adapter, false)
	sets := permissionset.NewPermissionSetService(adapter.PermissionSets(), adapter.UserGrants(), slog.Default())
	defer sets.Close()
	service := consent.NewService(adapter.Agents(), nil, adapter.UserGrants(), adapter.UserSessions(), adapter.RefreshTokenSessions(), sets, slog.Default(), recorder)
	for _, operation := range []string{"absent", "revoke", "renew"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			provider := createTestService(operation+"-provider", "Consent provider", nil)
			provider.Secret, provider.TokenEndpointAuthMethod = model.NewAbsentSecret(), model.TokenEndpointAuthMethodNone
			require.NoError(t, adapter.Services().Create(ctx, provider))
			now := time.Now().UTC()
			set := &storage.PermissionSet{ID: id.NewPermissionSetID(), Name: operation, Description: operation, ServiceScopes: []storage.ServiceScope{{ServiceID: provider.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}}, CreatedAt: now, UpdatedAt: now}
			require.NoError(t, adapter.PermissionSets().Create(ctx, set))
			agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: operation, Description: operation, PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: set.ID, RequirementType: storage.RequirementTypeOptional}}}
			require.NoError(t, adapter.Agents().Create(ctx, agent))
			request := &consent.GrantRequest{Principal: id.Principal(operation + "-user"), AgentID: agent.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: set.ID, IncludedServiceIDs: []id.ServiceID{provider.ID}}}}
			require.NoError(t, adapter.UserSessions().Create(ctx, &storage.UserSession{ID: id.NewSessionID(), Principal: request.Principal, ServiceID: provider.ID, EncryptedAccessToken: []byte("opaque-session-fixture"), TokenType: "Bearer", Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID}, InitiatedAt: now, CreatedAt: now, UpdatedAt: now}))
			refresh := &storage.RefreshTokenSession{Signature: operation, RequestID: operation, AgentID: agent.ID, ClientID: id.NewClientID("consent-client"), Principal: request.Principal, ExpiresAt: now.Add(time.Hour), CreatedAt: now}
			require.NoError(t, adapter.RefreshTokenSessions().Create(ctx, refresh))
			if operation == "absent" {
				require.ErrorIs(t, service.RevokeConsentForPrincipal(ctx, request.Principal, agent.ID), consent.ErrGrantNotFound)
				require.NoError(t, service.RevokeConsent(ctx, request.Principal, agent.ID))
			} else {
				grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: request.Principal, AgentID: agent.ID, GrantedPermissionSets: request.GrantedPermissionSets, CreatedAt: now, UpdatedAt: now}
				require.NoError(t, adapter.UserGrants().Create(ctx, grant))
				if operation == "renew" {
					_, err := db.ExecContext(ctx, `UPDATE user_grants SET valid_until=$1 WHERE id=$2`, now.Add(-time.Hour), grant.ID)
					require.NoError(t, err)
				}
				blocker, err := db.BeginTxx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = blocker.Rollback() }()
				var blockerPID, locked int
				require.NoError(t, blocker.GetContext(ctx, &blockerPID, `SELECT pg_backend_pid()`))
				require.NoError(t, blocker.GetContext(ctx, &locked, `SELECT 1 FROM agents WHERE id=$1 FOR UPDATE`, agent.ID))
				finished := make(chan error, 1)
				go func() {
					if operation == "renew" {
						_, err := service.GrantConsent(ctx, request)
						finished <- err
					} else {
						finished <- service.RevokeConsentForPrincipal(ctx, request.Principal, agent.ID)
					}
				}()
				require.Eventually(t, func() bool {
					var waiting bool
					err := db.GetContext(ctx, &waiting, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))`, blockerPID)
					return err == nil && waiting
				}, 3*time.Second, 10*time.Millisecond, "consent must reach the held agent lock")
				require.NoError(t, blocker.GetContext(ctx, &locked, `SELECT 1 FROM user_grants WHERE id=$1 FOR UPDATE NOWAIT`, grant.ID), "agent deletion must be able to lock its grants while consent waits on the agent")
				require.NoError(t, blocker.Commit())
				select {
				case err := <-finished:
					require.NoError(t, err)
				case <-ctx.Done():
					t.Fatal("consent did not complete after releasing the agent")
				}
				stored, err := adapter.UserGrants().FindByPrincipalAndAgent(ctx, request.Principal, agent.ID)
				if operation == "renew" {
					require.NoError(t, err)
					require.Equal(t, grant.ID, stored.ID)
					require.Nil(t, stored.ValidUntil)
				} else {
					require.ErrorIs(t, err, ports.ErrNotFound)
					require.ErrorIs(t, service.RevokeConsentForPrincipal(ctx, request.Principal, agent.ID), consent.ErrGrantNotFound)
				}
			}
			retired, err := adapter.RefreshTokenSessions().FindBySignature(ctx, refresh.Signature)
			require.NoError(t, err)
			require.NotNil(t, retired.UsedAt, "absent revocation and expired renewal must still invalidate refresh sessions")
			events, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: request.Principal}, Start: now.Add(-time.Hour), End: now.Add(time.Hour), Limit: 100})
			require.NoError(t, err)
			if operation == "absent" {
				require.Empty(t, events, "absent revocation is not a business fact")
			} else {
				require.Len(t, events, 1)
				eventName := "grant-revoked"
				if operation == "renew" {
					eventName = "grant-updated"
				}
				require.Equal(t, model.BusinessEventTypePrefix+eventName, events[0].Type)
			}
		})
	}
}
