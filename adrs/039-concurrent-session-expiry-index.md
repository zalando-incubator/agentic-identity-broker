# ADR 039: Concurrent Session-Expiry Index Exception

**Status**: Accepted
**Date**: 2026-10-08
**Scope**: Phase 2d of proactive token refresh, migration `037` only

## Context

DB-001 requires an index on `user_sessions(access_token_expires_at, id)` for the session sweep. `AGENTS.md` requires `CREATE INDEX CONCURRENTLY` and a no-transaction directive for large tables. A normal index build can block writes to a live session table.

Constitution Principle IX requires each migration to fully apply or fully roll back on failure. PostgreSQL cannot run `CREATE INDEX CONCURRENTLY` or `DROP INDEX CONCURRENTLY` inside a transaction block. A failed concurrent build can leave an invalid index behind. That index is not usable for queries and can still add write overhead. The user selected a concurrent build with this operational risk instead of a blocking, transactional build.

The migration image in `build/docker/Dockerfile.migrate` runs the `golang-migrate` v4.17.0 CLI. The Helm Job in `charts/agentic-identity-broker/templates/job-migrate.yaml` passes `up` to that image. The Go integration harness in `tests/integration/migrations/framework.go` calls `golang-migrate` v4.20.1 directly. Neither path recognizes a SQL no-transaction directive today.

The PostgreSQL driver sends migration SQL without an explicit migration-wide transaction. Its separate version updates do not make the index build atomic. A directive comment without a reader has no effect.

Migration `030_cimd_client_uri_pattern_index.up.sql` already contains a concurrent build without a recognized directive. That historical file does not grant this exception or make migration `037` safe.

## Decision and named exception

On 2026-10-09, the user explicitly accepted this ADR and the named Principle IX non-atomic exception in the feature discussion for PR #196. This same-PR approval applies only to feature 029, despite the general new-ADR proposal rule in `AGENTS.md`.

Migration `037_user_sessions_access_token_expiry_index.{up,down}.sql` is the **only** permitted non-atomic migration under this decision. On 2026-10-09, the stakeholder approved amending this single-migration exception from a single-column index to a plain B-tree index named `idx_user_sessions_access_token_expires_at` on `user_sessions(access_token_expires_at, id)`. The UP statement is exactly `CREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at, id);`. The index keys follow the sweep's expiry-then-UUID keyset order. This amendment does not change the atomicity requirement for other migrations or permit ad-hoc schema changes.

On 2026-10-12, before this feature PR merged, the stakeholder amended this decision in place. Remove the guard, but keep the separate migration image and Helm migration/grants Job. Accept the loss of pre-execution validation. This is the final accepted feature-branch decision.

On 2026-10-12, merging `main` into this unmerged PR renumbered the migration from `036` to `037`. `main` already uses version 36 for `036_add_refresh_token_grant_id.{up,down}.sql`, and `golang-migrate` rejects two migrations with one version. The SQL, index, and named exception are unchanged.

The UP and DOWN files each retain the first-line `-- migrate:no-transaction` comment required by `AGENTS.md`. The comment is informational because neither `golang-migrate` runner interprets it. UP contains one `CREATE INDEX CONCURRENTLY` statement, and DOWN contains one `DROP INDEX CONCURRENTLY` statement. Neither file contains `BEGIN`, `COMMIT`, or additional SQL statements. UP does not use `IF NOT EXISTS`, which can conceal an invalid index. Both files use the existing `NNN_description.{up,down}.sql` naming scheme.

This is an **exception** to Principle IX's atomic-failure rule, not a way to satisfy that rule. A failed UP can leave an invalid index and a dirty `schema_migrations` version. A failed DOWN can leave an index present, absent, or invalid. Operators must inspect the database before they repair or retry either failure.

### Direct migration execution

The separate image in `build/docker/Dockerfile.migrate` invokes `/usr/local/bin/migrate` directly. It packages version-matched SQL at `/app/migrations`. Both Helm grants modes pass `-path /app/migrations -database ... up`. The optional grants container still runs after migration. The broker image has no migration tool (ADR 009).

Go test runners use their existing migration directories and invoke `golang-migrate` directly. Neither image nor Go runner performs a shared SQL preflight. The PostgreSQL drivers execute migration SQL without a migration-wide transaction. Their separate version updates do not make concurrent DDL atomic.

No preflight checks the SQL pair, statement text, URL options, or alternate image migration paths. A missing DOWN can remain unnoticed until after UP succeeds, leaving rollback unavailable. Incorrect SQL can execute before the runner reports an error. The informational comment does not prevent either outcome. If a driver starts wrapping migration `037` in a transaction, the concurrent SQL will fail. Review this decision before changing either runner.

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

Match the table and the ordered B-tree keys `(access_token_expires_at, id)` to this ADR. A `pg_index.indisvalid` value of `false` means that the index is invalid. No row with a non-NULL `relation` means that another relation owns the name. Investigate that collision. Do not delete that relation. A NULL `relation` means that the index is absent.

For an interrupted or failed UP at **dirty version 37**:

- If the named index is invalid, stop competing migration Jobs. Drop **only that inspected index** with `DROP INDEX CONCURRENTLY` outside a transaction. Confirm that it is absent. Then use `migrate force 36` and retry UP through the migration image.
- If the index is absent, investigate the original error. When the schema matches version 36, use `migrate force 36` and retry UP through the migration image.
- If the index is valid and has the expected table and definition, the build can have finished before version recording failed. After review, use `migrate force 37`. Do not drop a valid index or rerun UP to repair version metadata.

For an interrupted or failed DOWN at **dirty version 36**:

- If the named index is still present, inspect it. After review, use `migrate force 37` and retry DOWN through the migration image.
- If the named index is absent, review the failure and use `migrate force 36` only after confirming that DOWN finished.

`force` edits migration metadata only. It does not create, remove, or validate an index. If version, index identity, and schema state disagree, stop the release and get a database review. A clean version 37 with a missing or invalid index also needs investigation: the version number is not proof that the index is usable. Make sure that successful UP ends at clean version 37 with the expected valid index. Make sure that successful DOWN ends at clean version 36 with that index absent.

### Required implementation evidence

Tests in `tests/integration/migrations/migrations_test.go` use real PostgreSQL. They cover UP, DOWN, reapply, and interrupted invalid-index recovery with a dirty version. They confirm both ordered index keys and preserved session data. The separate migration image needs a smoke check of its direct CLI entrypoint. The PostgreSQL adapter test checks valid index use for first, middle, and late pages under normal planner settings. The failure detection and recovery decision is in this ADR.

## Consequences and alternatives

A concurrent build lets ordinary session writes continue, but it can take longer and use more CPU and I/O than a blocking build. On failure, it can consume space and add write overhead until an operator removes the invalid index. A failed Helm pre-upgrade hook blocks the release. Without preflight, a missing DOWN can leave rollback unavailable after UP. The operational recovery is deliberate and observable. It is **not** atomic rollback.

A blocking `CREATE INDEX` inside a transaction can meet Principle IX, but it can stop writes to a large session table. The user did not select that policy. An ignored `-- +migrate notransaction` comment cannot enforce anything. `IF NOT EXISTS` can conceal an invalid index, so it is not an acceptable recovery strategy.

## Implementation gate

On 2026-10-12, PostgreSQL 15 integration tests passed UP, DOWN, reapply, invalid-index interruption, and dirty-version recovery through direct Go runners. They confirmed both ordered index keys. The direct ARM64 migration image applied, dropped, and reapplied `036` on disposable PostgreSQL (as migration `036`, before the `main` merge renumbered it to `037`). UP ended at clean version `36` with a valid two-key index. DOWN ended at clean version `35` without that index. Both Helm grants modes retained their migration arguments and grants order. The image, the runbook, and the tests do not make concurrent DDL atomic.

After the renumbering, direct Go-runner PostgreSQL tests passed UP, DOWN, reapply, invalid-index interruption, and dirty-version recovery at versions `36`/`37` on 2026-10-12.

## References

- `.specify/memory/constitution.md`: Principle IX and exception procedure.
- `AGENTS.md`: large-table concurrent-index rule and feature-029 scoped approval of these ADRs.
- `specs/029-proactive-token-refresh/spec.md`: DB-001 and SC-006.
- `specs/029-proactive-token-refresh/research.md`: R4.
- `adrs/009-separate-migration-docker-image.md`: separate deployment image.
- PostgreSQL 15: [CREATE INDEX](https://www.postgresql.org/docs/15/sql-createindex.html), [DROP INDEX](https://www.postgresql.org/docs/15/sql-dropindex.html), and [`pg_index`](https://www.postgresql.org/docs/15/catalog-pg-index.html).
