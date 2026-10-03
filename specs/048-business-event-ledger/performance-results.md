# Business Event Ledger — Performance Results

## Acceptance status

**NOT PROVEN.** Historical reference samples are diagnostics. They prove neither a 5 ms target failure nor a pass.

The old `ledger.record` span included pool checkout and physical owner commit. The 25-open/5-idle pool was smaller than the 32 concurrent workers. Backends churned. Ledger copying also ran a delivery worker against the measured database. Baseline `d500f36378dd914f8a516604a08525f737e8ddff` predates other mainline changes.

The 5 ms recording and transaction-overhead targets are unchanged. The operations owner has not supplied an approved deployment profile. That separate missing approval blocks release. The corrected controlled reference below is diagnostic only; no accepted result is reported here. The first attempt also failed its event-identity verifier. T095 and T096 remain open.

## Historical reference profile (before correction)

- Baseline revision: `d500f36378dd914f8a516604a08525f737e8ddff`; corrected reference pins the later pre-ledger main revision `e6902cf296e617ff02e315a069d5dfb73f44bb51`.
- Feature source for the initial attempt: `185c8c084fffcab8dd991a943f476a906562fff5+working-tree`
- Host: Apple M3 Max, 14 CPUs, 36 GiB memory, Darwin/arm64
- PostgreSQL: 17.11 in `postgres:17-alpine`, mapped container port, Docker VM memory 3904 MB
- Network: loopback HTTP and local mapped PostgreSQL port
- Pool: 25 open connections, 5 idle connections
- Telemetry: OTLP gRPC traces, logs, and metrics enabled, including ledger copying
- Dataset: 10,000 principals, 100 agents, 20 services
- Feature history: 1,000,000 safe events across 90 days
- Baseline history: no ledger table
- Concurrency: 32
- Warmup: 120 seconds per version
- Measured actions: 100,000 per version, plus 20,000 untimed approval preparations
- Mix: 50% exchanges, 20% approval transitions, 10% grant updates, 10% session refreshes, 10% admin mutations
- Required repetitions: three, with alternating baseline/feature order

These were the original comparison settings, not the corrected recording reference or an operator-approved deployment profile.

## Initial attempt

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" \
  AIB_LEDGER_PERFORMANCE_RESULTS_DIR="test-results/ledger-reference-final" \
  just test-e2e-performance 'performance && business-event-ledger' integration
```

Only the first baseline result completed verification and report storage:

| Metric | First baseline repetition |
|---|---:|
| Completed warmup actions | 130,086 |
| Measured actions | 100,000 |
| Transaction p50 | 4.812041 ms |
| Transaction p95 | 14.360542 ms |
| Transaction p99 | 21.774792 ms |
| Throughput | 1,043.822 actions/s |
| Allocation bytes | 53,454,274,416 |
| Allocation objects | 1,038,026,358 |
| HTTP errors | 0 |
| PostgreSQL rollbacks | 0 |

The retained raw report contains all 100,000 transaction samples in nanoseconds:
`tests/e2e/test-results/ledger-reference-final/reference-repetition-1-baseline.json`.

The feature completed 103,695 warmup actions. Its measured-run verifier then reported `action 6038 retained wrong agent`. The verifier incorrectly required an agent reference on direct-user session refresh. The published refresh schema requires service/session references, not an agent reference.

The corrected verifier requires the refresh's service, session, and initiating user, and rejects an invented agent reference. Other workflows retain their exact agent checks. The preflight now exercises and verifies every measured workflow before history seeding. Per-action completeness, preparation counts, delegation, and credential checks remain intact.

No feature latency, recording p99, allocation, or paired-overhead pass is claimed from this failed attempt.

## Corrected verifier: first measured pair

The corrected run used the same reference settings and a separate results directory, `test-results/ledger-reference-final-2`.

| Metric | Baseline | Feature |
|---|---:|---:|
| Completed warmup actions | 117,748 | 60,953 |
| Measured actions | 100,000 | 100,000 |
| Transaction p50 | 6.688917 ms | 23.778667 ms |
| Transaction p95 | 19.081042 ms | 52.328209 ms |
| Transaction p99 | 28.112458 ms | 81.625917 ms |
| Recording p99 | Not applicable | 26.889833 ms |
| Throughput | 745.256 actions/s | 505.414 actions/s |
| Allocation bytes | 57,472,103,640 | 62,174,079,824 |
| Allocation objects | 1,116,880,135 | 1,197,623,883 |
| HTTP errors | 0 | 0 |
| PostgreSQL rollbacks | 0 | 0 |
| Measured event size | Not applicable | 658–1,020 bytes |

The feature verifier confirmed all 100,000 measured events, 20,000 approval-preparation events, and 60,953 warmup events. Credential canaries were absent. Full raw latency and recording distributions reside in the two `reference-repetition-1-*.json` files in this directory.

The independent transaction-p99 difference was **53.513459 ms**; recording p99 was **26.889833 ms** under the old span boundary. Neither is a paired per-request effect. Both were numerically above 5 ms, but the confounded attempt cannot establish either target verdict. The old recording assertion stopped the run after the first pair; repetitions two and three did not run.


## Diagnostic profiling

A separate profiled broker completed 93,964 fixed-reference actions with one million historical events. The verifier confirmed their facts and credential exclusion. A temporary Go build overlay enabled CPU, allocation, mutex, and blocking profiles. It added no production endpoint or configuration switch. The overlay and profiling scenario source were removed after capture.

Profiles remain in `test-results/ledger-profile-1/`. PostgreSQL activity sampling found transaction-row locks, WAL synchronization, and active query execution. Token-exchange recording had a higher recording tail than workflows already inside a transaction.

The allocation profile attributed approximately 2.77 GiB to JSON Schema validation and 0.69 GiB to event wire views. Existing encryption-library allocations dominated the total. Validation remains unchanged because storage must validate the final authoritative recording timestamp.

The first bounded change combines queued event/reference insertion into one data-modifying CTE. It removes one PostgreSQL round trip, retains the existing validation and gates, and preserves atomic failure. Non-copying appends still require only event insertion. Its acceptance effect must come from the unchanged paired measurement, not from these instrumented profiles.


After the CTE change, `just check` passed with zero linter issues. Focused real-PostgreSQL event, lifecycle, delivery, projection, privilege, and rollback tests passed in both affected packages. The paired measurement runs separately, with no concurrent build/test load.


## Event/reference CTE measurement

The first pair after combined insertion retained all 100,000 measured facts, all 20,000 approval preparations, and 90,281 warmup facts. Credential checks passed. Both versions reported zero HTTP errors and zero PostgreSQL rollbacks.

| Metric | Baseline | Feature |
|---|---:|---:|
| Transaction p50 | 5.350042 ms | 22.645333 ms |
| Transaction p95 | 16.964250 ms | 45.858167 ms |
| Transaction p99 | 25.361709 ms | 60.890542 ms |
| Recording p99 | Not applicable | 21.136500 ms |
| Throughput | 891.773 actions/s | 581.635 actions/s |
| Allocation bytes | 52,892,579,960 | 61,210,570,496 |
| Allocation objects | 1,027,772,526 | 1,179,951,575 |

The independent transaction-p99 difference was 35.528833 ms and the old recording p99 exceeded 5 ms. The first-pair assertion stopped repetitions two and three. Because the timing and pool boundaries were confounded, target status is NOT PROVEN. Raw samples remain in `test-results/ledger-reference-final-3/`.

The second experiment batched lifecycle and subject gates for a transaction with one subject and a shared lifecycle gate. A materialized dependency preserved lifecycle-before-subject order. Full PostgreSQL adapter and storage-infrastructure tests passed.


Its first pair reported baseline transaction p99 of 29.623334 ms, feature p99 of 67.500333 ms, and old recording p99 of 20.721583 ms. The independent transaction-p99 difference was 37.876999 ms. Correctness and credential checks passed, with zero HTTP errors and rollbacks. Raw reports remain in `test-results/ledger-reference-final-4/`.

This confounded first pair cannot establish whether batching improved the tail. The gate specialization and its helper were removed. The original ordered gates remain, with the safe signed-key conversion. Combined event/reference insertion remains. Later diagnostics separated validation, transaction setup, SQL writes, and commit timing.


## Bounded stage timing and approval lock-order correction

The timing-overlay diagnostic completed 20,000 measured actions after 99,890 warmup actions, with the same concurrency, mix, and one-million-event history. All measured/preparation/warmup facts and credential checks passed. Its nested scopes are non-additive and are not acceptance measurements.

| Diagnostic stage | p99 |
|---|---:|
| Record validation and hints | 0.18 ms |
| Append final validation | 0.16 ms |
| Append serialization | 0.04 ms |
| Append clock query | 1.98 ms |
| Append INSERT | 2.50 ms |
| Append total | 3.72 ms |
| Physical transaction begin, including pool wait | 9.50 ms |
| Physical commit | 3.64 ms |
| Transaction work, including approval preparation | 127.80 ms |

Raw histograms and samples remain in `test-results/ledger-stage-diagnostic/stages-2483263829.json`. Installed jsonschema-go v0.4.2 exposes no safe reusable validator state or struct-validation shortcut. Validation was not the dominant tail and remains unchanged.

The ledger transaction wrapper acquired a pooled connection before approval creation waited on `creationMu`. Queued creators therefore occupied pool slots. The mutex now precedes transaction acquisition and remains held through commit. Rate limiting, idempotency, and atomic state/event/synchronization behavior remain unchanged.

A permanent concurrent-creation regression failed with the old lock placement: a duplicate request returned before the winning commit. It passed after the correction, and the complete approval package passed under the race detector. Static checks passed with zero linter issues. The temporary timing-overlay source was removed after capture.


## Retained fixes: final first-pair measurement

The retained implementation uses combined event/reference insertion and acquires the approval creation mutex before transaction checkout. The low-value gate batching experiment remains reverted.

| Metric | Baseline | Feature |
|---|---:|---:|
| Completed warmup actions | 117,412 | 56,328 |
| Measured actions | 100,000 | 100,000 |
| Transaction p50 | 5.802125 ms | 10.701666 ms |
| Transaction p95 | 19.516084 ms | 26.125666 ms |
| Transaction p99 | 33.619375 ms | 42.990458 ms |
| Recording p99 | Not applicable | 22.370709 ms |
| Throughput | 769.4 actions/s | 455.3 actions/s |
| Allocation bytes | 60,172,926,520 | 60,234,250,552 |
| Allocation objects | 1,168,605,407 | 1,159,785,813 |
| HTTP errors | 0 | 0 |
| PostgreSQL rollbacks | 0 | 0 |

All 100,000 measured facts, 20,000 approval preparations, and 56,328 warmup facts passed verification and credential checks. Raw distributions remain in `test-results/ledger-reference-final-5/`. Event sizes were 658–1,020 bytes.

The independent transaction-p99 difference was **9.371083 ms**; the old recording p99 was **22.370709 ms**. Repetitions two and three did not run. These observations do not prove a target failure or pass under corrected boundaries.

The user chose **Stop tuning and report the blocked gate** on 2026-10-02. Further tuning stopped. The missing approved deployment profile remains a separate blocker. Tasks T094, T095, T096, T107, and T109 remained unchecked. No acceptance limit or durability/security contract was weakened.

## Convergence diagnosis and bounded pair-gate trial

The fresh post-correction diagnostic ran the unchanged fixed reference against a profiled production feature binary: concurrency 32, one million historical events, a two-minute warmup, and 100000 measured actions. It verified all 100000 facts, 20000 approval preparations, 70876 warmup facts, and credential exclusion. Observed envelope sizes were 659–1020 bytes.

| Diagnostic distribution | p50 | p95 | p99 |
|---|---:|---:|---:|
| Transaction latency | 10.167750 ms | 30.637875 ms | 55.180417 ms |
| Recording duration | 3.400333 ms | 11.539625 ms | 26.443416 ms |

These are profiled feature-only distributions, not paired acceptance. Profiling perturbs execution. Raw arrays are retained in `test-results/ledger-profile-next/diagnostic-samples.json`, with the exact executable in `test-results/ledger-profile-next/feature-broker`. The measured process profiles are in `test-results/ledger-profile-next/broker-3966257169/`.

```bash
go tool pprof -top test-results/ledger-profile-next/feature-broker \
  test-results/ledger-profile-next/broker-3966257169/mutex.pprof
go tool pprof -top test-results/ledger-profile-next/feature-broker \
  test-results/ledger-profile-next/broker-3966257169/block.pprof
```

The mutex profile attributes nearly all reported delay to approval creation. The blocking profile reports 9119.50 aggregate seconds beneath `CreatePendingApproval`; aggregate goroutine wait time is not request latency. CPU and allocation profiles still show substantial existing cryptographic work. No cryptographic or schema-validation safety check was removed.

The bounded trial replaces the process-wide creation mutex with 64 fixed gates selected by the existing rate-limit identity, `(principal, agent)`. One pair remains serialized through owner commit; waiting for its gate still precedes transaction checkout. Hash collisions only serialize unrelated pairs, and no per-identity map grows with workload cardinality. Compiler escape analysis confirms that FNV hashing is inlined and the principal byte view uses a zero-copy conversion without escaping.

An HTTP regression held one committed creation before response completion and attempted creation for another principal with the same agent. It failed before the trial because the unrelated request remained blocked. It passed afterward, retaining one exact approval fact for each principal. The complete approval package passed under the race detector, including the existing committed-winner and rate-limit tests.

This establishes concurrency and safety behavior, not a performance improvement. The earlier independent-p99 comparison cannot establish a paired per-action effect or either target verdict. The missing operations-owner-approved deployment profile remains a separate release blocker.


## Pair-gate trial: all three measured pairs, not acceptance evidence

The isolated unprofiled run completed all six version runs at baseline `d500f36378dd914f8a516604a08525f737e8ddff` and feature `fe698de161cfbac244e40eb9ecf8feb37b7eb5ff+working-tree`. It used the fixed reference conditions above, PostgreSQL 17.11, 14 host CPUs/GOMAXPROCS, the 25/5 connection pool, enabled local OTLP traces/logs/metrics and feature copying, one million feature-history rows, 120-second warmups, and 100000 measured actions per version. Pair order was baseline/feature, feature/baseline, baseline/feature. No other assistant check or workload ran concurrently.

| Pair | Version | p50 (ms) | p95 (ms) | p99 (ms) | Recording p99 (ms) | Actions/s |
|---|---|---:|---:|---:|---:|---:|
| 1 | Baseline | 4.943375 | 14.420000 | 22.448875 | Not applicable | 1021.169 |
| 1 | Feature | 27.255042 | 137.085125 | 280.754292 | 44.828375 | 497.303 |
| 2 | Feature | 27.390667 | 130.127167 | 263.170959 | 37.622833 | 506.105 |
| 2 | Baseline | 5.420625 | 18.653125 | 33.913875 | Not applicable | 834.056 |
| 3 | Baseline | 8.844042 | 57.379166 | 145.013625 | Not applicable | 375.344 |
| 3 | Feature | 16.771375 | 84.328125 | 175.931375 | 23.396042 | 759.881 |

| Pair | Difference of independent transaction p99s (ms) | Baseline allocation bytes / objects | Feature allocation bytes / objects | Baseline / feature warmup actions |
|---|---:|---|---|---|
| 1 | 258.305417 | 59865936000 / 1162994285 | 63318144608 / 1219964427 | 124361 / 74972 |
| 2 | 229.257084 | 55912395424 / 1085623658 | 62854871728 / 1210407546 | 91531 / 68789 |
| 3 | 30.917750 | 51514308928 / 997499615 | 65093686328 / 1257703413 | 73855 / 71744 |

Every version reported zero HTTP errors and zero PostgreSQL rollbacks. Every feature run verified exactly 100000 measured facts, 20000 approval preparations, its entire warmup, and credential exclusion. Observed feature envelopes were 658–1020 bytes. Full raw latency/recording arrays and conditions are retained in the six `reference-repetition-{1,2,3}-{baseline,feature}.json` files under `test-results/ledger-reference-pair-gates-20261002/`.

The recorded numbers were above 5 ms in each pair, but baseline p99 varied sharply and the old recording span included connection checkout and commit. These observations do not prove either 5 ms target. The pair-gate trial was **reverted** with its trial-only progress expectation and architecture note. The original process-wide creation gate remains. The committed-winner, rate-limit, atomicity, and acceptance expectations are unchanged.

The earlier 9.371083 ms independent transaction-p99 difference and 22.370709 ms old-span p99 remain raw diagnostic values, not accepted gate results. The missing approved deployment profile independently blocks deployment-load acceptance. T095/T096 and T112/T113 remain open.


## Continued SQL and cold-backend diagnosis

The user chose to preserve the 25-open/5-idle pool and the accepted storage design.
No connection-retention change or partition-local append path was implemented.

A disposable pgx tracer ran 20000 fixed-reference actions with concurrency 32 and one million historical events.
It retained the two-minute warmup and every recording, validation, and deletion barrier.
The diagnostic verified 20000 measured facts, 4000 approval preparations, 76714 warmup facts, and credential exclusion.
Transaction p99 was 36.048459 ms. Recording p99 was 20.935792 ms.
These are diagnostic feature-only measurements, not paired acceptance.

| SQL class | Calls | p99 (ms) |
|---|---:|---:|
| Physical BEGIN wire query | 20000 | 2.139083 |
| Lifecycle gate | 20000 | 1.800458 |
| Subject gate | 18000 | 1.557458 |
| Recording clock | 20000 | 1.861208 |
| Queued event insert | 20000 | 16.435000 |
| Physical COMMIT | 20000 | 2.904875 |
| Physical begin, including checkout | 20000 | 5.506291 |

The pool closed 427 idle connections during the measured interval.
WaitCount increased by 161 and WaitDuration by 412103336 ns.
These are aggregate pool counters, not latency for one request.
The statement-cache capacity remained 512.

The queued insert had 416 prepare misses.
Without an observed prepare miss, its p99 was 2.161625 ms across 19584 calls.
With a prepare miss, its p99 was 94.904166 ms across 416 calls.
The slowest insert took 110.626333 ms. Its prepare callback took 0.541209 ms.
This supports a cold-backend/cache-churn hypothesis. It does not establish an accepted optimization.

A separate rollback-only probe measured 40 cold/warm parent inserts with 389 event partitions.
Planning remained between 0.007 and 0.056 ms.
Cold queued execution took 5.435–8.766 ms. Warm queued execution took 0.125–0.236 ms.
Cold unqueued execution took 3.102–5.663 ms. Warm unqueued execution took 0.113–0.217 ms.
The probe used runtime privileges and left no committed events.
It points to backend execution/initialization cost, not SQL planning. The exact internal cause remains unproven.

Sanitized evidence is retained in these ignored files:

- `test-results/ledger-sql-trace-20261002.json`
- `test-results/ledger-sql-diagnostic-20261002.json`
- `test-results/ledger-cold-plan-20261002.json`

The exact instrumented binary remains with the original diagnostic folder:
`/private/var/folders/8t/c0ct0rc51k1__jw0mf2zwd0r0000gn/T/aib-ledger-sql-diagnostic.kmYWG0I3/`.
No SQL text, arguments, database credentials, or event bodies appear in the sanitized traces.

The user chose to retain the accepted design rather than start an ADR proposal.
No new performance optimization is retained. Both performance limits and deployment-profile acceptance remain open.

## Corrected controlled reference and delivery diagnostic

The corrected reference pins baseline `e6902cf296e617ff02e315a069d5dfb73f44bb51`, the current pre-ledger main revision. Both binaries use the same **test-only Go build overlay**: 40 open and 40 idle PostgreSQL connections. Startup opens all 40 backends before returning them to the idle pool. The production adapter remains at 25 open and 5 idle. The overlay comparison is not an unmodified deployment measurement.

The recording reference sets the existing `business_events.telemetry_copy_enabled=false` configuration. This excludes both the in-transaction delivery-reference insert and the delivery worker's database load. Ordinary OTLP traces, logs, and metrics remain enabled. A separate delivery diagnostic sets copying to true and includes both costs; neither mode adds a production bypass switch. Copy-disabled append measurements do not establish the cost of default copy-enabled recording.

The pre-transaction `ledger.record.validate` span measures event preflight. The `ledger.record` span measures append, including final-timestamp validation, serialization, and SQL. The runner sums these measured stages per action before calculating p99. Both stages exclude connection checkout, lifecycle and subject advisory-gate waits, and physical commit. The gate waits are separate and unaccounted for by the validation+append total. Thus, the stage-sum p99 cannot establish the full 5 ms recording-overhead target.

The `ledger.transaction` span measures the full owning transaction, including checkout, advisory gates, and commit. The HTTP action duration remains a separate distribution.

The fixed workload retains 10,000 principals, 100 agents, 20 services, one million historical feature events, and the 50/20/10/10/10 action mix. Each version receives a two-minute warmup and 100,000 measured actions. Three repetitions alternate baseline/feature, feature/baseline, baseline/feature. Event verification still checks every measured and warmup fact, each untimed approval preparation, relationships, credential canaries, and rollback counts. The 5 ms target is not relaxed.

Raw reports retain action latencies, per-action validation+append stage sums, each stage, and full owning-transaction durations. The `recording_span_nanoseconds` and `recording_p99_nanoseconds` fields name the stage sum, not total recording overhead. The runner reports aggregate pool wait count and duration and the baseline p99 range across repetitions. Pool wait counters are not per-action advisory-gate wait times. Feature p99 minus baseline p99 is a **difference of independent percentiles**, not a paired per-action effect. A noisy reference VM result cannot prove a pass or failure under deployment load.

Run the isolated recording reference from the repository root:

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" \
  AIB_LEDGER_PERFORMANCE_RESULTS_DIR="test-results/ledger-reference-corrected" \
  just test-e2e-performance 'performance && business-event-ledger && recording-reference' integration
```

Run the delivery-load diagnostic separately. Do not treat it as recording acceptance:

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" \
  AIB_LEDGER_PERFORMANCE_RESULTS_DIR="test-results/ledger-delivery-diagnostic" \
  just test-e2e-performance 'performance && business-event-ledger && delivery-diagnostic' integration
```

The release-acceptance label still requires `AIB_LEDGER_PERFORMANCE_PROFILE` from the operations owner. It runs the approved deployment settings with unmodified production binaries, then checks corrected reference completeness. Until that profile exists and runs, deployment approval remains blocked independently of the reference diagnostics.

## Corrected recording run (2026-10-03)

The corrected reference completed all three alternating pairs. Each version received a two-minute warmup and 100,000 measured actions. Baseline was main `e6902cf296e617ff02e315a069d5dfb73f44bb51`. Feature was `0ed87362eda183450ce9ff39ac569364e91aa50e+working-tree`.

The run used PostgreSQL 17.11, Darwin/arm64, 14 CPUs/GOMAXPROCS, and the local Docker VM with 3904 MB memory. Both binaries used the identical 40-open/40-idle, pre-warmed test-only pool overlay. Ordinary gRPC traces, logs, and metrics remained enabled. Ledger copying was disabled, so no delivery worker added database load. The feature retained one million historical events.

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" \
  AIB_LEDGER_PERFORMANCE_RESULTS_DIR="$PWD/test-results/ledger-recording-corrected-20261003" \
  just test-e2e-performance 'performance && business-event-ledger && recording-reference' integration
```

All durations in the table are milliseconds. The action difference is a difference of independent p99 values, not a paired per-action effect.

| Pair | Baseline action p99 | Feature action p99 | Independent difference | Append p99 | Validation+append p99 | Full owner transaction p99 |
|---|---:|---:|---:|---:|---:|---:|
| 1 | 7.781750 | 9.787208 | 2.005458 | 1.546375 | 1.581375 | 5.953541 |
| 2 | 15.634792 | 18.216167 | 2.581375 | 3.745875 | 3.851458 | 10.811083 |
| 3 | 12.128958 | 18.832500 | 6.703542 | 3.918250 | 4.017625 | 11.266750 |

Each binary reported zero pool waits, zero HTTP errors, and zero PostgreSQL rollbacks. Each feature repetition verified exactly 100,000 measured facts and 20,000 approval preparations. It also verified 106,753, 110,462, and 98,300 warmup facts respectively. Credential canaries were absent. Observed feature envelopes were 658–1022 bytes.

The baseline p99 range was 7.781750–15.634792 ms, a spread of 7.853042 ms. That variation exceeds the 5 ms target. Append and validation+append remained below 5 ms in this controlled run. Their durations exclude advisory-gate waits and physical commit, so they do not establish the full attributable overhead limit. The third action-p99 difference exceeded 5 ms, but these independent, variable distributions cannot establish a target failure.

**Verdict: NOT PROVEN.** The reference scenario passed its completeness, atomicity, credential, and measurement checks. It did not establish deployment acceptance. An approved deployment profile remains unavailable. Production pool defaults and the 5 ms target are unchanged. The separate delivery-load diagnostic was not run in this execution.

All six reports are under `test-results/ledger-recording-corrected-20261003/`, named `reference-repetition-{1,2,3}-{baseline,feature}.json`. They retain every action sample, every feature stage and transaction sample, pool counters, allocations, throughput, conditions, and verification counts.
