# Quickstart: Tool Approval API & UI

**Feature Branch**: `024-approval-api-ui`
**Date**: 2026-03-28

## Overview

This guide covers implementing the tool approval system end-to-end. Follow these steps in order — each builds on the previous.

## Step 1: Typed ID Registration

Add `ApprovalID` to `internal/domain/id/gen_ids.go`:

```go
// In the ID type list, add:
ApprovalID
```

Update `internal/domain/id/AGENTS.md` with the new type documentation.

Run code generation: `go generate ./internal/domain/id/...`

## Step 2: Domain Entity

Create `internal/domain/storage/tool_approval.go`:

```go
package storage

import (
    "time"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type ApprovalStatus string

const (
    ApprovalStatusPending  ApprovalStatus = "pending"
    ApprovalStatusApproved ApprovalStatus = "approved"
    ApprovalStatusDenied   ApprovalStatus = "denied"
)

type ApprovalPersistence string

const (
    ApprovalPersistenceOnce      ApprovalPersistence = "once"
    ApprovalPersistenceSession   ApprovalPersistence = "session"
    ApprovalPersistencePermanent ApprovalPersistence = "permanent"
)

type ToolApproval struct {
    ID                   id.ApprovalID        `db:"id" json:"id"`
    Principal            id.Principal         `db:"principal" json:"principal"`
    AgentID              id.AgentID           `db:"agent_id" json:"agent_id"`
    GatewayClientID      string               `db:"gateway_client_id" json:"-"`
    ToolName             string               `db:"tool_name" json:"tool_name"`
    Arguments            map[string]any       `db:"arguments" json:"arguments"`
    ArgumentsHash        string               `db:"arguments_hash" json:"-"`
    ToolPattern          string               `db:"tool_pattern" json:"-"`
    ParamsPattern        map[string]string    `db:"params_pattern" json:"-"`
    Description          string               `db:"description" json:"description"`
    RiskLevel            string               `db:"risk_level" json:"risk_level"`
    MCPSessionID         *string              `db:"mcp_session_id" json:"-"`
    AgentSessionID       *string              `db:"agent_session_id" json:"-"`
    ToolInvocationID     *string              `db:"tool_invocation_id" json:"-"`
    OpenTelemetryTraceparent *string              `db:"opentelemetry_traceparent" json:"-"` // captured from the creation request header
    Status               ApprovalStatus       `db:"status" json:"status"`
    Persistence          *ApprovalPersistence `db:"persistence" json:"persistence"`
    Consumed             bool                 `db:"consumed" json:"consumed"`
    ApprovalURL          string               `db:"approval_url" json:"approval_url"`
    CreatedAt            time.Time            `db:"created_at" json:"created_at"`
    ApprovedAt           *time.Time           `db:"approved_at" json:"approved_at,omitempty"`
    DeniedAt             *time.Time           `db:"denied_at" json:"denied_at,omitempty"`
    ConsumedAt           *time.Time           `db:"consumed_at" json:"consumed_at,omitempty"`
    ExpiresAt            time.Time            `db:"expires_at" json:"expires_at"`
}

func (a *ToolApproval) IsExpired(now time.Time) bool {
    return now.After(a.ExpiresAt)
}

func (a *ToolApproval) IsActionable(now time.Time) bool {
    return a.Status == ApprovalStatusPending && !a.IsExpired(now)
}
```

## Step 3: Repository Port

Add to `internal/ports/storage.go`:

```go
// ToolApprovalRepository defines storage operations for tool approval entities.
type ToolApprovalRepository interface {
    Create(ctx context.Context, approval *storage.ToolApproval) (*storage.ToolApproval, error)
    Get(ctx context.Context, id id.ApprovalID) (*storage.ToolApproval, error)
    Approve(ctx context.Context, id id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error)
    Deny(ctx context.Context, id id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error)
    Consume(ctx context.Context, id id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error)
    ListActiveByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error)
    CountPendingByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error)
    ListAllActive(ctx context.Context, principalFilter *id.Principal) ([]*storage.ToolApproval, error)
    ListPermanentByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.ToolApproval, error)
}

// ApprovalSyncStateRepository manages the global approval sync version counter.
type ApprovalSyncStateRepository interface {
    GetVersion(ctx context.Context) (int64, error)
    IncrementVersion(ctx context.Context) (int64, error)
}
```

## Step 4: Database Migrations

Create `migrations/008_create_tool_approvals.up.sql` and `009_create_approval_sync_state.up.sql` as defined in [data-model.md](data-model.md).

Verify with: `just test` (integration tests apply and rollback all migrations).

## Step 5: Storage Adapters

Implement both adapters following `specs/004-persistence-layer/quickstart.md`:

1. **In-memory**: `internal/adapters/storage/memory/tool_approval_repository.go`
   - Use `sync.RWMutex` + `map[id.ApprovalID]*storage.ToolApproval`
   - Implement deduplication check in Create (scan for matching pending record)
   - In-memory version counter (atomic int64) for ApprovalSyncState

2. **PostgreSQL**: `internal/adapters/storage/postgres/tool_approval_repository.go`
   - Use sqlx with parameterized queries
   - Leverage partial unique index for idempotent Create (ON CONFLICT)
   - VERSION increment + NOTIFY in same transaction

## Step 6: Configuration

Add to `internal/config/schema.go`:

```go
type ApprovalsConfig struct {
    PendingTTL         time.Duration           `mapstructure:"pending_ttl"`
    SyncCoalesceWindow time.Duration           `mapstructure:"sync_coalesce_window"`
    RateLimit          ApprovalRateLimitConfig `mapstructure:"rate_limit"`
}

type ApprovalRateLimitConfig struct {
    MaxPendingPerPair    int `mapstructure:"max_pending_per_pair"`
    MaxRequestsPerMinute int `mapstructure:"max_requests_per_minute"`
}
```

Add defaults in loader.go:

```go
viper.SetDefault("approvals.pending_ttl", "10m")
viper.SetDefault("approvals.sync_coalesce_window", "1s")
viper.SetDefault("approvals.rate_limit.max_pending_per_pair", 50)
viper.SetDefault("approvals.rate_limit.max_requests_per_minute", 10)
```

## Step 7: Domain Service

Create `internal/domain/approval/service.go`:

Key methods:

- `CreatePendingApproval(ctx, req) (*ToolApproval, error)` — compute arguments_hash, check rate limit, check idempotency, create record, increment version
- `ApproveApproval(ctx, id, principal, request)` — verify principal, resolve and validate the requested pattern, check expiry, transition state via repository, increment version
- `DenyApproval(ctx, id, principal, persistence)` — verify principal, check expiry, transition state via repository, increment version
- `ConsumeApproval(ctx, id) (*ToolApproval, error)` — verify once-persistence, mark consumed, increment version
- `GetApproval(ctx, id, principal) (*ToolApproval, error)` — verify principal, return record
- `GetSyncState(ctx, principalFilter) (*SyncState, error)` — return all active approvals grouped by pair

Dependencies (injected via constructor):

- `ToolApprovalRepository`
- `ApprovalSyncStateRepository`
- `AgentRepository` (for resolving agent display names)
- `ApprovalsConfig` (for TTL, rate limits)
- `PublicURL` (for constructing approval_url)

## Step 8: HTTP Handlers

Create handlers in `internal/adapters/http/handlers/approval/`:

Each handler follows the existing pattern:

```go
type CreateHandler struct {
    service *approval.Service
    logger  *slog.Logger
}

func (h *CreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. Extract principal from subject token
    // 2. Validate client assertion
    // 3. Parse request body
    // 4. Call domain service
    // 5. Write JSON response
}
```

The approve handler decodes optional pattern fields without collapsing a missing `params_pattern` into an empty object:

```go
type approveRequest struct {
    Persistence   string            `json:"persistence"`
    ToolPattern   string            `json:"tool_pattern"`
    ParamsPattern map[string]string `json:"params_pattern"`
}
```

Register in `internal/adapters/http/routing/enduser.go` within the authenticated route group.

## Step 9: Long-Poll Infrastructure

The sync handler (`GET /api/approvals`) requires:

1. **ApprovalSyncSubscriber**: Goroutine listening on PostgreSQL `NOTIFY approval_sync` via raw pgx connection
2. **ApprovalSyncBroadcaster**: Manages subscription channels for waiting long-poll goroutines
3. **Coalesce timer**: Buffers notifications within the `sync_coalesce_window` before waking subscribers

Wired in `builder.go` and started as a background goroutine alongside the HTTP server.

## Step 10: Builder Wiring

In `internal/app/builder.go`:

```go
// Phase 5.5: Approval service (after storage, before handlers)
approvalService := approval.NewService(
    b.storage.ToolApprovals(),
    b.storage.ApprovalSyncState(),
    b.storage.Agents(),
    b.config.Approvals,
    b.config.Server.Enduser.PublicURL,
)

// Add to EnduserHandlers
enduserHandlers.ApprovalCreate = approvalhandler.NewCreateHandler(approvalService, logger)
enduserHandlers.ApprovalGet = approvalhandler.NewGetHandler(approvalService, logger)
// ... etc
```

## Step 11: Frontend Components

Create React components in `web/src/components/approvals/` using the design system:

1. **ApprovalPage** (`web/src/pages/ApprovalPage.tsx`): Route-level page, fetches approval by ID
2. **ToolCallCard**: Displays tool name/title, arguments, description, risk badge using `Card` + `Badge` primitives
3. **PersistenceSelector**: `Radio` group with three options, warning text on permanent
4. **ApprovalConfirmation**: Success/denial confirmation with `Alert` primitive
5. **ApprovalLoadingSkeleton**: `Skeleton` primitive matching ToolCallCard layout
6. **ApprovalErrorBanner**: `Alert` primitive with error-specific messages

Add route to `web/src/App.tsx`:

```tsx
<Route path="/approvals/:id" element={<ApprovalPage />} />
```

Add API client in `web/src/services/api/approvals.ts`:

```typescript
export const getApproval = (id: string) => client.get(`/approvals/${id}`);
export const approveApproval = (id: string, request: ApproveRequest) =>
    client.post(`/approvals/${id}/approve`, request);
export const denyApproval = (id: string, persistence?: string) =>
    client.post(`/approvals/${id}/deny`, persistence ? { persistence } : {});
```

## Step 12: Helm Chart & Config Examples

Update `charts/agentic-identity-broker/values.yaml`:

```yaml
approvals:
  pending_ttl: "10m"
  sync_coalesce_window: "1s"
  rate_limit:
    max_pending_per_pair: 50
    max_requests_per_minute: 10
```

Add example config to `examples/config/`.
