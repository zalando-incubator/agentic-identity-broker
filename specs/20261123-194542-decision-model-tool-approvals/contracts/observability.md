# Contract: Observability (logs, metrics, traces)

**Feature**: [spec.md](../spec.md) FR-060 to FR-062, SR-007, SR-009 | **Decisions**: [research.md](../research.md) D-12, D-13

Operators make alerts and audits from these names. After release, a new name for an event, a
span, an instrument, or an attribute is a breaking change.

This contract obeys `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. The
implementation adds the namespace `assessment` to the namespace table in section 4.1 of
`coding-standards/OPENTELEMETRY.md`. It adds the instruments to "What is Instrumented" in
`docs/configuration.md`.

## Data that no signal contains

This contract applies to the signals that ExtProc and the broker write for this feature. No such
log record, span, or metric contains these items:

- An argument value or an argument name
- The assessment context or a part of it
- Text from a tool definition
- Question text or criteria
- The request body or the response body of the model request
- The credential for the decision endpoint.

A tool name is in no signal of the feature, because no configuration bounds that value (D-12). A principal is
never in a metric.

Two outputs are not signals of this contract, because the operator configures them:

- The decision logs of OPA. If the OPA configuration enables them, OPA writes the full input and the result of each evaluation. For the second evaluation, the input contains `assessment` and `mcp.arguments`. Thus, these logs show the call, the answers, and the decision, and an operator can use them for an analysis outside the product. The operator decides who can read them and whether to mask a part of them.
- The `reasons` of a policy result. The existing record "opa authorization decision" contains them without a change. A policy must not write argument values or assessment data into its reasons.

## Structured log events

| `event` | Level | Written by | Attributes |
|---|---|---|---|
| `assessment.tool_call.assessed` | INFO | ExtProc, one time for each assessed call (FR-060) | See the table that follows |
| `assessment.question_set.loaded` | INFO | ExtProc at startup | `question_count`, `mode` |

Attributes of `assessment.tool_call.assessed`:

| Attribute | Value |
|---|---|
| `principal` | The principal through the principal policy of the logging standard |
| `agent_id` | The typed agent identifier |
| `target_server_name` | From the gateway metadata |
| `arguments_ref` | `sha256:` and the arguments hash of the shared leaf package (D-07) |
| `assessment_status` | A status of D-04 |
| `outcome` | `automatic_approval`, `human_review`, or `denied` |
| `assessment_mode` | `shadow`, `guidance`, or `auto_approve` |
| `policy_action` | The action of the policy result after the assessment: `allow`, `approval_required`, or `deny`. Absent if the second evaluation had an error or a timeout |
| `would_auto_approve` | Boolean. True in the `shadow` mode and in the `guidance` mode if all other conditions of FR-041 are true (FR-044) |
| `unmet_condition` | For an `allow` result that did not proceed: `mode`, `status`, or `denial` |
| `model` | The answering model |
| `answers` | A group with the question name as the key. The value is the probability, or the list of level probabilities, or the answer state if no value exists |
| `duration_ms` | The duration of the model request |

The record for an automatic approval is an audit record. ExtProc writes it before it forwards
the call. If the write returns an error, the call waits for a person (D-13).

The existing record "opa authorization decision" occurs two times for an assessed call: one time
for each evaluation.

## Spans

| Span name | Kind | Parent | Attributes |
|---|---|---|---|
| `extproc.assessment` | Internal | The span of the request | `assessment.status`, `assessment.outcome` |

The feature adds no client span. The outbound client of ExtProc (`internal/telemetryhttp`)
already starts the client span `http.request` for each request. For the model request, that span
is a child of `extproc.assessment`. It has the HTTP attributes, and it gets the error status for
a transport error and for an HTTP error status.

The span `extproc.assessment` has the error status only for `unavailable` and `timeout`. The
other statuses are not errors of the span.

## Metrics

All instruments use the existing meter `extproc`. ExtProc makes each instrument one time in a
constructor.

| Instrument | Kind | Unit | Attributes | Recorded |
|---|---|---|---|---|
| `extproc.assessment.requests` | Counter | `{assessment}` | `assessment.status`, `assessment.outcome` | One time for each assessed call |
| `extproc.assessment.model_request.duration` | Histogram | `s` | `assessment.status` | One time for each model request |
| `extproc.assessment.tool_definitions` | Gauge | `{definition}` | none | The number of definitions in the store |
| `extproc.assessment.listings` | Counter | `{listing}` | `assessment.listing_result` | One time for each observed `tools/list` response |

Attribute values:

| Attribute | Values |
|---|---|
| `assessment.status` | The six statuses of D-04 |
| `assessment.outcome` | `automatic_approval`, `human_review`, `denied` |
| `assessment.listing_result` | `stored`, `too_large`, `not_parsed`, `ignored` |

The number of attribute combinations is constant. A target server name and a tool name are not
metric attributes.

Useful signals for an operator:

- A high share of `timeout` or `unavailable` shows a problem with the decision endpoint.
- A high share of `definition_missing` after a start shows that agents do not list tools through this instance.
- The share of `would_auto_approve` in the `shadow` mode or the `guidance` mode estimates the effect of the `auto_approve` mode.

## Broker

The broker adds no event, no instrument, and no attribute. The record "approval created" does
not change. The audit record `assessment.tool_call.assessed` of ExtProc has the assessment status
of each call that waits for a person.
