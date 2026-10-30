# Quickstart: Validate Canonical Resource IDs

## Prerequisites

- Go toolchain and project dependencies installed.
- PostgreSQL integration environment available for migration/repository checks when running integration tests.
- Review [data-model.md](data-model.md) and [admin contract](contracts/admin-canonical-ids.md) before implementation.

## Test-first validation sequence

1. Add the Ginkgo suite `tests/e2e/canonical_resource_ids_test.go` with one `It` block per acceptance scenario in `spec.md`; use the production admin-server bootstrap and a fresh server/storage per test.
2. Verify the suite compiles and fails semantically before feature implementation. It must make real admin HTTP calls, not use skipped or placeholder assertions.
3. Add table-driven domain validation tests for absent, empty, UUID-shaped, invalid-character, 128-character valid, and 129-character invalid canonical IDs.
4. Add memory-repository tests proving canonical lookup, per-type duplicate rejection, update replacement/removal, delete release, and concurrent atomic index behavior.
5. Add PostgreSQL integration tests proving migration apply/rollback, each partial unique index, same value allowed across different tables, duplicate rejection within a table, conflict mapping, and lookup after update/delete.
6. Run the targeted Go and E2E suites, then `just check` as the local static gate.

## Required E2E scenarios

| Spec scenario | Expected proof |
|---|---|
| User Story 1.1 | Create each resource type with a canonical ID; response returns the stable UUID `id` and the `canonical_id`. |
| User Story 1.2 | GET, PUT, and DELETE each resource by both its UUID and canonical ID; client-credential operations also resolve the agent path by either form. |
| User Story 1.3 | Create without a canonical ID; UUID remains the returned and accepted identifier. |
| User Story 1.4 | Update a canonical ID by omitting it to preserve, sending `null` to remove, and sending a valid string to replace it. |
| User Story 2.1 | Write every supported relationship with the target canonical ID and verify it resolves to the target UUID. |
| User Story 2.2 | Repeat supported relationships with the target UUID. |
| User Story 2.3 | Submit an unresolved relationship and verify the write fails with no partial persistence. |
| User Story 3.1 | Read/list resources with canonical IDs; each returns its stable UUID `id` plus `canonical_id`, and every nested reference returns the target UUID. |
| User Story 3.2 | Read/list resources without canonical IDs; each returns its UUID `id` with a null `canonical_id`, and nested references return the target UUID. |
| User Story 3.3 | Read a resource with `Prefer: reference-id=canonical`; nested references render as target canonical IDs where present, the resource `id` stays UUID, and the response carries `Preference-Applied: reference-id=canonical` plus `Vary: Prefer`. |

## Edge validation

Exercise `400` for UUID-shaped and empty canonical values; `409` for a duplicate within the same table; reuse the same canonical text in a different table successfully; `404` for unknown path IDs; canonical removal followed by UUID lookup/reference; and deletion followed by reuse of the released canonical value. Also confirm that a read sending no `Prefer` or an unrecognized one returns default UUID nested references identical to the pre-feature output, and that `Vary: Prefer` is present on in-scope reads.

## Expected final commands

Use the repository's focused test commands while iterating. Before handoff run the feature E2E suite, the affected PostgreSQL integration suite, and `just check`. No frontend Playwright test is expected: the repository has no admin React client and this feature does not change the consent SPA.