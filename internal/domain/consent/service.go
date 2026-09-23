package consent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var (
	// ErrAgentNotFound is returned when the specified agent does not exist.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrInvalidScopes is returned when requested scopes don't exist in the service configuration.
	ErrInvalidScopes = errors.New("invalid scopes requested")
	// ErrServiceNotFound is returned when a referenced service does not exist.
	ErrServiceNotFound = errors.New("third-party service not found")
	// ErrAgentAccessDenied is returned when a user has not granted an agent access.
	ErrAgentAccessDenied = errors.New("agent access denied")
	// ErrGrantExpired is returned when a user grant has expired.
	ErrGrantExpired = errors.New("grant expired")
	// ErrGrantNotFound is returned by RevokeConsentForPrincipal when no active grant exists
	// for the (principal, agent) pair. The handler maps this to HTTP 404.
	ErrGrantNotFound = errors.New("grant not found")
	// ErrUnconnectedServices is returned when the submission includes services without active sessions.
	ErrUnconnectedServices = errors.New("unconnected services")
	// ErrMissingMandatoryPS is returned when a mandatory permission set is not present in the grant.
	ErrMissingMandatoryPS = errors.New("missing mandatory permission set")
	// ErrInvalidServiceInclusion is returned when included_service_ids contains invalid service IDs.
	ErrInvalidServiceInclusion = errors.New("invalid service inclusion")
	// ErrGrantValidation is returned when a UserGrant fails domain validation (e.g. empty
	// scope list, duplicate scopes). The handler maps this to HTTP 400.
	ErrGrantValidation = errors.New("grant validation failed")
)

// PermissionSetQuerier is the minimal interface of permissionset.Service used by consent.
// Defined here to allow test doubles without coupling to the concrete type.
type PermissionSetQuerier interface {
	ValidateIDs(ctx context.Context, ids []id.PermissionSetID) error
	GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error)
}

// Service provides consent management business logic.
// This service orchestrates between agent, service, and grant repositories
// to implement consent workflows following FR-009 through FR-020.
type Service struct {
	agentRepo       ports.AgentRepository
	providerService *thirdparty.ThirdpartyOAuth2ProviderService
	grantRepo       ports.UserGrantRepository
	psService       PermissionSetQuerier
	sessionRepo     ports.UserSessionRepository
	logger          *slog.Logger
}

// NewService creates a new ConsentService.
func NewService(
	agentRepo ports.AgentRepository,
	providerService *thirdparty.ThirdpartyOAuth2ProviderService,
	grantRepo ports.UserGrantRepository,
	sessionRepo ports.UserSessionRepository,
	psService PermissionSetQuerier,
	logger *slog.Logger,
) *Service {
	return &Service{
		agentRepo:       agentRepo,
		providerService: providerService,
		grantRepo:       grantRepo,
		sessionRepo:     sessionRepo,
		psService:       psService,
		logger:          logger,
	}
}

// ServiceScopeInfo is an OAuth2 scope with its human-readable description.
type ServiceScopeInfo struct {
	Name        string
	Description string
}

// ServiceRequirementStatus is an agent service requirement enriched with the user's session status.
type ServiceRequirementStatus struct {
	ServiceID       id.ServiceID
	DisplayName     string
	RequirementType storage.RequirementType
	RequiredScopes  []ServiceScopeInfo
	IsConnected     bool
}

// ResolvedPermissionSetEntry represents a permission set that has been resolved from the agent's
// PermissionSets array, paired with its requirement type (mandatory or optional).
type ResolvedPermissionSetEntry struct {
	PermissionSet   *storage.PermissionSet
	RequirementType storage.RequirementType
}

// AgentConsentDetail is the unified result of GetAgentConsentDetail, containing everything
// needed to render the consent screen: the agent, enriched service requirements with
// connection status, resolved permission sets, and active session service IDs.
type AgentConsentDetail struct {
	Agent                   *storage.Agent
	ServiceRequirements     []ServiceRequirementStatus
	ResolvedPermissionSets  []ResolvedPermissionSetEntry
	ActiveSessionServiceIDs []id.ServiceID
}

// GetAgentConsentDetail retrieves all information needed to render the consent screen
// for the given agent and principal. This includes:
//   - The agent entity
//   - Service requirements enriched with user connection status
//   - Resolved permission sets with requirement types
//   - Active session service IDs for the principal
//
// Returns ErrAgentNotFound if the agent does not exist.
func (s *Service) GetAgentConsentDetail(ctx context.Context, agentID id.AgentID, principal id.Principal) (*AgentConsentDetail, error) {
	agent, err := s.agentRepo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}

	// Resolve permission sets before enriching requirements so the consent response can
	// disclose every scope a require_all_scopes requirement can receive.
	resolvedPermissionSets, err := s.resolvePermissionSets(ctx, agent)
	if err != nil {
		return nil, err
	}

	// Enrich service requirements with connection status and disclosed scopes.
	requirements, err := s.resolveServiceRequirements(ctx, agent, principal, resolvedPermissionSets)
	if err != nil {
		return nil, err
	}

	// Get active session service IDs
	sessions, err := s.sessionRepo.ListActiveByPrincipal(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions for principal: %w", err)
	}
	activeSessionServiceIDs := make([]id.ServiceID, len(sessions))
	for i, session := range sessions {
		activeSessionServiceIDs[i] = session.ServiceID
	}

	return &AgentConsentDetail{
		Agent:                   agent.Copy(),
		ServiceRequirements:     requirements,
		ResolvedPermissionSets:  resolvedPermissionSets,
		ActiveSessionServiceIDs: activeSessionServiceIDs,
	}, nil
}

func (s *Service) resolveServiceRequirements(ctx context.Context, agent *storage.Agent, principal id.Principal, resolvedPermissionSets []ResolvedPermissionSetEntry) ([]ServiceRequirementStatus, error) {
	if len(agent.ServiceRequirements) == 0 {
		return []ServiceRequirementStatus{}, nil
	}

	serviceIDs := make(map[id.ServiceID]bool, len(agent.ServiceRequirements))
	for _, req := range agent.ServiceRequirements {
		serviceIDs[req.ServiceID] = true
	}

	serviceMap := make(map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity, len(serviceIDs))
	for serviceID := range serviceIDs {
		svc, err := s.providerService.Get(ctx, serviceID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				s.logger.Warn("service not found for agent requirement",
					"service_id", serviceID,
					"agent_id", agent.ID)
				continue
			}
			return nil, fmt.Errorf("loading service %s: %w", serviceID, err)
		}
		serviceMap[serviceID] = svc
	}

	requirements := make([]ServiceRequirementStatus, 0, len(agent.ServiceRequirements))
	for _, req := range agent.ServiceRequirements {
		svc, ok := serviceMap[req.ServiceID]
		if !ok {
			continue
		}

		session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, req.ServiceID)
		if err != nil {
			return nil, fmt.Errorf("checking session status for service %s: %w", req.ServiceID, err)
		}

		scopeDesc := make(map[string]string, len(svc.Scopes))
		for _, scope := range svc.Scopes {
			scopeDesc[scope.ScopeValue] = scope.Description
		}

		scopeNames := req.RequiredScopes
		if req.RequireAllScopes {
			scopeSet := make(map[string]struct{})
			for _, entry := range resolvedPermissionSets {
				for _, serviceScopes := range entry.PermissionSet.ServiceScopes {
					if serviceScopes.ServiceID != req.ServiceID {
						continue
					}
					for _, scope := range serviceScopes.Scopes {
						scopeSet[scope] = struct{}{}
					}
				}
			}
			scopeNames = make([]string, 0, len(scopeSet))
			for scope := range scopeSet {
				scopeNames = append(scopeNames, scope)
			}
			sort.Strings(scopeNames)
		}

		scopes := make([]ServiceScopeInfo, len(scopeNames))
		for i, name := range scopeNames {
			scopes[i] = ServiceScopeInfo{Name: name, Description: scopeDesc[name]}
		}

		requirements = append(requirements, ServiceRequirementStatus{
			ServiceID:       req.ServiceID,
			DisplayName:     svc.DisplayName,
			RequirementType: req.RequirementType,
			RequiredScopes:  scopes,
			IsConnected:     session != nil && !session.IsExpired(),
		})
	}

	return requirements, nil
}

func (s *Service) resolvePermissionSets(ctx context.Context, agent *storage.Agent) ([]ResolvedPermissionSetEntry, error) {
	if len(agent.PermissionSets) == 0 {
		return nil, fmt.Errorf("%w: agent must declare at least one permission set", ErrMissingMandatoryPS)
	}

	psIDs := make([]id.PermissionSetID, len(agent.PermissionSets))
	for i, entry := range agent.PermissionSets {
		psIDs[i] = entry.PermissionSetID
	}

	resolvedSets, err := s.psService.GetByIDs(ctx, psIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve permission sets for consent: %w", err)
	}

	requiredByID := make(map[id.PermissionSetID]storage.RequirementType)
	for _, entry := range agent.PermissionSets {
		requiredByID[entry.PermissionSetID] = entry.RequirementType
	}

	result := make([]ResolvedPermissionSetEntry, 0, len(resolvedSets))
	for _, ps := range resolvedSets {
		if requirementType, exists := requiredByID[ps.ID]; exists {
			result = append(result, ResolvedPermissionSetEntry{
				PermissionSet:   ps,
				RequirementType: requirementType,
			})
		}
	}

	return result, nil
}

// GrantRequest represents a request to grant or update permissions.
type GrantRequest struct {
	Principal             id.Principal
	AgentID               id.AgentID
	ValidUntil            *time.Time
	GrantedPermissionSets []storage.GrantedPermissionSetEntry
}

// GrantConsent creates or updates a user grant (upsert semantics per FR-013, FR-015).
// Validates that:
// - Agent exists (FR-020)
// - All requested scopes exist in their respective service configurations (FR-018)
// - ValidUntil is in the future if provided (FR-016)
// Returns the created/updated grant or an error.
func (s *Service) GrantConsent(ctx context.Context, req *GrantRequest) (*storage.UserGrant, error) {
	agent, err := s.agentRepo.Get(ctx, req.AgentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, fmt.Errorf("%w: agent_id=%s", ErrAgentNotFound, req.AgentID)
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("%w: agent_id=%s", ErrAgentNotFound, req.AgentID)
	}

	// Validate permission sets
	psIDs := make([]id.PermissionSetID, len(req.GrantedPermissionSets))
	for i, entry := range req.GrantedPermissionSets {
		psIDs[i] = entry.PermissionSetID
	}
	if len(psIDs) > 0 {
		if err := s.psService.ValidateIDs(ctx, psIDs); err != nil {
			return nil, fmt.Errorf("invalid permission set IDs: %w", err)
		}
	}

	// Resolve permission sets once; pass into both validators to avoid double GetByIDs.
	var resolvedPS []*storage.PermissionSet
	if len(psIDs) > 0 {
		var resolveErr error
		resolvedPS, resolveErr = s.psService.GetByIDs(ctx, psIDs)
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to resolve permission sets: %w", resolveErr)
		}
	}

	// FR-018: Validate that every IncludedServiceID is declared in the referenced
	// permission set's ServiceScopes.
	if err := s.validateScopeInclusionWithResolved(ctx, req.GrantedPermissionSets, resolvedPS); err != nil {
		return nil, err
	}

	// T047: Validate mandatory PS presence and service ID subset.
	if err := s.validateGrantStructureWithResolved(ctx, agent, req.GrantedPermissionSets, resolvedPS); err != nil {
		return nil, err
	}

	// FR-020: Validate every included service has an active OAuth2 session.
	if len(req.GrantedPermissionSets) > 0 {
		sessions, sessErr := s.sessionRepo.ListActiveByPrincipal(ctx, req.Principal)
		if sessErr != nil {
			return nil, fmt.Errorf("failed to list sessions for submission validation: %w", sessErr)
		}
		activeIDs := make([]id.ServiceID, len(sessions))
		for i, sess := range sessions {
			activeIDs[i] = sess.ServiceID
		}
		if err := s.ValidateSubmission(ctx, agent, req.GrantedPermissionSets, activeIDs); err != nil {
			return nil, err
		}
	}

	// Check for existing grant (upsert semantics)
	existingGrant, err := s.grantRepo.FindByPrincipalAndAgent(ctx, req.Principal, req.AgentID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, fmt.Errorf("failed to find existing grant: %w", err)
	}

	var grant *storage.UserGrant
	if existingGrant != nil {
		if grantMatchesRequest(existingGrant, req) {
			return existingGrant.Copy(), nil
		}

		// Update existing grant (FR-013)
		existingGrant.ValidUntil = req.ValidUntil
		existingGrant.GrantedPermissionSets = req.GrantedPermissionSets
		existingGrant.UpdatedAt = time.Now()

		if err := existingGrant.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrGrantValidation, err)
		}

		if err := s.grantRepo.Update(ctx, existingGrant); err != nil {
			return nil, fmt.Errorf("failed to update grant: %w", err)
		}
		grant = existingGrant
	} else {
		// Create new grant (FR-011)
		grant = &storage.UserGrant{
			ID:                    id.NewGrantID(),
			Principal:             req.Principal,
			AgentID:               req.AgentID,
			ValidUntil:            req.ValidUntil,
			GrantedPermissionSets: req.GrantedPermissionSets,
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		}

		if err := grant.ValidateForCreate(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrGrantValidation, err)
		}

		if err := s.grantRepo.Create(ctx, grant); err != nil {
			return nil, fmt.Errorf("failed to create grant: %w", err)
		}
	}

	return grant.Copy(), nil
}

// ValidateSubmission validates that every service in the grant's included_service_ids
// has an active session (FR-020). Returns a descriptive error listing unconnected services if not.
func (s *Service) ValidateSubmission(ctx context.Context, agent *storage.Agent, grantedPS []storage.GrantedPermissionSetEntry, activeSessionServiceIDs []id.ServiceID) error {
	if len(grantedPS) == 0 {
		return nil
	}

	// Build set of active session service IDs for O(1) lookup
	activeSet := make(map[id.ServiceID]bool, len(activeSessionServiceIDs))
	for _, svcID := range activeSessionServiceIDs {
		activeSet[svcID] = true
	}

	// Collect all unique included service IDs from the grant
	includedServices := make(map[id.ServiceID]bool)
	for _, entry := range grantedPS {
		for _, svcID := range entry.IncludedServiceIDs {
			includedServices[svcID] = true
		}
	}

	// Check each included service has an active session
	var unconnected []string
	for svcID := range includedServices {
		if !activeSet[svcID] {
			unconnected = append(unconnected, svcID.String())
		}
	}

	if len(unconnected) > 0 {
		return fmt.Errorf("%w: services without active sessions: %v", ErrUnconnectedServices, unconnected)
	}

	return nil
}

// validateGrantStructureWithResolved validates the structure of granted permission sets
// against the agent's declaration (T047). Accepts pre-resolved permission sets.
// Checks:
// 1. Every granted permission set is declared by the agent
// 2. All mandatory permission_set_ids for the agent are present
// 3. Mandatory PS entries must have len(IncludedServiceIDs) >= 1
// 4. For each PS entry, included_service_ids is a subset of PS ServiceScope ∩ agent SR
func (s *Service) validateGrantStructureWithResolved(_ context.Context, agent *storage.Agent, entries []storage.GrantedPermissionSetEntry, resolvedSets []*storage.PermissionSet) error {
	if len(agent.PermissionSets) == 0 {
		return fmt.Errorf("%w: agent must declare at least one permission set", ErrMissingMandatoryPS)
	}

	// FR-014: empty granted_permission_sets is never valid for PS-using agents
	if len(entries) == 0 {
		return fmt.Errorf("%w: at least one permission set must be granted for agents that use permission sets",
			ErrMissingMandatoryPS)
	}

	declaredPS := make(map[id.PermissionSetID]storage.RequirementType, len(agent.PermissionSets))
	for _, aps := range agent.PermissionSets {
		declaredPS[aps.PermissionSetID] = aps.RequirementType
	}

	// Build set of granted PS IDs
	grantedPSSet := make(map[id.PermissionSetID]bool, len(entries))
	for _, entry := range entries {
		if _, ok := declaredPS[entry.PermissionSetID]; !ok {
			return fmt.Errorf("%w: permission set %s is not declared by the agent", ErrGrantValidation, entry.PermissionSetID)
		}
		grantedPSSet[entry.PermissionSetID] = true
	}

	// Check all mandatory PSes are present
	var missingMandatory []string
	for _, aps := range agent.PermissionSets {
		if aps.RequirementType == storage.RequirementTypeMandatory {
			if !grantedPSSet[aps.PermissionSetID] {
				missingMandatory = append(missingMandatory, aps.PermissionSetID.String())
			}
		}
	}
	if len(missingMandatory) > 0 {
		return fmt.Errorf("%w: %v", ErrMissingMandatoryPS, missingMandatory)
	}

	// Mandatory PS entries must include at least one service.
	// An empty IncludedServiceIDs on a mandatory PS is a no-op grant and must be rejected.
	for _, entry := range entries {
		if declaredPS[entry.PermissionSetID] == storage.RequirementTypeMandatory && len(entry.IncludedServiceIDs) == 0 {
			return fmt.Errorf("%w: mandatory permission set %s must include at least one service",
				ErrInvalidServiceInclusion, entry.PermissionSetID)
		}
	}

	// Build indexes: SR mandatory services and PS-level effective mandatory services
	agentSRServiceIDs := make(map[id.ServiceID]bool, len(agent.ServiceRequirements))
	srMandatoryServiceIDs := make(map[id.ServiceID]bool, len(agent.ServiceRequirements))
	for _, sr := range agent.ServiceRequirements {
		agentSRServiceIDs[sr.ServiceID] = true
		if sr.RequirementType == storage.RequirementTypeMandatory {
			srMandatoryServiceIDs[sr.ServiceID] = true
		}
	}

	psValidServices := make(map[id.PermissionSetID]map[id.ServiceID]bool, len(resolvedSets))
	// psMandatoryServices: services that must always be in included_service_ids for a selected PS
	// (SR mandatory OR ServiceScope.requirement_type=mandatory, intersected with agent SR)
	psMandatoryServices := make(map[id.PermissionSetID][]id.ServiceID, len(resolvedSets))
	for _, ps := range resolvedSets {
		validServices := make(map[id.ServiceID]bool)
		for _, ss := range ps.ServiceScopes {
			// When the agent has no service_requirements, all PS ServiceScopes are valid.
			// When the agent has SR, only services in the intersection (PS ∩ SR) are valid.
			if len(agentSRServiceIDs) == 0 || agentSRServiceIDs[ss.ServiceID] {
				validServices[ss.ServiceID] = true
				if srMandatoryServiceIDs[ss.ServiceID] || ss.RequirementType == storage.RequirementTypeMandatory {
					psMandatoryServices[ps.ID] = append(psMandatoryServices[ps.ID], ss.ServiceID)
				}
			}
		}
		psValidServices[ps.ID] = validServices
	}

	// Validate each grant entry's included_service_ids
	for _, entry := range entries {
		validServices, ok := psValidServices[entry.PermissionSetID]
		if !ok {
			continue // already rejected by validateScopeInclusionWithResolved
		}

		for _, svcID := range entry.IncludedServiceIDs {
			if !validServices[svcID] {
				return fmt.Errorf("%w: service %s is not in the valid set for permission set %s (must be in PS ServiceScope ∩ agent ServiceRequirements)",
					ErrInvalidServiceInclusion, svcID, entry.PermissionSetID)
			}
		}

		// Enforce effective-mandatory services are always included (FR-014)
		includedSet := make(map[id.ServiceID]bool, len(entry.IncludedServiceIDs))
		for _, svcID := range entry.IncludedServiceIDs {
			includedSet[svcID] = true
		}
		for _, mandatorySvcID := range psMandatoryServices[entry.PermissionSetID] {
			if !includedSet[mandatorySvcID] {
				return fmt.Errorf("%w: effectively mandatory service %s must be included in permission set %s",
					ErrInvalidServiceInclusion, mandatorySvcID, entry.PermissionSetID)
			}
		}
	}

	return nil
}

func grantMatchesRequest(grant *storage.UserGrant, req *GrantRequest) bool {
	if grant == nil || req == nil {
		return false
	}

	return validUntilMatches(grant.ValidUntil, req.ValidUntil) &&
		permissionSetsMatch(grant.GrantedPermissionSets, req.GrantedPermissionSets)
}

func validUntilMatches(left, right *time.Time) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.Equal(*right)
	}
}

func permissionSetsMatch(left, right []storage.GrantedPermissionSetEntry) bool {
	return slices.EqualFunc(left, right, func(l, r storage.GrantedPermissionSetEntry) bool {
		return l.PermissionSetID == r.PermissionSetID &&
			slices.Equal(l.IncludedServiceIDs, r.IncludedServiceIDs)
	})
}

// RevokeConsent deletes a user grant (FR-014).
// Idempotent: returns nil if the grant doesn't exist (absence is not an error).
// Used by the POST /grants path with empty tokens.
func (s *Service) RevokeConsent(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	if err := s.grantRepo.DeleteByPrincipalAndAgentID(ctx, principal, agentID); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil // idempotent: absence is not an error
		}
		return fmt.Errorf("failed to revoke consent: %w", err)
	}
	return nil
}

// RevokeConsentForPrincipal revokes the authenticated user's grant for the given agent (FR-014).
// This is the user-facing revocation entry point for DELETE /api/consent/agent/{agent-id}/grants.
//
// Unlike RevokeConsent, this method:
// - Is NOT idempotent: absence of grant returns ErrGrantNotFound (handler maps to 404)
// - Emits a structured audit log on success with action, principal, agent_id, and grant_id
func (s *Service) RevokeConsentForPrincipal(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	// Phase 1: look up the grant to capture the ID for the audit log.
	grant, err := s.grantRepo.FindByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("%w", ErrGrantNotFound)
		}
		return fmt.Errorf("failed to find grant: %w", err)
	}

	// Phase 2: delete the grant.
	if err := s.grantRepo.DeleteByPrincipalAndAgentID(ctx, principal, agentID); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			// Concurrent revocation raced us — treat as not found.
			return fmt.Errorf("%w", ErrGrantNotFound)
		}
		return fmt.Errorf("failed to revoke consent: %w", err)
	}

	s.logger.Info("grant revoked",
		"action", "grant_revoked",
		"principal", principal,
		"agent_id", agentID,
		"grant_id", grant.ID)

	return nil
}

// GetActiveGrants retrieves all active grants for a principal and agent.
// Filters expired grants per FR-019.
// Returns empty slice if no active grants exist (not an error per FR-012).
func (s *Service) GetActiveGrants(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	grants, err := s.grantRepo.ListByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list grants: %w", err)
	}

	// Filter to only active grants (FR-019)
	activeGrants := make([]*storage.UserGrant, 0, len(grants))
	for _, grant := range grants {
		if grant.IsActive() {
			activeGrants = append(activeGrants, grant.Copy())
		}
	}

	return activeGrants, nil
}

// VerifyAgentAccess verifies that a user has granted an agent access.
// This method checks for grant existence, expiration, and revocation status.
// Returns the active grant if valid, or an error if missing, expired, or revoked.
//
// Error handling:
// - ErrAgentAccessDenied: User has not granted the agent any access
// - ErrGrantExpired: User grant has expired
// - Other errors: Repository or system errors
//
// This method is used by token exchange flows to verify authorization before
// issuing delegated tokens. Per Constitution Principle I (Security-First),
// fails closed with access denied for any ambiguous state.
func (s *Service) VerifyAgentAccess(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	// Look up grant by principal and agent
	grant, err := s.grantRepo.FindByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		// Check if it's a NotFound error
		if errors.Is(err, ports.ErrNotFound) {
			return nil, fmt.Errorf("%w: user has not granted permission for agent (principal: %s, agent: %s)",
				ErrAgentAccessDenied, principal, agentID)
		}
		return nil, fmt.Errorf("failed to verify user grant: %w", err)
	}

	// Defensive check: ensure grant is not nil
	if grant == nil {
		return nil, fmt.Errorf("%w: user has not granted permission for agent (principal: %s, agent: %s)",
			ErrAgentAccessDenied, principal, agentID)
	}

	// Check grant is active (not expired)
	if !grant.IsActive() {
		return nil, fmt.Errorf("%w: user grant expired at %s (principal: %s, agent: %s)",
			ErrGrantExpired, grant.ValidUntil.Format(time.RFC3339), principal, agentID)
	}

	// Hard deletion is the revocation mechanism: a missing grant (ports.ErrNotFound) is treated
	// as access denied (fail closed). No revoked status field is needed.

	// Return copy to prevent external mutation
	return grant.Copy(), nil
}

// validateScopeInclusionWithResolved checks that every IncludedServiceID in each
// grant entry is present in the referenced permission set's ServiceScopes (FR-018).
// Accepts pre-resolved permission sets to avoid a redundant GetByIDs call.
// Returns ErrInvalidScopes if any included service is not declared in the PS.
func (s *Service) validateScopeInclusionWithResolved(_ context.Context, entries []storage.GrantedPermissionSetEntry, resolvedSets []*storage.PermissionSet) error {
	if len(entries) == 0 {
		return nil
	}

	psServiceIDs := make(map[id.PermissionSetID]map[id.ServiceID]bool, len(resolvedSets))
	for _, ps := range resolvedSets {
		svcSet := make(map[id.ServiceID]bool, len(ps.ServiceScopes))
		for _, ss := range ps.ServiceScopes {
			svcSet[ss.ServiceID] = true
		}
		psServiceIDs[ps.ID] = svcSet
	}

	for _, entry := range entries {
		validServices, ok := psServiceIDs[entry.PermissionSetID]
		if !ok {
			// PS was not resolved — it may have been deleted concurrently between
			// the earlier existence check and this GetByIDs call. Reject to avoid
			// persisting a grant that references a non-existent permission set.
			return fmt.Errorf("%w: permission set %s no longer exists",
				ErrInvalidServiceInclusion, entry.PermissionSetID)
		}
		for _, svcID := range entry.IncludedServiceIDs {
			if !validServices[svcID] {
				return fmt.Errorf("%w: service %s is not declared in permission set %s",
					ErrInvalidScopes, svcID, entry.PermissionSetID)
			}
		}
	}

	return nil
}

// AgentDelegation represents aggregated information about grants for a specific agent.
// This is used for the consent management UI to display active delegations.
type AgentDelegation struct {
	AgentID          id.AgentID `json:"agentId"`
	DisplayName      string     `json:"displayName"`
	LogoURL          *string    `json:"logoUrl,omitempty"`
	ActiveGrantCount int        `json:"activeGrantCount"`
	LastModifiedAt   time.Time  `json:"lastModifiedAt"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

func sortAgentDelegations(delegations []AgentDelegation) {
	sort.Slice(delegations, func(i, j int) bool {
		if delegations[i].DisplayName != delegations[j].DisplayName {
			return delegations[i].DisplayName < delegations[j].DisplayName
		}
		return delegations[i].AgentID.String() < delegations[j].AgentID.String()
	})
}

// GetUserGrants retrieves all grants for a specific principal and agent.
// This is used for User Story 2 to display what permissions the user has already granted to an agent.
// Returns empty slice if no grants exist (not an error).
// Returns ErrAgentNotFound if the agent doesn't exist.
func (s *Service) GetUserGrants(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	// Verify agent exists first
	agent, err := s.agentRepo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}

	// Fetch grants for this principal and agent
	grants, err := s.grantRepo.ListByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list grants: %w", err)
	}

	// Return copies to prevent external mutation
	result := make([]*storage.UserGrant, len(grants))
	for i, grant := range grants {
		result[i] = grant.Copy()
	}

	return result, nil
}

// GetAgentDelegations retrieves all agent delegations for a principal.
// Groups grants by agent_id and returns summary information for each agent.
// Returns empty slice if no grants exist (not an error).
func (s *Service) GetAgentDelegations(ctx context.Context, principal id.Principal) ([]AgentDelegation, error) {
	// Fetch all active grants for this principal
	grants, err := s.grantRepo.ListByPrincipal(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("failed to list grants: %w", err)
	}

	// Group grants by agent_id
	agentMap := make(map[id.AgentID]*AgentDelegation)

	for _, grant := range grants {
		delegation, exists := agentMap[grant.AgentID]
		if !exists {
			// Fetch agent information
			agent, err := s.agentRepo.Get(ctx, grant.AgentID)
			if err != nil {
				// If agent not found, skip this grant (defensive: should not happen)
				if errors.Is(err, ports.ErrNotFound) {
					continue
				}
				return nil, fmt.Errorf("failed to get agent %s: %w", grant.AgentID, err)
			}

			delegation = &AgentDelegation{
				AgentID:          grant.AgentID,
				DisplayName:      agent.DisplayName,
				LogoURL:          nil, // TODO: add logo_url field to Agent entity
				ActiveGrantCount: 0,
				LastModifiedAt:   grant.UpdatedAt,
				ExpiresAt:        grant.ValidUntil,
			}
			agentMap[grant.AgentID] = delegation
		}

		// Update delegation stats
		delegation.ActiveGrantCount++
		if grant.UpdatedAt.After(delegation.LastModifiedAt) {
			delegation.LastModifiedAt = grant.UpdatedAt
		}
		// Update ExpiresAt to the earliest expiration if multiple grants exist
		if grant.ValidUntil != nil {
			if delegation.ExpiresAt == nil || grant.ValidUntil.Before(*delegation.ExpiresAt) {
				delegation.ExpiresAt = grant.ValidUntil
			}
		}
	}

	// Convert map to slice
	delegations := make([]AgentDelegation, 0, len(agentMap))
	for _, delegation := range agentMap {
		delegations = append(delegations, *delegation)
	}

	sortAgentDelegations(delegations)

	return delegations, nil
}
