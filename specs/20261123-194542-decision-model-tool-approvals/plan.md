# Implementation Plan: Policy-Driven Tool Approvals with Decision Models

**Branch**: `claude/zealous-shannon-sz2gm1` | **Date**: 2026-11-24, scope narrowed on 2026-11-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/20261123-194542-decision-model-tool-approvals/spec.md`

## Summary

ExtProc assesses a tool call only if the policy sends the call to human approval and no human
decision applies. It builds a small context, sends one request with fixed typed questions to a
decision endpoint, and evaluates the policy again with the answers. The call then proceeds
without a person, waits for a person, or is denied.

A call that waits for a person gets the description and the risk level that the policy derived
from the answers. ExtProc sends the two values in the existing fields of the approval request.
The broker, its API, its database, and the approval page do not change (D-16).

ExtProc learns tool definitions from the `tools/list` responses that pass through it. The
context has the tool definition as the server listed it and the call as the agent sent it. The
organization values and the questions are keys of the ExtProc configuration, with the existing
loader, defaults, and validation. The default value of the questions is the default question
set of the spec (D-17).

The feature does not record cases, measure a model, or tune questions and thresholds. It has no
evaluation run and no evaluation record (D-18).

[research.md](research.md) records the design decisions `D-01` to `D-19` with their alternatives
and the verification record of 2026-11-24.
The contracts are in [contracts/](contracts/).

## Technical Context

**Language/Version**: Go 1.27.4. The React application does not change.

**Primary Dependencies**: Existing OPA library, `net/http` through `internal/extproc/httpclient`, `encoding/json`, Cobra and Viper, OpenTelemetry. The plan adds no dependency. It adds no OpenAI SDK and no gRPC protocol.

**Storage**: None. ExtProc keeps tool definitions in memory only. The feature adds no migration (DB-001).

**Testing**: Go unit tests and the Ginkgo ExtProc suite with `httptest` doubles and agentgateway containers.

**Target Platform**: The standalone ExtProc service behind agentgateway `v1.7.0` or later (D-14). The Helm chart does not deploy ExtProc and does not change.

**Performance Goals**: A call that the policy allows or denies gets no delay and no model request. An assessed call gets a maximum added delay of `request_timeout` (500 ms by default) and one more policy evaluation (100 ms by default). The ExtProc filter of agentgateway has no message timeout of its own, thus the deadlines of ExtProc limit the delay. A request timeout of a route or of the client can still end the call (D-15). The copy of a `tools/list` response does not delay the response. A warm local `winnow:e4b` model needed 762 to 1,247 ms, thus the documentation tells operators to measure the model latency before they set `request_timeout` (D-15).

**Constraints**: A context of a maximum of 16 KiB. A listing of a maximum of 1 MiB. A maximum of 10,000 tool definitions for each instance. A maximum of 64 model requests in progress for each instance. No argument value in a log, span, or metric of the feature. Each failure keeps the call for a person or denies it.

**Scale/Scope**: 20 acceptance scenarios in four user stories. One new ExtProc package. No subcommand, no migration, no API change, and no change to the web application. Hundreds of target servers without configuration for each server.

## Constitution Check

The feature adds no domain entity and no typed identifier. The broker does not change.

| Principles | Decision and gate |
|---|---|
| I, III | The default mode is `disabled`, and automatic approval needs the mode `auto_approve` (D-19). Each failure keeps the call for a person or denies it (D-04). The system conditions of FR-041 are Go code that a policy cannot disable (D-05). The plan adds no cryptography. The assurance case gets the new trust boundary (SR-011). |
| II, V | Write `adrs/20261124-212043-decision-model-assessment.md`. It records D-01, D-02, D-03, D-05, D-07, D-14, D-12, D-15, D-16, D-17, D-18, and D-19. It records that the leaf package `internal/domain/approval/toolpattern` of `adrs/035-shared-tool-pattern-matching.md` also owns the arguments hash. The import rule of that ADR stays. It supersedes in part two Accepted ADRs: the request flow and the policy contract of `adrs/028-opa-extproc-authorization.md`, and the gate of `adrs/036-extproc-approval-cache-and-gate.md`. The new ADR names each superseded decision. The two older ADR files do not change (Principle II). The rule "no approval records in the policy input" of `adrs/036-extproc-approval-cache-and-gate.md` stays. Add the terms of the spec to the glossary in `ARCHITECTURE.md`. Correct the stale `OPADecision` entry there, because this feature changes that type. |
| IV, X | The feature changes no OpenAPI document (API-001). The three contracts of API-006 are drafts. The stakeholder approved the direction of the spec but not these contracts. Approval of the contracts is a gate before implementation. |
| VI, XII | ExtProc has its own structure under `adrs/011-extproc-standalone-binary.md`: packages under `internal/extproc/` and wiring in `cmd/extproc-token-exchange/root.go`. The new package has no import from broker adapters. The broker gets no change and no new wiring. |
| VII | New keys are in the ExtProc configuration schema with defaults and validation ([contracts/assessment-configuration.md](contracts/assessment-configuration.md)). They include the organization values and the questions with their default. The plan adds no file format for these values. Update `docs/configuration.md`, the examples, `configs/config.extproc.docker.yaml`, and `internal/extproc/AGENTS.md`. The documentation states the minimum agentgateway release `v1.7.0` and the gateway requirements of [contracts/decision-request.md](contracts/decision-request.md). The Helm chart does not deploy ExtProc, thus it does not change. |
| VIII, XIII | Write one Ginkgo case for each of the 20 scenarios before behavior, with realistic assertions and no pending or skipped case. Write unit tests first for the parser, the context builder, the validation, the automatic approval conditions, and the telemetry. |
| IX | The feature adds no migration, no column, and no repository change (DB-001). |
| XI, XIV | The feature changes no page and no component, thus the design system rules have no subject and the spec needs no wireframe. The spec has three journeys with configuration, HTTP exchanges, and a table of the page values before and after. |
| XV | [contracts/observability.md](contracts/observability.md) defines the one event, the span, and the instruments. Add the `assessment` namespace to `coding-standards/OPENTELEMETRY.md`. The feature writes no audit record of its own (D-12), thus it needs no change of `coding-standards/LOGGING.md`. The tests that the two standards require come before the telemetry code: the credential in log records and spans, and the attribute set of each instrument. |

**Gate outcome**: The design has no violation of the constitution. The verification of D-02,
D-03, D-07, D-14, and D-15 is complete, and the contracts agree with the results. Two prerequisites stay open
before implementation starts. These are unfinished work, not exceptions:

1. The stakeholder approves the contracts of API-006.
2. The ADR has the status Accepted.

## Project Structure

### Documentation (this feature)

```text
specs/20261123-194542-decision-model-tool-approvals/
├── spec.md
├── decision-model-research.md       # Guidance about decision models (R-01 to R-20)
├── plan.md                          # This file
├── research.md                      # Design decisions (D-01 to D-19) and verification record
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── policy-assessment.md         # Policy input and policy result
│   ├── decision-request.md          # Request to the decision endpoint
│   ├── assessment-configuration.md  # Configuration keys and the question set
│   └── observability.md             # Logs, spans, metrics
├── checklists/requirements.md
└── tasks.md                         # Output of /speckit-tasks
```

### Source areas

```text
internal/extproc/
├── assessment/                      # NEW: tool definition store, listing parser, context
│                                    #      builder, model client, answer validation,
│                                    #      automatic approval conditions
├── authorization/decision.go        # approval_context on an allow result (D-05)
├── approval/gate.go                 # Gate in two steps (D-01)
├── approval/cache.go                # Lookup of a matching pending approval (D-07)
├── config/                          # New keys, defaults (with the default questions), validation
└── server/server.go                 # Assessment in the approval branch, response observation
cmd/extproc-token-exchange/root.go   # Wiring

internal/domain/approval/toolpattern/              # Arguments hash with its canonical form (D-07)

tests/e2e/extproc/
docker-compose.yml, tests/e2e/**/agentgateway*_test.go, .github/workflows/   # agentgateway v1.7.0 at each pin site (D-14)
mocks/agentgateway/config.yaml       # decision endpoint example in the llm section
examples/config/, configs/, docs/configuration.md, docs/resources/assurance-case.md
examples/config/extproc-tool-assessment.yaml   # each key, with the default questions in full (FR-024)
examples/config/assessment-policy.rego      # example policy of the spec journeys
docs/guides/assess-tool-calls.md           # NEW: authoring rules with examples (FR-027, FR-039), SR-010 notice
ARCHITECTURE.md, adrs/, coding-standards/OPENTELEMETRY.md, internal/extproc/AGENTS.md
```

The plan changes no file under `internal/adapters/`, `internal/domain/storage/`, `migrations/`,
`api/`, `docs/api/`, or `web/`.

**Structure Decision**: The feature uses the existing structure. ExtProc gets one new package,
because the assessment is one cohesive subject with no existing owner. The package receives its
dependencies through constructors, and `root.go` connects them. The server calls the package
through one small interface, as it calls the authorizer and the gate today.

Four seams need care:

- **The approval branch** (`server.go:589-607`). The new sequence is in D-01. The gate gets the description and the risk level of the second evaluation in the existing fields of `approval.Invocation`.
- **The response phase** (`server.go:406-415`). Today, the server discards the stream state after the request body. For a `tools/list` request, it keeps a small state and copies the response chunks (D-02). The verification showed that the request and the response of one exchange use one gRPC stream. The bytes that the client receives do not change.
- **The pending lookup** (`cache.go:332-344`). `Record.matchable` accepts only approved records. The lookup for a pending approval is a separate function, so that a pending record never lets a call proceed (D-07).
- **The decision parser** (`decision.go:23-59`). The `allow` case discards all other fields today. It must read the `approval_context`. The audit switch in `finalizeDecision` must also change.

## Implementation Phase Overview

| Phase | Purpose |
|---|---|
| Phase 0 | Refactoring without a behavior change: divide `Gate.Evaluate` into "find an existing decision" and "create a pending approval". Move the arguments hash to the leaf package `internal/domain/approval/toolpattern`, which ExtProc can import (D-07). |
| Phase 1 | Setup: read the scoped `AGENTS.md` files and prepare the test doubles. Change the pinned agentgateway release to `v1.7.0` at each pin site of D-14: the Compose image with its index digest, the two test defaults, the image test, and the three CI workflows. Run the existing agentgateway suites in `tests/e2e/` and `tests/e2e/extproc/` and the verification cases. Change the report entries of the verification cases into assertions (see "Verification with agentgateway"). |
| Phase 2 | Design preconditions: the stakeholder approvals, the Accepted ADR, the glossary, the configuration examples, the review of the default question set, the observability contract, and the 20 acceptance cases. |
| Phase 2.5 | Foundations, each with tests first: configuration keys with the default questions, listing parser and store, context builder, model client, answer validation, decision parser, and the automatic approval conditions of FR-041. |
| US2 | The model interface in the request path: context, one request, no request for other calls. |
| US3 | Each failure status, the second identical call, and the system rules for the policy result. |
| US1 | The four modes. The description and the risk level of the second evaluation in the approval request. The example policy. The authoring rules for questions and for policy descriptions. |
| US4 | Automatic approval with the conditions of FR-041. |
| Phase N | Constitution compliance: documentation, assurance case, security review, full gate. |

- [x] Phase 0 (refactoring): Include. The gate does two tasks in one function today. A separate change lets reviewers approve the structure before the feature.
- [x] Phase 2.7 (entity boilerplate): Skip. The feature adds no entity, no handler, and no repository.

The sequence of the user stories differs from their numbers for one reason. US2 and US3 supply
the request path that US1 uses for the approver.

One pull request delivers the full feature. The phases give the sequence of the work in that
pull request. Automatic approval is a part of the delivery and stays disabled by default.

**The default questions have no measurement.** A measurement of a model, of the questions, or of
a policy is out of scope (D-18). The wording of three default questions changed after a smoke
test, and no model measured the new wording (D-17). A design review compares each default
question with the authoring rules before the question gets into code. The documentation states
that the questions and the example thresholds have no measured results, and that the operator is
responsible for the thresholds of each automatic approval rule (SR-010).

## Testing Strategy

### Canonical acceptance mapping

Each row is one future Ginkgo `It` with its scenario identifier. The paths are design targets.
The tests do not exist at this time. All cases use the ExtProc suite with the broker double.

| Scenario | Observable result |
|---|---|
| US1.1 | External editor share: the tool double gets no call. The create request at the broker double has `risk_level: critical` and the description "Share a Drive file with a party outside the organization". |
| US1.2 | Internal reader share in the `guidance` mode: the create request has `risk_level: low` and the description "Share a Drive file inside the organization". |
| US1.3 | The mode `disabled`: the model double gets no request, and the create request equals the request of today byte for byte. |
| US1.4 | The `shadow` mode: one model request, and the create request equals the request of today. |
| US2.1 | One model request with the default questions, the organization values, the server name, the listed definition, and the tool name and the arguments as the agent sent them. |
| US2.2 | The request body has no principal, agent identifier, session identifier, credential, risk level, policy description, or other tool. |
| US2.3 | Arguments that make the context larger than the limit: no model request, and the call waits in each mode. |
| US2.4 | A server without a listing: no model request, and the create request equals the request of today. |
| US2.5 | Two requests with equal bytes for the questions and the first three sections. The injected text is only in `call`. |
| US2.6 | Startup fails for an invalid question in the configuration and for a missing organization value. The error names the question or the key. |
| US2.7 | Calls with an `allow` or `deny` policy result cause no model request. |
| US2.8 | An existing permanent denial and an existing session approval apply without a model request. |
| US3.1 | Timeout in the `auto_approve` mode: the create request has the description and the risk level of the policy for the status `timeout`. |
| US3.2 | A refusal, a missing answer, and an invalid probability each give `incomplete` and a create request. |
| US3.3 | Endpoint not available: the create request arrives at the broker double as today. |
| US3.4 | A second identical call causes no second model request and gets the same approval URL. |
| US3.5 | With `incomplete`, an `allow` result ends as a pending approval. |
| US4.1 | The call gets to the tool double, and the broker double gets no create request. |
| US4.2 | The external editor share waits for a person. |
| US4.3 | The `guidance` mode with an `allow` result after the assessment: the call waits. |

The cases are in `tests/e2e/extproc/tool_assessment_test.go`. This file contains only the 20
scenario cases. The signal test is in `tests/e2e/extproc/assessment_signals_test.go`. It is not
an acceptance scenario. The cases use the existing bootstrap with the broker double
(`bootstrap/broker.go`) and a new decision endpoint double in the same directory. The test sends
response-phase messages with a `tools/list` result to supply definitions. The messages use the
observed gateway form: `text/event-stream`, one `data:` line, and one empty end-of-stream chunk
(D-02).

The feature changes no page, thus it adds no browser case. The existing journey test
`tests/e2e/extproc_approval_journey_test.go` already proves that the broker stores and shows the
description and the risk level of a create request.

The ExtProc suite uses the constructor that `root.go` uses for the assessment parts, so that the
tests do not copy the wiring.

New fixtures: the organization values in the test configuration, the policy of the spec under
`tests/e2e/extproc/fixtures/policies/`, and one tool listing. The cases use the default question
set. One fixture is a configuration with an invalid question for US2.6. All values are
synthetic.

### Verification with agentgateway

The verification cases of 2026-11-24 stay in the suites. They are not acceptance scenarios. The
results are in the verification record of [research.md](research.md).

Some results of that record are report entries that a person read. Phase 1 changes them into
assertions on the supported image, so that a passing run proves them:

- The first listing page has a `nextCursor` value. The case follows it and finds a known tool of the second page.
- The content type, the event framing, and the compression of a listing response have the values that the listing parser needs.
- The request with two `input_text` parts gets to the OpenAI-format double and to the System One-format double. The case compares the full translated body and the credential.
- The response `model` value arrives without a change. The request and the response use different model names.
- A missing container runtime makes the cases fail. It does not skip them.

| Decision | File | Proof |
|---|---|---|
| D-02, D-15 | `tests/e2e/extproc/assessment_gateway_verification_test.go` | ExtProc receives the body of a `tools/list` response on the stream of the request. The definition is intact, and the client receives the bytes that ExtProc returned. Compression, pages, two targets on one route, and a 6 s delay in the request-body phase. |
| D-03, D-14 | `tests/e2e/extproc/assessment_decision_endpoint_verification_test.go` | The text form gets to an OpenAI-format double and to a System One-format double behind agentgateway `v1.7.0`. An `input_json` part fails for System One. One instance serves `llm` and `binds`. |
| D-07 | `tests/e2e/approval_api_test.go` | The sync response of the real broker contains an unexpired pending approval with exact patterns. A duplicate create returns it without a change. |

### Unit proof

Unit tests come first for each of these subjects:

- The questions: the default value equals the questions of the spec and of the example file and obeys each rule, a list in the configuration replaces the full default list, and each invalid question stops the startup.
- The listing parser: event-stream bodies with one or more events, JSON bodies, pages, a result with unknown fields, an error result, a size at the limit and one byte more, and bodies that are not valid. A tool without an input schema.
- The store: the latest definition applies, the limit, the removal of all definitions of a server after a listing that ExtProc cannot read, and the marker for an oversized definition.
- The context builder: the tool definition and the arguments without a selection or a change, an argument that the input schema does not list, sorted map keys, a context at the size limit and one byte more, and a constant byte sequence for equal input. A call without arguments.
- The answer validation: each status of D-04, the tolerance of 0.01, level entries in a different sequence, a missing or duplicate level `value`, a wrong level `label`, and each invalid `model` value.
- The decision parser and the system rules of [contracts/policy-assessment.md](contracts/policy-assessment.md): the `approval_context` of an `allow` result after a downgrade, and the fallback to the first result after an error in the second evaluation.
- The gate: the pending lookup by tool name and `arguments_hash` (a call with an added argument does not match), the read-only lookup for a permanent denial, and the create request of the reuse path with the result of the first evaluation.
- The arguments hash in `internal/domain/approval/toolpattern`: `1`, `1.0`, `1e0`, and numbers in nested values give one hash through the decoder of ExtProc and through the decoder of the broker. The hash of each existing broker value does not change.
- The authorizer: each evaluation outcome (a result, an undefined result, an error, a timeout) with the prepared query and with the SDK.
- The modes: each of the four modes, with the result that the system uses in the modes `shadow` and `guidance` (FR-009).
- The automatic approval conditions: each condition of FR-041. A permanent denial gives a denial (FR-042).
- The telemetry: the startup event, the span name with its attributes and status, the attribute set of each instrument, and no tool name in a signal of the feature.
- The server: a batched MCP request and a request that is not MCP cause no assessment. The create request has no field that the request of today does not have (FR-038).
- The failure conditions of SC-002: a minimum of 20 conditions in the `auto_approve` mode with a policy that allows the call. No condition lets the call proceed.
- The configuration: defaults, each validation rule, and a missing organization value.
- The example policy: the OPA library evaluates it against the inputs of journeys 1 to 3, and each description is a constant text of the policy.

A fuzz test covers the listing parser, because it reads data from MCP servers.

One test reads all log records, spans, and metric attributes of the ExtProc cases. It makes sure
that no signal contains an argument value, a definition text, a question text, or the credential
of the fixtures (FR-061). The task list writes this test before the telemetry code.

**Coverage goals**: 100% of the acceptance scenarios. Each rule of the contracts has a minimum of
one unit test. Run the checks of [quickstart.md](quickstart.md).

## Complexity Tracking

The plan requests no exception to the constitution. Two items add complexity, and each has a
reason:

| Item | Reason | Simpler alternative and its problem |
|---|---|---|
| Observation of response bodies in ExtProc | The model needs the tool definition, and the operator cannot describe hundreds of servers. | A catalog of tool descriptions. The stakeholder rejected it. |
| A second policy evaluation | The policy stays the only decision source. | A decision in Go code from the answers. The operator cannot review or audit it as policy. |

The plan does not add these items:

- A change to the broker, its API, its database, or the approval page
- A list of findings, a recommendation, or a notice for the approver
- A catalog file, a question set file, or a key for an individual server or tool
- A filter or a setting that withholds an argument from the model
- An evaluation run, an evaluation record, a case set, or a measurement of a model
- A fingerprint of the questions or of a tool definition
- A registry of servers or tools in the broker
- A cache of positive results
- A reload of files while the service operates
- A second gRPC protocol
- A new dependency.
