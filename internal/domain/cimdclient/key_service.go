package cimdclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
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

const cimdKeyGracePeriod = 10 * time.Minute

const cimdPublicJWKCacheTTL = 45 * time.Second
const cimdPublicJWKRebuildTimeout = 30 * time.Second

var errCIMDPublicJWKCacheInvalidated = errors.New("CIMD public JWK cache invalidated during rebuild")

// KeyService manages the dedicated CIMD client-authentication key domain.
type KeyService struct {
	repository ports.SigningKeyRepository
	encryption ports.EncryptionPort
	logger     *slog.Logger
	engine     *keylifecycle.Engine

	publicMu         sync.RWMutex
	publicSet        jwk.Set
	publicExpiresAt  time.Time
	publicVersion    int64
	publicGeneration uint64
	publicFlight     singleflight.Group
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
	s.invalidatePublicJWKCache()
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
	s.invalidatePublicJWKCache()
	s.audit(key.KID, "promote", "success")
	return key, nil
}

// DeleteKey removes a non-current, non-effective CIMD key while preserving domain guards.
func (s *KeyService) DeleteKey(ctx context.Context, kid id.KeyID) error {
	if err := s.engine.Delete(ctx, cimdClientAuthenticationPolicy(), kid, time.Now().UTC()); err != nil {
		s.audit(kid, "remove", "rejected")
		return err
	}
	s.invalidatePublicJWKCache()
	s.audit(kid, "remove", "success")
	return nil
}

// EnsureInitialKey creates one immediately usable CIMD key when the domain is empty.
func (s *KeyService) EnsureInitialKey(ctx context.Context) (*storage.SigningKey, bool, error) {
	key, created, err := s.engine.EnsureInitialKey(ctx, cimdClientAuthenticationPolicy(), time.Now().UTC())
	if err == nil && created {
		s.invalidatePublicJWKCache()
	}
	if err != nil {
		s.audit("", "bootstrap", "rejected")
	}
	return key, created, err
}

// RequireUsablePublishedKey requires an effective CIMD signing key whose kid is advertised.
func (s *KeyService) RequireUsablePublishedKey(ctx context.Context) error {
	if _, err := s.currentUsableCIMDAssertionKey(ctx); err != nil {
		return ports.ErrCIMDPublicKeyUnavailable
	}
	return nil
}

// PublicJWKSet returns public ES256 CIMD verification keys only.
func (s *KeyService) PublicJWKSet(ctx context.Context) (jwk.Set, error) {
	for {
		version, err := s.repository.KeySetVersion(ctx)
		if err != nil {
			return nil, fmt.Errorf("check CIMD key set version: %w", err)
		}
		now := time.Now().UTC()
		s.publicMu.RLock()
		if s.publicSet != nil && s.publicVersion == version && now.Before(s.publicExpiresAt) {
			set := s.publicSet
			s.publicMu.RUnlock()
			return set, nil
		}
		s.publicMu.RUnlock()

		s.publicMu.Lock()
		if s.publicVersion > version {
			s.publicMu.Unlock()
			continue
		}
		if s.publicVersion != version {
			s.publicGeneration++
			s.publicSet = nil
			s.publicVersion = version
		}
		if s.publicSet != nil && time.Now().UTC().Before(s.publicExpiresAt) {
			set := s.publicSet
			s.publicMu.Unlock()
			return set, nil
		}
		generation := s.publicGeneration
		resultCh := s.publicFlight.DoChan(strconv.FormatUint(generation, 10), func() (interface{}, error) {
			rebuildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cimdPublicJWKRebuildTimeout)
			defer cancel()
			return s.rebuildPublicJWKSet(rebuildCtx, generation, version)
		})
		s.publicMu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultCh:
			s.publicMu.RLock()
			currentGeneration := s.publicGeneration
			s.publicMu.RUnlock()
			if currentGeneration != generation || errors.Is(result.Err, errCIMDPublicJWKCacheInvalidated) {
				continue
			}
			if result.Err != nil {
				return nil, result.Err
			}
			set, ok := result.Val.(jwk.Set)
			if !ok {
				return nil, fmt.Errorf("build CIMD public JWK set: unexpected result %T", result.Val)
			}
			return set, nil
		}
	}
}

func (s *KeyService) rebuildPublicJWKSet(ctx context.Context, generation uint64, version int64) (jwk.Set, error) {
	set, err := s.buildPublicJWKSet(ctx)
	if err != nil {
		return nil, err
	}
	currentVersion, err := s.repository.KeySetVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("check CIMD key set version after rebuild: %w", err)
	}
	if currentVersion != version {
		return nil, errCIMDPublicJWKCacheInvalidated
	}
	s.publicMu.Lock()
	defer s.publicMu.Unlock()
	if s.publicGeneration != generation {
		return nil, errCIMDPublicJWKCacheInvalidated
	}
	s.publicSet = set
	s.publicExpiresAt = time.Now().UTC().Add(cimdPublicJWKCacheTTL)
	return set, nil
}

func (s *KeyService) invalidatePublicJWKCache() {
	s.publicMu.Lock()
	defer s.publicMu.Unlock()
	s.publicGeneration++
	s.publicSet = nil
	s.publicExpiresAt = time.Time{}
}

func (s *KeyService) buildPublicJWKSet(ctx context.Context) (jwk.Set, error) {
	keys, err := s.repository.ListActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	if err != nil {
		return nil, fmt.Errorf("list CIMD keys: %w", err)
	}
	set := jwk.NewSet()
	for _, key := range keys {
		if key == nil {
			return nil, fmt.Errorf("%w: nil CIMD key returned from repository", ports.ErrCIMDPublicKeyUnavailable)
		}
		if key.Algorithm != "ES256" {
			continue
		}
		jwkKey, err := s.publicJWKForKey(ctx, key)
		if err != nil {
			return nil, err
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

func (s *KeyService) publicJWKForKey(ctx context.Context, key *storage.SigningKey) (jwk.Key, error) {
	publicJSON := key.PublicJWK
	if len(publicJSON) == 0 {
		privatePEM, err := s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, cimdKeyEncryptionContext(key.KID))
		if err != nil {
			return nil, fmt.Errorf("%w: decrypt legacy CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
		publicKey, err := cimdPublicJWKFromPEM(privatePEM, key.KID)
		if err != nil {
			return nil, fmt.Errorf("%w: parse legacy CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
		publicJSON, err = json.Marshal(publicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: serialize legacy CIMD key", ports.ErrCIMDPublicKeyUnavailable)
		}
		updated, err := s.repository.SetPublicJWK(ctx, key.KID, publicJSON)
		if err != nil {
			return nil, fmt.Errorf("%w: persist legacy CIMD public key", ports.ErrCIMDPublicKeyUnavailable)
		}
		if !updated {
			stored, err := s.repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, key.KID)
			if err != nil || stored == nil || len(stored.PublicJWK) == 0 {
				return nil, fmt.Errorf("%w: retrieve legacy CIMD public key", ports.ErrCIMDPublicKeyUnavailable)
			}
			publicJSON = stored.PublicJWK
		}
	}
	publicKey, err := jwk.ParseKey(publicJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: parse stored CIMD public key", ports.ErrCIMDPublicKeyUnavailable)
	}
	kid, hasKID := publicKey.KeyID()
	algorithm, hasAlgorithm := publicKey.Algorithm()
	usage, hasUsage := publicKey.KeyUsage()
	if !hasKID || kid != key.KID.String() || !hasAlgorithm || algorithm != jwa.ES256() || !hasUsage || usage != "sig" {
		return nil, fmt.Errorf("%w: invalid CIMD public key metadata", ports.ErrCIMDPublicKeyUnavailable)
	}
	publicKey, err = jwk.PublicKeyOf(publicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: derive CIMD public key", ports.ErrCIMDPublicKeyUnavailable)
	}
	ecdsaKey, ok := publicKey.(jwk.ECDSAPublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: CIMD key is not ECDSA", ports.ErrCIMDPublicKeyUnavailable)
	}
	curve, ok := ecdsaKey.Crv()
	if !ok || curve != jwa.P256() {
		return nil, fmt.Errorf("%w: invalid CIMD key curve", ports.ErrCIMDPublicKeyUnavailable)
	}
	return publicKey, nil
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
		PublicJWK: func(privatePEM []byte, kid id.KeyID, _ string) ([]byte, error) {
			publicKey, err := cimdPublicJWKFromPEM(privatePEM, kid)
			if err != nil {
				return nil, err
			}
			return json.Marshal(publicKey)
		},
	}
}

func cimdPublicJWKFromPEM(privatePEM []byte, kid id.KeyID) (jwk.Key, error) {
	privateKey, err := cimdES256PrivateKeyFromPEM(privatePEM)
	if err != nil {
		return nil, err
	}
	key, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	if err := key.Set(jwk.KeyIDKey, kid.String()); err != nil {
		return nil, err
	}
	if err := key.Set(jwk.AlgorithmKey, jwa.ES256()); err != nil {
		return nil, err
	}
	if err := key.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *KeyService) audit(kid id.KeyID, operation, outcome string) {
	s.logger.Info("CIMD client-authentication key lifecycle", "key_id", kid, "operation", operation, "outcome", outcome)
}

var _ ports.CIMDClientKeyService = (*KeyService)(nil)
