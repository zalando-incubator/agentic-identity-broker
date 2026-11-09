package oauth2server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	keyset "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	assertionTestClientID = "https://client.example/metadata.json"
	assertionTestIssuer   = "https://broker.example"
	assertionTestTokenURL = "https://broker.example/oauth2/token"
)

type clientAssertionTestKey struct {
	private any
	public  jwk.Key
	kid     string
	alg     jwa.SignatureAlgorithm
}

func newClientAssertionTestKey(t *testing.T, algorithm jwa.SignatureAlgorithm, kid string) clientAssertionTestKey {
	t.Helper()
	var private crypto.Signer
	switch algorithm.String() {
	case "RS256":
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		private = key
	case "ES256":
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		private = key
	default:
		t.Fatalf("unsupported test signing algorithm %q", algorithm)
	}
	public, err := jwk.Import[jwk.Key](private.Public())
	require.NoError(t, err)
	require.NoError(t, public.Set(jwk.KeyIDKey, kid))
	return clientAssertionTestKey{private: private, public: public, kid: kid, alg: algorithm}
}

type clientAssertionKeySourceFake struct {
	keys         jwk.Set
	err          error
	refreshKeys  jwk.Set
	refreshErr   error
	refreshCalls int
}

func (f *clientAssertionKeySourceFake) KeysFor(context.Context, *ports.CIMDMetadataDTO) (jwk.Set, error) {
	return f.keys, f.err
}

func (f *clientAssertionKeySourceFake) RefreshForKID(context.Context, *ports.CIMDMetadataDTO, string) (jwk.Set, error) {
	f.refreshCalls++
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	if f.refreshKeys != nil {
		return f.refreshKeys, nil
	}
	return f.keys, nil
}

type clientAssertionReplayFake struct {
	seen         map[string]bool
	consumeCalls int
	err          error
}

func newClientAssertionReplayFake() *clientAssertionReplayFake {
	return &clientAssertionReplayFake{seen: make(map[string]bool)}
}

func (f *clientAssertionReplayFake) Consume(_ context.Context, clientID, jti string, _ time.Time) error {
	f.consumeCalls++
	if f.err != nil {
		return f.err
	}
	key := clientID + "\x00" + jti
	if f.seen[key] {
		return storage.NewStorageError("test replay", storage.ErrorKindConflict, nil, "already consumed")
	}
	f.seen[key] = true
	return nil
}

func (*clientAssertionReplayFake) DeleteExpired(context.Context) (int, error) { return 0, nil }

type clientAssertionTestOptions struct {
	algorithm         jwa.SignatureAlgorithm
	signingKey        any
	clientID          string
	issuer            string
	subject           string
	audience          []string
	expiration        *time.Time
	omitExpiration    bool
	issuedAt          *time.Time
	omitIssuedAt      bool
	notBefore         *time.Time
	jti               string
	omitJTI           bool
	emptyJTI          bool
	kid               string
	omitKID           bool
	assertionType     string
	omitAssertionType bool
	extraHeaders      map[string]any
}

func signClientAssertionForTest(t *testing.T, key clientAssertionTestKey, now time.Time, options clientAssertionTestOptions) (string, string) {
	t.Helper()
	if options.algorithm.String() == "" {
		options.algorithm = key.alg
	}
	if options.signingKey == nil {
		options.signingKey = key.private
	}
	clientID := options.clientID
	if clientID == "" {
		clientID = assertionTestClientID
	}
	issuer := options.issuer
	if issuer == "" {
		issuer = clientID
	}
	subject := options.subject
	if subject == "" {
		subject = clientID
	}
	audience := options.audience
	if audience == nil {
		audience = []string{assertionTestTokenURL}
	}
	builder := jwt.NewBuilder().Issuer(issuer).Subject(subject).Audience(audience)
	if !options.omitExpiration {
		expiresAt := now.Add(5 * time.Minute)
		if options.expiration != nil {
			expiresAt = *options.expiration
		}
		builder = builder.Expiration(expiresAt)
	}
	if options.issuedAt != nil {
		builder = builder.IssuedAt(*options.issuedAt)
	} else if !options.omitExpiration && !options.omitIssuedAt {
		builder = builder.IssuedAt(now)
	}
	if options.notBefore != nil {
		builder = builder.NotBefore(*options.notBefore)
	}
	if !options.omitJTI {
		jti := options.jti
		if jti == "" && !options.emptyJTI {
			jti = "test-jti"
		}
		builder = builder.JwtID(jti)
	}
	token, err := builder.Build()
	require.NoError(t, err)
	headers := jws.NewHeaders()
	if !options.omitKID {
		kid := options.kid
		if kid == "" {
			kid = key.kid
		}
		require.NoError(t, headers.Set(jws.KeyIDKey, kid))
	}
	for name, value := range options.extraHeaders {
		require.NoError(t, headers.Set(name, value))
	}
	var signed []byte
	if options.algorithm == jwa.NoSignature() {
		signed, err = jwt.Sign(token, jwt.WithInsecureNoSignature())
	} else {
		signed, err = jwt.Sign(token, jwt.WithKey(options.algorithm, options.signingKey, jws.WithProtectedHeaders(headers)))
	}
	require.NoError(t, err)
	assertionType := options.assertionType
	if assertionType == "" && !options.omitAssertionType {
		assertionType = ClientAssertionTypeJWTBearer
	}
	return string(signed), assertionType
}

func newClientAssertionVerifierForTest(t *testing.T, now time.Time, key clientAssertionTestKey, logger *slog.Logger) (*ClientAssertionVerifier, *clientAssertionReplayFake, *ports.CIMDMetadataDTO) {
	t.Helper()
	keys := jwk.NewSet()
	require.NoError(t, keys.AddKey(key.public))
	replay := newClientAssertionReplayFake()
	verifier := NewClientAssertionVerifier(
		&clientAssertionKeySourceFake{keys: keys},
		replay,
		[]string{assertionTestIssuer, assertionTestTokenURL},
		func() time.Time { return now },
		logger,
	)
	document := &ports.CIMDMetadataDTO{
		ClientID:   assertionTestClientID,
		AuthMethod: "private_key_jwt",
		SigningAlg: key.alg.String(),
		KeySet:     keys,
	}
	return verifier, replay, document
}

func TestClientAssertionVerifierAcceptsRS256AndES256(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	for _, algorithm := range []jwa.SignatureAlgorithm{jwa.RS256(), jwa.ES256()} {
		t.Run(algorithm.String(), func(t *testing.T) {
			key := newClientAssertionTestKey(t, algorithm, "key-1")
			verifier, replay, document := newClientAssertionVerifierForTest(t, now, key, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
			assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{})
			verified, err := verifier.Verify(context.Background(), document, assertionType, assertion)
			require.NoError(t, err)
			assert.Equal(t, assertionTestClientID, verified.ClientID)
			assert.Equal(t, "test-jti", verified.JTI)
			assert.Equal(t, "private_key_jwt", verified.Method)
			assert.Equal(t, 1, replay.consumeCalls)
		})
	}
}

func TestClientAssertionVerifierValidatesAudienceAndLifetime(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	cases := []struct {
		name     string
		audience []string
		exp      time.Time
		iat      *time.Time
		omitIAT  bool
		valid    bool
	}{
		{name: "issuer audience", audience: []string{assertionTestIssuer}, exp: now.Add(5 * time.Minute), valid: true},
		{name: "token endpoint audience", audience: []string{assertionTestTokenURL}, exp: now.Add(5 * time.Minute), valid: true},
		{name: "approved audience in array", audience: []string{"https://other.example/token", assertionTestTokenURL}, exp: now.Add(5 * time.Minute), valid: true},
		{name: "both approved audiences", audience: []string{assertionTestIssuer, assertionTestTokenURL}, exp: now.Add(5 * time.Minute), valid: true},
		{name: "unapproved audience", audience: []string{"https://other.example/token"}, exp: now.Add(5 * time.Minute)},
		{name: "expired beyond skew", audience: []string{assertionTestTokenURL}, exp: now.Add(-31 * time.Second), iat: ptrTime(now.Add(-5 * time.Minute))},
		{name: "within expiration skew", audience: []string{assertionTestTokenURL}, exp: now.Add(-29 * time.Second), iat: ptrTime(now.Add(-5*time.Minute - 29*time.Second)), valid: true},
		{name: "maximum lifetime with skew and no iat", audience: []string{assertionTestTokenURL}, exp: now.Add(5*time.Minute + 30*time.Second), omitIAT: true, valid: true},
		{name: "over maximum lifetime with no iat", audience: []string{assertionTestTokenURL}, exp: now.Add(5*time.Minute + 31*time.Second), omitIAT: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			verifier, _, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
			assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{audience: test.audience, expiration: &test.exp, issuedAt: test.iat, omitIssuedAt: test.omitIAT})
			_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrClientAuthentication)
			}
		})
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestClientAssertionVerifierRejectsInvalidAssertions(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	attackerKey := newClientAssertionTestKey(t, jwa.RS256(), "attacker-key")
	future := now.Add(31 * time.Second)
	longIAT := now.Add(-6 * time.Minute)
	longExp := now.Add(time.Minute)
	publicRSA, ok := key.public.(jwk.RSAPublicKey)
	require.True(t, ok)
	publicModulus, ok := publicRSA.N()
	require.True(t, ok)
	cases := []struct {
		name    string
		options clientAssertionTestOptions
	}{
		{name: "missing assertion type", options: clientAssertionTestOptions{omitAssertionType: true}},
		{name: "wrong assertion type", options: clientAssertionTestOptions{assertionType: "urn:wrong:type"}},
		{name: "wrong issuer", options: clientAssertionTestOptions{issuer: "https://wrong.example/client"}},
		{name: "wrong subject", options: clientAssertionTestOptions{subject: "https://wrong.example/client"}},
		{name: "missing expiration", options: clientAssertionTestOptions{omitExpiration: true}},
		{name: "missing jti", options: clientAssertionTestOptions{omitJTI: true}},
		{name: "empty jti", options: clientAssertionTestOptions{emptyJTI: true}},
		{name: "missing kid", options: clientAssertionTestOptions{omitKID: true}},
		{name: "unknown kid", options: clientAssertionTestOptions{kid: "unknown"}},
		{name: "header algorithm differs from document", options: clientAssertionTestOptions{algorithm: jwa.PS256()}},
		{name: "HS256 using public modulus bytes", options: clientAssertionTestOptions{algorithm: jwa.HS256(), signingKey: publicModulus}},
		{name: "unsigned algorithm", options: clientAssertionTestOptions{algorithm: jwa.NoSignature()}},
		{name: "future issued-at", options: clientAssertionTestOptions{issuedAt: &future}},
		{name: "future not-before", options: clientAssertionTestOptions{notBefore: &future}},
		{name: "exp minus iat exceeds maximum", options: clientAssertionTestOptions{issuedAt: &longIAT, expiration: &longExp}},
		{name: "bad signature", options: clientAssertionTestOptions{signingKey: attackerKey.private}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			verifier, replay, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
			assertion, assertionType := signClientAssertionForTest(t, key, now, test.options)
			_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
			require.ErrorIs(t, err, ErrClientAuthentication)
			if test.name == "bad signature" {
				assert.Zero(t, replay.consumeCalls, "bad signatures must not consume replay state")
			}
		})
	}
}

func TestClientAssertionVerifierConsumesJTIPerClient(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	verifier, _, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
	assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{jti: "one-use"})
	_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.NoError(t, err)
	_, err = verifier.Verify(context.Background(), document, assertionType, assertion)
	require.ErrorIs(t, err, ErrClientAuthentication)

	otherDocument := *document
	otherDocument.ClientID = "https://other.example/client"
	otherAssertion, otherType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{
		clientID: otherDocument.ClientID,
		jti:      "one-use",
	})
	_, err = verifier.Verify(context.Background(), &otherDocument, otherType, otherAssertion)
	require.NoError(t, err, "same jti from a different client must be independent")
}

func TestClientAssertionVerifierRefreshesUnknownKIDOnce(t *testing.T) {
	now := time.Now().UTC()
	oldKey := newClientAssertionTestKey(t, jwa.RS256(), "old-key")
	newKey := newClientAssertionTestKey(t, jwa.RS256(), "new-key")
	oldKeys, newKeys := jwk.NewSet(), jwk.NewSet()
	require.NoError(t, oldKeys.AddKey(oldKey.public))
	require.NoError(t, newKeys.AddKey(newKey.public))
	keySource := &clientAssertionKeySourceFake{keys: oldKeys, refreshKeys: newKeys}
	verifier := NewClientAssertionVerifier(keySource, newClientAssertionReplayFake(), []string{assertionTestTokenURL}, func() time.Time { return now }, slog.Default())
	document := &ports.CIMDMetadataDTO{ClientID: assertionTestClientID, AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	assertion, assertionType := signClientAssertionForTest(t, newKey, now, clientAssertionTestOptions{jti: "rotated-key-assertion"})

	verified, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.NoError(t, err)
	assert.Equal(t, assertionTestClientID, verified.ClientID)
	assert.Equal(t, 1, keySource.refreshCalls)
}

func TestClientAssertionVerifierKeyRefreshFailureIsUnavailable(t *testing.T) {
	now := time.Now().UTC()
	oldKey := newClientAssertionTestKey(t, jwa.RS256(), "old-key")
	newKey := newClientAssertionTestKey(t, jwa.RS256(), "new-key")
	oldKeys := jwk.NewSet()
	require.NoError(t, oldKeys.AddKey(oldKey.public))
	keySource := &clientAssertionKeySourceFake{keys: oldKeys, refreshErr: keyset.ErrKeySetUnavailable}
	verifier := NewClientAssertionVerifier(keySource, newClientAssertionReplayFake(), []string{assertionTestTokenURL}, func() time.Time { return now }, slog.Default())
	document := &ports.CIMDMetadataDTO{ClientID: assertionTestClientID, AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	assertion, assertionType := signClientAssertionForTest(t, newKey, now, clientAssertionTestOptions{jti: "refresh-failure-assertion"})

	_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.ErrorIs(t, err, ErrClientAuthenticationUnavailable)
	assert.Equal(t, 1, keySource.refreshCalls)
}

type verifierKeySetFetcher struct {
	result *ports.CIMDFetchResult
	err    error
	calls  int
}

func (f *verifierKeySetFetcher) Fetch(context.Context, string) (*ports.CIMDFetchResult, error) {
	f.calls++
	return f.result, f.err
}

func TestClientAssertionVerifierUnknownKIDWithinIntervalDoesNotFetch(t *testing.T) {
	now := time.Now().UTC()
	oldKey := newClientAssertionTestKey(t, jwa.RS256(), "old-key")
	newKey := newClientAssertionTestKey(t, jwa.RS256(), "new-key")
	oldKeys := jwk.NewSet()
	require.NoError(t, oldKeys.AddKey(oldKey.public))
	oldBody, err := json.Marshal(oldKeys)
	require.NoError(t, err)
	fetcher := &verifierKeySetFetcher{result: &ports.CIMDFetchResult{Body: oldBody}}
	keySource := keyset.NewClientKeySetService(fetcher, ports.CIMDClientAuthenticationConfig{
		KeySetCacheTTL:           time.Hour,
		KeySetMinRefetchInterval: time.Minute,
		KeySetMaxBytes:           65536,
	}, func() time.Time { return now }, slog.Default())
	verifier := NewClientAssertionVerifier(keySource, newClientAssertionReplayFake(), []string{assertionTestTokenURL}, func() time.Time { return now }, slog.Default())
	document := &ports.CIMDMetadataDTO{ClientID: assertionTestClientID, AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	assertion, assertionType := signClientAssertionForTest(t, newKey, now, clientAssertionTestOptions{jti: "too-soon-assertion"})

	_, err = verifier.Verify(context.Background(), document, assertionType, assertion)
	require.ErrorIs(t, err, ErrClientAuthentication)
	assert.Equal(t, 1, fetcher.calls, "an unknown KID inside the minimum interval must not trigger another fetch")
}

func TestClientAssertionVerifierKeyAndReplayFailuresAreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{})
	document := &ports.CIMDMetadataDTO{ClientID: assertionTestClientID, AuthMethod: "private_key_jwt", SigningAlg: "RS256"}

	keyErrorVerifier := NewClientAssertionVerifier(&clientAssertionKeySourceFake{err: errors.New("upstream down")}, newClientAssertionReplayFake(), []string{assertionTestTokenURL}, func() time.Time { return now }, slog.Default())
	_, err := keyErrorVerifier.Verify(context.Background(), document, assertionType, assertion)
	require.ErrorIs(t, err, ErrClientAuthenticationUnavailable)

	keys := jwk.NewSet()
	require.NoError(t, keys.AddKey(key.public))
	replay := newClientAssertionReplayFake()
	replay.err = errors.New("database down")
	replayErrorVerifier := NewClientAssertionVerifier(&clientAssertionKeySourceFake{keys: keys}, replay, []string{assertionTestTokenURL}, func() time.Time { return now }, slog.Default())
	_, err = replayErrorVerifier.Verify(context.Background(), document, assertionType, assertion)
	require.ErrorIs(t, err, ErrClientAuthenticationUnavailable)
}

func TestClientAssertionVerifierAuditDoesNotLogCredentials(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	verifier, _, document := newClientAssertionVerifierForTest(t, now, key, logger)
	assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{jti: "sensitive-full-jti"})
	_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.NoError(t, err)
	logs := logBuffer.String()
	assert.Contains(t, logs, "client_authenticated")
	assert.NotContains(t, logs, assertion)
	assert.NotContains(t, logs, "sensitive-full-jti")

	badAssertion, badType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{audience: []string{"https://wrong.example/token"}, jti: "another-sensitive-jti"})
	_, err = verifier.Verify(context.Background(), document, badType, badAssertion)
	require.Error(t, err)
	logs = logBuffer.String()
	assert.Contains(t, logs, "client_authentication_rejected")
	assert.NotContains(t, logs, badAssertion)
	assert.NotContains(t, logs, "another-sensitive-jti")
	assert.NotContains(t, logs, strings.TrimPrefix(assertion, "Bearer "))
}

func TestClientAssertionVerifierAcceptsArrayWithApprovedAudience(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	verifier, _, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
	assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{audience: []string{"https://untrusted.example/aud", assertionTestIssuer, "https://other.example/aud"}})
	_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.NoError(t, err)
}

func TestClientAssertionVerifierMissingAssertionIsAuthenticationFailure(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	verifier, _, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
	_, err := verifier.Verify(context.Background(), document, ClientAssertionTypeJWTBearer, "")
	require.ErrorIs(t, err, ErrClientAuthentication)
}

func TestClientAssertionVerifierIgnoresKeyURLsAndEmbeddedJWKHeaders(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	verifier, _, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
	assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{
		extraHeaders: map[string]any{"jku": "https://attacker.example/keys", "jwk": key.public, "x5u": "https://attacker.example/cert"},
	})
	_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.NoError(t, err)
}

func TestClientAssertionVerifierAcceptsAbsentIssuedAt(t *testing.T) {
	now := time.Now().UTC()
	key := newClientAssertionTestKey(t, jwa.RS256(), "key-1")
	verifier, _, document := newClientAssertionVerifierForTest(t, now, key, slog.Default())
	assertion, assertionType := signClientAssertionForTest(t, key, now, clientAssertionTestOptions{omitIssuedAt: true})
	_, err := verifier.Verify(context.Background(), document, assertionType, assertion)
	require.NoError(t, err)
}
