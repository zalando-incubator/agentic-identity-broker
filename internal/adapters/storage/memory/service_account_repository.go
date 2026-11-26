package memory

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ServiceAccountRepository = (*ServiceAccountRepository)(nil)

// ServiceAccountRepository stores service accounts in memory.
type ServiceAccountRepository struct{}

// NewServiceAccountRepository creates an empty in-memory service-account repository.
func NewServiceAccountRepository() *ServiceAccountRepository {
	return &ServiceAccountRepository{}
}

func serviceAccountNotFound(operation string) error {
	return storage.NewStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "service account not found")
}

func (r *ServiceAccountRepository) Create(context.Context, *model.ServiceAccount) error {
	return serviceAccountNotFound("CreateServiceAccount")
}

func (r *ServiceAccountRepository) Get(context.Context, id.ServiceID, id.ServiceAccountID) (*model.ServiceAccount, error) {
	return nil, serviceAccountNotFound("GetServiceAccount")
}

func (r *ServiceAccountRepository) GetByCanonicalID(context.Context, id.ServiceID, string) (*model.ServiceAccount, error) {
	return nil, serviceAccountNotFound("GetServiceAccountByCanonicalID")
}

func (r *ServiceAccountRepository) ListByService(context.Context, id.ServiceID) ([]*model.ServiceAccount, error) {
	return []*model.ServiceAccount{}, nil
}

func (r *ServiceAccountRepository) Update(context.Context, *model.ServiceAccount, *int64) error {
	return serviceAccountNotFound("UpdateServiceAccount")
}

func (r *ServiceAccountRepository) Delete(context.Context, id.ServiceID, id.ServiceAccountID, *int64) error {
	return serviceAccountNotFound("DeleteServiceAccount")
}

func (r *ServiceAccountRepository) CountByService(context.Context, id.ServiceID) (int, error) {
	return 0, nil
}
