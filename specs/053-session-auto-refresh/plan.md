# Implementation Plan: Automatic Third-Party Session Refresh

**Branch**: `053-session-auto-refresh` | **Date**: 2026-10-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/053-session-auto-refresh/spec.md`

## Summary

PR #196 added `POST /api/sessions/sweep`, but nothing schedules it. This feature adds three things:

1. **Chart-managed sweep schedule (US1)**: an opt-in CronJob and ServiceAccount in `charts/agentic-identity-broker/`.
   - The job calls the sweep hourly with a `PT2H` lookahead.
   - It targets the release's admin Service by default and derives `Host` from the admin public URL.
   - It sends credentials the operator supplies: literal headers, headers read from mounted files at run time, or a TLS client certificate.
   - The chart has no identity model. It refuses to render without an identity, and it keeps file contents out of the manifest, argv, environment, and log by letting curl read each file through a config variable (research R1).
2. **Session-health gauge (US2)**: `oauth2_sessions{service_id,expiry,refreshable}`.
   - One grouped count query runs per metric export cycle, through a new read-only ISP port.
   - The domain classifies each group into an `(expiry, refreshable)` pair and zero-fills all eight per service.
3. **Sweep run metrics (US3)**: a `session_sweep_duration_seconds{outcome,dry_run}` histogram recorded beside the existing audit event, and a `session_sweep_skipped_total` counter.

There is no migration, OpenAPI field change, or new module dependency. The chart change is self-contained. The broker change is one port with two adapters, one domain component, and three instruments.

## Technical Context

**Language/Version**: Go 1.27.1. Helm 3 chart templates (Sprig). POSIX `sh` for the job script.

**Primary Dependencies**: OpenTelemetry Go (`metric`, global providers per ADR 011), sqlx + pgx v5, Viper/Cobra. The default job image is `curlimages/curl:8.16.0`, a public image already used in the runbook. **No new Go module and no CI change.** The chart render checks skip when `helm` is not installed, like the existing ones.

**Storage**: PostgreSQL and in-memory. Reads `user_sessions` metadata columns only. No schema change (DB-001).

**Testing**: stdlib `testing` + testify (unit). testcontainers PostgreSQL (`-tags=integration`) for adapter parity. Ginkgo/Gomega E2E in `tests/e2e/` for the metrics. Chart render checks through the `helm` binary in `tests/integration/helm_chart_test.go`; the rendered script's run-time behavior is verified manually (research R14).

**Target Platform**: Linux containers on Kubernetes, deployed by the broker chart. The CronJob uses `batch/v1`. `timeZone` requires Kubernetes 1.27 or later and is omitted when unset.

**Project Type**: Single Go service plus Helm chart in the monorepo. No frontend change.

**Performance Goals**:
- The health query runs once per export cycle (default 30s) per replica. It is one sequential scan with `GROUP BY`, bounded by `query_timeout` (default 5s) alone; `storage.timeouts.read` does not apply (research R10).
- The sweep job uses 50m CPU and 64Mi memory, and holds one HTTP request open for up to `timeoutSeconds`.

**Constraints**:
- Fail closed: rendering fails without an identity (SR-002).
- No credential in the manifest, argv, environment, or log (SR-003, SR-005).
- Metrics carry no principal or session ID (FR-022).
- The job pod must not join the broker Service (FR-016, research R6).
- The job image must provide curl 8.3 or later for config variables (quickstart step 4).

**Scale/Scope**:
- Chart: 2 templates, 1 helper, a values block, the schema, and the README.
- Broker: 1 port method in 2 adapters, 1 domain file, edits to `sweep.go` and the sweep handler, config keys, and builder wiring.
- Tests: 1 E2E file with the 12 US2 and US3 scenarios, and chart render checks for US1.
- Docs: runbook, configuration reference, `ARCHITECTURE.md`, and the OpenAPI description text.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](data-model.md). No new entity. The new value objects are `AccessExpiryClass`, `SessionExpiryGroup`, `SessionHealthKey`, and `SessionHealthSnapshot`. The process-scoped component is `SessionHealthMetrics`, with a lifecycle diagram. The existing `SessionSweepRun` audit event gains a duration measurement.
- [x] **Domain Concepts**: Session Health State, Session Health Snapshot, and Session Sweep Schedule go into the `ARCHITECTURE.md` glossary under Third-Party OAuth2 Session Management (`ARCHITECTURE.md:1502`).
- [x] **Entity IDs**: N/A. There is no new UUID-keyed entity. Groups use the existing `id.ServiceID`.
- [x] **Configuration Design**: [contracts/configuration.md](contracts/configuration.md) lists both keys with defaults, env vars, flags, and validation. [contracts/chart-values.md](contracts/chart-values.md) lists every `sessionSweep.*` value and its schema.
- [x] **Config Examples**: `examples/config/telemetry.yaml` and `examples/config/config.production.yaml` gain the `token_refresh.health_metrics` block.
- [x] **Helm Deployment Contract**: The broker is chart-managed. `values.yaml`, `values.schema.json` (strict), `templates/configmap.yaml`, the two new templates, and `README.md` are updated in this feature.
- [x] **API Design First**: No endpoint or field changes (API-001). The spec permits only descriptive OpenAPI edits (API-002): the `POST /api/sessions/sweep` description and the example at `api/admin/openapi.yaml:1701, 1752` move to the chart-managed hourly `PT2H` schedule.
- [x] **API Documentation**: N/A for `docs/api/` (admin API). The operator runbook is updated (FR-025).
- [x] **API Changes**: None.
- [x] **Database Design**: No schema change and no migration (DB-001). The new query reads existing columns (DB-002, DB-003).
- [x] **E2E Acceptance Tests**: The 12 US2 and US3 scenarios have E2E tests in `tests/e2e/session_metrics_test.go`, written before implementation. US1 is a recorded deviation (Complexity Tracking): none of its 11 scenarios has an `It()` in `tests/e2e/`. Render checks in `tests/integration/helm_chart_test.go` cover US1-S1, S2, S7, S10, and S11 and the rendered parts of S3 to S5, and the run-time behavior of S3 to S6, S8, and S9 is verified manually.
- [x] **E2E Test Mapping**: 1:1 `It()` per US2 and US3 scenario, and a render subtest per covered US1 scenario, tabulated below.
- [x] **E2E Red Phase**: Each test asserts real output that does not exist yet:
  - US1: rendered resources and render-time failures.
  - US2: `oauth2_sessions` data points.
  - US3: histogram and skipped-counter data points.

  Before implementation, `helm template` renders no CronJob and the instruments do not exist, so each test fails on its own assertion.
- [x] **Frontend Playwright E2E**: N/A. No UI change.
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [x] **Security-First**: The sweep is off by default. When enabled, rendering fails without an identity. File headers and TLS files are read at run time and kept off argv and the environment. Header file values with quote, backslash, or newline are rejected. The container is non-root with a read-only root filesystem, all capabilities dropped, and `RuntimeDefault` seccomp. Both the ServiceAccount and the pod set `automountServiceAccountToken: false`. The job pod cannot become a broker Service endpoint. The broker still refuses sweeps without an operator principal. Metrics have no principal attribute.
- [x] **Architecture Docs**: `ARCHITECTURE.md` updates:
  - admin sweep description (`:538-542`)
  - the "does not start a session-sweep timer" statement (`:573`), which stays true and gains a pointer to the chart schedule
  - refresh telemetry (`:750-763`), which gains the gauge, histogram, and skipped counter
  - the glossary
- [x] **ADRs**: None added or superseded. The chart job is an external operator schedule as ADR 038 describes. The chart adds no authentication model, and the documented path keeps the authenticating proxy (spec, Architecture Decisions). ADR 011 (global OpenTelemetry providers) and ADR 004 (dual server) apply unchanged.
- [x] **Library-First Security**: No cryptography. TLS is curl's. Credentials are opaque bytes the chart never parses.
- [x] **Zalando Guidelines**: No API change.
- [x] **End-User Docs**: Operator docs:
  - `docs/operations/session-sweep.md` (FR-025)
  - `docs/configuration.md`
  - the chart README (FR-026)
- [x] **Migration Testing**: N/A. No migration.
- [x] **Hexagonal Architecture**:
  - `SessionHealthMetrics` depends on the new `ports.UserSessionHealthRepository`.
  - The adapters implement the predicates, and the domain owns state precedence (research R9).
  - The histogram is recorded in the handler's existing audit block, because only the handler sees every outcome (research R12). It contains no business decision.
  - Wiring is in `builder.go`. No adapter imports another.
- [x] **Persistence Patterns**: A focused one-method ISP port beside `UserSessionExpiryRepository`. sqlx `SelectContext` under the caller's `query_timeout` context, with `wrapError`. Memory and PostgreSQL implementations get the same parity scenarios H1 to H11.

**Gate status**: PASS. No violation needs justification.

**Post-Design Re-check**: PASS.
- The design adds no schema, API field, or ADR.
- The only port addition is the one DB-002 allows.
- The script contract (`contracts/sweep-job.md`) satisfies SR-003 and SR-005 without a Secret-backed environment variable.
- Research R8 tightened FR-012: any non-`2xx` status, including `3xx`, fails the job. Spec Journey 3 now shows the script's own failure line.

## Spec Alignment

- FR-006 (`Host` only for the default target unless set) is in research R3.
- FR-011 and SR-002 (fail closed) are in research R4.
- FR-016 (labels) is in research R6.
- FR-020 (grouped query per export cycle) is in research R9 to R11.
- FR-023 (histogram scope) is in research R12.
- The spec's value objects and glossary note include Session Health Snapshot, which the domain builds in every export cycle.

## Project Structure

### Documentation (this feature)

```text
specs/053-session-auto-refresh/
├── spec.md
├── plan.md                         # This file
├── research.md                     # Phase 0 (R1–R16)
├── data-model.md                   # Phase 1
├── quickstart.md                   # Phase 1
├── contracts/
│   ├── chart-values.md             # sessionSweep.* and broker.tokenRefresh.healthMetrics.*
│   ├── sweep-job.md                # rendered resources and script behavior
│   ├── ports.md                    # port, domain component, handler and sweep changes
│   ├── configuration.md            # token_refresh.health_metrics.*
│   └── observability.md            # instruments, log record, queries
├── checklists/requirements.md
└── tasks.md                        # /speckit-tasks (not created here)
```

### Source Code (repository root)

```text
charts/agentic-identity-broker/
├── templates/_helpers.tpl                         # session-sweep labels, target, script, fail checks
├── templates/cronjob-session-sweep.yaml           # new
├── templates/serviceaccount-session-sweep.yaml    # new
├── templates/configmap.yaml                       # token_refresh.health_metrics
├── values.yaml / values.schema.json / README.md

internal/
├── ports/storage.go                               # UserSessionHealthRepository
├── ports/config.go                                # TokenRefreshHealthMetricsConfig
├── domain/storage/user_session.go                 # AccessExpiryClass, SessionExpiryGroup
├── domain/oauth2session/health_metrics.go         # new: state, classification, gauge
├── domain/oauth2session/health_metrics_test.go    # new
├── domain/oauth2session/sweep.go                  # skipped counter
├── adapters/storage/{memory,postgres}/user_session.go (+ _test.go)
├── adapters/storage/factory.go                    # SessionHealth()
├── adapters/http/handlers/admin/session_sweep_handler.go (+ _test.go)
├── config/{loader.go,validator.go} (+ _test.go)
└── app/builder.go

cmd/agentic-identity-broker/root.go                # flags
tests/integration/helm_chart_test.go              # US1 render checks (skips without helm)
tests/e2e/session_metrics_test.go                  # new, US2 + US3
docs/operations/session-sweep.md, docs/configuration.md, ARCHITECTURE.md, api/admin/openapi.yaml (text only)
examples/config/{telemetry.yaml,config.production.yaml}
```

**Structure Decision**: Existing monorepo layout. Chart, broker, and tests live in their established directories, and nothing new is added at the top level.

## Implementation Phase Overview

| Phase | Purpose | Included |
|---|---|---|
| Phase 0 | Pre-implementation refactoring | Skip. No structural change is needed first. |
| Phase 1 | Setup | Skip. The chart render checks reuse `tests/integration/helm_chart_test.go`, which skips without `helm`; no CI change. |
| Phase 2 | Design preconditions | Glossary, config contract, chart values and schema skeleton, the metrics E2E file and the chart render checks in red. |
| Phase 2.7 | Entity boilerplate | Skip. No new entity. |
| Phase 2.5 | Foundational | Port, both adapters with parity tests, factory accessor, config keys. |
| Phase 3 | US1 (P1) chart schedule | Templates, helper, script, fail checks, README. |
| Phase 4 | US2 (P1) health gauge | Domain component, builder wiring, shutdown. |
| Phase 5 | US3 (P2) run metrics | Histogram in the handler, skipped counter. |
| Phase 6 | Docs | Runbook, configuration reference, `ARCHITECTURE.md`, OpenAPI text, examples. |
| Phase N | Constitution compliance | `just check`, `just verify`, diff review, internal-reference scan. |

- [x] Phase 0 (refactoring): skip. The handler, sweep service, and adapters accept the additions without restructuring.
- [x] Phase 2.7 (entity boilerplate): skip. No new entity.

US1 and US3 need only Phase 2. US2 needs Phase 2.5. All three can proceed in parallel once their prerequisites are met.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md) and `tests/e2e/AGENTS.md`. Each `It` carries a `// US<n>-S<m> from specs/053-session-auto-refresh/spec.md` comment.

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1-S1 | `tests/integration/helm_chart_test.go` (`TestHelmTemplate_SessionSweep`) | `renders no sweep resources when disabled` |
| US1-S2 | same | `renders an hourly Forbid CronJob targeting the admin Service with the public-URL Host and PT2H` |
| US1-S3, S4 | render check (same subtest as US1-S2 asserts the `url`, `Host`, and literal header lines); run-time behavior verified manually (quickstart §5) | — |
| US1-S5 | same file: `reads file headers through curl config variables and never renders their value`; argv/env hygiene is curl's `variable@file` behavior, verified manually | — |
| US1-S6, S8, S9 | manual verification against the admin server (quickstart §5); curl's TLS and status handling are not the chart's to test | — |
| US1-S7 | same file | `fails to render with an invalid sweep configuration/no identity` |
| US1-S10 | same file | `keeps the job pod out of the broker Service and PDB selectors` |
| US1-S11 | same file | `fails to render with an invalid sweep configuration/{timeout not below the deadline, unpaired TLS certificate, non-ISO lookahead, zero lookahead}` |
| US2-S1 | `tests/e2e/session_metrics_test.go` | `It("reports one value per service and series that sums to the stored sessions")` |
| US2-S2 | same | `It("reports zero for series without sessions")` |
| US2-S3 | same | `It("counts a session expiring after the lookahead as valid")` |
| US2-S4 | same | `It("counts a due session as expiring and an already expired one as expired, both refreshable")` |
| US2-S5 | same | `It("counts expired sessions without a usable refresh token as expired and unrefreshable")` |
| US2-S6 | same | `It("counts a session without recorded expiry under unknown")` |
| US2-S7 | same | `It("reports no points and logs a warning when the query fails, then recovers")` |
| US2-S8 | same | `It("issues no health query when health metrics are disabled")` |
| US2-S9 | same | `It("exports only service_id, expiry, and refreshable attributes")` |
| US3-S1 | same | `It("records a completed run with its dry_run value in the duration histogram")` |
| US3-S2 | same | `It("records rejected, aborted, and canceled runs under their outcome and no completed run")` |
| US3-S3 | same | `It("increments the skipped counter for already-refreshed or deleted candidates")` |

**Red Phase Requirements**: As in the template. No `Skip`, `XIt`, or `PIt` in the E2E suite. The chart render checks follow the existing `tests/integration/helm_chart_test.go` convention and skip only when `helm` is not installed. US1 checks fail on render assertions until the templates exist. US2 and US3 tests fail on missing data points until the instruments exist.

**Test Data Strategy**:
- Existing fixtures: `fixtures.AdminPrincipal()`, the third-party service and session fixtures used by `tests/e2e/proactive_token_refresh_test.go`.
- New fixtures: `tests/e2e/fixtures/session_health.go` session builders with controlled `AccessTokenExpiresAt`, refresh-token presence, and `RefreshTokenExpiresAt` for each `(expiry, refreshable)` series.

**Bootstrap Strategy**:
- Admin server via `bootstrap.NewServerFactory(config, logger).BuildApp(store)` and `bootstrap.NewAdminTestServer` (production builder).
- US2 and US3 install a `sdkmetric.ManualReader` as the global MeterProvider in `BeforeEach` with `DeferCleanup`, as in `tests/e2e/approval_api_test.go:136-143`, before `BuildApp`.
- US2-S7 uses a storage decorator in `tests/e2e/` that fails the health query on demand. US2-S8 uses the same decorator to count calls. Both are test code only.

**Chart render checks** (`tests/integration/helm_chart_test.go`): `runHelmTemplate` wraps the existing `renderHelmTemplate` so a subtest can assert on helm's stderr for the render-time `fail` paths. No new helper package, module, or CI dependency.

### Unit & Integration Tests

- **Unit**:
  - `health_metrics_test.go`: classification, zero-fill, attributes, error, timeout, `Close`
  - `sweep_test.go`: skipped counter
  - `session_sweep_handler_test.go`: histogram per outcome, nothing recorded without a principal
  - `internal/config`: defaults, env, flags, `query_timeout` validation
- **Integration**: PostgreSQL parity H1 to H9 (`-tags=integration`), plus the memory equivalents.
- **Coverage goals**: every new branch in the domain component and adapters. 100% of acceptance scenarios in E2E.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| No US1 scenario has an `It()` in `tests/e2e/` (constitution: every acceptance scenario has one). S1, S2, S5, S7, S10, and S11 are render subtests in `tests/integration/helm_chart_test.go`; the run-time behavior of S3 to S6, S8, and S9 is verified manually. | US1 is a chart feature. What the chart controls is its rendered output, which the render checks assert next to the existing chart tests. The run-time behavior (header files, TLS, exit status) belongs to curl and the admin server. This follows the chart precedent: spec 014 covered its chart scenarios outside `tests/e2e/` (`specs/014-helm-charts/plan.md`, "No Go E2E Tests Required"). | A backend E2E harness (chart render helper, recording proxy, script runner, test CA) was built and removed as disproportionate: it added Helm and a recent curl to every E2E runner for behavior the chart does not own (research R14). |

## Implementation Records

- **API (T011)**: No endpoint, field, status, or schema change (API-001). Only the description text at `api/admin/openapi.yaml:1701` and the example at `:1752` change (API-002, T046). No stakeholder API sign-off is required.
- **Database (T012)**: No schema change and no migration (DB-001). The health query reads `service_id`, `access_token_expires_at`, `octet_length(encrypted_refresh_token)`, and `refresh_token_expires_at` only (DB-002, DB-003).
- **Frontend (T013)**: N/A, no UI change.

### Red phase results (T017, 2026-10-19)

Both files compile and contain no `Skip`, `XIt`, `PIt`, or placeholder assertions. Each scenario failed on its own assertion before implementation:

| Scenario | Failure before implementation |
|---|---|
| US1-S1 | Enabled render has no `CronJob` or `ServiceAccount` named `broker-agentic-identity-broker-session-sweep` |
| US1-S2, S5, S10 | rendered chart has no CronJob `broker-agentic-identity-broker-session-sweep` |
| US1-S7 | Render without identity returned no error |
| US1-S11 | Timeout-versus-deadline render returned no error |
| US2-S1 to S6, S9 | No `oauth2_sessions` metric collected |
| US2-S7 | No `session health query failed` WARN while `user_sessions` was locked |
| US2-S8 | Positive control with health metrics enabled logged no WARN under the lock |
| US3-S1 | No `session_sweep_duration_seconds` data points |
| US3-S2 | Timed out waiting for a `canceled` histogram data point |
| US3-S3 | `session_sweep_skipped_total` was `0`, want `2` (the response already reported `skipped: 2`) |

The script-run scenarios (US1-S3, S4, S6, S8, S9) were first covered by a backend E2E harness that was later removed as disproportionate (research R14); they are verified manually per quickstart §5.

### Verification results (T060, 2026-10-19)

| Command | Result |
|---|---|
| `just check` (with `golangci-lint` v2.14.0) | Passed, 0 lint issues |
| `just test` | Passed |
| `just test-integration-all` | Passed (8 packages, including PostgreSQL parity H1 to H9) |
| `just helm-lint`, plus `helm lint` with the sweep enabled | Passed |
| Backend E2E, `!performance` (710 specs) | The 710 include the 23 feature scenarios, which all passed. The first run had 700 passed and 10 failed: SPA and CIMD redirect specs returned 404 because `web/dist` was not built locally. After `just web-build`, a re-run of the SPA and CIMD redirect suites (15 specs, including the 10 failures) passed, so all 710 passed. |
| Rendered script in `curlimages/curl:8.16.0` | Passed (research R7 verification) |
| `just security` | Not run successfully: the scan script resolved a Go 1.26.8 toolchain with `GOTOOLCHAIN=local` while `go.mod` requires 1.27.1, so gosec loaded no files. This is environmental. |
| ExtProc and frontend E2E, `web-test`, `cdk-test`, mock suites | Not run. This feature does not change them. |

TDD notes (T059): the config, memory adapter, health metrics, skipped counter, and histogram tests compiled and failed on their assertions before implementation. The PostgreSQL parity tests share the memory tests' expectations and ran only after implementation. `TestSessionSweepDryRunSkippedCandidateIsNotCounted` guards an existing behavior and passed before and after. No E2E test changed after T017.

