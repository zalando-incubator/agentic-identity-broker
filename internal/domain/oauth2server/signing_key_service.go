package oauth2server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"golang.org/x/sync/singleflight"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/keylifecycle"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Keep this aligned with the public JWKS Cache-Control max-age.
const jwksCacheMaxAge = 300 * time.Second

const jwksRebuildTimeout = 30 * time.Second

// A 300-second HTTP cache plus a 30-second rebuild leaves 270 seconds of margin
// within this grace period when storage and key publication remain healthy.
const jwksGracePeriod = 2 * jwksCacheMaxAge

const jwksCacheTTL = 45 * time.Second

var errJWKSCacheInvalidated = errors.New("JWKS cache invalidated during rebuild")

var errSignerCacheInvalidated = errors.New("signer cache invalidated during load")

// Limits reuse of decrypted signing material, even when the selected key does not change.
const signingKeyCacheTTL = 45 * time.Second

const signerLoadTimeout = 30 * time.Second

type cachedSigningKey struct {
	kid        id.KeyID
	privateKey jwk.Key
	publicJWK  []byte
	algorithm  jwa.SignatureAlgorithm
	expiresAt  time.Time
}

// SigningKeyService manages signing key lifecycle including generation,
// encryption, storage, and JWKS building.
type SigningKeyService struct {
	repo       ports.SigningKeyRepository
	encryption ports.EncryptionPort
	logger     *slog.Logger
	engine     *keylifecycle.Engine

	jwksMu         sync.RWMutex
	jwksSet        jwk.Set
	jwksExpiresAt  time.Time
	jwksVersion    int64
	jwksGeneration uint64
	jwksFlight     singleflight.Group
	signerFlight   singleflight.Group
	signerMu       sync.RWMutex
	cachedSigner   *cachedSigningKey
	signerTimer    *time.Timer
	signerVersion  uint64
}

// NewSigningKeyService creates a new SigningKeyService.
func NewSigningKeyService(
	repo ports.SigningKeyRepository,
	bootstrapCoordinator ports.SigningKeyBootstrapCoordinator,
	encryption ports.EncryptionPort,
	branchKeyManager ports.BranchKeyManager,
	logger *slog.Logger,
) *SigningKeyService {
	return &SigningKeyService{
		repo:       repo,
		encryption: encryption,
		logger:     logger,
		engine:     keylifecycle.NewEngine(repo, bootstrapCoordinator, encryption, branchKeyManager, logger),
	}
}

// GenerateAndStoreKey stores a new signing key. A new current key waits for the
// publication grace period before signing; independent verifiers must refresh
// their JWKS caches to discover it.
func (s *SigningKeyService) GenerateAndStoreKey(ctx context.Context, algorithm string, makeCurrent bool) (*storage.SigningKey, error) {
	return s.generateAndStore(ctx, algorithm, makeCurrent, time.Time{})
}

// generateAndStore accepts an explicit activation time for bootstrap and tests.
// A zero time selects the normal publication grace period immediately before storage.
func (s *SigningKeyService) generateAndStore(ctx context.Context, algorithm string, makeCurrent bool, activatesAt time.Time) (*storage.SigningKey, error) {
	key, err := s.engine.GenerateAndStore(ctx, tokenSigningPolicy(), algorithm, makeCurrent, activatesAt)
	if err != nil {
		if strings.Contains(err.Error(), "provision branch key") {
			return nil, fmt.Errorf("failed to provision branch key for signing key: %w", err)
		}
		return nil, fmt.Errorf("generate and store signing key: %w", err)
	}
	s.invalidateJWKSCache()
	s.invalidateSigner()

	s.logger.Info("signing key generated", "kid", key.KID, "algorithm", key.Algorithm, "is_current", makeCurrent, "activates_at", key.ActivatesAt)
	return key, nil
}

// All active keys are included regardless of activates_at, so new keys appear in the
// JWKS during the grace period and clients can cache them before they start signing.
func (s *SigningKeyService) BuildJWKS(ctx context.Context) (jwk.Set, error) {
	for {
		version, err := s.repo.KeySetVersion(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check signing key set version: %w", err)
		}
		now := time.Now().UTC()
		s.jwksMu.RLock()
		if s.jwksSet != nil && s.jwksVersion == version && now.Before(s.jwksExpiresAt) {
			set := s.jwksSet
			s.jwksMu.RUnlock()
			return set, nil
		}
		s.jwksMu.RUnlock()

		s.jwksMu.Lock()
		if s.jwksVersion > version {
			s.jwksMu.Unlock()
			continue
		}
		if s.jwksVersion != version {
			s.jwksGeneration++
			s.jwksSet = nil
			s.jwksVersion = version
		}
		now = time.Now().UTC()
		if s.jwksSet != nil && now.Before(s.jwksExpiresAt) {
			set := s.jwksSet
			s.jwksMu.Unlock()
			return set, nil
		}
		generation := s.jwksGeneration
		resultCh := s.jwksFlight.DoChan(strconv.FormatUint(generation, 10), func() (interface{}, error) {
			rebuildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jwksRebuildTimeout)
			defer cancel()
			return s.rebuildJWKS(rebuildCtx, generation, version)
		})
		s.jwksMu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultCh:
			s.jwksMu.RLock()
			currentGeneration := s.jwksGeneration
			s.jwksMu.RUnlock()
			if currentGeneration != generation {
				continue
			}
			if errors.Is(result.Err, errJWKSCacheInvalidated) {
				continue
			}
			if result.Err != nil {
				return nil, result.Err
			}

			set, ok := result.Val.(jwk.Set)
			if !ok {
				return nil, fmt.Errorf("failed to build JWKS: unexpected rebuild result %T", result.Val)
			}
			return set, nil
		}
	}
}

func (s *SigningKeyService) rebuildJWKS(ctx context.Context, generation uint64, version int64) (jwk.Set, error) {
	set, err := s.buildJWKS(ctx)
	if err != nil {
		return nil, err
	}
	currentVersion, err := s.repo.KeySetVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check signing key set version after rebuild: %w", err)
	}
	if currentVersion != version {
		return nil, errJWKSCacheInvalidated
	}

	s.jwksMu.Lock()
	defer s.jwksMu.Unlock()
	if s.jwksGeneration != generation {
		return nil, errJWKSCacheInvalidated
	}
	s.jwksSet = set
	s.jwksExpiresAt = time.Now().UTC().Add(jwksCacheTTL)
	return set, nil
}

func (s *SigningKeyService) buildJWKS(ctx context.Context) (jwk.Set, error) {
	keys, err := s.repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
	if err != nil {
		return nil, fmt.Errorf("failed to list active keys: %w", err)
	}

	set := jwk.NewSet()
	processedKids := make(map[id.KeyID]struct{}, len(keys))
	for _, key := range keys {
		if key == nil {
			s.logger.Error("nil signing key returned from repository, skipping")
			continue
		}

		jwkKey, err := s.publicJWKForSigningKey(ctx, key)
		if err != nil {
			s.logger.Error("failed to build signing key public JWK, skipping", "kid", key.KID, "error", err)
			continue
		}
		if err := set.AddKey(jwkKey); err != nil {
			s.logger.Error("failed to add signing key to JWKS, skipping", "kid", key.KID, "error", err)
			continue
		}
		processedKids[key.KID] = struct{}{}
	}

	if current := keylifecycle.EffectiveCurrent(keys, time.Now().UTC()); current != nil {
		if _, ok := processedKids[current.KID]; !ok {
			return nil, fmt.Errorf("failed to build JWKS: current signing key %s is missing from JWKS", current.KID)
		}
	}
	if len(keys) > 0 && set.Len() == 0 {
		return nil, fmt.Errorf("failed to build JWKS: all %d active key(s) failed processing", len(keys))
	}

	return set, nil
}

func (s *SigningKeyService) publicJWKForSigningKey(ctx context.Context, key *storage.SigningKey) (jwk.Key, error) {
	if len(key.PublicJWK) > 0 {
		jwkKey, err := publicJWKFromJSON(key.PublicJWK, key.KID, key.Algorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stored public JWK: %w", err)
		}
		return jwkKey, nil
	}

	jwkKey, publicJWK, err := s.publicJWKFromLegacyKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to build legacy public JWK: %w", err)
	}
	fingerprint, err := jwkKey.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to fingerprint derived public JWK: %w", err)
	}
	updated, err := s.repo.SetPublicJWK(ctx, key.KID, publicJWK)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			return nil, fmt.Errorf("failed to backfill public JWK: %w", err)
		}
		s.logger.Warn("failed to backfill public JWK; publishing derived key", "kid", key.KID, "error", err)
		return jwkKey, nil
	}
	outcome := "written"
	if !updated {
		stored, err := s.repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key.KID)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing public JWK after backfill race: %w", err)
		}
		publicKey, err := publicJWKFromJSON(stored.PublicJWK, stored.KID, stored.Algorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing public JWK after backfill race: %w", err)
		}
		fingerprint, err = publicKey.Thumbprint(crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("failed to fingerprint existing public JWK: %w", err)
		}
		outcome = "already_set"
	}
	s.logger.InfoContext(ctx, "signing key public JWK backfill", "kid", key.KID, "outcome", outcome,
		"at", time.Now().UTC(), "public_key_fingerprint", base64.RawURLEncoding.EncodeToString(fingerprint))
	return jwkKey, nil
}

func (s *SigningKeyService) invalidateJWKSCache() {
	s.jwksMu.Lock()
	defer s.jwksMu.Unlock()
	s.jwksGeneration++
	s.jwksSet = nil
	s.jwksExpiresAt = time.Time{}
}

// ListKeys returns all active signing keys for admin listing.
func (s *SigningKeyService) ListKeys(ctx context.Context) ([]*storage.SigningKey, error) {
	return s.engine.List(ctx, tokenSigningPolicy())
}

// PromoteKey promotes a signing key and returns the updated key metadata.
func (s *SigningKeyService) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	key, err := s.engine.Promote(ctx, tokenSigningPolicy(), kid, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	s.invalidateJWKSCache()
	s.invalidateSigner()
	return key, nil
}

// GetCurrent returns the active signing key used for token signing.
func (s *SigningKeyService) GetCurrent(ctx context.Context) (*storage.SigningKey, error) {
	return s.repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
}

func (s *SigningKeyService) signingMaterial(ctx context.Context) (*cachedSigningKey, error) {
	for {
		key, err := s.GetCurrent(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get current signing key: %w", err)
		}

		s.signerMu.RLock()
		if signer := s.cachedSigner; signer != nil && len(key.PublicJWK) > 0 && signer.kid == key.KID && bytes.Equal(signer.publicJWK, key.PublicJWK) && time.Now().Before(signer.expiresAt) {
			s.signerMu.RUnlock()
			return signer, nil
		}
		s.signerMu.RUnlock()

		s.signerMu.Lock()
		if signer := s.cachedSigner; signer != nil && len(key.PublicJWK) > 0 && signer.kid == key.KID && bytes.Equal(signer.publicJWK, key.PublicJWK) && time.Now().Before(signer.expiresAt) {
			s.signerMu.Unlock()
			return signer, nil
		}
		s.clearSignerLocked()
		generation := s.signerVersion
		flightKey := strconv.FormatUint(generation, 10) + ":" + key.KID.String() + ":" + string(key.PublicJWK)
		resultCh := s.signerFlight.DoChan(flightKey, func() (interface{}, error) {
			return s.loadAndCacheSigner(ctx, key, generation)
		})
		s.signerMu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultCh:
			if errors.Is(result.Err, errSignerCacheInvalidated) {
				continue
			}
			if result.Err != nil {
				return nil, result.Err
			}
			return result.Val.(*cachedSigningKey), nil
		}
	}
}

func (s *SigningKeyService) loadAndCacheSigner(ctx context.Context, key *storage.SigningKey, generation uint64) (*cachedSigningKey, error) {
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), signerLoadTimeout)
	defer cancel()
	signer, err := s.loadSigner(loadCtx, key)

	s.signerMu.Lock()
	defer s.signerMu.Unlock()
	if s.signerVersion != generation {
		return nil, errSignerCacheInvalidated
	}
	if err != nil {
		return nil, err
	}
	if len(key.PublicJWK) > 0 {
		s.cachedSigner = signer
		s.signerVersion++
		version := s.signerVersion
		s.signerTimer = time.AfterFunc(time.Until(signer.expiresAt), func() {
			s.signerMu.Lock()
			defer s.signerMu.Unlock()
			if s.signerVersion == version {
				s.clearSignerLocked()
			}
		})
	}
	return signer, nil
}

func (s *SigningKeyService) loadSigner(ctx context.Context, key *storage.SigningKey) (*cachedSigningKey, error) {
	algorithm, err := algorithmToJWA(key.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("signing key %s has unrecognized algorithm %q: %w", key.KID, key.Algorithm, err)
	}
	privatePEM, err := s.DecryptPrivateKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt signing key: %w", err)
	}
	privateKey, err := jwk.ParseKey(privatePEM, jwk.WithX509(true))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	if len(key.PublicJWK) > 0 {
		publicKey, err := jwk.PublicKeyOf(privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to derive signing public key: %w", err)
		}
		storedPublicKey, err := publicJWKFromJSON(key.PublicJWK, key.KID, key.Algorithm)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stored signing public JWK: %w", err)
		}
		derivedThumbprint, err := publicKey.Thumbprint(crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("failed to fingerprint signing public key: %w", err)
		}
		storedThumbprint, err := storedPublicKey.Thumbprint(crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("failed to fingerprint stored signing public key: %w", err)
		}
		if !bytes.Equal(derivedThumbprint, storedThumbprint) {
			return nil, fmt.Errorf("signing key %s public key does not match private key", key.KID)
		}
	}
	if err := privateKey.Set(jwk.KeyIDKey, key.KID.String()); err != nil {
		return nil, fmt.Errorf("failed to set signing key ID: %w", err)
	}
	return &cachedSigningKey{kid: key.KID, privateKey: privateKey, publicJWK: key.PublicJWK, algorithm: algorithm, expiresAt: time.Now().Add(signingKeyCacheTTL)}, nil
}

func (s *SigningKeyService) invalidateSigner() {
	s.signerMu.Lock()
	s.clearSignerLocked()
	s.signerVersion++
	s.signerMu.Unlock()
}

func (s *SigningKeyService) clearSignerLocked() {
	if s.signerTimer != nil {
		s.signerTimer.Stop()
		s.signerTimer = nil
	}
	s.cachedSigner = nil
}

// CountActive returns the number of non-removed signing keys.
func (s *SigningKeyService) CountActive(ctx context.Context) (int, error) {
	return s.repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
}

// EnsureInitialKey creates a single immediately-active signing key when none exist.
func (s *SigningKeyService) EnsureInitialKey(ctx context.Context, algorithm string) (*storage.SigningKey, bool, error) {
	if algorithm == "" {
		algorithm = "ES256"
	}
	if algorithm != "ES256" {
		return nil, false, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
	key, created, err := s.engine.EnsureInitialKey(ctx, tokenSigningPolicy(), time.Now().UTC().Add(-time.Second))
	if err != nil {
		if strings.Contains(err.Error(), "count active keys") {
			return nil, false, fmt.Errorf("failed to count active keys: %w", err)
		}
		return nil, false, fmt.Errorf("failed to generate initial signing key: %w", err)
	}
	return key, created, nil
}

// DeleteKey removes a signing key after validating lifecycle invariants in the shared engine.
func (s *SigningKeyService) DeleteKey(ctx context.Context, kid id.KeyID) error {
	if err := s.engine.Delete(ctx, tokenSigningPolicy(), kid, time.Now().UTC()); err != nil {
		return fmt.Errorf("delete signing key: %w", err)
	}
	s.invalidateJWKSCache()
	s.invalidateSigner()
	return nil
}

// DecryptPrivateKey decrypts the private key material of a signing key.
func (s *SigningKeyService) DecryptPrivateKey(ctx context.Context, key *storage.SigningKey) ([]byte, error) {
	return s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, signingKeyEncCtx(key.KID))
}

func newSigningKeySubject(kid id.KeyID) (domainencryption.BranchKeySubject, error) {
	subject := domainencryption.NewSigningKeyBranchKeySubject(kid)
	if err := subject.Validate(); err != nil {
		return domainencryption.BranchKeySubject{}, fmt.Errorf("invalid signing key subject: %w", err)
	}
	return subject, nil
}

// signingKeyEncCtx returns the encryption context AAD for a signing key.
// The signing-key subject uses the well-known JWT kid term in AAD and routes the
// hierarchical keyring to the dedicated signing-key branch key namespace.
func signingKeyEncCtx(kid id.KeyID) map[string]string {
	return domainencryption.NewSigningKeyBranchKeySubject(kid).EncryptionContext()
}

func tokenSigningPolicy() keylifecycle.Policy {
	return keylifecycle.Policy{
		Domain:          storage.KeyDomainTokenSigning,
		ActivationGrace: jwksGracePeriod,
		NewKID:          func() id.KeyID { return keylifecycle.UUIDKID("") },
		NewSubject: func(kid id.KeyID) (domainencryption.BranchKeySubject, error) {
			return newSigningKeySubject(kid)
		},
		PublicJWK: func(privatePEM []byte, kid id.KeyID, algorithm string) ([]byte, error) {
			_, publicJWK, err := publicJWKAndJSONFromPrivatePEM(privatePEM, kid, algorithm)
			return publicJWK, err
		},
	}
}

func generateES256KeyPEM() ([]byte, error) {
	return keylifecycle.GenerateES256PEM()
}

func encodePEMBlock(block *pem.Block) ([]byte, error) {
	if block == nil {
		return nil, fmt.Errorf("PEM block is required")
	}

	encoded := pem.EncodeToMemory(block)
	if encoded == nil {
		return nil, fmt.Errorf("failed to encode PEM block")
	}

	return encoded, nil
}

func (s *SigningKeyService) publicJWKFromLegacyKey(ctx context.Context, key *storage.SigningKey) (jwk.Key, []byte, error) {
	privatePEM, err := s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, signingKeyEncCtx(key.KID))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt signing key: %w", err)
	}
	return publicJWKAndJSONFromPrivatePEM(privatePEM, key.KID, key.Algorithm)
}

func publicJWKAndJSONFromPrivatePEM(privPEM []byte, kid id.KeyID, algorithm string) (jwk.Key, []byte, error) {
	publicKey, err := publicKeyFromPEM(privPEM, algorithm)
	if err != nil {
		return nil, nil, err
	}

	key, err := jwk.Import[jwk.Key](publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to import public key to JWK: %w", err)
	}
	return publicJWKAndJSON(key, kid, algorithm)
}

func publicJWKFromJSON(publicJWK []byte, kid id.KeyID, algorithm string) (jwk.Key, error) {
	key, err := jwk.ParseKey(publicJWK)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public JWK: %w", err)
	}

	publicKey, err := sanitizePublicJWK(key, kid, algorithm)
	if err != nil {
		return nil, err
	}
	return publicKey, nil
}

func publicJWKAndJSON(key jwk.Key, kid id.KeyID, algorithm string) (jwk.Key, []byte, error) {
	publicKey, err := sanitizePublicJWK(key, kid, algorithm)
	if err != nil {
		return nil, nil, err
	}

	publicJWK, err := json.Marshal(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public JWK: %w", err)
	}
	return publicKey, publicJWK, nil
}

func sanitizePublicJWK(key jwk.Key, kid id.KeyID, algorithm string) (jwk.Key, error) {
	publicKey, err := jwk.PublicKeyOf(key)
	if err != nil {
		return nil, fmt.Errorf("failed to derive public JWK: %w", err)
	}

	asymmetricKey, ok := publicKey.(jwk.AsymmetricKey)
	if !ok || asymmetricKey.IsPrivate() {
		return nil, fmt.Errorf("public JWK must contain an asymmetric public key")
	}

	switch algorithm {
	case "ES256":
		ecKey, err := jwk.Export[*ecdsa.PublicKey](publicKey)
		if err != nil {
			return nil, fmt.Errorf("public JWK does not contain an ES256 key: %w", err)
		}
		if ecKey.Curve != elliptic.P256() {
			return nil, fmt.Errorf("public JWK does not contain a P-256 key")
		}
		publicKey, err = jwk.Import[jwk.Key](ecKey)
		if err != nil {
			return nil, fmt.Errorf("failed to import ES256 public JWK: %w", err)
		}
	case "RS256":
		rsaKey, err := jwk.Export[*rsa.PublicKey](publicKey)
		if err != nil {
			return nil, fmt.Errorf("public JWK does not contain an RS256 key: %w", err)
		}
		publicKey, err = jwk.Import[jwk.Key](rsaKey)
		if err != nil {
			return nil, fmt.Errorf("failed to import RS256 public JWK: %w", err)
		}
	default:
		return nil, fmt.Errorf("unrecognized algorithm: %q", algorithm)
	}

	jwaAlgorithm, err := algorithmToJWA(algorithm)
	if err != nil {
		return nil, err
	}
	if err := setJWKMetadata(publicKey, kid, jwaAlgorithm); err != nil {
		return nil, err
	}
	return publicKey, nil
}

func publicKeyFromPEM(privPEM []byte, algorithm string) (interface{}, error) {
	return keylifecycle.PublicKeyFromPEM(privPEM, algorithm)
}

func algorithmToJWA(algorithm string) (jwa.SignatureAlgorithm, error) {
	switch algorithm {
	case "ES256":
		return jwa.ES256(), nil
	case "RS256":
		return jwa.RS256(), nil
	default:
		return jwa.SignatureAlgorithm{}, fmt.Errorf("unrecognized algorithm: %q", algorithm)
	}
}

func setJWKMetadata(jwkKey jwk.Key, kid id.KeyID, algorithm jwa.SignatureAlgorithm) error {
	if err := jwkKey.Set(jwk.KeyIDKey, kid.String()); err != nil {
		return fmt.Errorf("failed to set kid: %w", err)
	}
	if err := jwkKey.Set(jwk.AlgorithmKey, algorithm); err != nil {
		return fmt.Errorf("failed to set alg: %w", err)
	}
	if err := jwkKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return fmt.Errorf("failed to set use: %w", err)
	}
	return nil
}
