package memory

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/require"
)

type memoryIntegrityGrantVerifier struct{ grants *UserGrantRepository }

func (v memoryIntegrityGrantVerifier) VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time) (ports.UserDelegationDecision, error) {
	grant, err := v.grants.FindByPrincipalAndAgent(ctx, principal, agentID)
	if ports.IsNotFoundErr(err) {
		return ports.UserDelegationDecision{Status: ports.UserDelegationMissing}, nil
	}
	if err != nil {
		return ports.UserDelegationDecision{}, err
	}
	if !grant.IsActive(at) {
		return ports.UserDelegationDecision{Status: ports.UserDelegationExpired}, nil
	}
	return ports.UserDelegationDecision{Status: ports.UserDelegationActive, GrantID: grant.ID, ValidUntil: grant.ValidUntil}, nil
}

func TestMemoryProviderRejectsCorruptedIntermediateBeforeFreshAndRetry(t *testing.T) {
	ctx := context.Background()
	agents := NewAgentRepository()
	grants := NewUserGrantRepository()
	credentials := NewClientCredentialStore()
	codes := NewAuthorizationCodeStore()
	pkce := NewPKCESessionStore()
	store := NewRefreshSessionStore(agents, grants, credentials, codes, pkce)
	cipher := testutil.NewTestEncryptionAdapter(t)
	keys := NewSigningKeyStore()
	logger := slog.Default()
	branchKeys := &noop.BranchKeyManager{}
	signer := oauth2server.NewSigningKeyService(keys, keys, cipher, branchKeys, logger)
	_, _, err := signer.EnsureInitialKey(ctx, "ES256")
	require.NoError(t, err)
	provider, err := oauth2server.NewProvider(codes, oauth2server.RefreshSessionDependencies{
		Agents: agents, Sessions: store, Tokens: NewRefreshTokenStore(store), Revocations: store,
		Coordinator: store, Clock: store, Verifier: memoryIntegrityGrantVerifier{grants: grants},
		Encryption: cipher, BranchKeys: branchKeys,
		Policy: storage.RefreshSessionPolicy{ReuseInterval: 30 * time.Second, InactivityLifetime: time.Hour},
	}, pkce, credentials, oauth2.NewAgentClientResolver(agents, logger), signer,
		"https://broker.example.com", time.Hour, "", logger, store)
	require.NoError(t, err)

	const redirect = "http://localhost:8080/callback"
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Lineage test agent", Description: "Integrity test",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}},
		RedirectURIs:   []string{redirect}, AllowedScopes: []string{"offline_access", "read"}}
	require.NoError(t, agents.Create(ctx, agent))
	require.NoError(t, grants.Create(ctx, &storage.UserGrant{
		ID: id.NewGrantID(), AgentID: agent.ID, Principal: id.NewPrincipal("owner@example.test"),
	}))
	credential, secret, err := provider.ClientAuth().GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, credentials.Create(ctx, credential))
	verifier := "integrity-pkce-verifier-012345678901234567890"
	challengeHash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])
	code, err := provider.HandleAuthorize(ctx, agent.ID.String(), redirect, "code", "offline_access read", "state", challenge, "S256", id.NewPrincipal("owner@example.test"))
	require.NoError(t, err)
	issued, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
	require.NoError(t, err)
	require.NotEmpty(t, issued.RefreshToken)
	roots, err := store.ListActive(ctx, id.RefreshSessionID{}, 10)
	require.NoError(t, err)
	require.Len(t, roots, 1)
	rootID := roots[0].ID

	raw := []string{issued.RefreshToken}
	for range 3 {
		result, refreshErr := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, raw[len(raw)-1], "")
		require.NoError(t, refreshErr)
		require.NotEmpty(t, result.AccessToken)
		require.NotEmpty(t, result.RefreshToken)
		raw = append(raw, result.RefreshToken)
	}
	previousResponse, err := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, raw[2], "")
	require.NoError(t, err, "the immediately previous token should recover its original pair in intact history")
	require.True(t, raw[3] == previousResponse.RefreshToken, "intact lineage must recover the identical successor")
	before, err := store.FindByID(ctx, rootID)
	require.NoError(t, err)
	currentSig := memoryIntegritySignature(raw[3])
	previousSig := memoryIntegritySignature(raw[2])
	middleSig := memoryIntegritySignature(raw[1])
	current, err := NewRefreshTokenStore(store).FindBySignature(ctx, currentSig)
	require.NoError(t, err)
	previous, err := NewRefreshTokenStore(store).FindBySignature(ctx, previousSig)
	require.NoError(t, err)
	require.NotNil(t, store.legacy[middleSig], "an intermediate anchored mirror must exist before corruption")
	store.mu.Lock()
	store.legacy[middleSig].ClientID = id.NewClientID("other-client")
	store.mu.Unlock()

	for _, token := range []string{raw[3], raw[2]} {
		response, rejectErr := provider.HandleRefreshToken(ctx, agent.ID.String(), secret, token, "")
		require.True(t, response == nil, "neither new authority nor an encrypted stored result may escape")
		require.True(t, errors.Is(rejectErr, oauth2server.ErrInvalidGrant), "corrupt middle ancestry must be invalid_grant: %v", rejectErr)
	}
	after, err := store.FindByID(ctx, rootID)
	require.NoError(t, err)
	require.Equal(t, before, after, "denied requests must not revoke, increment retries, or alter session clocks")
	storedCurrent, err := NewRefreshTokenStore(store).FindBySignature(ctx, currentSig)
	require.NoError(t, err)
	require.Equal(t, current, storedCurrent)
	storedPrevious, err := NewRefreshTokenStore(store).FindBySignature(ctx, previousSig)
	require.NoError(t, err)
	require.Equal(t, previous, storedPrevious)
	require.Nil(t, store.legacy[currentSig].UsedAt)
}
