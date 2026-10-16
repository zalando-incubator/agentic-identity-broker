# Implementation Plan: Inline and File-Based JWKS for Trust Anchors

**Branch**: `052-inline-jwks-trust-anchors` | **Date**: 2026-10-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/052-inline-jwks-trust-anchors/spec.md`

## Summary

Let three trust anchors take their public verification keys from an inline JWK Set (`jwks`) or a
key-set file (`jwks_file`), as alternatives to `jwks_uri` and discovery:

- the token-exchange client-assertion trust anchor, which machine-facing approval authentication
  reuses;
- every impersonation trusted issuer;
- JWT pre-authentication on the end-user server and on the admin server (stakeholder decision
  2026-10-16, research R9).

The design adds one pure domain context, `internal/domain/trustanchor/`. It resolves exactly one
Key Set Source per trust anchor and validates every Local Key Set: public asymmetric keys only,
approved algorithms, unique key identifiers, and at most 1 MiB. Two new adapters in
`internal/adapters/jwks/` implement the existing `ports.JWKSPort`, so domain validation stays
source-agnostic (ADR 008):

- a static adapter for inline sets;
- a file adapter that re-reads within the trust anchor's refresh bounds and keeps the last valid
  set while degraded.

Files are read only beneath `security.trust_material_dir`, through Go's `os.Root`. A new
`/health` component, `trust_anchor_key_files`, reports file re-read failures. The Helm chart
projects existing Secrets or ConfigMaps into the trust directory. A new ADR 040 extends ADR 029.

## Technical Context

**Language/Version**: Go 1.27.1

**Primary Dependencies**:

- `github.com/lestrrat-go/jwx/v4` v4.5.0 for key parsing, classification, and verification
- `github.com/lestrrat-go/httprc/v3` v3.0.6 and `github.com/jwx-go/jwkfetch/v4` v4.0.4, unchanged for remote key sets
- Go `os.Root` for trust-directory containment
- Viper and Cobra, `log/slog`, chi v5
- Ginkgo v2 and Gomega, testify
- Helm 3

**Storage**: None. The feature adds no persisted state, migration, or repository. It reads
operator-mounted key-set files read-only.

**Testing**:

- Go unit tests for the domain context, adapters, configuration loader and validator, jwtauth
  adapter, and builder.
- Production-bootstrap Ginkgo E2E tests with in-memory keys and per-test temporary trust
  directories.
- Self-contained integration tests for Helm rendering and example configuration loading.

**Target Platform**: Linux broker container in production. Darwin is a supported development
platform. `os.Root` behaves the same on both for this use.

**Project Type**: Backend configuration feature in the Go monorepo, plus Helm chart changes. No
React UI change and no new HTTP route. It makes one additive change to the `/health` response
contract.

**Performance Goals**:

- No added per-request I/O. Verification reads an in-memory `jwk.Set`.
- Startup with only inline or file sources makes zero outbound requests for those trust anchors
  (SC-002).
- A file re-read reads at most 1 MiB once per `jwks_min_refresh` for each distinct file adapter.

**Constraints**:

- Fail closed on every invalid, ambiguous, or unreadable source. Never fall back to discovery or
  another source (SR-001).
- No inline content, file contents, or key material in errors, logs, audit records, or health
  output (FR-008).
- Read files only beneath the trust directory. Containment, the regular-file check, and the read
  use one file descriptor (FR-023).
- Credential validation is identical for all sources (FR-005).
- ExtProc is unaffected.

**Scale/Scope**:

- 3 trust-anchor kinds and 7 new configuration keys
- 1 new domain package and 2 new adapters
- 1 new `/health` component and 1 new ADR
- 30 acceptance scenarios plus one SC-003 parity table

**NEEDS CLARIFICATION**: None. [research.md](./research.md) resolves every design question. Four
stakeholder decisions are listed under [Stakeholder decisions](#stakeholder-decisions-before-implementation).

## Constitution Check

*GATE: Pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Phase 0 gate

| Check | Status | Plan |
|---|---|---|
| Domain model | Pass | The spec defines Trust Anchor, Key Set Source, Local Key Set, and Trust Directory. [data-model.md](./data-model.md) adds their invariants and resolution rules. |
| Domain concepts | Pass | Add Key Set Source, Local Key Set, and Trust Directory to the `ARCHITECTURE.md` glossary. Disambiguate Key Set Source from the existing JWKS-aggregation **KeySource**. |
| Entity IDs | Not applicable | No persisted entity and no UUID identifier. |
| Configuration design | Pass | Seven keys with YAML examples in the spec and [contracts/configuration.md](./contracts/configuration.md). |
| Configuration examples | Pass | Add `examples/config/trust-anchor-local-jwks.yaml`, reference it in `examples/config/README.md`, and add commented alternatives to `token-exchange.yaml`, `impersonation.yaml`, and `jwt-preauth.yaml`. |
| Helm deployment contract | Pass | The broker chart deploys the affected workload. Update values, schema, ConfigMap and Deployment templates, and README ([contracts/helm-values.md](./contracts/helm-values.md)). |
| API design first | Pass with precondition | No new route. The additive `/health` component is designed in [contracts/health-endpoint.md](./contracts/health-endpoint.md). |
| OpenAPI documentation | Pass | Update `/health` in `api/enduser/openapi.yaml`. The admin API does not change. |
| API changes confirmed | **Pending** | The stakeholder must confirm the `trust_anchor_key_files` component before implementation (Principles IV and X). |
| Database design | Not applicable | No schema change. |
| E2E acceptance tests | Pass | 30 production-bootstrap Ginkgo `It()` blocks, written before implementation. |
| E2E mapping | Pass | The Testing Strategy maps every scenario to one `It()`. |
| E2E red phase | Pass | Every `It()` asserts a concrete status, response body, error substring, log field, or `/health` value. |
| Frontend Playwright E2E | Not applicable | No React change. |
| Frontend screenshots | Not applicable | No UI state. |
| Concrete example journeys (Principle XIV) | **Resolved in this pass** | The spec predated constitution v4.2.0 and had no journeys. This planning pass added five linked journeys with configuration, HTTP exchanges, logs, and decoded JWT claims. They need stakeholder review with the plan. |

### Implementation considerations

| Check | Status | Plan |
|---|---|---|
| Security-first | Pass | Startup validation is mandatory and fails closed. Public asymmetric keys only. `os.Root` containment. No fallback. The self-trust check applies to every source. |
| Architecture documentation | Pass | Update §3.1.4.1 (trust-anchor key sources and the health component), the JWT pre-authentication section, glossary entries, and the ADR index. |
| ADRs | Pass with precondition | Add `adrs/040-trust-anchor-local-key-sets.md`, extending ADR 029 and ADR 008. It must be Accepted before implementation tasks start. |
| Library-first security | Pass | jwx parses, classifies, and verifies keys. `os.Root` contains file access. The broker adds invariant checks only. |
| Zalando guidelines | Pass | The `/health` change is additive and optional, and uses the existing component vocabulary. |
| End-user API docs | Pass | Add `docs/api/health.md` with the `components` contract and the `trust_anchor_key_files` examples (Principle IV requires rendered end-user API docs in `docs/api/`), and rebuild the Redoc site with `just docs-build`. |
| Migration testing | Not applicable | No migration. |
| Hexagonal architecture | Pass | Domain rules in `trustanchor/`. File I/O in a driven adapter behind `ports.JWKSPort`. The jwtauth adapter receives the key-set port by injection, without a cross-adapter import. All wiring is in the builder. |
| Persistence patterns | Not applicable | No persistence. |
| Configuration system | Pass | All keys go through `internal/ports/config.go`, the loader (`BindEnv` where an override mechanism exists), and the validator. |
| Feature scope | Pass | Every task traces to an FR, SR, SC, or binding rule. Four unrelated findings are deferred to the user ([research.md](./research.md), Deferred findings). |

### Post-design re-check

The gate passes after Phase 1, with the preconditions listed below.

- The domain context is a genuine bounded context. Three existing contexts consume it, and it has
  no I/O.
- Ports do not change. The new adapters implement `ports.JWKSPort`, and the jwtauth adapter config
  gains an injected port.
- The only external-contract change is the additive `/health` component. It is designed and
  awaits confirmation.
- No unresolved security, configuration, Helm, or test-traceability question remains.

## Stakeholder decisions before implementation

1. **Confirm the `/health` contract delta** in [contracts/health-endpoint.md](./contracts/health-endpoint.md)
   (Principle X).
2. **Accept ADR 040** after it is drafted in Phase 2a (Principle II).
3. **Confirm two scope interpretations** recorded in [research.md](./research.md):
   - R4: absolute symbolic links inside the trust directory are rejected. Relative links, which
     Kubernetes uses, are accepted. **Confirmed 2026-10-16.**
   - R9: the stakeholder chose to **support** `jwks` and `jwks_file` on
     `server.admin.authentication.jwt` (2026-10-16). FR-002 and User Story 5 scenario 4 now cover
     it.
4. **Review the five example journeys** added to [spec.md](./spec.md) under Principle XIV.
   **Confirmed 2026-10-16**, together with the `/health` contract (decision 1) and ADR 040
   (decision 2).
5. **Decide whether to file GitHub issues** for the four deferred findings in
   [research.md](./research.md). They are not tasks of this feature.

## Project Structure

### Documentation for this feature

```text
specs/052-inline-jwks-trust-anchors/
├── spec.md               # Amended: Concrete Example Journeys (Principle XIV)
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── configuration.md   # Settings, startup errors, log events
│   ├── health-endpoint.md # /health component delta (pending confirmation)
│   └── helm-values.md     # Chart values, schema, and rendering delta
└── checklists/
    └── requirements.md
```

### Source code and documentation affected

```text
adrs/
└── 040-trust-anchor-local-key-sets.md            # New; extends ADR 029 and ADR 008

api/enduser/openapi.yaml                            # /health trust_anchor_key_files component

internal/
├── domain/
│   ├── trustanchor/                                # New bounded context (no I/O)
│   │   ├── source.go                               # KeySetSource, AnchorRef, resolution rules
│   │   ├── local_key_set.go                        # ParseLocalKeySet, invariants, DiffKeyIDs
│   │   ├── algorithms.go                           # Approved asymmetric algorithm set
│   │   └── errors.go                               # Categorized, content-free errors
│   └── impersonation/constants.go                  # IsApprovedAlgorithm delegates to trustanchor
├── adapters/
│   ├── jwks/
│   │   ├── adapter.go                              # Size bound refers to trustanchor.MaxKeySetBytes
│   │   ├── static_adapter.go                       # New: inline source
│   │   ├── file_adapter.go                         # New: re-read loop, staleness, health
│   │   └── trust_directory.go                      # New: os.Root wrapper and containment
│   └── jwtauth/jwx_authenticator.go                # Optional injected KeySet port
├── ports/config.go                                 # jwks, jwks_file, trust_material_dir fields
├── config/
│   ├── loader.go                                   # BindEnv for new client-assertion and security keys
│   └── validator.go                                # Structural rules via trustanchor; redacted values
└── app/builder.go                                  # Source resolution, adapters, logs, health, shutdown

charts/agentic-identity-broker/
├── values.yaml
├── values.schema.json
├── templates/configmap.yaml
├── templates/deployment.yaml
└── README.md

docs/configuration.md
docs/api/health.md                                  # New: /health components and examples (Principle IV)
examples/config/
├── trust-anchor-local-jwks.yaml                    # New complete example
├── token-exchange.yaml, impersonation.yaml, jwt-preauth.yaml
└── README.md

ARCHITECTURE.md, AGENTS.md, internal/AGENTS.md, internal/domain/AGENTS.md, internal/adapters/AGENTS.md

.github/workflows/ci.yml                            # setup-helm in e2e-backend (US2-S5)

tests/
├── e2e/
│   ├── helpers/local_jwks.go                       # ES256/RS256 key sets with chosen kids, signing, trust-dir layouts
│   ├── helpers/counting_server.go                  # httptest server that counts every request
│   ├── helpers/helm_render.go                      # helm template wrapper for US2-S5
│   ├── trust_anchor_inline_jwks_test.go            # US1, US3
│   ├── trust_anchor_key_files_test.go              # US2
│   ├── impersonation_local_jwks_test.go            # US4
│   ├── jwt_preauth_local_jwks_test.go              # US5
│   └── trust_anchor_key_source_parity_test.go      # SC-003 table
└── integration/
    ├── helm_chart_test.go                          # Trust-material rendering
    └── config_test.go                              # New example loads through the broker loader
```

**Structure Decision**: Create `internal/domain/trustanchor/` as an independent bounded context
for key-source rules shared by three existing contexts. Keep all filesystem access in
`internal/adapters/jwks/`, the package that already owns key-set retrieval. The jwtauth adapter
receives a `ports.JWKSPort` from the builder instead of importing the jwks adapter.

## Design and Implementation Approach

### 1. Resolve one source per trust anchor before any I/O

The builder resolves each trust anchor's `KeySetSource` with `trustanchor.ResolveKeySetSource`
before discovery, adapter creation, or file access. The resolution rules are in
[data-model.md](./data-model.md). Resolution failures stop `Build()` with the messages in
[contracts/configuration.md](./contracts/configuration.md) §2.

The configuration validator calls the same structural rules, so YAML users see errors at load time.
For inline values, it uses the `[inline JWK Set redacted]` placeholder.

### 2. Validate local key sets in the domain

`trustanchor.ParseLocalKeySet` enforces the size bound, the `keys` structure, jwx strict parsing,
non-emptiness, no symmetric or private keys, approved `alg`, and unique `kid`. Errors are
categorized and never wrap jwx error text (research R5).

The approved algorithm set moves to `trustanchor`. `impersonation.IsApprovedAlgorithm` delegates to
it. The remote adapter's 1 MiB bound refers to `trustanchor.MaxKeySetBytes`.

### 3. Serve inline sets from a static adapter

`jwks.StaticAdapter` holds the parsed set and is always healthy. No refresh, no network.

### 4. Serve file sets from a re-reading adapter

`jwks.FileAdapter` opens the file through `jwks.TrustDirectory` (`os.Root`). It checks the regular
file on the open descriptor, reads at most 1 MiB plus one byte, and validates the result with
`ParseLocalKeySet`.

- It re-reads every `jwks_min_refresh` (JWT pre-authentication: 5m).
- It swaps the set atomically on success and logs key-identifier changes.
- On failure it keeps the last set and reports degraded health.
- After `jwks_max_refresh` since the last success (JWT pre-authentication: 15m), it fails closed.
- `Shutdown` stops the loop.

The state machine is in [data-model.md](./data-model.md).

### 5. Wire every trust anchor

- **Client assertion**: add `inline` and `file` branches to the source switch. They skip discovery.
  "Unset" now includes empty `jwks` and `jwks_file`. Keep the upstream issuer as the expected
  issuer in proxy or hybrid mode without `issuer_uri` (FR-015). Machine-facing approval
  authentication uses the same validator, so it needs no separate change.
- **Impersonation**: extend the factory's sharing key with the source kind, inline document, and
  file path (FR-017). Chain shutdown for each new adapter.
- **JWT pre-authentication**: inject the static or file adapter into
  `jwtauth.JWXAuthenticatorConfig.KeySet`, which bypasses `jwkfetch` and its startup fetch. Do this
  for both servers in the existing per-server loop; each server is its own trust anchor
  (`server.enduser.authentication.jwt`, `server.admin.authentication.jwt`).
- **Self-trust checks** run before source loading, unchanged (FR-014).
- **Logging**: log `trust anchor key set loaded` once per local source (FR-018).
- **Trust directory**: open it once, only when a `jwks_file` exists, and close it at shutdown.

### 6. Report file health

`App.EnduserHealthComponents()` merges `trust_anchor_key_files` with `upstream_jwks`. The component
is degraded when any file adapter is degraded, and absent when no file source exists. The HTTP
status does not change ([contracts/health-endpoint.md](./contracts/health-endpoint.md)).

### 7. Package the deployment contract

Add `broker.security.trustMaterial` and the client-assertion `jwks`/`jwksFile` values. Render a
read-only projected volume without `subPath`, and extend the schema for the pass-through blocks
([contracts/helm-values.md](./contracts/helm-values.md)).

## Implementation Phase Overview

*`/speckit-tasks` generates the detailed task list.*

| Phase | Purpose | Applies |
|---|---|---|
| Phase 0 | Pre-implementation refactoring | Skip |
| Phase 1 | Setup | Yes |
| Phase 2 | Design preconditions (2a–2f) | **Mandatory** |
| Phase 2.5 | Foundational infrastructure | Yes |
| Phase 2.7 | Entity boilerplate | Skip |
| Phase 3–7 | User stories 1–5 | Yes |
| Phase N | Constitution compliance verification | **Mandatory** |

- [x] Phase 0 (refactoring): **skip**. The only existing-code moves are the approved algorithm set
  and the 1 MiB constant. Both are a few lines, change no behaviour, and belong with the
  foundational domain package in Phase 2.5.
- [x] Phase 2.7 (entity boilerplate): **skip**. No new persisted entity, port, route, or handler.

### Phase 1: Setup

- Add E2E helpers: local key sets with chosen `kid`, `alg`, and key type; signing; trust-directory
  writers, including the Kubernetes `..data` layout and atomic swap; a counting `httptest` server;
  a `helm template` wrapper.
- Add `azure/setup-helm` to the `e2e-backend` CI job and document Helm as an E2E prerequisite.

### Phase 2: Design preconditions

- **2a Domain model and glossary**: draft ADR 040 and obtain acceptance. Update `ARCHITECTURE.md`
  (glossary, §3.1.4.1, JWT pre-authentication, ADR index), root `AGENTS.md` (ADR table), and the
  `internal/` agent guides (package maps, JWKS adapter section).
- **2b Configuration design**: add the seven keys to `docs/configuration.md` and
  `examples/config/` with the README reference. Update the Helm values, schema, templates, and
  README per [contracts/helm-values.md](./contracts/helm-values.md).
- **2c API design**: apply [contracts/health-endpoint.md](./contracts/health-endpoint.md) to
  `api/enduser/openapi.yaml` after stakeholder confirmation. Add `docs/api/health.md` with the
  component contract and examples, and rebuild the Redoc site.
- **2d Database design**: none. Record "no schema change".
- **2e Frontend review**: none. Record "no UI change".
- **2f E2E acceptance test design**: review the spec journeys. Write the 30 `It()` blocks and the
  SC-003 parity table. Compile them and confirm a semantic red run.

### Phase 2.5: Foundational infrastructure

- Add compile-only scaffolding first: `trustanchor` types and function signatures, new config
  fields, adapter constructors that return "not implemented" errors, the jwtauth `KeySet` field,
  and the health-component hook. Tests then fail semantically, not at compile time.
- Then, test-first:
  - `trustanchor` resolution and Local Key Set invariants, and the algorithm-set move;
  - `TrustDirectory`, `StaticAdapter`, and `FileAdapter`;
  - loader `BindEnv` and validator rules.

### Phase 3–7: User stories

- **US1 (P1)**: client-assertion inline source, discovery skip, FR-015 issuer fallback, approval
  authentication, startup log.
- **US2 (P1)**: client-assertion file source, trust directory, re-read, health component, Helm
  projection.
- **US3 (P1)**: complete the fail-closed matrix and redaction across all trust anchors.
- **US4 (P2)**: impersonation inline and file sources, sharing key, shutdown chaining.
- **US5 (P3)**: JWT pre-authentication injection on both servers, `verification: none`
  exclusivity, per-server isolation.

### Phase N: Constitution compliance verification

Verify against the principles: ADR 040 Accepted and matching the code; OpenAPI and docs updated;
Helm contract complete; glossary updated; no key material in any error, log, or health output;
`just check`, `just test`, `just test-integration`, and `just verify` green; all 30 scenarios and
SC-003 passing.

## Testing Strategy

### End-to-end acceptance tests

**Framework**: Ginkgo v2 and Gomega with `app.Builder` through `tests/e2e/bootstrap/`. Each test gets
fresh storage, servers, keys, and trust directory.

**Labels**:

- Every scenario `It()` uses `Label("inline-jwks-trust-anchors")`.
- The SC-003 table uses `Label("inline-jwks-trust-anchors-parity")`.
- `ginkgo -v --label-filter="inline-jwks-trust-anchors" ./tests/e2e/` selects exactly the 30
  scenarios.

**Observation techniques** ([research.md](./research.md) R14):

| Technique | Use |
|---|---|
| Counting `httptest` issuer server | Zero outbound requests |
| `BuildApp` error substrings | Startup failures |
| `bootstrap.NewBufferedJSONLogger` | Log fields and absence of key material |
| `/health` polling with `Eventually` | Health component |
| `jwks_min_refresh: 200ms`, `jwks_max_refresh: 2s` | Fast re-read scenarios |
| Absence of base64url `x`, `y`, `n`, `d` values | Errors and logs leak no key material |

**Scenario mapping**:

| Spec scenario | E2E location | Test description |
|---|---|---|
| US1-S1 | `trust_anchor_inline_jwks_test.go` | Starts in local mode with an inline set and sends zero requests to the issuer host. |
| US1-S2 | `trust_anchor_inline_jwks_test.go` | Exchanges a subject token with an inline-key client assertion and returns the vault token. |
| US1-S3 | `trust_anchor_inline_jwks_test.go` | Rejects an assertion signed by an unlisted key with `401 invalid_client` and issues no token. |
| US1-S4 | `trust_anchor_inline_jwks_test.go` | Authenticates `POST /api/approvals` with an inline-key `X-Client-Assertion`. |
| US1-S5 | `trust_anchor_inline_jwks_test.go` | In hybrid mode without `issuer_uri`, expects the upstream issuer, verifies assertions only with inline keys, and verifies subject tokens with upstream keys. |
| US2-S1 | `trust_anchor_key_files_test.go` | Starts with a file inside the trust directory and verifies assertions with its keys. |
| US2-S2 | `trust_anchor_key_files_test.go` | Fails startup for a missing path and a directory path, naming the setting and the path. |
| US2-S3 | `trust_anchor_key_files_test.go` | Applies a replaced file within the refresh bounds and rejects credentials signed only by a removed key. |
| US2-S4 | `trust_anchor_key_files_test.go` | Keeps verifying after a corrupt re-read, reports `trust_anchor_key_files: degraded`, logs without contents, and recovers. |
| US2-S5 | `trust_anchor_key_files_test.go` | Renders the chart with a Secret source, projects it, boots from the rendered config, and verifies. |
| US2-S6 | `trust_anchor_key_files_test.go` | Fails startup for a path outside the trust directory, naming the setting, path, and directory, without opening it. |
| US2-S7 | `trust_anchor_key_files_test.go` | Fails startup for `..` and symbolic-link escapes from inside the trust directory. |
| US2-S8 | `trust_anchor_key_files_test.go` | Fails startup for `jwks_file` without `security.trust_material_dir`. |
| US2-S9 | `trust_anchor_key_files_test.go` | Starts with the Kubernetes `..data` relative-link layout. |
| US3-S1 | `trust_anchor_inline_jwks_test.go` | Fails startup for two sources, naming the trust anchor and both settings. |
| US3-S2 | `trust_anchor_inline_jwks_test.go` | Fails startup for a non-set document, an empty set, and an oversized set. |
| US3-S3 | `trust_anchor_inline_jwks_test.go` | Fails startup for private key material, with no key material in the error or logs. |
| US3-S4 | `trust_anchor_inline_jwks_test.go` | Fails startup for a symmetric key and for an unapproved `alg`. |
| US3-S5 | `trust_anchor_inline_jwks_test.go` | Fails startup for a duplicate `kid` and names it. |
| US3-S6 | `trust_anchor_inline_jwks_test.go` | Fails startup for inline `jwks` with explicit refresh bounds. |
| US3-S7 | `trust_anchor_inline_jwks_test.go` | Still rejects the broker's own issuer with inline and file sources. |
| US3-S8 | `trust_anchor_inline_jwks_test.go` | Logs source kind, key count, and key identifiers without key material. |
| US4-S1 | `impersonation_local_jwks_test.go` | Starts without discovery for an inline trusted issuer and mints an impersonation token. |
| US4-S2 | `impersonation_local_jwks_test.go` | Validates impersonation credentials with a file trusted issuer. |
| US4-S3 | `impersonation_local_jwks_test.go` | Rejects an RS256 credential when the trusted issuer allows only ES256. |
| US4-S4 | `impersonation_local_jwks_test.go` | Denies a credential signed by rule B's key when rule A, for the same issuer, holds a different key. |
| US5-S1 | `jwt_preauth_local_jwks_test.go` | Derives the principal for `GET /api/me` with inline and file sources. |
| US5-S2 | `jwt_preauth_local_jwks_test.go` | Fails startup for `verification: none` with `jwks` or `jwks_file`. |
| US5-S3 | `jwt_preauth_local_jwks_test.go` | Fails startup for `verification: jwks` with no source, as today. |
| US5-S4 | `jwt_preauth_local_jwks_test.go` | Admin and end-user servers with different local key sets: the admin API accepts an admin-key JWT and rejects an end-user-key JWT with `401`. |

**SC-003 parity table** (`trust_anchor_key_source_parity_test.go`): a `DescribeTable` crosses five
credentials (valid, wrongly signed, expired, wrong audience, wrong issuer) with three sources
(remote, inline, file). It asserts identical status codes and error codes.

**SC-005 regression**: the existing token-exchange, approval, impersonation, and JWT
pre-authentication suites run unchanged.

**Red phase**: tests compile against the Phase 2.5 scaffolding and fail on concrete assertions. No
`XIt`, `PIt`, `Skip()`, or red-phase comments.

### Unit tests

| Package | Coverage |
|---|---|
| `internal/domain/trustanchor` | Table tests for every resolution rule and Local Key Set invariant, jwx edge cases (bare JWK, empty set, duplicate `kid`, `oct`, private EC and RSA, unknown `kty`, malformed JSON), error text free of input fragments, `DiffKeyIDs`, approved algorithms. |
| `internal/domain/impersonation` | Delegation keeps existing algorithm checks and messages. |
| `internal/adapters/jwks` | Static adapter health and lookup. File adapter: startup failures by category; re-read success, failure, and recovery; staleness expiry at `max`; no partial application; logs without contents; shutdown stops the loop. Trust directory: `os.Root` escapes, absolute links, Kubernetes layout, directories, root `/` rejection. Injected filesystem errors cover unreadable files when tests run as root. |
| `internal/adapters/jwtauth` | Injected key set bypasses `jwkfetch` and verifies with the same options. Unknown `kid` returns the existing error. |
| `internal/config` | New fields, `BindEnv` for the three bound keys, structural rules for every trust anchor including `server.admin.authentication.jwt`, `[inline JWK Set redacted]` placeholder. |
| `internal/app` | Discovery skipped for local sources; FR-015 issuer; local sources never reach the JWKS publisher (FR-016); impersonation sharing key (FR-017); health component merge; shutdown chaining. |

### Integration tests

- `tests/integration/helm_chart_test.go`: the projected volume from Secret and ConfigMap sources,
  a read-only mount without `subPath`, `security.trust_material_dir`, client-assertion `jwks` and
  `jwks_file` rendering, and the template failure for sources without a directory.
- `tests/integration/config_test.go`: `examples/config/trust-anchor-local-jwks.yaml` loads through
  the broker loader.

**Coverage goals**: every `trustanchor` branch; every file-adapter state transition; 100% of
acceptance scenarios (Principle XIII).

## Complexity Tracking

No constitution violation requires justification.
