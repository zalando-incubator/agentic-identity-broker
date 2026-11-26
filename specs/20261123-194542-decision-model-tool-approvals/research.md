# Research: Design Decisions for Decision Model Assessment

**Date**: 2026-11-24

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

This document records the design decisions of the planning phase. The guidance about decision
models is in [decision-model-research.md](decision-model-research.md) (`R-01` to `R-20`). This
document refers to its own decisions as `D-01` to `D-17`.

The first version of this document used the code of this repository and published documentation
only. On 2026-11-24, Docker tests against agentgateway `v1.5.0` and `v1.7.0` replaced the
assumptions of D-02, D-03, and D-07 with observed results. Each decision states its evidence. The
section "Verification record" at the end gives the commands and the test files.

On 2026-11-26, the stakeholder narrowed the scope. D-16 and D-17 record the two decisions. D-09
and D-10 are withdrawn. D-02, D-05, D-07, and D-11 changed with them.

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
6. The call proceeds, or the gate creates a pending approval with the description and the risk level of the policy, or ExtProc denies the call.

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
`target_server_name`. It keeps the stream state of these requests from the request phase to the
response phase. It copies each response chunk into a bounded buffer and returns the chunk to the
gateway without a change. At the end of the stream, it parses the result and stores each tool
definition in memory. The key is the target server name and the tool name as the client sees it.

The parser accepts `text/event-stream` and `application/json`. In an event stream, it uses the
`data` line of the JSON-RPC response that has the identifier of the request. It ignores unknown
result fields. A listing with a cursor adds or replaces only the tools on that page.

ExtProc removes all stored definitions of a target server if a listing response is not usable. A
response is not usable with an error status, a content encoding, an oversized body, or a body
that ExtProc cannot parse. Calls to the tools of that server then have the status
`definition_missing` until a valid listing arrives. Thus, a
listing that ExtProc cannot read never keeps an old definition that matches the evaluation record.
A single definition larger than its limit replaces the stored entry with a marker. A call to that
tool then has the status `context_too_large`.

**Evidence**: The Docker cases in
`tests/e2e/extproc/assessment_gateway_verification_test.go` ran the real ExtProc server behind a
real agentgateway container. The test MCP servers use `mcp-go`, as `mocks/mcp-server` does. Each
result below is the same for `v1.5.0` and `v1.7.0`, and for the MCP protocol versions
`2025-11-25` and `2026-07-28`, unless the table names a difference.

| Question | Observed result |
|---|---|
| Does ExtProc get the response body? | Yes. The gateway configuration sets no processing mode. |
| One gRPC stream for request and response? | Yes. The message kinds on one stream are `request_headers`, `request_body`, `response_headers`, `response_body`, `response_body`. |
| Content type at ExtProc | `text/event-stream`, also when the MCP server returns `application/json`. The gateway converts the response. |
| Event framing | One event: `event: message`, then one `data:` line with the JSON-RPC response. |
| Chunk sizes | One chunk with the full body, then one empty chunk with the end of the stream. A page of 100 tools gave one chunk of 70,298 bytes. |
| `Accept-Encoding: gzip` | The gateway sends `Accept-Encoding: gzip` to the server. A compressed server response arrives at ExtProc and at the client without `Content-Encoding` and in plain text. |
| Bytes at the client | Equal to the bytes that ExtProc returned, in each case. |
| Tool definition | The `description`, `inputSchema`, and `annotations` at ExtProc are equal to the definition that the server registered. |
| Pagination in `v1.5.0` | The server returned `nextCursor`, but the client and ExtProc got the first page without `nextCursor`. The tools after the first page are not visible. |
| Pagination in `v1.7.0` | `nextCursor` arrives. The second page arrives on its own stream with the next 100 tools. |
| One route with one MCP target | Tool names are not changed (`drive.share_file`). |
| One route with two MCP targets | Tool names have the target name as prefix (`drive_drive.share_file`, `wiki_wiki.read_page`). ExtProc sees the prefixed name in `tools/list` and in `tools/call`. The gateway removes the prefix before the call gets to the server. |

**Rationale**: ExtProc is in the path of each MCP request and gets the response body today
(`echoResponseBody`). The gateway configuration does not change. The `mcp_server` metadata value
is mandatory for MCP requests (`adrs/028-opa-extproc-authorization.md`, amendment of 2026-09-21).
Thus, each listing has one target server name. The prefix of the gateway keeps the tool names of
two targets on one route apart, and the names in `tools/list` and in `tools/call` agree.

**Risks**:

- Definitions apply to all users of one instance. A server can return a different listing for each user. The latest listing applies. The fingerprint condition and the change report (FR-063) limit this risk.
- If an operator adds a second target to a route, the tool names of the first target get a prefix. Each tool of that route then has no definition and no evaluation record entry until a new listing and a new evaluation run. The calls wait for a person.
- On a route with two or more MCP targets, the target server name is the `mcp_server` value of the route. It is not the name of one server. The `server` section of the context and the keys of the evaluation record thus name the route. The verification case "two targets on one route" used `mcp_server: 'workspace-mcp'` for two servers.
- After a start, an instance knows no definitions.
- The tests used `mcp-go` servers only. A server that sends the result as an event stream with other events before the result was not tested. The parser looks for the response with the request identifier and thus does not depend on the event sequence.

**Alternatives considered**: An ExtMCP guardrail processor gets parsed MCP data. But it needs a
second gRPC protocol and a second gateway configuration, and it also gets only the merged
listing. An active `tools/list` request from ExtProc needs credentials for each server and user.
A catalog of tool descriptions by the operator is not practical for hundreds of servers. The
stakeholder rejected it. The fallbacks of the first version of this decision are not necessary.

An optional replacement description for one tool and an optional description of a server were
parts of this decision until 2026-11-26. The stakeholder rejected the two for the same reason.
MCP servers have their own owners and their own release cycles. A description in the
configuration of the operator couples that configuration to each change of a server, and the
operator cannot keep the two in agreement. Thus, the configuration has no key that describes a server
or a tool. The `server` section has only the target server name
from the gateway, and the `tool` section has no author field: the server is always the author,
and the framing text states it. If an operator does not trust the definitions of a server, the
operator writes no automatic approval rule for its tools.

On the same date, the stakeholder rejected an optional `servers` part of the configuration. It had
exceptions for one server: no assessment, or a different disclosure. Each entry for a server
needs maintenance when servers change. The disclosure settings now apply to each server, the
configuration has no key for a server or a tool, and the status `excluded` does not exist.

## D-03 — The decision request is plain JSON in the Decisions shape, with the context as text

**Decision**: ExtProc sends `POST <endpoint>` with a JSON body in the OpenAI Decisions shape. It
uses the existing `internal/extproc/httpclient` package and adds no SDK dependency. The input is
one user message with two `input_text` parts. The first part is the fixed framing. The second
part is the context as compact JSON text. The sections keep the sequence of FR-022. Inside a section,
the keys of a map are sorted, as the Go JSON encoder writes them. The sequence has one purpose:
the service and the evaluation run send the same text for the same call. The contract is in [contracts/decision-request.md](contracts/decision-request.md).

ExtProc sends no second attempt after an error. The deadline of the request is `request_timeout`.
ExtProc reads the credential file for each request and sends the value as a bearer token. TLS
uses the existing `oauth2.tls` settings. The existing rule for plaintext URLs applies
(`internal/extproc/config/validate.go:86`). That rule is one development setting for each
outbound endpoint. It does not examine the host, and the spec does not add a rule for local
hosts.

**Evidence**: The Docker cases in
`tests/e2e/extproc/assessment_decision_endpoint_verification_test.go` sent the request of
Journey 1, step 6, through agentgateway `v1.7.0`. Two test doubles recorded the upstream
requests: one in the OpenAI Decisions format and one in the TypeSafe System One format. The
gateway configuration used the `llm` section with two `custom` providers.

| Question | Observed result |
|---|---|
| First release with the Decisions route | `v1.7.0`. The `v1.7.0` documentation says: "Version 1.6 had no Decisions or SystemOne route type." |
| `input_json` to an OpenAI-format upstream | Arrives unchanged at `/v1/decisions`. |
| `input_json` to a System One-format upstream | HTTP 400 with the body `failed to process LLM request: failed to parse Decisions request: data did not match any variant of untagged enum DecisionInput`. No request gets to the upstream. |
| Text form to a System One-format upstream | Arrives at `/v1/systemone`. `state` is `[{"role": "user", "content": "<framing>\n<context JSON text>"}]`. The gateway joins the two parts with a line break. |
| Request `model` | The request names a model entry of the gateway. The provider entry of the test set `params.model`, and the gateway sent that value to the upstream. |
| Credential | The gateway replaces the bearer token of the client with the provider key. |
| Response `model` | The client gets the value of the upstream response unchanged, in the two formats. |
| `predicate` and `score` answers from System One | The gateway translates `noul` to `predicate`. It translates a `score` to the Decisions form with `value`, `label`, and `probability` for each level. It takes the labels from the request. |
| `refusal` from an OpenAI-format upstream | Arrives at the client as `{"type": "refusal", "name": "<question>"}`. |
| A question without an answer from System One | The answer is absent. The System One API has no refusal. ExtProc thus sees "missing", which gives the status `incomplete`. |
| One instance for MCP and decisions | Yes. One `v1.7.0` instance served the `llm` models and a `binds` route. All existing agentgateway suites of this repository pass on `v1.7.0` (D-14). |

At the time of these cases and of the smoke test, the `tool` section of the request had one more
field: `"written_by": "server"`. The spec removed that field on 2026-11-26 (D-02). The cases were
not run again after the change, because no container runtime was available. The gateway does not
interpret the text of an `input_text` part, thus the transport results do not depend on the field.

The OpenAI reference documents `input_json` for user messages, and its guide says the model
renders it as text. The gateway parser of `v1.7.0` does not know the part. Thus, the text form is
the only form that works for the two upstream formats.

**Rationale**: The request has a fixed shape and the response has four answer shapes. A
hand-written client is small and keeps the dependency set stable. A file read for each request
lets the operator replace a mounted credential without a restart. One request form for the two
upstream formats keeps one question set fingerprint for both.

**Consequences**:

- The input encoding `input_text` is part of the question set fingerprint (D-06).
- The limits of `input_json` (256 KiB, 32 levels, 10,000 values) do not apply. The context limit of 16 KiB applies.
- The `model` field of the request names a model entry of the gateway. The answering model is the `model` field of the response. Its value comes from the provider. An OpenAI response can name `gpt-6-luna` without a version. Then a model change behind that name is not visible to the model condition of FR-041. The tests did not call OpenAI, thus this behavior is not known.
- The gateway route for decisions must not have the ExtProc policy. Otherwise, ExtProc gets its own model request.

**Alternatives considered**: The `openai-go` SDK adds a large dependency for one request type. A
direct System One client removes the common contract that the issue prefers. An `input_json` part
works only for OpenAI-format upstreams. A setting that selects the part for each upstream adds a
configuration key and a second fingerprint for one question set.

## D-04 — One status results from each assessment attempt

**Decision**: The table gives the status for each result of the model request.

| Result | Status |
|---|---|
| No definition of the tool is known | `definition_missing` |
| The definition or the context is larger than its limit | `context_too_large` |
| The deadline passes | `timeout` |
| A transport error, or an HTTP status that is not 200 | `unavailable` |
| The body is not valid JSON, the `model` value is not valid, or an answer is refused, missing, or invalid | `incomplete` |
| Each question has a valid answer | `complete` |

An answer is invalid in these cases:

- An unknown name or a duplicate name
- A wrong type
- A probability that is not between 0 and 1
- A wrong number of levels
- A set of level `value` entries that is not exactly `0` to `n-1`, each one time
- A level `label` that is not equal to the label of that level in the question set
- A total of the level probabilities that differs from 1 by more than 0.01.

ExtProc puts each level probability at the position of its `value`, not at its position in the
response. Thus, index 0 of the policy input is always the level with the least risk.

The `model` value is not valid if it is absent, empty, longer than 100 characters, or contains a
control character.

ExtProc reads a maximum of 256 KiB of a response.

**Rationale**: Each status is one observable cause. The policy and the audit record can show the
cause (FR-031, FR-032).

**Alternatives considered**: One "failed" status hides the cause from the operator.

## D-05 — The policy contract adds one input key and one result field

**Decision**: The second evaluation uses the same prepared query and the same input, with the
added key `assessment`. `ParseDecision` (`internal/extproc/authorization/decision.go:23-59`)
reads one new field: `auto_approval.rule` on an `allow` result. It also reads the existing
`approval_context` on an `allow` result. Go code applies the conditions of FR-041 after it parses
the result. The contract is in [contracts/policy-assessment.md](contracts/policy-assessment.md).

An `allow` result can carry an `approval_context`. The system uses it only if a condition of
FR-041 is false and the call waits for a person. Without it, the recommendation mode gives the
person no call-specific description for exactly the calls that a rule approves (US1.2, FR-044).

If the second evaluation fails, ExtProc uses the result of the first evaluation, with its
description and its risk level. The call waits for a person. An error in the second evaluation
thus never removes the decision of a person, and it never lets a call proceed.

The authorizer of today cannot show this failure. The prepared query, the SDK path, and the
timeout path each return a `deny` decision with a `nil` error
(`internal/extproc/authorization/authorizer.go:158`, `:194`, `:231`). The feature adds the
outcome of the evaluation to the decision: a result, an undefined result, an error, or a timeout.
The first evaluation ignores the outcome and denies as today. The second evaluation falls back
only for an error or a timeout. A `deny` result and an undefined result deny the call. Tests use
the two real backends for each outcome. The log call of the record "opa authorization decision"
does not change.

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
characters. In an HTTP body of the spec, `...` marks the shortened value (Principle XIV).

The canonical document of the question set contains each field of each question, because the
model receives each field (FR-029, [contracts/decision-request.md](contracts/decision-request.md)
section 5).

**Rationale**: A short value lets a server search for a second definition with the same
fingerprint. The standard library supplies the hash (Principle III).

**Consequence**: A new format version changes all fingerprints. The operator must then do a new
evaluation run.

**Alternatives considered**: RFC 8785 canonical JSON needs a new dependency. Its extra precision
is not necessary, because the same binary calculates all values.

## D-07 — The broker state finds a second identical call

**Decision**: The gate reads the approval state of the principal from the broker before an
assessment, as it does today before it creates a pending approval (`gate.go:86-98`). After that
read, ExtProc looks for a pending approval of the same agent with the same tool name and the same
`arguments_hash`. The sync response already contains `arguments_hash` (`sync_handler.go:46`), and
the ExtProc `Record` type adds the field. If such an approval exists, ExtProc sends no model
request. It calls the idempotent create operation with the result of the first evaluation, which
returns the existing approval URL. The stored description and risk level of the existing approval
do not change. If that approval expired or got a decision after the read, the broker creates a
new approval (`internal/domain/approval/service.go:558`). The new approval then has the
description and the risk level of the first evaluation.

The hash function moves to the shared leaf package `internal/domain/approval/toolpattern`,
because `adrs/035-shared-tool-pattern-matching.md` permits ExtProc no other import from
`internal/domain`. `storage.ComputeArgumentsHash` calls the function of the leaf package. The ADR
of this feature records that the leaf package also owns the arguments hash.

The function makes one canonical form before it calculates the hash: it encodes the arguments as
JSON, decodes that JSON without `UseNumber`, and encodes the result. ExtProc decodes MCP bodies
with `UseNumber` (`server.go:740`), and the broker decodes to `float64`
(`create_handler.go:72`). Without the canonical form, the argument `1.0` gives two hashes. For
the values that the broker has today, the canonical form does not change the hash, thus stored
hashes stay valid.

The match does not use the target server, because the approval key of the broker has no target
server. Two servers on different routes that list one tool name thus share pending approvals,
as they share human decisions today. SR-010 names this limit.

ExtProc does not use the patterns of the pending record for this lookup. `toolpattern.Matches`
checks only the keys of the pattern. A call with an added argument thus matches, but the broker
creates a new approval for it, because the arguments hash differs. ExtProc and the broker use one
hash function with one canonical form, thus the two calculations agree for each call.

ExtProc keeps no memory of its own outcomes for this purpose. The broker state is common to all
instances and is fresh after the read.

**Evidence**:

- `ListAllActive` returns each record that is not consumed, and a `pending` record only if `expires_at` is in the future (`tool_approval_repository.go:810-811`). `ApplyExactPatterns` gives each pending approval exact patterns on creation.
- The case "returns an unexpired pending approval with exact patterns and keeps it on a duplicate create (D-07 verification)" in `tests/e2e/approval_api_test.go` passed against the real broker with in-memory storage. The sync response contained the record with `status: pending`, `tool_pattern: drive.share_file`, and `params_pattern` equal to the two arguments. A second create request with a different description returned HTTP 200, the same identifier, and the same `created_at`. The detail kept the first description.
- The PostgreSQL integration tests `TestToolApprovalRepository_CRUD` (subtests `Create_idempotent` and `ListAllActive`) and `TestToolApprovalRepository_DedupReturnsPersistedApprovalWithoutSyncBump` passed. The last test shows that a duplicate create returns the stored record unchanged and does not change the sync version.

**Consequences**:

- The ExtProc cache keeps pending records today, but `Record.matchable` (`cache.go:332-344`) accepts only `approved` records. The feature adds a separate lookup for `pending` records. The existing match for human decisions does not change.
- An idempotent create keeps the description and the risk level of the first create request. The broker needs no change.
- The sync record has no expiry time. ExtProc thus cannot know that a pending approval is near its end. The create request with the result of the first evaluation covers this case.
- Two instances can assess the same call at the same time. Each sends one model request, and the broker keeps the first create request.
- A pending record never lets a call proceed. The lookup only selects the path without a model request.

**Alternatives considered**: A create request before the assessment makes a record for calls
that then proceed automatically. This is against FR-045. A memory of outcomes in each instance
does not satisfy FR-008 for two or more instances.

## D-08 — The evaluation run is the first subcommand of the ExtProc binary

**Decision**: The ExtProc binary gets the subcommand `assessment evaluate`. It uses the same
packages as the service for the question set, the context, the model request, the policy, and the
conditions of FR-041. It reads the service configuration, a case file, and one tool listing file
for each server. It writes a report and an evaluation record. The contract is in
[contracts/assessment-files.md](contracts/assessment-files.md).

The run calculates the 95% upper bound as an exact one-sided Clopper–Pearson bound. It uses the
`math` package and adds no dependency.

**Rationale**: One code path for production and for the measurement is the purpose of the run
(FR-050). The binary has only a root command today (`cmd/extproc-token-exchange/root.go:33`). The
subcommand must not change the behavior of the root command.

**Consequence**: `RegisterFlags` (`internal/extproc/config/loader.go:56`) registers local flags
on the command that it gets. The root command calls it (`root.go:354`), and a subcommand does not
inherit these flags. Thus, the subcommand calls `RegisterFlags` for itself and then
`LoadWithCommand`. `Validate` requires the full service configuration, including the `oauth2`
keys. The run thus uses the configuration file of the service without a change. The subcommand
does not require that `evaluation_record_file` exists, because the run writes that file.

The run builds the policy input of each case with the production input builder. A case can
supply the request values and the context values of the production input, and the run uses
documented defaults for absent values ([contracts/assessment-files.md](contracts/assessment-files.md) section 4).

The subcommand has the flag `--config` and loads the configuration in a mode that does not
examine `evaluation_record_file`. `Load` reads the configuration path from `config_path` today
(`internal/extproc/config/loader.go:121`).

The starter case set has a tuning part and a validation part. The project changes questions and
thresholds only with the tuning part. It publishes the false-approval rate and its bound only
from the validation part (R-16, FR-053).

The run sets `input.assessment.evaluated` to `true` for each case. Thus, a policy rule that
requires `evaluated` applies in the run as it will apply in production after the record exists.

**Alternatives considered**: A separate binary doubles the release and container work. A test
helper is not usable by operators.

## D-09 — Withdrawn: the broker stores the summary in one JSON column

Withdrawn on 2026-11-26 (D-16). The feature stores no assessment data in the broker and adds no
migration.

## D-10 — Withdrawn: the approval page gets one application component

Withdrawn on 2026-11-26 (D-16). The approval page does not change.

## D-11 — The evaluation record is a YAML file that the service reads at startup

**Decision**: ExtProc reads the evaluation record one time at startup with strict decoding. An
unknown key, a missing required key, or a file larger than 1 MiB stops the startup. The service
does not read the file again while it operates. The questions, the organization values, and the
disclosure settings are not in a file of their own. They are keys of the service configuration
(D-17).

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

**Existing deviation**: The existing record "opa authorization decision"
(`internal/extproc/authorization/authorizer.go:323-332`) logs `tool_name` from the request for
each decision. This value is not bounded by configuration. The record existed before section 6.2
(added on 2026-11-02). This feature does not change that record. A separate issue must decide
whether that record keeps the field.

**Alternatives considered**: An exception to the logging standard needs an amendment of the
standard (Principle XV) for no clear benefit. The behavior of the existing record is not a
precedent, because it is older than the standard.

## D-13 — An audit write error stops the automatic approval

**Decision**: For an automatic approval, ExtProc writes the audit record before it forwards the
call. It writes through the `Handle` method of the logger handler and reads the returned error.
If the handler returns an error, the call waits for a person.

While assessment is enabled, the configuration validation requires `log.level` `debug` or
`info`. FR-060 requires the record for each assessed call in each mode, and a `warn` level loses
it. Before each write for an automatic approval, ExtProc also calls `Enabled` for the INFO level.
If it returns false, the call waits for a person.

**Rationale**: SR-009 requires the record for each call that proceeds. The usual logger methods
do not return an error.

**Limits**: The ExtProc logger is `principalPolicyHandler` around the standard output handler,
or around `multiHandler` with the standard output handler and the `otelslog` handler
(`cmd/extproc-token-exchange/root.go:116-165`). `multiHandler.Handle` joins the errors of its
handlers. The standard output handler returns a write error. The `otelslog` handler always
returns `nil`, because the export is asynchronous. Thus, the check finds a failed write to
standard output only. It does not find a failed export or a lost record after the write.

`multiHandler.Handle` skips each handler that is not enabled for the level and then returns
`nil` (`internal/adapters/telemetry/slog_handler.go:33-42`). With `log.level: warn`, an INFO
audit record thus disappears without an error. The level rule above closes this gap.

**Relation to the logging standard**: The record is an authorization decision record, not a
mutation record. An automatic approval changes no stored state (FR-045). ExtProc writes the
record at the time of the decision, as it writes the existing record "opa authorization
decision". The rule "write the record after the mutation commits" in section 8 of
`coding-standards/LOGGING.md` thus does not apply.

Two parts of the standard need a scoped rule, and the pull request of the feature amends the
standard with them (Principle XV). Section 1 requires a `*Context` logger method, and these
methods return no error. The scoped rule permits a direct call of `Handler.Handle` with the
request context for an audit record whose write error decides the action. Section 8 gets one
sentence: a decision record is written at the time of the decision.

**Alternatives considered**: A durable audit store is not part of this repository. The logging
standard defines audit records as structured logs.

## D-14 — The deployment uses agentgateway `v1.7.0` for MCP and for decisions

**Decision**: The pinned agentgateway release changes from `v1.5.0` to `v1.7.0` at each pin site:

- `docker-compose.yml:249`, with the index digest of `v1.7.0` (`sha256:60216aafb03d37f94e952ff3d51d2f630a65b10ecaebdc0a864882f92610c45b` from `docker buildx imagetools inspect` on 2026-11-24). The current pin `sha256:bf2f339e…` is the index digest of `v1.5.0`.
- `defaultAgentgatewayImage` in `tests/e2e/extproc/agentgateway_e2e_test.go` and in `tests/e2e/agentgateway_token_exchange_e2e_test.go`
- The expected value in `tests/e2e/extproc/agentgateway_image_test.go`
- The `docker pull` steps in `.github/workflows/ci.yml`, `container-tests.yml`, and `scheduled-coverage.yml`.

One gateway instance can serve the MCP routes with ExtProc and the `llm` models for decisions.
The documentation states `v1.7.0` as the minimum release.

**Evidence**: The Decisions route first exists in `v1.7.0` (D-03). With
`AGENTGATEWAY_IMAGE=cr.agentgateway.dev/agentgateway:v1.7.0`, the existing suites "Agentgateway
Integration", "Agentgateway Re-authentication Challenge", "Metadata Input via Agentgateway",
"Agentgateway Telemetry Integration", and "OPA Authorization via Agentgateway" passed with 17 of
17 cases. The suite "agentgateway oauthTokenExchange with an inline client-assertion key set" in
`tests/e2e/` passed with 1 of 1 case. The verification cases of D-02 passed on `v1.5.0` and on `v1.7.0`. `v1.5.0` removes
`nextCursor` from a paged `tools/list` result (D-02). Thus, clients of `v1.5.0` see only the
first page.

**Alternatives considered**: Two gateway instances with two releases double the operation work.
They also keep the pagination defect of `v1.5.0` on the MCP path.

## D-15 — The existing deadlines limit the time of an assessed call

**Decision**: The feature adds no total deadline. Each step of an assessed call has a deadline:

| Step | Deadline |
|---|---|
| First policy evaluation | `authorization.evaluation_timeout` (default 100 ms) |
| Read of the approval state from the broker (D-07) | `tool_approvals.request_timeout` (default 5 s), as today |
| Model request | `authorization.assessment.request_timeout` (default 500 ms, maximum 2 s) |
| Second policy evaluation | `authorization.evaluation_timeout` (default 100 ms) |
| Create request to the broker | `tool_approvals.request_timeout` (default 5 s), as today |

Just before an automatic approval, ExtProc looks in its local approval cache for a permanent
denial of the tool, with the rule of `Cache.Match` (`cache.go:297-301`). This lookup is a new
function that only reads, because `Match` reserves a `once` approval. The cache gets new
decisions through the existing sync during the
assessment. If a denial matches, ExtProc denies the call, and the audit record has
`unmet_condition: denial`. This check reads no data from the broker. Thus, the assessment adds a
maximum of the model deadline and one evaluation deadline to the path of today. SC-005 of the
spec states this sum.

**Evidence**: agentgateway has no message timeout for ExtProc. The ExtProc policy of `v1.5.0` and
`v1.7.0` has no timeout field (`crates/agentgateway/src/http/ext_proc.rs`, struct `ExtProc`). In
the verification case "waits for a slow ExtProc answer", ExtProc held the request body of
`tools/call` for 6 s. The call completed with HTTP 200 on the two releases. A manual run of the
same case with 45 s gave HTTP 200 after 45.0 s on the two releases.

**Scope of the evidence**: The evidence is for the ExtProc filter only. That filter waits for
the answer of ExtProc without a timer of its own. A request timeout of a route, of the gateway,
or of the client can still end the call. The verification did not test such a timeout.

**Consequence**: The ExtProc filter of the gateway does not stop a slow ExtProc. A deadline that ExtProc misses delays
the call of the agent and does not let it proceed. The MCP client of the agent can have its own
timeout. With the defaults, the worst case of an assessed call that waits for a person is about
10.7 s, and 10.2 s of that is the broker path of today.

The local smoke test measured 762 to 1,247 ms for a warm `winnow:e4b` model and 6.1 s for the
first request ([decision-model-research.md](decision-model-research.md), "Smoke test with a local
model"). With the default `request_timeout` of 500 ms, each call to that model has the status
`timeout`. The calls then wait for a person. This is safe, but the assessment then has no effect.
An operator must measure the latency of the model and set `request_timeout` to a value of up to
2 s. The documentation must state this.

**Alternatives considered**: A total deadline for the assessment duplicates the deadlines of its
steps.

## D-16 — The approver sees the description and the risk level of the policy, and no new element

**Decision**: The feature adds no approver-facing element. The policy derives the existing
`approval_context.description` and `approval_context.risk_level` from the answers. ExtProc sends
the two values to the broker in the existing fields of the create request. The broker, its API,
its database, and the approval page do not change.

The earlier design had a list of findings with operator-written statements, a recommendation, a
fixed notice, and an assessment summary that the broker stored. The stakeholder rejected it on
2026-11-26.

**Rationale**: An approver reads one statement, not a list. That statement must agree with the
policy decision, thus it is a part of the policy result. The policy result already has it: the
description, with the risk level. The model returns only numbers, thus no model text can get to
the approver.

**Consequences**:

- The feature needs no migration, no OpenAPI change, no web component, and no browser test.
- The approval page does not show that a model contributed to the two values. It shows no notice for a failed assessment. The policy sees the assessment status and selects the description for that case.
- The system cannot make sure that a description is a constant text. A policy can format an argument value or an answer value into it, as it can today. The authoring rules and the example policy use constant texts (FR-039, SR-008).
- The answering model and the question set fingerprint of a call that waits for a person are only in the audit record of ExtProc. The approval record does not have them.

**Alternatives considered**: A new result field for one statement duplicates the description. A
finding list from the policy keeps a second text channel to the approver and all broker changes.

## D-17 — The organization values, the disclosure settings, and the questions are ExtProc configuration, and the product has a default question set

**Decision**: The organization values, the disclosure settings, and the questions are keys of the
ExtProc configuration under `authorization.assessment`. They use the existing approach of
`internal/extproc/config/`: the schema in `config.go`, the defaults in `loader.go`, and the rules
in `validate.go`. The key `questions` is a list, as `rules` is in the impersonation configuration
of the broker. Its default value is the default question set of the spec. A list in the
configuration replaces the full default list. The feature has no catalog file, no question set
file, and no file format of its own for these values. The contract is in
[contracts/assessment-files.md](contracts/assessment-files.md).

**Rationale**: The values are deployment configuration, as the endpoint and the model are. One
configuration file with one loader is easier to operate than two. With a default set, an operator
enables the feature without writing a question, and the project can measure and publish results
for one known set (FR-054).

**Consequences**:

- A policy refers to the default questions by name. The names are part of the product contract for one release.
- A release that changes a default question changes the question set fingerprint of each deployment that uses the default set. Automatic approval then stops until a new evaluation run. The release notes must state the change.
- The existing loader ignores a key that the schema does not have. Thus, a wrong key name does not stop the startup, as for each other key of the service.

**Quality of the default questions**: The smoke test of 2026-11-24 showed weak answers for three
default questions ([decision-model-research.md](decision-model-research.md), "Smoke test with a
local model"). On 2026-11-26, the spec changed their wording:

| Question | Observation | Cause in the wording | Change |
|---|---|---|---|
| `persistent_effect` | 0.74 to 0.80 on the top level for each share, also for the reader share inside the organization | The question "What remains" was vague. Level 0 excluded each call that sends something, thus the notification email of the tool moved each share to the top level. The top level also collected each doubt. | The question asks how hard it is to undo the main effect. It tells the model to ignore a notification about the change. Each level gives examples, and the top level is for a call that does not show what it changes (R-04, R-05). |
| `definition_addresses_assessor` | 0.23 to 0.28 for a description that addresses no reviewer | The yes criterion named text that addresses "a model". Each tool description addresses a model: the agent that calls the tool. | The question asks if the text tries to influence a review or an approval. The no criterion states that instructions for the caller are not a yes (R-05). |
| `addresses_assessor` | 0.044 to 0.053, at the example threshold of 0.05 | The same criterion: an argument value can address a model or a person as normal data. | The same change. The no criterion states that text for a recipient of the tool is not a yes. |

`leaves_organization` and `changes_access` gave clear answers and did not change.

No model measured the new wording. A container without a model wrote this revision. Thus, the
revision is a hypothesis, and these controls apply:

- The project measures each default question on its own, with expected answers for each case and a minimum number of cases for each answer (FR-054). The cases include difficult cases for the answer no.
- The evaluation run and this measurement are parts of the same pull request as the feature. The pull request does not merge with a default question that has no published results and no approval of the stakeholder (SC-011).
- The project changes questions only with the tuning part of the case set and publishes results only from the validation part (FR-053).
- The project measures with each model that it can reach, and it names each model with its results. A result for one model does not apply to a different model (R-19, R-20).

**Disclosure**: The disclosure settings have one key, `arguments`, with the values `all` and
`none`. A list of argument names whose values the system does not send was a part of the design
until 2026-11-26. The stakeholder removed it, because an operator cannot maintain the argument
names of externally managed tools. With `all`, the value of each described argument goes to the
decision endpoint (SR-010).

**Alternatives considered**: A separate catalog file or a question set file. The stakeholder
rejected the two on 2026-11-26: the service has one configuration approach, and the name
"catalog" suggests a list of servers. A set of questions that each operator must write gives no
default and no common measured result.

## Verification record## Verification record

The verification ran on 2026-11-24 on macOS (arm64) with Docker Engine 29.5.2 and
`testcontainers-go` v0.44.0. The test files stay in the repository.

| Command | Result |
|---|---|
| `AGENTGATEWAY_IMAGE=cr.agentgateway.dev/agentgateway:v1.5.0 ginkgo -v --focus "Assessment gateway verification" ./tests/e2e/extproc/` | 10 of 10 passed |
| `AGENTGATEWAY_IMAGE=cr.agentgateway.dev/agentgateway:v1.7.0 ginkgo -v --focus "Assessment gateway verification" ./tests/e2e/extproc/` | 10 of 10 passed |
| `ginkgo -v --focus "Decision endpoint verification" ./tests/e2e/extproc/` (agentgateway `v1.7.0`) | 5 of 5 passed |
| `AGENTGATEWAY_IMAGE=cr.agentgateway.dev/agentgateway:v1.7.0 ginkgo -v --focus "Agentgateway\|OPA Authorization via Agentgateway" ./tests/e2e/extproc/` | 17 of 17 passed |
| `ginkgo -v --focus "D-07 verification" ./tests/e2e/` | 1 of 1 passed |
| `go test -tags integration ./internal/adapters/storage/postgres/ -run 'TestToolApprovalRepository_(CRUD\|DedupReturnsPersistedApprovalWithoutSyncBump)'` | passed |
| `AGENTGATEWAY_IMAGE=cr.agentgateway.dev/agentgateway:v1.7.0 ginkgo --label-filter=docker --focus "agentgateway oauthTokenExchange" ./tests/e2e/` | 1 of 1 passed |
| A temporary Ginkgo case sent the Journey 1, 2, and 4 requests through agentgateway `v1.7.0` to a local `winnow:e4b` System One model. The case was deleted after the run. | 9 of 9 requests returned HTTP 200 with five answers |

The verification cases record their observations with `AddReportEntry`. Run them with `-v` to
see the values.

These subjects were not tested:

- A hosted decision model. The smoke test used one local model only. The answers, the latency, and the `model` value of the OpenAI and TypeSafe services are not known.
- The `model` value as a version check. The local server returned the requested name `winnow:e4b`, not a versioned identifier. A provider that does the same hides a change of weights from the model condition of FR-041. The operator documentation must state this.
- MCP servers other than `mcp-go`, listings larger than 70 KB, and event streams with more than one event.
- The measured results of the example policy on the starter case set. The spec requires them before publication, and they need a real model.
