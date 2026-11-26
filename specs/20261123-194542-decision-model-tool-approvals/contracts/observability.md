# Contract: Observability (logs, metrics, traces)

**Feature**: [spec.md](../spec.md) FR-060 to FR-062, SR-007 | **Decisions**: [research.md](../research.md) D-12

Operators make alerts from these names. After release, a new name for an event, a span, an
instrument, or an attribute is a breaking change.

This contract obeys `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. The
implementation adds the namespace `assessment` to the namespace table in section 4.1 of
`coding-standards/OPENTELEMETRY.md`. It adds the instruments to "What is Instrumented" in
`docs/configuration.md`.

## The record of an assessed call

The feature writes no audit record of its own (FR-060, D-12). The record of an assessed call is
the decision log of OPA:

- If the OPA configuration enables decision logs, OPA writes the full input and the result of each evaluation. For the second evaluation, the input contains `assessment` and `mcp.arguments`. Thus, the log shows the call, the answers, and the decision. An operator can use it for an audit and for an analysis outside the product.
- OPA writes decision logs only if ExtProc uses an OPA configuration file. With a local policy path, no decision log exists.
- The operator decides who can read the log and whether to mask a part of it, for example `/input/mcp/arguments`.

The existing record "opa authorization decision" occurs two times for an assessed call: one time
for each evaluation. The feature does not change it. It contains the `reasons` of a policy
result without a change. A policy must not write argument values or assessment data into its
reasons.

## Data that no signal contains

This contract applies to the signals that ExtProc writes for this feature. No such log record,
span, or metric contains these items:

- An argument value or an argument name
- The assessment context or a part of it
- Text from a tool definition
- Question text or criteria
- The request body or the response body of the model request
- The credential for the decision endpoint
- A tool name or a principal.

## Structured log events

| `event` | Level | Written by | Attributes |
|---|---|---|---|
| `assessment.question_set.loaded` | INFO | ExtProc at startup | `question_count`, `mode` |

The feature writes no record for one call.

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
