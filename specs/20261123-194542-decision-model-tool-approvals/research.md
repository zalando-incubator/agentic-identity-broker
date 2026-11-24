# Research: Design Decisions for Decision Model Assessment

**Date**: 2026-11-24

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

This document records the design decisions of the planning phase. The guidance about decision
models is in [decision-model-research.md](decision-model-research.md) (`R-01` to `R-20`). This
document refers to its own decisions as `D-01` to `D-13`.

Each decision used the code of this repository and published documentation. No decision used a
test against a running gateway or model. Thus, three decisions have a verification task before
the contracts get approval (D-02, D-03, D-07).

## D-01 — The assessment runs in the request-body phase of ExtProc

**Decision**: ExtProc assesses a call in the `approval_required` branch of `processRequestBody`
(`internal/extproc/server/server.go:589-607`). A new package `internal/extproc/assessment/`
contains the context builder, the model client, the answer validation, and the fingerprints. The
sequence for one call is:

1. ExtProc evaluates the policy without an assessment.
2. If the result is `approval_required`, the gate looks for an existing human decision.
3. If no decision applies, ExtProc builds the context and sends one model request.
4. ExtProc evaluates the policy again with `input.assessment`.
5. ExtProc applies the conditions for automatic approval.
6. The call proceeds, or the gate creates a pending approval with the summary, or ExtProc denies the call.

**Rationale**: The branch has all necessary data in scope: the parsed tool call, the target
server name, the principal, and the agent. The policy stays the only decision source. Calls that
the policy allows or denies cause no model request (FR-002).

**Consequence**: `Gate.Evaluate` (`internal/extproc/approval/gate.go:54-120`) does two tasks
today. It looks for an existing decision, and it creates the pending approval. The plan divides
it into two steps. This change is a refactoring without a behavior change (Phase 0).

**Alternatives considered**: A policy can call the model with `http.send`. This puts request
construction and untrusted text into each policy. It also removes the system conditions of FR-041
and cannot use the 100 ms evaluation timeout. An assessment in the broker is against the
direction of issue #206. A gateway webhook moves the policy contract out of this repository.

## D-02 — ExtProc learns tool definitions from `tools/list` responses

**Decision**: ExtProc reads the response body of each `tools/list` request that has a resolved
`target_server_name`. It keeps the stream state for these requests. It copies each response chunk
into a bounded buffer and returns the chunk to the gateway without a change. At the end of the
stream, it parses the result and stores each tool definition in memory. The key is the target
server name and the tool name.

The parser accepts `application/json` and `text/event-stream`. In an event stream, it uses the
JSON-RPC response that has the identifier of the request. It ignores a response that has an error
status, a content encoding, or a size larger than the limit. A listing with a cursor adds or
replaces only the tools on that page.

**Rationale**: ExtProc is in the path of each MCP request. The agentgateway documentation says
that the gateway streams response bodies to ExtProc by default. The existing configurations set
no processing mode, and `echoResponseBody` (`server.go:1403`) receives the chunks today. Thus,
the gateway configuration does not change. The `mcp_server` metadata value is mandatory for MCP
requests (`adrs/028-opa-extproc-authorization.md`, amendment of 2026-09-21). Thus, each listing
has one target server name.

**Risks**:

- Definitions apply to all users of one instance. A server can return a different listing for each user. The latest listing applies. The fingerprint condition and the change report (FR-063) limit this risk.
- A route that puts two or more MCP targets behind one `mcp_server` name shows prefixed tool names. The names in `tools/list` and in `tools/call` agree, because ExtProc sees the client side of the two exchanges.
- After a start, an instance knows no definitions.

**Verification task**: The Docker suite with agentgateway
(`tests/e2e/extproc/opa_agentgateway_e2e_test.go`) must prove three facts. ExtProc receives the
response body of `tools/list`. The content type is one of the two accepted types. The client
receives the response without a change.

**Alternatives considered**: An ExtMCP guardrail processor gets parsed MCP data. But it needs a
second gRPC protocol and a second gateway configuration, and it also gets only the merged
listing. An active `tools/list` request from ExtProc needs credentials for each server and user.
A catalog of tool descriptions by the operator is not practical for hundreds of servers. The
stakeholder rejected it.

## D-03 — The decision request is plain JSON in the Decisions shape

**Decision**: ExtProc sends `POST <endpoint>` with a JSON body in the OpenAI Decisions shape. It
uses the existing `internal/extproc/httpclient` package and adds no SDK dependency. The input is
one user message with two parts: an `input_text` part with the fixed framing, and an `input_json`
part with the context. The contract is in
[contracts/decision-request.md](contracts/decision-request.md).

ExtProc sends no second attempt after an error. The deadline of the request is `request_timeout`.
ExtProc reads the credential file for each request and sends the value as a bearer token. TLS
uses the existing `oauth2.tls` settings. The existing rule for plaintext URLs applies
(`internal/extproc/config/validate.go:86`).

**Rationale**: The request has six fields and the response has four answer shapes. A hand-written
client is small and keeps the dependency set stable. A file read for each request lets the
operator replace a mounted credential without a restart.

**Verification task**: The agentgateway documentation shows message arrays with `input_text`. It
does not show `input_json` for the translation to System One. A Docker test with agentgateway and
a System One test double must prove that the context arrives. If the gateway rejects or removes
`input_json`, the request puts the canonical JSON text of the context into the `input_text` part.
The framing is part of the question set fingerprint, thus this selection is visible.

**Alternatives considered**: The `openai-go` SDK adds a large dependency for one request type. A
direct System One client removes the common contract that the issue prefers.

## D-04 — One status results from each assessment attempt

**Decision**: The table gives the status for each result of the model request.

| Result | Status |
|---|---|
| The server has an exception with `assess: false` | `excluded` |
| No definition of the tool is known | `definition_missing` |
| The definition or the context is larger than its limit | `context_too_large` |
| The deadline passes | `timeout` |
| A transport error, or an HTTP status that is not 200 | `unavailable` |
| The body is not valid JSON, or an answer is refused, missing, or invalid | `incomplete` |
| Each question has a valid answer | `complete` |

An answer is invalid in these cases:

- An unknown name or a duplicate name
- A wrong type
- A probability that is not between 0 and 1
- A wrong number of levels
- A total of the level probabilities that differs from 1 by more than 0.01.

ExtProc reads a maximum of 256 KiB of a response.

**Rationale**: Each status is one observable cause. The policy and the audit record can show the
cause (FR-031, FR-032).

**Alternatives considered**: One "failed" status hides the cause from the operator.

## D-05 — The policy contract adds one input key and three result fields

**Decision**: The second evaluation uses the same prepared query and the same input, with the
added key `assessment`. `ParseDecision` (`internal/extproc/authorization/decision.go:23-59`)
reads three new fields: `auto_approval.rule` on an `allow` result, and
`approval_context.recommendation` and `approval_context.findings` on an `approval_required`
result. Go code applies the conditions of FR-041 after it parses the result. The contract is in
[contracts/policy-assessment.md](contracts/policy-assessment.md).

**Rationale**: `OPAInput` is a map, thus the added key needs no interface change. The system
conditions are in Go code, thus a policy cannot disable them (FR-043).

**Consequence**: Each evaluation has its own `evaluation_timeout`. The existing audit record "opa
authorization decision" occurs two times for an assessed call.

**Alternatives considered**: A second policy package for the second evaluation divides one
decision between two documents.

## D-06 — Fingerprints are full SHA-256 values of canonical JSON

**Decision**: A fingerprint is the SHA-256 value of a canonical JSON document, as 64 hexadecimal
characters with a prefix: `qs-` or `tc-`. The document starts with a format version. The
canonical form is the Go `encoding/json` output of a generic value: sorted keys, no HTML escapes,
and numbers as the source wrote them.

The examples in the spec show eight characters to keep them short. Real values have 64
characters.

**Rationale**: A short value lets a server search for a second definition with the same
fingerprint. The standard library supplies the hash (Principle III).

**Consequence**: A new format version changes all fingerprints. The operator must then do a new
evaluation run.

**Alternatives considered**: RFC 8785 canonical JSON needs a new dependency. Its extra precision
is not necessary, because the same binary calculates all values.

## D-07 — The broker state finds a second identical call

**Decision**: Before a model request, ExtProc looks for an unexpired pending approval for the
same principal, agent, tool, and arguments. If one exists, ExtProc sends no model request. It
calls the idempotent create operation, which returns the existing approval. ExtProc also keeps a
bounded memory of its own "waits for a person" outcomes for the lifetime of a pending approval.

**Rationale**: The memory of one instance does not satisfy FR-008 for two or more instances. The
approval state of the broker is common to all instances.

**Verification task**: The schema of the sync response permits `pending` records
(`api/enduser/openapi.yaml`, `ToolApprovalSummary.status`). The task must make sure that
`GetSyncState` returns unexpired pending records with their exact patterns. If it does not, the
broker must add them. This addition does not change the schema.

**Alternatives considered**: A create request before the assessment makes a record for calls
that then proceed automatically. This is against FR-045.

## D-08 — The evaluation run is the first subcommand of the ExtProc binary

**Decision**: The ExtProc binary gets the subcommand `assessment evaluate`. It uses the same
packages as the service for the catalog, the context, the model request, the policy, and the
conditions of FR-041. It reads the service configuration, a case file, and one tool listing file
for each server. It writes a report and an evaluation record. The contract is in
[contracts/assessment-files.md](contracts/assessment-files.md).

The run calculates the 95% upper bound as an exact one-sided Clopper–Pearson bound. It uses the
`math` package and adds no dependency.

**Rationale**: One code path for production and for the measurement is the purpose of the run
(FR-050). The binary has only a root command today (`cmd/extproc-token-exchange/root.go:33`). The
subcommand must not change the behavior of the root command.

**Consequence**: `RegisterFlags` registers flags on the root command only. The subcommand
registers its own flags and uses the configuration loader again.

**Alternatives considered**: A separate binary doubles the release and container work. A test
helper is not usable by operators.

## D-09 — The broker stores the summary in one JSON column

**Decision**: Migration `049` adds a nullable `assessment` JSONB column to `tool_approvals`. The
domain service validates the limits of API-003 before it stores the value. The two storage
adapters store and return it with explicit columns. An idempotent create request for an existing
pending approval does not change the stored summary (FR-037).

**Rationale**: The summary belongs to one approval, has no identity of its own, and is not a
query condition. The repository has no code generation for API types, thus the DTO, the OpenAPI
document, and `web/src/types/approval.ts` change by hand.

**Alternatives considered**: A child table for findings adds joins for data that the system
always reads as one unit.

## D-10 — The approval page gets one application component

**Decision**: A new component `AssessmentSummary` in `web/src/components/approvals/` shows the
recommendation text, the findings, and the fixed notice. `ApprovalReviewPage` puts it between the
request summary and the technical details. The component shows only fixed product texts and the
statements from the API.

**Rationale**: The component is specific to approvals. The design system needs no new universal
component.

**Alternatives considered**: An addition to `ApprovalRequestSummary` mixes operator statements
with the policy description.

## D-11 — The catalog and the evaluation record are YAML files that the service reads at startup

**Decision**: ExtProc reads the two files one time at startup with strict decoding. Unknown keys,
a missing required key, or a file larger than 1 MiB stop the startup. The service does not read
the files again while it operates.

**Rationale**: A fingerprint must be stable while the service operates. A startup error shows a
mistake at once (FR-028).

**Alternatives considered**: A reload while the service operates changes fingerprints between
two calls and makes the audit trail difficult to read.

## D-12 — The audit record obeys the logging standard for tool names

**Decision**: The audit record `assessment.tool_call.assessed` always contains the target server
name and the tool context fingerprint. It contains the tool name only if the evaluation record
names the tool. The contract is in [contracts/observability.md](contracts/observability.md).

**Rationale**: `coding-standards/LOGGING.md` section 6.2 permits a tool name only if
configuration bounds it. The evaluation record is configuration. The target server name comes
from the gateway configuration. Each automatic approval has a tool in the evaluation record, thus
its audit record always contains the tool name.

**Consequence**: FR-060 of the spec changes to agree with this decision.

**Alternatives considered**: An exception to the logging standard needs an amendment of the
standard (Principle XV) for no clear benefit.

## D-13 — An audit write error stops the automatic approval

**Decision**: For an automatic approval, ExtProc writes the audit record before it forwards the
call. It writes through the `Handle` method of the logger handler and reads the returned error.
If the handler returns an error, the call waits for a person.

**Rationale**: SR-009 requires the record for each call that proceeds. The usual logger methods
do not return an error.

**Alternatives considered**: A durable audit store is not part of this repository. The logging
standard defines audit records as structured logs.
