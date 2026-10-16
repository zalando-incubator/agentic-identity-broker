package jwtauth

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// fakeKeySet is an injected ports.JWKSPort, standing in for the static or file key-set adapter.
type fakeKeySet struct {
	set jwk.Set
	err error
}

func (f *fakeKeySet) GetKeySet(context.Context) (jwk.Set, error) {
	return f.set, f.err
}

func (f *fakeKeySet) GetKey(_ context.Context, kid string) (jwk.Key, error) {
	key, ok := f.set.LookupKeyID(kid)
	if !ok {
		return nil, errors.New("key not found")
	}
	return key, nil
}

func (f *fakeKeySet) HealthState() ports.ComponentHealth {
	return ports.ComponentHealthHealthy
}

func publicSet(t *testing.T, signingKey jwk.Key) jwk.Set {
	t.Helper()
	public, err := signingKey.PublicKey()
	require.NoError(t, err)
	set := jwk.NewSet()
	require.NoError(t, set.AddKey(public))
	return set
}

func newKeySetAuthenticator(t *testing.T, keySet ports.JWKSPort, client *http.Client) *JWXAuthenticator {
	t.Helper()
	celEvaluator, err := jwtauth.NewCELEvaluator(jwtauth.CELEvaluatorConfig{PrincipalExpression: "claims.sub"}, testLogger())
	require.NoError(t, err)
	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig: &ports.JWTConfig{
			HeaderName:      "Authorization",
			Verification:    "jwks",
			ClaimExtraction: ports.JWTClaimExtractionConfig{PrincipalExpression: "claims.sub"},
		},
		CELEvaluator: celEvaluator,
		HTTPClient:   client,
		Logger:       testLogger(),
		KeySet:       keySet,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Shutdown(context.Background())) })
	return auth
}

func TestJWXAuthenticator_InjectedKeySetMakesNoRequest(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("no outbound request expected")
	})}

	_, _, signingKey := generateTestKeyPair(t)
	auth := newKeySetAuthenticator(t, &fakeKeySet{set: publicSet(t, signingKey)}, client)

	token := createSignedJWT(t, map[string]interface{}{"sub": "alice@example.com", "exp": time.Now().Add(time.Hour)}, signingKey)
	result, err := auth.Authenticate(context.Background(), "Bearer "+token)

	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result.Principal)
	assert.Zero(t, requests.Load())
}

func TestJWXAuthenticator_InjectedKeySetMatchesRemoteVerification(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	remote := newTestAuthenticator(t, startMockJWKSServer(t, jwksBytes).URL, signingKey, nil)
	local := newKeySetAuthenticator(t, &fakeKeySet{set: publicSet(t, signingKey)}, nil)
	_, _, otherKey := generateTestKeyPair(t)

	tests := map[string]string{
		"valid":        createSignedJWT(t, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(time.Hour)}, signingKey),
		"wrong key":    createSignedJWT(t, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(time.Hour)}, otherKey),
		"expired":      createSignedJWT(t, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(-time.Hour)}, signingKey),
		"missing exp":  createSignedJWT(t, map[string]interface{}{"sub": "alice"}, signingKey),
		"not a token":  "not-a-jwt",
		"unknown kid":  createSignedJWT(t, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(time.Hour)}, withKID(t, otherKey, "unknown")),
		"no signature": createUnsignedJWT(t, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(time.Hour)}),
	}
	sentinels := []error{
		jwtauth.ErrInvalidSignature, jwtauth.ErrTokenExpired, jwtauth.ErrMalformedToken,
		jwtauth.ErrMissingExpiry, jwtauth.ErrJWKSUnavailable,
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			_, remoteErr := remote.Authenticate(context.Background(), token)
			_, localErr := local.Authenticate(context.Background(), token)
			assert.Equal(t, remoteErr == nil, localErr == nil, "remote=%v local=%v", remoteErr, localErr)
			for _, sentinel := range sentinels {
				assert.Equal(t, errors.Is(remoteErr, sentinel), errors.Is(localErr, sentinel), "sentinel %v", sentinel)
			}
		})
	}
}

func TestJWXAuthenticator_InjectedKeySetUnavailable(t *testing.T) {
	_, _, signingKey := generateTestKeyPair(t)
	auth := newKeySetAuthenticator(t, &fakeKeySet{err: errors.New("key set has not been read successfully")}, nil)

	token := createSignedJWT(t, map[string]interface{}{"sub": "alice", "exp": time.Now().Add(time.Hour)}, signingKey)
	_, err := auth.Authenticate(context.Background(), token)

	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrJWKSUnavailable)
}

func withKID(t *testing.T, key jwk.Key, kid string) jwk.Key {
	t.Helper()
	clone, err := key.Clone()
	require.NoError(t, err)
	require.NoError(t, clone.Set(jwk.KeyIDKey, kid))
	return clone
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
