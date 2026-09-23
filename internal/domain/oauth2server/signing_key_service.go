package oauth2server

import (
	"context"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/keylifecycle"
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

// SigningKeyService manages signing key lifecycle including generation,
// encryption, storage, and JWKS building.
type SigningKeyService struct {
	repo       ports.SigningKeyRepository
	encryption ports.EncryptionPort
	logger     *slog.Logger
	engine     *keylifecycle.Engine
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
// The caller controls the activation time, allowing tests to bypass the jwksGracePeriod.
func (s *SigningKeyService) generateAndStore(ctx context.Context, algorithm string, makeCurrent bool, activatesAt time.Time) (*storage.SigningKey, error) {
	key, err := s.engine.GenerateAndStore(ctx, tokenSigningPolicy(), algorithm, makeCurrent, activatesAt)
	if err != nil {
		if strings.Contains(err.Error(), "provision branch key") {
			return nil, fmt.Errorf("failed to provision branch key for signing key: %w", err)
		}
		return nil, fmt.Errorf("generate and store signing key: %w", err)
	}
	s.logger.Info("signing key generated", "kid", key.KID, "algorithm", key.Algorithm, "is_current", makeCurrent, "activates_at", activatesAt)
	return key, nil
}

// All active keys are included regardless of activates_at, so new keys appear in the
// JWKS during the grace period and clients can cache them before they start signing.
func (s *SigningKeyService) BuildJWKS(ctx context.Context) (jwk.Set, error) {
	keys, err := s.repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
	if err != nil {
		return nil, fmt.Errorf("failed to list active keys: %w", err)
	}

	set := jwk.NewSet()
	processedKids := make(map[id.KeyID]struct{}, len(keys))
	for _, key := range keys {
		privPEM, err := s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, signingKeyEncCtx(key.KID))
		if err != nil {
			s.logger.Error("failed to decrypt signing key, skipping", "kid", key.KID, "error", err)
			continue
		}

		pubKey, err := keylifecycle.PublicKeyFromPEM(privPEM, key.Algorithm)
		if err != nil {
			s.logger.Error("failed to parse signing key, skipping", "kid", key.KID, "error", err)
			continue
		}

		jwkKey, err := jwk.Import[jwk.Key](pubKey)
		if err != nil {
			s.logger.Error("failed to import signing key to JWK, skipping", "kid", key.KID, "error", err)
			continue
		}

		jwaAlg, err := algorithmToJWA(key.Algorithm)
		if err != nil {
			s.logger.Error("signing key has unrecognized algorithm, skipping", "kid", key.KID, "algorithm", key.Algorithm, "error", err)
			continue
		}
		if err := setJWKMetadata(jwkKey, key.KID, jwaAlg); err != nil {
			s.logger.Error("failed to attach signing key metadata, skipping", "kid", key.KID, "error", err)
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

// ListKeys returns all active signing keys for admin listing.
func (s *SigningKeyService) ListKeys(ctx context.Context) ([]*storage.SigningKey, error) {
	return s.engine.List(ctx, tokenSigningPolicy())
}

// PromoteKey promotes a signing key and returns the updated key metadata.
func (s *SigningKeyService) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	return s.engine.Promote(ctx, tokenSigningPolicy(), kid, time.Now().UTC())
}

// GetCurrent returns the active signing key used for token signing.
func (s *SigningKeyService) GetCurrent(ctx context.Context) (*storage.SigningKey, error) {
	return s.repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
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
		Domain: storage.KeyDomainTokenSigning,
		NewKID: func() id.KeyID { return keylifecycle.UUIDKID("") },
		NewSubject: func(kid id.KeyID) (domainencryption.BranchKeySubject, error) {
			return newSigningKeySubject(kid)
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
