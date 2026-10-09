package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConsentVerifier struct {
	*consent.Service
	grants *memory.UserGrantRepository
}

func newTestGrantVerifier(agents ports.AgentRepository) *testConsentVerifier {
	grants := memory.NewUserGrantRepository()
	return &testConsentVerifier{consent.NewService(agents, nil, grants, nil, memory.NewRefreshTokenSessionStore(), nil, testSlogger()), grants}
}

func seedTestGrant(t *testing.T, verifier grantVerifier, agentID id.AgentID, principal id.Principal) {
	t.Helper()
	require.NoError(t, verifier.(*testConsentVerifier).grants.Create(context.Background(), &storage.UserGrant{
		ID: id.NewGrantID(), AgentID: agentID, Principal: principal, CreatedAt: time.Now().UTC(),
	}))
}

type failingGrantVerifier struct {
	err error
}

func (v failingGrantVerifier) VerifyAgentAccess(context.Context, id.Principal, id.AgentID) (*storage.UserGrant, error) {
	return nil, v.err
}

type failingRefreshLookup struct {
	ports.RefreshTokenSessionRepository
}

func (f failingRefreshLookup) FindBySignature(context.Context, string) (*storage.RefreshTokenSession, error) {
	return nil, storage.NewStorageError("refresh lookup", storage.ErrorKindConnection, nil, "database unavailable")
}

type failingRefreshRevocation struct {
	ports.RefreshTokenSessionRepository
}

func (f failingRefreshRevocation) RevokeByRequestID(context.Context, string) error {
	return storage.NewStorageError("refresh revocation", storage.ErrorKindConnection, nil, "database unavailable")
}

type refreshRotationHook struct {
	ports.RefreshTokenSessionRepository
	beforeMarkUsed func() error
}

func (r refreshRotationHook) MarkUsed(ctx context.Context, signature string) error {
	if err := r.beforeMarkUsed(); err != nil {
		return err
	}
	return r.RefreshTokenSessionRepository.MarkUsed(ctx, signature)
}

func TestProvider_RefreshContinuingConsent(t *testing.T) {
	for _, name := range []string{"active", "missing", "expired", "mismatch", "legacy older", "legacy newer", "legacy equal UTC", "verifier failure", "storage failure", "revocation failure", "grant replaced during rotation"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			provider, agents, _ := newTestProvider(t)
			agent, _, secret := setupTestCredentials(t, provider, agents)
			agent.RedirectURIs = []string{"https://client.example.com/callback"}
			require.NoError(t, agents.Update(ctx, agent))
			grants := memory.NewUserGrantRepository()
			grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: id.Principal("user@example.com"), CreatedAt: time.Now().UTC().Add(-time.Hour)}
			require.NoError(t, grants.Create(ctx, grant))
			provider.fositeStorage.verifier = consent.NewService(agents, nil, grants, nil, provider.fositeStorage.refreshRepo, nil, testSlogger())
			verifier := "refresh-consent-verifier-12345678901234567890"
			code, err := provider.HandleAuthorize(ctx, agent.ID.String(), agent.RedirectURIs[0], "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", grant.Principal)
			require.NoError(t, err)
			issued, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, agent.RedirectURIs[0], verifier)
			require.NoError(t, err)
			repo := provider.fositeStorage.refreshRepo
			signature := sha256Hex(issued.RefreshToken)
			row, err := repo.FindBySignature(ctx, signature)
			require.NoError(t, err)
			assert.Equal(t, &grant.ID, row.GrantID)
			other := *row
			other.Signature, other.RequestID = "other-chain", "other-request"
			require.NoError(t, repo.Create(ctx, &other))
			chain := *row
			chain.Signature = "same-chain"
			require.NoError(t, repo.Create(ctx, &chain))
			var logs bytes.Buffer
			provider.fositeStorage.logger = slog.New(slog.NewJSONHandler(&logs, nil))

			expected := error(nil)
			switch name {
			case "missing", "mismatch":
				_, err := grants.DeleteByPrincipalAndAgentID(ctx, grant.Principal, agent.ID)
				require.NoError(t, err)
				if name == "mismatch" {
					grant.ID = id.NewGrantID()
					require.NoError(t, grants.Create(ctx, grant))
				}
				expected = ErrInvalidGrant
			case "expired":
				grant.ValidUntil = ptr.To(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
				require.NoError(t, grants.Update(ctx, grant))
				expected = ErrInvalidGrant
			case "legacy older", "legacy newer", "legacy equal UTC":
				legacyRepo := memory.NewRefreshTokenSessionStore()
				row.GrantID = nil
				switch name {
				case "legacy newer":
					row.CreatedAt = grant.CreatedAt.Add(-time.Microsecond)
					expected = ErrInvalidGrant
				case "legacy equal UTC":
					row.CreatedAt = grant.CreatedAt.In(time.FixedZone("Berlin summer", 2*60*60))
				}
				require.NoError(t, legacyRepo.Create(ctx, row))
				require.NoError(t, legacyRepo.Create(ctx, &other))
				require.NoError(t, legacyRepo.Create(ctx, &chain))
				repo, provider.fositeStorage.refreshRepo = legacyRepo, legacyRepo
			case "verifier failure":
				provider.fositeStorage.verifier = failingGrantVerifier{err: errors.New("verifier unavailable")}
				expected = ErrServerError
			case "storage failure":
				provider.fositeStorage.refreshRepo = failingRefreshLookup{repo}
				expected = ErrServerError
			case "revocation failure":
				_, err := grants.DeleteByPrincipalAndAgentID(ctx, grant.Principal, agent.ID)
				require.NoError(t, err)
				provider.fositeStorage.refreshRepo = failingRefreshRevocation{repo}
				expected = ErrServerError
			case "grant replaced during rotation":
				replacement := *grant
				replacement.ID = id.NewGrantID()
				provider.fositeStorage.refreshRepo = refreshRotationHook{repo, func() error {
					if _, err := grants.DeleteByPrincipalAndAgentID(ctx, grant.Principal, agent.ID); err != nil {
						return err
					}
					return grants.Create(ctx, &replacement)
				}}
			}

			rotated, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, issued.RefreshToken, "")
			if expected == nil {
				require.NoError(t, err)
				successor, err := repo.FindBySignature(ctx, sha256Hex(rotated.RefreshToken))
				require.NoError(t, err)
				assert.Equal(t, &grant.ID, successor.GrantID)
				assert.Equal(t, row.RequestID, successor.RequestID)
			} else {
				require.ErrorIs(t, err, expected)
				assert.Nil(t, rotated)
				var protocolErr *RFC6749Error
				require.ErrorAs(t, err, &protocolErr)
				assert.Equal(t, expected.Error(), protocolErr.Code())
				wantStatus := http.StatusInternalServerError
				if expected == ErrInvalidGrant {
					wantStatus = http.StatusBadRequest
					assert.Equal(t, translateFositeError(fosite.ErrInvalidGrant).(*RFC6749Error).Description(), protocolErr.Description())
					var audit map[string]any
					require.NoError(t, json.Unmarshal(logs.Bytes(), &audit))
					reason := "grant_mismatch"
					switch name {
					case "missing":
						reason = "grant_missing"
					case "expired":
						reason = "grant_expired"
					}
					assert.Equal(t, reason, audit["reason"])
				}
				assert.Equal(t, wantStatus, protocolErr.HTTPStatus())
			}
			for _, sig := range []string{signature, chain.Signature} {
				stored, err := repo.FindBySignature(ctx, sig)
				require.NoError(t, err)
				if expected == ErrServerError || expected == nil && sig == chain.Signature {
					assert.Nil(t, stored.UsedAt)
				} else {
					assert.NotNil(t, stored.UsedAt)
				}
			}
			untouched, err := repo.FindBySignature(ctx, other.Signature)
			require.NoError(t, err)
			assert.Nil(t, untouched.UsedAt)
			if name == "grant replaced during rotation" {
				provider.fositeStorage.refreshRepo = repo
				again, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, rotated.RefreshToken, "")
				require.ErrorIs(t, err, ErrInvalidGrant)
				assert.Nil(t, again)
				successor, err := repo.FindBySignature(ctx, sha256Hex(rotated.RefreshToken))
				require.NoError(t, err)
				assert.NotNil(t, successor.UsedAt)
			}
			assert.NotContains(t, logs.String(), issued.RefreshToken)
			assert.NotContains(t, logs.String(), signature)
		})
	}
}

type refreshRollbackTransactions struct {
	rollbackErr error
	rolledBack  bool
}

func (tx *refreshRollbackTransactions) BeginTX(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (tx *refreshRollbackTransactions) Commit(context.Context) error {
	return nil
}

func (tx *refreshRollbackTransactions) Rollback(context.Context) error {
	tx.rolledBack = true
	return tx.rollbackErr
}

func TestProvider_RefreshConsentRevokedBeforeRotation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rollbackErr error
		wantCode    string
		wantStatus  int
	}{
		{"revocation wins", nil, "invalid_grant", http.StatusBadRequest},
		{"rollback fails", errors.New("rollback unavailable"), "server_error", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			provider, agents, _ := newTestProvider(t)
			agent, _, secret := setupTestCredentials(t, provider, agents)
			agent.RedirectURIs = []string{"https://client.example.com/callback"}
			require.NoError(t, agents.Update(ctx, agent))
			principal := id.Principal("user@example.com")
			repo := provider.fositeStorage.refreshRepo
			grants := provider.fositeStorage.verifier.(*testConsentVerifier).grants
			consentService := consent.NewService(agents, nil, grants, nil, repo, nil, testSlogger())
			provider.fositeStorage.verifier = consentService
			verifier := "refresh-consent-verifier-12345678901234567890"
			code, err := provider.HandleAuthorize(ctx, agent.ID.String(), agent.RedirectURIs[0], "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", principal)
			require.NoError(t, err)
			issued, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, agent.RedirectURIs[0], verifier)
			require.NoError(t, err)

			rotationReady, resumeRotation := make(chan struct{}), make(chan struct{})
			provider.fositeStorage.refreshRepo = refreshRotationHook{repo, func() error {
				close(rotationReady)
				select {
				case <-resumeRotation:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}}
			tx := &refreshRollbackTransactions{rollbackErr: tc.rollbackErr}
			provider.fositeStorage.transactions = tx
			type result struct {
				response *ports.TokenResponse
				err      error
			}
			completed := make(chan result, 1)
			go func() {
				response, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, issued.RefreshToken, "")
				completed <- result{response, err}
			}()
			select {
			case <-rotationReady:
			case <-ctx.Done():
				t.Fatal("refresh did not reach rotation")
			}
			require.NoError(t, consentService.RevokeConsentForPrincipal(ctx, principal, agent.ID))
			close(resumeRotation)
			select {
			case got := <-completed:
				assert.Nil(t, got.response)
				var protocolErr *RFC6749Error
				require.ErrorAs(t, got.err, &protocolErr)
				assert.Equal(t, tc.wantCode, protocolErr.Code())
				assert.Equal(t, tc.wantStatus, protocolErr.HTTPStatus())
				if tc.rollbackErr == nil {
					assert.ErrorIs(t, got.err, ErrInvalidGrant)
					assert.Equal(t, translateFositeError(fosite.ErrInvalidGrant).(*RFC6749Error).Description(), protocolErr.Description())
				}
			case <-ctx.Done():
				t.Fatal("refresh did not complete after revocation")
			}
			assert.True(t, tx.rolledBack)
			stored, err := repo.FindBySignature(ctx, sha256Hex(issued.RefreshToken))
			require.NoError(t, err)
			assert.NotNil(t, stored.UsedAt)
		})
	}
}

func TestProvider_RefreshIssuanceRequiresConsent(t *testing.T) {
	for _, name := range []string{"missing", "expired", "verifier failure"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			provider, agents, _ := newTestProvider(t)
			agent, _, secret := setupTestCredentials(t, provider, agents)
			agent.RedirectURIs = []string{"https://client.example.com/callback"}
			require.NoError(t, agents.Update(ctx, agent))
			grants := memory.NewUserGrantRepository()
			provider.fositeStorage.verifier = consent.NewService(agents, nil, grants, nil, provider.fositeStorage.refreshRepo, nil, testSlogger())
			if name == "expired" {
				require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: id.Principal("user@example.com"), ValidUntil: ptr.To(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))}))
			}
			expected := ErrInvalidGrant
			if name == "verifier failure" {
				provider.fositeStorage.verifier = failingGrantVerifier{err: errors.New("verifier unavailable")}
				expected = ErrServerError
			}
			verifier := "refresh-consent-verifier-12345678901234567890"
			code, err := provider.HandleAuthorize(ctx, agent.ID.String(), agent.RedirectURIs[0], "code", "offline_access", "state", generateS256Challenge(verifier), "S256", id.Principal("user@example.com"))
			require.NoError(t, err)
			issued, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, agent.RedirectURIs[0], verifier)
			require.ErrorIs(t, err, expected)
			var protocolErr *RFC6749Error
			require.ErrorAs(t, err, &protocolErr)
			assert.Equal(t, expected.Error(), protocolErr.Code())
			wantStatus := http.StatusBadRequest
			if expected == ErrServerError {
				wantStatus = http.StatusInternalServerError
			}
			assert.Equal(t, wantStatus, protocolErr.HTTPStatus())
			assert.Nil(t, issued)
		})
	}
}
