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
	"github.com/ory/fosite"
	"github.com/stretchr/testify/require"
)

func TestProviderLedgerLocalIssuanceAndAuthenticationFailure(t *testing.T) {
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	store := &ledgerfixture.Store{}
	provider.ledger, provider.fositeStorage.transactions = store.Recorder(t), store
	response, err := provider.HandleClientCredentials(context.Background(), agent.ID.String(), secret, "read")
	require.NoError(t, err)
	require.Equal(t, "Bearer", response.TokenType)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-issued", store.Events[0].Type)
	require.Equal(t, agent.ID, store.Events[0].AgentID)
	require.Nil(t, store.Events[0].Subject, "an agent credential is not an authenticated end-user subject")
	response, err = provider.HandleClientCredentials(context.Background(), agent.ID.String(), "wrong-secret-canary", "read")
	require.ErrorIs(t, err, ErrInvalidClient)
	require.Nil(t, response)
	require.Len(t, store.Events, 2)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[1].Type)
	require.Equal(t, map[string]any{"reason_code": "authentication_failed"}, store.Events[1].Data)
}

func TestProviderLedgerScopeFailureReason(t *testing.T) {
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	agent.AllowedScopes = []string{"read"}
	require.NoError(t, agents.Update(context.Background(), agent))
	store := &ledgerfixture.Store{}
	provider.ledger, provider.fositeStorage.transactions = store.Recorder(t), store
	response, err := provider.HandleClientCredentials(context.Background(), agent.ID.String(), secret, "admin")
	require.ErrorIs(t, err, ErrInvalidScope)
	require.Nil(t, response)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	require.Equal(t, map[string]any{"reason_code": "authorization_failed"}, store.Events[0].Data)
}

func TestProviderLedgerStorageFailureReason(t *testing.T) {
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	store := &ledgerfixture.Store{}
	provider.ledger, provider.fositeStorage.transactions = store.Recorder(t), store
	provider.fositeStorage.codeRepo = &mockCodeRepo{findByCodeHashFunc: func(context.Context, string) (*storage.AuthorizationCode, error) {
		return nil, storage.NewStorageError("FindByCodeHash", storage.ErrorKindConnection, nil, "storage unavailable")
	}}
	response, err := provider.HandleAuthorizationCodeExchange(context.Background(), agent.ID.String(), secret, "code-canary", "http://localhost:8080/callback", "verifier-canary")
	require.ErrorIs(t, err, ErrServerError)
	require.Nil(t, response)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	require.Equal(t, map[string]any{"reason_code": "internal_failure"}, store.Events[0].Data)
}

func TestProviderLedgerInvalidRequestClassification(t *testing.T) {
	store := &ledgerfixture.Store{}
	provider := &Provider{ledger: store.Recorder(t)}
	facts := model.BusinessEvent{Actor: model.BusinessEventActor{Kind: "agent"}}
	var err error = fosite.ErrInvalidRequest
	provider.finishTokenRequest(context.Background(), &facts, &err)
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	require.Equal(t, map[string]any{"reason_code": "invalid_request"}, store.Events[0].Data)
}

func TestProviderLedgerFailureWithholdsConstructedToken(t *testing.T) {
	for _, failure := range []string{"append", "commit"} {
		t.Run(failure, func(t *testing.T) {
			provider, agents, _ := newTestProvider(t)
			agent, _, secret := setupTestCredentials(t, provider, agents)
			store := &ledgerfixture.Store{}
			failed := errors.New("ledger unavailable")
			if failure == "append" {
				store.AppendError = failed
			} else {
				store.CommitError = failed
			}
			provider.ledger, provider.fositeStorage.transactions = store.Recorder(t), store
			response, err := provider.HandleClientCredentials(context.Background(), agent.ID.String(), secret, "read")
			require.Error(t, err)
			require.Nil(t, response, "a signed credential must not escape failed recording")
			require.Empty(t, store.Events)
		})
	}
}

func TestProviderLedgerDoesNotReleaseTokenBeforeCommit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider, agents, _ := newTestProvider(t)
		agent, _, secret := setupTestCredentials(t, provider, agents)
		store := &ledgerfixture.Store{}
		provider.ledger, provider.fositeStorage.transactions = store.Recorder(t), store
		reached, release := make(chan struct{}), make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		store.BeforeCommit = func() { close(reached); <-release }
		type outcome struct {
			response *ports.TokenResponse
			err      error
		}
		result := make(chan outcome, 1)
		go func() {
			response, err := provider.HandleClientCredentials(context.Background(), agent.ID.String(), secret, "read")
			result <- outcome{response, err}
		}()
		synctest.Wait()
		select {
		case <-reached:
		default:
			t.Fatal("issuance did not reach an owning transaction commit")
		}
		select {
		case <-result:
			t.Fatal("token response was released before commit completed")
		default:
		}
		close(release)
		released = true
		completed := <-result
		require.NoError(t, completed.err)
		require.Equal(t, "Bearer", completed.response.TokenType)
		require.Len(t, store.Events, 1)
		require.Equal(t, model.BusinessEventTypePrefix+"token-issued", store.Events[0].Type)
	})
}

func TestProviderLedgerReplayDenialPreservesRevocation(t *testing.T) {
	for _, replay := range []string{"authorization-code", "refresh-token"} {
		t.Run(replay, func(t *testing.T) {
			for _, recording := range []string{"available", "unavailable"} {
				t.Run(recording, func(t *testing.T) {
					provider, agents, _ := newTestProvider(t)
					agent, _, secret := setupTestCredentials(t, provider, agents)
					ctx := context.Background()
					redirect := "http://localhost:8080/callback"
					agent.RedirectURIs = []string{redirect}
					require.NoError(t, agents.Update(ctx, agent))
					verifier := "offline-access-verifier-123456789012345678901"
					code, err := provider.HandleAuthorize(ctx, agent.ID.String(), redirect, "code", "read offline_access", "state", generateS256Challenge(verifier), "S256", id.NewPrincipal("ledger-user"))
					require.NoError(t, err)
					issued, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
					require.NoError(t, err)
					live := issued.RefreshToken
					if replay == "refresh-token" {
						rotated, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, issued.RefreshToken, "")
						require.NoError(t, err)
						live = rotated.RefreshToken
					}
					store := &ledgerfixture.Store{}
					if recording == "unavailable" {
						store.AppendError = errors.New("ledger unavailable")
					}
					provider.ledger, provider.fositeStorage.transactions = store.Recorder(t), store
					if replay == "authorization-code" {
						response, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
						require.Error(t, err)
						require.Nil(t, response)
					} else {
						response, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, issued.RefreshToken, "")
						require.Error(t, err)
						require.Nil(t, response)
					}
					signature := provider.refreshStrategy.RefreshTokenSignature(ctx, live)
					retained, err := provider.fositeStorage.refreshRepo.FindBySignature(ctx, signature)
					require.NoError(t, err)
					require.NotNil(t, retained.UsedAt, "failure-event rollback must not restore the compromised token chain")
					if recording == "unavailable" {
						require.Empty(t, store.Events)
					} else {
						require.Len(t, store.Events, 1)
						require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
						require.Equal(t, map[string]any{"reason_code": "authorization_failed"}, store.Events[0].Data)
					}
				})
			}
		})
	}
}
