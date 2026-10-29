# Quickstart: Validate Incremental Service Authorization

**Feature**: [spec.md](./spec.md) | **Contracts**: [contracts/](./contracts/) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end after implementation. It references contracts instead of
repeating them. All values are synthetic and follow the spec's shared cast.

## Prerequisites

- `just install-tools` has completed, including Playwright browsers.
- Docker or Podman is available for infrastructure-backed integration tests and the full
  verification gate's container-backed suites. This feature's broker-plus-ExtProc challenge tests
  use `bootstrap.ExtProcEnvironment` and assert gRPC immediate responses without a gateway container.
- The maintainer has approved the items in [plan.md › Approval Items](./plan.md#approval-items).
  ADRs 045 and 046 have been moved to `adrs/` with an Accepted status.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected:

- The new `internal/domain/servicescope` table tests pass for the grammar in
  [contracts/virtual-service-scope.md](./contracts/virtual-service-scope.md).
- The `oauth2server` refresh tests show that `invalid_scope` leaves the refresh session unused.
- The ExtProc challenge tests cover `scope` and `resource_metadata` inclusion and omission.

## 2. Migration and repositories

```bash
just test-integration-infra
```

Expected: migration `046_add_service_scope_bindings` applies, rolls back, and reapplies. Bindings
survive the round trip on `authorization_codes` and `refresh_token_sessions`, and `NULL` rows stay
readable. Both adapters enforce exact successor binding inheritance and scope subset. Exercise code
issuance with full scopes → successful narrowed refresh → scope-less successor refresh: successor
`Scope` and response/JWT stay narrowed, every original binding survives and is rechecked, and neither
a scope-less refresh nor a retained binding can restore a removed scope. Include third-step service
failure and re-requested removed-scope refusal, both without token consumption.

## 3. Backend acceptance journeys (spec Journeys 1, 3, and 4)

```bash
just test-e2e
```

Run the suites below, or the whole E2E set:

| Suite | Proves |
|---|---|
| `tests/e2e/incremental_authorization_test.go` | Story 1, Scenarios 2, 3, 5, 6 and 7; Story 3, Scenarios 1 and 4; Story 4, Scenarios 2 and 3. Complete authorization/consent/code/refresh/exchange journeys cover non-consuming widening refusal, server-derived coverage, submission enforcement, exact visible scopes, local refresh rechecks, and rename/reuse identity continuity. |
| `tests/e2e/incremental_authorization_proxy_test.go` | Story 3, Scenarios 2 and 3. A mock upstream IdP records the received `scope`: virtual scopes never appear, and the rotated upstream refresh token is returned unchanged. |
| `tests/e2e/service_session_page_test.go` | Story 2, Scenarios 1, 3 and 4: both detail identifiers, `session: null`, `401` and `404`, principal isolation, provider login/callback to the same service page and successful exchange after direct recovery. |
| `tests/e2e/incremental_authorization_gateway_test.go` | Story 1, Scenario 1 and Story 4, Scenario 1. Each owns one complete broker-plus-ExtProc scenario, including persisted provider rejection, scope/action propagation and unchanged elicitation, with metadata-present/absent variants. |
| `tests/e2e/incremental_authorization_docs_test.go` | Story 4, Scenario 4: live discovery excludes virtual scopes and the operator guide's route/config example agrees with the contract. |
| `tests/e2e/frontend/incremental_authorization_test.go` | Story 1, Scenario 4 and Story 2, Scenario 2. Consent States C/D and service-page States A/B are exercised through the existing page objects, including login, Save/tool retry and refresh/termination. |

The [task mapping](./tasks.md#acceptance-scenario--task-mapping) assigns exactly one owning `It`
to each of the 19 scenarios; supporting unit/integration assertions do not duplicate acceptance
ownership. Run `just test-e2e-backend` for these broker-plus-ExtProc and proxy journeys, and
`just test-e2e-frontend` for the two browser journeys.

### Required regression variants

Keep these variants inside their existing owning scenarios, with semantic-red assertions first:

- US1 S4/S7: all-scopes and no-explicit-requirements agents, empty configured required scopes,
  selected Read Access needing `repo user:email`, but valid access with only `repo`. Selecting the
  failing candidate disables Save, deselecting it can enable Save; direct submission and refresh
  cannot claim usability until the effective union is present. Complete login/Save/issuance/exchange.
- US4 S2: Save successfully, deterministically reassign the canonical ID, then follow the returned
  authorize URL with its original JWE. Assert `invalid_scope`, no authorization code and no proxy
  redirect, even when the replacement service has a usable session and grant coverage.
- US3 S1: full → narrowed → scope-less refresh retains narrowed scopes and every assurance binding;
  test subsequent unusability and attempts to restore removed scopes without consuming the token.
- Supporting T024 unit cases: valid scope-sufficient, identity-matching access with absent refresh
  and with expired refresh is usable without provider calls; expired access with those same refresh
  states is unusable.

### Maintained UI captures

Normal verification does not capture screenshots. For maintained artifacts, use the existing
capture-enabled serial frontend harness:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

The harness stages captures under `coverage/screenshots/` relative to the frontend suite working
directory. Retain the four plan-named images under `tests/e2e/screenshots/` using existing screenshot
handling; do not modify workflows. Verify consent C/D and session A/B against the spec's Wireloom
states. Task T010 records the design-system composition decisions here during implementation;
no frontend design review or capture is claimed by task generation.

## 4. Manual spot check (optional)

1. Start the broker with an in-memory store and `oauth2_authorization_server.mode: "local"`, using
   the configuration from spec Journey 1, Step 1.
2. Reproduce spec Journey 1, Step 2 with the token-exchange request. The JSON error contains
   `"scope":"https://broker.example.com/sessions/github"`
   ([contracts/enduser-api.md §1](./contracts/enduser-api.md#1-post-oauth2token-rfc-8693-error-scope-member-api-exchange-challenge)).
3. Repeat spec Journey 1, Step 3 with the refresh request. The response is
   `400 invalid_scope`. Repeating the same refresh without `scope` then succeeds, which shows the
   token was not consumed.
4. Open `https://broker.example.com/sessions/github` while signed in. State A renders. Login
   returns to the same page in State B.
5. Rename the service's canonical ID to `github-production` through the admin API.
   - `/sessions/github` now shows not found.
   - A local refresh bound to the old scope returns `invalid_scope`.
   - A new exchange error emits `/sessions/github-production`.

## 5. Gateway metadata variant

Add `resource_metadata_url` to the agentgateway route's `metadataContext`, as in
[contracts/extproc-challenge.md §1](./contracts/extproc-challenge.md#1-optional-metadata-field-aibtokenexchangeresource_metadata_url-api-metadata-config).
Trigger an exchange error.

- With the field, the `WWW-Authenticate` header contains `resource_metadata="…"`.
- Without it, the header omits that attribute. The `scope` and `error_uri` are identical in both
  variants.

## 6. Rollback note

Rolling back requires reverting the code first, then migration 046. Rolling back the code alone is
not enough: the pre-feature broker treats a granted virtual scope as an ordinary scope string.
After the rollback, affected clients may need to authorize again. No user session or grant data is
lost. The discarded service-scope bindings cannot be recovered by reapplying migration 046;
affected clients need fresh interactive authorization to reestablish identity pinning.

## 7. Full completion gate

```bash
just verify
```

All tasks, including migration/repository validation and the complete acceptance journeys, remain
required. Unavailable PostgreSQL/container/browser infrastructure is a validation blocker, not a
passing or optional check. The implementation report must distinguish successful commands from
remaining validation and leave unfinished tasks unchecked.
