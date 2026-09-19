---
title: Glossary
description: Precise definitions of the terms used across the Agentic Identity Broker documentation.
---

# Glossary

This page defines the terms used in these documents. The terms are grouped by area. See
[delegation and consent](./delegation-and-consent.md) to see how they work together.

## Delegation model

**Principal** — The authenticated user. A trusted reverse proxy authenticates the request and
sends the principal identifier in a header. The default header is `X-Remote-User`.

**Agent** — An AI agent registered in the broker. An agent has a system-generated UUID for
its OAuth2 `client_id`. It has a display name, description, and optional governance and
documentation URLs. It declares mandatory or optional services and permission sets.

**Third-party service** (third-party OAuth2 provider) — An external OAuth2 provider, such as
GitHub or Google. A service has a provider client ID, encrypted client secret, issuer,
endpoints, scopes, and optional token-exchange resource URIs.

**Scope** — A permission string from a service, such as `repo` or `user:email`. Each scope
has a human-readable description.

**Permission set** — An administrator-defined group of scopes for one or more services. It
is the unit that users consent to. It uses business terms instead of raw provider scopes.

**Requirement type** — Whether an agent needs a service or permission set. **Mandatory**
requirements stop authorization until satisfied. **Optional** requirements let the agent
continue without that access.

**Grant** (user grant) — The record of a principal giving permission sets to an agent. A user
has at most one active grant for each agent. A grant can expire through `valid_until`. A user
can revoke it at any time.

**User session** — An authenticated OAuth2 session for one principal and one third-party
service. It contains encrypted access and refresh tokens, expiry, and scopes. A user has one
session for each service.

**Consent flow** — The process where a user reviews, grants, adjusts, or revokes an agent
request in the consent interface.

## OAuth2 and token exchange

**Authorization server** — The broker OAuth2 surface for RFC 6749 authorization code and RFC
8414 metadata. It can proxy an upstream server or issue its own tokens. The selected
[server mode](./oauth2-server-modes.md) determines its behavior.

**Server mode** — `proxy` forwards to an upstream authorization server. `local` issues tokens
in the broker. `hybrid` uses both modes based on agent registration.

**PKCE** — Proof Key for Code Exchange (RFC 7636). The broker always requires the `S256`
challenge method for authorization flows.

**Token exchange** — RFC 8693. A privileged gateway exchanges an agent subject token for the
appropriate third-party token. The broker first validates the user grant.

**Subject token** — The JWT that identifies a user and agent in token exchange. The default
user claim is `sub`. The default agent claim is `azp`.

**Client assertion** — The JWT that a privileged gateway uses to authenticate to the broker.
The broker validates it against upstream authorization server keys.

**Resource / protected resource** — A URI that identifies a target third-party service for
token exchange. The broker matches it to service `protected_resources`.

**Broker client credential** — OAuth2 client credentials that the broker issues for an agent
in `local` or `hybrid` mode. The broker stores the secret hash, shows the secret once, and
can rotate the credential.

**Signing key** — An asymmetric ES256 key that the broker uses to sign local JWT access
tokens. The broker encrypts private material at rest and publishes public keys through its
JWKS.

**JWKS** — JSON Web Key Set (RFC 7517). The broker publishes public keys at
`/oauth2/jwks.json`. Token validators use these keys.

**CIMD** — Client ID Metadata Document. An agent can identify itself with an HTTPS URL that
points to this JSON document. The broker fetches and validates the document with SSRF
protection. The consent interface shows the document metadata.

**CEL** — Common Expression Language. The broker uses it to authorize token exchange, get
identifiers from tokens, and add claims to local tokens.

## Encryption

**Envelope encryption** — The broker encrypts data with a new data key. It then encrypts that
data key with a higher-level key. One key-management request can protect many tokens. See
[encryption at rest](./encryption.md).

**KEK / DEK** — Key Encryption Key (KEK) is the production root AWS KMS customer-managed key.
Data Encryption Key (DEK) encrypts one token for an operation.

**Branch key** — An intermediate key between the KEK and DEK. DynamoDB caches this key in the
hierarchical keyring. This cache reduces KMS calls.

**Encryption context** — Non-secret data bound to ciphertext as additional authenticated data.
The context identifies one service. A token for one service cannot be decrypted for another
service.

**Token vault** — Encrypted third-party OAuth2 tokens stored in user sessions.

## Deployment and operations

**Reverse-proxy pre-authentication** — A trusted proxy authenticates the user and adds the
principal header. This is the broker baseline authentication mode.

**JWT pre-authentication** — An optional mode where the broker validates a signed JWT from a
header. It uses a JWKS for signed tokens. It accepts unsigned claims only in a trusted
environment. It uses CEL to get a user profile.

**ExtProc token-exchange sidecar** — A standalone gRPC service that implements the Envoy
External Processor protocol. It performs token exchange and optional OPA policy checks at an
Envoy-based agent gateway.

**OPA** — Open Policy Agent. The ExtProc sidecar uses it to authorize proxied requests and
MCP tool calls. It is independent of the broker token-exchange policy.

**IRSA** — IAM Roles for Service Accounts. On AWS, this lets broker pods use the KMS key and
DynamoDB table without static credentials.

**Migration job** — A one-shot container image that applies database schema changes with a
least-privilege database user. It runs separately from the broker service.
