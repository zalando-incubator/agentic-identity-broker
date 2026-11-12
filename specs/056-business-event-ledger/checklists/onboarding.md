# Onboarding Checklist: One Business Event per Pull Request

Use this checklist for each pull request that onboards an event to the business event ledger
([ADR 048](../../../adrs/048-business-events-as-typed-log-records.md), [spec](../spec.md) User Story 2).
It applies from delivery step 5.

## Scope of the pull request

- [ ] The pull request onboards one use case: one call site, or the call sites of one operation.
- [ ] The pull request changes only the service package and its tests. It adds no migration,
      schema file, API contract, or builder wiring.

## Declaration

- [ ] The event is declared one time, in the package of the service that emits it.
- [ ] The identifier follows LOGGING.md section 2.2: `<bounded_context>.<subject>.<past_tense_verb>`.
- [ ] If the identifier replaces an older one (for example an `action` value), the changelog
      records the rename (LOGGING.md section 2.2).
- [ ] The resource-owner visibility flag is set on purpose. An operator-only event is not visible.

## Log call

- [ ] The record satisfies the LOGGING.md section 10 review checklist.
- [ ] The call runs after the mutation commits and only when the operation changed state
      (LOGGING.md section 8).
- [ ] The record has `principal` (an `id.Principal`) when the event has a resource owner.
- [ ] Affected resources are identified by their canonical `<entity>_id` keys (LOGGING.md section 2.3).
- [ ] A principal appears only under `principal` or `operator_principal`. No other attribute
      holds a principal (FR-010).
- [ ] The attributes are sufficient for a consumer to describe the event without a second lookup
      of data that the operation can delete.

## Tests

- [ ] A service test injects the memory ledger through the production constructor path and asserts
      one row with the event identifier and the key attributes.
- [ ] A test asserts that a rejected or unchanged operation stores no row.
- [ ] On a path that handles a credential, a test asserts that the credential is absent from the
      stored row.
- [ ] Existing log assertions for the call site assert `event`, not message text.
