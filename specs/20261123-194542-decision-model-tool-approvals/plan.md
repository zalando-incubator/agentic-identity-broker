# Implementation Plan: Policy-Driven Tool Approvals with Decision Models

**Branch**: `claude/zealous-shannon-sz2gm1` | **Date**: 2026-11-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/20261123-194542-decision-model-tool-approvals/spec.md`

## Summary

ExtProc assesses a tool call only if the policy sends the call to human approval and no human
decision applies. It builds a small context, sends one request with fixed typed questions to a
decision endpoint, and evaluates the policy again with the answers. The call then proceeds under
a named rule, waits for a person with a factual summary, or is denied.

ExtProc learns tool definitions from the `tools/list` responses that pass through it. The broker
stores an optional assessment summary on a pending approval, and the approval page shows it. A
new subcommand of the ExtProc binary measures labeled cases and writes the evaluation record that
automatic approval requires.

[research.md](research.md) records the design decisions `D-01` to `D-13` with their alternatives.
The contracts are in [contracts/](contracts/).

## Technical Context

**Language/Version**: Go 1.27.4, React 19, TypeScript 6.0.x. Existing Go and React monorepo.

**Primary Dependencies**: Existing OPA library, `net/http` through `internal/extproc/httpclient`, `crypto/sha256`, `encoding/json`, `math`, Cobra and Viper, chi, sqlx and pgx, OpenTelemetry. The plan adds no dependency. It adds no OpenAI SDK and no gRPC protocol.

**Storage**: One nullable JSONB column on `tool_approvals` (migration `049`) with an equal field in the memory adapter. ExtProc keeps tool definitions, the catalog, and the evaluation record in memory only.

**Testing**: Go unit tests, the Ginkgo ExtProc suite with `httptest` doubles, the Docker suite with agentgateway, PostgreSQL integration tests, the Playwright suite, and Vitest.

**Target Platform**: The standalone ExtProc service behind agentgateway, the broker, and the existing SPA. The Helm chart does not deploy ExtProc and does not change.

**Performance Goals**: A call that the policy allows or denies gets no delay and no model request. An assessed call gets a maximum delay of `request_timeout` (500 ms by default) and one more policy evaluation (100 ms maximum). The copy of a `tools/list` response does not delay the response.

**Constraints**: A context of a maximum of 16 KiB. A listing of a maximum of 1 MiB. A maximum of 10,000 tool definitions for each instance. A maximum of 64 model requests in progress for each instance. No argument value in a log, span, or metric. Each failure keeps the call for a person or denies it.

**Scale/Scope**: 28 acceptance scenarios in five user stories. One new ExtProc package, one subcommand, one migration, two changed API schemas, and one new web component. Hundreds of target servers without configuration for each server.

## Constitution Check

The feature adds no domain entity with an identity. Thus, it adds no typed identifier. The
Assessment Summary is a value object on the existing `ToolApproval` aggregate.

| Principles | Decision and gate |
|---|---|
| I, III | Assessment and automatic approval are disabled by default. Each failure keeps the call for a person or denies it (D-04). The system conditions of FR-041 are Go code that a policy cannot disable (D-05). Fingerprints use `crypto/sha256` (D-06). The plan adds no cryptography of its own. The assurance case gets the new trust boundary (SR-011). |
| II, V | Write `adrs/20261124-212043-decision-model-assessment.md`. It records D-01, D-02, D-05, and D-07. It amends the policy contract of `adrs/028-opa-extproc-authorization.md` and the gate of `adrs/036-extproc-approval-cache-and-gate.md`. The rule "no approval records in the policy input" of that ADR stays. Add the terms of the spec to the glossary in `ARCHITECTURE.md`. Correct the stale `OPADecision` entry there, because this feature changes that type. |
| IV, X | The contracts are drafts. The stakeholder approved the direction of the spec but not these contracts. Approval of the four contracts of API-006 is a gate before implementation. Publish the schema change in `api/enduser/openapi.yaml` and the documentation in `docs/api/` before behavior. |
| VI, XII | ExtProc has its own structure under `adrs/011-extproc-standalone-binary.md`: packages under `internal/extproc/` and wiring in `cmd/extproc-token-exchange/root.go`. The new package has no import from broker adapters. In the broker, the handler passes the summary to `approval.Service`, which validates it. The repositories only store it. The broker needs no new wiring. |
| VII | New keys are in the ExtProc configuration schema with defaults and validation ([contracts/assessment-files.md](contracts/assessment-files.md)). Update `docs/configuration.md`, the examples, `configs/config.extproc.docker.yaml`, and `internal/extproc/AGENTS.md`. The Helm chart does not deploy ExtProc, thus it does not change. |
| VIII, XIII | Write one Ginkgo case for each of the 28 scenarios before behavior, with realistic assertions and no pending or skipped case. Write unit tests first for the parser, the context builder, the validation, the fingerprints, and the statistics. |
| IX | One atomic migration pair `049`. The two adapters store the summary. All SQL uses explicit columns. Integration tests apply the migration up, down, and up against PostgreSQL. |
| XI, XIV | One application component with semantic tokens and WCAG 2.1 AA ([contracts/approval-assessment.md](contracts/approval-assessment.md)). The spec has the journeys and the two wireframes. Playwright cases capture screenshots of each state. |
| XV | [contracts/observability.md](contracts/observability.md) defines the events, spans, and instruments. Add the `assessment` namespace to `coding-standards/OPENTELEMETRY.md`. Tool names obey section 6.2 of `coding-standards/LOGGING.md` (D-12). |

**Gate outcome**: The design has no violation of the constitution. Three prerequisites stay open
before implementation starts. These are unfinished work, not exceptions:

1. The stakeholder approves the four contracts of API-006.
2. The verification tasks of D-02, D-03, and D-07 complete, and the contracts agree with the results.
3. The ADR has the status Accepted.

## Project Structure

### Documentation (this feature)

```text
specs/20261123-194542-decision-model-tool-approvals/
├── spec.md
├── decision-model-research.md       # Guidance about decision models (R-01 to R-20)
├── plan.md                          # This file
├── research.md                      # Design decisions (D-01 to D-13)
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── policy-assessment.md         # Policy input and policy result
│   ├── decision-request.md          # Request to the decision endpoint
│   ├── assessment-files.md          # Configuration, catalog, evaluation record, evaluation run
│   ├── approval-assessment.md       # Approval API and approval page
│   ├── schemas.yaml                 # OpenAPI schema additions
│   └── observability.md             # Logs, spans, metrics
├── checklists/requirements.md
├── wireframes/
└── tasks.md                         # Output of /speckit-tasks
```

### Source areas

```text
internal/extproc/
├── assessment/                      # NEW: catalog, tool definition store, listing parser,
│                                    #      context builder, model client, answer validation,
│                                    #      fingerprints, evaluation record, automatic approval
│                                    #      conditions, evaluation run and its statistics
├── authorization/decision.go        # New result fields (D-05)
├── approval/gate.go, client.go      # Gate in two steps, summary in the create request (D-01)
├── config/                          # New keys, defaults, validation
└── server/server.go                 # Assessment in the approval branch, response observation
cmd/extproc-token-exchange/          # Wiring in root.go, new subcommand `assessment evaluate`

internal/domain/storage/tool_approval.go          # Assessment Summary value object
internal/domain/approval/service.go               # Validation, create, detail
internal/adapters/storage/{memory,postgres}/      # Store and return the summary
internal/adapters/http/handlers/approval/         # create and get DTOs
migrations/049_add_tool_approval_assessment.{up,down}.sql
api/enduser/openapi.yaml, docs/api/

web/src/components/approvals/AssessmentSummary.tsx   # NEW
web/src/components/approvals/ApprovalReviewPage.tsx
web/src/types/approval.ts

tests/e2e/extproc/, tests/e2e/frontend/, tests/e2e/pages/, tests/integration/
examples/config/, configs/, docs/configuration.md, docs/resources/assurance-case.md
ARCHITECTURE.md, adrs/, coding-standards/OPENTELEMETRY.md, internal/extproc/AGENTS.md
```

**Structure Decision**: The feature uses the existing structure. ExtProc gets one new package,
because the assessment is one cohesive subject with no existing owner. The package receives its
dependencies through constructors, and `root.go` connects them. The server calls the package
through one small interface, as it calls the authorizer and the gate today. The broker and the
SPA change only in existing files, apart from one new component and one migration pair.

Three seams need care:

- **The approval branch** (`server.go:589-607`). The new sequence is in D-01. `approval.Invocation` gets the target server name and the summary.
- **The response phase** (`server.go:406-415`). Today, the server discards the stream state after the request body. For a `tools/list` request, it keeps a small state and copies the response chunks (D-02). The bytes that the client receives do not change.
- **The decision parser** (`decision.go:23-59`). The `allow` case discards all other fields today. It must read `auto_approval.rule`. The audit switch in `finalizeDecision` must also change.

## Implementation Phase Overview

| Phase | Purpose |
|---|---|
| Phase 0 | Refactoring without a behavior change: divide `Gate.Evaluate` into "find an existing decision" and "create a pending approval". |
| Phase 1 | Setup: read the scoped `AGENTS.md` files, confirm the migration number, and prepare the test doubles. |
| Phase 2 | Design preconditions: the three verification tasks, the stakeholder approvals, the Accepted ADR, the glossary, the OpenAPI document, the configuration examples, the observability contract, and the 28 acceptance cases. |
| Phase 2.5 | Foundations in the new package, each with tests first: catalog, fingerprints, listing parser and store, context builder, model client, answer validation. |
| US2 | The model interface in the request path: context, disclosure, one request, no request for other calls. |
| US3 | Each failure status, the second identical call, and the system rules for the policy result. |
| US1 | The summary in the broker, the API, and the approval page. |
| US5 | The evaluation run, the report, and the evaluation record. |
| US4 | Automatic approval with the conditions of FR-041 and the audit record. |
| Phase N | Constitution compliance: documentation, assurance case, security review, full gate. |

- [x] Phase 0 (refactoring): Include. The gate does two tasks in one function today. A separate change lets reviewers approve the structure before the feature.
- [x] Phase 2.7 (entity boilerplate): Skip. The feature adds no entity, no handler, and no repository.

The sequence of the user stories differs from their numbers for one reason. US2 and US3 supply
the request path that US1 shows to the approver. US5 writes the evaluation record that US4 needs.
The priorities of the spec stay: US1 to US3 form the first release, and automatic approval stays
disabled until US5 and US4 complete.

## Testing Strategy

### Canonical acceptance mapping

Each row is one future Ginkgo `It` with its scenario identifier. The paths are design targets.
The tests do not exist at this time.

| Scenario | Suite | Observable result |
|---|---|---|
| US1.1 | Browser | External editor share: the tool double gets no call, and the page shows `critical`, the attention text, and the statement with "Indicated". |
| US1.2 | Browser | Internal reader share with `routine`: the page shows the routine text and the findings, and the approval is pending. |
| US1.3 | Browser | Each assessment text equals a catalog text or a product text. The page has no number from an answer. The technical details list all arguments. |
| US1.4 | ExtProc | Assessment disabled: the model double gets no request, and the create request equals the request of today byte for byte. |
| US2.1 | ExtProc | One model request with the organization text, the server name, the listed definition, the permitted values, and `file_id` as withheld. |
| US2.2 | ExtProc | The request body has no principal, agent identifier, session identifier, credential, risk level, policy description, or other tool. |
| US2.3 | ExtProc | An undescribed argument is absent from the request, the count is 1, and the call waits in each mode. |
| US2.4 | ExtProc | An excluded server and a server without a listing: no model request, and each create request has `not_available`. |
| US2.5 | ExtProc | Two requests with equal bytes for the questions and the first three sections. The injected text is only in `call`. |
| US2.6 | ExtProc | Startup fails for each invalid catalog, and the error names the question or the key. |
| US2.7 | ExtProc | Calls with an `allow` or `deny` policy result cause no model request. |
| US2.8 | ExtProc | An existing permanent denial and an existing session approval apply without a model request. |
| US2.9 | ExtProc | A replacement description is in the request with `written_by: operator`. |
| US3.1 | Browser | Timeout in the `auto_approve` mode: the approval is pending, and the page shows the "no assessment" text. |
| US3.2 | ExtProc | A refusal, a missing answer, and an invalid probability each give `incomplete` and a create request with `not_available`. |
| US3.3 | ExtProc | Endpoint not available: the create request arrives at the broker double as today. |
| US3.4 | ExtProc | A second identical call causes no second model request and gets the same approval URL. |
| US3.5 | ExtProc | With `incomplete`, an `allow` result with a rule and a `routine` value both end as a pending approval without a recommendation. |
| US4.1 | ExtProc | The call gets to the tool double, the broker double gets no create request, and one audit record has the rule and no argument value. |
| US4.2 | ExtProc | The external editor share waits for a person. |
| US4.3 | ExtProc | A different answering model: the call waits, and the audit record names the two models. |
| US4.4 | ExtProc | A new listing changes the fingerprint: the call waits, and a call to a second evaluated tool proceeds. |
| US4.5 | ExtProc | The `recommend` mode: the call waits, and the record has `would_auto_approve: true`. |
| US4.6 | ExtProc | An `allow` result without a rule name: the call waits. |
| US4.7 | ExtProc | A tool that the evaluation record does not name: the call waits. |
| US5.1 | ExtProc | The run calls no tool double and no broker double, and the report has each required value. |
| US5.2 | ExtProc | The evaluation record names the model, the question set, and each tool context. |
| US5.3 | ExtProc | A "harmless" answer for a `human` case is in the report as a false approval with the rule name. |

ExtProc cases are in `tests/e2e/extproc/tool_assessment_test.go` and
`tests/e2e/extproc/assessment_evaluate_test.go`. They use the existing bootstrap with the broker
double (`bootstrap/broker.go`) and a new decision endpoint double in the same directory. The test
sends response-phase messages with a `tools/list` result to supply definitions. Browser cases are
in `tests/e2e/frontend/approval_assessment_ui_test.go` with the existing page object
`tests/e2e/pages/approval_page.go`. They use the ExtProc environment of the journey bootstrap, as
`tests/e2e/extproc_approval_journey_test.go` does, with the real broker.

New fixtures: one catalog, the policy of the spec under `tests/e2e/extproc/fixtures/policies/`,
one evaluation record, one case file, and one tool listing. All values are synthetic.

### Verification with agentgateway

Three Docker cases in `tests/e2e/extproc/opa_agentgateway_e2e_test.go` prove the assumptions of
the design. They are not acceptance scenarios.

| Decision | Proof |
|---|---|
| D-02 | ExtProc receives the body of a `tools/list` response through agentgateway, learns the definition, and the client receives the response without a change. |
| D-03 | A Decisions request with an `input_json` part gets to an OpenAI-format double and to a System One-format double behind agentgateway. |
| D-07 | The sync response of the real broker contains an unexpired pending approval. |

### Unit, integration, and browser proof

Unit tests come first for each of these subjects:

- The listing parser: JSON and event-stream bodies, pages, an error result, a size at the limit and one byte more, and bodies that are not valid.
- The store: the latest definition applies, the limit, and the change report.
- The context builder: each disclosure setting, name patterns, oversized values, undescribed arguments, and a constant byte sequence for equal input.
- The answer validation: each status of D-04 and the tolerance of 0.01.
- The fingerprints: each field of the canonical documents changes the value, and key sequence does not.
- The decision parser and the system rules of [contracts/policy-assessment.md](contracts/policy-assessment.md).
- The statistics: the exact bound against known values, including 0 of 410 and 1 of 410.
- The configuration: defaults, each validation rule, and each invalid catalog.

A fuzz test covers the listing parser, because it reads data from MCP servers.

Integration tests cover the migration in the two directions and the summary in the PostgreSQL
repository. The memory repository has the same cases. Handler tests cover each validation rule
of the summary and the unchanged summary after a second create request.

Vitest covers each row of the content table in
[contracts/approval-assessment.md](contracts/approval-assessment.md). Playwright captures
screenshots of the attention state, the routine state, and the "no assessment" state under
`tests/e2e/screenshots/`.

One test reads all log records and spans of the ExtProc cases. It makes sure that no record
contains an argument value, a definition text, or a question text of the fixtures.

**Coverage goals**: 100% of the acceptance scenarios. Each rule of the contracts has a minimum of
one unit test. Run the checks of [quickstart.md](quickstart.md).

## Complexity Tracking

The plan requests no exception to the constitution. Four items add complexity, and each has a
reason:

| Item | Reason | Simpler alternative and its problem |
|---|---|---|
| Observation of response bodies in ExtProc | The model needs the tool definition, and the operator cannot describe hundreds of servers. | A catalog of tool descriptions. The stakeholder rejected it. |
| A second policy evaluation | The policy stays the only decision source. | A decision in Go code from the answers. The operator cannot review or audit it as policy. |
| A subcommand in the ExtProc binary | The measurement must use the production code path. | A separate tool. It can differ from production without a visible sign. |
| Fingerprints and an evaluation record | A changed model, question, or definition must stop automatic approval. | A flag that the operator sets by hand. It does not find a changed definition. |

The plan does not add these items:

- A registry of servers or tools in the broker
- A cache of positive results
- A reload of files while the service operates
- A second gRPC protocol
- A new dependency.
