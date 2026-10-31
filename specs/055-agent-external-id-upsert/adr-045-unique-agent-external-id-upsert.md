# ADR 045: Unique Agent External ID and Create-or-Update by External ID

**Status**: Proposed
**Date**: 2026-10-30
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
   is never the empty string. Any number of agents may have no external ID. Both storage adapters
   enforce the invariant atomically: PostgreSQL with the partial unique index
   `uq_agents_external_id` and check constraint `chk_agents_external_id_not_empty`, and the memory
   adapter with an index guarded by its write lock. Violations surface as
   `StorageError{Kind: Conflict}` wrapping `ports.ErrExternalIDConflict`.
2. **Collection PUT as reconcile.** `PUT /api/agents?external_id={value}` is a create-or-update
   keyed only by the external ID. It returns `201` with a UUID `Location` on creation, `200` on
   update, and `200` with no write when the effective definition is unchanged. It never selects by
   UUID, canonical ID, or client ID. The body is `AgentCreateRequest`. The update branch follows
   the FR-012 field policy: omitted `client_id` and `canonical_id` are preserved (ADR 017), and
   metadata and collections are replaced.
3. **Race handling without locks.** The domain service reads by external ID, then calls the
   existing repository `Create` or `Update`. If `Create` reports any conflict, the service re-reads
   by external ID once and, when a holder now exists, continues on the update branch. No new
   repository method or advisory lock is added. Without `If-Match`, concurrent different
   definitions are last-writer-wins.
4. **Safe migration.** Migration `046` locks `agents`, inventories duplicate and empty external IDs,
   and fails before changing anything with a report covering both duplicate groups and empty-value
   agent UUIDs. The report lists the first 100 of each category with remaining counts; the runbook
   provides the complete read-only inventory of every affected value and UUID. It never clears,
   merges, deletes, or reassigns data. The runbook covers remediation, retry, and rollback. The migration remains atomic; ADR 039's non-atomic exception is not extended.
5. **Versioning.** The new `409` and `400` outcomes on existing create and update operations are
   breaking. The Admin API moves from `2.1.0` to `3.0.0` on the existing routes, following
   ARCHITECTURE.md §5 and the feature 051 precedent.

## Consequences

- Sync jobs reconcile one registry entry with one retry-safe call.
- Existing clients that relied on shared or empty external IDs receive `409` or `400` and must
  migrate. Upgrades stop until operators resolve legacy duplicates and empties.
- `GET /api/agents?external_id=` keeps its array shape but returns at most one agent.
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
- **Keep Admin API 2.x**: treats a behavior change that breaks existing callers as a fix, which
  Principles IV and X forbid.
