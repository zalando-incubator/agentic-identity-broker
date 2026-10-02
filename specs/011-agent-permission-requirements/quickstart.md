# Quickstart: Agent Permission Requirements Implementation

**Feature**: 011-agent-permission-requirements  
**Date**: 2026-01-08  
**Audience**: Developers implementing this feature

## Overview

This guide provides step-by-step instructions for implementing agent permission requirements. Follow the order presented to maintain consistency with the project's hexagonal architecture and constitution principles.

## Prerequisites

Before starting implementation:

1. ✅ Read [spec.md](spec.md) - Understand user stories and acceptance criteria
2. ✅ Read [research.md](research.md) - Understand technical decisions
3. ✅ Read [data-model.md](data-model.md) - Understand entities and relationships
4. ✅ Read [contracts/openapi-changes.md](contracts/openapi-changes.md) - Understand API changes
5. ✅ Review Constitution Principle XIII - E2E tests MUST be written BEFORE implementation
6. ✅ Review ADR 004 - Storage layer patterns

## Implementation Order

### Phase 1: Database Migration

**File**: `migrations/005_add_agent_service_requirements.up.sql`

```sql
-- Add service_requirements JSONB column to agents table
ALTER TABLE agents 
  ADD COLUMN service_requirements JSONB DEFAULT NULL;

-- Add GIN index for efficient JSONB queries
CREATE INDEX idx_agents_service_requirements 
  ON agents USING GIN (service_requirements);

-- Add comment explaining backward compatibility
COMMENT ON COLUMN agents.service_requirements IS 
  'JSONB array of ServiceRequirement objects. NULL = no requirements (backward compatible).';
```

**File**: `migrations/005_add_agent_service_requirements.down.sql`

```sql
-- Remove index first
DROP INDEX IF EXISTS idx_agents_service_requirements;

-- Remove column (data loss warning: existing service requirements will be deleted)
ALTER TABLE agents DROP COLUMN IF EXISTS service_requirements;
```

**Testing**:
- Write integration test in `internal/adapters/storage/postgres/migration_test.go`
- Verify migration applies cleanly
- Verify rollback works (can apply/rollback repeatedly without data loss)
- Use testcontainers for real PostgreSQL

### Phase 2: Domain Layer - Value Objects & Enums

**File**: `internal/domain/storage/requirement_type.go` (NEW)

```go
package storage

import "fmt"

// RequirementType represents whether a service is mandatory or optional
type RequirementType string

const (
    RequirementTypeMandatory RequirementType = "mandatory"
    RequirementTypeOptional  RequirementType = "optional"
)

// Valid returns true if the requirement type is valid
func (r RequirementType) Valid() bool {
    return r == RequirementTypeMandatory || r == RequirementTypeOptional
}

// Validate returns an error if the requirement type is invalid
func (r RequirementType) Validate() error {
    if !r.Valid() {
        return fmt.Errorf("invalid requirement type: %s (must be 'mandatory' or 'optional')", r)
    }
    return nil
}
```

**File**: `internal/domain/storage/service_requirement.go` (NEW)

```go
package storage

import (
    "fmt"
    "github.com/google/uuid"
)

// ServiceRequirement represents a third-party service that an agent requires
type ServiceRequirement struct {
    ServiceID       uuid.UUID       `json:"service_id" db:"service_id"`
    RequirementType RequirementType `json:"requirement_type" db:"requirement_type"`
    RequiredScopes  []string        `json:"required_scopes" db:"required_scopes"`
}

// Validate performs structure validation (not referential integrity)
func (sr *ServiceRequirement) Validate() error {
    // Validate ServiceID
    if sr.ServiceID == uuid.Nil {
        return fmt.Errorf("service_id is required")
    }
    
    // Validate RequirementType
    if err := sr.RequirementType.Validate(); err != nil {
        return err
    }
    
    // Validate RequiredScopes
    if len(sr.RequiredScopes) == 0 {
        return fmt.Errorf("required_scopes cannot be empty")
    }
    
    return nil
}
```

**Testing**:
- Write table-driven tests in `service_requirement_test.go`
- Test valid/invalid UUIDs, requirement types, empty scopes

### Phase 3: Domain Layer - Extend Agent Entity

**File**: `internal/domain/storage/agent.go` (EXTEND existing)

```go
// Agent represents an AI agent or autonomous system
type Agent struct {
    ID                  uuid.UUID            `json:"id" db:"id"`
    ClientID            string               `json:"client_id" db:"client_id"`
    ClientSecret        string               `json:"client_secret" db:"client_secret"`
    Name                string               `json:"name" db:"name"`
    Description         string               `json:"description" db:"description"`
    RedirectURIs        []string             `json:"redirect_uris" db:"redirect_uris"`
    ServiceRequirements []ServiceRequirement `json:"service_requirements,omitempty" db:"service_requirements"` // NEW
    CreatedAt           time.Time            `json:"created_at" db:"created_at"`
    UpdatedAt           time.Time            `json:"updated_at" db:"updated_at"`
}

// Validate validates the agent (extend existing method)
func (a *Agent) Validate() error {
    // ... existing validation ...
    
    // Validate service requirements (NEW)
    if err := a.ValidateServiceRequirements(); err != nil {
        return err
    }
    
    return nil
}

// ValidateServiceRequirements validates service requirements structure and duplicates
func (a *Agent) ValidateServiceRequirements() error {
    if len(a.ServiceRequirements) == 0 {
        return nil // Empty is valid
    }
    
    // Check for duplicate service_id (FR-003a)
    seen := make(map[uuid.UUID]int)
    for i, req := range a.ServiceRequirements {
        if err := req.Validate(); err != nil {
            return fmt.Errorf("service requirement at index %d: %w", i, err)
        }
        
        if prevIdx, exists := seen[req.ServiceID]; exists {
            return fmt.Errorf("duplicate service_id %s at indices %d and %d", req.ServiceID, prevIdx, i)
        }
        seen[req.ServiceID] = i
    }
    
    return nil
}
```

**Testing**:
- Extend `agent_test.go` with service requirement validation tests
- Test duplicate service_id detection
- Test invalid requirement structures

### Phase 4: Storage Layer - Extend Repository Adapters

**File**: `internal/adapters/storage/memory/agent_repository.go` (EXTEND)

```go
// Create stores a new agent (extend existing method to handle service_requirements)
func (r *AgentRepository) Create(ctx context.Context, agent *storage.Agent) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    // ... existing logic ...
    
    // Copy service requirements (no special handling needed for in-memory)
    agentCopy.ServiceRequirements = append([]storage.ServiceRequirement{}, agent.ServiceRequirements...)
    
    r.agents[agent.ID] = &agentCopy
    return nil
}
```

**File**: `internal/adapters/storage/postgres/agent_repository.go` (EXTEND)

```go
// Create stores a new agent (extend existing method to handle JSONB)
func (r *AgentRepository) Create(ctx context.Context, agent *storage.Agent) error {
    query := `
        INSERT INTO agents (
            id, client_id, client_secret, name, description, 
            redirect_uris, service_requirements, created_at, updated_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9
        )`
    
    // Serialize service_requirements to JSONB
    var serviceReqsJSON []byte
    var err error
    if len(agent.ServiceRequirements) > 0 {
        serviceReqsJSON, err = json.Marshal(agent.ServiceRequirements)
        if err != nil {
            return fmt.Errorf("failed to marshal service_requirements: %w", err)
        }
    }
    // If nil, serviceReqsJSON remains nil (NULL in database)
    
    _, err = r.db.ExecContext(
        ctx, query,
        agent.ID, agent.ClientID, agent.ClientSecret,
        agent.Name, agent.Description,
        pq.Array(agent.RedirectURIs), serviceReqsJSON,
        agent.CreatedAt, agent.UpdatedAt,
    )
    
    if err != nil {
        return wrapStorageError(err)
    }
    return nil
}

// GetByID retrieves an agent by ID (extend to deserialize JSONB)
func (r *AgentRepository) GetByID(ctx context.Context, id uuid.UUID) (*storage.Agent, error) {
    query := `
        SELECT id, client_id, client_secret, name, description, 
               redirect_uris, service_requirements, created_at, updated_at
        FROM agents WHERE id = $1`
    
    var agent storage.Agent
    var redirectURIs pq.StringArray
    var serviceReqsJSON []byte
    
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &agent.ID, &agent.ClientID, &agent.ClientSecret,
        &agent.Name, &agent.Description,
        &redirectURIs, &serviceReqsJSON,
        &agent.CreatedAt, &agent.UpdatedAt,
    )
    
    if err != nil {
        return nil, wrapStorageError(err)
    }
    
    agent.RedirectURIs = redirectURIs
    
    // Deserialize service_requirements from JSONB
    if len(serviceReqsJSON) > 0 {
        if err := json.Unmarshal(serviceReqsJSON, &agent.ServiceRequirements); err != nil {
            return nil, fmt.Errorf("failed to unmarshal service_requirements: %w", err)
        }
    }
    
    return &agent, nil
}
```

**Testing**:
- Write integration tests in `agent_repository_test.go`
- Test JSONB serialization/deserialization
- Test NULL handling (backward compatibility)
- Use testcontainers for real PostgreSQL

### Phase 5: Application Layer - Admin Handler Validation

**File**: `internal/adapters/http/handlers/admin/agent_handler.go` (EXTEND)

```go
// CreateAgent handles POST /api/agents (extend existing method)
func (h *AgentHandler) CreateAgent(w http.ResponseWriter, r *http.Request) {
    // ... existing parsing and validation ...
    
    // Validate service requirements referential integrity (NEW)
    if err := h.validateServiceRequirements(r.Context(), agent.ServiceRequirements); err != nil {
        h.logger.Error("service requirement validation failed", "error", err)
        writeErrorResponse(w, http.StatusBadRequest, "INVALID_SERVICE_REQUIREMENTS", err.Error(), nil)
        return
    }
    
    // ... continue with existing logic ...
}

// validateServiceRequirements checks referential integrity
func (h *AgentHandler) validateServiceRequirements(ctx context.Context, requirements []storage.ServiceRequirement) error {
    for i, req := range requirements {
        // Check service exists (FR-003)
        service, err := h.thirdPartyServiceRepo.GetByID(ctx, req.ServiceID)
        if err != nil {
            return fmt.Errorf("requirement %d: service_id %s does not exist", i, req.ServiceID)
        }
        
        // Check scopes exist in service (FR-004)
        validScopes := make(map[string]bool)
        for _, scope := range service.Scopes {
            validScopes[scope.Name] = true
        }
        
        var invalidScopes []string
        for _, reqScope := range req.RequiredScopes {
            if !validScopes[reqScope] { // Case-sensitive per SR-006
                invalidScopes = append(invalidScopes, reqScope)
            }
        }
        
        if len(invalidScopes) > 0 {
            return fmt.Errorf("requirement %d: service %s does not have scopes: %v (available: %v)",
                i, service.Name, invalidScopes, service.GetScopeNames())
        }
    }
    
    return nil
}
```

**Testing**:
- Write unit tests for `validateServiceRequirements`
- Test non-existent service_id
- Test invalid scopes
- Use table-driven tests

### Phase 6: Application Layer - Authorization Endpoint Enhancement

**File**: `internal/adapters/http/handlers/enduser/oauth2_handler.go` (EXTEND)

```go
// Authorize handles GET /oauth2/authorize (extend existing method)
func (h *OAuth2Handler) Authorize(w http.ResponseWriter, r *http.Request) {
    // ... existing client_id validation ...
    
    // Load agent
    agent, err := h.agentRepo.GetByClientID(ctx, clientID)
    if err != nil {
        http.Error(w, "Invalid client", http.StatusBadRequest)
        return
    }
    
    // Check user consent exists (existing logic)
    grant, err := h.grantRepo.GetByUserAndAgent(ctx, userID, agent.ID)
    if err != nil || grant.IsExpired() {
        redirectToConsent(w, r, agent.ID)
        return
    }
    
    // Check mandatory service requirements (NEW)
    if !h.validateMandatoryRequirements(ctx, userID, agent.ServiceRequirements) {
        h.logger.Info("mandatory requirements not satisfied", 
            "agent_id", agent.ID, 
            "user_id", userID)
        redirectToConsent(w, r, agent.ID)
        return
    }
    
    // Proxy to upstream OAuth2 server (existing logic)
    h.proxyToUpstream(w, r)
}

// validateMandatoryRequirements checks if user has sessions for all mandatory services
func (h *OAuth2Handler) validateMandatoryRequirements(ctx context.Context, userID uuid.UUID, requirements []storage.ServiceRequirement) bool {
    if len(requirements) == 0 {
        return true // No requirements = pass (FR-015)
    }
    
    for _, req := range requirements {
        // Only check mandatory requirements (FR-014)
        if req.RequirementType != storage.RequirementTypeMandatory {
            continue
        }
        
        // Check session exists and not expired (FR-010)
        session, err := h.sessionRepo.GetByUserAndService(ctx, userID, req.ServiceID)
        if err != nil || session.IsExpired() {
            h.logger.Debug("mandatory service session missing or expired",
                "service_id", req.ServiceID,
                "user_id", userID)
            return false
        }
        
        // Check session has required scopes (FR-011)
        if !hasRequiredScopes(session.Scopes, req.RequiredScopes) {
            h.logger.Debug("mandatory service session lacks required scopes",
                "service_id", req.ServiceID,
                "required", req.RequiredScopes,
                "actual", session.Scopes)
            return false
        }
    }
    
    return true
}

// hasRequiredScopes checks if sessionScopes contains all requiredScopes (superset check)
func hasRequiredScopes(sessionScopes, requiredScopes []string) bool {
    scopeSet := make(map[string]bool)
    for _, s := range sessionScopes {
        scopeSet[s] = true // Case-sensitive per SR-006
    }
    
    for _, required := range requiredScopes {
        if !scopeSet[required] {
            return false
        }
    }
    return true
}
```

**Testing**:
- Write unit tests for `validateMandatoryRequirements` and `hasRequiredScopes`
- Test missing sessions, expired sessions, insufficient scopes
- Test optional requirements don't block

### Phase 7: Application Layer - Consent Endpoints

**File**: `internal/adapters/http/handlers/enduser/consent_handler.go` (EXTEND)

```go
// GetAgentForConsent handles GET /api/consent/agent/{agent-id} (extend existing)
func (h *ConsentHandler) GetAgentForConsent(w http.ResponseWriter, r *http.Request) {
    // ... existing agent loading ...
    
    // Build service requirements with user session status (NEW)
    serviceReqsForUser := h.buildServiceRequirementsForUser(r.Context(), userID, agent.ServiceRequirements)
    
    response := map[string]interface{}{
        "agent": map[string]interface{}{
            "id":          agent.ID,
            "name":        agent.Name,
            "description": agent.Description,
        },
        "service_requirements": serviceReqsForUser, // NEW
    }
    
    writeJSONResponse(w, http.StatusOK, response)
}

// buildServiceRequirementsForUser enriches requirements with user session status
func (h *ConsentHandler) buildServiceRequirementsForUser(ctx context.Context, userID uuid.UUID, requirements []storage.ServiceRequirement) []map[string]interface{} {
    result := make([]map[string]interface{}, 0, len(requirements))
    
    for _, req := range requirements {
        // Load service metadata
        service, err := h.thirdPartyServiceRepo.GetByID(ctx, req.ServiceID)
        if err != nil {
            h.logger.Error("failed to load service", "service_id", req.ServiceID, "error", err)
            continue
        }
        
        // Check user session status
        session, err := h.sessionRepo.GetByUserAndService(ctx, userID, req.ServiceID)
        hasActiveSession := err == nil && !session.IsExpired()
        
        // Build scopes with descriptions
        scopes := make([]map[string]interface{}, 0, len(req.RequiredScopes))
        for _, scopeName := range req.RequiredScopes {
            scopeData := map[string]interface{}{
                "name": scopeName,
            }
            // Add description if available
            if scopeDesc := service.GetScopeDescription(scopeName); scopeDesc != "" {
                scopeData["description"] = scopeDesc
            }
            scopes = append(scopes, scopeData)
        }
        
        result = append(result, map[string]interface{}{
            "service_id":               req.ServiceID,
            "service_name":             service.Name,
            "requirement_type":         req.RequirementType,
            "required_scopes":          scopes,
            "user_has_active_session":  hasActiveSession,
        })
    }
    
    return result
}

// ApproveConsent handles POST /api/consent/agent/{agent-id}/approve (extend existing)
func (h *ConsentHandler) ApproveConsent(w http.ResponseWriter, r *http.Request) {
    // ... existing consent approval logic ...
    
    // Handle redirect_uri parameter (NEW)
    redirectURI := r.URL.Query().Get("redirect_uri")
    if redirectURI != "" {
        // Validate same-origin (SR-001, SR-002)
        if err := validateRedirectURI(redirectURI, r); err != nil {
            h.logger.Error("invalid redirect_uri", "uri", redirectURI, "error", err)
            writeErrorResponse(w, http.StatusBadRequest, "INVALID_REDIRECT_URI", 
                "redirect_uri must be same-origin or relative path", 
                map[string]string{"provided_uri": redirectURI})
            return
        }
        
        // Issue redirect (FR-025)
        http.Redirect(w, r, redirectURI, http.StatusSeeOther)
        return
    }
    
    // No redirect_uri: return success message (FR-028)
    writeJSONResponse(w, http.StatusOK, map[string]interface{}{
        "message":  "Consent approved",
        "grant_id": grant.ID,
    })
}

// validateRedirectURI checks same-origin or relative path
func validateRedirectURI(redirectURI string, r *http.Request) error {
    u, err := url.Parse(redirectURI)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }
    
    // Allow relative URLs (no scheme/host)
    if u.Scheme == "" && u.Host == "" {
        return nil
    }
    
    // Same-origin check
    requestOrigin := fmt.Sprintf("%s://%s", r.URL.Scheme, r.Host)
    redirectOrigin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
    
    if requestOrigin != redirectOrigin {
        return fmt.Errorf("external redirect not allowed: %s", redirectURI)
    }
    
    return nil
}
```

**Testing**:
- Write unit tests for `buildServiceRequirementsForUser` and `validateRedirectURI`
- Test session status enrichment
- Test same-origin validation (allow relative, reject external)

### Phase 8: Frontend - Service Requirements Components

> **Historical visual scope**: Styling and token examples below record this feature's original design, not a current constitutional aesthetic mandate. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

**File**: `web/src/components/consent/ServiceRequirementCard.tsx` (NEW)

```tsx
import { Card, CardHeader, CardBody, Badge, Button } from '@/design-system/components';

interface ServiceRequirementCardProps {
  serviceName: string;
  requirementType: 'mandatory' | 'optional';
  requiredScopes: Array<{ name: string; description?: string }>;
  hasActiveSession: boolean;
  onLogin: () => void;
  onDisconnect?: () => void;
}

export function ServiceRequirementCard({
  serviceName,
  requirementType,
  requiredScopes,
  hasActiveSession,
  onLogin,
  onDisconnect,
}: ServiceRequirementCardProps) {
  return (
    <Card variant="elevated">
      <CardHeader>
        <h3 className="text-lg font-semibold">{serviceName}</h3>
        <Badge variant={requirementType === 'mandatory' ? 'primary' : 'secondary'}>
          {requirementType === 'mandatory' ? 'Required' : 'Optional'}
        </Badge>
      </CardHeader>
      <CardBody>
        <div className="space-y-2">
          <h4 className="text-sm font-medium text-neutral-700">Required Scopes:</h4>
          <ul className="space-y-1">
            {requiredScopes.map((scope) => (
              <li key={scope.name} className="text-sm">
                <code className="text-trust-deep">{scope.name}</code>
                {scope.description && (
                  <p className="text-xs text-neutral-600 mt-0.5">{scope.description}</p>
                )}
              </li>
            ))}
          </ul>
        </div>
        
        <div className="mt-4">
          {hasActiveSession ? (
            <div className="flex items-center justify-between">
              <span className="text-sm text-success-primary">✓ Active Session</span>
              {onDisconnect && (
                <Button variant="secondary" size="sm" onClick={onDisconnect}>
                  Disconnect
                </Button>
              )}
            </div>
          ) : (
            <Button variant="primary" onClick={onLogin}>
              Login to {serviceName}
            </Button>
          )}
        </div>
      </CardBody>
    </Card>
  );
}
```

**File**: `web/src/pages/ConsentPage.tsx` (EXTEND)

```tsx
// Add to ConsentPage component

const [serviceRequirements, setServiceRequirements] = useState([]);

// Fetch service requirements
useEffect(() => {
  const fetchServiceRequirements = async () => {
    const response = await api.get(`/api/consent/agent/${agentId}`);
    setServiceRequirements(response.data.service_requirements || []);
  };
  fetchServiceRequirements();
}, [agentId]);

// Handle login to third-party service
const handleLogin = (serviceId: string) => {
  const currentUrl = encodeURIComponent(window.location.href);
  window.location.href = `/api/third-party/${serviceId}/oauth2/authorize?redirect_uri=${currentUrl}`;
};

// Render service requirements (grouped by mandatory/optional)
const mandatoryServices = serviceRequirements.filter(req => req.requirement_type === 'mandatory');
const optionalServices = serviceRequirements.filter(req => req.requirement_type === 'optional');

return (
  <>
    {/* Mandatory services first (FR-018) */}
    {mandatoryServices.length > 0 && (
      <section>
        <h2>Required Services</h2>
        {mandatoryServices.map(req => (
          <ServiceRequirementCard
            key={req.service_id}
            serviceName={req.service_name}
            requirementType={req.requirement_type}
            requiredScopes={req.required_scopes}
            hasActiveSession={req.user_has_active_session}
            onLogin={() => handleLogin(req.service_id)}
          />
        ))}
      </section>
    )}
    
    {/* Optional services after mandatory (FR-018) */}
    {optionalServices.length > 0 && (
      <section>
        <h2>Optional Services</h2>
        {optionalServices.map(req => (
          <ServiceRequirementCard
            key={req.service_id}
            serviceName={req.service_name}
            requirementType={req.requirement_type}
            requiredScopes={req.required_scopes}
            hasActiveSession={req.user_has_active_session}
            onLogin={() => handleLogin(req.service_id)}
          />
        ))}
      </section>
    )}
    
    {/* Disable approve button if mandatory requirements not met (FR-024) */}
    <Button
      onClick={handleApprove}
      disabled={mandatoryServices.some(req => !req.user_has_active_session)}
    >
      Approve
    </Button>
  </>
);
```

**Testing**:
- Write component tests in `ServiceRequirementCard.test.tsx`
- Test mandatory vs optional styling
- Test session status display
- Test button states

### Phase 9: E2E Acceptance Tests (MANDATORY - Before Implementation)

**File**: `tests/e2e/agent_permission_requirements_test.go` (NEW)

```go
package e2e_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Agent Permission Requirements", func() {
    Describe("User Story 1: Administrator Configures Agent Service Requirements", func() {
        Context("when creating a new agent", func() {
            It("should accept optional service_requirements array in POST /api/agents", func() {
                // Scenario 1 from spec.md
                // ... test implementation ...
            })
            
            It("should require service_id, requirement_type, and required_scopes in each requirement", func() {
                // Scenario 2 from spec.md
                // ... test implementation ...
            })
            
            // ... continue for all 37 scenarios ...
        })
    })
    
    Describe("User Story 2: Authorization Endpoint Validates Service Requirements", func() {
        // ... implement all scenarios from User Story 2 ...
    })
    
    // ... continue for all user stories ...
})
```

**CRITICAL**: Write all 37 E2E test scenarios BEFORE implementing backend/frontend logic. Tests must FAIL initially (red phase).

## Testing Checklist

- [ ] **Phase 9a: Write E2E Tests** (MANDATORY FIRST)
  - [ ] All 37 scenarios from spec.md mapped to `It()` blocks
  - [ ] Run `ginkgo -v ./tests/e2e/agent_permission_requirements_test.go`
  - [ ] Verify all tests FAIL (red phase)
- [ ] **Phase 1-8: Implementation**
  - [ ] Database migration tested (apply/rollback)
  - [ ] Domain validation unit tests pass
  - [ ] Storage layer integration tests pass
  - [ ] Admin handler validation unit tests pass
  - [ ] Authorization endpoint unit tests pass
  - [ ] Consent handler unit tests pass
  - [ ] Frontend component tests pass
- [ ] **Phase 9b: E2E Green Phase**
  - [ ] Run E2E tests after implementation
  - [ ] Verify all tests PASS (green phase)
  - [ ] Tests changed minimally (only fixture adjustments, not logic)

## Constitution Compliance

- [x] **Principle I (Security-First)**: Authorization fail-closed, redirect_uri validation
- [x] **Principle II (ADRs)**: Follows ADR 004 for storage layer
- [x] **Principle III (Library-First Security)**: Uses `net/url` for redirect validation
- [x] **Principle IV (API Documentation)**: OpenAPI specs updated
- [x] **Principle VI (Hexagonal Architecture)**: Domain validation, port interfaces
- [x] **Principle VIII (TDD)**: Tests written first, fail before implementation
- [x] **Principle IX (Persistence Patterns)**: Follows quickstart.md patterns, JSONB with sqlx
- [x] **Principle X (API-First)**: APIs designed before implementation
- [x] **Principle XI (Design System)**: Uses Card, Badge, Button components
- [x] **Principle XIII (E2E Testing)**: 37 scenarios mapped 1:1 to `It()` blocks

## Common Pitfalls

1. **❌ Forgetting duplicate service_id validation**: Check for duplicates in domain validation (FR-003a)
2. **❌ Case-insensitive scope matching**: Scopes MUST be case-sensitive per RFC 6749 and SR-006
3. **❌ Optional requirements blocking auth**: Only "mandatory" requirements should block (FR-014)
4. **❌ External redirect allowed**: MUST validate same-origin (SR-001, SR-002)
5. **❌ Writing E2E tests after implementation**: Tests MUST be written FIRST per Principle XIII

## Next Steps After Implementation

- [ ] Update ARCHITECTURE.md Glossary with new domain terms
- [ ] Run `just check` (fmt, vet, lint)
- [ ] Run `just verify`
- [ ] Request code review
- [ ] Run codeql_checker for security vulnerabilities
- [ ] Update agent context via `.specify/scripts/bash/update-agent-context.sh copilot`
