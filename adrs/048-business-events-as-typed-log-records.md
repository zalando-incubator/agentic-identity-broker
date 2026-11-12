# ADR 048: Business Events as Typed Log Records

**Status**: Proposed
**Date**: 2026-11-12
**Feature**: 048-business-event-ledger (issue #188)
**Supersedes**: No ADR. Replaces the design of the closed PR #381 (the first design of feature 048, never merged).

## Context

Issue #188 asks for a user-facing activity view of what agents did with a user's delegated access.
Later work adds an OpenID Shared Signals Framework (SSF) transmitter. Both need a durable, queryable
record of security-relevant facts: a grant was revoked, a session was established, an approval was
denied.

The broker already produces these facts. `coding-standards/LOGGING.md` requires an audit record
with an `event` identifier for each security-critical operation (section 8), fixes the identifier
shape and the canonical attribute keys (section 2), and forbids credentials and profile data in
any record (section 6). One handler chain, built in `internal/app/builder.go`, processes every
record: `telemetry.ContextHandler` adds `trace_id`, `actor`, and `calling_peer` from the request
security context (ADR 033), and the principal policy handler renders each `id.Principal`.

The first design for a ledger (PR #381) added a separate recorder API. Each producing service
called it inside the storage transaction of the state change, and each event type had a JSON
Schema. That design gave an atomic guarantee: a state change and its event commit together. It
also had these costs:

- A second event catalogue that mirrors the `event` identifiers of the logging standard. Each
  change to an audit record must be made in two places.
- Changes to every producing service, both storage adapters, and every transaction boundary. The
  pull request changed 368 files and was not reviewable.
- A fixed schema for every event type before the first consumer existed.

LOGGING.md section 8 requires that an audit record is written after the mutation commits. A record
that is logged inside a transaction reaches stdout before the commit and is false if the commit
fails. A ledger that is fed from log records therefore cannot also be transactional.

## Decision

1. **A business event is a log record.** A business event is an audit or security record
   (LOGGING.md section 8) whose `event` attribute is a typed business event definition. The broker
   has no second API to record business events. A domain service records one by a log call on the
   logger that its constructor already requires.
2. **One declaration per event.** A business event definition is declared one time in Go, in the
   package of the service that emits it. The declaration holds the LOGGING.md section 2.2
   identifier and the flags that consumers need, for example whether the resource owner can see
   the event. The definition implements `slog.LogValuer` and renders as the identifier. The
   stdout and OpenTelemetry output of a typed event is the same as the output of a string
   identifier. No JSON Schema, registry file, or catalogue document repeats the declaration.
3. **A ledger handler persists typed events.** A `slog.Handler` in `internal/adapters/telemetry`
   joins the builder chain behind the context handler and beside the principal policy handler. It
   recognises a business event by the type of the `event` value, as the principal policy handler
   recognises `id.Principal`. It ignores all other records. A record with a string `event` stays
   an operational log only.
4. **The ledger row is the record.** The handler builds one row from the record time, the event
   identifier, the `principal` attribute, the `trace_id` and `actor` that the context handler
   supplies, and all other attributes as one JSON document. The ledger table promotes to a column
   only what a query filters or sorts on. A new event type or a new attribute needs no migration.
   Consumers read the canonical keys of LOGGING.md section 2.3.
5. **The row is written after the commit.** The handler writes the row when the log call runs,
   which LOGGING.md section 8 places after the commit. The write is not part of the state change's
   transaction. The repository operation uses the connection pool and never a transaction that the
   context carries. The write is synchronous, has a short timeout, and does not stop when the
   request context is cancelled.
6. **A failed write is counted, not hidden.** slog discards handler errors, so the call site
   cannot react. On a failed write the handler increments a metric and writes one `Error` record
   directly to the operational log sinks, not through itself. The original record still reaches
   stdout and OpenTelemetry.
7. **The ledger stores the principal that the service logged.** The handler takes the typed
   `id.Principal` before the principal policy renders it. A later change of the log policy, for
   example pseudonymisation, does not break queries by principal. Retention must exist before the
   ledger is enabled in a production deployment.
8. **The ledger is off by default** until the retention operation is merged. A
   configuration flag enables it.
9. **A stored event is immutable.** No operation updates a ledger row. Retention deletes
   whole rows when they are older than the retention period.
10. **Consumers read the table.** The activity API and the SSF transmitter read the ledger table.
   They do not read the log stream. Each consumer projects an allow-list of events. A user-facing
   sentence or an SSF event mapping belongs to the consumer and is keyed by the event identifier.

The package name of the definition type, the table and column names, the configuration key, and
the metric name are set in the implementing pull requests and their contracts in
`specs/048-business-event-ledger/`.

## Consequences

**Positive**

- One source of truth. The audit record, the ledger row, and the OpenTelemetry log record are the
  same fact with the same identifier and attributes.
- To onboard a use case, a pull request changes one log call and declares one definition. Each
  use case can merge separately, and no event needs a schema before it is onboarded.
- The data rules of LOGGING.md section 6 and their safety tests apply to the ledger without
  repetition, because a row holds only what the record holds.
- Domain services get no new dependency. The ledger is wiring in `internal/app`.

**Negative**

- **No atomic guarantee.** The ledger row is written after the commit, when the log call runs.
  - If the process stops after the commit and before the log call, the ledger has no row and the
    operational log has no record of a state change that happened. No metric counts this loss.
    This window exists on `main` today for the audit record; the ledger inherits it.
  - If the database rejects the insert, the ledger has no row. The operational log has the
    record, and the failure metric counts the miss.
  - If the write times out, the result is unknown: the database can commit the insert after the
    handler stops waiting. The failure metric counts the attempt, and the row can exist.
  - If the process stops during the log call, either sink can be without the record.
- Each onboarded state change costs one more database round trip on the request path.
- The ledger holds personal data (principals) in a new table.
- The ledger has no operation that erases the events of one principal. A row stays until
  retention deletes it. If a deployment needs erasure on request, that is a later decision, and
  the operation must delete whole rows (Decision 9).
- ExtProc must not import broker domain packages (ADR 027) and cannot use the typed definition.
  Events from ExtProc and agentgateway need a different ingest path and are not part of this
  decision.
- A record below the enabled level does not reach a handler. Business events are logged at `Info`
  or above (LOGGING.md section 3), and the ledger handler must report `Info` as enabled
  independently of `log.level`.

**Revisit when**

- The SSF transmitter delivers revocation events to receivers that act on them. A lost
  `consent.grant.revoked` row then means a missed revocation signal. The SSF feature must decide,
  in its own ADR, between a reconciliation job that derives missed terminal events from the source
  tables and a transactional append for the SSF-mapped event types. Neither option changes the
  ledger table or the call sites of this decision.
- The failure metric is not zero in a production deployment.
- High-volume events (token issuance, token exchange) are onboarded. The synchronous single-row
  write must then be measured and can be replaced by a batch writer.

## Alternatives Considered

- **Recorder port with a same-transaction outbox (PR #381).** Rejected for the costs in Context.
  It stays the reference design if a consumer needs the atomic guarantee for specific event types.
- **Persist every record that has an `event` attribute.** Needs no marker type. Rejected because
  diagnostic and lifecycle identifiers, for example `session.oauth2.refresh_retry`, would enter a
  table that end users and SSF receivers are served from, and each new diagnostic identifier
  would need a deny-list entry.
- **Read the ledger back from the OpenTelemetry Collector.** Rejected. The collector's documented
  loss scenarios are weaker than a direct write, and the broker would depend on infrastructure
  that some deployments do not run.
- **Log inside the transaction so that the handler joins it.** Rejected. It contradicts LOGGING.md
  section 8 and makes stdout report changes that can still roll back.

## Compliance

- Principle I: the ledger adds no new data class. Section 6 of the logging standard bounds its
  content.
- Principle VI: the handler is an adapter and depends on a storage port. Domain packages do not
  import it.
- Principle IX: the ledger table arrives by a tested migration, with a repository port and both
  storage adapters.
- Principle XV: `coding-standards/LOGGING.md` section 8 is amended in the pull request that
  introduces this ADR. The rule for the typed definition is added to section 2.2 in the pull
  request that adds the type.
