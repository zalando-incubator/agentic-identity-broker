package admin

import (
	"context"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type handlerCoverage []id.ServiceID

func (services handlerCoverage) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	ps := &storage.PermissionSet{}
	for _, serviceID := range services {
		ps.ServiceScopes = append(ps.ServiceScopes, storage.ServiceScope{ServiceID: serviceID})
	}
	return []*storage.PermissionSet{ps}, nil
}
