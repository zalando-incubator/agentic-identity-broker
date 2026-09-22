package oauth2server

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type strategySigningKeyStore struct {
	mu          sync.RWMutex
	bootstrapMu sync.Mutex
	byID        map[id.SigningKeyID]*storage.SigningKey
	byKID       map[id.KeyID]*storage.SigningKey
}

var _ ports.SigningKeyRepository = (*strategySigningKeyStore)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*strategySigningKeyStore)(nil)

func newStrategySigningKeyStore() *strategySigningKeyStore {
	return &strategySigningKeyStore{
		byID:  make(map[id.SigningKeyID]*storage.SigningKey),
		byKID: make(map[id.KeyID]*storage.SigningKey),
	}
}

func newStrategyTestSigningKeyService() (*SigningKeyService, *strategySigningKeyStore) {
	repo := newStrategySigningKeyStore()
	return NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger()), repo
}

func cloneStrategySigningKey(key *storage.SigningKey) *storage.SigningKey {
	clone := *key
	clone.PrivateKeyEncrypted = append([]byte(nil), key.PrivateKeyEncrypted...)
	return &clone
}

func (s *strategySigningKeyStore) Create(_ context.Context, key *storage.SigningKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byKID[key.KID]; exists {
		return storage.NewStorageError("strategySigningKeyStore.Create", storage.ErrorKindConflict, nil, "signing key already exists")
	}

	clone := cloneStrategySigningKey(key)
	s.byID[clone.ID] = clone
	s.byKID[clone.KID] = clone
	return nil
}

func (s *strategySigningKeyStore) CreateAndSetCurrent(_ context.Context, key *storage.SigningKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byKID[key.KID]; exists {
		return storage.NewStorageError("strategySigningKeyStore.CreateAndSetCurrent", storage.ErrorKindConflict, nil, "signing key already exists")
	}

	for _, existing := range s.byID {
		if existing.KeyDomain == key.KeyDomain {
			existing.IsCurrent = false
		}
	}

	clone := cloneStrategySigningKey(key)
	clone.IsCurrent = true
	s.byID[clone.ID] = clone
	s.byKID[clone.KID] = clone
	return nil
}

func (s *strategySigningKeyStore) GetByKIDInDomain(_ context.Context, domain storage.KeyDomain, kid id.KeyID) (*storage.SigningKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil || key.KeyDomain != domain {
		return nil, storage.NewStorageError("strategySigningKeyStore.GetByKIDInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
	}
	return cloneStrategySigningKey(key), nil
}

func (s *strategySigningKeyStore) GetCurrentInDomain(_ context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if current := currentUsableStrategySigningKeyInDomain(s.byID, domain, time.Now()); current != nil {
		return cloneStrategySigningKey(current), nil
	}
	return nil, storage.NewStorageError("strategySigningKeyStore.GetCurrentInDomain", storage.ErrorKindNotFound, nil, "no current signing key")
}

func (s *strategySigningKeyStore) ListActiveInDomain(_ context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]*storage.SigningKey, 0, len(s.byID))
	for _, key := range s.byID {
		if key.KeyDomain == domain && key.RemovedAt == nil {
			keys = append(keys, cloneStrategySigningKey(key))
		}
	}
	return keys, nil
}

func (s *strategySigningKeyStore) SetCurrentInDomain(_ context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target, exists := s.byKID[kid]
	if !exists || target.RemovedAt != nil || target.KeyDomain != domain {
		return nil, storage.NewStorageError("strategySigningKeyStore.SetCurrentInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
	}
	for _, key := range s.byID {
		if key.KeyDomain == domain {
			key.IsCurrent = false
		}
	}
	target.IsCurrent = true
	target.ActivatesAt = activatesAt
	return cloneStrategySigningKey(target), nil
}

func currentUsableStrategySigningKeyInDomain(keys map[id.SigningKeyID]*storage.SigningKey, domain storage.KeyDomain, now time.Time) *storage.SigningKey {
	var best *storage.SigningKey
	for _, key := range keys {
		if key.KeyDomain != domain || key.RemovedAt != nil || key.ActivatesAt.After(now) {
			continue
		}
		if best == nil || (!best.IsCurrent && key.IsCurrent) || (best.IsCurrent == key.IsCurrent && key.ActivatesAt.After(best.ActivatesAt)) {
			best = key
		}
	}
	return best
}

func (s *strategySigningKeyStore) DeleteInDomain(_ context.Context, domain storage.KeyDomain, kid id.KeyID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil || key.KeyDomain != domain {
		return storage.NewStorageError("strategySigningKeyStore.DeleteInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
	}

	activeCount := 0
	for _, existing := range s.byID {
		if existing.KeyDomain == domain && existing.RemovedAt == nil {
			activeCount++
		}
	}
	if activeCount <= 1 {
		return ports.ErrLastActiveKey
	}
	if key.IsCurrent {
		return ports.ErrCurrentKey
	}
	if current := currentUsableStrategySigningKeyInDomain(s.byID, domain, time.Now()); current != nil && current.KID == kid {
		return ports.ErrEffectiveCurrentKey
	}

	now := time.Now()
	key.RemovedAt = &now
	return nil
}

func (s *strategySigningKeyStore) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	s.bootstrapMu.Lock()
	defer s.bootstrapMu.Unlock()
	return fn(ctx)
}

func (s *strategySigningKeyStore) CountActiveInDomain(_ context.Context, domain storage.KeyDomain) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, key := range s.byID {
		if key.KeyDomain == domain && key.RemovedAt == nil {
			count++
		}
	}
	return count, nil
}

func TestJWXAccessTokenStrategy_GenerateAccessToken_DecryptFailure(t *testing.T) {
	t.Run("decrypt failure returns wrapped error", func(t *testing.T) {
		// Use failingDecryptor so Encrypt succeeds (key is stored) but Decrypt always fails.
		repo := newStrategySigningKeyStore()
		svc := NewSigningKeyService(repo, repo, &failingDecryptor{}, newNoopBranchKeyManager(), testSlogger())
		ctx := context.Background()

		// generateAndStore with time.Now() so activates_at is in the past and GetCurrent returns the key.
		_, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
		require.NoError(t, err)

		strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, nil, testSlogger())
		require.NoError(t, err)

		_, _, err = strategy.GenerateAccessToken(ctx, buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt signing key")
	})
}

func TestJWXAccessTokenStrategy_GenerateAccessToken_CELAudienceListFails(t *testing.T) {
	svc, _ := newStrategyTestSigningKeyService()
	ctx := context.Background()

	_, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
	require.NoError(t, err)

	expr := `{"agent_name": agent.display_name, "cid": agent.id, "https://identity.zalando.com/global-uuid": principal.id, "aud": ["https://agentic.identity.zalando.com"]}`
	eval, err := NewTokenClaimsEvaluator(expr)
	require.NoError(t, err)

	logger, buf := bufLogger()
	strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, eval, logger)
	require.NoError(t, err)

	req := buildTestRequest("agent", "user@example.com", []string{"read"})
	_, _, err = strategy.GenerateAccessToken(ctx, req)
	require.Error(t, err)
	assert.ErrorContains(t, err, `failed to set claim "aud"`)
	assert.ErrorContains(t, err, `invalid type: []ref.Val`)
	assert.Contains(t, buf.String(), "failed to build JWT")
	assert.Contains(t, buf.String(), "failed to set claim")
	assert.Contains(t, buf.String(), "aud")
	assert.Contains(t, buf.String(), `invalid type: []ref.Val`)
}

func TestRandomCodeStrategy_GenerateAuthorizeCode(t *testing.T) {
	strategy := &RandomCodeStrategy{}

	t.Run("generates unique codes", func(t *testing.T) {
		code1, sig1, err := strategy.GenerateAuthorizeCode(context.Background(), nil)
		require.NoError(t, err)
		code2, sig2, err := strategy.GenerateAuthorizeCode(context.Background(), nil)
		require.NoError(t, err)

		assert.NotEqual(t, code1, code2)
		assert.NotEqual(t, sig1, sig2)
		assert.NotEmpty(t, code1)
		assert.NotEmpty(t, sig1)
	})

	t.Run("signature is deterministic for same code", func(t *testing.T) {
		code, sig, err := strategy.GenerateAuthorizeCode(context.Background(), nil)
		require.NoError(t, err)

		computedSig := strategy.AuthorizeCodeSignature(context.Background(), code)
		assert.Equal(t, sig, computedSig)
	})

}

func TestRandomCodeStrategy_ValidateAuthorizeCode(t *testing.T) {
	for _, tt := range []struct {
		name    string
		expires bool
		ttl     time.Duration
		wantErr error
	}{
		{name: "active", expires: true, ttl: time.Minute},
		{name: "expired", expires: true, ttl: -time.Minute, wantErr: fosite.ErrTokenExpired},
		{name: "at expiry", expires: true, wantErr: fosite.ErrTokenExpired},
		{name: "missing expiry", wantErr: fosite.ErrTokenExpired},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				req := fosite.NewRequest()
				req.Session = &fosite.DefaultSession{}
				if tt.expires {
					req.Session.SetExpiresAt(fosite.AuthorizeCode, time.Now().Add(tt.ttl))
				}
				err := (&RandomCodeStrategy{}).ValidateAuthorizeCode(context.Background(), req, "code")
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				} else {
					require.NoError(t, err)
				}
			})
		})
	}
}

func TestRandomRefreshTokenStrategy(t *testing.T) {
	strategy := &RandomRefreshTokenStrategy{}

	t.Run("generates unique tokens", func(t *testing.T) {
		token1, sig1, err := strategy.GenerateRefreshToken(context.Background(), nil)
		require.NoError(t, err)
		token2, sig2, err := strategy.GenerateRefreshToken(context.Background(), nil)
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2)
		assert.NotEqual(t, sig1, sig2)
		assert.NotEmpty(t, token1)
		assert.NotEmpty(t, sig1)
	})

	t.Run("signature is deterministic for same token", func(t *testing.T) {
		token, sig, err := strategy.GenerateRefreshToken(context.Background(), nil)
		require.NoError(t, err)

		computedSig := strategy.RefreshTokenSignature(context.Background(), token)
		assert.Equal(t, sig, computedSig)
	})

	t.Run("validate accepts unexpired token", func(t *testing.T) {
		req := buildTestRequest("agent", "user@example.com", []string{"offline_access"})
		req.GetSession().SetExpiresAt(fosite.RefreshToken, time.Now().Add(time.Hour))

		err := strategy.ValidateRefreshToken(context.Background(), req, "any-token")
		assert.NoError(t, err)
	})

	t.Run("validate rejects expired token", func(t *testing.T) {
		req := buildTestRequest("agent", "user@example.com", []string{"offline_access"})
		req.GetSession().SetExpiresAt(fosite.RefreshToken, time.Now().Add(-time.Minute))

		err := strategy.ValidateRefreshToken(context.Background(), req, "any-token")
		require.ErrorIs(t, err, fosite.ErrTokenExpired)
	})
}

func TestJWXAccessTokenStrategy_GenerateAccessToken(t *testing.T) {
	t.Run("no key provisioned returns actionable error", func(t *testing.T) {
		svc, _ := newStrategyTestSigningKeyService() // no key stored
		strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, nil, testSlogger())
		require.NoError(t, err)
		_, _, err = strategy.GenerateAccessToken(context.Background(), buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "POST /api/oauth2-server/signing-keys")
	})

	t.Run("JWT header contains the current signing key kid", func(t *testing.T) {
		svc, _ := newStrategyTestSigningKeyService()
		ctx := context.Background()

		key, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
		require.NoError(t, err)

		strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, nil, testSlogger())
		require.NoError(t, err)
		tokenStr, sig, err := strategy.GenerateAccessToken(ctx, buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.NoError(t, err)
		require.NotEmpty(t, sig)

		// Regression guard for privKey.Set(kid): kid must survive into the JWS header.
		msg, err := jws.Parse([]byte(tokenStr))
		require.NoError(t, err)
		require.Len(t, msg.Signatures(), 1)
		kid, ok := msg.Signatures()[0].ProtectedHeaders().KeyID()
		require.True(t, ok, "JWS protected header must carry a kid")
		assert.Equal(t, key.KID.String(), kid)
	})

	t.Run("signature equals SHA-256 of the token string", func(t *testing.T) {
		svc, _ := newStrategyTestSigningKeyService()
		_, err := svc.generateAndStore(context.Background(), "ES256", true, time.Now())
		require.NoError(t, err)

		strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, nil, testSlogger())
		require.NoError(t, err)
		tokenStr, sig, err := strategy.GenerateAccessToken(context.Background(), buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.NoError(t, err)

		assert.Equal(t, sha256Hex(tokenStr), sig)
	})

	t.Run("signature verifies against JWKS and all required claims are present", func(t *testing.T) {
		const issuer = "https://issuer.example.com"
		const agentID = "my-agent"
		const subject = "user@example.com"

		svc, _ := newStrategyTestSigningKeyService()
		ctx := context.Background()
		_, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
		require.NoError(t, err)

		strategy, err := NewJWXAccessTokenStrategy(svc, issuer, time.Hour, nil, testSlogger())
		require.NoError(t, err)

		req := buildTestRequest(agentID, subject, []string{"read", "write"})
		tokenStr, _, err := strategy.GenerateAccessToken(ctx, req)
		require.NoError(t, err)

		jwks, err := svc.BuildJWKS(ctx)
		require.NoError(t, err)

		tok, err := jwt.Parse([]byte(tokenStr), jwt.WithKeySet(jwks))
		require.NoError(t, err, "JWT signature must verify against the JWKS public key")

		iss, ok := tok.Issuer()
		require.True(t, ok, "iss must be present")
		assert.Equal(t, issuer, iss)

		sub, ok := tok.Subject()
		require.True(t, ok, "sub must be present")
		assert.Equal(t, subject, sub)

		iat, ok := tok.IssuedAt()
		require.True(t, ok, "iat must be present")
		assert.False(t, iat.IsZero())

		exp, ok := tok.Expiration()
		require.True(t, ok, "exp must be present")
		assert.True(t, exp.After(time.Now()), "exp must be in the future")

		jti, ok := tok.JwtID()
		require.True(t, ok, "jti must be present")
		assert.NotEmpty(t, jti)

		gotAgentID, err := jwt.Get[string](tok, "agent_id")
		require.NoError(t, err, "agent_id must be present")
		assert.Equal(t, req.GetClient().GetID(), gotAgentID)

		scope, err := jwt.Get[string](tok, "scope")
		require.NoError(t, err, "scope must be present")
		assert.Equal(t, "read write", scope)
	})
}

func TestNewJWXAccessTokenStrategy_Validation(t *testing.T) {
	svc, _ := newStrategyTestSigningKeyService()
	validIssuer := "https://issuer.example.com"

	issuerCases := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"empty string rejected", "", true},
		{"whitespace-only rejected", "   ", true},
		{"tab-only rejected", "\t", true},
		{"relative URI rejected", "/oauth2", true},
		{"no scheme rejected", "issuer.example.com", true},
		{"unsupported scheme rejected", "ftp://issuer.example.com", true},
		{"fragment rejected (RFC 8414)", "https://issuer.example.com#frag", true},
		{"query component rejected (RFC 8414)", "https://issuer.example.com?tenant=a", true},
		{"host-less https rejected", "https:///path", true},
		{"single-slash https rejected", "https:/issuer.example.com", true},
		{"port-only host rejected", "https://:443", true},
		{"empty query component rejected", "https://issuer.example.com?", true},
		{"whitespace-padded accepted after trim", " https://issuer.example.com ", false},
		{"valid https accepted", "https://issuer.example.com", false},
		{"valid http accepted", "http://localhost:8080", false},
		{"https with path accepted", "https://issuer.example.com/realms/myrealm", false},
	}
	for _, tc := range issuerCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewJWXAccessTokenStrategy(svc, tc.uri, time.Hour, nil, testSlogger())
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("zero tokenTTL rejected", func(t *testing.T) {
		_, err := NewJWXAccessTokenStrategy(svc, validIssuer, 0, nil, testSlogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tokenTTL")
	})

	t.Run("negative tokenTTL rejected", func(t *testing.T) {
		_, err := NewJWXAccessTokenStrategy(svc, validIssuer, -time.Second, nil, testSlogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tokenTTL")
	})

	t.Run("valid inputs accepted", func(t *testing.T) {
		strategy, err := NewJWXAccessTokenStrategy(svc, validIssuer, time.Hour, nil, testSlogger())
		require.NoError(t, err)
		assert.NotNil(t, strategy)
	})

	t.Run("whitespace-padded issuer is stored trimmed", func(t *testing.T) {
		_, err := svc.generateAndStore(context.Background(), "ES256", true, time.Now())
		require.NoError(t, err)
		strategy, err := NewJWXAccessTokenStrategy(svc, "  "+validIssuer+"  ", time.Hour, nil, testSlogger())
		require.NoError(t, err)

		tokenStr, _, err := strategy.GenerateAccessToken(context.Background(), buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.NoError(t, err)

		tok, err := jwt.ParseInsecure([]byte(tokenStr))
		require.NoError(t, err)
		iss, ok := tok.Issuer()
		require.True(t, ok)
		assert.Equal(t, validIssuer, iss, "stored issuer must be trimmed, not padded")
	})
}

func TestJWXAccessTokenStrategy_SubClaimNotOverridable(t *testing.T) {
	svc, _ := newStrategyTestSigningKeyService()
	_, err := svc.generateAndStore(context.Background(), "ES256", true, time.Now())
	require.NoError(t, err)

	eval, err := NewTokenClaimsEvaluator(`{"sub": "attacker@evil.com", "extra": "ok"}`)
	require.NoError(t, err)

	strategy, err := NewJWXAccessTokenStrategy(svc, "https://broker.example.com", time.Hour, eval, testSlogger())
	require.NoError(t, err)
	req := buildTestRequest("my-agent", "legitimate@example.com", []string{"read"})

	tokenStr, _, err := strategy.GenerateAccessToken(context.Background(), req)
	require.NoError(t, err)

	tok, err := jwt.ParseInsecure([]byte(tokenStr))
	require.NoError(t, err)
	sub, ok := tok.Subject()
	require.True(t, ok)
	assert.Equal(t, "legitimate@example.com", sub)

	extra, err := jwt.Get[string](tok, "extra")
	require.NoError(t, err, "non-reserved CEL claim should be present")
	assert.Equal(t, "ok", extra)
}

func TestJWXAccessTokenStrategy_ValidateAccessToken(t *testing.T) {
	const issuer = "https://broker.example.com"
	svc, repo := newStrategyTestSigningKeyService()
	ctx := context.Background()
	_, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
	require.NoError(t, err)

	strategy, err := NewJWXAccessTokenStrategy(svc, issuer, time.Hour, nil, testSlogger())
	require.NoError(t, err)

	t.Run("valid token passes", func(t *testing.T) {
		tokenStr, _, err := strategy.GenerateAccessToken(ctx, buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.NoError(t, err)
		assert.NoError(t, strategy.ValidateAccessToken(ctx, nil, tokenStr))
	})

	t.Run("garbage string rejected", func(t *testing.T) {
		assert.Error(t, strategy.ValidateAccessToken(ctx, nil, "not-a-jwt"))
	})

	t.Run("tampered signature rejected", func(t *testing.T) {
		tokenStr, _, err := strategy.GenerateAccessToken(ctx, buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.NoError(t, err)

		parts := strings.SplitN(tokenStr, ".", 3)
		require.Len(t, parts, 3)
		tampered := parts[0] + "." + parts[1] + ".invalidsignature"

		assert.Error(t, strategy.ValidateAccessToken(ctx, nil, tampered))
	})

	t.Run("expired token rejected", func(t *testing.T) {
		key, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		privPEM, err := svc.DecryptPrivateKey(ctx, key)
		require.NoError(t, err)
		privKey, err := jwk.ParseKey(privPEM, jwk.WithX509(true))
		require.NoError(t, err)
		_ = privKey.Set(jwk.KeyIDKey, key.KID.String())

		now := time.Now()
		expiredTok, err := jwt.NewBuilder().
			Issuer(issuer).
			Subject("user@example.com").
			IssuedAt(now.Add(-2 * time.Hour)).
			Expiration(now.Add(-time.Hour)).
			JwtID("expired-jti").
			Build()
		require.NoError(t, err)

		signed, err := jwt.Sign(expiredTok, jwt.WithKey(jwa.ES256(), privKey))
		require.NoError(t, err)

		assert.Error(t, strategy.ValidateAccessToken(ctx, nil, string(signed)))
	})

	t.Run("token from different issuer rejected", func(t *testing.T) {
		evilStrategy, err := NewJWXAccessTokenStrategy(svc, "https://evil.example.com", time.Hour, nil, testSlogger())
		require.NoError(t, err)

		tokenStr, _, err := evilStrategy.GenerateAccessToken(ctx, buildTestRequest("agent", "user@example.com", []string{"read"}))
		require.NoError(t, err)

		assert.Error(t, strategy.ValidateAccessToken(ctx, nil, tokenStr))
	})
}

// connectionErrorSigningKeyRepo returns a connection-kind StorageError from GetCurrent
// (and GetByKID) to simulate a DB outage or timeout. All other methods delegate to
// an in-memory store so the rest of the service is functional.
type connectionErrorSigningKeyRepo struct {
	*strategySigningKeyStore
}

var _ ports.SigningKeyRepository = (*connectionErrorSigningKeyRepo)(nil)

func (r *connectionErrorSigningKeyRepo) GetCurrentInDomain(_ context.Context, _ storage.KeyDomain) (*storage.SigningKey, error) {
	return nil, storage.NewStorageError(
		"SigningKeyRepo.GetCurrentInDomain",
		storage.ErrorKindConnection,
		nil,
		"connection refused",
	)
}

func (r *connectionErrorSigningKeyRepo) GetByKIDInDomain(_ context.Context, _ storage.KeyDomain, _ id.KeyID) (*storage.SigningKey, error) {
	return nil, storage.NewStorageError(
		"SigningKeyRepo.GetByKIDInDomain",
		storage.ErrorKindConnection,
		nil,
		"connection refused",
	)
}

func TestJWXAccessTokenStrategy_GetCurrent_NonNotFoundError(t *testing.T) {
	t.Run("connection error does not produce 'no signing key provisioned' message", func(t *testing.T) {
		repo := &connectionErrorSigningKeyRepo{strategySigningKeyStore: newStrategySigningKeyStore()}
		enc := &testEncryptor{}
		svc := NewSigningKeyService(repo, repo, enc, newNoopBranchKeyManager(), testSlogger())
		strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, nil, testSlogger())
		require.NoError(t, err)

		_, _, err = strategy.GenerateAccessToken(
			context.Background(),
			buildTestRequest("agent", "user@example.com", []string{"read"}),
		)
		require.Error(t, err)
		// Must NOT produce the misleading "no signing key provisioned" message.
		assert.NotContains(t, err.Error(), "no signing key provisioned",
			"connection errors must not be misreported as missing keys")
		// Must propagate the real error so operators see 'failed to get current signing key'.
		assert.Contains(t, err.Error(), "failed to get current signing key")
	})
}

func TestSHA256Hex(t *testing.T) {
	t.Run("produces consistent output", func(t *testing.T) {
		hash1 := sha256Hex("test-input")
		hash2 := sha256Hex("test-input")
		assert.Equal(t, hash1, hash2)
	})

	t.Run("produces different output for different input", func(t *testing.T) {
		hash1 := sha256Hex("input-1")
		hash2 := sha256Hex("input-2")
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("output is hex encoded", func(t *testing.T) {
		hash := sha256Hex("test")
		assert.Len(t, hash, 64) // SHA-256 produces 32 bytes = 64 hex chars
	})
}
