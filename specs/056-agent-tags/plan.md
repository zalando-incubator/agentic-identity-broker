# Implementation Plan: Agent Tags

**Branch**: `repo-assist/fix-issue-427-agent-tags-design` | **Date**: 2026-11-14
**Spec**: [spec.md](spec.md) | **Contract proposal**: [contracts/agent-tags.md](contracts/agent-tags.md)
**Status**: Draft plan, awaiting detailed API review; implementation not started.

🤖 Prepared by Repo Assist, an automated AI assistant.

## Summary

Add administrator-controlled literal string tags to the existing Agent aggregate, persist them through both repositories, and display them in the existing end-user delegation overview and Agent detail/consent header. Defer all tag filtering and admin UI work. No new dependency, service, repository interface, route, or entity ID is required.

The maintainer clarified representation and display scope, not the detailed API semantics. The proposed 32-member/255-code-point bounds, duplicate/null rejection, omission-as-empty, response omission/order, and additive API versions require written approval before runtime implementation. This PR supplies review documents and Wireloom artifacts only; it does not amend deployed contracts or mark the feature complete.

## Technical Context

- **Language/version**: Go 1.27.3 (`go.mod`); React 19, TypeScript 6.0.x, Node.js >=24 (`web/package.json`). Recheck manifests when implementing.
- **Primary dependencies**: Existing chi, sqlx/pgx, lib/pq array transport, Ginkgo/Gomega, Vitest/Testing Library, and Playwright Go harness. Add none.
- **Storage**: Existing memory Agent repository and PostgreSQL `agents` table. Proposed `tags text[] NOT NULL DEFAULT '{}'` column; no search index or new table.
- **Platform**: Broker dual-server application and root-mounted SPA, preserving accepted ADRs 004 and 035.
- **Scale/performance**: Maximum 32 members per Agent, 255 Unicode code points per member; at most O(n log n) presentation/equality on copied slices. Carry tags in existing Agent reads, with no additional queries per delegation.
- **Configuration**: None. No change to broker config port, CLI, Helm, ExtProc, or infrastructure.

## Constitution Check

| Principle | Design disposition / remaining gate |
|---|---|
| I / III | Tags confer no privilege; retain existing auth, Host/origin/JSON protections and use escaped text. No crypto change. |
| II / V | Agent owns tag value set. Add glossary and architecture description once the design is approved. No deviation from accepted ADRs is proposed. |
| IV / X | Detailed contract is proposed, not approved. Update both authoritative OpenAPI contracts and record approval before implementation. |
| VI / XII | Entity validation and reconcile equality stay in domain; handlers parse/present; adapters persist; existing Builder wiring is sufficient. |
| VII | No new config or deployment settings; verify that boundary rather than creating empty examples. |
| VIII / XIII | Tests and semantic-red evidence remain mandatory before production changes; map all ten scenarios to one Ginkgo It each, with browser and database evidence. None is claimed implemented here. |
| IX | Both storage backends, atomic sequential migration, complete PostgreSQL read-path and rollback coverage. Container evidence must come from an appropriate CI environment. |
| XI / XIV | Existing Badge/Stack/Card primitives, semantic roles, both themes, escaped text, wrapping, linked journeys and rendered editable Wireloom source. Browser screenshots remain implementation work. |
| XV | No new logs/spans/metrics planned. Read applicable standards before implementation; preserve mutation audits and absence on replay without adding raw tags to telemetry. |

**Blocking checkpoint**: approval, authoritative contract changes, glossary/schema design, frontend review, and semantic-red acceptance tests are not complete. Do not start production implementation until Phase 2 in [tasks.md](tasks.md) is complete. These documents are not authorization to deviate from an Accepted ADR.

## Verified Current Sources and Change Map

| Area | Observed behavior | Planned change |
|---|---|---|
| `internal/domain/storage/agent.go` | Agent contains metadata/collections; `Copy` clones slices; no tags | Add owned tag slice, pure validation, and copy isolation. Do not introduce infra imports or a new bounded context. |
| `internal/domain/agents/service.go` | Shared update candidate validates Agent and effective service coverage | Retain shared policy; validate tag constraints on all writes without changing identity or reference rules. |
| `internal/domain/agents/reconcile.go` | `sameDefinition` compares known fields; client URIs are compared as a set | Include tag membership independent of order. Validate before equality; no-op returns existing copy without persistence/audit. |
| `internal/adapters/storage/memory/agent_repository.go` | Existing entity create/update/get/batch contracts | Use Agent copy/validation, testing input/output alias safety. No tag index. |
| `internal/adapters/storage/postgres/agent_repository.go` | Explicit column lists and manually paired scans across multiple reads | Add array write/scan bindings to create/update, Get, GetByIDs, List, GetByExternalID, GetByClientID, and client-URI resolution. Canonical lookup delegates to Get. Test every route, not only Get. |
| `internal/adapters/http/handlers/admin/agents_handler.go` | One AgentRequest and response projections serve multiple operations | Parse explicit null/wrong type correctly, convert tags, sort response copies, omit empty metadata, and keep existing error envelopes. |
| `internal/domain/consent/service.go` | Delegation aggregation batches Agent reads via GetByIDs; timestamps come from grants | Copy Agent tags into AgentDelegation without extra queries or changing grant stats. |
| `internal/adapters/http/handlers/consent/agent_detail_handler.go` | `AgentMetadata` projects `detail.Agent` into `data.agent` | Add sorted tags from the stored Agent, not CIMD content or session credentials. |
| `web/src/types/consent.ts` and `web/src/services/api/consent.ts` | Raw detail/list responses and view types are separate | Carry optional tags through actual API transforms; missing tags must remain safe for older backends/fixtures. |
| `web/src/components/consent/DelegationCard.tsx` | Custom React.memo comparator checks grant timestamps/count/expiration, not all Agent metadata | Include displayed tag changes in comparison so unchanged grant timestamps do not suppress a tag-only update; add a rerender regression test. |
| `web/src/pages/AgentGrantDetailPage.tsx` | Existing header and consent/grant editing UI | Reuse a noninteractive application-specific tag list; do not change grant controls or add filters/editor. |
| `api/admin/openapi.yaml`, `api/enduser/openapi.yaml` | Admin 3.0.0, end-user 1.2.0; Agent and consent schemas have no tags | Apply approved optional-property changes and versioning, keeping existing names/envelopes. |

## Design Details

### Domain and write policy

Keep tags as an Agent-owned collection, represented in Go by a slice with set validation. Reject duplicates rather than silently deduplicating. Literal matching includes case, whitespace, and Unicode normalization form. Domain validation checks UTF-8/no-NUL/length/cardinality; HTTP parsing must not lose explicit null or coerce element types. Review malformed JSON/encoding behavior before relying on `encoding/json`, which may replace malformed text.

Omitted/empty tags replace with the empty set on all write paths, consistent with ADR 047's metadata/collection replacement policy. Keep the existing special identity policies intact. Deep-copy before sorting or persisting. Compare order-independent membership on copied values; ordinary ID-addressed PUT retains its current write semantics, whereas reconcile retains no-write replay.

Tags are categorization metadata only. Do not add them to JWT claims, CEL inputs, authorization/client classification, discovery/CIMD, ExtProc, grant records, or telemetry. Audit real mutations through the existing mechanisms, and verify tag-only reconciliation is not accidentally treated as a no-op.

### Persistence and rollout

Allocate the next free migration number at implementation time; main currently ends at 047 but other branches may add migrations. Use a standard atomic UP/DOWN migration with a non-null empty-default array. Existing rows acquire no tags and must retain all pre-feature identities, timestamps, and relationships. Do not add concurrent DDL or extend ADR 039's exception.

Keep database array shape consistent with an ordinary one-dimensional Go string slice and reject null elements. Enforce domain rules in both adapter write paths as well as the domain service. Review appropriate database shape/cardinality constraints before freezing migration SQL; no filter index is justified.

Deploy migrations before the new binary. Old binaries use explicit projections, but old full-definition API clients must be warned that omission clears tags when talking to the new binary. Rollback of the column deliberately destroys tag metadata; preserve it externally if needed and document that trade-off. DOWN/reapply must preserve unrelated data, not pretend removed tags reappear.

### Frontend

Use an application-specific Agent tag list composed from existing Badge/Stack primitives; no new universal design-system component is required. Use a labelled semantic list with neutral badges, no interactive semantics, and wrapping for long values. Omit the list when absent/empty. Reuse shared semantic tokens and existing visual direction, not success/verified colors or raw palettes.

The delegation list and detail raw data shapes differ; update both transformations/types. Keep existing API cache TTLs and user scoping. Verify tags change after an existing reload even when the grant's lastModifiedAt is unchanged; do not bump grant timestamps as a rendering workaround. No live subscription/polling is added.

## Testing Strategy

**Backend acceptance location**: `tests/e2e/agent_tags_test.go` using Ginkgo/Gomega, production bootstrap, dual-server routes, fixtures and per-test isolation. Each scenario below gets exactly one referenced It, with table-driven variants inside a scenario where needed. Browser tests supplement rather than replace the backend mapping.

| Scenario | Planned It behavior |
|---|---|
| AT-1.1 | Create tagged Agent, retrieve via ID/list, verify sorted same members |
| AT-1.2 | Replace then clear tags, preserving UUID and delegation |
| AT-1.3 | Omit tags on legacy create/ID update/reconcile and verify empty set |
| AT-1.4 | Create/change/replay by External ID, verify stable exact stored times and no writes/audits on replay |
| AT-1.5 | Reject invalid sets on all write paths/branches and read back unchanged state |
| AT-1.6 | Preserve literal members across writes, lookup, and presentation |
| AT-2.1 | Fetch principal-scoped delegation list then detail with identical tags |
| AT-2.2 | Fetch untagged/legacy-compatible projections without a tag property |
| AT-2.3 | Mutate only tags, reread consent projections with unchanged grant stats |
| AT-2.4 | Supply safe literal maximum/markup-looking data through the complete end-user HTTP journey |

**Browser lane**: Add `tests/e2e/frontend/agent_tags_test.go` following the existing page-object/Playwright harness. Cover overview → detail, empty removal, tag-only data reload, literal markup and maximum-length wrapping; run light/dark and narrow-viewport cases and save meaningful states under `tests/e2e/screenshots/`. HTTP scenario AT-2.4 supplies the data; this browser lane proves the actual rendering/accessibility criteria.

**Focused regression tests**:

- Domain validation table: zero/32/33 members, 1/255/256 code points including multibyte/non-BMP, empty, duplicates, null/type handling at HTTP boundary, UTF-8/NUL and literal differences.
- Agent Copy and both adapters: input/output aliasing, clear/replace, failed writes preserve state.
- Effective-definition field-coverage test in `internal/domain/agents/reconcile_test.go`: add tags to the existing explicit field-mutation catalogue; test reordered sets and a genuine tag-only difference.
- PostgreSQL integration: production migrations; create/update/all read paths, legacy rows, array shape/null-element constraints, UP/DOWN/reapply, relationship/timestamp preservation and tag-loss rollback warning.
- Admin/consent handler contracts: raw JSON presence/type cases, every projection, no null/empty tags property, sorted copies without mutating domain records, unchanged authentication/error/header behavior.
- Vitest: tagged/untagged/maximum text rendering, raw API transformations, escaped markup, and memoized-card rerender with unchanged grant stats.

Run semantic-red tests before implementation, then green with minimal test changes. Local runnable tests must not wait for missing container infrastructure; container suites need real CI evidence on the implementation commit. Never mark an unrun suite passed.

## Implementation Phase Overview

Phase 1 records the feature selection and approvals. Phase 2 contains mandatory design/contract/schema/acceptance-test gates. Phase 3 implements persistence and API behavior after red evidence. Phase 4 implements end-user display after browser/unit red evidence. Phase N performs mandatory compliance and full verification. No preparatory refactor or entity-boilerplate phase is warranted: this is metadata on an existing aggregate.

Use `SPECIFY_FEATURE_DIRECTORY=specs/056-agent-tags` when selecting this feature for implementation. The repository currently selects `specs/054-agent-external-id-filter` in `.specify/feature.json`; this documentation-only PR deliberately leaves that unrelated tracked pointer unchanged.
