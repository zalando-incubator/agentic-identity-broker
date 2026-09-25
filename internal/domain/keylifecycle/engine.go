package keylifecycle

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

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const bootstrapRecoveryProbeTimeout = 5 * time.Second

// Policy fixes lifecycle behavior to one trust domain.
type Policy struct {
	Domain     storage.KeyDomain
	NewKID     func() id.KeyID
	NewSubject func(id.KeyID) (encryption.BranchKeySubject, error)
}

// Engine shares encrypted ES256 lifecycle mechanics without sharing a trust domain.
type Engine struct {
	repository           ports.SigningKeyRepository
	bootstrapCoordinator ports.SigningKeyBootstrapCoordinator
	encryption           ports.EncryptionPort
	branchKeyManager     ports.BranchKeyManager
	logger               *slog.Logger
}

func NewEngine(repository ports.SigningKeyRepository, bootstrapCoordinator ports.SigningKeyBootstrapCoordinator, encryptor ports.EncryptionPort, branchKeyManager ports.BranchKeyManager, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{repository: repository, bootstrapCoordinator: bootstrapCoordinator, encryption: encryptor, branchKeyManager: branchKeyManager, logger: logger}
}

// GenerateAndStore encrypts and persists one ES256 key under a fixed policy.
func (e *Engine) GenerateAndStore(ctx context.Context, policy Policy, algorithm string, isCurrent bool, activatesAt time.Time) (*storage.SigningKey, error) {
	if err := policy.Domain.Validate(); err != nil {
		return nil, err
	}
	if policy.NewKID == nil || policy.NewSubject == nil {
		return nil, errors.New("key lifecycle policy is incomplete")
	}
	if algorithm == "" {
		algorithm = "ES256"
	}
	if algorithm != "ES256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
	kid := policy.NewKID()
	if kid.IsZero() {
		return nil, errors.New("key lifecycle policy generated an empty kid")
	}
	privatePEM, err := GenerateES256PEM()
	if err != nil {
		return nil, fmt.Errorf("generate ES256 key: %w", err)
	}
	subject, err := policy.NewSubject(kid)
	if err != nil {
		return nil, fmt.Errorf("create branch key subject: %w", err)
	}
	if err := subject.Validate(); err != nil {
		return nil, fmt.Errorf("validate branch key subject: %w", err)
	}
	branchKeyID, err := e.branchKeyManager.Create(ctx, subject)
	if err != nil {
		return nil, fmt.Errorf("provision branch key: %w", err)
	}
	ciphertext, err := e.encryption.Encrypt(ctx, privatePEM, subject.EncryptionContext())
	if err != nil {
		e.warnOrphanedBranchKey("orphaned branch key after encryption failure; manual cleanup required", kid, branchKeyID)
		return nil, fmt.Errorf("encrypt private key: %w", err)
	}
	key := &storage.SigningKey{ID: id.NewSigningKeyID(), KID: kid, KeyDomain: policy.Domain, Algorithm: algorithm, PrivateKeyEncrypted: ciphertext, IsCurrent: isCurrent, ActivatesAt: activatesAt, CreatedAt: time.Now().UTC()}
	if isCurrent {
		err = e.repository.CreateAndSetCurrent(ctx, key)
	} else {
		err = e.repository.Create(ctx, key)
	}
	if err != nil {
		e.warnOrphanedBranchKey("orphaned branch key after storage failure; manual cleanup required", kid, branchKeyID)
		return nil, fmt.Errorf("store signing key: %w", err)
	}
	return key, nil
}

// EnsureInitialKey creates one key under the bootstrap lock when its domain is empty.
func (e *Engine) EnsureInitialKey(ctx context.Context, policy Policy, activatesAt time.Time) (*storage.SigningKey, bool, error) {
	var created *storage.SigningKey
	err := e.bootstrapCoordinator.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
		count, err := e.repository.CountActiveInDomain(lockCtx, policy.Domain)
		if err != nil {
			return fmt.Errorf("count active keys: %w", err)
		}
		if count > 0 {
			return nil
		}
		created, err = e.GenerateAndStore(lockCtx, policy, "ES256", true, activatesAt)
		return err
	})
	if err != nil {
		if created != nil || isTimeoutOrCancellation(err) {
			recoveryCtx, cancel := context.WithTimeout(context.Background(), bootstrapRecoveryProbeTimeout)
			defer cancel()
			if count, probeErr := e.repository.CountActiveInDomain(recoveryCtx, policy.Domain); probeErr == nil && count > 0 {
				e.logger.Warn("bootstrap lock returned error after key bootstrap work; treating startup as recovered", "key_domain", policy.Domain, "active_key_count", count, "created_locally", created != nil, "error", err)
				return created, created != nil, nil
			} else if probeErr != nil {
				e.logger.Warn("bootstrap recovery count probe failed", "key_domain", policy.Domain, "created_locally", created != nil, "error", err, "recovery_error", probeErr)
			}
		}
		return nil, false, err
	}
	return created, created != nil, nil
}

// GenerateWithInitialActivation serializes count-and-create so concurrent first-key
// requests cannot create two immediately usable keys.
func (e *Engine) GenerateWithInitialActivation(ctx context.Context, policy Policy, firstActivatesAt, laterActivatesAt time.Time) (*storage.SigningKey, error) {
	var key *storage.SigningKey
	err := e.bootstrapCoordinator.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
		count, err := e.repository.CountActiveInDomain(lockCtx, policy.Domain)
		if err != nil {
			return fmt.Errorf("count active keys: %w", err)
		}
		activatesAt := firstActivatesAt
		if count > 0 {
			activatesAt = laterActivatesAt
		}
		key, err = e.GenerateAndStore(lockCtx, policy, "ES256", true, activatesAt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return key, nil
}

// List returns active keys within the fixed policy domain.
func (e *Engine) List(ctx context.Context, policy Policy) ([]*storage.SigningKey, error) {
	if err := policy.Domain.Validate(); err != nil {
		return nil, err
	}
	return e.repository.ListActiveInDomain(ctx, policy.Domain)
}

// Promote immediately selects an existing key within the fixed policy domain.
func (e *Engine) Promote(ctx context.Context, policy Policy, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	if err := policy.Domain.Validate(); err != nil {
		return nil, err
	}
	return e.repository.SetCurrentInDomain(ctx, policy.Domain, kid, activatesAt)
}

// Delete preserves the last, current, and effective-current guards for one domain.
func (e *Engine) Delete(ctx context.Context, policy Policy, kid id.KeyID, now time.Time) error {
	if err := policy.Domain.Validate(); err != nil {
		return err
	}
	key, err := e.repository.GetByKIDInDomain(ctx, policy.Domain, kid)
	if err != nil {
		return fmt.Errorf("failed to get signing key: %w", err)
	}
	count, err := e.repository.CountActiveInDomain(ctx, policy.Domain)
	if err != nil {
		return fmt.Errorf("count active keys: %w", err)
	}
	if count <= 1 {
		return ports.ErrLastActiveKey
	}
	if key.IsCurrent {
		return ports.ErrCurrentKey
	}
	keys, err := e.repository.ListActiveInDomain(ctx, policy.Domain)
	if err != nil {
		return fmt.Errorf("failed to list active keys: %w", err)
	}
	if current := EffectiveCurrent(keys, now); current != nil && current.KID == kid {
		return ports.ErrEffectiveCurrentKey
	}
	return e.repository.DeleteInDomain(ctx, policy.Domain, kid)
}

func (e *Engine) warnOrphanedBranchKey(message string, kid id.KeyID, branchKeyID string) {
	if branchKeyID == "" {
		e.logger.Debug("skipping orphan warning: branch key ID is empty (noop backend or unexpected empty return)", "kid", kid)
		return
	}
	e.logger.Warn(message, "kid", kid, "branch_key_id", branchKeyID)
}

// EffectiveCurrent returns the eligible key with the current marker preferred.
func EffectiveCurrent(keys []*storage.SigningKey, now time.Time) *storage.SigningKey {
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

func GenerateES256PEM() ([]byte, error) {
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
		return nil, errors.New("encode private key")
	}
	return encoded, nil
}

func PublicKeyFromPEM(privatePEM []byte, algorithm string) (any, error) {
	block, _ := pem.Decode(privatePEM)
	if block == nil {
		return nil, errors.New("decode PEM block")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	if algorithm != "ES256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
	ecdsaKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected ECDSA private key, got %T", privateKey)
	}
	return &ecdsaKey.PublicKey, nil
}

func isTimeoutOrCancellation(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var storageErr *storage.StorageError
	return errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindTimeout
}

// UUIDKID returns a random KID with the given optional namespace prefix.
func UUIDKID(prefix string) id.KeyID { return id.NewKeyID(prefix + uuid.New().String()) }
