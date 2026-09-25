# ADR 036: ExtProc Approval Cache and Gate

**Status**: Accepted
**Date**: 2026-09-09
**Feature**: 026-extproc-approval-sync

---

## Context

ADR 028 lets ExtProc evaluate OPA policy before it forwards an MCP tool call. OPA can now return `approval_required` as well as `allow` and `deny`.

An approval check must not add a broker read to every tool call. ExtProc also must not treat a local approval snapshot as permanent authority after a user revokes it.

ExtProc holds the caller's opaque bearer token. It has no JWT verifier, JWKS client, or claim-extraction configuration. It cannot safely derive the principal or agent from either the caller token or the issued access token.

## Decision

### Identity and cache scope

The broker derives `principal` and `agent_id` from the caller's signature-verified subject token during token exchange. It returns both values in the RFC 8693 token-exchange response.

ExtProc uses these response fields as the only source of the approval identity. It keys the bounded in-memory approval cache with the verified `(principal, agent_id)` pair.

If either field is absent, ExtProc denies an `approval_required` call. It does not construct a cache key, read approval state, or create an approval.

The broker returns the resolved canonical agent UUID. That UUID is already public through the admin API. This response mirrors the established `granted_permission_sets` response extension.

The response contains only the principal and agent that the broker derived for the caller's verified token. It does not disclose another user's identity or approval state.

### Broker authority and cache freshness

The broker remains authoritative for every approval. The cache can authorize a matching approval only when its broker confirmation is no older than `tool_approvals.max_staleness`.

A `200 OK` response replaces the complete approval snapshot for its returned scope. ExtProc does not merge that response with earlier records. A `304 Not Modified` response confirms that the current ETag remains current and refreshes the cache confirmation time.

ExtProc uses ADR 014's ETag long-poll protocol to keep the cache current. It retains one background long-poll connection per ExtProc instance.

ExtProc evicts inactive identity pairs after `tool_approvals.approval_cache_idle_ttl`. This limit bounds the lifetime of unused approval records.

If a pair has never synchronized, or its confirmation exceeds `tool_approvals.max_staleness`, ExtProc treats the pair as stale. It performs the FR-005 client-assertion authenticated authoritative read before it can authorize a record or create an approval.

If that read fails or is malformed, ExtProc denies the tool call. It does not create an approval from an unconfirmed cache miss.

ExtProc records the latest accepted ETag version. A targeted read that returns an ETag older than that version is stale. ExtProc rejects that response, keeps it out of the cache, and denies the tool call.

A revocation reaches ExtProc within one successful long-poll cycle during normal operation. If synchronization fails, a cached approval cannot authorize after `tool_approvals.max_staleness`.

### Request gate and one-time approvals

For body-bearing MCP requests, ExtProc performs token exchange before `processRequestBody`. `processRequestBody` evaluates OPA first and runs the approval gate only after OPA returns `approval_required`.

An OPA `allow` forwards the already-exchanged token. An OPA `deny` does not enter the approval gate and does not create an approval.

A fresh matching `permanent` or `session` approval proceeds without an approval-broker read. A cache miss, stale cache, or never-synchronized cache follows FR-005 before approval creation.

For a matching `once` approval, ExtProc first reserves the record locally with compare-and-swap. It forwards the token only after `POST /api/approvals/{id}/consume` returns `200 OK`.

If consumption fails or returns a status other than `200 OK`, ExtProc releases the reservation and denies the call. The local reservation gives best-effort at-most-once use within one ExtProc instance.

## Rationale

The response identity prevents a forged caller claim from selecting another user's approval. It also prevents a second identity endpoint or a JWT parsing feature in ExtProc.

The approval cache keeps fresh permanent and session approvals off the broker request path. The ETag protocol updates state after changes and the freshness limit prevents an unavailable broker from extending an old grant indefinitely.

The targeted read has a different purpose from the long poll. It establishes an authoritative, current miss before ExtProc creates a user-visible pending approval.

The ETag regression rule prevents a delayed or inconsistent targeted response from restoring a revoked approval. Full-snapshot replacement prevents the same problem when a later synchronization removes a record.

The gate belongs after OPA because OPA decides whether approval is required. Approval data does not enter the OPA input, so OPA policies remain independent of broker synchronization state.

## Separate from ADR 012

ADR 012 caches exchanged tokens by `(subject_token, resource_uri)`. This ADR caches approval records by verified `(principal, agent_id)` and then matches tools, parameters, and persistence scope.

ADR 012 invalidates token data at token expiry and `cache.max_ttl`. This ADR updates approval data by ETag synchronization, removes inactive pairs, and rejects authorization after `tool_approvals.max_staleness`.

ADR 012 can return a bounded stale token during a transient exchange error. This approval cache must deny when it lacks a fresh broker confirmation or when the required authoritative read fails.

The mechanisms share an in-memory process-local implementation pattern. Their keys, invalidation rules, and failure postures differ, so they remain separate caches with separate configuration.

## Consequences

### Positive

- Fresh `permanent` and `session` matches require no approval-broker read.
- Revocation exposure is bounded by a successful long-poll cycle in normal operation and by `tool_approvals.max_staleness` during synchronization failure.
- A cache that exceeds `tool_approvals.max_staleness`, a rejected ETag, or a failed authoritative read is an alertable denial signal.
- Only broker-verified identity values scope approval records.

### Negative

- ExtProc maintains a second in-memory cache beside the ADR 012 token cache.
- Operators must set and monitor a per-feature authorization freshness policy through `tool_approvals.max_staleness`.
- A stale cache or broker outage can deny an approval-required tool call and add an authoritative read before a new approval is created.
- Each ExtProc instance maintains its own cache and long-poll connection.

## Architecture Findings Addressed

This ADR answers architecture findings 2, 3, and 7. It defines the verified approval identity source, the body-phase approval-gate location and one-time consumption order, and the broker-authority freshness boundary.

## Relationships

- **ADR 011**: ExtProc remains a standalone process with its own `EXTPROC_` configuration boundary. `tool_approvals.max_staleness` belongs to that boundary.
- **ADR 012**: The token cache stays separate because its key, invalidation model, and failure posture differ from the approval cache.
- **ADR 014**: The approval cache uses the ETag long-poll endpoint and its broker wake-up behavior.
- **ADR 018**: Sync and targeted reads use the client assertion. Approval creation uses the subject token and client assertion. One-time consumption uses the subject token.
- **ADR 028**: Embedded OPA makes the `approval_required` decision. The gate acts on that decision after OPA evaluation.
- **ADR 029**: The proposed client-assertion trust anchor defines the assertion trust relationship used by sync, targeted reads, and approval creation.
- **ADR 035**: ExtProc uses the sanctioned `internal/domain/approval/toolpattern` import for approval matching. This ADR does not permit other ExtProc imports from `internal/domain`.

## Alternatives Considered

1. **Read the broker for every approval-required call**: Rejected because it adds request-path latency and load even when a fresh approval is already available.
2. **Reuse the ADR 012 token cache**: Rejected because token and approval data have different keys, invalidation triggers, and failure rules.
3. **Authorize with an unbounded stale approval cache**: Rejected because a broker outage could extend a revoked approval indefinitely.
4. **Derive identity by parsing a token in ExtProc**: Rejected because ExtProc cannot verify that token and could use a forged claim as an authorization key.

## References

- [ADR 011: ExtProc Token Exchange as Standalone Binary with Separate Configuration Schema](011-extproc-standalone-binary.md)
- [ADR 012: In-Memory Token Cache for ExtProc Token Exchange](012-extproc-in-memory-token-cache.md)
- [ADR 014: Long-Poll with PostgreSQL LISTEN/NOTIFY for Approval Sync](014-long-poll-listen-notify.md)
- [ADR 018: Approval Endpoint Authentication Boundaries](018-approval-endpoint-auth-boundaries.md)
- [ADR 028: OPA-Based Authorization in ExtProc via Embedded SDK](028-opa-extproc-authorization.md)
- [ADR 029: Token Exchange Client-Assertion Trust Anchor](029-token-exchange-client-assertion-trust-anchor.md)
- [ADR 035: Shared Tool Pattern Matching](035-shared-tool-pattern-matching.md)
- Feature specification: `specs/026-extproc-approval-sync/spec.md`
