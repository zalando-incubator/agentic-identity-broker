package oauth2server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"golang.org/x/sync/singleflight"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// jwksCacheMaxAge is the max-age value (in seconds) sent in the Cache-Control header on
// GET /oauth2/jwks.json. The grace period below must be a multiple of this value.
// Keep in sync with the constant in internal/adapters/http/handlers/enduser/jwks_handler.go.
const jwksCacheMaxAge = 300 * time.Second

// jwksGracePeriod is how long a newly created current key waits before it starts signing tokens.
// During this window the key is already present in the JWKS response, so every client cache
// will have learned about it before the first token signed with it appears.
const jwksGracePeriod = 2 * jwksCacheMaxAge

const jwksCacheTTL = 45 * time.Second

var errJWKSCacheInvalidated = errors.New("JWKS cache invalidated during rebuild")

const bootstrapRecoveryProbeTimeout = 5 * time.Second

// SigningKeyService manages signing key lifecycle including generation,
// encryption, storage, and JWKS building.
type SigningKeyService struct {
	repo                 ports.SigningKeyRepository
	bootstrapCoordinator ports.SigningKeyBootstrapCoordinator
	encryption           ports.EncryptionPort
	branchKeyManager     ports.BranchKeyManager
	logger               *slog.Logger

	jwksMu         sync.RWMutex
	jwksSet        jwk.Set
	jwksExpiresAt  time.Time
	jwksGeneration uint64
	jwksFlight     singleflight.Group
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
		repo:                 repo,
		bootstrapCoordinator: bootstrapCoordinator,
		encryption:           encryption,
		branchKeyManager:     branchKeyManager,
		logger:               logger,
	}
}

// GenerateAndStoreKey generates a new ES256 signing key, encrypts the private material,
// and stores it. When makeCurrent is true the key is marked as current but will not begin
// signing tokens until jwksGracePeriod has elapsed, giving JWKS caches time to pick up the
// new key before any token signed with it is issued.
func (s *SigningKeyService) GenerateAndStoreKey(ctx context.Context, algorithm string, makeCurrent bool) (*storage.SigningKey, error) {
	activatesAt := time.Now().UTC()
	if makeCurrent {
		activatesAt = activatesAt.Add(jwksGracePeriod)
	}
	return s.generateAndStore(ctx, algorithm, makeCurrent, activatesAt)
}

// generateAndStore creates and persists a signing key with an explicit activatesAt timestamp.
// The caller controls the activation time, allowing tests to bypass the
// jwksGracePeriod that GenerateAndStoreKey applies.
func (s *SigningKeyService) generateAndStore(ctx context.Context, algorithm string, makeCurrent bool, activatesAt time.Time) (*storage.SigningKey, error) {
	if algorithm == "" {
		algorithm = "ES256"
	}

	kid := id.NewKeyID(uuid.New().String())

	var privKeyPEM []byte
	var err error

	switch algorithm {
	case "ES256":
		privKeyPEM, err = generateES256KeyPEM()
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	publicJWK, err := publicJWKFromPrivatePEM(privKeyPEM, kid, algorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to derive public JWK: %w", err)
	}

	signingKeySubject, err := newSigningKeySubject(kid)
	if err != nil {
		return nil, err
	}

	branchKeyID, err := s.branchKeyManager.Create(ctx, signingKeySubject)
	if err != nil {
		return nil, fmt.Errorf("failed to provision branch key for signing key: %w", err)
	}

	encrypted, err := s.encryption.Encrypt(ctx, privKeyPEM, signingKeyEncCtx(kid))
	if err != nil {
		s.warnOrphanedBranchKey("orphaned branch key after encryption failure; manual cleanup required", kid, branchKeyID)
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 kid,
		Algorithm:           algorithm,
		PrivateKeyEncrypted: encrypted,
		PublicJWK:           publicJWK,
		IsCurrent:           makeCurrent,
		ActivatesAt:         activatesAt,
		CreatedAt:           time.Now().UTC(),
	}

	if makeCurrent {
		if err := s.repo.CreateAndSetCurrent(ctx, key); err != nil {
			s.warnOrphanedBranchKey("orphaned branch key after storage failure; manual cleanup required", kid, branchKeyID)
			return nil, fmt.Errorf("failed to store and promote signing key: %w", err)
		}
	} else {
		if err := s.repo.Create(ctx, key); err != nil {
			s.warnOrphanedBranchKey("orphaned branch key after storage failure; manual cleanup required", kid, branchKeyID)
			return nil, fmt.Errorf("failed to store signing key: %w", err)
		}
	}
	s.invalidateJWKSCache()

	s.logger.Info("signing key generated", "kid", kid, "algorithm", algorithm, "is_current", makeCurrent, "activates_at", activatesAt)
	return key, nil
}

// BuildJWKS constructs a JWK Set from all active signing keys (public keys only).
// All active keys are included regardless of activates_at, so new keys appear in the
// JWKS during the grace period and clients can cache them before they start signing.
func (s *SigningKeyService) BuildJWKS(ctx context.Context) (jwk.Set, error) {
	for {
		now := time.Now().UTC()
		s.jwksMu.RLock()
		if s.jwksSet != nil && now.Before(s.jwksExpiresAt) {
			set := s.jwksSet
			s.jwksMu.RUnlock()
			return set, nil
		}
		s.jwksMu.RUnlock()

		s.jwksMu.Lock()
		now = time.Now().UTC()
		if s.jwksSet != nil && now.Before(s.jwksExpiresAt) {
			set := s.jwksSet
			s.jwksMu.Unlock()
			return set, nil
		}
		generation := s.jwksGeneration
		resultCh := s.jwksFlight.DoChan(strconv.FormatUint(generation, 10), func() (interface{}, error) {
			return s.rebuildJWKS(context.WithoutCancel(ctx), generation)
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

func (s *SigningKeyService) rebuildJWKS(ctx context.Context, generation uint64) (jwk.Set, error) {
	set, err := s.buildJWKS(ctx)
	if err != nil {
		return nil, err
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
	keys, err := s.repo.ListActive(ctx)
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

	if current := currentSigningKey(keys, time.Now().UTC()); current != nil {
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
	if err := s.repo.SetPublicJWK(ctx, key.KID, publicJWK); err != nil {
		return nil, fmt.Errorf("failed to backfill public JWK: %w", err)
	}
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
	return s.repo.ListActive(ctx)
}

// PromoteKey promotes a signing key and returns the updated key metadata.
// Admin-driven promotion takes effect immediately because the target key is already
// present in JWKS and the operator explicitly requested activation now.
func (s *SigningKeyService) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	key, err := s.repo.SetCurrent(ctx, kid, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	s.invalidateJWKSCache()
	return key, nil
}

// GetCurrent returns the active signing key used for token signing.
func (s *SigningKeyService) GetCurrent(ctx context.Context) (*storage.SigningKey, error) {
	return s.repo.GetCurrent(ctx)
}

// CountActive returns the number of non-removed signing keys.
func (s *SigningKeyService) CountActive(ctx context.Context) (int, error) {
	return s.repo.CountActive(ctx)
}

// EnsureInitialKey creates a single immediately-active signing key when none exist.
// The repository-backed bootstrap lock serializes this check-and-create flow across
// replicas so concurrent startup cannot generate multiple initial keys. The bootstrap
// callback may call repository methods that open their own transactions; correctness
// depends on every bootstrap caller acquiring the same lock before entering the
// callback, not on reusing the outer lock transaction for the inner write path.
func (s *SigningKeyService) EnsureInitialKey(ctx context.Context, algorithm string) (*storage.SigningKey, bool, error) {
	var created *storage.SigningKey

	err := s.bootstrapCoordinator.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
		count, err := s.repo.CountActive(lockCtx)
		if err != nil {
			return fmt.Errorf("failed to count active keys: %w", err)
		}
		if count > 0 {
			return nil
		}

		// Bootstrap activates effectively immediately because no prior JWKS caches exist to
		// invalidate. Backdate by one second so a just-created key is readable even when the
		// broker clock is slightly ahead of PostgreSQL during concurrent startup.
		created, err = s.generateAndStore(lockCtx, algorithm, true, time.Now().UTC().Add(-time.Second))
		if err != nil {
			return fmt.Errorf("failed to generate initial signing key: %w", err)
		}
		return nil
	})
	if err != nil {
		if created != nil || isBootstrapTimeoutOrCancellation(err) {
			recoveryCtx, cancel := context.WithTimeout(context.Background(), bootstrapRecoveryProbeTimeout)
			defer cancel()

			count, countErr := s.repo.CountActive(recoveryCtx)
			if countErr != nil {
				s.logger.Warn("bootstrap recovery count probe failed",
					"created_locally", created != nil,
					"error", err,
					"recovery_error", countErr)
			} else if count > 0 {
				s.logger.Warn("bootstrap lock returned error after signing key bootstrap work; treating startup as recovered",
					"active_key_count", count,
					"created_locally", created != nil,
					"error", err)
				return created, created != nil, nil
			}
		}
		return nil, false, err
	}
	return created, created != nil, nil
}

// DeleteKey removes a signing key after validating lifecycle invariants in the
// domain service. Repository implementations still enforce the same rules
// atomically as a fail-closed backstop for concurrent delete/promotion races.
func (s *SigningKeyService) DeleteKey(ctx context.Context, kid id.KeyID) error {
	key, err := s.repo.GetByKID(ctx, kid)
	if err != nil {
		return fmt.Errorf("failed to get signing key: %w", err)
	}

	count, err := s.repo.CountActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to count active keys: %w", err)
	}
	if count <= 1 {
		return ports.ErrLastActiveKey
	}
	if key.IsCurrent {
		return ports.ErrCurrentKey
	}

	keys, err := s.repo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active keys: %w", err)
	}
	if current := currentSigningKey(keys, time.Now().UTC()); current != nil && current.KID == kid {
		return ports.ErrEffectiveCurrentKey
	}
	if err := s.repo.Delete(ctx, kid); err != nil {
		return err
	}
	s.invalidateJWKSCache()
	return nil
}

// DecryptPrivateKey decrypts the private key material of a signing key.
func (s *SigningKeyService) DecryptPrivateKey(ctx context.Context, key *storage.SigningKey) ([]byte, error) {
	return s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, signingKeyEncCtx(key.KID))
}

func isBootstrapTimeoutOrCancellation(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var storageErr *storage.StorageError
	return errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindTimeout
}

func currentSigningKey(keys []*storage.SigningKey, now time.Time) *storage.SigningKey {
	var best *storage.SigningKey
	for _, key := range keys {
		if key == nil || key.ActivatesAt.After(now) {
			continue
		}
		if best == nil || (!best.IsCurrent && key.IsCurrent) || (best.IsCurrent == key.IsCurrent && key.ActivatesAt.After(best.ActivatesAt)) {
			best = key
		}
	}
	return best
}

func newSigningKeySubject(kid id.KeyID) (domainencryption.BranchKeySubject, error) {
	subject := domainencryption.NewSigningKeyBranchKeySubject(kid)
	if err := subject.Validate(); err != nil {
		return domainencryption.BranchKeySubject{}, fmt.Errorf("invalid signing key subject: %w", err)
	}
	return subject, nil
}

func (s *SigningKeyService) warnOrphanedBranchKey(message string, kid id.KeyID, branchKeyID string) {
	if branchKeyID == "" {
		s.logger.Debug("skipping orphan warning: branch key ID is empty (noop backend or unexpected empty return)", "kid", kid)
		return
	}
	s.logger.Warn(message, "kid", kid, "branch_key_id", branchKeyID)
}

// signingKeyEncCtx returns the encryption context AAD for a signing key.
// The signing-key subject uses the well-known JWT kid term in AAD and routes the
// hierarchical keyring to the dedicated signing-key branch key namespace.
func signingKeyEncCtx(kid id.KeyID) map[string]string {
	return domainencryption.NewSigningKeyBranchKeySubject(kid).EncryptionContext()
}

func generateES256KeyPEM() ([]byte, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
	}

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	}

	return encodePEMBlock(pemBlock)
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

func publicJWKFromPrivatePEM(privPEM []byte, kid id.KeyID, algorithm string) ([]byte, error) {
	_, publicJWK, err := publicJWKAndJSONFromPrivatePEM(privPEM, kid, algorithm)
	return publicJWK, err
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
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PKCS8 private key: %w", err)
	}

	switch algorithm {
	case "ES256":
		ecKey, ok := privKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("expected ECDSA private key, got %T", privKey)
		}
		return &ecKey.PublicKey, nil
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
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
