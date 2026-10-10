# Implementation Plan: Explicit OAuth2 Credential Sources

**Branch**: `051-oauth2-secret-file-overrides` | **Date**: 2026-10-06 | **Spec**: [spec.md](./spec.md)

**Propagated**: 2026-10-07 — Clarified paired inputs versus admin representation; moved API compatibility and Helm work into blocking gates; made shared regressions test-first; defined identity checks across both source transitions.

**Propagated**: 2026-10-07 — Define pair acquisition, target-identity validation, concurrent-publication regressions, and filesystem-only administrative credential omission.

**Input**: Feature specification from `specs/051-oauth2-secret-file-overrides/spec.md`

## Summary

Eligible third-party OAuth2 services implement explicit `credential_source: stored | filesystem`. Stored mode uses existing inline credentials and ignores bindings. Filesystem mode uses operator-configured client-ID and secret files without stored placeholders. Missing bindings or unusable required files fail closed before provider requests.

~~Preserve registration, public APIs, database schema, and filesystem-independent authorization initiation.~~ Superseded by the explicit source selector and file-backed client ID. Registration, administrative representation, storage, and authorization/session identity association must change.

Keep source selection in `OAuth2SessionService` and registration invariants in the existing third-party service domain. Put bounded filesystem access behind one port and adapter. Preserve stored-secret encryption, provider authentication style, generic callback/refresh errors, and excluded modes.

The immediate deployment binds `zalando-platform` to `/meta/credentials/employee-client-id` and `/meta/credentials/employee-client-secret`. It retains the read-only `agentic-platform-credentials` mount. The registered service explicitly selects filesystem mode with neither inline credential.

Each binding contains both `client_id_file` and `client_secret_file`. Omitting inline fields from filesystem administrative responses does not omit either configured path or the client ID sent to providers. Administrative reads report the source without resolving files, so outages do not make metadata unavailable.

## Technical Context

**Language/Version**: Go 1.27.1. YAML configuration and Helm templates.

**Primary Dependencies**: Existing `golang.org/x/oauth2` v0.37.0, Viper v1.21.0, Cobra v1.10.2, mapstructure v2.5.0, godotenv, chi v5, `golang.org/x/sys/unix`, Ginkgo v2, Gomega, and testify. Case-preserving YAML decoding uses the already-resolved `go.yaml.in/yaml/v3` dependency when necessary. No provider SDK or cryptographic dependency is added.

**Storage**: Extend the service model and both PostgreSQL and in-memory adapters with the source selector and source-specific credential invariants. Migrate existing records without changing authentication behavior. Persist the non-secret client-identity association needed by authorizations and sessions and a service-owned source-transition marker, never file secrets or paths. Preserve token/session encryption and normal persistence.

**Testing**: Configuration, filesystem, registration, source-transition, identity-continuity, and persistence tests, existing chart integration tests, and 50 Ginkgo production-bootstrap acceptance scenarios. Normal HTTP journeys run in process. Privilege and blocking-file cases re-execute the current Go test executable. T012 reviewed the actual consumers on 2026-10-08: admin DTOs/handlers, administrative test callers, and operator documentation. No administrative management UI/client exists in the reviewed SPA. Consent visuals and types remain unchanged.

**Target Platform**: Linux broker deployment, including the existing Kubernetes non-root identity. Darwin remains supported for development. The new file adapter uses Unix non-blocking flags for these platforms.

**Project Type**: Backend, administrative API/consumer, persistence, and deployment feature in the Go/React monorepo, plus coordinated external deployment inputs. No consent visual redesign.

**Performance Goals**: One bounded fresh client-ID read per filesystem authorization initiation. Each filesystem exchange/refresh attempt acquires both values once and revalidates their target identities. Stored and excluded services perform no credential-file reads. No background work, cache, retained descriptor, or rotation-related service-record write. No numerical latency target is specified.

**Constraints**:

- Bindings match canonical IDs exactly, including case. UUIDs remain the identifiers in events, storage, and encryption contexts.
- Startup validates mapping syntax, canonical IDs, and both absolute paths in each pair, but not credential-file availability or service registration.
- The opened target must be a regular file of at most 65,536 bytes. Non-regular sources must not block the broker.
- Surrounding whitespace is removed. Internal characters remain unchanged.
- Filesystem mode is mandatory even without a binding. There is no stored-credential or last-known-value fallback.
- Public clients, CIMD confidential clients, and Google-flavor services ignore bindings and reject filesystem registration or updates.
- Administration, consent, and session metadata do not access credential files. Authorization initiation reads only the client-ID file.
- Client-facing source errors retain generic callback, refresh, and authorization-initiation internal-error responses, without file detail.
- The mapping does not hot reload. Complete file publication takes effect on the next operation.
- Pair acquisition opens both targets, reads bounded values, and revalidates both configured paths against the opened descriptors. Detected changes stop authentication.
- Providers publish changed values as fresh immutable targets. They do not modify published targets or republish retired targets during acquisition.
- Publication after validated acquisition can leave an in-flight operation using that pair. Later acquisitions use current targets. No acquisition retry is added.
- Credential delivery and old/new validity overlap remain operator responsibilities. There is no uninterrupted-rotation guarantee.
- Filesystem creation and updates reject inline credentials. A valid canonical ID is mandatory.
- Stored-to-filesystem transitions remove both stored credentials atomically. Filesystem-to-stored transitions require explicit inline credentials. Both preserve established client identities and atomically set a durable, internal source-transition marker.
- Omitted source on creation selects stored mode. Omitted source on update preserves the current mode. Unknown source values are invalid.
- New authorizations in either eligible mode retain the selected client ID. Existing codes and sessions compare it before authentication in either mode, including source transitions. Legacy missing identities remain usable only for stored services that have never transitioned; returning to stored cannot restore that exception.

**Scale/Scope**: One optional pair mapping, one reader port and filesystem adapter, three operation boundaries, an explicit persisted source, seven user stories, and 50 acceptance scenarios. The immediate deployment uses one pair binding in each of three environment inputs. Multiple bindings remain independent.

**Approval status, 2026-10-08**: The user explicitly selected “Accept ADR 038” and “Major contract bump to 2.0.0”. ADR 038 is Accepted. Published `v0.1.81` contains Admin API `1.0.0`, whose required response `client_id` makes filesystem omission a breaking change. Admin API `2.0.0` is implemented on existing `/api/services` routes without a compatibility endpoint or shim. T010 defined the source-aware schemas and breaking-change record before behavior changes. Historical approval and semantic-red evidence remain in the [quickstart](./quickstart.md#status-and-prerequisites).

**Implementation status, 2026-10-08**: T001–T069 are complete. All 50 acceptance cases passed in individual story runs. Selected unit/integration packages and 30 legacy public/CIMD journeys passed. The full final gate remains pending. External test, prod, and sandbox manifests regenerated from the current local chart, including uncommitted feature changes.

The local linux/arm64 release image was built and verified with UID 1000. CDP `DEP_BROKER_VERSION` placeholders remain. No registry publication or live deployment occurred. [Deployment evidence](./contracts/deployment.md#deployment-repository-inputs-and-outputs) records the image and historical chart revision.

## Constitution Check

*The planning prerequisites and historical test-first evidence preceded implementation. Current statuses record completed feature work, not a completed full final gate.*

### Pre-research gate

| Design precondition | Status | Design or implementation commitment |
|---|---|---|
| Domain model | Implemented | The service owns source selection and irreversible transition history. The session domain owns pair acquisition and identity enforcement. Both stores and existing migration 036 preserve the model. |
| Domain concepts | Documented, T005 | Architecture/glossary define source ownership, bindings, operation credentials, identity association, and safe source errors under accepted ADR 038. |
| Typed entity IDs | Not applicable | No new UUID entity. Reuse `id.ServiceID` for existing services. |
| Configuration design | Implemented | The existing loader strictly decodes paired inputs, preserves exact keys and whole-map precedence, accepts explicit empty mappings, and defers file availability to use. |
| Configuration examples | Indexed, T008 | YAML/default/single-pair and README multi-pair/environment/CLI examples are current. |
| Helm deployment contract | Implemented and verified, T064/T065 | Typed tests passed after initial semantic red. Values, schema, ConfigMap template, and README changed together. Actual target rendering preserves credential/configuration/tmp mounts and non-root settings. |
| API design first | Approved design | T010 defines OpenAPI 3.0 source-specific create/update and response variants for Admin API 2.0.0. The user approved the breaking contract on existing routes. Release v0.1.81 establishes the released 1.0.0 baseline. `docs/changelog.md` records required consumer migration. FR-020 preserves generic operational errors. |
| OpenAPI documentation | Implemented | `api/admin/openapi.yaml`, handlers, and repository administrative consumers use Admin API 2.0.0. Unrelated and end-user contracts remain unchanged. External consumers must migrate before deployment. |
| API confirmation | Approved on 2026-10-08 | The 2026-10-07 clarification requests source-specific registration and representation. Explicit acceptance of ADR 038 and the 2.0.0 major contract bump confirms the concrete direction. |
| Database design | Implemented under T011 design | Migration 036 and both stores preserve source-aware constraints, atomic marker/credential changes, nullable session identity, and guarded rollback. The exact design preceded persistence implementation. |
| E2E acceptance tests | Authored before implementation; green by story | All 50 feature-labeled journeys existed before chart or broker behavior and passed in individual story runs. |
| E2E mapping | Complete | Exactly one feature scenario runs per numbered identifier across all seven stories. |
| E2E red phase | Observed, T064/T021 | Initial and post-chart runs selected 50 cases: five stored/excluded controls passed and 45 missing-feature expectations failed. No selected feature case was skipped or pending. |
| Frontend Playwright and screenshots | Verified not applicable, T012 | The reviewed SPA has consent, session, and approval routes, not administrative service-management routes. Its consent API/types do not consume the admin service representation. No screen, consent DTO, styling, or screenshot change is required. |

### Administrative consumer inventory (2026-10-08)

| Consumer | Evidence | Feature impact |
|---|---|---|
| Admin DTOs/handler | `internal/adapters/http/handlers/admin/services_handler.go` defines presence-aware service requests and source-specific responses. | Implemented source-aware parsing and projection through the domain. |
| Existing administrative E2E callers | `tests/e2e/oauth2_provider_flavor_test.go` sends service CRUD requests and reads map-based stored credentials. | Stored cases remain valid. Affected source-aware assertions are current. |
| Operator service-management guide | `docs/guides/manage-agents-and-services.md` documents service registration and response variants. | Includes filesystem registration, field omission, and explicit transitions. |
| SPA routes | `web/src/App.tsx:48-74` contains delegations, agents, sessions, and approvals routes. | No administrative management screen. No route or visual changes. |
| Consent API client | `web/src/services/api/consent.ts:33-257` calls `/me` and `/consent/agents` endpoints. The API directory contains consent, sessions, and approvals clients, not an admin client. | Keep the end-user client unchanged. |
| Consent service types | `web/src/types/consent.ts:91-118` defines service IDs, display metadata, requirements, and scopes without administrative inline credentials. | Keep consent DTOs unchanged. |

Phase 2e is complete for the current scope. Frontend design, Playwright journeys, screenshots, and design-token changes are not applicable. T078 rechecks this boundary after implementation. If UI scope changes, design and acceptance tasks must change first.

### Implementation considerations

| Principle or check | Status | Approach |
|---|---|---|
| I: Security-first | Pass | Stored default preserves existing behavior. Explicit filesystem selection is mandatory and fails closed, including missing bindings and client-identity mismatch. |
| II: Architecture and ADRs | Approved design | Accepted ADR `038-oauth2-client-secret-file-overrides.md` defines the source boundary and narrowly supersedes ADR 036's stored-secret invariant for filesystem services. Public/CIMD protocol behavior and encryption boundaries remain binding. |
| III: Library-first security | Pass | Use existing OAuth2, encryption, and standard filesystem libraries. Add no custom crypto. |
| IV and X: API transparency and approval | Implemented after approval | The user approved Admin API 2.0.0 on existing routes. T010 defined schemas and breaking-change documentation before handlers or consumers changed. Provider rejection and generic operational errors remain unchanged. |
| V: Domain model | Pass | The service owns source choice. Registration enforces source-specific invariants. Bindings remain operator configuration, not persisted paths. |
| VI: Hexagonal architecture | Pass | Domain selects the source through a port. The filesystem adapter owns OS access. Builder owns wiring. |
| VII: Configuration | Pass | Decode the structured field inside the existing loader. Preserve source precedence and key case. Extend the chart and operator documentation. |
| VIII and XIII: TDD and acceptance | Historical red and story green observed | All 50 acceptance journeys preceded chart or broker behavior. Shared configuration, rotation, mismatch, exclusion, transition, and metadata regressions preceded their implementations. Story runs passed the unchanged expectations. The full final gate remains pending. |
| IX: Persistence | Implemented | Both stores and migration 036 enforce source-specific persistence. Transitions atomically remove obsolete credentials. File use and rotation never write service credentials. |
| XI: Design system | Not applicable to current scope | No management UI or consent visual change occurred. End-user clients and DTOs remain unchanged. |
| XII: Builder wiring | Pass | Construct and inject the reader only through `internal/app/builder.go`. Do not instantiate it in handlers or routing. |
| Zalando API/Event guidelines | Pass | Reuse current HTTP envelopes and credential-free structured events. |
| End-user API rendering | Intentionally unchanged | Operational responses remain unchanged. Administrative API, source-specific registration, and operator documentation change. |
| Migration testing and persistence patterns | Implemented regression coverage | Existing persistence patterns cover backfill, both adapters, source invariants, transitions, nullable client identity, and guarded rollback. |

### Post-design re-check

The refined design has no identified constitutional exception. Accepted ADR 038 and the approved API version strategy govern the implemented behavior. Design prerequisites, the database design, the test-first chart contract, and semantic-red acceptance evidence preceded production changes. The full final gate remains pending.

- The existing service gains a source field without a new aggregate or UUID type.
- Configuration retains exact canonical IDs, pair objects, and whole-map precedence.
- Each file read bounds bytes and rejects non-regular targets without blocking.
- Administrative source selection is explicit. Operational source failures preserve generic responses.
- Authorization and session contexts retain the established non-secret client identity. Both source modes enforce it after transitions. A durable transition marker prevents legacy missing-identity contexts from becoming usable again after a return to stored.
- The test mapping covers all 50 scenarios, including registration, transitions, excluded modes, and deployment inputs.
- Prerequisites include refreshed optional artifacts, accepted ADRs, architecture/glossary, OpenAPI, migrations, examples, chart contract, and test-first evidence.

## Project Structure

### Documentation for this feature

```text
specs/051-oauth2-secret-file-overrides/
├── plan.md
├── tasks.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── configuration.md
    ├── outbound-authentication.md
    └── deployment.md
```

[`tasks.md`](./tasks.md) contains 80 dependency-ordered tasks generated on 2026-10-07. Supporting documents, accepted ADR 038, confirmed API 2.0.0 schemas, database design, consumer review, all 50 compiled acceptance expectations, initial semantic red, and the actual test-first Helm contract complete Phase 2. T021 records post-chart broker semantic red. Foundational broker implementation starts next.

### Source and documentation affected during implementation

```text
cmd/agentic-identity-broker/root.go             # Register the JSON-object CLI flag
internal/config/                              # Case-preserving source decode and startup validation
internal/ports/config.go                      # ThirdPartyOAuth2Config.CredentialFiles pair mapping
internal/ports/storage.go                     # Existing repository contracts, only if required
internal/ports/credential_file.go              # Implemented client-ID and coherent-pair reader contract
internal/domain/model/thirdparty_oauth2_provider.go # Source and credential invariants
internal/domain/model/credential_source_error.go # Implemented safe source-error category
internal/domain/thirdparty/                    # Registration, updates, and atomic source transitions
internal/domain/oauth2session/                 # Three operation boundaries and client-identity association
internal/domain/tokenexchange/                 # Source-failure telemetry classification
internal/adapters/credentialfile/              # Bounded filesystem reader under accepted ADR 038
internal/adapters/storage/memory/              # Source-specific service and session persistence
internal/adapters/storage/postgres/            # Source-specific records, queries, and session persistence
internal/adapters/http/                        # Administrative parsing and source-aware representations
migrations/                                   # Source/backfill and required identity-association migrations
api/admin/openapi.yaml                        # Selector, credential omission, and transitions
web/src/                                      # Consumer inventory review only; no UI changes
internal/app/builder.go                        # Reader construction and injection
charts/agentic-identity-broker/
├── values.yaml
├── values.schema.json
├── templates/configmap.yaml
└── README.md
examples/config/third-party-oauth2.yaml
examples/config/README.md
docs/configuration.md
docs/changelog.md                             # Required Admin API 2.0.0 breaking-change and migration record
ARCHITECTURE.md
adrs/038-oauth2-client-secret-file-overrides.md  # Accepted on 2026-10-08
justfile                                      # CLI-process E2E build prerequisite when required
tests/e2e/
├── oauth2_secret_file_selection_test.go
├── oauth2_secret_file_rotation_test.go
├── oauth2_secret_file_failures_test.go
├── oauth2_secret_file_boundaries_test.go
├── oauth2_secret_file_configuration_test.go
├── oauth2_secret_file_deployment_test.go
├── bootstrap/                                # Thin production-binary wrapper when needed
├── fixtures/                                 # Synthetic services, config, chart values
└── helpers/                                  # Reused provider and journey utilities
tests/integration/helm_chart_test.go            # Existing chart boundary checks
```

External deployment repository:

```text
deploy/values/{test,prod,sandbox}.yaml          # Explicit canonical-ID mapping
README.md                                     # Mount, binding, and rotation limits
scripts/update-k8s-manifests.sh                # Existing generator, used unchanged
scripts/chart_ref                             # Generated chart provenance
deploy/kubernetes/{test,prod,sandbox}/          # Regenerated outputs
```

**Structure Decision:** Extend the existing third-party and session bounded contexts. Add one focused driven adapter, not a credential-provider framework. Extend current storage and HTTP adapters without bypassing domain services. Keep deployment generation in its owning repository. Consent visuals, unrelated routing/contracts, and standalone ExtProc configuration remain unchanged.

## Design and Implementation Approach

### 1. Decode and validate configuration

~~Add `ClientSecretFiles map[string]string` and `--third_party_oauth2.client_secret_files` for secret-only bindings.~~ Removed because filesystem mode requires a pair.

Add `CredentialFiles` to `ports.ThirdPartyOAuth2Config` with `client_id_file` and `client_secret_file` in each canonical-ID binding. Resolve `third_party_oauth2.credential_files` inside the existing loader in CLI, environment, YAML, default order. Preserve exact YAML keys and whole-map replacement. Decode JSON-string sources strictly and keep `{}` as an explicit winning empty mapping.

Reuse `canonical.Validate()` and absolute-path checks. Reject malformed mappings, non-object pairs, missing fields, non-string paths, empty/whitespace-only paths, and relative paths before listeners serve requests. Do not stat files, query registration, or reinterpret keys as UUIDs.

Register `--third_party_oauth2.credential_files` and `IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES` through the existing Cobra/Viper system. Diagnostics must not dump the mapping or resolve credentials. Update redaction for the renamed key rather than relying on `SECRET` in the old name. Keep unrelated decoding unchanged.

### 2. Enforce the file boundary

The focused `CredentialFileReader` port exposes `ReadClientID(path string) (string, error)` and `ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)`. Initiation uses `ReadClientID` and never accesses the secret path. Exchange and refresh use one `ReadPair` call, not two independent value reads.

`ReadPair` opens both current targets with non-blocking read-only flags before it reads either value. Each descriptor must identify a regular file within the per-file size limit. After both bounded reads, the adapter revalidates each configured path against its opened descriptor with `os.SameFile`. A target mismatch returns `generation_changed`. An unavailable target returns its applicable safe source error. Both descriptors remain open through validation and close before return, including error paths. Errors return neither credential value.

Pair coherence depends on the spec's immutable-target publication contract. Providers publish changed values as fresh targets and never republish retired targets during acquisition. No common-parent path restriction or provider-specific generation identifier is added. Detected changes abort before authentication without an acquisition retry. After validated acquisition, a later publication can leave that operation using its validated pair. Provider validity and established client-identity checks still apply. Subsequent acquisitions reopen current targets.

Source-error reasons are bounded identifiers. They include missing/unavailable, permission denial, non-regular target, oversize, empty value, `generation_changed`, and other read failure. Raw file errors and contents do not enter printable error chains.

### 3. Select credentials at all three operation boundaries

Carry the immutable pair mapping in session configuration. Inject the reader through Builder using the existing dependency-injection style.

~~Binding presence selects a file secret, and `buildOAuth2Config()` never reads files.~~ Removed because the registered source selects filesystem mode and authorization initiation needs the current file client ID.

Use one source-selection path for eligibility, explicit service source, and exact canonical-ID lookup. Stored and excluded modes preserve their behavior without file access. Filesystem mode requires a canonical ID and matching pair, even with an empty mapping.

At authorization initiation, filesystem mode calls `ReadClientID` for the redirect. Stored mode uses its stored client ID without file access. Seal the selected non-secret identity for both eligible modes in the existing authorization context. Never read the secret merely to build a redirect, consent view, or metadata response.

In `exchangeCodeWithRetry()`, filesystem mode calls `ReadPair` once before each broker-owned attempt. Stored mode selects stored credentials without file access. Compare the acquired client ID with the sealed authorization identity before sending the code. Stop on source error or mismatch, including `generation_changed`. Do not retry source acquisition within that attempt. Keep existing OAuth2 auth-style negotiation and provider retry policy.

In `RefreshAccessToken()`, filesystem mode calls `ReadPair` once for the actual refresh attempt. Stored mode selects stored credentials without file access. Compare the selected client ID with the session's established identity before provider authentication. A mismatch or acquisition error stops before a provider request. Preserve locking, singleflight, and excluded-mode paths.

Persist the established non-secret identity with each new eligible stored or filesystem session after successful exchange. Legacy missing-identity contexts retain existing behavior only for never-transitioned stored services. Excluded modes remain unchanged. Never persist file secrets or assign file values to the registered service.

### 3a. Persist source selection and enforce registration invariants

Extend the existing service entity and third-party domain service with the `stored | filesystem` choice. Reject unknown values. Creation defaults to stored. Update omission preserves the current choice.

Stored confidential mode retains existing non-empty inline credential validation. Filesystem mode requires a canonical ID and rejects either inline credential, including placeholders. Reject filesystem mode for public, CIMD confidential, or Google-flavor services without reading files or mutating records.

Switch stored to filesystem in one atomic update that removes both stored credentials. Require explicitly supplied non-empty credentials for the reverse transition. Set the internal `credential_source_transitioned` marker to true in the same update whenever the source changes; once true, it never resets. Creation and legacy backfill initialize it to false. No-op source selections and metadata updates preserve it. Never import file contents. Metadata updates and binding removal preserve the source. Reject canonical-ID clearing in filesystem mode.

Both storage adapters and migration 036 implement the source-specific absent-credential representation and nullable session identity. Legacy services backfill as stored without changing excluded-mode behavior or encrypted credentials. Existing canonical-ID uniqueness and concurrency semantics remain unchanged. T011 defined persistence before implementation.

Source transitions never rewrite established code/session identities. Recorded identities are checked in both stored and filesystem modes; matching identities may continue subject to provider validity, while mismatches require a new connection before authentication. A legacy missing identity is allowed only when the service is stored and `credential_source_transitioned` is false. Otherwise fail closed, including a stored-to-filesystem-to-stored round trip. Do not infer an identity from current credentials. Excluded modes retain existing behavior.

The implemented administrative create/update/read contracts appear in `api/admin/openapi.yaml`, with `info.version: 2.0.0`. Responses report the source. Filesystem responses omit both inline fields, paths, and internal transition evidence. Stored responses retain their client ID and applicable `REDACTED` secret. The user approved the major contract bump on existing routes on 2026-10-08. Published `v0.1.81` remains the released `1.0.0` baseline.

Repository management consumers use the new schema without resolving files. `docs/changelog.md` records the unreleased breaking change and external consumer migration.

### 4. Preserve errors and add safe observability

Source errors bypass provider-rejection wrapping in `refreshSessionTokens()` and code-exchange PKCE/provider classification. Preserve `callback_failed`, session-refresh `500 internal_error`, and token-exchange `500 server_error`. Authorization-initiation source failures use its generic internal-error response without file detail. Provider rejection keeps its existing responses.

Emit filesystem-selection and source-failure events with service UUID, operation, and non-secret outcome/reason across initiation, exchange, and refresh. Include missing-binding, identity-mismatch, and `generation_changed` outcomes in the safe source category. Classify provider rejection separately. Extend existing token-exchange telemetry without adding an API error field. Never log file values, raw OS errors, paths, or secret-bearing requests.

### 5. Extend chart and deployment inputs

Render non-empty `broker.thirdPartyOauth2.credentialFiles` as `third_party_oauth2.credential_files`. Complete the actual values, schema, templates, and README contract through T064/T065 in Phase 2b before broker production code. T064 runs the already-authored acceptance suite for initial semantic-red evidence and adds focused Helm red tests before T065. Validate canonical-ID pair objects with both string paths. Default values render no binding. US7 later validates delivery and regeneration, not first-time chart implementation.

Update all three deployment inputs with both target paths. Explicitly configure the registered `zalando-platform` service as filesystem, without stored placeholders. Preserve credential, configuration, and temporary-storage mounts. Regenerate outputs through the existing script with a feature-capable chart and broker image.

Document non-root readability, read-only directory mounts without credential-file `subPath`, and complete pair publication through fresh immutable targets. Retain required validity overlap. Document `generation_changed` acquisition failures and permitted use of an already validated pair after publication. Secret rotation can preserve sessions with the same client ID. A client-ID change requires new connections, not identity migration or continuity guarantees.

## Implementation Phase Overview

| Phase | Purpose | Applies |
|---|---|---|
| Phase 0: Pre-implementation refactoring | Separate behavior-neutral restructuring | Skip. No broad restructure is necessary. |
| Phase 1: Setup | Reuse provider, logging, and bootstrap patterns | Include only the fixture and process support needed by these scenarios. No new framework. |
| Phase 2: Design Preconditions | ADR, glossary, refreshed contracts, confirmed API compatibility/versioning, DB migration design, examples, all acceptance tests, and test-first actual Helm contract | Mandatory. All acceptance expectations precede chart changes; the completed chart contract and T021 gate precede broker production code. |
| Phase 2.5: Foundational Infrastructure | Reader port, safe error category, adapter, and Builder wiring | Include after focused tests establish their required behavior. |
| Phase 2.7: Entity Boilerplate | New CRUD entity scaffolding | Skip. No new persistent entity or CRUD routes. |
| Phase 3+: User Stories | Explicit selection/registration, rotation/identity continuity, failures, exclusions, source transitions/metadata, configuration, and deployment | Include all seven stories. |
| Phase N: Constitution Compliance verification | Requirements, boundaries, tests, smoke scenarios, docs, and release changelog | Mandatory. |

Phase 2a defines source, credential lifecycle, identity association, and durable transition evidence. Phase 2b updates pair configuration and the actual Helm contract; T064/T065 wait for Phase 2f expectations and initial semantic-red evidence before changing templates. Phases 2c and 2d define and confirm administrative API compatibility/versioning, consumer changes, database migrations, and source invariants. Phase 2e validates the administrative consumer inventory and records frontend non-applicability. Phase 2f establishes all 50 acceptance tests. T021 closes the design gate only after the Helm contract is complete.

Detailed execution prerequisites, story checkpoints, and parallel examples are in [`tasks.md`](./tasks.md).

## Testing Strategy

### End-to-end acceptance tests

**Framework**: Ginkgo/Gomega in `tests/e2e/`, with `Label("oauth2-secret-file-overrides")` on all 50 feature scenarios. Add a nearby `USx-Sy from specs/051-oauth2-secret-file-overrides/spec.md` comment for every `It()`.

Use production `app.Builder` through the existing bootstrap wrappers. Use separate admin and end-user servers from the same application. Fresh storage, file directories, log captures, and provider state isolate each scenario. Do not depend on execution order.

The files in this table contain the implemented one-to-one acceptance mapping. All 50 cases passed in individual story runs. This does not claim a completed combined final gate.

| Spec scenario | E2E file under `tests/e2e/` | Acceptance behavior |
|---|---|---|
| US1-S1 | `oauth2_secret_file_selection_test.go` | Stored connect and refresh use stored client ID and secret without file reads. |
| US1-S2 | `oauth2_secret_file_selection_test.go` | Filesystem initiation uses the file client ID. Exchange uses that ID and the file secret without placeholders. |
| US1-S3 | `oauth2_secret_file_selection_test.go` | Refresh uses current file credentials for the session's established client identity. |
| US1-S4 | `oauth2_secret_file_selection_test.go` | A stored service ignores even its own configured binding on connect and refresh. |
| US1-S5 | `oauth2_secret_file_selection_test.go` | Only the exact canonical ID selects the pair despite unrelated names, client labels, filenames, and UUID. |
| US1-S6 | `oauth2_secret_file_selection_test.go` | Two filesystem services use only their own pairs during connect and refresh. |
| US1-S7 | `oauth2_secret_file_selection_test.go` | Filesystem registration without inline credentials succeeds. Admin reads report source, omit credential fields, and persist neither file value. |
| US1-S8 | `oauth2_secret_file_selection_test.go` | Missing binding, including an empty mapping, stops initiation and renewal before provider requests without selecting stored mode. |
| US1-S9 | `oauth2_secret_file_selection_test.go` | Filesystem registration/update rejects either inline credential without file access or service mutation. |
| US2-S1 | `oauth2_secret_file_rotation_test.go` | Secret replacement for the same client ID reaches the next exchange and refresh without restart, registration update, or service writes. |
| US2-S2 | `oauth2_secret_file_rotation_test.go` | Changing one pair affects only its service's new connections. |
| US2-S3 | `oauth2_secret_file_rotation_test.go` | A projected-volume generation change reaches the next operation. Provider requests contain coherent pairs, never mixed-generation values. |
| US2-S4 | `oauth2_secret_file_rotation_test.go` | Both files lose surrounding whitespace while internal characters remain intact during connect and refresh. |
| US2-S5 | `oauth2_secret_file_rotation_test.go` | New authorization uses client B after a change. Codes and sessions established as A stop before authentication as B. |
| US3-S1 | `oauth2_secret_file_failures_test.go` | Each missing required file independently stops affected operations before provider requests without fallback. |
| US3-S2 | `oauth2_secret_file_failures_test.go` | Each permission-denied file independently stops affected operations under a non-root broker. |
| US3-S3 | `oauth2_secret_file_failures_test.go` | Each empty required file independently stops affected operations without fallback. |
| US3-S4 | `oauth2_secret_file_failures_test.go` | Each whitespace-only required file independently stops affected operations without fallback. |
| US3-S5 | `oauth2_secret_file_failures_test.go` | Each broken symlink independently stops affected operations without fallback. |
| US3-S6 | `oauth2_secret_file_failures_test.go` | Source loss after success never reuses the previous value. |
| US3-S7 | `oauth2_secret_file_failures_test.go` | A fresh broker instance fails closed before its first successful source read. |
| US3-S8 | `oauth2_secret_file_failures_test.go` | Source restoration fixes the next operation without restart. |
| US3-S9 | `oauth2_secret_file_failures_test.go` | Source errors and provider rejection have distinct events/categories and unchanged HTTP responses. |
| US3-S10 | `oauth2_secret_file_failures_test.go` | Each directory target independently stops affected operations before provider requests. |
| US3-S11 | `oauth2_secret_file_failures_test.go` | Each FIFO, socket, or device target independently fails promptly before provider requests. |
| US3-S12 | `oauth2_secret_file_failures_test.go` | Each file larger than 65,536 bytes independently stops affected operations. |
| US4-S1 | `oauth2_secret_file_boundaries_test.go` | Public connect and refresh ignore the unavailable binding and send no secret. |
| US4-S2 | `oauth2_secret_file_boundaries_test.go` | CIMD connect and refresh retain valid signed assertions and ignore the binding. |
| US4-S3 | `oauth2_secret_file_boundaries_test.go` | A real Google-flavor fixture retains existing stored-credential behavior. |
| US4-S4 | `oauth2_secret_file_boundaries_test.go` | Excluded-mode filesystem registration/update is invalid, reads no files, and preserves existing records. |
| US5-S1 | `oauth2_secret_file_boundaries_test.go` | Admin/persisted views report filesystem after exchange and refresh, contain no file values, and expose no inline fields or paths. |
| US5-S2 | `oauth2_secret_file_boundaries_test.go` | Admin read/update work during source loss without file access. Unrelated updates preserve source. |
| US5-S3 | `oauth2_secret_file_boundaries_test.go` | Consent/session metadata survive outages. Initiation needs only a usable client-ID file, never the secret. |
| US5-S4 | `oauth2_secret_file_boundaries_test.go` | File-value markers appear in no event, error, log, diagnostic, or admin/consent output. Administrative reads report source without file access. |
| US5-S5 | `oauth2_secret_file_boundaries_test.go` | Stored-to-filesystem update atomically removes credentials and records the transition. Old codes/sessions continue only with a matching recorded identity; different/missing identities stop before authentication. Binding removal never restores stored mode. |
| US5-S6 | `oauth2_secret_file_boundaries_test.go` | Filesystem-to-stored update requires explicit credentials and imports no file values. Old codes/sessions continue only with a matching recorded identity; different/missing identities stop, including legacy contexts after a round trip. |
| US5-S7 | `oauth2_secret_file_boundaries_test.go` | Omitted source on unrelated stored updates preserves stored mode, client ID, and existing secret redaction. |
| US5-S8 | `oauth2_secret_file_boundaries_test.go` | Upgrade preserves legacy authentication and marks eligible confidential records stored without implicit filesystem selection. |
| US6-S1 | `oauth2_secret_file_configuration_test.go` | YAML pair configuration and explicit filesystem registration support a complete connection. |
| US6-S2 | `oauth2_secret_file_configuration_test.go` | Environment startup and connects use multiple independent pairs. |
| US6-S3 | `oauth2_secret_file_configuration_test.go` | Real CLI startup and connects use the JSON pair mapping. |
| US6-S4 | `oauth2_secret_file_configuration_test.go` | Whole-map precedence selects complete pairs. Winning `{}` removes bindings but never changes service source. |
| US6-S5 | `oauth2_secret_file_configuration_test.go` | Invalid canonical IDs stop startup before service availability. |
| US6-S6 | `oauth2_secret_file_configuration_test.go` | Missing, non-string, empty, whitespace-only, or relative paths in either field stop startup. |
| US6-S7 | `oauth2_secret_file_configuration_test.go` | Valid syntax permits startup/metadata with unavailable files. Affected operations fail only when they require those files. |
| US6-S8 | `oauth2_secret_file_configuration_test.go` | Unknown-ID bindings create no service and select no service's source. |
| US6-S9 | `oauth2_secret_file_configuration_test.go` | Filesystem canonical-ID clearing is invalid. Reassignment preserves source and requires the new ID's pair without fallback. |
| US7-S1 | `oauth2_secret_file_deployment_test.go` | Paired configuration supports a filesystem connection. Integration Helm tests cover default omission and rendered mount preservation. |
| US7-S2 | `oauth2_secret_file_deployment_test.go` | A non-root filesystem broker connects and renews access after secret rotation with current credentials. |
| US7-S3 | `oauth2_secret_file_deployment_test.go` | Target paths permit explicit filesystem registration without placeholders. Integration Helm tests verify their exact rendered configuration and deployment mounts. |

### Red-green evidence and fixtures

Write acceptance expectations before feature behavior. Demonstrate semantic red for explicit source selection, source transitions, client-ID continuity, file use/rotation, failures, pair configuration, and deployment. Existing-behavior controls can already pass. Do not manufacture failures, weaken assertions, or mark scenarios pending.

T064 records the first full acceptance run before chart implementation. T021 reruns it after the chart contract is complete; chart-only checks may then pass while broker feature assertions remain red. Do not manufacture a new failure. Author shared focused regressions in T023 and T029–T034 before T026/T035–T042; T044/T045, T052/T053, T055–T057, and T060/T061 execute the unchanged regressions later.

Reuse `helpers.MockUpstreamOAuth2Server`, its captured headers/forms, error responses, refresh tokens, and expiry controls. Use `helpers.MockCIMDUpstream` for real assertion validation. Move shared connect steps into a test helper only where the new scenario files genuinely reuse them.

Use `bootstrap.NewBufferedJSONLogger()` for structured-event assertions. Compare service source, credential presence, ciphertext, version, and timestamps before/after authentication to detect rotation-related mutation. Source-transition tests separately require intentional atomic record changes. Normal session-token updates and non-secret identity association remain permitted.

Source-failure assertions require zero new provider token requests. US3-S1–S8 and US3-S10–S12 exercise each file independently inside the corresponding journey. Client-ID failures also stop authorization initiation before a provider redirect. Unavailable secrets do not stop initiation. Include callback and explicit refresh. US3-S9 also exercises RFC 8693 automatic refresh and initiation's generic error. Do not count library auth-style probes as separate broker attempts.

Use temporary projected-volume generations and atomic symlink replacement. Exercise 65,536-byte success and 65,537-byte rejection in adapter tests. Exercise disappearance after success, first-use failure, and recovery as separate transitions.

T022 adds deterministic real-I/O interleavings between target opens, reads, and revalidation through the adapter's production internal stages. A test synchronizes file publication, not fake OS results or production test-only hooks. Cover `generation_changed`, zero partial results, descriptor closure, and recovery with the next acquisition. T033 verifies zero provider requests for acquisition errors in callback and refresh paths. It also covers publication after validated acquisition without weakening established-identity checks. These focused regressions supplement US2-S3's complete provider-observed generation journey.

### CLI, non-root, and chart execution

Normal HTTP journeys use the production configuration loader, Builder, and dual-server bootstrap in process. Package tests retain exhaustive loader and actual root-command flag tables. US6-S5 and US6-S6 use representative startup cases for each configuration source, with no reachable listener or provider request after rejection. No package or backend E2E test requires a prebuilt broker.

Use an actual non-root broker for unreadable-file and deployment-readability scenarios. If the suite runs as root, drop child privileges and grant traversal access to its temporary directory. Use request deadlines and observable readiness, not fixed sleeps. Restore file permissions before cleanup.

Real Helm rendering assertions use the existing integration harness. Missing Helm skips these tests locally. The existing Helm CI job sets `REQUIRE_HELM=1` and requires the checks. HTTP journeys do not invoke Helm. Parse Kubernetes objects and embedded configuration as data, not template-source assertions. Synthetic target values require neither an external deployment checkout nor real credentials.

The external deployment validation uses the real generator and all three actual source inputs. It is a separate delivery check, not a replacement for US7-S3's portable E2E scenario.

### Unit and integration tests

| Area | Consumer-visible boundaries |
|---|---|
| `internal/config` | Exact key case/dots, strict pair types, both required paths, winning empty mapping, whole-map replacement, invalid IDs/paths, no source access at startup, safe renamed-key diagnostics. |
| `internal/adapters/credentialfile` | Real file I/O, bounded values, symlinks, safe errors, and coherent pair acquisition. Deterministic replacement between opens/reads/revalidation rejects mixed pairs and closes both descriptors. |
| `internal/domain/model` and `internal/domain/thirdparty` | Defaults, unknown source rejection, mixed-source rejection, excluded modes, canonical-ID requirement, update omission, source-specific representations, atomic transitions, and an irreversible internal transition marker. |
| `internal/domain/oauth2session` | Three boundaries, no stored/excluded reads, initiation without secret access, and fresh coherent pairs. Acquisition errors send zero provider requests. Post-acquisition publication preserves only the validated operation-local pair, subject to identity checks. Existing fallback, transition, and persistence invariants remain. |
| Both storage adapters and migrations | Legacy backfill preserves behavior/encryption. Source, absent filesystem credentials, atomic transitions/marker, and non-secret identity association survive persistence without file secrets. |
| Affected management API consumers | Explicit source selection, inline-field omission, preserved mode on unrelated updates, and source-specific create/update validation. |
| `internal/domain/tokenexchange` | Source failure remains `server_error` with typed detail `credential_source_unavailable` and outcome `infrastructure_error`. Provider rejection retains upstream diagnostic classifications. |
| Existing chart integration harness | Typed rendered configuration, mount preservation, default omission, and invalid chart-value rejection. |

Do not add tests for copied wiring, source text, field forwarding, or incidental wording. Exercise new migration and PostgreSQL behavior through existing integration conventions.

### Verification and smoke proof

The [quickstart.md](./quickstart.md) defines verification commands and synthetic smoke procedures. Its evidence records distinguish historical semantic-red runs, completed story/package checks, and the pending full repository gate. No frontend behavior changed. Run the full repository gate before delivery.

Smoke proof must show actual source-aware registration/read/update responses and provider observations for initiation, exchange, refresh, rotation, and source loss. Demonstrate client-ID mismatch stopping existing code/session authentication and new connections using the new ID. During file outages, metadata remains available. A test pass alone is not runtime proof.

### Requirement traceability

The plan covers each current requirement and success criterion. The scenario table covers all seven stories and 50 numbered journeys. [`tasks.md`](./tasks.md) maps the requirements and scenarios to executable tasks, story checkpoints, and an explicit execution-wave DAG.

| Spec items | Plan implementation or verification target |
|---|---|
| FR-001, FR-002, FR-003, FR-004, FR-008 | Sections 1 and 3: explicit source and exact independent pair selection. US1-S1–S6, US1-S8, US6-S8–S9. |
| FR-005, FR-025 | Sections 3 and 3a: initiation reads client ID only, exchange/refresh read both for filesystem, both modes enforce established identity after transitions, and legacy missing identities cannot survive a source round trip. US1-S2–S3, US2-S5, US5-S3, US5-S5–S6. |
| FR-006, FR-011, FR-012 | Sections 2–4: missing bindings and unusable required values fail closed with bounded, non-blocking reads. US1-S8, US3-S1–S7, US3-S10–S12. |
| FR-007 | Sections 3 and 3a: excluded modes ignore bindings and reject filesystem selection. US4-S1–S4. |
| FR-009, FR-010, FR-013, FR-014, FR-015, FR-016 | Sections 2–3: fresh coherent pairs, target-identity validation, publication-race rejection/recovery, normalization, isolation, and no rotation-related service writes. T022/T033 and US1-S6, US2-S1–S5, US3-S8, US5-S1. |
| FR-017, FR-018 | Section 3a: source-aware representation and file-independent administration/metadata. US1-S7, US5-S1–S4, US5-S7. |
| FR-019, FR-020 | Section 4: credential-free events, source/provider separation, and preserved generic responses. US1-S8, US3-S9, US5-S3–S4. |
| FR-021, FR-022, FR-023, FR-024 | Section 3a and Constitution Check: API-first source invariants, defaults, atomic transitions, both stores, and migrations. US1-S7, US1-S9, US5-S2, US5-S5–S8. |
| CR-001, CR-002, CR-003 | Section 1: all sources, pair validation, exact keys, complete-map precedence, and explicit `{}`. US6-S1–S6. |
| CR-004, CR-005, CR-006 | Sections 1 and 3: configured paths only, availability at required use, no hot reload, source preserved after binding removal. US2-S1, US3-S8, US5-S2–S5, US6-S7. |
| CR-007 | Section 5: pair-valued chart contract and existing volume inputs. US7-S1–S3. |
| SR-001, SR-002, SR-003, SR-004 | Sections 2–4: fail closed, no disclosure or file-secret persistence, atomic removal, excluded-mode and security preservation. US1-S8, US2-S5, US3-S1–S12, US4-S1–S4, US5-S1–S6. |
| SR-005 | Section 5 and non-root execution: readable, read-only directory mounts without credential-file `subPath`. US7-S1–S2. |
| DR-001, DR-002, DR-003, DR-004, DR-005 | Documentation and release delivery, Sections 1, 3a, and 5: source/API/configuration guides, continuity limits, generated deployment inputs, ADR, architecture, consumers, and changelog. |
| SC-001, SC-004, SC-005 | US1-S1–S7, US4-S1–S4, US5-S7, and US2-S2: filesystem field omission, stored client ID/REDACTED preservation, authentication compatibility, and independent services. |
| SC-002, SC-003, SC-011 | US2-S1–S5, US1-S8, US3-S1–S12, and US5-S5–S6: first-use freshness, zero fallback/provider requests on failures, and identity continuity across file changes and source transitions. |
| SC-006, SC-007, SC-010 | US5-S1–S8 and US3-S9: outage-safe metadata, no disclosure, credential-free classification, and source transitions. |
| SC-008, SC-009 | US6-S1–S6 and US7-S1–S3: configuration validity and exact target deployment generation. |


## Documentation and release delivery

Architecture, accepted ADR, administrative OpenAPI, supporting documents, examples/index, configuration guides, chart documentation, and repository consumers describe implemented behavior. External deployment inputs/README and generated manifests are current. The feature remains unreleased. Documentation records source selection, representation, transitions, pair precedence, missing-binding errors, and identity limits. Generation used the local working chart, not a chart snapshot identified solely by `scripts/chart_ref`.

The release workflow includes source/API/configuration changes, rotation limits, fail-closed behavior, and migration safeguards in generated notes. ~~Do not recreate the removed manual changelog.~~ Superseded by required breaking-change documentation. T010 resolved the Admin API version strategy and defined `docs/changelog.md` before implementation. T071 aligns the delivered model and recorded evidence with that decision. No broker release tag, registry publication, or live deployment is claimed.

## Complexity Tracking

No identified constitution violation requires an exception. One focused port owns client-ID reads and coherent pair acquisition outside the domain. Target validation uses standard file identity, not a provider framework or persistent generation metadata. Source-specific decoding preserves canonical-ID case. The refined scope retains the explicit service source, irreversible transition marker, source-aware persistence, and non-secret client-identity association. The marker prevents a source round trip from restoring the legacy missing-identity exception. No aggregate, path persistence, credential cache, background component, or automatic identity migration is added.
