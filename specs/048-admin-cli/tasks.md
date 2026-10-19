# Tasks: Admin CLI (`aibctl`)

**Input**: `specs/048-admin-cli/spec.md`, `specs/048-admin-cli/plan.md`, ADR 038
**Status**: all tasks complete on branch `claude/nifty-fermat-uptxnc`

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [x] T001 Create `cmd/aibctl/main.go`, `internal/aibctl/version`, justfile `aibctl-build`/`aibctl-test` targets.
- [x] T002 Add `internal/aibctl/architecture_test.go` forbidding broker package imports.

## 🔒 Phase 2: Design Preconditions

### Phase 2a: Domain Model & Glossary
- [x] T010 Glossary entries: Context (aibctl), Token Command, Canonical-first display (ARCHITECTURE.md §12).

### Phase 2b: Configuration Design
- [x] T011 Config schema, precedence, env mapping documented in `docs/configuration.md`; example `examples/config/aibctl.yaml`; Helm chart deliberately untouched (standalone deployment boundary, ADR 011 condition 5).

### Phase 2c: API Design
- [x] T012 No Admin API change; DTOs re-declared from `api/admin/openapi.yaml` in `internal/aibctl/client/types.go`.

### Phase 2d: Database Design
- [x] T013 Not applicable.

### Phase 2e: Frontend/Design System Review
- [x] T014 Not applicable (no frontend).

### Phase 2f: E2E Acceptance Test Design
- [x] T015 One `It()` per scenario US1-S1..US7-S1 in `tests/e2e/aibctl/`, in-process harness against `bootstrap.NewAdminTestServer`.

## Phase 2.5: Foundational Infrastructure

- [x] T020 [P] `internal/aibctl/config` with tests (precedence matrix, validation, save permissions, redaction).
- [x] T021 [P] `internal/aibctl/auth` with helper-process tests (trim, exit status, timeout, empty, missing binary).
- [x] T022 [P] `internal/aibctl/client` core, errors, health, DTOs with tests.
- [x] T023 [P] `internal/aibctl/output` and `internal/aibctl/input` with tests.
- [x] T024 `internal/aibctl/cmd` root, runtime, exit codes, confirm, version, config, context, health with tests.

## Phase 3: User Story 1 – Contexts (P1)
- [x] T030 [US1] E2E `context_test.go` (US1-S1..S3).

## Phase 4: User Story 2 – Agents (P1)
- [x] T040 [US2] Client agents, `cmd/agents.go`, three-state canonical/client IDs, completers; unit tests with fake server.
- [x] T041 [US2] E2E `agents_test.go` (US2-S1..S6).

## Phase 5: User Story 3 – Services (P2)
- [x] T050 [US3] Client services + protected resources, `cmd/services.go`, `cmd/protected_resources.go`, ETag/If-Match, redacted-secret guard; unit tests.
- [x] T051 [US3] E2E `services_test.go` (US3-S1..S4).

## Phase 6: User Story 4 – Permission sets (P2)
- [x] T060 [US4] Client + `cmd/permission_sets.go`; unit tests; E2E (US4-S1..S2).

## Phase 7: User Stories 5–6 – Credentials and keys (P3)
- [x] T070 [US5][US6] `client/keys.go`, `cmd/client_credentials.go`, `cmd/keys.go`; unit tests; E2E `keys_test.go` (US5-S1, US6-S1..S2).

## Phase 8: User Story 7 – Completion (P2)
- [x] T080 [US7] `completeResource` with timeout and silent failure; E2E (US7-S1).

## 🔒 Phase N: Constitution Compliance & Polish

- [x] T090 ADR 038, ARCHITECTURE.md §1, §3.3, §10, §12.
- [x] T091 `docs/guides/admin-cli.md`, `docs/configuration.md` section, docs sidebar entry.
- [x] T092 `cmd/aibctl/AGENTS.md`, `internal/aibctl/AGENTS.md`, root `AGENTS.md` index, `tests/e2e/AGENTS.md`.
- [x] T093 justfile cross-compile targets, `release.yml` artefacts, `ci.yml` binary verification, `go mod tidy`.
- [x] T094 `golangci-lint` clean on new packages; unit and E2E suites green.

## Dependencies & Execution Order

Phase 2.5 before any user story; US1 before the others; US2 before US5; services before protected resources.
