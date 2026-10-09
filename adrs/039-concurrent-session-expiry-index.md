# ADR 039: Concurrent Session-Expiry Index Exception

**Status**: Accepted
**Date**: 2026-10-08
**Scope**: Phase 2d of proactive token refresh, migration `036` only

## Context

DB-001 requires an index on `user_sessions(access_token_expires_at)` for the session sweep. `AGENTS.md` requires `CREATE INDEX CONCURRENTLY` and a no-transaction directive for large tables. A normal index build can block writes to a live session table.

Constitution Principle IX requires each migration to fully apply or fully roll back on failure. PostgreSQL cannot run `CREATE INDEX CONCURRENTLY` or `DROP INDEX CONCURRENTLY` inside a transaction block. A failed concurrent build can leave an invalid index behind. That index is not usable for queries and can still add write overhead. The user selected a concurrent build with this operational risk instead of a blocking, transactional build.

The migration image in `build/docker/Dockerfile.migrate` runs the `golang-migrate` v4.17.0 CLI. The Helm Job in `charts/agentic-identity-broker/templates/job-migrate.yaml` passes `up` to that image. The Go integration harness in `tests/integration/migrations/framework.go` calls `golang-migrate` v4.20.1 directly. Neither path recognizes a SQL no-transaction directive today.

The PostgreSQL driver sends migration SQL without an explicit migration-wide transaction. Its separate version updates do not make the index build atomic. A directive comment without a reader has no effect.

Migration `030_cimd_client_uri_pattern_index.up.sql` already contains a concurrent build without a recognized directive. That historical file does not grant this exception or make migration `036` safe.

## Decision and named exception

On 2026-10-09, the user explicitly accepted this ADR and the named Principle IX non-atomic exception in the feature discussion for PR #196. This same-PR approval applies only to feature 029, despite the general new-ADR proposal rule in `AGENTS.md`.

Migration `036_user_sessions_access_token_expiry_index.{up,down}.sql` is the **only** permitted non-atomic migration under this decision. The exception applies only to a plain B-tree index named `idx_user_sessions_access_token_expires_at` on `user_sessions(access_token_expires_at)`. It does not change the atomicity requirement for other migrations or permit ad-hoc schema changes.

The UP file will contain the recognized `-- migrate:no-transaction` directive and one `CREATE INDEX CONCURRENTLY` statement. The DOWN file will contain the same directive and one `DROP INDEX CONCURRENTLY` statement. Neither file will contain `BEGIN`, `COMMIT`, or additional SQL statements. UP will not use `IF NOT EXISTS`: that clause treats an invalid index with the same name as success. Both files will use the existing `NNN_description.{up,down}.sql` naming scheme.

This is an **exception** to Principle IX's atomic-failure rule, not a way to satisfy that rule. A failed UP can leave an invalid index and a dirty `schema_migrations` version. A failed DOWN can leave an index present, absent, or invalid. Operators must inspect the database before they repair or retry either failure.

### Directive enforcement required before migration 036

The directive is a contract for a new fail-closed preflight, **not** a native `golang-migrate` feature. The implementation must provide one shared validator before it introduces migration `036`:

1. Validate the exact first-line directive and one matching concurrent statement in each `036` file. Reject other statements and transaction controls. Reject `x-multi-statement=true` and directives on unrelated migrations. If either `036` file is present, require both. Fail if a required file is unreadable.
2. Add a small guarded entrypoint to `build/docker/Dockerfile.migrate`. Validate `/app/migrations` before passing the unchanged Job arguments to the existing CLI. Both grants modes in `charts/agentic-identity-broker/templates/job-migrate.yaml` already inherit the image entrypoint. Keep both paths guarded and update their entrypoint comments. A validation error must stop the Helm hook before schema changes.
3. Call the **same validator** before every Go migration invocation that can apply `036`. The table that follows lists current callsites. Review the `Migrate(35)` pin when `036` lands.
4. Confirm that the packaged v4.17.0 CLI and the v4.20.1 integration driver each execute this SQL outside an explicit transaction. If either runner cannot do that, stop migration `036` and revise this ADR. The validator does not make the build atomic.

| Go migration path | Current route to migration SQL |
|---|---|
| `tests/integration/migrations/framework.go` | Full migration lifecycle. |
| `tests/integration/bootstrap/postgres.go` | Direct SQL file execution. |
| `tests/e2e/bootstrap/postgres.go` | Full E2E PostgreSQL bootstrap. |
| `tests/integration/infra/oauth2_signing_key_bootstrap_test.go` | Full integration migration bootstrap. |
| `tests/integration/storage/infra/postgres_test.go` | Version-selecting migration runner. |
| `tests/integration/storage/infra/thirdparty_service_test.go` | Version-selecting runner, currently pinned at 35. |
| `internal/adapters/storage/postgres/testhelpers_test.go` | Full adapter migrations and version-selecting runner. |

Use a shared guarded runner helper where packages can share one. Do not leave a path that can execute `036` without the directive check.

The validator can use a narrow allowlist for this one migration pair. It does not need a general SQL parser. The image stays separate from the broker image under ADR 009. Direct use of an unguarded `migrate` binary is outside the approved deployment path.

### Failure detection and recovery

The migration Job must fail closed on an error. The chart sets `migration.backoffLimit: 10`. Kubernetes can restart the failed Job, but `golang-migrate` refuses a dirty version. Do not clear the dirty flag or retry the DDL automatically. Before a manual retry, inspect the migration version and `dirty` flag in `schema_migrations`. With the migration role and its schema search path, inspect the named relation and index:

```sql
SELECT version, dirty FROM schema_migrations;
SELECT to_regclass('idx_user_sessions_access_token_expires_at') AS relation;
SELECT i.indrelid::regclass AS table_name, i.indisvalid,
       pg_get_indexdef(i.indexrelid) AS definition
FROM pg_index AS i
WHERE i.indexrelid = to_regclass('idx_user_sessions_access_token_expires_at');
```

Match the table and definition to this ADR. A `pg_index.indisvalid` value of `false` means that the index is invalid. No row with a non-NULL `relation` means that another relation owns the name. Investigate that collision. Do not delete that relation. A NULL `relation` means that the index is absent.

For an interrupted or failed UP at **dirty version 36**:

- If the named index is invalid, stop competing migration Jobs. Drop **only that inspected index** with `DROP INDEX CONCURRENTLY` outside a transaction. Confirm that it is absent. Then use `migrate force 35` and retry UP through the guarded runner.
- If the index is absent, investigate the original error. When the schema matches version 35, use `migrate force 35` and retry UP through the guarded runner.
- If the index is valid and has the expected table and definition, the build can have finished before version recording failed. After review, use `migrate force 36`. Do not drop a valid index or rerun UP to repair version metadata.

For an interrupted or failed DOWN at **dirty version 35**:

- If the named index is still present, inspect it. After review, use `migrate force 36` and retry DOWN through the guarded runner.
- If the named index is absent, review the failure and use `migrate force 35` only after confirming that DOWN finished.

`force` edits migration metadata only. It does not create, remove, or validate an index. If version, index identity, and schema state disagree, stop the release and get a database review. A clean version 36 with a missing or invalid index also needs investigation: the version number is not proof that the index is usable. Make sure that successful UP ends at clean version 36 with the expected valid index. Make sure that successful DOWN ends at clean version 35 with that index absent.

### Required implementation evidence

Tests in `tests/integration/migrations/migrations_test.go` must use real PostgreSQL and the shared validator. They must cover UP, DOWN, reapply, and an interrupted-build state with an invalid index and dirty version. They must reject a missing or malformed directive before execution and exercise the documented recovery path. The Helm migration image needs a smoke check of its real entrypoint. The index test in `internal/adapters/storage/postgres/user_session_test.go` must check validity and normal-planner `EXPLAIN` use for SC-006. Document the recovery commands in `docs/operations/session-sweep.md` when the guarded migration is implemented.

## Consequences and alternatives

A concurrent build lets ordinary session writes continue, but it can take longer and use more CPU and I/O than a blocking build. On failure, it can consume space and add write overhead until an operator removes the invalid index. A failed Helm pre-upgrade hook blocks the release. The operational recovery is deliberate and observable. It is **not** atomic rollback.

A blocking `CREATE INDEX` inside a transaction can meet Principle IX, but it can stop writes to a large session table. The user did not select that policy. An ignored `-- +migrate notransaction` comment cannot enforce anything. `IF NOT EXISTS` can conceal an invalid index, so it is not an acceptable recovery strategy.

## Implementation gate

The accepted exception does not make concurrent DDL atomic. The shared validator guards the image entrypoint and every listed Go runner. On 2026-10-09, both runner versions applied, dropped, and reapplied the named index against PostgreSQL 15. The image rejected an incomplete pair before SQL. Permanent migration `036` passed apply, rollback, reapply, and interrupted-build recovery tests. ADR 038 and the four admin API choices received written user approval in this conversation for PR #196.

## References

- `.specify/memory/constitution.md`: Principle IX and exception procedure.
- `AGENTS.md`: large-table concurrent-index rule and feature-029 scoped approval of these ADRs.
- `specs/029-proactive-token-refresh/spec.md`: DB-001 and SC-006.
- `specs/029-proactive-token-refresh/research.md`: R4.
- `adrs/009-separate-migration-docker-image.md`: separate deployment image.
- PostgreSQL 15: [CREATE INDEX](https://www.postgresql.org/docs/15/sql-createindex.html), [DROP INDEX](https://www.postgresql.org/docs/15/sql-dropindex.html), and [`pg_index`](https://www.postgresql.org/docs/15/catalog-pg-index.html).
