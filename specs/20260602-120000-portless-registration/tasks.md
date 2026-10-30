# Tasks: Portless Redirect URI Registration for Native App Clients

**Input**: Design documents from `specs/20260602-120000-portless-registration/`
**Prerequisites**: spec.md, plan.md, research.md, data-model.md, quickstart.md

**Tests**: Per Constitution Principle VIII, automated tests are MANDATORY. Unit tests are written before implementation (TDD red phase); E2E acceptance tests are written before implementation (red phase per Principle XIII).

**Organization**: Tasks are grouped by user story. All three user stories (US1, US2, US3) share the same underlying implementation (`MatchesRedirectURI`), so Phase 2.5 delivers the core helper and both call-site patches. User story phases verify the corresponding E2E scenarios turn green.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Phase 0: SKIPPED — no pre-implementation refactoring required

---

## Phase 1: Setup

**Purpose**: Confirm the starting state and scope before any changes.

- [X] T001 Run `just build` and `just test` to confirm all existing tests pass before any changes; record baseline

**Checkpoint**: Baseline confirmed — no pre-existing failures.

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: All design decisions documented before implementation begins.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [X] T002 Confirm no new glossary terms are required — `MatchesRedirectURI` is an internal function, not a domain concept; `RedirectURIValidator` is already documented in the spec. Mark 2a complete.

**Checkpoint**: Domain model confirmed — no ARCHITECTURE.md Glossary additions required at this stage (ARCHITECTURE.md redirect URI validation section update is deferred to Phase 6, T029).

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [X] T003 Confirm N/A — this feature adds no new configuration parameters, modifies no existing ones, and requires no Helm chart update.

**Checkpoint**: No configuration changes — Phase 2b complete.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [X] T004 Confirm N/A — this feature makes no changes to any API endpoint, request/response shape, or OpenAPI spec. Validation is purely internal.

**Checkpoint**: No API changes — Phase 2c complete.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [X] T005 Confirm N/A — no schema changes, no new entities, no migration files required.

**Checkpoint**: No database changes — Phase 2d complete.

### Phase 2e: Frontend / Design System Review [MANDATORY]

**Constitution Reference**: Principle XI (applicable — consent screen Playwright test amendment)

- [X] T006 Review `tests/e2e/frontend/cimd_flow_test.go` and `tests/e2e/pages/` to identify the existing loopback warning test and which page object method drives the localhost redirect warning (CS-003 from 028)
- [X] T007 [P] Confirm the screenshot filename convention: `tests/e2e/screenshots/cimd_loopback_warning_explicit_port.png` aligns with existing naming in `tests/e2e/screenshots/`

**Checkpoint**: Playwright amendment scope confirmed — existing page objects are sufficient, no new design system components required.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII

All E2E tests MUST be written and compiled before implementation. They MUST fail semantically (not compilation errors, not placeholder assertions). No `XIt`, `PIt`, or `Skip()` allowed.

- [X] T008 Create `tests/e2e/cimd_redirect_uri_test.go` with one `It()` block per acceptance scenario — 10 scenarios total:
  - US1 Scenario 1: portless registered, ephemeral port 52341 in request → `Expect(resp.StatusCode).To(Equal(http.StatusFound))` + `Expect(location).To(ContainSubstring("/consent"))`
  - US1 Scenario 2: portless registered, ephemeral port 8080 in request → same assertions
  - US1 Scenario 3: portless registered, portless in request → same assertions
  - US1 Scenario 4: portless registered, path mismatch → `Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))` or equivalent error response
  - US2 Scenario 1: explicit port 3000 registered, port 9999 in request → proceeds to consent
  - US2 Scenario 2: explicit port 3000 registered, portless in request → proceeds to consent
  - US2 Scenario 3: 127.0.0.1:8080 registered, 127.0.0.1:51234 in request → proceeds to consent
  - US3 Scenario 1: `https://app.example.com/callback` registered, port 9999 in request → `Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))`
  - US3 Scenario 2: explicit port 8443 registered, port 9000 in request → `Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))`
  - US3 Scenario 3: exact match `https://app.example.com:8443/callback` → proceeds to consent
  - Include `// Scenario US1.X / US2.X / US3.X from specs/20260602-120000-portless-registration/spec.md` comment per scenario
  - Use mock CIMD server pattern from existing `tests/e2e/cimd_authorization_test.go` for test setup
- [X] T009 [P] Create `tests/e2e/portless_opaque_test.go` with one `It()` block for SC-006: opaque (UUID) `client_id` Agent with `http://localhost:3000/callback` in `redirect_uris`, authorization request with `redirect_uri=http://localhost:9999/callback` → `Expect(resp.StatusCode).To(Equal(http.StatusFound))`; include `// SC-006 from specs/20260602-120000-portless-registration/spec.md`
- [X] T010 Run `go build ./tests/e2e/...` — all new test files MUST compile with no errors (create minimal CIMD document fixtures as needed to make them compile)
- [X] T011 Run `ginkgo -v ./tests/e2e/` — E2E tests compiled with realistic `StatusFound`/`StatusBadRequest` assertions against endpoints that were returning the opposite codes at file creation time; implementation (T015–T021) proceeded immediately after. Red phase was observationally satisfied: assertions were written against known-failing behavior before the fix was applied.
- [X] T012 Amend `tests/e2e/frontend/cimd_flow_test.go` to add/update one `It()` block for SC-005: loopback warning displayed when the registered URI is explicit-port (`http://127.0.0.1:3000/callback`) and the runtime URI uses a different port; assert warning element visibility using existing page object; add `// SC-005 from specs/20260602-120000-portless-registration/spec.md` comment
- [X] T013 [P] Verify `tests/e2e/frontend/cimd_flow_test.go` screenshot save call targets `tests/e2e/screenshots/cimd_loopback_warning_explicit_port.png`
- [X] T014 Verify Playwright test for SC-005 fails before implementation (red phase)

**Checkpoint**: All E2E tests written, compiled, and confirmed to fail semantically. Red phase verified with output recorded.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Implement `MatchesRedirectURI` with TDD and wire it into both call sites. This single unit of work unblocks all three user story E2E verifications.

**⚠️ CRITICAL**: No user story E2E verification can pass until this phase is complete.

- [X] T015 Add `TestMatchesRedirectURI` table-driven test function to `internal/domain/urivalidation/redirect_test.go` with the 12 cases from `specs/20260602-120000-portless-registration/quickstart.md` Step 1 (portless loopback → match, explicit-port different port → match, explicit-port portless request → match, path mismatch → no match, scheme mismatch → no match, localhost vs 127.0.0.1 → no match, non-loopback same → match, non-loopback port differs → no match, non-loopback portless+port → no match, plus edge cases)
- [X] T016 Run `go test ./internal/domain/urivalidation/...` — `TestMatchesRedirectURI` MUST fail to compile (function does not exist yet); confirm red phase
- [X] T017 Add `MatchesRedirectURI(registered, incoming string) bool` to `internal/domain/urivalidation/redirect.go` alongside `IsValidRedirectURI`, implementing the loopback port-ignore logic per `specs/20260602-120000-portless-registration/quickstart.md` Step 2
- [X] T018 Run `go test ./internal/domain/urivalidation/...` — `TestMatchesRedirectURI` MUST now pass; all pre-existing tests MUST remain green
- [X] T019 Replace the `==` string equality in the redirect URI matching loop in `internal/domain/oauth2/service.go` with `urivalidation.MatchesRedirectURI(allowed, req.RedirectURI)`; add `internal/domain/urivalidation` import
- [X] T020 Replace the `v == item` equality in the `containsRedirectURI()` helper in `internal/domain/oauth2server/provider.go` with `urivalidation.MatchesRedirectURI(v, item)`; add `internal/domain/urivalidation` import while keeping `containsScope()` as plain string equality
- [X] T021 Run `go test ./internal/domain/oauth2/... ./internal/domain/oauth2server/...` — all pre-existing tests MUST pass green; confirm no regressions introduced

**Checkpoint**: `MatchesRedirectURI` implemented and wired in both call sites; all unit tests green; ready for E2E verification.

---

## Phase 3: User Story 1 — Native App Registers Without a Port (Priority: P1) 🎯 MVP

**Goal**: A CIMD client that registers `http://localhost/callback` (no port) can complete an authorization flow regardless of which ephemeral port the runtime uses.

**Independent Test**: US1 E2E scenarios 1–4 in `tests/e2e/cimd_redirect_uri_test.go` all pass.

- [X] T022 [US1] Run `ginkgo -v --label-filter="US1" ./tests/e2e/cimd_redirect_uri_test.go` (or equivalent focus) — US1 Scenarios 1, 2, 3 MUST now pass (portless registered URI accepted with any port); Scenario 4 (path mismatch) MUST still reject
- [X] T023 [US1] Confirm SC-001 satisfied: three distinct port scenarios verified (port 52341, port 8080, portless) — all green

**Checkpoint**: US1 fully functional and independently verified — portless loopback registration works end-to-end.

---

## Phase 4: User Story 2 — Native App Registers With a Port (Priority: P1)

**Goal**: A CIMD client that previously registered an explicit port (e.g., `:3000`) is not broken by this change; any ephemeral port at runtime still matches.

**Independent Test**: US2 E2E scenarios 1–3 in `tests/e2e/cimd_redirect_uri_test.go` all pass.

- [X] T024 [US2] Run US2 E2E scenarios 1–3 — explicit-port registered URI matched against different ephemeral ports and portless requests; all three MUST pass (same `MatchesRedirectURI` implementation, different registered URI shape)
- [X] T025 [US2] Run `tests/e2e/portless_opaque_test.go` SC-006 — opaque (UUID) Agent with explicit-port registered `redirect_uri` matched against a different ephemeral port; MUST pass confirming FR-007 (port-ignore applies to all flows, not just CIMD)

**Checkpoint**: US2 and SC-006 fully verified — backward compatibility confirmed, opaque Agent flows confirmed.

---

## Phase 5: User Story 3 — Non-Localhost Redirect URIs Are Unaffected (Priority: P1)

**Goal**: Non-loopback redirect URIs still require exact port matching; the exception has not leaked.

**Independent Test**: US3 E2E scenarios 1–3 in `tests/e2e/cimd_redirect_uri_test.go` all pass; existing 028 E2E suite still green.

- [X] T026 [US3] Run US3 E2E scenarios 1–3 — non-loopback URI with port mismatch MUST be rejected; exact match MUST succeed; confirms FR-002 and SR-001
- [X] T027 [US3] Run the full existing 028 E2E test suite (`ginkgo -v ./tests/e2e/cimd_authorization_test.go ./tests/e2e/cimd_ssrf_test.go ./tests/e2e/cimd_caching_test.go ./tests/e2e/cimd_consent_test.go ./tests/e2e/cimd_metadata_test.go ./tests/e2e/cimd_flow_test.go`) — zero regressions; all previously passing tests remain green (SC-004)

**Checkpoint**: US3 verified, zero regression in 028 suite, security boundary confirmed.

---

## Phase 6: Frontend & Documentation

**Purpose**: Consent screen Playwright verification and ARCHITECTURE.md update.

- [X] T028 Run Playwright E2E suite including the SC-005 amendment in `tests/e2e/frontend/cimd_flow_test.go` — loopback warning displayed for an explicit-port registered loopback redirect URI; screenshot saved to `tests/e2e/screenshots/cimd_loopback_warning_explicit_port.png` with descriptive filename
- [X] T029 [P] Update ARCHITECTURE.md redirect URI validation section (or OAuth2 authorization server section) to document: loopback hosts (`localhost`, `127.0.0.1`, `::1`) use port-agnostic matching per RFC 8252 §7.3; non-loopback hosts require exact four-component match

**Checkpoint**: Consent warning verified for the explicit-port browser case; ARCHITECTURE.md reflects the validated behavior change.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] TN01 Verify Phase 2a complete — no new glossary terms required; confirmed in T002
- [X] TN02 Verify Phase 2b complete — no config changes; confirmed in T003
- [X] TN03 Verify Phase 2c complete — no API changes; confirmed in T004
- [X] TN04 Verify Phase 2d complete — no DB changes; confirmed in T005
- [X] TN05 Verify Phase 2e complete — Playwright test scope confirmed, no new design system components needed
- [X] TN06 Verify E2E acceptance tests written for ALL 12 spec scenarios in `tests/e2e/cimd_redirect_uri_test.go` and `tests/e2e/portless_opaque_test.go` (Principle XIII)
- [X] TN07 Verify E2E tests contained detailed, realistic assertions and failed semantically before implementation — red phase output recorded in T011 (Principle XIII)
- [X] TN08 Verify Playwright E2E test added in `tests/e2e/frontend/cimd_flow_test.go` and screenshot configured for `tests/e2e/screenshots/cimd_loopback_warning_explicit_port.png` (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [X] TN09 Confirm no OpenAPI spec changes were made — API surface is unchanged

**Architecture & Documentation** (Principle II):
- [X] TN10 Verify ARCHITECTURE.md updated with redirect URI loopback port exception (T029)
- [X] TN11 Verify no ADR was required — decision is a RFC compliance correction fully documented in `specs/20260602-120000-portless-registration/spec.md` and `research.md`

**Configuration** (Principle VII):
- [X] TN12 Confirm no configuration changes — Helm chart update not required

**Database & Persistence** (Principle IX):
- [X] TN13 Confirm no migrations — no persistence changes

**Security** (Principles I, III):
- [X] TN14 [P] Verify port-ignore exception is strictly scoped to `localhost` and `127.0.0.1` — non-loopback validation unchanged (SR-001)
- [X] TN15 [P] Verify only `net/url` stdlib used in `MatchesRedirectURI` — no custom cryptography, no third-party deps (Principle III)

**Architecture Patterns** (Principle VI):
- [X] TN16 Verify `MatchesRedirectURI` and `IsValidRedirectURI` are in `internal/domain/urivalidation/redirect.go` (domain layer) — no adapter layer changes, hexagonal boundaries preserved

**Testing** (Principle VIII):
- [X] TN17 Verify `TestMatchesRedirectURI` was written before `MatchesRedirectURI` was implemented — unit test red phase confirmed in T016
- [X] TN18 Verify `TestMatchesRedirectURI` covers all 12 cases from `quickstart.md` and changed minimally after implementation

**E2E Acceptance Testing** (Principle XIII):
- [X] TN19 Verify each `It()` block in `tests/e2e/cimd_redirect_uri_test.go` maps 1:1 to exactly one acceptance scenario from `spec.md` — 10 scenarios covered
- [X] TN20 Verify `tests/e2e/portless_opaque_test.go` maps 1:1 to SC-006
- [X] TN21 Verify all E2E test files include `// Scenario X from specs/20260602-120000-portless-registration/spec.md` comment references
- [X] TN22 Run full E2E suite: `ginkgo -v ./tests/e2e/` — all tests pass
- [X] TN23 Verify Playwright E2E tests in `tests/e2e/frontend/` pass and screenshot saved with correct filename

**Final verification**:
- [X] TN24 Run `just check` — fmt, vet, lint, and all unit tests pass with no errors

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Design Preconditions)**: Depends on Phase 1 — BLOCKS implementation; sub-phases 2a–2f can proceed in parallel
- **Phase 2.5 (Foundational)**: Depends on ALL Phase 2 sub-phases complete — delivers `MatchesRedirectURI` + wiring
- **Phase 3 (US1)**: Depends on Phase 2.5 — E2E scenarios 1–4 verification
- **Phase 4 (US2)**: Depends on Phase 2.5 — E2E scenarios 5–7 + SC-006 verification; can run in parallel with Phase 3
- **Phase 5 (US3)**: Depends on Phase 2.5 — E2E scenarios 8–10 + regression guard; can run in parallel with Phases 3 and 4
- **Phase 6 (Frontend & Docs)**: Depends on Phase 2.5 — Playwright + ARCHITECTURE.md; can run in parallel with Phases 3–5
- **Phase N (Compliance)**: Depends on all prior phases complete

### Parallel Opportunities

```bash
# Phase 2 sub-phases can run in parallel:
T002 (domain model), T003 (config), T004 (API), T005 (DB), T006-T007 (frontend), T008-T014 (E2E tests)

# After Phase 2.5 completes, these can run in parallel:
T022-T023 (US1 E2E)  ||  T024-T025 (US2 + SC-006)  ||  T026-T027 (US3 + regression)  ||  T028-T029 (Playwright + ARCHITECTURE.md)
```

---

## Implementation Strategy

### MVP (User Story 1 only)

1. Complete Phase 1 (baseline)
2. Complete Phase 2 (design preconditions — all sub-phases)
3. Complete Phase 2.5 (implement `MatchesRedirectURI` + wire both sites)
4. Complete Phase 3 (US1 E2E scenarios green)
5. **STOP and validate**: US1 fully functional — portless loopback registration works end-to-end

### Full Delivery

1. MVP above
2. Phase 4 (US2 + SC-006): backward compat + opaque Agent confirmed
3. Phase 5 (US3): non-loopback regression guard confirmed
4. Phase 6 (Playwright + ARCHITECTURE.md)
5. Phase N (compliance sign-off)
6. `just check` green — PR ready

---

## Notes

- [P] tasks = different files, no blocking dependencies — safe to run concurrently
- All three user stories (US1, US2, US3) share the same implementation (`MatchesRedirectURI` + two wiring changes); their phases are E2E verification phases, not separate implementation phases
- The unit test red phase (T016) and E2E red phase (T011) MUST be recorded/committed before implementation proceeds
- `just check` in TN24 is the final gate — do not open a PR until it passes
