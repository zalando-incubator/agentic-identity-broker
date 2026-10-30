# API Quick Reference

Quick reference guide for implementing and consuming the Domain Model and Consent APIs.

## Endpoints Overview

### Admin APIs

| Method | Endpoint | Purpose | Auth Required |
|--------|----------|---------|---------------|
| GET | `/api/agents` | List all agents | Admin |
| POST | `/api/agents` | Create agent | Admin |
| GET | `/api/agents/{id}` | Get agent details | Admin |
| PUT | `/api/agents/{id}` | Update agent | Admin |
| DELETE | `/api/agents/{id}` | Delete agent (cascade) | Admin |
| GET | `/api/third-party/oauth2/clients` | List OAuth2 services | Admin |
| POST | `/api/third-party/oauth2/clients` | Create OAuth2 service | Admin |
| GET | `/api/third-party/oauth2/clients/{id}` | Get service details | Admin |
| PUT | `/api/third-party/oauth2/clients/{id}` | Update service | Admin |
| DELETE | `/api/third-party/oauth2/clients/{id}` | Delete service | Admin |

### User APIs

| Method | Endpoint | Purpose | Auth Required |
|--------|----------|---------|---------------|
| GET | `/api/consent/agent/{id}` | Get agent + services for consent | User |
| GET | `/api/consent/agent/{id}/grants` | List user's grants | User |
| POST | `/api/consent/agent/{id}/grants` | Create/update/revoke grant | User |

## Request Examples

### Create Agent

```bash
curl -X POST http://localhost:8081/api/agents \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -d '{
    "oauth2_client_id": "agent-research-assistant",
    "display_name": "Research Assistant",
    "description": "AI assistant for academic research",
    "governance_url": "https://example.com/governance",
    "user_documentation_url": "https://example.com/docs"
  }'
```

**Response (201)**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "oauth2_client_id": "agent-research-assistant",
  "external_id": null,
  "display_name": "Research Assistant",
  "description": "AI assistant for academic research",
  "governance_url": "https://example.com/governance",
  "user_documentation_url": "https://example.com/docs",
  "agent_interface_url": null
}
```

### Create OAuth2 Service (with Discovery)

```bash
curl -X POST http://localhost:8081/api/third-party/oauth2/clients \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -d '{
    "display_name": "GitHub Production",
    "client_id": "Iv1.1234567890abcdef",
    "client_secret": "secret123",
    "issuer_uri": "https://github.com",
    "discovery": {
      "enable_discovery": true
    },
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
  }'
```

**Response (201)**:
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "display_name": "GitHub Production",
  "client_id": "Iv1.1234567890abcdef",
  "client_secret": "***REDACTED***",
  "issuer_uri": "https://github.com",
  "discovery": {
    "enable_discovery": true,
    "metadata_url": null
  },
  "endpoints": {
    "token_endpoint": "https://github.com/login/oauth/access_token",
    "authorize_endpoint": "https://github.com/login/oauth/authorize"
  },
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
```

### Create OAuth2 Service (Manual Configuration)

```bash
curl -X POST http://localhost:8081/api/third-party/oauth2/clients \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -d '{
    "display_name": "Custom OAuth Provider",
    "client_id": "custom-client-123",
    "client_secret": "secret456",
    "issuer_uri": "https://oauth.custom.com",
    "discovery": {
      "enable_discovery": false
    },
    "endpoints": {
      "token_endpoint": "https://oauth.custom.com/token",
      "authorize_endpoint": "https://oauth.custom.com/authorize"
    },
    "scopes": [
      {
        "scope_value": "profile",
        "description": "Access user profile"
      }
    ]
  }'
```

### Get Agent Information for Consent

```bash
curl http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000 \
  -H "X-Remote-User: alice@example.com"
```

**Response (200)**:
```json
{
  "agent": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "display_name": "Research Assistant",
    "description": "AI assistant for academic research",
    "governance_url": "https://example.com/governance",
    "user_documentation_url": "https://example.com/docs",
    "agent_interface_url": null
  },
  "requested_services": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
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

### Create Grant

```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: alice@example.com" \
  -d '{
    "valid_until": "2025-12-31T23:59:59Z",
    "delegated_oauth2_tokens": [
      {
        "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
        "scopes": ["repo", "read:org"]
      }
    ]
  }'
```

**Response (201)**:
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440002",
  "principal": "alice@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "valid_until": "2025-12-31T23:59:59Z",
  "delegated_oauth2_tokens": [
    {
      "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
      "scopes": ["repo", "read:org"]
    }
  ]
}
```

### Create Indefinite Grant

```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: alice@example.com" \
  -d '{
    "delegated_oauth2_tokens": [
      {
        "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
        "scopes": ["repo"]
      }
    ]
  }'
```

**Response (201)**:
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440002",
  "principal": "alice@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "valid_until": null,
  "delegated_oauth2_tokens": [
    {
      "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
      "scopes": ["repo"]
    }
  ]
}
```

### Update Grant (Upsert)

```bash
# Same endpoint as create - updates existing grant
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: alice@example.com" \
  -d '{
    "valid_until": "2026-06-30T23:59:59Z",
    "delegated_oauth2_tokens": [
      {
        "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
        "scopes": ["repo", "read:org", "write:org"]
      }
    ]
  }'
```

**Response (200)**: Updated grant

### Revoke Grant

```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: alice@example.com" \
  -d '{
    "delegated_oauth2_tokens": []
  }'
```

**Response (204)**: No Content

### List User's Grants

```bash
curl http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "X-Remote-User: alice@example.com"
```

**Response (200)**:
```json
{
  "grants": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "principal": "alice@example.com",
      "agent_id": "550e8400-e29b-41d4-a716-446655440000",
      "valid_until": "2025-12-31T23:59:59Z",
      "delegated_oauth2_tokens": [
        {
          "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
          "scopes": ["repo", "read:org"]
        }
      ]
    }
  ],
  "count": 1
}
```

### Delete Agent (with Cascade)

```bash
curl -X DELETE http://localhost:8081/api/agents/550e8400-e29b-41d4-a716-446655440000 \
  -H "X-Remote-User: admin@example.com"
```

**Response (204)**: No Content (all associated grants also deleted)

### Delete OAuth2 Service (with Protection)

```bash
# Fails if active grants reference the service
curl -X DELETE http://localhost:8081/api/third-party/oauth2/clients/660e8400-e29b-41d4-a716-446655440001 \
  -H "X-Remote-User: admin@example.com"
```

**Response (409)** if grants exist:
```json
{
  "error": "Cannot delete OAuth2 service with active grants",
  "message": "Service 'GitHub Production' has 15 active grants referencing it. Remove grants first.",
  "grants_count": 15
}
```

**Response (204)** if no grants: No Content

## Error Response Examples

### Validation Error

```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: alice@example.com" \
  -d '{
    "valid_until": "2020-01-01T00:00:00Z",
    "delegated_oauth2_tokens": []
  }'
```

**Response (400)**:
```json
{
  "error": "Validation failed",
  "message": "One or more fields contain invalid values",
  "validation_errors": [
    {
      "field": "valid_until",
      "message": "Timestamp must be in the future",
      "code": "FUTURE_DATE_REQUIRED"
    },
    {
      "field": "delegated_oauth2_tokens",
      "message": "At least one service must be delegated",
      "code": "ARRAY_EMPTY"
    }
  ]
}
```

### Invalid Scope

```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: alice@example.com" \
  -d '{
    "delegated_oauth2_tokens": [
      {
        "thirdparty_oauth2_service_id": "660e8400-e29b-41d4-a716-446655440001",
        "scopes": ["repo", "invalid_scope"]
      }
    ]
  }'
```

**Response (400)**:
```json
{
  "error": "Invalid scopes",
  "message": "Requested scopes do not exist in service configuration",
  "validation_errors": [
    {
      "field": "scopes[1]",
      "message": "Scope 'invalid_scope' does not exist in service",
      "code": "SCOPE_NOT_FOUND"
    }
  ]
}
```

### Unauthorized

```bash
curl http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000
# Missing X-Remote-User header
```

**Response (401)**:
```json
{
  "error": "Unauthorized",
  "message": "Missing or invalid authentication credentials"
}
```

### Not Found

```bash
curl http://localhost:8080/api/consent/agent/99999999-9999-9999-9999-999999999999 \
  -H "X-Remote-User: alice@example.com"
```

**Response (404)**:
```json
{
  "error": "Not found",
  "message": "Agent with ID '99999999-9999-9999-9999-999999999999' not found"
}
```

## Implementation Checklist

### Phase 1: Database Schema
- [ ] Create agents table with indexes
- [ ] Create thirdparty_oauth2_services table with indexes
- [ ] Create oauth2_scopes table with foreign keys
- [ ] Create user_grants table with unique constraint
- [ ] Create delegated_oauth2_tokens table with foreign keys
- [ ] Create delegated_token_scopes table
- [ ] Add cascade delete constraints
- [ ] Test referential integrity
- [ ] Add client_secret encryption functions

### Phase 2: Domain Layer
- [ ] Define Agent, OAuth2Service, UserGrant types
- [ ] Implement validation functions
- [ ] Create repository interfaces (ports)
- [ ] Implement OAuth2 discovery service interface
- [ ] Define domain errors
- [ ] Write unit tests for domain logic

### Phase 3: Persistence Layer
- [ ] Implement AgentRepository (memory + postgres)
- [ ] Implement OAuth2ServiceRepository (memory + postgres)
- [ ] Implement GrantRepository (memory + postgres)
- [ ] Implement client secret encryption/decryption
- [ ] Write repository integration tests
- [ ] Test cascade deletion behavior
- [ ] Test service deletion protection

### Phase 4: HTTP Layer
- [ ] Define request/response DTOs
- [ ] Implement agent handlers (CRUD)
- [ ] Implement OAuth2 service handlers (CRUD)
- [ ] Implement consent handler (GET)
- [ ] Implement grant handlers (GET, POST)
- [ ] Add validation middleware
- [ ] Add audit logging
- [ ] Write HTTP integration tests

### Phase 5: Security
- [ ] Implement admin authorization middleware
- [ ] Test principal extraction from headers
- [ ] Test grant isolation (users can't see others' grants)
- [ ] Add URL validation for governance/documentation URLs
- [ ] Test client secret redaction
- [ ] Implement rate limiting
- [ ] Add security headers
- [ ] Perform security audit

### Phase 6: OAuth2 Discovery
- [ ] Implement metadata URL construction
- [ ] Implement HTTP client for discovery
- [ ] Add SSL certificate validation
- [ ] Test discovery success path
- [ ] Test discovery failure fallback
- [ ] Add retry logic
- [ ] Write discovery integration tests

### Phase 7: Testing
- [ ] Unit tests for all handlers
- [ ] Integration tests for all endpoints
- [ ] End-to-end consent flow test
- [ ] Test expired grant filtering
- [ ] Test upsert semantics
- [ ] Test validation errors
- [ ] Test referential integrity violations
- [ ] Performance tests (100 concurrent requests)

### Phase 8: Documentation
- [ ] Update ARCHITECTURE.md with domain entities
- [ ] Add API examples to README
- [ ] Create Postman collection
- [ ] Generate OpenAPI documentation site
- [ ] Write migration guide (if applicable)
- [ ] Update audit logging documentation

## Common Patterns

### Principal Extraction

```go
// In middleware
principal := r.Header.Get("X-Remote-User")
ctx := principal.WithContext(r.Context(), principal)

// In handler
principal, err := principal.FromContext(r.Context())
if err != nil {
    // Handle missing principal
}
```

### Validation Pattern

```go
type AgentCreateRequest struct {
    DisplayName string `json:"display_name" validate:"required,min=1,max=200"`
    Description string `json:"description" validate:"required,min=1,max=2000"`
    // ...
}

if err := validator.Struct(req); err != nil {
    return ValidationError(err)
}
```

### Repository Pattern

```go
agent, err := h.agentRepo.Get(ctx, agentID)
if err != nil {
    if errors.Is(err, domain.ErrNotFound) {
        return http.StatusNotFound
    }
    return http.StatusInternalServerError
}
```

### Audit Logging

```go
h.auditLogger.Info("Agent created",
    "operation", "agent.create",
    "actor", principal,
    "agent_id", agent.ID,
    "display_name", agent.DisplayName,
)
```

### Error Response

```go
func respondError(w http.ResponseWriter, status int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
```

## Testing Examples

### Unit Test (Handler)

```go
func TestCreateAgent(t *testing.T) {
    mockRepo := &MockAgentRepository{}
    handler := NewAgentHandler(mockRepo, logger)

    body := strings.NewReader(`{
        "oauth2_client_id": "test-agent",
        "display_name": "Test Agent",
        "description": "Test description"
    }`)

    req := httptest.NewRequest("POST", "/api/agents", body)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Remote-User", "admin@example.com")

    w := httptest.NewRecorder()
    handler.CreateAgent(w, req)

    assert.Equal(t, http.StatusCreated, w.Code)
    // Assert response body...
}
```

### Integration Test (Full Flow)

```go
func TestConsentFlow(t *testing.T) {
    // Setup test database
    db := setupTestDB(t)
    defer db.Close()

    // Create agent
    agent := createTestAgent(t, db)

    // Create OAuth2 service
    service := createTestOAuth2Service(t, db)

    // Get consent information
    resp := httpGet(t, fmt.Sprintf("/api/consent/agent/%s", agent.ID))
    assert.Equal(t, http.StatusOK, resp.StatusCode)

    // Create grant
    grantReq := GrantRequest{
        DelegatedOAuth2Tokens: []DelegatedToken{
            {
                ServiceID: service.ID,
                Scopes: []string{"repo"},
            },
        },
    }
    resp = httpPost(t, fmt.Sprintf("/api/consent/agent/%s/grants", agent.ID), grantReq)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)

    // Verify grant exists
    resp = httpGet(t, fmt.Sprintf("/api/consent/agent/%s/grants", agent.ID))
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    // Assert grant details...
}
```

## Performance Targets

| Operation | Target | Measurement |
|-----------|--------|-------------|
| Create agent | < 100ms | p95 response time |
| Get agent info | < 50ms | p95 response time |
| Create OAuth2 service | < 200ms | p95 (including discovery) |
| Get consent info | < 150ms | p95 (agent + all services) |
| Create grant | < 100ms | p95 response time |
| List grants | < 100ms | p95 response time |
| Concurrent requests | 100/sec | No errors, < 500ms p95 |

## Additional Resources

- OpenAPI Specification: `openapi.yaml`
- Design Documentation: `API_DESIGN.md`
- Feature Specification: `../spec.md`
- Architecture Overview: `/ARCHITECTURE.md`
- Development Guide: `/AGENT.md`
