# Data Model: Agent Permission Requirements

**Feature**: 20260108-120000-agent-permission-requirements  
**Date**: 2026-01-08  
**Status**: Phase 1 - Data Model Design

## Overview

This feature extends the Agent entity to include service requirements (mandatory and optional third-party OAuth2 services with required scopes). No new entities are created - we add a new value object (ServiceRequirement) and extend the existing Agent entity.

## Entities

### Agent (Extended)

**Description**: Represents an AI agent or autonomous system that users can grant access to their data and services. Extended to include service requirements defining which third-party OAuth2 services the agent needs to operate.

**Type**: Entity (Aggregate Root)

**Location**: `internal/domain/storage/agent.go` (existing, to be extended)

**Fields**:

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| ID | UUID | Required, Unique | Agent identifier |
| ClientID | string | Required, Unique | OAuth2 client_id |
| ClientSecret | string | Required, Encrypted | OAuth2 client_secret (encrypted at rest) |
| Name | string | Required, 1-100 chars | Display name |
| Description | string | Optional, max 500 chars | Agent description |
| RedirectURIs | []string | Required, non-empty | Valid OAuth2 redirect URIs |
| **ServiceRequirements** | **[]ServiceRequirement** | **Optional, can be empty** | **NEW: Third-party services required by agent** |
| CreatedAt | timestamp | Required | Creation timestamp |
| UpdatedAt | timestamp | Required | Last update timestamp |

**Validation Rules**:
- ServiceRequirements array can be empty or NULL (backward compatible)
- ServiceRequirements MUST NOT contain duplicate service_id values (FR-003a)
- Each ServiceRequirement MUST pass ServiceRequirement validation rules

**Business Rules**:
- When ServiceRequirements is NULL or empty, agent authorization skips requirement checks (FR-015)
- ServiceRequirements can only be set/updated via Admin API (end-user API is read-only)
- Deleting a ThirdPartyOAuth2Service referenced in ServiceRequirements makes validation fail (edge case handled at authorization time)

**Relationships**:
- Has many UserGrants (existing)
- References ThirdPartyOAuth2Services via ServiceRequirement.ServiceID (not enforced at DB level)

## Value Objects

### ServiceRequirement (New)

**Description**: Represents a single third-party OAuth2 service that an agent requires (mandatory) or can optionally use (optional). Includes the required OAuth2 scopes that must be granted.

**Type**: Value Object (immutable)

**Location**: `internal/domain/storage/service_requirement.go` (new file)

**Fields**:

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| ServiceID | UUID | Required, Valid UUID | Reference to ThirdPartyOAuth2Service.ID |
| RequirementType | RequirementType | Required, enum | Whether service is "mandatory" or "optional" |
| RequiredScopes | []string | Required, non-empty | OAuth2 scopes that must be granted |

**Validation Rules**:
- ServiceID MUST be a valid UUID format (FR-002)
- ServiceID MUST reference an existing ThirdPartyOAuth2Service (FR-003) - validated at application layer
- RequirementType MUST be either "mandatory" or "optional" (FR-002)
- RequiredScopes array MUST NOT be empty (FR-002)
- All scopes in RequiredScopes MUST exist in the referenced service's scope configuration (FR-004) - validated at application layer
- Scope names are case-sensitive (SR-006)

**Immutability**:
- ServiceRequirement is immutable once created
- Changes to agent's service requirements replace the entire array (not individual updates)

**Storage**:
- Stored as JSONB in Agent entity (single column, not separate table)
- JSON Schema:
```json
{
  "service_id": "550e8400-e29b-41d4-a716-446655440000",
  "requirement_type": "mandatory",
  "required_scopes": ["user:email", "read:user"]
}
```

## Enumerations

### RequirementType (New)

**Description**: Enum representing whether a third-party service is mandatory (agent cannot function without it) or optional (agent can use it if available).

**Type**: String Enum

**Location**: `internal/domain/storage/requirement_type.go` (new file)

**Values**:

| Value | Description |
|-------|-------------|
| `mandatory` | Agent MUST have access to this service to function. Authorization is blocked until requirement is satisfied. |
| `optional` | Agent CAN use this service if available, but doesn't require it. Authorization proceeds even if requirement is not satisfied. |

**Validation**:
- MUST be exactly "mandatory" or "optional" (lowercase)
- Case-sensitive (reject "Mandatory", "MANDATORY", etc.)

**Usage**:
- Authorization endpoint checks only "mandatory" requirements (FR-014)
- Consent screen displays both types with different visual styling (FR-017)
- Optional requirements do not block authorization flow (FR-014)

## Domain Events

### AgentRequirementsUpdated (New)

**Description**: Fired when an agent's service requirements are created or modified through the Admin API.

**Type**: Domain Event

**Payload**:

| Field | Type | Description |
|-------|------|-------------|
| AgentID | UUID | Agent whose requirements changed |
| OldRequirements | []ServiceRequirement | Previous requirements (NULL for first-time creation) |
| NewRequirements | []ServiceRequirement | New requirements after update |
| UpdatedBy | string | Principal ID of admin who made the change |
| Timestamp | timestamp | When the change occurred |

**Triggered By**:
- `POST /api/agents` with service_requirements (FR-005)
- `PUT /api/agents/{agent-id}` with modified service_requirements (FR-006)

**Consumers**:
- Audit logging system (SR-004)
- Future: notification system to alert users of affected grants

## Relationships

```
Agent (1) ─────contains────> (*) ServiceRequirement [value object]
                                        │
                                        │ references (not FK)
                                        ▼
                              ThirdPartyOAuth2Service (1)
                                        │
                                        │ has
                                        ▼
                                   (*) OAuthScope [value object]
```

**Notes**:
- ServiceRequirement → ThirdPartyOAuth2Service: Logical reference via ServiceID (no DB foreign key constraint)
- ServiceRequirement → OAuthScope: Scopes validated at application layer against service's scope list
- Validation happens at API layer (Admin handler) before persisting Agent

## Storage Schema

### Database Migration: 005_add_agent_service_requirements

**Up Migration** (`005_add_agent_service_requirements.up.sql`):

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

**Down Migration** (`005_add_agent_service_requirements.down.sql`):

```sql
-- Remove index first
DROP INDEX IF EXISTS idx_agents_service_requirements;

-- Remove column (data loss warning: existing service requirements will be deleted)
ALTER TABLE agents DROP COLUMN IF EXISTS service_requirements;
```

**JSONB Structure**:
```json
[
  {
    "service_id": "550e8400-e29b-41d4-a716-446655440000",
    "requirement_type": "mandatory",
    "required_scopes": ["user:email", "read:user"]
  },
  {
    "service_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "requirement_type": "optional",
    "required_scopes": ["chat:write"]
  }
]
```

**Backward Compatibility**:
- NULL value = no service requirements (existing agents)
- Empty array `[]` = agent explicitly has no requirements
- Authorization logic: `if service_requirements IS NULL OR jsonb_array_length(service_requirements) = 0 then skip requirement checks`

## Validation Flow

### Creation/Update (Admin API)

```
1. Admin POST/PUT request with service_requirements
2. Domain layer: Agent.Validate()
   - Check structure: service_id format, requirement_type enum, scopes non-empty
   - Check for duplicate service_id values (FR-003a)
3. Application layer: AdminHandler validates referential integrity
   - For each ServiceRequirement:
     - Verify service_id exists in ThirdPartyOAuth2Service repository (FR-003)
     - Verify all required_scopes exist in service's scope list (FR-004)
   - If any validation fails: return HTTP 400 with specific error (FR-008, API-007)
4. Storage layer: Persist Agent with service_requirements JSONB
```

### Authorization Validation (OAuth2 Flow)

```
1. User initiates OAuth2 authorization request
2. Authorization endpoint loads Agent entity (includes service_requirements)
3. If service_requirements is NULL or empty: skip to consent check (FR-015)
4. For each ServiceRequirement where requirement_type = "mandatory":
   - Query ThirdPartyOAuth2Session for user + service_id
   - If session not found OR session expired: redirect to consent screen (FR-010, FR-012)
   - If session found: validate scopes (FR-011)
     - Extract session scopes
     - Check session scopes ⊇ required_scopes (superset check)
     - If scope missing: redirect to consent screen (FR-012)
5. If all mandatory requirements satisfied: proceed to consent check
6. Optional requirements do NOT block authorization (FR-014)
```

## API Request/Response Examples

### Admin API: Create Agent with Service Requirements

**Request**: `POST /api/agents`

```json
{
  "client_id": "my-agent-client",
  "client_secret": "secret123",
  "name": "GitHub Bot",
  "description": "Automated GitHub workflow agent",
  "redirect_uris": ["https://example.com/callback"],
  "service_requirements": [
    {
      "service_id": "550e8400-e29b-41d4-a716-446655440000",
      "requirement_type": "mandatory",
      "required_scopes": ["repo", "user:email"]
    },
    {
      "service_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "requirement_type": "optional",
      "required_scopes": ["chat:write"]
    }
  ]
}
```

**Response**: `201 Created`

```json
{
  "id": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
  "client_id": "my-agent-client",
  "name": "GitHub Bot",
  "description": "Automated GitHub workflow agent",
  "redirect_uris": ["https://example.com/callback"],
  "service_requirements": [
    {
      "service_id": "550e8400-e29b-41d4-a716-446655440000",
      "service_name": "GitHub",
      "requirement_type": "mandatory",
      "required_scopes": ["repo", "user:email"]
    },
    {
      "service_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "service_name": "Slack",
      "requirement_type": "optional",
      "required_scopes": ["chat:write"]
    }
  ],
  "created_at": "2026-01-08T17:43:11Z",
  "updated_at": "2026-01-08T17:43:11Z"
}
```

### End-User API: Get Agent with Service Requirements

**Request**: `GET /api/consent/agent/{agent-id}`

**Response**: `200 OK`

```json
{
  "agent": {
    "id": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
    "name": "GitHub Bot",
    "description": "Automated GitHub workflow agent"
  },
  "service_requirements": [
    {
      "service_id": "550e8400-e29b-41d4-a716-446655440000",
      "service_name": "GitHub",
      "requirement_type": "mandatory",
      "required_scopes": [
        {
          "name": "repo",
          "description": "Access to public and private repositories"
        },
        {
          "name": "user:email",
          "description": "Read access to user email address"
        }
      ],
      "user_has_active_session": true
    },
    {
      "service_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "service_name": "Slack",
      "requirement_type": "optional",
      "required_scopes": [
        {
          "name": "chat:write",
          "description": "Send messages to Slack channels"
        }
      ],
      "user_has_active_session": false
    }
  ]
}
```

## Validation Error Examples

### Error: Non-existent service_id

**Request**: `POST /api/agents` with invalid service_id

**Response**: `400 Bad Request`

```json
{
  "error": {
    "code": "INVALID_SERVICE_REFERENCE",
    "message": "Service requirement references non-existent service",
    "details": {
      "service_id": "00000000-0000-0000-0000-000000000000",
      "requirement_index": 0
    }
  }
}
```

### Error: Invalid scope names

**Request**: `POST /api/agents` with scopes not in service

**Response**: `400 Bad Request`

```json
{
  "error": {
    "code": "INVALID_SCOPES",
    "message": "Service requirement contains scopes not defined in service",
    "details": {
      "service_id": "550e8400-e29b-41d4-a716-446655440000",
      "service_name": "GitHub",
      "invalid_scopes": ["nonexistent:scope", "fake:permission"],
      "available_scopes": ["repo", "user:email", "read:user", "admin:org"]
    }
  }
}
```

### Error: Duplicate service_id

**Request**: `POST /api/agents` with same service_id twice

**Response**: `400 Bad Request`

```json
{
  "error": {
    "code": "DUPLICATE_SERVICE_REQUIREMENT",
    "message": "Service requirement contains duplicate service_id",
    "details": {
      "service_id": "550e8400-e29b-41d4-a716-446655440000",
      "duplicate_indices": [0, 2]
    }
  }
}
```

## Next Steps

- [x] Phase 0: Research completed
- [x] Phase 1: Data model completed
- [ ] Phase 1: Generate API contracts in contracts/ directory (OpenAPI specs)
- [ ] Phase 1: Generate quickstart.md implementation guide
- [ ] Phase 1: Update agent context via update-agent-context.sh
- [ ] Phase 1: Re-evaluate Constitution Check post-design
