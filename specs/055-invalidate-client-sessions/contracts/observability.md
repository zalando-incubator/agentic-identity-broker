# Contract: Observability

**Status**: Internal, following `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. Event identifiers are a dashboard contract (LOGGING §2.2).

## Log events

| Event | Level | When | Attributes |
|---|---|---|---|
| `session.oauth2.client_identity_changed` (new) | `Info` | A revision increment commits, once per increment and only from the writer that won the compare-and-set or committed the update | `service_id`, `client_identity_revision` (new value), `channel` (`admin_update` or `filesystem_observation`), `operator_principal` (for `admin_update` only, through `id.Principal`) |
| `session.oauth2.credential_source_failed` (existing) | `Error` for source unavailability; `Warn` for identity reasons | A use is denied before provider authentication | Existing attributes. `reason` gains `identity_superseded`. `operation` gains `token_acquisition`. |
| `session.oauth2.refresh_failed` (existing) | existing | A refresh or sweep candidate is denied for identity | Existing `oauth2_session` metadata with detail `credential_source_unavailable` |

Never log the client ID value, file paths or contents, tokens, or secrets (LOGGING §6.1). The client ID is non-secret, but it is not needed for audit, and keeping it out of the log keeps the data surface unchanged.

## Spans and metrics

No new span or instrument. Existing session refresh, sweep, and token-exchange spans and counters classify identity denials through their existing failure-detail attributes. No principal and no client ID appears in any metric or span attribute.
