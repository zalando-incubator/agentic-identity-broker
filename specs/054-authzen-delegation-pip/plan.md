# Implementation Plan: AuthZEN Delegation Policy Information Point

**Branch**: `speckit/issue-289` | **Date**: 2026-10-27 (revised 2026-10-29 and 2026-11-06) | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/054-authzen-delegation-pip/spec.md`

## Summary

The broker gains an AuthZEN 1.0 single Access Evaluation endpoint, `POST /access/v1/evaluation`,
and its discovery document, `GET /.well-known/authzen-configuration`, on the end-user server.
Both exist whenever token exchange is configured. An authenticated gateway learns whether a
verified user's delegation to an agent covers one protected resource. The answer is
credential-free: a permit with a Delegation Context of UUIDs, optional canonical IDs, effective
scopes, the grant version, and the grant expiry, or a denial with a bounded diagnostic and a
consent URL.

Evaluation and RFC 8693 exchange share one authoritative resolution. Steps 2–9a of
`TokenExchangeService.Exchange` move unchanged into `tokenexchange.DelegationResolver`. A new
`AccessEvaluationService` classifies the outcomes and computes a fixed ≤60 s permit freshness.
Evaluation reuses the unchanged caller CEL policy with
`request.grant_type = urn:ietf:params:oauth:grant-type:token-exchange`.

The standalone ExtProc gains an opt-in `PermitResolver`, its fourth port. At startup it
discovers and verifies the PDP. It caches only permits, until a deadline of
`requestStart + min(max-age, max_staleness)` that is computed once. A permit without a usable
positive `max-age` fails closed. ExtProc supplies the delegation to OPA before every policy
decision: concurrently with exchange on the body path, and before policy and exchange on the
header-only path. Context that expires before policy use is rejected with 503. Every failure is
closed. ExtProc decodes the permit into its own typed struct and adds `principal`, `agent`,
`resource`, `grant`, and `permission_sets` directly to `input.context`. On both enabled paths,
it fills the currently retained compatibility `granted_permission_sets` and
`granted_permission_sets_available: true` from the permit (research R15).

The `granted_permission_sets` field of the exchange response is marked deprecated. Its removal
is a later, separately approved 2.0.0 contract change.

In enabled mode the permit governs result-handle reuse, not provider-token capability or exchange provenance (Proposed ADR-2 and D-8; research R19). Binding compares permit principal/agent UUID, scope sets and grant-update instants. Exchange response identities must match the permit; mismatch or absence returns terminal API-7 503 without an identity-repair retry. Approval identity remains exchange-derived under ADR 036. Opaque entry identities preserve same-token replacements; a synchronized binding cell shared with followers permits one atomic first claim, never rebinding. A mismatched additional-exchange delivery fails closed with Proposed API-7 503.

## Clarifications — apply-review 2026-11-11

Reason: [F1/P1–F4/P4 proposal approved by mjungsbluth](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6751402783) at `2026-11-11T18:57:51Z`.

- F1/P1: include principal/agent UUID in `ExchangeResultBinding` equality and check the exchange response against the permit before handle claim. Identity mismatch/absence returns terminal API-7 503; scope/grant changes retain the bounded retry. Remove the permit override from approval state, preserving ADR 036 authority and session extraction. Test each missing/different field on both paths using the production exchanger; US3.1 expects 503, no mutation/body or approval creation. This supersedes the historical 2026-11-09 override below.
- F2/P2: restore merged ADR 028's original Accepted Status, moving the ADR 048 partial-supersession note into a banner after the header. Revise same-PR ADR 048 §§9.1/10 with the supplied approval, not ADR 036.
- F3/P3: open [Follow-ups for #292](https://github.com/zalando-incubator/agentic-identity-broker/issues?q=is%3Aissue%20in%3Atitle%20%22Follow-ups%20for%20%23292%22) for the deferred `/oauth2/token` grant-version/effective-scope redesign and its consistency contract; link ADR 048 §10 and FR-020. Do not implement the future API or infer its guarantees here.
- F4/P4: rename `token_binding*.go` to `exchange_result_binding*.go` and update the two diagnostic messages; keep the API-7 error text and telemetry keys.

## Clarifications — apply-review 2026-11-09

Reason: [selected proposal approved by mjungsbluth](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6729032045) at `2026-11-09T20:20:21Z`.

- F2/P2: after enabled-mode exchange/binding succeeds, set the approval state's principal and agent UUID from the request permit. OPA already uses it. Keep disabled identity and session extraction unchanged; regression tests exercise conflicting and missing exchange identities through the headers/body/approval flow.
- F3/P3: correct the released end-user API baseline to `1.2.0` and publish `1.3.0`. Retain response fields/deprecation; revise ADR 048 §8 and register its partial supersession in ADR 028 and the architecture index. Earlier version/approval entries below are historical, not the current release contract.
- F4/P4: return `errInvalidGrantTimestamp` from `exchangeWithDelegation` and match with `errors.Is`. Preserve ordinary exchange error mapping for unrelated `delegation.Error` values. F5/P5: document the canonical `error_kind` key and all three unavailable-event emitters in observability §2.1.
- F6/P6: rename the local result-reuse value to `ExchangeResultBinding`, and both ExtProc resolver port and concrete cache to `PermitResolver`, including constructors, wiring and test doubles. Update ADR 048, instructions, contracts and both glossaries. Preserve existing wire text and telemetry names.
- F1/P1 remains unresolved. Returning only exchange grant version and effective scopes does not specify how the separately fetched permission-set/context snapshot becomes consistent, or what happens when versions differ. Removing reconciliation without resolving that would replace the documented limitation with an unproven guarantee. No F1 API, ADR or broker implementation changes are included, and its alternative is not selected.

## Current task reconciliation and approval disposition

The original 102-task inventory and D-6 alternative validation are historical completion records in quickstart §§11–12, not evidence for new work. Original API-2/D-5/D-6 approval is recorded on the 2026-11-05 approval comment; D-6 is closed by its completed alternative validation, not by performing the missing original sequence. F4–F6 are recorded complete in §13. T103/F2 and T104/F3 remain required open work.

The current 122-task inventory retains 104 historical checked markers and 18 unchecked tasks, including T001, T103/T104 and T108–T122. It covers the Proposed binding revision, telemetry mapping, canonical-field guidance and US3.11–US3.13 with genuine test-first evidence. No new approval or implementation is recorded here.

## Clarifications — analysis remediation (run 42232121230/1)

- I1/U1: delete by result identity, never credential bytes. All cache/flight/non-cacheable results carry shared first-claim binding ownership. A failed delete preserves the replacement; inspect it once, then at most one extra exchange. Revalidate that delivery; conflict is terminal API-7 503. Main alternative: rebind/loop, rejected as unsafe. Contract §3.1 and model §3.6 are authoritative.
- I2: FR-020 now states compatible reuse only, explicitly retaining unchanged broader provider tokens and concurrent/unbound first use without exchange grant provenance. US3.12/US3.13 and Journey 6 pin the limits. Stronger narrowing or exchange-version members are the main alternative, requiring separate expanded API/ADR scope. The recommended limited guarantee is Proposed D-8 together with ADR-2, not an unapproved assertion of upstream downscoping.
- C1/A1: observability §3 specifies exact old→new identifiers, explicit kinds, required namespace rows and rollout/changelog behavior. D-7 is Proposed; T103 remains mandatory. The maintainer must enable a permitted standard-file publication path or supply an earlier namespace-registration change before implementation can publish it. No waiver or alternate namespace is silently selected.
- U2: Proposed ADR-3 and contract §4.1 designate `input.context.permission_sets` canonical for new enabled policies, retaining both compatibility inputs today. Permanent retention is replaced only by a future separately approved removal boundary. T104 still requires a maintainer-created issue; current removal/deprecation is excluded. Main alternative: permanent duplication.
- I3/I4: repair approval references and active original-scope dispositions, preserving prior approval and historical evidence. New work is not covered by D-6.

### Historical planning/review direction (2026-11-06)

F1 requested upstream scope authority; the Proposed limited interpretation and stronger alternative are now explicit in D-8. The earlier selected F2/F3 recommendations and F4–F6 provenance remain in quickstart §13 and ADR 048 Revision history. Previous failed publication of namespace registration is historical, not a standards exception or completion evidence. F3's follow-up issue remains an external prerequisite; this document worker cannot create it. No approval is inferred from this reconciliation.

## Technical Context

**Language/Version**: Go 1.27.3

**Primary Dependencies** (all existing, no new modules):

- chi v5 (routing)
- `github.com/lestrrat-go/jwx/v4` through the existing `JWTValidator` (JWT validation)
- cel-go through the existing `CELEvaluator`
- OPA v1 SDK (ExtProc policy)
- `golang.org/x/sync/singleflight`
- `envoyproxy/go-control-plane` (ExtProc gRPC)
- Viper, Cobra, and `internal/configutil` (ExtProc configuration)
- `otelhttp` through `internal/extproc/httpclient`
- Ginkgo v2, Gomega, and testify

**Storage**: None new. The feature reads existing `Agent`, `UserGrant`, `PermissionSet`, and
`ThirdpartyOAuth2Provider` repositories through existing services. It adds no migration. The
ExtProc keeps an in-memory permit cache.

**Testing**:

- Go unit tests (testify) for the domain, the HTTP adapters, the builder and routing, and ExtProc
  config, delegation, server, and authorization
- Backend Ginkgo E2E through `tests/e2e/bootstrap/`, including the real-broker ExtProc
  composition in `bootstrap/extproc.go`
- ExtProc Ginkgo E2E in `tests/e2e/extproc/`
- Example-config loading test in the existing explicit ExtProc example table (`internal/extproc/config/loader_test.go`)

**Target Platform**: Linux containers (broker and ExtProc). Darwin for development.

**Project Type**: Go monorepo backend: a broker end-user API addition plus a standalone ExtProc
consumer. No React UI change.

**Performance Goals**:

- Evaluation cost equals exchange steps 2–9a, plus one batched canonical-ID read, with no
  provider I/O.
- ExtProc adds no latency on a cache hit.
- On a body-path cache miss, latency is `max(exchange, resolve)`, not the sum.

**Constraints**:

- Fail closed everywhere (SR-002).
- No credentials or raw claims in responses, logs, or traces (SR-003).
- Grant-before-session ordering stays as it is (SR-004).
- No stale delegation (FR-015).
- Permit freshness ≤ 60 s and ≤ the grant expiry.
- ExtProc imports no broker domain, port, or adapter packages (ADR 011).
- The ADR 036 metadata-only input boundary is preserved.

**Scale/Scope**:

- 2 new broker routes and 0 new broker configuration keys
- 4 new ExtProc keys
- 1 new ExtProc package (`internal/extproc/delegation`)
- 2 Proposed feature-local ADR 048 revision drafts
- 47 acceptance scenarios (US1.1–US1.8, US2.1–US2.20, US3.1–US3.13, and US4.1–US4.6);
  US3.11–US3.13 are new under Proposed ADR-2/API-7/D-8 and not covered by D-6

**Review decisions (2026-10-28)**: The maintainer review submitted with `/speckit-plan`
answered I1 (an unusable `max-age` fails closed), U1 (one simple deadline with accepted skew),
and U2 (expiry before policy use returns 503). It endorsed the AuthZEN contract and the R16 ADR
decision set, which includes the I2 audit-seam extension. [remediation.md](./remediation.md)
records the answers and where they apply. These answers are maintainer direction for the
design. Formal sign-off happens through the pipeline's approval comment and covers the rows in
[Approval Items](#approval-items). Original approval is now recorded in quickstart §§11–12; the new revision drafts remain Proposed.

**Spec clarification (2026-10-28)**: The review also asked to extend the existing OPA context
attributes instead of adding `input.context.delegation` and `delegation_available`. The
`/speckit-clarify` session (spec Clarifications, Session 2026-10-28) revised FR-017, FR-018,
US3.1, US3.2, US3.6, US3.8, US4.1, Journey 4, and SC-005. This plan applies the revision in
research R15, data model §§3.3.1 and 3.5, and ExtProc contract §4.

## Constitution Check

*GATE: Pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Phase 0 gate: design preconditions

The Pass entries below describe design coverage, not new runtime validation or historical
test-first compliance. Implementation history and remaining evidence gaps are retained in
`quickstart.md`; task regeneration does not close them. Original API-2/D-5 approval and D-6 closure are recorded in quickstart §§11–12. Current sign-off gates only the new revisions (ADR-2/ADR-3/API-7/D-7/D-8). Local work must not wait for container-only evidence.

| Check | Status | Plan |
|---|---|---|
| Domain model | Pass | The spec defines Delegation Decision and Delegation Context. [data-model.md](./data-model.md) adds `DelegationRequest`, `Delegation`, the outcome variants, and the projection rules. No new aggregate. |
| Domain concepts | Pass | Add **Delegation Decision**, **Delegation Context**, **Policy Decision Point (PDP) identifier**, and **ExchangeResultBinding** (ADR-2) to the `ARCHITECTURE.md` glossary. Add a fourth row, "Broker delegation (AuthZEN)", to the policy-gates table. |
| Entity IDs | Not applicable | No new UUID-keyed entity. Existing typed IDs are reused. |
| Configuration design | Pass | Broker: no new keys (FR-001). ExtProc: 4 keys with YAML in [contracts/extproc-delegation-context.md](./contracts/extproc-delegation-context.md) §1 and spec Journey 4. |
| Config examples | Pass | Add `examples/config/extproc-delegation-context.yaml` and index it in `examples/config/README.md`. |
| Helm deployment contract | Pass (boundary) | The broker chart gets no change: the broker has no new keys, and ExtProc is not deployed by `charts/agentic-identity-broker/`. ExtProc keys go to its own config path (ADR 011): `docs/configuration.md`, `configs/config.extproc.docker.yaml` (commented), and `docs/docker-compose-setup.md`. |
| API design first | Original approval recorded; new API-7 pending | Original OpenAPI delta has approval/evidence in quickstart §§11–12. API-7 is the new ExtProc outcome contract in feature contracts §3.1; implement only after supplied sign-off. |
| API documentation | Pass | `api/enduser/openapi.yaml` (end-user; released `1.2.0` → `1.3.0`, approved F3/P3) and `docs/api/authzen-evaluation.md`. No admin change. |
| API changes confirmed | Pass with precondition | The maintainer endorsed [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md) in the 2026-10-28 review: the endpoints, denial form (a), and the deprecation marker (R12). The API-003 check against the final AuthZEN text found that the standard requires receivers to ignore unknown request fields; the maintainer confirmed that revision (2026-10-31) and directed on 2026-11-05 that R10, data model §2.5, the contract and the spec ignore unknown members, AuthZEN `properties` and `context` members other than `agent`, matching ADR 048 §1 and the OpenAPI. Original supplied API-1–API-3 approval, including revised API-2, is recorded in quickstart §11; this is not an outstanding gate. |
| Database design | Not applicable | No schema change and no migration. |
| E2E acceptance tests | Design covered; original D-6 disposition closed | 47 scenarios mapped below. US3.11–US3.13 are new and require test-first evidence. D-6 covers only the original 44; quickstart §§11–12 records supplied approval and completed alternative validation, not a performed original sequence. |
| E2E test mapping | Pass (design coverage) | 1:1 mapping per scenario. The denial-matrix rows are separate `It()` blocks; current mapping and passing runs do not establish original test-first order. |
| E2E red phase | Original deviation disposed of through D-6 | Concrete status/header/JSON/gRPC/count assertions remain required. Original missing sequencing was not performed; historical tasks are checked only through approved D-6 alternative validation in quickstart §12. New T110–T122 require genuine semantic-red evidence. |
| Frontend Playwright E2E | Not applicable | No React change. Consent and revocation reuse the existing UI and APIs. |
| Frontend screenshots | Not applicable | No UI state. |
| Concrete example journeys (XIV) | Pass | The spec has six linked journeys with configuration, HTTP exchanges, and decoded input JWT claims, including Journey 5 for US3.11. No wireframes, because there is no UI. |

### Implementation considerations

| Check | Status | Plan |
|---|---|---|
| Security-first | Pass with rationale | Broker evaluation is on whenever exchange is on, and it is always authenticated and authorized. ExtProc's opt-in default follows ADR 028's Security-First reconciliation: an additive consumer whose absence leaves today's controls in force. The ADR records this (R16). Every enabled-mode failure is closed. |
| Architecture docs | Pass | Update `ARCHITECTURE.md` §3.1.4 (routes), §3.2.1 (resolver port and permit cache), §3.2.3 (enabled-mode ordering), the policy-gates table, the glossary, and the ADR index. Update `internal/extproc/AGENTS.md`, `internal/AGENTS.md` (tokenexchange description), and the root `AGENTS.md` ADR table. |
| ADRs | Pass with precondition | The maintainer endorsed the R16 decision set in the 2026-10-28 review. ADR 048 ([adrs/048-authzen-delegation-pip.md](../../adrs/048-authzen-delegation-pip.md)) supersedes ADR 028's input schema and enabled-mode ordering, and extends ADR 033's seam. It records the clarified R15 shape. Sign-off is approval item ADR-1; implementation recorded that approval in the Status line and moved the draft to `adrs/`. Its same-PR revisions (§1 request-member handling, the body-path cooldown consequence) are registered as the revised API-2 and D-5 (Principle II allows same-PR revision before merge). The permit-bound token revision (new §10) is a Proposed feature-local draft, [adr-048-revision-permit-bound-tokens.md](./adr-048-revision-permit-bound-tokens.md), registered as ADR-2; implementation applies it to `adrs/` only after approval. |
| Library-first security | Pass | No new cryptography. JWTs are verified by the existing jwx-based `JWTValidator`. TLS uses the existing ExtProc HTTP client. |
| Zalando guidelines | Pass with documented exception | The AuthZEN-mandated paths (`/access/v1/...` and `/.well-known/...`) and snake_case members are standard-defined. The ADR documents the exception (API-004). |
| End-user docs | Pass | `docs/api/authzen-evaluation.md` with examples, plus a deprecation note in `docs/reference/token-exchange.md`. |
| Migration testing | Not applicable | No migration. |
| Hexagonal architecture | Pass | Domain: resolver and evaluation service in `tokenexchange`. Driving adapter: `internal/adapters/http/enduser/authzen_*.go`. Wiring: the builder. Routes: `SetupEnduserRoutes`. ExtProc: the `PermitResolver` port in `server`, with its implementation in `delegation/`. |
| Persistence patterns | Not applicable | No persistence. |
| Configuration system | Pass | Broker: none. ExtProc: `internal/extproc/config` with file, env, and flag precedence and startup validation (Principle VII, standalone exception under ADR 011). |
| DI via Builder | Pass | `AuthZENHandlers` are created in `builder.go` and passed through `EnduserHandlers`. Routing only registers. ExtProc wiring is in `cmd/extproc-token-exchange/root.go`, the existing composition root. |
| Feature scope | Pass | Every task traces to an FR, SR, SC, scenario, or binding rule. Two deferred findings are in [research.md](./research.md#deferred-findings-not-tasks-of-this-feature). Removing the exchange field is follow-up work for a later release (R12). |

### Post-design re-check

The post-design **coverage** gate is distinct from runtime compliance. Original VIII/XIII deviation is disposed of through supplied D-6 approval and completed alternative validation (quickstart §12); no original sequencing is retroactively claimed. Current T103 compliance and new-work validation remain open. The 2026-10-28 spec clarification settled the OPA input shape,
and research R15, data model §3.5, and ExtProc contract §4 now apply it. The freshness findings
are answered and applied, as recorded in [remediation.md](./remediation.md). The open items
in the next section are implementation-time gates. They do not block the design.

- The OPA input change is additive. The five new members use `omitempty` and appear only in
  enabled mode, and the disabled-mode input is pinned byte-for-byte. A malformed permit context
  fails closed (SR-002), and no unknown broker member reaches policy (R15).
- No new bounded context is invented. The resolver stays in `tokenexchange`, which owns every
  invariant (R1).
- Port changes: one new ExtProc port, and the ExtProc `Exchanger` port gains a binding-aware
  exchange for ADR-2. Broker ports are unchanged.
- The resolver depends on no session service, so FR-013 holds by construction.
- ADR 028's enabled-mode input and ordering need a new superseding ADR. That ADR also extends
  ADR 033's token-only finalization seam to evaluation. The maintainer endorsed the decision
  set in review. [ADR 047](../../adrs/047-authzen-delegation-pip.md) records it, and neither
  existing ADR is amended in place.
- The freshness rules (I1/U1/U2) are decided. The I2 perimeter extension is recorded in
  ADR 047; `quickstart.md` §§7–8 records implementation evidence and remaining historical
  test-first gaps. Preserve that distinction rather than reopening the settled design.

## Stakeholder decisions before implementation

Maintainer direction from the 2026-10-28 review and clarification, applied in the artifacts:

- **Freshness** (I1/U1/U2): see [remediation.md](./remediation.md).
- **OPA input shape**: the spec clarification of 2026-10-28, applied in research R15.
- **AuthZEN contract**: endorsed. This covers the endpoint shapes, the error mapping (R6),
  request-member handling (R10, revised 2026-11-05 to ignore unknown members per the AuthZEN
  1.0 HTTPS binding), the plain-UUID anchor, denial form (a) (R11), and the
  deprecation marker (R12). The review used a stale baseline; the corrected `1.1.0` → `1.2.0` delta has original API-3 approval recorded in quickstart §11.
- **New ADR**: the R16 decision set is endorsed, including the I2 audit-perimeter extension,
  and recorded in [ADR 047](../../adrs/047-authzen-delegation-pip.md).

Original implementation approvals and D-6 closure are recorded in quickstart §§11–12 and ADR 048. The table below preserves that inventory as recorded history; this phase does not grant any approval or reopen original API-2/D-5/D-6.

Still open for implementation:

1. Supplied approval must cover Proposed ADR-2, ADR-3, API-7, D-7 and D-8 before implementing those changes. T001 incorporates only covered drafts and preserves original ADR 048 provenance.
2. T103 requires a permitted publication path for the exact namespace registration and migration (observability §3); no standards waiver or partial compliance claim.
3. T104 requires a maintainer-created follow-up issue before canonical-field ADR/docs implementation. Future OPA removal and FR-019 exchange-response removal are separately approved work, not this feature.
4. Keep final-standard API-003 verification from quickstart §7; a genuinely new conflict requires a new Proposed item, not a silent change. The two unrelated collision findings remain deferred in research.

## Approval Items

| ID | Type | Decision | Details |
|---|---|---|---|
| API-1 | API | Recorded original approval (quickstart §§11–12): add `GET /.well-known/authzen-configuration` to the end-user API whenever token exchange is configured, publishing `policy_decision_point` (public URL) and `access_evaluation_endpoint`; no broker configuration key. Alternative: a broker `authzen.enabled` flag (rejected by FR-001). | `specs/054-authzen-delegation-pip/contracts/authzen-evaluation-api.md:32-61` |
| API-2 | API | Recorded original approval (quickstart §§11–12): add `POST /access/v1/evaluation` with bearer client-assertion auth; request members follow the AuthZEN 1.0 HTTPS binding — unknown members, AuthZEN `properties` and `context` members other than `agent` are ignored and never influence the decision, while `subject.type` (JWT token-type URN), `action.name` (token-exchange grant-type URN) and `resource.type` (`protected_resource`) are required exactly and a present `context` must be an object whose optional `agent` is a plain Agent UUID string anchor; the delegation is resolved from the verified subject token alone (R10, ADR 048 §1); permit context with non-empty principal/permission sets and non-empty canonical IDs when present (optional omissions and empty scope/service arrays retained), denial form (a) `reason_admin.code`/`reason_user.consent_url`, 400/401/403/500 `ErrorResponse` mapping, fixed 60 s `max-age` cap, `no-store` otherwise, and `X-Request-ID` echo. Alternatives: strict rejection of unknown members (violates AuthZEN 1.0); language-tagged denial form (b). | `specs/054-authzen-delegation-pip/contracts/authzen-evaluation-api.md:63-262` |
| API-3 | API | Recorded original approval (quickstart §§11–12): bump end-user API from the current `1.1.0` to `1.2.0` and mark `/oauth2/token` `granted_permission_sets` deprecated (still populated); removal is a later, separately approved `2.0.0` change. Alternative: intentionally reuse `1.1.0` (would not distinguish the new additive revision); immediate removal is rejected by FR-019. | `specs/054-authzen-delegation-pip/contracts/authzen-evaluation-api.md:264-277` |
| API-4 | API | Recorded original approval (quickstart §§11–12): add ExtProc keys `delegation_context.enabled` (false), `policy_decision_point` (origin of `oauth2.token_endpoint`), `request_timeout` (5s), `max_staleness` (60s), with startup validation and discovery failures. Alternative: no local staleness cap (broker `max-age` only). | `specs/054-authzen-delegation-pip/contracts/extproc-delegation-context.md:22-62` |
| API-5 | API | Recorded original approval (quickstart §§11–12): ExtProc enabled-mode client responses — 403 `access_denied` with `error_uri` consent URL for a delegation denial, and 503 `service_unavailable` `delegation context unavailable` for resolution errors or an expired permit deadline. Alternative: fall through to policy with availability `false` (rejected by FR-017). | `specs/054-authzen-delegation-pip/contracts/extproc-delegation-context.md:93-114` |
| API-6 | API | Recorded original approval (quickstart §§11–12): ExtProc OPA `input.context` gains direct `principal`, `agent`, `resource`, `grant`, `permission_sets` from a typed permit mapping; `granted_permission_sets*` are permit-derived and retained today; the original permanent-retention commitment is separately revised by Proposed ADR-3, `true` on both enabled paths (header-only changes from `false`); disabled mode unchanged. Alternative: `delegation` wrapper and `delegation_available` flag (rejected by the 2026-10-28 clarification). | `specs/054-authzen-delegation-pip/contracts/extproc-delegation-context.md:154-213` |
| ADR-1 | ADR | Recorded original ADR 048 approval (quickstart §§11–12): AuthZEN delegation PIP — information boundary, CEL reuse, freshness, fourth ExtProc port and ordering, supersession of ADR 028's enabled-mode input schema and ordering, ADR 033 deferred-finalization seam extension to evaluation, opt-in rationale, exchange-field deprecation. Alternative: amend ADR 028 in place (forbidden by Principle II). | `adrs/048-authzen-delegation-pip.md:1-197` |
| ADR-2 | ADR | Proposed ADR 048 §10 revision: opaque result identities, atomic shared first-claim bindings, identity-based compare-and-delete, one additional exchange and terminal conflict without stale fallback. Governs compatible reuse, not provider downscoping or exchange provenance; D-5 cooldown unchanged. Main alternative: verified exchange-version fields or provider downscoping (separate API/scope). | `specs/054-authzen-delegation-pip/adr-048-revision-permit-bound-tokens.md:31-104` |
| D-1 | Decision | Recorded original approval (quickstart §§11–12): evaluation reuses the unchanged token-exchange caller CEL policy with `request.grant_type` set to the token-exchange URN, so a policy cannot distinguish evaluation from exchange. Alternative: an evaluation-specific grant type or discriminator. | `specs/054-authzen-delegation-pip/research.md:118-135` |
| D-2 | Decision | Recorded original approval (quickstart §§11–12): ExtProc fails closed on delegation — an unusable `max-age` (missing, zero, malformed, duplicate) or a permit deadline that passes before policy use returns 503; no stale-on-error, even with a cached exchanged token; one deadline `requestStart + min(max-age, max_staleness)` with accepted skew. Alternative: serve stale context during broker outages. | `adrs/048-authzen-delegation-pip.md:62-76` |
| D-3 | Decision | Recorded original approval (quickstart §§11–12): ExtProc delegation context ships disabled by default (Principle I exception, reconciled as in ADR 028); the broker endpoint has no opt-out. Alternative: enabled by default. | `adrs/048-authzen-delegation-pip.md:110-116` |
| D-4 | Decision | Recorded original scope approval (quickstart §§11–12): exchange-field removal, configurable broker cap, batch evaluation, Resource Search, actor tokens, UI, and schema changes are excluded from this feature. Alternative: include the removal now as a breaking change. | `specs/054-authzen-delegation-pip/spec.md:768-774` |
| D-5 | Decision | Recorded original approval (quickstart §§11–12): on the body-bearing path, consent given after a delegation denial takes effect once the existing ADR 012 exchange re-consent cooldown (5 s) lapses, because the concurrent exchange during the denied request records that cooldown; the exchange cache is unchanged, the header-only path permits immediately, and delegation denials are never cached. Alternative: bypass the re-consent cooldown in enabled mode (changes ADR 012 exchange-cache semantics). | `adrs/048-authzen-delegation-pip.md:162-166` |
| D-6 | Decision | Recorded original approval (quickstart §§11–12): a one-time Constitution VIII/XIII exception for the original test-first/compile-only sequencing not performed for existing feature code, conditional on independent test review, all 44 scenario mappings and required local/downstream validation; retain the deviation permanently and require genuine test-first corrections. Alternative: decline the exception and retain the obligations as unresolved, without declaring completion. No existing validation failure or security guarantee is waived. | `specs/054-authzen-delegation-pip/historical-validation-exception.md:1-78` |
| API-7 | API | Proposed: enabled ExtProc returns 503 `service_unavailable` / `delegation token binding unavailable` after a mismatched additional-exchange delivery; permit expiry before final forwarding also returns the existing delegation-unavailable 503. Main alternative: rebind or forward an incompatible/expired result (rejected fail-open). | `specs/054-authzen-delegation-pip/contracts/extproc-delegation-context.md:128-149` |
| ADR-3 | ADR | Proposed ADR 048 §5 revision: canonical `input.context.permission_sets` guidance while preserving current compatibility; replace permanent retention only with future separately approved removal and a required follow-up issue. Alternative: permanent duplicate representations. | `specs/054-authzen-delegation-pip/adr-048-revision-canonical-permission-sets.md:13-34` |
| D-7 | Decision | Proposed: breaking broker span/event rename to `tokenexchange.access_evaluation*`, explicit broker/resolver kinds and required `access_evaluation`/`delegation` namespace registration with atomic migration and changelog guidance. Alternative: retain old identifiers through a standards exception (rejected). | `specs/054-authzen-delegation-pip/contracts/observability.md:45-71` |
| D-8 | Decision | Proposed: limit F1 to compatible result reuse; unchanged broader provider tokens and unbound/concurrent first use without exchange provenance remain possible. US3.12/US3.13 expose those limits and fail-closed conflict. Main alternative: expand scope to provider downscoping or exchange-version attestation. | `specs/054-authzen-delegation-pip/adr-048-revision-permit-bound-tokens.md:70-81` |

## Project Structure

### Documentation for this feature

```text
specs/054-authzen-delegation-pip/
├── spec.md
├── plan.md                                  # this file
├── research.md                              # R1–R20, deferred findings
├── data-model.md                            # value objects, projection, cache, OPA input
├── quickstart.md                            # validation guide
├── contracts/
│   ├── authzen-evaluation-api.md            # original approved end-user OpenAPI delta
│   ├── extproc-delegation-context.md        # ExtProc config, broker calls, outcomes, OPA input
│   └── observability.md                    # signal inventory, ADR-2 diagnostic, unresolved F2 gate
├── adr-048-revision-permit-bound-tokens.md # Proposed reuse revision (ADR-2)
├── adr-048-revision-canonical-permission-sets.md # Proposed compatibility revision (ADR-3)
├── remediation.md                           # analysis findings and the 2026-10-28 decisions
├── checklists/requirements.md
└── tasks.md                                 # 122 stable tasks; ADR-2 continuation is unchecked
```

### Source code and documentation affected

```text
adrs/048-authzen-delegation-pip.md                       # Moved from the feature draft with the recorded approval; number rechecked; ADR-2 revision applied after approval

api/enduser/openapi.yaml                                  # 2 paths, schemas, examples, deprecation, approved 1.3.0

internal/
├── domain/tokenexchange/
│   ├── delegation.go                                     # New: DelegationRequest, Delegation, DelegationResolver (exchange steps 2–9a)
│   ├── evaluation.go                                     # New: AccessEvaluationService, outcomes, freshness, context projection
│   ├── service.go                                        # Exchange delegates to DelegationResolver (no behavior change)
│   └── errors.go                                         # ErrAgentAnchorMismatch sentinel
├── adapters/http/
│   ├── enduser/authzen_configuration.go                  # New: discovery handler
│   ├── enduser/authzen_evaluation.go                     # New: evaluation handler (decode, bearer, serialize, X-Request-ID)
│   ├── routing/enduser.go                                # Register both routes; reserve /access
│   └── middleware/security_context.go                    # approved evaluation deferred-finalization extension
├── app/
│   ├── handlers.go                                       # EnduserHandlers.AuthZENConfiguration / AuthZENEvaluation
│   └── builder.go                                        # Construct resolver, evaluation service, handlers when tokenExchangeEnabled
└── extproc/
    ├── config/{config.go,loader.go,validate.go}          # DelegationContextConfig, defaults, flags, validation
    ├── delegation/                                       # New package (no broker imports)
    │   ├── discovery.go                                  # Discover + PDP verification
    │   ├── client.go                                     # AuthZEN POST, typed permit decode and validation, max-age, deadline
    │   └── resolver.go                                   # permit-only cache, singleflight, eviction
    ├── authorization/input.go, input_builder.go          # ContextInput: principal, agent, resource, grant, permission_sets (R15); header-only input
    └── server/server.go                                  # PermitResolver port; contextInputFromDelegation; body concurrency; header-only ordering; deadline check; responses
        server/exchanger.go, server/delegation.go         # ADR-2 token binding: binding-aware exchange, compare-and-delete, no stale fallback

cmd/extproc-token-exchange/root.go                        # Discovery at startup; resolver wiring; shutdown

ARCHITECTURE.md, AGENTS.md, internal/AGENTS.md, internal/extproc/AGENTS.md
docs/api/authzen-evaluation.md (new), docs/api/enduser-api.md, docs/reference/token-exchange.md
docs/configuration.md, docs/docker-compose-setup.md, configs/config.extproc.docker.yaml
examples/config/extproc-delegation-context.yaml (new), examples/config/README.md

tests/
├── e2e/
│   ├── authzen_delegation_test.go                        # US1.1–US1.8
│   ├── authzen_delegation_denial_test.go                 # US2.1–US2.10
│   ├── authzen_delegation_errors_test.go                 # US2.11–US2.20
│   ├── authzen_delegation_compat_test.go                 # US4.6
│   ├── delegation_context_journey_test.go                # US3.1–US3.13, US4.5 (real broker + ExtProc)
│   ├── helpers/authzen.go                                # evaluation/discovery request helpers
│   ├── bootstrap/extproc.go                              # delegation_context option for ExtProcEnvironment
│   └── fixtures/policies/delegation_canonical.rego       # canonical-ID policy (no UUIDs)
├── e2e/extproc/
│   ├── delegation_context_startup_test.go                # US4.1–US4.4
│   └── bootstrap/authzen_broker.go                       # MockAuthZENBroker (counts, mismatch, failure)
internal/extproc/config/loader_test.go                    # example ExtProc config loads (explicit example table)
```

**Structure Decision**: The broker domain logic stays in `internal/domain/tokenexchange/`, the
bounded context that already owns every delegation invariant (R1). A new package would be
anemic or cyclic. The HTTP surface is a driving adapter in `internal/adapters/http/enduser/`,
next to the token handler. The ExtProc consumer is a new leaf package,
`internal/extproc/delegation/`, behind a `server.PermitResolver` port. This preserves the
ADR 011 isolation.

## Design and Implementation Approach

### 1. Extract the shared resolution (Phase 0, no behavior change)

Move exchange steps 2–9a into `DelegationResolver.Resolve`. The existing `stage`, `serviceRef`,
`registeredAgentID`, and `grant` observation bookkeeping move with them. The resolver returns
them on the error through `WithObservation`, so Exchange's diagnostics are byte-identical.
`Exchange` becomes: validate the request, resolve, then sessions, scopes, and response. The
existing tests are the guard (R1).

### 2. Evaluation domain service

`AccessEvaluationService.Evaluate(ctx, AccessEvaluationRequest, clientAssertion)`:

1. Validate the request members (FR-003).
2. Resolve, with `ExpectedAgentID` when `context.agent` is present (R2).
3. Classify the result into Permit, Deny, or RequestFailure (R6).
4. For a permit, run one batched `CanonicalIDs` lookup for the included services, project the
   context (R4), and compute `MaxAge` (R5).

The consent URL is `public_url + "/agents/" + agentUUID`, the same as exchange's `error_uri`.

### 3. HTTP adapters and routing

- The discovery handler serializes the metadata (R8).
- The evaluation handler:
  - applies `MaxBytesReader(64 KiB)` and decoding that ignores unknown members but requires the
    supported values exactly (R10);
  - requires `Authorization: Bearer` (401 with `WWW-Authenticate`);
  - delegates to the evaluation service and maps the outcome to the status, headers, and body
    in the contract;
  - echoes `X-Request-ID` on every path (R7);
  - records a credential-free audit event and span attributes. It reuses the token-exchange
    diagnostic observation helpers and adds `operation = access_evaluation`.
- Routes are registered only when the handlers are non-nil. `/access` is always reserved (R9).
- After architectural approval, extend the shared perimeter deferred-finalization matcher to
  `POST /access/v1/evaluation`; do not capture transport metadata again in routing. The shared
  resolver finalizes verified subject Actor and distinct assertion CallingPeer before caller
  CEL, so both permits and caller-policy denials carry those identities. On early failures,
  use the existing idempotent request-principal/anonymous finalization helper before emitting
  in-handler audit; retain the logging perimeter fallback. Preserve trace identity and
  ordinary/token request behavior. T044/T047 provide test-first coverage (HIGH I2).

### 4. Builder wiring

When `tokenExchangeEnabled`, build one `DelegationResolver` from the same `approvalJWTValidator`,
`approvalCELEvaluator`, services, and repositories that exchange uses. Inject it into both
`TokenExchangeService` and `AccessEvaluationService`, then create both handlers. No new
configuration is read (FR-001).

### 5. ExtProc configuration and startup

Add `DelegationContextConfig`, its defaults, flags, and validation (R17). In `root.go`, when the
feature is enabled:

1. Derive or read the expected PDP.
2. Run `delegation.Discover` with the `oauth2.tls` HTTP client and `request_timeout`. On any
   error, exit before listening (US4.3 and US4.4).
3. Build the `Client` with the exchanger as the `AssertionProvider` and build the `Resolver`.
4. Pass the resolver to a new server constructor option.
5. Chain `Resolver.Shutdown`.

### 6. ExtProc request path

- Implement the R14 ordering in `processRequestHeadersOPA` and `processHeadersOnlyOPA`.
- Freshness follows the decisions in [remediation.md](./remediation.md):
  - The client rejects a permit with a missing, zero, malformed, or duplicate `max-age` (503,
    not used, not cached).
  - The client computes `Deadline = requestStart + min(max-age, max_staleness)` once.
    `Result`, the cache, the singleflight followers, and `requestState` carry it unchanged.
  - Before every OPA evaluation that receives delegation input (header-only, body, and each
    batch decision), ExtProc checks `now < Deadline`. If the check fails, it returns 503
    `service_unavailable` with no policy call, no forwarding, and no re-resolution.
  - T049/T050/T052/T057/T058 cover these rules test-first.
- Policy input (R15, data model §3.5, and contract §4):
  - The `delegation` client decodes the permit's `context` into its own wire struct, ignoring
    unknown members. It validates the documented shape (data model §3.3.1) and returns a typed
    `delegation.Context`. A violation is `invalid_response` and returns 503 (uncached).
  - `authorization.ContextInput` gains `principal`, `agent`, `resource`, `grant`, and
    `permission_sets` (`omitempty` on these five additions). Enabled-mode `permission_sets`
    is non-empty; nested `services` and all `scopes` arrays always serialize as arrays,
    including `[]` when empty. Nested optional `canonical_id` and `valid_until` stay omitted.
  - The server's pure function `contextInputFromDelegation(result, agentSessionID)` builds
    the input for the body path, each batch decision, and the header-only path. It deep-copies
    the slices, derives `granted_permission_sets` from the permit, and sets
    `granted_permission_sets_available: true`. Exchange extensions are not used in enabled mode.
    `BuildOPAInputHeadersOnly` accepts the `ContextInput` for this path.
  - No `delegation` wrapper, `subject` member, or `delegation_available` flag is emitted.
- Token binding (Proposed ADR-2, research R19, data model §3.6, contract §3.1):
  - Every successful result carries an opaque entry identity and synchronized binding cell shared with cache/flight followers, including non-cacheable results. Scope sets/instants derive from validated permits, never credential bytes or broker provenance.
  - First claim is atomic; all later deliveries compare the cell, never rebind, including the exchange leader. Compare-and-delete targets only observed identity; same-token replacement survives. Read a replacement once and use only if compatible; otherwise perform at most one extra exchange through existing singleflight/breaker.
  - Revalidate retry delivery: conflict returns terminal API-7 503, no loop, competing deletion or stale fallback. Contract §3.1 defines the binding-aware port/results and full outcome table.
  - Body validation follows concurrency/precedence and precedes buffering/mutation; header exchange uses the evaluated permit. Deadline checks also precede final forwarding, including header exchange/retry expiry.
  - Mismatch Debug diagnostic stays log-only; no credentials/scopes/principal. D-8 explicitly limits guarantees to compatible result reuse, not provider downscoping or the grant read by exchange.
- Disabled mode keeps the existing code path, and its OPA input is byte-for-byte unchanged.
- The approval gate keeps using the exchange-derived `principal` and `agent_id`.

### 7. Contract, documentation, and deprecation

After stakeholder confirmation:

1. Apply the OpenAPI delta, including the `granted_permission_sets` deprecation and approved `1.3.0` (released baseline `1.2.0`; F3/P3).
2. Write `docs/api/authzen-evaluation.md` and rebuild the docs site.
3. Update the token-exchange reference with the deprecation.
4. Update `ARCHITECTURE.md` and the AGENTS guides. In `internal/extproc/AGENTS.md`, document
   the five new `ContextInput` members and their enabled-mode sources (R15). Note that the
   permit, not the exchange cache window, sets `granted_permission_sets` in enabled mode.
   Document the ADR-2 token binding in `ARCHITECTURE.md` §3.2.1 and `internal/extproc/AGENTS.md`.
5. Add the ExtProc configuration docs and the example.

## Implementation Phase Overview

*`/speckit-tasks` generates the detailed task list. The table below shows which phases apply.*

| Phase | Purpose | Applies |
|---|---|---|
| Phase 0 | Pre-implementation refactoring: extract `DelegationResolver` with no behavior change | **Yes** |
| Phase 1 | Setup: E2E helpers, mock AuthZEN broker, policy fixture | Yes |
| Phase 2 | Design preconditions (2a–2f) | **Mandatory** |
| Phase 2.5 | Foundational scaffolding (compile-only types, stub handlers and resolver) | Yes |
| Phase 2.7 | Entity boilerplate | Skip |
| Phase 3 | US1 (P1): discovery and permit | Yes |
| Phase 4 | US2 (P1): denials and request errors | Yes |
| Phase 5 | US3 (P2): ExtProc fresh context | Yes |
| Phase 6 | US4 (P2): rollout, startup validation, and deprecation | Yes |
| Phase N | Constitution compliance verification | **Mandatory** |

- [x] **Phase 0 (refactoring)**: **include**. Extracting steps 2–9a is isolated in a
  no-behavior-change commit ahead of feature commits in this PR, with unchanged existing
  test assertions as the regression guard (FR-006; Principles II and VIII). T002–T005
  retain the already-recorded `b882d7b` extraction and its evidence. The current constitution
  requires this commit boundary, not a separate PR or maintainer-arranged handoff. Do not
  repeat the extraction, rewrite history or create another PR.
- [x] **Phase 2.7 (entity boilerplate)**: **skip**. There is no new entity, repository, or CRUD
  handler.

### Phase 2: Design preconditions

The current continuation adds **2g Observability Design** (T108/T109): inventory existing signals and the Proposed log-only binding diagnostic in `contracts/observability.md`, review credential safety and standards, and preserve unresolved T103/F2 registry/migration work. No new metric instrument or trace namespace is needed for ADR-2. The original 2a–2f design remains in place; no historical validation is recertified.

- **2a Domain model and ADR**:
  - Record the maintainer's approval (from the approval comment) in the Status line of
    the ADR 047 draft and move it to `adrs/` at the next unused number (done:
    [adrs/047-authzen-delegation-pip.md](../../adrs/047-authzen-delegation-pip.md)). The draft already records the R16 decision set, the
    freshness decisions, and the clarified R15 shape: direct members, a typed mapping,
    currently retained permit-derived compatibility `granted_permission_sets*`, and no wrapper or flag.
  - After approval of ADR-2, apply [adr-048-revision-permit-bound-tokens.md](./adr-048-revision-permit-bound-tokens.md)
    to `adrs/048-authzen-delegation-pip.md`, record the approval in its Status and Revision
    history, and remove the draft.
  - Add the glossary entries and the policy-gates row to `ARCHITECTURE.md`.
  - Update the ADR index and the root `AGENTS.md` ADR table.
- **2b Configuration design**:
  - Add the ExtProc keys to `docs/configuration.md`.
  - Add `examples/config/extproc-delegation-context.yaml` and its README entry.
  - Record "no broker or Helm change".
- **2c API design**:
  - Verify the API-003 items.
  - After stakeholder confirmation, apply [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md)
    to `api/enduser/openapi.yaml`.
  - Write `docs/api/authzen-evaluation.md` and rebuild the docs site.
- **2d Database design**: none. Record "no schema change".
- **2e Frontend review**: none. Record "no UI change".
- **2f E2E acceptance test design**: write all 47 `It()` blocks (US3.11–US3.13 test-first, red before binding exists). Compile them against the
  Phase 2.5 scaffolding and confirm a semantic red run.

### Phase 2.5: Foundational scaffolding

Add compile-only scaffolding so the tests fail semantically:

- types and method signatures for `DelegationResolver`, `AccessEvaluationService`, and the outcomes
- handlers that return `501`
- `DelegationContextConfig` fields
- the `delegation` package with constructors that return errors
- the `ContextInput` members and the policy-input types (`AgentContext`, `ResourceContext`,
  `GrantContext`, `PermissionSetContext`, `ServiceContext`), plus `delegation.Context`
- the server option

### Phases 3–6: User stories (test-first)

- **US1**:
  - resolver anchor support
  - evaluation service permit path, context projection, and freshness
  - both handlers and the routes
  - builder wiring
  - the canonical-ID omission rules
- **US2**:
  - outcome classification and the denial envelope
  - the request-validation matrix
  - the 401, 403, and 500 mapping
  - the `X-Request-ID` echo on all paths
- **US3**:
  - ExtProc client, resolver, and cache
  - body-path concurrency and precedence
  - header-only ordering
  - the typed permit decoding and validation, the direct `input.context` members, and the
    currently retained permit-derived compatibility `granted_permission_sets*`
  - the canonical-ID policy fixture on both paths
- **US4**:
  - config defaults and validation
  - discovery and PDP verification
  - disabled-mode guarantees
  - the deprecation marker and documentation

### Phase N: Constitution compliance verification

Verify each of these:

- The ADR in `adrs/` records the maintainer's approval and matches the code.
- The OpenAPI contract, `docs/api/`, and the configuration docs are updated.
- No credential appears in any response, log, or span. Each relevant test asserts this.
- The disabled ExtProc mode is unchanged.
- All non-container recipes required by `just verify` pass, including `just check`,
  `just test`, `just extproc-test`, `just test-integration`, frontend/CDK/mock tests, docs
  and non-container E2E as the final guard. Use the workflow-prepared
  `bash "$SECURITY_SCAN_SCRIPT" .` instead of rebuilding scanners through `just security`.
- Container-inclusive aggregates (`just verify`, `just test-e2e`) are not invoked in the
  sandbox. US2.19, infra integration and Agentgateway container suites run downstream on
  the pushed commit; record them as deferred until results exist.
- All 47 acceptance scenarios require evidence, separating the 46 local scenarios from
  US2.19's downstream result. Historical test-first/scaffolding gaps remain explicitly
  unfinished; later green runs do not establish past sequencing.

## Testing Strategy

### End-to-end acceptance tests

**Framework**: Ginkgo v2 and Gomega. Broker tests use the production `app.Builder` through
`tests/e2e/bootstrap/`. Consumer journeys use the production ExtProc composition against the
real broker through `tests/e2e/bootstrap/extproc.go`. Startup scenarios use the ExtProc suite
and a counting `MockAuthZENBroker`. Each test gets fresh storage, servers, and keys.

**Labels**:

- `Label("authzen-delegation")`: 29 broker specs
- `Label("delegation-context")`: 14 consumer specs
- `Label("delegation-context-startup")`: 4 ExtProc-suite specs

US2.19 also carries `Label("docker")` and uses real PostgreSQL. `just test-e2e-backend` excludes that label, so it covers only 28 of the 29 broker scenarios; `just test-e2e-docker` supplies the infrastructure gate, and `just test-e2e` includes both recipes. The unfiltered `authzen-delegation` label covers all 29 only with infrastructure available; `quickstart.md` uses the non-container local filter. Container suites run only downstream on the pushed commit; their absence in the sandbox is not a blocker for non-container work and never permission to skip US2.19, remove its label or claim local validation. The local focused broker filter is `authzen-delegation && !docker` (28 scenarios).

**Observation techniques**:

| Technique | Use |
|---|---|
| Status codes, `Cache-Control`, `X-Request-ID`, `WWW-Authenticate`, and JSON members | Broker outcomes |
| Absence of the subject and assertion markers in bodies and captured logs (`bootstrap.NewBufferedJSONLogger`) | SR-003 and SC-004 |
| Grant `valid_until = now + 15s` with `max-age ≤ 15`, and a short-lived grant expiring | US1.6 |
| Renaming the `user_grants` table (pattern from `impersonation_test.go`) | US2.19 |
| A mock upstream with no session, then a `/oauth2/token` call | US1.3 and US3.10 |
| `delegation_context.max_staleness: 2s` and `Eventually` | US3.4 and US3.7 (no clock injection) |
| Mock-broker request counters | US4.1 and US4.4 |
| A grant updated through the consent API from a two-scope to a one-scope permission set, a provider session replaced with distinct token values per scope set, the forwarded `authorization` mutation, broker exchange counts, and `max_staleness: 2s` with `Eventually` | US3.11 (ADR-2) |
| ExtProc gRPC `ImmediateResponse` status and body, and header mutations | US3 outcomes |
| Canonical-ID Rego fixture with two fixture sets that use different UUIDs, on both paths. The fixture reads `input.context.agent.canonical_id`, `input.context.resource.canonical_id`, and `input.context.permission_sets[].canonical_id`, and it denies with distinct reasons when a `delegation` wrapper, `delegation_available`, or `subject` appears, or when availability is not `true` (research R18). | US3.1, US3.2, US3.8, and SC-005 |

**Scenario mapping**:

| Spec scenario | E2E test location | Test description |
|---|---|---|
| US1.1 | `tests/e2e/authzen_delegation_test.go` | `It("publishes the PDP identifier and evaluation endpoint from the public URL")` |
| US1.2 | `tests/e2e/authzen_delegation_test.go` | `It("permits with the same context with a matching anchor, without context.agent, and without context")` |
| US1.3 | `tests/e2e/authzen_delegation_test.go` | `It("permits without a provider session while exchange returns its session error")` |
| US1.4 | `tests/e2e/authzen_delegation_test.go` | `It("reports only ceiling-capped effective scopes in resource.scopes")` |
| US1.5 | `tests/e2e/authzen_delegation_test.go` | `It("resolves an equivalent resource URI to the same service and decision as exchange")` |
| US1.6 | `tests/e2e/authzen_delegation_test.go` | `It("bounds max-age by grant expiry and denies with grant_expired at expiry")` |
| US1.7 | `tests/e2e/authzen_delegation_test.go` | `It("serves neither route without token exchange and both routes once it is configured")` |
| US1.8 | `tests/e2e/authzen_delegation_test.go` | `It("omits unset canonical_id fields everywhere and keeps configured ones")` |
| US2.1 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_missing when no grant exists")` |
| US2.2 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_missing after revocation of a permitted grant")` |
| US2.3 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_expired for an expired grant")` |
| US2.4 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_empty for a grant without permission sets")` |
| US2.5 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_permission_set_undeclared")` |
| US2.6 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_permission_set_missing")` |
| US2.7 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_service_omitted")` |
| US2.8 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_service_definition_missing")` |
| US2.9 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_service_requirement_excluded")` |
| US2.10 | `tests/e2e/authzen_delegation_denial_test.go` | `It("denies with grant_scope_intersection_empty")` |
| US2.11 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 400 for a subject token with an invalid signature")` |
| US2.12 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 400 for an expired subject token")` |
| US2.13 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 400 for an asserted identity subject")` |
| US2.14 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 400 for malformed JSON without resolving delegation")` |
| US2.15 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 401 without a bearer client assertion")` |
| US2.16 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 401 for an expired or untrusted client assertion")` |
| US2.17 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 403 when the unchanged token-exchange caller policy rejects the gateway")` |
| US2.18 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 400 for a mismatched context.agent anchor")` |
| US2.19 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 500 during a grant-repository outage")` |
| US2.20 | `tests/e2e/authzen_delegation_errors_test.go` | `It("returns 400 for an unregistered protected resource")` |
| US3.1 | `tests/e2e/delegation_context_journey_test.go` | `It("resolves concurrently with exchange and gives body policy the documented direct context members")` |
| US3.2 | `tests/e2e/delegation_context_journey_test.go` | `It("resolves before header-only policy with the same direct members and exchanges only after allow")` |
| US3.3 | `tests/e2e/delegation_context_journey_test.go` | `It("returns 403 access_denied with the consent URL before policy")` |
| US3.4 | `tests/e2e/delegation_context_journey_test.go` | `It("re-resolves after permit freshness lapses and denies a revoked grant despite a cached token")` |
| US3.5 | `tests/e2e/delegation_context_journey_test.go` | One `It()` proves no delegation denial caching: broker re-consultation on both paths, immediate header-only permit and body-path permit after the unchanged 5 s exchange re-consent cooldown (D-5). |
| US3.6 | `tests/e2e/delegation_context_journey_test.go` | `It("fails closed when the broker is unavailable after permit expiry")` |
| US3.7 | `tests/e2e/delegation_context_journey_test.go` | `It("isolates the cache per subject and resource and honors max_staleness")` |
| US3.8 | `tests/e2e/delegation_context_journey_test.go` | `It("makes the same canonical-ID policy decision across different UUIDs on both paths with permit-derived permission-set inputs")` |
| US3.9 | `tests/e2e/delegation_context_journey_test.go` | `It("denies a header-only request by policy without calling exchange")` |
| US3.10 | `tests/e2e/delegation_context_journey_test.go` | `It("blocks forwarding when exchange fails despite a permitted delegation")` |
| US3.11 | `tests/e2e/delegation_context_journey_test.go` | `It("re-exchanges before forwarding when a fresh permit narrows the grant and forwards the narrowed-scope token upstream")` |
| US3.12 | `tests/e2e/delegation_context_journey_test.go` | One `It()` for unchanged provider-session capability and new compatible result identity on both paths. |
| US3.13 | `tests/e2e/delegation_context_journey_test.go` | One `It()` for barrier-driven concurrent live permit ownership, identical-token replacement preservation and terminal API-7 503 on retry conflict. |
| US4.1 | `tests/e2e/extproc/delegation_context_startup_test.go` | `It("makes no discovery or evaluation requests when disabled by default")` |
| US4.2 | `tests/e2e/extproc/delegation_context_startup_test.go` | `It("fails startup when delegation context is enabled without authorization")` |
| US4.3 | `tests/e2e/extproc/delegation_context_startup_test.go` | `It("fails startup when discovery is unavailable or invalid")` |
| US4.4 | `tests/e2e/extproc/delegation_context_startup_test.go` | `It("fails startup on a PDP identifier mismatch without sending evaluation credentials")` |
| US4.5 | `tests/e2e/delegation_context_journey_test.go` | `It("discovers from the token endpoint origin by default and uses the advertised endpoint")` |
| US4.6 | `tests/e2e/authzen_delegation_compat_test.go` | `It("keeps principal, agent_id, and deprecated granted_permission_sets on exchange")` |

*Phase 2f records actual test line numbers and red/green evidence in `quickstart.md`; implementation keeps this plan and `spec.md` unchanged.*

**Red phase**: Tests compile against the Phase 2.5 scaffolding and fail on concrete assertions,
because the stub handlers return 501 and the stub resolver returns an error. No `XIt`, `PIt`, or
`Skip()` appears, and no red-phase comments are added. Every test file carries
`// Scenario X.Y from specs/054-authzen-delegation-pip/spec.md` comments.

**Regression guard (SC-006)**: These suites must pass unchanged:

- `token_exchange_test.go`
- `grant_diagnostics_test.go`
- `local_token_exchange_e2e_test.go`
- `extproc_approval_journey_test.go`
- `tests/e2e/extproc/opa_authorization_test.go`
- `tests/e2e/extproc/metadata_input_test.go`

### Unit tests

| Package | Coverage |
|---|---|
| `internal/domain/tokenexchange` | Resolver parity with the pre-extraction Exchange: a table over every detail and stage, plus observation refs. Anchor mismatch occurs before the grant read (asserted with a grant-repository spy). Evaluation request validation table. Outcome classification for every `FailureDetail` row in R6. Context projection: canonical-ID omission at every level, included-only services, `[]` scopes, ordering, and absent `valid_until`. `MaxAge` boundaries: none, 60+, 15, 0.9 s, and expired. No credential in any outcome. CEL receives the token-exchange grant type and an empty scope. No session service is invoked. |
| `internal/adapters/http/enduser` | Discovery body and headers. Evaluation: body limit, decoding (unknown members, `properties` and other `context` members ignored without influencing the decision; exact supported values; non-object `context` and non-string `context.agent` rejected), missing or non-Bearer `Authorization`, status and header and body per outcome, `X-Request-ID` echo on every path and no generated-ID echo, `WWW-Authenticate`, and `Cache-Control`. |
| `internal/adapters/http/routing` | Routes are absent when the handlers are nil. `/access` is reserved with a 404. The SPA does not shadow the routes. |
| `internal/app` | Handlers are wired only when `tokenExchangeEnabled`. One resolver instance is shared by exchange and evaluation. |
| `internal/extproc/config` | Defaults, env and flag precedence, each validation rule and message, the origin derivation, and that disabled mode skips validation. |
| `internal/extproc/delegation` | Discovery: exact PDP match, path-preserving mismatch, invalid endpoint, http without `allow_http`, and no credentials sent. Client: request body, bearer assertion, decision parsing, deny parsing and code allowlist, non-200 and timeout errors. Typed context: unknown members at every level are dropped. Every data model §3.3.1 violation (missing member, wrong type, empty `permission_sets`, bad timestamp, and `null` or `""` optional members) is `invalid_response` and is not cached. Absent optional members stay absent. `max-age` handling: missing, `0`, malformed, duplicate, and non-integer values are errors and not usable; positive values; values above 60 and above `max_staleness`; `Deadline = requestStart + min(max-age, max_staleness)` taken from the clock read before the send, so a delayed response does not extend it. Resolver: cache until `Deadline`, hits and singleflight followers get the original deadline, no hit at or after the deadline, no denial or error caching, no stale use, key isolation, eviction, and shutdown. Run with `-race`. |
| `internal/extproc/authorization` | Golden JSON for the disabled-mode body and header-only inputs, unchanged from today. In enabled mode, the five members are serialized with the documented names. `canonical_id` and `valid_until` are omitted, never `null`. `scopes` serializes `[]`. No `delegation`, `delegation_available`, or `subject` key appears. The header-only builder carries the supplied `ContextInput`. |
| `internal/extproc/server` | Body path: both calls complete before the response. The precedence matrix covers deny, error, exchange error, and success in both completion orders. Header-only path: resolve, then policy, then exchange; no exchange on policy deny. Deadline check: the deadline passes while ExtProc waits for exchange, while it buffers the body, and between batch decisions, and the header-only path is covered too. Each case returns 503 with no policy call and no forwarding. The tests use synchronization barriers and an unexported clock that `export_test.go` sets, as in R5. Disabled mode makes no resolver calls. The response bodies are covered. `contextInputFromDelegation`: `principal` comes from `subject.id`, `granted_permission_sets` comes from the permit and not from the exchange result, availability is `true`, the slices are deep-copied (mutating one input leaves the cached `Result` intact), and `agent_session_id` is preserved. The body, batch, and header-only paths receive the same members. Exchange-result binding (ADR-2, corrected by approved 2026-11-11 F1/P1): identity mismatch/absence on either field returns terminal API-7 503 on both paths, with no approval creation or identity-repair retry; approval state uses the exchange response. Binding equality includes principal and agent UUID as well as scope/version. Remaining binding tests: contract §3.1 outcomes, opaque identities and atomic shared binding cells, identical-token concurrent replacement preservation, retry-delivery terminal API-7 503, unchanged broader provider capability and differing concurrent grant reads; equal binding reuses the cached token with no broker call; reordered scopes and an equal instant in another RFC 3339 form count as equal; a scope or `grant.updated_at` difference compare-and-deletes and exchanges once before forwarding on both paths; a concurrent entry with identical token bytes and different identity/binding is not removed; a failed re-exchange returns the existing exchange error without the removed token, including within its stale window; unbound tokens bind to the first permit; disabled mode and cooldown entries are unaffected. Run with `-race`. |

### Integration tests

- Preserve `tests/integration/delegation_contract_test.go` (F5): complete nested producer/consumer field-set and denial-code parity against `api/enduser/openapi.yaml`, without test-only production APIs or broker imports in ExtProc. Include it in the current `just test-integration` regression gate; ADR-2 adds no wire field or token claim.
- No new infrastructure-backed integration test is needed: there is no persistence or migration.
  The PostgreSQL behavior that matters (the grant-repository outage) is covered by E2E US2.19.
- `examples/config/extproc-delegation-context.yaml` is added to the explicit example table in
  `internal/extproc/config/loader_test.go`, which loads and validates it through the ExtProc
  loader. That test runs in `just extproc-test`.

**Coverage goals**: every outcome-classification branch, every resolver cache transition, every
ExtProc precedence branch, and 100% of the acceptance scenarios (Principle XIII).

## Complexity Tracking

The historical original test-first/compile-only sequencing deviation requires the explicitly recorded D-6 exception and alternative validation in `historical-validation-exception.md` and `quickstart.md` §§11–12. Those records do not certify the missing sequence or waive tests for new work. US3.11–US3.13 and ADR-2 require genuine test-first development and fresh validation. ADR 048's partial supersession of merged ADR 028 separately follows Principle II; its Proposed ADR-2 same-PR revision is applied only after supplied approval. F2's remaining observability registration/migration obligation remains open as T103, not a constitutional exception.
