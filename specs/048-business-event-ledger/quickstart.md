# Business Event Ledger — Validation Quickstart

This guide records implementation commands and observed validation results. The broker implements recording, investigation, retention, erasure, and recoverable telemetry. Performance release acceptance remains blocked until an operator supplies an approved deployment profile and its required measurements pass.

## Pre-refactor baseline evidence (T001)

The production baseline is `main` revision `d500f36378dd914f8a516604a08525f737e8ddff`.
The capture checkout is `7d830d685715e03e5d054c3ef51d8ed5641a58df`.
Its differences from `main` contain planning artifacts, not production or test code.

The baseline run used Go 1.27.1 and Ginkgo v2.32.2.
The first attempt lacked the Ginkgo executable.
After installation, one scenario failed because testcontainers did not discover the active Colima socket.
The following command supplied the socket and passed all 608 functional scenarios:

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
Counts describe the named journey, not a promise of one old log per new ledger event.
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


Existing backend tests did not execute session termination or agent deletion.
Two temporary extensions completed those HTTP workflows through the production builder:

- Session establishment followed by DELETE returned 200, then GET returned 404.
- Agent creation followed by DELETE returned 204, then GET returned 404.

Both extensions passed.
The fixture identifies their source scenarios and additional actions.
No production capture code or new ledger behavior formed part of these runs.

`just fmt`, `just check`, and `go test ./tests/e2e/fixtures` passed for the baseline fixture.
`golangci-lint` was unavailable, so `just check` used its documented `go vet` fallback.
This result does not claim a golangci-lint run.

The session smoke exposed an existing contract difference.
The DELETE handler returns a top-level `message`, but OpenAPI documents `data.terminated` and `data.affected_agents`.
The first smoke assertion assumed the documented shape and failed.
The corrected baseline smoke checked the deletion status and subsequent 404.
This feature must preserve the existing response unless stakeholders separately approve a contract change.
T011 must record this difference.

### T001 commit gate

The first signed commit attempt failed with `gpg: signing failed: Operation cancelled`.
After user approval, the signed retry succeeded as `f9dcb7cb` (`test: capture business event legacy slog baseline`).
T001 is complete.
The commit-signing configuration remains unchanged.

## Refactor verification (T005)

T002–T004 introduce no ledger persistence or telemetry copies.
The shared transaction rename preserves current backend behavior.
The credential domain service owns the existing creation, rotation, and removal policy.
The proxy domain service owns client association and successful-response verification.
The transport adapter preserves streaming, permitted headers, response closure, and existing trace scopes.
`oauth2_token.go` retains its resolution and dispatch interface because that interface requires no change.

The following checks passed after extraction:

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

A separate `go run ./tmp/ledger-refactor-smoke` process built the production application and exercised real HTTP requests:

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
The Fosite boundary wording now names `HandleTokenEndpointRequest`, not the request constructor.

The feature branch contains baseline commit `f9dcb7cb` and refactor commit `9ad008c4`.
The isolated branch is `refactor/048-business-event-ledger-prerequisites`, based on `d500f36378dd914f8a516604a08525f737e8ddff`.
It contains only the baseline and refactor commits, as `d3c5de45` and `e8ddd218`.
Its worktree is `.worktrees/ledger-prerequisites`.

The isolated refactor PR is [#123](https://github.com/zalando-incubator/agentic-identity-broker/pull/123).
GitHub authentication completed through its device flow before submission.
Both branches reached the remote despite an OS keychain storage warning from Git's credential helper.
The independent assistant reviews and local runtime checks complete T005.
This statement does not claim human PR approval, CI completion, or approval of the ledger event and operational contracts.

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
The performance comparison retains T001's baseline revision `d500f36378dd914f8a516604a08525f737e8ddff`.

The user reported no approved deployment profile in the implementation conversation on 2026-09-27.
T095 therefore has an open release blocker.
The reference profile does not replace operator-approved deployment evidence.

## Design prerequisite evidence (T008–T011)

The architecture and root agent context contain the seven ledger glossary terms and the accepted transaction, privacy, retention, and delivery design.
Both documents explicitly distinguish accepted design from deployed behavior.

Helm 3.22.0 supplied the chart validation after the initial command reported that Helm was absent.
The integrated configuration checks passed:

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-lint
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-template
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just check
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" \
  go test ./tests/integration -run TestHelmTemplate
```

A rendered ConfigMap with `retention=720h` and `telemetryCopyEnabled=false` contained the string `720h` and the boolean `false`.
The strict chart schema accepted both optional role names.
The default-backend render introduced no operational grants.
The example parsed as one YAML document with `2160h` and boolean `true`.
The example commands use the actual `just build` output path, `./bin/agentic-identity-broker`.
These are configuration and template checks, not runtime ledger evidence.

The user approved `contracts/events.md` and `contracts/storage.md` in this implementation conversation on 2026-09-27.
The recorded answer is `Approve both contracts`.
ADR 037 separately records maintainer Jan Brennenstuhl's acceptance on 2026-09-26.
ADR acceptance and assistant code review are not substitutes for that contract approval.

T011 reviewed the existing OpenAPI failure representations:

| Workflow family | Existing failure representation |
|---|---|
| Admin agents, credentials, signing keys | HTTP 500, `InternalServerError` using `ErrorResponse` |
| Consent grants | HTTP 500, `InternalServerError` using `ErrorResponse` |
| Third-party sessions | HTTP 500, `InternalError` using `Error`; refresh also documents upstream HTTP 502 |
| OAuth2 authorization/token | HTTP 500 using `OAuth2Error`, including `server_error` |
| Approvals | Existing handlers return HTTP 500 with `error` and `message`; endpoint response lists omit these existing 5xx cases |

The approval handlers use the existing `ApprovalError` field shape for internal failures.
The session-deletion response mismatch remains the separate baseline issue recorded in T001.
Both OpenAPI files remain unchanged.
No new HTTP or CLI read, erase, export, or ingestion surface is intended.
An additional public behavior change or correction to these existing contract gaps requires separate approval.

### Schema publication (T012)

`api/events/v1/` contains the 30 approved schemas and 28 synthetic examples.
The schema files match the reviewed source files byte for byte.
A temporary Go program used the pinned `google/jsonschema-go v0.4.2` validator with an offline URN loader.
It resolved all 30 schemas and validated every example against its selected schema and the catalogue.
It rejected 168 mutated examples covering extra envelope/actor/data fields, invalid outcomes, UUIDv4 event IDs, and unknown types.
The loader also rejected an external HTTPS schema reference.

```text
schemas=30 examples=28 negative_cases=168 offline_refs_rejected=1
```

The temporary program was removed.
This result validates the published shapes, not domain provenance, semantic time parsing, storage, or ledger runtime behavior.

### Database design review (T013)

The review confirmed migration `035_business_event_ledger.{up,down}.sql` against the data model, storage contract, and binding storage/migration ADRs.
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
Defaults, bindings, validation, schema registration, storage behavior, and producers are not implemented by this declaration step.

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
Storage-port decorators provide append failure and an owner-commit completion barrier; no production fault endpoint was added.

A temporary Ginkgo smoke ran this command:

```bash
env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  go test -tags=integration ./tests/e2e/bootstrap \
  -run TestLedgerHarnessSmoke -v -count=1
```

It created and read back an agent over real HTTP on both backends (201 then 200), checked the end-user listener against the configured public URL, and launched a separate production-builder child against the same PostgreSQL clone.
The child read the committed agent over HTTP; termination and repeated termination completed successfully.
The run passed one smoke scenario with zero skipped cases and terminated its PostgreSQL 17 container.
The first smoke request omitted required agent fields; the next fixture attempt exposed the memory-only assumptions in `SeedDefaultConsentData`.
The passing smoke used the existing FK-complete `SeedPlaceholderGrantData` instead, without changing those existing fixtures.
These checks prove bootstrap behavior, not ledger recording, atomicity, or crash recovery.

The temporary smoke was removed.
`just check` passed, including golangci-lint with zero issues, and the bootstrap/helpers/matchers/fixtures package tests passed.

### Acceptance lanes (T016)

| Lane | Command | Selection |
|---|---|---|
| Memory | `just test-e2e-backend` | Untagged functional scenarios, including ledger specs |
| PostgreSQL | `just test-e2e-ledger-postgres` | Integration-tagged ledger specs, both backends, one process, excluding performance |
| Performance | `just test-e2e-performance 'performance && business-event-ledger' integration` | US4-AS4 alone, one process |

The tagged recipe is a dependency of `just verify` and the `e2e-ledger-postgres` CI job is a dependency of the required CI gate.
The performance recipe retains its default `performance` label filter and empty build tags.
Both dedicated selections build frontend assets and use `--fail-on-empty`.
`actionlint .github/workflows/ci.yml` passed, and the performance command's dry run selected the requested labels and build tag.
Before acceptance authoring, the tagged command selected zero of 609 existing specs and exited unsuccessfully because `--fail-on-empty` was set.
That is an empty-selection check, not semantic-red ledger acceptance.
The installed Ginkgo CLI initially reported 2.33.0 against the pinned 2.32.2 package; `go install github.com/onsi/ginkgo/v2/ginkgo@v2.32.2` corrected the local tool version.

### Catalogue and US1 acceptance authoring (T017)

A temporary workflow smoke exercised all 28 mapped catalogue actions through real HTTP on both memory and PostgreSQL 17: 56 completed workflow variants.
It used fresh applications and storage per variant, the shared migrated PostgreSQL template, trusted upstream/JWKS fixtures, and the actual signing-operator authentication boundary.
Grants were created with a positive short lifetime and allowed to expire naturally; no production time-travel control was added.
The smoke passed and its source was removed.

The seven US1 scenarios compile and ran with:

```bash
env DOCKER_HOST=unix:///Users/brennenstuhl/.colima/default/docker.sock \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  /Users/brennenstuhl/go/bin/ginkgo -v --tags=integration --procs=1 \
  --fail-on-empty --label-filter='business-event-ledger' ./tests/e2e/
```

The run selected seven ledger specs; all seven failed semantically, with no pending ledger cases.
The 609 other specs were excluded by the label filter.

| Scenario | Observed red result |
|---|---|
| US1-AS1 | Real grant creation completed; retained event count was zero instead of one |
| US1-AS2 | Production-builder child launched; committed-event SQL oracle reported missing ledger relation (`42P01`) |
| US1-AS3 | Forced append failure could not intercept absent recording; revocation returned 204 instead of failing closed |
| US1-AS4 | Refused exchange completed; retained denial count was zero |
| US1-AS5 | Concurrent revocations completed; retained transition count was zero |
| US1-AS6 | Natural expiry and repeated recognition completed; retained expiry count was zero |
| US1-AS7 | PostgreSQL grant creation completed; no event existed for backend comparison |

These results establish US1's initial red phase, not completed crash recovery, rollback, catalogue persistence, or backend parity.
T021 still requires all 21 scenarios before feature behavior can begin.

### Investigation acceptance authoring (T018)

The five US2 scenarios compile. The memory command below selected all five and each failed on absent retained events, not on compilation or fixture setup:

```bash
/Users/brennenstuhl/go/bin/ginkgo -v --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger' \
  --focus='safe operational investigation' ./tests/e2e/
```

US2-AS1 completed its before/within/after, multi-agent, multi-principal and denied-exchange dataset; the query returned zero rather than the two expected in-window successes.
US2-AS2 completed a verified exchange with hostile forwarding/user-agent metadata and authoritative trace context; no attributed event existed.
US2-AS3 completed its first credential-canary workflow with a real local OTLP receiver; the missing stored envelope was the initial red failure, so OTLP privacy is not claimed passed.
US2-AS4 completed an unauthenticated token rejection, and US2-AS5 completed a grant through the schema-extended production builder; both found no retained event.
The full tagged, all-scenario red gate remains T021.

### Lifecycle acceptance authoring (T019)

The five US3 scenarios compile. The tagged `retention and exact-subject erasure` selection exercised all five.
US3-AS1–AS4 initially failed on absent retained events before erasure, retention, concurrency, or restart assertions could proceed.
The first US3-AS5 draft passed for the wrong reason: an unrelated missing JWE signing key rejected every startup.
A successful positive-retention control exposed that fixture error; the test now supplies both required keys before varying retention.
The corrected US3-AS5 run loads the valid configuration and fails because the zero-retention environment override is not rejected by the pre-feature loader.
The runnable example now includes the required `IDENTITY_BROKER_JWE_SIGNING_KEY` export.

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

The harness corrections address the 20 acceptance-review findings without changing the approved event or HTTP contracts.
Catalogue assertions compare complete event deltas, caller identities, known references, and effective expiry times.
Privacy assertions track actual consumed and returned credentials and inspect only ledger-scope telemetry after shutdown.

PostgreSQL comparisons inspect raw persisted JSON before model decoding.
The telemetry comparator checks the raw envelope, export-time observation, native correlation, empty body, and zero dropped attributes.
A temporary live gRPC smoke accepted a valid historical copy and rejected six malformed copies.
The malformed cases covered stale observation, unrelated native trace/span, dropped attributes, nonempty body, and an omitted nullable subject.
The smoke source was removed.

Crash coordination kills the child before closing stdin and joins the outstanding HTTP request after process exit.
Erasure overlaps a recorder paused before its owner commit.
Recovery assertions require an unaffected pending control to deliver and acknowledge before rejecting deleted history.
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
The runtime role now receives only SELECT permission on that migration metadata.
A later run exposed cancellation of the child before its graceful cleanup.
The bootstrap now leaves child termination to its bounded cleanup rather than the cancelled spec context.
Neither correction changes production code or gives the runtime role DDL privileges.

A separate temporary HTTP smoke completed all 28 catalogue workflows on both production backends: 56 workflow variants.
It observed the explicit signing-key promotion and repeated current-key selection without bypassing the permanent ledger assertions.
The smoke source was removed.
The retained acceptance suite contains exactly 21 scenario references and 21 `It` blocks.
All 21 scenarios now have observable semantic-red evidence across their declared lanes.
The T008–T020 prerequisites and recorded contract approvals permit foundation work after this gate.
The unavailable approved deployment profile still blocks T095 release acceptance.

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
Permission sets serialize as a sorted unique set without changing the caller's slice.
The decoder rejects additional envelope, actor, and client fields instead of silently removing them.
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
Direct validation fixtures now supply the mandatory ledger defaults rather than relying on a compatibility bypass.
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
User approval on 2026-10-01 defers unchanged delivery-only checks to T090 rather than moving dispatch before lifecycle implementation.

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
Independent failures use a fresh request context after caller rollback. No external operation is retried by the recorder.
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
Those findings were reviewed without replacing distinct domain contracts with a generic constructor.

### Factory, builder and foundation gate (T042–T043)

The factory supplies the real backend event repository and shared transaction manager.
The builder compiles the embedded catalogue before assembling the ledger recorder.
Invalid schema registration stops `Build`; no application is returned for serving.
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
The unchanged delivery-only tests are deferred to T090 under the approved gate clarification; no dispatch behavior is claimed here.
The scoped factory quality report was reviewed, not declared clean: it flags `BusinessEvents` as dead code despite its interface consumers in the builder and repository tests, confirmed by language-server references. Other rows concern short-horizon churn rather than a required design change.

### Producer mutation regressions (T044)

Focused tests live beside the existing service tests in each package's `business_event_test.go`.
They cover exact grant/approval/agent transitions, silent unchanged and repeated operations, a losing approval CAS, append/commit rollback, effective-expiry renewal, and actual agent-deletion child facts.
The agent cascade includes two principals, a permanent approved approval, an already-denied approval that must not become a new denial, and a credential identified only by its safe record ID. An unrelated agent's grant must remain unchanged.
The shared test-only transaction fixture stages events and snapshots the small mock datasets, so tests observe business state and sync versions after failed recording. It is not a production transaction implementation.
Private producer dependencies and the narrow agent-dependent enumeration facet are declarations for the forthcoming implementation; no producer recording behavior is enabled by this test task.

```bash
go test -count=1 ./internal/domain/consent ./internal/domain/approval ./internal/domain/agents \
  -run '^(TestConsentLedger|TestApprovalLedger|TestAgentLedger)'
just fmt
just check
```

The test command compiled and failed semantically: transitions retained zero events, and injected append/commit failures still returned success. The CAS-loser case preserved its existing failure semantics. Static checks then passed with zero lint issues. Existing acceptance expectations were unchanged.
The scoped quality report was reviewed, not declared clean. It identifies the rollback matrices as new test complexity and typed mock enumeration/snapshot shapes as duplication. The separate domain fixtures preserve explicit state assertions instead of introducing a generic repository framework; interface implementations and Go test entry points are reported as statically unused.

### Issuance, credential, signing and session regressions (T045)

The new focused tests cover credential generation/replacement/revocation, secret and token withholding while an owning commit is blocked, append/commit failures, real Fosite replay revocation, bootstrap competition, actual current-key selection versus future eligibility, callback reauthorization, session termination, terminal refresh failure, and accepted refresh survival after a later rollback.
Callbacks and refresh use a local HTTP token endpoint and valid JWE state. The fixture initially omitted the provider's explicit public-client authentication method; that fixture error was corrected before accepting session red evidence.

```bash
go test -count=1 ./internal/domain/oauth2server ./internal/domain/oauth2session \
  -run '^(TestCredentialLedger|TestProviderLedger|TestSigningLedger|TestSessionLedger)'
go test -count=1 ./internal/domain/oauth2session ./internal/domain/oauth2server \
  -run '^(TestSessionLedger|TestSigningLedgerCompetingBootstrap)'
just fmt
just check
```

The tests compiled. Credential/token withholding failed because no owning commit was reached; injected recording failures returned success; signing/session transitions produced zero facts. The corrected session paths reached callback/refresh/termination before failing for absent recording. Competing bootstrap created one key but retained zero selection facts. Replay-revocation preservation remained green under injected failure-event append errors. Static checks passed with zero lint issues.
The scoped quality report was reviewed, not declared clean: it flags Go test/interface entry points as unused, similar explicit rollback matrices as clones, and a session-specific copy helper as a clone of an unrelated document copy. Those typed domain fixtures are retained without a generic cross-domain copy abstraction.

### Typed outcomes and staged transport regressions (T046)

The focused tests cover one specific exchange refusal without generic duplicate facts, a committed impersonation decision before minting, decision survival after mint failure, one final policy denial across candidate rules, internal-error classification, and no mint after recording failure.
Proxy completion must stage every response, preserve known agent attribution without inventing a subject from unverified upstream claims, record terminal transport/verification failures, and withhold a staged body if recording fails. HTTP tests distinguish the approved staged bytes from different unstaged transport bytes, including hybrid proxy dispatch.

```bash
go test -count=1 ./internal/domain/oauth2 ./internal/domain/tokenexchange ./internal/domain/impersonation ./internal/adapters/http/enduser \
  -run '^(TestProxyLedger|TestExchangeLedger|TestImpersonationLedger|TestTokenGrantProxy)'
just fmt
just check
go test -count=1 ./internal/adapters/http/enduser \
  -run '^(TestLocalGrantStrategy_AcceptsLocalClient|TestHybridTokenGrantStrategy_DispatchByClientType|TestProxyGrantStrategy_InfraErrorsReturnJSON)$'
```

The new tests compiled and failed semantically: specific domain outcomes retained zero facts, permission decisions were absent when minting began, and proxy/hybrid HTTP paths wrote the unstaged canary rather than completed bytes. Existing local/hybrid routing and proxy infrastructure-error response tests passed. Static checks passed with zero lint issues.
The scoped quality report had no preexisting major regressions. New rows report test/interface entry points as unused and distinct impersonation-outcome assertions as clone shapes; these cases retain their different decision and mint boundaries.

### Expiration recognition repository regressions (T047)

Memory and PostgreSQL tests cover expired candidates, one recognition winner, unchanged-versus-renewed validity, stale recognizers, ambient rollback, competing recognizers, and PostgreSQL connection/deadline classifications. Resolved permanent approvals remain governed by their existing lifecycle rather than the pending TTL.
The methods were declared without behavior solely to compile the focused tests; T048 must replace those inert bodies before any producer uses them.

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

The tests compiled and failed semantically: candidate lists were empty, recognition returned false, competing recognizers had zero winners, and missing/deadline databases returned no error. The live grant and adapter-package approval cases were red for absent recognition. The operational approval SQL fixture needed separate UUID/text parameters and its required gateway identity; after both fixture corrections, that live case was red for zero candidates rather than SQL errors. Static checks passed with zero lint issues.
The scoped quality report had no preexisting major regressions. It flags the two-family live test's fixture branches and typed equivalent assertions as new complexity/duplication; each family retains observable database marker checks.

### Expiration marker implementations (T048)

Both existing grant/approval adapters now implement the separate expiration facets. Factory accessors expose those facets without adding operations to the business repository interfaces.
Memory keeps payload-free expiry instants in adapter-owned maps, journals touched markers with the shared transaction, and removes grant markers with physical business deletion. PostgreSQL uses conditional ambient-executor updates and the existing schema columns. Candidate reads are bounded; stale or already recognized expiries lose, and resolved approvals do not acquire pending-expiration facts.

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

Static checks, focused memory/PostgreSQL unit tests, both live PostgreSQL suites, and memory recognition races passed. The filtered factory package had no matching tests; its accessors were exercised by the runtime smoke:

```text
factory_expiration_facets=true rollback_reopens_recognition=true competing_recognizers_one_winner=true recognized_candidate_removed=true
```

The smoke source was removed after proof. The scoped cumulative quality report was reviewed, not declared clean: it retains earlier transaction-guard/typed-constructor clone findings, reports the new PostgreSQL grant row scan as a clone of the existing typed scan, and reports interface accessors as statically unused. These shapes preserve existing adapter conventions and avoid an additional storage framework or N+1 candidate reads.

### Atomic grant change recording (T049)

Grant creation, changed effective permissions/validity, and actual revocation now append their exact facts in the owning transaction. Unchanged state and absent idempotent revocations are silent; the user-facing revoke endpoint retains its not-found result and existing success log fields.
The builder injects the required recorder. All language-server-discovered constructor callers were migrated. The ledger's owning/joined transaction orchestration is shared by recording and business mutations; no second transaction mechanism was introduced.
PostgreSQL ambient grant lookup takes a transaction-scoped principal/agent pair lock before its row lock, including the absent-row case. The separate two-int lock class avoids the lifecycle/subject lock classes. Memory uses its existing visibility gate.
An additional regression demonstrated that reordered included services created a false update fact; it failed before the allocation-free effective-permission comparison and passed afterward.

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

Static checks and the affected grant/ledger/builder/handler tests passed. The exclusions leave the already-authored expiry and exchange recording assertions for their assigned T050/T055 implementations, without changing those assertions.
The production-builder smoke produced:

```text
production_builder_grant_facts=3 unchanged_update_silent=true concurrent_revoke_one_fact=true
```

The live PostgreSQL domain smoke completed eight concurrent idempotent revocations without an error, retained one exact `grant-revoked` fact, and removed the grant. It exercised domain results, not HTTP status codes. Both temporary sources were removed after proof.
The scoped quality report was reviewed, not declared clean. It reports small constructor/transaction growth, the order-independent comparison's explicit membership loops, and cumulative clone findings for unchanged typed repository reads/constructors. Those findings do not justify a generic repository or cross-domain constructor abstraction.

### Lazy grant expiration recording (T050)

Grant access, active/user-grant reads, delegation reads, and OAuth2 authorization now recognize effective expiry through the consent-domain invariant. Marker and fact commit together; repeated recognition is silent, failed recording restores recognition, and a changed validity permits a different expiry. Expiration facts use effective expiry as occurrence time and the system lifecycle actor.
Authorization receives the consent service rather than querying its grant repository independently. All discovered constructor callers were migrated, and the existing expired-grant consent redirect remains unchanged.
The grant expiration facet adds exact-principal candidate selection before the bounded limit. Memory and live PostgreSQL regressions failed for absent scoped candidates before implementation and passed afterward. This prevents unrelated expired rows from exhausting a delegation-read batch without recognizing another principal's state.

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

The domain, builder, HTTP, and backend checks passed after legacy grant mocks acquired their required marker facets; no expectations were changed. Separate HTTP integration expiry/metadata checks passed after their fixture migration. The exclusions retain the pending T054/T055 outcome regressions.
The production-builder smoke exercised expiry first through authorization, verified its consent redirect, then repeated grant/delegation reads and observed:

```text
production_builder_lazy_expiry_one_fact=true effective_expiry_timestamp=true system_actor=true public_expired_grants_filtered=true
```

The smoke initially omitted the required original authorization URL for JWE session claims; the fixture was corrected before accepting runtime proof. Its source was removed afterward.
The cumulative scoped quality report was reviewed, not declared clean: it reports small added branches on authorization/delegation, existing transaction-guard/typed-reader clone shapes, and interface entry points as unused. Exact-subject candidate selection and consent-domain reuse are retained without a separate scheduler or public ledger API.

### Session lifecycle recording (T051)

Accepted callbacks, including reauthorization, store encrypted session state with one `session-established` fact. Accepted refresh state commits with `session-refreshed` before later exchange work; caller-visible refreshed state is updated only after commit. Actual termination and its captured session/service/subject references share the owning transaction.
Upstream and encryption preparation remain outside the write scope. A failed terminal refresh records one fixed `session-refresh-failed` reason after unsuccessful mutation rollback, not one per network retry. Typed refresh classifications preserve existing error wrapping and logs without putting raw diagnostics in event data.
PostgreSQL ambient session lookup locks the principal/service pair before its row, including absent callbacks. It uses a distinct two-int lock class. The builder and all discovered constructor callers supply the required recorder.
The newly authored failure test initially expected empty data; it was corrected to the reviewed schema's required `reason_code: upstream_rejected`, with the correction reported before editing. The published schema and existing acceptance expectations were unchanged.

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
The scoped quality report was reviewed, not declared clean. It reports added termination/refresh orchestration lines needed for commit boundaries and terminal failure recording, the constructor's recorder parameter, and cumulative typed PostgreSQL read clones. Existing structured logging remains explicit rather than being replaced with a generic logging wrapper.

### Accepted authorization request recording (T052)

The authorization service records `authorization-requested` after client/mode/redirect/scope validation and before grant checks or consent/proceed output. Invalid redirects do not become accepted requests. Recording failure produces the existing server-error decision without releasing a consent session token.
A new boundary regression compiled and failed for zero accepted-request facts before implementation. The builder and all discovered constructor callers now supply the required recorder; no authorization-code persistence hook or second dispatcher recorder was added.

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

The temporary source was removed after proof. The scoped quality report had no preexisting major regressions; it reports small authorization/constructor growth and the explicit authorization recipe as a clone shape of another typed event recipe. These distinct fact owners remain separate rather than introducing a generic event-construction layer.

### Local token issuance ownership (T053)

The local provider requires the recorder and shared storage transaction manager. Client-credential, authorization-code, and refresh responses release only after their owning transaction commits response-population mutations and `token-issued`. Terminal failures record independently after rollback; code-replay and refresh-reuse revocations keep their independent failure-side boundaries. Existing Fosite storage transaction methods already join the foundation's owner and needed no second transaction mechanism.
The focused local-outcome test compiled and failed for zero `token-issued` facts before implementation. An earlier constructor-only check failed compilation for an unused import and is not semantic-red evidence.
The additional real-memory rollback regression first failed because Fosite consumes PKCE during request validation, before response population. The repaired owner includes PKCE validation after replay detection: validation rejection still commits its one-shot challenge consumption without issuing a token; later issuance/append failure restores the challenge, authorization code, and refresh state. The regression now proves retry usability, exact independently committed failure counts, and preserved one-shot rejection. The pinned producer contract records this newly inspected library boundary.

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

The temporary source was removed afterward. Scoped quality review reports no preexisting major regressions, but retains constructor arity/size growth, added authorization-code ownership orchestration, and the new real-rollback test's size. The explicit rejection-versus-issuance boundary is retained rather than hidden behind a configurable generic transaction helper.

### Staged proxy and token parse outcomes (T054)

Every upstream response is staged by the domain outcome service; the transport port no longer offers `StreamBody`. The proxy/hybrid strategy forwards only the completed body's permitted headers, status, and bytes after recording. Verification/read/transport refusals produce one terminal failure, and recording failures withhold credential bytes with the existing server-error response form. No upstream JWT claims are decoded for subject attribution.
Typed parse failures now complete through the domain service before their HTTP error response. Local parsing and proxy completion share the same builder-owned outcome service in hybrid mode, while actual local minting remains recorded once by the provider. Missing identity stays null. Existing local issuance logs and proxy verification/write-failure logs retain their names, levels, messages, and fields.
The new HTTP/local parse-boundary regression compiled and failed for zero recorded failures before those adapters were changed. Its green checks include a commit callback proving no error response bytes/status escape before commit and an append failure that produces a 500 without claiming a second event.
The pre-existing proxy regression's empty-data assertion was removed after identifying its conflict with the published failure schema's required `reason_code`; its response, type, count, and subject assertions remain. Obsolete streaming mock methods/flags and their now-tautological assertions were removed with the port operation, not replaced with source-text checks.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2 ./internal/adapters/http/enduser ./internal/app
go test -count=1 ./tests/integration -run '^TestOAuth2Token'
go run ./.ledger-foundation-smoke
```

Static checks and all matching packages/integration tests passed, including the previously pending proxy/hybrid staged-response tests.
The production-builder smoke used an actual local upstream HTTP server and the token handler, checking accepted/refused status/body preservation, permitted headers, excluded cookies, malformed requests, recording-failure response withholding, and stored credential canaries:

```text
production_builder_proxy_http_status_and_body_preserved=true recording_failure_http=500 uncommitted_token_withheld=true issued=1 failed=2 credential_canaries_absent=true
```

The temporary source was removed afterward. Scoped quality review is not clean: it reports 20 major gating clone/clone-of-helper findings for constructor shapes shared with unrelated constructors/matchers, plus minor control-flow growth and the new HTTP error helper's parameter count. These are explicit local constructor/fact recipes and transport-response arguments, not duplicated business behavior; no unrelated constructor rewrite or generic event/DI framework was introduced to eliminate the normalized shape matches.

### Exchange success and primary refusal recording (T055)

The exchange service requires the recorder and constructs facts from progressively established identities/references, not request or audit struct serialization. It retains the verified initiating client separately from the subject and sets `on_behalf_of` only after the active user grant establishes delegation. A successful `token-exchanged` requires receiving-agent attribution through the closed schema and completes recording before the response escapes.
Authentication/authorization refusals produce one `token-exchange-denied`; parse, transport, and internal failures remain `token-request-failed`. The service does not emit `token-issued` for exchange. Terminal recording runs after the existing workflow scopes have returned, so accepted refresh and expiry-recognition commits are not enclosed in an exchange-wide rollback.
The specific-refusal regression compiled and failed for zero occurrences in both authentication and authorization paths before implementation. The full exchange, builder, and token-handler packages pass, including an additional deterministic CEL evaluation-error test that retains the generic internal-failure fact without claiming a permission denial or established delegation.

```bash
just fmt
just check
go test -count=1 ./internal/domain/tokenexchange ./internal/app ./internal/adapters/http/enduser
go run ./.ledger-foundation-smoke
```

The production-builder smoke used a local JWKS/discovery server, library-signed subject/client JWTs, and an encrypted-session fixture with the same configured test key/AAD. It exercised the real token HTTP handler for success, invalid client assertion, and missing delegation, then queried subject/no-subject records:

```text
production_builder_exchange_http=200 authentication_denial_http=401 delegation_denial_http=403 token_exchanged=1 token_exchange_denied=2 receiving_agent_and_grant_session_service_preserved=true verified_caller_distinct_from_subject=true
```

The first smoke request correctly failed audience validation because its fixture used the public URL instead of the exchange audience. The fixture was corrected to an explicit configured audience; signature/claim validation was not weakened. Its temporary source was removed after proof.
Scoped quality review initially gated on added orchestration complexity. Reusing the same typed refusal predicate at its three call sites reduced that finding to minor growth. The remaining major finding is the predicate's normalized `errors.As`/code-check shape matching an unrelated rate-limit predicate; its distinct domain meaning is retained rather than creating a cross-domain generic error classifier. Constructor arity and added fact/reference orchestration remain explicit measured tradeoffs.

### Impersonation permission and minting facts (T056)

Rule evaluation now selects the authorized mint input without minting. The selected rule's active delegation is required before `impersonation-granted` commits; minting starts afterward. Successful output adds `token-exchanged` before response release. A mint failure retains the committed granted decision and adds only `token-request-failed`, while evaluation/storage/internal errors never become false permission denials.
Final policy/credential/delegation refusals produce one permission denial plus their primary exchange denial in a shared non-mutating owner transaction, not one pair per candidate rule. Known target lookup rejection is a permission refusal plus the malformed-target primary failure; syntax-only activation failures and malformed impersonation forms record generic request failures. Existing HTTP forms and audit serialization remain unchanged.
Ledger identities are constructed separately from audit fields. A signed subject is attributable after verified extraction; ADR 031's unverified subject is not attributable until the subject-bound policy and active delegation establish it. The verified client assertion identity, verified actor-token identity, affected subject, and established `on_behalf_of` remain distinct.
The compiling pre-mint regression failed because minting started with zero permission facts before implementation. The full impersonation, builder, and token-handler packages now pass, including granted-before-mint, retained grant after mint failure, one final candidate denial, internal-error classification, recording failure preventing mint, and existing audit/credential-role/unsigned-subject/delegation behavior.

```bash
just fmt
just check
go test -count=1 ./internal/domain/impersonation ./internal/app ./internal/adapters/http/enduser
go run ./.ledger-foundation-smoke
```

The production builder smoke used a real local JWKS receiver and three separately identified signed credential roles, drove the token HTTP handler, and verified the minted output against the actual broker JWKS handler:

```text
production_builder_impersonation_http=200 signed_output_verified=true missing_delegation_http=403 granted_then_exchanged=true denied_then_exchange_denied=true verified_client_actor_and_subject_distinct=true
```

A fixture call used `ServeHTTP` instead of the JWKS handler's actual `ServeJWKS` method and failed compilation before correction; it is not runtime proof. The temporary source was removed after the successful run.
Scoped quality review initially flagged orchestration growth. Isolating the authorized permission/minting boundary removed that major finding. Remaining major findings are the mandatory recorder constructor check and activated target-failure recording crossing complexity thresholds; typed fact-construction clone shape and constructor arity remain recorded tradeoffs, not a claim of a clean quality report.

### Approval winning transitions and post-commit signals (T057)

The approval service requires the recorder. Its five mutation APIs acquire the owning transaction and subject gate before business reads, reuse the existing CAS/idempotent repository results, and append only winning `approval-requested`, `approval-approved`, `approval-denied`, `approval-consumed`, or `approval-revoked` facts. Sync-state updates share that owner. Existing pending returns, CAS losers, and already-consumed repeats are silent; revocation is not an extra denial.
Notifications, pending metrics, and existing success logs run through transaction after-commit effects. Failed append/commit cannot expose a successful mutation response or success signal. The notification regression's fixture now uses the same real memory manager for approval, sync, event, and owning transaction state rather than a separate fixture transaction stack.
The winning-transition regression compiled and failed for zero facts before implementation. Transition/no-op/CAS-loser/state+sync rollback and owning-notification tests now pass. All approval tests other than the still-pending T058 lazy-expiry regression, plus the affected builder/approval-handler/token-handler packages, pass.

```bash
just fmt
just check
go test -count=1 ./internal/domain/approval -skip '^TestApprovalLedgerLazyExpiry'
go test -count=1 ./internal/app ./internal/adapters/http/handlers/approval ./internal/adapters/http/enduser
go run ./.ledger-foundation-smoke
```

The production-builder smoke exercised all five transitions, pending deduplication and consumed repeats, then rejected an append and observed restored pending state/sync version and no subscriber notification:

```text
production_builder_approval_facts=9 all_five_transition_types=true pending_dedup_and_consumed_repeat_silent=true failed_append_restores_pending_and_sync_version=true rollback_notification_absent=true
```

The temporary source was removed after proof. Scoped quality review identified duplicated new transaction-result wrappers; one private helper now handles the five real callers and their two concrete result types, reducing their matched wrapper tokens from roughly 94–96 to 42–44. The report still has ten major normalized wrapper/effect-access clone findings plus inherited bodies classified as new after internal extraction. This is not a clean quality claim: the remaining thin typed calls and context-effect checks are retained, and no unrelated domain/error helper was merged to remove the shape matches.

### Once-only approval expiry and guarded dedup retirement (T058)

Approval reads, expired mutation races, and existing filtered/create paths recognize expired pending candidates through the separate ISP facet. Recognition rechecks the owning record under the subject gate and commits its effective-expiry marker with `approval-expired`; the event uses the effective expiry time and the `broker-lifecycle` system actor. Expired mutation recognition runs after the failed mutation owner rolls back, so it is not lost with the rejected decision. Marker failure is rolled back and remains retryable.
Pending retirement is guarded in both adapters: an expired duplicate without the matching marker is returned rather than consumed. The domain records it and invokes creation again in the same owner, then records the genuinely new request. PostgreSQL uses database execution time rather than the owner's earlier transaction-start time for this retirement predicate. Existing consumed/persistence/status rules remain: no expired enum and no new expiry scheduler.
The lazy-expiry regression compiled and failed for zero facts before implementation. The memory retirement guard regression also failed before adapter changes because the repository prematurely returned a new approval. The original post-recognition replacement/consumed assertions remain; the PostgreSQL equivalent now protects the same gate. Additional tests cover marker append rollback, recognition after expired approve/deny rollback, and a deterministic TTL crossing between discovery and creation.

```bash
just fmt
just check
go test -count=1 ./internal/domain/approval ./internal/app ./internal/adapters/http/handlers/approval ./internal/adapters/http/enduser
go test -count=1 ./internal/adapters/storage/memory -run '^TestToolApproval'
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestToolApprovalRepository'
go run ./.ledger-foundation-smoke
```

Static checks, complete affected domain/handler/builder tests, memory approval tests, and the tagged PostgreSQL repository checks passed. The production-builder smoke expired a real created approval, repeated reads and a rejected mutation, created a replacement, and queried the retained facts/candidate marker state:

```text
production_builder_expiry_once=true effective_expiry_time_preserved=true expired_read_and_mutation_gone=true retired_pending_consumed_without_new_status=true replacement_requested_committed=true old_expiry_marker_survives_retirement=true
```

An unused throwaway import initially prevented compilation and was removed before runtime proof. The temporary source was removed afterward. Scoped quality remains consciously non-clean: it reports the cumulative memory transaction-guard/constructor/typed-reader clone shapes, approval owner/effect-access shapes, and distinct grant/approval marker recipes, plus the new candidate/retirement orchestration. No generic cross-domain expiry/event framework or unrelated adapter rewrites were introduced to remove those shapes.

### Atomic agent lifecycle and effective-state comparisons (T059)

Agent creation, changed configuration, and actual deletion commit credential-free facts with the business mutation. Equivalent collection ordering and absent deletion are silent. PostgreSQL agent reads inside an owner acquire `FOR UPDATE`, so competing identical updates compare the committed winner rather than append duplicate facts from stale reads.

The lifecycle and rollback tests pass. A collection regression first failed because replacing duplicate `read` scopes with `read,write` was incorrectly treated as equal; both-direction membership checks now preserve that change while keeping reordered scopes silent. A real PostgreSQL competing-update regression first retained two events and now retains exactly one after the waiting owner rechecks locked state.

```bash
just check
go test -count=1 ./internal/domain/agents ./internal/adapters/http/handlers/admin ./internal/app ./tests/integration \
  -run 'Agent|BuilderMinimal' -skip '^TestAgentLedgerDeletionCapturesCompleteActualCascade$'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestAgentLedgerConcurrentIdenticalUpdateRecordsOnlyWinner$'
go run ./.ledger-foundation-smoke
```

Static checks and these targeted packages passed. The excluded complete-cascade regression remains red and belongs to T062; it is not counted as a passed agent acceptance journey. The production-builder smoke drove real HTTP create, unchanged update, changed update, delete, and repeated delete, then queried the retained envelopes:

```text
production_builder_agent_http=201,200,200,204,204 agent_facts=3 unchanged_update_and_absent_delete_silent=true credential_configuration_excluded=true
```

The initial smoke lacked the builder's required storage adapter; it was corrected before runtime proof. Its temporary source was removed afterward. The scoped quality report is not clean: it reports 19 major cumulative clone findings involving dependent-list and fixture shapes. No unrelated fixture rewrite or generic transaction/event abstraction was introduced to hide those findings.

### Atomic credential lifecycle and secret-release boundary (T060)

Credential generation prepares cryptographic material before the write owner, then locks and rechecks the owning agent and credential. The record mutation and safe-ID fact share the transaction. Replacement emits only `credential-rotated`; removal captures the actual credential ID and preserves the existing absent-credential error. Secret/hash/configuration values never enter the envelope, and plaintext responses remain withheld until commit succeeds.

The focused tests initially failed for missing facts, missing rollback errors, and secret release before any owner commit. They now pass. The repeated-revoke expectation was corrected from success to `ErrNotFound` to match both real adapters and the existing HTTP 404 contract; the exact three-fact assertion remains unchanged.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2server -skip '^TestSigningLedger'
go test -count=1 ./internal/app ./internal/adapters/http/handlers/admin
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestClientCredential'
go run ./.ledger-foundation-smoke
```

Static checks, affected domain/builder/handler packages, and tagged PostgreSQL credential tests passed. Signing-selection regressions remain assigned to T061. The earlier memory `Credential` filter selected no tests and is not counted as adapter proof. An initial static run rejected unformatted code; the successful run followed formatting and smoke removal.

The production-builder smoke drove real HTTP generation, rotation, forced append rejection, revocation, and repeated revocation. It observed the restored prior ID/hash after the rejected rotation, no secret in the 500 response, and exactly the three safe credential facts:

```text
production_builder_credential_http=201,200,500,204,404 credential_facts=3 replacement_rotation_only=true failed_append_preserves_old_credential_and_withholds_secret=true plaintext_and_hash_excluded=true
```

The temporary source was removed after proof. Scoped quality remains non-clean: eight major normalized constructor/owner-shape clone findings remain, including the mandatory credential-recorder constructor. No generic event/transaction framework or unrelated constructor migration was added to remove those shape findings.

### Atomic signing selection and bootstrap ownership (T061)

Selecting a newly generated current key joins the owning storage transaction and records its safe `signing_key_id` and UTC eligibility instant. Explicit promotion uses the shared serializable transaction protection to compare the target's prior current flag with the selected result. Re-selection retains immediate activation behavior but emits no additional fact. `occurred_at` is selection time, separate from the future `data.activates_at`; bootstrap retains the existing lock-before-lifecycle order and recovery probes.

The signing regressions initially failed for zero selections and missing rollback behavior. Bootstrap, changed/repeated selection, future eligibility, append/commit rollback, and competing bootstrap tests now pass. Unit assertions were corrected to the approved `signing_key_id` schema rather than `kid`. The implementation-only no-target-read subtest and its helper were removed; activation-boundary assertions now inspect returned metadata directly rather than a repository forwarding spy.

```bash
just fmt
just check
go test -count=1 ./internal/domain/oauth2server ./internal/app \
  ./internal/adapters/http/handlers/admin ./internal/adapters/http/enduser
go test -tags=integration -count=1 ./internal/adapters/storage/postgres -run '^TestSigningKey'
go run ./.ledger-foundation-smoke
```

Static checks, the complete affected packages, and tagged PostgreSQL signing repository checks passed. The production-builder HTTP smoke built twice over the same storage, selected a generated current key while observing the existing grace fallback, promoted and re-selected an older key, and rejected both promotion and current-key generation during forced append failure:

```text
production_builder_signing_selections=3 bootstrap_restart_silent=true generated_current_grace_preserved=true same_key_promotion_silent=true failed_promotion_and_generation_rollback=true selection_time_distinct_from_eligibility=true
```

The temporary source was removed after proof. Scoped quality remains non-clean: six major normalized constructor/fact-owner clone findings remain. The serializable selection boundary and separate bootstrap owner are retained rather than adding a second selection/lock API or generic cross-domain orchestration framework.

### Actual dependent terminal facts and subject gates (T062)

Agent deletion discovers grant/approval subjects before the owner acquires its sorted gates, locks the parent and dependent rows, and rechecks discovery before mutation. A newly discovered subject fails closed without deleting anything. The owner captures credential/permission references, removes business rows, and records only the actual agent/grant/permanent-approved-approval/credential terminal facts; an already denied approval is removed without inventing another denial or revocation.

The memory factory connects those child stores to the agent repository. Its cascade owns or joins a transaction, including standalone repository deletion, so cancellation/rollback cannot leave a partial cascade. The existing T044 complete-cascade regression went from one fact to the required five. A new memory adapter regression first failed because grants survived parent deletion and now verifies child removal and rollback restoration. A discovery-boundary regression fails without the recheck and passes with the guard restored.

Provider session behavior is intentionally unchanged: migration 004 uses `ON DELETE RESTRICT`, not a session cascade. The production PostgreSQL smoke confirms rejection, retained provider/session rows, and no synthetic `session-terminated` event. Changing that destructive HTTP behavior requires separate approval; it is not inferred from the catalogue's conditional cascade guidance.

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

Static checks, affected packages, domain/memory race checks and the existing PostgreSQL race regression passed. The throwaway production-builder PostgreSQL HTTP smoke retained exactly the five terminal facts across two subjects, verified the physical child rows were gone, and checked the existing provider restriction:

```text
production_builder_postgres_agent_cascade_http=204 terminal_facts=5 denied_approval_not_reclassified=true safe_references_retained=true all_dependents_removed=true provider_session_restriction_preserved=true no_synthetic_session_termination=true
```

Its first runs lacked the required logger and included-service grant reference; both fixtures were corrected before runtime proof. The source was removed afterward. Adapter-backed empty cascade fixtures live in `tests/unit/cascadefixture`, separate from the infrastructure-free ledger fixture: an initial shared-fixture import produced domain test cycles and was corrected. Scoped quality improved after separating parent transaction ownership, discovery/recheck, and child fact rules; 45 cumulative major clone findings remain, so the report is not claimed clean.

### US1 acceptance gate dependency findings (T063, open)

After T059–T062, the existing US1 acceptance scenarios were run without expectation changes:

```bash
just web-build
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='committed broker facts' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='committed broker facts|committed history across process boundaries' ./tests/e2e/
```

The frontend prerequisite passed. Memory ran five selected US1 scenarios: zero passed, five failed. The tagged lane ran seven selected scenarios: zero passed, seven failed. These are failed acceptance results, not completion evidence; T063 remains unchecked.

Observed failures include missing exchange gateway attribution; a rollback scenario waiting for an actual ledger OTLP copy before arming its storage fault; refusal actor/gateway mismatches; an absent-grant GET returning the existing HTTP 200 while the scenario expects 404; an expiry query finding no target fact; PostgreSQL child startup failure; and no target fact in the raw parity journey. Non-selected scenarios were excluded by the declared focus, not changed or skipped in source.

The rollback scenario at `business_event_ledger_test.go:659` requires successful ledger dispatch, whose implementation is explicitly assigned to T088–T092 and closure to T093. The current binding task graph nevertheless requires T063 before those tasks. Trusted-context integration is likewise assigned to T066–T067 after this gate. This is a task-order dependency conflict, not grounds to remove the telemetry/provenance assertions or mark the gate passed. The grant GET assumption also conflicts with the existing OpenAPI's HTTP 200 empty-array contract and requires correction rather than a production status change.

The two Ginkgo invocations ran concurrently. **[INFERENCE]** The child-start failure may come from their shared test-binary path being removed by the first completed invocation; future child/crash verification must run in an isolated binary location or sequentially. No child/crash pass is claimed from this run.

### Trusted-context and offline-schema regression authoring (T064–T065)

After the user approved the dependency-order correction, the disjoint context and builder-schema test slices were authored without changing shared acceptance criteria. Both compile and expose missing implementation:

```bash
go test -count=1 ./internal/domain/ledger ./internal/app \
  -run '^TestContextFacts|^TestBuilderBusinessEventSchemas'
```

The context tests fail on missing initiating caller/user/admin attribution and missing authoritative trace/span enrichment. They also protect domain-established subjects/delegation, explicit gateway association, nullable unavailable identity, allowlisted client metadata, and untrusted-session exclusion. The temporary identity declaration in `ledger/context.go` exists only to compile this red gate and must be replaced by T066.

The schema persistence tests fail at `Record`, not compilation or a placeholder assertion: the builder registry accepts the added offline type, but the existing storage registry still knows only the embedded catalogue. Invalid-source startup tests cover redefinition, namespace/source changes, remote references and flattened collisions; unknown/invalid payload tests retain an unchanged persisted control fact. T068 must integrate one compiled registry across the domain and storage validation paths without replacing existing event history.

### Trusted event context and existing HTTP capture reuse (T066)

`ledger.ContextFacts` preserves domain-established subject, delegation, caller category and gateway association while enriching the initiating caller from verified context. It never substitutes the represented SecurityContext actor for an initiating agent. Unavailable anonymous identities stay null. Request trace and span come only from authoritative capture with a valid matching OTel trace; client IP is normalized and user agents become fixed family values, never raw request text. Session-string provenance remains protected by the model's existing trusted-correlation mechanism.

The existing `internal/adapters/http/middleware/security_context.go` already captures authoritative IP/request trace and supports deferred identity finalization. It is reused instead of introducing a competing `business_event_context.go` capture path or changing existing slog fields. The compiling identity declaration from T064 is replaced by the real constructor.

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

The source was removed after proof. The scoped quality command returned success, not a claim that new-symbol debt is absent; producer integration and its larger quality review remain T067. The separately approved absent-grant acceptance correction preserves HTTP 200 and now checks that the revoked delegation is actually absent; a lint-requested tagged switch replaced its conditional chain without changing assertions.

### Trusted context across the full catalogue (T067)

The event constructor now requires its owning context and applies `ContextFacts` once for every typed recipe. All discovered callers were migrated, including tests; the old two-argument constructor is removed. Verified exchange/impersonation callers and gateway references remain separate from subjects. Delegation is recorded only after the owning domain establishes it; approval requests retain their verified gateway/represented-user relationship. Initial signing-key bootstrap uses the fixed lifecycle-system actor, while authenticated promotions retain their operator identity.

The complete 28-type memory HTTP catalogue journey now passes. Its credential-canary checks and exact action deltas are retained. Corrected fixture assumptions preserve the reviewed contracts: a missing-grant exchange refusal has no `on_behalf_of`; impersonation refusal includes its primary exchange-refusal companion; signing eligibility is compared against the precise stored instant, not the HTTP response's second-only formatting. No HTTP representation was changed to satisfy those assertions.

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

Static checks and all eleven matched producer/handler packages passed; builder tests passed with the unchanged T068 schema-seam regressions excluded. The focused catalogue run passed one acceptance scenario covering all 28 workflows. T063 is still open: this is not a claim that its rollback/expiry/parity/crash/delivery gate has passed.

An earlier concurrent package run failed the existing same-identity security-finalization assertion. A bounded diagnostic exercised that path and observed the expected access denial with finalized context; the final complete package run passed. No timeout, validation guard, or failing expectation was weakened, and the diagnostic source was removed. Scoped quality remains non-clean with sixteen major cumulative constructor/flow-shape findings; no unrelated policy/constructor framework was introduced.

### Offline schema registration through one compiled validator (T068)

The builder supplies its compiled registry to both storage validation paths through the storage composition facet. Atomic validator replacement retains existing events. The real builder extension, unsafe-source, and unknown/invalid-payload tests pass. The HTTP fixture schema now declares its fixed reasons. The pre-storage validation fixture uses a prepared copy rather than weakening the recorded-time invariant.

Verification: `just check`; complete app, ledger, and untagged PostgreSQL packages; memory BusinessEvent tests excluding the still-pending delivery regressions. The initial memory filter selected a T090 reference-list test and failed; the final focused run passed with that pending case excluded.

The throwaway production-builder smoke reported `production_builder_offline_type_retained=true shared_compiled_validation=true unknown_type_rejected=true no_schema_migration=true`. Its source was removed. Scoped quality retains seven cumulative major findings, including interface-dispatch reachability warnings; no clean-quality claim is made.

### Authorized investigation guide (T069)

`docs/api/business-event-ledger.md` publishes exact-subject and explicit no-subject SQL, receiving-agent-set interpretation, reader access, closed schemas, and credential-free examples. Documentation links resolve. Both OpenAPI documents remain unchanged. This feature adds no public ledger endpoint.

### Stored investigation verification and remaining delivery gates (T070, open)

The memory investigation run selected all five US2 scenarios. US2-AS1, US2-AS2, and US2-AS4 passed. US2-AS3 and US2-AS5 failed at their OTLP waits. T070 remains unchecked.

The request-context fix captures the actual HTTP server span before domain child spans start. The event retains that span only with its matching authoritative trace. The forged-token fixture expects the specific authentication denial and forbids a generic companion failure. It retains null subject, caller, delegation, and receiving-agent assertions.

PostgreSQL then exposed a grant-expiry conversion error. The timezone-free `valid_until` column received local wall time instead of the expiry instant. The adapter now converts create, upsert, and update parameters to UTC in SQL. A regression failed before this change for positive and negative timezone offsets. It passes after the change and preserves indefinite grants.

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

The temporary smoke source was removed after proof. Static checks and the real PostgreSQL expiry-marker regression group passed. All four affected package tests passed. This evidence proves stored behavior, not delivery. US2-AS3 and US2-AS5 remain open until the real exporter works. The user approved deferral of both OTLP checks to T093. T070 remains unchecked while lifecycle work proceeds.

Scoped structural quality reports four cumulative major duplication findings, including the three-line context-key wrapper. Existing PostgreSQL lookup/delete shapes account for two findings. The remaining middleware finding compares existing actor extraction with an approval callback. No clean-quality claim or unrelated refactor is made.

### Lifecycle regression authoring (T071–T074)

The four independent test slices compile and fail semantically before lifecycle implementation. Memory assertions expose no-op erasure, retention, cancellation, and collector handling. Domain assertions expose invalid selectors reaching storage. Builder assertions expose absent startup and scheduled sweeps. Helm assertions expose the missing CronJob. PostgreSQL assertions expose the absent erasure function, missing drops, unchanged policy, missing barriers, and readiness gaps.

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

The operational SQL eraser uses fixed qualified names, fresh VOLATILE reads, and lifecycle/subject locks before delivery-row locks. Its UTF-8 FNV-1a key matches the broker's existing subject gate. PUBLIC execution is revoked. Maintenance validates policy and paired metadata before transactional drops. Provisioning remains drop-free and repairs current/future windows.

Startup uses the shared upward-rounding normalization and applies policy only after valid construction. Memory completes its startup sweep before Build returns, runs five-minute sweeps independently of telemetry, and cancels/joins maintenance before other shutdown callbacks. PostgreSQL startup performs policy DML, never partition DDL. Existing storage health now requires both current partitions and recovers after scheduler repair.

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

Static checks, all four complete packages, focused memory lifecycle tests, and the real PostgreSQL lifecycle group passed. The PostgreSQL operational-privilege case was not selected in this intermediate run. Chart privilege verification remains T081/T083. An initial builder run exposed policy application before missing-encryption validation. Moving policy application after construction preserved that existing failure path, and the complete builder package then passed.

The temporary memory program exercised real production-builder startup, sub-microsecond normalization, telemetry-independent retention, exact committed erasure, and its unaffected-subject control. The PostgreSQL program applied real migrations and used the production builder. It called maintenance as migration owner, erased through an authorized role, committed, and queried through a separate reader role.

```text
production_builder_startup_retention=true positive_submicrosecond_rounding=true telemetry_independent=true exact_committed_erasure=true unaffected_subject_retained=true idempotent_erasure=true
real_postgres_production_builder=true migration_owner_retention_drop=true authorized_eraser_committed_count=1 reader_target_count=0 reader_unaffected_count=1 current_partition_readiness=true
```

These results do not close dispatch, recovery, chart scheduling, or full US3 acceptance. T083/T084 remain open.

### Scheduler, executable grants, and operations guide (T080–T082)

The PostgreSQL chart schedules five-minute migration-owned maintenance with bounded SQL and Job deadlines. Migration completion provisions partitions even without optional grants. Memory renders neither database Job. The maintenance template omits `spec.suspend`. Disabling migration Jobs requires external migration and scheduler procedures.

Default grants enforce parent-only event insertion, restricted history mutation, and no runtime execution of ledger functions. Future partition creation removes inherited child grants. A custom script retains its agreed full-replacement behavior and must provide its own restrictions. The operations guide documents scheduler installation, roles, erasure, coordinated policy changes, alerts, external copies, and rollback acknowledgement.

```bash
go test -count=1 ./tests/integration -run '^TestHelmTemplate_BusinessEvent'
just helm-lint
just helm-template
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventLifecycle_'
go run ./.ledger-postgres-smoke
```

Rendering tests, Helm lint/template, static checks, and the complete PostgreSQL lifecycle group passed. The real smoke executed rendered grants through `psql`. Reader and erasure role names contained apostrophes, double quotes, and backslashes. Runtime mutation/erasure stayed denied. New partitions stripped direct-write grants inherited from deliberately broad default table privileges.

```text
real_postgres_production_builder=true migration_owner_retention_drop=true rendered_grants_executed=true quoted_operational_roles=true runtime_mutation_and_erasure_denied=true future_partition_direct_writes_denied=true authorized_eraser_committed_count=1 reader_target_count=0 reader_unaffected_count=1 current_partition_readiness=true
```

The initial expanded smoke imported an unavailable YAML module. It was corrected to the repository's existing `go.yaml.in/yaml/v3`, without installing a dependency. Its successful source and temporary PostgreSQL container were removed after proof.

### US3 acceptance progress (T083/T084, open)

```bash
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' \
  --focus='retention and exact-subject erasure' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' \
  --focus='retention and exact-subject erasure' ./tests/e2e/
```

Memory passed both selected shared scenarios. The tagged lane passed four scenarios: exact erasure on both backends, invalid retention, physical retention boundaries, and concurrent predecessor/later-writer ordering. US3-AS4 failed at `WaitForLedgerAttempts` because real delivery is not implemented yet. No timeout or assertion was weakened. T083/T084 remain unchecked until the original delivery/restart and dispatch-dependent T072 assertions pass after T092.

SC-008 evidence remains separated by source. Tagged shared scenarios prove cross-backend erasure results. T072 non-dispatch lifecycle tests and the complete T074 builder-worker tests prove memory retention, cancellation, recording/erasure boundaries, and expiry-marker preservation. T090 must still close collector ordering, claims, and retry behavior.

### Synchronous delivery regression authoring (T085–T087)

All telemetry, worker, and dispatch tests compile before delivery behavior. The combined red gate exposes missing encoding/export acknowledgement, absent startup/periodic delivery, inert candidate selection, and missing claims/barriers. PostgreSQL dispatch tests fail at real candidate, synchronous callback, lock-order, acknowledgement-rollback, and retry assertions.

```bash
go test -count=1 ./internal/adapters/telemetry ./internal/app \
  ./internal/adapters/storage/postgres ./internal/adapters/storage/memory \
  -run '^(TestBusinessEvent(Exports|Distinguishes|DoesNotInherit|TraceWithout|Rejects|Overrides|Missing|Failed|Cancellation|Canceled|Delivery)|TestBuilderBusinessEvent|TestMemoryBusinessEvent(Queues|Rollback|Delivery|Collector|DeletedReference))'
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventDelivery_'
```

Both commands failed semantically. Their temporary declarations must be replaced by T088–T092, not delivered as working behavior. The acknowledgement-rollback test acquires the subject hint before business SQL, exports under a joined transaction, rolls back its owner, then requires a same-ID retry.

### Full-envelope encoding and actual synchronous export (T088–T089)

The non-global ledger provider uses the existing Resource construction and shared configured log-exporter factory. Ordinary slog still uses its unchanged batch processor. Ledger records use literal sorted attributes, explicit null/empty-object markers, typed arrays, occurred/emission timestamps, and empty body values. The processor verifies attribute count and zero drops, clears worker correlation, applies envelope trace/span, and captures the actual synchronous SDK result per call.

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

The initial smoke incorrectly required a nil protobuf body pointer. The SDK sends a present AnyValue with no value. The smoke now verifies that empty representation, matching the SDK test's EMPTY body contract. No production behavior or permanent expectation was changed for this observation. The smoke source was removed after proof. End-to-end recoverable delivery remains open until repository and worker integration.

### Retained-reference dispatch and ordered workers (T090–T092)

Both backends implement payload-free due scans and synchronous barrier-held dispatch. PostgreSQL claims with SKIP LOCKED after ordered lifecycle/subject gates. Memory keeps one claim per reference, releases business visibility during export, and retains lifecycle protection. Acknowledgement requires successful export and commit. Failed attempts retain the original ID and use the fixed 30-second retry.

The builder starts startup/one-second bounded scans, retries provider initialization, honors storage/export deadlines, and preserves disablement without backfill. Shutdown drains HTTP, cancels/joins delivery and memory maintenance, closes the ledger provider, finishes existing telemetry, then closes storage. The obsolete inert factory contract repository is removed.

```bash
just fmt
just check
go test -count=1 ./internal/app ./internal/domain/ledger ./internal/config \
  ./cmd/agentic-identity-broker ./internal/adapters/telemetry \
  ./internal/adapters/storage/memory ./internal/adapters/storage/postgres
go test -tags=integration -count=1 ./tests/integration/storage/infra \
  -run '^TestBusinessEventDelivery_|^TestBusinessEventLifecycle_'
```

Static checks, all seven complete packages, and both complete PostgreSQL lifecycle/delivery groups passed. PostgreSQL candidate timestamps now use UTC, preserving cross-backend key identity. The worker acknowledgement-contention fixture waits for bootstrap export before pausing its target attempt. It still requires a second copy with the same ID after failed acknowledgement.

The simplicity review removed redundant cancellation branches, impossible pending rechecks, and test-only provider/retention guards. Both worker shutdown paths use closed completion channels. No blanket quality suppression or unrelated transaction framework was introduced. Structural quality remains non-clean, including worker nesting and cumulative interface-dispatch reachability findings.

### Final non-performance acceptance and cross-story closure (T063/T070/T083/T084/T093)

```bash
ginkgo -v --procs=1 --fail-on-empty --label-filter='business-event-ledger && !performance' ./tests/e2e/
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' ./tests/e2e/
```

The complete memory lane passed 14 selected scenarios. The complete tagged lane passed all 20 non-performance scenarios. Shared scenarios exercised both backends. All 28 catalogue workflows and the offline reviewed fixture exported actual OTLP records. Legacy slog descriptor comparison, effective-copy switches, rollback withholding, concurrent winners, exact investigation, credential exclusion, erasure, retention, crash recovery, and no post-deletion replay passed.

Earlier acceptance runs exposed fixture assumptions, not grounds to weaken production contracts. Signing selection is distinct from eligibility during JWKS grace. Revoked grants retain the existing HTTP 200 empty-array response. Raw parity targets only new action IDs while comparing the complete event multiset. Rollback assertions exclude independent committed failure copies. Expiry timing uses the recording backend's authoritative clock. Recovery waits for the persisted 30-second retry before its unchanged ten-second copy assertion. A typed grant identity assertion now compares values rather than value versus pointer.

The recovery fixture uses bounded phase contexts and a separate controlled child-process lifetime. It preserves all copy, pending-reference, stable-ID, and rollback assertions. The original failed scenarios passed in focused runs, then the complete tagged lane passed. No user-visible HTTP contract or assertion timeout was widened.

### Actual command-binary broker/collector restart (T097)

```bash
just build
go run -tags=integration ./.ledger-runtime-smoke
```

The temporary program started the actual broker executable against a migrated PostgreSQL container and local OTLP receiver. It completed an admin HTTP workflow, observed a pending reference during collector rejection, stopped both broker and collector, then restarted them. The original event ID reached the receiver and its acknowledged reference disappeared.

```text
actual_broker_http_created=true collector_outage_pending=true broker_and_collector_restarted=true original_event_id_recovered=true successful_copy_acknowledged=true
```

The first request omitted required permission-set entries and returned the real HTTP 400 validation error. Reusing the existing seeded permission fixture corrected the smoke. The successful source and test container were removed after proof. Performance acceptance remains separate and requires the approved deployment profile.

### Final security regression corrections

The temporary-type erasure regression failed before the fix with PostgreSQL SQLSTATE 23514: the caller's `pg_temp.bytea` domain replaced the privileged function's local `bytea` type. The function now explicitly puts `pg_temp` after `pg_catalog` in its fixed search path.

Automatic-refresh success and failure regressions failed before the fix because delegated exchanges recorded actor kind `user` instead of the verified initiating agent. No-context and anonymous refreshes also invented the session subject as the caller. Token exchange now carries its established actor only after grant and requested-service authorization. Refresh keeps the session principal as subject, uses trusted initiating context, and records established delegation and the known gateway reference. Callback establishment and termination behavior remain unchanged.

```bash
go test -tags=integration -count=1 ./internal/domain/oauth2session \
  ./internal/domain/tokenexchange ./tests/integration/storage/infra \
  -run 'TestSessionLedgerAutomaticRefreshActor|TestExchangeLedgerDelegatedAutomaticRefreshActor|TestBusinessEventLifecycle_Erasure'
```

All three selected packages passed after both fixes. Regression controls include direct-user and peer-only contexts, successful and failed refresh, and real role-scoped PostgreSQL erasure with caller-created temporary types. These results do not establish the separate performance gate.

The temporary HTTP smoke used the production builder on both memory and PostgreSQL. Each backend exercised successful and rejected upstream refresh during an authenticated delegated token exchange. Stored facts and actual local OTLP copies identified `smoke-refresh-client` as actor, preserved the session principal as subject, and retained established delegation and the gateway reference.

```bash
ginkgo -v --tags=integration --procs=1 --fail-on-empty \
  --focus='Delegated automatic refresh runtime smoke' ./tests/e2e/
```

The corrected smoke passed all four backend/outcome paths. Its initial failure expected HTTP 400 for rejected upstream refresh; the unchanged token endpoint returns HTTP 500. No production status mapping or permanent acceptance expectation was changed. The temporary smoke source was removed after proof.

### Final compliance and regression gates

The final review confirmed unchanged OpenAPI/routing contracts, 28 published event types, and domain-to-ports dependency direction. It found no runtime DDL, custom telemetry port, or adapter cross-import. ADR 037 remains the accepted decision. ADR 033 remains Proposed. Frontend, design-system, Playwright, and screenshot changes do not apply to this feature.

Examples, role-value comments, and design headers now describe the implemented runtime rather than the planning phase. The configuration reference includes both optional operational roles and the custom-script replacement behavior. The approved deployment-profile performance gate remains open.

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

The first static gate found two unformatted test files. Formatting corrected them. The first full infrastructure run found a missed required retention value in `newLocalModePostgresConfig`. The fixture now supplies a valid retention policy. Production startup validation remains fail-closed.

The executable Helm grants regression passed default, colliding-role, and distinct-role cases. It runs rendered SQL through real container `psql`, including variable expansion and `\gexec`. Runtime writes to ledger history, direct partition access, and operational function execution remain denied. Parent insertion, pending-reference operations, and retention-policy updates remain permitted. CLI retention regressions reject explicit `0s` and `-1h` over valid file/environment values.

The source inventory contains 28 published examples and 21 unique scenario mappings. Relative file links in the reviewed documentation resolve. Website-root routes were not checked as filesystem links. These inventory results do not replace runtime acceptance.

The scoped structural quality check remains non-clean: 76 cumulative major findings against Git HEAD. Findings include cross-context clone matches and longer transactional workflows. No blanket suppression or unrelated abstraction was added.

```bash
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-lint
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just helm-template
ginkgo -v --procs=1 --fail-on-empty \
  --label-filter='business-event-ledger && !performance' ./tests/e2e/
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just test-e2e-ledger-postgres
```

Helm lint and template rendering passed. After the security fixes, the memory lane passed 14 selected scenarios and the tagged lane passed 20. US3-AS1 includes historical events for the erased subject across partitions. US4-AS1 requires ordinary non-ledger OTLP telemetry alongside enabled ledger copies.

The first `just verify` attempt stopped at three gosec G115 findings. The fixes reject out-of-range observed timestamps, compare reflected bytes without narrowing, and explicitly map the FNV subject key to signed range. The lock key remains compatible with the operational SQL eraser. No security-rule suppression was added.

The next verification attempt passed Go security checks but stopped at 15 npm advisories. The user approved targeted frontend/docs dependency updates. Installed versions are Axios 1.20.0, DOMPurify 3.4.16, js-yaml 5.4.1, and markdown-it 14.3.1. The js-yaml override applies only to markdownlint-cli2. Other documentation consumers retain compatible js-yaml 4.3.2.

Axios installation exposed an existing ESLint peer mismatch. Aligning `@eslint/js` with the installed ESLint 9.39.5 resolved it without `--force` or `--legacy-peer-deps`. The next security stage reported zero gosec issues, no called Go vulnerabilities, and no OSV issues.

```bash
npm run build # assets/docusaurus
```

The documentation build passed after source links adopted the existing public GitHub link convention. The first build identified repository-relative links that were not public site routes. The corrected build retains the strict broken-link gate. Its existing CSS minimizer and Node localStorage warnings remain unrelated to this feature.

An inline Node smoke exercised the installed DOMPurify, js-yaml, and markdown-it packages. DOMPurify removed executable attributes and URLs while retaining safe markup. YAML parsing preserved explicit false, and Markdown rendering produced the expected heading. No smoke file remains.

The full race gate found concurrent writes in the shared `ledgerfixture.Store` event slice. Fixture commits now protect that slice with a mutex. This changes test-fixture synchronization, not production transaction behavior.

The first reference measurement stopped at an incorrect agent-reference assertion for direct-user refresh. The [performance report](performance-results.md) records the observed baseline and the corrected verifier. No paired overhead pass is claimed from that attempt.

```bash
go test -race -count=1 ./internal/domain/tokenexchange
env PATH="/opt/homebrew/opt/helm@3/bin:$PATH" just verify
```

The focused race regression passed. The complete `just verify` gate then passed, including static/security scans, Go race tests, web tests, CDK tests, mock-service tests, both integration layers, and all functional E2E lanes.

| Final verification lane | Actual result |
|---|---|
| Web unit tests | 28 files, 246 tests passed |
| Backend functional E2E | 622 passed, only the performance scenario excluded |
| ExtProc E2E | 112 passed |
| Frontend browser E2E | 61 passed |
| Tagged ledger E2E | 20 passed, shared scenarios on both backends |
| Dependency/security scan | Zero gosec issues, no called Go vulnerabilities, no OSV issues |

This is local verification evidence, not a claim that GitHub CI ran. The separately controlled performance gate and approved deployment-profile requirement remain open.

After the retained event/reference CTE and approval mutex fixes, `just verify` passed again on the final code. The totals remained 246 web tests, 622 backend scenarios, 112 ExtProc scenarios, 61 browser scenarios, and 20 tagged ledger scenarios. Security scans remained clear.

The user chose to stop further performance tuning and report the blocked gate. The latest paired transaction-p99 difference is 9.371083 ms and recording p99 is 22.370709 ms, both above 5 ms. No approved deployment profile is available. T094, T095, T096, T107, and T109 remain unchecked. The [performance report](performance-results.md) contains the raw-report locations, experiments, and explicit limits.

The final scoped structural quality check remains non-clean, with four cumulative major findings against Git HEAD. Findings include similar approval terminal workflows. No suppression or unrelated refactor was added. All temporary smoke/diagnostic source files were removed. Raw measurement and profiling artifacts remain as evidence.

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
just test-e2e-ledger-postgres
```

The tagged recipe builds frontend assets, rejects empty selections, runs with one process, and participates in `just verify` and a dedicated CI job. Verify all 21 mapped scenario IDs across memory, tagged PostgreSQL and performance lanes, not simply a successful command exit. The tagged lane runs every shared scenario against both memory and PostgreSQL through the build-tag-selected backend list in `tests/e2e/bootstrap/`, and US1-AS7 compares the two backends directly. That run is the SC-008 evidence for event contents, query results and erasure results. US3-AS2/AS3/AS4 need database-clock seeding or a process restart and run on PostgreSQL only; their memory equivalence comes from the memory lifecycle and retention-worker tests (T072, T074). Record both evidence sources.

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
   | Operations owner / approver | Unavailable: no operator-approved profile supplied |
   | Approval date and reference | No operator approval. User response: `No approved profile available`, implementation conversation, 2026-09-27 |
   | Profile location | Not supplied |
   | Release acceptance | **Blocked (T095)** until an operator approves a profile and its required measurements pass |
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
