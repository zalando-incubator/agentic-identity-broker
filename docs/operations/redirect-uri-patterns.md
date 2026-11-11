# Redirect URI Pattern Operations

## Existing Agent registrations

A stored `*` in an HTTPS hostname gains wildcard meaning after deployment of ADR 048. Before this feature, the broker treated the value as an exact registration.

Before upgrading, examine every Agent `redirect_uris` value that contains `*` in its hostname. Make sure that each pattern has the intended domain boundary, callback path, port, and query.

Remove or replace entries that use `*` as a literal hostname character. Keep the existing registration unchanged if it is an intended bounded pattern.

## `golang.org/x/net` upgrades

The broker uses the bundled Public Suffix List from `golang.org/x/net` to validate wildcard registrations. It validates stored patterns again when it authorizes a request.

An update to `golang.org/x/net` can change Public Suffix List data. A pattern that was valid can then fail closed during authorization.

Before merging an `golang.org/x/net` update:

- Run `TestAgentRedirectURIWildcardPSLBoundaries`.
- Review changes to the bundled Public Suffix List.
- Test each stored pattern against the updated dependency.
- If a pattern fails, update it only when its replacement keeps the same or narrower trust boundary.

Do not weaken the public-suffix or registrable-domain checks to preserve a pattern.

See [ADR 048](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/048-wildcard-web-redirect-uris.md) and [the wildcard redirect specification](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/055-wildcard-redirect-uri/spec.md).
