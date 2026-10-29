# Quickstart: Validating Agent-Subject Impersonation

**Feature**: `specs/054-agent-subject-impersonation` | **Plan**: [plan.md](plan.md)

This guide shows how to prove the feature works after implementation. It does not contain the implementation. Contract details are in [contracts/](contracts/) and [data-model.md](data-model.md).

## Prerequisites

- The maintainer's 👍 on the pipeline approval comment covering every item in [plan.md](plan.md#approval-items): ADR 045 (A1), the configuration and OpenAPI contracts (A2–A5), and the decisions A6–A9. The first implementation task records the supplied `context.approval` (approving maintainers, dates, and pipeline comment URL) in [ADR 045](adr-045-draft.md), then moves it into `adrs/`. Record its final path and all A1–A9 approval coverage here; keep `spec.md` and `plan.md` unchanged during implementation.
- FR-011, SR-005, FR-012, SC-005, and CFG-005 in `spec.md` are aligned with the C4 replacement semantics (spec Clarifications, session 2026-10-28, maintainer command `6480240156`; research R5 *Consequence*).
- Go toolchain and `just`, per the repository README. Run `just install-tools` once.
- The default E2E tests use in-memory storage, so no Docker is needed.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected: everything passes. The new unit tables cover `classifySubject`, category eligibility, rule invariants, YAML shape validation, agent-subject minting (`sub == agent_id`, no category claim), `CELEvaluator.IsAgentSubject` with the default and replacing override expressions, rule invariants accepting an unsigned rule that lists `agent`, the token-exchange rejection, and handler audit fields.

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
