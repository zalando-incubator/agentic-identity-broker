---
title: "Reference"
description: API contracts and token-exchange fields for the Agentic Identity Broker. This page explains the two API ports, their authentication, and configuration reference.
---

# Reference

This section documents API contracts for broker operation and integration. Use it after you
understand the [concepts](../concepts/index.md). It contains exact endpoints, fields, error codes,
and configuration keys.

## What's here

| Reference | What it covers |
|---|---|
| [End-user API](/api/enduser) | The full OpenAPI contract for the end-user server (port 8000): health, user info, consent, third-party sessions, and the OAuth2 server surface. |
| [Admin API](/api/admin) | The full OpenAPI contract for the admin server (port 14000): agents, services, permission sets, client credentials, and signing keys. |
| [API overview](./api.md) | A hand-authored companion to the two contracts — the auth model, response envelopes, error codes, and a compact endpoint map. |
| [Token exchange](./token-exchange.md) | The field-level reference for the RFC 8693 grant on `POST /oauth2/token`. |
| [Configuration](../configuration.md) | Every configuration section and key, and how the configuration sources compose. |

The source OpenAPI specifications generate the end-user and admin API pages. These pages are
the authoritative contracts. The [API overview](./api.md) describes conventions
that both contracts share.

## How the API is organized

The broker serves end-user and administrator traffic through two independent HTTP servers.
Each server has a separate port. You can route, firewall, and expose the ports independently:

- **End-user API — port 8000.** This port serves the consent interface (`/api/consent/*`),
  third-party sessions (`/api/third-party/*`), user information (`/api/me`), and OAuth2
  endpoints (`/oauth2/*`, `/.well-known/*`).
- **Admin API — port 14000.** This port manages agents, services, permission sets, client
  credentials, and signing keys.

## How authentication works

The broker does not authenticate users. A **trusted reverse proxy** authenticates each caller.
The proxy can be oauth2-proxy, nginx `auth_request`, or a service mesh. It sends the
principal in a request header. The default header is **`X-Remote-User`**. The broker accepts
the header only from a trusted source. The proxy enforces administrator privilege before
requests reach the admin API.

Some endpoints do not require pre-authentication:

- `GET /health` on both servers.
- `GET /oauth2/jwks.json` on the end-user server.
- `GET /.well-known/oauth-authorization-server` on the end-user server.

`GET /oauth2/authorize` and `POST /oauth2/token` use OAuth2 parameters for authentication.
They do not use the principal header. See
[Configure authentication](../guides/configure-authentication.md) for the proxy boundary.
