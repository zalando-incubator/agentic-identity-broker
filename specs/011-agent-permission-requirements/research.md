# Research: Agent Permission Requirements

**Feature**: 011-agent-permission-requirements  
**Date**: 2026-01-08  
**Status**: Phase 0 Complete

## Overview

This document consolidates research findings for implementing agent permission requirements. Each section addresses unknowns from the Technical Context and provides rationale for technology choices.

## Research Questions

### 1. Service Requirement Validation Approach

**Question**: How should service requirement validation be implemented to ensure referential integrity and scope validation?

**Decision**: Multi-layer validation with fail-fast approach

**Rationale**:
- **Domain Layer**: Agent entity validates structure (service_id format, requirement_type enum, required_scopes non-empty)
- **Application Layer**: Admin handler validates referential integrity (service exists, scopes valid) before persisting
- **Database Layer**: JSONB column stores requirements without foreign key constraints for flexibility

**Alternatives Considered**:
1. **Foreign keys at database level**: Rejected - JSONB doesn't support foreign keys, would require junction tables (complexity)
2. **Validation only at API layer**: Rejected - violates hexagonal architecture (domain logic leaks to handlers)
3. **No validation, fail at runtime**: Rejected - violates security-first principle (fail closed)

**Implementation Pattern**:
```go
// Domain entity validation (structure)
func (a *Agent) Validate() error {
    for _, req := range a.ServiceRequirements {
        if req.ServiceID == "" || !isValidUUID(req.ServiceID) {
            return ErrInvalidServiceID
        }
        if req.RequirementType != "mandatory" && req.RequirementType != "optional" {
            return ErrInvalidRequirementType
        }
        if len(req.RequiredScopes) == 0 {
            return ErrEmptyRequiredScopes
        }
    }
    return nil
}

// Application layer validation (referential integrity)
func (h *AgentHandler) CreateAgent(req CreateAgentRequest) error {
    // Validate service_id references exist
    for _, req := range agent.ServiceRequirements {
        service, err := h.serviceRepo.GetByID(req.ServiceID)
        if err != nil {
            return ErrServiceNotFound
        }
        // Validate scopes exist in service
        for _, scope := range req.RequiredScopes {
            if !service.HasScope(scope) {
                return ErrInvalidScope
            }
        }
    }
    return h.agentRepo.Create(agent)
}
```

### 2. Scope Validation and Matching Patterns

**Question**: How should scope matching work when validating user sessions against required scopes?

**Decision**: Superset validation with case-sensitive string matching

**Rationale**:
- **Superset Check**: User's session scopes must include ALL required scopes (additional scopes acceptable per spec clarification)
- **Case-Sensitive**: OAuth2 scopes are case-sensitive per RFC 6749 Section 3.3 (prevents scope confusion attacks per SR-006)
- **Exact String Match**: No wildcards or regex (simpler, more secure)

**Alternatives Considered**:
1. **Exact match only**: Rejected - too restrictive (user can't have extra scopes)
2. **Case-insensitive**: Rejected - violates OAuth2 spec and introduces security risk (SR-006)
3. **Wildcard scopes**: Rejected - not in spec, adds complexity

**Implementation Pattern**:
```go
func hasRequiredScopes(sessionScopes []string, requiredScopes []string) bool {
    scopeSet := make(map[string]bool)
    for _, s := range sessionScopes {
        scopeSet[s] = true // Case-sensitive
    }
    
    for _, required := range requiredScopes {
        if !scopeSet[required] {
            return false
        }
    }
    return true
}
```

### 3. Authorization Flow Enhancement Strategy

**Question**: How should the authorization endpoint be extended to check service requirements without breaking existing behavior?

**Decision**: Sequential validation with early returns and backward compatibility

**Rationale**:
- **Backward Compatible**: Agents without service_requirements (NULL or empty array) skip requirement checks
- **Sequential Checks**: Validate in order: 1) client_id, 2) consent exists, 3) mandatory requirements met, 4) proxy to upstream
- **Early Return**: Redirect to consent screen immediately when any mandatory requirement is unmet (fail-fast)
- **No Partial State**: Either all requirements met or redirect (no partial success)

**Alternatives Considered**:
1. **Collect all missing requirements before redirect**: Rejected - adds complexity, no benefit (consent screen evaluates independently)
2. **Make requirement checks optional**: Rejected - violates security-first principle
3. **Parallel validation**: Rejected - sequential is simpler and sufficient for performance goals (<200ms p95)

**Implementation Pattern**:
```go
func (h *OAuth2Handler) Authorize(w http.ResponseWriter, r *http.Request) {
    // 1. Validate client_id
    agent, err := h.agentRepo.GetByClientID(clientID)
    if err != nil {
        http.Error(w, "Invalid client", http.StatusBadRequest)
        return
    }
    
    // 2. Check consent exists
    grant, err := h.grantRepo.GetByUserAndAgent(userID, agent.ID)
    if err != nil || grant.IsExpired() {
        redirectToConsent(w, r)
        return
    }
    
    // 3. Check mandatory service requirements (NEW)
    if len(agent.ServiceRequirements) > 0 {
        if !h.validateMandatoryRequirements(userID, agent.ServiceRequirements) {
            redirectToConsent(w, r)
            return
        }
    }
    
    // 4. Proxy to upstream
    h.proxyToUpstream(w, r)
}
```

### 4. Consent UI Patterns for Requirement Display

> **Historical visual scope**: The colors and Principle XI rationale below record this feature's original design review, not a current constitutional aesthetic mandate. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

**Question**: How should the consent screen UI display service requirements (mandatory vs optional, read-only scopes)?

**Decision**: Use design system components with semantic status indicators and grouped layout

**Rationale**:
- **Design System Compliance**: Use existing Card, Badge, Button components from `web/src/design-system/`
- **Status Indicators**: Badge component with semantic colors (trust-deep for required, neutral for optional per Principle XI)
- **Grouped Layout**: Mandatory services first, then optional (per FR-018)
- **Read-Only Scopes**: Display as list items with descriptions (no checkboxes per FR-021)
- **Action Buttons**: "Login" button for missing sessions, "Disconnect" for active sessions (per FR-020)

**Alternatives Considered**:
1. **Custom component styling**: Rejected - violates Principle XI (design system compliance)
2. **Editable scope selection**: Rejected - spec explicitly removes edit mode (User Story 5)
3. **Single list without grouping**: Rejected - violates FR-018 (mandatory services must be first)

**Component Structure**:
```tsx
// ServiceRequirementCard component
<Card variant="elevated">
  <CardHeader>
    <ServiceIcon />
    <ServiceName>{service.name}</ServiceName>
    <Badge variant={requirement.type === 'mandatory' ? 'primary' : 'secondary'}>
      {requirement.type === 'mandatory' ? 'Required' : 'Optional'}
    </Badge>
  </CardHeader>
  <CardBody>
    <ScopesList>
      {requirement.scopes.map(scope => (
        <ScopeItem key={scope}>
          {scope}
          {scopeDescriptions[scope] && <ScopeDescription>{scopeDescriptions[scope]}</ScopeDescription>}
        </ScopeItem>
      ))}
    </ScopesList>
    {!hasActiveSession && <Button onClick={() => redirectToOAuth(service.id)}>Login</Button>}
    {hasActiveSession && <SessionStatus>Active Session</SessionStatus>}
  </CardBody>
</Card>
```

### 5. Redirect URI Security Patterns

**Question**: How should redirect_uri validation prevent open redirect vulnerabilities while supporting seamless OAuth2 flow?

**Decision**: Strict same-origin validation using Go standard library url.Parse

**Rationale**:
- **Library-First**: Use `net/url` package for URL parsing (Principle III)
- **Same-Origin Only**: Check scheme, host, and port match request origin (SR-001, SR-002)
- **Relative URLs Allowed**: Support relative paths (e.g., `/oauth2/authorize?...`) for convenience
- **Fail Closed**: Reject invalid redirect_uri with HTTP 400 (never redirect to untrusted destination)

**Alternatives Considered**:
1. **Whitelist approach**: Rejected - not in spec, adds operational complexity
2. **Allow external redirects**: Rejected - violates SR-002 (open redirect vulnerability)
3. **Custom URL parser**: Rejected - violates Principle III (library-first security)

**Implementation Pattern**:
```go
func validateRedirectURI(redirectURI string, requestOrigin string) error {
    // Parse redirect_uri
    u, err := url.Parse(redirectURI)
    if err != nil {
        return ErrInvalidRedirectURI
    }
    
    // Allow relative URLs
    if u.Scheme == "" && u.Host == "" {
        return nil
    }
    
    // Parse request origin
    origin, err := url.Parse(requestOrigin)
    if err != nil {
        return ErrInvalidOrigin
    }
    
    // Same-origin check
    if u.Scheme != origin.Scheme || u.Host != origin.Host {
        return ErrExternalRedirect
    }
    
    return nil
}
```

### 6. JSONB Storage for Service Requirements

**Question**: Should service requirements be stored as JSONB or normalized tables?

**Decision**: JSONB column on agents table

**Rationale**:
- **Follows ADR 004**: PostgreSQL adapter uses sqlx (not ORM), JSONB is well-supported
- **No Foreign Keys Needed**: Service requirement is a value object, not an entity with lifecycle
- **Simpler Queries**: Single read for agent includes all requirements (no joins)
- **Backward Compatible**: NULL value for existing agents (treated as empty array)
- **Flexible Schema**: Can add fields to ServiceRequirement without migration

**Alternatives Considered**:
1. **Normalized tables**: `agent_service_requirements(agent_id, service_id, requirement_type)` + `requirement_scopes(requirement_id, scope)` - Rejected - over-engineering for simple value object
2. **JSON (not JSONB)**: Rejected - JSONB has better query performance and validation
3. **Serialized string**: Rejected - no type safety, validation issues

**Migration Pattern**:
```sql
-- 005_add_agent_service_requirements.up.sql
ALTER TABLE agents ADD COLUMN service_requirements JSONB DEFAULT NULL;
CREATE INDEX idx_agents_service_requirements ON agents USING GIN (service_requirements);

-- 005_add_agent_service_requirements.down.sql
DROP INDEX IF EXISTS idx_agents_service_requirements;
ALTER TABLE agents DROP COLUMN service_requirements;
```

## Technology Stack Summary

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Backend Language | Go 1.23.0+ | Existing project standard |
| Database | PostgreSQL 12+ | Existing project standard, JSONB support |
| Database Library | sqlx v1.3.5+ | Follows ADR 004 (no ORM) |
| HTTP Router | chi v5.2.3 | Existing project standard |
| Frontend Framework | React 18+ | Existing consent UI |
| Frontend Build | Vite 5+ | Existing project standard |
| Testing Framework | Ginkgo/Gomega | E2E tests per Principle XIII |
| URL Validation | net/url (stdlib) | Library-first security (Principle III) |

## Dependencies

| Dependency | Type | Purpose | Risk |
|------------|------|---------|------|
| ThirdPartyOAuth2Service | Entity (spec 006) | Service metadata for validation | Must exist before agents reference it |
| ThirdPartyOAuth2Session | Entity (spec 008) | User session validation | Must be operational for authorization checks |
| OAuth2AuthorizationEndpoint | Handler (spec 009) | Extend to validate requirements | Must be modified carefully to avoid breaking existing flows |
| ConsentFrontend | React app (spec 007) | Display requirements | Must be extended without breaking existing consent flows |

## Security Considerations

1. **Service Requirement Injection**: Validated at domain and application layers (fail closed)
2. **Scope Confusion Attacks**: Case-sensitive scope matching per RFC 6749 (SR-006)
3. **Open Redirect**: Strict same-origin validation for redirect_uri (SR-001, SR-002)
4. **Authorization Bypass**: Mandatory requirements are enforced before proxy (fail closed per FR-012)
5. **Audit Logging**: All requirement validation failures logged for security audit (SR-004)

## Performance Considerations

1. **Authorization Validation**: Sequential checks with early return (estimated <50ms overhead)
2. **JSONB Queries**: GIN index on service_requirements for query performance
3. **Session Lookups**: For each mandatory service, query user's session (O(n) where n = number of mandatory services)
4. **Frontend Rendering**: Service requirements loaded once with agent metadata (no additional API calls)

**Estimated Performance Impact**:
- Authorization with 3 mandatory services: +30-50ms (well under <200ms p95 constraint)
- Consent screen load with service requirements: +0ms (included in agent metadata fetch)
- Admin API agent creation with validation: +10-20ms (scope validation queries)

## Next Steps

- [ ] **Phase 1**: Generate data-model.md (entities, value objects, relationships)
- [ ] **Phase 1**: Generate API contracts in contracts/ directory (OpenAPI specs)
- [ ] **Phase 1**: Generate quickstart.md implementation guide
- [ ] **Phase 1**: Update agent context via update-agent-context.sh
- [ ] **Phase 1**: Re-evaluate Constitution Check post-design
