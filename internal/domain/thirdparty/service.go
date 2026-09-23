package thirdparty

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ThirdpartyOAuth2ProviderService manages external OAuth2 providers, coordinating
// conditional encryption and decryption with branch-key provisioning across provider
// lifecycle operations.
//
// Credential lifecycle:
//   - Create: validates entity, provisions the branch key, encrypts a confidential
//     Secret{plaintext} → Secret{ciphertext}, or stores a public Secret{absent} unchanged.
//   - Get/List: retrieve confidential entities with Secret{ciphertext} and decrypt them,
//     or return public entities with Secret{absent} unchanged.
//   - Update: validates entity, provisions the branch key idempotently, encrypts a
//     confidential Secret{plaintext} → Secret{ciphertext}, or stores a public
//     Secret{absent} unchanged.
//
// The repository (ThirdpartyOAuth2ProviderRepository) is unaware of encryption mechanics
// and treats Secret ciphertext as opaque binary data.
type ThirdpartyOAuth2ProviderService struct {
	repo                ports.ThirdpartyOAuth2ProviderRepository
	encryption          ports.EncryptionPort
	branchKeyManager    ports.BranchKeyManager
	cimdKeyReadiness    ports.CIMDClientKeyReadiness
	cimdPublicURL       string
	permissionSetRepo   ports.PermissionSetRepository // may be nil
	skipHTTPSValidation bool
	logger              *slog.Logger
}

// NewThirdpartyOAuth2ProviderService creates a new provider service.
// branchKeyManager must not be nil; inject the noop BranchKeyManager when no KMS store is configured.
// skipHTTPSValidation allows HTTP issuer/metadata URLs in development or test environments;
// set from config.Security.SkipThirdpartyHTTPSValidation.
func NewThirdpartyOAuth2ProviderService(
	repo ports.ThirdpartyOAuth2ProviderRepository,
	encryption ports.EncryptionPort,
	branchKeyManager ports.BranchKeyManager,
	permissionSetRepo ports.PermissionSetRepository,
	skipHTTPSValidation bool,
	logger *slog.Logger,
) *ThirdpartyOAuth2ProviderService {
	if logger == nil {
		logger = slog.Default()
	}
	if repo == nil {
		panic("thirdparty.NewThirdpartyOAuth2ProviderService: repo must not be nil")
	}
	if encryption == nil {
		panic("thirdparty.NewThirdpartyOAuth2ProviderService: encryption must not be nil")
	}
	if branchKeyManager == nil {
		panic("thirdparty.NewThirdpartyOAuth2ProviderService: branchKeyManager must not be nil")
	}
	return &ThirdpartyOAuth2ProviderService{
		repo:                repo,
		encryption:          encryption,
		branchKeyManager:    branchKeyManager,
		permissionSetRepo:   permissionSetRepo,
		skipHTTPSValidation: skipHTTPSValidation,
		logger:              logger,
	}
}

// WithCIMDKeyReadiness injects the publication guard for CIMD confidential services.
func (s *ThirdpartyOAuth2ProviderService) WithCIMDKeyReadiness(readiness ports.CIMDClientKeyReadiness) *ThirdpartyOAuth2ProviderService {
	s.cimdKeyReadiness = readiness
	return s
}

// WithCIMDPublicURL configures the fixed public origin used for broker-hosted CIMD identities.
func (s *ThirdpartyOAuth2ProviderService) WithCIMDPublicURL(publicURL string) *ThirdpartyOAuth2ProviderService {
	s.cimdPublicURL = publicURL
	return s
}

func (s *ThirdpartyOAuth2ProviderService) auditCIMD(serviceID id.ServiceID, operation, outcome string) {
	s.logger.Info("CIMD confidential service", "service_id", serviceID, "operation", operation, "outcome", outcome)
}

// ResolveID accepts a UUID or a type-scoped canonical ID.
func (s *ThirdpartyOAuth2ProviderService) ResolveID(ctx context.Context, value string) (id.ServiceID, error) {
	if parsed, err := id.ParseServiceID(value); err == nil {
		return parsed, nil
	}
	repo, ok := s.repo.(ports.ThirdpartyOAuth2ProviderCanonicalIDRepository)
	if !ok {
		return id.ServiceID{}, storage.NewStorageError("ResolveServiceID", storage.ErrorKindNotFound, nil, "provider not found")
	}
	entity, err := repo.GetByCanonicalID(ctx, value)
	if err != nil {
		return id.ServiceID{}, err
	}
	return entity.ID, nil
}

// CanonicalIDs returns canonical IDs for the requested providers.
func (s *ThirdpartyOAuth2ProviderService) CanonicalIDs(ctx context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error) {
	if repo, ok := s.repo.(ports.ThirdpartyOAuth2ProviderCanonicalIDRepository); ok {
		return repo.GetCanonicalIDs(ctx, ids)
	}
	return map[id.ServiceID]string{}, nil
}

// HasCompatibleCIMDServices verifies that every persisted CIMD service has the client ID
// derived from the configured public origin. It never rewrites persisted identities.
func (s *ThirdpartyOAuth2ProviderService) HasCompatibleCIMDServices(ctx context.Context) (bool, error) {
	services, err := s.repo.List(ctx)
	if err != nil {
		return false, fmt.Errorf("list persisted CIMD services: %w", err)
	}
	hasCIMDService := false
	for _, service := range services {
		if !service.IsCIMDConfidentialClient() {
			continue
		}
		hasCIMDService = true
		expected, err := model.CIMDClientID(s.cimdPublicURL, service.ID)
		if err != nil {
			return false, fmt.Errorf("server.enduser.public_url is invalid for CIMD service %s: %w", service.ID, err)
		}
		if service.ClientID != expected {
			return false, fmt.Errorf("CIMD service %s client_id does not match server.enduser.public_url", service.ID)
		}
	}
	return hasCIMDService, nil
}

// Create validates, provisions a branch key, conditionally encrypts the secret, and stores the entity.
// A confidential entity.Secret must be in plaintext state on entry and is encrypted on success.
// A public entity.Secret is absent and remains unchanged.
func (s *ThirdpartyOAuth2ProviderService) Create(
	ctx context.Context,
	entity *model.ThirdpartyOAuth2ProviderEntity,
) error {
	if err := entity.ValidateForCreate(s.skipHTTPSValidation); err != nil {
		if entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "create", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}
	entity.NormalizeProtectedResources()
	if err := entity.ValidateProtectedResources(); err != nil {
		if entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "create", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}

	if entity.ID.IsZero() {
		entity.ID = id.NewServiceID()
	}
	if entity.IsCIMDConfidentialClient() {
		clientID, err := model.CIMDClientID(s.cimdPublicURL, entity.ID)
		if err != nil {
			s.auditCIMD(entity.ID, "create", "rejected")
			return err
		}
		entity.ClientID = clientID
	}
	if entity.IsCIMDConfidentialClient() {
		if s.cimdKeyReadiness == nil {
			s.auditCIMD(entity.ID, "create", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := s.cimdKeyReadiness.RequirePublishedKey(ctx); err != nil {
			s.auditCIMD(entity.ID, "create", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", err)
		}
	}

	if !entity.IsCIMDConfidentialClient() {
		serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)
		branchKeyID, err := s.branchKeyManager.Create(ctx, serviceSubject)
		if err != nil {
			s.logger.Error("failed to provision branch key", "service_id", entity.ID, "error", err)
			return fmt.Errorf("branch key provisioning failed: %w", err)
		}
		s.logger.Info("branch key provisioned", "service_id", entity.ID, "branch_key_id", branchKeyID)
	}

	if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() {
		serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)
		plaintext, err := entity.Secret.GetPlaintext()
		if err != nil {
			return fmt.Errorf("entity secret must be in plaintext state for create: %w", err)
		}
		ciphertext, err := s.encryption.Encrypt(ctx, []byte(plaintext), serviceSubject.EncryptionContext())
		if err != nil {
			s.logger.Error("encryption_failed", "operation", "create_provider", "service_id", entity.ID, "reason", err)
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		entity.Secret = model.NewEncryptedSecret(ciphertext)
		s.logger.Info("service_secret_encrypted", "operation", "create", "service_id", entity.ID)
	}

	if err := s.repo.Create(ctx, entity); err != nil {
		if entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "create", "rejected")
		}
		return fmt.Errorf("failed to store provider: %w", err)
	}

	if entity.IsCIMDConfidentialClient() {
		s.auditCIMD(entity.ID, "create", "success")
		return nil
	}
	s.logger.Info("provider_created", "event", "service.thirdparty.provider_created", "service_id", entity.ID, "display_name", entity.DisplayName, "public_client", entity.IsPublicClient())
	return nil
}

// Get retrieves a provider by ID and decrypts its confidential secret.
// Public providers are returned with their Secret in absent state.
// If confidential-secret decryption fails (e.g. after switching encryption backends),
// the entity is returned with its Secret still in encrypted state and a nil error.
// This enables admin workflows (list, view, update) to continue operating
// even when the encryption backend has changed and old ciphertexts cannot
// be decrypted. Callers that require the plaintext secret (e.g. OAuth2
// session flows) should check entity.Secret.IsPlaintext() before use.
func (s *ThirdpartyOAuth2ProviderService) Get(
	ctx context.Context,
	serviceID id.ServiceID,
) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	entity, err := s.repo.Get(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	// Guard: verify ID consistency for data integrity
	if entity.ID != serviceID {
		s.logger.Error("service_id_mismatch",
			"expected_id", serviceID,
			"actual_id", entity.ID)
		return nil, fmt.Errorf("provider ID mismatch: expected %s, got %s", serviceID, entity.ID)
	}

	if entity.IsPublicClient() || entity.IsCIMDConfidentialClient() {
		return entity, nil
	}

	dec, decErr := s.decryptSecret(ctx, entity)
	if decErr != nil {
		s.logger.Warn("secret_decryption_failed_returning_encrypted",
			"operation", "get",
			"service_id", entity.ID,
			"reason", decErr)
		return entity, nil
	}
	return dec, nil
}

// GetCIMDClientService returns only the public state needed to compose CIMD metadata.
func (s *ThirdpartyOAuth2ProviderService) GetCIMDClientService(
	ctx context.Context,
	serviceID id.ServiceID,
) (*ports.CIMDClientServiceProjection, error) {
	entity, err := s.repo.Get(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.ID != serviceID || !entity.IsCIMDConfidentialClient() || entity.ClientID.IsZero() {
		return nil, ports.ErrNotFound
	}
	return &ports.CIMDClientServiceProjection{
		ID:                      entity.ID,
		ClientID:                entity.ClientID,
		TokenEndpointAuthMethod: string(entity.TokenEndpointAuthMethod),
	}, nil
}

// Update validates, provisions a branch key, conditionally encrypts the secret, and stores the entity.
// A confidential entity.Secret must be in plaintext state on entry and is encrypted on success.
// A public entity.Secret is absent and remains unchanged.
//
// Update provisions the branch key before encrypting. This handles the migration case
// where a service was originally created with a different encryption backend (e.g. raw
// AES in-memory) that has no branch key entry in the current KMS key store.
// branchKeyManager.Create is idempotent: it is safe to call on services whose branch
// key already exists.
func (s *ThirdpartyOAuth2ProviderService) Update(
	ctx context.Context,
	entity *model.ThirdpartyOAuth2ProviderEntity,
	expectedVersion *int64,
) error {
	if err := entity.ValidateForUpdate(s.skipHTTPSValidation); err != nil {
		if entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "update", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}
	entity.NormalizeProtectedResources()
	if err := entity.ValidateProtectedResources(); err != nil {
		if entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "update", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}
	if entity.IsCIMDConfidentialClient() {
		persisted, err := s.repo.Get(ctx, entity.ID)
		if err != nil {
			s.auditCIMD(entity.ID, "update", "rejected")
			return fmt.Errorf("get existing CIMD service: %w", err)
		}
		clientID, err := model.CIMDClientID(s.cimdPublicURL, entity.ID)
		if err != nil {
			s.auditCIMD(entity.ID, "update", "rejected")
			return err
		}
		if persisted.IsCIMDConfidentialClient() && persisted.ClientID != clientID {
			s.auditCIMD(entity.ID, "update", "rejected")
			return fmt.Errorf("persisted CIMD client_id for service %s does not match server.enduser.public_url", entity.ID)
		}
		entity.ClientID = clientID
	}
	if entity.IsCIMDConfidentialClient() {
		if s.cimdKeyReadiness == nil {
			s.auditCIMD(entity.ID, "update", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := s.cimdKeyReadiness.RequirePublishedKey(ctx); err != nil {
			s.auditCIMD(entity.ID, "update", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", err)
		}
	}

	if !entity.IsCIMDConfidentialClient() {
		serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)
		branchKeyID, err := s.branchKeyManager.Create(ctx, serviceSubject)
		if err != nil {
			s.logger.Error("failed to ensure branch key for update", "service_id", entity.ID, "error", err)
			return fmt.Errorf("branch key provisioning failed: %w", err)
		}
		s.logger.Info("branch key ready for update", "service_id", entity.ID, "branch_key_id", branchKeyID)
	}

	if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() {
		serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)
		plaintext, err := entity.Secret.GetPlaintext()
		if err != nil {
			return fmt.Errorf("failed to read plaintext secret for update: %w", err)
		}
		ciphertext, err := s.encryption.Encrypt(ctx, []byte(plaintext), serviceSubject.EncryptionContext())
		if err != nil {
			s.logger.Error("encryption_failed", "operation", "update_provider", "service_id", entity.ID, "reason", err)
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		entity.Secret = model.NewEncryptedSecret(ciphertext)
		s.logger.Info("service_secret_encrypted", "operation", "update", "service_id", entity.ID)
	}

	if err := s.repo.Update(ctx, entity, expectedVersion); err != nil {
		if entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "update", "rejected")
		}
		return err
	}

	if entity.IsCIMDConfidentialClient() {
		s.auditCIMD(entity.ID, "update", "success")
		return nil
	}
	s.logger.Info("provider_updated", "event", "service.thirdparty.provider_updated", "service_id", entity.ID, "display_name", entity.DisplayName, "public_client", entity.IsPublicClient())
	return nil
}

// List retrieves all providers and decrypts confidential secrets.
// Public providers are returned with their Secret in absent state. If decryption fails
// for individual confidential entities (e.g. after switching encryption backends),
// those entities are returned with their Secret still in encrypted state. A warning is
// logged for each decryption failure. This enables admin workflows to continue operating
// even when old ciphertexts cannot be decrypted.
func (s *ThirdpartyOAuth2ProviderService) List(
	ctx context.Context,
) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	entities, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*model.ThirdpartyOAuth2ProviderEntity, 0, len(entities))
	for _, entity := range entities {
		if entity.IsPublicClient() || entity.IsCIMDConfidentialClient() {
			result = append(result, entity)
			continue
		}
		dec, err := s.decryptSecret(ctx, entity)
		if err != nil {
			s.logger.Warn("secret_decryption_failed_returning_encrypted",
				"operation", "list",
				"service_id", entity.ID,
				"reason", err)
			result = append(result, entity)
			continue
		}
		result = append(result, dec)
	}

	return result, nil
}

// Delete removes a provider by ID.
// Returns a conflict error if permission sets reference this service.
func (s *ThirdpartyOAuth2ProviderService) Delete(
	ctx context.Context,
	serviceID id.ServiceID,
) error {
	// Check if any permission sets reference this service (deletion protection)
	if s.permissionSetRepo != nil {
		count, err := s.permissionSetRepo.CountPermissionSetsForService(ctx, serviceID)
		if err != nil {
			return fmt.Errorf("failed to check permission set references: %w", err)
		}

		if count > 0 {
			return storage.NewStorageError(
				"DeleteService",
				storage.ErrorKindConflict,
				nil,
				fmt.Sprintf("cannot delete service: %d permission set(s) reference it", count),
			)
		}
	}

	if err := s.repo.Delete(ctx, serviceID); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	s.logger.Info("provider_deleted", "service_id", serviceID)
	return nil
}

// FindByProtectedResource retrieves a provider by resource URI and decrypts confidential secrets.
// Public providers are returned with their Secret in absent state. If decryption fails (e.g. after
// switching encryption backends), the entity is returned with its Secret still in encrypted state.
// This ensures that callers performing existence/ID checks (such as duplicate resource URI detection
// in PUT/POST handlers) continue to work even when old ciphertexts cannot be decrypted.
func (s *ThirdpartyOAuth2ProviderService) FindByProtectedResource(
	ctx context.Context,
	resourceURI string,
) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	entity, err := s.repo.FindByProtectedResource(ctx, resourceURI)
	if err != nil {
		return nil, err
	}

	if entity.IsPublicClient() {
		return entity, nil
	}

	dec, decErr := s.decryptSecret(ctx, entity)
	if decErr != nil {
		s.logger.Warn("secret_decryption_failed_returning_encrypted",
			"operation", "find_by_protected_resource",
			"service_id", entity.ID,
			"reason", decErr)
		return entity, nil
	}
	return dec, nil
}

// AddProtectedResource validates and atomically adds one protected resource.
func (s *ThirdpartyOAuth2ProviderService) AddProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	normalized, err := model.NormalizeAndValidateProtectedResource(resourceURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, storage.NewStorageError("AddProtectedResource", storage.ErrorKindValidation, err, err.Error())
	}
	return s.repo.AddProtectedResource(ctx, serviceID, normalized)
}

// RemoveProtectedResource validates and atomically removes one protected resource.
func (s *ThirdpartyOAuth2ProviderService) RemoveProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	normalized, err := model.NormalizeAndValidateProtectedResource(resourceURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, storage.NewStorageError("RemoveProtectedResource", storage.ErrorKindValidation, err, err.Error())
	}
	return s.repo.RemoveProtectedResource(ctx, serviceID, normalized)
}

// RenameProtectedResource validates and atomically renames one protected resource.
func (s *ThirdpartyOAuth2ProviderService) RenameProtectedResource(ctx context.Context, serviceID id.ServiceID, fromURI, toURI string) (ports.ProtectedResourceMutationResult, error) {
	from, err := model.NormalizeAndValidateProtectedResource(fromURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, storage.NewStorageError("RenameProtectedResource", storage.ErrorKindValidation, err, err.Error())
	}
	to, err := model.NormalizeAndValidateProtectedResource(toURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, storage.NewStorageError("RenameProtectedResource", storage.ErrorKindValidation, err, err.Error())
	}
	return s.repo.RenameProtectedResource(ctx, serviceID, from, to)
}

// ListProtectedResources returns the normalized resource set and current version.
func (s *ThirdpartyOAuth2ProviderService) ListProtectedResources(ctx context.Context, serviceID id.ServiceID) ([]string, int64, error) {
	return s.repo.ListProtectedResources(ctx, serviceID)
}

// ValidateServiceRequirements validates that all service_ids in the requirements exist and
// that all required_scopes are valid for the referenced service. This enforces referential
// integrity between agents and their declared service requirements.
//
// Uses the repository directly to avoid unnecessary secret decryption — only Scopes and
// DisplayName are needed for validation.
func (s *ThirdpartyOAuth2ProviderService) ValidateServiceRequirements(
	ctx context.Context,
	serviceReqs []storage.ServiceRequirement,
) error {
	if len(serviceReqs) == 0 {
		return nil
	}

	for i, sr := range serviceReqs {
		entity, err := s.repo.Get(ctx, sr.ServiceID)
		if err != nil {
			var storageErr *storage.StorageError
			if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
				s.logger.Warn("service not found for requirement",
					"service_id", sr.ServiceID, "index", i)
				return storage.NewStorageError(
					"ValidateServiceRequirements",
					storage.ErrorKindValidation,
					nil,
					"service_id "+sr.ServiceID.String()+" not found (index "+strconv.Itoa(i)+")",
				)
			}
			return err
		}

		serviceScopes := make(map[string]bool)
		for _, scope := range entity.Scopes {
			serviceScopes[scope.ScopeValue] = true
		}

		for _, requiredScope := range sr.RequiredScopes {
			if !serviceScopes[requiredScope] {
				s.logger.Warn("invalid scope for service",
					"service_id", sr.ServiceID,
					"service_name", entity.DisplayName,
					"scope", requiredScope,
					"index", i)
				return storage.NewStorageError(
					"ValidateServiceRequirements",
					storage.ErrorKindValidation,
					nil,
					"scope "+requiredScope+" not found in service "+entity.DisplayName+" (index "+strconv.Itoa(i)+")",
				)
			}
		}
	}

	return nil
}

// decryptSecret decrypts the entity's Secret field and returns a copy with the
// plaintext secret. It does not log on failure — callers are responsible for
// logging at the appropriate severity level for their use case.
func (s *ThirdpartyOAuth2ProviderService) decryptSecret(
	ctx context.Context,
	entity *model.ThirdpartyOAuth2ProviderEntity,
) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	ciphertext, err := entity.Secret.GetCiphertext()
	if err != nil {
		return nil, fmt.Errorf("entity has no encrypted secret: %w", err)
	}

	serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)

	plaintext, err := s.encryption.Decrypt(ctx, ciphertext, serviceSubject.EncryptionContext())
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client secret: %w", err)
	}

	// Work on a copy to avoid mutating the entity returned by the repository
	result := entity.Copy()
	result.Secret = model.NewPlaintextSecret(string(plaintext))

	s.logger.InfoContext(ctx, "service_secret_decrypted",
		"service_id", entity.ID)

	return result, nil
}
