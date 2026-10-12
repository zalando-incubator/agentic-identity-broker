package consent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingConsentRefreshRepo struct {
	ports.RefreshTokenSessionRepository
	err error
}

func (r failingConsentRefreshRepo) RevokeByPrincipalAndAgent(context.Context, id.Principal, id.AgentID) (int, error) {
	return 0, r.err
}

type failingConsentDeleteRepo struct {
	ports.UserGrantRepository
	err error
}

func (r failingConsentDeleteRepo) DeleteByPrincipalAndAgentID(context.Context, id.Principal, id.AgentID) (*storage.UserGrant, error) {
	return nil, r.err
}

func assertConsentRevocationAudit(t *testing.T, logs []byte, trigger string, count int) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(logs))
	found := 0
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if _, ok := record["trigger"]; ok {
			found++
			assert.Equal(t, trigger, record["trigger"])
			assert.Equal(t, float64(count), record["row_count"])
		}
	}
	assert.Equal(t, 1, found)
}

func TestService_RevocationInvalidatesRefreshSessions(t *testing.T) {
	for _, userFacing := range []bool{false, true} {
		for _, failure := range []string{"none", "delete", "refresh"} {
			t.Run(fmt.Sprintf("user_facing=%t/failure=%s", userFacing, failure), func(t *testing.T) {
				ctx := context.Background()
				agentID, otherAgent := id.NewAgentID(), id.NewAgentID()
				principal, otherPrincipal := id.Principal("alice"), id.Principal("bob")
				grants := memory.NewUserGrantRepository()
				refresh := memory.NewRefreshTokenSessionStore()
				usedAt := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
				for _, row := range []storage.RefreshTokenSession{
					{Signature: "old", AgentID: agentID, Principal: principal},
					{Signature: "another-chain", AgentID: agentID, Principal: principal},
					{Signature: "used", AgentID: agentID, Principal: principal, UsedAt: &usedAt},
					{Signature: "other-principal", AgentID: agentID, Principal: otherPrincipal},
					{Signature: "other-agent", AgentID: otherAgent, Principal: principal},
				} {
					require.NoError(t, refresh.Create(ctx, &row))
				}
				for _, pair := range []struct {
					principal id.Principal
					agent     id.AgentID
				}{{principal, agentID}, {otherPrincipal, agentID}, {principal, otherAgent}} {
					require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), Principal: pair.principal, AgentID: pair.agent}))
				}
				var grantRepo ports.UserGrantRepository = grants
				var refreshRepo ports.RefreshTokenSessionRepository = refresh
				failureErr := errors.New("storage unavailable")
				switch failure {
				case "delete":
					grantRepo = failingConsentDeleteRepo{grants, failureErr}
				case "refresh":
					refreshRepo = failingConsentRefreshRepo{refresh, failureErr}
				}
				var logs bytes.Buffer
				svc := consent.NewService(nil, nil, grantRepo, nil, refreshRepo, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
				var err error
				if userFacing {
					err = svc.RevokeConsentForPrincipal(ctx, principal, agentID)
				} else {
					err = svc.RevokeConsent(ctx, principal, agentID)
				}
				if failure == "none" {
					require.NoError(t, err)
					assertConsentRevocationAudit(t, logs.Bytes(), "consent_revoked", 2)
				} else {
					require.ErrorIs(t, err, failureErr)
				}
				_, err = grants.FindByPrincipalAndAgent(ctx, principal, agentID)
				if failure == "delete" {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ports.ErrNotFound)
				}
				for _, signature := range []string{"old", "another-chain", "used", "other-principal", "other-agent"} {
					row, err := refresh.FindBySignature(ctx, signature)
					require.NoError(t, err)
					switch {
					case signature == "used":
						assert.Equal(t, usedAt, *row.UsedAt)
					case failure == "none" && (signature == "old" || signature == "another-chain"):
						assert.NotNil(t, row.UsedAt)
						require.Error(t, refresh.MarkUsed(ctx, signature))
					default:
						assert.Nil(t, row.UsedAt)
					}
				}
				_, err = grants.FindByPrincipalAndAgent(ctx, otherPrincipal, agentID)
				require.NoError(t, err)
				_, err = grants.FindByPrincipalAndAgent(ctx, principal, otherAgent)
				require.NoError(t, err)
			})
		}
	}
}

func TestService_ExpiredGrantRenewalInvalidatesRefreshSessions(t *testing.T) {
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name          string
		validUntil    *time.Time
		revoked, fail bool
	}{{"expired", &past, true, false}, {"active", &future, false, false}, {"indefinite", nil, false, false}, {"revocation-fails", &past, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			agentID, serviceID, psID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
			principal := id.Principal("alice")
			entries := []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID}}}
			grants, refresh := memory.NewUserGrantRepository(), memory.NewRefreshTokenSessionStore()
			require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), Principal: principal, AgentID: agentID, ValidUntil: tc.validUntil, GrantedPermissionSets: entries}))
			require.NoError(t, refresh.Create(ctx, &storage.RefreshTokenSession{Signature: "old", Principal: principal, AgentID: agentID}))
			require.NoError(t, refresh.Create(ctx, &storage.RefreshTokenSession{Signature: "other", Principal: id.Principal("bob"), AgentID: agentID}))
			agents := memory.NewAgentRepository()
			require.NoError(t, agents.Create(ctx, &storage.Agent{ID: agentID, DisplayName: "Agent", Description: "Lifecycle test", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeOptional}}, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional}}}))
			sessions := memory.NewInMemoryUserSessionRepository()
			require.NoError(t, sessions.Create(ctx, &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, EncryptedAccessToken: []byte("opaque"), TokenType: "Bearer", RefreshTokenExpiresAt: &future}))
			psRepo := memory.NewPermissionSetRepository()
			require.NoError(t, psRepo.Create(ctx, &storage.PermissionSet{ID: psID, Name: "Access", Description: "Lifecycle test", ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional}}}))
			psService := permissionset.NewPermissionSetService(psRepo, grants, slog.Default())
			t.Cleanup(psService.Close)
			var refreshRepo ports.RefreshTokenSessionRepository = refresh
			failureErr := errors.New("revocation unavailable")
			if tc.fail {
				refreshRepo = failingConsentRefreshRepo{refresh, failureErr}
			}
			var logs bytes.Buffer
			svc := consent.NewService(agents, nil, grants, sessions, refreshRepo, psService, slog.New(slog.NewJSONHandler(&logs, nil)))
			grant, err := svc.GrantConsent(ctx, &consent.GrantRequest{Principal: principal, AgentID: agentID, GrantedPermissionSets: entries})
			if tc.fail {
				require.ErrorIs(t, err, failureErr)
				assert.Nil(t, grant)
				stored, err := grants.FindByPrincipalAndAgent(ctx, principal, agentID)
				require.NoError(t, err)
				assert.Equal(t, tc.validUntil, stored.ValidUntil)
			} else {
				require.NoError(t, err)
				require.NotNil(t, grant)
				assert.Nil(t, grant.ValidUntil)
				if tc.revoked {
					assertConsentRevocationAudit(t, logs.Bytes(), "expired_grant_renewed", 1)
				}
			}
			old, err := refresh.FindBySignature(ctx, "old")
			require.NoError(t, err)
			assert.Equal(t, tc.revoked, old.UsedAt != nil)
			other, err := refresh.FindBySignature(ctx, "other")
			require.NoError(t, err)
			assert.Nil(t, other.UsedAt)
		})
	}
}
