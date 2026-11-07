# Historical Browser Test Evidence — Proposed Scoped Exception

**Status**: Proposed; approval item DEC-006. No exception is approved by this document.
**Scope**: Missing contemporaneous compiling semantic-red execution evidence for the original
Feature 055 browser acceptance tests US1 S4 and US2 S2 only (Principles VIII and XIII).

## Evidence audit

The available local Git history and feature-owned archived checkpoints were reviewed at
`cb5d0baba5e87fbabd146983dd1a33cd0b450c19`:

- `8863e7a3877fd908dc45fce44311917ee2637933` first adds both browser journeys and page-object
  locators in `tests/e2e/frontend/incremental_authorization_test.go`. Its commit message reports
  semantic red for the 17 backend scenarios only; it records no browser semantic failures.
- The archived quickstart at that revision records `just test-e2e-frontend` exiting 127 with
  `ginkgo: command not found` after frontend build. No browser journey or capture executed.
  This is infrastructure failure, not compiling semantic red.
- Feature UI behavior follows in `75697216` (service page) and `0043190b` (requested consent).
  `quickstart.md`'s 2026-10-31 checkpoint still reports the missing playwright-go driver.
- `48c399bd` adjusts consent navigation after Save. The later 2026-11-07 checkpoint records
  `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend` green:
  69 passing specs and four captures. That does not prove execution before the earlier behavior.

No supplied context or feature archive contains a browser-red revision, successful compilation,
command and semantic failure output from before behavior. No claim is made about unavailable
external archives, and no historical failure is fabricated. T022/T098/T109 remain unchecked;
this document resolves the design of the evidence gap, not the final compliance obligation.

## Recommended decision (DEC-006; Proposed)

Approve a one-time exception to the missing historical execution evidence for these two original
browser journeys, subject to all of the alternative validation below. This does not declare that
historical TDD occurred and does not waive any present acceptance assertion, screenshot,
accessibility, isolation, security or full-gate requirement. All new revision behavior retains
normal test-first compiling semantic-red requirements. No unit, Vitest, integration or other
acceptance-test evidence gap is covered by this exception.

### Alternative validation required before compliance closure

1. Preserve and inspect the original browser test revision above and the subsequent test/behavior
   diffs; verify concrete scenario assertions and account for every later change, especially Save
   navigation. Record exact full revisions and the relevant assertions in `quickstart.md`.
2. In an isolated local checkout under `/tmp/gh-aw/agent/`, run those original browser tests against
   their pre-behavior revision with the supported harness and dependencies. Any compile-enabling
   scaffolding must contain no feature behavior and be recorded as an explicit patch. Record the
   exact revision, command, successful compilation and semantic assertion failures for both
   scenarios. Label this **retrospective sensitivity evidence**, never a historical test-first run.
   Tool/setup failures do not satisfy this condition; if the baseline cannot be reproduced, leave
   compliance open and report the actual limitation rather than weakening tests.
3. Run both complete current browser journeys with the production bootstrap, four maintained
   screenshots and no skipped/pending specs; retain passing output and complete the existing
   light/dark accessibility and full verification obligations.
4. Implementation may close the browser-evidence portions of T022/T098/T109 only after the supplied
   approval explicitly covers DEC-006 and the evidence above is recorded. Other portions of those
   tasks still require their own evidence. Cite approval URL, maintainer and date; never relabel
   the original missing execution as a pass.

## Main alternative

Recover contemporaneous archived output showing the exact pre-behavior test revision, browser
command, successful compilation and the expected semantic failures for both scenarios. If genuine
output is supplied and verified, no exception is needed; remove the proposal in a document phase.
Current green runs alone, pending tests, infrastructure failures, assertion weakening and deliberate
production-code regressions are not acceptable substitutes.
