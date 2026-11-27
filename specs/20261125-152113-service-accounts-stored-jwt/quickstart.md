# Quickstart: Validate Service Accounts with a Stored JWT

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

This guide lists the runs that prove the feature end to end. Contracts are in
[contracts/admin-service-accounts-openapi.yaml](./contracts/admin-service-accounts-openapi.yaml)
and [contracts/observability.md](./contracts/observability.md); rules and schema are in
[data-model.md](./data-model.md).

## Prerequisites

- Go, `just` and Ginkgo as pinned by the repository (`just install-tools`).
- Docker or Podman for the PostgreSQL-backed runs. Without a container runtime, those runs are
  left to CI (Constitution Principle VIII).
- No configuration change: the feature adds no configuration parameter, Helm value or dependency.

## 1. Static checks

```bash
just check
```

Expected: formatting, vet and lint pass, including `telemetrylint` (no `model.Secret` in a log or
span attribute) and `TestLoggedPrincipalsAreTyped`.

## 2. Unit tests

```bash
just test
```

Expected to pass, among others:

| Package | Proves |
|---|---|
| `internal/domain/model` | Every credential rule of [data-model.md](./data-model.md#validation-of-a-signed_jwt-credential) with its reason and message; summary and fingerprint; status at, before and after `exp` |
| `internal/domain/thirdparty` | Create, rotation, ceiling change, rename, no-op, precondition, delete, bound-account refusal, service-delete guard, encryption failure stores nothing, audit records, credential absent from logs |
| `internal/adapters/storage/memory` | Repository contract |
| `internal/adapters/http/handlers/admin` | Status codes, headers, error envelope, operator required, credential absent from responses, logs and spans |
| `internal/domain/id` | `ServiceAccountID` parsing and serialization |

## 3. Acceptance tests (memory store)

```bash
ginkgo -v --label-filter="!performance && !docker" --focus "Service accounts" ./tests/e2e/
```

Expected: one passing `It()` for each acceptance scenario of the spec except the four
store-dependent scenarios of step 4. The mapping
is in [plan.md](./plan.md#scenario-mapping).

## 4. PostgreSQL

```bash
just test-integration-infra
ginkgo -v --procs=1 --label-filter="docker" --focus "Service accounts" ./tests/e2e/
```

Expected:

- Migration `050` applies, rolls back and reapplies; the unique constraint, the checks and the
  `RESTRICT` foreign key hold.
- The PostgreSQL repository passes the same contract test as the memory repository.
- User Story 1 Scenario 5, User Story 3 Scenarios 7 and 9, and User Story 5 Scenario 4 pass on
  both stores.

## 5. Manual journey (optional)

Start the broker with the development configuration and an existing service, then replay
[Journey 1](./spec.md#concrete-example-journeys-mandatory)
against the admin server with a JWT signed by any key you hold.

| Step | Request | Expected |
|---|---|---|
| Register | `POST /api/services/{service}/service-accounts` | `201`, `ETag: "1"`, summary with `iss`, `sub`, `kid`, `exp`, fingerprint; no `credential` member |
| List | `GET /api/services/{service}/service-accounts` | One entry, same representation |
| Status | `GET .../{account}/status` | `ready`, `failure_reason: null`, the token's expiry |
| Rotate | `PUT .../{account}` with a second JWT and `If-Match: "1"` | `200`, `ETag: "2"`, new `kid`, `exp` and fingerprint, same `id` |
| Stale write | The same `PUT` again with `If-Match: "1"` | `412`, nothing changed |
| Refuse | `POST` with an `alg: none` token | `400`, `credential.jwt must be a signed compact JWS`, list unchanged |
| Delete service | `DELETE /api/services/{service}` | `409` while the account exists |
| Remove | `DELETE .../{account}` | `204`; the list is `[]` |

In the broker log, expect one record each for `service.service_account.created`,
`service.service_account.credential_rotated`, `service.service_account.credential_rejected` and
`service.service_account.deleted`. Searching the log for any segment of either JWT finds nothing.

## 6. Reference pages and full gate

```bash
just telemetry-doc
just verify
```

Expected: `docs/reference/telemetry.md` lists the seven `storage.*.service_account` spans and is
committed; the full gate passes.
