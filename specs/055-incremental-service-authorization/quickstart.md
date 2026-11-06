# Quickstart: Validate Incremental Service Authorization

**Feature**: [spec.md](./spec.md) | **Contracts**: [contracts/](./contracts/) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end after implementation. It references contracts instead of
repeating them. All values are synthetic and follow the spec's shared cast.

## Prerequisites

- `just install-tools` has completed, including Playwright browsers.
- Docker or Podman is available for infrastructure-backed integration tests and agentgateway E2E
  scenarios.
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
readable.

## 3. Backend acceptance journeys (spec Journeys 1, 3, and 4)

```bash
just test-e2e
```

Run the suites below, or the whole E2E set:

| Suite | Proves |
|---|---|
| `tests/e2e/incremental_authorization_test.go` | Story 1, Scenarios 1–7; Story 3, Scenarios 1 and 4; Story 4, Scenarios 2 and 3. It covers the `scope` member on exchange errors, refresh widening that is refused without rotation, focused consent, `requested_service_ids`, submission enforcement, code exchange with visible virtual scopes, local refresh recheck, and the rename and reuse cases. |
| `tests/e2e/incremental_authorization_proxy_test.go` | Story 3, Scenarios 2 and 3. A mock upstream IdP records the received `scope`: virtual scopes never appear, and the rotated upstream refresh token is returned unchanged. |
| `tests/e2e/service_session_page_test.go` | Story 2, Scenarios 1–4 at the API level: `GET /api/third-party/sessions/{service}` for both identifier forms, `session: null`, `401` and `404`, and the recovery return to `/sessions/<identifier>`. |
| `tests/e2e/extproc/incremental_authorization_challenge_test.go` | Story 1, Scenario 1 (gateway half) and Story 4, Scenario 1: the challenge with and without `resource_metadata_url` metadata. |
| `tests/e2e/frontend/incremental_authorization_test.go` | Consent States C and D, plus the service page States A and B. It saves screenshots to `tests/e2e/screenshots/`. |

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
lost.
