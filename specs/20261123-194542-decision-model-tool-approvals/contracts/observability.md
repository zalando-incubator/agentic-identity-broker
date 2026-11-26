# Contract: Observability (logs, metrics, traces)

**Feature**: [spec.md](../spec.md) FR-060 to FR-062, SR-007 | **Decisions**: [research.md](../research.md) D-12

Operators make alerts from these names. After release, a new name for an event, a span, an
instrument, or an attribute is a breaking change.

This contract obeys `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. The
implementation adds the namespace `assessment` to the namespace table in section 4.1 of
`coding-standards/OPENTELEMETRY.md`. It adds the instruments to "What is Instrumented" in
`docs/configuration.md`.

## The record of an assessed call

The feature writes no separate audit record (FR-060, D-12). It uses the existing decision record
of ExtProc.

### The existing decision record of ExtProc

ExtProc already writes the record "opa authorization decision" for each policy evaluation
(`internal/extproc/authorization/authorizer.go`). It occurs two times for an assessed call: one
time for each evaluation.

| Attribute | Value | Present |
|---|---|---|
| `action` | The action of the policy result | Each evaluation, as today |
| `reasons` | The reasons of a denial | Each evaluation, as today |
| `tool_name`, `method`, `protocol`, `target_server_name` | The dimensions of the request | Each evaluation, as today |
| `duration_ms`, `result_code` | The duration and the result of the evaluation | Each evaluation, as today |
| `assessment_mode` | `shadow`, `guidance`, or `auto_approve` | New. Only for the evaluation after an assessment |
| `assessment_status` | A status of D-04 | New. Only for the evaluation after an assessment |
| `assessment_answers` | A group with the question name as the key. The value is the probability, or the list of level probabilities, or the answer state if no value exists | New. Only for the evaluation after an assessment |

The record of an evaluation without an assessment does not change. The question names are
values of the configuration. The record does not have the answering model, because that value
comes from the response of the decision endpoint and no configuration bounds it
(`coding-standards/LOGGING.md`, section 6.2).

In the `shadow` mode, the `action` of the second record is the action that the policy returns
with the answers. The system does not use it (FR-009).

The record already goes through OpenTelemetry. The logger of ExtProc has a bridge to the
OpenTelemetry log pipeline, which is active if `telemetry.enabled` and `telemetry.logs.enabled`
are true. Each evaluation also has the existing span `extproc.opa.evaluate`. The span gets no new
attribute.

A policy must not write argument values or assessment data into its `reasons`.

### The decision log of OPA

- If the OPA configuration enables decision logs, OPA writes the full input and the result of each evaluation. For the second evaluation, the input contains `assessment` and `mcp.arguments`. An operator can use the log for an analysis outside the product.
- OPA writes decision logs only if ExtProc uses an OPA configuration file. With a local policy path, no decision log exists. The decision record of ExtProc exists in each case.
- The operator decides who can read the log and whether to mask a part of it, for example `/input/mcp/arguments`.

## Data that no signal contains

This contract applies to the signals that ExtProc writes for this feature and to the new
attributes of the existing decision record. None of them contains these items:

- An argument value or an argument name
- The assessment context or a part of it
- Text from a tool definition
- Question text or criteria
- The request body or the response body of the model request
- The credential for the decision endpoint
- A principal.

The feature adds no tool name to a signal. The existing decision record keeps its `tool_name`
attribute (D-12).

## Structured log events

| `event` | Level | Written by | Attributes |
|---|---|---|---|
| `assessment.question_set.loaded` | INFO | ExtProc at startup | `question_count`, `mode` |

The feature writes no separate record for one call.

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

## Broker

The broker adds no event, no instrument, and no attribute. The record "approval created" does
not change.
