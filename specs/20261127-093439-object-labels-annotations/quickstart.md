# Quickstart: Validate Labels and Annotations

**Feature**: [spec.md](./spec.md) | **Contract**: [contracts/object-metadata.md](./contracts/object-metadata.md) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end once it is implemented. It does not replace the automated
suites. PostgreSQL and container-backed suites first run red, and later green, in the workflow on
the pushed commit (Constitution VIII and XIII). Local runs are not claimed as evidence for them.

## Prerequisites

- The Go toolchain and the tools from `just install-tools`.
- Docker or Podman for `just test-integration-infra` and `just test-e2e-docker`.
- `curl` and `jq` for the manual checks.
- An existing Permission Set with canonical ID `github-read`.

## Automated validation

| Command | Proves |
|---|---|
| `just check` | Formatting, vet, and lint, including `forbidigo` and `telemetrylint` |
| `go test ./internal/domain/model/ -run ObjectMetadata` | FR-004 to FR-009 tables: grammar, reserved prefix, the 64-label limit, the 65,536-byte limit, NUL, literal matching, and map equality |
| `go test ./internal/adapters/http/handlers/admin/ -run Metadata` | JSON shape, `null`, duplicates, invalid UTF-8, the top-level `metadata` spelling, the fixed messages that contain no submitted text, and the 512 KiB caps on Agent PUT and collection PUT |
| `go test ./internal/domain/agents/ ./internal/domain/permissionset/ ./internal/domain/thirdparty/` | Agent no-op map equality, Service omission and replacement in `prepareUpdate`, and validation before discovery and encryption side effects |
| `just test` | All of the above, plus round-trips in the memory adapter |
| `just test-integration-infra` | Migration `050` up, down, and up again on the production chain. Pre-migration rows read as empty. PostgreSQL round-trips are exact, and the `jsonb_typeof` checks reject a JSON `null` |
| `just test-e2e-backend` | US1.1, US1.3–US1.5, US2.1–US2.5, and US3.1–US3.5 on the in-memory broker |
| `just test-e2e-docker` | US1.2: a reordered replay against PostgreSQL makes no write and emits no audit record |
| `just telemetry-doc` | `docs/reference/telemetry.md` is unchanged, or the regenerated version is committed |

Expected: every command passes, and each of the 15 acceptance scenarios maps to exactly one
`It()` in `tests/e2e/object_metadata_test.go`.

## Manual journey (in-memory broker)

Set up the shell:

```bash
just run
```

```bash
ADMIN=http://localhost:14000
```

```bash
H=(-H 'X-Remote-User: agents-sync' -H 'Content-Type: application/json')
```

```bash
SEL='acme%2Freview-bot'
```

1. **Reconcile with metadata** (US1.1). Send Journey 1, Step 1 from the spec:
   `curl -si -X PUT "${H[@]}" "$ADMIN/api/agents?external_id=$SEL" -d @step1.json`.
   Expect `201`. The response has `metadata.labels` and `metadata.annotations`, with keys in byte
   order.
2. **Reordered replay** (US1.2). Send the same maps with their keys in a different order. Expect
   `200` and the same `updated_at`. The broker log has no `agents.agent.updated` record.
3. **One label changed** (US1.3). Change `deployment` to `external`. Expect `200`, a later
   `updated_at`, and one `agents.agent.updated` record. Search the log for the label value with
   `grep -c external`; it is not in the record.
4. **Omit, `{}`, and `null`** (US1.4, US1.5). A body without `metadata` returns a response with no
   `metadata`. A body with `"metadata": null` returns
   `400 {"error":"validation failed","message":"metadata must not be null"}`, and a following
   `GET` shows the stored Agent unchanged.
5. **Reserved and invalid keys** (US2.1, US2.2, US2.4). Send Journey 4 from the spec. Expect
   `400` with `metadata.labels contains a reserved key`. Then send the label value `"external "`
   with a trailing space. Expect `metadata.labels contains an invalid value`. Neither response
   body contains the key or the value.
6. **Limits** (US2.3). Build 64 labels and annotations of exactly 65,536 bytes with `jq`, and send
   them in an Agent ID-addressed `PUT`. Expect `200`. Add a 65th label, or one more annotation
   byte. Expect `400`.
7. **Permission Set replace** (US3.1, US3.2). Send Journey 2 from the spec. After the `PUT`
   without `metadata`, the response and a following `GET` omit `metadata`.
8. **Service keep and replace** (US3.3, US3.4). Send Journey 3 from the spec. A `PUT` without
   `metadata` keeps `region` and `tier`. A `PUT` with `{"labels":{"tier":"platinum"}}` leaves
   only `tier`.
9. **Admin-only** (US3.5). Open the delegation overview and agent detail as `alice@example.com`
   on `:8000`, and look for the field with `curl ... | grep -c metadata`. Expect `0`, and no label
   or annotation value.

## Manual upgrade check (PostgreSQL)

1. Migrate a scratch database to version `49`, and insert one Agent, one Service, and one
   Permission Set.
2. Run `migrate up`. All three read with empty metadata, and Admin responses omit `metadata`.
3. Write metadata through the Admin API. Then run `migrate down 1` followed by `migrate up`. The
   other data in the three rows is unchanged, and the metadata is empty again. Rollback loses only
   metadata.
