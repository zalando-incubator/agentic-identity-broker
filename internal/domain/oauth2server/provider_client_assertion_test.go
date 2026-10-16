package oauth2server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	dstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_ClientAssertionFailuresLeaveCodeAndRefreshUsable(t *testing.T) {
	verifier := &providerAssertionVerifierFake{err: ErrClientAuthentication}
	provider, _, clientID := newCIMDProviderForAssertionTest(t, "private_key_jwt", verifier)
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
	badCredentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "bad-assertion"}

	_, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, ports.ClientCredentials{}, code, redirectURI, codeVerifier)
	assert.ErrorIs(t, err, ErrInvalidClient)

	_, err = provider.HandleAuthorizationCodeExchange(context.Background(), clientID, badCredentials, code, redirectURI, codeVerifier)
	assert.ErrorIs(t, err, ErrInvalidClient)
	assert.Equal(t, 2, verifier.calls)

	verifier.err = nil
	validCredentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "valid-code-assertion"}
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, validCredentials, code, redirectURI, codeVerifier)
	require.NoError(t, err, "failed authentication must not consume the authorization code")
	require.NotEmpty(t, issued.RefreshToken)

	verifier.err = ErrClientAuthentication
	_, err = provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{}, issued.RefreshToken, "")
	assert.ErrorIs(t, err, ErrInvalidClient)
	_, err = provider.HandleRefreshToken(context.Background(), clientID, badCredentials, issued.RefreshToken, "")
	assert.ErrorIs(t, err, ErrInvalidClient)

	verifier.err = nil
	refreshed, err := provider.HandleRefreshToken(context.Background(), clientID, validCredentials, issued.RefreshToken, "")
	require.NoError(t, err, "failed authentication must not rotate the refresh token")
	assert.NotEmpty(t, refreshed.RefreshToken)
	assert.NotEqual(t, issued.RefreshToken, refreshed.RefreshToken)
}

func TestProvider_NoneClientAssertionRejectedWithoutChangingGrantState(t *testing.T) {
	verifier := &providerAssertionVerifierFake{}
	provider, _, clientID := newCIMDProviderForAssertionTest(t, "none", verifier)
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
	strayAssertion := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "stray-assertion"}

	_, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, strayAssertion, code, redirectURI, codeVerifier)
	assert.ErrorIs(t, err, ErrInvalidClient)
	assert.Zero(t, verifier.calls)

	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, ports.ClientCredentials{}, code, redirectURI, codeVerifier)
	require.NoError(t, err, "stray assertion must not consume a none client's code")
	require.NotEmpty(t, issued.RefreshToken)

	_, err = provider.HandleRefreshToken(context.Background(), clientID, strayAssertion, issued.RefreshToken, "")
	assert.ErrorIs(t, err, ErrInvalidClient)
	assert.Zero(t, verifier.calls)
	refreshed, err := provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{}, issued.RefreshToken, "")
	require.NoError(t, err, "stray assertion must not rotate a none client's refresh token")
	assert.NotEmpty(t, refreshed.RefreshToken)
}

func TestProvider_RefreshRequiresStricterStoredOrCurrentAuthMethod(t *testing.T) {
	for _, test := range []struct {
		name           string
		issuedWith     string
		currentMethod  string
		requiresMethod bool
	}{
		{name: "stored private and current private", issuedWith: "private_key_jwt", currentMethod: "private_key_jwt", requiresMethod: true},
		{name: "stored private and current none", issuedWith: "private_key_jwt", currentMethod: "none", requiresMethod: true},
		{name: "stored none and current private", issuedWith: "none", currentMethod: "private_key_jwt", requiresMethod: true},
		{name: "stored none and current none", issuedWith: "none", currentMethod: "none"},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier := &providerAssertionVerifierFake{}
			provider, agent, clientID := newCIMDProviderForAssertionTest(t, test.issuedWith, verifier)
			currentMethod := test.issuedWith
			provider.fositeStorage.clientResolver = &mockClientResolver{resolveFunc: func(_ context.Context, resolvedID id.ClientID) (*ports.ClientResolution, error) {
				if string(resolvedID) != clientID {
					return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "not found"}
				}
				return ports.NewClientResolution(agent, &ports.CIMDMetadataDTO{
					ClientID:     clientID,
					AuthMethod:   currentMethod,
					RedirectURIs: []string{"https://client.example/callback"},
					GrantTypes:   []string{"authorization_code", "refresh_token"},
					SigningAlg:   "RS256",
					JWKSURI:      "https://client.example/jwks.json",
				}), nil
			}}
			code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
			var exchangeCredentials ports.ClientCredentials
			if test.issuedWith == "private_key_jwt" {
				exchangeCredentials = ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "code-assertion"}
			}
			issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, exchangeCredentials, code, redirectURI, codeVerifier)
			require.NoError(t, err)

			currentMethod = test.currentMethod
			var refreshCredentials ports.ClientCredentials
			if test.requiresMethod {
				refreshCredentials = ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "refresh-assertion"}
			}
			refreshed, err := provider.HandleRefreshToken(context.Background(), clientID, refreshCredentials, issued.RefreshToken, "")
			require.NoError(t, err)
			require.NotEmpty(t, refreshed.RefreshToken)
			expectedCalls := 0
			if test.issuedWith == "private_key_jwt" {
				expectedCalls++
			}
			if test.requiresMethod {
				expectedCalls++
			}
			assert.Equal(t, expectedCalls, verifier.calls)
		})
	}
}

func TestProvider_UnknownRefreshSignatureDoesNotRevealSessionExistence(t *testing.T) {
	verifier := &providerAssertionVerifierFake{}
	provider, _, clientID := newCIMDProviderForAssertionTest(t, "private_key_jwt", verifier)
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
	credentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "valid-code-assertion"}
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, credentials, code, redirectURI, codeVerifier)
	require.NoError(t, err)

	knownErr := error(nil)
	_, knownErr = provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{}, issued.RefreshToken, "")
	unknownErr := error(nil)
	_, unknownErr = provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{}, "unknown-refresh-token", "")
	require.ErrorIs(t, knownErr, ErrInvalidClient)
	require.ErrorIs(t, unknownErr, ErrInvalidClient)
	assert.Equal(t, knownErr.Error(), unknownErr.Error())

	refreshed, err := provider.HandleRefreshToken(context.Background(), clientID, credentials, issued.RefreshToken, "")
	require.NoError(t, err, "failed authentication for a known token must not consume it")
	require.NotEmpty(t, refreshed.RefreshToken)
}

func TestProvider_ClientCannotRedeemAnotherClientsCodeOrRefreshToken(t *testing.T) {
	verifier := &providerAssertionVerifierFake{}
	provider, agentA, clientIDA := newCIMDProviderForAssertionTest(t, "private_key_jwt", verifier)
	clientIDB := "https://other-client.example/metadata.json"
	agentB := &dstorage.Agent{
		ID:             id.NewAgentID(),
		ClientURIs:     []string{clientIDB},
		DisplayName:    "Other CIMD Client",
		Description:    "Second client for cross-client token tests",
		PermissionSets: testAgentPermissionSets(),
	}
	provider.fositeStorage.clientResolver = &mockClientResolver{resolveFunc: func(_ context.Context, resolvedID id.ClientID) (*ports.ClientResolution, error) {
		clientID := string(resolvedID)
		var agent *dstorage.Agent
		switch clientID {
		case clientIDA:
			agent = agentA
		case clientIDB:
			agent = agentB
		default:
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "not found"}
		}
		return ports.NewClientResolution(agent, &ports.CIMDMetadataDTO{
			ClientID:     clientID,
			AuthMethod:   "private_key_jwt",
			RedirectURIs: []string{"https://client.example/callback"},
			GrantTypes:   []string{"authorization_code", "refresh_token"},
			SigningAlg:   "RS256",
			JWKSURI:      "https://client.example/jwks.json",
		}), nil
	}}

	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientIDA)
	clientBCredentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "client-b-code-assertion"}
	response, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientIDB, clientBCredentials, code, redirectURI, codeVerifier)
	assert.Error(t, err)
	assert.Nil(t, response)

	clientACredentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "client-a-code-assertion"}
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientIDA, clientACredentials, code, redirectURI, codeVerifier)
	require.NoError(t, err, "client B must not consume client A's authorization code")
	require.NotEmpty(t, issued.RefreshToken)

	response, err = provider.HandleRefreshToken(context.Background(), clientIDB, ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "client-b-refresh-assertion"}, issued.RefreshToken, "")
	assert.Error(t, err)
	assert.Nil(t, response)

	refreshed, err := provider.HandleRefreshToken(context.Background(), clientIDA, ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "client-a-refresh-assertion"}, issued.RefreshToken, "")
	require.NoError(t, err, "client B must not rotate client A's refresh token")
	require.NotEmpty(t, refreshed.RefreshToken)
}

type concurrentProviderAssertionVerifier struct {
	mu    sync.Mutex
	calls int
	ready chan struct{}
}

func (v *concurrentProviderAssertionVerifier) Verify(_ context.Context, document *ports.CIMDMetadataDTO, assertionType, assertion string) (*VerifiedAssertion, error) {
	if assertionType != ClientAssertionTypeJWTBearer || assertion == "" {
		return nil, ErrClientAuthentication
	}
	v.mu.Lock()
	v.calls++
	if v.calls == 2 {
		close(v.ready)
	}
	v.mu.Unlock()
	<-v.ready
	return &VerifiedAssertion{ClientID: document.ClientID, JTI: assertion, ExpiresAt: time.Now().Add(time.Minute), Method: "private_key_jwt"}, nil
}

func TestProvider_ConcurrentRefreshAssertionsRotateExactlyOnce(t *testing.T) {
	initialVerifier := &providerAssertionVerifierFake{}
	provider, _, clientID := newCIMDProviderForAssertionTest(t, "private_key_jwt", initialVerifier)
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
	initialCredentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "initial-assertion"}
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, initialCredentials, code, redirectURI, codeVerifier)
	require.NoError(t, err)

	provider.clientAssertions = &concurrentProviderAssertionVerifier{ready: make(chan struct{})}
	results := make(chan error, 2)
	for _, assertion := range []string{"concurrent-assertion-a", "concurrent-assertion-b"} {
		go func(assertion string) {
			_, err := provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: assertion}, issued.RefreshToken, "")
			results <- err
		}(assertion)
	}

	var successes int
	var failures []error
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else {
			failures = append(failures, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Len(t, failures, 1)
	assert.True(t, errors.Is(failures[0], ErrInvalidGrant), "the losing rotation should follow refresh-token reuse handling: %v", failures[0])
}

func TestProvider_RevokedGrantAfterCIMDAuthenticationRejectsRefresh(t *testing.T) {
	verifier := &providerAssertionVerifierFake{}
	provider, agent, clientID := newCIMDProviderForAssertionTest(t, "private_key_jwt", verifier)
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
	credentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "valid-code-assertion"}
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, credentials, code, redirectURI, codeVerifier)
	require.NoError(t, err)

	grants := provider.fositeStorage.verifier.(*testConsentVerifier).grants
	_, err = grants.DeleteByPrincipalAndAgentID(context.Background(), id.Principal("user@example.com"), agent.ID)
	require.NoError(t, err)

	response, err := provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{
		AssertionType: ClientAssertionTypeJWTBearer,
		Assertion:     "valid-refresh-assertion",
	}, issued.RefreshToken, "")
	assert.ErrorIs(t, err, ErrInvalidGrant)
	assert.Nil(t, response)
}

func TestProvider_DeletedAgentAfterCIMDAuthenticationRejectsRefresh(t *testing.T) {
	provider, agentRepo, _ := newTestProvider(t)
	clientID := "https://client.example/metadata.json"
	agent := &dstorage.Agent{
		ID:             id.NewAgentID(),
		ClientURIs:     []string{clientID},
		DisplayName:    "CIMD Agent Deletion Test",
		Description:    "Agent deleted during refresh authentication",
		PermissionSets: testAgentPermissionSets(),
	}
	require.NoError(t, agentRepo.Create(context.Background(), agent))
	seedTestGrant(t, provider.fositeStorage.verifier, agent.ID, id.Principal("user@example.com"))
	verifier := &providerAssertionVerifierFake{}
	provider.clientAssertions = verifier
	provider.fositeStorage.clientResolver = &mockClientResolver{resolveFunc: func(ctx context.Context, resolvedID id.ClientID) (*ports.ClientResolution, error) {
		if string(resolvedID) != clientID {
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "not found"}
		}
		resolvedAgent, err := agentRepo.GetByClientURI(ctx, clientID)
		if err != nil {
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "not found"}
		}
		return ports.NewClientResolution(resolvedAgent, &ports.CIMDMetadataDTO{
			ClientID:     clientID,
			AuthMethod:   "private_key_jwt",
			RedirectURIs: []string{"https://client.example/callback"},
			GrantTypes:   []string{"authorization_code", "refresh_token"},
			SigningAlg:   "RS256",
			JWKSURI:      "https://client.example/jwks.json",
		}), nil
	}}
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)
	credentials := ports.ClientCredentials{AssertionType: ClientAssertionTypeJWTBearer, Assertion: "valid-code-assertion"}
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, credentials, code, redirectURI, codeVerifier)
	require.NoError(t, err)

	var deleted bool
	verifier.onVerify = func() {
		deleted, err = agentRepo.Delete(context.Background(), agent.ID)
	}
	response, refreshErr := provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{
		AssertionType: ClientAssertionTypeJWTBearer,
		Assertion:     "valid-refresh-assertion",
	}, issued.RefreshToken, "")
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.ErrorIs(t, refreshErr, ErrInvalidGrant)
	assert.Nil(t, response)
}

func TestProvider_ReplayStoreFailureLeavesCodeAndRefreshUsable(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "replay-store-key")
	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(key.public))
	replayStore := &clientAssertionReplayFake{seen: make(map[string]bool), err: errors.New("replay storage unavailable")}
	verifier := NewClientAssertionVerifier(
		&clientAssertionKeySourceFake{keys: keySet},
		replayStore,
		[]string{assertionTestTokenURL},
		func() time.Time { return now },
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)
	provider, _, clientID := newCIMDProviderForAssertionTest(t, "private_key_jwt", &providerAssertionVerifierFake{})
	provider.clientAssertions = verifier
	code, codeVerifier, redirectURI := createCIMDAuthorizationCode(t, provider, clientID)

	codeAssertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{jti: "code-replay-store-failure"})
	_, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, ports.ClientCredentials{AssertionType: assertionType, Assertion: codeAssertion}, code, redirectURI, codeVerifier)
	assert.ErrorIs(t, err, ErrServerError)

	replayStore.err = nil
	validCodeAssertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{jti: "code-retry"})
	issued, err := provider.HandleAuthorizationCodeExchange(context.Background(), clientID, ports.ClientCredentials{AssertionType: assertionType, Assertion: validCodeAssertion}, code, redirectURI, codeVerifier)
	require.NoError(t, err, "replay storage failure must not consume the authorization code")
	require.NotEmpty(t, issued.RefreshToken)

	replayStore.err = errors.New("replay storage unavailable")
	refreshAssertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{jti: "refresh-replay-store-failure"})
	_, err = provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{AssertionType: assertionType, Assertion: refreshAssertion}, issued.RefreshToken, "")
	assert.ErrorIs(t, err, ErrServerError)

	replayStore.err = nil
	validRefreshAssertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{jti: "refresh-retry"})
	refreshed, err := provider.HandleRefreshToken(context.Background(), clientID, ports.ClientCredentials{AssertionType: assertionType, Assertion: validRefreshAssertion}, issued.RefreshToken, "")
	require.NoError(t, err, "replay storage failure must not rotate the refresh token")
	require.NotEmpty(t, refreshed.RefreshToken)
	assert.NotEqual(t, issued.RefreshToken, refreshed.RefreshToken)
}
