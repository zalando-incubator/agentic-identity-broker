# Implementation resumption — 2026-11-08

**Outcome: progress validated; final implementation compliance remains open.**
The supplied approval still covers the same 15 items. No API, configuration key, migration,
ADR decision, scope grammar or authorization outcome changes in this resumption. The specification
and plan remain unchanged. T097's journey/composition audit is complete; 102/115 tasks are checked.

## Changes and decisions

- Extend the existing owning US3 S3 acceptance test with upstream `400 invalid_grant` body/status
  preservation, ordinary-scope forwarding, zero provider-usability calls and a successful same-token
  retry returning the upstream rotated refresh token. This is additional regression coverage of
  already implemented behavior, not a newly claimed historical semantic-red run.
- Expand local-refresh security-record tests to all nine refusal reasons: `binding_unresolved`,
  `not_selected`, `required_scopes_uncovered`, `missing`, `unusable_definitive`,
  `service_lookup_failed`, `grant_lookup_failed`, `permission_set_lookup_failed` and
  `usability_unverifiable`. Each asserts event, level, definitive/transient classification,
  agent/principal fields, credential absence and successful reuse of the same refresh token after
  repair. The grant failure occurs on the continuing-service recheck, after credential validation.
- Inject distinct synthetic access, refresh and subject tokens, client assertion/secret, code,
  PKCE verifier, state, JWE and redirect query/fragment sentinels into dependency causes in the
  authorization and refresh safety tests. Check each sentinel individually, including actual
  issued access/refresh credentials and the original JWE. Existing behavior passes these expanded
  regressions; no production security semantics were changed and no new historical red is claimed.
- Keep color repairs application-specific. Requested Login uses the existing primary Button;
  requested-candidate and provider-scope badges use existing `info-dark`; connected badges use
  `success-dark`; Refresh uses the existing `success-hover`/`success-dark` tokens. Neutral no-session
  text uses `text-primary`. Shared Button/Badge definitions and the palette are unchanged.
- Bring the active detail composition into line with Journey 2 State B: display the existing
  `is_encrypted` field, label the establishment timestamp and show that refresh rejection has not
  been observed. No predicted refresh expiry or new response field is introduced.
- Browser tests now measure requested-service/candidate text, service-detail text and action
  text/boundaries. Native disabled Save references a readable external reason by `aria-describedby`.
  Tab/Shift-Tab focus followed by Enter performs requested Login and Refresh through the production
  browser journey. No acceptance owner or existing permission/context/termination assertion is lost.

## Test-first evidence for new UI corrections

All browser checks below compiled and ran against the actual pre-correction frontend. Test/helper
compile fixes and an incorrect initial error-description assertion are not semantic-red evidence.

| Command | Semantic failure before the corresponding correction |
|---|---|
| `E2E_FRONTEND_MODE=built ginkgo -v --procs=1 --focus='Incremental Service Authorization UI' ./tests/e2e/frontend/` | Requested Login text was 3.77:1; Active text was 3.77:1; provider-scope badges were 3.68:1. Both owning journeys failed at their new 4.5:1 checks before these component color corrections. |
| Same focused browser command, after reaching the terminated state | `No active session` was 4.05:1, failing before its text-token correction. |
| Same focused browser command with requested-candidate measurement added | Both `Covers requested GitHub Production` badges were 3.68:1, failing before their feature-owned token correction. |
| `just web-test` | Active detail lacked `Encrypted`; the concrete existing active-session test failed before adding the encryption/establishment/rejection summary. A subsequent test also failed because the aborted test left its unused mock response queued; that secondary failure is not feature-red evidence. |

The browser measurement evaluates rendered sRGB colors and alpha-composes ancestor backgrounds,
then applies WCAG relative luminance. It requires 4.5:1 for measured text and 3:1 for measured button
background boundaries. Disabled controls are excluded from the contrast check, not from the Save
state/description assertions. The audited feature regions have solid backgrounds; this is not a
claim to audit arbitrary gradients, icons, every shared control or all visual states.

### Current measured states

| State | Measured text nodes | Minimum text contrast |
|---|---:|---:|
| Requested consent, login needed | 11 | 4.88:1 |
| Requested consent, ready | 8 | 6.81:1 |
| Service detail, active | 14 | 4.83:1 |
| Service detail, no session | 5 | 7.72:1 |

Every measured button boundary is at least 3:1. Each state passes with both light and dark browser
color-scheme preferences. **This is not evidence of a distinct dark theme:** the current shared
production stylesheet retains the same colors under both preferences. T112 remains open for actual
dark-theme validation, remaining control/confirmation-focus coverage and complete accessibility
review. DOM description checks are evidence of screen-reader semantics, not a manual screen-reader
session or a complete accessibility certification.

## T097 journey and composition audit

- Journey 1: configured public URL and local mode, service-specific error/action, widening refusal,
  protected consent/Save continuation, exact local response/JWT scopes and durable refresh narrowing
  agree with the approved contract and current owning authorization/gateway tests. Existing library
  time/claim generation remains unchanged; synthetic example claims are not literal test clock values.
- Journey 2: nullable detail and both identifiers agree with the self-contained OpenAPI contract
  checks and session-page tests. The active summary now includes encryption, establishment, access
  expiry and observed-rejection semantics. Login/Refresh/Terminate preserve UUID actions and never
  implicitly mutate an agent grant. The refreshed four maintained captures match States A/B/C/D.
- Journey 3: upstream authorization/refresh never receives a virtual scope. The owning proxy test
  now also asserts upstream error preservation and successful retry/rotation. The separate remaining
  lookup-failure HTTP acceptance expansion is not silently treated as complete by this audit.
- Journey 4: the existing guide/discovery/gateway tests cover optional route metadata, exact origins,
  current identifiers, rename/reuse pinning and catalog non-advertisement. Issue #446 remains the
  documented dot-segment limitation; no fallback or canonical-validation change was introduced.
- The editable Wireloom source and all four SVGs remain unchanged. Composition uses existing
  application-specific Alert/Badge/Button/Card primitives and semantic tokens, with no new universal
  component, brand asset, remote font, custom palette or visual-direction change.

## Current passing validation

| Command | Outcome |
|---|---|
| `just check` | Format/vet/lint pass; 0 lint issues after resolving all change-caused diagnostics. |
| `just test` | Fast Go/package race-enabled suite passes, including the expanded safety tests. |
| `just extproc-test` | Race-enabled ExtProc packages pass, including metadata/span safety. |
| `just web-test` | 34 files / 279 tests pass after the UI corrections. |
| `just test-integration` | Self-contained integration suites pass. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `just test-e2e-backend` | 977 pass, 0 failed/pending; 18 excluded by the existing performance/container filter. All 17 feature backend scenarios pass, including expanded US3 S3. |
| `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend` | 69 pass, 0 failed/pending/skipped; both complete feature journeys, contrast/keyboard checks and all four captures pass. |
| `ginkgo -v --procs=4 --skip=Agentgateway ./tests/e2e/extproc/` | 99 container-free cases pass; 17 Agentgateway cases excluded and deferred, not validated. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Prepared default-branch security scan passes without changing scanners or exceptions. |

Updated captures are retained under `tests/e2e/screenshots/`: `consent_requested_service_needs_login`,
`consent_requested_service_ready`, `service_session_active` and `service_session_needs_login`.

## Remaining obligations

| Tasks | Why unchecked / work still needed |
|---|---|
| T024, T107 | Nine local-refresh reasons and distinct credential causes now have regression coverage. Complete the end-to-end central principal-policy/span evidence and final standards audit; raw JSON logger comparisons alone do not establish the whole central pipeline. |
| T069, T077 | Upstream-error and successful retry variants now pass. The owning US3 S3 lookup-failure HTTP variant remains unimplemented. Domain/handler refusal tests pass, but the production memory bootstrap has no supported failing-repository injection seam. Do not add a test-only production API or use unsafe field mutation to claim this acceptance coverage. |
| T112 | Complete actual dark-theme evidence and the remaining full keyboard/confirmation-focus/accessibility coverage. Browser dark preference currently keeps the shared light palette; the measured ratios above do not satisfy this whole task. |
| T022, T098, T109 | Complete the original unit/Vitest/integration historical test-first traceability audit and DEC-006's remaining alternative validation conditions. Preserve the already recorded retrospective browser sensitivity, never label it historical TDD. |
| T110, T114 | Full current mapped journeys pass, but final complete acceptance/evidence and checklist handoff still depend on the open local obligations above. No reviewer-owned checklist marker was changed. |
| T040, T042, T103 | SQL, mapping and container-backed tests are retained. Real migration/repository apply/down/repeat and round-trip evidence remains deferred to pushed-commit container tests, not a sandbox blocker. |

`just test-integration-infra`, `just test-e2e-docker` and the 17 container-backed ExtProc cases are
**deferred to container tests on the pushed commit**. No container runtime was attempted. No remaining
local obligation is waiting solely for container evidence, and no deferred suite is listed as passed.
