# Compose Loopback OAuth Issuer and AuthZEN PDP

**Status**: Accepted
**Date**: 2026-11-25
**Supersedes in part**: `048-authzen-delegation-pip` §1, only the claim that AuthZEN and RFC 8414 metadata use the same base URL.

## Context

Docker Compose exposes the Vite frontend as `aib.localhost` to both the host browser and containers. ExtProc uses that network alias to discover the AuthZEN PDP. Claude Code rejects an HTTP token endpoint at `aib.localhost`, but permits HTTP for the literal loopback host `localhost`.

Changing `server.enduser.public_url` to `localhost` would break discovery inside the ExtProc container. The existing `oauth2_authorization_server.local.issuer_uri` already separates the OAuth2 issuer and endpoint base from the public URL.

## Decision

- Compose sets `oauth2_authorization_server.local.issuer_uri` to `http://localhost:3000`. RFC 8414 discovery advertises that issuer and its authorization, token, and JWKS endpoints.
- Compose keeps `server.enduser.public_url` at `http://aib.localhost:3000`. Consent links and AuthZEN metadata use this URL. ExtProc still requires exact PDP identity matching before it sends credentials.
- The gateway's protected-resource metadata and local JWT verifier use the OAuth2 issuer. The broker's issuer-gated agent lookup uses the same issuer. The external client-assertion trust anchor remains separate.
- This HTTP split is for local development only. Production deployments use HTTPS for both identifiers. Neither the broker nor Claude Code gains a non-loopback HTTP exception.

## Consequences

- An OAuth2 client can reach the Compose token endpoint on literal `localhost`. ExtProc can still reach the PDP through the Compose alias.
- Broker-issued tokens carry the new issuer. Existing tokens with the former issuer no longer pass gateway validation. Recreating the in-memory broker also clears its grants and sessions.
- Tests cover the two metadata contracts with distinct identifiers. The AuthZEN PDP remains independent of any OAuth2 issuer override.

## Alternatives

- Use `localhost` for both identifiers: rejected because `localhost` inside ExtProc names its own container, not the frontend.
- Add local TLS certificates for `aib.localhost`: rejected for the Compose demo because it requires a trusted certificate on every client host.
