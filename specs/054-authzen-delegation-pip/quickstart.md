# Quickstart Validation: AuthZEN Delegation Policy Information Point

**Feature**: `specs/054-authzen-delegation-pip` | **Plan**: [plan.md](./plan.md)

This guide proves the feature end to end after implementation. It is a validation guide, not an
implementation description. The contracts are in [contracts/](./contracts/). The value objects
are in [data-model.md](./data-model.md).

## Status and prerequisites

Before any implementation, these preconditions must hold:

1. The maintainer has signed off the items in the plan's
   [Approval Items](./plan.md#approval-items) with a 👍 on the pipeline's approval comment.
   These cover [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md),
   [contracts/extproc-delegation-context.md](./contracts/extproc-delegation-context.md), and the
   ADR draft [adr-045-authzen-delegation-pip.md](./adr-045-authzen-delegation-pip.md).
2. T001 has recorded only the supplied approval in each covered ADR's Status line and moved the approved draft to `adrs/`. Review direction alone is not sign-off.
3. The API-003 **Verify** checks against the final AuthZEN text have passed before the OpenAPI
   is frozen.
4. The spec revision for the OPA input shape (research R15) has landed. This was done on
   2026-10-28.

Tooling: `just install-tools` (Go 1.27.3, Ginkgo, golangci-lint). The focused broker run needs Docker or Podman for the real PostgreSQL outage scenario US2.19; the full gate also includes existing container-backed ExtProc/agentgateway and integration suites. Docker-in-Docker is unavailable in this sandbox. Run infrastructure gates on a capable runner; report any unfinished gate as a blocker, never as passed or waived.

Implementation evidence belongs in this guide: record approval provenance, the separate refactor PR and integrated revision, scenario-to-test `file:line` mappings, semantic-red assertions, green commands/results and deferred-task reasons. Keep the approved `spec.md` and `plan.md` unchanged throughout implementation.

## 1. Static checks and fast tests

```bash
just check            # fmt → vet → lint
just test             # broker domain/adapters/app unit tests
just extproc-test     # ExtProc unit tests with -race, including the example-config loading test
```

**Expected**: All pass. The existing token-exchange unit tests pass unchanged after the Phase 0
resolver extraction, which proves FR-006 does not regress.

## 2. Broker acceptance journeys (US1, US2, US4.6)

```bash
ginkgo -v --label-filter="authzen-delegation" ./tests/e2e/
```

US2.19 MUST have both the shared `authzen-delegation` label and `Label("docker")`. The command above includes it and requires real PostgreSQL infrastructure. The ordinary `just test-e2e-backend` recipe excludes `docker`; it therefore covers 28 of these broker scenarios, not all 29.

```bash
just test-e2e-backend     # excludes US2.19; not the complete broker acceptance gate
just test-e2e-docker      # includes US2.19; requires a capable container runner
```

**Expected with infrastructure available**: 29 focused specs pass:

- US1.1–US1.8 (8)
- US2.1–US2.20 (20)
- US4.6 (1)

Spot-check these observable outcomes against spec Journeys 1–3:

| Check | Expected |
|---|---|
| `GET /.well-known/authzen-configuration` | `policy_decision_point` equals the configured `server.enduser.public_url` |
| Permit with `X-Request-ID: delegation-001` | 200, `decision: true`, `Cache-Control: private, max-age=60`, request ID echoed, no `subject.id` JWT anywhere in the body |
| Grant `valid_until` in 15 s | `max-age` ≤ 15 |
| Revoked grant | 200, `decision: false`, `reason_admin.code = grant_missing`, `Cache-Control: no-store` |
| `subject.type: identity` | 400 |
| No `Authorization` | 401 with `WWW-Authenticate: Bearer` |
| CEL rejects the gateway | 403 |
| Renamed `user_grants` table | 500 |
| Token exchange disabled | Both routes return 404 |

## 3. Consumer journeys (US3, US4.5)

```bash
ginkgo -v --label-filter="delegation-context" ./tests/e2e/
```

**Expected**: 11 specs pass (US3.1–US3.10 and US4.5). They run the production ExtProc
composition against the real broker test server. Confirm these outcomes:

- The body path forwards only after both exchange and resolution succeed.
- The header-only path never calls exchange when policy denies.
- After revocation, a repeat request past `max_staleness` is denied with 403 and `error_uri`
  set to the consent URL.
- A broker outage with a still-cached exchanged token returns 503, not a stale permit.
- A permit with a missing or zero `max-age`, or one that expires before policy use, returns 503.
- The canonical-ID policy allows on both paths. The policy reads `input.context.agent`,
  `input.context.resource`, and `input.context.permission_sets`, and
  `granted_permission_sets_available` is `true`. Neither `input.context.delegation` nor
  `delegation_available` is present (contract §4).

## 4. ExtProc startup journeys (US4.1–US4.4)

```bash
just test-e2e-extproc          # or: ginkgo -v --label-filter="delegation-context-startup" ./tests/e2e/extproc/
```

**Expected**: The disabled default makes zero discovery or evaluation requests to the mock
broker. Each of these conditions stops startup with the error substring from
[contracts/extproc-delegation-context.md](./contracts/extproc-delegation-context.md) §1:

- `authorization.enabled: false`
- discovery failure
- an identifier mismatch

The mismatch case makes **zero** evaluation requests.

## 5. Manual smoke test (optional, local compose)

1. Start the stack with `just compose-extproc-up`.
2. Add to `configs/config.extproc.docker.yaml`:
   `delegation_context: {enabled: true}` and `authorization.enabled: true`. Use a policy that
   keys on canonical IDs.
3. Configure canonical IDs `data-assistant`, `repo-reader`, and `repo-api` through the admin API.
   Grant consent in the SPA.
4. Send an MCP `tools/call` through agentgateway. **Expected**: The request is allowed.
5. Revoke consent in the SPA. Wait 60 s and repeat the request. **Expected**: 403
   `access_denied` with the consent URL.

## 6. Full gate

```bash
just test-e2e           # backend + docker + ExtProc + frontend, including US2.19
just docs-build         # docs/api/authzen-evaluation.md renders
just verify             # full gate, E2E last
```

**Expected**: All suites pass. The existing suites `token_exchange_test.go`,
`grant_diagnostics_test.go`, `tests/e2e/extproc/opa_authorization_test.go`, and
`extproc_approval_journey_test.go` pass unchanged, which proves SC-006 compatibility.
