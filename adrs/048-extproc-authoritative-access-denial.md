# ADR 048: ExtProc Authoritative Access Denial

**Status**: Accepted
**Date**: 2026-11-05

## Supersedes

This decision supersedes only the no-`error_uri` `access_denied` outcome in
[ADR 036](036-extproc-metadata-token-exchange-input.md)'s retained exchange-response
matrix and Feature 023's no-recovery error handling. The metadata trust boundary,
recovery challenges, cache, and standalone deployment remain unchanged.

## Context

[Issue #416](https://github.com/zalando-incubator/agentic-identity-broker/issues/416)
requests that an authoritative broker authorization denial remain a denial at the
gateway. Currently, a broker `403 access_denied` without `error_uri` becomes a
500 response and an ERROR log. Agent-subject third-party rejection (ADR 045) is
one such denial; it intentionally has no human-consent recovery URL.

## Decision

After retaining the existing non-transient `error_uri` recovery branch, ExtProc
maps a non-transient broker `access_denied` without `error_uri` to HTTP 403 with
JSON `error: access_denied` and the broker's `error_description`. It emits no
`WWW-Authenticate` challenge, URL elicitation, or authorization mutation.

This applies to every protocol and to token-only, OPA body-bearing, and OPA
header-only paths. The denial is logged once at INFO with event
`extproc.token_exchange.denied` and existing bounded error metadata. Broker
descriptions and wrapped causes remain excluded from logs. Existing diagnostic,
span, and metric classifications are unchanged.

Broker 5xx and 429 without recovery URLs remain transient, even if their OAuth
error code is `access_denied`. Other errors retain their 500/503 behavior. No
change permits stale-cache fallback for authoritative denials.

## Consequences

Clients and operators see an authorization denial rather than a gateway failure.
Clients that treated this particular 500 as retryable now receive 403; this is the
scoped response change requested in #416. No automatic re-consent is suggested
when the broker supplies no recovery URL.

The broker API, configuration, dependencies, and database schema do not change.
The sidecar remains outside the broker Helm chart under ADR 011.

## References

- [Feature 043 response matrix and denial journey](../specs/043-extproc-metadata-input/spec.md)
- [ExtProc gRPC contract](../specs/015-extproc-token-exchange/contracts/extproc-grpc.md)
- [ADR 045 agent-subject issuance](045-agent-subject-impersonation.md)
