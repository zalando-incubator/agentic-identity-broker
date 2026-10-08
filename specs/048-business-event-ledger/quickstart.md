# Business Event Ledger — Validation Quickstart

This guide records implementation commands and observed validation results. The broker implements recording, investigation, retention, erasure, and recoverable telemetry.

The user removed the performance release gate and deployment-profile requirement on 2026-10-04. Functional verification remains required. The 5 ms goal is unverified and diagnostic only.

On 2026-10-06, the user also removed feature-specific diagnostic suites, runner commands, and setup. No replacement performance suite is required. All 20 active scenarios remain required in the existing backend E2E and integration lanes. Accepted ADR 039's storage/security design is unchanged.

Evidence sections preserve historical commands, results, former requirements, and blockers. They are not current usage instructions or new verification results. Use the current acceptance-lane table and sections 1–7 for runnable guidance.

## Pre-refactor baseline evidence (T001)

The production baseline is `main` revision `d500f36378dd914f8a516604a08525f737e8ddff`.
The capture checkout is `7d830d685715e03e5d054c3ef51d8ed5641a58df`.
The capture checkout differs from `main` only in planning artifacts, not production or test code.

The baseline run used Go 1.27.1 and Ginkgo v2.32.2.
The first attempt lacked the Ginkgo executable.
After installation, one scenario failed because testcontainers did not discover the active Colima socket.
This command supplied the socket and passed all 608 functional scenarios:

```bash
env PATH="/Users/brennenstuhl/go/bin:$PATH" \
  DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  just test-e2e-backend
```

The run reported 608 passed, zero failed, and one excluded performance scenario.
The frontend build also passed.
These results describe existing behavior, not ledger acceptance.

A temporary Go overlay captured JSON through `bootstrap.NewBufferedJSONLogger`.
The overlay wrapped test-factory loggers and the test process default logger.
It preserved the original log sinks and left the checkout unchanged.
A single-process run with `--label-filter='!performance'` and `--json-report` also passed all 608 scenarios.
The capture contained 6,398 log descriptors from 605 scenario contexts.

The fixture `tests/e2e/fixtures/business_event_ledger_legacy_slog.go` retains 28 catalogue entries and 31 workflow variants.
Each record contains its event name, level, message, field names, and observed count.
It contains no runtime attribute values.
The counts describe the named journey. They do not promise one old log per new ledger event.

Grant expiry and proxy issuance have explicit entries without dedicated fact logs.
Local and hybrid issuance retain their distinct field sets.

The acceptance review required a separate verified-client denial variant for US4-AS1.
A temporary checkout of `d500f36378dd914f8a516604a08525f737e8ddff` ran the original US3-S2 no-grant journey.
The run passed one selected scenario and captured this descriptor through the original buffered JSON logger:

```text
level=ERROR message="Token exchange failed" count=1
keys=actor,calling_peer,details,error,error_type,level,msg,resource,time,trace_id
```

The fixture retains this observed descriptor as `verified-client-no-grant`.
The original invalid-client variant remains unchanged.
The temporary capture code and checkout were removed.


Existing backend tests did not run session termination or agent deletion.
Two temporary extensions completed those HTTP workflows through the production builder:

- Session establishment followed by DELETE returned 200, then GET returned 404.
- Agent creation followed by DELETE returned 204, then GET returned 404.

Both extensions passed.
The fixture identifies their source scenarios and additional actions.
These runs included no production capture code or new ledger behavior.

`just fmt`, `just check`, and `go test ./tests/e2e/fixtures` passed for the baseline fixture.
`golangci-lint` was unavailable, so `just check` used its documented `go vet` fallback.
This result does not claim a golangci-lint run.

The session smoke exposed an existing contract difference.
The DELETE handler returns a top-level `message`, but OpenAPI documents `data.terminated` and `data.affected_agents`.
The first smoke assertion assumed the documented shape and failed.
The corrected baseline smoke verified the deletion status and subsequent 404.
This feature must preserve the existing response unless stakeholders separately approve a contract change.
T011 must record this difference.

### T001 commit gate

The first signed commit attempt failed with `gpg: signing failed: Operation cancelled`.
After user approval, the signed retry succeeded as `f9dcb7cb` (`test: capture business event legacy slog baseline`).
T001 is completed.
The commit-signing configuration remains unchanged.

## Refactor verification (T005)

T002–T004 introduce no ledger persistence or telemetry copies.
The shared transaction rename preserves current backend behavior.
The credential domain service owns the existing creation, rotation, and removal policy.
The proxy domain service owns client association and successful-response verification.
The transport adapter preserves streaming, permitted headers, response closure, and existing trace scopes.
`oauth2_token.go` retains its resolution and dispatch interface because that interface requires no change.

These checks passed after extraction:

```bash
just fmt
just check
go test ./internal/domain/oauth2/... ./internal/domain/oauth2server/... \
  ./internal/adapters/http/enduser/... ./internal/adapters/http/handlers/admin/... \
  ./internal/adapters/storage/... ./tests/integration/...
```

The Go test command reported 13 passing packages and one package without tests.
`just check` used the `go vet` fallback because golangci-lint was unavailable.
The format gate required the deleted transaction filename and its replacement to enter the Git index together.
No obsolete transaction names remain in `internal/` or `tests/`.

The single-process JSON capture run passed all 608 functional E2E scenarios after extraction.
The two deletion extensions also completed in that run.
The comparison found zero differences across all 31 baseline workflow variants.
It compared event names, levels, messages, field names, and counts.
Complete journey comparisons also passed for workflows without dedicated fact logs.
These results do not claim ledger acceptance or the excluded performance scenario.

A separate `go run ./tmp/ledger-refactor-smoke` process built the production application and sent real HTTP requests:

| Operation | Observed status |
|---|---:|
| Generate credential | 201 |
| Rotate credential | 200 |
| Issue with old secret | 401 |
| Issue with new secret | 200 |
| Revoke credential | 204 |
| Issue with revoked secret | 401 |
| Read revoked credential metadata | 404 |
| Delete agent | 204 |
| Read deleted agent | 404 |

The smoke observed one each of `CredentialGenerated`, `CredentialRotated`, `CredentialRevoked`, and `TokenIssued`.
It observed two `TokenRequestFailed` records.
Neither generated secret appeared in the logs.
The temporary smoke source and capture overlays were removed after these checks.

Two independent assistant reviews found no actionable compatibility or security defects.
One review covered transactions and credentials. The other covered proxy transport and completion.
The reviews were read-only and ran no additional checks.
The Fosite boundary wording names `HandleTokenEndpointRequest`, not the request constructor.

The feature branch contains baseline commit `f9dcb7cb` and refactor commit `9ad008c4`.
The isolated branch is `refactor/048-business-event-ledger-prerequisites`, based on `d500f36378dd914f8a516604a08525f737e8ddff`.
It contains only the baseline and refactor commits, as `d3c5de45` and `e8ddd218`.
Its worktree is `.worktrees/ledger-prerequisites`.

The isolated refactor PR is [#123](https://github.com/zalando-incubator/agentic-identity-broker/pull/123).
GitHub authentication completed through its device flow before submission.
Both branches reached the remote despite an OS keychain storage warning from Git's credential helper.
The independent assistant reviews and local runtime checks complete T005.
This statement does not claim human PR approval or CI completion. It does not claim approval of the ledger event and operational contracts.

## Setup evidence (T006–T007)

The existing `github.com/google/jsonschema-go v0.4.2` dependency is direct in `go.mod`.
No dependency version or `go.sum` entry changed.
`just check` passed with the existing vet fallback.
`go list -m github.com/google/jsonschema-go` returned `github.com/google/jsonschema-go v0.4.2`.

The setup review covered Go, Node, Docker, ESLint, Prettier, and Helm ignore files.
Only `.gitignore` needed the missing generic `*.log` pattern.
This application does not publish an npm package, and the repository contains no Terraform files.

Migration 035 remained free after `git fetch origin` and inspection of all 13 open PR branches.
The inventory came from:

```bash
gh pr list --repo zalando-incubator/agentic-identity-broker \
  --state open --limit 100 \
  --json number,headRefName,headRepository,headRepositoryOwner
git ls-tree -r --name-only origin/main -- migrations
git ls-tree -r --name-only origin/046-cimd-upstream-client -- migrations
```

The same tree inspection covered each same-repository PR head.
The two fork heads used their GitHub migration listings:
[PR 61](https://api.github.com/repos/axsaucedo/agentic-identity-broker/contents/migrations?ref=fix%2Fchart-token-exchange-defaults)
and [PR 62](https://api.github.com/repos/axsaucedo/agentic-identity-broker/contents/migrations?ref=fix%2Fchart-secret-checksums).

| Inspected heads | Highest migration |
|---|---:|
| `main`, PRs 123, 122, 120, 100 | 032 |
| PR 121 (`feat/cache-public-keys`) | 033 |
| PR 77 (`046-cimd-upstream-client`) | 034 |
| PRs 96, 85, 50, 43 | 031 |
| Fork PRs 61, 62 | 031 |
| PR 31 | 030 |

Both PR 121 and PR 77 currently contain migration 033.
This feature retains 035 and does not change those branches.
The performance comparison retains the T001 baseline revision `d500f36378dd914f8a516604a08525f737e8ddff`.

The user reported no approved deployment profile in the implementation conversation on 2026-09-27.
As a result, T095 had an open release blocker at that time.
The reference profile does not replace operator-approved deployment evidence.

## Design prerequisite evidence (T008–T011)

At this checkpoint, the architecture and root agent context contained the seven ledger glossary terms.
They also contained the accepted transaction, privacy, retention, and delivery design.
Both documents explicitly distinguished accepted design from deployed behavior.

Helm 3.22.0 supplied the chart validation after the initial command reported that Helm was absent.
The integrated configuration checks passed:

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-lint
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-template
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just check
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" \
  go test ./tests/integration -run TestHelmTemplate
```

A rendered ConfigMap used `retention=720h` and `telemetryCopyEnabled=false`. It contained the string `720h` and the boolean `false`.
The strict chart schema accepted both optional role names.
The default-backend render introduced no operational grants.
The example parsed as one YAML document with `2160h` and boolean `true`.

The example commands use the actual `just build` output path, `./bin/agentic-identity-broker`.
These are configuration and template checks, not runtime ledger evidence.

The user approved `contracts/events.md` and `contracts/storage.md` in this implementation conversation on 2026-09-27.
The recorded answer is `Approve both contracts`.
ADR 039 separately records maintainer Jan Brennenstuhl's acceptance on 2026-09-26.
ADR acceptance and assistant code review are not substitutes for that contract approval.

T011 reviewed the existing OpenAPI failure representations:

| Workflow family | Existing failure representation |
|---|---|
| Admin agents, credentials, signing keys | HTTP 500, `InternalServerError` using `ErrorResponse` |
| Consent grants | HTTP 500, `InternalServerError` using `ErrorResponse` |
| Third-party sessions | HTTP 500, `InternalError` using `Error`. Refresh also documents upstream HTTP 502 |
| OAuth2 authorization/token | HTTP 500 using `OAuth2Error`, including `server_error` |
| Approvals | Existing handlers return HTTP 500 with `error` and `message`. Endpoint response lists omit these existing 5xx cases |

The approval handlers use the existing `ApprovalError` field shape for internal failures.
The session-deletion response mismatch remains the separate baseline issue recorded in T001.
Both OpenAPI files remain unchanged.
The feature plan includes no public HTTP or CLI surface to read, erase, export, or ingest events.
An additional public behavior change or correction to these existing contract gaps requires separate approval.

### Schema publication (T012)

`api/events/v1/` contains the 30 approved schemas and 28 synthetic examples.
The schema files match the reviewed source files byte for byte.

A temporary Go program used the pinned `google/jsonschema-go v0.4.2` validator with an offline URN loader.
It resolved all 30 schemas. It validated every example against its selected schema and the catalogue.

It rejected 168 mutated examples. The mutations covered extra envelope/actor/data fields, invalid outcomes, UUIDv4 event IDs, and unknown types.
The loader also rejected an external HTTPS schema reference.

```text
schemas=30 examples=28 negative_cases=168 offline_refs_rejected=1
```

The temporary program was removed.
This result validates the published shapes. It does not validate domain provenance, semantic time parsing, storage, or ledger runtime behavior.

### Database design review (T013)

The review verified migration `035_business_event_ledger.{up,down}.sql` against the data model, storage contract, and binding storage/migration ADRs.
The approved design retains:

- Paired six-hour event and delivery partitions with the partition-pair helper
- Drop-free provisioning and separate scheduled maintenance
- Immutable envelopes, matching projected columns, and positive microsecond policy values
- Business-row expiry markers behind separate ISP ports
- Lifecycle, sorted subject, business-row, and delivery-row lock order
- Separate runtime, reader, erasure, and migration capabilities
- No event cascade foreign keys and no default partition
- Feature-only rollback with explicit history-loss acknowledgement, backup, and matching old binary.

T036 owns the foundation migration after its red tests.
T076, T077, and T081 add erasure, retention drops, and privileges after their own red tests.
This review applied no migration and supplies no database runtime evidence.

### Acceptance declarations (T014)

The generated `BusinessEventID`, envelope/query values, repository interfaces, lifecycle interface, and `App.LedgerService` compile.
The builder accepts offline schema sources through `WithBusinessEventSchemas`.
The configuration DTO also declares the two fields needed by acceptance fixtures.
This declaration step does not implement defaults, bindings, validation, schema registration, storage behavior, or producers.

`just fmt` and `just check` passed.
Existing ID, model, storage-factory, app, and configuration package tests passed.
The ledger package has no tests yet.
A separate production-builder smoke observed:

```text
ledger_service_declared=true retained_events=0 default_uuid_version=4
```

The empty repository results and UUIDv4 constructor are intentional inputs to the required red phase.
T023 changes only the event ID constructor to UUIDv7 after its failing test.
The temporary smoke source was removed.
These declarations are not a working ledger and must not be released.

### Acceptance harness runtime evidence (T015)

The ledger bootstrap selects memory without build tags and memory plus PostgreSQL with `integration`.
Each iteration creates its own production storage adapter, application, and two HTTP listeners.
The PostgreSQL path clones the shared migrated template and uses separate runtime, reader, erasure, and migration-owner connections.
Storage-port decorators provide append failure and an owner-commit completion barrier. The harness adds no production fault endpoint.

A temporary Ginkgo smoke ran this command:

```bash
env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  go test -tags=integration ./tests/e2e/bootstrap \
  -run TestLedgerHarnessSmoke -v -count=1
```

The smoke created and read an agent over real HTTP on both backends (201 then 200).
It verified the end-user listener against the configured public URL.
It started a separate production-builder child against the same PostgreSQL clone.
The child read the committed agent over HTTP. Termination and repeated termination completed successfully.
The run passed one smoke scenario with zero skipped cases and terminated its PostgreSQL 17 container.

The first smoke request omitted required agent fields. The next fixture attempt exposed the memory-only assumptions in `SeedDefaultConsentData`.
The passing smoke used the existing FK-complete `SeedPlaceholderGrantData` instead. Those existing fixtures remained unchanged.
These checks prove bootstrap behavior, not ledger recording, atomicity, or crash recovery.

The temporary smoke was removed.
`just check` passed, including golangci-lint with zero issues, and the bootstrap/helpers/matchers/fixtures package tests passed.

### Acceptance lanes (T016, current commands)

| Lane | Command | Selection |
|---|---|---|
| Memory and PostgreSQL | `just test-e2e-backend 'business-event-ledger && !performance'` | All 20 active scenarios, default `integration` tag, configured worker count |
| Memory only | `just test-e2e-backend 'business-event-ledger && !performance' ''` | Shared scenarios without the integration tag |
| Backend CI | `just verify-e2e-backend-junit` | Full integration-tagged functional backend suite, configured worker count, existing backend reports |

Normal `just verify` includes ledger acceptance through the shared integration-tagged backend lane, without a duplicate feature run.
CI runs PostgreSQL-backed acceptance through the normal `just verify-e2e-backend-junit` command.
It uses the integration tag and configured `GINKGO_BACKEND_PROCS` worker count.
Existing `Serial` declarations handle process-global cases. No ledger-specific CI job or step is necessary.

The existing `test-e2e-performance` recipe retains the original SC-001 broker measurement, without ledger-specific options.
The shared backend command and CI invocation build frontend assets and use `--fail-on-empty`.

Historical setup evidence: `actionlint .github/workflows/ci.yml` passed, and the former performance command's dry run selected the requested labels and build tag.
Before acceptance authoring, the tagged command selected zero of 609 existing specs and exited unsuccessfully because `--fail-on-empty` was set.
That is an empty-selection check, not semantic-red ledger acceptance.
The installed Ginkgo CLI initially reported 2.33.0 against the pinned 2.32.2 package. `go install github.com/onsi/ginkgo/v2/ginkgo@v2.32.2` corrected the local tool version.


### Catalogue and US1 acceptance authoring (T017)

A temporary workflow smoke exercised all 28 mapped catalogue actions through real HTTP on memory and PostgreSQL 17.
It completed 56 workflow variants.
Each variant used fresh applications and storage, the shared migrated PostgreSQL template, trusted upstream/JWKS fixtures, and the actual signing-operator authentication boundary.
The grants had a positive short lifetime and expired naturally. The smoke added no production time-travel control.
The smoke passed and its source was removed.

The seven US1 scenarios compile and ran with:

```bash
env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  /Users/brennenstuhl/go/bin/ginkgo -v --tags=integration --procs=1 \
  --fail-on-empty --label-filter='business-event-ledger' ./tests/e2e/
```

The run selected seven ledger specs. All seven failed semantically, with no pending ledger cases.
The 609 other specs were excluded by the label filter.

| Scenario | Observed red result |
|---|---|
| US1-AS1 | Real grant creation completed. The retained event count was zero instead of one |
| US1-AS2 | The production-builder child started. The committed-event SQL oracle reported a missing ledger relation (`42P01`) |
| US1-AS3 | Recording was absent, so forced append failure did not intercept it. Revocation returned 204 instead of a fail-closed error |
| US1-AS4 | The refused exchange completed. The retained denial count was zero |
| US1-AS5 | Concurrent revocations completed. The retained transition count was zero |
| US1-AS6 | Natural expiry and repeated recognition completed. The retained expiry count was zero |
| US1-AS7 | PostgreSQL grant creation completed. No event existed for backend comparison |

These results establish US1's initial red phase, not completed crash recovery, rollback, catalogue persistence, or backend parity.
T021 still requires all 21 scenarios before feature behavior can begin.

### Investigation acceptance authoring (T018)

The five US2 scenarios compile. This memory command selected all five. Each failed on absent retained events, not compilation or fixture setup:

```bash
/Users/brennenstuhl/go/bin/ginkgo -v --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger' \
  --focus='safe operational investigation' ./tests/e2e/
```

US2-AS1 completed its before/within/after, multi-agent, multi-principal and denied-exchange dataset. The query returned zero rather than the two expected in-window successes.

US2-AS2 completed a verified exchange with hostile forwarding/user-agent metadata and authoritative trace context. No attributed event existed.

US2-AS3 completed its first credential-canary workflow with a real local OTLP receiver. The missing stored envelope was the initial red failure.
This result does not claim that OTLP privacy passed.

US2-AS4 completed an unauthenticated token rejection. US2-AS5 completed a grant through the schema-extended production builder. Both found no retained event.

The full tagged, all-scenario red gate remains T021.

### Lifecycle acceptance authoring (T019)

The five US3 scenarios compile. The tagged `retention and exact-subject erasure` selection exercised all five.
US3-AS1–AS4 initially failed on absent retained events. The erasure, retention, concurrency, and restart assertions did not run.

The first US3-AS5 draft passed for the wrong reason: an unrelated missing JWE signing key rejected every startup.
A successful positive-retention control exposed that fixture error. The test supplies both required keys before it varies retention.

The corrected US3-AS5 run loads the valid configuration. It fails because the pre-feature loader accepts the zero-retention environment override.
The runnable example includes the required `IDENTITY_BROKER_JWE_SIGNING_KEY` export.

```bash
env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  /Users/brennenstuhl/go/bin/ginkgo -v --tags=integration --procs=1 \
  --fail-on-empty --label-filter='business-event-ledger' \
  --focus='retention and exact-subject erasure' ./tests/e2e/

/Users/brennenstuhl/go/bin/ginkgo -v --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger' \
  --focus='rejects malformed or non-positive retention' ./tests/e2e/
```

No lifecycle guarantee is claimed green. The unavailable-collector variant restores its test fault before teardown so the existing slog exporter can close normally.

### Integrated acceptance review and red runs (T020–T021)

The harness corrections address the 20 acceptance-review findings. The approved event and HTTP contracts remain unchanged.
Catalogue assertions compare complete event deltas, caller identities, known references, and effective expiry times.
Privacy assertions track actual consumed and returned credentials and inspect only ledger-scope telemetry after shutdown.

PostgreSQL comparisons inspect raw persisted JSON before model decoding.
The telemetry comparator verifies the raw envelope, export-time observation, native correlation, empty body, and zero dropped attributes.
A temporary live gRPC smoke accepted a valid historical copy and rejected six malformed copies.
The malformed cases covered stale observation, unrelated native trace/span, dropped attributes, nonempty body, and an omitted nullable subject.
The smoke source was removed.

Crash coordination kills the child before it closes stdin. It joins the outstanding HTTP request after process exit.
Erasure overlaps a recorder paused before its owner commit.
Recovery assertions require an unaffected pending control to deliver and acknowledge first. Then they reject deleted history.
Retention fixtures distinguish occurrence age from recording age and retain mixed-age partition pairs.

The integrated functional red runs used:

```bash
/Users/brennenstuhl/go/bin/ginkgo -v --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' \
  --json-report=/tmp/ledger-memory-red-048.json ./tests/e2e/

env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  /Users/brennenstuhl/go/bin/ginkgo -v --tags=integration --procs=1 \
  --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --json-report=/tmp/ledger-functional-red-048.json ./tests/e2e/
```

The memory run selected 14 scenarios, and all 14 failed semantically.
The tagged run selected 20 scenarios, and all 20 failed semantically.
No selected scenario was pending, skipped, or excluded.
Initial failures were missing retained facts, missing ledger relations (`42P01`), or acceptance of invalid retention.
These failures do not prove the later rollback, privacy, deletion, or recovery assertions green.

Each scenario has one `It` and one adjacent specification reference:

| Scenario | Source file under `tests/e2e/` | Initial functional red result |
|---|---|---|
| US1-AS1 | `business_event_ledger_test.go` | No retained catalogue fact |
| US1-AS2 | `business_event_ledger_postgres_test.go` | Missing ledger relation after child workflow |
| US1-AS3 | `business_event_ledger_test.go` | No retained export-control fact |
| US1-AS4 | `business_event_ledger_test.go` | No retained denial |
| US1-AS5 | `business_event_ledger_test.go` | No retained winning transition |
| US1-AS6 | `business_event_ledger_test.go` | No retained expiry fact |
| US1-AS7 | `business_event_ledger_postgres_test.go` | Missing relation for raw parity query |
| US2-AS1 | `business_event_ledger_test.go` | Zero successes instead of two |
| US2-AS2 | `business_event_ledger_test.go` | No attributed exchange fact |
| US2-AS3 | `business_event_ledger_test.go` | No stored credential-free envelope |
| US2-AS4 | `business_event_ledger_test.go` | No explicit-null failure fact |
| US2-AS5 | `business_event_ledger_test.go` | No retained catalogue control |
| US3-AS1 | `business_event_ledger_test.go` | No retained subject facts |
| US3-AS2 | `business_event_ledger_postgres_test.go` | No retained live retention control |
| US3-AS3 | `business_event_ledger_postgres_test.go` | No retained predecessor control |
| US3-AS4 | `business_event_ledger_postgres_test.go` | Missing relation for deferred-copy control |
| US3-AS5 | `business_event_ledger_test.go` | Non-positive retention accepted |
| US4-AS1 | `business_event_ledger_test.go` | No retained fact for signal comparison |
| US4-AS2 | `business_event_ledger_test.go` | No retained fact with copying disabled |
| US4-AS3 | `business_event_ledger_postgres_test.go` | Missing relation for committed-copy query |
| US4-AS4 | `business_event_ledger_performance_test.go` | Completed admin mutation without a retained event or ledger relation |

The isolated performance selection used the production baseline and feature binaries:

```bash
env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  /Users/brennenstuhl/go/bin/ginkgo -vv --tags=integration --procs=1 \
  --fail-on-empty --label-filter='performance && business-event-ledger' \
  --json-report=/tmp/ledger-performance-red-048.json ./tests/e2e/
```

Both binaries built from separate source worktrees.
The feature executable started against the migrated PostgreSQL database under an isolated DML role.
Its real HTTP admin mutation completed, but the retention preflight failed because `public.business_events` was absent.
The selected scenario had one semantic failure and no additional cleanup failure.
The temporary baseline checkout and test database were removed.
No workload latency distribution or performance pass is claimed.

The first performance attempt exposed missing read permission on `schema_migrations`.
The runtime role receives only SELECT permission on that migration metadata.
A later run exposed cancellation of the child before its graceful cleanup.
The bootstrap leaves child termination to its bounded cleanup rather than the cancelled spec context.
Neither correction changes production code or gives the runtime role DDL privileges.

A separate temporary HTTP smoke completed all 28 catalogue workflows on both production backends: 56 workflow variants.
The smoke observed explicit signing-key promotion and repeated current-key selection. It did not bypass the permanent ledger assertions.
The smoke source was removed.

The retained acceptance suite contains exactly 21 scenario references and 21 `It` blocks.
All 21 scenarios have observable semantic-red evidence across their declared lanes.
The T008–T020 prerequisites and recorded contract approvals permit foundation work after this gate.

At that time, the unavailable approved deployment profile still blocked T095 release acceptance.

`just fmt`, `just check`, and the bootstrap, helpers, matchers, and fixtures package tests passed after integration.
The linter reported zero issues.
The quality-delta tool reported stub reachability and repeated synchronization/cleanup patterns rather than a clean gate.
The stub removal belongs to T042.
The bounded test controls remain separate because their state and lifecycle semantics differ.
No quality acknowledgement suppresses those findings.

### Foundation boundary tests (T022)

```bash
go test ./internal/domain/id ./internal/domain/ledger \
  -run 'Test(BusinessEventIDGeneration|OtherEntityIDsRemainUUIDv4|Event)' -count=1
```

The tests compiled and failed semantically before implementation.
The event constructor produced UUIDv4 instead of UUIDv7.
Validation accepted malformed identities, times, references, context, outcomes, reasons, and payloads.
Serialization included unavailable zero-valued UUID references and retained duplicate, unsorted permission sets.
Decoding silently discarded additional envelope and actor fields.
The existing entity constructors remained UUIDv4.

### Identifier generation (T023)

The existing generator selects UUIDv7 only for `BusinessEventID`.
`go generate ./internal/domain/id/` regenerated the constructors, and the full ID package tests passed.
A standalone API smoke observed:

```text
event_uuid_version=7 agent_uuid_version=4 credential_uuid_version=4 roundtrip_equal=true
```

`just fmt` and `just check` passed with zero lint issues.
The ID catalogue records immutability, request exclusion, and the absence of a global causal-order guarantee.
The smoke source was removed.

### Event values, offline registry, and configuration boundaries (T024–T030)

The event wire view retains required nullable identities and omits unavailable zero-valued references.
Permission sets serialize as a sorted unique set. The caller's slice remains unchanged.
The decoder rejects additional envelope, actor, and client fields. It does not silently remove them.
Typed validation rejects invalid UTC times, zero identifiers, IPs, correlation IDs, uncontrolled reasons, and invalid payloads.

The value API smoke validated all 28 published examples and their serialization round trips.
Its source was removed after the run.
Storage assignment of recording time and durable immutability remain repository obligations in the later foundation tasks.

The registry and configuration tests compiled and exposed missing behavior before implementation:

```bash
go test ./api/events ./internal/domain/ledger ./internal/config \
  ./cmd/agentic-identity-broker \
  -run 'Test(Published|Registry|BusinessEvents|RootCommandLoadsBusinessEvent)' -count=1
```

Registry failures covered additive types, fixed reasons, redefinitions, reserved property names, flattened collisions, and remote references.
Configuration failures covered defaults, environment/CLI precedence, positive retention, microsecond normalization, overflow, and missing root flags.

The integrated implementations passed:

```bash
just fmt
just check
go test ./api/events ./internal/domain/ledger ./internal/domain/model \
  ./internal/domain/id ./internal/config ./cmd/agentic-identity-broker -count=1
```

The linter reported zero issues.
An independent smoke exercised the embedded catalogue and canonical configuration loader:

```text
embedded_catalogue_validated=28 normalized_retention=2µs explicit_false_preserved=true zero_retention_rejected=true
```

The smoke source was removed.
General `Validate` enforces the same retention contract as the loader.
Direct validation fixtures supply the mandatory ledger defaults. They use no compatibility bypass.
Schema validation uses the returned primitive wire view, without a marshal/unmarshal validation round trip.
The obsolete custom per-type shape validator was removed after the compiled registry replaced it.

### Transaction and repository semantic-red gate (T032–T035)

The foundation tests compiled and failed on missing transaction and repository behavior:

```bash
go test ./internal/adapters/storage/memory ./internal/adapters/storage/postgres \
  ./internal/domain/ledger \
  -run 'Test(MemoryTransaction|MemoryStorage|MemoryDelivery|MemoryBusinessEvent|BusinessEventRepository_|ServiceRecord|ServiceQuery)' -count=1
go test -tags=integration -count=1 ./internal/adapters/storage/postgres \
  ./tests/integration/storage/infra \
  -run '^(TestPostgresBusinessEvent_|TestBusinessEventMigration_|TestBusinessEventLedger_|TestBusinessEventQuery_)'
```

Memory rollback retained changed rows and indexes. Joined scopes exposed uncommitted state and accepted stronger isolation.
Event accessors did not assign recording times. PostgreSQL tests exposed the missing migration and advisory gates.
Recorder tests exposed missing transaction ownership. Invalid investigation filters reached storage.

The PostgreSQL agent fixture initially lacked a required permission set.
After fixture repair, its test compiled and failed because the ledger table was absent.
No assertions were weakened.

### Foundation migration (T036)

The real PostgreSQL migration and provisioning tests passed:

```bash
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^(TestBusinessEventMigration_|TestBusinessEventLedger_Provision)'
```

The tests covered up/down/up, existing business-data survival, all 32 partition pairs, historical provisioning, and no default partition.
A separate `go run` smoke applied every migration and exercised direct SQL insertion, immutable-update rejection, and repeated provisioning:

```text
partition_pairs=32 retained_event=1 immutable_update_rejected=true provisioning_idempotent=true
```

The smoke container terminated, and the temporary source was removed.
Foundation maintenance only provisions partitions. Retention and erasure remain their later story tasks.

### PostgreSQL owner/join cutover (T037)

The shared manager acquires lifecycle and sorted subject gates before business access.
Joined scopes cannot commit the owner. Joined rollback poisons the owner.
Stronger isolation and late subject discovery require a fresh transaction.
Repository writes and reads use the ambient executor. Signing bootstrap acquires its lock before lifecycle protection.

Permission-set deletion retains serializable isolation and only retries standalone transactions.

The following checks passed:

```bash
just fmt
just check
go test -count=1 ./internal/adapters/storage/postgres ./internal/domain/oauth2server \
  -skip '^TestBusinessEventRepository_'
go test -tags=integration -count=1 ./internal/adapters/storage/postgres \
  -run 'Test(SigningKeyRepo|AgentRepository|PermissionSet|UserGrant|ClientCredential|PostgresBusinessEvent_LifecycleAndSubjectGatesPrecedeBusinessRowLock)'
```

The excluded ledger-repository unit tests remain red until T039.
The obsolete SQL-text locking test was removed. Real PostgreSQL signing-key and lock-contention tests remain.
A separate production-adapter smoke applied migrations through the migration library and produced:

```text
joined_commit_invisible=true owner_commit_publishes=true joined_rollback_restores=true stronger_isolation_rejected=true commit_only_effect=true late_subject_rejected=true
```

The smoke source and container were removed after the run.

### Memory owner/join cutover (T038)

The factory injects one required coordinator into all memory stores.
Lifecycle protection precedes the visibility gate and store-local locks.
Rollback restores touched records and indexes in reverse order without a whole-store snapshot.
Copy-on-write mutations preserve journal entries. Returned mutable values are independent.
Leaf operations do not allocate operation contexts or release closures.

The public approval-denial regression first failed because an uncommitted decision woke its subscriber.
After commit-only notification registration, both commit and rollback cases passed.
The transaction tests retain the existing `invalid_target` resource-lookup contract and test the original proposed session ID after upsert.

These checks passed:

```bash
just fmt
just check
go test -count=1 ./internal/adapters/storage/memory ./internal/adapters/storage \
  ./internal/domain/approval ./internal/domain/oauth2server ./internal/domain/oauth2session \
  -skip 'TestMemory(BusinessEvent|DeliveryCallback)'
go test -race -count=1 ./internal/adapters/storage/memory ./internal/domain/approval \
  -skip 'TestMemory(BusinessEvent|DeliveryCallback)'
```

Event repository tests remain red until T040. The collector callback case requires the later T090 dispatch implementation.
A separate production-factory and approval-service smoke produced:

```text
reader_excluded=true multi_store_rollback=true joined_commit_silent=true rollback_notification_silent=true owner_commit_notifies=true
```

The smoke source was removed after the run.

The scoped quality report was reviewed, not declared clean.
It reports repeated repository guards, typed map constructors, transaction getters, and explicit touched-key journals as clone groups.
The agent update method adds 11 lines for gate and journal participation.
These are deliberate transaction obligations, not a reason to introduce a generic repository framework.

The report's unused-test findings refer to Go test entry points. `AfterCommit` is consumed through its port interface.

### PostgreSQL event persistence (T039)

Append acquires the ordered gates, assigns database recording time, validates the final wire view, and serializes it once.
Events and optional payload-free delivery references share the owning transaction.
Prepared retries retain their original ID and recording time. Duplicate append returns a conflict.
Query uses exact subject selection, bounded limits, occurrence order, and a strict tuple cursor without business-row joins.

Fractional-microsecond bounds retain exact `[start,end)` membership despite PostgreSQL timestamp precision.

Database errors retain classifications and safe SQLSTATE codes, not rejected event values.

These direct-repository checks passed:

```bash
just fmt
just check
go test -count=1 ./internal/adapters/storage/postgres ./internal/domain/model ./api/events \
  -run 'Test(BusinessEventRepository_|Published|BusinessEvent)'
go test -tags=integration -count=1 ./internal/adapters/storage/postgres \
  -run '^TestPostgresBusinessEvent_'
```

The filtered model package had no matching tests. The factory-backed investigation suite awaits T042 wiring.
Its new fractional-boundary regression compiled and failed before implementation.
Shared registry query validation is available for both repositories and the T041 service.
A separate direct-repository smoke applied real migrations and produced:

```text
exact_precision_queries=3 payload_free_reference=true rollback_removes_event=true retry_keeps_composite_key=true duplicate_conflict=true
```

The temporary source and container were removed after the smoke.
The scoped quality report had no preexisting major regressions.
It reports the new explicit query builder and wire serializer as complex, and JSON interface methods as unused static entry points.

### Memory event persistence and review corrections (T040)

The memory repository uses the shared coordinator for append/get/query and stores independent event values.
It canonicalizes reference sets at append, omits untrusted correlations, and preserves typed primitive array values without aliases.
Event rows and payload-free pending-reference keys share the touched-key journal.
Scoped queries retain exact time bounds and strict occurrence/ID cursors.

These checks passed:

```bash
just fmt
just check
go test -count=1 ./internal/adapters/storage/memory \
  -run '^TestMemoryEventReferencesCommitAndRollbackWithTheirOccurrences$'
```

A production-constructor smoke produced:

```text
independent_input_and_get=true unsafe_correlation_omitted=true fractional_query_exact=true rollback_removes_event=true retry_keeps_recorded_time=true
```

The smoke source was removed after proof. Factory-backed event tests await T042 wiring.
User approval on 2026-10-01 defers unchanged delivery-only checks to T090. It does not move dispatch before lifecycle implementation.

Independent reviews identified three concrete regressions. New assertions failed before their corrections:
Memory rollback retained changed protected-resource child sets. PostgreSQL idempotent provider deletion poisoned its joined owner.
Missing-partition append failures used the input-validation category.
The corrections journal individual child keys, complete successful no-op joined scopes, and classify internal CHECK failures as storage failures.
The focused regressions, static checks, and memory/approval race suites passed afterward.

The memory-event quality report was reviewed, not declared clean.
It reports typed constructors/copy helpers as clone groups and the explicit scoped-query loop as new complexity.
Those shapes retain the existing repository convention without a generic storage framework.

### Recorder service and investigation policy (T041)

Registered recipes generate UUIDv7 IDs and select the fixed type, source, outcome, and safe reasons.
Record validates before transaction work, joins an existing owner or opens a fresh owner, and reports success only after scope commit.
Append or commit errors roll back the unsuccessful scope. Committed scopes do not receive redundant rollback calls.
Independent failures use a fresh request context after caller rollback. The recorder retries no external operation.

The investigation service rejects absent subject selection and incompatible type/outcome pairs before repository access.

These checks passed:

```bash
just fmt
just check
go test -count=1 ./internal/domain/ledger ./api/events ./internal/domain/model
```

A production service/repository smoke produced:

```text
committed_record_visible=true joined_record_rolled_back=true missing_subject_rejected=true credential_payload_rejected=true
```

The temporary source was removed after the run.
The scoped quality report identifies constructor-shaped clones across unrelated typed constructors.
Those findings were reviewed. Distinct domain contracts remain separate without a generic constructor.

### Factory, builder and foundation gate (T042–T043)

The factory supplies the real backend event repository and shared transaction manager.
The builder compiles the embedded catalogue before it assembles the ledger recorder.
Invalid schema registration stops `Build`. The builder returns no application to serve requests.
Lifecycle operations and delivery dispatch remain assigned to their later story tasks.

These integrated checks passed:

```bash
just fmt
just check
go test -count=1 ./internal/adapters/storage ./internal/adapters/storage/memory ./internal/app \
  -skip 'TestMemory(BusinessEventQueues|BusinessEventRollback|DeliveryCallback)'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^(TestBusinessEventMigration_|TestBusinessEventLedger_|TestBusinessEventQuery_)'
go test -count=1 ./internal/domain/ledger ./internal/domain/id ./api/events ./internal/config ./cmd/agentic-identity-broker
go test -race -count=1 ./internal/adapters/storage/memory \
  -skip 'TestMemory(BusinessEventQueues|BusinessEventRollback|DeliveryCallback)'
go run ./.ledger-foundation-smoke
```

The production-builder smoke produced:

```text
production_builder_record_query=true owner_rollback_atomic=true invalid_schema_blocks_startup=true
```

The temporary source was removed after proof.
The approved gate clarification defers unchanged delivery-only tests to T090. This section claims no dispatch behavior.

The scoped factory quality report was reviewed, not declared clean.
It flags `BusinessEvents` as dead code despite its interface consumers in the builder and repository tests.
Language-server references verified those consumers. Other rows concern short-horizon churn rather than a required design change.

### Producer mutation regressions (T044)

Focused tests live beside the existing service tests in each package's `business_event_test.go`.
The tests cover exact grant/approval/agent transitions, silent unchanged and repeated operations, a losing approval CAS, and append/commit rollback.
They also cover effective-expiry renewal and actual agent-deletion child facts.

The agent cascade includes two principals and a permanent approved approval.
It includes an already-denied approval that must not become a new denial.
It includes a credential identified only by its safe record ID. An unrelated agent's grant must remain unchanged.

The shared test-only transaction fixture stages events and snapshots the small mock datasets.
As a result, tests observe business state and sync versions after failed recording. This fixture is not a production transaction implementation.

Private producer dependencies and the narrow agent-dependent enumeration facet are declarations for the later implementation.
This test task enables no producer recording behavior.

```bash
go test -count=1 ./internal/domain/consent ./internal/domain/approval ./internal/domain/agents \
  -run '^(TestConsentLedger|TestApprovalLedger|TestAgentLedger)'
just fmt
just check
```

The test command compiled and failed semantically. Transitions retained zero events, and injected append/commit failures still returned success.
The CAS-loser case preserved its existing failure semantics. Static checks then passed with zero lint issues. Existing acceptance expectations were unchanged.

The scoped quality report was reviewed, not declared clean.
It identifies the rollback matrices as new test complexity and typed mock enumeration/snapshot shapes as duplication.
The separate domain fixtures preserve explicit state assertions instead of a generic repository framework.
The report identifies interface implementations and Go test entry points as statically unused.

### Issuance, credential, signing and session regressions (T045)

The new focused tests cover credential generation/replacement/revocation and secret/token withholding during a blocked owning commit.
They cover append/commit failures, real Fosite replay revocation, bootstrap competition, and actual current-key selection versus future eligibility.
They cover callback reauthorization, session termination, terminal refresh failure, and accepted refresh survival after a later rollback.
Callbacks and refresh use a local HTTP token endpoint and valid JWE state.
The fixture initially omitted the provider's explicit public-client authentication method.
The fixture correction preceded acceptance of session red evidence.

```bash
go test -count=1 ./internal/domain/oauth2server ./internal/domain/oauth2session \
  -run '^(TestCredentialLedger|TestProviderLedger|TestSigningLedger|TestSessionLedger)'
go test -count=1 ./internal/domain/oauth2session ./internal/domain/oauth2server \
  -run '^(TestSessionLedger|TestSigningLedgerCompetingBootstrap)'
just fmt
just check
```

The tests compiled. Credential/token withholding failed because the tests reached no owning commit.
Injected recording failures returned success. Signing/session transitions produced zero facts.
The corrected session paths completed callback/refresh/termination before they failed for absent recording.
Competing bootstrap created one key but retained zero selection facts.

Replay-revocation preservation remained green under injected failure-event append errors. Static checks passed with zero lint issues.

The scoped quality report was reviewed, not declared clean.
It flags Go test/interface entry points as unused and similar explicit rollback matrices as clones.
It flags a session-specific copy helper as a clone of an unrelated document copy.
Those typed domain fixtures remain without a generic cross-domain copy abstraction.

### Typed outcomes and staged transport regressions (T046)

The focused tests cover one specific exchange refusal without generic duplicate facts and a committed impersonation decision before minting.
They cover decision survival after mint failure and one final policy denial across candidate rules.
They cover internal-error classification and no mint after recording failure.

Proxy completion must stage every response and preserve known agent attribution.
It must not invent a subject from unverified upstream claims. It must record terminal transport/verification failures.
If recording fails, proxy completion must withhold the staged body.

HTTP tests distinguish approved staged bytes from different unstaged transport bytes, including hybrid proxy dispatch.

```bash
go test -count=1 ./internal/domain/oauth2 ./internal/domain/tokenexchange ./internal/domain/impersonation ./internal/adapters/http/enduser \
  -run '^(TestProxyLedger|TestExchangeLedger|TestImpersonationLedger|TestTokenGrantProxy)'
just fmt
just check
go test -count=1 ./internal/adapters/http/enduser \
  -run '^(TestLocalGrantStrategy_AcceptsLocalClient|TestHybridTokenGrantStrategy_DispatchByClientType|TestProxyGrantStrategy_InfraErrorsReturnJSON)$'
```

The new tests compiled and failed semantically. Specific domain outcomes retained zero facts.
When minting began, permission decisions were absent. Proxy/hybrid HTTP paths wrote the unstaged canary rather than completed bytes.
Existing local/hybrid routing and proxy infrastructure-error response tests passed. Static checks passed with zero lint issues.

The scoped quality report had no preexisting major regressions.
New rows report test/interface entry points as unused and distinct impersonation-outcome assertions as clone shapes.
These cases retain their different decision and mint boundaries.

### Expiration recognition repository regressions (T047)

Memory and PostgreSQL tests cover expired candidates, one recognition winner, unchanged-versus-renewed validity, stale recognizers, ambient rollback, and competing recognizers.
They also cover PostgreSQL connection/deadline classifications.
Resolved permanent approvals retain their existing lifecycle rather than the pending TTL.
The method declarations contain no behavior and only let the focused tests compile.
T048 must replace those inert bodies before a producer uses them.

```bash
go test -count=1 ./internal/adapters/storage/memory ./internal/adapters/storage/postgres \
  -run '^(TestMemory(Grant|Approval)Expiration|Test(Grant|Approval)ExpirationRepository)'
go test -tags=integration -count=1 ./tests/integration/storage/infra ./internal/adapters/storage/postgres \
  -run '^(TestBusinessEventExpirationMarkers_|TestPostgres(Grant|Approval)Expiration)'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventExpirationMarkers_AtomicRecognitionAndRenewal$/^approval$'
just fmt
just check
```

The tests compiled and failed semantically. Candidate lists were empty, recognition returned false, and competing recognizers had zero winners.
Missing/deadline databases returned no error. The live grant and adapter-package approval cases were red for absent recognition.

The operational approval SQL fixture needed separate UUID/text parameters and its required gateway identity.
After both fixture corrections, that live case was red for zero candidates rather than SQL errors.
Static checks passed with zero lint issues.

The scoped quality report had no preexisting major regressions.
It flags the two-family live test's fixture branches and typed equivalent assertions as new complexity/duplication.
Each family retains observable database marker checks.

### Expiration marker implementations (T048)

Both existing grant/approval adapters implement the separate expiration facets.
Factory accessors expose those facets without new operations in the business repository interfaces.

Memory keeps payload-free expiry instants in adapter-owned maps. It journals touched markers with the shared transaction.
It removes grant markers with physical business deletion.

PostgreSQL uses conditional ambient-executor updates and the existing schema columns.
Candidate reads are bounded. Stale or already recognized expiries lose.
Resolved approvals do not acquire pending-expiration facts.

```bash
just fmt
just check
go test -count=1 ./internal/adapters/storage ./internal/adapters/storage/memory ./internal/adapters/storage/postgres \
  -run '^(TestMemory(Grant|Approval)Expiration|Test(Grant|Approval)ExpirationRepository)'
go test -tags=integration -count=1 ./tests/integration/storage/infra ./internal/adapters/storage/postgres \
  -run '^(TestBusinessEventExpirationMarkers_|TestPostgres(Grant|Approval)Expiration)'
go run ./.ledger-foundation-smoke
go test -race -count=1 ./internal/adapters/storage/memory -run '^TestMemory(Grant|Approval)Expiration'
```

Static checks, focused memory/PostgreSQL unit tests, both live PostgreSQL suites, and memory recognition races passed.
The filtered factory package had no matching tests. The runtime smoke exercised its accessors:

```text
factory_expiration_facets=true rollback_reopens_recognition=true competing_recognizers_one_winner=true recognized_candidate_removed=true
```

The smoke source was removed after proof.

The scoped cumulative quality report was reviewed, not declared clean.
It retains earlier transaction-guard/typed-constructor clone findings.
It reports the new PostgreSQL grant row scan as a clone of the existing typed scan.
It reports interface accessors as statically unused.
These shapes preserve existing adapter conventions without an additional storage framework or N+1 candidate reads.

### Atomic grant change recording (T049)

Grant creation, changed effective permissions/validity, and actual revocation append their exact facts in the owning transaction.
Unchanged state and absent idempotent revocations are silent.
The user-facing revoke endpoint retains its not-found result and existing success log fields.

The builder injects the required recorder. All language-server-discovered constructor callers were migrated.
Recording and business mutations share the ledger's owning/joined transaction orchestration. The implementation adds no second transaction mechanism.

PostgreSQL ambient grant lookup locks the principal/agent pair before its row, including the absent-row case.
The pair lock is transaction-scoped. Its separate two-int lock class does not use the lifecycle/subject lock classes.
Memory uses its existing visibility gate.

An additional regression showed that reordered included services created a false update fact.
It failed before the allocation-free effective-permission comparison and passed afterward.

```bash
just fmt
just check
go test -count=1 ./internal/domain/ledger ./internal/domain/consent ./internal/app ./internal/adapters/http/handlers/consent \
  -skip '^TestConsentLedgerExpiry'
go test -count=1 ./internal/domain/tokenexchange -skip 'TestExchangeLedger'
go test -count=1 ./internal/domain/consent -run '^TestConsentLedgerPermissionOrderingDoesNotCreateUpdateFact$'
go run ./.ledger-foundation-smoke
go test -tags=integration -count=1 -v ./tests/integration/storage/infra -run '^TestGrantAtomicRuntimeSmoke$'
```

Static checks and the affected grant/ledger/builder/handler tests passed.
The exclusions retain the already-authored expiry and exchange recording assertions for their assigned T050/T055 implementations.
Those assertions remain unchanged.
The production-builder smoke produced:

```text
production_builder_grant_facts=3 unchanged_update_silent=true concurrent_revoke_one_fact=true
```

The live PostgreSQL domain smoke completed eight concurrent idempotent revocations without an error.
It retained one exact `grant-revoked` fact and removed the grant. It exercised domain results, not HTTP status codes.
Both temporary sources were removed after proof.

The scoped quality report was reviewed, not declared clean.
It reports small constructor/transaction growth and explicit membership loops in the order-independent comparison.
It reports cumulative clone findings for unchanged typed repository reads/constructors.
Those findings do not justify a generic repository or cross-domain constructor abstraction.

### Lazy grant expiration recording (T050)

Grant access, active/user-grant reads, delegation reads, and OAuth2 authorization recognize effective expiry through the consent-domain invariant.
Marker and fact commit together. Repeated recognition is silent. Failed recording restores recognition.
A changed validity permits a different expiry. Expiration facts use effective expiry as occurrence time and the system lifecycle actor.

Authorization receives the consent service. It does not independently query its grant repository.
All discovered constructor callers were migrated. The existing expired-grant consent redirect remains unchanged.

The grant expiration facet selects exact-principal candidates before the bounded limit.
Memory and live PostgreSQL regressions failed for absent scoped candidates before implementation and passed afterward.
This selection prevents unrelated expired rows from exhausting a delegation-read batch. It does not recognize another principal's state.

```bash
just fmt
just check
go test -count=1 ./internal/domain/ledger ./internal/domain/consent ./internal/domain/oauth2 ./internal/domain/tokenexchange ./internal/app ./internal/adapters/http/enduser ./internal/adapters/http/handlers/consent \
  -skip '^(TestProxyLedger|TestExchangeLedger|TestTokenGrantProxy)'
go test -count=1 ./internal/adapters/storage/memory ./internal/adapters/storage/postgres \
  -run '^(TestMemoryGrantExpiration|TestGrantExpirationRepository)'
go test -tags=integration -count=1 ./tests/integration/storage/infra -run '^TestBusinessEventExpirationMarkers_'
go test -count=1 ./tests/integration -run 'OAuth2(Authorize|Authorization|Metadata)'
go run ./.ledger-foundation-smoke
```

The domain, builder, HTTP, and backend checks passed after legacy grant mocks acquired their required marker facets.
No expectations changed. Separate HTTP integration expiry/metadata checks passed after their fixture migration.
The exclusions retain the pending T054/T055 outcome regressions.
The production-builder smoke exercised expiry first through authorization and verified its consent redirect.
Then it ran grant/delegation reads again and observed:

```text
production_builder_lazy_expiry_one_fact=true effective_expiry_timestamp=true system_actor=true public_expired_grants_filtered=true
```

The smoke initially omitted the required original authorization URL for JWE session claims.
The fixture correction preceded acceptance of runtime proof. Its source was removed afterward.

The cumulative scoped quality report was reviewed, not declared clean.
It reports small added branches on authorization/delegation, existing transaction-guard/typed-reader clone shapes, and unused interface entry points.
Exact-subject candidate selection and consent-domain reuse remain without a separate scheduler or public ledger API.

### Session lifecycle recording (T051)

Accepted callbacks, including reauthorization, store encrypted session state with one `session-established` fact.
Accepted refresh state commits with `session-refreshed` before later exchange work.
Caller-visible refreshed state changes only after commit.
Actual termination and its captured session/service/subject references share the owning transaction.

Upstream and encryption preparation remain outside the write scope.
A failed terminal refresh records one fixed `session-refresh-failed` reason after unsuccessful mutation rollback, not one per network retry.
Typed refresh classifications preserve existing error wrapping and logs without raw diagnostics in event data.

PostgreSQL ambient session lookup locks the principal/service pair before its row, including absent callbacks.
It uses a distinct two-int lock class. The builder and all discovered constructor callers supply the required recorder.

The newly authored failure test initially expected empty data.
The correction used the reviewed schema's required `reason_code: upstream_rejected`. The correction report preceded the edit.
The published schema and existing acceptance expectations were unchanged.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2session ./internal/app ./internal/domain/tokenexchange -skip '^TestExchangeLedger'
go test -count=1 ./tests/integration \
  -run '^Test(ListSessions|AuthorizeEndpoint|CallbackEndpoint|DeleteSession|RefreshSession|GetSession)_'
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestPostgresUserSession|^TestUserSession'
go run ./.ledger-foundation-smoke
```

Static checks, the complete session domain package, affected builder/exchange tests, matching HTTP integration scenarios, and PostgreSQL session repository tests passed. An earlier `OAuth2Sessions` filter selected no tests and is not counted as integration proof.
The production-builder smoke drove a local upstream callback, accepted refresh, rejected refresh, and termination, then observed:

```text
production_builder_session_facts=4 upstream_refresh_rejection_recorded=true session_reference_preserved=true
```

Its provider fixture needed the required scope description before runtime proof. The temporary source was removed afterward.
The scoped quality report was reviewed, not declared clean.
It reports added termination/refresh orchestration lines for commit boundaries and terminal failure recording.
It reports the constructor's recorder parameter and cumulative typed PostgreSQL read clones.
Existing structured logging remains explicit rather than a generic logging wrapper.

### Accepted authorization request recording (T052)

The authorization service records `authorization-requested` after client/mode/redirect/scope validation and before grant checks or consent/proceed output.
Invalid redirects do not become accepted requests.
Recording failure produces the existing server-error decision. It releases no consent session token.

A new boundary regression compiled and failed for zero accepted-request facts before implementation.
The builder and all discovered constructor callers supply the required recorder.
The implementation adds no authorization-code persistence hook or second dispatcher recorder.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2 ./internal/app ./internal/adapters/http/enduser \
  -skip '^(TestProxyLedger|TestTokenGrantProxy)'
go test -count=1 ./tests/integration -run 'OAuth2(Authorize|Authorization|Metadata)'
go run ./.ledger-foundation-smoke
```

Static checks and all matching domain/builder/HTTP integration checks passed. The unchanged pending proxy outcome tests remain assigned to T054.
The production-builder smoke observed:

```text
production_builder_authorization_requested_once=true invalid_redirect_not_recorded=true consent_response_preserved=true
```

The temporary source was removed after proof.

The scoped quality report had no preexisting major regressions. It reports small authorization/constructor growth.
It reports the explicit authorization recipe as a clone shape of another typed event recipe.
These distinct fact owners remain separate without a generic event-construction layer.

### Local token issuance ownership (T053)

The local provider requires the recorder and shared storage transaction manager.
Client-credential, authorization-code, and refresh responses release only after their owning transaction commits response-population mutations and `token-issued`.
Terminal failures record independently after rollback. Code-replay and refresh-reuse revocations keep their independent failure-side boundaries.
Existing Fosite storage transaction methods already join the foundation's owner. They needed no second transaction mechanism.

The focused local-outcome test compiled and failed for zero `token-issued` facts before implementation.
An earlier constructor-only check failed compilation for an unused import. It is not semantic-red evidence.

The additional real-memory rollback regression first failed because Fosite consumes PKCE during request validation, before response population.
The repaired owner includes PKCE validation after replay detection.
Validation rejection still commits its one-shot challenge consumption without a token.
Later issuance/append failure restores the challenge, authorization code, and refresh state.
The regression proves retry usability, exact independently committed failure counts, and preserved one-shot rejection.
The pinned producer contract records this newly inspected library boundary.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2server -run '^TestProvider'
go test -count=1 ./internal/app ./internal/adapters/http/enduser \
  -skip '^(TestProxyLedger|TestTokenGrantProxy)'
go run ./.ledger-foundation-smoke
```

Static checks, all provider tests, and the affected builder/HTTP checks passed. The unchanged staged-proxy tests remain assigned to T054.
The production builder's actual credential-generation and token HTTP handlers observed:

```text
production_builder_local_token_http=200 invalid_client_http=401 token_issued=1 token_request_failed=1 end_user_subject_null=true
```

The temporary source was removed afterward.

Scoped quality review reports no preexisting major regressions.
It retains constructor arity/size growth, added authorization-code ownership orchestration, and the new real-rollback test's size.
The explicit rejection-versus-issuance boundary remains visible rather than hidden behind a configurable generic transaction helper.

### Staged proxy and token parse outcomes (T054)

The domain outcome service stages every upstream response. The transport port no longer offers `StreamBody`.
After recording, the proxy/hybrid strategy forwards only the completed body's permitted headers, status, and bytes.
Verification/read/transport refusals produce one terminal failure.
Recording failures withhold credential bytes with the existing server-error response form.
The service decodes no upstream JWT claims for subject attribution.

Typed parse failures complete through the domain service before their HTTP error response.
In hybrid mode, local parsing and proxy completion share the same builder-owned outcome service.
The provider still records actual local minting once. Missing identity stays null.
Existing local issuance logs and proxy verification/write-failure logs retain their names, levels, messages, and fields.

The new HTTP/local parse-boundary regression compiled and failed for zero recorded failures before the adapter changes.
Its green checks include a commit callback that proves no error response bytes/status escape before commit.
They include an append failure that produces a 500 without a second event claim.

The pre-existing proxy regression's empty-data assertion conflicted with the published failure schema's required `reason_code`.
That assertion was removed. Its response, type, count, and subject assertions remain.
Obsolete streaming mock methods/flags and their tautological assertions were removed with the port operation.
No source-text checks replace them.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2 ./internal/adapters/http/enduser ./internal/app
go test -count=1 ./tests/integration -run '^TestOAuth2Token'
go run ./.ledger-foundation-smoke
```

Static checks and all matching packages/integration tests passed, including the previously pending proxy/hybrid staged-response tests.
The production-builder smoke used an actual local upstream HTTP server and the token handler.
It verified accepted/refused status/body preservation, permitted headers, excluded cookies, and malformed requests.
It also verified recording-failure response withholding and stored credential canaries:

```text
production_builder_proxy_http_status_and_body_preserved=true recording_failure_http=500 uncommitted_token_withheld=true issued=1 failed=2 credential_canaries_absent=true
```

The temporary source was removed afterward.

Scoped quality review is not clean. It reports 20 major gating clone/clone-of-helper findings for constructor shapes shared with unrelated constructors/matchers.
It also reports minor control-flow growth and the new HTTP error helper's parameter count.
These are explicit local constructor/fact recipes and transport-response arguments, not duplicated business behavior.
The implementation adds no unrelated constructor rewrite or generic event/DI framework to remove the normalized shape matches.

### Exchange success and primary refusal recording (T055)

The exchange service requires the recorder. It constructs facts from identities/references as the workflow establishes them, not request or audit struct serialization.
It retains the verified initiating client separately from the subject.
It sets `on_behalf_of` only after the active user grant establishes delegation.
A successful `token-exchanged` requires receiving-agent attribution through the closed schema. Recording completes before the response escapes.

Authentication/authorization refusals produce one `token-exchange-denied`. Parse, transport, and internal failures remain `token-request-failed`.
The service does not emit `token-issued` for exchange.
Terminal recording runs after the existing workflow scopes return.
As a result, an exchange-wide rollback does not enclose accepted refresh and expiry-recognition commits.

The specific-refusal regression compiled and failed for zero occurrences in both authentication and authorization paths before implementation.
The full exchange, builder, and token-handler packages pass.
An additional deterministic CEL evaluation-error test retains the generic internal-failure fact.
It claims no permission denial or established delegation.

```bash
just fmt
just check
go test -count=1 ./internal/domain/tokenexchange ./internal/app ./internal/adapters/http/enduser
go run ./.ledger-foundation-smoke
```

The production-builder smoke used a local JWKS/discovery server and library-signed subject/client JWTs.
Its encrypted-session fixture used the same configured test key/AAD.
It exercised the real token HTTP handler for success, invalid client assertion, and missing delegation.
Then it queried subject/no-subject records:

```text
production_builder_exchange_http=200 authentication_denial_http=401 delegation_denial_http=403 token_exchanged=1 token_exchange_denied=2 receiving_agent_and_grant_session_service_preserved=true verified_caller_distinct_from_subject=true
```

The first smoke request correctly failed audience validation because its fixture used the public URL instead of the exchange audience.
The fixture correction used an explicit configured audience. Signature/claim validation was not weakened.
Its temporary source was removed after proof.

Scoped quality review initially gated on added orchestration complexity.
Reuse of the same typed refusal predicate at its three call sites reduced that finding to minor growth.
The remaining major finding compares the predicate's normalized `errors.As`/code-check shape with an unrelated rate-limit predicate.
Its distinct domain meaning remains without a cross-domain generic error classifier.
Constructor arity and added fact/reference orchestration remain explicit measured tradeoffs.

### Impersonation permission and minting facts (T056)

Rule evaluation selects the authorized mint input without minting.
The selected rule's active delegation is required before `impersonation-granted` commits. Minting starts afterward.
Successful output adds `token-exchanged` before response release.
A mint failure retains the committed granted decision and adds only `token-request-failed`.
Evaluation/storage/internal errors never become false permission denials.

Final policy/credential/delegation refusals produce one permission denial plus their primary exchange denial.
These facts share a non-mutating owner transaction. The service does not produce one pair per candidate rule.
Known target lookup rejection is a permission refusal plus the malformed-target primary failure.
Syntax-only activation failures and malformed impersonation forms record generic request failures.
Existing HTTP forms and audit serialization remain unchanged.

Ledger identities are separate from audit fields. A signed subject is attributable after verified extraction.
ADR 031's unverified subject is not attributable until the subject-bound policy and active delegation establish it.
The verified client assertion identity, verified actor-token identity, affected subject, and established `on_behalf_of` remain distinct.

The pre-mint regression compiled. Before implementation, it failed because minting started with zero permission facts.
The full impersonation, builder, and token-handler packages pass.
They cover granted-before-mint, retained grant after mint failure, one final candidate denial, and internal-error classification.
They cover recording failure that prevents minting and existing audit/credential-role/unsigned-subject/delegation behavior.

```bash
just fmt
just check
go test -count=1 ./internal/domain/impersonation ./internal/app ./internal/adapters/http/enduser
go run ./.ledger-foundation-smoke
```

The production-builder smoke used a real local JWKS receiver and three separately identified signed credential roles.
It drove the token HTTP handler and verified the minted output against the actual broker JWKS handler:

```text
production_builder_impersonation_http=200 signed_output_verified=true missing_delegation_http=403 granted_then_exchanged=true denied_then_exchange_denied=true verified_client_actor_and_subject_distinct=true
```

A fixture call used `ServeHTTP` instead of the JWKS handler's actual `ServeJWKS` method.
It failed compilation before correction and is not runtime proof.
The temporary source was removed after the successful run.

Scoped quality review initially flagged orchestration growth.
Isolation of the authorized permission/minting boundary removed that major finding.
The mandatory recorder constructor check and activated target-failure recording still cross complexity thresholds.
Typed fact-construction clone shape and constructor arity remain recorded tradeoffs.
These results do not claim a clean quality report.

### Approval winning transitions and post-commit signals (T057)

The approval service requires the recorder. Its five mutation APIs acquire the owning transaction and subject gate before business reads.
They reuse the existing CAS/idempotent repository results. They append only winning `approval-requested`, `approval-approved`, `approval-denied`, `approval-consumed`, or `approval-revoked` facts.
Sync-state updates share that owner.

Existing pending returns, CAS losers, and already-consumed repeats are silent. Revocation is not an extra denial.
Notifications, pending metrics, and existing success logs run through transaction after-commit effects.
Failed append/commit cannot expose a successful mutation response or success signal.

The notification regression's fixture uses the same real memory manager for approval, sync, event, and owning transaction state.
It does not use a separate fixture transaction stack.
The winning-transition regression compiled and failed for zero facts before implementation.
Transition/no-op/CAS-loser/state+sync rollback and owning-notification tests pass.
All approval tests pass except the still-pending T058 lazy-expiry regression.
The affected builder/approval-handler/token-handler packages pass.

```bash
just fmt
just check
go test -count=1 ./internal/domain/approval -skip '^TestApprovalLedgerLazyExpiry'
go test -count=1 ./internal/app ./internal/adapters/http/handlers/approval ./internal/adapters/http/enduser
go run ./.ledger-foundation-smoke
```

The production-builder smoke exercised all five transitions, pending deduplication, and consumed repeats.
Then it rejected an append and observed restored pending state/sync version and no subscriber notification:

```text
production_builder_approval_facts=9 all_five_transition_types=true pending_dedup_and_consumed_repeat_silent=true failed_append_restores_pending_and_sync_version=true rollback_notification_absent=true
```

The temporary source was removed after proof.

Scoped quality review identified duplicated new transaction-result wrappers.
One private helper handles the five real callers and their two concrete result types.
Their matched wrapper tokens decreased from roughly 94–96 to 42–44.
The report still has ten major normalized wrapper/effect-access clone findings.
It also classifies inherited bodies as new after internal extraction.

This is not a clean quality claim. The remaining thin typed calls and context-effect checks remain.
No unrelated domain/error helper was merged to remove the shape matches.

### Once-only approval expiry and guarded dedup retirement (T058)

Approval reads, expired mutation races, and existing filtered/create paths recognize expired pending candidates through the separate ISP facet.
Recognition verifies the owning record again under the subject gate. It commits the effective-expiry marker with `approval-expired`.
The event uses the effective expiry time and the `broker-lifecycle` system actor.
Expired mutation recognition runs after the failed mutation owner rolls back. The rejected decision does not discard recognition.

A marker failure rolls back and remains retryable.
Both adapters guard pending retirement. They return an expired duplicate without the matching marker rather than consume it.
The domain records that duplicate and runs creation again in the same owner.
Then it records the genuinely new request.

For this retirement predicate, PostgreSQL uses database execution time rather than the owner's earlier transaction-start time.
Existing consumed/persistence/status rules remain. There is no expired enum or new expiry scheduler.

The lazy-expiry regression compiled and failed for zero facts before implementation.
The memory retirement guard regression also failed before adapter changes because the repository prematurely returned a new approval.
The original post-recognition replacement/consumed assertions remain. The PostgreSQL equivalent protects the same gate.
Additional tests cover marker append rollback, recognition after expired approve/deny rollback, and a deterministic TTL crossing between discovery and creation.

```bash
just fmt
just check
go test -count=1 ./internal/domain/approval ./internal/app ./internal/adapters/http/handlers/approval ./internal/adapters/http/enduser
go test -count=1 ./internal/adapters/storage/memory -run '^TestToolApproval'
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestToolApprovalRepository'
go run ./.ledger-foundation-smoke
```

Static checks, complete affected domain/handler/builder tests, memory approval tests, and the tagged PostgreSQL repository checks passed.
The production-builder smoke expired a real created approval. It ran reads again and a rejected mutation, then created a replacement.
It queried the retained facts/candidate marker state:

```text
production_builder_expiry_once=true effective_expiry_time_preserved=true expired_read_and_mutation_gone=true retired_pending_consumed_without_new_status=true replacement_requested_committed=true old_expiry_marker_survives_retirement=true
```

An unused temporary import initially prevented compilation. Its removal preceded runtime proof.
The temporary source was removed afterward.

Scoped quality remains non-clean. It reports cumulative memory transaction-guard/constructor/typed-reader clone shapes and approval owner/effect-access shapes.
It reports distinct grant/approval marker recipes and new candidate/retirement orchestration.
No generic cross-domain expiry/event framework or unrelated adapter rewrite was introduced to remove those shapes.

### Atomic agent lifecycle and effective-state comparisons (T059)

Agent creation, changed configuration, and actual deletion commit credential-free facts with the business mutation.
Equivalent collection ordering and absent deletion are silent.
PostgreSQL agent reads inside an owner acquire `FOR UPDATE`.
Competing identical updates compare the committed winner rather than append duplicate facts from stale reads.

The lifecycle and rollback tests pass.
A collection regression first failed because the comparison treated duplicate `read` scopes as equal to `read,write`.
Both-direction membership checks preserve that change and keep reordered scopes silent.
A real PostgreSQL competing-update regression first retained two events.
It retains exactly one after the waiting owner verifies locked state again.

```bash
just check
go test -count=1 ./internal/domain/agents ./internal/adapters/http/handlers/admin ./internal/app ./tests/integration \
  -run 'Agent|BuilderMinimal' -skip '^TestAgentLedgerDeletionCapturesCompleteActualCascade$'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestAgentLedgerConcurrentIdenticalUpdateRecordsOnlyWinner$'
go run ./.ledger-foundation-smoke
```

Static checks and these targeted packages passed. The excluded complete-cascade regression remains red and belongs to T062.
It does not count as a passed agent acceptance journey.
The production-builder smoke drove real HTTP create, unchanged update, changed update, delete, and repeated delete.
Then it queried the retained envelopes:

```text
production_builder_agent_http=201,200,200,204,204 agent_facts=3 unchanged_update_and_absent_delete_silent=true credential_configuration_excluded=true
```

The initial smoke lacked the builder's required storage adapter. The correction preceded runtime proof.
Its temporary source was removed afterward.

The scoped quality report is not clean. It reports 19 major cumulative clone findings involving dependent-list and fixture shapes.
No unrelated fixture rewrite or generic transaction/event abstraction was introduced to hide those findings.

### Atomic credential lifecycle and secret-release boundary (T060)

Credential generation prepares cryptographic material before the write owner. Then it locks and verifies the owning agent and credential again.

The record mutation and safe-ID fact share the transaction. Replacement emits only `credential-rotated`.
Removal captures the actual credential ID and preserves the existing absent-credential error.

Secret/hash/configuration values never enter the envelope. Plaintext responses remain withheld until commit succeeds.

The focused tests initially failed for missing facts, missing rollback errors, and secret release before an owner commit. They pass.
The repeated-revoke expectation changed from success to `ErrNotFound` to match both real adapters and the existing HTTP 404 contract.
The exact three-fact assertion remains unchanged.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2server -skip '^TestSigningLedger'
go test -count=1 ./internal/app ./internal/adapters/http/handlers/admin
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestClientCredential'
go run ./.ledger-foundation-smoke
```

Static checks, affected domain/builder/handler packages, and tagged PostgreSQL credential tests passed.
Signing-selection regressions remain assigned to T061.
The earlier memory `Credential` filter selected no tests and does not count as adapter proof.
An initial static run rejected unformatted code. The successful run followed formatting and smoke removal.

The production-builder smoke drove real HTTP generation, rotation, forced append rejection, revocation, and repeated revocation.
After the rejected rotation, it observed the restored prior ID/hash and no secret in the 500 response.
It observed exactly the three safe credential facts:

```text
production_builder_credential_http=201,200,500,204,404 credential_facts=3 replacement_rotation_only=true failed_append_preserves_old_credential_and_withholds_secret=true plaintext_and_hash_excluded=true
```

The temporary source was removed after proof.

Scoped quality remains non-clean. Eight major normalized constructor/owner-shape clone findings remain, including the mandatory credential-recorder constructor.
No generic event/transaction framework or unrelated constructor migration was added to remove those shape findings.

### Atomic signing selection and bootstrap ownership (T061)

Selection of a newly generated current key joins the owning storage transaction.
It records the safe `signing_key_id` and UTC eligibility instant.
Explicit promotion uses shared serializable transaction protection to compare the target's prior current flag with the selected result.
Re-selection retains immediate activation behavior but emits no additional fact.
`occurred_at` is selection time, separate from the future `data.activates_at`.
Bootstrap retains the existing lock-before-lifecycle order and recovery probes.

The signing regressions initially failed for zero selections and missing rollback behavior.
Bootstrap, changed/repeated selection, future eligibility, append/commit rollback, and competing bootstrap tests pass.
Unit assertions changed to the approved `signing_key_id` schema rather than `kid`.
The implementation-only no-target-read subtest and its helper were removed.
Activation-boundary assertions inspect returned metadata directly rather than a repository forwarding spy.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2server ./internal/app \
  ./internal/adapters/http/handlers/admin ./internal/adapters/http/enduser
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestSigningKey'
go run ./.ledger-foundation-smoke
```

Static checks, the complete affected packages, and tagged PostgreSQL signing repository checks passed.
The production-builder HTTP smoke built twice over the same storage.
It selected a generated current key and observed the existing grace fallback. It promoted and re-selected an older key.
During forced append failure, it rejected both promotion and current-key generation:

```text
production_builder_signing_selections=3 bootstrap_restart_silent=true generated_current_grace_preserved=true same_key_promotion_silent=true failed_promotion_and_generation_rollback=true selection_time_distinct_from_eligibility=true
```

The temporary source was removed after proof.

Scoped quality remains non-clean. Six major normalized constructor/fact-owner clone findings remain.
The serializable selection boundary and separate bootstrap owner remain.
The implementation adds no second selection/lock API or generic cross-domain orchestration framework.

### Actual dependent terminal facts and subject gates (T062)

Agent deletion discovers grant/approval subjects before the owner acquires its sorted gates.
The owner locks the parent and dependent rows. It verifies discovery again before mutation.
A newly discovered subject fails closed without deletion.

The owner captures credential/permission references and removes business rows.
It records only actual agent/grant/permanent-approved-approval/credential terminal facts.
It removes an already denied approval without another denial or revocation.

The memory factory connects those child stores to the agent repository.
Its cascade owns or joins a transaction, including standalone repository deletion. Cancellation/rollback cannot leave a partial cascade.

The existing T044 complete-cascade regression went from one fact to the required five.
A new memory adapter regression first failed because grants survived parent deletion. It verifies child removal and rollback restoration.
The discovery-boundary regression fails without the discovery check and passes with the guard restored.

Provider session behavior is unchanged: migration 004 uses `ON DELETE RESTRICT`, not a session cascade.
The production PostgreSQL smoke verifies rejection, retained provider/session rows, and no synthetic `session-terminated` event.
A change to that destructive HTTP behavior requires separate approval.
The catalogue's conditional cascade guidance does not imply that approval.

```bash
just fmt
just check
go test -count=1 ./internal/domain/agents ./internal/domain/thirdparty ./internal/app \
  ./internal/adapters/http/handlers/admin ./tests/integration \
  -run 'Agent|Builder|ThirdpartyOAuth2ProviderService_Delete'
go test -race -count=1 ./internal/domain/agents
go test -race -count=1 ./internal/adapters/storage/memory -run '^TestMemoryAgent|^TestMemoryTransaction'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestAgentLedgerConcurrentIdenticalUpdateRecordsOnlyWinner$'
go test -tags=integration -count=1 -v ./tests/integration/storage/infra -run '^TestAgentCascadeRuntimeSmoke$'
```

Static checks, affected packages, domain/memory race checks and the existing PostgreSQL race regression passed.
The temporary production-builder PostgreSQL HTTP smoke retained exactly the five terminal facts across two subjects.
It verified removal of the physical child rows and the existing provider restriction:

```text
production_builder_postgres_agent_cascade_http=204 terminal_facts=5 denied_approval_not_reclassified=true safe_references_retained=true all_dependents_removed=true provider_session_restriction_preserved=true no_synthetic_session_termination=true
```

The first runs lacked the required logger and included-service grant reference. Both fixture corrections preceded runtime proof.
The source was removed afterward.

Adapter-backed empty cascade fixtures live in `tests/unit/cascadefixture`, separate from the infrastructure-free ledger fixture.
An initial shared-fixture import produced domain test cycles. The correction removed those cycles.

Scoped quality improved after separation of parent transaction ownership, discovery checks, and child fact rules.
There are still 45 cumulative major clone findings. These results do not claim a clean report.

### US1 acceptance gate dependency findings (T063, open)

After T059–T062, the existing US1 acceptance scenarios were run without expectation changes:

```bash
just web-build
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='committed broker facts' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='committed broker facts|committed history across process boundaries' ./tests/e2e/
```

The frontend prerequisite passed. Memory ran five selected US1 scenarios: zero passed, five failed.
The tagged lane ran seven selected scenarios: zero passed, seven failed.
These are failed acceptance results, not completion evidence. T063 remains unchecked.

Observed failures included:

- Missing exchange gateway attribution
- A rollback scenario that waited for an actual ledger OTLP copy before it armed its storage fault
- Refusal actor/gateway mismatches
- An absent-grant GET that returned the existing HTTP 200 while the scenario expected 404
- An expiry query that found no target fact
- PostgreSQL child startup failure
- No target fact in the raw parity journey.

The declared focus excluded non-selected scenarios. Their source remained unchanged and did not skip them.

The rollback scenario at `business_event_ledger_test.go:659` requires successful ledger dispatch.
T088–T092 explicitly own its implementation, and T093 owns closure. But the binding task graph requires T063 before those tasks.
T066–T067 also own trusted-context integration after this gate.
This task-order dependency conflict does not permit removal of telemetry/provenance assertions or a passed gate status.
The grant GET assumption also conflicts with OpenAPI's existing HTTP 200 empty-array contract.

That assumption requires correction, not a production status change.

The two Ginkgo commands ran concurrently.
**[INFERENCE]** The first completed command can remove the shared test-binary path and cause the child-start failure.
Future child/crash verification must run in an isolated binary location or sequentially.
This run claims no child/crash pass.

### Trusted-context and offline-schema regression authoring (T064–T065)

After the user approved the dependency-order correction, the disjoint context and builder-schema test slices were authored.
The shared acceptance criteria remained unchanged. Both slices compile and expose missing implementation:

```bash
go test -count=1 ./internal/domain/ledger ./internal/app \
  -run '^TestContextFacts|^TestBuilderBusinessEventSchemas'
```

The context tests fail on missing initiating caller/user/admin attribution and missing authoritative trace/span enrichment.
They also protect domain-established subjects/delegation, explicit gateway association, nullable unavailable identity, allowlisted client metadata, and untrusted-session exclusion.
The temporary identity declaration in `ledger/context.go` exists only to compile this red gate.
T066 must replace that declaration.

The schema persistence tests fail at `Record`, not compilation or a placeholder assertion.
The builder registry accepts the added offline type, but the existing storage registry knows only the embedded catalogue.
Invalid-source startup tests cover redefinition, namespace/source changes, remote references and flattened collisions.
Unknown/invalid payload tests retain an unchanged persisted control fact.
T068 must integrate one compiled registry across domain and storage validation paths without replacement of existing event history.

### Trusted event context and existing HTTP capture reuse (T066)

`ledger.ContextFacts` preserves domain-established subject, delegation, caller category and gateway association.
It enriches the initiating caller from verified context. It never substitutes the represented SecurityContext actor for an initiating agent.
Unavailable anonymous identities stay null.

Request trace and span come only from authoritative capture with a valid matching OTel trace.
The constructor normalizes client IP. It converts user agents to fixed family values, never raw request text.
The model's existing trusted-correlation mechanism protects session-string provenance.

The existing `internal/adapters/http/middleware/security_context.go` captures authoritative IP/request trace and supports deferred identity finalization.
The implementation reuses this capture. It adds no competing `business_event_context.go` capture path and changes no existing slog fields.
The real constructor replaces the compiling identity declaration from T064.

```bash
just fmt
just check
go test -count=1 ./internal/domain/ledger ./internal/adapters/http/middleware
go run ./.ledger-foundation-smoke
```

Static checks and both complete packages passed. A real HTTP server using the existing capture middleware exercised the constructor and published-schema validation:

```text
actual_http_context_operator_distinct_from_null_subject=true authoritative_client_ip_and_request_trace=true raw_user_agent_and_query_credentials_excluded=true existing_capture_reused=true
```

The source was removed after proof. The scoped quality command returned success but did not establish the absence of new-symbol debt.
Producer integration and its larger quality review remain T067.

The separately approved absent-grant acceptance correction preserves HTTP 200. It verifies that the revoked delegation is absent.
A lint-requested tagged switch replaced the conditional chain without changed assertions.

### Trusted context across the full catalogue (T067)

The event constructor requires its owning context and applies `ContextFacts` once for every typed recipe.
All discovered callers were migrated, including tests. The old two-argument constructor is removed.
Verified exchange/impersonation callers and gateway references remain separate from subjects.
Delegation enters a fact only after the owning domain establishes it.
Approval requests retain their verified gateway/represented-user relationship.

Initial signing-key bootstrap uses the fixed lifecycle-system actor. Authenticated promotions retain their operator identity.
The complete 28-type memory HTTP catalogue journey passes. Its credential-canary checks and exact action deltas remain.

Corrected fixture assumptions preserve the reviewed contracts. A missing-grant exchange refusal has no `on_behalf_of`.
An impersonation refusal includes its primary exchange-refusal companion.
Signing eligibility comparisons use the precise stored instant, not the HTTP response's second-only formatting.
No HTTP representation changed to satisfy those assertions.

```bash
just fmt
just check
go test -count=1 ./internal/domain/ledger ./internal/domain/agents ./internal/domain/approval \
  ./internal/domain/consent ./internal/domain/impersonation ./internal/domain/oauth2 \
  ./internal/domain/oauth2server ./internal/domain/oauth2session ./internal/domain/tokenexchange \
  ./internal/adapters/http/enduser ./internal/adapters/http/handlers/admin
go test -count=1 ./internal/app -skip '^TestBuilderBusinessEventSchemas'
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='records each catalogue occurrence once' ./tests/e2e/
```

Static checks and all eleven matched producer/handler packages passed.
Builder tests passed with the unchanged T068 schema-seam regressions excluded.
The focused catalogue run passed one acceptance scenario for all 28 workflows.
T063 is still open. These results do not claim a passed rollback/expiry/parity/crash/delivery gate.

An earlier concurrent package run failed the existing same-identity security-finalization assertion.
A bounded diagnostic exercised that path and observed the expected access denial with finalized context.
The final complete package run passed. No timeout, validation guard, or failing expectation was weakened.
The diagnostic source was removed.

Scoped quality remains non-clean with sixteen major cumulative constructor/flow-shape findings.
No unrelated policy/constructor framework was introduced.

### Offline schema registration through one compiled validator (T068)

The builder supplies its compiled registry to both storage validation paths through the storage composition facet.
Atomic validator replacement retains existing events.
The real builder extension, unsafe-source, and unknown/invalid-payload tests pass.
The HTTP fixture schema declares its fixed reasons.
The pre-storage validation fixture uses a prepared copy and preserves the recorded-time invariant.

`just check` passed. Complete app, ledger, and untagged PostgreSQL package tests passed.
Memory BusinessEvent tests passed with the still-pending delivery regressions excluded.
The initial memory filter selected a T090 reference-list test and failed.
The final focused run passed with that pending case excluded.

The temporary production-builder smoke reported `production_builder_offline_type_retained=true shared_compiled_validation=true unknown_type_rejected=true no_schema_migration=true`.
Its source was removed.
Scoped quality retains seven cumulative major findings, including interface-dispatch reachability warnings.
These results do not claim clean quality.

### Authorized investigation guide (T069)

`docs/api/business-event-ledger.md` publishes exact-subject and explicit no-subject SQL, receiving-agent-set interpretation, reader access, closed schemas, and credential-free examples. Documentation links resolve. Both OpenAPI documents remain unchanged. This feature adds no public ledger endpoint.

### Stored investigation verification and remaining delivery gates (T070, open)

The memory investigation run selected all five US2 scenarios. US2-AS1, US2-AS2, and US2-AS4 passed. US2-AS3 and US2-AS5 failed at their OTLP waits. T070 remains unchecked.

The request-context fix captures the actual HTTP server span before domain child spans start. The event retains that span only with its matching authoritative trace. The forged-token fixture expects the specific authentication denial and forbids a generic companion failure. It retains null subject, caller, delegation, and receiving-agent assertions.

PostgreSQL then exposed a grant-expiry conversion error.
The timezone-free `valid_until` column received local wall time instead of the expiry instant.
The adapter converts create, upsert, and update parameters to UTC in SQL.
A regression failed before this change for positive and negative timezone offsets.
It passes after the change and preserves indefinite grants.

```bash
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='safe operational investigation' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' \
  --focus='returns exactly the receiving agents|retains verified initiating caller|uses explicit null identities' ./tests/e2e/
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventGrantExpiry|^TestBusinessEventExpirationMarkers'
just fmt
just check
```

The first tagged investigation run failed the expiry assertion. After the expiry fix, the three selected acceptance scenarios passed against both backends. The PostgreSQL scenario queried actual HTTP-generated records with operational reader credentials. Its SQL result matched the two expected successful occurrences and exact receiving-agent set within `[start,end)`. Other subjects, denials, and out-of-window occurrences were excluded.

A temporary stored-only smoke exercised all 28 HTTP workflows with seven credential classes in hostile user-agent comments. It also verified offline schema registration, retained-event meaning, and unknown/invalid rejection. Rejected IDs were absent from PostgreSQL event and pending-reference tables. The combined tagged smoke and three acceptance scenarios passed: four selected specs, zero failures.

```bash
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='ledger-stored-smoke || business-event-ledger' \
  --focus='Temporary stored ledger smoke|returns exactly the receiving agents|retains verified initiating caller|uses explicit null identities' ./tests/e2e/
```

```text
stored_catalogue_backend=memory types=28 seven_credential_classes_absent=true offline_schema_retained=true unknown_invalid_rejected=true
stored_catalogue_backend=postgres types=28 seven_credential_classes_absent=true offline_schema_retained=true unknown_invalid_rejected=true
```

The temporary smoke source was removed after proof. Static checks and the real PostgreSQL expiry-marker regression group passed.
All four affected package tests passed. This evidence proves stored behavior, not delivery.

US2-AS3 and US2-AS5 remain open until the real exporter works.
The user approved deferral of both OTLP checks to T093. T070 remains unchecked during lifecycle work.

Scoped structural quality reports four cumulative major duplication findings, including the three-line context-key wrapper. Existing PostgreSQL lookup/delete shapes account for two findings. The remaining middleware finding compares existing actor extraction with an approval callback. No clean-quality claim or unrelated refactor is made.

### Lifecycle regression authoring (T071–T074)

The four independent test slices compile and fail semantically before lifecycle implementation.
Memory assertions expose no-op erasure, retention, cancellation, and collector handling.
Domain assertions expose invalid selectors that get to storage. Builder assertions expose absent startup and scheduled sweeps.
Helm assertions expose the missing CronJob.
PostgreSQL assertions expose the absent erasure function, missing drops, unchanged policy, missing barriers, and readiness gaps.

```bash
go test -count=1 ./internal/adapters/storage/memory ./internal/adapters/storage/postgres \
  ./internal/domain/ledger ./internal/app ./tests/integration \
  -run '^(TestMemoryBusinessEvent(Erasure|Retention|SubMicrosecond|Lifecycle|Maintenance|InvalidPolicy|Collector|DeletedReference)|TestBusinessEventLifecycle_|TestLifecycleService|TestBuilderRetention|TestMemoryRetentionWorker|TestBuilderShutdown.*(Retention|Sweep)|TestHelmTemplate_BusinessEvent)'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventLifecycle_'
```

Both commands failed for the expected missing behavior, not compilation or placeholder assertions. The temporary compiling lifecycle methods are replaced by T075. The memory dispatch declarations remain part of T090's still-open implementation. Their collector, claim, retry, and deleted-reference tests remain red until real dispatch exists.

### Core lifecycle, startup maintenance, and readiness (T075–T079)

Both storage backends implement exact-subject erasure and committed counts. Event/reference deletion preserves unrelated subjects, null subjects, and business expiry markers. Later legitimate occurrences remain possible. Memory retention uses one clock reading and the same six-hour eligibility boundary as PostgreSQL.

The operational SQL eraser uses fixed qualified names, fresh VOLATILE reads, and lifecycle/subject locks before delivery-row locks.
Its UTF-8 FNV-1a key matches the broker's existing subject gate. PUBLIC has no execution permission.
Maintenance validates policy and paired metadata before transactional drops.
Provisioning remains drop-free and repairs current/future windows.

Startup uses the shared upward-rounding normalization and applies policy only after valid construction.
Memory completes its startup sweep before Build returns. It runs five-minute sweeps independently of telemetry.
It cancels/joins maintenance before other shutdown callbacks.
PostgreSQL startup runs policy DML, never partition DDL.
Existing storage health requires both current partitions and recovers after scheduler repair.

```bash
just fmt
just check
go test -count=1 ./internal/app ./internal/domain/ledger ./internal/config \
  ./internal/adapters/storage/postgres
go test -count=1 ./internal/adapters/storage/memory \
  -run '^TestMemoryBusinessEvent(Erasure|Retention|SubMicrosecond|Lifecycle|Maintenance|InvalidPolicy)'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventLifecycle_' -skip '^TestBusinessEventLifecycle_OperationalPrivileges$'
go run ./.ledger-foundation-smoke
go run ./.ledger-postgres-smoke
```

Static checks, all four complete packages, focused memory lifecycle tests, and the real PostgreSQL lifecycle group passed.
This intermediate run did not select the PostgreSQL operational-privilege case. Chart privilege verification remains T081/T083.

An initial builder run exposed policy application before missing-encryption validation.
Policy application moved after construction and preserved that existing failure path.
Then the complete builder package passed.

The temporary memory program exercised real production-builder startup, sub-microsecond normalization, telemetry-independent retention, exact committed erasure, and its unaffected-subject control.
The PostgreSQL program applied real migrations and used the production builder.
It called maintenance as migration owner and erased through an authorized role.
It committed and queried through a separate reader role.

```text
production_builder_startup_retention=true positive_submicrosecond_rounding=true telemetry_independent=true exact_committed_erasure=true unaffected_subject_retained=true idempotent_erasure=true
real_postgres_production_builder=true migration_owner_retention_drop=true authorized_eraser_committed_count=1 reader_target_count=0 reader_unaffected_count=1 current_partition_readiness=true
```

These results do not close dispatch, recovery, chart scheduling, or full US3 acceptance. T083/T084 remain open.

### Scheduler, executable grants, and operations guide (T080–T082)

The PostgreSQL chart schedules five-minute migration-owned maintenance with bounded SQL and Job deadlines.
Migration completion provisions partitions even without optional grants. Memory renders neither database Job.
The maintenance template omits `spec.suspend`.
If migration Jobs are disabled, external migration and scheduler procedures are required.

Default grants enforce parent-only event insertion, restricted history mutation, and no runtime execution of ledger functions. Future partition creation removes inherited child grants. A custom script retains its agreed full-replacement behavior and must provide its own restrictions. The operations guide documents scheduler installation, roles, erasure, coordinated policy changes, alerts, external copies, and rollback acknowledgement.

```bash
go test -count=1 ./tests/integration -run '^TestHelmTemplate_BusinessEvent'
just helm-lint
just helm-template
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventLifecycle_'
go run ./.ledger-postgres-smoke
```

Rendering tests, Helm lint/template, static checks, and the complete PostgreSQL lifecycle group passed.
The real smoke ran rendered grants through `psql`.
Reader and erasure role names contained apostrophes, double quotes, and backslashes. Runtime mutation/erasure stayed denied.
New partitions removed direct-write grants inherited from deliberately broad default table privileges.

```text
real_postgres_production_builder=true migration_owner_retention_drop=true rendered_grants_executed=true quoted_operational_roles=true runtime_mutation_and_erasure_denied=true future_partition_direct_writes_denied=true authorized_eraser_committed_count=1 reader_target_count=0 reader_unaffected_count=1 current_partition_readiness=true
```

The initial expanded smoke imported an unavailable YAML module.
The correction used the repository's existing `go.yaml.in/yaml/v3`. It installed no dependency.
The successful source and temporary PostgreSQL container were removed after proof.

### US3 acceptance progress (T083/T084, open)

```bash
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='retention and exact-subject erasure' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' \
  --focus='retention and exact-subject erasure' ./tests/e2e/
```

Memory passed both selected shared scenarios.
The tagged lane passed four scenarios: exact erasure on both backends, invalid retention, physical retention boundaries, and concurrent predecessor/later-writer ordering.
US3-AS4 failed at `WaitForLedgerAttempts` because real delivery was not implemented at that time.
No timeout or assertion was weakened.
T083/T084 remain unchecked until the original delivery/restart and dispatch-dependent T072 assertions pass after T092.

SC-008 evidence remains separate by source. Tagged shared scenarios prove cross-backend erasure results.
T072 non-dispatch lifecycle tests and the complete T074 builder-worker tests prove memory retention and cancellation.
They also prove recording/erasure boundaries and expiry-marker preservation.
T090 must still close collector ordering, claims, and retry behavior.

### Synchronous delivery regression authoring (T085–T087)

All telemetry, worker, and dispatch tests compile before delivery behavior. The combined red gate exposes missing encoding/export acknowledgement, absent startup/periodic delivery, inert candidate selection, and missing claims/barriers. PostgreSQL dispatch tests fail at real candidate, synchronous callback, lock-order, acknowledgement-rollback, and retry assertions.

```bash
go test -count=1 ./internal/adapters/telemetry ./internal/app \
  ./internal/adapters/storage/postgres ./internal/adapters/storage/memory \
  -run '^(TestBusinessEvent(Exports|Distinguishes|DoesNotInherit|TraceWithout|Rejects|Overrides|Missing|Failed|Cancellation|Canceled|Delivery)|TestBuilderBusinessEvent|TestMemoryBusinessEvent(Queues|Rollback|Delivery|Collector|DeletedReference))'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventDelivery_'
```

Both commands failed semantically. T088–T092 must replace their temporary declarations, not deliver them as working behavior.
The acknowledgement-rollback test acquires the subject hint before business SQL. It exports under a joined transaction and rolls back its owner.
Then it requires a same-ID retry.

### Full-envelope encoding and actual synchronous export (T088–T089)

The non-global ledger provider uses the existing Resource construction and shared configured log-exporter factory.
Ordinary slog still uses its unchanged batch processor.
Ledger records use literal sorted attributes, explicit null/empty-object markers, typed arrays, occurred/emission timestamps, and empty body values.
The processor verifies attribute count and zero drops. It clears worker correlation and applies envelope trace/span.
It captures the actual synchronous SDK result per call.

```bash
just fmt
just check
go test -count=1 ./internal/adapters/telemetry
go run ./.ledger-telemetry-smoke
```

Static checks and the complete telemetry package passed. The real local OTLP smoke exported all 28 catalogue examples. Each Emit returned only after receiver success. Event names, IDs, required flat fields, empty body values, and zero dropped attributes were observed:

```text
real_otlp_synchronous_return=true catalogue_types_exported=28 exact_event_names_and_ids=true full_flat_envelope=true empty_body=true zero_dropped_attributes=true
```

The initial smoke incorrectly required a nil protobuf body pointer. The SDK sends a present AnyValue with no value.
The smoke verifies that empty representation, which matches the SDK test's EMPTY body contract.
No production behavior or permanent expectation changed for this observation. The smoke source was removed after proof.

End-to-end recoverable delivery remains open until repository and worker integration.

### Retained-reference dispatch and ordered workers (T090–T092)

Both backends implement payload-free due scans and synchronous barrier-held dispatch. PostgreSQL claims with SKIP LOCKED after ordered lifecycle/subject gates. Memory keeps one claim per reference, releases business visibility during export, and retains lifecycle protection. Acknowledgement requires successful export and commit. Failed attempts retain the original ID and use the fixed 30-second retry.

The builder starts bounded scans at startup and every one second.
It retries provider initialization, obeys storage/export deadlines, and preserves disablement without backfill.
Shutdown drains HTTP and cancels/joins delivery and memory maintenance.
Then it closes the ledger provider, finishes existing telemetry, and closes storage.
The obsolete inert factory contract repository is removed.

```bash
just fmt
just check
go test -count=1 ./internal/app ./internal/domain/ledger ./internal/config \
  ./cmd/agentic-identity-broker ./internal/adapters/telemetry \
  ./internal/adapters/storage/memory ./internal/adapters/storage/postgres
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventDelivery_|^TestBusinessEventLifecycle_'
```

Static checks, all seven complete packages, and both complete PostgreSQL lifecycle/delivery groups passed.
PostgreSQL candidate timestamps use UTC and preserve cross-backend key identity.
The worker acknowledgement-contention fixture waits for bootstrap export before it pauses its target attempt.
It still requires a second copy with the same ID after failed acknowledgement.

The simplicity review removed redundant cancellation branches, impossible pending rechecks, and test-only provider/retention guards. Both worker shutdown paths use closed completion channels. No blanket quality suppression or unrelated transaction framework was introduced. Structural quality remains non-clean, including worker nesting and cumulative interface-dispatch reachability findings.

### Final non-performance acceptance and cross-story closure (T063/T070/T083/T084/T093)

```bash
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' ./tests/e2e/
```

The complete memory lane passed 14 selected scenarios. The complete tagged lane passed all 20 non-performance scenarios.
Shared scenarios exercised both backends. All 28 catalogue workflows and the offline reviewed fixture exported actual OTLP records.

These behaviors passed:

- Legacy slog descriptor comparison
- Effective-copy switches
- Rollback withholding
- Concurrent winners
- Exact investigation
- Credential exclusion
- Erasure and retention
- Crash recovery
- No post-deletion replay.

Earlier acceptance runs exposed fixture assumptions. They did not permit weaker production contracts.
Signing selection is distinct from eligibility during JWKS grace.
Revoked grants retain the existing HTTP 200 empty-array response.
Raw parity targets only new action IDs and compares the complete event multiset.
Rollback assertions exclude independent committed failure copies.

Expiry timing uses the recording backend's authoritative clock.
Recovery waits for the persisted 30-second retry before its unchanged ten-second copy assertion.
A typed grant identity assertion compares values rather than a value against a pointer.

The recovery fixture uses bounded phase contexts and a separate controlled child-process lifetime. It preserves all copy, pending-reference, stable-ID, and rollback assertions. The original failed scenarios passed in focused runs, then the complete tagged lane passed. No user-visible HTTP contract or assertion timeout was widened.

### Actual command-binary broker/collector restart (T097)

```bash
just build
go run -tags=integration ./.ledger-runtime-smoke
```

The temporary program started the actual broker executable against a migrated PostgreSQL container and local OTLP receiver.
It completed an admin HTTP workflow and observed a pending reference during collector rejection.
It stopped both broker and collector, then restarted them.
The original event ID reached the receiver and its acknowledged reference disappeared.

```text
actual_broker_http_created=true collector_outage_pending=true broker_and_collector_restarted=true original_event_id_recovered=true successful_copy_acknowledged=true
```

The first request omitted required permission-set entries and returned the real HTTP 400 validation error.
The existing seeded permission fixture corrected the smoke.
The successful source and test container were removed after proof.
At that time, performance acceptance remained separate and required the approved deployment profile.

### Final security regression corrections

The temporary-type erasure regression failed before the fix with PostgreSQL SQLSTATE 23514.
The caller's `pg_temp.bytea` domain replaced the privileged function's local `bytea` type.
The function explicitly puts `pg_temp` after `pg_catalog` in its fixed search path.

Automatic-refresh success and failure regressions failed before the fix.
Delegated exchanges recorded actor kind `user` instead of the verified initiating agent.
No-context and anonymous refreshes also invented the session subject as the caller.
Token exchange carries its established actor only after grant and requested-service authorization.
Refresh keeps the session principal as subject and uses trusted initiating context.
It records established delegation and the known gateway reference.

Callback establishment and termination behavior remain unchanged.

```bash
go test -tags=integration -count=1 ./internal/domain/oauth2session \
  ./internal/domain/tokenexchange ./tests/integration/storage/infra \
  -run 'TestSessionLedgerAutomaticRefreshActor|TestExchangeLedgerDelegatedAutomaticRefreshActor|TestBusinessEventLifecycle_Erasure'
```

All three selected packages passed after both fixes. Regression controls include direct-user and peer-only contexts, successful and failed refresh, and real role-scoped PostgreSQL erasure with caller-created temporary types. These results do not establish the separate performance gate.

The temporary HTTP smoke used the production builder on both memory and PostgreSQL.
Each backend exercised successful and rejected upstream refresh during an authenticated delegated token exchange.
Stored facts and actual local OTLP copies identified `smoke-refresh-client` as actor.
They preserved the session principal as subject and retained established delegation and the gateway reference.

```bash
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --focus='Delegated automatic refresh runtime smoke' ./tests/e2e/
```

The corrected smoke passed all four backend/outcome paths.
Its initial failure expected HTTP 400 for rejected upstream refresh. The unchanged token endpoint returns HTTP 500.
No production status mapping or permanent acceptance expectation changed.
The temporary smoke source was removed after proof.

### Final compliance and regression gates

This historical checkpoint records the former commands and observed results. Removed recipes in its command blocks are not current instructions.

The final review verified unchanged OpenAPI/routing contracts, 28 published event types, and domain-to-ports dependency direction.
It found no runtime DDL, custom telemetry port, or adapter cross-import.
ADR 039 remains the accepted decision. ADR 033 remains Proposed.
Frontend, design-system, Playwright, and screenshot changes do not apply to this feature.

Examples, role-value comments, and design headers describe the implemented runtime rather than the planning phase.
The configuration reference includes both optional operational roles and the custom-script replacement behavior.
At that time, the approved deployment-profile performance gate remained open.

```bash
just fmt
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just check
go test -count=1 ./api/events/... ./internal/app ./internal/domain/ledger \
  ./internal/domain/oauth2session ./internal/domain/tokenexchange ./internal/config \
  ./cmd/agentic-identity-broker ./internal/adapters/telemetry \
  ./internal/adapters/storage/memory ./internal/adapters/storage/postgres \
  ./tests/integration/bootstrap ./tests/integration
go test -race -count=1 ./internal/adapters/storage/memory/...
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just test-integration-infra
```

Static checks passed with zero linter issues. All 12 selected package results passed. Memory race tests passed. All four infrastructure package results passed, including migration up/down/up, both repository implementations, lifecycle/delivery races, and real privilege checks.

The first static gate found two unformatted test files. Formatting corrected them.
The first full infrastructure run found a missed required retention value in `newLocalModePostgresConfig`.
The fixture supplies a valid retention policy. Production startup validation remains fail-closed.

The executable Helm grants regression passed default, colliding-role, and distinct-role cases. It runs rendered SQL through real container `psql`, including variable expansion and `\gexec`. Runtime writes to ledger history, direct partition access, and operational function execution remain denied. Parent insertion, pending-reference operations, and retention-policy updates remain permitted. CLI retention regressions reject explicit `0s` and `-1h` over valid file/environment values.

The source inventory contains 28 published examples and 21 unique scenario mappings.
Relative file links in the reviewed documentation resolve.
The review did not verify website-root routes as filesystem links.
These inventory results do not replace runtime acceptance.

The scoped structural quality check remains non-clean: 76 cumulative major findings against Git HEAD. Findings include cross-context clone matches and longer transactional workflows. No blanket suppression or unrelated abstraction was added.

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-lint
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-template
ginkgo -v --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' ./tests/e2e/
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just test-e2e-ledger-postgres
```

Helm lint and template rendering passed. After the security fixes, the memory lane passed 14 selected scenarios and the tagged lane passed 20. US3-AS1 includes historical events for the erased subject across partitions. US4-AS1 requires ordinary non-ledger OTLP telemetry alongside enabled ledger copies.

The first `just verify` attempt stopped at three gosec G115 findings.
The fixes reject out-of-range observed timestamps and compare reflected bytes without a narrowing conversion.
They explicitly map the FNV subject key to signed range.
The lock key remains compatible with the operational SQL eraser. No security-rule suppression was added.

The next verification attempt passed Go security checks but stopped at 15 npm advisories. The user approved targeted frontend/docs dependency updates. Installed versions are Axios 1.20.0, DOMPurify 3.4.16, js-yaml 5.4.1, and markdown-it 14.3.1. The js-yaml override applies only to markdownlint-cli2. Other documentation consumers retain compatible js-yaml 4.3.2.

Axios installation exposed an existing ESLint peer mismatch.
Alignment of `@eslint/js` with the installed ESLint 9.39.5 resolved it without `--force` or `--legacy-peer-deps`.
The next security stage reported zero gosec issues, no called Go vulnerabilities, and no OSV issues.

```bash
npm run build # assets/docusaurus
```

The documentation build passed after source links adopted the existing public GitHub link convention. The first build identified repository-relative links that were not public site routes. The corrected build retains the strict broken-link gate. Its existing CSS minimizer and Node localStorage warnings remain unrelated to this feature.

An inline Node smoke exercised the installed DOMPurify, js-yaml, and markdown-it packages.
DOMPurify removed executable attributes and URLs. It retained safe markup.
YAML parsing preserved explicit false, and Markdown rendering produced the expected heading.
No smoke file remains.

The full race gate found concurrent writes in the shared `ledgerfixture.Store` event slice.
Fixture commits protect that slice with a mutex.
This changes test-fixture synchronization, not production transaction behavior.

The first reference measurement stopped at an incorrect agent-reference assertion for direct-user refresh. The [performance report](performance-results.md) records the observed baseline and the corrected verifier. No paired overhead pass is claimed from that attempt.

```bash
go test -race -count=1 ./internal/domain/tokenexchange
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just verify
```

The focused race regression passed. Then the complete `just verify` gate passed.
It covered static/security scans, Go race tests, web tests, CDK tests, mock-service tests, both integration layers, and all functional E2E lanes.

| Final verification lane | Actual result |
|---|---|
| Web unit tests | 28 files, 246 tests passed |
| Backend functional E2E | 622 passed, only the performance scenario excluded |
| ExtProc E2E | 112 passed |
| Frontend browser E2E | 61 passed |
| Tagged ledger E2E | 20 passed, shared scenarios on both backends |
| Dependency/security scan | Zero gosec issues, no called Go vulnerabilities, no OSV issues |

This is local verification evidence, not a claim that GitHub CI ran.
At that time, the separately controlled performance gate and approved deployment-profile requirement remained open.

After the retained event/reference CTE and approval mutex fixes, `just verify` passed again on the final code.
The totals remained 246 web tests, 622 backend scenarios, 112 ExtProc scenarios, 61 browser scenarios, and 20 tagged ledger scenarios.
Security scans remained clear.

The user chose to stop further performance tuning and report the blocked gate.
The latest paired transaction-p99 difference is 9.371083 ms. Recording p99 is 22.370709 ms.
Both exceed 5 ms. No approved deployment profile is available.

T094, T095, T096, T107, and T109 remain unchecked.
The [performance report](performance-results.md) contains the raw-report locations, experiments, and explicit limits.

The final scoped structural quality check remains non-clean, with four cumulative major findings against Git HEAD. Findings include similar approval terminal workflows. No suppression or unrelated refactor was added. All temporary smoke/diagnostic source files were removed. Raw measurement and profiling artifacts remain as evidence.

## Prerequisites

The validation environment requires:

- Go 1.27.1, just, the repository-pinned Ginkgo CLI, and frontend build prerequisites for production bootstrap
- Docker or Podman for PostgreSQL acceptance/integration
- PostgreSQL that matches the existing deployment/test baseline
- Broker DML credentials distinct from migration/maintenance credentials
- Authorized operational query/erase access
- A local OTLP receiver configured through the existing telemetry configuration
- Reviewed/published event schemas, applied migrations, and a matching broker build
- An installed five-minute partition-maintenance schedule.

Use the shared bootstrap rather than a new container per test.
Do not use real credentials or a production telemetry destination in the test environment.

Read [configuration.md](contracts/configuration.md), [storage.md](contracts/storage.md), and [data-model.md](data-model.md) for exact configuration, SQL operations and lifecycle invariants.
Do not duplicate those contracts in test helper constants.

## 1. Static and focused validation

After you implement the relevant packages/scenarios, run these commands from the repository root:

```bash
just check
go test ./api/events/... ./internal/domain/ledger/...
go test -race ./internal/adapters/storage/memory/...
just test-e2e-backend 'business-event-ledger && !performance' ''
```

The explicit empty build-tag argument selects memory only.
The normal `just test-e2e-backend` command uses `integration` and includes PostgreSQL acceptance.

Expected results are offline schema resolution and rejection of unknown types, extra data, and invalid references.
All memory catalogue workflows pass without PostgreSQL.
Each backend iteration bootstraps its own server and storage (a fresh memory adapter or a fresh template-database clone).
It releases them with `DeferCleanup`. Iterations share no state.

During the required red phase, implemented tests must compile and fail semantically because ledger guarantees are absent.
Do not accept an empty focus result as a passing suite.

## 2. PostgreSQL, migrations and crash recovery

```bash
just test-integration-infra
just test-e2e-backend 'business-event-ledger && !performance'
```

The shared backend recipe builds frontend assets, rejects empty selections, and retains configured worker parallelism.
Normal `just verify` includes this coverage without a duplicate ledger run.
CI runs the full integration-tagged backend suite through `just verify-e2e-backend-junit`.
It retains configured parallelism and the existing JUnit/JSON reports.

Verify all 20 active scenario IDs across shared and PostgreSQL-specific tests.

Shared scenarios run against both backends. US1-AS7 compares them directly.
This supplies SC-008 evidence for event contents, query results and erasure.
PostgreSQL-only lifecycle scenarios use database-clock seeding and process restarts.
Lifecycle and retention-worker tests cover their memory equivalents.
The removal of feature-specific diagnostics does not reduce these acceptance requirements.

Verify the PostgreSQL evidence:

1. Verify that all business state-change events commit atomically. Forced validation/append failure leaves neither changed state nor its success event.
2. After you observe a committed row, kill the production-bootstrap child process before response/export. Restart against the same database. Business state and one retained event remain. The broker emits the missing ledger telemetry copy with that ID.
3. Verify that export success followed by acknowledgement failure permits a same-ID telemetry duplicate. It never permits a duplicate authoritative event.
4. Verify that competing revoke/terminate/approval/credential operations produce only actual transition events. Repeated expiry recognition remains silent, even after ledger erasure.
5. Verify migration apply/down/apply on a disposable database. The migrations preserve pre-existing business records. Down removes feature history by design. Never use a production database for this test.

Do not replace the child-process crash with a memory-adapter restart.
Keep deterministic fault control in test helpers and port wrappers, not production flags/endpoints.
When issuance fails, failure-side security revocations in OAuth2 replay handling must continue to work.

## 3. Operational investigation

Run the registered multi-principal/agent token workflows through the real end-user/admin HTTP bootstrap.
Use operational credentials and exact subject/time filters from the storage contract.
For example, run this query in an authorized psql session:

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

The expected result is the exact known receiving-agent set, with no unrelated principals, failed exchanges or credentials.
Deletion of a referenced business object does not remove or anonymize its event.

## 4. Erasure, retention and non-resurrection

Use a disposable test dataset.
Connect as the role named by `migration.grants.businessEvents.erasureRole` (or the migration owner).
In autocommit mode, run these statements:

```sql
SET default_transaction_isolation = 'read committed';
SET lock_timeout = '5s';
SET statement_timeout = '30s';
SELECT public.business_event_erase_subject('principal-example');
SELECT public.business_event_erase_subject('principal-example');
```

The first call reports the removed count. Repeated erasure returns zero.
The function rejects incompatible isolation and missing or excessive statement deadlines.
These settings must precede the function call.

Open a separate connection as the role named by
`migration.grants.businessEvents.readerRole` (or the migration owner).
Run the verification query:

```sql
SELECT count(*) FROM public.business_events WHERE subject = 'principal-example';
```

The count query returns zero. Other subjects remain unchanged.
The erasure role receives function execution, not ledger `SELECT`.

A concurrent recorder either commits before or after the erasure boundary.
Erasure deletes an occurrence that commits before the boundary.
An occurrence that commits after the boundary is a legitimate new occurrence.
Delayed delivery cannot emit deleted records. The broker role must have no access to the erasure function.

Exercise retention through the integration harness with records immediately before/after boundaries and the real maintenance function.

The database clock drives maintenance, and `recorded_at` is immutable.

Seed history as the migration owner:

1. Call `public.business_event_create_partition_pair` for the required past six-hour windows.
2. Put registry-validated envelopes in those windows with explicit historical `recorded_at`.
3. Let live workflows supply current events.

With maintenance credentials, run these statements:

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30s';
SELECT public.business_event_maintain_partitions();
```

Only whole eligible six-hour partitions and their delivery references disappear.
No partition younger than retention disappears. Current and future windows exist.
The default 90-day and changed positive durations obey the same no-early-deletion rule.
The scheduler catches up automatically after downtime.

The migration and pre-install/pre-upgrade job call `SELECT public.business_event_provision_partitions();` instead.
This function creates current and future windows and never drops partitions.

Verify a retention increase, for example `720h` to `2160h`, with seeded 30–90-day history:

1. Verify that provisioning under the stored 720h policy removes nothing.
2. With the CronJob suspended, verify that the new broker stores 2160h at startup.
3. Verify that the next maintenance run keeps the 30–90-day partitions.

Runtime credentials cannot create/drop partitions or run maintenance.
A false telemetry-copy configuration does not stop this Job or ledger writes.

Verify the rendered deployment separately:

```bash
just helm-lint
just helm-template
```

The PostgreSQL maintenance CronJob uses migration credentials and the schedule. It renders no `spec.suspend`.
The pre-upgrade migration job calls provisioning, not maintenance.
The broker Deployment does not receive those credentials.
The template preserves a false telemetry-copy configuration. A memory deployment has no PostgreSQL maintenance Job.

Keep the two replicas on one retention policy during rollout.

## 5. Credential safety and monitoring continuity

Exercise every catalogue type on applicable success/failure paths with distinct credential canaries:

- Access-token, refresh-token, and client-secret canaries
- Client-assertion and raw-JWT canaries
- Authorization-code and PKCE-verifier canaries.

Put these canaries in user-agent comments, upstream error text, URLs, tool arguments and optional opaque session strings.

The expected result is zero canary values in stored envelopes or ledger telemetry copies.
Reasons are controlled templates. Normalized user-agent output contains only an allowed family or is absent.
Domain identity provenance, not a regex, determines whether a subject is safe.

Do not limit assertions to fields named token or secret.
Compare existing slog output with the Phase 0 baseline in `tests/e2e/fixtures/business_event_ledger_legacy_slog.go`.
Verify unchanged names, levels, messages and field keys.
Verify the ledger telemetry copy's EventName and every flat envelope attribute.
Include null versus absent, empty data, arrays, and matching trace/span.

Disable only business_events.telemetry_copy_enabled.

Ledger writes and old telemetry remain. Additional copies stop.

Re-enable this configuration.

Only retained pending work resumes. Occurrences recorded while disabled do not resume.

Stop the collector.

Business commits remain durable, and pending work is recoverable.
Request processing does not wait for collector export.

## 6. Performance scope and historical evidence

On 2026-10-04 the user directed, `we dont need that gate and neither a performance profile`.
US4-AS4 and mandatory FR-008/SC-007 performance criteria are retired.
On 2026-10-06, the user also removed feature-specific diagnostic suites, runner commands, and setup.
No replacement performance suite, deployment profile, approval, benchmark run, or numeric pass is required.
The 5 ms goal remains unverified and non-blocking.
The [specification](spec.md) and [plan](plan.md) record these decisions.
Accepted ADR 039's storage/security design and the production 25-open/5-idle pool remain unchanged.

The [performance report](performance-results.md) preserves historical workloads, commands, raw-report locations, and measurement limits.
Those removed diagnostic commands are historical evidence, not runnable current instructions.
They establish neither a 5 ms pass nor a failure.
All 20 active functional scenarios remain required in the shared backend E2E and integration lanes.

## 7. Final implementation gate

Run normal `just verify`.
Retain results for all 20 active scenarios and the required evidence:

- Red/green evidence
- OTLP capture
- Erasure/crash behavior
- Migration privileges and deployment rendering.

For focused production runtime security, run `just security broker`. It verifies broker code and reachable dependency vulnerabilities, not unrelated tooling.
PostgreSQL-backed ledger acceptance runs within the normal integration-tagged backend E2E command and its existing reports.

The feature adds no standalone ledger CLI, ledger-specific CI job or step, or CI security gate.
Scheduled security scans on main remain unchanged.

`just security` retains repository-wide module and manifest scans. Normal `just verify` uses shared verification lanes, including existing Helm checks.
The feature introduces no blanket advisory ignore or dependency update.

## Historical convergence verification (2026-10-02)

### Insufficient session scopes (T110)

The new unit regression compiled and failed before the fix for empty and partially covered session scopes.
It observed `token-request-failed` instead of `token-exchange-denied`. Its fully covered control passed.

US1-AS4 includes a real HTTP exchange with an authorized grant and an established session that lacks the required scope.
The initial fixture had no required scope for that service and returned 200.
After the fixture added the required scope, the regression failed because no denial event existed.
The fix classifies the refusal at the existing scope check without changes to its HTTP error or reauthentication URI.

```bash
go test -count=1 ./internal/domain/tokenexchange
ginkgo -v --procs=1 --fail-on-empty \
  --focus='records final refusals and failures independently of mutations' ./tests/e2e/
```

Both commands passed after the fix. The HTTP scenario returned 400 with `invalid_grant` and the reauthentication URI.
It retained exactly one authorization-denial fact without a generic failure companion.
It preserved established caller/delegation/resource references and left the session unchanged.
These results cover the memory backend. Tagged-backend and final-gate results are recorded separately after each run.

At that time, the missing operator-approved deployment profile and recorded reference-performance failure remained release blockers.
This regression pass does not complete performance acceptance.


### Deployment harness and operational smoke (T111)

At that time, the implementation accepted the explicit deployment-profile input and kept one US4-AS4 acceptance scenario for both workloads.
Missing approval failed before expensive work.
The implementation rejected unsupported pool, placement, measurement, or TLS conditions rather than silently replace them.
Reports distinguished the configured copy switch from effective copying.
The harness verified runtime-role PostgreSQL configuration before it started the child binary, not only through the admin connection.

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just check
go test -tags=integration -count=1 \
  -run '^TestLedgerDeploymentProfileHelpers$' ./tests/e2e/
```

Both commands passed. Static checks reported zero linter issues.
The helper tests cover workload-floor/mix boundaries, missing or unknown conditions, actual scheduling and live-service grant invariants.
They cover host mismatch, explicit false, and unavailable database/receiver placement.
Forwarding/default-copy assertions were removed rather than treated as coverage.

A temporary overlay started the actual production CLI with a fictional, non-reference smoke fixture.
It used 8 principals, 2 agents, 4 services, and three permission sets covering 2/1/1 services.
It used 64-byte principal identifiers and a 35/25/15/20/5 action mix.
A separate fresh template validated 150 historical events.

The first smoke incorrectly seeded history into its already-used preflight database. It observed 276 rows instead of 150.
Isolation of the history template corrected that fixture error without a weaker count invariant.
The corrected smoke completed 100 live actions and verified exact event type counts, identities, permission references, and credential exclusion.
Persisted live envelopes measured 697–1168 bytes.
An actual external gRPC receiver received all 100 recording spans through the capture proxy.

A change to the benchmark runtime role's `synchronous_commit` caused startup rejection, although the admin connection still matched.
After reset, the broker started.
The same smoke ran the documented successful-exchange investigation query and exact-subject erasure.
It ran repeated erasure, which returned zero, and partition maintenance through its authorized owner connection.
The receiving-agent set was exact, erased-subject rows were absent, and the other principal's count was unchanged.

This is execution-path and operational evidence, not an operator-approved workload or performance pass.
Full paired release measurement remains T113.
Temporary smoke and profiling sources are removed after verification. Raw diagnostic profiles remain as evidence.


### Tagged refusal regression and functional gate

The first convergence `just verify` run passed static/security checks, package/race tests, and both integration layers.
It passed 246 web tests, 622 backend scenarios, 112 ExtProc scenarios, and 61 browser scenarios.
Its final tagged ledger lane passed 19 scenarios and failed US1-AS4's new full-session equality assertion.

The fixture compared its final session with the object from before the scope-setting upsert.
PostgreSQL assigns `updated_at = NOW()` on that upsert.
The test reads the persisted fixture again before the exchange.
It retains the complete no-mutation equality assertion afterward.

```bash
ginkgo --tags=integration --procs=1 --fail-on-empty \
  --focus='records final refusals and failures independently of mutations' ./tests/e2e/
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-lint helm-template
```

The corrected US1-AS4 selection passed on both backends. Helm lint and rendering passed.
The complete functional gate runs again separately after that fixture correction.
This does not relax an event, HTTP, or session-state expectation.

The source inventory retains 21 unique scenario-reference comments and one US4-AS4 `It`. The `Skip` methods in its strict infrastructure handle call `Fail`, not a skipped acceptance result. Ten local relative documentation links resolve, and the published catalogue contains 28 examples.

The final scoped structural quality delta remains non-clean with 11 cumulative major findings against Git HEAD.
Profile-specific fixture/verification branches and preflight variants account for findings.
No suppression or unrelated abstraction was added.


### Post-reversion ledger verification

The final all-repository run stopped in the standalone ExtProc agentgateway journey. MCP Initialize returned HTTP EOF at `tests/e2e/extproc/agentgateway_metadata_e2e_test.go:269`. The cause is not established. No ExtProc code or retry policy changed.

An overlapping pair of Ginkgo commands encountered three ledger subprocess launch errors. The crash harness starts the executable from `os.Executable()`. [INFERENCE] Shared package-executable creation or cleanup caused the interference. A later serial shell command returned a backend timeout without usable output.

The recovered verification used a dedicated executable with a stable path. The standalone binary uses one worker by default.

```bash
go test -c -tags=integration \
  -o /tmp/aib-ledger-retained-20261002.test ./tests/e2e/
# From tests/e2e:
/tmp/aib-ledger-retained-20261002.test \
  -test.run='TestE2E|TestLedgerDeploymentProfileHelpers' \
  -test.timeout=0 -ginkgo.fail-on-empty=true \
  -ginkgo.label-filter='business-event-ledger && !performance'
```

The dedicated executable passed all 20 ledger scenarios on the retained code. Shared scenarios covered both backends. Crash, restart, erasure, retention, recovery, credential, and legacy-log checks passed.

The isolated US4-AS4 command failed before it built workload binaries.
It reported `AIB_LEDGER_PERFORMANCE_PROFILE is required: no operator-approved deployment profile is available`.
This is an exercised release gate, not a performance pass.

Further profiling separates workflow families. In the retained profiled diagnostic, recording p99 was 31.526250 ms for exchanges and 15.525875–21.113625 ms for other families. These distributions do not identify a safe optimization by themselves. The pinned validator review found no reusable state or direct-struct shortcut. Validation remains mandatory before storage and after the database assigns recording time.


### Accepted-design decision

The user chose to preserve the pool and the accepted storage design.
The continued diagnostics found cold-backend insert costs, not an accepted application optimization.
The [performance report](performance-results.md) contains the SQL, pool, prepare, and cold/warm observations.

All diagnostic transactions preserved recording and deletion safety. The cold/warm probe rolled back every insert.
Temporary diagnostic sources were removed. Sanitized raw evidence remains under ignored `test-results/`.

At that time, performance limits, the missing approved deployment profile, and the observed final full-suite ExtProc EOF failure blocked release acceptance.

## Historical review corrections (2026-10-03)

The branch was rebased onto main `e6902cf296e617ff02e315a069d5dfb73f44bb51`. The duplicate prerequisite refactor and legacy-capture commits were dropped. The rebase preserves main's refresh coordination, verification caches, signing-key caches, and CIMD support. Ledger migration 036 follows main's CIMD migration 035.

The PostgreSQL export-deadline regression failed before the fix.
Its pending reference stayed immediately due and received no 30-second backoff.
After the fix, all `TestBusinessEventDelivery_` tests passed against PostgreSQL.
The deadline case also proved barrier release for erasure and maintenance.

The token-input, approval polling, actor, and timing regressions failed before their respective fixes. The integrated package tests passed for the modified domain, HTTP, memory, schema, application-builder, and telemetry paths. The PostgreSQL session pool-one and ledger tests passed. `just check` completed with zero linter issues.

An actual broker executable used an isolated memory configuration. `/health` returned 200. Invalid content type, an empty token form, and a missing grant type each returned the original 400 response. The broker shut down with exit code 0. Unit tests also covered an unavailable ledger at the input boundary.

The first tagged ledger run passed 19 of 20 scenarios.
The monitoring failure concerned two refresh log lines that main's #124 removed.
Those obsolete expectations were removed, not restored in production.
The second run passed monitoring but exposed a race in the crash harness.
Stdin EOF released a held post-commit response.
A deterministic EOF regression failed before the harness correction.

No durability or response-boundary assertion was weakened.
The actor rules are in `contracts/events.md`.
Published examples distinguish gateways, represented principals, and lifecycle actors.
Historical performance values remain diagnostic evidence. They do not prove either outcome of the 5 ms gate.

### Final functional verification

The refresh review found a global memory gate around provider I/O.
A real-memory service regression reproduced the blocked unrelated reads.
The corrected flow prepares tokens under per-session coordination.
Then it conditionally writes tokens and the event in a short transaction.
Real-memory isolation, atomic rollback, logout/reauthorization, and caller-cancellation tests passed.
PostgreSQL refresh isolation, atomicity, and the one-connection-pool regression also passed.

The deterministic child EOF regression passed after the crash-harness correction. The final tagged ledger executable then passed all 20 selected functional scenarios. Shared journeys covered memory and PostgreSQL. The other 646 scenarios were excluded by the label filter, not assessed in that run.

```bash
go test -c -tags=integration -o bin/ledger-review-20261003.test ./tests/e2e
# From tests/e2e:
../../bin/ledger-review-20261003.test \
  -test.run='TestE2E|TestLedgerDeploymentProfileHelpers' -test.timeout=0 \
  -ginkgo.fail-on-empty=true \
  -ginkgo.label-filter='business-event-ledger && !performance'
```

The scoped PostgreSQL ledger and expiry suite passed. Helm lint and template rendering passed. These results do not establish full-repository release acceptance.

The final structural quality delta at `0ed87362e+dirty` reported 28 major heuristic gates. These included test clones, recent edits, collector parsing, and the larger refresh-coordination method. No suppression was added. Review corrected the material isolation and timing defects. The structural tool did not report a clean gate.

### Corrected recording measurement

The isolated reference completed all three alternating pairs with 100,000 measured actions per version. Both pre-warmed 40/40 benchmark pools reported zero waits. Ledger copying stayed disabled. The feature verified every measured fact, approval preparation, warmup fact, and credential canary check.

Append p99 was 1.546375, 3.745875, and 3.918250 ms. Validation+append p99 was 1.581375, 3.851458, and 4.017625 ms. Baseline action p99 varied by 7.853042 ms. The full attributable 5 ms gate remains **not proven**, not failed or passed. The [performance report](performance-results.md#corrected-recording-run-2026-10-03) records the six raw reports and the separate timing boundaries.

The final `just check` completed with zero linter issues. No full-repository release pass is claimed. The approved deployment profile and the unnamed findings from the unavailable review document remain outside this verification.

## Historical convergence execution (2026-10-04)

This snapshot preserves the commands, infrastructure, and results that existed on 2026-10-04. The 2026-10-06 consolidation removes its feature-specific recipes and diagnostic packages. Their command blocks are historical evidence, not current instructions.

### CLI-selected configuration file (T115)

The new loader and command regressions failed before the fix.
`--config=selected.yaml` loaded the working-directory default (48h) or the environment-selected file (24h), not the selected file (12h).
Environment and CLI value overrides also exposed the wrong YAML source path.
The loader resolves the explicitly changed configuration flag before it reads YAML.
It does not mutate `IDENTITY_BROKER_CONFIG_PATH`.

```bash
go test ./internal/config ./cmd/agentic-identity-broker \
  -run 'TestBusinessEventsCLISelectsConfigurationFile|TestRootCommandSelectsBusinessEventConfigurationFile' -count=1
# Before implementation: semantic failures for wrong values/source paths.
go test ./internal/config ./cmd/agentic-identity-broker -count=1
# After implementation: both complete packages passed.
just build
```

The built broker loaded `examples/config/business-event-ledger.yaml` with `--config`.
`IDENTITY_BROKER_CONFIG_PATH` was unset, and the broker used fresh local encryption/JWE keys.
Both listener ports used unused loopback ports for isolation.
Its configuration output identified the example's absolute YAML path.
`/health` returned 200, and SIGTERM shutdown exited 0.
The smoke used no environment-path workaround or temporary source file.

Existing Git, Docker, ESLint, Prettier, and Helm ignore files cover the detected build/dependency/test artifacts.
The review found no Terraform files or npm publishing workflow. No additional ignore file was necessary.

### Acceptance inventory and diagnostic relocation (T116, before scope revision)

At that checkpoint, the shared performance runner was in `tests/e2e/ledgerperformance/scenario.go`. Its private helper tests moved with it.
`tests/e2e/business_event_ledger_performance_test.go` retained only US4-AS4.
The two unmapped diagnostics were in `tests/diagnostics/ledgerperformance/`, outside the acceptance inventory.
The relocation removed no measurement, completeness, credential, or approval check.

```bash
go test -tags=integration -run '^$' ./tests/e2e \
  ./tests/e2e/ledgerperformance ./tests/diagnostics/ledgerperformance
go test -tags=integration \
  -run '^(TestLedgerDeploymentProfileHelpers|TestLedgerPerformanceCollectorStagesAndPoolWaits)$' \
  ./tests/e2e/ledgerperformance
ginkgo -v --dry-run --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger' ./tests/e2e/
ginkgo -v --dry-run --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && performance && diagnostic' \
  ./tests/diagnostics/ledgerperformance/
```

All three packages compiled and both helper tests passed.
The acceptance dry run selected exactly 21 of 664 cases. The separate diagnostic dry run selected 2 of 2.
Dry runs prove selection, not runtime acceptance.
The relocated US4-AS4 case ran with an empty profile path.
It rejected that path before workload setup with `AIB_LEDGER_PERFORMANCE_PROFILE is required: no operator-approved deployment profile is available`.

This was an expected exercised prerequisite gate, not a performance pass.

### Complete legacy-log compatibility (T117)

The count assertion first failed for `grant-updated`.
The old selection found one matching log, but the reviewed creation/update journey requires two.
Complete equivalent sequences include initial consent creation, both session refreshes, and the local authorization-code flow.
They include a single explicit signing-key promotion, invalid-signature and verified-client/no-grant denials, and local/proxy/hybrid/hybrid-proxy issuance.
The fixture itself is unchanged.

The expansion to 32 reviewed variants exposed PostgreSQL SQLSTATE 53300.
Operational owner/reader/erasure connections accumulated until the whole scenario ended.
`LedgerHarness.Close` closes those connections with the listeners, application and runtime storage.
Then the unchanged complete scenario passed all 32 variants on both backends.
It included exact descriptor counts and full ledger telemetry envelopes:

```bash
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' \
  --focus='preserves legacy slog descriptors' ./tests/e2e/
```

### ExtProc EOF investigation

Retained artifacts did not establish the cause of the previously observed MCP Initialize EOF.
Source investigation found that TCP listener readiness is not application readiness.
It found that the current MCP client probes `server/discover` before legacy initialization.
These remain hypotheses, not diagnosed defects.

A single focused run used temporary credential-free gateway/RPC diagnostics.
It passed both existing metadata journeys with the previously recorded image digest.
The investigation introduced no retries, fail-open configuration or assertion change.
Temporary instrumentation was removed. These results claim no production ExtProc fix.

```bash
env AGENTGATEWAY_IMAGE='cr.agentgateway.dev/agentgateway:v1.5.0@sha256:bf2f339ef326d32def2aaeb44b1b4549801293c19b89e764a4228667d97d9896' \
  ginkgo run -v --procs=1 --fail-on-empty \
    --focus='Metadata Input via Agentgateway' ./tests/e2e/extproc/
```

### User-approved acceptance scope revision

After the original 21-case inventory and missing-profile gate ran, the user explicitly removed that gate and profile requirement.
US4-AS4 is retired, not reassigned or counted as passed. T095/T096/T112/T113/T118 are closed as retired.
The remaining 20 functional scenarios remain mandatory.
All atomicity, credential, privacy, lifecycle, telemetry and compatibility requirements remain mandatory.
Historical performance evidence remains without a 5 ms achievement claim.

### Operational SQL and restart smoke (T109)

A temporary smoke used PostgreSQL and the production builder in a separate broker process. Authorized reader SQL returned exactly one receiving agent for the selected principal and interval.

The smoke stopped the collector, completed an exchange, and killed the broker. A new collector and broker process recovered the original event ID. Acknowledgement removed its pending reference. Credential checks passed.

The erasure role removed three facts. Repeated erasure returned zero. The unrelated principal retained both facts. Runtime-role erasure failed as required. Migration-owner partition maintenance succeeded. The temporary smoke source was removed after execution.

```bash
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='operational-smoke' ./tests/e2e/
```

This command referred to the temporary smoke, not a retained acceptance case. The permanent acceptance inventory remains the 20 active specification scenarios.

### Optional diagnostic runtime smoke

Both optional diagnostic modes passed a reduced execution smoke. A temporary Go build overlay used these values:

- 100 principals, 10 agents and 2 services
- 1,000 historical facts
- Concurrency 4 and matched 8-open/8-idle test pools
- Two-second warmups and 1,000 actions per version
- Three alternating baseline/feature repetitions per mode.

All six feature runs verified 1,000 measured facts, 200 approval preparations and their completed warmup facts. Credential canaries were absent. All twelve reports recorded zero HTTP errors and PostgreSQL rollbacks. Recording and copy-enabled delivery modes both ran through real production binaries and PostgreSQL.

The overlay source, mapping and smoke executable were removed. Reports remain under `test-results/ledger-diagnostic-smoke-20261004/`. This smoke proves execution and correctness paths, not the full reference workload or useful allocation/latency results. At that checkpoint, the full-reference defaults and production pool remained unchanged. The former runner reported the 5 ms goal as unverified and never used it as a release gate.

`just check` passed with zero linter issues. The retained collector and reference-mix helper tests also passed.
The format recipe excludes deleted tracked Go paths from `gofmt` input.
Before that correction, removal of obsolete profile/acceptance files caused `fmt-check` to fail on missing files.

### Prior integrated verification (before scope cleanup)

An earlier `just verify` run passed before the unrelated dependency repairs and scanner exceptions were removed. That historical result is not a full-gate pass for the narrowed tree. Broad security findings remain outside this feature and are not suppressed.

The prior functional run passed 256 web tests, 655 backend scenarios, 112 ExtProc scenarios, and 63 browser scenarios.
It passed all 20 active ledger scenarios.
The ledger journeys covered all 28 facts and all 32 reviewed legacy-log variants on both backends.

Reviews removed duplicate cleanup ownership introduced by the connection-leak fix.
The harness owns its operational pools and closes partial startup allocations.
The root command tests cover an unset configuration flag.
Decoded-value and selected-path assertions remain. An incidental key-list assertion was removed.

The scoped structural report retained three heuristic dead-code gates for SDK-dispatched gRPC `Export` methods. Real exports and collector tests exercised those services. No clean structural-quality result is claimed.

The task list contains 119 feature entries, including five explicit performance retirements.
Unrelated security-repair tasks were removed rather than counted as ledger work.
Temporary scaffolds are removed. Diagnostic reports remain as evidence.
No new ADR accompanies the criteria change.

### Post-implementation hooks

The extension configuration parsed successfully. It registers one enabled optional `speckit.git.commit` post-hook and no mandatory post-hook. `/speckit-git-commit` was offered but not executed. No commit was created.

### Feature-only scope cleanup

The user excluded unrelated side topics and requested no dedicated ADR for the criteria change.
ADR 038 and its references were removed.
The specification and plan hold the scope decision. Accepted ADR 039 remains unchanged.

All Docusaurus feature-branch changes were restored to the pre-ledger main revision `e6902cf296e617ff02e315a069d5dfb73f44bb51`. A scoped Git comparison reported no Docusaurus difference. This includes the package manifest, lockfile, and original scanner configuration. The new scanner exception and split documentation scan were removed. The verification-only gRPC dependency change and its added checksums were also reverted.

After restoration, `just check` passed with zero linter issues.
Configuration, command, bootstrap, and telemetry package tests passed.
`just test-e2e-ledger-postgres` passed all 20 active scenarios on the restored dependency state.
The run included shared memory/PostgreSQL journeys and all 32 monitoring variants.

These results do not claim a fresh full `just verify` pass for the narrowed tree.
Previously observed broad security findings remain outside the feature.
They did not prompt dependency changes or scanner exceptions.
Ledger behavior and its functional acceptance contracts are unchanged.

### Historical focused security and normal backend CI

The ledger runs inside the existing broker. It has no standalone CLI.
PostgreSQL/crash acceptance requires the integration build tag, but not a separate CI job or step.
CI runs the full tagged suite through `just verify-e2e-backend-junit`.
It retains the configured backend worker count and existing reports.
The extra job, required-job dependency and separate PostgreSQL acceptance step were removed.

`actionlint` passed. No new CI security step remains. Existing scheduled scans on main are unchanged.
Local checks do not claim GitHub CI execution.

At that checkpoint, local `just security-broker` derived the first-party production import closure. It scanned those package directories with gosec.
This covered 56 packages and 256 files, including schema, credential, encryption, storage and telemetry code.
The observed scan reported zero issues.

Govulncheck analyzed the production broker entrypoint and reported zero reachable vulnerabilities.
It still showed package/module notices for vulnerabilities without a called path. No advisory ignore file suppressed them.

Documentation tooling, CDK, mocks and standalone ExtProc test infrastructure do not enter the feature security scan.
The work required no dependency upgrade, new advisory exception, or performance gate.

### Parallel backend CI verification

Local `just verify-e2e-backend-junit` passed with the integration tag and four workers.
It ran 661 of 663 specs in 178.584 seconds: 661 passed, zero failed and zero pending.
The two excluded specs carry the `performance` label.
The backend JSON report verifies that all 20 active ledger scenario IDs passed.
Ledger cases ran across all four workers.

Existing `Serial` declarations were sufficient. No test code or worker-count default changed.

This verifies the single backend-suite CI command without a ledger-specific job or step.
JUnit, JSON and execution logs remain under the existing backend report names in `test-results/`.
At that checkpoint, the focused local ledger recipe remained available. It was not a separate CI command.

### Diagnostic suite container cleanup

Commit review found that relocation omitted the original E2E suite's shared PostgreSQL teardown.
The former standalone diagnostic suite called the existing `TerminateSharedPostgres` helper from `AfterSuite`.

A temporary workload-only Go overlay exercised the actual diagnostic suite.
It replaced the benchmark with shared PostgreSQL startup.
Before the fix, the selected case passed but left one new container in operation.
That smoke-owned container was removed.

After the fix, the case and `AfterSuite` passed.
The runtime logged container termination, and no new container remained.
The suite and lifecycle hooks were not substituted. The temporary overlay was removed.
This proves cleanup, not benchmark results or a full-reference performance run.

The former diagnostics reported rollback counts but did not assert thresholds.
Functional E2E scenarios remained responsible for atomicity and rollback guarantees.
At that checkpoint, the lane table pointed to relocated optional commands rather than the retired US4-AS4 selection.

## Historical numeric convergence (2026-10-05, T120–T122)

The checklist gate passed: 16 checked requirements-quality items and no unchecked items. Checklist markers remained unchanged. Only T120–T122 were open.

### Semantic-red evidence

`go test -count=1 ./internal/domain/model ./internal/adapters/storage/postgres ./internal/adapters/storage/memory -run '^(TestBusinessEventEnvelopePreservesExactNumericPayload|TestBusinessEventRepository_DecodePreservesExactRegisteredIntegers|TestMemoryBusinessEventRetainsExactNumericPayloadAndIndependentArrays)$'` exposed scalar and array rounding in the model and PostgreSQL decoder: expected `9007199254740993`, observed `9007199254740992`. Memory retained the exact values.

`go test -tags=integration -count=1 ./tests/integration/storage/infra -run '^TestBusinessEventQuery_GetPreservesExactRegisteredIntegers$'` reproduced the same rounding through actual PostgreSQL Get and scoped Query. Raw stored JSON retained the correct integer.

The new registry tests rejected valid `json.Number` payloads and exposed acceptance of an unsigned value that OTLP cannot represent losslessly. The synchronous SDK numeric-width and HTTP OTLP tests failed on unsupported numeric inputs. The existing US2-AS5 scenario failed because the committed numeric fixture never reached its actual receiver.

### Corrected behavior and runtime proof

Retained-envelope decoding uses `json.Decoder.UseNumber`.
One shared numeric conversion policy preserves exact signed integers and losslessly representable unsigned values.
It handles finite floats and rejects unrepresentable values before commit.
The wire view supplies native numeric values because the pinned schema validator classifies `json.Number` as a string.
It copies only payload branches that require conversion. The original payload remains unchanged.

`go test -count=1 ./internal/domain/model ./internal/domain/ledger ./internal/adapters/storage/memory ./internal/adapters/storage/postgres ./internal/adapters/telemetry` passed all five packages.
`ginkgo -v --tags=integration --procs=1 --fail-on-empty --focus='adds an offline reviewed type' ./tests/e2e/` passed US2-AS5 on memory and PostgreSQL.
The scenario compares raw retained JSON, exact scoped-query values, full actual OTLP attributes, and the original event ID.
It also compares PostgreSQL delivery-reference acknowledgement. It remains one mapped acceptance scenario.

A temporary `go run ./tmp/ledger-numeric-smoke` program built the production application and registered the closed fixture.
It recorded and queried the fixture's numeric data. It verified exact envelope decoding.
It dispatched a committed reference through the actual synchronous provider and gRPC receiver.
It observed successful acknowledgement with zero pending references. Its output was:

```text
runtime smoke: exact large scalar/array, int32 and float32 retained; envelope decoding exact; actual gRPC OTLP unchanged; original dispatch ID acknowledged; pending references=0
```

The smoke source was removed.
Active configuration, chart, architecture, and storage-contract status text records the retired performance/profile criteria.
The 20 functional scenarios remain required.
Accepted ADR 039, historical measurements, and existing task descriptions remain unchanged.

### Final verification

The separate untagged memory command, `ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' ./tests/e2e/`, passed all 14 applicable scenarios. The six PostgreSQL-only scenarios remain in the tagged lane. All 20 active scenario identifiers still have one mapped acceptance case.

`ripwire . --quality-delta --json` reported zero gating regressions. It reported new numeric-helper complexity and test-related heuristics, not zero structural findings. The Wire signature check reported no incompatible direct callers. No structural suppression was added.

All 122 task entries are checked, including the five previously approved performance retirements.
T120–T122 completed in this execution.
The extension configuration parsed successfully. It registered one enabled optional post-implementation commit hook, with no mandatory post-hook.
`/speckit-git-commit` was offered but not executed. No commit was created.

## Refresh locking and acceptance baseline review (2026-10-06, N1/N4)

N1 documents the session-level refresh lock, backend affinity, context-bound acquisition, and connection discard.
Direct PostgreSQL connections or session-mode pooling are required.
The storage contract, operations guide, architecture, and ledger ADR agree.
The ledger ADR is 039, formerly 037. Its original acceptance date and decision remain intact.

Initial four-process tagged runs passed 19 of 20 ledger scenarios.
Legacy compatibility stopped at strict log field-key assertions.
A comparison with older main `e6902cf296e617ff02e315a069d5dfb73f44bb51` first suggested a production logging regression.
That interpretation was incorrect.

The ledger commit's actual main parent, `446183e095381afec8252b274768b831d6ba4e0e`, already contains correlated refresh logs and the exchange `failure_reason` field.
The attempted logging correction and its test changes were reverted.
No merged main logging or context-forwarding test changed.

A detached checkout of that actual main parent ran the existing public-client refresh success/failure journeys and verified-client no-grant journey. All three passed. A temporary test logger captured only event names, levels, messages, and field keys, never attribute values. The no-grant journey also passed separately with this descriptor-only logger. Observed changes were:

- `access token refreshed`: `actor` and `trace_id`, count 2.
- `oauth2_token_refreshed`: `actor` and `trace_id`, count 2.
- `oauth2_refresh_failed`: `actor` and `trace_id`, count 1.
- Verified-client `Token exchange failed`: `failure_reason`, count 1.

Only those three workflow fixtures use a `CaptureRevision` override for `446183e095381afec8252b274768b831d6ba4e0e`. The original capture revision remains `d500f36378dd914f8a516604a08525f737e8ddff` for unchanged workflows. Strict field-key and multiplicity assertions remain intact. This is an observed main-baseline update, not a re-pin to feature output. The temporary instrumentation and detached checkout were removed.

N4 retains the specification-approved shared CI lane, four workers, and process-global Serial cases.
The dated task snapshots are explicitly historical.
The functional contract required no exact PostgreSQL image digest. The existing shared image tag remains unchanged.

Findings M1, M2, and M9 remain explicit prerequisites for future OTLP ingestion.
Their definitions and resolution evidence were unavailable. These results claim no resolution.

Final verification used `ginkgo -v --tags=integration --procs=4 --fail-on-empty --label-filter='business-event-ledger && !performance' ./tests/e2e/`. All 20 scenarios passed, with zero failures or pending cases. This includes shared memory/PostgreSQL workflows, crash and deletion barriers, and all legacy-log variants with real OTLP export. `just check` passed with zero linter issues. No fresh full-repository `just verify` pass is claimed.

## Token denial recording review (2026-10-06, N2/N3)

N2 classifies the pre-rule `scope_not_permitted` rejection as structural.
N3 records resolver `server_error` as `internal_failure`.
It classifies recognized proxy OAuth refusal codes without upstream descriptions.
Focused regression tests failed for the reported defects before correction and passed afterward:

```sh
go test -count=1 ./internal/domain/impersonation ./internal/domain/oauth2 \
  ./internal/adapters/http/enduser \
  -run 'TestImpersonationLedgerScopeRejectionBypassesRecording|TestTokenResolverServerErrorRecordsInternalFailure|TestProxyLedgerClassifiesOAuthFailuresWithoutRetainingUpstreamText'
go test -count=1 -race ./internal/domain/impersonation ./internal/domain/oauth2 \
  ./internal/adapters/http/enduser
```

Temporary production-builder HTTP smokes passed on memory and PostgreSQL.
Disallowed impersonation scopes returned `400 invalid_scope` with zero new events.
This held both with recording available and with injected append failure.
Proxy `invalid_client` returned the upstream 400 and recorded `authentication_failed`.
Proxy `invalid_grant` returned the upstream 400 and recorded `authorization_failed`.
The temporary source was removed.

The final four-process tagged acceptance run passed all 20 active ledger scenarios.
`just check` passed with zero linter issues.
The structural quality report retains one gating heuristic.
`completeImpersonation` complexity increased from 14 to 16 for the three-line structural-rejection guard.
The guard preserves other failure classifications without a broader refactor. No suppression was added.

These results do not claim a fresh full-repository `just verify` pass.

## Outcome and concurrency review (2026-10-06)

The five review findings required test corrections or additional coverage, not production behavior changes:

- PostgreSQL approval fixtures recognize expiry before retirement. An unrecognized duplicate returns unchanged and does not advance the sync version.
- A post-mint impersonation append error returns `server_error` without a token response. Only the committed permission fact remains.
- Two PostgreSQL consent requests wait on the pair lock before first-time creation. They return one grant ID and retain exactly one creation fact.
- Refresh faults clear after rollback. A determinate rollback retains one failure fact, but an indeterminate commit error retains no contradictory failure fact.
- Provider workflows assert exact authentication, authorization, and internal-failure codes. A focused classifier test covers `invalid_request` because normal provider workflows do not produce it.

The affected domain packages passed with the race detector:

```sh
go test -count=1 -race ./internal/domain/ledger ./internal/domain/agents \
  ./internal/domain/tokenexchange ./internal/domain/impersonation \
  ./internal/domain/oauth2session ./internal/domain/oauth2server
```

The focused PostgreSQL run passed all selected tests, including the unchanged-duplicate subtest and both concurrency cases:

```sh
go test -count=1 -race -tags=integration -p 2 \
  ./internal/adapters/storage/postgres ./tests/integration/storage/infra \
  -run '^Test(ToolApprovalRepository_(CRUD|ActiveQueriesExcludeExpiredAndConsumed|FailedCreateDoesNotPersistOrAdvanceSync)|ConsentLedgerConcurrentIdenticalFirstGrantRecordsOnlyWinner|AgentLedgerConcurrentIdenticalUpdateRecordsOnlyWinner)$' -v
```

The run used the active Colima socket with `DOCKER_HOST` and `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`.
The initial new consent fixture failed domain validation. The corrected fixture includes a permission set, an included service, and an active session.
`just check` passed with zero linter issues. No full infrastructure gate or E2E run is claimed.

The structural report retained five gating heuristics for test complexity, fixture duplication, and approval-test length.
The tests keep domain-specific fixtures and existing rollback assertions. No structural suppression was added.

### Producer simplifications

Trace and span validation use the existing OTel parsers instead of the custom hex validator.
Agent lifecycle facts reuse `recordDependent`. Token exchange no longer assigns three denial reasons that `completeExchange` overwrites.
The contract checks reported unchanged signatures and no incompatible direct callers.

A temporary `go run ./tmp/ledger-review-smoke` program exercised the in-memory adapter and domain services.
It observed exactly three admin facts for agent creation, update, and deletion, with unchanged trace and span correlation.
It also rejected uppercase, zero, non-hex, and incorrect-width trace and span values. The temporary source was removed.
The affected package tests and PostgreSQL agent-concurrency test passed as recorded in this review section.

## Shared-suite consolidation verification

The ledger uses the existing backend E2E and integration suites. The optional performance suite, scenario runner, and private bootstrap are removed.
The shared server factory owns application construction, including optional schema, tracer, and CIMD injection.

The existing `test-e2e-backend` recipe accepts a label filter and build tags. Its default integration tag includes PostgreSQL acceptance.
Normal `verify` runs that backend lane once and includes the existing Helm checks. The shared `security` recipe accepts repository or broker scope.

The standard commands passed:

- `just check` reported zero linter issues.
- `just test` passed the Go/package tests with the race detector.
- `just test-integration-infra` passed all four infrastructure-backed packages.
- `just security broker` reported zero gosec issues and zero reachable vulnerabilities. It retained one notice for an uncalled module vulnerability.
- `just helm-lint helm-template` passed chart validation and rendering.
- `just test-e2e-backend` passed 664 of 666 scenarios across four workers. The filter excluded the two existing performance scenarios.
- `just test-e2e-backend 'business-event-ledger && !performance' ''` passed all 14 memory-applicable ledger scenarios.

The full backend run included all 20 active ledger scenarios, with memory and PostgreSQL coverage.
The initial backend command stopped before E2E execution because the local web installation lacked `vite-plugin-istanbul`.
`npm ci --no-audit --no-fund` restored the existing locked dependencies. The subsequent standard command passed without dependency-version changes.

The Markdown rendering check passed for all 29 reviewed documents and found no new broken anchors among them.
The structural quality report found zero regressions. No suppression or replacement feature-specific setup was added.
This evidence does not claim a full `just verify` run.
