# Implementation validation — 2026-11-08

The current implementation has **101/115 tasks complete**. All locally executed gates pass,
including all 19 feature acceptance scenarios. Remaining test expansion, accessibility measurements
and final evidence audits are not declared complete. Container-backed results are deferred, not passed.

## Approval and decision recording

The supplied approval is mjungsbluth's 2026-11-08 approval of all 15 items at
https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-6708996503.
Implementation records this in ADRs 048, 049, approved ADR 049 Revision 1 and DEC-006's evidence record.
The local `origin/main` tree contains neither ADR 048 nor ADR 049; Revision 1 is a pre-merge revision,
not an amendment of a default-branch ADR. The feature-local ADR revision becomes a reference pointer.
The specification and plan are unchanged.

- Proxy refresh validates every exact broker-prefixed token before any upstream call. Malformed
  input is classified before repository lookup. The resolved AgentID comes from the existing
  `TokenGrantResolution`; service existence and eligibility use the shared resolver. Ordinary-only
  requests add no lookups; no provider-session or grant-coverage dependency is introduced.
- `400 invalid_scope` and lookup-failure `500 server_error` do not consume upstream credentials.
  Valid requests strip virtual scopes, preserve unrelated form values and retain upstream rotation.
- End-User OpenAPI becomes `1.3.0`, with explicit continuation error/state/callback precedence.
- Dot-segment canonical IDs remain unchanged; the guide documents issue #446 and another-ID/no-ID
  workarounds. No emission, page or recovery UUID fallback is implemented.
- Feature-owned security records use canonical event attributes, bounded reasons/error kinds and
  lowercase message phrases. Wrapped resolver/evaluator/refresh causes and redirect URLs are removed
  from these records. Transient refresh failures log at Error rather than Warn. This is the pre-merge
  label reconciliation in the observability contract, not an unrelated telemetry rename.

## Refactoring validation

Commit `e22d83d7` threads request context through existing continuation/redirect helpers ahead of
feature changes. In an isolated checkout at `15308f74c5c403c1008e409d383f709072a99ebb` plus that patch,
`just check` and the unchanged existing `just test` suite pass. Authorization decisions, error
codes, redirect values and message/attribute values are unchanged; the helper receives the request
context. Credential-safe diagnostic changes follow in the feature commit.

## Test-first revision evidence

Tests compiled and failed on actual missing behavior before their matching correction:

| Command | Semantic failure before correction |
|---|---|
| `just test` | `TestTokenOutcomeServiceValidatesBeforeForwarding` failed for malformed/unknown/ineligible scopes and agent/service/permission-set failures; actual failure classification was `TokenProxyNoFailure` instead of refusal. `TestTokenOutcomeServiceMissingAgentFailsClosed` also failed. |
| `go test ./internal/adapters/http/enduser -run '^TestProxyTokenGrantRejects'` | All four refusal variants returned upstream success rather than 400/500. |
| `just test-integration` | Revision contract test found version `1.2.0`, missing explicit continuation failure narrative and missing proxy error examples. Journey 2 already validated against `ServiceSessionDetailResponse`; this was passing regression validation, not historical red. |
| `just test-e2e-backend` | US3 S3 failed at malformed `.../sessions/github/`: expected 400, received upstream 200. |
| `go test ./internal/domain/oauth2 -run 'TestTokenOutcomeServiceValidatesBeforeForwarding/malformed_before_lookup'` | Mixed malformed scope with unavailable agent lookup returned lookup-failure classification rather than `invalid_scope`. |
| `go test ./internal/domain/oauth2 -run 'TestRequestedServiceSecurityRecords\|TestContinuationRedirectBuildDiagnostic'` | All six OAuth authorization security-record variants lacked canonical `event`/bounded metadata. Synthetic dependency causes and redirect query/fragment appeared in logs. |
| `go test ./internal/domain/consent ./internal/domain/oauth2server -run 'TestRequestedServiceSubmissionSecurityRecord\|TestRefreshScopeDependencyCauseNotLogged\|TestProviderRefreshRecheckAuditAndFastPath'` | Submission records lacked `event`; refresh records lacked `event`/`error_kind`, transient records were Warn, and a synthetic provider cause appeared in logs. |

The corrected tests pass under the current `just test` and `just extproc-test` runs. ExtProc's new
invalid-metadata credential/span-safety checks already passed before its message-only phrase
reconciliation; no historical red is claimed for existing safe serialization.

`TestAuthorizeContinuationFailureContract` exercises the domain through the real authorize handler:
forged/malformed/expired, request/principal/agent mismatch, valid-JWE binding mismatch and unregistered
callback, each with and without state. Registered callbacks receive exact 302 errors; no code,
continuation JWE or PKCE parameter is forwarded. The proceed strategy is never called. Invalid
callbacks are direct 400 before continuation processing. These were already-correct behavior
regressions; no production authorization semantics were changed.

Existing ADR 046 usability/reconnect tests pass: valid access without refresh/no rejection performs
zero provider calls; recorded rejection refuses valid access; 429/503 remains transient without a
marker; successful reconnect clears rejection. The existing evaluator/local-refresh/consent/exchange
and detail regressions pass with no second persisted state or refresh-expiry prediction.

## DEC-006 retrospective browser sensitivity — not historical TDD

Original browser test revision: `8863e7a3877fd908dc45fce44311917ee2637933`.
The isolated checkout was under `/tmp/gh-aw/agent/055-browser-baseline/`; no current production
behavior was regressed. Original browser tests and page-object assertions were unchanged.

Reviewed subsequent behavior revisions:

- `75697216bc1a0b945b167f506a1e9a6d5c66b848`: service-page behavior.
- `0043190b63f6edbb98c181711506edaed423a066`: requested-consent behavior.
- `48c399bd36a2c90a9411931ea849f949dd16b837`: the only subsequent diff of the owning browser file
  through head `15308f74c5c403c1008e409d383f709072a99ebb`. It changes the post-Save navigation into
  an `Eventually` retry, preserving the coverage, selections, end date, successful exchange and
  session-action assertions. The page-object files have no subsequent diff over that range.

`just web-build` succeeded against the original frontend (existing current dependency installation
was linked into the isolated checkout). The first browser compilation failed because a historical
fixture still assigned `UserSession.RefreshTokenExpiresAt`, while the baseline had already merged
ADR 046's field rename. This was not sensitivity evidence. The sole compile-enabling patch was:

```diff
--- tests/e2e/fixtures/sessions.go
+++ tests/e2e/fixtures/sessions.go
@@
- session.RefreshTokenExpiresAt = &rejectedAt
+ session.RefreshRejectedAt = &rejectedAt
```

This repairs the name of the same rejection marker in a test fixture; it adds no Feature 055 behavior.

Command in that checkout:

```bash
E2E_FRONTEND_MODE=built E2E_CAPTURE_SCREENSHOTS=false ginkgo -v --procs=1 --focus='Incremental Service Authorization UI' ./tests/e2e/frontend/
```

Compilation and browser bootstrap succeeded. Both selected original scenarios failed semantically:

- US1 S4: `incremental_authorization_test.go:60` expected the focused `/agents/<id>` consent
  destination, received `/oauth/authorize`. The baseline silently proceeded without requested-service
  enforcement. This failure precedes the later UI assertions, not a tool/setup failure.
- US2 S2: `incremental_authorization_test.go:180` expected the visible level-2 `GitHub Production`
  service heading, but the original SPA did not render that service page. Playwright successfully
  ran and timed out on the concrete missing heading, not browser startup.

Result: **2 selected / 2 semantic failures / 0 pending**, with 67 unrelated cases excluded by focus.
This is **retrospective sensitivity evidence**, not proof that a browser-red execution happened before
implementation in the original session. DEC-006's remaining accessibility and final audit conditions
are still open; T022/T098/T109 are not closed by this result alone.

## Current passing validation

| Command | Actual outcome |
|---|---|
| `just check` | Format/vet/lint pass on the final source tree. |
| `just test` | Fast Go/package tests pass with race detection, including new proxy/continuation and security-record tests and the central typed-principal guard. |
| `just extproc-test` | Pass with race detection, including invalid optional metadata log/span safety. |
| `just web-test` | 34 files / 279 tests pass. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `just test-integration` | Self-contained suites pass, including OpenAPI 1.3.0 and the concrete Journey 2 schema check. |
| `just test-e2e-backend` | 977 pass, 0 failed, 0 pending; 18 excluded by the existing performance/container filter. All 17 feature backend scenarios pass. |
| `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend` | 69 pass, 0 failed/pending/skipped. Both complete feature browser journeys and all four capture calls pass. |
| `ginkgo -v --procs=4 --skip='Agentgateway' ./tests/e2e/extproc/` | 99 container-free cases pass, 0 failed/pending; 17 Agentgateway cases excluded and deferred, never validated here. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Prepared default-branch security scan passes. |

Reviewed fresh captures under `tests/e2e/frontend/coverage/screenshots/`:
`consent_requested_service_needs_login.png`, `consent_requested_service_ready.png`,
`service_session_needs_login.png`, `service_session_active.png`. They show requested-service markers,
editable optional choices, disabled/enabled Save reasons, active actions, and no-session Login after
termination. Existing maintained captures under `tests/e2e/screenshots/` remain unchanged. Light-theme
screenshots do not establish measured light/dark WCAG contrast or keyboard/screen-reader/focus coverage.

## Unfinished obligations

| Tasks | Remaining work and why unchecked |
|---|---|
| T024, T107 | New buffer tests cover all six record rows, all scope rejection reasons, injected authorization/refresh dependency causes, redirect diagnostic and ExtProc spans. Expand distinct credential sentinels, remaining refresh failure classifications and end-to-end central principal-policy/span assertions before declaring the complete safety evidence obligation met. |
| T069, T077 | Domain and handler tests cover all refusal classes and same-token retry. Owning US3 S3 covers malformed/unknown/ineligible refusal and subsequent rotation. Its explicit lookup-failure HTTP variant and upstream-error preservation expansion remain to be added/validated. Current passing subset is not the full T069 obligation. |
| T097 | Complete the cross-artifact journey/config/JWT/Wireloom composition audit. |
| T112 | Runtime keyboard/focus/screen-reader and measured 4.5:1 text / 3:1 UI contrast evidence in both themes remains uncollected. No accessibility waiver is inferred from approval or screenshots. |
| T022, T098, T109 | Retrospective sensitivity and current green/captures are present. Remaining original unit/Vitest/integration test-first traceability audit and DEC-006 accessibility/final-evidence conditions are not complete. |
| T110, T114 | Final complete scenario/evidence and accurate-checklist handoff depends on the open local obligations above. No checklist marker was changed. |
| T040, T042, T103 | SQL/mappings/tests already exist and were reviewed, including explicit columns, exact successor equality/subset and transaction/lock order. Real migration/repository red/green evidence is deferred to container tests on the pushed commit, not a local blocker. |

`just test-integration-infra`, `just test-e2e-docker` and the 17 container-backed ExtProc cases are
**deferred to container tests on the pushed commit**. No Docker/Podman setup is attempted and no
real PostgreSQL/container passing result is claimed. Independent local work does not wait for it.
