package keylifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveCurrent(t *testing.T) {
	now := time.Now().UTC()
	previous := &storage.SigningKey{KID: id.NewKeyID("previous"), ActivatesAt: now.Add(-time.Minute)}
	pending := &storage.SigningKey{KID: id.NewKeyID("pending"), IsCurrent: true, ActivatesAt: now.Add(time.Minute)}
	active := &storage.SigningKey{KID: id.NewKeyID("active"), IsCurrent: true, ActivatesAt: now.Add(-time.Second)}

	assert.Equal(t, active, EffectiveCurrent([]*storage.SigningKey{previous, pending, active}, now))
	assert.Equal(t, previous, EffectiveCurrent([]*storage.SigningKey{previous, pending}, now))
	assert.Nil(t, EffectiveCurrent([]*storage.SigningKey{pending}, now))
}

func TestEngine_GenerateWithInitialActivationSerializesConcurrentFirstGeneration(t *testing.T) {
	repository := &lifecycleTestRepository{}
	engine := NewEngine(repository, &lifecycleTestCoordinator{}, lifecycleTestEncryption{}, lifecycleTestBranchKeys{}, nil)
	policy := Policy{
		Domain: storage.KeyDomainCIMDClientAuthentication,
		NewKID: func() id.KeyID { return UUIDKID(domainencryption.CIMDClientAuthenticationKeyIDPrefix) },
		NewSubject: func(kid id.KeyID) (domainencryption.BranchKeySubject, error) {
			return domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(kid), nil
		},
	}
	firstActivation := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	laterActivation := firstActivation.Add(10 * time.Minute)

	start := make(chan struct{})
	errs := make(chan error, 2)
	keys := make(chan *storage.SigningKey, 2)
	for range 2 {
		go func() {
			<-start
			key, err := engine.GenerateWithInitialActivation(context.Background(), policy, firstActivation, laterActivation)
			keys <- key
			errs <- err
		}()
	}
	close(start)
	for range 2 {
		require.NoError(t, <-errs)
		require.NotNil(t, <-keys)
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	assert.Equal(t, []int{0, 1}, repository.counts)
	require.Len(t, repository.keys, 2)
	activations := map[time.Time]struct{}{}
	for _, key := range repository.keys {
		activations[key.ActivatesAt] = struct{}{}
	}
	assert.Contains(t, activations, firstActivation)
	assert.Contains(t, activations, laterActivation)
}

type lifecycleTestCoordinator struct{ mu sync.Mutex }

func (c *lifecycleTestCoordinator) WithBootstrapLock(_ context.Context, fn func(context.Context) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fn(context.Background())
}

type lifecycleTestRepository struct {
	mu     sync.Mutex
	keys   []*storage.SigningKey
	counts []int
}

func (r *lifecycleTestRepository) Create(context.Context, *storage.SigningKey) error { return nil }

func (r *lifecycleTestRepository) CreateAndSetCurrent(_ context.Context, key *storage.SigningKey) error {
	r.keys = append(r.keys, key)
	return nil
}

func (r *lifecycleTestRepository) GetByKIDInDomain(context.Context, storage.KeyDomain, id.KeyID) (*storage.SigningKey, error) {
	return nil, ports.ErrNotFound
}

func (r *lifecycleTestRepository) GetCurrentInDomain(context.Context, storage.KeyDomain) (*storage.SigningKey, error) {
	return nil, ports.ErrNotFound
}

func (r *lifecycleTestRepository) ListActiveInDomain(context.Context, storage.KeyDomain) ([]*storage.SigningKey, error) {
	return nil, nil
}

func (r *lifecycleTestRepository) SetCurrentInDomain(context.Context, storage.KeyDomain, id.KeyID, time.Time) (*storage.SigningKey, error) {
	return nil, ports.ErrNotFound
}

func (r *lifecycleTestRepository) DeleteInDomain(context.Context, storage.KeyDomain, id.KeyID) error {
	return ports.ErrNotFound
}

func (r *lifecycleTestRepository) CountActiveInDomain(_ context.Context, _ storage.KeyDomain) (int, error) {
	count := len(r.keys)
	r.counts = append(r.counts, count)
	return count, nil
}

type lifecycleTestEncryption struct{}

func (lifecycleTestEncryption) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (lifecycleTestEncryption) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

type lifecycleTestBranchKeys struct{}

func (lifecycleTestBranchKeys) Create(context.Context, domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

var _ ports.SigningKeyRepository = (*lifecycleTestRepository)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*lifecycleTestCoordinator)(nil)
var _ ports.EncryptionPort = lifecycleTestEncryption{}
var _ ports.BranchKeyManager = lifecycleTestBranchKeys{}
