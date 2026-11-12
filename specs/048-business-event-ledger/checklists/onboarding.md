# Onboarding Checklist: One Business Event

Use this checklist for each change that onboards an event to the business event ledger
([ADR 048](../../../adrs/048-business-events-as-typed-log-records.md)). In feature 048 it applies
to `consent.grant.revoked` ([spec](../spec.md) FR-012, SC-001). Later features reuse it.

## Scope of the change

- [ ] The change onboards one use case: one call site, or the call sites of one operation.
- [ ] Production code changes only in the package of the emitting service: the declaration and
      the log call.
- [ ] The change adds no migration, schema file, API contract, configuration key, or builder
      wiring.
- [ ] Besides production code, the change touches only tests (service tests, E2E tests, existing
      assertions on the changed record) and documents (glossary, pull request description).

## Declaration

- [ ] The event is declared one time, in the package of the service that emits it.
- [ ] The identifier follows LOGGING.md section 2.2: `<bounded_context>.<subject>.<past_tense_verb>`.
- [ ] If the identifier replaces an older one (for example an `action` value), the pull request
      description records the rename for the release notes (LOGGING.md section 2.2).
- [ ] The resource-owner visibility flag is set on purpose. A future owner-facing consumer must
      deny unknown or non-owner-visible definitions even if its supported-event list includes
      the identifier; ownership authorization is also required.

## Log call

- [ ] The record satisfies the LOGGING.md section 10 review checklist.
- [ ] The logger of the call has no attribute group (`WithGroup`). The ledger reads `event` and
      `principal` only at the top level, so a grouped record is not stored (FR-001).
- [ ] The call runs after the mutation commits and only when the operation changed state
      (LOGGING.md section 8).
- [ ] The record has `principal` (an `id.Principal`) when the event has a resource owner.
- [ ] Affected resources are identified by their canonical `<entity>_id` keys (LOGGING.md section 2.3).
- [ ] A principal that the log call supplies appears only under `principal` or
      `operator_principal`. No other attribute that the call supplies holds a principal. The
      `actor` attribute is not part of this rule: the context handler adds it.
- [ ] The attributes are sufficient for a consumer to describe the event without a second lookup
      of data that the operation can delete.
- [ ] If the event is also stored when a follow-up step fails, an attribute reports that outcome.
      The event contract distinguishes the committed fact from the follow-up outcome and states
      how consumers must use the attribute when describing or signalling either one.

## Tests

- [ ] A service test uses the production ledger handler with a test-local capturing appender and
      asserts one row with the event identifier and key attributes, without a production reader.
- [ ] A test asserts that a rejected or unchanged operation stores no row.
- [ ] On a path that handles a credential, a test asserts that the credential is absent from the
      stored row.
- [ ] Existing log assertions for the call site assert `event`, not message text.
