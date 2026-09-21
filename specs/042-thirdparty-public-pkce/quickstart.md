# Quickstart: Validating Public Client Support

**Feature**: `042-thirdparty-public-pkce` | **Date**: 2026-09-10

Runnable validation scenarios that prove the feature works end to end. Contract details live in
[contracts/admin-api.md](./contracts/admin-api.md) and
[contracts/upstream-token-requests.md](./contracts/upstream-token-requests.md); entity and schema
details live in [data-model.md](./data-model.md). This guide does not repeat them.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| Go 1.26.8 | `just build` must succeed |
| Docker or Podman | Only for the infra-backed integration layer (`just test-integration-infra`) |
| Playwright browsers | Not needed — this feature changes no UI |

---

## Validation ladder

Run in this order. Each rung is cheap relative to the next and localizes failures.

```bash
just check                                              # 1. fmt, vet, lint
go test ./internal/domain/model/... ./internal/domain/thirdparty/...   # 2. domain unit
go test ./internal/adapters/storage/... ./internal/adapters/http/...   # 3. adapter unit
just test-integration-infra                             # 4. migration 031 + postgres repo
ginkgo -v --label-filter="!performance" --focus="Public Client" ./tests/e2e/   # 5. acceptance
just verify                                             # 6. full gate
```

---

## Scenario 1 — Register a public service (User Story 1)

Start the broker with an in-memory backend and register a service with no credential.

```bash
just build && ./bin/agentic-identity-broker --config examples/config/local.yaml &
```

```bash
curl -sS -X POST http://localhost:14000/api/services \
  -H 'Content-Type: application/json' \
  -d '{
        "display_name": "Public Provider",
        "client_id": "public-client-abc",
        "token_endpoint_auth_method": "none",
        "issuer_uri": "https://provider.example.com",
        "discovery": {"enable_discovery": true}
      }'
```

**Expected**: `201 Created`. The body reports `"token_endpoint_auth_method": "none"` and contains
**no** `client_secret` property — not `"REDACTED"`, not `null`, absent.

**Then read it back**:

```bash
curl -sS http://localhost:14000/api/services | jq '.[] | {display_name, token_endpoint_auth_method, has_secret: has("client_secret")}'
```

**Expected**: the public entry shows `"token_endpoint_auth_method": "none"` and
`"has_secret": false`; any confidential entry shows `null` and `true`.

### Validation to confirm

| Request | Expected |
|---|---|
| `token_endpoint_auth_method: null` | `200`/`201` — identical to omitting the property |
| `token_endpoint_auth_method: "client_secret_post"` | `400`, message names `none` as the only accepted value |
| `token_endpoint_auth_method: "none"` with a non-empty `client_secret` | `400`, message names the contradiction |
| `token_endpoint_auth_method: "none"` with `oauth2_flavor: "google"` | `400`, message names the `google` variant |
| `token_endpoint_auth_method: "none"` with no `client_id` | `400`, `client_id is required` |
| No method, no `client_secret` | `400`, `client_secret is required` — **unchanged from today** |

**Round trip**: `GET` a confidential service, change `display_name`, and `PUT` the document back
unmodified otherwise — including the `"token_endpoint_auth_method": null` the read returned. It
must succeed and leave the service confidential (US4-S6).

---

## Scenario 2 — Complete the connect journey (User Story 2)

Exercised by the E2E suite against the strict-mode upstream double, which **rejects** any request
carrying client credentials and **verifies** the PKCE code verifier
(see [contracts/upstream-token-requests.md §5](./contracts/upstream-token-requests.md#5-test-double-conformance)).

```bash
ginkgo -v --label-filter="!performance" --focus="authorizes a user against a public-client provider" ./tests/e2e/
```

**Expected**:

1. The authorization redirect carries `code_challenge` and `code_challenge_method=S256`.
2. The captured token request carries `client_id` and `code_verifier`.
3. The captured token request carries **no** `client_secret` form value.
4. The captured token request carries **no** `Authorization` header — including none with an empty
   password.
5. An encrypted `UserSession` exists for the principal and service afterwards.

If the double returns `401 invalid_client`, the broker sent a credential and the test has caught
exactly the regression it exists to catch.

---

## Scenario 3 — Refresh a public-client session (User Story 3)

```bash
ginkgo -v --label-filter="!performance" --focus="keeps a public-client session alive" ./tests/e2e/
```

**Expected**: the captured refresh request body is
`grant_type=refresh_token&refresh_token=…&client_id=…` with no `client_secret`, no `Authorization`
header, and the new tokens replace the stored ones. On upstream rejection the failure surfaces and
no second, credentialed attempt is captured.

---

## Scenario 4 — Switch a service between modes (User Story 4)

```bash
# confidential -> public: the stored credential is removed
curl -sS -X PUT http://localhost:14000/api/services/$SERVICE_ID \
  -H 'Content-Type: application/json' \
  -d '{"display_name":"Now Public","client_id":"public-client-abc","token_endpoint_auth_method":"none","issuer_uri":"https://provider.example.com","discovery":{"enable_discovery":true}}'
```

**Expected**: `200`, response omits `client_secret`. Confirm the credential is gone at the storage
layer, not merely hidden:

```bash
psql "$DATABASE_URL" -c \
  "SELECT display_name, token_endpoint_auth_method, client_secret_encrypted IS NULL AS secret_removed
   FROM thirdparty_oauth2_services WHERE id = '$SERVICE_ID';"
```

**Expected**: `none | t`. A dormant ciphertext here is a failure of SR-006.

| Update request on a public service | Expected |
|---|---|
| Omits the method, omits `client_secret` | `400`; the service remains public and otherwise untouched |
| Omits the method, supplies `client_secret` | `200`; the service becomes confidential and subsequent upstream requests carry the credential |
| Declares `none`, changes only `display_name` | `200`; still public, still no credential |

Stored user sessions survive every transition; the next refresh uses the new setting (FR-022).

---

## Scenario 5 — Migration apply and guarded rollback

```bash
just test-integration-infra
```

The migration suite (`tests/integration/migrations/`) must show:

1. `031` applies cleanly on a database populated through `030`.
2. Every pre-existing row has `token_endpoint_auth_method IS NULL` — no backfill (DB-004).
3. The `CHECK` constraint rejects a direct `INSERT` of a contradictory row in both directions:
   a method of `none` with a ciphertext, and a null method with no ciphertext (DB-005).
4. Rolling back **fails** with an error naming the blocking services while a public service exists
   (DB-006), and the schema is left intact — `token_endpoint_auth_method` still exists, so the
   guard aborted the whole step rather than dropping half of it.
5. The failed rollback leaves `schema_migrations.dirty = true`. This is expected: golang-migrate
   sets the flag before running a step and clears it only on success. Recovery is
   `Force(31)` in the test framework, or `migrate force 31` for an operator.
6. Rolling back **succeeds** once the flag is cleared and no public service remains, and the schema
   re-applies cleanly.

---

## Scenario 6 — Existing services are untouched (SC-004)

```bash
just test-e2e-backend          # every pre-existing suite, unmodified
```

**Expected**: all pre-existing third-party suites — `oauth2_provider_flavor_test.go`,
`oauth2_provider_github_flavor_test.go`, `provider_authorization_params_test.go`,
`oauth2_token_test.go`, `canonical_resource_ids_test.go` — pass without modification. Any change
required in those files means confidential behaviour was altered, which FR-017 forbids.

---

## Scenario 7 — No credential material in logs (SC-008)

```bash
./bin/agentic-identity-broker --config examples/config/local.yaml 2>&1 | tee /tmp/broker.log
# ... run scenarios 1-4 ...
grep -Ei 'client_secret=|code_verifier|code_challenge|REDACTED_ACTUAL|access_token=|refresh_token=' /tmp/broker.log
```

**Expected**: no matches beyond structured field *names* with redacted or absent values. Audit
entries for the paths this feature touches carry an `event` key and a `public_client` attribute
(see [research.md §D9](./research.md#d9--audit-logging)).

---

## Definition of done

- [ ] `just check` clean
- [ ] `just test` green
- [ ] `just test-integration-infra` green, including the rollback-refusal test
- [ ] `just test-e2e-backend` green, with all 22 acceptance scenarios mapped, each carrying its
      `// USx-Sy from specs/042-thirdparty-public-pkce/spec.md` traceability comment
- [ ] No pre-existing E2E **scenario or assertion** changed. `tests/e2e/helpers/mock_upstream.go`
      and `tests/e2e/fixtures/services.go` are extended by design (D10) — additively, with strict
      mode default-off — so every existing suite runs against unchanged behaviour. A required
      change to an existing `_test.go` scenario, by contrast, means confidential behaviour was
      altered, which FR-017 forbids.
- [ ] Scenario 7 produces no credential material in captured output
