# Business Event Ledger — Historical Performance Results

## Retirement decision and status

**NOT PROVEN.** These historical distributions establish neither a deployment-load pass nor failure of the 5 ms p99 goal.

On 2026-10-04, the user directed, `we dont need that gate and neither a performance profile`. US4-AS4, the mandatory FR-008/SC-007 criteria, and deployment-profile approval are retired. T095/T096/T112/T113/T118 closed as retired, not measured passes.

On 2026-10-06, the user removed feature-specific diagnostic suites, runner commands, and setup. No replacement suite is required. All 20 functional scenarios remain required in the [shared validation lanes](quickstart.md#current-validation-commands).

The profiling overlays, runners, and production spans described here no longer provide current instrumentation. `ledger.record.validate`, `ledger.record`, and `ledger.transaction` names identify historical measurement boundaries only. This report retains results and provenance, not executable setup recipes.

Accepted ADR 039's storage/security design and the production 25-open/5-idle pool remain unchanged. Recording has no performance bypass.

## Conditions and measurement limits

The original reference used these conditions:

| Item | Historical condition |
|---|---|
| Initial baseline | `d500f36378dd914f8a516604a08525f737e8ddff` |
| Initial feature | `185c8c084fffcab8dd991a943f476a906562fff5+working-tree` |
| Host | Apple M3 Max, 14 CPUs/GOMAXPROCS, 36 GiB, Darwin/arm64 |
| Database | PostgreSQL 17.11, `postgres:17-alpine`, mapped container port, Docker VM memory 3904 MB |
| Network | Loopback HTTP and local mapped PostgreSQL port |
| Pool | 25 open, 5 idle connections |
| Telemetry | Local OTLP gRPC traces/logs/metrics and ledger copying enabled |
| Dataset | 10,000 principals, 100 agents, 20 services |
| Feature history | 1,000,000 safe events across 90 days. Baseline had no ledger table. |
| Workload | Concurrency 32, 120-second warmup per version, 100,000 measured actions plus 20,000 untimed approval preparations |
| Action mix | 50% exchange, 20% approval transitions, 10% grant updates, 10% session refreshes, 10% admin mutations |
| Repetitions | Three alternating pairs: baseline/feature, feature/baseline, baseline/feature |

The original baseline preceded other main changes. Its pool was smaller than the 32 workers, and backends churned. Feature delivery also loaded the measured database.

The old recording span included pool checkout and physical owner commit. A difference between independent p99 values is not a paired per-request effect. Instrumented runs perturb execution. Aggregate pool/goroutine wait counters are not request latency. These limits prevent a target verdict.

## Original reference distributions

All durations are milliseconds. Each complete baseline/feature run measured 100,000 actions. Original recording p99 values use the old boundary.

### Initial baseline and corrected-verifier first pair

| Metric | Initial baseline | Corrected-verifier baseline | Corrected-verifier feature |
|---|---:|---:|---:|
| Warmup actions | 130,086 | 117,748 | 60,953 |
| Transaction p50 | 4.812041 | 6.688917 | 23.778667 |
| Transaction p95 | 14.360542 | 19.081042 | 52.328209 |
| Transaction p99 | 21.774792 | 28.112458 | 81.625917 |
| Recording p99 | Not applicable | Not applicable | 26.889833 |
| Actions/s | 1,043.822 | 745.256 | 505.414 |
| Allocation bytes | 53,454,274,416 | 57,472,103,640 | 62,174,079,824 |
| Allocation objects | 1,038,026,358 | 1,116,880,135 | 1,197,623,883 |
| HTTP errors | 0 | 0 | 0 |
| PostgreSQL rollbacks | 0 | 0 | 0 |

The initial feature completed 103,695 warmup actions, then failed verification with `action 6038 retained wrong agent`. Direct-user refresh requires service/session references, not an invented agent. This attempt provides no feature latency or allocation result.

The corrected-verifier feature retained 100,000 measured facts, 20,000 approval preparations, and all 60,953 warmup facts. Credential checks passed. Envelope sizes were 658–1,020 bytes.

Its independent transaction-p99 difference was 53.513459 ms. The assertion stopped after pair one. Pairs two and three did not run.

Recorded raw provenance:

- Initial baseline: `tests/e2e/test-results/ledger-reference-final/reference-repetition-1-baseline.json` (100,000 nanosecond samples)
- Corrected verifier: `test-results/ledger-reference-final-2/reference-repetition-1-{baseline,feature}.json` (full latency and recording arrays).

### Combined insertion and retained-fix first pairs

Combined event/reference insertion removed one SQL round trip without removing validation, gates, or atomic failure. The approval creation mutex moved before checkout and remained held through commit. Gate batching was reverted.

| Metric | CTE baseline | CTE feature | Retained-fix baseline | Retained-fix feature |
|---|---:|---:|---:|---:|
| Warmup actions | Not reported | 90,281 | 117,412 | 56,328 |
| Transaction p50 | 5.350042 | 22.645333 | 5.802125 | 10.701666 |
| Transaction p95 | 16.964250 | 45.858167 | 19.516084 | 26.125666 |
| Transaction p99 | 25.361709 | 60.890542 | 33.619375 | 42.990458 |
| Recording p99 | Not applicable | 21.136500 | Not applicable | 22.370709 |
| Actions/s | 891.773 | 581.635 | 769.4 | 455.3 |
| Allocation bytes | 52,892,579,960 | 61,210,570,496 | 60,172,926,520 | 60,234,250,552 |
| Allocation objects | 1,027,772,526 | 1,179,951,575 | 1,168,605,407 | 1,159,785,813 |
| HTTP errors | 0 | 0 | 0 | 0 |
| PostgreSQL rollbacks | 0 | 0 | 0 | 0 |

Both feature runs verified all 100,000 facts, 20,000 preparations, their entire warmups, and credential exclusion. Retained-fix envelopes were 658–1,020 bytes.

The CTE independent-p99 difference was 35.528833 ms. The retained-fix difference was 9.371083 ms. Neither run completed pairs two and three. Neither establishes a target verdict.

A reverted gate-batching first pair reported baseline p99 29.623334 ms, feature p99 67.500333 ms, and recording p99 20.721583 ms. Its independent difference was 37.876999 ms. Correctness/credential checks passed with zero HTTP errors and rollbacks.

Recorded raw provenance: `test-results/ledger-reference-final-3/` (CTE), `test-results/ledger-reference-final-4/` (reverted batching), and `test-results/ledger-reference-final-5/` (retained fixes).

## Historical profiling and stage distributions

The first profiled broker completed 93,964 fixed-reference actions with one million historical events. Fact and credential checks passed. CPU, allocation, mutex, and blocking profiles used a temporary build overlay, with no production endpoint or switch.

Allocation samples attributed approximately 2.77 GiB to JSON Schema validation and 0.69 GiB to wire views. Existing encryption allocations dominated the total. PostgreSQL sampling observed row locks, WAL synchronization, and active queries. Validation safety checks remained intact.

A timing overlay completed 20,000 measured actions after 99,890 warmup actions under the original concurrency, mix, and history. Fact/preparation/warmup and credential checks passed. Nested scopes were non-additive.

| Historical diagnostic stage | p99 (ms) |
|---|---:|
| Record validation and hints | 0.18 |
| Append final validation | 0.16 |
| Append serialization | 0.04 |
| Append clock query | 1.98 |
| Append INSERT | 2.50 |
| Append total | 3.72 |
| Physical begin, including pool wait | 9.50 |
| Physical commit | 3.64 |
| Transaction work, including approval preparation | 127.80 |

A later profiled feature-only run used concurrency 32, one million events, a two-minute warmup, and 100,000 measured actions. It verified 100,000 facts, 20,000 preparations, 70,876 warmup facts, and credential exclusion. Envelopes were 659–1,020 bytes.

| Feature-only diagnostic | p50 (ms) | p95 (ms) | p99 (ms) |
|---|---:|---:|---:|
| Transaction latency | 10.167750 | 30.637875 | 55.180417 |
| Recording duration | 3.400333 | 11.539625 | 26.443416 |

The mutex profile attributed nearly all reported delay to approval creation. The blocking profile reported 9119.50 aggregate seconds beneath `CreatePendingApproval`. This is aggregate goroutine wait, not request latency.

Recorded provenance:

- `test-results/ledger-profile-1/`
- `test-results/ledger-stage-diagnostic/stages-2483263829.json`
- `test-results/ledger-profile-next/diagnostic-samples.json`
- `test-results/ledger-profile-next/feature-broker`
- `test-results/ledger-profile-next/broker-3966257169/`.

The temporary profiling sources were removed. These measurements are not current instrumentation instructions or acceptance results.

## Reverted pair-gate trial distributions

The trial used 64 fixed creation gates. It was reverted, including its trial-only expectation. The original process-wide creation gate and committed-winner/rate-limit/atomicity contracts remain.

The unprofiled run used baseline `d500f36378dd914f8a516604a08525f737e8ddff` and feature `fe698de161cfbac244e40eb9ecf8feb37b7eb5ff+working-tree`. It retained the original PostgreSQL 17.11, 14 CPUs, 25/5 pool, copy-enabled telemetry, one-million-event history, 120-second warmups, and 100,000 actions per version. No concurrent workload ran.

| Pair | Version | p50 (ms) | p95 (ms) | p99 (ms) | Recording p99 (ms) | Actions/s |
|---|---|---:|---:|---:|---:|---:|
| 1 | Baseline | 4.943375 | 14.420000 | 22.448875 | Not applicable | 1021.169 |
| 1 | Feature | 27.255042 | 137.085125 | 280.754292 | 44.828375 | 497.303 |
| 2 | Feature | 27.390667 | 130.127167 | 263.170959 | 37.622833 | 506.105 |
| 2 | Baseline | 5.420625 | 18.653125 | 33.913875 | Not applicable | 834.056 |
| 3 | Baseline | 8.844042 | 57.379166 | 145.013625 | Not applicable | 375.344 |
| 3 | Feature | 16.771375 | 84.328125 | 175.931375 | 23.396042 | 759.881 |

| Pair | Independent p99 difference (ms) | Baseline allocation bytes / objects | Feature allocation bytes / objects | Baseline / feature warmup actions |
|---|---:|---|---|---|
| 1 | 258.305417 | 59865936000 / 1162994285 | 63318144608 / 1219964427 | 124361 / 74972 |
| 2 | 229.257084 | 55912395424 / 1085623658 | 62854871728 / 1210407546 | 91531 / 68789 |
| 3 | 30.917750 | 51514308928 / 997499615 | 65093686328 / 1257703413 | 73855 / 71744 |

Every version reported zero HTTP errors and rollbacks. Every feature verified 100,000 facts, 20,000 preparations, its full warmup, and credential exclusion. Envelopes were 658–1,020 bytes.

The six raw arrays and conditions were recorded as `reference-repetition-{1,2,3}-{baseline,feature}.json` under `test-results/ledger-reference-pair-gates-20261002/`. Baseline variation and the old recording boundary prevent a target verdict.

## Historical SQL and cold-backend distributions

The user retained the production pool and accepted storage design. No connection-retention change or partition-local append path was implemented.

A disposable pgx tracer measured 20,000 actions with concurrency 32, one million events, and a two-minute warmup. It verified 20,000 facts, 4,000 preparations, 76,714 warmup facts, and credential exclusion. Feature-only transaction p99 was 36.048459 ms. Recording p99 was 20.935792 ms.

| SQL class | Calls | p99 (ms) |
|---|---:|---:|
| Physical BEGIN wire query | 20000 | 2.139083 |
| Lifecycle gate | 20000 | 1.800458 |
| Subject gate | 18000 | 1.557458 |
| Recording clock | 20000 | 1.861208 |
| Queued event insert | 20000 | 16.435000 |
| Physical COMMIT | 20000 | 2.904875 |
| Physical begin, including checkout | 20000 | 5.506291 |

The pool closed 427 idle connections. WaitCount increased by 161 and WaitDuration by 412103336 ns. Statement-cache capacity remained 512. These are aggregate counters.

Queued inserts had 416 prepare misses. Without a miss, p99 was 2.161625 ms across 19,584 calls. With a miss, p99 was 94.904166 ms across 416 calls. The slowest insert took 110.626333 ms, with a 0.541209 ms prepare callback.

A separate rollback-only probe measured 40 cold/warm parent inserts with 389 event partitions. It used runtime privileges and committed no events.

| Probe measurement | Range (ms) |
|---|---:|
| Planning | 0.007–0.056 |
| Cold queued execution | 5.435–8.766 |
| Warm queued execution | 0.125–0.236 |
| Cold unqueued execution | 3.102–5.663 |
| Warm unqueued execution | 0.113–0.217 |

The measurements support a cold-backend/cache-churn hypothesis. They do not prove its exact internal cause or an accepted optimization.

Recorded sanitized provenance: `test-results/ledger-sql-trace-20261002.json`, `test-results/ledger-sql-diagnostic-20261002.json`, and `test-results/ledger-cold-plan-20261002.json`. The instrumented executable's recorded location was `/private/var/folders/8t/c0ct0rc51k1__jw0mf2zwd0r0000gn/T/aib-ledger-sql-diagnostic.kmYWG0I3/`. Sanitized traces contained no SQL text, arguments, credentials, or event bodies.

## Corrected recording run (2026-10-03)

The corrected reference pinned pre-ledger main `e6902cf296e617ff02e315a069d5dfb73f44bb51`. Feature source was `0ed87362eda183450ce9ff39ac569364e91aa50e+working-tree`.

Both binaries used the same test-only overlay: 40 open/40 idle connections, with all 40 backends pre-warmed. Production remained 25/5. Host/database conditions, workload cardinalities, mix, warmups, actions, and alternating pair order remained as listed in the original reference.

Ledger copying was disabled. This excluded pending-reference insertion and delivery-worker database load. Ordinary OTLP gRPC traces/logs/metrics remained enabled. A copy-disabled run does not establish default copy-enabled costs. The separate full delivery-load diagnostic did not run.

Historical `ledger.record.validate` measured preflight. Historical `ledger.record` measured append, final-timestamp validation, serialization, and SQL. The per-action validation+append sum excluded checkout, advisory-gate waits, and physical commit. Historical `ledger.transaction` included those stages in the full owner duration. HTTP action duration was separate.

The stage sum therefore does not establish full attributable recording overhead. Raw `recording_span_nanoseconds` and `recording_p99_nanoseconds` fields name that sum, not total overhead.

| Pair | Baseline action p99 (ms) | Feature action p99 (ms) | Independent difference (ms) | Append p99 (ms) | Validation+append p99 (ms) | Full owner transaction p99 (ms) |
|---|---:|---:|---:|---:|---:|---:|
| 1 | 7.781750 | 9.787208 | 2.005458 | 1.546375 | 1.581375 | 5.953541 |
| 2 | 15.634792 | 18.216167 | 2.581375 | 3.745875 | 3.851458 | 10.811083 |
| 3 | 12.128958 | 18.832500 | 6.703542 | 3.918250 | 4.017625 | 11.266750 |

Each binary reported zero pool waits, HTTP errors, and rollbacks. Each feature verified 100,000 measured facts, 20,000 preparations, and respectively 106,753, 110,462, and 98,300 warmup facts. Credential canaries were absent. Envelopes were 658–1,022 bytes.

Baseline p99 ranged from 7.781750 to 15.634792 ms, a 7.853042 ms spread. Append/stage sums below 5 ms omit required costs. The third independent action difference above 5 ms is not a paired target failure.

**Verdict: NOT PROVEN.** Completeness, atomicity, credential, and measurement checks passed. No deployment-load acceptance was established.

The six `reference-repetition-{1,2,3}-{baseline,feature}.json` reports were recorded under `test-results/ledger-recording-corrected-20261003/`. They contain every action/stage/transaction sample, counters, allocations, throughput, conditions, and verification counts.

## Reduced execution smoke (2026-10-04)

Recording and delivery modes each completed three alternating pairs with test-only reduced inputs:

| Input | Value |
|---|---:|
| Principals | 100 |
| Agents | 10 |
| Services | 2 |
| Historical facts | 1,000 |
| Concurrency | 4 |
| Matched pools | 8 connections |
| Warmup | 2 seconds |
| Actions per version | 1,000 |

Each of six feature runs verified 1,000 measured facts, 200 preparations, its completed warmup, and credential exclusion. Every report recorded zero HTTP errors and rollbacks.

This was an execution smoke, not a full reference measurement or 5 ms pass. Short-run allocations and latency comparisons do not establish that goal.

Twelve raw reports were recorded under:

- `test-results/ledger-diagnostic-smoke-20261004/d9ac4f0a6a33478a93d0d3b6e44c9ec4/`
- `test-results/ledger-diagnostic-smoke-20261004/ff54c3fb1163438d83a03f85d781894c/`.

Their `mode` distinguishes recording from delivery. The temporary overlay, mapping, and executable were removed. The retired tools require no replacement. Historical evidence remains non-blocking and does not weaken functional guarantees.
