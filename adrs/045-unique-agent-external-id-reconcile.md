# ADR 045: Unique Agent External ID and Reconcile by External ID

**Status**: Accepted (approved by mjungsbluth on 2026-11-04: https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6629376767)
**Date**: 2026-10-30 (revised 2026-11-04)
**Revision**: Withdraws the microsecond timestamp presentation and `+1 microsecond` advancement
(API-TIMESTAMPS) on maintainer direction. Admin Agent timestamps keep the whole-second RFC 3339
presentation of Admin API 2.1.0.
**Feature**: 055-agent-external-id-upsert (issue #334)
**Supersedes**: No ADR. Replaces feature 054's non-uniqueness clarification and FR-011, which are specification decisions.

## Context

Governance sync jobs identify agents by `external_id`, an identifier from an external AI registry.
Feature 054 added an exact `GET /api/agents?external_id=` filter and deliberately allowed several
agents to share one external ID and stored empty strings. A sync job must therefore read, decide
between `POST` and `PUT /api/agents/{agent-id}`, and write. Retries after lost responses and
concurrent runs create duplicate agents.

Issue #334 asks for one idempotent call that puts the desired state of an agent under its external
ID: `PUT /api/agents?external_id={value}`. That is only dependable when every write path keeps
the external ID unique.

The Zalando RESTful API Guidelines, which Principle IV adopts, describe `PUT` as usually applied to
single resources and creating a resource only when the client addresses it. A `PUT` on a collection
URL with a query selector is not the guideline's usual shape. The broker has no earlier operation
that creates or updates by an alternate key.

## Decision

1. **Invariant.** A set Agent External ID is globally unique by exact, case-sensitive equality and
   is valid UTF-8 text with 1–255 Unicode code points and no NUL character. Any number of agents may have no external ID. Both storage adapters
   enforce the invariant atomically: PostgreSQL with the partial unique index
   `uq_agents_external_id` and check constraint `chk_agents_external_id_not_empty`, and the memory
   adapter with an index guarded by its write lock. Violations surface as
   `StorageError{Kind: Conflict}` wrapping `ports.ErrExternalIDConflict`.
2. **Collection PUT as reconcile.** `PUT /api/agents?external_id={value}` reconciles the Agent definition
   keyed only by the external ID. It returns `201` with a UUID `Location` on creation, `200` on
   update, and `200` with no write when the effective definition is unchanged. It never selects by
   UUID, canonical ID, or client ID. The body is `AgentCreateRequest`. The update branch follows
   the FR-012 field policy: omitted `client_id` and `canonical_id` are preserved (ADR 017), and
   metadata and collections are replaced. No-op compares client URIs as a set; shared Admin Agent
   presentation sorts a copied URI slice in ascending Go string byte order on all write/read
   responses, without changing URI spelling, stored order, or other collections (API-ORDER).
   Timestamps follow the existing write paths: creation sets both to UTC now, and an actual update
   preserves `CreatedAt` and sets `UpdatedAt` to UTC now. No-op preserves stored times without a
   write, so a replay without an intervening change returns the unchanged representation,
   including `updated_at`. All Admin Agent timestamps keep the whole-second RFC 3339 presentation
   of Admin API 2.1.0. A change within the same second as the previous write may present an
   unchanged `updated_at`; it is not a revision or concurrent-write guarantee.
3. **Race handling without locks.** The domain service reads through `GetByExternalID`, which
   returns one Agent or not-found, then calls repository `Create` or `Update`. Only a create
   error wrapping `ports.ErrExternalIDConflict` triggers one reread; when a holder now exists,
   reconciliation continues on the update branch. Other conflicts propagate without a reread.
   No advisory lock is added. Without `If-Match`, concurrent different
   definitions are last-writer-wins.
4. **Safe migration.** Migration `046` locks `agents`, inventories duplicate and empty external IDs,
   and fails before changing anything with a report covering both duplicate groups and empty-value
   agent UUIDs. The report lists the first 100 of each category with remaining counts; the runbook
   provides the complete read-only inventory of every affected value and UUID. It never clears,
   merges, deletes, or reassigns data. The runbook covers remediation, retry, and rollback. The migration remains atomic; ADR 039's non-atomic exception is not extended.
5. **Versioning.** The new `409` and `400` outcomes on existing create and update operations are
   breaking. The Admin API moves from `2.1.0` to `3.0.0` on the existing routes, following
   ARCHITECTURE.md §5 and the feature 051 precedent.

The Agent domain service uses one candidate-building and validation helper for ID-addressed
updates and existing-Agent reconciliation. It owns keep-or-clear identity policy, entity validation,
service reference validation, permission-set service coverage (FR-019), and client-ID uniqueness.
Create enforces the same coverage rule. Handlers parse presence but do not decide field policy.
All selectors and Agent writes share the full External ID format validator. Reconcile outcomes are
`ReconcileCreated`, `ReconcileUpdated`, and `ReconcileUnchanged`; errors have no outcome.

These F1–F5 revisions were approved in the [remediation proposal](https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6629376767);
the [earlier approval](https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6625694887)
continues to cover the unchanged API, migration, ordering, and timestamp decisions.

## Consequences

- Sync jobs reconcile one registry entry with one retry-safe call.
- Existing clients that relied on shared or empty external IDs receive `409` or `400` and must
  migrate. Upgrades stop until operators resolve legacy duplicates and empties.
- `GET /api/agents?external_id=` keeps its array shape but returns at most one agent. Admin Agent
  responses consistently sort client URIs; timestamp presentation is unchanged. Callers detect an
  update by the returned definition, not by `updated_at`, which can stay the same for two writes
  within one second and is not an ETag.
- A future alternate-key reconcile operation for another resource should follow this pattern:
  adapter-enforced uniqueness, collection `PUT` with a required selector, bounded conflict re-read,
  and effective-definition no-op detection.
- The deviation from the Zalando guidelines' usual single-resource `PUT` is limited to this
  operation and documented in the OpenAPI description.

## Alternatives considered

- **`PUT /api/agents/by-external-id/{value}`**: closer to a single-resource `PUT`, but external IDs
  such as URNs and URLs may contain `/` and other characters that proxies normalize in paths, and
  issue #334 fixes the collection shape.
- **`POST /api/agents` with an `Idempotency-Key` header**: makes retries safe but does not update an
  existing agent and needs stored idempotency records.
- **`INSERT … ON CONFLICT DO UPDATE` repository upsert**: cannot express the domain field policy,
  no-op detection, or configuration-dependent client-ID uniqueness, and needs a parallel memory
  implementation.
- **Automatic duplicate cleanup in the migration**: violates FR-004 and can lose governed agents
  and their grants.
- **Preserve client-URI input order**: requires persisted PostgreSQL ordinal positions to match
  memory; response-only sorting avoids a schema addition and makes create/update replays stable.
- **Microsecond timestamps with `+1 microsecond` minimum advancement** (the earlier API-TIMESTAMPS
  text): makes every same-second update observable, but changes every Admin Agent response and can
  put `updated_at` ahead of wall time for a guarantee the issue does not require.
- **Whole-second timestamp advancement**: preserves former formatting but requires a full-second
  logical jump for rapid changes, causing larger wall-clock drift.
- **Keep Admin API 2.x**: treats a behavior change that breaks existing callers as a fix, which
  Principles IV and X forbid.
