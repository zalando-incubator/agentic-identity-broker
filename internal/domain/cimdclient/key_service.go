package cimdclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const cimdKeyGracePeriod = 10 * time.Minute

// KeyService manages the dedicated CIMD client-authentication key domain.
type KeyService struct {
	repository           ports.SigningKeyRepository
	bootstrapCoordinator ports.SigningKeyBootstrapCoordinator
	encryption           ports.EncryptionPort
	branchKeyManager     ports.BranchKeyManager
	logger               *slog.Logger
}

// NewKeyService creates a CIMD key-domain service.
func NewKeyService(
	repository ports.SigningKeyRepository,
	bootstrapCoordinator ports.SigningKeyBootstrapCoordinator,
	encryption ports.EncryptionPort,
	branchKeyManager ports.BranchKeyManager,
	logger *slog.Logger,
) *KeyService {
	if logger == nil {
		logger = slog.Default()
	}
	return &KeyService{
		repository:           repository,
		bootstrapCoordinator: bootstrapCoordinator,
		encryption:           encryption,
		branchKeyManager:     branchKeyManager,
		logger:               logger,
	}
}

// GenerateKey creates a CIMD ES256 key. The first key is immediately usable;
// later keys wait through the publication grace period.
func (s *KeyService) GenerateKey(ctx context.Context, algorithm string) (*storage.SigningKey, error) {
	if algorithm == "" {
		algorithm = "ES256"
	}
	if algorithm != "ES256" {
		s.audit("", "generate", "rejected")
		return nil, ErrUnsupportedCIMDAlgorithm
	}

	count, err := s.repository.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	if err != nil {
		s.audit("", "generate", "rejected")
		return nil, fmt.Errorf("count CIMD keys: %w", err)
	}
	activatesAt := time.Now().UTC()
	if count > 0 {
		activatesAt = activatesAt.Add(cimdKeyGracePeriod)
	}
	key, err := s.generateAndStore(ctx, activatesAt)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// ListKeys returns active CIMD client-authentication key metadata.
func (s *KeyService) ListKeys(ctx context.Context) ([]*storage.SigningKey, error) {
	return s.repository.ListActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
}

// PromoteKey makes an existing CIMD key immediately current.
func (s *KeyService) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	key, err := s.repository.SetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, kid, time.Now().UTC())
	if err != nil {
		s.audit(kid, "promote", "rejected")
		return nil, err
	}
	s.audit(key.KID, "promote", "success")
	return key, nil
}

// DeleteKey removes a non-current, non-effective CIMD key while preserving domain guards.
func (s *KeyService) DeleteKey(ctx context.Context, kid id.KeyID) error {
	key, err := s.repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, kid)
	if err != nil {
		s.audit(kid, "remove", "rejected")
		return err
	}
	count, err := s.repository.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	if err != nil {
		s.audit(kid, "remove", "rejected")
		return fmt.Errorf("count CIMD keys: %w", err)
	}
	if count <= 1 {
		s.audit(kid, "remove", "rejected")
		return ports.ErrLastActiveKey
	}
	if key.IsCurrent {
		s.audit(kid, "remove", "rejected")
		return ports.ErrCurrentKey
	}
	keys, err := s.repository.ListActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	if err != nil {
		s.audit(kid, "remove", "rejected")
		return fmt.Errorf("list CIMD keys: %w", err)
	}
	if current := currentUsableKey(keys, time.Now().UTC()); current != nil && current.KID == kid {
		s.audit(kid, "remove", "rejected")
		return ports.ErrEffectiveCurrentKey
	}
	if err := s.repository.DeleteInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, kid); err != nil {
		s.audit(kid, "remove", "rejected")
		return err
	}
	s.audit(kid, "remove", "success")
	return nil
}

// EnsureInitialKey creates one immediately usable CIMD key when the domain is empty.
func (s *KeyService) EnsureInitialKey(ctx context.Context) (*storage.SigningKey, bool, error) {
	var created *storage.SigningKey
	err := s.bootstrapCoordinator.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
		count, err := s.repository.CountActiveInDomain(lockCtx, storage.KeyDomainCIMDClientAuthentication)
		if err != nil {
			return fmt.Errorf("count CIMD keys: %w", err)
		}
		if count > 0 {
			return nil
		}
		created, err = s.generateAndStore(lockCtx, time.Now().UTC())
		return err
	})
	if err != nil {
		s.audit("", "bootstrap", "rejected")
		return nil, false, err
	}
	return created, created != nil, nil
}

// RequirePublishedKey confirms that at least one public CIMD verification key is usable.
func (s *KeyService) RequirePublishedKey(ctx context.Context) error {
	set, err := s.PublicJWKSet(ctx)
	if err != nil {
		return err
	}
	if set.Len() == 0 {
		return ports.ErrCIMDPublicKeyUnavailable
	}
	return nil
}

// PublicJWKSet returns public ES256 CIMD verification keys only.
func (s *KeyService) PublicJWKSet(ctx context.Context) (jwk.Set, error) {
	keys, err := s.repository.ListActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	if err != nil {
		return nil, fmt.Errorf("list CIMD keys: %w", err)
	}
	set := jwk.NewSet()
	for _, key := range keys {
		if key.Algorithm != "ES256" {
			continue
		}
		privatePEM, err := s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, cimdKeyEncryptionContext(key.KID))
		if err != nil {
			return nil, fmt.Errorf("%w: decrypt CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
		publicKey, err := cimdPublicKeyFromPEM(privatePEM)
		if err != nil {
			return nil, fmt.Errorf("%w: parse CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
		jwkKey, err := jwk.Import[jwk.Key](publicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: import CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := jwkKey.Set(jwk.KeyIDKey, key.KID.String()); err != nil {
			return nil, fmt.Errorf("%w: set CIMD key id", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := jwkKey.Set(jwk.AlgorithmKey, jwa.ES256()); err != nil {
			return nil, fmt.Errorf("%w: set CIMD key algorithm", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := jwkKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
			return nil, fmt.Errorf("%w: set CIMD key use", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := set.AddKey(jwkKey); err != nil {
			return nil, fmt.Errorf("%w: add CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
	}
	if set.Len() == 0 {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}
	return set, nil
}

func (s *KeyService) generateAndStore(ctx context.Context, activatesAt time.Time) (*storage.SigningKey, error) {
	kid := id.NewKeyID(domainencryption.CIMDClientAuthenticationKeyIDPrefix + uuid.New().String())
	privatePEM, err := generateCIMDES256PEM()
	if err != nil {
		s.audit(kid, "generate", "rejected")
		return nil, fmt.Errorf("generate CIMD ES256 key: %w", err)
	}
	subject := domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(kid)
	if err := subject.Validate(); err != nil {
		s.audit(kid, "generate", "rejected")
		return nil, fmt.Errorf("validate CIMD branch key subject: %w", err)
	}
	if _, err := s.branchKeyManager.Create(ctx, subject); err != nil {
		s.audit(kid, "generate", "rejected")
		return nil, fmt.Errorf("provision CIMD branch key: %w", err)
	}
	ciphertext, err := s.encryption.Encrypt(ctx, privatePEM, subject.EncryptionContext())
	if err != nil {
		s.audit(kid, "generate", "rejected")
		return nil, fmt.Errorf("encrypt CIMD private key: %w", err)
	}
	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 kid,
		KeyDomain:           storage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: ciphertext,
		IsCurrent:           true,
		ActivatesAt:         activatesAt,
		CreatedAt:           time.Now().UTC(),
	}
	if err := s.repository.CreateAndSetCurrent(ctx, key); err != nil {
		s.audit(kid, "generate", "rejected")
		return nil, fmt.Errorf("store CIMD key: %w", err)
	}
	s.audit(key.KID, "generate", "success")
	return key, nil
}

func cimdKeyEncryptionContext(kid id.KeyID) map[string]string {
	return domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(kid).EncryptionContext()
}

func generateCIMDES256PEM() ([]byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if encoded == nil {
		return nil, errors.New("encode CIMD private key")
	}
	return encoded, nil
}

func cimdPublicKeyFromPEM(privatePEM []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(privatePEM)
	if block == nil {
		return nil, errors.New("decode CIMD private key")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("CIMD private key is not ECDSA")
	}
	return &ecdsaKey.PublicKey, nil
}

func currentUsableKey(keys []*storage.SigningKey, now time.Time) *storage.SigningKey {
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

func (s *KeyService) audit(kid id.KeyID, operation, outcome string) {
	s.logger.Info("CIMD client-authentication key lifecycle", "key_id", kid, "operation", operation, "outcome", outcome)
}

var _ ports.CIMDClientKeyService = (*KeyService)(nil)
