---
title: "ExtProc token cache"
description: Current ExtProc token-cache lifetimes, stale fallback, metadata, and discrepancies with accepted policy.
---

# ExtProc token cache

This reference describes the current implementation, not a new cache policy.
Accepted [ADR 012](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/012-extproc-in-memory-token-cache.md) remains unchanged.
Its decision specifies an expiry sweep every `default_ttl / 2`, with a 30-second minimum.
The implementation uses a one-second minimum and retains successful entries through a separate stale deadline.
These differences remain implementation discrepancies, not approved changes to the accepted decision.

## Fresh reads and refresh

A fresh entry returns its token and metadata without a broker call, provided the client assertion remains usable.
A fresh re-authentication entry returns its cached error instead.
An absent or expired assertion bypasses both fresh-entry paths and prevents stale fallback.
Sources: [fresh reads and stale fallback in `TokenExchanger.Exchange`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go), and [assertion check in `TokenExchanger.isAssertionExpired`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

An expired entry remains in the map during refresh. A miss or expiry enters singleflight, which checks the cache again.
Each caller can cancel its wait without canceling the shared exchange.
Sources: [refresh path in `TokenExchanger.Exchange`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go) and [caller cancellation in `awaitResult`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

## Expiry and stale reuse

A successful exchange sets `expiresAt` to insertion time plus the fresh-cache lifetime.
For positive `expires_in`, that lifetime is `max(lifetime - max(10s, lifetime / 10), lifetime / 2)`, capped at `cache.max_ttl`.
Only absent or null expiry uses `cache.default_ttl`, capped at `cache.max_ttl`.
Known nonpositive lifetime deletes the entry and is not cached, including for outage fallback.
Sources: [population](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go) and [TTL calculation](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

For positive `expires_in`, insertion sets `staleUntil` to insertion time plus `min(expires_in, cache.max_ttl) + 5s`, capped at `cache.max_ttl`.
For absent or null `expires_in`, the fallback window is `min(min(cache.default_ttl, cache.max_ttl) + 5s, cache.max_ttl)` from insertion.
The broker reports no token lifetime in that case. With the default configuration, the fallback window is five minutes and five seconds.
Transient failures do not extend this deadline. This can permit reuse after reported token expiry.
Sources: [five-second constant, population, and fallback](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

After an exchange error, fallback requires a nonempty cached token, a usable assertion, and a current time strictly before `staleUntil`.
The classifier permits broker 5xx errors and HTTP 429 without `error_uri`.
It rejects assertion expiry and other `BrokerExchangeError` values, including HTTP 429 with `error_uri`.
Other errors also qualify, including network, circuit-open, and malformed-success-response errors. The classifier does not restrict fallback to transport errors.
If a response body read fails, `doExchange` returns the read error before it creates a `BrokerExchangeError`, even for HTTP 403.
The classifier treats that error as transient, even for an HTTP 403 response.
Fallback can return a cached token within its stale window if the assertion remains usable.
Sources: [fallback and circuit breaker in `TokenExchanger.Exchange`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go), [classifier `isTransientBrokerError`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go), and [response errors in `TokenExchanger.doExchange`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

A non-transient broker error with `error_uri` replaces the entry with a five-second re-authentication error.
Other exchange errors do not delete the previous entry.
A rejection represented by `BrokerExchangeError` returns an error for that call, but does not generally erase the old token.
Source: [error population and propagation in `TokenExchanger.Exchange`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

## Retention, capacity, and cleanup

The sweep runs on a ticker with interval `max(cache.default_ttl / 2, 1s)`.
It deletes an expired entry only when `staleUntil` is zero or strictly earlier than the sweep time.
Thus, success entries survive ordinary expiry through their stale window. Re-authentication entries have no stale window.
Sources: [error entries in `TokenExchanger.Exchange`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go) and [sweep in `TokenExchanger.evictExpired`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

Neither a fresh read nor a failed refresh guarantees deletion.
An entry can remain in memory after reuse is forbidden, until a later sweep or replacement.
The eviction loop also performs assertion refresh, so its ticker interval is not an exact deletion deadline.
`Shutdown` signals the loop to stop. It does not clear the map.
Sources: [`TokenExchanger.Exchange`, `TokenExchanger.Shutdown`, and `TokenExchanger.runEviction`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

There is no capacity eviction, maximum entry count, or byte limit.
TTL cleanup limits retention during normal operation, not the number of distinct keys that can arrive before cleanup.
Sources: [`CacheConfig`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/config/config.go), and [insertion in `TokenExchanger.Exchange` and eviction in `TokenExchanger.evictExpired`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/exchanger.go).

The server uses successful results, including stale results, to replace the Authorization header.
For body-bearing requests, token exchange precedes OPA evaluation. The OPA input receives the returned granted permission sets.
The broker-issued principal and agent ID go to the approval gate, not the OPA input.
For header-only requests, `processHeadersOnlyOPA` evaluates OPA before token exchange. OPA does not receive the returned token-bound metadata on that path.
Sources: [`Server.processRequestHeaders`, `Server.processRequestHeadersOPA`, `Server.processRequestBody`, and `Server.processHeadersOnlyOPA`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/internal/extproc/server/server.go).

## Unresolved policy conflict

[Feature 015, FR-015](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/015-extproc-token-exchange/spec.md#functional-requirements) requires eviction before reuse of expired entries.
[ADR 012](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/012-extproc-in-memory-token-cache.md) also describes expiry-based removal. Current stale retention and reuse do not satisfy that literal requirement.
Accepted [036-extproc-approval-cache-and-gate.md](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/036-extproc-approval-cache-and-gate.md#separate-from-adr-012) acknowledges bounded stale-token reuse but also describes invalidation at token expiry.
These statements do not establish one consistent expiry policy.

[Constitution II](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/.specify/memory/constitution.md#ii-architecture-documentation--decision-records) requires a superseding ADR for deviations from accepted decisions.
This reference records the implementation discrepancies for review. It does not amend the accepted ADRs.
It neither approves stale-token reuse as a new security exception nor changes runtime behavior.
