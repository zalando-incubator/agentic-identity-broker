# Implementation Plan: ExtProc Metadata Input

**Branch**: `043-extproc-metadata-input` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/043-extproc-metadata-input/spec.md`

## Summary

The ExtProc token-exchange service currently derives both token-exchange inputs from raw HTTP
attributes: the subject credential from the `Authorization` header, and the resource URI assembled
from the `:scheme`, `:authority`, and `:path` pseudo-headers. This feature replaces both sources
with one trusted source — Envoy dynamic metadata published by Agentgateway in the
`aib.tokenexchange` namespace as the string fields `subject_token` and `resource_uri` — and removes
the raw-attribute path entirely, with no compatibility window.

The change is small in code and consequential in contract. Research (R4) established the decisive
fact: an Agentgateway `jwtAuth` policy with default `preserveToken: false` **strips** the validated credential from the request before ExtProc sees it. Under the configuration the spec mandates, the Authorization header is not merely untrusted — it is gone.


The producer expression is unaffected by that stripping, and the two facts together are what make
the design work. `Jwt::apply()` removes the header first and *then* inserts `Claims` into the
request extensions (`jwt.rs:618-631`), and `Claims` carries its own `jwt: SecretString` copy of the
raw token (`jwt.rs:505-508`). `jwt.rawToken` resolves from that copy, not from the header
(`jwt.rs:535-540`). So inside agentgateway the validated credential lives on in the `Claims`
extension, while the header a raw-attribute reader would have used is gone.

That in-process copy is not reachable from ExtProc. `Claims` is an agentgateway request extension;
the only thing that crosses the gRPC boundary is what the `metadataContext` expressions project
into `MetadataContext.FilterMetadata`. Dynamic metadata is therefore the only carrier of a
gateway-validated credential **exposed to ExtProc** — the `Claims` copy is what makes that
projection possible, not a competing source ExtProc could read instead. That makes this a
correctness fix rather than a hardening preference.

Implementation replaces the two per-path extraction blocks with one `extractTokenExchangeInput`
helper that validates `subject_token` before `resource_uri` and returns a typed rejection mapped to
a 503 JSON immediate response. `extractBearerToken` and `buildResourceURI` are deleted, the
"no Bearer token → pass through" branches are deleted, and every Agentgateway configuration in the repository gains a `jwtAuth` policy plus the `metadataContext` producer.


## Technical Context

**Language/Version**: Go 1.26.8
**Primary Dependencies**: `envoyproxy/go-control-plane` (ExtProc v3 protobuf), `google.golang.org/protobuf` (`structpb`), `google.golang.org/grpc`, OpenTelemetry Go SDK
**Storage**: N/A — ExtProc is stateless apart from its in-memory token cache (ADR 012), which is unaffected
**Testing**: stdlib `testing` + `testify` for `internal/extproc/`; Ginkgo/Gomega for `tests/e2e/extproc/`; testcontainers for Docker agentgateway scenarios
**Target Platform**: Linux container, standalone binary `cmd/extproc-token-exchange/` (ADR 011)
**Project Type**: Single Go service within the monorepo; no frontend, no database
**Performance Goals**: No regression. Metadata extraction replaces three header scans and a string
concatenation with two map lookups, so the header phase gets marginally cheaper; no additional
allocation on the success path.
**Constraints**: Fail closed on every invalid input (SR-002); never emit a credential or a raw URI
into logs, spans, metrics, or response bodies (SR-001, SR-003); `subject_token` reaches
`Exchanger.Exchange` byte-for-byte (FR-004); `internal/extproc/` must not import broker domain,
port, or adapter packages (ADR 011)
**Scale/Scope**: Two header paths in one file (~120 lines rewritten, ~25 deleted), one new ADR,
three Agentgateway configurations, one E2E helper, two E2E test files, three documentation files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: No new domain entities. The spec's three entities are wire-contract elements,
  documented in [data-model.md](data-model.md) and [contracts/](contracts/extproc-metadata-input.md).
  `internal/extproc/` is domain-free by ADR 011.
- [x] **Domain Concepts**: No new ubiquitous-language terms. `Subject Token Metadata` and
  `Resource URI Metadata` are ExtProc wire-protocol names, not broker domain concepts; the existing
  `ExtProc` and `agentgateway` glossary entries in ARCHITECTURE.md are amended rather than extended.
- [x] **Entity IDs**: N/A — no new domain entities, no UUID primary keys.
- [x] **Configuration Design**: N/A — both inputs are per-request wire data, not configuration. No
  new `EXTPROC_` key, no change to `internal/extproc/config/` (R9).
- [x] **Config Examples**: N/A for the sidecar schema. The *gateway* configuration example lives in
  `docs/guides/token-exchange-gateway.md` per FR-016 and in `mocks/agentgateway/config.yaml`;
  `examples/config/extproc-token-exchange.yaml` remains the `EXTPROC_` schema and is untouched.
- [x] **Helm Chart**: N/A — `charts/agentic-identity-broker/` deploys only the broker. It renders no
  ExtProc or Agentgateway configuration, and no configuration parameter changes (R9).
- [x] **API Design First**: N/A — no HTTP API change. The gRPC surface is the Envoy
  `ExternalProcessor` service, whose proto is upstream and unmodified. The changed contract is the
  dynamic-metadata channel, specified in `contracts/extproc-metadata-input.md` before implementation.
- [x] **API Documentation**: N/A — no end-user or admin HTTP API. Operator documentation is
  `docs/guides/token-exchange-gateway.md` (FR-016).
- [x] **API Changes**: N/A — `api/enduser/openapi.yaml` and `api/admin/openapi.yaml` are unaffected.
- [x] **Database Design**: N/A — no schema change, no migration.
- [x] **E2E Acceptance Tests**: All eight acceptance scenarios get E2E tests before implementation,
  in `tests/e2e/extproc/metadata_input_test.go` (direct gRPC) and
  `tests/e2e/extproc/agentgateway_metadata_e2e_test.go` (real gateway). See Testing Strategy.
- [x] **E2E Test Mapping**: 1:1 scenario-to-`It()` mapping, tabulated in Testing Strategy. Edge cases
  from the spec get their own `It()` blocks in a dedicated `Context`.
- [x] **E2E Red Phase**: Assertions target concrete values — HTTP 503, exact JSON error codes,
  the exact `subjectToken`/`resourceURI` arguments recorded by the stub exchanger, and the
  `Authorization` header observed by the mock MCP server. No placeholder assertions, no `Skip()`
  outside the existing Docker-availability guard.
- [x] **Frontend Playwright E2E**: N/A — no React UI change.
- [x] **Frontend Screenshots**: N/A — no React UI change.

**Implementation Considerations**:

- [x] **Security-First**: Fails closed on every invalid or absent input, with no configuration knob
  to relax it (SR-002). Removing the pass-through branch closes a path where an unauthenticated
  request reached the upstream unmodified. Diagnostics name the rejected field and reason but never
  the value (SR-003); response bodies are fixed strings (SR-001).
- [x] **Architecture Docs**: ARCHITECTURE.md states the obsolete contract in four places
  (`:830-835`, `:871-875`, `:878-895`, `:944`). All four are rewritten in the same PR, and the
  `ExtProc` glossary entry is amended.
- [x] **ADRs**: **REQUIRED** — `adrs/036-extproc-metadata-token-exchange-input.md` must be written
  and accepted in Phase 2 before implementation. Its scope is the token-exchange **input trust
  boundary**: it supersedes the raw-attribute extraction described in `specs/015-extproc-token-exchange/`
  (spec.md:103-106), the request-processing flow narrated by ADR 028 and ARCHITECTURE.md:878-895,
  and the "Resource URI construction" invariant in `internal/extproc/AGENTS.md:199-201`. It leaves
  **ADR 011 intact** — that ADR establishes the standalone binary and package isolation, not input
  extraction, and nothing here changes it. ADR 012 (token cache) is likewise unaffected. The ADR must
  record the rejected alternatives (raw-header fallback, dual-source precedence, a configuration
  toggle), the FR-015 validation precedence, and the FR-017 producer ordering requirement. Number 036
  is the next free ADR number (`adrs/035-root-mounted-spa.md` is the current maximum); it is
  unrelated to `specs/036-*`.
- [x] **Library-First Security**: No cryptography is introduced. Validation uses stdlib `net/url`
  and `strings`; metadata access uses `structpb` accessors. JWT validation stays in the gateway.
- [x] **Zalando Guidelines**: N/A — no REST API change.
- [x] **End-User Docs**: `docs/guides/token-exchange-gateway.md` gains the producer contract per
  FR-016. `docs/configuration.md` needs no change (no configuration key).
- [x] **Migration Testing**: N/A — no database migration.
- [x] **Hexagonal Architecture**: Unchanged. Extraction and validation sit in the ExtProc driving
  adapter (`server.go`), behind the existing `Exchanger` port whose signature is untouched. No new
  import crosses the ADR 011 boundary.
- [x] **Persistence Patterns**: N/A — no persistence.

*All BLOCKING checks pass. The ADR is a Phase 2 deliverable and gates implementation.*

**Post-Design Re-check (after Phase 1)**: Re-evaluated against the generated artifacts. No new
entity, configuration key, API, or persistence surface emerged during design. The one gate that
moved is ADRs, which design confirmed as required rather than optional: R4 shows the change alters
the service's trust boundary, and `internal/extproc/AGENTS.md` documents an invariant this feature
deletes. No Complexity Tracking entries are needed.

## Project Structure

### Documentation (this feature)

```text
specs/043-extproc-metadata-input/
├── plan.md                                 # This file
├── research.md                             # Phase 0 — R1..R10, all unknowns resolved
├── data-model.md                           # Phase 1 — wire contract, types, telemetry vocabulary
├── quickstart.md                           # Phase 1 — operator + developer validation guide
├── contracts/
│   └── extproc-metadata-input.md           # Phase 1 — gateway ↔ ExtProc metadata contract
├── checklists/
│   └── requirements.md                     # Existing
└── tasks.md                                # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
adrs/
└── 036-extproc-metadata-token-exchange-input.md   # NEW — binding decision

internal/extproc/
├── server/
│   ├── server.go                           # MODIFIED — extraction, validation, both header paths
│   ├── server_test.go                      # MODIFIED — fixtures migrated; 7 obsolete tests removed
│   ├── security_test.go                    # MODIFIED — validateResourceURI message assertions
│   ├── circuit_breaker_test.go             # MODIFIED — request fixture migrated
│   ├── exchanger_test.go                   # MODIFIED — resource-URI propagation assertions
│   └── exchanger_cache_test.go             # MODIFIED — resource-URI cache-key assertions
└── AGENTS.md                               # MODIFIED — invariants 2 and 8 rewritten

tests/e2e/extproc/
├── metadata_input_test.go                  # NEW — US1-S2..S4, US2-S2, US3-S1..S2, edge cases
├── agentgateway_metadata_e2e_test.go       # NEW — US1-S1, US2-S1 through a real gateway
├── helpers/grpc_helpers.go                 # MODIFIED — WithTokenExchangeMetadata builder
├── fixtures/tokens.go                      # MODIFIED — metadata-oriented fixture docs
├── fixtures/jwks.go                        # NEW — RS256 key pair, JWKS document, token minting
├── token_exchange_test.go                  # MODIFIED — fixtures migrated
├── opa_authorization_test.go               # MODIFIED — fixtures migrated
├── request_trace_context_test.go           # MODIFIED — fixtures migrated
├── telemetry_test.go                       # MODIFIED — manual constructor migrated
├── agentgateway_e2e_test.go                # MODIFIED — gateway config and client use per-suite minted JWT
├── agentgateway_otel_e2e_test.go           # MODIFIED — reuses the valid minted-JWT client path
└── opa_agentgateway_e2e_test.go            # MODIFIED — gateway config and client use per-suite minted JWT

mocks/agentgateway/config.yaml              # MODIFIED — Compose gateway jwtAuth policy + producer

mocks/agentgateway/jwks.json                # NEW — dev JWKS, Compose-mounted read-only
mocks/agentgateway/jwks-dev-key.pem         # NEW — matching dev private key, non-secret fixture
docker-compose.yml                          # MODIFIED — read-only JWKS mount on agentgateway
docs/guides/token-exchange-gateway.md       # MODIFIED — producer contract (FR-016)
ARCHITECTURE.md                             # MODIFIED — four ExtProc sections + glossary
```

**Structure Decision**: No new package or directory. The change is confined to the ExtProc driving
adapter, its tests, the Agentgateway configurations that feed it, and the documents that describe
the contract. `internal/extproc/config/`, `internal/extproc/authorization/`, `cmd/`, the broker, the
frontend, `infra/`, and `charts/` are untouched.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | Skipped |
| **Phase 1** | Setup — dependency verification | Required |
| **Phase 2** | Design Preconditions — ADR, E2E tests (red), contract docs | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | Skipped |
| **Phase 2.5** | Foundational — extraction and validation primitives | Required |
| **Phase 3** | US1 (P1) — metadata-only cutover | Required |
| **Phase 4** | US2 (P2) — instance resource configuration | Required |
| **Phase 5** | US3 (P3) — raw-attribute removal and test cleanup | Required |
| **Phase 6** | Documentation and architecture alignment | Required |
| **Phase N** | Constitution compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **skip** — the two extraction blocks are replaced, not restructured.
  There is no behavior-preserving refactor that would make the change smaller or the PR clearer.
- [x] Phase 2.7 (entity boilerplate): **skip** — no new entities, handlers, or repositories.
### Phase 1: Setup

1. Verify the existing protobuf metadata dependencies cover the feature and require no `go.mod` or runtime-initialization change. This required precondition produces no source or configuration change.


### Phase 2: Design Preconditions

1. Write `adrs/036-extproc-metadata-token-exchange-input.md` with Context, Decision, Consequences,
   and Status: Accepted. Context must carry the R4 finding (default `preserveToken: false` strips
   the header) and the R2 finding (a failed producer expression yields an absent field). Consequences
   must state that the invariant in `internal/extproc/AGENTS.md:199-201` is superseded and that no
   compatibility window exists.
2. Write `tests/e2e/extproc/metadata_input_test.go` covering US1-S2, US1-S3, US1-S4, US2-S2, US3-S1,
   US3-S2, and the six spec edge cases.
3. Write `tests/e2e/extproc/agentgateway_metadata_e2e_test.go` covering US1-S1 and US2-S1 through a
   real agentgateway container with a `Strict` JWT policy backed by a mounted JWKS file (R7),
   following the existing `Ordered` + `BeforeAll` container-sharing pattern.
4. Add `WithTokenExchangeMetadata(subjectToken, resourceURI)` to
   `tests/e2e/extproc/helpers/grpc_helpers.go`, composable with the existing
   `WithAgentgatewayProtocol` so one request can carry both namespaces.
5. Verify the red phase: the new suites compile and fail semantically; no `Skip()` outside the
   established Docker-availability guard.

### Phase 2.5: Foundational Infrastructure

6. Add `tokenExchangeMetadataNamespace`, `subjectTokenFieldKey`, and `resourceURIFieldKey` constants
   beside the existing `agentgatewayProtocolMetadataKey` pair.
7. Add the `tokenExchangeInput` and `inputRejection` types and the `extractTokenExchangeInput`
   function per [data-model.md](data-model.md). Decoding is explicit: guard `req.MetadataContext`
   for nil, look up `FilterMetadata["aib.tokenexchange"]` (a flat key — the dot is not a path), guard
   the `*structpb.Struct` and its `Fields` map for nil, then inspect each `*structpb.Value` with
   `GetKind()` and type-assert `*structpb.Value_StringValue`. Never use `GetStringValue()` as the
   presence test: it returns `""` for every non-string kind, which collapses "absent", "wrong type",
   and "empty" into one indistinguishable case and defeats SC-005.
   Validation operates on a **temporary view** of the value: `strings.TrimSpace` for the blank check
   and an ASCII case-insensitive `bearer ` prefix test (`strings.EqualFold` on the first 7 bytes) for
   the scheme check. Neither result is ever written back — the accepted value reaches
   `Exchanger.Exchange` byte-for-byte, so an opaque token with significant surrounding whitespace is
   preserved (FR-004).
   This function is the **single** implementation of input validation. Neither header path may
   re-derive, re-check, or supplement its result; duplicated validation is how precedence bugs
   reappear.
8. Reword `validateResourceURI`'s error strings and doc comment from `:path` to source-neutral
   "resource URI" wording (R10), and update the message assertions in
   `internal/extproc/server/security_test.go`. Leaving `"empty :path"` in logs would keep the
   obsolete contract alive in exactly the diagnostics operators read first.
9. Add the two 503 response builders on top of the existing `immediateResponse` helper.

### Phase 3: Metadata-Only Exchange (US1 — P1)

10. Rewrite `processRequestHeaders` (non-OPA): start the observation, call
    `extractTokenExchangeInput`, map a rejection to its 503 and telemetry outcome, then exchange with
    the metadata values. Delete the `:path`/`:scheme`/`:authority` reads and the `extractBearerToken`
    pass-through branch. **Retain** `extractProtocolFromMetadata` and its `"mcp"` default after input
    validation — protocol still selects MCP URL-elicitation versus 503 in `exchangeErrorResponse`,
    and that re-authentication behavior is unchanged by this feature.
11. Rewrite `processRequestHeadersOPA`: begin the observation, then call the same
    `extractTokenExchangeInput`, ahead of the protocol-metadata check and the MCP transport check
    (R5). Both rejections return their 503 directly and return `nil` state, so a misconfigured
    gateway can never reach OPA and surface as a 403 or an allow. For header-only requests
    (`endOfStream=true`) the exchange stays deferred to `processHeadersOnlyOPA`: validation has
    already run, and the validated values are carried in `requestState` for the post-allow exchange.
    `processHeadersOnlyOPA` reads only `state.subjectToken`/`state.resourceURI` and never touches
    the Authorization header.
12. Rename `requestState.bearerToken` to `subjectToken` and populate it **only** from
    `tokenExchangeInput`. No raw header value may reach `requestState`.
13. Delete `extractBearerToken` and `buildResourceURI`.
14. Update the stale doc comments that still narrate the removed contract: the `server.go` package
    comment, the `Process` comment, and the `processRequestHeaders` comment block (`server.go:383-391`,
    which still lists "No Bearer token → pass through (FR-009)" and "Empty/invalid :path"). Misleading
    guidance in the nearest source outlives any document.
15. Add unit coverage in `server_test.go` for extraction and validation: present/absent namespace,
    nil `MetadataContext`, absent field, each non-string `structpb` kind, empty and whitespace-only
    values, `Bearer `/`bearer `/`BEARER ` prefixes, FR-015 precedence when both values are invalid,
    verbatim propagation of a value with surrounding whitespace, and the absence of any pass-through
    response from either header path. Cover all three OPA shapes: body-bearing, header-only, and
    body-phase.

### Phase 4: Instance Resource Configuration (US2 — P2)

16. Confirm `resource_uri` reaches `Exchanger.Exchange` unmodified and drives the cache key
    (`tokenCacheKey{subjectToken, resourceURI}`) with no other behavior change.
17. Add the `jwtAuth` policy and the `aib.tokenexchange` producer to all three Agentgateway

    configurations. **Their JWKS provisioning uses two different mechanisms** (R8):
    - **Testcontainer configs** — `tests/e2e/extproc/agentgateway_e2e_test.go:378-393` and
      `tests/e2e/extproc/opa_agentgateway_e2e_test.go:834-851` generate `/config.yaml` at runtime.
      Add a second `testcontainers.ContainerFile` holding the suite-generated JWKS document and
      point `jwks.file` at it (R7). Keys are generated per suite run; nothing is committed.
    - **Compose config** — `mocks/agentgateway/config.yaml` is a repository file bind-mounted
      read-only (`docker-compose.yml:250-252`) and cannot reference a testcontainer path. Commit
      `mocks/agentgateway/jwks.json`, add a read-only mount for it in the `agentgateway` service
      beside the existing `config.yaml` mount, and reference it with `jwks.file`.
18. Commit `mocks/agentgateway/jwks-dev-key.pem`, the RSA private key matching
    `mocks/agentgateway/jwks.json`, with a header comment marking it a non-secret development
    fixture. Without it the Compose gateway boots but rejects every request, since no one can mint a
    token it will accept. Document the minting step in the guide's development section.
19. Set `preserveToken: false` **explicitly** in every gateway configuration rather than relying on
    the upstream default. This pins the property the feature depends on — the validated credential
    is stripped from the request, so dynamic metadata is the only carrier that reaches ExtProc — and
    makes the E2E prove it rather than assume it. The gateway's own `Claims` copy is unaffected, so
    `jwt.rawToken.unredacted()` still resolves (R4).
20. Add the E2E RS256 key-pair fixture, JWKS document, and token-minting helper in
    `tests/e2e/extproc/fixtures/jwks.go`. Generated once per suite, reachable without a network
    listener, and independent of the committed Compose fixture. After the generated configurations
    enable Strict JWT validation, migrate the shared `connectAgentgwMCPClient` used by
    `agentgateway_e2e_test.go` and `agentgateway_otel_e2e_test.go`, plus the separate OPA
    client/helper in `opa_agentgateway_e2e_test.go`, to use those minted tokens. This T025a work
    depends on the fixture and generated configuration, and completes before the Phase N E2E run.

### Phase 5: Raw-Attribute Removal (US3 — P3)

21. Remove the seven tests that assert the deleted semantics:
    `TestServer_Process_NoBearerToken_PassThrough`, `TestServer_Process_NonBearerAuth_PassThrough`,
    `TestServer_Process_EmptyPath_Returns503`, `TestServer_Process_RelativePath_Returns503`,
    `TestServer_Process_NonHTTPPath_Returns503`, `TestServer_Process_MalformedPathWithSecrets_Returns503`,
    and `TestServer_OPA_NoBearerToken_PassThrough`. Their replacements live in Phase 3 unit tests and
    the Phase 2 E2E suite; do not re-pin them to new wording.
22. Migrate the remaining ~50 unit-test fixtures and the E2E fixtures to metadata, keeping raw
    `:method` headers where OPA transport checks consume them. `tests/e2e/extproc/token_exchange_test.go`
    asserts the obsolete contract throughout (US1 `:74,:109,:136`; US2 `:174,:227`; US3 `:279`; edge
    `:336,:361,:397,:425,:548`; scopes `:464,:503`) — every one of those requests must move to
    metadata, and `:336`/`:361` must become rejection coverage rather than pass-through coverage.
23. Add the adversarial case asserting FR-011: a request with a valid `Authorization` header and a
    valid `:path` but no metadata is rejected, and the exchanger is never called.

### Phase 6: Documentation and Architecture

24. Rewrite the two obsolete steps in `docs/guides/token-exchange-gateway.md:43,55-56` and add the
    producer subsection inside `## How the exchange flows`, before `### Fail-closed behavior`,
    including the `.unredacted()` warning, the `mode: strict` requirement, and the JWT-before-ExtProc

    ordering (FR-016, FR-017).
25. Update ARCHITECTURE.md at `:830-835`, `:871-875`, `:878-895`, and `:944`, plus the `ExtProc`
    glossary entry. ARCHITECTURE.md — not `AGENTS.md` — is the architecture source of truth
    (Principle II); the AGENTS.md edit follows it, it does not substitute for it.
26. Rewrite invariant 8 in `internal/extproc/AGENTS.md:199-201` ("Resource URI construction") to
    describe metadata input, remove the no-Bearer pass-through described in invariant 2, and
    reference ADR 036.

### Phase N: Constitution Compliance

27. `just check`, then `just extproc-test`, then `just test-e2e-extproc`, then
    `just compose-extproc-up` to confirm the development gateway still boots (R8).
28. Confirm every acceptance scenario has a green `It()`, no `Skip()` was added, and no credential
    or raw URI appears in any log line, span attribute, metric label, or response body produced by
    the new paths.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/extproc/metadata_input_test.go` and
`tests/e2e/extproc/agentgateway_metadata_e2e_test.go`

**Framework**: Ginkgo/Gomega following `tests/e2e/extproc/` conventions (separate suite
`TestExtProcTokenExchange`, `bootstrap.TestEnvironment`, helpers in `helpers/`, fixtures in
`fixtures/`).

**Test Organization**:
- **Top-level Describe**: `"ExtProc Metadata Input"` and `"Metadata Input via Agentgateway"`
- **Nested Context**: one per user story, plus `"Edge Cases"`
- **It blocks**: one per acceptance scenario from spec.md

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1-S1 | `tests/e2e/extproc/agentgateway_metadata_e2e_test.go` | `It("exchanges the JWT-validated raw token for the configured MCP resource")` |
| US1-S2 | `tests/e2e/extproc/metadata_input_test.go` | `It("uses only the metadata values when raw HTTP attributes conflict")` |
| US1-S3 | `tests/e2e/extproc/metadata_input_test.go` | `It("rejects the request and forwards no credential when the exchange fails")` |
| US1-S4 | `tests/e2e/extproc/metadata_input_test.go` | `It("rejects without exchanging when a required value is outside the namespace or not a string")` |
| US2-S1 | `tests/e2e/extproc/agentgateway_metadata_e2e_test.go` | `It("exchanges for the resource URL configured in the instance ExtProc policy")` |
| US2-S2 | `tests/e2e/extproc/metadata_input_test.go` | `It("returns 503 invalid_resource for missing, empty, non-string, or invalid resource metadata")` |
| US3-S1 | `tests/e2e/extproc/metadata_input_test.go` | `It("returns 503 invalid_subject_token for missing, empty, scheme-prefixed, or non-string subject metadata")` |
| US3-S2 | `tests/e2e/extproc/metadata_input_test.go` | `It("returns 503 invalid_subject_token for a raw-attribute-only request")` |
| Edge: no metadata / one value / whitespace | `tests/e2e/extproc/metadata_input_test.go` | `It("returns the defined 503 response for absent, partial, or whitespace-only metadata")` |
| Edge: `Bearer ` prefix | `tests/e2e/extproc/metadata_input_test.go` | `It("rejects a scheme-prefixed subject token without forwarding it")` |
| Edge: relative resource URL | `tests/e2e/extproc/metadata_input_test.go` | `It("rejects a resource value that is not an absolute HTTP or HTTPS URL with a host")` |
| Edge: both invalid | `tests/e2e/extproc/metadata_input_test.go` | `It("returns invalid_subject_token when both required values are invalid")` |
| Edge: unrelated namespace fields | `tests/e2e/extproc/metadata_input_test.go` | `It("ignores unrelated fields in the aib.tokenexchange namespace")` |

**Red Phase Requirements**:
- Suites compile with no errors; every `It()` asserts concrete values — `503` status, exact
  `{"error":"..."}` codes, the exact `(subjectToken, resourceURI)` pair recorded by the stub
  exchanger, and the `Authorization` value observed by the mock MCP server.
- `XIt`, `PIt`, `XDescribe`, `PDescribe`, `XContext`, `PContext`, and new `Skip()` calls are
  forbidden. The only permitted `Skip()` is the pre-existing Docker-availability guard in the
  container suites.
- No red-phase comments.

**Test Data Strategy**:
- Reuse `tests/e2e/extproc/fixtures/tokens.go` and `fixtures/configs.go`. `fixtures/tokens.go:16-25`
  documents the values as `:path` inputs; rewrite those comments to describe metadata values —
  the constants themselves stay.
- New fixtures: an RS256 key pair plus JWKS document and a token-minting helper for the Docker
  scenarios, and a non-string `structpb.Value` fixture for kind-mismatch cases.

**Test Execution Flow**:
1. Phase 2: write both suites with detailed expectations.
2. Verify red: `ginkgo -v --focus "Metadata Input" ./tests/e2e/extproc/` fails semantically.
3. Phases 2.5–5: implement.
4. Verify green as each scenario is satisfied.
5. Only fixture adjustments after that — no test-logic rewrites.

**Bootstrap Strategy**:
- Direct-gRPC scenarios use `bootstrap.TestEnvironment` (`StartWithAuthorizer` for OPA cases), with
  fresh server and storage per test.
- The gateway scenario extends the existing testcontainer helper with a second `ContainerFile` for the JWKS document and a `jwtAuth` policy in the generated configuration. Container startup is shared via `Ordered` + `BeforeAll`, matching `agentgateway_e2e_test.go` and `opa_agentgateway_e2e_test.go`.


**Helper Utilities**:
- New: `WithTokenExchangeMetadata(subjectToken, resourceURI)` on `ProcessingRequestBuilder`,
  composable with `WithAgentgatewayProtocol`; both namespaces must coexist in one
  `MetadataContext`. `BuildWithMetadata()` currently hardcodes a single namespace and needs
  generalizing.
- Existing matchers in `helpers/matchers.go` cover authorization mutation and immediate responses;
  no new matcher is required.
- Mock services: the existing mock broker, mock OAuth2 server, and mock MCP server are reused
  unchanged.

### Frontend Playwright E2E Tests

N/A — this feature changes no React UI.

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/extproc/server/server_test.go`, `internal/extproc/server/security_test.go`, `internal/extproc/server/exchanger_test.go`, and `internal/extproc/server/exchanger_cache_test.go`
- Coverage: `extractTokenExchangeInput` truth table (namespace/field presence, value kind, blank
  and whitespace-only values, case-insensitive `bearer ` prefix), FR-015 ordering, verbatim
  propagation of the accepted value, resource-URI cache-key behavior, reworded
  `validateResourceURI` messages, and the removal of the pass-through branch in both header paths.
- Strategy: table-driven, black-box `package server_test` where possible, `go test -race`.

**Integration Tests**:
- N/A — no database, no repository, no PostgreSQL adapter is touched.

**Test Coverage Goals**:
- Unit: every branch of `extractTokenExchangeInput` and both rewritten header paths.
- E2E: 100% of acceptance scenarios plus all six spec edge cases (Principle XIII).
- Frontend E2E: N/A.

## Complexity Tracking

No constitution violations. No entries.
