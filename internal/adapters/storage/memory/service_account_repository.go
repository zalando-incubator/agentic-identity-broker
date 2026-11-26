package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ServiceAccountRepository = (*ServiceAccountRepository)(nil)

type serviceAccountName struct {
	serviceID   id.ServiceID
	canonicalID string
}

// ServiceAccountRepository stores service accounts in memory with the semantics of the
// PostgreSQL table: unique names per service, version preconditions and a service that must
// exist. Lock order is provider repository, then this repository.
type ServiceAccountRepository struct {
	mu        sync.RWMutex
	accounts  map[id.ServiceAccountID]*model.ServiceAccount
	names     map[serviceAccountName]id.ServiceAccountID
	providers *InMemoryThirdpartyOAuth2ProviderRepository
}

// NewServiceAccountRepository creates an empty in-memory service-account repository.
func NewServiceAccountRepository() *ServiceAccountRepository {
	return &ServiceAccountRepository{
		accounts: make(map[id.ServiceAccountID]*model.ServiceAccount),
		names:    make(map[serviceAccountName]id.ServiceAccountID),
	}
}

// WithProviderRepository supplies the service-existence check that mirrors the foreign key.
func (r *ServiceAccountRepository) WithProviderRepository(providers *InMemoryThirdpartyOAuth2ProviderRepository) *ServiceAccountRepository {
	r.providers = providers
	return r
}

func serviceAccountError(operation string, kind storage.ErrorKind, cause error, message string) error {
	return storage.NewStorageError(operation, kind, cause, message)
}

func serviceAccountNotFound(operation string) error {
	return serviceAccountError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "service account not found")
}

func copyServiceAccount(account *model.ServiceAccount) *model.ServiceAccount {
	copied := *account
	copied.Scopes = slices.Clone(account.Scopes)
	if ciphertext, err := account.Credential.GetCiphertext(); err == nil {
		copied.Credential = model.NewEncryptedSecret(ciphertext)
	}
	copied.Summary.Iss = clonePointer(account.Summary.Iss)
	copied.Summary.Sub = clonePointer(account.Summary.Sub)
	copied.Summary.Kid = clonePointer(account.Summary.Kid)
	copied.Summary.Exp = clonePointer(account.Summary.Exp)
	copied.CredentialExpiresAt = clonePointer(account.CredentialExpiresAt)
	return &copied
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (r *ServiceAccountRepository) Create(_ context.Context, account *model.ServiceAccount) error {
	const operation = "CreateServiceAccount"
	if account == nil || account.ID.IsZero() {
		return serviceAccountError(operation, storage.ErrorKindValidation, nil, "service account ID is required")
	}
	if r.providers == nil {
		return serviceAccountError(operation, storage.ErrorKindUnknown, nil, "service repository is not configured")
	}
	return r.providers.withExistingService(account.ServiceID, func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		if _, exists := r.accounts[account.ID]; exists {
			return serviceAccountError(operation, storage.ErrorKindConflict, nil, "service account already exists")
		}
		name := serviceAccountName{account.ServiceID, account.CanonicalID}
		if _, taken := r.names[name]; taken {
			return serviceAccountError(operation, storage.ErrorKindConflict, ports.ErrServiceAccountCanonicalIDConflict, "service account canonical_id already exists on this service")
		}
		account.Version = 1
		r.accounts[account.ID] = copyServiceAccount(account)
		r.names[name] = account.ID
		return nil
	}, operation)
}

func (r *ServiceAccountRepository) Get(_ context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (*model.ServiceAccount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	account, ok := r.accounts[accountID]
	if !ok || account.ServiceID != serviceID {
		return nil, serviceAccountNotFound("GetServiceAccount")
	}
	return copyServiceAccount(account), nil
}

func (r *ServiceAccountRepository) GetByCanonicalID(_ context.Context, serviceID id.ServiceID, canonicalID string) (*model.ServiceAccount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	accountID, ok := r.names[serviceAccountName{serviceID, canonicalID}]
	if !ok {
		return nil, serviceAccountNotFound("GetServiceAccountByCanonicalID")
	}
	return copyServiceAccount(r.accounts[accountID]), nil
}

func (r *ServiceAccountRepository) ListByService(_ context.Context, serviceID id.ServiceID) ([]*model.ServiceAccount, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	accounts := []*model.ServiceAccount{}
	for _, account := range r.accounts {
		if account.ServiceID == serviceID {
			accounts = append(accounts, copyServiceAccount(account))
		}
	}
	slices.SortFunc(accounts, func(a, b *model.ServiceAccount) int {
		switch {
		case a.CanonicalID < b.CanonicalID:
			return -1
		case a.CanonicalID > b.CanonicalID:
			return 1
		}
		return 0
	})
	return accounts, nil
}

// lockedAccount returns the stored account of the service after the version check. The caller
// holds the write lock.
func (r *ServiceAccountRepository) lockedAccount(operation string, serviceID id.ServiceID, accountID id.ServiceAccountID, expectedVersion *int64) (*model.ServiceAccount, error) {
	stored, ok := r.accounts[accountID]
	if !ok || stored.ServiceID != serviceID {
		return nil, serviceAccountNotFound(operation)
	}
	if expectedVersion != nil && *expectedVersion != stored.Version {
		return nil, serviceAccountError(operation, storage.ErrorKindConflict, ports.ErrServiceAccountVersionMismatch, "service account version mismatch")
	}
	return stored, nil
}

func (r *ServiceAccountRepository) Update(_ context.Context, account *model.ServiceAccount, expectedVersion *int64) error {
	const operation = "UpdateServiceAccount"
	if account == nil {
		return serviceAccountError(operation, storage.ErrorKindValidation, nil, "service account is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, err := r.lockedAccount(operation, account.ServiceID, account.ID, expectedVersion)
	if err != nil {
		return err
	}
	oldName := serviceAccountName{stored.ServiceID, stored.CanonicalID}
	newName := serviceAccountName{stored.ServiceID, account.CanonicalID}
	if holder, taken := r.names[newName]; taken && holder != account.ID {
		return serviceAccountError(operation, storage.ErrorKindConflict, ports.ErrServiceAccountCanonicalIDConflict, "service account canonical_id already exists on this service")
	}
	updated := copyServiceAccount(account)
	updated.Kind, updated.CreatedAt = stored.Kind, stored.CreatedAt
	updated.Version = stored.Version + 1
	if updated.UpdatedAt.IsZero() {
		updated.UpdatedAt = time.Now().UTC()
	}
	delete(r.names, oldName)
	r.names[newName] = account.ID
	r.accounts[account.ID] = updated
	account.Version = updated.Version
	return nil
}

func (r *ServiceAccountRepository) Delete(_ context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID, expectedVersion *int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, err := r.lockedAccount("DeleteServiceAccount", serviceID, accountID, expectedVersion)
	if err != nil {
		return err
	}
	delete(r.names, serviceAccountName{stored.ServiceID, stored.CanonicalID})
	delete(r.accounts, accountID)
	return nil
}

func (r *ServiceAccountRepository) CountByService(_ context.Context, serviceID id.ServiceID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, account := range r.accounts {
		if account.ServiceID == serviceID {
			count++
		}
	}
	return count, nil
}
