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
| I, III | Standard XML parser plus named rejections; browser secure animated image mode. The approved security design and CSP are recorded in Accepted `20261119-211813-service-icons-image-mode`. The security analysis below distinguishes design evidence from pending runtime checks. No security toggle. |
| II, V | ARCHITECTURE.md records Service Icon ownership, glossary and trust boundaries. Accepted `20261119-211813-service-icons-image-mode` records stakeholder approval. Accepted `004-storage-layer-architecture` and `035-root-mounted-spa` remain binding. |
| IV, X | The stakeholder approved the API drafts as recorded in contracts/service-icons.md. Canonical OpenAPI publication precedes behavior. No `available_services` response is added. The approved cleanup removes the unused service `logoUrl`. |
| VI, XII | Domain service enforces source invariants; existing ports/adapters persist/project. No handler-to-repository shortcut or routing construction. No new dependency requiring builder changes. |
| VII | No runtime configuration; no YAML or Helm changes. |
| VIII, XIII | Write twelve canonical complete-journey acceptance cases and focused unit tests before behavior. Browser cases include meaningful screenshots; no pending/skipped placeholders. |
| IX | Both adapters; next unused atomic migration pair; production-chain up/down/up and PostgreSQL restart tests. |
| XI, XIV | Existing semantic tokens, decorative image semantics, visible names, existing wireframes and original inline illustration. No required third-party artwork. |
| XV | No new telemetry; review existing error paths so source/credentials never enter signals. Read binding standards before changing telemetry. |

**Gate outcome:** All stakeholder approvals and preconditions completed before production behavior: security analysis, architecture/glossary updates, migration design, Accepted ADR, canonical API publication and twelve compiling test-first cases. Runtime verification follows `tasks.md`. Rendering-resource quotas remain out of scope.

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

Each scenario has exactly one canonical Ginkgo It. Backend journeys use `tests/e2e/service_icons_test.go`; browser journeys use `tests/e2e/frontend/service_icons_test.go`.
The browser journeys run only in built mode (`built-frontend`) with in-memory storage. The migration integration test checks that pre-feature PostgreSQL rows have no icon.

| Scenario | Suite | Canonical It |
|---|---|---|
| US1.1 | Backend/docker | should retain exact SVG source after a PostgreSQL-backed broker restart |
| US1.2 | Backend | should replace the icon without changing service identity grants sessions or OAuth settings |
| US1.3 | Browser | should preserve an omitted icon and restore original fallbacks after explicit null [US1.3] |
| US1.4 | Browser | should reject URL path and empty icon writes atomically without fetching them [US1.4] |
| US2.1 | Browser | should round-trip and decode original visual SVGs as isolated images while remote avatars render [US2.1] |
| US2.2 | Browser | should replace every fresh service identity without changing an identical-source peer [US2.2] |
| US2.3 | Browser | should reject named active constructs atomically and isolate accepted external references [US2.3] |
| US2.4 | Backend | should protect every icon delivery response and preserve ownership and rejected-write atomicity |
| US3.1 | Browser | should preserve initials name-only session identities and existing actions without an icon [US3.1] |
| US3.2 | Browser | should retain original fallbacks and working controls after deterministic image errors [US3.2] |
| US3.3 | Browser | should recover changed source in open consent and session views without a full page reload [US3.3] |
| US3.4 | Browser | should preserve decorative semantics readable names and keyboard actions at narrow and wide sizes [US3.4] |

Backend cases belong in `tests/e2e/service_icons_test.go`; browser cases in `tests/e2e/frontend/service_icons_test.go`, using existing bootstrap/page objects and isolated fixtures. US1.1 uses the existing backend Docker restart pattern in `thirdparty_protected_resource_status_test.go`, retaining PostgreSQL data without reseeding. PostgreSQL integration tests cover exact-source persistence too. Do not add restart support to the browser harness. Supplemental checks do not duplicate canonical mappings.

### Unit, integration and browser proof

Cover every named XML rejection, namespace/character-reference variants, valid Unicode, exact byte boundary, omit/null/value handling and exact accepted-source round trips. Positive examples include styles, filters, masks, text, patterns, local reuse, embedded media, animation and SVG 1.1 DOCTYPE declarations without internal subsets. Reject internal subsets and entity declarations; never resolve a DTD. Do not reject all CSS, references or animation to pass negative tests.

Use actual built SPA headers and Chromium network/execution observation. Test accepted references in styles, attributes and changing values; inspect absence of inline SVG children and script execution. Verify exact source in API round trips and decoded data URIs, with images complete and naturalWidth > 0. Preserve remote user-avatar rendering under the revised CSP. Retain meaningful-state screenshots; no image-diff tooling, synchronized animation comparisons or Firefox/WebKit CI changes are required. Do not treat a brief render as a future security or resource-use proof.

Test memory/PostgreSQL parity, atomic rejected updates with simultaneous credentials/metadata changes, unchanged preexisting timestamps, migration rollback/reapply and restart durability. Use nullable text plus octet_length check; application XML validation remains authoritative for content.

Consent/session caller and API-mapping unit tests precede caller integration. Existing caches last five/two minutes: observe a genuinely fresh network response through supported expiration/invalidation before asserting replacement/clear/recovery. Do not change cache lifetimes or introduce live push.

Capture meaningful configured/unset/failed/recovered states at narrow/wide sizes under `tests/e2e/screenshots/`. Use original synthetic SVG only. Run the implementation checks in [quickstart.md](quickstart.md).

## Complexity Tracking

No constitution exception is requested. Avoid a custom SVG/CSS capability analyzer, sanitization comparison, server browser, rendering derivative, new repository or asset subsystem. The accepted source can retain inactive references; image mode, not arbitrary reuse safety, is the documented rendering contract.

## Design Review — 2026-11-19

### Security analysis and decision

Accepted [20261119-211813-service-icons-image-mode](../../adrs/20261119-211813-service-icons-image-mode.md) records the approved design. Source inspection covered the provider Create/prepareUpdate/Update flow, Admin request presence handling, principal-protected end-user route group, SPA headers and consent/session identity callers. The assurance case confirms that Admin authorization belongs to the deployment proxy. This feature preserves that trust boundary rather than adding a new authentication mechanism.

| Risk | Required control and analysis | Verification status at design gate |
|---|---|---|
| Active SVG reaches page privileges | Only URI-encoded `<img alt="">` delivery. No markup interpolation, document endpoint, object, embed or iframe. Named upload rejections supplement image mode. | W3C SVG 2 secure animated mode and MDN image-context restrictions reviewed. Chromium execution/network observations remain implementation acceptance work. |
| Server-side reference or DTD fetch | Standard `encoding/xml` decoder, strict mode, no custom entity table or resolver. One pre-root SVG DOCTYPE without an internal subset is permitted. | `go doc` confirms strict entity handling, directive/PI tokens and input offsets. Duplicate attributes and document-level grammar require explicit checks. No dependency or resolver is needed. |
| Source disclosure | Existing end-user principal/ownership boundaries protect consent/session JSON. Admin remains proxy-authorized. No public/shared image resource or cache is introduced. | Current routing inspected. Actual response/authentication cases precede behavior. Authentication controls receipt, not reuse by an authorized recipient. |
| Copied source becomes an active document | API and architecture warn that source is supported only in image mode. Inert external references can remain in exact stored bytes. | Contract and Accepted ADR state the limitation. No claim of arbitrary reuse safety. |
| CSP breaks existing images | Preserve DENY/nosniff, add object-src none, retain frame-ancestors none. Add no default-src/img-src restriction or script/style permission. Parent CSP is not the SVG boundary. | Current SPA policy inspected. Header regression and actual remote-avatar Chromium rendering remain required. |
| Invalid icon partly changes credentials/configuration | Validate source before discovery, branch-key provisioning, encryption or persistence. Build an independent update candidate before merge. | Current Create performs discovery before ordinary entity validation, so the new icon check must run earlier. Actual prepareUpdate, not only ValidateUpdate's copy, must apply source presence semantics. |
| Payload and rendering cost | Nullable service metadata, 65,536 decoded UTF-8 bytes, no extra image requests or icon-only N+1 queries. | One hundred maximum-size occurrences cost 6.25 MiB before escaping/compression. URI encoding adds memory. Browser resource budgets and stress tests are explicitly excluded. |
| Sensitive signal data | No new telemetry. Fixed validation errors never contain source, parser text, credentials or personal data. | LOGGING.md and OPENTELEMETRY.md reviewed. Existing Admin decode/validation logs, provider audits and storage error spans require non-disclosure regression coverage. |

No blocking design finding remains. Runtime checks in this table are required acceptance work, not claimed results or further approval requests. A finite smoke render does not prove future safety, copied-source safety or bounded rendering work.

Structural triage, not a taint proof: the scoped Admin UnmarshalJSON forward query reached ServiceRequest/isJSONNull. The model AST scan reported no unsafe-call or weak-crypto findings and no truncated rows. Existing naming findings are outside this feature. Source inspection remains the evidence for the parser/error and browser trust boundaries; graph results do not prove dataflow safety.

### Configuration and persistence

Operator input is existing Admin JSON, not runtime YAML. No configuration, feature flag, `examples/config/`, Helm or builder change applies. The inspected next migration pair is `049_service_icon_svg.up.sql`/`.down.sql`. Nullable text enforces “NULL means absent; decoded UTF-8 source is 1–65,536 bytes”. There is no backfill, default, timestamp rewrite, session snapshot or new repository. Rollback loses only icon metadata.


The Admin handler enforces a supplied strong ETag when the request replaces `protected_resources`. An icon-only update does not add that field and keeps the existing no-mandatory-If-Match behavior. Concurrency tests exercise the protected-resource precondition separately from icon-only preserve/clear/replace.
### Frontend and examples

The design review covered INDEX.md, COMMON_MISTAKES.md, DECISION_TREES.md, DESIGN_PRINCIPLES.md and COMPONENT_PAIRING_GUIDE.md. Use the existing Card, Stack, Avatar, Button, Badge and Modal compositions. Keep their semantic colors, spacing, dimensions and light theme. ServiceIcon is service-specific composition, not a new universal avatar primitive. Consent retains its existing initials avatar; session identities retain name-only fallback. Do not use logoUrl as a remote icon fallback. Visible names supply accessible identity, and icon alt remains empty. Key only image state by source, never action controls.

The HTTP examples, original inline Atlas illustration, permitted DOCTYPE and existing configured/unset Wireloom source/renderings agree with the approved contract. No example or wireframe change is required. No JWT or runtime YAML example applies.

## Implementation Compliance Review

The implementation retains existing service IDs, ports, builder composition, API versions and configuration. Both adapters retain exact optional source. Current consent metadata stays within existing service requirements; session list/detail/refresh reuse their existing provider lookups. No icon repository, snapshot, extra service catalog or image endpoint exists. Canonical field casing and omission/null/value behavior match the approved contract and contract tests.

No log, span or metric call changed. Validator errors use fixed descriptions rather than parser/source text. Rejection tests assert that source and credential sentinels do not enter logs. The implementation adds no crypto, infrastructure dependency, signal registry, security switch or validation bypass. Existing proxy Admin authorization and end-user principal/ownership boundaries remain intact.

Review follow-up restored successful historical Agent upgrades to `MigrateTo(0)`. The first workaround had incorrectly restricted them to `047`. The fixture now supplies `049` only during the legacy-broker window, removes it before the real upgrade, and uses the production migration directory. Normal rollback/reapply runs the complete chain. Its original API, dirty-state and relationship-preservation assertions remain unchanged.

Ordinary Save and session actions use the maintained `ConsentPage` and `SessionsPage` objects. `ServiceIconsPage` retains image observations and keyboard traversal. The consent waiter accepts only a success toast that appears after the action.

The live consent service response never populated `logoUrl`. The approved cleanup removes the field from the unused schema rather than using it as an SVG fallback. Screenshot publication uses the same generated basenames, so the initial tracked captures are compatible with CI refresh. Detail-header variants remain because they cover required service identity surfaces.

After the corrections, the scoped quality delta reports 25 gating heuristics, including 22 page-object selector/wait matches. The corresponding pre-review counts were 27 and 27. The original icon Save/Refresh/Confirm/KeyboardSave clone groups are gone. Remaining matches normalize different selectors and fixed-path/factory patterns. They are not interchangeable operations. No findings were suppressed or acknowledged. New validator-loop and complete-journey test-length findings remain visible.
