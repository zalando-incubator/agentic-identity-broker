package thirdparty

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
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
	Metadata    model.ObjectMetadata
}

// ServiceAccountUpdate is an update request after input parsing. Nil members keep the stored
// value; Scopes always replaces the ceiling, and Metadata replaces both maps.
type ServiceAccountUpdate struct {
	CanonicalID     *string
	Kind            *model.ServiceAccountKind
	Scopes          []string
	Credential      *model.ServiceAccountCredentialInput
	Metadata        *model.ObjectMetadata
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

// Create validates, encrypts and stores a new account. The service must exist; a refused request
// stores nothing.
func (s *ServiceAccountService) Create(ctx context.Context, operator id.Principal, serviceID id.ServiceID, request ServiceAccountCreate) (*ServiceAccountView, error) {
	if _, err := s.services.Get(ctx, serviceID); err != nil {
		return nil, err
	}
	if err := model.ValidateServiceAccountKind(request.Kind); err != nil {
		return nil, err
	}
	if err := model.ValidateServiceAccountCanonicalID(request.CanonicalID); err != nil {
		return nil, err
	}
	scopes, err := model.NormalizeServiceAccountScopes(request.Scopes)
	if err != nil {
		return nil, err
	}
	if err := request.Metadata.Validate(); err != nil {
		return nil, err
	}
	if request.Credential == nil {
		return nil, &model.ServiceAccountValidationError{Message: "credential is required"}
	}
	now := s.now().UTC()
	validated, err := s.validateCredential(ctx, operator, serviceID, nil, operationCreate, *request.Credential, now)
	if err != nil {
		return nil, err
	}
	account := &model.ServiceAccount{
		ID: id.NewServiceAccountID(), ServiceID: serviceID, CanonicalID: request.CanonicalID, Kind: request.Kind, Scopes: scopes,
		Summary: validated.Summary, CredentialExpiresAt: validated.ExpiresAt, ObjectMetadata: request.Metadata,
		CreatedAt: now, UpdatedAt: now,
	}
	if account.Credential, err = s.encrypt(ctx, operator, account, operationCreate, validated.Document); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, account); err != nil {
		s.logWriteFailure(ctx, operator, serviceID, nil, operationCreate, err)
		return nil, err
	}
	s.logger.InfoContext(ctx, "service account created", s.auditArgs("service.service_account.created", operator, account)...)
	return s.view(account, s.now().UTC()), nil
}

// List returns the service's accounts in canonical_id byte order.
func (s *ServiceAccountService) List(ctx context.Context, serviceID id.ServiceID) ([]ServiceAccountView, error) {
	if _, err := s.services.Get(ctx, serviceID); err != nil {
		return nil, err
	}
	accounts, err := s.repo.ListByService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	views := make([]ServiceAccountView, len(accounts))
	for i, account := range accounts {
		views[i] = *s.view(account, now)
	}
	return views, nil
}

// Get returns one account of the service.
func (s *ServiceAccountService) Get(ctx context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (*ServiceAccountView, error) {
	account, err := s.repo.Get(ctx, serviceID, accountID)
	if err != nil {
		return nil, err
	}
	return s.view(account, s.now().UTC()), nil
}

// Resolve accepts the account's UUID or its canonical_id within the service. Canonical IDs are
// never UUID-shaped, so the two cannot collide.
func (s *ServiceAccountService) Resolve(ctx context.Context, serviceID id.ServiceID, reference string) (id.ServiceAccountID, error) {
	if accountID, err := id.ParseServiceAccountID(reference); err == nil {
		return accountID, nil
	}
	account, err := s.repo.GetByCanonicalID(ctx, serviceID, reference)
	if err != nil {
		return id.ServiceAccountID{}, err
	}
	return account.ID, nil
}

// Status returns the account's status at the time of the request.
func (s *ServiceAccountService) Status(ctx context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (*ServiceAccountView, error) {
	return s.Get(ctx, serviceID, accountID)
}

// Update rotates the credential, changes the ceiling or renames the account. Request errors win
// over a stale version; an update that changes nothing writes nothing.
func (s *ServiceAccountService) Update(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID id.ServiceAccountID, request ServiceAccountUpdate) (*ServiceAccountView, ServiceAccountUpdateOutcome, error) {
	stored, err := s.repo.Get(ctx, serviceID, accountID)
	if err != nil {
		s.logWriteFailure(ctx, operator, serviceID, &accountID, operationUpdate, err)
		return nil, "", err
	}
	if request.Kind != nil {
		if err := model.ValidateServiceAccountKind(*request.Kind); err != nil {
			return nil, "", err
		}
		if *request.Kind != stored.Kind {
			return nil, "", &model.ServiceAccountValidationError{Message: "kind cannot be changed"}
		}
	}
	if request.CanonicalID != nil {
		if err := model.ValidateServiceAccountCanonicalID(*request.CanonicalID); err != nil {
			return nil, "", err
		}
	}
	scopes, err := model.NormalizeServiceAccountScopes(request.Scopes)
	if err != nil {
		return nil, "", err
	}
	if request.Metadata != nil {
		if err := request.Metadata.Validate(); err != nil {
			return nil, "", err
		}
	}
	now := s.now().UTC()
	var validated *model.ValidatedCredential
	if request.Credential != nil {
		if validated, err = s.validateCredential(ctx, operator, serviceID, &accountID, operationUpdate, *request.Credential, now); err != nil {
			return nil, "", err
		}
	}
	for attempt := 1; ; attempt++ {
		view, outcome, err := s.applyUpdate(ctx, operator, stored, request, scopes, validated, s.now().UTC())
		if request.ExpectedVersion != nil || attempt == unconditionalWriteAttempts || !errors.Is(err, ports.ErrServiceAccountVersionMismatch) {
			return view, outcome, err
		}
		if stored, err = s.repo.Get(ctx, serviceID, accountID); err != nil {
			s.logWriteFailure(ctx, operator, serviceID, &accountID, operationUpdate, err)
			return nil, "", err
		}
	}
}

// unconditionalWriteAttempts bounds how often an update or delete without If-Match re-reads an
// account that another writer changed between its read and its write.
const unconditionalWriteAttempts = 3

// applyUpdate writes the requested state over stored. The write is always conditional on the
// version that was read, so a concurrent change is never overwritten with stale members; without
// If-Match the caller re-reads and applies the request again.
func (s *ServiceAccountService) applyUpdate(ctx context.Context, operator id.Principal, stored *model.ServiceAccount, request ServiceAccountUpdate, scopes []string, validated *model.ValidatedCredential, now time.Time) (*ServiceAccountView, ServiceAccountUpdateOutcome, error) {
	if request.ExpectedVersion != nil && *request.ExpectedVersion != stored.Version {
		return nil, "", storage.NewStorageError("UpdateServiceAccount", storage.ErrorKindConflict, ports.ErrServiceAccountVersionMismatch, "service account version mismatch")
	}
	candidate := *stored
	candidate.Scopes = scopes
	if request.CanonicalID != nil {
		candidate.CanonicalID = *request.CanonicalID
	}
	if request.Metadata != nil {
		candidate.ObjectMetadata = *request.Metadata
	}

	changes := []string{}
	rotated := validated != nil && (validated.Summary.Fingerprint != stored.Summary.Fingerprint ||
		validated.Summary.Format != stored.Summary.Format || validated.Summary.Use != stored.Summary.Use)
	if rotated {
		changes = append(changes, changeCredential)
	}
	if !slices.Equal(candidate.Scopes, stored.Scopes) {
		changes = append(changes, changeScopes)
	}
	if candidate.CanonicalID != stored.CanonicalID {
		changes = append(changes, changeCanonicalID)
	}
	if !candidate.ObjectMetadata.Equal(stored.ObjectMetadata) {
		changes = append(changes, changeMetadata)
	}
	if len(changes) == 0 {
		return s.view(stored, s.now().UTC()), ServiceAccountUnchanged, nil
	}

	var err error
	if rotated {
		candidate.Summary, candidate.CredentialExpiresAt = validated.Summary, validated.ExpiresAt
		if candidate.Credential, err = s.encrypt(ctx, operator, &candidate, operationUpdate, validated.Document); err != nil {
			return nil, "", err
		}
	}
	candidate.UpdatedAt = now
	readVersion := stored.Version
	if err := s.repo.Update(ctx, &candidate, &readVersion); err != nil {
		s.logWriteFailure(ctx, operator, stored.ServiceID, &stored.ID, operationUpdate, err)
		return nil, "", err
	}

	args := s.auditArgs(updateEvent(changes), operator, &candidate)
	args = append(args, "changes", changes)
	if slices.Contains(changes, changeCanonicalID) {
		args = append(args, "previous_observed_canonical_id", stored.CanonicalID)
	}
	if slices.Contains(changes, changeScopes) {
		args = append(args, "previous_observed_scopes", stored.Scopes)
	}
	if rotated {
		args = append(args, "previous_observed_credential", stored.Summary)
	}
	s.logger.InfoContext(ctx, "service account updated", args...)
	return s.view(&candidate, s.now().UTC()), ServiceAccountUpdated, nil
}

// Delete erases the account and its credential. It is refused while an agent grant binds the
// account.
func (s *ServiceAccountService) Delete(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID id.ServiceAccountID, expectedVersion *int64) error {
	for attempt := 1; ; attempt++ {
		stored, err := s.repo.Get(ctx, serviceID, accountID)
		if err != nil {
			s.logWriteFailure(ctx, operator, serviceID, &accountID, operationDelete, err)
			return err
		}
		if expectedVersion != nil && *expectedVersion != stored.Version {
			return storage.NewStorageError("DeleteServiceAccount", storage.ErrorKindConflict, ports.ErrServiceAccountVersionMismatch, "service account version mismatch")
		}
		bound, err := s.bindings.IsBound(ctx, accountID)
		if err != nil {
			return fmt.Errorf("check service account bindings: %w", err)
		}
		if bound {
			return storage.NewStorageError("DeleteServiceAccount", storage.ErrorKindConflict, ErrServiceAccountBound, ErrServiceAccountBound.Error())
		}
		readVersion := stored.Version
		err = s.repo.Delete(ctx, serviceID, accountID, &readVersion)
		if err == nil {
			s.logger.InfoContext(ctx, "service account deleted", s.auditArgs("service.service_account.deleted", operator, stored)...)
			return nil
		}
		if expectedVersion == nil && attempt < unconditionalWriteAttempts && errors.Is(err, ports.ErrServiceAccountVersionMismatch) {
			continue
		}
		s.logWriteFailure(ctx, operator, serviceID, &accountID, operationDelete, err)
		return err
	}
}

const (
	operationCreate = "create"
	operationUpdate = "update"
	operationDelete = "delete"

	changeCredential  = "credential"
	changeScopes      = "scopes"
	changeCanonicalID = "canonical_id"
	changeMetadata    = "metadata"
)

// updateEvent picks the event of an update by precedence; changes lists every changed aspect.
func updateEvent(changes []string) string {
	switch {
	case slices.Contains(changes, changeCredential):
		return "service.service_account.credential_rotated"
	case slices.Contains(changes, changeScopes):
		return "service.service_account.ceiling_changed"
	case slices.Contains(changes, changeCanonicalID):
		return "service.service_account.renamed"
	default:
		return "service.service_account.metadata_changed"
	}
}

func (s *ServiceAccountService) view(account *model.ServiceAccount, now time.Time) *ServiceAccountView {
	status, reason := account.StatusAt(now)
	return &ServiceAccountView{Account: account, Status: status, FailureReason: reason, Narrowing: account.Narrowing()}
}

func (s *ServiceAccountService) auditArgs(event string, operator id.Principal, account *model.ServiceAccount) []any {
	return []any{
		"event", event,
		"operator_principal", operator,
		"service_id", account.ServiceID,
		"service_account_id", account.ID,
		"canonical_id", account.CanonicalID,
		"kind", string(account.Kind),
		"scopes", account.Scopes,
		"credential", account.Summary,
	}
}

func (s *ServiceAccountService) validateCredential(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID *id.ServiceAccountID, operation string, input model.ServiceAccountCredentialInput, now time.Time) (*model.ValidatedCredential, error) {
	validated, err := model.ValidateServiceAccountCredential(input, now)
	if err != nil {
		var rejection *model.CredentialValidationError
		if errors.As(err, &rejection) {
			args := []any{"event", "service.service_account.credential_rejected", "operator_principal", operator, "service_id", serviceID}
			if accountID != nil {
				args = append(args, "service_account_id", *accountID)
			}
			args = append(args, "operation", operation, "reason", string(rejection.Reason))
			s.logger.WarnContext(ctx, "service account credential rejected", args...)
		}
		return nil, err
	}
	return validated, nil
}

// encrypt seals the credential document under the owning service's context. The cause is not
// logged or returned, because this is a credential boundary.
func (s *ServiceAccountService) encrypt(ctx context.Context, operator id.Principal, account *model.ServiceAccount, operation string, document model.Secret) (model.Secret, error) {
	plaintext, err := document.GetPlaintext()
	if err != nil {
		return model.Secret{}, errServiceAccountEncryption
	}
	encryptionContext := domainencryption.NewServiceBranchKeySubject(account.ServiceID).EncryptionContext()
	ciphertext, err := s.encryption.Encrypt(ctx, []byte(plaintext), encryptionContext)
	if err != nil {
		var accountID *id.ServiceAccountID
		if operation != operationCreate {
			accountID = &account.ID
		}
		s.logFailure(ctx, operator, account.ServiceID, accountID, operation, "encryption")
		return model.Secret{}, errServiceAccountEncryption
	}
	return model.NewEncryptedSecret(ciphertext), nil
}

var errServiceAccountEncryption = errors.New("service account credential encryption failed")

// logWriteFailure records a storage failure that needs operator action. Not-found, conflict and
// validation results are client errors and are not logged here.
func (s *ServiceAccountService) logWriteFailure(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID *id.ServiceAccountID, operation string, err error) {
	kind := storage.ErrorKindUnknown
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		kind = storageErr.Kind
	}
	switch kind {
	case storage.ErrorKindNotFound, storage.ErrorKindConflict, storage.ErrorKindValidation:
		return
	}
	s.logFailure(ctx, operator, serviceID, accountID, operation, string(kind))
}

func (s *ServiceAccountService) logFailure(ctx context.Context, operator id.Principal, serviceID id.ServiceID, accountID *id.ServiceAccountID, operation, errorKind string) {
	args := []any{"event", "service.service_account.write_failed", "operator_principal", operator, "service_id", serviceID}
	if accountID != nil {
		args = append(args, "service_account_id", *accountID)
	}
	args = append(args, "operation", operation, "error_kind", errorKind)
	s.logger.ErrorContext(ctx, "service account write failed", args...)
}
