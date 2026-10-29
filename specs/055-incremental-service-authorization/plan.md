# Implementation Plan: Incremental Service Authorization

**Branch**: `speckit/issue-299` | **Date**: 2026-10-29 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/055-incremental-service-authorization/spec.md`

## Summary

The feature lets an MCP client recover a failed tool call on one broker-managed third-party
service, without a silent reauthorization loop. Each configured service gets a broker-only
**Virtual Service Scope**, `<public URL>/sessions/<canonical-id-or-UUID>`. The scope works in four
places:

- **Token-exchange errors** name the scope in a new `scope` member. ExtProc copies it into its
  existing 403 `insufficient_scope` challenge. When the route configures it, the challenge also
  carries an optional `resource_metadata` URL.
- **Interactive authorization** treats every requested service as required for that request only.
  It checks grant coverage and verified usability for exchange's effective provider-scope union,
  not merely configured required scopes. Protected JWE bindings survive Save and authorize re-entry. Local
  issuance and proxy share these checks.
- **Consent** highlights each requested service and its covering permission sets. Save stays
  blocked until the service is covered and its session is usable, and the server enforces the same
  rule on submission. Per-candidate contribution usability supports live unsaved selections without
  exposing raw permission-set scope lists.
- **Local issuance** exposes granted virtual scopes in the token response and the JWT `scope`
  claim. Refresh cannot widen the scope and must re-check every originally bound service before
  rotation. Successful narrowing persists narrowed `Scope` but retains all original assurance
  bindings; scope-less successors cannot restore removed scopes.
  Proxy flows strip virtual scopes before any upstream request.

The scope URL is also a new SPA page, `/sessions/<identifier>`. It is backed by an additive
detail read, and recovery links return to it after provider sign-in.

The technical approach adds one pure domain context, `internal/domain/servicescope/`, which owns
the scope grammar, requested-set resolution, eligibility, coverage, and usability evaluation. The
authorization, consent, local-issuance, and token-exchange services consume it.

Persisted unusability reuses the existing #271 rejection marker, with no schema change. A small
migration, `046`, persists **Service-Scope Bindings** that pin each granted scope to its service
UUID (FR-019).

Two Proposed ADR drafts record the decisions:

- [ADR 045](./adrs/045-service-session-recovery-return.md) partially supersedes ADR 044.
- [ADR 046](./adrs/046-virtual-service-scopes.md) introduces virtual scopes.

## Technical Context

**Language/Version**: Go 1.27.3 for the broker and ExtProc. TypeScript and React 19 with Vite 7 and
Tailwind 4 for the SPA.

**Primary Dependencies**: chi v5, `ory/fosite` v0.49.0, sqlx with pgx v5, Viper and Cobra, and
`envoyproxy/go-control-plane` for ExtProc. Ginkgo v2 and Gomega for backend E2E tests,
Playwright for frontend E2E tests, and Vitest for SPA unit tests. The feature adds no new
dependency.

**Storage**: PostgreSQL in production and the in-memory adapter for development and tests. The
only schema change is migration `046_add_service_scope_bindings`, which adds a nullable `JSONB`
column to `authorization_codes` and `refresh_token_sessions` (DEC-004). The `user_sessions` table
does not change (DEC-003).

**Testing**:

- Go unit tests, table-driven, with hand-rolled domain mocks.
- PostgreSQL integration tests for the migration and both repositories.
- Ginkgo E2E tests through the production bootstrap, including the broker plus ExtProc bootstrap.
- Playwright tests with screenshots.
- Vitest tests for the new components.

**Target Platform**: Linux containers. The broker uses the dual-server layout (ADR 004). ExtProc
is a standalone sidecar (ADR 011).

**Project Type**: Go monorepo web service with an embedded SPA and a standalone gRPC binary.

**Performance Goals**:

- Authorization or refresh with no virtual scope and no retained assurance binding adds zero I/O.
- Each requested service adds one service lookup and one session lookup. An expired token adds
  one single-flight refresh, the same cost as a real tool call.
- Candidate usability uses one verified session snapshot per service, not a provider call per set.
- The session-detail read performs no provider call.

**Constraints**:

- Security controls fail closed (Principle I, SR-001–SR-005).
- No virtual scope reaches an upstream IdP or a provider.
- No origin is derived from the request Host.
- A transient failure never persists unusability or permits authorization.
- Refresh rejections leave the refresh token valid.

**Scale/Scope**:

- 4 user stories with 19 acceptance scenarios.
- 1 new domain context and 1 new HTTP route.
- 5 additive or extended End-User API surfaces and 1 optional ExtProc metadata field.
- 1 migration, 2 ADR drafts, 2 new SPA views or components, and 1 operator guide.

**NEEDS CLARIFICATION**: None. [research.md](./research.md) resolves every design question.
Consequential choices are listed under [Approval Items](#approval-items).

## Approval Items

None of these items is approved. Each one is Proposed and awaits the maintainer's 👍 on the
pipeline's approval comment. Implementation records the approval. Details give the authoritative
text.

| ID | Type | Decision | Details |
|---|---|---|---|
| API-EXCHANGE-CHALLENGE | API | Add a `scope` member (the canonical Virtual Service Scope) to recoverable RFC 8693 errors after service resolution. ExtProc appends `scope` and an optional `resource_metadata` to its existing 403 `insufficient_scope` challenge. The elicitation body is unchanged. | specs/055-incremental-service-authorization/contracts/enduser-api.md:16-47; specs/055-incremental-service-authorization/contracts/extproc-challenge.md:40-76 |
| API-METADATA-CONFIG | API | Add an optional `aib.tokenexchange.resource_metadata_url` metadata field, configured for each Agentgateway route. There is no ExtProc YAML, environment, or CLI key. An invalid value is omitted, not rejected. | specs/055-incremental-service-authorization/contracts/extproc-challenge.md:13-38 |
| API-CONSENT | API | `GET /oauth2/authorize` accepts virtual scopes and optional JWE `session_token` for protected post-Save re-entry. It independently validates sealed request/bindings before issuance or proxy redirect and returns `invalid_scope` on identity reassignment. It returns `temporarily_unavailable` when usability cannot be verified. Consent gains requested-service arrays including baseline and per-candidate contribution usability for live selection; submitted effective provider scopes follow exchange's union/ceiling rule. Unsatisfied submission returns `400 requested service not satisfied`. Successful `redirect_url` carries the same JWE. | specs/055-incremental-service-authorization/contracts/enduser-api.md:49-78; specs/055-incremental-service-authorization/contracts/enduser-api.md:127-214 |
| API-SESSION-DETAIL | API | Add `GET /api/third-party/sessions/{service}`, where `{service}` is a canonical ID or UUID. It returns `200` with `session: null` when the service has no session, `404` for an unknown service, and `401` without a principal. The existing UUID route does not change. | specs/055-incremental-service-authorization/contracts/enduser-api.md:216-275 |
| API-LOCAL-SCOPES | API | Local responses/JWT scope and successor `Scope` equal the requested subset or predecessor current `Scope` when omitted; narrowing cannot be undone by scope-less refresh. All original bindings remain inherited as assurance-only obligations and are rechecked with grant-derived effective provider scopes even after narrowing. Widening/definitive failure returns non-consuming `invalid_scope`, transient failure non-consuming `server_error`. `client_credentials` with a virtual scope returns `invalid_scope`. | specs/055-incremental-service-authorization/contracts/enduser-api.md:80-118 |
| DEC-001 | Decision | Only UUID scopes survive canonical-ID renames, and no historical aliases are kept. A refresh bound to an old canonical scope returns `invalid_scope` without consuming the token. | specs/055-incremental-service-authorization/spec.md:607; specs/055-incremental-service-authorization/contracts/virtual-service-scope.md:61-68 |
| DEC-002 | Decision | Proxy refresh strips virtual scopes and forwards the upstream outcome unchanged. It applies no post-response usability check and does not attest to usability. | specs/055-incremental-service-authorization/spec.md:609; specs/055-incremental-service-authorization/contracts/enduser-api.md:120-125 |
| DEC-003 | Decision | Definitive provider refresh rejection stays persisted through the existing #271 marker (`refresh_token_expires_at` = rejection time) as the single unusability state for all consumers, with no schema change. Transient failures never write it. | specs/055-incremental-service-authorization/research.md:77-116 |
| DEC-004 | Decision | Schema change: migration `046_add_service_scope_bindings` adds nullable `service_scope_bindings JSONB` to codes and refresh sessions. Code bindings lie within granted `Scope`; refresh bindings remain an immutable assurance set even after durable scope narrowing. Successors inherit the map exactly and cannot widen predecessor `Scope`. | specs/055-incremental-service-authorization/data-model.md:95-128; specs/055-incremental-service-authorization/data-model.md:145-170 |
| ADR-RECOVERY | ADR | ADR 045 draft. Recovery `error_uri` returns to `/sessions/<identifier>`. The ADR records the persisted rejection marker. It supersedes only those two ADR 044 clauses. | specs/055-incremental-service-authorization/adrs/045-service-session-recovery-return.md:1-113; specs/055-incremental-service-authorization/contracts/enduser-api.md:282-289 |
| ADR-SCOPES | ADR | ADR 046 draft. Virtual Service Scope semantics, shared effective-scope usability, candidate consent checks, protected post-Save re-entry, durable local narrowing with immutable assurance bindings, proxy boundaries, identity pinning and non-advertisement. | specs/055-incremental-service-authorization/adrs/046-virtual-service-scopes.md:1-104 |

The approved API items also cover these mechanical consequences:

- End-User API `info.version` changes from `1.1.0` to `1.2.0`.
- Matching `docs/api/` and `docs/reference/token-exchange.md` text is updated.
- Existing tests that assert the old `/sessions` return target are updated under ADR-RECOVERY.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

### Design preconditions

| Check | Status | Plan |
|---|---|---|
| Domain model | Pass | [data-model.md](./data-model.md) defines these value objects: Virtual Service Scope, Service-Scope Binding, Requested Service Set, Service Requirement Outcome, and Session Usability. It also covers the extended records and the state machines. |
| Domain concepts | Pass | Add Virtual Service Scope, Requested Service Set, Service-Scope Binding, and Session Usability to the `ARCHITECTURE.md` glossary and the root `AGENTS.md` glossary. |
| Entity IDs | Not applicable | No new entity with a UUID primary key. Bindings use the existing `id.ServiceID`. |
| Configuration design | Pass | No broker key is added. The existing `server.enduser.public_url` and `oauth2_authorization_server.mode` keep their roles. The only new operator setting is the optional Agentgateway route metadata field, documented with YAML in [contracts/extproc-challenge.md](./contracts/extproc-challenge.md). |
| Configuration examples | Pass | Add `examples/config/incremental-authorization.yaml` and an Agentgateway route example, both referenced from `examples/config/README.md`. |
| Helm deployment contract | Pass (no change) | No configuration of a chart-managed workload changes. The metadata field belongs to the Agentgateway route deployed with ExtProc (ADR 011 and ADR 036 boundary). It must not be added to the broker chart. |
| API design first | Pass with precondition | Contract deltas are in [contracts/](./contracts/). Implementation copies the approved deltas into `api/enduser/openapi.yaml` before writing code. |
| API documentation | Pass | The End-User OpenAPI changes, and the admin API does not. |
| API changes confirmed | **Pending** | Items API-EXCHANGE-CHALLENGE, API-METADATA-CONFIG, API-CONSENT, API-SESSION-DETAIL, and API-LOCAL-SCOPES need maintainer approval (Principles IV and X). |
| Database design | Pass with precondition | Migration 046 is designed in [data-model.md](./data-model.md). It needs DEC-004 approval. |
| E2E acceptance tests | Pass | 19 scenarios map one-to-one to `It()` blocks, as the [mapping](#scenario-mapping) shows. All are written before implementation. |
| E2E mapping | Pass | One `It()` per scenario, with no shared state and a production bootstrap. |
| E2E red phase | Pass | Each `It()` asserts concrete statuses, JSON members, header attributes, upstream-recorded forms, or UI locators. |
| Frontend Playwright E2E | Pass | `tests/e2e/frontend/incremental_authorization_test.go` covers both consent states and both service-page states. |
| Frontend screenshots | Pass | Four screenshots that match the spec's Wireloom states. |
| Concrete example journeys (Principle XIV) | Pass | The spec's Journeys 1–4 already cover configuration, HTTP, Wireloom, and JWT claims. Planning aligned the Journey 2 and Journey 4 examples with the designed contracts (spec Clarifications, Session 2026-10-29 planning). |

### Implementation considerations

| Check | Status | Plan |
|---|---|---|
| Security-first | Pass | All new checks fail closed. Eligibility is never widened by a scope. The JWE and the bindings pin identity. Origins come from configuration only. Challenge values are validated. Transient failures are distinct from definitive ones. Audit logging is structured and contains no credentials (SR-005). |
| Architecture docs | Pass | Update the `ARCHITECTURE.md` sections on OAuth2 server mode, token exchange recovery, consent, ExtProc challenge, and session management. Also update the glossary, the ADR index, and the versioning note. |
| ADRs | Pass with precondition | ADR 045 (ADR-RECOVERY) and ADR 046 (ADR-SCOPES) are Proposed drafts in `adrs/` inside this feature. The first task of implementation records the approval and moves them to `adrs/`. ADR 044 is not edited. |
| Library-first security | Pass | No new cryptography. The design reuses the JWE token service, fosite, and PKCE. |
| Zalando guidelines | Pass | Additive fields use snake_case and nullable objects. The new route is a resource read. Errors follow the existing handler error shape. |
| End-user API docs | Pass | Update `docs/api/enduser-api.md` and `docs/reference/token-exchange.md`. Add `docs/guides/incremental-authorization.md` (FR-017) and link it from `docs/guides/token-exchange-gateway.md`. |
| Migration testing | Pass | Apply, rollback, and repeat in `tests/integration/migrations`. Round-trip tests for both adapters. |
| Hexagonal architecture | Pass | `servicescope` has no I/O and depends on interfaces. Handlers only parse and serialize. All wiring is in `internal/app/builder.go`. There are no imports between adapters. ExtProc stays independent of broker packages. |
| Persistence patterns | Pass | Existing repositories gain one column in explicit column lists. No new repository and no `SELECT *`. |
| Configuration system | Pass | No new broker configuration. ExtProc loading does not change. |
| Feature scope | Pass | Every task traces to an FR, SR, SC, or ADR. Unrelated findings are deferred (see [research.md](./research.md#deferred-findings-outside-this-feature-not-tasks)). |

### Post-design re-check

The gate passes once the approval items are approved.

- `servicescope` is a genuine bounded context. It owns one invariant that four services share, so
  it is not an anemic proxy.
- The design adds no new port in `internal/ports/` and no new repository. The usability checker
  and the service lookup are small interfaces that the consumer declares. The builder satisfies
  them with existing domain services and repositories.
- Every externally visible change appears in [contracts/](./contracts/) and is registered above.

## Project Structure

### Documentation (this feature)

```text
specs/055-incremental-service-authorization/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── virtual-service-scope.md   # Grammar, resolution, emission, durability
│   ├── enduser-api.md             # End-User OpenAPI delta
│   └── extproc-challenge.md       # ExtProc metadata field and challenge delta
├── adrs/
│   ├── 045-service-session-recovery-return.md   # Proposed; partially supersedes ADR 044
│   └── 046-virtual-service-scopes.md            # Proposed
├── wireframes/                    # Spec Wireloom renders (existing)
├── checklists/requirements.md
└── tasks.md                       # Created by /speckit-tasks
```

### Source code and documentation affected

```text
adrs/045-*.md, adrs/046-*.md                      # Moved from the feature directory after approval
api/enduser/openapi.yaml                          # Contract delta, version 1.2.0
migrations/046_add_service_scope_bindings.{up,down}.sql

internal/domain/
├── servicescope/                                 # New bounded context (no I/O)
│   ├── scope.go                                  # VirtualServiceScope Parse/Format
│   ├── requested_set.go                          # Resolve, dedupe, eligibility, bindings
│   ├── coverage.go                               # Coverage rule extracted from token exchange
│   ├── evaluate.go                               # ServiceRequirementOutcome
│   └── errors.go
├── oauth2/service.go                             # Requested services in HandleAuthorization; proxy stripping
├── oauth2/sessiontoken/claims.go                 # requested_service_ids, service_scope_bindings
├── oauth2server/{provider.go,fosite_storage.go,fosite_client.go}  # Scope strategy, bindings, refresh recheck before rotation
├── consent/service.go                            # Detail coverage, submission enforcement
├── tokenexchange/service.go, errors.go           # scope member on recoverable errors; shared coverage
├── oauth2session/service.go                      # CheckServiceUsability; ServiceAuthorizeURL return target
└── storage/{authorization_code.go,refresh_token_session.go}  # ServiceScopeBindings

internal/adapters/
├── http/enduser/{oauth2_token.go,oauth2_token_proxy.go,proceed_strategy.go}
├── http/handlers/consent/{agent_detail_handler.go,grants_handler.go}
├── http/oauth2_sessions/handler.go               # GET /third-party/sessions/{service}
└── storage/{memory,postgres}/{authorization_code*,refresh_token_session*}.go

internal/app/builder.go                           # Wire servicescope into consumers
internal/extproc/server/{exchanger.go,reauth_challenge.go,server.go}

web/src/
├── App.tsx                                       # /sessions/:service route
├── pages/ThirdPartySessionDetailPage.tsx         # New
├── pages/AgentGrantDetailPage.tsx                # Requested-service state, Save gating
├── components/consent/RequestedServiceBanner.tsx # New (application component)
├── components/consent/PermissionSetsList.tsx     # Candidate and requested markers
└── services/api/{consent.ts,sessions.ts}

docs/guides/incremental-authorization.md          # New (FR-017)
docs/guides/token-exchange-gateway.md, docs/api/enduser-api.md, docs/reference/token-exchange.md
examples/config/incremental-authorization.yaml, examples/config/README.md
ARCHITECTURE.md, AGENTS.md, internal/AGENTS.md, internal/domain/AGENTS.md, internal/extproc/AGENTS.md, web/AGENTS.md

tests/
├── e2e/incremental_authorization_test.go         # US1, US3 S1/S4, US4 S2–S3
├── e2e/incremental_authorization_proxy_test.go   # US3 S2–S3
├── e2e/incremental_authorization_gateway_test.go # US1 S1, US4 S1 (broker + ExtProc bootstrap)
├── e2e/service_session_page_test.go              # US2 S1–S4 (API + return flow)
├── e2e/incremental_authorization_docs_test.go    # US4 S4 (guide and discovery contract)
├── e2e/frontend/incremental_authorization_test.go
├── e2e/pages/{consent_page.go,sessions_page.go}  # New locators
└── integration/migrations/…                      # 046 apply, rollback, repeat
```

**Structure Decision**: Business rules live in the new `internal/domain/servicescope/` context.
Existing domain services consume it through constructor-injected, consumer-declared interfaces, so
domain imports stay narrow. Handlers stay thin. ExtProc changes stay inside
`internal/extproc/server`, which keeps the ExtProc package-isolation invariant.

## Design and Implementation Approach

1. **Scope grammar and requested set** ([contracts/virtual-service-scope.md](./contracts/virtual-service-scope.md)):
   `servicescope.Parse` and `Format` use exact prefix matching on the configured public URL.
   `Resolve` deduplicates scopes, checks eligibility, and verifies any binding.
2. **Coverage rule extraction**: Move the per-service coverage computation from
   `tokenexchange.resolveEffectiveScopes` into `servicescope`, without changing behavior. Existing
   token-exchange tests guard the move. The authorization and consent services reuse the rule.
3. **Usability** ([research R3](./research.md#r3--session-usability-check-fr-008-dec-003)):
   `CheckServiceUsability` wraps `GetValidAccessToken` and checks the distinct effective-scope input
   from the shared selected-grant/ceiling rule. All-scopes and agents without explicit requirements
   use the full selected union. Valid access with sufficient scopes and matching identity remains
   usable without refresh or with expired refresh; lack of refresh matters only once access expires.
   Classify definitive/transient outcomes without changing the rejection marker.
4. **Authorization** ([research R5](./research.md#r5--authorization-endpoint-flow-with-requested-services-fr-007-fr-013)):
   Requested services are parsed after `redirect_uri` validation. They are evaluated after the
   grant and mandatory checks, and they redirect to consent with the sealed IDs and bindings. Proxy
   agents get their upstream scope stripped. Local agents get a code that carries the bindings.
   After Save, independently authenticate the same JWE on authorize re-entry, compare the original
   request and re-resolve against sealed bindings before either result; never fall back to unbound
   resolution or extend the original expiry.
5. **Consent** ([research R11](./research.md#r11--consent-api-additions-fr-009fr-011-api-consent)):
   The detail response derives `requested_service_ids` and `requested_service_coverage` from the
   validated JWE. Baseline and per-candidate effective-contribution usability use one verified session
   snapshot, so live toggles gate Save without raw scope disclosure. Submission recomputes the selected
   effective union before persistence and returns the original URL plus the same `session_token`.
   The provider-login round trip uses existing consent-state transport.
6. **Local issuance** ([research R6](./research.md#r6--local-refresh-fr-012-fr-020-api-local-scopes)):
   - The scope strategy lets virtual scopes pass the per-client allow-list.
   - Bindings flow from the code to the refresh session, and from each predecessor to its
     successor unchanged, including assurance-only bindings outside narrowed current `Scope`.
   - Successful narrowing returns and persists the requested subset; omitted scope uses predecessor
     current `Scope`. Later scope-less refresh cannot restore removed privileges. Successor storage
     validates exact binding inheritance and scope subset; every binding is rechecked before rotation.
   - `HandleRefreshToken` runs the widening check and the recheck after fosite validation and
     before `PopulateTokenEndpointResponse`.
   - `client_credentials` rejects virtual scopes.
7. **Proxy refresh** ([research R7](./research.md#r7--proxy-refresh-dec-002-fr-021)): The proxy
   token path rewrites the forwarded `scope` form value and changes nothing else.
8. **Token exchange and ExtProc** ([contracts/extproc-challenge.md](./contracts/extproc-challenge.md)):
   The broker adds `scope` to recoverable errors after service resolution, and changes the
   recovery return target (ADR 045). ExtProc decodes `scope`, reads the optional metadata field,
   and appends validated attributes.
9. **Session page** ([contracts/enduser-api.md §7](./contracts/enduser-api.md#7-get-apithird-partysessionsservice-service-session-detail-api-session-detail)):
   A domain read, `OAuth2SessionService.GetServiceSessionDetail(principal, identifier)`, resolves
   the identifier, the session or `nil`, and the principal-owned dependent agents. The handler
   maps the result to JSON. The SPA route renders States A and B with existing session actions.
10. **Audit** (SR-005): The broker writes structured, credential-free log events:
    - `incremental_authorization_required` (agent, principal, service IDs, reason);
    - `requested_service_submission_rejected`;
    - `refresh_service_recheck_failed` (reason, plus `definitive` or `transient`);
    - the existing refresh-rejection log for usability transitions.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | Skipped |
| **Phase 1** | Setup: record approvals and move ADR drafts | Yes |
| **Phase 2** | Design preconditions: glossary, configuration examples, OpenAPI, migration, design-system review, red E2E tests | **MANDATORY** |
| **Phase 2.7** | Entity boilerplate | Skipped |
| **Phase 2.5** | Foundations: `servicescope` context, coverage extraction, `CheckServiceUsability`, bindings persistence | Yes |
| **Phase 3+** | User stories in priority order: US1 → US3 → US2 → US4 | Yes |
| **Phase N** | Constitution compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **skip**. The coverage extraction is small and behavior-preserving.
  Token-exchange tests cover it, and it belongs to Phase 2.5 as part of the feature path.
- [x] Phase 2.7 (entity boilerplate): **skip**. The feature has no new entity, repository, or CRUD
  surface. It extends two existing records with one field each.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Framework**: Ginkgo and Gomega with the production bootstrap (`tests/e2e/bootstrap/`). Each test
gets a fresh server and in-memory storage. Each file references `specs/055-incremental-service-authorization/spec.md`.

**Test Organization**: `Describe("Incremental Service Authorization")` → `Context` for each
precondition → one `It` for each scenario.

#### Scenario Mapping

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1 S1 | `tests/e2e/incremental_authorization_gateway_test.go` | `It("returns the service scope and recovery URL and the gateway challenges with insufficient_scope while keeping URL elicitation")` |
| US1 S2 | `tests/e2e/incremental_authorization_test.go` | `It("refuses refresh widening with invalid_scope, keeps the refresh token valid, and sends interactive step-up to consent")` |
| US1 S3 | `tests/e2e/incremental_authorization_test.go` | `It("identifies the requested service and every covering set while preserving selections and end date")` |
| US1 S4 | `tests/e2e/frontend/incremental_authorization_test.go` | `It("lets the user choose covering sets, log in, and save without auto-selecting other optional sets, then the retried call succeeds")` |
| US1 S5 | `tests/e2e/incremental_authorization_test.go` | `It("rejects a grant submission that does not cover the requested service even with a usable session")` |
| US1 S6 | `tests/e2e/incremental_authorization_test.go` | `It("requires every requested service and explains each unsatisfied one without changing agent requirements")` |
| US1 S7 | `tests/e2e/incremental_authorization_test.go` | `It("uses a successfully refreshed session and continues without login")` |
| US2 S1 | `tests/e2e/service_session_page_test.go` | `It("shows the service with no session and returns to the same page after provider sign-in")` |
| US2 S2 | `tests/e2e/frontend/incremental_authorization_test.go` | `It("shows the active session, refreshes it, and terminates it back to the no-session state")` |
| US2 S3 | `tests/e2e/service_session_page_test.go` | `It("requires authentication and returns not found for an unknown service")` |
| US2 S4 | `tests/e2e/service_session_page_test.go` | `It("returns from the recovery link to the service page and the next exchange succeeds without agent authorization")` |
| US3 S1 | `tests/e2e/incremental_authorization_test.go` | `It("rechecks granted service scopes on local refresh and returns invalid_scope once the service is unusable")` |
| US3 S2 | `tests/e2e/incremental_authorization_proxy_test.go` | `It("verifies requested services then redirects upstream with only ordinary scopes")` |
| US3 S3 | `tests/e2e/incremental_authorization_proxy_test.go` | `It("strips virtual scopes from proxy refresh, forwards the rotated upstream token, and the next exchange still challenges")` |
| US3 S4 | `tests/e2e/incremental_authorization_test.go` | `It("includes the granted virtual scope in the token response and JWT scope and rechecks it on scope-less refresh")` |
| US4 S1 | `tests/e2e/incremental_authorization_gateway_test.go` | `It("includes resource_metadata only when the route configures it, keeping scope and recovery link otherwise")` |
| US4 S2 | `tests/e2e/incremental_authorization_test.go` | `It("resolves both identifier forms, keeps UUID scopes across rename, and fails old canonical refresh without consuming it")` |
| US4 S3 | `tests/e2e/incremental_authorization_test.go` | `It("rejects unknown, unconfigured, and malformed service scopes and treats external lookalikes as ordinary scopes")` |
| US4 S4 | `tests/e2e/incremental_authorization_docs_test.go` | `It("documents the initial challenge, metadata variant, recovery paths, bounded retries, and mode limits, and discovery omits virtual scopes")` |

Regression variants remain within their existing owning scenarios: US1 S4/S7 cover effective-scope
failure with valid access and empty configured required scopes for all-scopes/no-requirements agents;
US4 S2 includes reassignment after Save before protected authorize re-entry; US3 S1 covers full →
narrowed → scope-less refresh with all assurance bindings retained and no scope restoration. T024
adds mutually exclusive valid-access/absent-or-expired-refresh unit cases.

US4 S4 is a documentation scenario. Its `It` asserts the live discovery document (no virtual
scopes). It also asserts that the documented route example in
`docs/guides/incremental-authorization.md` parses and matches the contract's metadata field and
scope format. This follows the pattern of existing tests that check documentation against the
contract.

**Red phase requirements**:

- Each test compiles against minimal stubs: new fields and an empty route that returns `501`.
- Each test fails on concrete assertions.
- No `XIt`, `PIt`, `Skip`, or red-phase comments.

**Test data strategy**:

- Extend `tests/e2e/fixtures/` with the spec's shared cast: the GitHub Production service with
  canonical ID `github`, the Identity Access, GitHub Read Access, and GitHub Issue Triage
  permission sets, and the Data Analysis Assistant agent with an optional GitHub requirement for
  `repo user:email`.
- Reuse the mock third-party provider. It needs a switch that returns `invalid_grant` on refresh.
- Reuse the mock upstream OAuth2 server in proxy mode. It records the received `scope` form value.

**Bootstrap strategy**:

- Broker-only tests use `bootstrap.ServerFactory`.
- Gateway scenarios use `bootstrap.ExtProcEnvironment`, as `extproc_approval_journey_test.go`
  does. The challenge is asserted at the ExtProc gRPC immediate response.
- Existing tests that assert the `/sessions` return target are updated under ADR-RECOVERY.

### Frontend Playwright E2E Tests

**Test Location**: `tests/e2e/frontend/incremental_authorization_test.go`, using the `consent_page.go`
and `sessions_page.go` page objects, which gain the new locators.

| UI Scenario | Playwright Test Location | Screenshot Filename |
|---|---|---|
| Consent State C: requested GitHub needs login | `tests/e2e/frontend/incremental_authorization_test.go` | `tests/e2e/screenshots/consent_requested_service_needs_login.png` |
| Consent State D: covered and connected, Save enabled | `tests/e2e/frontend/incremental_authorization_test.go` | `tests/e2e/screenshots/consent_requested_service_ready.png` |
| Service page State A: no active session | `tests/e2e/frontend/incremental_authorization_test.go` | `tests/e2e/screenshots/service_session_needs_login.png` |
| Service page State B: active with actions | `tests/e2e/frontend/incremental_authorization_test.go` | `tests/e2e/screenshots/service_session_active.png` |

### Validation Gate

The generated [tasks.md](./tasks.md) requires compiling semantic-red acceptance tests before
feature logic, test-first unit/integration pairs, `just check`, the matching unit/frontend/ExtProc
and integration suites, and the complete feature journeys. Completion requires `just verify`,
including real PostgreSQL migration/repository validation and all E2E suites. Infrastructure that
cannot run is an implementation blocker, not a skipped passing gate. Maintained screenshots use
the existing capture-enabled serial harness, as [quickstart.md](./quickstart.md) describes.

### Unit and Integration Tests

**Unit tests**:

- `servicescope`: grammar tables, resolution, eligibility, distinct effective-scope unions for
  fixed/all-scopes/no-requirements, candidate usability composition, and binding mismatch.
- `oauth2`: authorize outcomes, protected post-Save JWE re-entry and proxy stripping.
- `oauth2server`: durable narrowing then scope-less refresh, no restoration, full assurance-set
  effective-scope recheck before rotation, widening, and `client_credentials`.
- `consent`: baseline/candidate snapshot usability, unsaved selection gating and fresh submission.
- `tokenexchange`: the `scope` member matrix.
- `oauth2session`: mutually exclusive usability with valid access/absent-or-expired refresh,
  effective-scope failures despite empty configured scopes, and the return URL.
- Handlers: the session-detail route and the consent fields.
- ExtProc: challenge assembly and metadata validation.
- Vitest: `RequestedServiceBanner`, Save gating, and `ThirdPartySessionDetailPage`.

**Integration tests**:

- Migration 046 apply, rollback, and repeat.
- PostgreSQL round trips for bindings on both repositories, including `CreateSuccessor`
  inheritance, narrowed successor scope with all assurance bindings retained, scope-subset enforcement
  and rejection of changed/added successor bindings.

**Coverage goals**: All 19 acceptance scenarios are covered. Every new domain branch has a unit
test, including the transient and definitive classifications. Both adapters are covered.

## Complexity Tracking

No constitution violation requires justification. The only schema change, migration 046, is a
standard atomic migration and is registered as DEC-004.
