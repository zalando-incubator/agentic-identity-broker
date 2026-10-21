---

description: "Task list for feature 052: inline and file-based JWKS for trust anchors"
---

# Tasks: Inline and File-Based JWKS for Trust Anchors

**Input**: Design documents from `/specs/052-inline-jwks-trust-anchors/`

**Prerequisites**:

- [plan.md](./plan.md)
- [spec.md](./spec.md)
- [research.md](./research.md)
- [data-model.md](./data-model.md)
- [contracts/configuration.md](./contracts/configuration.md)
- [contracts/health-endpoint.md](./contracts/health-endpoint.md)
- [contracts/helm-values.md](./contracts/helm-values.md)
- [quickstart.md](./quickstart.md)

**Tests**: Constitution Principle VIII (TDD) and Principle XIII (E2E acceptance) make tests mandatory. Every implementation task follows a test task that is written first and fails semantically.

**Organization**: Tasks are grouped by user story, so each story can be implemented and tested independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: The task can run in parallel. It touches different files and depends on no incomplete task.
- **[Story]**: The user story that the task belongs to. US1 to US5 map to spec.md User Stories 1 to 5.
- Every task names exact file paths.
- `US<n>-S<m>` means spec.md User Story *n*, Acceptance Scenario *m*.

## 🔒 Feature Scope [MANDATORY]

- Every task cites one of the following:
  - an approved requirement (FR, SR, or SC);
  - an acceptance scenario;
  - a binding constraint for the affected path (an ADR, a constitution principle, or the Helm contract).
- [research.md](./research.md) lists four "Deferred findings". They are **not** tasks. The user decides whether to file GitHub issues for them. Only user-approved issue URLs go under the **Future Work** note at the end of this file.
- On 2026-10-16 the stakeholder approved extending the scope to `server.admin.authentication.jwt` (research R9). `spec.md` FR-002 and User Story 5 scenario 4 were updated before implementation, as the Feature Scope rule requires. Tasks T033, T035, T080, T082, T083, and T084 cover it.

---

## Phase 0: Pre-implementation Refactoring — Skipped

The plan skips this phase. The only moves of existing code are the approved-algorithm set and the 1 MiB constant. Both are a few lines, change no behaviour, and are done in Phase 2.5 (T046, T047).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: E2E helpers and CI prerequisites that every user story's acceptance tests need.

- [X] T001 [P] Add local key-set E2E helpers in `tests/e2e/helpers/local_jwks.go`. Rely on the jwx `jwk`/`jws`/`jwt` packages already used by `tests/e2e/helpers/jwt_helpers.go`, and add no new library. The helpers must:
  - generate in-memory ES256 (P-256) and RS256 (2048-bit) key pairs with a chosen `kid` and `alg`;
  - marshal the public JWK Set as compact JSON;
  - deliberately marshal invalid sets: a private EC key, a private RSA key, a `kty: oct` key, an unapproved `alg` (`HS256`), a duplicate `kid`, an empty `keys` array, a bare JWK without `keys`, and a document over 1 MiB;
  - sign client-assertion, actor, subject, and pre-auth JWTs with chosen `iss`, `sub`, `aud`, and `exp`;
  - expose each key's base64url `x`, `y`, `n`, and `d` member values, so tests can assert those values never appear in output.

  Supports US1–US5 and research R14.
- [X] T002 [P] Add trust-directory writers to `tests/e2e/helpers/local_jwks.go`, in a separate section after T001's content:
  - create a per-test trust directory under `GinkgoT().TempDir()`;
  - write a plain key-set file;
  - write the Kubernetes layout: `<name>` → `..data/<name>` → `..<timestamp>/<name>`, using relative links only;
  - swap atomically to a new timestamped directory and delete the old one;
  - create a relative link escaping to `../outside/<name>` and an absolute link to a file outside the directory;
  - write a corrupt (truncated) file in place.

  Supports US2-S3, US2-S4, US2-S6, US2-S8, and research R4.
- [X] T003 [P] Add a counting `httptest` server helper in `tests/e2e/helpers/counting_server.go`. It counts every request regardless of path, exposes `Count()` and `URL()`, and serves 404 for every path. Supports US1-S1, US4-S1, and SC-002.

**Checkpoint**: The helpers compile (`go vet ./tests/e2e/...`).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete the domain, configuration, API, database, spec-journey, and E2E design before implementation.

**⚠️ CRITICAL**: No production code changes, apart from the T037–T042 compile-only scaffolding that T036 needs, until this phase is complete and the stakeholder gates T007 (ADR 040), T022 (`/health`), and T028 (journeys) are recorded.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II and V

- [X] T006 Draft `adrs/040-trust-anchor-local-key-sets.md` with status Proposed. It extends ADR 029 and ADR 008 and supersedes neither. Record each of the following, per research R15:
  - the three mutually exclusive key sources (`jwks_uri`, `jwks`, `jwks_file`), with no precedence;
  - the `security.trust_material_dir` boundary through `os.Root`, including the R4 consequence that "absolute symbolic links inside the trust directory are rejected; relative links (Kubernetes) are accepted";
  - the file re-read cadence: every `jwks_min_refresh`, and fail closed after `jwks_max_refresh` since the last success;
  - the `trust_anchor_key_files` `/health` component;
  - the JWT pre-authentication scope (revised 2026-10-16: both servers, each a separate trust anchor).
- [X] T007 Obtain stakeholder acceptance of ADR 040 and of the two scope interpretations: R4 (absolute links rejected; confirmed) and R9 (decided 2026-10-16: support `server.admin` local sources, and revise ADR 040 accordingly). Then set ADR 040 to Accepted in `adrs/040-trust-anchor-local-key-sets.md`, and record the acceptance date in the PR description. Implementation and unit-test tasks from T043 on MUST NOT start until this is done (Principle II, plan "Stakeholder decisions" 2–3).
- [X] T008 [P] Add the glossary terms to `ARCHITECTURE.md`, using the exact definitions in [data-model.md](./data-model.md) "Glossary additions": **Key Set Source**, **Local Key Set**, and **Trust Directory**. Disambiguate Key Set Source from the JWKS-aggregation **KeySource**. Update the existing **ClientAssertion Trust Anchor**, **Trusted Token Issuer**, and **JWTAuthConfig** entries to name `jwks` and `jwks_file` as alternatives to `jwks_uri` (Principle V).
- [X] T009 [P] Update `ARCHITECTURE.md` §3.1.4.1 for the trust-anchor key sources, the resolution order, the trust directory, and the `trust_anchor_key_files` health component. Update the JWT pre-authentication section to describe the injected local key set and the admin rejection. Add ADR 040 to the ADR index (Principle II, plan "Architecture documentation").
- [X] T010 [P] Add ADR 040 to the root `AGENTS.md` ADR Decision Index, with the decision "Inline and file-based key sets for trust anchors (trust directory, re-read, health)". Add Key Set Source, Local Key Set, and Trust Directory to its Domain Glossary table (Principle V).
- [X] T011 [P] Update the internal guides:
  - `internal/AGENTS.md`: add `trustanchor/` to the package map, and add static and file adapters to `jwks/`.
  - `internal/domain/AGENTS.md`: describe the `trustanchor` bounded context and its consumers.
  - `internal/adapters/AGENTS.md`: describe `StaticAdapter`, `FileAdapter`, and `TrustDirectory` in the JWKS section.

  Supports plan "Project Structure".

**Checkpoint**: ADR 040 is Accepted, the glossary and architecture are updated, and the agent guides are consistent.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [X] T012 Document the seven new keys in `docs/configuration.md`, exactly as [contracts/configuration.md](./contracts/configuration.md) §1–§3 specifies them:
  - `security.trust_material_dir`;
  - `jwks` and `jwks_file` on `token_exchange.client_assertion`, on impersonation `trusted_issuers[]`, and on `server.enduser.authentication.jwt`.

  The section must include:
  - the environment overrides `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS`, `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_FILE`, and `IDENTITY_BROKER_SECURITY_TRUST_MATERIAL_DIR`;
  - the `${VAR}` expansion rule: the reference is the whole value (corrected 2026-10-16: the loader checks the YAML value, not the substituted one);
  - the refresh-bound table, including the fixed 5m/15m for JWT pre-authentication;
  - the key-set document and file rules, with every startup error message;
  - the log events;
  - the recommendation to set `alg` and `kid` on every key;
  - the R4 absolute-symlink rule.

  Supports FR-021.
- [X] T013 [P] Create `examples/config/trust-anchor-local-jwks.yaml`, a complete, loadable local-mode example with these parts:
  - `security.trust_material_dir: "/etc/agentic-identity-broker/trust"`;
  - an inline `token_exchange.client_assertion.jwks` for issuer `https://agentgateway.internal.example.com`, using the ES256 `kid` `agentgateway-2026-10`;
  - an impersonation trusted issuer with `jwks_file: "/etc/agentic-identity-broker/trust/agentgateway-jwks.json"`, `allowed_algorithms: ["ES256"]`, and `signs_roles: ["client_assertion", "actor"]`;
  - `server.enduser.authentication.jwt` with `verification: jwks` and `jwks_file: "/etc/agentic-identity-broker/trust/mesh-jwks.json"`.

  Match the spec's Configuration Requirements example and Journeys 1–5. Supports FR-021.
- [X] T014 [P] Add commented `jwks` and `jwks_file` alternatives next to the existing `jwks_uri` lines in `examples/config/token-exchange.yaml`, `examples/config/impersonation.yaml`, and `examples/config/jwt-preauth.yaml`. Note that they are mutually exclusive and that `jwks_file` requires `security.trust_material_dir`. Supports FR-021.
- [X] T015 Reference `trust-anchor-local-jwks.yaml` in `examples/config/README.md`, with a one-line description of each of the three trust anchors it configures (Principle VII).
- [X] T016 [P] Add the new chart values to `charts/agentic-identity-broker/values.yaml`, with `# --` comments, exactly as [contracts/helm-values.md](./contracts/helm-values.md) "New values" specifies:
  - `broker.security.trustMaterial.directory`, default `""`;
  - `broker.security.trustMaterial.sources`, default `[]`, with the commented Secret and ConfigMap examples;
  - `broker.tokenExchange.clientAssertion.jwks`;
  - `broker.tokenExchange.clientAssertion.jwksFile`.

  Supports FR-020.
- [X] T017 [P] Extend `charts/agentic-identity-broker/values.schema.json` per [contracts/helm-values.md](./contracts/helm-values.md) "Schema":
  - `broker.security.trustMaterial`: `directory` is a string that is empty or an absolute path; `sources` is an array of `oneOf` `{secret: {name, items}}` / `{configMap: {name, items}}` objects with `additionalProperties: false`, where items are `{key, path}`;
  - `jwks` and `jwksFile` on `broker.tokenExchange.clientAssertion`;
  - `jwks` and `jwks_file` on the impersonation `trusted_issuers[]` items and on `definitions.serverConfig.authentication.jwt`.

  Supports FR-020.
- [X] T018 Render the new values in `charts/agentic-identity-broker/templates/configmap.yaml`:
  - `security.trust_material_dir` when `trustMaterial.directory` is non-empty;
  - `token_exchange.client_assertion.jwks` as a YAML block scalar;
  - `token_exchange.client_assertion.jwks_file`;
  - the template failure `broker.security.trustMaterial.directory is required when sources are set`.

  Do not create a second top-level `security:` key. Supports FR-020 and R13.
- [X] T019 Render the `trust-material` volume in `charts/agentic-identity-broker/templates/deployment.yaml`. It is a `projected` volume with one `secret` or `configMap` source per `trustMaterial.sources` entry, mounted `readOnly: true` at `trustMaterial.directory` **without `subPath`**. Render nothing when `sources` is empty. Supports FR-020 and R13.
- [X] T020 Add the four new values to the parameter table in `charts/agentic-identity-broker/README.md`. Add a "Trust material for local key sets" section with the Secret example, the no-`subPath` rule, and the rotation behaviour. Supports FR-020 and FR-021.
- [X] T021 Run `helm lint charts/agentic-identity-broker`. Also run `helm template` with the [contracts/helm-values.md](./contracts/helm-values.md) Secret and ConfigMap example. Confirm the projected volume, the read-only mount without `subPath`, and the rendered `security.trust_material_dir` (Principle VII).

**Checkpoint**: The configuration reference, examples, and Helm contract are complete.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV and X

- [X] T022 Present [contracts/health-endpoint.md](./contracts/health-endpoint.md) to the stakeholder: the optional `trust_anchor_key_files` component, its values `healthy` and `degraded`, and that it has no effect on HTTP status. Record the confirmation date in the PR description. T023 MUST NOT start before this is confirmed (plan "Stakeholder decisions" 1).
- [X] T023 Update the `/health` operation in `api/enduser/openapi.yaml`. Replace the description paragraph that starts "In proxy and hybrid OAuth2 modes" with the text in [contracts/health-endpoint.md](./contracts/health-endpoint.md), and add the `healthyWithDegradedKeySetFile` `200` example. Make no schema change, because `HealthResponse.components` is already a string map (Principle IV).
- [X] T024 [P] Confirm and record in the PR description that `api/admin/openapi.yaml` does not change. The admin `/health` is unchanged and no admin route is added (Principle IV).
- [X] T025 Create `docs/api/health.md`, following the style of `docs/api/approval-endpoints.md`. It documents `GET /health`, the `components` map with `upstream_jwks` and `trust_anchor_key_files`, when each component appears, the fact that components never change the HTTP status, the RFC-style example exchange from [contracts/health-endpoint.md](./contracts/health-endpoint.md), and the operator action for `degraded` (check the `trust anchor key set re-read failed` log). Then run `just docs-build` and confirm the Redoc `/health` description and example render (Principle IV: end-user API docs rendered in `docs/api/`).

**Checkpoint**: The `/health` delta is confirmed and documented in OpenAPI.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [X] T026 Record "no schema change" in the PR description: no migration, repository, or persisted entity. Per [data-model.md](./data-model.md), every concept is built from configuration at startup (Principle IX).

**Checkpoint**: Confirmed that no database changes are needed.

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI

- [X] T027 Record "no UI change" in the PR description. The feature changes no React code and has no UI state, so no Wireloom wireframes, Playwright tests, or screenshots apply (Principles XI, XIII, XIV).

**Checkpoint**: Confirmed that no design-system work is needed.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII and XIV

All E2E tests use production bootstrap through `tests/e2e/bootstrap/`, with fresh storage, servers, keys, and trust directory per test. Each scenario `It()`:

- carries `Label("inline-jwks-trust-anchors")`;
- has a comment that cites its scenario ID;
- sits inside `Describe` (feature) → `Context` (precondition) → `It` (scenario).

Use `bootstrap.NewBufferedJSONLogger` for log assertions and `Eventually` for `/health` polling. File scenarios set `jwks_min_refresh: 200ms` and `jwks_max_refresh: 2s`.

- [X] T028 Review the five Concrete Example Journeys in `specs/052-inline-jwks-trust-anchors/spec.md` with the stakeholder. Check each one's link to its Given/When/Then scenarios, its configuration snippets against [contracts/configuration.md](./contracts/configuration.md), its HTTP exchanges against `api/enduser/openapi.yaml`, and its decoded JWT claims against FR-005 and FR-015. Correct any mismatch in the spec (Principle XIV, plan "Stakeholder decisions" 4).
- [X] T029 Write the User Story 1 `It()` blocks in `tests/e2e/trust_anchor_inline_jwks_test.go`:
  - **US1-S1**: Local mode with an inline set starts successfully. The counting server at the issuer host records 0 requests after startup and after a verification.
  - **US1-S2**: A token exchange whose client assertion is signed by the inline key returns `200` with the vault token.
  - **US1-S3**: An assertion signed by an unlisted key returns `401` with `error=invalid_client`, and no token is issued.
  - **US1-S4**: `POST /api/approvals` with an inline-key `X-Client-Assertion` authenticates the gateway.
  - **US1-S5**: Hybrid mode without `issuer_uri` expects the upstream issuer, accepts an inline-key assertion, rejects an assertion signed by an upstream key, and verifies subject tokens against the upstream keys.
- [X] T030 Write the User Story 3 `It()` blocks in `tests/e2e/trust_anchor_inline_jwks_test.go`, in a separate `Context` after T029's blocks. Each asserts the `BuildApp` error substring from [contracts/configuration.md](./contracts/configuration.md) §2:
  - **US3-S1**: `only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)`.
  - **US3-S2**: a bare JWK gives `key set is not a valid JWK Set document`; `{"keys":[]}` gives `key set contains no keys`; a document over 1 MiB gives `key set exceeds 1048576 bytes`.
  - **US3-S3**: `key set contains private key material; only public keys are allowed`, and neither the error nor the buffered log contains the key's `d`, `x`, `y`, or `n` values.
  - **US3-S4**: `key set contains a symmetric key; only public asymmetric keys are allowed`, and `key "<kid>" declares an algorithm outside the approved asymmetric set`.
  - **US3-S5**: `key identifier "<kid>" appears more than once`.
  - **US3-S6**: `jwks_min_refresh and jwks_max_refresh are not allowed with jwks; an inline key set never refreshes`.
  - **US3-S7**: The existing self-trust rejection fires for the broker's own issuer, with both an inline and a file source.
  - **US3-S8**: The `trust anchor key set loaded` log has `trust_anchor`, `key_source`, `key_count`, and `key_ids`, and no key-member values.
- [X] T031 [P] Write the User Story 2 `It()` blocks in `tests/e2e/trust_anchor_key_files_test.go`:
  - **US2-S1**: A file inside the trust directory verifies assertions.
  - **US2-S2**: A missing path gives `jwks_file "<path>" does not exist`, and a directory path gives `jwks_file "<path>" is not a regular file`.
  - **US2-S3**: After an atomic swap, the new key is accepted and the removed key is rejected within `Eventually` 3s.
  - **US2-S4**: A corrupt re-read keeps verification working. `/health` reports `components.trust_anchor_key_files == "degraded"`, the log `trust anchor key set re-read failed` has `error_kind` and no file contents, and a valid file restores `healthy`.
  - **US2-S5**: A path outside the trust directory gives `jwks_file "<path>" is outside security.trust_material_dir "<dir>"`, and the file is never opened. Make the outside path a FIFO with no writer, and assert that `BuildApp` returns the error within 2s: opening the FIFO would block, so a timely error proves the broker never opened it. Do not rely on atime, which `noatime` and `relatime` mounts make meaningless.
  - **US2-S6**: A `..` escape, a relative-link escape, and an absolute link whose target is inside the trust directory all give the US2-S5 message (FR-023).
  - **US2-S7**: `jwks_file requires security.trust_material_dir to be set`.
  - **US2-S8**: The Kubernetes `..data` layout starts successfully.
- [X] T032 [P] Write the User Story 4 `It()` blocks in `tests/e2e/impersonation_local_jwks_test.go`:
  - **US4-S1**: An inline trusted issuer makes 0 discovery requests (counting server), and an impersonation request mints a token whose decoded `act.iss` and `act.sub` match the gateway.
  - **US4-S2**: A file trusted issuer validates the credentials.
  - **US4-S3**: With `allowed_algorithms: ["ES256"]`, an RS256 credential is rejected.
  - **US4-S4**: Rules A and B for the same `issuer_uri` with different inline keys. A credential signed by rule B's key never matches rule A.
- [X] T033 [P] Write the User Story 5 `It()` blocks in `tests/e2e/jwt_preauth_local_jwks_test.go`:
  - **US5-S1**: `GET /api/me` derives the principal with an inline source and, in a second `Context`, with a file source.
  - **US5-S2**: `verification: none` with `jwks`, and with `jwks_file`, gives `verification 'none' and jwks are mutually exclusive` (or `… jwks_file …`).
  - **US5-S3**: `verification: jwks` with no source fails startup with the existing message.
  - **US5-S4**: The admin server with an inline admin key set and the end-user server with a different file key set. `POST /api/sessions/sweep` (dry run) with an admin-key JWT returns `200`, and with an end-user-key JWT is rejected as for an unknown remote key.
- [X] T034 [P] Write the SC-003 parity `DescribeTable` in `tests/e2e/trust_anchor_key_source_parity_test.go`, labelled `Label("inline-jwks-trust-anchors-parity")`. Cross five credentials (valid, wrongly signed, expired, wrong audience, wrong issuer) with three sources (remote via `httptest.NewServer` in `tests/e2e/trust_anchor_key_source_parity_test.go`, inline, file). Assert identical HTTP status codes and `error` codes across the sources for each credential.
- [X] T035 Verify that `ginkgo -v --label-filter="inline-jwks-trust-anchors" ./tests/e2e/` selects exactly 29 `It()` blocks, one per spec scenario: US1 ×5, US2 ×8, US3 ×8, US4 ×4, US5 ×4. Confirm that no file contains `XIt`, `PIt`, `Skip()`, a placeholder always-fail assertion, or a "red phase" comment (Principle XIII).
- [X] T036 After the T037–T042 compile-only scaffolding, run the suite and confirm the red phase. Every `It()` and parity entry must compile and fail on its concrete assertion, such as a status, error substring, log field, or `/health` value, and not on a compile error or panic. This is the only allowed dependency from Phase 2f on Phase 2.5 (Principle XIII, plan "Red phase").

**Checkpoint**: Journeys are reviewed, and all 29 scenarios plus the parity table are written and verified red.

---

## Phase 2.7: Entity Boilerplate — Skipped

The plan skips this phase: there is no new persisted entity, port, route, or handler.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: The `trustanchor` domain context, the configuration fields, the three adapter primitives, and loader and validator support. Every user story depends on them.

**⚠️ CRITICAL**: No user-story work can begin until this phase is complete. T046 onward also requires T007 (ADR 040 Accepted).

### Compile-only scaffolding (enables T036)

- [X] T037 [P] Add the seven configuration fields to `internal/ports/config.go`, with `mapstructure` tags exactly as [data-model.md](./data-model.md) "Configuration model changes" specifies:
  - `ClientAssertionTrustConfig.JWKS` (`jwks`) and `.JWKSFile` (`jwks_file`);
  - `TrustedTokenIssuerConfig.JWKS` and `.JWKSFile`;
  - `JWTConfig.JWKS` and `.JWKSFile`;
  - `SecurityConfig.TrustMaterialDir` (`trust_material_dir`).

  Add one-line doc comments that match the surrounding style. Supports FR-001, FR-006, and FR-022.
- [X] T038 [P] Create the `internal/domain/trustanchor/` scaffolding with exported types and signatures only:
  - `source.go`: `AnchorKind`, `AnchorRef`, `SourceKind`, `SourceInput`, `RefreshBounds`, `KeySetSource`, and `ResolveKeySetSource`;
  - `local_key_set.go`: `LocalKeySet`, `ParseLocalKeySet`, `DiffKeyIDs`, and `MaxKeySetBytes = 1 << 20`;
  - `algorithms.go`: `IsApprovedAlgorithm`;
  - `errors.go`: `Category` and `Error`.

  Function bodies return a `not implemented` error. Supports plan §1–§2.
- [X] T039 [P] Create the adapter scaffolding in `internal/adapters/jwks/`. Each constructor returns a `not implemented` error, and each type satisfies `ports.JWKSPort`:
  - `static_adapter.go`: `StaticAdapter` and `NewStaticAdapter`;
  - `file_adapter.go`: `FileAdapter`, `NewFileAdapter`, and `Shutdown`;
  - `trust_directory.go`: `TrustDirectory`, `OpenTrustDirectory`, `Open`, and `Close`.

  Supports plan §3–§4.
- [X] T040 [P] Add the optional `KeySet ports.JWKSPort` field to `jwtauth.JWXAuthenticatorConfig` in `internal/adapters/jwtauth/jwx_authenticator.go`. Leave its behaviour unchanged for now. Supports research R9.
- [X] T041 Add a no-op `trust_anchor_key_files` contribution hook to `App.EnduserHealthComponents()` in `internal/app/builder.go`. It contributes nothing until US2. Supports research R11.
- [X] T042 Run `just build` and `go vet ./...` to confirm that the scaffolding compiles. Then run T036.

### Domain: `trustanchor` (test-first)

- [X] T043 [P] Write table tests in `internal/domain/trustanchor/algorithms_test.go`. Exactly RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512, and EdDSA are approved. `none`, `HS256`, `HS384`, `HS512`, the empty string, and lower-case variants are rejected. Supports research R6.
- [X] T044 [P] Write table tests in `internal/domain/trustanchor/source_test.go` for each [data-model.md](./data-model.md) resolution rule, in order:
  1. More than one of `jwks_uri`, `jwks`, `jwks_file` is non-empty: `conflicting_key_sources`, naming the anchor and the set keys (FR-009).
  2. `jwks` is set and an explicit refresh bound is set: `refresh_bounds_with_inline` (FR-012).
  3. JWT pre-authentication with `verification: none` and any key source: `verification_disabled_with_key_source` (FR-013).
  4. `jwks_file` is not an absolute path: `relative_path` (FR-010).
  5. `jwks_file` is set and the trust directory is unset: `missing_trust_directory` (FR-022).
  6. `jwks_file`, after `filepath.Clean`, is not under the cleaned trust directory: `outside_trust_directory`, and nothing is read (FR-023).
  7. `jwks` set → `inline`.
  8. `jwks_file` set → `file`.
  9. `jwks_uri` set → `remote`.
  10. No explicit source → existing behaviour (FR-003).

  Also cover the trust-directory string invariants (absolute, cleaned, not `/` → `invalid_trust_directory`) and the `RefreshBounds` defaults:
  - file client assertion and impersonation: `Min` defaults to 15m. An unset `Max` or one below `Min` defaults to `max(3 × Min, 1h)`, saturating at the maximum duration;
  - remote and implicit sources: `Min` defaults to 15m and `Max` to `max(Min, 1h)`;
  - JWT pre-authentication: fixed 5m and 15m;
  - the invariant `0 < Min <= Max`, including an explicit `Max` equal to `Min`.

  Assert that every error message names `AnchorRef.ConfigPath`.
- [X] T045 [P] Write table tests in `internal/domain/trustanchor/local_key_set_test.go` for each Local Key Set invariant, in order:
  1. `len(doc) <= MaxKeySetBytes` → `too_large`.
  2. Top-level JSON object with a `keys` array → `not_a_jwk_set`.
  3. jwx parses the document → `not_a_jwk_set`.
  4. At least one key → `empty_key_set`.
  5. No `oct` key → `symmetric_key`.
  6. No private key material, covering EC and RSA → `private_key`.
  7. A present `alg` is approved → `unapproved_algorithm`.
  8. Every key has a non-empty `kid` → `missing_kid`, with `key at index <zero-based index> requires a non-empty kid`.
  9. `kid` values are unique → `duplicate_kid`, naming the identifier.

  Add these extra cases:
  - malformed JSON, an unknown `kty`, a key without `kid` (rejected), an empty `kid` (rejected), a mixed set with a missing `kid` at index 1 (rejected), and a key without `alg` (accepted);
  - an `unapproved_algorithm` key without `kid`, which reports `unapproved_algorithm` first and names the zero-based index;
  - `KeyIDs` in document order;
  - `DiffKeyIDs` returning added and removed identifiers.

  Assert that no error string contains any substring of the input document or any jwx error text (FR-008).
- [X] T046 Implement `internal/domain/trustanchor/algorithms.go` so that T043 passes. Then delegate `impersonation.IsApprovedAlgorithm` in `internal/domain/impersonation/constants.go` to `trustanchor.IsApprovedAlgorithm`. Keep the existing impersonation error messages, and confirm the existing tests in `internal/domain/impersonation/` pass unchanged (research R6).
- [X] T047 Implement `internal/domain/trustanchor/errors.go` and `internal/domain/trustanchor/local_key_set.go` so that T045 passes. Parse with jwx `jwk.Parse(doc, jwk.WithStrictKeySetParsing(true))`, classify keys with jwx accessors, and never wrap jwx error text. Change the size bound in `internal/adapters/jwks/adapter.go` to `trustanchor.MaxKeySetBytes`, with no behaviour change. Supports FR-007, FR-008, SR-002, and SR-003.
- [X] T048 Implement `internal/domain/trustanchor/source.go` so that T044 passes. Supports FR-001, FR-003, FR-009, FR-010, FR-012, FR-013, FR-022, and FR-023 (lexical).

### Adapters: trust directory, static, file (test-first)

- [X] T049 [P] Write tests in `internal/adapters/jwks/trust_directory_test.go`. Accepted:
  - the Kubernetes `..data` relative-link layout;
  - a plain file.

  Rejected:
  - `../outside.json` → `outside_trust_directory`;
  - a relative link escape → `outside_trust_directory`;
  - an absolute link to a file outside → `outside_trust_directory`;
  - an absolute link to a file inside → `outside_trust_directory` (R4);
  - a subdirectory → `not_regular_file`;
  - a missing file → `not_found`;
  - an unreadable file → `unreadable`, using an injected filesystem error so the test also works as root;
  - a trust directory that is `/`, relative, missing, or a file → `invalid_trust_directory`.

  Also assert:
  - the containment check, the regular-file check, and the read use one descriptor;
  - reads are capped at `MaxKeySetBytes+1`;
  - `Close` releases the `*os.Root`.

  Supports FR-010, FR-022, FR-023, and SR-006.
- [X] T050 Implement `internal/adapters/jwks/trust_directory.go` with the research R4 algorithm so that T049 passes. Resolve the directory with `filepath.EvalSymlinks`, open it once with `os.OpenRoot`, and require `filepath.IsLocal` on the relative name. Open through `root.Open`, `Stat` the handle, and read through the same handle.
- [X] T051 [P] Write tests in `internal/adapters/jwks/static_adapter_test.go`. `GetKeySet` returns the parsed set. `GetKey` finds a known `kid` and returns the existing "not found" error for an unknown one. `HealthState()` is always `healthy`. No goroutine is started. Supports FR-019 and research R2.
- [X] T052 Implement `internal/adapters/jwks/static_adapter.go` so that T051 passes.
- [X] T053 [P] Write tests in `internal/adapters/jwks/file_adapter_test.go` covering the [data-model.md](./data-model.md) state machine with fake clock and re-read intervals:
  - a startup read failure returns the categorized error;
  - Current → Current on a valid re-read; the set is replaced and `trust anchor key set changed` is logged with `added_key_ids` and `removed_key_ids`; unchanged content logs nothing;
  - Current → Degraded on a failed re-read; the last set is kept, the `trust anchor key set re-read failed` Warn log has `error_kind`, `last_success_at`, and `fails_closed_at`, and health is `degraded`;
  - Degraded → Current; `trust anchor key set recovered` is logged and health is `healthy`;
  - Degraded → FailedClosed when `now - lastSuccessAt > Max`; `GetKeySet` returns an error and `trust anchor key set expired` is logged once with `error_kind=stale`;
  - FailedClosed → Current;
  - a partially valid document is never applied;
  - no log contains file contents;
  - `Shutdown` stops the loop.

  Supports FR-011, FR-018, and FR-019.
- [X] T054 Implement `internal/adapters/jwks/file_adapter.go` so that T053 passes. Re-read every `RefreshBounds.Min` through `TrustDirectory`, validate with `trustanchor.ParseLocalKeySet`, and swap atomically. Fail closed after `Max` since the last success, and stop on `Shutdown`.

### Configuration loader and validator (test-first)

- [X] T055 [P] Write loader tests in `internal/config/loader_trust_anchor_test.go`. The three new `BindEnv` keys load from `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS`, `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_FILE`, and `IDENTITY_BROKER_SECURITY_TRUST_MATERIAL_DIR`. A bound multi-line JSON value is accepted without expansion. A YAML `jwks: |` block scalar loads for all three trust anchors. `jwks: ${VAR}` loads a multi-line environment value, and a multi-line YAML value that contains a `${VAR}` reference is rejected, as today. Supports FR-006 and research R8.
- [X] T056 Add the three `BindEnv` calls in `internal/config/loader.go` next to the existing client-assertion bindings so that T055 passes.
- [X] T057 [P] Write validator tests in `internal/config/validator_trust_anchor_test.go`. Structural rules 1–6 run at load time for every in-scope trust anchor, with full config paths such as `oauth2_authorization_server.impersonation.rules[0].trusted_issuers[1].jwks_file`. `security.trust_material_dir` string invariants are enforced. Every `ConfigError` about an inline set uses the value `[inline JWK Set redacted]` and never echoes the document. Supports FR-007, FR-008, FR-009, FR-012, FR-013, FR-022, and research R7.
- [X] T058 Call the `trustanchor` structural rules from `internal/config/validator.go` (`validateTokenExchangeConfig`, the impersonation trusted-issuer loop, and `validateJWTConfig`) so that T057 passes. Use the `[inline JWK Set redacted]` placeholder for inline values.
- [X] T059 Run `just check` and `go test ./internal/domain/trustanchor/... ./internal/domain/impersonation/... ./internal/adapters/jwks/... ./internal/config/...`, and confirm that all of them pass.

**Checkpoint**: The domain rules, adapters, and configuration support are green. User stories can begin.

---

## Phase 3: User Story 1 — Trust a locally signing gateway for token exchange with an inline key set (Priority: P1) 🎯 MVP

**Goal**: The client-assertion trust anchor accepts an inline `jwks`, skips discovery, and verifies token exchange and approval authentication with the inline keys, including FR-015 in proxy and hybrid modes.

**Independent Test**: Local mode with an inline client-assertion set and an unreachable issuer: startup makes 0 key requests, a matching-key assertion exchanges successfully, and an unlisted-key assertion gets `401 invalid_client`. These are US1-S1 to US1-S5 in `tests/e2e/trust_anchor_inline_jwks_test.go`.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [X] T060 [US1] Write builder tests in `internal/app/builder_trust_anchor_test.go`:
  - an inline client-assertion source creates a `StaticAdapter` and never calls `DiscoverOAuth2Endpoints` (FR-004);
  - "client-assertion config unset" now also requires empty `jwks` and `jwks_file`;
  - in proxy and hybrid modes with an empty `issuer_uri`, the expected issuer is the upstream issuer and the adapter is the static one, not the shared upstream adapter (FR-015);
  - the static adapter is never passed to the JWKS publisher or the subject-token policy (FR-016);
  - exactly one `trust anchor key set loaded` Info log is written, with `trust_anchor=token_exchange.client_assertion`, `key_source=inline`, `key_count`, and `key_ids` (FR-018).

### Implementation for User Story 1

- [X] T061 [US1] In `internal/app/builder.go`, resolve the client-assertion `KeySetSource` with `trustanchor.ResolveKeySetSource` before any discovery. Add the `inline` branch to the client-assertion source switch. It parses with `trustanchor.ParseLocalKeySet` and builds `jwks.NewStaticAdapter`. Extend the "unset" condition with `JWKS` and `JWKSFile`. Keep the self-trust check and the approval-auth issuer requirement ahead of source loading (FR-014). Supports FR-001, FR-004, FR-006, and FR-015.
- [X] T062 [US1] Emit the `trust anchor key set loaded` log in `internal/app/builder.go` after the inline source loads, with only the [contracts/configuration.md](./contracts/configuration.md) §3 fields. Supports FR-018 and SR-004.
- [X] T063 [US1] Confirm that machine-facing approval authentication (`POST /api/approvals` with `X-Client-Assertion`) uses the same client-assertion validator built in T061, with no separate change. Cover it with the US1-S4 E2E test, and make no code change unless that test shows a separate path.
- [X] T064 [US1] Run `go test ./internal/app/...` and `ginkgo -v --label-filter="inline-jwks-trust-anchors" --focus="User Story 1" ./tests/e2e/`. US1-S1 to US1-S5 must be green, with no E2E change beyond fixture adjustments.

**Checkpoint**: The MVP is in place. A locally signing gateway works for token exchange and approvals with an inline key set.

---

## Phase 4: User Story 2 — Provide the key set as a file (Priority: P1)

**Goal**: The client-assertion trust anchor accepts a `jwks_file` beneath `security.trust_material_dir`. The file is re-read within the refresh bounds, failures are reported through `trust_anchor_key_files`, and the Helm chart projects Secrets and ConfigMaps into the trust directory.

**Independent Test**: A trust directory with a valid file starts and verifies. A replaced file applies within the bounds. A corrupt file degrades `/health` and recovers. Out-of-directory paths and a missing trust directory fail startup. These are US2-S1 to US2-S8 in `tests/e2e/trust_anchor_key_files_test.go`.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [X] T065 [US2] Write builder tests in `internal/app/builder_trust_anchor_test.go`:
  - a file client-assertion source opens the trust directory once and creates a `FileAdapter`;
  - with no `jwks_file` anywhere, the filesystem is never touched;
  - `App.Shutdown` closes the file adapter and the trust directory;
  - `App.EnduserHealthComponents()` includes `trust_anchor_key_files` only when a file source exists, is `degraded` when any file adapter is degraded, and merges with `upstream_jwks` in every OAuth2 mode.

  Supports FR-011, FR-019, FR-022, and research R11/R12.
- [X] T067 [P] [US2] Add a test to `tests/integration/config_test.go` that loads `examples/config/trust-anchor-local-jwks.yaml` through the broker loader and validator with no error. Supports FR-021.

### Implementation for User Story 2

- [X] T068 [US2] In `internal/app/builder.go`, open the `jwks.TrustDirectory` from `security.trust_material_dir` only when at least one trust anchor declares `jwks_file`, and chain its `Close` into `App.Shutdown`. Add the `file` branch to the client-assertion source switch, building `jwks.NewFileAdapter` with the anchor's `RefreshBounds` and the `token_exchange.client_assertion` config path, and chain its `Shutdown`. Emit `trust anchor key set loaded` with `key_source=file` and `path`. Supports FR-010, FR-011, FR-018, FR-022, and FR-023.
- [X] T069 [US2] Report `trust_anchor_key_files` from `App.EnduserHealthComponents()` in `internal/app/builder.go`, replacing the T041 no-op. Return `degraded` if any registered `FileAdapter.HealthState()` is degraded and `healthy` otherwise, and omit the component when no file adapter exists. Do not change the HTTP status in `internal/adapters/http/health.go`. Supports FR-019 and [contracts/health-endpoint.md](./contracts/health-endpoint.md).
- [X] T070 [US2] Run `go test ./internal/app/... ./tests/integration/...` and `ginkgo -v --label-filter="inline-jwks-trust-anchors" --focus="User Story 2" ./tests/e2e/`. US2-S1 to US2-S8 must be green.

**Checkpoint**: File-based client-assertion keys work with rotation, health reporting, containment, and Helm projection.

---

## Phase 5: User Story 3 — Fail closed on invalid or ambiguous key configuration (Priority: P1)

**Goal**: Every invalid, ambiguous, or unsafe key-source configuration stops startup for every in-scope trust anchor, with an error that names the setting and contains no key material.

**Independent Test**: Each US3 configuration fails `BuildApp` with the [contracts/configuration.md](./contracts/configuration.md) §2 message and no key-member values in the error or logs. These are US3-S1 to US3-S8 in `tests/e2e/trust_anchor_inline_jwks_test.go`.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [X] T071 [US3] Write builder tests in `internal/app/builder_trust_anchor_test.go`. For each of the three trust-anchor kinds and each category, check that `Build()` returns `<path>: <message>`: `conflicting_key_sources`, `refresh_bounds_with_inline`, `too_large`, `not_a_jwk_set`, `empty_key_set`, `symmetric_key`, `private_key`, `unapproved_algorithm`, `missing_kid`, `duplicate_kid`, `relative_path`, `missing_trust_directory`, `invalid_trust_directory`, `outside_trust_directory`, `not_found`, `unreadable`, and `not_regular_file`. Also check:
  - no outbound request or file read happens before resolution fails;
  - the self-trust check fires for inline and file sources;
  - neither the error nor the buffered log contains the base64url `x`, `y`, `n`, or `d` values.

  Supports FR-007, FR-008, FR-009, FR-014, SR-001, and SR-004.

### Implementation for User Story 3

- [X] T072 [US3] In `internal/app/builder.go`, resolve and validate **every** trust anchor's `KeySetSource` before any discovery, adapter creation, or file read, so that no misconfigured anchor triggers I/O. Format builder errors as `<path>: <category description>`, with the source setting (`….jwks` or `….jwks_file`) as `<path>` for key-set rules. Supports FR-007, FR-009, SR-001, and research R12.
- [X] T073 [US3] Check that the error-wrapping paths in `internal/app/builder.go` and `internal/adapters/jwks/` never format a `LocalKeySet`, a document, or a jwx error with `%v` or `%w`. Fix any path that T071 shows leaking. Supports FR-008 and SR-004.
- [X] T074 [US3] Run `go test ./internal/app/...` and `ginkgo -v --label-filter="inline-jwks-trust-anchors" --focus="User Story 3" ./tests/e2e/`. US3-S1 to US3-S8 must be green.

**Checkpoint**: The fail-closed matrix is complete for every trust anchor.

---

## Phase 6: User Story 4 — Use local key sets for impersonation trusted issuers (Priority: P2)

**Goal**: Impersonation trusted issuers accept `jwks` and `jwks_file`. Each entry verifies only against its own declared keys, and the new adapters are shut down.

**Independent Test**: An impersonation rule with an inline trusted issuer mints a token without discovery, and a credential signed by an undeclared key is rejected. These are US4-S1 to US4-S4 in `tests/e2e/impersonation_local_jwks_test.go`.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [X] T075 [US4] Write builder tests in `internal/app/builder_trust_anchor_test.go` for `newImpersonationJWKSFactory`:
  - the sharing key is `{kind, issuer_uri, jwks_uri, inline document, jwks_file, min, max}`, so entries with the same issuer and different local sources get different providers, and identical sources share one provider;
  - inline and file entries skip discovery;
  - each new static or file adapter is chained into `App.Shutdown`;
  - one `trust anchor key set loaded` log is written per distinct local source, with `trust_anchor=oauth2_authorization_server.impersonation.rules[i].trusted_issuers[j]`.

  Supports FR-002, FR-004, FR-017, and FR-018.

### Implementation for User Story 4

- [X] T076 [US4] Extend `newImpersonationJWKSFactory` in `internal/app/builder.go` to dispatch on `KeySetSource.Kind`:
  - `inline` → `jwks.NewStaticAdapter`;
  - `file` → `jwks.NewFileAdapter` through the shared `TrustDirectory`;
  - `remote` → the existing discovery and `jwks.NewJWKSAdapter` path.

  Use the extended sharing key, chain `Shutdown` for each new adapter only, and log per FR-018. Keep the issuer self-trust check at its current position (FR-014). Supports FR-017 and research R10.
- [X] T077 [US4] Confirm that `allowed_algorithms` enforcement in `internal/domain/impersonation/` applies to local sources unchanged. US4-S3 covers this; make no code change unless the test fails. Supports FR-005.
- [X] T078 [US4] Run `go test ./internal/app/... ./internal/domain/impersonation/...` and `ginkgo -v --label-filter="inline-jwks-trust-anchors" --focus="User Story 4" ./tests/e2e/`. US4-S1 to US4-S4 must be green.

**Checkpoint**: Impersonation trusted issuers support local key sets with per-entry isolation.

---

## Phase 7: User Story 5 — Use local key sets for end-user JWT pre-authentication (Priority: P3)

**Goal**: `server.enduser.authentication.jwt` and `server.admin.authentication.jwt` accept `jwks` and `jwks_file` through an injected key set, each as its own trust anchor. `verification: none` stays exclusive with every source.

**Independent Test**: A JWT signed by an inline key gets the principal from `GET /api/me`, and an unknown key is rejected. These are US5-S1 to US5-S4 in `tests/e2e/jwt_preauth_local_jwks_test.go`.

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [X] T079 [P] [US5] Write tests in `internal/adapters/jwtauth/jwx_authenticator_keyset_test.go`:
  - an injected `KeySet` bypasses `jwkfetch` and the startup fetch (no HTTP server is contacted);
  - it verifies with `jwt.WithKeySet(set, jws.WithInferAlgorithmFromKey(true))`, the same options as the remote path;
  - an unknown `kid` returns the existing error;
  - the authenticator does not own the injected adapter; the builder chains the adapter's shutdown into `App.Shutdown` (matching the `KeySet` field contract).

  Supports FR-005 and research R9.
- [X] T080 [P] [US5] Write tests in `internal/config/validator_trust_anchor_test.go` and `internal/app/builder_trust_anchor_test.go`:
  - `server.admin.authentication.jwt.jwks` and `.jwks_file` are accepted and validated with the same structural rules as the end-user server, with errors naming `server.admin.authentication.jwt.…`;
  - `verification: none` with `jwks` or `jwks_file` on either server fails with the `verification_disabled_with_key_source` message;
  - the builder creates a separate adapter for each server, even when both declare identical sources;
  - the builder's defense-in-depth exclusivity check covers all three sources.

  Supports FR-002, FR-013, and research R9.

### Implementation for User Story 5

- [X] T081 [US5] Implement the injected-key-set path in `internal/adapters/jwtauth/jwx_authenticator.go` so that T079 passes. When `KeySet` is set, skip `jwkfetch` creation and the startup `Refresh`, and verify against `KeySet.GetKeySet(ctx)` with the same options as the remote path.
- [X] T082 [US5] In `internal/app/builder.go`:
  - in the existing per-server loop, resolve each server's source (`server.enduser.authentication.jwt`, `server.admin.authentication.jwt`) with fixed JWT pre-authentication `RefreshBounds` (5m/15m);
  - build the static or file adapter, inject it into `jwtauthadapter.JWXAuthenticatorConfig.KeySet`, and chain shutdown;
  - extend the existing `verification: none` defense-in-depth check to `JWKS` and `JWKSFile`;
  - log `trust anchor key set loaded` with `trust_anchor=server.<name>.authentication.jwt`.

  Supports FR-002, FR-013, and FR-018.
- [X] T083 [US5] Make `validateJWTConfig` in `internal/config/validator.go` apply the key-source structural rules for both server prefixes, so that T080 passes. Remove the unused `CategoryUnsupportedTrustAnchor` from `internal/domain/trustanchor/errors.go`. Supports FR-002 and research R9.
- [X] T084 [US5] Run `go test ./internal/adapters/jwtauth/... ./internal/config/... ./internal/app/...` and `ginkgo -v --label-filter="inline-jwks-trust-anchors" --focus="User Story 5" ./tests/e2e/`. US5-S1 to US5-S4 must be green.

**Checkpoint**: All five user stories work independently.

---

## Phase 8: Cross-Cutting Verification

**Purpose**: Success criteria that span the stories.

- [X] T085 Run `ginkgo -v --label-filter="inline-jwks-trust-anchors-parity" ./tests/e2e/`. The SC-003 table must give identical outcomes for remote, inline, and file sources across all five credentials.
- [X] T086 Run the existing token-exchange, approval, impersonation, and JWT pre-authentication E2E suites with `ginkgo -v ./tests/e2e/`. They must pass with no change to any existing test file (SC-005).
- [X] T087 [P] Walk through [quickstart.md](./quickstart.md) Scenarios A–D and §5 against a local binary. Confirm SC-001, SC-002, SC-006, and SC-007, and record any discrepancy as a spec, contract, or code fix in this PR.

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

### Design Phase Verification [MANDATORY]

- [X] T088 Verify that every task traces to an FR, SR, SC, acceptance scenario, or binding constraint, and that no deferred finding from [research.md](./research.md) became a task (Feature Scope).
- [X] T089 Verify that `ARCHITECTURE.md` contains the Key Set Source, Local Key Set, and Trust Directory glossary entries and the updated trust-anchor entries (Principle V).
- [X] T090 Verify that `examples/config/trust-anchor-local-jwks.yaml` exists and loads, and that `examples/config/README.md` references it (Principle VII).
- [X] T091 Verify that `api/enduser/openapi.yaml` documents `trust_anchor_key_files`, and that the PR description records the stakeholder confirmation from T022 (Principles IV and X).
- [X] T092 Verify that the PR description records "no schema change" (T026) and "no UI change" (T027) (Principles IX and XI).
- [X] T093 Verify that `specs/052-inline-jwks-trust-anchors/spec.md` contains the five linked Concrete Example Journeys, with configuration, HTTP exchanges, and decoded JWT claims that match the implemented behaviour and OpenAPI. No Wireloom wireframes are required because the feature has no UI (Principle XIV).
- [X] T094 Verify that `tests/e2e/` holds exactly 29 labelled scenario `It()` blocks plus the parity table, and that T036 recorded a semantic red run before implementation (Principle XIII).

### Implementation Phase Verification [MANDATORY]

**Feature Scope**:

- [X] T095 Verify that the four deferred findings are not implemented, and that any user-approved issue URLs appear only under **Future Work** below.

**API & Documentation** (Principles IV, X):

- [X] T096 [P] Verify that the `/health` response matches [contracts/health-endpoint.md](./contracts/health-endpoint.md) exactly. The component is present only with a file source, has the values `healthy` and `degraded`, and does not change the HTTP status.
- [X] T097 Verify that `docs/api/health.md` documents the `trust_anchor_key_files` component with examples that match `api/enduser/openapi.yaml`, and that `just docs-build` renders the updated Redoc `/health` page (Principle IV).

**Architecture & Documentation** (Principle II):

- [X] T098 Verify that ADR 040 is Accepted, that its decisions match the merged code (Principle II: the final ADR and code must agree before merge), and that it is listed in `ARCHITECTURE.md` and the root `AGENTS.md`.
- [X] T099 [P] Verify that `internal/AGENTS.md`, `internal/domain/AGENTS.md`, and `internal/adapters/AGENTS.md` describe `trustanchor/`, `StaticAdapter`, `FileAdapter`, and `TrustDirectory`.

**Configuration** (Principle VII):

- [X] T100 [P] Verify that all seven keys flow only through `internal/ports/config.go`, `internal/config/loader.go`, and `internal/config/validator.go`, with no ad-hoc config loading.
- [X] T101 Verify that the Helm values, schema, ConfigMap and Deployment templates, and README in `charts/agentic-identity-broker/` cover every chart-managed setting from [contracts/helm-values.md](./contracts/helm-values.md), and that `helm lint` passes.

**Database & Persistence** (Principle IX):

- [X] T102 Verify that no migration, repository, or storage adapter changed (`git diff --stat origin/main -- migrations/ internal/adapters/storage/ internal/ports/storage.go` is empty).

**Security** (Principles I, III):

- [X] T103 Verify fail-closed behaviour. No local-source failure falls back to discovery or another source, and the self-trust check applies to every source (SR-001, SR-005, FR-014).
- [X] T104 [P] Verify that only jwx and `os.Root` are used for key parsing, verification, and containment, with no custom cryptography or path resolution (SR-003, Principle III).
- [X] T105 [P] Verify the redaction guarantee. Grep the E2E and unit test logs to confirm that no error, log line, or `/health` output contains inline content, file contents, or key-member values (FR-008, SR-004).

**Architecture Patterns** (Principle VI):

- [X] T106 Verify hexagonal boundaries:
  - `internal/domain/trustanchor/` has no I/O imports;
  - `internal/adapters/jwtauth/` does not import `internal/adapters/jwks/`;
  - all wiring is in `internal/app/builder.go` (Principles VI and XII).

**Testing** (Principle VIII):

- [X] T107 Verify that unit tests were written first and failed before each implementation (red-green TDD) and changed minimally afterwards. Every new package has branch-level coverage of `trustanchor` and of every `FileAdapter` state transition. No Bash script validates correctness.
- [X] T108 Run `just check`, `just test`, `just test-integration`, and `just verify`, and confirm that all of them are green.

**E2E Acceptance Testing** (Principle XIII):

- [X] T109 Verify that each `It()` maps to exactly one spec scenario, uses `Describe` → `Context` → `It`, cites its scenario ID in a comment, and changed only in fixtures during implementation.
- [X] T110 Run the full E2E suite with `ginkgo -v ./tests/e2e/`. All tests must pass, including the 29 scenarios and the SC-003 parity table.

---

## Dependencies & Execution Order

### Phase dependencies

| Phase | Depends on | Notes |
|---|---|---|
| Phase 1 (Setup) | Nothing | Can start immediately. |
| Phase 2 (Design) | Phase 1 | 2a–2f can run in parallel. T007 (ADR 040 accepted) and T022 (`/health` confirmed) are stakeholder gates. T036 runs after T037–T042. |
| Phase 2.5 (Foundational) | Phase 2 | T037–T042 scaffolding can start once the T029–T034 tests exist. T043 onward needs T007. |
| Phases 3–7 (US1–US5) | Phase 2.5 | — |
| Phase 8 (Cross-cutting) | All five user stories | — |
| Phase N (Compliance) | Phase 8 | — |

### User story dependencies

| Story | Depends on | Notes |
|---|---|---|
| **US1 (P1)** | Phase 2.5 only | The MVP. |
| **US2 (P1)** | Phase 2.5 | T061 and T068 both edit the client-assertion switch, so implement US2 after US1 or rebase onto it. Testable on its own. |
| **US3 (P1)** | Phase 2.5 | T072 hoists resolution for all anchors. Its full matrix asserts file categories (US2) and impersonation and JWT paths (US4, US5), so run it last among the builder changes, or extend T071 as each anchor lands. |
| **US4 (P2)** | Phase 2.5 | Independent of US1 and US2. Its file entries reuse the T068 `TrustDirectory` opening; if US4 runs before US2, move that opening logic into T076. |
| **US5 (P3)** | Phase 2.5 | Independent. Like US4, it reuses the T068 `TrustDirectory` opening for file sources. |

### Within each story

1. Tests (T060, T065, T071, T075, T079, T080) are written first and fail.
2. Builder wiring follows.
3. The story's E2E label run turns green.

### File contention

- `internal/app/builder.go` and `internal/app/builder_test.go` are touched by every story, so story tasks on these files are sequential.
- `tests/e2e/trust_anchor_inline_jwks_test.go` holds both US1 and US3, so T029 and T030 are sequential.

---

## Parallel Execution Examples

### Phase 1

```text
T001 tests/e2e/helpers/local_jwks.go (keys)
T003 tests/e2e/helpers/counting_server.go
```

### Phase 2

```text
T008 ARCHITECTURE.md glossary   |  T013 examples/config/trust-anchor-local-jwks.yaml
T010 AGENTS.md ADR index        |  T016 values.yaml   |  T017 values.schema.json
T031 trust_anchor_key_files_test.go  |  T032 impersonation_local_jwks_test.go
T033 jwt_preauth_local_jwks_test.go  |  T034 trust_anchor_key_source_parity_test.go
```

### Phase 2.5

```text
T043 algorithms_test.go  |  T044 source_test.go  |  T045 local_key_set_test.go
T049 trust_directory_test.go  |  T051 static_adapter_test.go  |  T053 file_adapter_test.go
T055 internal/config/loader_trust_anchor_test.go  |  T057 internal/config/validator_trust_anchor_test.go
```

### User Story 2

```text
T067 tests/integration/config_test.go
```

### User Story 5

```text
T079 internal/adapters/jwtauth/jwx_authenticator_keyset_test.go  |  T080 internal/config/validator_trust_anchor_test.go and internal/app/builder_trust_anchor_test.go
```

---

## Implementation Strategy

### MVP first (User Story 1)

1. Phase 1: Setup (T001–T003).
2. Phase 2: design preconditions, including the stakeholder gates T007 and T022, journey review (T028), and E2E tests written red (T029–T036).
3. Phase 2.5: the foundation (T037–T059).
4. Phase 3: US1 (T060–T064).
5. **Stop and validate.** Inline client-assertion trust works end to end, and a locally signing gateway needs no key-set endpoint (SC-001).

### Incremental delivery

1. Foundation, then **US1** (inline client assertion): the MVP.
2. **US2** (file source, rotation, health, Helm): together with US1, this completes issue 185's two requested alternatives for token exchange.
3. **US3** (fail-closed matrix across all anchors).
4. **US4** (impersonation).
5. **US5** (JWT pre-authentication).
6. Phase 8 and Phase N.

### Parallel team strategy

- During Phase 2, the docs and ADR work (2a), the configuration and Helm work (2b), and the E2E test files (2f) can proceed in parallel.
- After Phase 2.5, US4 (impersonation factory) and US5 (jwtauth adapter) touch mostly separate code. Their builder edits must be serialized.

---

## Future Work

None yet. [research.md](./research.md) "Deferred findings" lists four items outside this feature's scope. Record issue URLs here only after the user approves filing them.

## Notes

- `[P]` means the task touches different files and has no dependency on an incomplete task.
- Commit after each task or logical group. Keep the ADR and docs commits separate from code commits.
- The ADR 040 decisions and the code MUST agree before merge. If implementation changes a decision, revise ADR 040 in this PR (Principle II).
