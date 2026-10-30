# ADR 20260811-120000: Token Exchange Client-Assertion Trust Anchor

**Status**: Proposed
**Date**: 2026-08-11

---

## Context

RFC 8693 token exchange validates two credentials with different trust relationships:

- The `subject_token` represents the user or agent context and is validated against the issuer appropriate to the selected OAuth2 authorization-server mode.
- The `client_assertion` identifies the privileged gateway or control-plane caller and must be validated against a trusted external identity provider.

Using the proxy upstream as the implicit source of both credentials couples client-assertion trust to proxy token minting. That coupling prevents a local-mode deployment from enabling token exchange even when it has an external identity provider that can issue privileged client assertions. It also obscures a critical boundary: a token minted by the broker is not a privileged-client credential and must never be accepted as a `client_assertion`.

Deployments need one explicit, independently configurable trust anchor for client assertions while retaining existing proxy and hybrid deployments without configuration changes. Some external identity providers do not expose OIDC discovery metadata, so the JWKS endpoint must also be configurable directly.

## Decision

Introduce `token_exchange.client_assertion` as the dedicated client-assertion trust configuration:

- `issuer_uri` identifies the issuer of trusted privileged-client assertions.
- `jwks_uri` optionally identifies that issuer's JWKS endpoint. When omitted, the broker discovers the endpoint from the issuer's OAuth2/OIDC metadata.
- `jwks_min_refresh` and `jwks_max_refresh` configure JWKS refresh bounds. Their defaults are 15 minutes and `max(jwks_min_refresh, 1 hour)`, respectively.

The resolved client-assertion issuer defaults to `oauth2_authorization_server.proxy.upstream_issuer_uri` when `issuer_uri` is unset. This preserves proxy and hybrid behavior while making the trust relationship explicit for configurations that need a different issuer.

Token exchange and machine-facing approval authentication are available in local, proxy, and hybrid modes when their required configuration and a resolved client-assertion trust anchor are present. Local mode therefore requires an explicit `token_exchange.client_assertion.issuer_uri`; it has no proxy upstream from which to derive one.

In local and hybrid modes, the resolved anchor MUST identify an external identity provider. Startup rejects an anchor equal to the broker's own issuer. Broker-minted tokens MUST NOT be accepted as client assertions.

Startup MUST fail closed when token-exchange or approval authentication is configured without a resolved client-assertion trust anchor, or when the required client-assertion JWKS provider cannot be initialized. Configuration validation permits an omitted optional anchor before mode resolution, but validates configured issuer and JWKS URLs and rejects negative refresh durations. HTTPS is required unless third-party HTTPS validation is explicitly disabled.

A defaulted proxy-upstream anchor may reuse the proxy-upstream JWKS provider. Any explicit client-assertion issuer, JWKS URI, or refresh setting creates a dedicated provider so that an operator's override is not discarded.

## Consequences

### Positive

- Local-mode deployments can perform token exchange with an external privileged-client identity provider without enabling proxy token minting.
- Proxy and hybrid deployments retain their existing upstream-based behavior when the new configuration is unset.
- The privileged-client boundary is explicit and auditable: broker-issued tokens cannot self-elevate into client assertions in modes that mint them.
- Operators can support identity providers without discovery metadata through an explicit `jwks_uri`.
- JWKS refresh policy is independently tunable for the client-assertion issuer.
- Invalid or incomplete trust configuration fails at startup rather than leaving machine-facing authentication partially configured.

### Negative

- Local-mode token-exchange deployments must configure and operate an external identity provider for client assertions.
- Operators must distinguish subject-token trust from client-assertion trust when configuring token exchange.
- A distinct client-assertion override can create an additional JWKS cache and outbound key-discovery or key-fetch dependency.

### Risks

- Misconfiguring the external issuer or JWKS endpoint prevents token exchange and machine-facing approval authentication from starting, by design.
- Choosing an external issuer with weak client-assertion issuance controls could grant privileged-client access too broadly; the issuer is therefore a security-sensitive deployment input.
- JWKS key rotation remains dependent on the configured refresh bounds and the availability of the external identity provider.
