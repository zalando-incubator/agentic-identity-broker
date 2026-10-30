# Data Model: Permission Sets (019)

> **ARCHIVED — provisional draft.** This document predates the positive-inclusion model
> change. The authoritative data model is reflected in the Go domain types under
> `internal/domain/storage/` and the migrations in `migrations/`. Do not rely on this
> document for implementation details.

**Branch**: `20260314-120000-permission-sets` | **Date**: 2026-03-25

---

## New Entities

### PermissionSet

A human-readable, admin-defined bundle of OAuth2 scopes spanning one or more third-party services.

| Field | Type | Constraints |
|-------|------|-------------|
| `ID` | `id.PermissionSetID` (UUID) | Primary key, generated on create |
| `Name` | `string` | Required, unique, max 255 chars |
| `Description` | `string` | Required, non-empty |
| `ServiceScopes` | `[]ServiceScope` | Required, min 1 entry (invariant) |
| `CreatedAt` | `time.Time` | Set on create |
| `UpdatedAt` | `time.Time` | Set on create and update |

**Invariants**:
- `len(ServiceScopes) >= 1` — a permission set must cover at least one service/scope pair
- `Name` must be unique across all permission sets (enforced at DB level via `UNIQUE` constraint)
- Each `ServiceScope.ServiceID` must reference an existing `ThirdpartyOAuth2Service`

**Go type** (`internal/domain/storage/permission_set.go`):
```go
type PermissionSet struct {
    ID            id.PermissionSetID
    Name          string
    Description   string
    ServiceScopes []ServiceScope
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

---

### ServiceScope (Value Object)

Pairs a `ThirdpartyOAuth2Service` reference with a list of OAuth2 scopes. Immutable within a `PermissionSet`.

| Field | Type | Constraints |
|-------|------|-------------|
| `ServiceID` | `id.ServiceID` (UUID) | References `thirdparty_oauth2_services.id`, FK with RESTRICT |
| `Scopes` | `[]string` | Required, min 1 scope string |

**Go type** (`internal/domain/storage/permission_set.go`):
```go
type ServiceScope struct {
    ServiceID id.ServiceID
    Scopes    []string
}
```

---

## Modified Entities

### Agent (extended)

Gains a `PermissionSets` field — an ordered list of `AgentPermissionSetEntry` entries. Orthogonal to `ServiceRequirements`.

**New field**:

| Field | Type | Constraints |
|-------|------|-------------|
| `PermissionSets` | `[]AgentPermissionSetEntry` | Required min 1 entry on create/update (422 if empty). All IDs must exist (422 if invalid). |

**Go type** (`internal/domain/storage/agent.go` — new nested type):
```go
// AgentPermissionSetEntry is one element of Agent.PermissionSets.
// Ordering in the slice reflects declaration order (preserved in storage).
type AgentPermissionSetEntry struct {
    PermissionSetID id.PermissionSetID
    RequirementType RequirementType // "mandatory" | "optional"
}
```

**Storage** (PostgreSQL): stored as `permission_sets JSONB` column on `agents` table (same pattern as `service_requirements`).

```json
// agents.permission_sets column value example:
[
  {"permission_set_id": "uuid-1", "requirement_type": "mandatory"},
  {"permission_set_id": "uuid-2", "requirement_type": "optional"}
]
```

---

### UserGrant (modified)

The `DelegatedOAuth2Tokens []DelegatedToken` field is **removed**. A new `GrantedPermissionSetIDs []id.PermissionSetID` field is added as a flat list of UUIDs.

| Field removed | Reason |
|---------------|--------|
| `DelegatedOAuth2Tokens []DelegatedToken` | Replaced by permission set model; per-service scope grouping is now derived at token exchange time (FR-016) |

| Field added | Type | Constraints |
|------------|------|-------------|
| `GrantedPermissionSetIDs` | `[]id.PermissionSetID` | Flat list; all mandatory IDs + user-selected optional IDs. May be empty (for grants created before migration, which are deleted). Min 1 for new grants (validated application-side). |

**Go type** (`internal/domain/storage/user_grant.go` — modified):
```go
type UserGrant struct {
    ID                      id.GrantID
    Principal               id.Principal
    AgentID                 id.AgentID
    ValidUntil              *time.Time
    GrantedPermissionSets   []GrantedPermissionSetEntry  // NEW: structured positive-inclusion, replaces DelegatedOAuth2Tokens
    CreatedAt               time.Time
    UpdatedAt               time.Time
}

// GrantedPermissionSetEntry pairs a granted PS with the explicit list of service IDs the user included.
type GrantedPermissionSetEntry struct {
    PermissionSetID  id.PermissionSetID
    IncludedServiceIDs []id.ServiceID
}
```

**Storage** (PostgreSQL): `granted_permission_sets JSONB` — structured map `{ps_id: [included_service_ids]}` stored as JSONB for flexible querying.

---

## New Typed ID

| Type | Entity | File |
|------|--------|------|
| `PermissionSetID` | `permission_set` | `internal/domain/id/gen_ids.go` |

Generator entry to add: `{"PermissionSetID", "permission_set"}`

Run after: `go generate ./internal/domain/id/`

---

## New Port Interface

**`PermissionSetRepository`** (`internal/ports/storage.go`):

```go
type PermissionSetRepository interface {
    // Create stores a new permission set. Returns Conflict if name already exists.
    Create(ctx context.Context, ps *storage.PermissionSet) error

    // Get retrieves a permission set by ID. Returns NotFound if absent.
    Get(ctx context.Context, id id.PermissionSetID) (*storage.PermissionSet, error)

    // GetByIDs retrieves multiple permission sets by IDs in one round-trip.
    // Returns all found sets; IDs not found are silently absent (caller validates).
    GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error)

    // Update replaces a permission set's name, description, and service scopes.
    // Returns NotFound if absent.
    Update(ctx context.Context, ps *storage.PermissionSet) error

    // Delete removes a permission set by ID. Returns NotFound if absent.
    // Caller MUST check CountAgentsReferencingPermissionSet before calling Delete.
    Delete(ctx context.Context, id id.PermissionSetID) error

    // List returns all permission sets, optionally filtered by service ID.
    // serviceID=zero-value means no filter (return all).
    List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error)

    // CountAgentsReferencingPermissionSet counts agents whose permission_sets JSONB
    // list contains the given permission set ID.
    // Used for deletion protection (FR-004, SR-002).
    CountAgentsReferencingPermissionSet(ctx context.Context, id id.PermissionSetID) (int, error)

    // CountPermissionSetsForService counts permission sets that contain a ServiceScope
    // for the given service ID.
    // Used to block ThirdpartyOAuth2Service deletion (DB-006 application layer).
    CountPermissionSetsForService(ctx context.Context, serviceID id.ServiceID) (int, error)
}
```

---

## Database Schema

### New Table: `permission_sets`

```sql
CREATE TABLE permission_sets (
    id          UUID PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT         NOT NULL,
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_permission_set_name UNIQUE (name)
);
```

### New Table: `permission_set_service_scopes`

```sql
CREATE TABLE permission_set_service_scopes (
    permission_set_id UUID   NOT NULL REFERENCES permission_sets(id) ON DELETE CASCADE,
    service_id        UUID   NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE RESTRICT,
    scopes            JSONB  NOT NULL,  -- string[]
    PRIMARY KEY (permission_set_id, service_id)
);
```

### Agent Table: New Column

```sql
ALTER TABLE agents
    ADD COLUMN permission_sets JSONB DEFAULT NULL;

-- GIN index for JSONB containment queries (deletion protection check)
CREATE INDEX idx_agents_permission_sets ON agents USING GIN (permission_sets);
```

Stored format (same as `service_requirements`):
```json
[{"permission_set_id": "<uuid>", "requirement_type": "mandatory"|"optional"}]
```

### UserGrants Table: Modified

Migration 010 (destructive — see research.md §8):
```sql
-- Delete all existing grants (FR-016: force re-consent)
DELETE FROM user_grants;

-- Add new column (JSONB for structured {ps_id: [service_ids]} map)
ALTER TABLE user_grants
    ADD COLUMN granted_permission_sets JSONB NOT NULL DEFAULT '{}';

-- Remove old column
ALTER TABLE user_grants
    DROP COLUMN delegated_oauth2_tokens;

-- GIN index for JSONB containment queries
CREATE INDEX idx_grants_permission_sets ON user_grants USING GIN (granted_permission_sets);
```

---

## Domain Events

| Event | Fields | Emitted By |
|-------|--------|------------|
| `PermissionSetCreated` | `permission_set_id`, `name` | Admin create handler |
| `PermissionSetUpdated` | `permission_set_id`, `name` | Admin update handler |
| `PermissionSetDeleted` | `permission_set_id`, `name` | Admin delete handler |
| `ConsentGrantedWithPermissionSets` | `principal`, `agent_id`, `granted_permission_sets: {ps_id: [included_service_ids]}` | Consent grant handler |

All events are emitted as structured audit log entries (SR-004).

---

## State Transitions

### PermissionSet Lifecycle

```
[None] --create--> [Active] --update--> [Active]
[Active] --delete (if unused)--> [Gone]
[Active] --delete (if in use by agent)--> [Active] (409 rejected)
[Active] --service deleted (if referenced)--> [Active] (409 rejected at service level)
```

### UserGrant with Permission Sets

```
[None] --consent submitted--> [Granted: {mandatory[] + selected optional[]}]
[Granted] --re-consent--> [Granted: {mandatory[] + new optional selection}] (upsert)
[Granted] --token exchange (scopes satisfied)--> [Exchanged: returns granted_permission_sets]
[Granted] --token exchange (scopes missing)--> [Error: FR-018]
```

---

## Consent Domain Service: Extended ConsentInfo

`consent.AgentConsentInfo` gains a resolved permission sets list:

```go
// ResolvedPermissionSetEntry pairs a resolved PermissionSet with its requirement type
// for a specific agent's declaration.
type ResolvedPermissionSetEntry struct {
    PermissionSet   *storage.PermissionSet
    RequirementType storage.RequirementType
}

// AgentConsentInfo extended (existing fields preserved)
type AgentConsentInfo struct {
    Agent                   *storage.Agent
    AvailableServices       []*model.ThirdpartyOAuth2ProviderEntity // NEW (populated via ThirdpartyOAuth2ProviderService.List())
    ResolvedPermissionSets  []ResolvedPermissionSetEntry            // NEW (populated via PermissionSetService.GetByIDs())
    ActiveSessionServiceIDs []id.ServiceID                          // NEW (for US2 SC-003)
}
```

---

## Glossary Updates (ARCHITECTURE.md)

| Term | Definition |
|------|------------|
| **PermissionSet** | Admin-defined bundle of OAuth2 scopes spanning one or more third-party services. Has a stable UUID, display name, description, and `ServiceScope` list. |
| **ServiceScope** | Value object within a `PermissionSet` pairing a `ThirdpartyOAuth2Service` reference with a list of OAuth2 scopes. |
| **AgentPermissionSetEntry** | One element of `Agent.PermissionSets` — pairs a `PermissionSetID` with a `RequirementType`. |
| **granted_permission_sets** | Structured JSONB stored in `UserGrant`; maps each granted permission set ID to the explicit list of included service IDs submitted during consent (positive-inclusion model). Scopes derived at token exchange time by resolving PS definitions and intersecting with SR scope ceiling. |
