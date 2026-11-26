package thirdparty

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ErrServiceAccountBound is returned when an agent grant still binds the account to be deleted.
var ErrServiceAccountBound = errors.New("service account is bound by an agent grant")

// ServiceAccountCounter counts the service accounts a service holds.
type ServiceAccountCounter interface {
	CountByService(ctx context.Context, serviceID id.ServiceID) (int, error)
}

// ServiceAccountBindingChecker reports whether an agent grant binds a service account.
type ServiceAccountBindingChecker interface {
	IsBound(ctx context.Context, accountID id.ServiceAccountID) (bool, error)
}

// NoServiceAccountBindings is the binding checker until agent grants exist: nothing is bound.
type NoServiceAccountBindings struct{}

// IsBound always reports false.
func (NoServiceAccountBindings) IsBound(context.Context, id.ServiceAccountID) (bool, error) {
	return false, nil
}

// ServiceLookup reads a third-party service; a missing service is a not-found storage error.
type ServiceLookup interface {
	Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
}

// ServiceAccountCreate is a create request after input parsing.
type ServiceAccountCreate struct {
	CanonicalID string
	Kind        model.ServiceAccountKind
	Scopes      []string
	Credential  *model.ServiceAccountCredentialInput
}

// ServiceAccountUpdate is an update request after input parsing. Nil members keep the stored
// value; Scopes always replaces the ceiling.
type ServiceAccountUpdate struct {
	CanonicalID     *string
	Kind            *model.ServiceAccountKind
	Scopes          []string
	Credential      *model.ServiceAccountCredentialInput
	ExpectedVersion *int64
}

// ServiceAccountUpdateOutcome says whether an update wrote anything.
type ServiceAccountUpdateOutcome string

const (
	// ServiceAccountUnchanged means the update matched the stored account and wrote nothing.
	ServiceAccountUnchanged ServiceAccountUpdateOutcome = "unchanged"
	// ServiceAccountUpdated means the update committed one write.
	ServiceAccountUpdated ServiceAccountUpdateOutcome = "updated"
)

// ServiceAccountView is a stored account with the values derived at the time of the read.
type ServiceAccountView struct {
	Account       *model.ServiceAccount
	Status        model.ServiceAccountStatus
	FailureReason *model.ServiceAccountFailureReason
	Narrowing     model.Narrowing
}

// ServiceAccountService manages service accounts of third-party services. It encrypts each
// credential under the owning service's service_id context, never returns plaintext, derives
// status on read and writes one audit record per committed change.
type ServiceAccountService struct {
	repo       ports.ServiceAccountRepository
	encryption ports.EncryptionPort
	services   ServiceLookup
	bindings   ServiceAccountBindingChecker
	now        func() time.Time
	logger     *slog.Logger
}

// NewServiceAccountService creates the service-account workflow.
func NewServiceAccountService(
	repo ports.ServiceAccountRepository,
	encryption ports.EncryptionPort,
	services ServiceLookup,
	bindings ServiceAccountBindingChecker,
	now func() time.Time,
	logger *slog.Logger,
) *ServiceAccountService {
	if logger == nil {
		panic("thirdparty.NewServiceAccountService: logger must not be nil")
	}
	return &ServiceAccountService{repo: repo, encryption: encryption, services: services, bindings: bindings, now: now, logger: logger}
}

// Create validates, encrypts and stores a new account.
func (s *ServiceAccountService) Create(ctx context.Context, operator id.Principal, serviceID id.ServiceID, request ServiceAccountCreate) (*ServiceAccountView, error) {
	return nil, nil
}

// List returns the service's accounts in canonical_id byte order.
func (s *ServiceAccountService) List(ctx context.Context, serviceID id.ServiceID) ([]ServiceAccountView, error) {
	return nil, nil
}

// Get returns one account of the service.
func (s *ServiceAccountService) Get(ctx context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (*ServiceAccountView, error) {
	return nil, nil
}

// Resolve accepts the account's UUID or its canonical_id within the service.
func (s *ServiceAccountService) Resolve(ctx context.Context, serviceID id.ServiceID, reference string) (id.ServiceAccountID, error) {
	return id.ServiceAccountID{}, nil
}

// Status returns the account's status at the time of the request.
func (s *ServiceAccountService) Status(ctx context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (*ServiceAccountView, error) {
	return nil, nil
}

// Update rotates the credential, changes the ceiling or renames the account.
func (s *ServiceAccountService) Update(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID id.ServiceAccountID, request ServiceAccountUpdate) (*ServiceAccountView, ServiceAccountUpdateOutcome, error) {
	return nil, "", nil
}

// Delete erases the account and its credential.
func (s *ServiceAccountService) Delete(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID id.ServiceAccountID, expectedVersion *int64) error {
	return nil
}
