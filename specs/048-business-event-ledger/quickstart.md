# Business Event Ledger — Validation Quickstart

The broker records all 28 catalogued facts atomically with broker-owned state. It supports PostgreSQL, memory, exact-subject investigation and erasure, retention, and recoverable telemetry.

The user retired the performance gate and deployment profile on 2026-10-04. The user removed feature-specific diagnostic tools on 2026-10-06. All 20 functional scenarios remain required. The 5 ms p99 goal is **not proven**, not a performance pass.

This guide retains observed functional evidence from implementation and review. Its command section describes current shared lanes, not a new execution of those checks.

## Prerequisites

The validation environment requires:

- Go 1.27.1, just, the repository-pinned Ginkgo CLI, and frontend build prerequisites
- Docker or Podman for PostgreSQL acceptance and infrastructure integration
- Migration 037, the canonical [schemas and examples](../../api/events/v1/), and a matching broker build
- Broker DML credentials separate from migration-owner credentials
- Authorized operational SQL access and a local OTLP receiver
- A five-minute PostgreSQL maintenance scheduler.

Use disposable data and the existing shared bootstrap. Do not use production credentials or a production telemetry destination.

Read [ADR 039](../../adrs/039-business-event-ledger.md), [configuration](contracts/configuration.md), [storage](contracts/storage.md), and [event contracts](contracts/events.md). The [operations guide](../../docs/operations/business-event-ledger.md) defines deployment procedures and credential isolation.

PostgreSQL refresh requires direct connections or session-mode pooling. Transaction-mode and statement-mode pooling cannot safely retain its session-level advisory lock. Provider I/O holds no ledger lifecycle or subject gate. The operation context bounds acquisition. Uncertain acquisition or failed unlock discards the connection.

## Current validation commands

Run these commands from the repository root:

```sh
just check
go test ./api/events/... ./internal/domain/ledger/...
go test -race ./internal/adapters/storage/memory/...
just test-e2e-backend 'business-event-ledger && !performance' ''
just test-integration-infra
just test-e2e-backend 'business-event-ledger && !performance'
just security broker
just helm-lint helm-template
just verify
```

The explicit empty build-tag argument selects memory only. The normal backend command uses `integration`, builds frontend assets, rejects empty selections, and retains configured parallelism.

Normal `just verify` includes the backend lane once. CI uses the existing `just verify-e2e-backend-junit` invocation and reports. There is no ledger-specific CI job, standalone CLI, or replacement diagnostic suite.

`just security broker` covers production broker code and reachable dependency vulnerabilities. Repository-wide `just security` retains its wider scope. This feature adds no advisory exception or CI security gate.

## Operational SQL lifecycle

### Authorization

Use a protected PostgreSQL connection as the migration owner for investigation, erasure, provisioning, and maintenance. Broker runtime credentials are not operator credentials.

A DBA can delegate narrowly to existing independently managed roles. For example, in an authorized `psql` session:

```sql
\set reader_role 'broker-ledger-reader'
\set erasure_role 'broker-ledger-eraser'
GRANT USAGE ON SCHEMA public TO :"reader_role", :"erasure_role";
GRANT SELECT ON public.business_events, public.business_event_delivery_pending TO :"reader_role";
GRANT EXECUTE ON FUNCTION public.business_event_erase_subject(text) TO :"erasure_role";
```

These statements do not create roles. Never grant ledger-function execution to the broker role. The chart manages neither operational roles nor future-function default privileges.

Explicit ledger ACLs deny runtime event mutations, direct child-partition access, DDL, and ledger-function execution. Pending-reference and policy DML remain permitted. Migration-owned partition creation retains child-partition ACL cleanup.

Removing former settings or script commands does not revoke previously applied ACLs. A DBA must review and reconcile those ACLs manually.

### Investigate an exact subject

In an authorized `psql` session, select the subject and half-open UTC interval:

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

The result identifies receiving agents, not callers. It excludes denied exchanges and unrelated subjects. Events survive deletion of referenced business objects.

The [investigation guide](../../docs/api/business-event-ledger.md) gives explicit no-subject selection and stable `(occurred_at,id)` pagination. Repository pages default to 200 records and permit at most 1000. No query implicitly selects all subjects.

### Erase an exact subject

Use the migration owner or an explicitly authorized erasure delegate. In autocommit mode, run:

```sql
SET default_transaction_isolation = 'read committed';
SET lock_timeout = '5s';
SET statement_timeout = '30s';
SELECT public.business_event_erase_subject('principal-example');
SELECT public.business_event_erase_subject('principal-example');
```

The first call returns the deleted event count. The repeated call returns zero. With an explicit transaction, commit before reporting completion.

The function rejects blank/null subjects, incompatible isolation, and missing or excessive statement deadlines. Set `statement_timeout` before the call, from `1ms` through `30s`.

With separately authorized read access, run:

```sql
SELECT count(*) FROM public.business_events WHERE subject = 'principal-example';
```

The count is zero. Other subjects remain unchanged, even when their actor names the erased principal. An erasure delegate needs function execution, not unrestricted `DELETE` or ledger `SELECT`.

Erasure waits for earlier recorders and synchronous dispatch attempts under ordered lifecycle/subject barriers. A legitimate occurrence after its commit is new history. Business-row expiry markers remain, so lazy discovery cannot replay an old expiry.

### Provision and maintain partitions

Use migration credentials and bounded deadlines before broker startup:

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30s';
SELECT public.business_event_provision_partitions();
```

Provisioning creates current/future six-hour UTC partition pairs and never deletes history. The migration and pre-install/pre-upgrade Job use provisioning, not retention maintenance.

The five-minute scheduler uses the existing PostgreSQL-client image and migration credentials:

```sh
PGOPTIONS='-c lock_timeout=5s -c statement_timeout=30s' \
  psql -v ON_ERROR_STOP=1 -c 'SELECT public.business_event_maintain_partitions();'
```

Use protected connection/password files for external schedulers. Do not put credentials in commands or logs. PostgreSQL deployments without the Helm migration Job require equivalent external provisioning and scheduling.

Maintenance drops only complete event/reference pairs whose upper bound is no later than database time minus retention. Retention uses `recorded_at`, not `occurred_at`.

The default is `2160h` (90 days). Use positive Go durations, such as `720h`, not `90d`. Invalid values reject startup. Positive sub-microsecond values round upward.

Normal grace is less than six hours plus five minutes, plus bounded execution, and must remain within 24 hours. No young event disappears early. Missing current partitions fail readiness and recording. Scheduled maintenance repairs windows and catches up after downtime. There is no DEFAULT partition or runtime DDL fallback.

For historical acceptance fixtures, use migration-owner partition creation and schema-valid envelopes with historical recording timestamps. Do not add production time-travel controls.

For a retention change, use this sequence:

1. Suspend the CronJob or external scheduler.
2. Upgrade every broker replica with the same retention value.
3. Query the policy with migration credentials.
4. After every replica completes startup with that policy, resume maintenance.

```sql
SELECT retention_microseconds FROM public.business_event_policy WHERE singleton;
```

An old shorter policy can delete history that an increased duration must retain. Pre-upgrade provisioning never sweeps that history. The chart renders no `spec.suspend`, so operator suspension survives an upgrade.

Memory applies equivalent logical retention at startup and every five minutes. It has neither physical partitions nor cross-process durability. Recording and maintenance remain mandatory when telemetry copying is disabled.

### Recover telemetry without resurrecting deleted history

Stop the collector and complete a real workflow. Read the committed event and its payload-free pending reference with authorized credentials. Restart the collector and PostgreSQL-backed broker against the same database.

The original event ID must reach the receiver. Successful export plus acknowledgement commit removes the reference. Failed attempts retry after 30 seconds. A failed acknowledgement permits a same-ID external duplicate, never a second authoritative event.

Synchronous export holds deletion barriers. Erasure and retention prevent later dispatch of deleted events. Packets already dispatched, external telemetry, and independent backups remain outside ledger erasure.

Disabled-period events have no delivery references and receive no later backfill. Re-enablement resumes only retained pending work. Collector availability never controls business commit.

## Acceptance inventory and semantic-red evidence

T017–T021 established one `It` and one adjacent specification reference per scenario. Initial reports were `/tmp/ledger-memory-red-048.json` and `/tmp/ledger-functional-red-048.json`.

The memory lane selected 14 cases, and all 14 failed semantically. The integration-tagged lane selected all 20 functional cases, and all 20 failed semantically. None was pending, skipped, or excluded. These initial failures did not prove later rollback, privacy, or recovery assertions green.

`Shared` means `tests/e2e/business_event_ledger_test.go`. `PG` means `tests/e2e/business_event_ledger_postgres_test.go`.

| Scenario | Required behavior | Source | Initial functional red result |
|---|---|---|---|
| US1-AS1 | All 28 catalogue workflows | Shared | No retained catalogue fact |
| US1-AS2 | Crash after commit | PG | Missing ledger relation after child workflow |
| US1-AS3 | Rollback and fail-closed recording | Shared | No retained export-control fact |
| US1-AS4 | Independent denial/failure outcomes | Shared | No retained denial |
| US1-AS5 | Only competing transition winners | Shared | No retained winning transition |
| US1-AS6 | Once-only expiration | Shared | No retained expiry fact |
| US1-AS7 | Backend contents and atomicity parity | PG | Missing relation for raw parity query |
| US2-AS1 | Exact subject/time receiving-agent set | Shared | Zero successes instead of two |
| US2-AS2 | Trusted caller, subject, delegation, correlation | Shared | No attributed exchange fact |
| US2-AS3 | All-type stored/OTLP credential exclusion | Shared | No stored credential-free envelope |
| US2-AS4 | Explicit unavailable identities | Shared | No explicit-null failure fact |
| US2-AS5 | Closed schema extension without migration | Shared | No retained catalogue control |
| US3-AS1 | Exact-subject, idempotent erasure | Shared | No retained subject facts |
| US3-AS2 | Default/changed retention without early deletion | PG | No retained live retention control |
| US3-AS3 | Concurrent recording/erasure boundary | PG | No retained predecessor control |
| US3-AS4 | No restart/deferred-copy resurrection | PG | Missing relation for deferred-copy control |
| US3-AS5 | Reject invalid retention at startup | Shared | Non-positive retention accepted |
| US4-AS1 | Unchanged logs plus full additional signal | Shared | No retained signal-comparison fact |
| US4-AS2 | Independent telemetry-copy disablement | Shared | No retained fact with copying disabled |
| US4-AS3 | Export failure, rollback, crash recovery | PG | Missing relation for committed-copy query |

The former US4-AS4 also failed semantically: a completed admin mutation had no retained event or ledger relation.
Its original report was `/tmp/ledger-performance-red-048.json`. It produced no workload latency distribution.
US4-AS4 retains its identifier as retired. It is not counted as a pass or reassigned.

Memory covers 14 applicable scenarios. The tagged lane supplies six PostgreSQL-specific cases and shared backend iterations.
Lifecycle and retention-worker tests prove memory equivalents without a restart-durability claim.

## Scope-reduction verification (2026-10-13)

The approved reduction removed optional chart delegation, global function-default changes, duplicate spec JSON, and profiling-only spans. Published JSON stayed unchanged.

- `just check` passed with zero linter issues.
- Race-enabled tests passed for published schemas, ledger, OAuth2 sessions, telemetry, and application wiring.
- Focused Helm rendering and three real PostgreSQL permission/erasure tests passed. Broker denials, manual delegation, and exact-subject deletion remained enforced.
- A runtime smoke recorded, exported, and acknowledged all 28 published types through a real OTLP receiver. Request correlation remained intact. Exact-subject erasure removed 19 matching facts and preserved other subjects. The temporary source was removed.
- All 20 functional mappings, historical measured values, and local documentation links remained intact.

The smoke exercised ledger operations and encoding, not complete producer journeys. Full `just verify` and E2E suites were not rerun.

## Retained functional evidence

These are historical observed results, not verification of this documentation reduction.

### Baseline, first green, and operational proof

| Provenance | Observed result and limit |
|---|---|
| T001, baseline `d500f36378dd914f8a516604a08525f737e8ddff`; capture checkout `7d830d685715e03e5d054c3ef51d8ed5641a58df` | Go 1.27.1/Ginkgo v2.32.2 passed 608 functional backend cases. Descriptor capture recorded names, levels, messages, keys, and counts, never values. This is pre-ledger evidence. |
| T063/T070/T083/T084/T093, first complete functional green | Untagged Ginkgo passed 14 cases. Integration-tagged Ginkgo passed 20. All 28 catalogue workflows and the offline fixture exported actual OTLP records. Rollback, winners, investigation, privacy, erasure, retention, crash recovery, and no replay passed. |
| T097, actual command-binary restart | A real broker HTTP mutation left pending work during collector rejection. Broker/collector restart recovered the original ID and removed the acknowledged reference. Temporary smoke source and container were removed. |
| T109, 2026-10-04 operational smoke | Authorized SQL returned exactly one receiving agent. Collector outage and broker restart recovered the original exchange ID. Erasure removed three facts, repeated erasure returned zero, and another principal retained both facts. Runtime erasure was denied. Migration-owner maintenance succeeded. |
| T115, 2026-10-04 configuration regression | Red tests loaded 48h/24h instead of the selected 12h file. Both complete config/command packages passed after correction. The actual broker loaded the documented example with `--config`, returned health 200, and exited 0 on SIGTERM. |
| T117, 2026-10-04 log compatibility | All 32 reviewed workflow variants passed on memory and PostgreSQL. Exact descriptor multiplicities and full telemetry-envelope assertions remained intact. |
| T120/T121, 2026-10-05 numeric regression | Red tests observed `9007199254740992` instead of `9007199254740993` in model/PostgreSQL decoding. Real Get/Query reproduced the loss despite correct stored JSON. Numeric-width/OTLP tests and US2-AS5 also failed before correction. |
| T120/T121, corrected runtime proof | Five model/ledger/storage/telemetry packages passed. US2-AS5 passed both backends with exact raw JSON, scoped queries, actual OTLP, original ID, and acknowledgement. A production-builder gRPC smoke retained large scalars/arrays, int32/float32 values, and zero pending references. The source was removed. |

Closed Draft 2020-12 schemas, offline URN resolution, fixed names/source, and compatible evolution remain required. Synthetic [published examples](../../api/events/v1/examples.json) are contract shapes, not runtime evidence. All 30 schema mirrors and the examples mirror were byte-for-byte equivalent before removal. The canonical publication remains unchanged.

### Final shared-lane results and limits

| Checkpoint | Successful checks | Limit |
|---|---|---|
| Historical pre-scope-cleanup full gate | `just verify`: static/security checks, Go race tests, both integration layers, mocks/CDK, 246 web tests, 622 backend cases, 112 ExtProc cases, 61 browser cases, and 20 tagged ledger cases. A repeat after CTE/approval-mutex fixes retained these totals. | Earlier dependency repairs/scanner scope do not establish a full-gate pass for the later narrowed tree. No GitHub CI run is claimed. |
| 2026-10-04 backend CI-command proof | Local `just verify-e2e-backend-junit`, integration tag, four workers: 661/663 cases passed in 178.584 seconds. Zero failed/pending. The JSON report identified all 20 ledger cases as passed. | Two performance-labelled cases were excluded. This is local command evidence, not hosted CI. |
| 2026-10-06 baseline and denial reviews | Four-process tagged Ginkgo passed all 20 ledger cases with zero failed/pending. `just check` reported zero linter issues. Denial-classification regressions failed before correction, then focused/race packages and real HTTP smokes passed. | No fresh full-repository `just verify` pass was claimed. |
| 2026-10-06 outcome/concurrency review | Six affected domain packages passed with race detection. Focused tagged PostgreSQL/storage tests passed approval, consent, and agent concurrency cases. `just check` reported zero linter issues. | This focused review claimed no full infrastructure or E2E gate. |
| 2026-10-06 shared-suite consolidation | `just check`, race-enabled `just test`, all four `just test-integration-infra` packages, and `just helm-lint helm-template` passed. `just security broker` reported zero gosec issues and reachable vulnerabilities, with one uncalled-module notice. Full backend: 664/666 passed across four workers, including all 20 ledger cases. Memory-only ledger: 14 passed. | Two existing performance cases were excluded. This checkpoint claimed no full `just verify` run. |
| 2026-10-10 main integration, base `46fabf9684e4bbcb27878a1a549af801126c2d1b` | `just check`, race-enabled `just test`, self-contained `just test-integration`, tagged grant/agent/permission-set/expiry regressions, and `just build` passed. A separate broker process completed admin HTTP creation/update/deletion. Full backend: 679/680 functional cases passed, including all 20 ledger cases. | The remaining obsolete orphan-grant setup failed. Its corrected scenario passed a focused tagged run, followed by clean static checks. No later full-suite pass was recorded. |

The legacy fixture retains baseline `d500f36378dd914f8a516604a08525f737e8ddff` for unchanged workflows. Three refresh/no-grant variants use observed main-parent revision `446183e095381afec8252b274768b831d6ba4e0e`, not feature output. Strict field-key and multiplicity assertions remain intact. Main's later grant/agent audit contract from PR #201 also remains represented.

Findings M1, M2, and M9 remain explicit prerequisites for future OTLP ingestion.
Their definitions and resolution evidence were unavailable. These results claim no resolution.

The [performance report](performance-results.md) retains every distinct measured distribution, original conditions, raw-report provenance, and limitations. Profiling spans and removed diagnostic commands are historical only. No deployment profile, numeric performance pass, or replacement suite is required. Functional safety remains mandatory.
