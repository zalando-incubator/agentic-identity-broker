# Quickstart: Validating Agent-Subject Impersonation

**Feature**: `specs/054-agent-subject-impersonation` | **Plan**: [plan.md](plan.md)

This guide records implementation progress and the checks required for delivery. **Delivery is blocked: application behavior and the 12-scenario acceptance suite are not implemented.** Contract details are in [contracts/](contracts/) and [data-model.md](data-model.md).

## Implementation record

### Supplied approval (T001)

The pipeline supplied approval by **mjungsbluth**, at **2026-10-29T13:35:38Z**, on [the approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6495558326), covering all ten items in the preceding reanalysis. This is the approval source, not an independent acceptance decision.

| Item | Approved scope recorded for implementation |
| --- | --- |
| A1 | ADR 045: agent-only exceptions to ADR 032 decisions 1/2, including target-derived subject despite mandatory extraction; C4 and major-version decision 8 |
| A2 | `allowed_subject_categories`, omission-only human default, strict validation and Helm schema |
| A3 | Optional guarded-default detector, replacing override, Helm setting and fail-closed evaluation; environment/CLI transports follow existing naming conventions (U1) |
| A4 | Token-endpoint self-agent behavior, category denial, matched third-party denial and detection-failure `server_error` |
| A5 | Credential-free `subject_category` audit and the two category diagnostic details |
| A6 | C4 full replacement: non-matching overrides remove categorical rejection and matching-human-state isolation |
| A7 | No unsigned-subject startup guard; unsigned subjects remain human |
| A8 | Signed exact-target rejection on human-only/omitted rules, even with active same-text delegation |
| A9 | Default equal-claim rejection for legacy, mapped/unsigned human and trusted external issuer tokens |
| A10 | End-User API 2.0.0 replacement on existing routes, collision migration and blocking version/behavior checks |

ADR relocation: `adr-045-draft.md` now resides at [adrs/045-agent-subject-impersonation.md](../../adrs/045-agent-subject-impersonation.md), with the exact approving login/date/URL in Status. This relocation record resolves the implementation-frozen spec/plan's draft links. Existing ADRs were not amended. `spec.md` and `plan.md` remain unchanged.

### Completed design and documentation

- T002: feature `specs/054-agent-subject-impersonation` remains selected; plan, research, model, contracts and journeys were reconciled without selecting another feature.
- T004–T005: ADR indexed in `AGENTS.md`; architecture and glossaries describe classification, separate eligibility, resolved identity, audit, target-derived minting, preserved human state, detection replacement, performance and the two proposed human-state observation boundaries. No boundary instrumentation has been implemented yet.
- T006: category contract retains the exact CFG-001 matrix and YAML-only rule-list boundary. U1's detector contract now names `IDENTITY_BROKER_TOKEN_EXCHANGE_CLAIM_EXTRACTION_AGENT_SUBJECT_EXPRESSION` and `--token_exchange.claim_extraction.agent_subject_expression`, explicit CLI > environment > YAML > evaluator-default precedence, unchanged flag behavior and explicit-empty fallback. Added T073–T074 for real production root registration/bindings and test-first precedence/no-YAML coverage; no scope or approval expansion.
- T007: complete local-mode agent example, concrete human/dual alternatives, explicit human setting in the existing example, example index and detector example. Runtime loader/startup verification remains outstanding.
- T010: canonical End-User OpenAPI declares 2.0.0 and includes C2–C4 descriptions and synthetic request/claim/denial examples. The actual checked-out baseline was 1.1.0, rather than the design artifacts' 1.0.0; the approved major-version replacement still applies. Admin OpenAPI is unchanged. Architecture records this baseline distinction and the existing-route strategy.
- T011: model reconfirms no new persistence; migration/new-adapter tasks remain inapplicable subject to final-diff review.
- T012: Journeys 1–3 agree with the signed target UUID, reserved equal claims without a category claim, existing actor, human consent, and default/compliant/non-matching/error detector outcomes. No specification change was needed; API-004 still excludes UI work.
- T047–T048: configuration, rendered token endpoint, concepts and reference now document eligibility, detector transports/default/replacement/failures, collision migration, audit/diagnostics and cross-links. These documents describe the approved contract, not verified application delivery.

### Executed checks and semantic-red evidence

All commands below ran at the supplied PR head plus this session's design/test changes; they do not prove the unimplemented feature works.

| Command | Actual outcome |
| --- | --- |
| `just check` | Exit 0 before and after the test edit. The repository falls back to `go vet` because `golangci-lint` is absent; this is **not** evidence that the linter ran. |
| `just test` | Exit 0 both at baseline and after all edits. Fast package tests exclude the top-level integration/E2E suites. |
| `TMPDIR=/tmp/gh-aw/agent go test ./tests/integration/ -run TestHelm` | T008 compiled and executed with Helm present, exit 1 with **seven semantic failures**: all four explicit valid category lists (`human`, `agent`, dual, reversed dual) and empty/guarded/replacing detector settings are rejected as unknown schema properties. Omission and invalid-input cases already pass. These are expected feature-red results, not Helm skips. No production chart behavior was changed. |
| `just test-e2e-backend` | Frontend TypeScript/Vite build succeeded; runner then exited 1 because `ginkgo` is not installed/on PATH. No backend acceptance specs ran. |
| `just verify` | Exit 127 in `security`: OSV `querybatch` failed TLS hostname validation (`x509: certificate is not valid for any names`). This is an external validation failure, not a vulnerability finding or successful security gate. No TLS/network policy was changed or bypassed. The command stopped before its later suites. |

The requirements checklist has 16 checked items and no unchecked items; its markers were not changed. No all-scenario acceptance-red evidence exists. T009 chart behavior and all broker behavior must remain deferred until T013–T017 supply it. The Helm tests intentionally remain red in this partial progress patch; do not skip them or call this feature complete.

### Outstanding work and technical blockers

- **T003, T013–T017, T070–T071:** provide the workflow's configured Ginkgo executable on the agent PATH and `golangci-lint` at the repository-required version. The supplied head has neither. The instruction not to rerun `just install-tools` without a feature tool-version change was honored. Author and run all 12 mapped acceptance scenarios; none have been authored in this session. Missing CLI tooling is not evidence that any scenario passed.
- **T009, T018–T046, T073–T074:** feature chart/config/domain/mint-characterization/audit/detection/observation implementation remains undone. It is deferred behind the mandatory all-scenario acceptance-red gate, not waived. U1 contract/task scheduling is resolved, but its production bindings and tests are not.
- **T049–T069:** finish integration evidence, startup/example checks and every final compliance task after behavior is implemented; no final verification checkbox is checked for document-only evidence.
- **T072:** restore valid TLS access for the configured OSV query service through the approved runner setup. Rerun the unmodified `just verify`; do not disable certificate validation, skip security, or suppress the failed check. Any later missing dependencies must likewise be reported rather than bypassed.

No new maintainer sign-off item is needed for the recorded progress. Existing A1–A10 approval remains the scope boundary.

## Prerequisites

- The maintainer's 👍 on the pipeline approval comment covering every item in [plan.md](plan.md#approval-items): ADR 045 (A1), the configuration and OpenAPI contracts (A2–A5), the decisions A6–A9, and the End-User API 2.0.0 major contract/migration proposal A10. The first implementation task records the supplied `context.approval` (approving maintainers, dates, and pipeline comment URL) in [ADR 045](../../adrs/045-agent-subject-impersonation.md), then moves it into `adrs/`. Record its final path and all A1–A10 approval coverage here; keep `spec.md` and `plan.md` unchanged during implementation.
- FR-011, SR-005, FR-012, SC-005, and CFG-005 in `spec.md` are aligned with the C4 replacement semantics (spec Clarifications, session 2026-10-28, maintainer command `6480240156`; research R5 *Consequence*).
- Go toolchain and `just`, per the repository README. Run `just install-tools` once.
- The default E2E tests use in-memory storage, so no Docker is needed.

## Blocking delivery order and compatibility evidence

The steps below are delivery requirements, not passing-check claims. The implementation record above distinguishes completed work from the remaining gates.

1. T001 must record supplied approval covering A1–A10. A10 treats the baseline End-User API 1.0.0 as released absent evidence of an unreleased contract and proposes 2.0.0 on existing routes. See [contracts/enduser-api-versioning.md](contracts/enduser-api-versioning.md) for the rationale, alternative and migration requirements. T010 applies the versioned OpenAPI and architecture strategy before production behavior.
2. T008 writes/runs Helm tests red; T013–T017 then write/compile/run all 12 acceptance scenarios with semantic-red evidence. No Helm schema/template or broker behavior may precede this all-scenario gate. Only necessary DTO/type compilation scaffolding is permitted.
3. T009 (Phase 2g) implements chart behavior after both red gates, runs Helm green, and reruns the acceptance suite to record remaining broker semantic failures before foundation work. Keep task IDs stable and follow dependencies rather than numeric order.
4. T030 characterizes unchanged email policy behavior: `Email=nil` means no credential-derived input; no email-emitting policy means no output `email`; `{"email": principal.email}` means `email: ""`; a literal policy-supplied email remains allowed. Record those results, not an unconditional email-suppression claim.
5. T048 publishes the End-User API 2.0.0 migration guide. T053/T058/T059 verify actual `info.version`, architecture strategy, migration guide and approval coverage, both incompatible A8/A9 denials, and unchanged nonmatching human behavior. T070–T072 run all acceptance/regression/full gates before delivery. Record the evidence here during implementation; do not claim readiness when consumers or checks remain blocked.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected: everything passes. The mint tables additionally distinguish no-policy email absence from policy-emitted empty-string/literal email without credential propagation. The new unit tables cover `classifySubject`, category eligibility, rule invariants, YAML shape validation, agent-subject minting (`sub == agent_id`, no category claim), `CELEvaluator.IsAgentSubject` with the default and replacing override expressions, rule invariants accepting an unsigned rule that lists `agent`, the token-exchange rejection, and handler audit fields.

## 2. Acceptance journeys (E2E)

```bash
ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/
```

Expected: 12 passing specs, one per scenario US1.1–US3.4 (mapping in [plan.md](plan.md#scenario-mapping)). Key observable outcomes:

| Journey | Check |
| --- | --- |
| 1 (US1.1–1.3) | `200`. The decoded token has `sub == agent_id ==` the target UUID, no `subject_category` claim, and `act` with the actor issuer and identity. No `UserGrant` exists before or after. An invalid client or actor signature produces no token. |
| 2 (US2.1–2.5) | The Journey 2 matrix holds. A human without delegation gets `403` with `error_uri`. Exact equality on a human-only rule (explicit or omitted) gets `403` with no `error_uri`, with or without a delegation for the same text. Invalid `allowed_subject_categories` shapes and values fail startup. |
| 3 (US3.1–3.4) | An unauthorized or unresolved agent gets no token. With `agent_subject_expression` at its default, and again with Journey 3 Step 3's compliant override, the third-party exchange of the agent token gets `403` with the agent-subject description, with and without a human grant and session for the same UUID text. The session's provider is never called. With `false` and valid extraction/authorization/resource fixtures, no human state yields the existing missing-grant `403 access_denied`, grant-missing diagnostic, consent `error_uri`, and grant boundary span. Matching resource-covering human grant and usable session yields both grant/session boundary spans. Both omit the agent-subject description/diagnostic; neither requires automatic success or isolation. With the detection-error override `subject_token.sub`, the response is `500 server_error` and no human state is consulted. |

### Required US3.4 read-access evidence

T026–T027 must implement and validate the plan's Human-state observation boundary before negative span assertions count as evidence. Capture the two production spans `tokenexchange.verify_agent_access` and `tokenexchange.get_valid_access_token` through existing `BuildAppWithTracerProvider(storage, tp)` / `NewInMemoryTracerProvider()`, with telemetry/traces enabled and the concrete production storage adapter. No E2E storage/port spy can be injected through this bootstrap.

For each exchange, use a unique sampled W3C trace header and require the completed HTTP server span before checking for zero child boundary spans. Use bounded completion/flush, filtering by that trace ID; never use recorder-wide totals. Demonstrate the grant span with an ordinary human missing-grant control, and both spans with an ordinary human resource-covering grant/usable-session control. Run these controls in each default/compliant/`false` broker; the `subject_token.sub` error broker uses a sequential otherwise-identical default-detector control broker as described in the plan, while requiring its own failing HTTP trace. No non-string subject fixture or weakened credential validation is permitted. Drain/close each broker and restore the prior global provider/propagator.

Record the control results, correlated zero-boundary-span assertions and zero mock-provider/refresh calls for agent-token default/compliant/error requests. A missing HTTP/control span is an observation failure, not proof of no reads. Unchanged storage state and zero provider calls alone are insufficient. Also record the positive `false` checkpoints in the Journey 3 row above. Unit repository fakes supplement, but never replace, this complete HTTP evidence.

## 3. Human-behavior regression

```bash
ginkgo -v --focus "OAuth2 User Impersonation|Local Mode Token Exchange" ./tests/e2e/
```

Expected: all existing specs pass unchanged (US2.4, FR-006).

## 4. Startup validation by hand

Put each invalid value from [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml) into a copy of `examples/config/impersonation-agent-subject.yaml`, then start the broker:

```bash
IDENTITY_BROKER_CONFIG_PATH=/tmp/gh-aw/agent/agent-subject.yaml ./bin/agentic-identity-broker
```

Expected: the process exits before listening. The error names `oauth2_authorization_server.impersonation.rules[0].allowed_subject_categories`. Removing the key starts the broker human-only.

Repeat with an invalid `token_exchange.claim_extraction.agent_subject_expression` from [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml), such as a syntax error or `'agent'`, whose static type is a string. Expected: startup fails. `subject_token.sub` is statically `dyn`, so it starts but fails closed with `500 server_error` at exchange time. Removing the key starts the broker with the default `sub == agent_id` detection.

## 5. Helm contract

```bash
go test ./tests/integration/ -run TestHelm
```

Expected: the chart accepts `allowed_subject_categories: [agent]` and renders it verbatim into `config.yaml`. The schema rejects `[]`, `[machine]`, and duplicates. `tokenExchange.claimExtraction.agentSubjectExpression` renders as `agent_subject_expression` only when set, and the rendered config omits it otherwise. Helm must be installed and these cases must execute; a skip when Helm is unavailable is not a successful feature validation.

## 6. Full gate

```bash
just verify
```

Expected: passes, with E2E as the final layer.
