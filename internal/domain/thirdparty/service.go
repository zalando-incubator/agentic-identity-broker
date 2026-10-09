package thirdparty

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"time"

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
	repo                 ports.ThirdpartyOAuth2ProviderRepository
	statusWriter         ports.ThirdpartyOAuth2ProviderDiscoveryStatusWriter
	oauthDiscoveryClient ports.OAuthDiscoveryClient
	encryption           ports.EncryptionPort
	branchKeyManager     ports.BranchKeyManager
	cimdKeyReadiness     ports.CIMDClientKeyReadiness
	cimdPublicURL        string
	dcrClientName        string
	permissionSetRepo    ports.PermissionSetRepository // may be nil
	userSessions         ports.UserSessionRepository
	skipHTTPSValidation  bool
	logger               *slog.Logger
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

// WithOAuthDiscoveryClient supplies outbound resource probes and bounded JSON I/O.
func (s *ThirdpartyOAuth2ProviderService) WithOAuthDiscoveryClient(client ports.OAuthDiscoveryClient) *ThirdpartyOAuth2ProviderService {
	s.oauthDiscoveryClient = client
	return s
}

// WithDCRClientName sets the deployment-wide name sent during client registration.
func (s *ThirdpartyOAuth2ProviderService) WithDCRClientName(name string) *ThirdpartyOAuth2ProviderService {
	s.dcrClientName = name
	return s
}

// WithUserSessions supplies the session count required before an issuer or audience change.
func (s *ThirdpartyOAuth2ProviderService) WithUserSessions(repo ports.UserSessionRepository) *ThirdpartyOAuth2ProviderService {
	s.userSessions = repo
	return s
}

// WithDiscoveryStatusWriter supplies the same-row failure-only storage port.
func (s *ThirdpartyOAuth2ProviderService) WithDiscoveryStatusWriter(writer ports.ThirdpartyOAuth2ProviderDiscoveryStatusWriter) *ThirdpartyOAuth2ProviderService {
	s.statusWriter = writer
	return s
}

// errDiscoveryInfrastructure marks local failures for audit classification. Its
// message is safe to return when an implementation error may contain secrets.
var errDiscoveryInfrastructure = errors.New("discovery infrastructure unavailable")

func (s *ThirdpartyOAuth2ProviderService) auditCIMD(serviceID id.ServiceID, operation, outcome string) {
	s.logger.Info("CIMD confidential service", "service_id", serviceID, "operation", operation, "outcome", outcome)
}

// auditDiscovery uses validated metadata or persisted state for issuer and method.
// It never records URL queries, remote bodies, or credentials.
func (s *ThirdpartyOAuth2ProviderService) auditDiscovery(entity *model.ThirdpartyOAuth2ProviderEntity, operation string, result error, known *model.ThirdpartyOAuth2ProviderEntity) {
	fields := []any{"service_id", entity.ID, "operation", operation}
	if known != nil {
		fields = append(fields, "issuer", known.IssuerURI, "client_method", known.Discovery.ClientMethod)
	}
	if result == nil {
		fields = append(fields, "outcome", "success")
	} else {
		fields = append(fields, "outcome", "rejected")
		var discoveryErr *DiscoveryError
		var storageErr *storage.StorageError
		var encryptionErr *domainencryption.EncryptionError
		switch {
		case errors.Is(result, context.Canceled):
			// The caller stopped the attempt; no discovery failure was recorded.
		case errors.As(result, &discoveryErr):
			fields = append(fields, "failure_code", discoveryErr.Code)
		case errors.Is(result, storage.ErrDuplicateDCRClientIdentity):
			fields = append(fields, "failure_code", "duplicate_client_identity")
		case errors.Is(result, storage.ErrIssuerChangeHasSessions):
			fields = append(fields, "failure_code", "issuer_change_requires_no_sessions")
		case errors.Is(result, storage.ErrResourceChangeHasSessions):
			fields = append(fields, "failure_code", "resource_change_requires_no_sessions")
		case errors.As(result, &storageErr):
			if storageErr.Kind == storage.ErrorKindConnection || storageErr.Kind == storage.ErrorKindTimeout || storageErr.Kind == storage.ErrorKindUnknown {
				fields = append(fields, "failure_code", "internal_failure")
			}
		case errors.As(result, &encryptionErr), errors.Is(result, errDiscoveryInfrastructure):
			fields = append(fields, "failure_code", "internal_failure")
		}
	}
	s.logger.Info("protected-resource discovery", fields...)
}

const discoveryAttemptTimeout = 15 * time.Second

func setEffectiveDiscoveryResource(entity *model.ThirdpartyOAuth2ProviderEntity) {
	if entity.AuthorizationParams == nil {
		entity.AuthorizationParams = make(map[string]string, 1)
	}
	_, entity.ResourceExplicit = entity.AuthorizationParams["resource"]
	if !entity.ResourceExplicit {
		entity.AuthorizationParams["resource"] = *entity.Discovery.ResourceURL
	}
}

func (s *ThirdpartyOAuth2ProviderService) prepareDiscoveryCreate(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) (time.Time, error) {
	if err := entity.ValidateDiscoveryRequest(); err != nil {
		return time.Time{}, fmt.Errorf("provider validation failed: %w", err)
	}
	if entity.ID.IsZero() {
		entity.ID = id.NewServiceID()
	}
	attemptCtx, cancel := context.WithTimeout(ctx, discoveryAttemptTimeout)
	defer cancel()
	selected, err := s.discoverProtectedResource(attemptCtx, *entity.Discovery.ResourceURL, entity.IssuerURI, entity.ID)
	if err := attemptCtx.Err(); err != nil {
		return time.Time{}, discoveryReadFailure(attemptCtx, err, "resource_metadata")
	}
	if err != nil {
		return time.Time{}, err
	}
	entity.IssuerURI = selected.issuer
	entity.Endpoints = selected.endpoints
	entity.Discovery.ClientMethod = selected.method
	entity.TokenEndpointAuthMethod = selected.auth
	if selected.method == model.ClientBootstrapDCR {
		entity.ClientID = selected.clientID
		entity.Secret = selected.secret
	}
	setEffectiveDiscoveryResource(entity)
	return time.Now().UTC(), nil
}

// prepareDiscoveryUpdate refreshes the active issuer without changing the client
// method. Issuer or effective audience changes first require zero user sessions.
func (s *ThirdpartyOAuth2ProviderService) prepareDiscoveryUpdate(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) (completedAt time.Time, persistedResult *model.ThirdpartyOAuth2ProviderEntity, resultErr error) {
	if entity.ID.IsZero() {
		return time.Time{}, nil, fmt.Errorf("provider validation failed: provider ID cannot be empty")
	}
	if err := entity.ValidateDiscoveryRequest(); err != nil {
		return time.Time{}, nil, fmt.Errorf("provider validation failed: %w", err)
	}
	persisted, err := s.repo.Get(ctx, entity.ID)
	if err != nil {
		return time.Time{}, nil, err
	}
	if persisted.Discovery.ResourceURL == nil || persisted.Discovery.ClientMethod != model.ClientBootstrapCIMD && persisted.Discovery.ClientMethod != model.ClientBootstrapDCR {
		return time.Time{}, nil, discoveryFailure("client_method_changed")
	}
	if entity.AuthorizationParams == nil {
		entity.AuthorizationParams = maps.Clone(persisted.AuthorizationParams)
		entity.ResourceExplicit = persisted.ResourceExplicit
	} else {
		_, entity.ResourceExplicit = entity.AuthorizationParams["resource"]
	}
	if entity.AuthorizationParams == nil {
		entity.AuthorizationParams = make(map[string]string, 1)
	}
	if !entity.ResourceExplicit {
		entity.AuthorizationParams["resource"] = *entity.Discovery.ResourceURL
	}
	issuerChanged := entity.IssuerURI != "" && entity.IssuerURI != persisted.IssuerURI
	resourceChanged := entity.AuthorizationParams["resource"] != persisted.AuthorizationParams["resource"]
	if issuerChanged || resourceChanged {
		if s.userSessions == nil {
			return time.Time{}, nil, fmt.Errorf("user session storage is unavailable: %w", errDiscoveryInfrastructure)
		}
		count, err := s.userSessions.CountByService(ctx, entity.ID)
		if err != nil {
			return time.Time{}, nil, fmt.Errorf("count service sessions: %w", err)
		}
		if count != 0 {
			if issuerChanged {
				return time.Now().UTC(), persisted, discoveryFailure("issuer_change_requires_no_sessions")
			}
			return time.Now().UTC(), persisted, discoveryFailure("resource_change_requires_no_sessions")
		}
	}
	if !issuerChanged {
		entity.IssuerURI = persisted.IssuerURI
	}
	attemptCtx, cancel := context.WithTimeout(ctx, discoveryAttemptTimeout)
	defer func() {
		if resultErr != nil {
			completedAt = time.Now().UTC()
			persistedResult = persisted
		}
	}()
	defer cancel()
	issuer, metadata, endpoints, err := s.readProtectedResourceMetadata(attemptCtx, *entity.Discovery.ResourceURL, entity.IssuerURI)
	if ctxErr := attemptCtx.Err(); ctxErr != nil {
		return time.Time{}, nil, discoveryReadFailure(attemptCtx, ctxErr, "resource_metadata")
	}
	if err != nil {
		return time.Time{}, nil, err
	}
	if issuer != persisted.IssuerURI && !issuerChanged {
		return time.Time{}, nil, discoveryFailure("client_method_changed")
	}
	switch persisted.Discovery.ClientMethod {
	case model.ClientBootstrapCIMD:
		if persisted.TokenEndpointAuthMethod != model.TokenEndpointAuthMethodPrivateKeyJWT || !supportsCIMD(metadata) {
			return time.Time{}, nil, discoveryFailure("client_method_changed")
		}
		if err := s.requireCIMDKey(attemptCtx); err != nil {
			return time.Time{}, nil, err
		}
	case model.ClientBootstrapDCR:
		if persisted.ClientID.IsZero() || !supportsDCRAuthentication(metadata, persisted.TokenEndpointAuthMethod) {
			return time.Time{}, nil, discoveryFailure("client_method_changed")
		}
		if !persisted.IsPublicClient() && !persisted.Secret.IsEncrypted() {
			return time.Time{}, nil, discoveryFailure("client_method_changed")
		}
		if issuerChanged {
			if !supportsDCRMethod(metadata, persisted.TokenEndpointAuthMethod) {
				return time.Time{}, nil, discoveryFailure("client_method_changed")
			}
			entity.ClientID, entity.Secret, err = s.registerDCRClient(attemptCtx, *metadata.RegistrationEndpoint, entity.ID, persisted.TokenEndpointAuthMethod)
			if err != nil {
				return time.Time{}, nil, err
			}
		} else {
			entity.ClientID = persisted.ClientID
			entity.Secret = persisted.Secret
		}
	}
	if ctxErr := attemptCtx.Err(); ctxErr != nil {
		return time.Time{}, nil, discoveryReadFailure(attemptCtx, ctxErr, "resource_metadata")
	}
	entity.IssuerURI = issuer
	entity.Endpoints = endpoints
	entity.Discovery.ClientMethod = persisted.Discovery.ClientMethod
	entity.TokenEndpointAuthMethod = persisted.TokenEndpointAuthMethod
	return time.Now().UTC(), persisted, nil
}

func (s *ThirdpartyOAuth2ProviderService) recordFailedDiscoveryUpdate(ctx context.Context, active *model.ThirdpartyOAuth2ProviderEntity, completedAt time.Time, failure error) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(failure, context.Canceled) {
		return context.Canceled
	}
	var discoveryErr *DiscoveryError
	code := ""
	switch {
	case errors.As(failure, &discoveryErr):
		code = discoveryErr.Code
	case errors.Is(failure, storage.ErrDuplicateDCRClientIdentity):
		code = "duplicate_client_identity"
	case errors.Is(failure, storage.ErrIssuerChangeHasSessions):
		code = "issuer_change_requires_no_sessions"
	case errors.Is(failure, storage.ErrResourceChangeHasSessions):
		code = "resource_change_requires_no_sessions"
	default:
		return failure
	}
	if s.statusWriter == nil {
		return fmt.Errorf("discovery status storage is unavailable: %w", errDiscoveryInfrastructure)
	}
	if err := s.statusWriter.RecordDiscoveryFailure(ctx, active.ID, active.Version, completedAt, code); err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict {
			return failure
		}
		return fmt.Errorf("record discovery status: %w", err)
	}
	return failure
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
) (resultErr error) {
	discovered := entity.Discovery.ResourceURL != nil
	var attemptedAt time.Time
	var selected *model.ThirdpartyOAuth2ProviderEntity
	if discovered {
		defer func() { s.auditDiscovery(entity, "create", resultErr, selected) }()
		var err error
		attemptedAt, err = s.prepareDiscoveryCreate(ctx, entity)
		if err != nil {
			return err
		}
		selected = entity
	}
	if err := entity.ValidateForCreate(s.skipHTTPSValidation); err != nil {
		if !discovered && entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "create", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}
	entity.NormalizeProtectedResources()
	if err := entity.ValidateProtectedResources(); err != nil {
		if !discovered && entity.IsCIMDConfidentialClient() {
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
			if !discovered {
				s.auditCIMD(entity.ID, "create", "rejected")
			}
			if discovered {
				return discoveryFailure("cimd_unavailable")
			}
			return err
		}
		entity.ClientID = clientID
	}
	if !discovered && entity.IsCIMDConfidentialClient() {
		if s.cimdKeyReadiness == nil {
			s.auditCIMD(entity.ID, "create", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := s.cimdKeyReadiness.RequireUsablePublishedKey(ctx); err != nil {
			s.auditCIMD(entity.ID, "create", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", err)
		}
	}

	serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)
	branchKeyID, err := s.branchKeyManager.Create(ctx, serviceSubject)
	if err != nil {
		if !discovered {
			s.logger.Error("failed to provision branch key", "service_id", entity.ID, "error", err)
			if entity.IsCIMDConfidentialClient() {
				s.auditCIMD(entity.ID, "create", "rejected")
			}
		}
		if discovered && errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		if discovered {
			return fmt.Errorf("branch key provisioning failed: %w", errDiscoveryInfrastructure)
		}
		return fmt.Errorf("branch key provisioning failed: %w", err)
	}
	if !discovered {
		s.logger.Info("branch key provisioned", "service_id", entity.ID, "branch_key_id", branchKeyID)
	}

	if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() {
		plaintext, err := entity.Secret.GetPlaintext()
		if err != nil {
			return fmt.Errorf("entity secret must be in plaintext state for create: %w", err)
		}
		ciphertext, err := s.encryption.Encrypt(ctx, []byte(plaintext), serviceSubject.EncryptionContext())
		if err != nil {
			if !discovered {
				s.logger.Error("encryption_failed", "operation", "create_provider", "service_id", entity.ID, "reason", err)
			}
			if discovered && errors.Is(ctx.Err(), context.Canceled) {
				return context.Canceled
			}
			if discovered {
				return fmt.Errorf("failed to encrypt client secret: %w", errDiscoveryInfrastructure)
			}
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		entity.Secret = model.NewEncryptedSecret(ciphertext)
		if !discovered {
			s.logger.Info("service_secret_encrypted", "operation", "create", "service_id", entity.ID)
		}
	}

	if discovered {
		entity.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &attemptedAt, LastSuccessAt: &attemptedAt}
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		if !discovered && entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "create", "rejected")
		}
		return fmt.Errorf("failed to store provider: %w", err)
	}

	if discovered {
		return nil
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

// DiscoveryStatusView exposes only the stored discovery state, never credentials.
type DiscoveryStatusView struct {
	Status        string
	ResourceURL   *string
	IssuerURI     *string
	ClientMethod  *string
	LastAttemptAt *time.Time
	LastSuccessAt *time.Time
	FailureReason *string
}

// GetDiscoveryStatus reads the service's committed status without provider I/O or decryption.
func (s *ThirdpartyOAuth2ProviderService) GetDiscoveryStatus(ctx context.Context, serviceID id.ServiceID) (*DiscoveryStatusView, error) {
	entity, err := s.repo.Get(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if entity.ID != serviceID {
		return nil, errors.New("provider ID mismatch")
	}
	status := &DiscoveryStatusView{Status: entity.DiscoveryStatus.Status(entity.Discovery.ResourceURL)}
	if entity.Discovery.ResourceURL == nil {
		return status, nil
	}
	resourceURL := *entity.Discovery.ResourceURL
	issuerURI := entity.IssuerURI
	clientMethod := string(entity.Discovery.ClientMethod)
	status.ResourceURL = &resourceURL
	status.IssuerURI = &issuerURI
	status.ClientMethod = &clientMethod
	status.LastAttemptAt = entity.DiscoveryStatus.LastAttemptAt
	status.LastSuccessAt = entity.DiscoveryStatus.LastSuccessAt
	status.FailureReason = entity.DiscoveryStatus.FailureReason
	return status, nil
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
) (resultErr error) {
	discovered := entity.Discovery.ResourceURL != nil
	var attemptedAt time.Time
	var persistedDiscovery *model.ThirdpartyOAuth2ProviderEntity
	var selected *model.ThirdpartyOAuth2ProviderEntity
	if discovered {
		defer func() {
			known := persistedDiscovery
			if selected != nil {
				known = selected
			}
			s.auditDiscovery(entity, "update", resultErr, known)
		}()
		var err error
		attemptedAt, persistedDiscovery, err = s.prepareDiscoveryUpdate(ctx, entity)
		if err != nil {
			if persistedDiscovery != nil {
				return s.recordFailedDiscoveryUpdate(ctx, persistedDiscovery, attemptedAt, err)
			}
			return err
		}
		selected = entity
	}
	if err := entity.ValidateForUpdate(s.skipHTTPSValidation); err != nil {
		if !discovered && entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "update", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}
	entity.NormalizeProtectedResources()
	if err := entity.ValidateProtectedResources(); err != nil {
		if !discovered && entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "update", "rejected")
		}
		return fmt.Errorf("provider validation failed: %w", err)
	}
	if entity.IsCIMDConfidentialClient() {
		if discovered {
			clientID, err := model.CIMDClientID(s.cimdPublicURL, entity.ID)
			if err != nil || persistedDiscovery.ClientID != clientID {
				return discoveryFailure("cimd_unavailable")
			}
			entity.ClientID = persistedDiscovery.ClientID
		} else {
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
	}
	if !discovered && entity.IsCIMDConfidentialClient() {
		if s.cimdKeyReadiness == nil {
			s.auditCIMD(entity.ID, "update", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := s.cimdKeyReadiness.RequireUsablePublishedKey(ctx); err != nil {
			s.auditCIMD(entity.ID, "update", "rejected")
			return fmt.Errorf("CIMD client-authentication key readiness: %w", err)
		}
	}

	serviceSubject := domainencryption.NewServiceBranchKeySubject(entity.ID)
	branchKeyID, err := s.branchKeyManager.Create(ctx, serviceSubject)
	if err != nil {
		if !discovered {
			s.logger.Error("failed to ensure branch key for update", "service_id", entity.ID, "error", err)
			if entity.IsCIMDConfidentialClient() {
				s.auditCIMD(entity.ID, "update", "rejected")
			}
		}
		if discovered && errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		if discovered {
			return fmt.Errorf("branch key provisioning failed: %w", errDiscoveryInfrastructure)
		}
		return fmt.Errorf("branch key provisioning failed: %w", err)
	}
	if !discovered {
		s.logger.Info("branch key ready for update", "service_id", entity.ID, "branch_key_id", branchKeyID)
	}

	retainsEncryptedDCRSecret := discovered && persistedDiscovery.Discovery.ClientMethod == model.ClientBootstrapDCR && entity.Secret.IsEncrypted()
	if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() && !retainsEncryptedDCRSecret {
		plaintext, err := entity.Secret.GetPlaintext()
		if err != nil {
			return fmt.Errorf("failed to read plaintext secret for update: %w", err)
		}
		ciphertext, err := s.encryption.Encrypt(ctx, []byte(plaintext), serviceSubject.EncryptionContext())
		if err != nil {
			if !discovered {
				s.logger.Error("encryption_failed", "operation", "update_provider", "service_id", entity.ID, "reason", err)
			}
			if discovered && errors.Is(ctx.Err(), context.Canceled) {
				return context.Canceled
			}
			if discovered {
				return fmt.Errorf("failed to encrypt client secret: %w", errDiscoveryInfrastructure)
			}
			return fmt.Errorf("failed to encrypt client secret: %w", err)
		}
		entity.Secret = model.NewEncryptedSecret(ciphertext)
		if !discovered {
			s.logger.Info("service_secret_encrypted", "operation", "update", "service_id", entity.ID)
		}
	}

	if discovered {
		entity.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &attemptedAt, LastSuccessAt: &attemptedAt}
	}
	if err := s.repo.Update(ctx, entity, expectedVersion); err != nil {
		if !discovered && entity.IsCIMDConfidentialClient() {
			s.auditCIMD(entity.ID, "update", "rejected")
		}
		if discovered && persistedDiscovery != nil &&
			(errors.Is(err, storage.ErrDuplicateDCRClientIdentity) || errors.Is(err, storage.ErrIssuerChangeHasSessions) || errors.Is(err, storage.ErrResourceChangeHasSessions)) {
			return s.recordFailedDiscoveryUpdate(ctx, persistedDiscovery, time.Now().UTC(), err)
		}
		return err
	}

	if discovered {
		return nil
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

// FindByProtectedResource resolves a provider without decrypting its secret.
// Confidential secrets remain encrypted until a caller retrieves the provider with Get or List.
func (s *ThirdpartyOAuth2ProviderService) FindByProtectedResource(
	ctx context.Context,
	resourceURI string,
) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return s.repo.FindByProtectedResource(ctx, resourceURI)
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
