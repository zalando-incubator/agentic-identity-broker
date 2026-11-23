# Implementation Plan: Configurable Third-Party Service Icons

**Branch**: `221-configure-service-icons` | **Date**: 2026-11-19 | **Spec**: [spec.md](spec.md)

## Summary

Store optional SVG source on the existing service. Carry it through existing authenticated Admin, consent and session JSON. Convert it to an encoded data image only at `<img>` rendering. Preserve source exactly, per-view fallbacks and controls. No image endpoint, asset service, remote fetching or third-party fixture.

The [contract](contracts/service-icons.md) defines the upload rules, rendering boundary and API field decision. [research.md](research.md) records the rationale and alternatives.

## Technical Context

**Language/Version**: Go 1.27.4, React 19, TypeScript 6.0.x; existing Go/React monorepo.
**Primary Dependencies**: Standard Go `encoding/xml`, `encoding/json`, `unicode/utf8`; existing chi, sqlx/pgx, React and API clients. Browser SVG image mode supplies capability isolation. No sanitizer, browser runtime on the server or new dependency.
**Storage**: Existing service aggregate; nullable PostgreSQL text column and equivalent memory field. No session snapshot or icon repository.
**Testing**: Existing Go unit tests, real PostgreSQL migrations/repository tests, Ginkgo acceptance and Playwright browser tests, Vitest caller/image tests.
**Target Platform**: Broker and existing root-mounted SPA; the existing Chromium suite verifies browser image-mode isolation. No browser matrix or CI expansion is required.
**Performance Goals**: No dependency fetch during upload; zero extra image HTTP requests; no icon-only N+1 queries. Browser rendering quotas and stress tests are explicitly out of scope.
**Constraints**: 65,536 decoded UTF-8 bytes, well-formed single SVG root, exact accepted source, named-construct rejection, authenticated JSON, image-only rendering. No full CSS/reference/animation analysis.
**Scale/Scope**: Twelve acceptance scenarios; Admin lifecycle, existing consent services/requirements and permission-set rows, session cards/detail headers. No editor, standalone raster upload, configuration, Helm, token or unrelated avatar changes.

## Constitution Check

The existing service aggregate owns the icon; no new entity or typed ID is needed. Existing storage and SPA boundaries remain applicable.

| Principles | Decision and remaining gate |
|---|---|
| I, III | Standard XML parser plus named rejections; browser secure animated image mode. The stakeholder approved the security design and CSP. T004 still requires documented security analysis and resolution of blocking findings before implementation. Cover authenticated JSON, copied-source limitations, Chromium isolation and remote-avatar compatibility. Do not claim security checks ran merely because the design is approved. No security toggle. |
| II, V | Add Service Icon glossary/trust-boundary text to ARCHITECTURE.md. Write the timestamp-named ADR with status Accepted under the recorded stakeholder approval. No further ADR sign-off is required. Accepted `004-storage-layer-architecture` and `035-root-mounted-spa` remain binding. |
| IV, X | The stakeholder approved all API drafts and current design decisions in this conversation, as recorded in contracts/service-icons.md. Publish both canonical OpenAPI contracts and rendered docs before behavior. Leave the unused ThirdpartyService schema and logoUrl unchanged; add no available_services response. |
| VI, XII | Domain service enforces source invariants; existing ports/adapters persist/project. No handler-to-repository shortcut or routing construction. No new dependency requiring builder changes. |
| VII | No runtime configuration; no YAML or Helm changes. |
| VIII, XIII | Write twelve canonical complete-journey acceptance cases and focused unit tests before behavior. Browser cases include meaningful screenshots; no pending/skipped placeholders. |
| IX | Both adapters; next unused atomic migration pair; production-chain up/down/up and PostgreSQL restart tests. |
| XI, XIV | Existing semantic tokens, decorative image semantics, visible names, existing wireframes and original inline illustration. No required third-party artwork. |
| XV | No new telemetry; review existing error paths so source/credentials never enter signals. Read binding standards before changing telemetry. |

**Gate outcome:** All stakeholder approvals for the current design are complete. No further sign-off is required. Implementation prerequisites remain: security analysis, resolution of blocking findings, canonical API publication, architecture/glossary updates, an ADR recording the approved design with status Accepted, and test-first cases. These are unfinished work, not pending approval requests. Rendering quotas remain out of scope. `tasks.md` records the execution order.

## Project Structure

### Documentation

`spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/service-icons.md`, `contracts/schemas.yaml`, `quickstart.md`, `tasks.md` and existing wireframes stay in this feature directory.

### Source areas

Extend `internal/domain/model/` and `internal/domain/thirdparty/` for the optional source and actual persisted preserve/clear/replace merge. Reuse `ServiceRequest.UnmarshalJSON` presence handling in `internal/adapters/http/handlers/admin/services_handler.go`. Validation must occur before discovery/credential/persistence side effects; do not mutate a memory-backed stored pointer and then validate.

Extend existing memory/PostgreSQL service records and explicit SQL columns; use `migrations/NNN_service_icon_svg.{up,down}.sql` with the next unused number, not a reserved guessed slot. Preserve row versions and existing ETag behavior; do not add a mandatory icon-only If-Match requirement.

Carry current service metadata through `internal/domain/consent/`, `internal/domain/oauth2session/`, existing handlers and session summary read models. Add `iconSvg` only to the existing `ServiceRequirementForUser` projection in `data.services`. Permission-set rows reuse that metadata through their current frontend mapping. Preserve service filtering; add no available_services catalog or source on permission-set scope records.

Use one service-specific image composition under `web/src/components/services/`, existing consent/session callers and API mappings. Session details use TerminationDialog, not a new page. Preserve name-only session fallback and existing consent initials. Source-key only the image, never action controls. Do not alter generic avatars.

## Implementation Phase Overview

| Phase | Purpose |
|---|---|
| Setup | Read nearest scoped instructions; confirm exact migration slot and existing harness prerequisites. |
| Mandatory design | Use the recorded approvals, write the Accepted ADR, update architecture/glossary, publish OpenAPI/docs and write acceptance tests. |
| Foundations | XML input policy, exact-source semantics, safe image composition and scoped SPA policy, each driven by tests. |
| US1 | Presence-aware Admin writes, both adapters, current metadata projections and every identity surface. |
| US2 | Prove named rejection and actual browser isolation, including CSS/animated/indirect references left in accepted source. |
| US3 | Original fallbacks, changed-source recovery, usable controls and accessibility at narrow/wide sizes. |
| Compliance | Targeted checks, real PostgreSQL restart/migration proof, browser screenshots, docs and full PR gate. |

Skip preparatory refactoring and entity scaffolding: no demonstrated structural blocker or new entity exists. Security and fallback are part of the first usable feature, never optional later additions.

## Testing Strategy

### Canonical acceptance mapping

Each row is one future Ginkgo It with its scenario ID. Proposed paths are design targets, not claims that tests exist.

| Scenario | Suite | Complete observable |
|---|---|---|
| US1.1 | Backend/docker | Admin create/read/list and real PostgreSQL-backed broker restart, then exact source in fresh authenticated consent/session JSON. |
| US1.2 | Backend | Replace source without changing identity, grants, sessions or OAuth configuration. |
| US1.3 | Browser | Omission preserves displayed source; null clears it and fresh data restores original fallback. |
| US1.4 | Browser | URL/path/empty input rejects atomically without fetch. |
| US2.1 | Browser | Original visual examples round-trip exactly and decode in consent/session images, with no icon-related external requests. |
| US2.2 | Browser | Fresh metadata replaces predecessor image without changing the other service. |
| US2.3 | Browser | Named active constructs fail atomically; accepted external references remain inert in actual image mode. |
| US2.4 | Backend | Unauthenticated callers get no source through any actual delivery response. |
| US3.1 | Browser | Pre-feature/no-icon services retain original identity and working actions. |
| US3.2 | Browser | Deterministic decode error yields fallback and no page error; controls work. |
| US3.3 | Browser | Fresh changed source recovers in the open page without remounting controls. |
| US3.4 | Browser | Decorative images, readable names, keyboard controls and narrow/wide layout. |

Backend cases belong in `tests/e2e/service_icons_test.go`; browser cases in `tests/e2e/frontend/service_icons_test.go`, using existing bootstrap/page objects and isolated fixtures. US1.1 uses the existing backend Docker restart pattern in `thirdparty_protected_resource_status_test.go`, retaining PostgreSQL data without reseeding. PostgreSQL integration tests cover exact-source persistence too. Do not add restart support to the browser harness. Supplemental checks do not duplicate canonical mappings.

### Unit, integration and browser proof

Cover every named XML rejection, namespace/character-reference variants, valid Unicode, exact byte boundary, omit/null/value handling and exact accepted-source round trips. Positive examples include styles, filters, masks, text, patterns, local reuse, embedded media, animation and SVG 1.1 DOCTYPE declarations without internal subsets. Reject internal subsets and entity declarations; never resolve a DTD. Do not reject all CSS, references or animation to pass negative tests.

Use actual built SPA headers and Chromium network/execution observation. Test accepted references in styles, attributes and changing values; inspect absence of inline SVG children and script execution. Verify exact source in API round trips and decoded data URIs, with images complete and naturalWidth > 0. Preserve remote user-avatar rendering under the revised CSP. Retain meaningful-state screenshots; no image-diff tooling, synchronized animation comparisons or Firefox/WebKit CI changes are required. Do not treat a brief render as a future security or resource-use proof.

Test memory/PostgreSQL parity, atomic rejected updates with simultaneous credentials/metadata changes, unchanged preexisting timestamps, migration rollback/reapply and restart durability. Use nullable text plus octet_length check; application XML validation remains authoritative for content.

Consent/session caller and API-mapping unit tests precede caller integration. Existing caches last five/two minutes: observe a genuinely fresh network response through supported expiration/invalidation before asserting replacement/clear/recovery. Do not change cache lifetimes or introduce live push.

Capture meaningful configured/unset/failed/recovered states at narrow/wide sizes under `tests/e2e/screenshots/`. Use original synthetic SVG only. Run the implementation checks in [quickstart.md](quickstart.md).

## Complexity Tracking

No constitution exception is requested. Avoid a custom SVG/CSS capability analyzer, sanitization comparison, server browser, rendering derivative, new repository or asset subsystem. The accepted source can retain inactive references; image mode, not arbitrary reuse safety, is the documented rendering contract.
