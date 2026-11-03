package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingCredentialRefreshRepo struct {
	ports.RefreshTokenSessionRepository
	err error
}

func (r failingCredentialRefreshRepo) RevokeByAgent(context.Context, id.AgentID) (int, error) {
	return 0, r.err
}

type failingCredentialDeleteRepo struct {
	ports.ClientCredentialRepository
	err error
}

func (r failingCredentialDeleteRepo) Delete(context.Context, id.AgentID) error {
	return r.err
}

func TestCredentialService_RevokeInvalidatesRefreshSessions(t *testing.T) {
	for _, failure := range []string{"none", "delete", "refresh"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			agentID, otherAgent := id.NewAgentID(), id.NewAgentID()
			manager := memory.NewTransactionManager()
			registry, err := ledger.NewRegistry(eventschemas.Schemas)
			require.NoError(t, err)
			events := memory.NewBusinessEventRepository(manager, registry)
			recorder := ledger.NewService(registry, events, nil, manager, false)
			credentials, refresh := memory.NewClientCredentialStore(manager), memory.NewRefreshTokenSessionStore(manager)
			for _, agent := range []id.AgentID{agentID, otherAgent} {
				require.NoError(t, credentials.Create(ctx, &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent}))
			}
			usedAt := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, row := range []storage.RefreshTokenSession{
				{Signature: "alice", AgentID: agentID, Principal: id.Principal("alice")},
				{Signature: "bob", AgentID: agentID, Principal: id.Principal("bob")},
				{Signature: "used", AgentID: agentID, UsedAt: &usedAt},
				{Signature: "other-agent", AgentID: otherAgent},
			} {
				require.NoError(t, refresh.Create(ctx, &row))
			}
			var logs bytes.Buffer
			svc := NewCredentialService(ledgerCredentialAgents{agentID: agentID}, credentials, refresh, nil, slog.New(slog.NewJSONHandler(&logs, nil)), recorder)
			failureErr := errors.New("storage unavailable")
			switch failure {
			case "delete":
				svc.credentials = failingCredentialDeleteRepo{credentials, failureErr}
			case "refresh":
				svc.refreshRepo = failingCredentialRefreshRepo{refresh, failureErr}
			}
			err = svc.Revoke(ctx, agentID)
			if failure == "none" {
				require.NoError(t, err)
				var audit map[string]any
				require.NoError(t, json.Unmarshal(logs.Bytes(), &audit))
				assert.Equal(t, "credential_revoked", audit["trigger"])
				assert.Equal(t, float64(2), audit["row_count"])
			} else {
				require.ErrorIs(t, err, failureErr)
				assert.Empty(t, logs.String(), "rolled-back revocation must not log success")
			}
			_, err = credentials.GetByAgentID(ctx, agentID)
			assert.Equal(t, failure != "none", err == nil, "cleanup failure rolls credential deletion back")
			_, err = credentials.GetByAgentID(ctx, otherAgent)
			require.NoError(t, err)
			for _, signature := range []string{"alice", "bob", "used", "other-agent"} {
				row, err := refresh.FindBySignature(ctx, signature)
				require.NoError(t, err)
				switch {
				case signature == "used":
					assert.Equal(t, usedAt, *row.UsedAt)
				case failure == "none" && signature != "other-agent":
					assert.NotNil(t, row.UsedAt)
					require.Error(t, refresh.MarkUsed(ctx, signature))
				default:
					assert.Nil(t, row.UsedAt)
				}
			}
			retained, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{NoSubject: true}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
			require.NoError(t, err)
			if failure == "none" {
				require.Len(t, retained, 1)
				assert.Equal(t, model.BusinessEventTypePrefix+"credential-revoked", retained[0].Type)
			} else {
				assert.Empty(t, retained)
			}
		})
	}
}

type ledgerCredentialAgents struct {
	ports.AgentRepository
	agentID id.AgentID
}

func (r ledgerCredentialAgents) Get(_ context.Context, agentID id.AgentID) (*storage.Agent, error) {
	if agentID != r.agentID {
		return nil, ports.ErrNotFound
	}
	return &storage.Agent{ID: agentID}, nil
}

type ledgerCredentialRecords struct {
	ports.ClientCredentialRepository
	credential *storage.ClientCredential
}

func (r *ledgerCredentialRecords) GetByAgentID(_ context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	if r.credential == nil || r.credential.AgentID != agentID {
		return nil, ports.ErrNotFound
	}
	copy := *r.credential
	return &copy, nil
}
func (r *ledgerCredentialRecords) Create(_ context.Context, credential *storage.ClientCredential) error {
	if r.credential != nil {
		return errors.New("credential already exists")
	}
	copy := *credential
	r.credential = &copy
	return nil
}
func (r *ledgerCredentialRecords) Rotate(_ context.Context, agentID id.AgentID, credential *storage.ClientCredential) error {
	if r.credential == nil || r.credential.AgentID != agentID {
		return ports.ErrNotFound
	}
	copy := *credential
	r.credential = &copy
	return nil
}
func (r *ledgerCredentialRecords) Delete(context.Context, id.AgentID) error {
	r.credential = nil
	return nil
}

type ledgerCredentialRefreshRecords struct {
	ports.RefreshTokenSessionRepository
	revoked bool
}

func (r *ledgerCredentialRefreshRecords) RevokeByAgent(context.Context, id.AgentID) (int, error) {
	r.revoked = true
	return 1, nil
}

type ledgerCredentialGenerator struct{}

func (ledgerCredentialGenerator) GenerateCredentials(agentID id.AgentID) (*storage.ClientCredential, string, error) {
	return &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agentID, SecretHash: "credential-hash-canary"}, "plaintext-secret-canary", nil
}

func newLedgerCredentialService(t *testing.T) (*CredentialService, *ledgerCredentialRecords, *ledgerfixture.Store, id.AgentID) {
	t.Helper()
	agentID := id.NewAgentID()
	repo := &ledgerCredentialRecords{}
	refresh := &ledgerCredentialRefreshRecords{}
	store := &ledgerfixture.Store{Snapshot: func() func() {
		wasRevoked := refresh.revoked
		var before *storage.ClientCredential
		if repo.credential != nil {
			copy := *repo.credential
			before = &copy
		}
		return func() {
			repo.credential = before
			refresh.revoked = wasRevoked
		}
	}}
	svc := NewCredentialService(ledgerCredentialAgents{agentID: agentID}, repo, refresh, ledgerCredentialGenerator{}, testSlogger(), store.Recorder(t))
	return svc, repo, store, agentID
}

func TestCredentialLedgerRotationIsOneReplacementFact(t *testing.T) {
	svc, repo, store, agentID := newLedgerCredentialService(t)
	first, err := svc.Generate(context.Background(), agentID)
	require.NoError(t, err)
	require.False(t, first.Rotated)
	second, err := svc.Generate(context.Background(), agentID)
	require.NoError(t, err)
	require.True(t, second.Rotated)
	require.NotEqual(t, first.Credential.ID, second.Credential.ID)
	require.Equal(t, second.Credential.ID, repo.credential.ID)
	require.NoError(t, svc.Revoke(context.Background(), agentID))
	require.ErrorIs(t, svc.Revoke(context.Background(), agentID), ports.ErrNotFound)
	require.Len(t, store.Events, 3)
	for i, name := range []string{"credential-generated", "credential-rotated", "credential-revoked"} {
		event := store.Events[i]
		credentialID := second.Credential.ID
		if i == 0 {
			credentialID = first.Credential.ID
		}
		require.Equal(t, model.BusinessEventTypePrefix+name, event.Type)
		require.Equal(t, agentID, event.AgentID)
		require.Nil(t, event.Subject)
		require.Equal(t, map[string]any{"credential_id": credentialID.String()}, event.Data)
	}
}

func TestCredentialLedgerFailureWithholdsSecretAndRestoresRecord(t *testing.T) {
	for _, action := range []string{"generate", "rotate", "revoke"} {
		for _, failure := range []string{"append", "commit"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				svc, repo, store, agentID := newLedgerCredentialService(t)
				var before *storage.ClientCredential
				if action != "generate" {
					result, err := svc.Generate(context.Background(), agentID)
					require.NoError(t, err)
					copy := *result.Credential
					before = &copy
					store.Events = nil
				}
				var logs bytes.Buffer
				svc.logger = slog.New(slog.NewJSONHandler(&logs, nil))
				failed := errors.New("ledger unavailable")
				if failure == "append" {
					store.AppendError = failed
				} else {
					store.CommitError = failed
				}
				if action == "revoke" {
					require.Error(t, svc.Revoke(context.Background(), agentID))
				} else {
					result, err := svc.Generate(context.Background(), agentID)
					require.Error(t, err)
					require.Empty(t, result.PlaintextSecret)
					require.Nil(t, result.Credential)
				}
				require.Equal(t, before, repo.credential)
				require.False(t, svc.refreshRepo.(*ledgerCredentialRefreshRecords).revoked)
				require.Empty(t, store.Events)
				require.Empty(t, logs.String())
			})
		}
	}
}

func TestCredentialLedgerRevocationLogWaitsForOwnerCommit(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, store, agentID := newLedgerCredentialService(t)
			generated, err := svc.Generate(ctx, agentID)
			require.NoError(t, err)
			store.Events = nil
			var logs bytes.Buffer
			svc.logger = slog.New(slog.NewJSONHandler(&logs, nil))
			owner, err := store.BeginTX(ctx)
			require.NoError(t, err)
			require.NoError(t, svc.Revoke(owner, agentID))
			require.Nil(t, repo.credential)
			require.True(t, svc.refreshRepo.(*ledgerCredentialRefreshRecords).revoked)
			require.Empty(t, store.Events)
			require.Empty(t, logs.String(), "a joined scope must not announce an uncommitted revocation")
			if outcome == "rollback" {
				require.NoError(t, store.Rollback(owner))
				require.Equal(t, generated.Credential, repo.credential)
				require.False(t, svc.refreshRepo.(*ledgerCredentialRefreshRecords).revoked)
				require.Empty(t, store.Events)
				require.Empty(t, logs.String())
			} else {
				require.NoError(t, store.Commit(owner))
				require.Len(t, store.Events, 1)
				require.Contains(t, logs.String(), "refresh sessions revoked")
			}
		})
	}
}

func TestCredentialLedgerDoesNotReleaseSecretBeforeCommit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, store, agentID := newLedgerCredentialService(t)
		reached, release := make(chan struct{}), make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		store.BeforeCommit = func() { close(reached); <-release }
		type outcome struct {
			generated ports.CredentialGenerationResult
			err       error
		}
		result := make(chan outcome, 1)
		go func() {
			generated, err := svc.Generate(context.Background(), agentID)
			result <- outcome{generated, err}
		}()
		synctest.Wait()
		select {
		case <-reached:
		default:
			t.Fatal("credential generation did not reach an owning transaction commit")
		}
		select {
		case <-result:
			t.Fatal("plaintext secret was released before commit completed")
		default:
		}
		close(release)
		released = true
		completed := <-result
		require.NoError(t, completed.err)
		require.Equal(t, "plaintext-secret-canary", completed.generated.PlaintextSecret)
		require.Len(t, store.Events, 1)
		require.Equal(t, model.BusinessEventTypePrefix+"credential-generated", store.Events[0].Type)
	})
}
