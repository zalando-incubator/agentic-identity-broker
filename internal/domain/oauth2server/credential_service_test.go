package oauth2server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

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

type ledgerCredentialGenerator struct{}

func (ledgerCredentialGenerator) GenerateCredentials(agentID id.AgentID) (*storage.ClientCredential, string, error) {
	return &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agentID, SecretHash: "credential-hash-canary"}, "plaintext-secret-canary", nil
}

func newLedgerCredentialService(t *testing.T) (*CredentialService, *ledgerCredentialRecords, *ledgerfixture.Store, id.AgentID) {
	t.Helper()
	agentID := id.NewAgentID()
	repo := &ledgerCredentialRecords{}
	store := &ledgerfixture.Store{Snapshot: func() func() {
		var before *storage.ClientCredential
		if repo.credential != nil {
			copy := *repo.credential
			before = &copy
		}
		return func() { repo.credential = before }
	}}
	svc := NewCredentialService(ledgerCredentialAgents{agentID: agentID}, repo, ledgerCredentialGenerator{}, testSlogger(), store.Recorder(t))
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
				require.Empty(t, store.Events)
			})
		}
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
