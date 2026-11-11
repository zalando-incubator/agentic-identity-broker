# ADR 048: Wildcard Web Redirect URIs

**Date**: 2026-10-29
**Status**: Accepted
**Feature**: 055-wildcard-redirect-uri

## Context

Preview deployments use different HTTPS callback hostnames, while the callback path and trust domain remain stable. Requiring an exact Agent callback registration for every preview creates repeated configuration. Allowing an unrestricted hostname wildcard would permit callbacks outside the intended domain boundary.

RFC 9700 §2.1 requires exact string matching, except for localhost ports in native-app redirects. This ADR defines a narrow exception for opaque Agent callbacks because preview hosts vary by deployment. The exception keeps the scheme, port, path, query, registrable domain, and public suffix literal. It does not define a general redirect pattern language.

The broker already supports wildcards for CIMD `client_uris`, but those are path-segment patterns with a different trust purpose. Private-use callback URIs have a separate exact-match rule in ADR 042. Neither behavior defines the policy for web redirect callback hosts.

## Decision

1. An Agent may register HTTPS callback patterns with `*` in one or more subdomain labels.
2. Each `*` matches zero or more characters within one hostname label. It never matches a dot. The resulting label must be a valid DNS label.
3. The wildcard pattern must have a literal public suffix and effective registrable domain. Determine the boundary with the bundled `golang.org/x/net/publicsuffix` data after applying the `golang.org/x/net/idna` lookup profile to a safe probe hostname. Reject wildcard patterns whose probe hostname is an IP literal, a single-label name, or cannot be classified. This restriction applies only to wildcard registrations; exact HTTPS callbacks retain existing validation behavior.
4. The wildcard applies only to the host. Scheme, port, path, query, and other URI components remain exact. The incoming authorization request must use a concrete callback URI without wildcard characters.
5. Use wildcard matching only for opaque Agent callbacks. CIMD document web callbacks remain exact-match-only. Private-use callbacks remain byte-for-byte exact and keep their Agent allowlist requirement. Loopback HTTP port matching remains unchanged.
6. Validate wildcard registrations when Agents are created or updated, and fail closed if a stored pattern is invalid when it is used for authorization.
7. After a successful match, redirect only to the concrete `redirect_uri` from the authorization request. Never construct a redirect destination from the registered pattern.
8. The suffix list is bundled with the dependency. Registration and authorization must not make network requests to resolve domain boundaries.
9. Keep exact string matching as the default. Apply this bounded exception only to valid HTTPS host patterns registered on opaque Agents. CIMD callbacks and private-use callbacks keep their existing exact-match rules.

## Consequences

- Operators can register one bounded callback pattern for a family of preview hosts.
- Existing exact HTTPS registrations without host wildcards, loopback web, CIMD, and private-use callback behaviors remain unchanged.
- The domain layer gains a dependency on the existing `golang.org/x/net/idna` and `golang.org/x/net/publicsuffix` packages; no new dependency, configuration setting, API endpoint, database field, or migration is required.
- The bundled Public Suffix List changes only when the project updates `golang.org/x/net`.
- Wildcard patterns that cannot be classified safely are rejected rather than matched with a fallback rule.
- A stored `*` in an HTTPS hostname gains wildcard meaning after this feature is deployed. Operators must audit existing Agent registrations before upgrading.
- An update to `golang.org/x/net` can change Public Suffix List classifications. Operators must review stored patterns when they upgrade this dependency.

## References

- Feature specification: `specs/055-wildcard-redirect-uri/spec.md`
- Research and alternatives: `specs/055-wildcard-redirect-uri/research.md`
- Operational guidance: `docs/operations/redirect-uri-patterns.md`
