# API Design Documentation

## Overview

This document provides design rationale, implementation guidance, and best practices for the Domain Model and Consent APIs.

## API Design Principles

### 1. RESTful Resource Architecture

All APIs follow RESTful principles with clear resource-oriented URLs:

- **Admin APIs**: `/api/agents`, `/api/third-party/oauth2/clients`
- **User APIs**: `/api/consent/agent/{agent-id}`, `/api/consent/agent/{agent-id}/grants`

### 2. HTTP Method Semantics

- **GET**: Retrieve resources (idempotent, cacheable)
- **POST**: Create new resources or perform operations
- **PUT**: Update existing resources (full replacement)
- **DELETE**: Remove resources (idempotent)

### 3. Status Code Usage

| Status Code | Usage |
|-------------|-------|
| 200 OK | Successful GET or PUT operation |
| 201 Created | Successful POST (resource created) |
| 204 No Content | Successful DELETE or grant revocation |
| 400 Bad Request | Validation errors, invalid input |
| 401 Unauthorized | Missing or invalid authentication |
| 403 Forbidden | Insufficient permissions |
| 404 Not Found | Resource does not exist |
| 409 Conflict | Duplicate resource or referential integrity violation |
| 500 Internal Server Error | Unexpected server error |

## API Categories

### Admin APIs

**Purpose**: Full CRUD operations for system configuration

**Authentication**: Requires administrative privileges

**Rate Limiting**: Stricter limits due to sensitive operations

**Audit Logging**: All operations logged for compliance

#### Agent Management (`/api/agents`)

Agents represent AI systems that request delegated access to third-party services.

**Resource Structure**:
```json
{
  "id": "uuid",
  "oauth2_client_id": "string",
  "external_id": "string?",
  "display_name": "string",
  "description": "string",
  "governance_url": "uri?",
  "user_documentation_url": "uri?",
  "agent_interface_url": "uri?"
}
```

**Design Decisions**:
- System-generated UUIDs for `id` (immutable, opaque identifiers)
- `oauth2_client_id` enables OAuth2 integration (mutable for credential rotation)
- Optional `external_id` links to external governance systems
- URL fields validated to prevent injection attacks

**Cascade Deletion**: When an agent is deleted, all associated grants are automatically deleted. This maintains referential integrity and prevents orphaned grants.

#### OAuth2 Service Management (`/api/third-party/oauth2/clients`)

Third-party OAuth2 services represent external providers (GitHub, Google, Databricks, etc.) that agents access on behalf of users.

**Resource Structure**:
```json
{
  "id": "uuid",
  "display_name": "string",
  "client_id": "string",
  "client_secret": "***REDACTED***",
  "issuer_uri": "uri",
  "discovery": {
    "enable_discovery": true,
    "metadata_url": "uri?"
  },
  "endpoints": {
    "token_endpoint": "uri",
    "authorize_endpoint": "uri"
  },
  "scopes": [
    {
      "scope_value": "string",
      "description": "string"
    }
  ]
}
```

**Design Decisions**:
- `client_secret` always redacted in responses (security requirement SR-003)
- Automatic endpoint discovery via well-known metadata URL
- Manual endpoint configuration available as fallback
- Scopes include human-readable descriptions for consent UIs

**Discovery Mechanism**:
1. If `enable_discovery: true`, construct metadata URL:
   - Default: `{issuer_uri}/.well-known/oauth-authorization-server`
   - Override: Use `metadata_url` if provided
2. Fetch metadata and populate `token_endpoint`, `authorize_endpoint`
3. If discovery fails, fall back to manual configuration
4. Validate SSL certificates (reject self-signed in production)

**Deletion Protection**: Services cannot be deleted if active grants reference them (FR-022). This prevents breaking existing user authorizations.

**Error Response Example**:
```json
{
  "error": "Cannot delete OAuth2 service with active grants",
  "message": "Service 'github-prod' has 15 active grants referencing it. Remove grants first.",
  "grants_count": 15
}
```

### User APIs

**Purpose**: User-facing consent and grant management

**Authentication**: Principal extracted from session (via reverse proxy)

**Authorization**: Users can only view and modify their own grants

**Rate Limiting**: Standard limits for end-user operations

#### Agent Information for Consent (`/api/consent/agent/{agent-id}`)

**Purpose**: Provide all information needed to build a consent interface

**Response Structure**:
```json
{
  "agent": {
    "id": "uuid",
    "display_name": "Research Assistant",
    "description": "AI assistant that helps with academic research",
    "governance_url": "https://example.com/governance",
    "user_documentation_url": "https://example.com/docs",
    "agent_interface_url": "https://example.com/agent"
  },
  "requested_services": [
    {
      "id": "uuid",
      "display_name": "GitHub Production",
      "scopes": [
        {
          "scope_value": "repo",
          "description": "Full control of private repositories"
        },
        {
          "scope_value": "read:org",
          "description": "Read organization membership"
        }
      ]
    }
  ]
}
```

**Design Decisions**:
- Returns ALL configured third-party services (FR-025)
- Agent-to-service associations deferred to future feature
- Scope descriptions enable informed consent
- No grants information (use grants endpoint for that)

**Client Usage Pattern**:
```javascript
// 1. Fetch agent information
const agentInfo = await fetch('/api/consent/agent/{agent-id}')

// 2. Display consent UI with:
//    - Agent name and description
//    - Available services and scopes
//    - Governance and documentation links

// 3. User selects services/scopes

// 4. Submit grant request (see grants endpoint)
```

#### Grant Management (`/api/consent/agent/{agent-id}/grants`)

**GET**: Retrieve user's active grants for the agent

**Response Structure**:
```json
{
  "grants": [
    {
      "id": "uuid",
      "principal": "alice@example.com",
      "agent_id": "uuid",
      "valid_until": "2025-12-31T23:59:59Z",
      "delegated_oauth2_tokens": [
        {
          "thirdparty_oauth2_service_id": "uuid",
          "scopes": ["repo", "read:org"]
        }
      ]
    }
  ],
  "count": 1
}
```

**Design Decisions**:
- Expired grants automatically filtered (FR-019)
- Indefinite grants have `valid_until: null`
- Empty array if no grants (not an error)
- Principal automatically populated from session

**POST**: Create, update, or revoke grant

**Request Structure**:
```json
{
  "valid_until": "2025-12-31T23:59:59Z",
  "delegated_oauth2_tokens": [
    {
      "thirdparty_oauth2_service_id": "uuid",
      "scopes": ["repo", "read:org"]
    }
  ]
}
```

**Upsert Semantics** (FR-015):
- One active grant per user-agent pair
- If grant exists: Update it (200 OK)
- If grant doesn't exist: Create it (201 Created)
- Principal derived from session (never in request body)

**Grant Revocation**:
```json
{
  "delegated_oauth2_tokens": []
}
```
Response: `204 No Content`

**Grant Duration** (FR-016):
- `valid_until` field is optional
- If provided: Must be in the future (validation error otherwise)
- If omitted: Grant never expires (`valid_until: null`)
- User-controlled expiration strategy

**Validation Examples**:

```json
// Valid: Future expiration
{
  "valid_until": "2025-12-31T23:59:59Z",
  "delegated_oauth2_tokens": [...]
}

// Valid: Indefinite grant
{
  "delegated_oauth2_tokens": [...]
}

// Invalid: Past expiration
{
  "valid_until": "2020-01-01T00:00:00Z",
  "delegated_oauth2_tokens": [...]
}
// Response: 400 Bad Request
// {
//   "error": "Validation failed",
//   "validation_errors": [
//     {
//       "field": "valid_until",
//       "message": "Timestamp must be in the future",
//       "code": "FUTURE_DATE_REQUIRED"
//     }
//   ]
// }

// Invalid: Non-existent scope
{
  "delegated_oauth2_tokens": [
    {
      "thirdparty_oauth2_service_id": "uuid",
      "scopes": ["repo", "invalid_scope"]
    }
  ]
}
// Response: 400 Bad Request
// {
//   "error": "Invalid scopes",
//   "message": "Requested scopes do not exist in service configuration",
//   "validation_errors": [
//     {
//       "field": "scopes[1]",
//       "message": "Scope 'invalid_scope' does not exist in service",
//       "code": "SCOPE_NOT_FOUND"
//     }
//   ]
// }
```

## Security Design

### Authentication

**Method**: Pre-authentication via reverse proxy (SR-001)

**Flow**:
1. User authenticates with reverse proxy (OAuth2, SAML, etc.)
2. Proxy validates credentials and sets principal header
3. Identity broker extracts principal from header
4. Principal propagated via request context

**Configuration**:
```yaml
servers:
  enduser:
    authentication:
      preauth:
        principal_header_name: X-Remote-User
```

**Middleware Stack**:
- `OptionalPrincipalMiddleware`: Extracts principal if present (all routes)
- `RequirePrincipalMiddleware`: Rejects if missing (protected routes)

### Authorization

**Admin APIs**:
- Require administrative role/permission
- Implementation-specific (RBAC, attribute-based, etc.)

**User APIs**:
- Automatically scoped to authenticated principal
- Users can only access their own grants
- Principal filtering applied at database level

**Grant Isolation** (SR-010):
```sql
-- All grant queries filtered by principal
SELECT * FROM user_grants
WHERE principal = ? AND agent_id = ?
```

### Data Protection

**Client Secrets** (SR-002, SR-003):
- Encrypted at rest using platform encryption service
- Redacted in all API responses: `"***REDACTED***"`
- Only accepted in create/update operations
- Never logged or exposed

**URL Validation** (SR-011):
- All URLs sanitized to prevent injection
- Schema validation (https required in production)
- Character set restrictions
- Length limits enforced

**Principal Validation**:
- Maximum length: 200 characters
- Trimmed of whitespace
- Non-empty validation
- Context-safe storage

### Audit Logging

**Admin Operations** (SR-007):
```json
{
  "timestamp": "2025-12-17T10:30:00Z",
  "operation": "agent.create",
  "actor": "admin@example.com",
  "resource_id": "550e8400-e29b-41d4-a716-446655440000",
  "changes": {
    "display_name": "Research Assistant",
    "oauth2_client_id": "agent-assistant-prod"
  }
}
```

**Grant Operations** (SR-008):
```json
{
  "timestamp": "2025-12-17T10:35:00Z",
  "operation": "grant.create",
  "principal": "alice@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "services": ["github-prod"],
  "scopes_count": 2,
  "valid_until": "2025-12-31T23:59:59Z"
}
```

### Rate Limiting

**Strategy**: Token bucket algorithm per principal/IP

**Limits**:
- Admin APIs: 100 requests/minute per admin
- User APIs: 300 requests/minute per user
- Global: 10,000 requests/minute per server

**Headers**:
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 287
X-RateLimit-Reset: 1703678400
```

**Error Response**:
```json
{
  "error": "Rate limit exceeded",
  "message": "Too many requests. Please try again in 42 seconds.",
  "retry_after": 42
}
```

## Error Handling

### Validation Errors

**Structure**:
```json
{
  "error": "Validation failed",
  "message": "One or more fields contain invalid values",
  "validation_errors": [
    {
      "field": "display_name",
      "message": "Field is required",
      "code": "REQUIRED_FIELD_MISSING"
    },
    {
      "field": "valid_until",
      "message": "Timestamp must be in the future",
      "code": "FUTURE_DATE_REQUIRED"
    }
  ]
}
```

**Common Validation Codes**:
- `REQUIRED_FIELD_MISSING`: Required field not provided
- `INVALID_FORMAT`: Field format invalid (UUID, URI, date, etc.)
- `FUTURE_DATE_REQUIRED`: Timestamp must be in the future
- `SCOPE_NOT_FOUND`: Requested scope not in service configuration
- `STRING_TOO_LONG`: Field exceeds maximum length
- `INVALID_CHARACTER`: Field contains invalid characters

### Referential Integrity Errors

**Service Deletion with Active Grants**:
```json
{
  "error": "Cannot delete OAuth2 service with active grants",
  "message": "Service 'github-prod' has 15 active grants referencing it. Remove grants first.",
  "grants_count": 15
}
```

**Non-existent References**:
```json
{
  "error": "Invalid reference",
  "message": "OAuth2 service 'abc123' does not exist",
  "validation_errors": [
    {
      "field": "thirdparty_oauth2_service_id",
      "message": "Service with ID 'abc123' not found",
      "code": "REFERENCE_NOT_FOUND"
    }
  ]
}
```

### Discovery Errors

**Failed Endpoint Discovery**:
```json
{
  "error": "Discovery failed",
  "message": "Unable to retrieve OAuth2 metadata from issuer",
  "details": {
    "issuer_uri": "https://github.com",
    "metadata_url": "https://github.com/.well-known/oauth-authorization-server",
    "http_status": 404,
    "suggestion": "Provide manual endpoint configuration"
  }
}
```

## Implementation Guidance

### Database Schema Considerations

**Agents Table**:
```sql
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oauth2_client_id VARCHAR(255) NOT NULL UNIQUE,
    external_id VARCHAR(255),
    display_name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,
    governance_url VARCHAR(2048),
    user_documentation_url VARCHAR(2048),
    agent_interface_url VARCHAR(2048),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agents_oauth2_client_id ON agents(oauth2_client_id);
CREATE INDEX idx_agents_external_id ON agents(external_id) WHERE external_id IS NOT NULL;
```

**OAuth2 Services Table**:
```sql
CREATE TABLE thirdparty_oauth2_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name VARCHAR(200) NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    client_secret_encrypted TEXT NOT NULL,
    issuer_uri VARCHAR(2048) NOT NULL,
    enable_discovery BOOLEAN NOT NULL DEFAULT false,
    metadata_url VARCHAR(2048),
    token_endpoint VARCHAR(2048) NOT NULL,
    authorize_endpoint VARCHAR(2048) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_oauth2_services_client_id ON thirdparty_oauth2_services(client_id);
```

**OAuth2 Scopes Table**:
```sql
CREATE TABLE oauth2_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thirdparty_oauth2_service_id UUID NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE CASCADE,
    scope_value VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(thirdparty_oauth2_service_id, scope_value)
);

CREATE INDEX idx_oauth2_scopes_service_id ON oauth2_scopes(thirdparty_oauth2_service_id);
```

**User Grants Table**:
```sql
CREATE TABLE user_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    principal VARCHAR(200) NOT NULL,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    valid_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(principal, agent_id)
);

CREATE INDEX idx_user_grants_principal ON user_grants(principal);
CREATE INDEX idx_user_grants_agent_id ON user_grants(agent_id);
CREATE INDEX idx_user_grants_valid_until ON user_grants(valid_until) WHERE valid_until IS NOT NULL;
```

**Delegated OAuth2 Tokens Table**:
```sql
CREATE TABLE delegated_oauth2_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_grant_id UUID NOT NULL REFERENCES user_grants(id) ON DELETE CASCADE,
    thirdparty_oauth2_service_id UUID NOT NULL REFERENCES thirdparty_oauth2_services(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_grant_id, thirdparty_oauth2_service_id)
);

CREATE INDEX idx_delegated_tokens_grant_id ON delegated_oauth2_tokens(user_grant_id);
CREATE INDEX idx_delegated_tokens_service_id ON delegated_oauth2_tokens(thirdparty_oauth2_service_id);
```

**Delegated Token Scopes Table**:
```sql
CREATE TABLE delegated_token_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delegated_oauth2_token_id UUID NOT NULL REFERENCES delegated_oauth2_tokens(id) ON DELETE CASCADE,
    scope_value VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(delegated_oauth2_token_id, scope_value)
);

CREATE INDEX idx_delegated_scopes_token_id ON delegated_token_scopes(delegated_oauth2_token_id);
```

### Hexagonal Architecture Integration

**Port Definitions** (`internal/ports/domain.go`):
```go
package ports

import (
    "context"
    "time"
)

// AgentRepository defines persistence operations for agents
type AgentRepository interface {
    Create(ctx context.Context, agent *Agent) error
    Get(ctx context.Context, id string) (*Agent, error)
    List(ctx context.Context) ([]*Agent, error)
    Update(ctx context.Context, agent *Agent) error
    Delete(ctx context.Context, id string) error
}

// OAuth2ServiceRepository defines persistence operations for OAuth2 services
type OAuth2ServiceRepository interface {
    Create(ctx context.Context, service *OAuth2Service) error
    Get(ctx context.Context, id string) (*OAuth2Service, error)
    List(ctx context.Context) ([]*OAuth2Service, error)
    Update(ctx context.Context, service *OAuth2Service) error
    Delete(ctx context.Context, id string) error
    HasActiveGrants(ctx context.Context, serviceID string) (bool, int, error)
}

// GrantRepository defines persistence operations for user grants
type GrantRepository interface {
    Create(ctx context.Context, grant *UserGrant) error
    Get(ctx context.Context, id string) (*UserGrant, error)
    GetByPrincipalAndAgent(ctx context.Context, principal, agentID string) (*UserGrant, error)
    ListByPrincipalAndAgent(ctx context.Context, principal, agentID string) ([]*UserGrant, error)
    Update(ctx context.Context, grant *UserGrant) error
    Delete(ctx context.Context, id string) error
}

// OAuth2DiscoveryService defines OAuth2 endpoint discovery operations
type OAuth2DiscoveryService interface {
    DiscoverEndpoints(ctx context.Context, issuerURI, metadataURL string) (*OAuth2Endpoints, error)
}
```

**Domain Types** (`internal/domain/agent/types.go`):
```go
package agent

import "time"

type Agent struct {
    ID                   string
    OAuth2ClientID       string
    ExternalID           *string
    DisplayName          string
    Description          string
    GovernanceURL        *string
    UserDocumentationURL *string
    AgentInterfaceURL    *string
    CreatedAt            time.Time
    UpdatedAt            time.Time
}

type OAuth2Service struct {
    ID                string
    DisplayName       string
    ClientID          string
    ClientSecret      string // Always encrypted
    IssuerURI         string
    EnableDiscovery   bool
    MetadataURL       *string
    TokenEndpoint     string
    AuthorizeEndpoint string
    Scopes            []OAuth2Scope
    CreatedAt         time.Time
    UpdatedAt         time.Time
}

type OAuth2Scope struct {
    ScopeValue  string
    Description string
}

type UserGrant struct {
    ID                     string
    Principal              string
    AgentID                string
    ValidUntil             *time.Time
    DelegatedOAuth2Tokens  []DelegatedOAuth2Token
    CreatedAt              time.Time
    UpdatedAt              time.Time
}

type DelegatedOAuth2Token struct {
    ThirdpartyOAuth2ServiceID string
    Scopes                    []string
}
```

### Handler Implementation Pattern

**Example: Create Agent Handler**:
```go
package http

import (
    "encoding/json"
    "net/http"
    "github.com/go-chi/chi/v5"
)

func (h *AgentHandler) CreateAgent(w http.ResponseWriter, r *http.Request) {
    // 1. Decode request
    var req AgentCreateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid JSON")
        return
    }

    // 2. Validate request
    if err := h.validator.Validate(req); err != nil {
        respondValidationError(w, err)
        return
    }

    // 3. Call domain service
    agent, err := h.agentService.CreateAgent(r.Context(), &req)
    if err != nil {
        handleDomainError(w, err)
        return
    }

    // 4. Emit audit log
    h.auditLogger.Log(r.Context(), "agent.create", agent.ID)

    // 5. Return response
    respondJSON(w, http.StatusCreated, agent)
}
```

### Testing Strategy

**Unit Tests**:
- Request/response serialization
- Validation logic
- Error handling paths
- Domain service mocking

**Integration Tests**:
- Full request/response cycle
- Database interactions
- Referential integrity constraints
- Cascade deletion behavior

**End-to-End Tests**:
- Complete user flows (consent + grant creation)
- Admin workflows (service configuration)
- Security boundaries (principal isolation)
- Error scenarios (invalid scopes, expired timestamps)

## Performance Considerations

### Caching Strategy

**Static Data** (cache aggressively):
- Agent information (invalidate on update)
- OAuth2 service configurations (invalidate on update)
- Scope definitions (invalidate on service update)

**Dynamic Data** (cache cautiously):
- User grants (short TTL, invalidate on grant modification)
- Grant listings (no caching, always fetch latest)

### Query Optimization

**Grant Listing**:
```sql
-- Fetch grants with all relationships in single query
SELECT
    g.id, g.principal, g.agent_id, g.valid_until,
    dt.thirdparty_oauth2_service_id,
    array_agg(dts.scope_value) as scopes
FROM user_grants g
LEFT JOIN delegated_oauth2_tokens dt ON dt.user_grant_id = g.id
LEFT JOIN delegated_token_scopes dts ON dts.delegated_oauth2_token_id = dt.id
WHERE g.principal = ?
  AND g.agent_id = ?
  AND (g.valid_until IS NULL OR g.valid_until > NOW())
GROUP BY g.id, g.principal, g.agent_id, g.valid_until, dt.id, dt.thirdparty_oauth2_service_id;
```

**Consent Information**:
```sql
-- Fetch agent + all services + scopes in single query
SELECT
    a.id as agent_id, a.display_name as agent_name, a.description as agent_desc,
    s.id as service_id, s.display_name as service_name,
    sc.scope_value, sc.description as scope_desc
FROM agents a
CROSS JOIN thirdparty_oauth2_services s
LEFT JOIN oauth2_scopes sc ON sc.thirdparty_oauth2_service_id = s.id
WHERE a.id = ?;
```

### Connection Pooling

**Configuration**:
```yaml
database:
  max_open_connections: 25
  max_idle_connections: 10
  connection_max_lifetime: 300s
  connection_max_idle_time: 60s
```

## Versioning Strategy

### Current Version

APIs are at version 1.0.0 (implicit in URLs, no version prefix yet)

### Future Versioning

When breaking changes are required:

**Option 1: URL Versioning**:
```
/api/v1/agents
/api/v2/agents
```

**Option 2: Header Versioning**:
```
Accept: application/vnd.agentic-identity-broker.v2+json
```

**Option 3: Content-Type Versioning**:
```
Content-Type: application/vnd.agentic-identity-broker.agent.v2+json
```

**Recommendation**: Start with URL versioning for simplicity

### Deprecation Policy

1. Announce deprecation 6 months in advance
2. Maintain backward compatibility for 12 months
3. Provide migration guides and tooling
4. Send deprecation warnings in response headers:
   ```
   Deprecation: true
   Sunset: Sat, 31 Dec 2025 23:59:59 GMT
   Link: <https://docs.example.com/migration>; rel="migration"
   ```

## Monitoring and Observability

### Metrics to Track

**Request Metrics**:
- Request rate per endpoint
- Response time (p50, p95, p99)
- Error rate by status code
- Request size distribution

**Business Metrics**:
- Agents registered per day
- Services configured per day
- Grants created per day
- Grant revocations per day
- Average grant duration
- Services per grant
- Scopes per service delegation

**Security Metrics**:
- Failed authentication attempts
- Rate limit violations
- Referential integrity violations
- Discovery failures
- Invalid scope requests

### Structured Logging

**Request Logging**:
```json
{
  "timestamp": "2025-12-17T10:30:00Z",
  "level": "info",
  "message": "HTTP request completed",
  "method": "POST",
  "path": "/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants",
  "status": 201,
  "duration_ms": 45,
  "principal": "alice@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Error Logging**:
```json
{
  "timestamp": "2025-12-17T10:35:00Z",
  "level": "error",
  "message": "Scope validation failed",
  "principal": "bob@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "invalid_scopes": ["repo:write", "admin:org"],
  "service_id": "660e8400-e29b-41d4-a716-446655440001"
}
```

## OpenAPI Specification

The complete OpenAPI 3.0 specification is available at:
- File: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/specs/20251217-120000-domain-model-apis/contracts/openapi.yaml`
- Interactive documentation: Can be viewed with Swagger UI or Redoc
- Code generation: Use `openapi-generator` for client SDK generation

### Viewing the Specification

**With Docker**:
```bash
docker run -p 8082:8080 -e SWAGGER_JSON=/openapi.yaml \
  -v $(pwd)/openapi.yaml:/openapi.yaml \
  swaggerapi/swagger-ui
```

**With npx**:
```bash
npx @redocly/cli preview-docs openapi.yaml
```

### Generating Client SDKs

**TypeScript/JavaScript**:
```bash
openapi-generator-cli generate \
  -i openapi.yaml \
  -g typescript-axios \
  -o ./clients/typescript
```

**Python**:
```bash
openapi-generator-cli generate \
  -i openapi.yaml \
  -g python \
  -o ./clients/python
```

**Go**:
```bash
openapi-generator-cli generate \
  -i openapi.yaml \
  -g go \
  -o ./clients/go
```

## Summary

This API design provides:

1. **Clear Resource Boundaries**: Admin vs. User APIs with appropriate authorization
2. **Security First**: Principal isolation, client secret protection, audit logging
3. **Developer Experience**: Comprehensive error messages, validation feedback, OpenAPI specification
4. **Scalability**: Efficient queries, caching strategy, connection pooling
5. **Maintainability**: Hexagonal architecture, clear separation of concerns
6. **Compliance**: Audit logs, referential integrity, cascade deletion

All design decisions map directly to functional and security requirements in the specification, ensuring complete coverage and traceability.
