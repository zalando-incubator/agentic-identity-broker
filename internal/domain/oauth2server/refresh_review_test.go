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

	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type preflightBarrierCoordinator struct {
	base    ports.AuthorizationSessionCoordinator
	entered chan struct{}
	proceed chan struct{}
}

func (c preflightBarrierCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	select {
	case <-c.proceed:
		return c.base.Run(ctx, agentID, operation)
	default:
	}
	err := c.base.Run(ctx, agentID, operation)
	c.entered <- struct{}{}
	<-c.proceed
	return err
}

func TestRefreshAudit_OverlappingUnusedPreflightCountsStoredResult(t *testing.T) {
	f := newProviderRetryFixture(t)
	var logs bytes.Buffer
	f.provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	entered, proceed := make(chan struct{}, 2), make(chan struct{})
	coordinator := preflightBarrierCoordinator{base: f.provider.refreshDeps.Coordinator, entered: entered, proceed: proceed}
	f.provider.refreshDeps.Coordinator = coordinator
	f.provider.fositeStorage.refresh.Coordinator = coordinator
	type result struct {
		value *ports.TokenResponse
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			value, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read")
			results <- result{value, err}
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("both requests must preflight the unused predecessor")
		}
	}
	close(proceed)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	requireOriginalRetryResult(t, first.value, second.value)
	events := map[string]int{}
	decoder := json.NewDecoder(&logs)
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if event, ok := record["event"].(string); ok {
			events[event]++
		}
	}
	require.Equal(t, 1, events["RefreshRotated"], "one request commits fresh rotation")
	require.Equal(t, 1, events["RefreshRetryAccepted"], "the overlapping request commits a stored-result authorization")
}

type retryDecryptBarrier struct {
	ports.EncryptionPort
	entered chan struct{}
	proceed chan struct{}
}

func (e retryDecryptBarrier) Decrypt(ctx context.Context, ciphertext []byte, aad map[string]string) ([]byte, error) {
	e.entered <- struct{}{}
	<-e.proceed
	return e.EncryptionPort.Decrypt(ctx, ciphertext, aad)
}

func TestRefreshRetry_ThreeConcurrentCurrentUsesRecoverWithinBound(t *testing.T) {
	f := newProviderRetryFixture(t)
	prepared, release := make(chan struct{}, 3), make(chan struct{})
	coordinator := preflightBarrierCoordinator{base: f.provider.refreshDeps.Coordinator, entered: prepared, proceed: release}
	f.provider.refreshDeps.Coordinator = coordinator
	f.provider.fositeStorage.refresh.Coordinator = coordinator
	opened, returnRetries := make(chan struct{}, 2), make(chan struct{})
	cipher := retryDecryptBarrier{EncryptionPort: f.provider.refreshDeps.Encryption, entered: opened, proceed: returnRetries}
	f.provider.refreshDeps.Encryption = cipher
	f.provider.fositeStorage.refresh.Encryption = cipher
	type outcome struct {
		response *ports.TokenResponse
		err      error
	}
	results := make(chan outcome, 3)
	for range 3 {
		go func() {
			response, err := f.provider.HandleRefreshToken(t.Context(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read")
			results <- outcome{response, err}
		}()
	}
	for range 3 {
		select {
		case <-prepared:
		case <-time.After(3 * time.Second):
			t.Fatal("all requests must snapshot the unused token before mutation")
		}
	}
	close(release)
	first := <-results
	for range 2 {
		select {
		case <-opened:
		case <-time.After(3 * time.Second):
			t.Fatal("both retry candidates must decrypt the same count-zero result")
		}
	}
	close(returnRetries)
	require.NoError(t, first.err)
	for range 2 {
		retry := <-results
		require.NoError(t, retry.err)
		requireOriginalRetryResult(t, first.response, retry.response)
	}
	require.Equal(t, 2, providerRetryRoot(t, f).RetryCount)
}

func TestSessionCleanup_HeldAgentGateCannotKeepOverdueReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, before, _ := newCleanupRefreshFixture(t, 0)
		now := *before.PreviousConsumedAt
		cleanup := newCleanupWorker(f, &now, slog.Default())
		clock := cleanupAdvancingClock{from: now, started: time.Now()}
		cleanup.refresh.Clock = clock
		cleanup.refresh.Coordinator = cleanupClockCoordinator{base: f.provider.refreshDeps.Coordinator, clock: clock}
		require.NoError(t, cleanup.Reconcile(t.Context()))
		held, release, ownerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			ownerDone <- f.provider.refreshDeps.Coordinator.Run(t.Context(), f.agent.ID, func(context.Context, time.Time) error { close(held); <-release; return nil })
		}()
		<-held
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan struct{})
		go func() { defer close(finished); cleanup.Run(ctx) }()
		defer func() { cancel(); <-finished }()
		time.Sleep(before.RetryExpiresAt.Sub(now) + time.Second)
		synctest.Wait()
		readinessErr := cleanup.HealthCheck(ctx)
		close(release)
		require.NoError(t, <-ownerDone)
		require.Error(t, readinessErr, "held owner gate cannot preserve ready health past live erasure bound")
		time.Sleep(time.Second)
		synctest.Wait()
		after := cleanupRoot(t, f, before.ID)
		require.Empty(t, after.RetryCiphertext)
		require.NoError(t, cleanup.HealthCheck(ctx))
	})
}

type gateProbeEncryption struct {
	base   ports.EncryptionPort
	prove  func() error
	sealed int
	opened int
}

func (e *gateProbeEncryption) Encrypt(ctx context.Context, plaintext []byte, aad map[string]string) ([]byte, error) {
	if err := e.prove(); err != nil {
		return nil, err
	}
	e.sealed++
	return e.base.Encrypt(ctx, plaintext, aad)
}

func (e *gateProbeEncryption) Decrypt(ctx context.Context, ciphertext []byte, aad map[string]string) ([]byte, error) {
	if err := e.prove(); err != nil {
		return nil, err
	}
	e.opened++
	return e.base.Decrypt(ctx, ciphertext, aad)
}

type gateProbeBranchKeys struct {
	prove func() error
	calls int
}

func (k *gateProbeBranchKeys) Create(ctx context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	if err := k.prove(); err != nil {
		return "", err
	}
	k.calls++
	return testRefreshBranchKeyManager{}.Create(ctx, subject)
}

type gateProbeCredentials struct {
	ports.ClientCredentialRepository
	prove func() error
	reads int
}

func (r *gateProbeCredentials) GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	if err := r.prove(); err != nil {
		return nil, err
	}
	r.reads++
	return r.ClientCredentialRepository.GetByAgentID(ctx, agentID)
}

func TestRefreshPreflight_ExternalCryptoAndBranchKeysDoNotHoldAgentGate(t *testing.T) {
	f := newProviderRetryFixture(t)
	base := f.provider.refreshDeps.Coordinator
	prove := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return base.Run(ctx, f.agent.ID, func(context.Context, time.Time) error { return nil })
	}
	credentials := &gateProbeCredentials{ClientCredentialRepository: f.provider.clientAuth.credentialRepo, prove: prove}
	f.provider.clientAuth.credentialRepo = credentials
	keys := &gateProbeBranchKeys{prove: prove}
	f.provider.refreshDeps.BranchKeys = keys
	f.provider.fositeStorage.refresh.BranchKeys = keys
	cipher := &gateProbeEncryption{base: f.provider.refreshDeps.Encryption, prove: prove}
	f.provider.refreshDeps.Encryption = cipher
	f.provider.fositeStorage.refresh.Encryption = cipher
	original := rotateProviderRetry(t, f, context.Background(), "read")
	require.Equal(t, 1, cipher.sealed)
	root := providerRetryRoot(t, f)
	f.decideAt(root.LastFreshAt.Add(time.Second))
	recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read")
	require.NoError(t, err)
	requireOriginalRetryResult(t, original, recovered)
	require.Equal(t, 1, cipher.opened)
	require.Equal(t, 2, keys.calls)
	require.Equal(t, 2, credentials.reads, "each refresh verifies its secret once before the guard")
}

func TestRefreshPreflight_BranchKeyFailurePreservesCurrentToken(t *testing.T) {
	f := newProviderRetryFixture(t)
	broken := &mockBranchKeyManager{createFn: func(context.Context, domainencryption.BranchKeySubject) (string, error) {
		return "", errors.New("branch key store unavailable")
	}}
	f.provider.refreshDeps.BranchKeys = broken
	f.provider.fositeStorage.refresh.BranchKeys = broken
	f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
	require.Equal(t, 1, broken.createCalls)
	f.provider.refreshDeps.BranchKeys = testRefreshBranchKeyManager{}
	f.provider.fositeStorage.refresh.BranchKeys = f.provider.refreshDeps.BranchKeys
	result, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, result.RefreshToken)
}

func TestRefreshRotation_FreshExpiresInUsesRemainingSignedLifetime(t *testing.T) {
	f := newProviderRetryFixture(t)
	at := time.Now().UTC()
	f.decideAt(at)
	later := at.Add(10 * time.Second)
	clock := &absoluteRecheckClock{times: [2]time.Time{later, later}}
	f.provider.refreshDeps.Clock = clock
	f.provider.fositeStorage.refresh.Clock = clock
	response, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	root := providerRetryRoot(t, f)
	require.NotNil(t, root.RetryAccessExpiresAt)
	require.Equal(t, int64(root.RetryAccessExpiresAt.Sub(later)/time.Second), response.ExpiresIn)
	require.Less(t, response.ExpiresIn, int64(f.provider.config.AccessTokenLifespan.Seconds()))
}

func TestRefreshRotation_SlowEncryptionDiscardsExpiredRetryCiphertext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderRetryFixture(t)
		coordinator := f.provider.refreshDeps.Coordinator.(*providerRefreshCoordinator).base
		f.provider.refreshDeps.Coordinator = coordinator
		f.provider.fositeStorage.refresh.Coordinator = coordinator
		clock := coordinator.(ports.AuthorizationClock)
		f.provider.refreshDeps.Clock = clock
		f.provider.fositeStorage.refresh.Clock = clock
		time.Sleep(time.Millisecond) // synctest otherwise leaves consumption equal to first issuance.
		cipher := &gateProbeEncryption{base: f.provider.refreshDeps.Encryption, prove: func() error {
			time.Sleep(f.provider.refreshDeps.Policy.ReuseInterval + time.Second)
			return nil
		}}
		f.provider.refreshDeps.Encryption = cipher
		f.provider.fositeStorage.refresh.Encryption = cipher
		fresh, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, fresh.RefreshToken)
		root := providerRetryRoot(t, f)
		require.NotNil(t, root.RetryExpiresAt)
		require.False(t, root.RetryExpiresAt.After(time.Now()), "encryption crossed the fixed retry deadline")
		require.Empty(t, root.RetryCiphertext, "overdue ciphertext must not be committed")
		current, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), root.CurrentSignature)
		require.NoError(t, err)
		require.Nil(t, current.UsedAt)
		f.provider.refreshDeps.Encryption = cipher.base
		f.provider.fositeStorage.refresh.Encryption = cipher.base
		next, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, fresh.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, next.RefreshToken)
		require.NotEqual(t, fresh.RefreshToken, next.RefreshToken)
	})
}

func TestRefreshPreflight_ConsentExpiryDuringEncryptionRejectsWithoutRotation(t *testing.T) {
	f := newProviderRetryFixture(t)
	grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
	grant, err := grants.FindByPrincipalAndAgent(context.Background(), f.root.Principal, f.agent.ID)
	require.NoError(t, err)
	decision := f.root.StartedAt.Add(2 * time.Second)
	f.decideAt(decision)
	base := f.provider.refreshDeps.Coordinator
	updates := 0
	cipher := &gateProbeEncryption{base: f.provider.refreshDeps.Encryption, prove: func() error {
		updates++
		return base.Run(context.Background(), f.agent.ID, func(owner context.Context, _ time.Time) error {
			grant.ValidUntil = &decision
			return grants.Update(owner, grant)
		})
	}}
	f.provider.refreshDeps.Encryption = cipher
	f.provider.fositeStorage.refresh.Encryption = cipher
	f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	require.Equal(t, 1, updates, "the encrypted candidate must be discarded after the authoritative reread")
}

func TestRefreshCodeReplay_ExpiredUsedCodeStillRevokesOnlyItsRoot(t *testing.T) {
	f := newProviderRetryFixture(t)
	other, _, otherSecret := setupTestCredentials(t, f.provider, f.agents)
	const verifier = "expired-code-replay-verifier-01234567890123456789"
	ctx := context.Background()
	code, err := f.provider.HandleAuthorize(ctx, f.agent.ID.String(), providerRefreshRedirect, "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", f.root.Principal)
	require.NoError(t, err)
	response, err := f.provider.HandleAuthorizationCodeExchange(ctx, f.agent.ID.String(), f.secret, code, providerRefreshRedirect, verifier)
	require.NoError(t, err)
	require.NotEmpty(t, response.RefreshToken)
	used, err := f.provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	replayedID, err := id.ParseRefreshSessionID(used.ID.String())
	require.NoError(t, err)
	f.decideAt(used.ExpiresAt)
	replayed, err := f.provider.HandleAuthorizationCodeExchange(ctx, other.ID.String(), otherSecret, code, providerRefreshRedirect, verifier)
	require.Nil(t, replayed)
	require.ErrorIs(t, err, ErrInvalidGrant)
	root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, replayedID)
	require.NoError(t, err)
	require.NotNil(t, root.TerminalReason)
	require.Equal(t, storage.RefreshReasonCodeReplay, *root.TerminalReason)
	unrelated, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
	require.NoError(t, err)
	require.Nil(t, unrelated.TerminalReason)
}

func TestRefreshCodeExchange_ExpiredUnusedCodeIsInvalidGrant(t *testing.T) {
	f := newProviderRetryFixture(t)
	const verifier = "unused-expired-code-verifier-01234567890123456789"
	ctx := context.Background()
	code, err := f.provider.HandleAuthorize(ctx, f.agent.ID.String(), providerRefreshRedirect, "code", "offline_access read", "state", generateS256Challenge(verifier), "S256", f.root.Principal)
	require.NoError(t, err)
	unused, err := f.provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	f.decideAt(unused.ExpiresAt)
	response, err := f.provider.HandleAuthorizationCodeExchange(ctx, f.agent.ID.String(), f.secret, code, providerRefreshRedirect, verifier)
	require.Nil(t, response)
	require.ErrorIs(t, err, ErrInvalidGrant)
	rootID, err := id.ParseRefreshSessionID(unused.ID.String())
	require.NoError(t, err)
	_, err = f.provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	require.True(t, ports.IsNotFoundErr(err))
}

func TestRefreshRotation_DeadlineReachedInsideFositeIsInvalidGrant(t *testing.T) {
	f := newProviderInactivityFixture(t, time.Minute)
	var logs bytes.Buffer
	f.provider.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	deadline := f.root.InactivityExpiresAt
	f.decideAt(deadline.Add(-time.Millisecond))
	clock := providerRefreshClock{at: deadline}
	f.provider.refreshDeps.Clock = clock
	f.provider.fositeStorage.refresh.Clock = clock
	f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	var audit map[string]any
	require.NoError(t, json.NewDecoder(&logs).Decode(&audit))
	require.Equal(t, "RefreshRejected", audit["event"])
	require.Equal(t, "session_lifetime", audit["reason"])
}
