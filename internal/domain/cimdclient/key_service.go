package cimdclient

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/keylifecycle"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const cimdKeyGracePeriod = 10 * time.Minute

// KeyService manages the dedicated CIMD client-authentication key domain.
type KeyService struct {
	repository ports.SigningKeyRepository
	encryption ports.EncryptionPort
	logger     *slog.Logger
	engine     *keylifecycle.Engine
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
		repository: repository,
		encryption: encryption,
		logger:     logger,
		engine:     keylifecycle.NewEngine(repository, bootstrapCoordinator, encryption, branchKeyManager, logger),
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
	now := time.Now().UTC()
	key, err := s.engine.GenerateWithInitialActivation(ctx, cimdClientAuthenticationPolicy(), now, now.Add(cimdKeyGracePeriod))
	if err != nil {
		s.audit("", "generate", "rejected")
		return nil, fmt.Errorf("generate CIMD key: %w", err)
	}
	s.audit(key.KID, "generate", "success")
	return key, nil
}

// ListKeys returns active CIMD client-authentication key metadata.
func (s *KeyService) ListKeys(ctx context.Context) ([]*storage.SigningKey, error) {
	return s.engine.List(ctx, cimdClientAuthenticationPolicy())
}

// PromoteKey makes an existing CIMD key immediately current.
func (s *KeyService) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	key, err := s.engine.Promote(ctx, cimdClientAuthenticationPolicy(), kid, time.Now().UTC())
	if err != nil {
		s.audit(kid, "promote", "rejected")
		return nil, err
	}
	s.audit(key.KID, "promote", "success")
	return key, nil
}

// DeleteKey removes a non-current, non-effective CIMD key while preserving domain guards.
func (s *KeyService) DeleteKey(ctx context.Context, kid id.KeyID) error {
	if err := s.engine.Delete(ctx, cimdClientAuthenticationPolicy(), kid, time.Now().UTC()); err != nil {
		s.audit(kid, "remove", "rejected")
		return err
	}
	s.audit(kid, "remove", "success")
	return nil
}

// EnsureInitialKey creates one immediately usable CIMD key when the domain is empty.
func (s *KeyService) EnsureInitialKey(ctx context.Context) (*storage.SigningKey, bool, error) {
	key, created, err := s.engine.EnsureInitialKey(ctx, cimdClientAuthenticationPolicy(), time.Now().UTC())
	if err != nil {
		s.audit("", "bootstrap", "rejected")
	}
	return key, created, err
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

func cimdKeyEncryptionContext(kid id.KeyID) map[string]string {
	return domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(kid).EncryptionContext()
}

func cimdClientAuthenticationPolicy() keylifecycle.Policy {
	return keylifecycle.Policy{
		Domain: storage.KeyDomainCIMDClientAuthentication,
		NewKID: func() id.KeyID { return keylifecycle.UUIDKID(domainencryption.CIMDClientAuthenticationKeyIDPrefix) },
		NewSubject: func(kid id.KeyID) (domainencryption.BranchKeySubject, error) {
			return domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(kid), nil
		},
	}
}

func cimdPublicKeyFromPEM(privatePEM []byte) (*ecdsa.PublicKey, error) {
	publicKey, err := keylifecycle.PublicKeyFromPEM(privatePEM, "ES256")
	if err != nil {
		return nil, err
	}
	ecdsaKey, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("CIMD private key is not ECDSA")
	}
	return ecdsaKey, nil
}

func currentUsableKey(keys []*storage.SigningKey, now time.Time) *storage.SigningKey {
	return keylifecycle.EffectiveCurrent(keys, now)
}

func (s *KeyService) audit(kid id.KeyID, operation, outcome string) {
	s.logger.Info("CIMD client-authentication key lifecycle", "key_id", kid, "operation", operation, "outcome", outcome)
}

var _ ports.CIMDClientKeyService = (*KeyService)(nil)
