# Data Model: Revoke Agent Consent (022)

**Branch**: `20260320-120100-revoke-agent-consent` | **Date**: 2026-03-20

---

## Domain Entities — Changes

### UserGrant (existing — no structural changes)

No new fields. Hard deletion remains the sole revocation mechanism. The entity is unchanged.

```go
// internal/domain/storage/user_grant.go — no changes required
type UserGrant struct {
    ID                    id.GrantID       `json:"id" db:"id"`
    Principal             id.Principal     `json:"principal" db:"principal"`
    AgentID               id.AgentID       `json:"agent_id" db:"agent_id"`
    ValidUntil            *time.Time       `json:"valid_until,omitempty" db:"valid_until"`
    DelegatedOAuth2Tokens []DelegatedToken `json:"delegated_oauth2_tokens" db:"delegated_oauth2_tokens"`
    CreatedAt             time.Time        `json:"created_at" db:"created_at"`
    UpdatedAt             time.Time        `json:"updated_at" db:"updated_at"`
}
```

---

## Domain Events

### GrantRevoked (audit log event)

Not a persistent event — emitted as a structured log entry from `ConsentService.RevokeConsentForPrincipal`:

```json
{
  "level": "INFO",
  "time": "2026-03-20T10:00:00Z",
  "msg": "grant revoked",
  "action": "grant_revoked",
  "principal": "user@example.com",
  "agent_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "grant_id": "yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
}
```

---

## Port Changes

### UserGrantRepository — new method

**File**: `internal/ports/storage.go`

Add to the `UserGrantRepository` interface:

```go
// DeleteByPrincipalAndAgentID deletes the grant owned by principal for the given agent.
// Returns ports.ErrNotFound if no matching grant exists.
// Returns StorageError for connection/timeout issues.
DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error
```

**Semantics**: Atomic lookup-and-delete. Returns `ports.ErrNotFound` (not nil) when no matching row exists. This distinguishes "revoked successfully" (nil return) from "grant didn't exist" (ErrNotFound). This avoids a two-phase check+delete race condition.

---

## Domain Service Changes

### ConsentService — new method + new sentinel error

**File**: `internal/domain/consent/service.go`

New sentinel error:
```go
// ErrGrantNotFound is returned when a revocation targets a non-existent grant.
// Distinguished from ErrAgentAccessDenied: this is used for user-initiated
// delete operations where "not found" should surface as 404.
ErrGrantNotFound = errors.New("grant not found")
```

New method:
```go
// RevokeConsentForPrincipal revokes the grant for a principal+agent pair.
// Non-idempotent: returns ErrGrantNotFound if no active grant exists.
// Emits structured audit log on success (SR-002).
// Fails closed: if lookup fails with transient error, grant is NOT deleted (SR-003).
func (s *Service) RevokeConsentForPrincipal(ctx context.Context, principal id.Principal, agentID id.AgentID) error
```

Existing `RevokeConsent` is **unchanged** (idempotent, used by the POST empty-array workaround path).

`VerifyAgentAccess` requires a one-line comment change only: remove the TODO block about a future `Revoked` field (lines 244-249) and replace with a comment clarifying that hard deletion satisfies the revocation enforcement.

**ConsentService interface** (`internal/adapters/http/handlers/consent/service_interface.go`) must also add `RevokeConsentForPrincipal` to the `ConsentService` interface used by HTTP handlers.

---

## Storage Adapter Changes

### Memory Adapter

**File**: `internal/adapters/storage/memory/user_grants.go`

Implement `DeleteByPrincipalAndAgentID`:

```go
func (r *UserGrantRepository) DeleteByPrincipalAndAgentID(
    ctx context.Context,
    principal id.Principal,
    agentID id.AgentID,
) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    key := principalAgentKey(principal, agentID)
    grantID, ok := r.byPrincipalAndAgent[key]
    if !ok {
        return ports.ErrNotFound
    }

    // Remove from all indexes (same as Delete)
    delete(r.grants, grantID)
    delete(r.byPrincipalAndAgent, key)
    // Remove from agent index
    r.removeFromAgentIndex(grantID, agentID)

    return nil
}
```

### Postgres Adapter

**File**: `internal/adapters/storage/postgres/user_grants.go`

Implement `DeleteByPrincipalAndAgentID`:

```go
func (r *UserGrantRepository) DeleteByPrincipalAndAgentID(
    ctx context.Context,
    principal id.Principal,
    agentID id.AgentID,
) error {
    ctx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
    defer cancel()

    const query = `DELETE FROM user_grants WHERE principal = $1 AND agent_id = $2`
    result, err := r.adapter.db.ExecContext(ctx, query, principal, agentID)
    if err != nil {
        return r.adapter.handlePostgresError(err)
    }

    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to check rows affected: %w", err)
    }
    if rows == 0 {
        return ports.ErrNotFound
    }
    return nil
}
```

---

## New HTTP Handler

**File**: `internal/adapters/http/handlers/consent/revoke_grant_handler.go`

```go
type RevokeGrantHandler struct {
    consentService ConsentService
    logger         *slog.Logger
}

func NewRevokeGrantHandler(consentService ConsentService, logger *slog.Logger) *RevokeGrantHandler

// DELETE /api/consent/agent/{agent-id}/grants
func (h *RevokeGrantHandler) RevokeGrant(w http.ResponseWriter, r *http.Request)
```

Authorization order per AGENTS.md: principal check (401) → UUID format validation (400) → service call.

Error mapping:
- `consent.ErrGrantNotFound` → 404
- `ports.ErrNotFound` (wrapped) → 404
- Other errors → 500

---

## App Wiring

**`internal/app/handlers.go`**: Add `RevokeGrant *consent.RevokeGrantHandler` to `EnduserHandlers` struct.

**`internal/app/builder.go`**: Instantiate `RevokeGrantHandler` in `Build()` and assign to `app.EnduserHandlers.RevokeGrant`.

**`internal/adapters/http/routing/enduser.go`**: Add route:
```go
r.Delete("/api/consent/agent/{agent-id}/grants", h.RevokeGrant.RevokeGrant)
```
Gated on `h.RevokeGrant != nil` (consistent with nil-guard pattern for existing handlers).

---

## Frontend Components

### New Components

**`web/src/components/consent/RevokeGrantButton.tsx`**
- Props: `agentId: string`, `agentName: string`, `onRevoked: () => void`
- Renders `Button` primitive with `variant="destructive"` (uses `error-primary` semantic token)
- `aria-label={`Revoke all access for ${agentName}`}`
- Opens `RevokeGrantDialog` on click

**`web/src/components/consent/RevokeGrantDialog.tsx`**
- Props: `agentName: string`, `agentId: string`, `isOpen: boolean`, `onConfirm: () => Promise<void>`, `onClose: () => void`, `isLoading?: boolean`
- Uses `Modal` primitive from `@design-system/components/overlays/Modal`
- Focus traps via Modal (WCAG 2.1 AA)
- Keyboard: `Escape` dismisses, `Enter` confirms
- Body text: "This will remove all permissions for **{agentName}**. Any connected services (e.g., GitHub, Google) remain active — they are not affected by this action."
- CTA: "Revoke All Access" (destructive/`error-primary`) + "Cancel" (secondary)

### Modified Components / Pages

**`web/src/pages/AgentGrantDetailPage.tsx`** (P1)
- When user has an active grant (`grants` non-null, non-empty): render `<RevokeGrantButton>` alongside "Approve & Delegate" section
- On `onRevoked`: navigate to `/` (consent overview) and show success toast

**`web/src/components/consent/DelegationCard.tsx`** (P2)
- Add optional `onRevoke?: (agentId: string) => void` prop
- Render a "Revoke" secondary action button on each card when prop provided
- `ConsentOverviewPage` passes the handler which opens `RevokeGrantDialog`

**`web/src/pages/ConsentOverviewPage.tsx`** (P2)
- Manage `revokingAgentId: string | null` state
- Pass `onRevoke` to `DelegationList` → `DelegationCard`
- On confirm: call `deleteGrant(agentId)`, refetch delegation list, show success toast

### API Client Change

**`web/src/hooks/useConsent.ts`** (or a new `useRevokeGrant.ts` hook):

```typescript
async function deleteGrant(agentId: string): Promise<void> {
    const response = await fetch(`/api/consent/agent/${agentId}/grants`, {
        method: 'DELETE',
    });
    if (response.status === 404) {
        throw new Error('Grant not found — it may have already been revoked');
    }
    if (!response.ok) {
        throw new Error(`Failed to revoke grant: ${response.status}`);
    }
}
```

---

## Database Schema

**No changes required** (DB-001). `UserGrant` deletion uses the existing `user_grants` table with `DELETE WHERE principal = $1 AND agent_id = $2`.

No migration files needed.

---

## Glossary Additions (for ARCHITECTURE.md)

| Term | Definition |
|---|---|
| **GrantRevoked** | Domain event representing a user's explicit deletion of their grant for an agent. Emitted as a structured audit log entry carrying `principal`, `agent_id`, `grant_id`, and `revoked_at`. |

(Existing entries for `UserGrant`, `Principal`, `Agent` remain unchanged.)
