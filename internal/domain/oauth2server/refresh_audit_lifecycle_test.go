package oauth2server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/stretchr/testify/require"
)

func TestRefreshAudit_KnownTokenWrongClientPreservesRootIdentityWithoutOwnerTransaction(t *testing.T) {
	f := newProviderRefreshFixture(t)
	other, _, otherSecret := setupTestCredentials(t, f.provider, f.agents)
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		ClientIP: "203.0.113.31", UserAgent: "other-client/1.0",
		RequestTarget: "/oauth2/token?refresh_token=do-not-log",
	})
	var logs bytes.Buffer
	f.provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	before, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
	require.NoError(t, err)
	first, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, before.CurrentSignature)
	require.NoError(t, err)
	base := f.provider.refreshDeps.Coordinator
	enteredOwner := false
	f.provider.refreshDeps.Coordinator = &providerRefreshCoordinator{base: base, before: func() error {
		enteredOwner = true
		return errors.New("mismatched client entered owner transaction")
	}}
	response, err := f.provider.HandleRefreshToken(ctx, other.ID.String(), otherSecret, f.issued.RefreshToken, "")
	require.ErrorIs(t, err, ErrInvalidGrant)
	require.Nil(t, response, "the other client cannot receive credentials")
	require.False(t, enteredOwner, "binding rejection must not enter any agent's coordinator")
	root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before, root)
	token, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, before.CurrentSignature)
	require.NoError(t, err)
	require.Equal(t, first, token)
	events := refreshTransitionEvents(t, &logs, "RefreshRejected")
	require.Len(t, events, 1)
	require.Equal(t, "client_binding", events[0]["reason"])
	require.Equal(t, before.ID.String(), events[0]["session_id"])
	require.Equal(t, before.Principal.String(), events[0]["principal"])
	require.Equal(t, before.AgentID.String(), events[0]["agent_id"])
	require.Equal(t, before.ClientID.String(), events[0]["client_id"], "root identity, not the presenting client, owns this session")
	require.Equal(t, other.ID.String(), events[0]["presented_client_id"])
	require.Equal(t, "203.0.113.31", events[0]["client_ip"])
	require.Equal(t, "other-client/1.0", events[0]["user_agent"])
	require.NotContains(t, logs.String(), f.issued.RefreshToken)
	require.NotContains(t, logs.String(), otherSecret)
	require.NotContains(t, logs.String(), "do-not-log")

	f.provider.refreshDeps.Coordinator = base
	ownerResult, err := f.provider.HandleRefreshToken(ctx, before.ClientID.String(), f.secret, f.issued.RefreshToken, "")
	require.NoError(t, err, "bound owner must still be able to rotate its original token")
	require.NotNil(t, ownerResult)
	require.NotEmpty(t, ownerResult.RefreshToken)
}

func TestRefreshAudit_UnknownTokenNeverInventsSessionIdentity(t *testing.T) {
	f := newProviderRefreshFixture(t)
	var logs bytes.Buffer
	f.provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	unknown := "unrecognized-refresh-credential"
	response, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, unknown, "")
	require.ErrorIs(t, err, ErrInvalidGrant)
	require.Nil(t, response)
	events := refreshTransitionEvents(t, &logs, "RefreshRejected")
	require.Len(t, events, 1)
	require.Equal(t, f.agent.ID.String(), events[0]["client_id"])
	for _, identity := range []string{"session_id", "principal", "agent_id"} {
		require.NotContains(t, events[0], identity, "no root can be inferred from an unknown token")
	}
	require.False(t, strings.Contains(logs.String(), unknown))
	require.NotContains(t, logs.String(), f.issued.RefreshToken)
	ownerResult, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, ownerResult.RefreshToken)
}

func TestRefreshAudit_ProhibitedReuseLogsOnlyAfterCommittedRevocation(t *testing.T) {
	f := newProviderRetryFixture(t)
	rotateProviderRetry(t, f, context.Background(), "read")
	before := providerRetryRoot(t, f)
	f.decideAt(before.LastFreshAt.Add(time.Second))
	var logs bytes.Buffer
	f.provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	base := f.provider.refreshDeps.Coordinator
	f.provider.refreshDeps.Coordinator = &credentialLifecycleFaultCoordinator{
		AuthorizationSessionCoordinator: base, afterErr: errors.New("confirmed owner rollback"),
	}
	response, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "write")
	require.ErrorIs(t, err, ErrServerError)
	require.Nil(t, response)
	require.Empty(t, refreshTransitionEvents(t, &logs, "RefreshSessionRevoked"))
	require.Equal(t, before, providerRetryRoot(t, f), "failed commit preserves the owner session")

	f.provider.refreshDeps.Coordinator = base
	logs.Reset()
	requireProhibitedProviderRetry(t, f, context.Background(), f.issued.RefreshToken, "write")
	events := refreshTransitionEvents(t, &logs, "RefreshSessionRevoked")
	require.Len(t, events, 1)
	require.Equal(t, "prohibited_reuse", events[0]["reason"])
	require.Equal(t, before.ID.String(), events[0]["session_id"])
	require.Equal(t, before.Principal.String(), events[0]["principal"])
	require.Equal(t, before.AgentID.String(), events[0]["agent_id"])
	require.Equal(t, before.ClientID.String(), events[0]["client_id"])
}

func TestRefreshAudit_CodeReplayLogsOriginalOwnerOnlyAfterCommit(t *testing.T) {
	provider, agents, _ := newTestProvider(t)
	owner, _, secret := setupTestCredentials(t, provider, agents)
	owner.RedirectURIs = []string{providerRefreshRedirect}
	owner.AllowedScopes = []string{"offline_access", "read"}
	require.NoError(t, agents.Update(context.Background(), owner))
	other, _, otherSecret := setupTestCredentials(t, provider, agents)
	const verifier = "refresh-audit-code-replay-verifier-123456789012345"
	ctx := context.Background()
	code, err := provider.HandleAuthorize(ctx, owner.ID.String(), providerRefreshRedirect, "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", id.NewPrincipal("user@example.com"))
	require.NoError(t, err)
	issued, err := provider.HandleAuthorizationCodeExchange(ctx, owner.ID.String(), secret, code, providerRefreshRedirect, verifier)
	require.NoError(t, err)
	require.NotEmpty(t, issued.RefreshToken)
	record, err := provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	rootID, err := id.ParseRefreshSessionID(record.ID.String())
	require.NoError(t, err)
	before, err := provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	require.NoError(t, err)
	var logs bytes.Buffer
	provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	base := provider.refreshDeps.Coordinator
	provider.refreshDeps.Coordinator = &credentialLifecycleFaultCoordinator{
		AuthorizationSessionCoordinator: base, afterErr: errors.New("confirmed replay rollback"),
	}
	response, err := provider.HandleAuthorizationCodeExchange(ctx, other.ID.String(), otherSecret, code, providerRefreshRedirect, verifier)
	require.ErrorIs(t, err, ErrServerError)
	require.Nil(t, response)
	require.Empty(t, refreshTransitionEvents(t, &logs, "RefreshSessionRevoked"))
	unchanged, err := provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	require.NoError(t, err)
	require.Equal(t, before, unchanged)

	provider.refreshDeps.Coordinator = base
	logs.Reset()
	response, err = provider.HandleAuthorizationCodeExchange(ctx, other.ID.String(), otherSecret, code, providerRefreshRedirect, verifier)
	require.ErrorIs(t, err, ErrInvalidGrant)
	require.Nil(t, response)
	events := refreshTransitionEvents(t, &logs, "RefreshSessionRevoked")
	require.Len(t, events, 1)
	require.Equal(t, "code_replay", events[0]["reason"])
	require.Equal(t, rootID.String(), events[0]["session_id"])
	require.Equal(t, before.Principal.String(), events[0]["principal"])
	require.Equal(t, owner.ID.String(), events[0]["agent_id"])
	require.Equal(t, before.ClientID.String(), events[0]["client_id"])
	require.NotContains(t, logs.String(), code)
	require.NotContains(t, logs.String(), secret)
	require.NotContains(t, logs.String(), otherSecret)
	logs.Reset()
	_, err = provider.HandleAuthorizationCodeExchange(ctx, other.ID.String(), otherSecret, code, providerRefreshRedirect, verifier)
	require.ErrorIs(t, err, ErrInvalidGrant)
	require.Empty(t, refreshTransitionEvents(t, &logs, "RefreshSessionRevoked"), "repeated replay does not claim a second terminal transition")
}
