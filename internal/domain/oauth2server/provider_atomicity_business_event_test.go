package oauth2server

import (
	"context"
	"errors"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type failingIssuanceEvents struct {
	ports.BusinessEventRepository
	fail bool
}

func (r *failingIssuanceEvents) Append(ctx context.Context, event *model.BusinessEvent, copyEnabled bool) error {
	if r.fail && event.Type == model.BusinessEventTypePrefix+"token-issued" {
		return errors.New("issuance recording unavailable")
	}
	return r.BusinessEventRepository.Append(ctx, event, copyEnabled)
}

func TestProviderLedgerResponsePopulationRollsBack(t *testing.T) {
	for _, action := range []string{"authorization-code", "refresh-token"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			manager := memory.NewTransactionManager()
			registry, err := ledger.NewRegistry(eventschemas.Schemas)
			require.NoError(t, err)
			events := &failingIssuanceEvents{BusinessEventRepository: memory.NewBusinessEventRepository(manager, registry)}
			recorder := ledger.NewService(registry, events, nil, manager, false)
			keys := memory.NewSigningKeyStore(manager)
			keyService := NewSigningKeyService(keys, keys, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger(), recorder)
			_, err = keyService.generateAndStore(ctx, "ES256", true, time.Now())
			require.NoError(t, err)
			agents := memory.NewAgentRepository(manager)
			codes := memory.NewAuthorizationCodeStore(manager)
			refresh := memory.NewRefreshTokenSessionStore(manager)
			pkce := memory.NewPKCESessionStore(manager)
			provider, err := NewProvider(codes, refresh, pkce, memory.NewClientCredentialStore(manager), &testClientResolver{agentRepo: agents}, keyService, "https://broker.example.com", time.Hour, time.Hour, "", testSlogger(), recorder, manager)
			require.NoError(t, err)
			agent, _, secret := setupTestCredentials(t, provider, agents)
			redirect, verifier := "http://localhost:8080/callback", "atomic-verifier-123456789012345678901234567890"
			agent.RedirectURIs = []string{redirect}
			require.NoError(t, agents.Update(ctx, agent))
			principal := id.Principal("atomic-user")
			code, err := provider.HandleAuthorize(ctx, agent.ID.String(), redirect, "code", "read offline_access", "state", generateS256Challenge(verifier), "S256", principal)
			require.NoError(t, err)
			codeSignature := provider.authCodeHandler.AuthorizeCodeStrategy.AuthorizeCodeSignature(ctx, code)
			var refreshToken string
			if action == "refresh-token" {
				response, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
				require.NoError(t, err)
				refreshToken = response.RefreshToken
			}
			events.fail = true
			if action == "authorization-code" {
				response, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
				require.Error(t, err)
				require.Nil(t, response)
				retained, err := codes.FindByCodeHash(ctx, codeSignature)
				require.NoError(t, err)
				require.Nil(t, retained.UsedAt)
				_, err = pkce.FindBySignature(ctx, codeSignature)
				require.NoError(t, err, "failed recording must restore the deleted PKCE challenge")
			} else {
				response, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, refreshToken, "")
				require.Error(t, err)
				require.Nil(t, response)
				retained, err := refresh.FindBySignature(ctx, provider.refreshStrategy.RefreshTokenSignature(ctx, refreshToken))
				require.NoError(t, err)
				require.Nil(t, retained.UsedAt)
			}
			events.fail = false
			if action == "authorization-code" {
				_, err = provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
			} else {
				_, err = provider.HandleRefreshToken(ctx, agent.ID.String(), secret, refreshToken, "")
			}
			require.NoError(t, err, "rolled-back input must remain usable for a fresh successful request")
			retainedEvents, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: principal}, Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
			require.NoError(t, err)
			var issued, failed int
			for _, event := range retainedEvents {
				switch event.Type {
				case model.BusinessEventTypePrefix + "token-issued":
					issued++
				case model.BusinessEventTypePrefix + "token-request-failed":
					failed++
					require.Equal(t, map[string]any{"reason_code": "internal_failure"}, event.Data)
				}
			}
			wantIssued := 1
			if action == "refresh-token" {
				wantIssued = 2
			}
			require.Equal(t, wantIssued, issued)
			require.Equal(t, 1, failed, "only the independently committed terminal failure survives the aborted issuance")
			if action == "authorization-code" {
				code, err = provider.HandleAuthorize(ctx, agent.ID.String(), redirect, "code", "read", "state", generateS256Challenge(verifier), "S256", principal)
				require.NoError(t, err)
				_, err = provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, "wrong-verifier-12345678901234567890123456789012")
				require.ErrorIs(t, err, ErrInvalidGrant)
				_, err = pkce.FindBySignature(ctx, provider.authCodeHandler.AuthorizeCodeStrategy.AuthorizeCodeSignature(ctx, code))
				require.True(t, ports.IsNotFoundErr(err), "PKCE rejection must retain its existing one-shot consumption")
				_, err = provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
				require.ErrorIs(t, err, ErrInvalidGrant)
				failures, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{NoSubject: true}, Type: model.BusinessEventTypePrefix + "token-request-failed", Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
				require.NoError(t, err)
				require.Len(t, failures, 2)
				for _, failure := range failures {
					require.Equal(t, map[string]any{"reason_code": "authorization_failed"}, failure.Data)
				}
			}
		})
	}
}
