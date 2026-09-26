# Business Event Ledger — Validation Quickstart

This is the execution guide for the planned implementation. The plan command does not install a working ledger or create its Go tests. Commands against the new schema package, labelled ledger scenarios, tables or functions become runnable after their corresponding implementation phase. Existing repository commands were checked with `just --list`; no runtime acceptance result is implied by this guide.

## Prerequisites

- Go 1.27.1, just, the repository-pinned Ginkgo CLI, and frontend build prerequisites for production bootstrap.
- Docker or Podman for PostgreSQL acceptance/integration; use the shared bootstrap rather than a new container per test.
- PostgreSQL matching the existing deployment/test baseline, with broker DML credentials distinct from migration/maintenance credentials and authorized operational query/erase access.
- A local OTLP receiver configured through the existing telemetry settings. No real credentials or production telemetry destination in the test environment.
- Reviewed/published event schemas, migrations applied, matching broker build, and the five-minute partition-maintenance schedule installed.

Use [configuration.md](contracts/configuration.md), [storage.md](contracts/storage.md), and [data-model.md](data-model.md) for exact settings, SQL operations and lifecycle invariants. Do not duplicate those contracts in test helper constants.

## 1. Static and focused validation

From the repository root after implementing the relevant packages/scenarios:

```bash
just check
go test ./api/events/... ./internal/domain/ledger/...
go test -race ./internal/adapters/storage/memory/...
just web-build
ginkgo -v --procs=1 --label-filter='business-event-ledger && !performance' ./tests/e2e/
```

The existing `just test-e2e-backend` recipe runs these memory scenarios as part of the normal E2E suite; the direct command above isolates them.

Expected: schema resolution is offline; unknown types/extra data/invalid references fail; all memory catalogue workflows pass without PostgreSQL. Each backend iteration inside a scenario bootstraps its own server and storage (a fresh memory adapter or a fresh template-database clone) and releases them with `DeferCleanup`; iterations share no state. At the required red phase the implemented tests must compile and fail semantically because the ledger guarantees are absent. Do not accept an empty focus result as a passing suite.

## 2. PostgreSQL, migrations and crash recovery

```bash
just test-integration-infra
ginkgo -v --tags=integration --procs=1 --label-filter='business-event-ledger && !performance' ./tests/e2e/
```

T016 wraps this command in `just test-e2e-ledger-postgres`, adds that recipe to `just verify`, and runs it in a dedicated CI lane. Verify all 21 mapped scenario IDs across memory, tagged PostgreSQL and performance lanes, not simply a successful command exit. The tagged lane runs every shared scenario against both memory and PostgreSQL through the build-tag-selected backend list in `tests/e2e/bootstrap/`, and US1-AS7 compares the two backends directly. That run is the SC-008 evidence for event contents, query results and erasure results. US3-AS2/AS3/AS4 need database-clock seeding or a process restart and run on PostgreSQL only; their memory equivalence comes from the memory lifecycle and retention-worker tests (T072, T074). Record both evidence sources.

Expected PostgreSQL evidence:

1. All business state-change events commit atomically. Forced validation/append failure leaves neither changed state nor its success event.
2. Kill a production-bootstrap child process after observing a committed row but before response/export; restart against the same database. Business state and one retained event remain, and the missing ledger telemetry copy is emitted with that ID.
3. Export success followed by acknowledgement failure permits a same-ID telemetry duplicate, never a duplicate authoritative event.
4. Competing revoke/terminate/approval/credential operations produce only actual transition events; repeated expiry recognition remains silent even after ledger erasure.
5. Migrations apply/down/apply on a disposable database and preserve pre-existing business records. Down removes feature history by design; never use a production database to test it.

Do not replace the child-process crash with a memory-adapter restart. Keep deterministic fault control in test helpers and port wrappers, not production flags/endpoints. Failure-side security revocations in OAuth2 replay handling must continue to work when issuance fails.

## 3. Operational investigation

Run the registered multi-principal/agent token workflows through the real end-user/admin HTTP bootstrap. Use operational credentials and exact subject/time filters from the storage contract. For example, in an authorized psql session:

```sql
\set principal 'principal-example'
\set start '2026-09-26T00:00:00Z'
\set end '2026-09-27T00:00:00Z'
SELECT DISTINCT envelope->>'agent_id' AS receiving_agent
FROM public.business_events
WHERE subject = :'principal'
  AND type = 'agentic-identity-broker.token-exchanged'
  AND outcome = 'success'
  AND occurred_at >= :'start'::timestamptz
  AND occurred_at < :'end'::timestamptz
ORDER BY receiving_agent;
```

Expected: exact known receiving-agent set; no unrelated principals, failed exchanges or credentials. Deleting a referenced business object does not remove or anonymize its event.

## 4. Erasure, retention and non-resurrection

In a disposable test dataset, connected as the role named by `migration.grants.businessEvents.erasureRole` (or the migration owner), in autocommit mode:

```sql
SELECT public.business_event_erase_subject('principal-example');
SELECT count(*) FROM public.business_events WHERE subject = 'principal-example';
SELECT public.business_event_erase_subject('principal-example');
```

Expected: first call reports the removed count; count is zero; repeated erasure returns zero; other subjects remain unchanged. A concurrent recorder either commits before the erasure boundary and is deleted or commits after it as a legitimate new occurrence. Delayed delivery cannot emit deleted records. The broker role must be denied access to the erasure function.

Exercise retention through the integration harness with records just before/after boundaries and the real maintenance function. The database clock drives maintenance and `recorded_at` is immutable, so seed history as the migration owner: call `public.business_event_create_partition_pair` for the needed past six-hour windows, insert registry-validated envelopes with explicit historical `recorded_at`, and let live workflows supply current events. With maintenance credentials:

```sql
SELECT public.business_event_maintain_partitions();
```

Expected: only whole eligible six-hour partitions and their delivery references disappear; none younger than retention; current plus future windows exist; default 90-day and changed positive durations obey the same no-early-deletion rule. Scheduler catch-up after downtime is automatic.

The migration and the pre-install/pre-upgrade job call `SELECT public.business_event_provision_partitions();` instead, which creates current and future windows and never drops. Verify a retention increase, for example `720h` to `2160h`, with seeded 30–90-day history: provisioning under the stored 720h policy removes nothing; with the CronJob suspended, the new broker stores 2160h at startup; the next maintenance run keeps the 30–90-day partitions. Runtime credentials cannot create/drop partitions or invoke maintenance. Setting telemetry copying false does not stop this Job or ledger writes.

Check rendered deployment separately:

```bash
just helm-lint
just helm-template
```

Expected: PostgreSQL maintenance CronJob uses migration credentials and schedule and renders no `spec.suspend`; the pre-upgrade migration job calls provisioning, not maintenance; broker Deployment does not receive those credentials; false telemetry-copy setting is preserved; memory deployment has no PostgreSQL maintenance Job. Keep the two replicas on one retention policy during rollout.

## 5. Credential safety and monitoring continuity

Exercise every catalogue type with distinct access-token, refresh-token, client-secret, client-assertion, raw-JWT, authorization-code and PKCE-verifier canaries on applicable success/failure paths. Also inject them into user-agent comments, upstream error text, URLs, tool arguments and optional opaque session strings.

Expected: zero canary values anywhere in stored envelopes or ledger telemetry copies. Do not limit assertions to fields named token or secret. Reasons are controlled templates; normalized user-agent output contains only an allowed family or is absent. Domain identity provenance—not a regex—determines whether a subject is safe.

Compare existing slog output with the Phase 0 baseline in `tests/e2e/fixtures/business_event_ledger_legacy_slog.go`: names, levels, messages and field keys unchanged. Confirm the ledger telemetry copy's EventName and every flat envelope attribute, including null versus absent, empty data, arrays, and matching trace/span. Disable only business_events.telemetry_copy_enabled: ledger writes and old telemetry remain, additional copies stop. Re-enable: only retained pending work resumes, not occurrences recorded while disabled. Stop the collector: business commits remain durable; pending work is recoverable; request processing does not wait for collector export.

## 6. Performance measurement

The fixed acceptance target is added recording transaction p99 <=5 ms. No current deployment measurement was supplied; the following protocol produces the evidence rather than inventing it.

1. In T007, request an operator-approved representative deployment profile from the operations owner of the target deployment. Required fields: action mix, throughput, concurrency, principals/agents/services, permission-set sizes, event sizes, 90-day retained volume, hardware/PostgreSQL settings, network placement, database pool, telemetry enablement and receiver behavior. Record the approver, approval date and the profile's storage location in the table below, and keep the profile with the measurement report. If no approved profile exists, record that as an open release blocker; the reference profile does not replace it.

   | Deployment profile | Value |
   |---|---|
   | Operations owner / approver | *pending (T007)* |
   | Approval date and reference | *pending (T007)* |
   | Profile location | *pending (T007)* |
2. Also use a deterministic reference profile: 10,000 principals, 100 agents, 20 services, 1,000,000 preseeded safe ledger events distributed across 90 days, concurrency 32, and a workflow mix of 50% exchanges, 20% approval transitions, 10% grant updates, 10% session refreshes, and 10% admin mutations. This is a reproducible reference, not a claim about production load. Compare equivalent business datasets; the baseline has no ledger table.
3. Build the pre-feature revision and feature revision in separate worktrees without modifying the active checkout. Warm each for two minutes, then run at least 100,000 measured actions per profile, alternating baseline/enabled order over three repetitions. Use identical collector and backend settings. No production ledger-disable switch is added for benchmarking.
4. Capture full transaction latency samples for each version plus feature recording-span duration (event validation, serialization, append, delivery-reference insert and attributable transaction work). Report p50/p95/p99, allocation data, throughput and error/rollback counts. Report both the difference of baseline/enabled transaction p99 and the recording-duration p99; do not call the former a paired per-request percentile.
5. Assert that the feature run retained exactly one event of the expected type for every completed catalogued action and contains no credential canary; before implementation this assertion fails, so the scenario is semantically red rather than trivially within budget.
6. Fail acceptance if added transaction p99 or measured recording p99 exceeds 5 ms, or any atomicity/credential requirement is weakened. Repeat under the deployment profile; the synthetic reference alone is insufficient for the 'current load' claim.

Run the performance-labelled acceptance separately through the existing recipe, using the label-filter and build-tag parameters T016 adds:

```bash
just test-e2e-performance 'performance && business-event-ledger' integration
```

The benchmark harness/report is an implementation deliverable, not generated measurement evidence in this plan.

## 7. Final implementation gate

Run `just verify`, which includes `test-e2e-ledger-postgres`, confirm the matching CI lane ran the tagged scenarios, and add the separately controlled performance run. Retain scenario mapping, red/green evidence, actual OTLP capture, concurrent erasure/crash results, migration privilege checks, rendered deployment output and the measured profile/report. No feature is complete solely because schemas validate or unit tests pass.
