# Data Model: Domain Model and Consent APIs

**Feature**: 20251217-120000-domain-model-apis
**Date**: 2025-12-17
**Status**: Design Complete

## Overview

This document defines the three core domain entities (Agent, ThirdpartyOAuth2Service, UserGrant) and their relationships for the identity broker's domain model. All entities follow Domain-Driven Design principles with clear validation rules and boundaries.

## Entity Relationship Diagram

```
┌──────────────────┐
│     Agent        │
│                  │
│ - id (PK)        │◀──┐
│ - client_id      │   │
│ - display_name   │   │ Many
│ - description    │   │ Grants
│ - external_id?   │   │
│ - governance_url?│   │
│ - ...            │   │
└──────────────────┘   │
                       │
                       │
         ┌─────────────┴──────────────────┐
         │        UserGrant                │
         │                                 │
         │ - id (PK)                       │
         │ - principal                     │
         │ - agent_id (FK)                 │
         │ - valid_until?                  │
         │ - delegated_oauth2_tokens[]     │
         │   - thirdparty_oauth2_service_id│──┐
         │   - scopes[]                    │  │
         └─────────────────────────────────┘  │
                                              │
                                              │ References
                                              │
         ┌────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────┐
│  ThirdpartyOAuth2Service        │
│                                 │
│ - id (PK)                       │
│ - display_name                  │
│ - client_id                     │
│ - client_secret (encrypted)     │
│ - issuer_uri                    │
│ - discovery                     │
│   - enable_discovery            │
│   - metadata_url?               │
│ - endpoints                     │
│   - token_endpoint              │
│   - authorize_endpoint          │
│ - scopes[]                      │
│   - scope_value                 │
│   - description                 │
└─────────────────────────────────┘
```

**Relationships**:
- **Agent** → **UserGrant**: One-to-Many (one agent has many grants)
- **UserGrant** → **Agent**: Many-to-One (many grants reference one agent, CASCADE DELETE)
- **UserGrant** → **ThirdpartyOAuth2Service**: Many-to-Many (via JSONB array in delegated_oauth2_tokens)
- **ThirdpartyOAuth2Service** → **UserGrant**: Referenced (BLOCK DELETE if grants exist)

---

## Entity 1: Agent

**Purpose**: Represents an AI agent registered in the identity broker system.

**Location**: `internal/ports/agent.go`

### Fields

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| `id` | string (UUID) | Yes | Unique identifier | UUID v4, generated on creation |
| `client_id` | string | Yes | OAuth2 client ID for this agent | Non-empty, used in OAuth2 flows |
| `external_id` | *string | No | Optional external governance system identifier | Nullable |
| `display_name` | string | Yes | Human-readable display name | Non-empty, max 255 chars |
| `description` | string | Yes | Human-readable description of agent purpose | Non-empty, max 1000 chars |
| `governance_url` | *string | No | Optional URL to governance documentation | Nullable, valid HTTP/HTTPS URL |
| `user_documentation_url` | *string | No | Optional URL to user documentation | Nullable, valid HTTP/HTTPS URL |
| `agent_interface_url` | *string | No | Optional URL to agent interface/chat | Nullable, valid HTTP/HTTPS URL |
| `created_at` | time.Time | Yes | Creation timestamp | Auto-generated on insert |
| `updated_at` | time.Time | Yes | Last update timestamp | Auto-updated on modify |

### Validation Rules

```go
func (a *Agent) Validate() error {
    // Required fields
    if a.ID == "" {
        return errors.New("agent ID cannot be empty")
    }
    if a.ClientID == "" {
        return errors.New("client_id is required")
    }
    if a.DisplayName == "" {
        return errors.New("display_name is required")
    }
    if len(a.DisplayName) > 255 {
        return errors.New("display_name exceeds 255 characters")
    }
    if a.Description == "" {
        return errors.New("description is required")
    }
    if len(a.Description) > 1000 {
        return errors.New("description exceeds 1000 characters")
    }

    // URL validation (SR-011: prevent injection attacks)
    if a.GovernanceURL != nil && !isValidURL(*a.GovernanceURL) {
        return errors.New("governance_url is not a valid HTTP/HTTPS URL")
    }
    if a.UserDocumentationURL != nil && !isValidURL(*a.UserDocumentationURL) {
        return errors.New("user_documentation_url is not a valid HTTP/HTTPS URL")
    }
    if a.AgentInterfaceURL != nil && !isValidURL(*a.AgentInterfaceURL) {
        return errors.New("agent_interface_url is not a valid HTTP/HTTPS URL")
    }

    return nil
}

func isValidURL(urlStr string) bool {
    u, err := url.Parse(urlStr)
    if err != nil {
        return false
    }
    return u.Scheme == "http" || u.Scheme == "https"
}
```

### Database Schema

```sql
CREATE TABLE agents (
    id UUID PRIMARY KEY,
    client_id VARCHAR(255) NOT NULL UNIQUE,
    external_id VARCHAR(255),
    display_name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    governance_url TEXT,
    user_documentation_url TEXT,
    agent_interface_url TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agents_client_id ON agents(client_id);
CREATE INDEX idx_agents_external_id ON agents(external_id) WHERE external_id IS NOT NULL;
```

### JSON Representation

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "client_id": "agent-github-assistant",
  "external_id": "GOV-2024-001",
  "display_name": "GitHub Assistant",
  "description": "AI agent that helps manage GitHub repositories and issues",
  "governance_url": "https://governance.example.com/agents/github-assistant",
  "user_documentation_url": "https://docs.example.com/agents/github-assistant",
  "agent_interface_url": "https://chat.example.com/github-assistant",
  "created_at": "2025-12-17T10:00:00Z",
  "updated_at": "2025-12-17T10:00:00Z"
}
```

---

## Entity 2: ThirdpartyOAuth2Service

**Purpose**: Represents an external OAuth2 provider (Google, GitHub, Databricks, etc.) that agents can access on behalf of users.

**Location**: `internal/ports/thirdparty_service.go`

### Fields

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| `id` | string (UUID) | Yes | Unique identifier | UUID v4, generated on creation |
| `display_name` | string | Yes | Human-readable service name | Non-empty, max 255 chars |
| `client_id` | string | Yes | OAuth2 client ID for this service | Non-empty |
| `client_secret` | string | Yes | OAuth2 client secret | Encrypted at rest (AES-256-GCM) |
| `issuer_uri` | string | Yes | OAuth2 issuer URI | Valid HTTPS URL |
| `discovery.enable_discovery` | bool | Yes | Enable automatic endpoint discovery | Default: true |
| `discovery.metadata_url` | *string | No | Optional custom metadata URL | Nullable, valid HTTPS URL |
| `endpoints.token_endpoint` | string | Conditional | OAuth2 token endpoint | Required if discovery disabled |
| `endpoints.authorize_endpoint` | string | Conditional | OAuth2 authorization endpoint | Required if discovery disabled |
| `scopes` | []OAuthScope | Yes | Available OAuth2 scopes | At least one scope required |
| `created_at` | time.Time | Yes | Creation timestamp | Auto-generated on insert |
| `updated_at` | time.Time | Yes | Last update timestamp | Auto-updated on modify |

### Nested Type: OAuthScope

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `scope_value` | string | Yes | OAuth2 scope value (e.g., "repo", "user:email") |
| `description` | string | Yes | Human-readable description of scope |

### Validation Rules

```go
func (s *ThirdpartyOAuth2Service) Validate() error {
    // Required fields
    if s.ID == "" {
        return errors.New("service ID cannot be empty")
    }
    if s.DisplayName == "" {
        return errors.New("display_name is required")
    }
    if s.ClientID == "" {
        return errors.New("client_id is required")
    }
    if s.ClientSecret == "" {
        return errors.New("client_secret is required")
    }
    if s.IssuerURI == "" {
        return errors.New("issuer_uri is required")
    }

    // Validate issuer_uri is HTTPS
    issuerURL, err := url.Parse(s.IssuerURI)
    if err != nil || issuerURL.Scheme != "https" {
        return errors.New("issuer_uri must be a valid HTTPS URL")
    }

    // If discovery disabled, endpoints are required
    if !s.Discovery.EnableDiscovery {
        if s.Endpoints.TokenEndpoint == "" {
            return errors.New("token_endpoint is required when discovery is disabled")
        }
        if s.Endpoints.AuthorizeEndpoint == "" {
            return errors.New("authorize_endpoint is required when discovery is disabled")
        }
    }

    // Validate metadata_url if provided
    if s.Discovery.MetadataURL != nil && !isValidURL(*s.Discovery.MetadataURL) {
        return errors.New("metadata_url must be a valid HTTPS URL")
    }

    // At least one scope required
    if len(s.Scopes) == 0 {
        return errors.New("at least one scope is required")
    }

    // Validate each scope
    for i, scope := range s.Scopes {
        if scope.ScopeValue == "" {
            return fmt.Errorf("scope %d: scope_value is required", i)
        }
        if scope.Description == "" {
            return fmt.Errorf("scope %d: description is required", i)
        }
    }

    return nil
}
```

### Database Schema

```sql
CREATE TABLE thirdparty_oauth2_services (
    id UUID PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    client_secret_encrypted BYTEA NOT NULL,  -- AES-256-GCM encrypted
    issuer_uri TEXT NOT NULL,
    enable_discovery BOOLEAN NOT NULL DEFAULT true,
    metadata_url TEXT,
    token_endpoint TEXT,
    authorize_endpoint TEXT,
    scopes JSONB NOT NULL,  -- Array of {scope_value, description}
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_services_display_name ON thirdparty_oauth2_services(display_name);
CREATE INDEX idx_services_scopes ON thirdparty_oauth2_services USING GIN(scopes);
```

### JSON Representation

```json
{
  "id": "650e8400-e29b-41d4-a716-446655440001",
  "display_name": "GitHub",
  "client_id": "Iv1.abcd1234",
  "client_secret": "REDACTED",
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
      "scope_value": "user:email",
      "description": "Access user email addresses"
    }
  ],
  "created_at": "2025-12-17T10:00:00Z",
  "updated_at": "2025-12-17T10:00:00Z"
}
```

**Note**: `client_secret` is always `"REDACTED"` in API responses (SR-003). Only accepted in POST/PUT requests.

---

## Entity 3: UserGrant

**Purpose**: Records a user (principal) delegating specific permissions to an agent for one or more third-party OAuth2 services.

**Location**: `internal/ports/user_grant.go`

### Fields

| Field | Type | Required | Description | Constraints |
|-------|------|----------|-------------|-------------|
| `id` | string (UUID) | Yes | Unique identifier | UUID v4, generated on creation |
| `principal` | string | Yes | User identifier (from session) | Non-empty, derived from auth session |
| `agent_id` | string (UUID) | Yes | Agent receiving delegation | Foreign key to agents.id (CASCADE DELETE) |
| `valid_until` | *time.Time | No | Grant expiration timestamp | Nullable (null = indefinite), must be in future |
| `delegated_oauth2_tokens` | []DelegatedToken | Yes | Delegated third-party services | At least one delegation required |
| `created_at` | time.Time | Yes | Creation timestamp | Auto-generated on insert |
| `updated_at` | time.Time | Yes | Last update timestamp | Auto-updated on modify |

### Nested Type: DelegatedToken

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `thirdparty_oauth2_service_id` | string (UUID) | Yes | Service ID being delegated |
| `scopes` | []string | Yes | Array of scope values (e.g., ["repo", "user:email"]) |

### Validation Rules

```go
func (g *UserGrant) Validate() error {
    // Required fields
    if g.ID == "" {
        return errors.New("grant ID cannot be empty")
    }
    if g.Principal == "" {
        return errors.New("principal is required")
    }
    if g.AgentID == "" {
        return errors.New("agent_id is required")
    }

    // valid_until must be in future if provided
    if g.ValidUntil != nil && g.ValidUntil.Before(time.Now()) {
        return errors.New("valid_until must be in the future")
    }

    // At least one delegation required
    if len(g.DelegatedOAuth2Tokens) == 0 {
        return errors.New("at least one delegated service is required")
    }

    // Validate each delegation
    for i, token := range g.DelegatedOAuth2Tokens {
        if token.ThirdpartyOAuth2ServiceID == "" {
            return fmt.Errorf("delegation %d: thirdparty_oauth2_service_id is required", i)
        }
        if len(token.Scopes) == 0 {
            return fmt.Errorf("delegation %d: at least one scope is required", i)
        }
        // Validate no duplicate scopes
        scopeSet := make(map[string]bool)
        for _, scope := range token.Scopes {
            if scopeSet[scope] {
                return fmt.Errorf("delegation %d: duplicate scope '%s'", i, scope)
            }
            scopeSet[scope] = true
        }
    }

    return nil
}
```

### Database Schema

```sql
CREATE TABLE user_grants (
    id UUID PRIMARY KEY,
    principal VARCHAR(255) NOT NULL,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    valid_until TIMESTAMP,  -- NULL = indefinite
    delegated_oauth2_tokens JSONB NOT NULL,  -- Array of {thirdparty_oauth2_service_id, scopes[]}
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- One grant per user-agent pair (upsert semantics)
    CONSTRAINT uq_principal_agent UNIQUE(principal, agent_id)
);

CREATE INDEX idx_grants_principal ON user_grants(principal);
CREATE INDEX idx_grants_agent ON user_grants(agent_id);
CREATE INDEX idx_grants_active ON user_grants(principal, agent_id)
    WHERE valid_until IS NULL OR valid_until > NOW();
CREATE INDEX idx_grants_tokens ON user_grants USING GIN(delegated_oauth2_tokens);
```

### JSON Representation

```json
{
  "id": "750e8400-e29b-41d4-a716-446655440002",
  "principal": "user123@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "valid_until": "2026-12-17T10:00:00Z",
  "delegated_oauth2_tokens": [
    {
      "thirdparty_oauth2_service_id": "650e8400-e29b-41d4-a716-446655440001",
      "scopes": ["repo", "user:email"]
    },
    {
      "thirdparty_oauth2_service_id": "650e8400-e29b-41d4-a716-446655440003",
      "scopes": ["read:user", "read:org"]
    }
  ],
  "created_at": "2025-12-17T10:00:00Z",
  "updated_at": "2025-12-17T10:00:00Z"
}
```

**Indefinite Grant Example**:
```json
{
  "id": "750e8400-e29b-41d4-a716-446655440003",
  "principal": "user456@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "valid_until": null,
  "delegated_oauth2_tokens": [
    {
      "thirdparty_oauth2_service_id": "650e8400-e29b-41d4-a716-446655440001",
      "scopes": ["repo"]
    }
  ],
  "created_at": "2025-12-17T10:00:00Z",
  "updated_at": "2025-12-17T10:00:00Z"
}
```

---

## Domain Rules

### Grant Cardinality
- **One grant per user-agent pair** (enforced via `UNIQUE(principal, agent_id)` constraint)
- Subsequent grant requests with same `principal + agent_id` update existing grant (upsert semantics)
- To revoke: submit POST with empty `delegated_oauth2_tokens` array (deletes grant)

### Grant Expiration
- **Nullable `valid_until`**: `NULL` = indefinite grant (never expires)
- **Future timestamp**: Grant expires when `valid_until < NOW()`
- **Query filtering**: Active grants filtered with `WHERE valid_until IS NULL OR valid_until > NOW()`
- **Validation**: Cannot create/update grant with `valid_until` in the past

### Cascade Deletion
- **Agent deleted** → All associated grants CASCADE deleted (FR-021)
- **Service deletion blocked** → If grants reference service (FR-022, returns 409 Conflict)

### Scope Validation
- **All scopes must exist** in referenced service's `scopes` array (FR-018)
- **Invalid scopes rejected** with validation error detailing non-existent scopes

### Client Secret Security
- **Encrypted at rest** using AES-256-GCM (SR-002)
- **Redacted in responses** via `RedactedCopy()` method (SR-003)
- **Never logged** in audit logs or error messages

### Principal Isolation
- **Derived from session** (FR-017)
- **Fail closed** if extraction fails (SR-006)
- **Principal-based filtering** on all grant queries (SR-004, SR-010)

---

## State Transitions

### UserGrant Lifecycle

```
┌─────────────┐
│   CREATE    │  POST /api/consent/agent/:id/grants
└──────┬──────┘  (with services + scopes + optional valid_until)
       │
       ▼
┌─────────────┐
│   ACTIVE    │  Grant is active (valid_until NULL or > NOW())
│             │  - Visible in GET /api/consent/agent/:id/grants
│             │  - Used for authorization
└──────┬──────┘
       │
       ├───────── UPDATE (UPSERT) ────┐
       │         POST same agent_id   │
       │         Updates scopes        │
       │         or valid_until        │
       │                               │
       ▼                               │
┌─────────────┐                       │
│  EXPIRED    │  valid_until < NOW()  │
│             │  - Filtered from      │
│             │    active listings    │
│             │  - Cannot be used     │
└──────┬──────┘                       │
       │                               │
       └──────── CLEANUP ──────────────┘
                (optional periodic purge)

       Parallel path:
       │
       └───────── REVOKE ──────────────┐
                POST empty scopes array │
                                        │
                                        ▼
                                  ┌─────────────┐
                                  │   DELETED   │
                                  │             │
                                  └─────────────┘
```

**State Definitions**:
- **ACTIVE**: `valid_until IS NULL OR valid_until > NOW()`
- **EXPIRED**: `valid_until IS NOT NULL AND valid_until <= NOW()`
- **DELETED**: Row removed from database

---

## Glossary

**Terms to add to ARCHITECTURE.md Glossary**:

- **Agent**: An AI agent registered in the identity broker, capable of acting on behalf of users with delegated permissions.

- **ThirdpartyOAuth2Service**: An external OAuth2 provider (e.g., GitHub, Google) that agents can access using delegated user credentials.

- **OAuth Scope**: A permission scope defined by an OAuth2 provider (e.g., `repo`, `user:email`) that determines what resources can be accessed.

- **User Grant**: A record of a user delegating specific OAuth2 scopes to an agent for one or more third-party services, with optional expiration.

- **Principal**: A unique identifier for an authenticated user, derived from the session management system.

- **Delegated Token**: A component of a user grant specifying which third-party service and scopes are delegated to an agent.

- **Grant Expiration**: The point at which a user grant becomes inactive (`valid_until < NOW()`); indefinite grants (`valid_until = NULL`) never expire.

---

## References

- **Specification**: [spec.md](spec.md)
- **Research**: [research.md](research.md)
- **Implementation Plan**: [implementation-plan.md](implementation-plan.md)
- **API Contracts**: [contracts/openapi.yaml](contracts/openapi.yaml)
- **Quickstart Guide**: [quickstart.md](quickstart.md)
