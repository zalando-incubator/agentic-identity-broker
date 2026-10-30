# ADR 20260510-120000: Optional Agent client_id (Nullable)

**Status**: Accepted
**Date**: 2026-05-10

---

## Context

The `client_id` field on Agent was introduced when all agents proxied to an upstream OAuth2 provider — the field held the credential the upstream provider recognizes. With the addition of local token minting mode (ADR 20260328-120100) and CIMD support (spec 20260421-120000), many agents never use `client_id` for upstream proxying:

- **Local token minting agents** use the agent's UUID as the OAuth2 `client_id` parameter at runtime (`OpaqueClientResolver` parses the request `client_id` as a UUID and calls `repo.Get()`). The stored `ClientID` field is unused in this flow.
- **CIMD agents** are resolved via `GetByClientURI` against pre-registered `client_uris`. The stored `ClientID` is again not consulted during authorization.

Despite this, the Admin API requires operators to supply a `client_id` for every agent. For agents that don't proxy upstream, operators must invent a meaningless value.

## Decision

Make `client_id` truly optional — nullable at every layer:

1. **Domain**: `Agent.ClientID` is `*id.ClientID` (nil = absent).
2. **Database**: The `agents.client_id` column is nullable (migration 016 drops the `NOT NULL` constraint).
3. **Admin API**: `client_id` is omitted from create/update requests for agents that don't need it. No auto-generation fallback.
4. **Behavior**: When `client_id` is nil, `GetByClientID` cannot resolve the agent — callers must use `Get` (by agent UUID) or `GetByClientURI` (for CIMD agents).

When explicitly provided, the operator-supplied value is stored unchanged (for upstream proxy scenarios).

## Consequences

- Operators no longer need to supply `client_id` for local or CIMD agents.
- Agents without an upstream client_id carry `NULL`, not a disguised value.
- `GetByClientID` and `ExistsOtherWithClientID` correctly exclude agents with no client_id.
- Migration 016 (`ALTER TABLE agents ALTER COLUMN client_id DROP NOT NULL`) makes the column nullable. Existing rows retain their values.
- Forward-compatible with hybrid setups: operators who need a real upstream `client_id` alongside `client_uris` can still supply one explicitly.
- On update, omitting `client_id` preserves the existing value.
