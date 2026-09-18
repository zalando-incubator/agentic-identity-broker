# Data Model: Tool Approval API & UI

**Feature Branch**: `024-approval-api-ui`
**Date**: 2026-03-28

## Entity: ToolApproval

**Aggregate Root**: Yes — self-contained authorization record with full lifecycle invariants.

### Fields

| Field | Type | Nullable | Default | Constraints | Description |
|-------|------|----------|---------|-------------|-------------|
| `id` | `ApprovalID` (UUID) | No | Generated | PK | Unique identifier (typed ID per ADR 013) |
| `principal` | `id.Principal` | No | — | VARCHAR(200) | Authenticated user who owns this approval |
| `agent_id` | `id.AgentID` (UUID) | No | — | FK to agents | Agent requesting the tool call |
| `gateway_client_id` | `string` | No | — | VARCHAR(255) | Gateway instance that created the approval |
| `tool_name` | `string` | No | — | VARCHAR(255) | MCP tool name — programmatic identifier |
| `arguments` | `map[string]interface{}` | No | — | JSONB | Actual argument values passed to the tool call |
| `arguments_hash` | `string` | No | — | VARCHAR(64) | SHA-256 hex of canonical JSON arguments |
| `tool_pattern` | `string` | No | Exact escaped tool name | VARCHAR(255) | Server-derived exact matcher for the approval tool name |
| `params_pattern` | `map[string]string` | No | `{}` | JSONB | Constrained argument names mapped to globs; absent names are unconstrained |
| `description` | `string` | Yes | `""` | TEXT | Caller-provided human-readable action description (primary UI text) |
| `risk_level` | `string` | Yes | `""` | VARCHAR(50) | Risk classification (low/medium/critical) |
| `mcp_session_id` | `string` | Yes | `nil` | VARCHAR(255) | MCP session identifier (stored for observability; not used for session scoping) |
| `agent_session_id` | `string` | Yes | `nil` | VARCHAR(255) | Agent session identifier — stable across MCP reconnects; used to scope `persistence: session` approvals |
| `tool_invocation_id` | `string` | Yes | `nil` | VARCHAR(255) | Tool invocation identifier |
| `opentelemetry_traceparent` | `string` | Yes | `nil` | VARCHAR(55) | W3C traceparent captured from approval creation request header |
| `status` | `ApprovalStatus` | No | `pending` | ENUM(pending,approved,denied) | Current lifecycle state |
| `persistence` | `ApprovalPersistence` | Yes | `nil` | ENUM(once,session,permanent) | Persistence scope (set on approve/deny) |
| `consumed` | `bool` | No | `false` | — | Whether a once-persistence approval has been used |
| `approval_url` | `string` | No | — | TEXT | Browser URL for user review |
| `created_at` | `time.Time` | No | — | TIMESTAMPTZ | Record creation timestamp |
| `approved_at` | `*time.Time` | Yes | `nil` | TIMESTAMPTZ | When user approved |
| `denied_at` | `*time.Time` | Yes | `nil` | TIMESTAMPTZ | When user denied |
| `consumed_at` | `*time.Time` | Yes | `nil` | TIMESTAMPTZ | When one-time approval was consumed |
| `expires_at` | `time.Time` | No | — | TIMESTAMPTZ | TTL expiry (created_at + pending_ttl) |

### State Machine

```
                 ┌─────────┐
                 │ pending  │
                 └────┬─────┘
                      │
            ┌─────────┼──────────┐
            │                    │
       ┌────▼────┐         ┌────▼────┐
       │ approved │         │ denied  │
       └────┬─────┘         └─────────┘
            │
            │ (only if persistence == once)
            │
     ┌──────▼───────┐
     │ consumed=true │
     └───────────────┘
```

**State transition invariants**:

1. Only `pending` approvals can transition to `approved` or `denied`
2. Only `approved` approvals with `persistence == once` can be consumed
3. Expired approvals (`now > expires_at`) cannot be approved or denied
4. The acting user MUST match the stored `principal`
5. `persistence` is immutable once set
6. `consumed` is a one-way flag (false → true, never back)

### Validation Rules

```go
func (a *ToolApproval) Validate() error {
    // Required fields
    if a.ID.IsZero() { return ErrMissingID }
    if a.Principal == "" { return ErrMissingPrincipal }
    if a.AgentID.IsZero() { return ErrMissingAgentID }
    if a.GatewayClientID == "" { return ErrMissingGatewayClientID }
    if a.ToolName == "" { return ErrMissingToolName }
    if a.ArgumentsHash == "" { return ErrMissingArgumentsHash }
    if a.ApprovalURL == "" { return ErrMissingApprovalURL }
    if a.ExpiresAt.IsZero() { return ErrMissingExpiresAt }
    
    // Status validation
    if !a.Status.IsValid() { return ErrInvalidStatus }
    
    // Persistence only set when approved/denied
    if a.Status == StatusPending && a.Persistence != nil {
        return ErrPersistenceOnPending
    }
    if a.Status == StatusApproved && a.Persistence == nil {
        return ErrMissingPersistence
    }
    
    // Consumed only valid for once-persistence approved
    if a.Consumed && (a.Status != StatusApproved || *a.Persistence != PersistenceOnce) {
        return ErrInvalidConsumed
    }
    
    return nil
}
```

Pattern validation: a tool pattern is non-empty, at most 255 bytes, permits only unescaped `*` or `[A-Za-z0-9_.:/-]`, and has no trailing lone `\`. A params pattern has at most 64 entries; keys are non-empty and at most 255 bytes; values are at most 1024 bytes and have no trailing lone `\`. A resolved pattern must match the approval's reviewed tool name and arguments; a constrained key absent from the reviewed arguments fails validation.

### Domain Methods

```go
// Approve transitions a pending approval to approved state with the persisted decision.
// Returns error if not pending, expired, or principal mismatch.
func (a *ToolApproval) Approve(actingPrincipal id.Principal, decision ApprovalDecision, now time.Time) error

// ApplyExactPatterns sets exact coverage for the approval's own tool name and arguments.
func (a *ToolApproval) ApplyExactPatterns()

// Deny transitions a pending approval to denied state.
// Returns error if not pending, expired, or principal mismatch.
func (a *ToolApproval) Deny(actingPrincipal id.Principal, persistence *ApprovalPersistence, now time.Time) error

// Consume marks a once-persistence approved approval as consumed.
// Returns error if not approved, not once-persistence, or already consumed.
func (a *ToolApproval) Consume(now time.Time) error

// IsExpired returns true if the approval has passed its TTL.
func (a *ToolApproval) IsExpired(now time.Time) bool

// IsActionable returns true if the approval can still be approved or denied.
func (a *ToolApproval) IsActionable(now time.Time) bool
```

---

## Value Object: ApprovalStatus

```go
type ApprovalStatus string

const (
    StatusPending  ApprovalStatus = "pending"
    StatusApproved ApprovalStatus = "approved"
    StatusDenied   ApprovalStatus = "denied"
)

func (s ApprovalStatus) IsValid() bool {
    switch s {
    case StatusPending, StatusApproved, StatusDenied:
        return true
    }
    return false
}
```

---

## Value Object: ApprovalPersistence

```go
type ApprovalPersistence string

const (
    PersistenceOnce      ApprovalPersistence = "once"
    PersistenceSession   ApprovalPersistence = "session"
    PersistencePermanent ApprovalPersistence = "permanent"
)

func (p ApprovalPersistence) IsValid() bool {
    switch p {
    case PersistenceOnce, PersistenceSession, PersistencePermanent:
        return true
    }
    return false
}
```

---

## Value Object: ApprovalDecision

```go
type ApprovalDecision struct {
    Persistence   ApprovalPersistence
    ToolPattern   string
    ParamsPattern map[string]string
}
```

The server derives the exact escaped tool pattern from the reviewed tool name. An omitted `params_pattern` stores exact coverage. A `null` `params_pattern` returns `422 invalid_pattern`. An explicit `{}` leaves all arguments unconstrained. A supplied `tool_pattern` returns `400 invalid_request` with `tool_pattern is not allowed`.

| Request beyond `persistence` | Resolved `tool_pattern` | Resolved `params_pattern` |
|---|---|---|
| nothing | escaped approval tool name | exact reviewed arguments |
| `params_pattern: {}` | escaped approval tool name | `{}` |
| `params_pattern: {"repo":"acme/*"}` | escaped approval tool name | as supplied |

---

## Entity: ApprovalSyncState

**Single-row table** tracking the global mutation version for ETag generation.

### Fields

| Field | Type | Nullable | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | `int64` | No | `0` | Monotonically increasing counter, incremented on every approval or permission-set mutation |

### Operations

```go
// IncrementAndGet atomically increments the version and returns the new value.
// Implementation: UPDATE approval_sync_state SET version = version + 1 RETURNING version
// Combined with NOTIFY approval_sync in the same transaction.
func (r *ApprovalSyncStateRepository) IncrementAndGet(ctx context.Context) (int64, error)

// GetVersion returns the current version without incrementing.
func (r *ApprovalSyncStateRepository) GetVersion(ctx context.Context) (int64, error)
```

---

## Typed ID: ApprovalID

Added to `internal/domain/id/gen_ids.go`:

```go
// ApprovalID identifies a ToolApproval entity.
type ApprovalID uuid.UUID

// NewApprovalID generates a new random ApprovalID.
func NewApprovalID() ApprovalID { return ApprovalID(uuid.New()) }

// ParseApprovalID parses a string into an ApprovalID, returning error on invalid format.
func ParseApprovalID(s string) (ApprovalID, error) { ... }

// MustParseApprovalID parses a string into an ApprovalID, panicking on error. Test use only.
func MustParseApprovalID(s string) ApprovalID { ... }
```

---

## Repository Interfaces: ToolApproval (ISP split)

Three focused interfaces added to `internal/ports/storage.go` — each ≤5 methods, per Constitution Principle IX (max 5–7 methods per interface).

### ToolApprovalRepository — core write + single-record read

```go
// ToolApprovalRepository defines core CRUD operations for tool approval entities.
// Max 5 methods — ISP-compliant.
type ToolApprovalRepository interface {
    // Create creates a new pending tool approval.
    // Uses partial unique index for deduplication (principal, agent_id, tool_name, arguments_hash)
    // where status=pending AND consumed=false.
    // Returns the existing record if a duplicate is found (idempotent).
    // All adapter errors MUST be wrapped in domain StorageError.
    Create(ctx context.Context, approval *storage.ToolApproval) (*storage.ToolApproval, error)

    // Get retrieves a tool approval by ID.
    // Returns StorageError{Kind: NotFound} if not found.
    Get(ctx context.Context, id id.ApprovalID) (*storage.ToolApproval, error)

    // Approve transitions a pending approval to approved status.
    // Sets persistence, approved_at. Returns updated record.
    // Returns StorageError{Kind: NotFound} if not found or not pending.
    Approve(ctx context.Context, id id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error)

    // Deny transitions a pending approval to denied status.
    // Sets persistence (optional), denied_at. Returns updated record.
    // Returns StorageError{Kind: NotFound} if not found or not pending.
    Deny(ctx context.Context, id id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error)

    // Consume marks an approved once-persistence approval as consumed.
    // Sets consumed=true, consumed_at. Idempotent.
    // Returns StorageError{Kind: NotFound} if not found.
    // Returns domain error if not once-persistence or not approved.
    Consume(ctx context.Context, id id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error)
}
```

### ToolApprovalQueryRepository — list operations for sync + consent UI

```go
// ToolApprovalQueryRepository defines read-side list queries for tool approvals.
// Used by the long-poll sync endpoint and the consent management UI.
// Max 3 methods — ISP-compliant.
type ToolApprovalQueryRepository interface {
    // ListAllActive lists all active approvals, optionally filtered by principal.
    // "Active" means non-expired pending records plus approved/denied records relevant for sync.
    // All adapter errors MUST be wrapped in domain StorageError.
    ListAllActive(ctx context.Context, principalFilter *id.Principal) ([]*storage.ToolApproval, error)

    // ListActiveByPrincipalAndAgent lists all active (non-expired, non-consumed) approvals
    // for a given principal and agent pair.
    ListActiveByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error)

    // ListPermanentByPrincipal lists all permanent approvals/denials for a principal.
    // Used by the consent management UI.
    ListPermanentByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.ToolApproval, error)
}
```

### ToolApprovalMetricsRepository — rate limiting counter

```go
// ToolApprovalMetricsRepository defines the single count query used by the rate limiter.
// Kept separate so the rate limiter depends only on this minimal interface.
type ToolApprovalMetricsRepository interface {
    // CountPendingByPrincipalAndAgent counts pending (non-expired) approvals for rate limiting.
    // All adapter errors MUST be wrapped in domain StorageError.
    CountPendingByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error)
}
```

**Adapter note**: Both in-memory and PostgreSQL adapters implement all three interfaces on the same struct. The factory exposes three typed accessor methods: `ToolApprovals() ToolApprovalRepository`, `ToolApprovalQueries() ToolApprovalQueryRepository`, `ToolApprovalMetrics() ToolApprovalMetricsRepository`.

---

## Repository Interface: ApprovalSyncStateRepository

```go
// ApprovalSyncStateRepository manages the global approval sync version counter.
type ApprovalSyncStateRepository interface {
    // GetVersion returns the current sync version.
    GetVersion(ctx context.Context) (int64, error)

    // IncrementVersion atomically increments the version counter and issues
    // NOTIFY approval_sync. Returns the new version.
    IncrementVersion(ctx context.Context) (int64, error)
}
```

---

## Database Schema

### Migration 008: tool_approvals

**Up** (`008_create_tool_approvals.up.sql`):

```sql
CREATE TABLE tool_approvals (
    id UUID PRIMARY KEY,
    principal VARCHAR(200) NOT NULL,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    gateway_client_id VARCHAR(255) NOT NULL,
    tool_name VARCHAR(255) NOT NULL,
    arguments JSONB NOT NULL DEFAULT '{}',
    arguments_hash VARCHAR(64) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    risk_level VARCHAR(50) NOT NULL DEFAULT '',
    mcp_session_id VARCHAR(255),
    agent_session_id VARCHAR(255),
    tool_invocation_id VARCHAR(255),
    opentelemetry_traceparent VARCHAR(55),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    persistence VARCHAR(20),
    consumed BOOLEAN NOT NULL DEFAULT FALSE,
    approval_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    denied_at TIMESTAMPTZ,
    consumed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL
);

-- Deduplication: one pending unconsumed approval per (principal, agent, tool, arguments)
CREATE UNIQUE INDEX idx_tool_approvals_dedup
    ON tool_approvals (principal, agent_id, tool_name, arguments_hash)
    WHERE status = 'pending' AND consumed = FALSE;

-- GIN index for pattern-based arguments queries
CREATE INDEX idx_tool_approvals_arguments ON tool_approvals USING GIN (arguments);

-- Lookup by principal for consent management
CREATE INDEX idx_tool_approvals_principal ON tool_approvals (principal);

-- Lookup by principal+agent for rate limiting and sync
CREATE INDEX idx_tool_approvals_principal_agent ON tool_approvals (principal, agent_id);

-- Expiry cleanup
CREATE INDEX idx_tool_approvals_expires ON tool_approvals (expires_at) WHERE status = 'pending';
```

**Down** (`008_create_tool_approvals.down.sql`):

```sql
DROP TABLE IF EXISTS tool_approvals;
```

### Migration 009: approval_sync_state

**Up** (`009_create_approval_sync_state.up.sql`):

```sql
CREATE TABLE approval_sync_state (
    version BIGINT NOT NULL DEFAULT 0
);

-- Seed with initial row
INSERT INTO approval_sync_state (version) VALUES (0);
```

**Down** (`009_create_approval_sync_state.down.sql`):

```sql
DROP TABLE IF EXISTS approval_sync_state;
```

---

### Migration 031: approval patterns

`031_add_approval_patterns.up.sql` adds `tool_pattern VARCHAR(255) NOT NULL` and `params_pattern JSONB NOT NULL DEFAULT '{}'` to `tool_approvals`. It backfills each row with `tool_pattern = tool_name` and an exact, escaped params pattern using the same canonical rendering as `internal/toolpattern.ExactParams`; no index is added because matching is client-side in ExtProc. The down migration drops both columns.


## Relationships

```
Agent (1) ◄────── (N) ToolApproval
  │                        │
  │ agent_id (FK)         │ principal (VARCHAR)
  │                        │
  └── ON DELETE CASCADE    └── No FK (string identity from upstream proxy)

ApprovalSyncState (1 row) ── Global version counter
```

- **ToolApproval → Agent**: Foreign key with CASCADE delete. If an agent is deleted, all its approvals are removed.
- **ToolApproval → Principal**: String reference (not FK). Principal is an authenticated user identity from the upstream proxy, not a database entity.
- **ApprovalSyncState**: Singleton row, no relationships. Incremented atomically on any approval mutation.
