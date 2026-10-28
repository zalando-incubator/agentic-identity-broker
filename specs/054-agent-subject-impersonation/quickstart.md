# Quickstart: Validating Agent-Subject Impersonation

**Feature**: `specs/054-agent-subject-impersonation` | **Plan**: [plan.md](plan.md)

This guide shows how to prove the feature works after implementation. It does not contain the implementation. Contract details are in [contracts/](contracts/) and [data-model.md](data-model.md).

## Prerequisites

- Before implementation, three confirmations are recorded on PR #293:
  - [ADR 044](adr-044-draft.md) is **Accepted**.
  - The OpenAPI delta in [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml) is confirmed. C2 and C3 were confirmed in the PR #293 review, and C1 was withdrawn. C4 (the `agent_subject_expression` override semantics, research R5) still needs confirmation.
  - The unsigned-subject startup guard (research R8) is confirmed.
- Go toolchain and `just`, per the repository README. Run `just install-tools` once.
- The default E2E tests use in-memory storage, so no Docker is needed.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected: everything passes. The new unit tables cover `classifySubject`, category eligibility, rule invariants, YAML shape validation, agent-subject minting (`sub == agent_id`, no category claim), `CELEvaluator.IsAgentSubject` with the default and override expressions, the token-exchange rejection, and handler audit fields.

## 2. Acceptance journeys (E2E)

```bash
ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/
```

Expected: 12 passing specs, one per scenario US1.1–US3.4 (mapping in [plan.md](plan.md#scenario-mapping)). Key observable outcomes:

| Journey | Check |
| --- | --- |
| 1 (US1.1–1.3) | `200`. The decoded token has `sub == agent_id ==` the target UUID, no `subject_category` claim, and `act` with the actor issuer and identity. No `UserGrant` exists before or after. An invalid client or actor signature produces no token. |
| 2 (US2.1–2.5) | The Journey 2 matrix holds. A human without delegation gets `403` with `error_uri`. Exact equality on a human-only rule (explicit or omitted) gets `403` with no `error_uri`, with or without a delegation for the same text. Invalid `allowed_subject_categories` shapes and values fail startup. |
| 3 (US3.1–3.4) | An unauthorized or unresolved agent gets no token. With `agent_subject_expression` left at its default, the third-party exchange of the agent token gets `403` with the agent-subject description, with and without a human grant and session for the same UUID text. The session's provider is never called. |

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

Repeat with an invalid `token_exchange.claim_extraction.agent_subject_expression` from [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml), such as a syntax error or `subject_token.sub`, which returns a string. Expected: startup fails. Removing the key starts the broker with the default `sub == agent_id` detection.

## 5. Helm contract

```bash
go test ./tests/integration/ -run TestHelm
```

Expected: the chart accepts `allowed_subject_categories: [agent]` and renders it verbatim into `config.yaml`. The schema rejects `[]`, `[machine]`, and duplicates. `tokenExchange.claimExtraction.agentSubjectExpression` renders as `agent_subject_expression` only when set, and the rendered config omits it otherwise. The test skips when `helm` is not installed.

## 6. Full gate

```bash
just verify
```

Expected: passes, with E2E as the final layer.
