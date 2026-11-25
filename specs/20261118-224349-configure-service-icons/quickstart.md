# Service Icons: Validation Guide

This guide records implementation and verification for configurable third-party Service Icons. See the [approved contract](contracts/service-icons.md), [data model](data-model.md) and [canonical acceptance mapping](plan.md#canonical-acceptance-mapping).

## Prerequisites

Use the Go and Node versions in the manifests, npm, just, Ginkgo, Playwright Chromium and Docker or Podman. Local verification used Go 1.27.4, Node 26.13.0, npm 11.20.0, just 1.59.0, Ginkgo 2.34.0 and Docker 29.5.2. Browser and PostgreSQL prerequisites worked.

The security script requires Bash with `mapfile`. The tool shell provides Bash 5.3, but commands launched by `just` selected the older platform Bash. Run the release gate with a modern Bash on PATH. No runtime configuration, Helm, dependency or ignore-file changes apply to this feature.

## Operator and API usage

Use the existing Admin service POST/PUT bodies. Supply SVG source, not an image URL, path or data URI. The complete HTTP examples are in the [API guide](../../docs/api/enduser-api.md#service-icons) and [specification journeys](spec.md#concrete-example-journeys-mandatory).

| `icon_svg` input | Create | Update |
|---|---|---|
| Omitted | No icon | Preserve current source |
| Null | No icon | Clear source |
| Permitted SVG string | Store exact source | Replace exact source |
| Invalid type, empty/invalid/prohibited/oversized source, URL or path | Reject entire write | Reject entire write |

Admin reads omit absent source. Existing consent `data.services` entries use `iconSvg`. Session list, detail and summary-returning refresh use current `service_icon_svg`, not a session snapshot. Clients replace cleared metadata rather than merge stale values. Existing authentication, row versions and five-/two-minute caches remain unchanged. Icon-only PUT adds no mandatory If-Match requirement.

The live consent response never populated service `logoUrl`. The field is removed from the unused schema. No remote URL is used as an SVG fallback.

## Observed verification

| Check | Result |
|---|---|
| `just check` | Passed after final fixture corrections, zero lint issues |
| `just web-build` | TypeScript and production Vite build passed |
| Affected Go package tests | All 10 packages passed |
| Focused frontend image/caller/API/page tests | 12 files, 110 tests passed |
| Service Icon acceptance | All 12 canonical scenarios passed: 3 backend and 9 browser |
| Live PostgreSQL migration/repository tests | Passed production-chain up/down/up, failed-DDL atomicity, byte-shape checks, independent copies, rejected updates and reconnect persistence |
| PostgreSQL-backed broker restart | Passed without reseeding, retaining exact source and existing service/grant/session state |
| `npm run build --prefix assets/docusaurus` | Passed, rendering both canonical OpenAPI documents. Existing CSS minimizer/Node warnings were non-fatal |
| `just test web-test cdk-test mock-sample-agent-test mock-upstream-oauth2-test test-integration-all` | All tiers passed before the later backend E2E contract failure |
| Full backend E2E | 961 of 962 cases initially passed. The approved CSP changed one exact-header contract. After updating it, all 3 focused root-SPA route cases passed |
| Full Docker backend E2E | Review follow-up restored full-chain upgrades. `just test-e2e-docker` passed all 16 cases. The rooted migration-file helper then passed all 4 historical Agent journeys |
| `just test-e2e-extproc` | 116 of 116 passed |
| `just test-e2e-frontend` | 76 of 76 passed with normal parallel workers, including all Service Icon browser cases |
| Review-focused browser E2E | All 16 affected Service Icon, session-control and consent-save journeys passed after maintained page-action reuse |
| Bootstrap security scan | `gosec -nosec-require-rules -nosec-require-justification ./tests/e2e/bootstrap` passed with zero issues. Migration files are read through `os.Root` |
| Security script in Bash 5.3 | Passed gosec, govulncheck and OSV under the repository's existing exceptions |

`just verify` was attempted. It stopped when the platform Bash lacked `mapfile`; it did not complete as one uninterrupted command. Its remaining test tiers ran separately. The unchanged security script passed from the tool's Bash 5.3 shell with:

```sh
(source scripts/security-scan.sh .)
```

GO-2026-6911 already has a maintained exception for the local `mocks/sample-agent` client until 2026-12-31. No exception, dependency or security control changed here. The initial E2E contract and fixture corrections received focused reruns before the full gate.

After those reruns, `PATH=/opt/homebrew/opt/bash/bin:$PATH just verify` passed with Bash 5.3.20. All verification tiers completed, including 76 of 76 frontend cases. Later consent chip fit and spacing changes received the focused checks in [Acceptance evidence](#acceptance-evidence).

US3.1 uses in-memory storage to check fallbacks and controls for services without icons. The [migration integration test](../../tests/integration/migrations/service_icons_migration_test.go) checks that pre-feature PostgreSQL rows keep a null icon.

Review found that the first historical-fixture workaround reduced successful upgrades from `MigrateTo(0)` to `MigrateTo(47)`. That reduction is corrected. Current provider columns require migration `049` before the legacy broker can start, so `PostgresFixture.ApplyMigrationFile` supplies the column temporarily. Successful paths remove that temporary column and run the complete migration chain with `MigrateTo(0)`. Normal rollback to `046` removes `049` before full reapply. Only the deliberately blocked case targets `047`. API remediation, duplicate reports, dirty-state checks and relationship-preservation assertions are unchanged.

## Acceptance evidence

US1.1–US1.4 cover exact Admin lifecycle, real PostgreSQL restart, preserved identity/grants/session/OAuth credentials, fresh omission/null transitions and atomic rejection without fetches.

US2.1–US2.4 cover original effects, text, styles, local reuse, embedded PNG, animation, aliases and permitted DOCTYPE exports. Chromium decoded the exact API/data-URI source with positive natural width. Accepted attribute, CSS and animated references caused no observed external loads or execution. The signed-profile remote avatar still rendered under the revised CSP. Every actual source-bearing end-user response enforced existing principal/ownership boundaries.

US3.1–US3.4 cover original initials and name-only fallbacks, native image errors against valid stored source, and usable actions. US3.3 checks fresh-source recovery without a full page reload. Accessible names and keyboard operation pass at 390px and 1280px. Uploaded images had no accessibility node or keyboard stop. Existing accessible scope-badge images remain permitted.

The square-icon regression in US3.4 failed before the clipping fix. After the fix, Chromium confirmed that all four image corners fit in required and optional consent chips at 390px and 1280px. `just check`, `just web-build`, and 56 focused Vitest tests passed. The configured consent screenshots were refreshed at both widths.

The consent chip spacing check failed before the inset change. After the change, Chromium verified a 4px inset and unclipped image corners in both chip variants at 390px and 1280px. `just check` and `just web-build` passed, and both configured consent screenshots were refreshed.

Forty meaningful-state screenshots are published under `tests/e2e/screenshots/`:

```text
service_icons_{configured,failed,recovered,unset}_{narrow,wide}_{consent_requirements,connection_cards,session_cards,detail_atlas_files,detail_cobalt_notes}.png
```

Representative configured narrow consent and failed-image narrow session captures were inspected. Screenshot capture used `E2E_CAPTURE_SCREENSHOTS=true` with one worker. Feature cases contain no pending/skipped placeholders. Filtered runs skipped unrelated scenarios only.

The screenshot workflow publishes updated captures and removes files that the suite no longer produces. Two pending-refetch captures and three `unset_pre_feature` captures remain tracked until the workflow runs on the updated PR. The suite now produces `unset_no_icon` captures.

Plain Save and named session actions use maintained `ConsentPage` and `SessionsPage` methods. The icon helper keeps observations and keyboard traversal. The consent waiter ignores success toasts already present before each Save.

On the final PR #523 patch, `just check` passed. One focused Vitest run passed all 52 tests in six files. One focused Chromium run with screenshot capture passed all 16 journeys, including all four icon states at both widths. The production frontend build and `TestServiceIconsMigrationUpDownUpPreservesExistingData` also passed during review.

## Security and quality limits

Accepted source is supported only as a URI-encoded decorative `<img>` data image. It is not certified safe for inline SVG or active-document reuse. Authentication controls receipt of bytes, not reuse by an authorized recipient. The byte limit does not bound browser CPU/GPU work, memory, filter cost or animation lifetime.

No additional validator defect arose during the US2 checks. No sanitizer dependency, CSS/reference-graph analyzer, renderer quota, third-party artwork, image endpoint or browser restart harness was added. Finite browser checks do not prove arbitrary future rendering safety or cost.

Review follow-up removed the original icon Save/Refresh/Confirm/KeyboardSave clone groups. Remaining heuristic findings normalize distinct selectors, waits and fixed-path/factory patterns. No findings were suppressed or acknowledged. See [plan.md](plan.md#implementation-compliance-review) for the retained validator/test-size findings and rationale.

| Scoped gating heuristics | Before review | After corrections |
|---|---:|---:|
| Whole change | 27 | 25 |
| Page objects | 27 | 22 |

The Accepted decision is [20261119-211813-service-icons-image-mode](../../adrs/20261119-211813-service-icons-image-mode.md). No maintained changelog was present. The temporary parser probe was removed. No commit or push was requested.
