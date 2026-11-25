---
title: "Observability and audit"
description: Living specification of how a platform operator follows a request through the gateway filter and the broker, reads the system in aggregate, and answers afterwards who did what. Every example on this page is checked against the acceptance suite.
---

# Observability and audit

A platform operator has three questions about the broker. Where did this one request go? How does
the system behave in aggregate? Who did what? Traces answer the first, metrics the second, and
logs with audit records the third. One **Trace ID** ties them together.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

Most evidence on this page is telemetry, not HTTP. Span names, metric names, log fields, and
configuration appear in plain code blocks and tables. They are illustrations taken from the
scenarios and from the binding standards. Only the blocks that show a request and a response are
compared.

## The cast

An agent calls a tool through a gateway. The gateway asks the **ExtProc** filter for a token, and
the filter asks the broker. The gateway authenticates to the broker as the client
`privileged-gateway-client` and acts for the user `user@example.com`. An administrator works on
the admin server as `admin@example.com`. The provider is *GitHub*.

The operator runs an OpenTelemetry collector and a log store.

## Telemetry is off until the operator turns it on

A broker with no telemetry configuration answers requests and records no request span. `example:observability-and-audit.telemetry.off-by-default`
The same holds for the filter: with its default configuration it exchanges tokens and exports
nothing. `example:observability-and-audit.extproc.off-by-default`

The operator turns telemetry on in configuration and names the collector (illustration, from the
configuration reference):

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker
  exporter:
    protocol: grpc              # grpc, http, or https
    endpoint: otel-collector:4317
```

The filter has the same `telemetry` block in its own configuration file.

### A dead collector does not stop the broker

Telemetry fails open. The broker starts when the collector cannot be reached, and it keeps
answering requests while every export fails. `example:observability-and-audit.telemetry.collector-unreachable`
The filter behaves the same way: token exchanges still reach the broker while the collector is
down. `example:observability-and-audit.extproc.collector-unreachable`

## One request, one trace

With tracing on, an inbound request produces a server span. The span name is the method and the
route, and the span carries the response status. `example:observability-and-audit.traces.request-span`
The broker runs two servers. A span from the admin server says so in the attribute
`net.host.name`, so the operator can tell the two apart in one backend. `example:observability-and-audit.traces.admin-server`

```text
GET /.well-known/oauth-authorization-server     http.status_code=200
GET /api/agents                                 http.status_code=200  net.host.name=admin
```

The health endpoint is not traced.

Below the request span, the broker creates child spans for the calls that leave the process.
Their names are constants; values go in attributes. These names are illustrations from the
OpenTelemetry standard and the configuration reference:

| Span | Covers |
|---|---|
| `storage.get.agent`, `storage.create.user_grant`, … | One PostgreSQL repository call each |
| `encryption.encrypt`, `encryption.decrypt` | Envelope encryption |
| `oauth2session.background_refresh` | A refresh that outlives the request. It is a new root span linked to the request. |
| `extproc.token_exchange` | One request in the filter |

### The trace continues through the gateway filter

The gateway sends the W3C `traceparent` header to the filter. The filter's span
`extproc.token_exchange` joins that trace; it does not start a new one. `example:observability-and-audit.extproc.trace-joined`

The filter then calls the broker and passes the trace on. The broker receives a `traceparent`
with the same Trace ID. Its parent is the filter's outbound call, which is a child of
`extproc.token_exchange`. `example:observability-and-audit.extproc.trace-passed-on`

```text
gateway span                         trace 4bf92f35…0e4736
└─ extproc.token_exchange            (server)
   └─ HTTP client span               (client)
      └─ POST /oauth2/token          (broker, server)
```

The broker's own span in this picture is an illustration: the filter scenarios run against a
stand-in for the broker.

## The Trace ID

Every request to the broker has a Trace ID. The broker returns it in the W3C `traceresponse`
response header, so a caller can quote it in a support ticket: `example:observability-and-audit.trace-id.traceresponse`

```http example="observability-and-audit.trace-id.traceresponse"
GET /api/me HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 200 OK
Traceresponse: 00-…-01

{
  "data": {
    "principal": "user@example.com",
    "displayName": "user@example.com"
  }
}
```

The value has four fields: version, Trace ID, span ID, and flags. The second field is the Trace ID.

When the caller sends a valid `traceparent`, the broker reuses its Trace ID: `example:observability-and-audit.trace-id.reused`

```http example="observability-and-audit.trace-id.reused"
GET /api/me HTTP/1.1
X-Remote-User: user@example.com
Traceparent: 00-1234567890abcdef1234567890abcdef-1111111111111111-01

HTTP/1.1 200 OK
Traceresponse: 00-1234567890abcdef1234567890abcdef-…-01

{
  "…": "…"
}
```

When the caller sends none, the broker generates one: 32 lowercase hexadecimal characters. `example:observability-and-audit.trace-id.generated`

A rejected request has a Trace ID too. The operator can find the refusal in the logs from the
header alone: `example:observability-and-audit.trace-id.rejected-request`

```http example="observability-and-audit.trace-id.rejected-request"
GET /api/consent/agents HTTP/1.1

HTTP/1.1 401 Unauthorized
Traceresponse: 00-…-01

{
  "error": "missing or empty principal"
}
```

The option `request_context.trace.response_enabled` controls the header. It is on by default.

### Every log record of the request carries it

The broker writes structured logs. Every record that it writes while it handles a request has
the field `trace_id` with the request's Trace ID. A token exchange writes several records, and
all of them carry the same value. `example:observability-and-audit.trace-id.every-log-record`

The filter does the same. Its log records carry the Trace ID of the gateway's `traceparent`,
whether or not policy authorization is on. `example:observability-and-audit.extproc.log-trace-id`
So one search for a Trace ID returns the filter's records and the broker's records together.

## Who made the request

The broker builds a **request security context** when a request arrives and completes it when
authentication finishes. Code does not add these fields to a log call; they come from the
context. When `user@example.com` calls `GET /api/me`, the access log record carries the Trace
ID, the actor, and the client address, and no calling peer. `example:observability-and-audit.context.actor`
The record looks like this (illustration):

```json
{
  "level": "INFO",
  "msg": "HTTP request",
  "method": "GET",
  "path": "/api/me",
  "status": 200,
  "trace_id": "899e6c7bed862915478c5db23482e74f",
  "actor": "user@example.com",
  "client_ip": "10.0.0.5"
}
```

| Field | Meaning |
|---|---|
| `trace_id` | The request's Trace ID |
| `actor` | The authenticated **Principal** the request acts for, or `anonymous` |
| `calling_peer` | The authenticated client that sent the request, when it is not the actor. Absent otherwise. |
| `client_ip` | The network address of the caller |

The same scenario shows that the audit record of an OAuth2 authorization request carries the
`trace_id` of its request and the `actor`.

### Anonymous is a value, not a gap

A public endpoint needs no authentication. Its records still have an actor, and the value is the
literal `anonymous`. There is no `calling_peer`. `example:observability-and-audit.context.anonymous`
The filter authenticates nobody itself, so its records also say `actor=anonymous` and have no
`calling_peer`. `example:observability-and-audit.extproc.anonymous-actor`

### The actor and the calling peer are two identities

In a token exchange, the gateway sends the request and the user is the subject. The broker keeps
them apart: `actor` is `user@example.com` and `calling_peer` is `privileged-gateway-client`. `example:observability-and-audit.context.calling-peer`
An investigator can therefore tell "the user did this" from "this client did it for the user".

The context does not change on the way down. The domain service and the storage repository see
the same Trace ID, actor, calling peer, client address, user agent, method, and target as the
HTTP layer. `example:observability-and-audit.context.every-layer`

### The client address comes from the connection unless a proxy is trusted

By default the broker ignores forwarding headers. A caller that sends `X-Forwarded-For: 1.2.3.4`
does not get `1.2.3.4` into the record; the broker records the address of the connection. `example:observability-and-audit.client-address.forged-header`

Behind a load balancer the operator sets `request_context.trusted_proxy.enabled` to `true` and
names the header in `request_context.trusted_proxy.forwarded_header`. The broker then takes the right-most entry of the header, which is the one the proxy added. For
`X-Forwarded-For: 9.9.9.9, 10.0.0.5` the record says `client_ip=10.0.0.5`. `example:observability-and-audit.client-address.trusted-proxy`

## What is never recorded

Logs and the request security context hold no credentials. A token exchange sent with an
authorization code in the query string, an `Authorization` header, and a cookie leaves none of
the three values in any log record or in the context. The recorded target is `/oauth2/token`,
without the query. `example:observability-and-audit.secrets.not-recorded`

The binding logging standard forbids more than this scenario checks: tokens, client secrets, key
material, request bodies, and end-user profile data. See "Sources".

## Audit records

An audit record is a structured log record written after a change commits. It is operational
evidence, not a durable ledger.

When Alice grants the *Research Assistant* access, the broker writes one record per committed
write. The record names the action (`grant_created` or `grant_updated`), the `principal`, the
`agent_id`, the `grant_id`, the timestamps of the grant, and the granted permission sets with
their included services. It does not contain the agent's display name or description, or the
permission set's description. Two concurrent first submissions produce one grant and two records,
one for each write. A later identical submission changes nothing and writes no record. `example:observability-and-audit.audit.grant-records`

A record written inside a request also gets `trace_id` and `actor` from the request security
context, as described above for the authorization audit record.

## The system in aggregate

Metrics have no user in them. No metric carries a principal, a session ID, or a grant ID.

The session-health gauge `oauth2_sessions` counts stored **UserSession** records. Its only
attributes are `service_id`, `expiry`, and `refreshable`. `example:observability-and-audit.metrics.session-health-attributes`

Refresh outcomes are counted per service. A sweep that refreshes one session at GitHub and fails
on a session of another service adds one to `session_sweep_refreshed_total` for the first and one
to `session_sweep_failed_total` for the second, with the reason as `failure_detail`. `example:observability-and-audit.metrics.refresh-outcomes`

| Instrument | Kind | Attributes |
|---|---|---|
| `oauth2_sessions` | Gauge | `service_id`, `expiry` (`valid`, `expiring`, `expired`, `unknown`), `refreshable` |
| `session_sweep_refreshed_total`, `session_sweep_failed_total` | Counter | `triggered_by`, `service_id`, `failure_detail` on failures |
| `proactive_refresh_triggered_total`, `proactive_refresh_failed_total` | Counter | `triggered_by`, `service_id`, `failure_detail` on failures |
| `session_sweep_duration_seconds` | Histogram | `outcome`, `dry_run` |

What the sweep and the refresh do is on [Third-party sessions](third-party-sessions.md).

The filter counts every token exchange in `extproc.token_exchange.requests` and measures it in
`extproc.token_exchange.duration`. Both carry the same six `token_exchange.*` attributes as the
span below and no caller-supplied value. For a successful exchange the outcome is `success` and
the failure and recovery attributes are `none`. `example:observability-and-audit.extproc.metrics`

The filter also counts the decisions of its approval gate; see [Tool approvals](tool-approvals.md).

## A re-authentication challenge is not a failure

The filter can answer the gateway with an error for two different reasons, and the operator must
not page anyone for the first.

The user must reconnect an account. The broker refuses the exchange and names a recovery URL. The
filter answers `403` with a challenge. Its span has the outcome `reauth_required`, and the span
status stays unset: `example:observability-and-audit.extproc.reauth-challenge`

```text
extproc.token_exchange     status=Unset
  token_exchange.outcome=reauth_required
  token_exchange.failure_stage=exchange_routing
```

The broker refuses the exchange and names no recovery URL. The filter answers `500`. The span
status is `Error`, and the attributes say what the broker answered: `example:observability-and-audit.extproc.broker-failure`

```text
extproc.token_exchange     status=Error
  token_exchange.outcome=authentication_failed
  token_exchange.failure_stage=exchange_routing
  token_exchange.failure_detail=credential_rejected
  token_exchange.recovery_action=none
  token_exchange.broker_status_code=400
  token_exchange.oauth_error_code=invalid_grant
```

An alert on spans with status `Error`, or on `token_exchange.outcome`, therefore separates users
who must act from a system that is broken. The span never carries the requested resource URI.

## Not yet on this page

- Child spans for storage, encryption, key fetches, and provider calls. The table lists their
  names, but no scenario on this page asserts them: the backend suite uses in-memory storage.
- HTTP request metrics and Go runtime metrics of the broker. No acceptance scenario reads them.
- Log export over OTLP, sampling, propagator choice, resource attributes, and exporter headers.
- The trace through a real gateway container, and the metrics scenarios that need PostgreSQL.
  Both need Docker.
- Audit records for agents, services, credentials, keys, and token issuance. Only the grant
  records are bound here.
- Principals on spans. The standard reserves the rule; the mechanism does not exist yet.

## Sources

The behavior on this page was decided in `specs/017-opentelemetry-support/spec.md`,
`specs/027-extproc-otel/spec.md`, `specs/033-request-security-context/spec.md`, and the metrics
part of `specs/053-session-auto-refresh/spec.md`. The decisions are
`adrs/011-opentelemetry-provider-pattern.md`, `adrs/027-extproc-telemetry-shared-dependency.md`,
and `adrs/033-request-security-context-propagation.md`, which still has the status Proposed. The
binding standards are `coding-standards/OPENTELEMETRY.md` and `coding-standards/LOGGING.md`. The
options are in `docs/configuration.md`. The scenarios are in `tests/e2e/telemetry_test.go`,
`tests/e2e/request_security_context_test.go`, `tests/e2e/session_metrics_test.go`,
`tests/e2e/extproc/telemetry_test.go`, `tests/e2e/extproc/request_trace_context_test.go`, and
`tests/integration/grant_lifecycle_audit_test.go`.
