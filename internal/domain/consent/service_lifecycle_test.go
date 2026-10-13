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

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
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

type failingConsentEventRepo struct {
	ports.BusinessEventRepository
	err error
}

func (r failingConsentEventRepo) Append(context.Context, *model.BusinessEvent, bool) error {
	return r.err
}

type consentCommitOwnerKey struct{}

type failingConsentCommitManager struct {
	ports.StorageTransactionManager
	err error
}

func (m failingConsentCommitManager) BeginTX(ctx context.Context) (context.Context, error) {
	_, joined := ports.StorageTransactionEffectsFromContext(ctx)
	txCtx, err := m.StorageTransactionManager.BeginTX(ctx)
	if err != nil {
		return nil, err
	}
	return context.WithValue(txCtx, consentCommitOwnerKey{}, !joined), nil
}

func (m failingConsentCommitManager) Commit(ctx context.Context) error {
	if owning, _ := ctx.Value(consentCommitOwnerKey{}).(bool); owning {
		return m.err
	}
	return m.StorageTransactionManager.Commit(ctx)
}

func newConsentLifecycleRecorder(t *testing.T, manager *memory.TransactionManager, failure string, cause error) *ledger.Service {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	var events ports.BusinessEventRepository = memory.NewBusinessEventRepository(manager, registry)
	var transactions ports.StorageTransactionManager = manager
	switch failure {
	case "append":
		events = failingConsentEventRepo{events, cause}
	case "commit":
		transactions = failingConsentCommitManager{transactions, cause}
	}
	return ledger.NewService(registry, events, nil, transactions, false)
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
		for _, failure := range []string{"none", "delete", "refresh", "append", "commit"} {
			t.Run(fmt.Sprintf("user_facing=%t/failure=%s", userFacing, failure), func(t *testing.T) {
				ctx := context.Background()
				agentID, otherAgent := id.NewAgentID(), id.NewAgentID()
				principal, otherPrincipal := id.Principal("alice"), id.Principal("bob")
				manager := memory.NewTransactionManager()
				grants := memory.NewUserGrantRepository(manager)
				refresh := memory.NewRefreshTokenSessionStore(manager)
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
				recorder := newConsentLifecycleRecorder(t, manager, failure, failureErr)
				switch failure {
				case "delete":
					grantRepo = failingConsentDeleteRepo{grants, failureErr}
				case "refresh":
					refreshRepo = failingConsentRefreshRepo{refresh, failureErr}
				}
				var logs bytes.Buffer
				svc := consent.NewService(memory.NewAgentRepository(manager), nil, grantRepo, nil, refreshRepo, nil, slog.New(slog.NewJSONHandler(&logs, nil)), recorder)
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
					assert.Empty(t, logs.String(), "rolled-back revocation must not log success")
				}
				_, err = grants.FindByPrincipalAndAgent(ctx, principal, agentID)
				if failure != "none" {
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
				successorErr := refresh.CreateSuccessor(ctx, &storage.RefreshTokenSession{Signature: "after-used", AgentID: agentID, Principal: principal}, "used")
				if failure == "none" {
					require.Error(t, successorErr, "consumed ancestors remain revoked after consent deletion")
				} else {
					require.NoError(t, successorErr, "rollback restores consumed ancestors' revocation state")
				}
				retained, queryErr := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: principal}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
				require.NoError(t, queryErr)
				if failure == "none" {
					require.Len(t, retained, 1)
					assert.Equal(t, model.BusinessEventTypePrefix+"grant-revoked", retained[0].Type)
				} else {
					assert.Empty(t, retained, "rolled-back deletion must not leave a ledger fact")
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
		name       string
		validUntil *time.Time
		revoked    bool
		failure    string
	}{{"expired", &past, true, ""}, {"active", &future, false, ""}, {"indefinite", nil, false, ""}, {"revocation-fails", &past, false, "refresh"}, {"append-fails", &past, false, "append"}, {"commit-fails", &past, false, "commit"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			agentID, serviceID, psID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
			principal := id.Principal("alice")
			entries := []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID}}}
			manager := memory.NewTransactionManager()
			grants, refresh := memory.NewUserGrantRepository(manager), memory.NewRefreshTokenSessionStore(manager)
			require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), Principal: principal, AgentID: agentID, ValidUntil: tc.validUntil, GrantedPermissionSets: entries}))
			require.NoError(t, refresh.Create(ctx, &storage.RefreshTokenSession{Signature: "old", Principal: principal, AgentID: agentID}))
			require.NoError(t, refresh.Create(ctx, &storage.RefreshTokenSession{Signature: "other", Principal: id.Principal("bob"), AgentID: agentID}))
			require.NoError(t, refresh.Create(ctx, &storage.RefreshTokenSession{Signature: "used", Principal: principal, AgentID: agentID, UsedAt: &future}))
			agents := memory.NewAgentRepository(manager)
			require.NoError(t, agents.Create(ctx, &storage.Agent{ID: agentID, DisplayName: "Agent", Description: "Lifecycle test", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeOptional}}, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional}}}))
			sessions := memory.NewInMemoryUserSessionRepository(manager)
			require.NoError(t, sessions.Create(ctx, &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, EncryptedAccessToken: []byte("opaque"), TokenType: "Bearer", RefreshTokenExpiresAt: &future}))
			psRepo := memory.NewPermissionSetRepository(manager)
			require.NoError(t, psRepo.Create(ctx, &storage.PermissionSet{ID: psID, Name: "Access", Description: "Lifecycle test", ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional}}}))
			psService := permissionset.NewPermissionSetService(psRepo, grants, slog.Default())
			t.Cleanup(psService.Close)
			var refreshRepo ports.RefreshTokenSessionRepository = refresh
			failureErr := errors.New("revocation unavailable")
			recorder := newConsentLifecycleRecorder(t, manager, tc.failure, failureErr)
			if tc.failure == "refresh" {
				refreshRepo = failingConsentRefreshRepo{refresh, failureErr}
			}
			var logs bytes.Buffer
			svc := consent.NewService(agents, nil, grants, sessions, refreshRepo, psService, slog.New(slog.NewJSONHandler(&logs, nil)), recorder)
			grant, err := svc.GrantConsent(ctx, &consent.GrantRequest{Principal: principal, AgentID: agentID, GrantedPermissionSets: entries})
			if tc.failure != "" {
				require.ErrorIs(t, err, failureErr)
				assert.Nil(t, grant)
				stored, err := grants.FindByPrincipalAndAgent(ctx, principal, agentID)
				require.NoError(t, err)
				assert.Equal(t, tc.validUntil, stored.ValidUntil)
				assert.Empty(t, logs.String(), "rolled-back renewal must not log success")
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
			successorErr := refresh.CreateSuccessor(ctx, &storage.RefreshTokenSession{Signature: "after-used", Principal: principal, AgentID: agentID}, "used")
			if tc.revoked {
				require.Error(t, successorErr, "renewal cannot resurrect a consumed ancestor with the retained grant ID")
			} else {
				require.NoError(t, successorErr)
			}
			retained, queryErr := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: principal}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
			require.NoError(t, queryErr)
			if tc.failure == "" && tc.validUntil != nil {
				require.Len(t, retained, 1)
				assert.Equal(t, model.BusinessEventTypePrefix+"grant-updated", retained[0].Type)
			} else {
				assert.Empty(t, retained, "failed and unchanged renewals must not create facts")
			}
		})
	}
}

func TestService_IdempotentAbsentRevocationJoinsOwningTransaction(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollback=%t", rollback), func(t *testing.T) {
			ctx := t.Context()
			manager := memory.NewTransactionManager()
			grants, refresh := memory.NewUserGrantRepository(manager), memory.NewRefreshTokenSessionStore(manager)
			principal, agentID := id.Principal("alice"), id.NewAgentID()
			require.NoError(t, refresh.Create(ctx, &storage.RefreshTokenSession{Signature: "old", Principal: principal, AgentID: agentID}))
			recorder := newConsentLifecycleRecorder(t, manager, "", nil)
			var logs bytes.Buffer
			svc := consent.NewService(memory.NewAgentRepository(manager), nil, grants, nil, refresh, nil, slog.New(slog.NewJSONHandler(&logs, nil)), recorder)
			rollbackErr := errors.New("outer operation failed")
			err := recorder.WithTransaction(ctx, ports.StorageTransactionHints{Subjects: []ports.StorageSubjectGate{{Principal: principal}}}, func(txCtx context.Context) error {
				if err := svc.RevokeConsent(txCtx, principal, agentID); err != nil {
					return err
				}
				assert.Empty(t, logs.String(), "joined cleanup logs only after the owner commits")
				if rollback {
					return rollbackErr
				}
				return nil
			})
			if rollback {
				require.ErrorIs(t, err, rollbackErr)
				assert.Empty(t, logs.String())
			} else {
				require.NoError(t, err)
				assertConsentRevocationAudit(t, logs.Bytes(), "consent_revoked", 1)
			}
			row, err := refresh.FindBySignature(ctx, "old")
			require.NoError(t, err)
			assert.Equal(t, !rollback, row.UsedAt != nil)
			retained, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: principal}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
			require.NoError(t, err)
			assert.Empty(t, retained, "absent grant revocation is not a grant-revoked fact")
		})
	}
}
