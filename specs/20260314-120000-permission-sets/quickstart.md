# Quickstart: Permission Sets (019)

**Branch**: `20260314-120000-permission-sets` | **Date**: 2026-03-25

This guide covers the end-to-end developer workflow for implementing the Permission Sets feature.

---

## Prerequisites

```bash
# Start development environment
just dev             # Go backend hot-reload (Air) on :8000 / :14000
just web-dev         # Vite frontend on :3000

# Run checks before committing
just check           # fmt → vet → lint
just verify          # full verification gate
```

---

## Step 0: Codebase Orientation

Key files for this feature:

| File | Role |
|------|------|
| `internal/domain/storage/permission_set.go` | New domain entity (create this) |
| `internal/domain/storage/agent.go` | Extend with `PermissionSets []AgentPermissionSetEntry` |
| `internal/domain/storage/user_grant.go` | Replace `DelegatedOAuth2Tokens` with `GrantedPermissionSetIDs` |
| `internal/domain/id/gen_ids.go` | Add `PermissionSetID` entry |
| `internal/ports/storage.go` | Add `PermissionSetRepository` interface |
| `internal/adapters/storage/memory/permission_set_repository.go` | In-memory implementation |
| `internal/adapters/storage/postgres/permission_set_repository.go` | Postgres implementation |
| `internal/adapters/http/handlers/admin/permission_sets_handler.go` | Admin CRUD handler |
| `internal/domain/consent/service.go` | Extend ConsentInfo with resolved permission sets |
| `internal/domain/tokenexchange/service.go` | Extend response + add PS cache + scope validation |
| `internal/app/builder.go` | Wire new handler and repository |
| `internal/app/handlers.go` | Add `PermissionSets` to `AdminHandlers` |
| `web/src/components/consent/PermissionSetCard.tsx` | New React component |
| `web/src/components/consent/PermissionSetsList.tsx` | New React component |
| `migrations/008_add_permission_sets.{up,down}.sql` | New schema (permission_sets, service_scopes) |
| `migrations/009_add_agent_permission_sets.{up,down}.sql` | Adds permission_sets JSONB to agents |
| `migrations/010_migrate_user_grants_to_permission_sets.{up,down}.sql` | Destructive UserGrant migration |

---

## Step 1: Domain ID — Add PermissionSetID

Edit `internal/domain/id/gen_ids.go`:

```go
var uuidTypes = []struct{ Type, Entity string }{
    {"AgentID", "agent"},
    {"ServiceID", "service"},
    {"GrantID", "grant"},
    {"SessionID", "session"},
    {"UserID", "user"},
    {"PermissionSetID", "permission_set"},  // ADD THIS
}
```

Regenerate:

```bash
cd internal/domain/id && go generate .
```

---

## Step 2: Domain Entities

### 2a. New: PermissionSet (`internal/domain/storage/permission_set.go`)

```go
package storage

import (
    "time"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type ServiceScope struct {
    ServiceID id.ServiceID
    Scopes    []string
}

type PermissionSet struct {
    ID            id.PermissionSetID
    Name          string
    Description   string
    ServiceScopes []ServiceScope
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### 2b. Extend Agent (`internal/domain/storage/agent.go`)

Add new type and field:

```go
type AgentPermissionSetEntry struct {
    PermissionSetID id.PermissionSetID
    RequirementType RequirementType
}

type Agent struct {
    // ... existing fields ...
    PermissionSets []AgentPermissionSetEntry  // NEW: ordered, min 1 required
}
```

### 2c. Modify UserGrant (`internal/domain/storage/user_grant.go`)

Remove `DelegatedOAuth2Tokens []DelegatedToken`. Add `GrantedPermissionSetIDs`:

```go
type UserGrant struct {
    ID                      id.GrantID
    Principal               id.Principal
    AgentID                 id.AgentID
    ValidUntil              *time.Time
    GrantedPermissionSetIDs []id.PermissionSetID  // replaces DelegatedOAuth2Tokens
    CreatedAt               time.Time
    UpdatedAt               time.Time
}
```

---

## Step 3: Storage Port

Add `PermissionSetRepository` to `internal/ports/storage.go`. See `data-model.md` for the full interface definition.

---

## Step 4: Database Migrations

Create three migration files in `/migrations/`:

### 008 — Permission sets tables
```sql
-- 008_add_permission_sets.up.sql
CREATE TABLE permission_sets (
    id          UUID         PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    description TEXT         NOT NULL,
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_permission_set_name UNIQUE (name)
);

CREATE TABLE permission_set_service_scopes (
    permission_set_id UUID  NOT NULL REFERENCES permission_sets(id) ON DELETE CASCADE,
    service_id        UUID  NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE RESTRICT,
    scopes            JSONB NOT NULL,
    PRIMARY KEY (permission_set_id, service_id)
);
```

### 009 — Agent permission_sets column
```sql
-- 009_add_agent_permission_sets.up.sql
ALTER TABLE agents ADD COLUMN permission_sets JSONB DEFAULT NULL;
CREATE INDEX idx_agents_permission_sets ON agents USING GIN (permission_sets);
```

### 010 — UserGrant migration (destructive)
```sql
-- 010_migrate_user_grants_to_permission_sets.up.sql
-- Delete existing grants (force re-consent)
DELETE FROM user_grants;
ALTER TABLE user_grants
    ADD COLUMN granted_permission_sets JSONB NOT NULL DEFAULT '{}',
    DROP COLUMN delegated_oauth2_tokens;
CREATE INDEX idx_grants_permission_sets ON user_grants USING GIN (granted_permission_sets);
```

---

## Step 5: Storage Adapters

### Memory adapter (`internal/adapters/storage/memory/permission_set_repository.go`)

```go
type permissionSetRepository struct {
    mu    sync.RWMutex
    byID  map[id.PermissionSetID]*storage.PermissionSet
    byName map[string]id.PermissionSetID
}
```

Key patterns:
- Deep-copy all returned values (same as `agent_repository.go`)
- Use `sync.RWMutex` (RLock for reads, Lock for writes)
- Return `&storage.StorageError{Kind: storage.KindConflict}` for duplicate names
- `CountAgentsReferencingPermissionSet` iterates the agent repository (injected dependency)

### Postgres adapter (`internal/adapters/storage/postgres/permission_set_repository.go`)

Key SQL patterns:
- Insert `permission_set_service_scopes` rows with `JSONB` scopes array
- `List(serviceID)` joins `permission_set_service_scopes` when `serviceID` is non-zero
- `CountAgentsReferencingPermissionSet` uses JSONB containment:
  ```sql
  SELECT COUNT(*) FROM agents
  WHERE permission_sets @> $1::jsonb
  ```
  where `$1` = `[{"permission_set_id": "<uuid>"}]`

---

## Step 6: Admin HTTP Handler

Create `internal/adapters/http/handlers/admin/permission_sets_handler.go`.

Pattern: follow `agents_handler.go` exactly:
- Constructor: `NewPermissionSetsHandler(repo ports.PermissionSetRepository, ...) *PermissionSetsHandler`
- Methods: `Create`, `Get`, `List`, `Update`, `Delete`
- 409 on name conflict or deletion of in-use set
- 422 on validation failures (empty service_scopes, invalid service_id references)
- Audit log entries on create/update/delete (SR-004)

Wire in `app/handlers.go`:
```go
type AdminHandlers struct {
    Agents         *admin.AgentsHandler
    Services       *admin.ServicesHandler
    PermissionSets *admin.PermissionSetsHandler  // NEW
}
```

Register routes in `internal/adapters/http/routing/admin.go`:
```go
r.Route("/permission-sets", func(r chi.Router) {
    r.Post("/", h.PermissionSets.Create)
    r.Get("/", h.PermissionSets.List)
    r.Get("/{id}", h.PermissionSets.Get)
    r.Put("/{id}", h.PermissionSets.Update)
    r.Delete("/{id}", h.PermissionSets.Delete)
})
```

---

## Step 7: Consent Domain Service

Extend `consent.AgentConsentInfo` to include resolved permission sets and active session IDs:

```go
type ResolvedPermissionSetEntry struct {
    PermissionSet   *storage.PermissionSet
    RequirementType storage.RequirementType
}

type AgentConsentInfo struct {
    Agent                       *storage.Agent
    AvailableThirdpartyServices []*model.ThirdpartyOAuth2ProviderEntity
    ResolvedPermissionSets      []ResolvedPermissionSetEntry  // NEW
    ActiveSessionServiceIDs     []id.ServiceID                // NEW
}
```

Inject `PermissionSetRepository` and `UserSessionRepository` into `consent.Service`. In `GetAgentConsentInfo`:
1. Resolve `agent.PermissionSets` via `psRepo.GetByIDs(ctx, psIDs)`
2. Fetch active sessions via `sessionRepo.ListByPrincipal(ctx, principal)` → extract service IDs

---

## Step 8: Token Exchange — Permission Set Cache + Scope Validation

Inject `PermissionSetRepository` into `TokenExchangeService`. Add an in-process cache:

```go
type psCache struct {
    mu      sync.RWMutex
    entries map[id.PermissionSetID]psCacheEntry
}

type psCacheEntry struct {
    ps        *storage.PermissionSet
    expiresAt time.Time
}
```

In `ExchangeToken`:
1. Load `grant.GrantedPermissionSetIDs`
2. For each ID: check cache (hit if `time.Now().Before(entry.expiresAt)`) or fetch from repo
3. Compute per-service scope union
4. For each service, validate `UserSession` token covers required scopes (FR-017, FR-018)
5. Include `GrantedPermissionSetIDs` in response (FR-012)

Start a background goroutine in `NewTokenExchangeService` to evict expired cache entries every 30s.

---

## Step 9: React Consent Screen

### New components

**`PermissionSetCard.tsx`** — renders one permission set:
- Mandatory: `role="checkbox"` with `aria-checked="true"` `aria-disabled="true"` (no click handler)
- Optional: `role="checkbox"` with toggle handler, default unchecked
- Body: name (heading), description, covered services list (static, no expand/collapse)
- Design tokens: `trust` palette for recommended, `neutral` for alternatives (FR-008)

**`PermissionSetsList.tsx`** — renders the "Agent Permissions" section:
- Maps `permission_sets` array from consent-info response
- Uses `PermissionSetCard` for each entry
- Exposes `selectedOptionalIds: string[]` state upward

### Restructure ConsentScreen

Move `PermissionSetsList` **above** the existing service connections section:

```tsx
<ConsentScreen>
  <PermissionSetsList   // FIRST
    permissionSets={consentInfo.permissionSets}
    onSelectionChange={setSelectedOptionalIds}
  />
  <ServiceConnectionsList  // SECOND (unchanged driver: service_requirements)
    serviceRequirements={consentInfo.agent.serviceRequirements}
    activeSessionServiceIds={consentInfo.activeSessionServiceIds}
  />
</ConsentScreen>
```

Service connections section: filter `service_requirements` to show only services NOT in `active_session_service_ids`.

---

## Step 10: E2E Tests

Location: `tests/e2e/permission_sets_test.go` (Ginkgo/Gomega)

Scenario mapping (21 It() blocks total):

| User Story | Scenarios | Test description |
|-----------|-----------|-----------------|
| US1 | 6 | Admin CRUD + 409 protection + list filtering |
| US2 | 6 | Consent screen rendering + session filtering + toggle |
| US3 | 4 | Agent declaration + validation + consent-info resolution |
| US4 | 4 | Grant storage + token exchange + scope union + upsert |

**Run E2E tests**:
```bash
ginkgo -v ./tests/e2e/permission_sets_test.go
```

**Playwright frontend tests** (`tests/e2e/frontend/permission_sets_frontend_test.go`):
- Permission sets section renders above service connections
- Mandatory card locked, optional card togglable
- Already-connected service shows satisfied indicator

Screenshots to: `tests/e2e/screenshots/consent_permission_sets_*.png`

---

## Verification Checklist

Before opening PR:

- [ ] `just check` passes (fmt + vet + lint)
- [ ] `just verify` passes
- [ ] All 24 E2E scenarios green
- [ ] All Playwright frontend tests green
- [ ] Screenshots captured for all UI states
- [ ] Admin OpenAPI spec updated (`/api/admin/openapi.yaml`)
- [ ] End-user OpenAPI spec updated (`/api/enduser/openapi.yaml`)
- [ ] ARCHITECTURE.md glossary updated (PermissionSet, ServiceScope, granted_permission_sets)
- [ ] `go generate ./internal/domain/id/` run after adding PermissionSetID
- [ ] Migration 008/009/010 up/down tested (apply → rollback → apply)
- [ ] Both memory and postgres adapters pass identical `PermissionSetRepository` test suite
- [ ] Token exchange cache TTL verified (entries expire after ~60s)
- [ ] Audit logs emitted on create/update/delete (SR-004)
