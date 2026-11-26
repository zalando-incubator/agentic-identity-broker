package postgres

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ServiceAccountRepository = (*ServiceAccountRepository)(nil)

// ServiceAccountRepository stores service accounts in PostgreSQL.
type ServiceAccountRepository struct {
	adapter *Adapter
}

// NewServiceAccountRepository creates a PostgreSQL service-account repository.
func NewServiceAccountRepository(adapter *Adapter) *ServiceAccountRepository {
	return &ServiceAccountRepository{adapter: adapter}
}

func serviceAccountNotImplemented(operation string) error {
	return storage.NewStorageError(operation, storage.ErrorKindUnknown, nil, "not implemented")
}

func (r *ServiceAccountRepository) Create(context.Context, *model.ServiceAccount) error {
	return serviceAccountNotImplemented("ServiceAccountRepository.Create")
}

func (r *ServiceAccountRepository) Get(context.Context, id.ServiceID, id.ServiceAccountID) (*model.ServiceAccount, error) {
	return nil, serviceAccountNotImplemented("ServiceAccountRepository.Get")
}

func (r *ServiceAccountRepository) GetByCanonicalID(context.Context, id.ServiceID, string) (*model.ServiceAccount, error) {
	return nil, serviceAccountNotImplemented("ServiceAccountRepository.GetByCanonicalID")
}

func (r *ServiceAccountRepository) ListByService(context.Context, id.ServiceID) ([]*model.ServiceAccount, error) {
	return nil, serviceAccountNotImplemented("ServiceAccountRepository.ListByService")
}

func (r *ServiceAccountRepository) Update(context.Context, *model.ServiceAccount, *int64) error {
	return serviceAccountNotImplemented("ServiceAccountRepository.Update")
}

func (r *ServiceAccountRepository) Delete(context.Context, id.ServiceID, id.ServiceAccountID, *int64) error {
	return serviceAccountNotImplemented("ServiceAccountRepository.Delete")
}

func (r *ServiceAccountRepository) CountByService(context.Context, id.ServiceID) (int, error) {
	return 0, serviceAccountNotImplemented("ServiceAccountRepository.CountByService")
}
