# Tasks: OAuth2 Provider Flavor Support

**Feature**: `20260312-120000-oauth2-provider-flavors`
**Input**: Design documents from `/specs/20260312-120000-oauth2-provider-flavors/`
**Prerequisites**: plan.md ✅ | spec.md ✅ | data-model.md ✅ | contracts/admin-api-changes.md ✅ | research.md ✅ | quickstart.md ✅

**Tests**: Per Constitution Principle VIII (TDD), unit tests in Phases 2.5/3/4/5 MUST be written and fail (red) before implementation. 24 E2E acceptance tests MUST be written and verified to fail in Phase 2f before any implementation begins.

**Organization**: Tasks grouped by user story to enable independent implementation and testing.

---

## Phase 1: Setup

**Purpose**: Review design artifacts and confirm implementation readiness before making any changes

- [X] T001 Review specs/20260312-120000-oauth2-provider-flavors/plan.md, data-model.md, contracts/admin-api-changes.md, and quickstart.md to confirm all design artifacts are complete and implementation-ready

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete all remaining design outputs before any implementation

**⚠️ CRITICAL**: No implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T002 Add OAuth2Flavor, ClientCredential, and GoogleServiceAccountKey domain terms to ARCHITECTURE.md Glossary section per data-model.md §ARCHITECTURE.md Glossary Additions in ARCHITECTURE.md

**Checkpoint**: Glossary updated with 3 new domain terms

### Phase 2b: Configuration Design [MANDATORY]

- [X] T003 Confirm no changes required to internal/ports/config.go — oauth2_flavor is per-entity data with no new runtime configuration parameters

**Checkpoint**: Configuration requirements confirmed (no changes needed)

### Phase 2c: API Design [MANDATORY]

- [X] T004 Update api/admin/openapi.yaml: add oauth2_flavor string enum field to Service response schema (required) and ServiceCreateRequest/ServiceUpdateRequest schemas (optional); update client_id, client_secret, and issuer_uri descriptions for flavor-specific semantics; remove client_id and issuer_uri from required lists per contracts/admin-api-changes.md
- [X] T004a Verify api/admin/openapi.yaml changes match spec.md API clarifications (client_id optional for google, issuer_uri optional for google, client_secret field name preserved per API-005)

**Checkpoint**: OpenAPI spec updated and confirmed

### Phase 2d: Database Design [MANDATORY]

- [X] T005 Create migrations/007_add_oauth2_flavor.up.sql with ALTER TABLE thirdparty_oauth2_services ADD COLUMN oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard' and COMMENT ON COLUMN per data-model.md §Database Schema
- [X] T005a [P] Create migrations/007_add_oauth2_flavor.down.sql with ALTER TABLE thirdparty_oauth2_services DROP COLUMN oauth2_flavor

**Checkpoint**: Migration 007 up and down files created

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

N/A — this feature has no frontend changes (admin-only API extension per spec.md §Out of Scope).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

- [X] T006 Create tests/e2e/oauth2_provider_flavor_test.go with 24 Ginkgo It() blocks mapping 1:1 to spec.md acceptance scenarios: US1.S1–US1.S5, US2.S1–US2.S7, US3.S1–US3.S6, US4.S1–US4.S2, EC1–EC4; use bootstrap.NewAdminTestServer() following tests/e2e/README.md patterns; add comment referencing spec scenario in each It() block
- [X] T006a [P] Extend tests/e2e/fixtures/services.go with GoogleServiceAccountFixture() returning a realistic non-functional service account JSON struct and ValidGoogleServiceRequest() helper returning a complete google flavor create request body
- [X] T006b Run `ginkgo -v ./tests/e2e/` and verify all 24 new tests fail semantically before any implementation (red phase — no implementation exists yet)

**Checkpoint**: 24 E2E acceptance tests written and verified to fail (red phase confirmed)

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Core domain model primitives required before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T007 Create internal/domain/model/oauth2_flavor.go with OAuth2Flavor string type, OAuth2FlavorStandard and OAuth2FlavorGoogle constants, DefaultOAuth2Flavor constant, Validate() error method (returns error for empty or unrecognized values), and String() string method
- [X] T008 Extend internal/domain/model/thirdparty_oauth2_provider.go: add Flavor OAuth2Flavor field to ThirdpartyOAuth2ProviderEntity struct; update Copy() to copy Flavor field; update RedactedCopy() to copy Flavor field (not sensitive)
- [X] T009 [P] Write TDD unit tests (red phase) in internal/domain/model/google_service_account_test.go covering 11 cases from plan.md §Unit Tests: valid JSON → populated struct; wrong type → error naming service_account; invalid JSON → parse error; missing private_key → error naming field; missing client_email → error naming field; missing token_uri → error naming field; missing client_id → error naming field; empty private_key → non-empty validation error; JSON >32KB → size limit error; issuer host matches token_uri host → valid; issuer host differs from token_uri host → validation error
- [X] T010 [P] Extend TDD unit tests (red phase) in internal/domain/model/thirdparty_oauth2_provider_test.go with 4 cases: ValidateForCreate with Flavor google and valid service account JSON credential → valid; ValidateForCreate with Flavor google and invalid JSON credential → error; ValidateForCreate with Flavor standard and empty credential → error; ValidateForCreate with unknown Flavor → error

**Checkpoint**: Domain primitives ready; TDD unit tests written and failing (red phase confirmed for T009/T010)

---

## Phase 3: User Story 1 — Configure Third-Party Service with Explicit OAuth2 Flavor (Priority: P1) 🎯 MVP

**Goal**: oauth2_flavor field accepted in create/update requests; stored persistently; returned in all GET responses; defaults to standard when omitted; unrecognized values rejected with HTTP 400.

**Independent Test**: POST /api/services without oauth2_flavor → GET returns oauth2_flavor: standard. POST with oauth2_flavor: standard → GET returns oauth2_flavor: standard. POST with oauth2_flavor: google and valid credential → GET returns oauth2_flavor: google. POST with oauth2_flavor: azure → HTTP 400 listing valid values.

### Implementation for User Story 1

- [X] T011 [US1] Implement flavor dispatch skeleton in ValidateForCreate and ValidateForUpdate in internal/domain/model/thirdparty_oauth2_provider.go: call entity.Flavor.Validate() first (rejects unknown flavors); for standard flavor apply existing non-empty credential check; for google flavor return placeholder error (completed in Phase 4)
- [X] T012 [P] [US1] Add Flavor OAuth2Flavor field to thirdPartyProviderRecord struct in internal/adapters/storage/postgres/thirdparty_provider_record.go with db tag `db:"oauth2_flavor"`
- [X] T013 [US1] Update all SQL queries in internal/adapters/storage/postgres/thirdparty_provider.go to include oauth2_flavor column in SELECT column lists and INSERT/UPDATE statements; scan into record.Flavor
- [X] T014 [US1] Persist Flavor field through Create, Get, List, and Update methods in internal/adapters/storage/memory/thirdparty_provider.go (copy Flavor value on store and retrieve)
- [X] T015 [US1] Parse oauth2_flavor field from ServiceRequest in internal/adapters/http/handlers/admin/services_handler.go; apply DefaultOAuth2Flavor when field is absent or empty; map Flavor to ServiceResponse in CreateService, UpdateService, GetService, and ListServices handlers
- [X] T015a [US1] Ensure all flavor validation errors from domain (ParseGoogleServiceAccountKey, Validate()) are mapped by the HTTP handler to a Zalando-conformant `{"title": "...", "status": 400, "detail": "..."}` JSON error response with the field-specific error message in the `detail` field, satisfying API-004

**Checkpoint**: US1 fully functional — oauth2_flavor stored and returned in all responses; standard behavior preserved; invalid flavor values rejected with HTTP 400

---

## Phase 4: User Story 2 — Configure Google OAuth2 Service with Service Account Credentials (Priority: P1)

**Goal**: google flavor accepts Google service account JSON as client_secret; client_id extracted from JSON automatically; token_endpoint derived from token_uri; authorize_endpoint set to google.Endpoint.AuthURL; issuer_uri optional with host-consistency validation when provided.

**Independent Test**: POST /api/services with oauth2_flavor: google and valid service account JSON → HTTP 201 with client_id populated from JSON's client_id field, endpoints auto-populated, credential stored encrypted. GET the service → oauth2_flavor: google, client_id correct, client_secret: REDACTED.

### Implementation for User Story 2

- [X] T016 [US2] Implement ParseGoogleServiceAccountKey in internal/domain/model/google_service_account.go: (1) size check len(credential) ≤ 32768 bytes; (2) call google.JWTConfigFromJSON to validate JSON and type==service_account; (3) unmarshal auxiliary struct for client_id extraction; (4) validate non-emptiness of ClientEmail (jwt.Config.Email), PrivateKey (jwt.Config.PrivateKey length), TokenURI (jwt.Config.TokenURL), and ClientID; return populated GoogleServiceAccountKey or field-specific error; satisfies red tests from T009
- [X] T017 [US2] Implement google flavor credential validation in ValidateForCreate and ValidateForUpdate in internal/domain/model/thirdparty_oauth2_provider.go: call ParseGoogleServiceAccountKey on credential; if IssuerURI is non-empty, validate that url.Parse(IssuerURI).Scheme+Host == url.Parse(gsk.TokenURI).Scheme+Host; return HTTP-400-worthy error on mismatch; satisfies red tests from T010
- [X] T018 [US2] Implement google flavor enrichment in CreateService handler in internal/adapters/http/handlers/admin/services_handler.go: when Flavor==google, call ParseGoogleServiceAccountKey; set entity.ClientID = gsk.ClientID; set entity.TokenEndpoint = gsk.TokenURI; set entity.AuthorizeEndpoint = google.Endpoint.AuthURL; if IssuerURI absent, set entity.IssuerURI = scheme+host of gsk.TokenURI
- [X] T019 [US2] Implement google flavor enrichment in UpdateService handler in internal/adapters/http/handlers/admin/services_handler.go: same enrichment logic as CreateService for google flavor (extract client_id, set endpoints, derive issuer_uri)

**Checkpoint**: US2 fully functional — Google service account JSON accepted; client_id extracted; endpoints auto-populated; issuer_uri host validated or derived

---

## Phase 5: User Story 3 — Validate Credentials According to OAuth2 Flavor (Priority: P2)

**Goal**: standard flavor requires non-empty plain string; google flavor requires structurally complete service account JSON with all required fields; each validation failure returns HTTP 400 with a message naming the failing field or constraint.

**Independent Test**: POST with standard flavor and empty credential → HTTP 400. POST with google flavor and non-JSON credential → HTTP 400. POST with google flavor and JSON missing each required field (type, private_key, client_email, token_uri, client_id) → HTTP 400 naming the missing field. POST with google flavor and credential >32KB → HTTP 400 naming size limit.

### Implementation for User Story 3

- [X] T020 [P] [US3] Verify internal/domain/model/google_service_account_test.go all 11 tests pass green after T016 implementation; run `go test ./internal/domain/model/... -run TestGoogleServiceAccountKey`
- [X] T021 [P] [US3] Verify internal/domain/model/thirdparty_oauth2_provider_test.go all 4 extended flavor tests pass green after T017 implementation; run `go test ./internal/domain/model/... -run TestThirdpartyOAuth2ProviderEntity`
- [X] T022 [US3] Extend tests/integration/migrations/migrations_test.go with 4 migration 007 test cases: migration 007 applies cleanly on schema with 001–006 applied; existing rows get oauth2_flavor = standard via column DEFAULT; migration 007 rolls back cleanly (DROP COLUMN); re-apply after rollback succeeds

**Checkpoint**: US3 validated — all flavor-specific credential validation rules pass unit and integration tests

---

## Phase 6: User Story 4 — List and Filter Third-Party Services by Flavor (Priority: P3)

**Goal**: oauth2_flavor field appears in every GET list response entry and every single-service GET response, enabling administrators to see the credential format for each configured service at a glance.

**Independent Test**: Create services with standard and google flavors; GET /api/services → each entry includes oauth2_flavor with its stored value; GET /api/services/{id} → response includes oauth2_flavor field.

### Implementation for User Story 4

- [X] T023 [P] [US4] Verify GET /api/services list response JSON includes oauth2_flavor for each service object in the handler's ServiceResponse mapping (covered by Phase 3 storage changes in T013/T014/T015)
- [X] T024 [US4] Verify GET /api/services/{id} single response includes oauth2_flavor field (same handler path; verify T015 maps entity.Flavor to all response shapes)

**Checkpoint**: US4 fully functional — oauth2_flavor present in all list and single-service GET responses

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify all constitution requirements are met; run full test suite; final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] T025 Verify domain model design documented in specs/20260312-120000-oauth2-provider-flavors/data-model.md and ARCHITECTURE.md Glossary section (Principle V)
- [X] T026 Verify no new parameters added to internal/ports/config.go (Principle VII)
- [X] T027 [P] Verify api/admin/openapi.yaml updated with oauth2_flavor field in all three schemas per contracts/admin-api-changes.md (Principles IV, X)
- [X] T028 Verify API design confirmed via spec.md clarifications and plan.md §API Design checklist (Principle X)
- [X] T029 [P] Verify migrations/007_add_oauth2_flavor.up.sql and .down.sql exist in /migrations/ with correct go-migrate naming format (Principle IX)
- [X] T030 Verify tests/e2e/oauth2_provider_flavor_test.go contains exactly 24 It() blocks with 1:1 mapping to spec.md acceptance scenarios (Principle XIII)
- [X] T031 Verify E2E tests were confirmed to fail in red phase before implementation began (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [X] T032 [P] Verify api/admin/openapi.yaml Service schema includes oauth2_flavor in both properties and required list
- [X] T033 [P] Verify ServiceCreateRequest and ServiceUpdateRequest schemas do NOT list client_id or issuer_uri as unconditionally required (they are optional for google flavor)

**Architecture & Documentation** (Principle II):
- [X] T034 Verify ARCHITECTURE.md Glossary contains OAuth2Flavor, ClientCredential, and GoogleServiceAccountKey with correct definitions from data-model.md
- [X] T035 [P] Confirm no new ADR required — this is an additive change following established model extension patterns per plan.md §ADRs

**Configuration** (Principle VII):
- [X] T036 [P] Verify internal/ports/config.go is unchanged — flavor configuration is per-entity, not per-server

**Database & Persistence** (Principle IX):
- [X] T037 [P] Verify migrations/007_add_oauth2_flavor.up.sql and .down.sql follow NNN_description.{up,down}.sql go-migrate naming convention
- [X] T038 [P] Verify tests/integration/migrations/migrations_test.go covers migration 007 apply, rollback, and re-apply cycles
- [X] T039 [P] Verify internal/adapters/storage/postgres/thirdparty_provider.go includes oauth2_flavor in all SELECT, INSERT, and UPDATE queries
- [X] T040 Verify internal/adapters/storage/memory/thirdparty_provider.go persists Flavor through Create, Get, List, and Update

**Security** (Principles I, III):
- [X] T041 Verify credential validation fails closed in ValidateForCreate/ValidateForUpdate — services with invalid credentials are never persisted (SR-005)
- [X] T042 [P] Verify google private_key value never appears in any error message, log line, or API response in any form (SR-002, SR-003)
- [X] T043 [P] Verify 32 KB size check is the FIRST operation in ParseGoogleServiceAccountKey before any JSON parsing (SR-004)

**Architecture Patterns** (Principle VI):
- [X] T044 Verify internal/domain/model/oauth2_flavor.go and google_service_account.go have no imports from adapters/ or app/ packages
- [X] T045 Verify adapters/storage/postgres, adapters/storage/memory, and adapters/http/handlers/admin import domain/model but domain/model does not import any adapter

**Testing** (Principle VIII):
- [X] T046 Verify internal/domain/model/google_service_account_test.go was written before implementation (committed in red phase) and all 11 tests pass green
- [X] T047 Verify internal/domain/model/thirdparty_oauth2_provider_test.go extended tests were written before implementation and all 4 new flavor tests pass green
- [X] T048 Verify tests/integration/migrations/migrations_test.go extended with migration 007 test cases

**E2E Acceptance Testing** (Principle XIII):
- [X] T049 Verify each of 24 It() blocks in tests/e2e/oauth2_provider_flavor_test.go maps to exactly one spec.md acceptance scenario (check comment references)
- [X] T050 Verify E2E tests use bootstrap.NewAdminTestServer() and fresh storage per test via BeforeEach/AfterEach per tests/e2e/README.md
- [X] T051 Verify E2E test file uses Describe (feature) → Context (preconditions) → It (scenario) hierarchy
- [X] T052 Run full E2E test suite: `ginkgo -v ./tests/e2e/` — all 24 new tests plus existing tests must pass

### Additional Polish

- [X] T053 [P] Run `just check` (fmt → vet → lint) and `just verify`, and confirm both pass with zero errors
- [X] T054 Run quickstart.md validation — execute the curl examples from specs/20260312-120000-oauth2-provider-flavors/quickstart.md against the running dev server and verify expected responses

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Design Preconditions (Phase 2)**: Depends on Phase 1 — BLOCKS all implementation
  - Phases 2a, 2b, 2c, 2d, 2f can run in parallel within Phase 2
  - **CRITICAL**: All 24 E2E test stubs (T006) must fail before Phase 2.5 begins
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 — BLOCKS all user stories
- **User Stories**: All depend on Phase 2 + Phase 2.5
  - US1 (Phase 3): No story dependencies
  - US2 (Phase 4): Depends on US1 (Flavor field must exist on entity and in storage)
  - US3 (Phase 5): Depends on US1 + US2 (validation rules implemented in those phases)
  - US4 (Phase 6): Depends on US1 storage changes only; can verify alongside US3
- **Polish (Phase N)**: Depends on all desired user stories complete

### User Story Dependencies

- **US1 (P1)**: Start after Phase 2.5 — no story dependencies
- **US2 (P1)**: Depends on US1 — Flavor field, storage adapters, and handler parsing must be in place
- **US3 (P2)**: Depends on US1 + US2 — verifies existing unit/integration tests turn green; no new domain code
- **US4 (P3)**: Depends on US1 only — flavor appears in responses once storage and handler changes are in place

### Within Each User Story

- TDD: unit tests written and failing before implementation (T009/T010 red before T016/T017)
- Storage adapter changes before HTTP handler changes
- Domain validation (entity) before handler enrichment
- Postgres adapter and memory adapter can be done in parallel within US1

### Parallel Opportunities

- T005/T005a — migration up/down files: parallel
- T006/T006a — E2E tests + fixtures: parallel
- T009/T010 — TDD unit tests: parallel
- T012 (postgres record) and T014 (memory adapter) within US1: parallel
- T020/T021 (verify unit tests green) within US3: parallel
- T023/T024 (US4 verification): parallel

---

## Parallel Execution Example: User Story 2

```bash
# After US1 complete, US2 tasks execute in this order:
Task A (T016): Implement ParseGoogleServiceAccountKey in internal/domain/model/google_service_account.go
Task B (T017): (after A) Implement google validation in internal/domain/model/thirdparty_oauth2_provider.go
Task C (T018): (after B) Implement CreateService google enrichment in internal/adapters/http/handlers/admin/services_handler.go
Task D (T019): (after B, serialize with C — same file) Implement UpdateService google enrichment in internal/adapters/http/handlers/admin/services_handler.go
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 Only)

1. Complete Phase 1 (review artifacts)
2. Complete Phase 2 all sub-phases (OpenAPI, migrations, 24 E2E tests in red)
3. Complete Phase 2.5 (OAuth2Flavor type, entity extension, TDD unit tests in red)
4. Complete Phase 3 (US1) — flavor field end-to-end
5. Complete Phase 4 (US2) — Google credential handling
6. **STOP and VALIDATE**: Run `ginkgo -v ./tests/e2e/` — US1 + US2 E2E tests (17 tests) should be green
7. Deploy/demo if ready

### Incremental Delivery

1. Phase 1 → Phase 2 → Phase 2.5 → Foundation ready
2. Phase 3 (US1) → Flavor field works → US1 E2E tests (5) green
3. Phase 4 (US2) → Google credentials work → US2 E2E tests (7) green
4. Phase 5 (US3) → Validation verified → US3 E2E tests (6) green + unit tests green
5. Phase 6 (US4) → Flavor in list/get → US4 E2E tests (2) + edge case tests (4) green
6. Phase N → Compliance verified → `just check` and `just verify` pass → PR ready

### Parallel Team Strategy

With multiple agents:

- **Phase 2 (parallel)**: Architecture agent handles 2a; API agent handles 2c; DB agent handles 2d; QA agent handles 2f
- **Phase 3 within-story parallel**: Storage agent handles T012 (postgres record) + T013 (SQL) + T014 (memory); Handler agent handles T015 (ServiceRequest parsing) after T014
- **Phase 4 within-story serial**: Domain agent handles T016 (parse) → T017 (validate); Handler agent handles T018 + T019 (both touch same file, serialize)

---

## Notes

- `[P]` tasks have no file conflicts and no dependencies on other pending tasks — run in parallel
- `[US#]` label maps each task to a user story for traceability
- US1 and US2 are both P1 but sequential — implement US1 first (flavor field foundation)
- All 24 E2E test scenarios must be in red phase before Phase 2.5 begins
- Existing standard flavor services must pass all existing tests unchanged throughout implementation
- Google private_key must never appear in logs, error messages, or API responses
- `golang.org/x/oauth2/google.JWTConfigFromJSON` is the only approved JSON parser for Google service accounts — no custom parsing
