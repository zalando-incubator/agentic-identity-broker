---
title: "Security posture"
description: Security controls for delegated credentials and vulnerability reporting. The broker uses encryption at rest, secret redaction, PKCE, single-use codes, consent anti-spoofing, SSRF protection, and a reverse-proxy trust boundary.
---

# Security posture

This page summarizes the security controls for the broker. The broker stores third-party
credentials for users. Its default settings protect those credentials. Security controls
are enabled by default and are not optional.

## Encryption at rest

Encryption at rest is **mandatory**. The broker refuses to start if no encryption backend is
configured — there is no plaintext fallback.

- **Envelope encryption.** A root key wraps intermediate keys. The intermediate keys wrap
  a new data key for each token. In production, an AWS KMS customer-managed key wraps
  DynamoDB-cached branch keys. Development uses a raw AES-256 key in memory.
- **Per-service context binding.** Each encryption layer binds authenticated data to one
  subject. Service tokens and secrets use `service_id`. A ciphertext for one service cannot
  be decrypted for another service.
- **Fail-closed.** If encryption or decryption does not complete correctly, the operation
  fails. The broker does not fall back to plaintext storage.

See [Encryption](/docs/concepts/encryption) for the full model and
[Configure encryption](/docs/guides/configure-encryption) for setup.

## Secret handling

- Provider client secrets are **encrypted before storage**. A secret value is plaintext or
  ciphertext. The type system prevents storage of a plaintext secret.
- Secrets are **always redacted in API responses.** A service response contains `REDACTED`
  instead of its client secret. The API never returns the secret.
- Broker-issued agent credentials are shown **once**, at creation or rotation. The broker
  stores a hash of each credential. The API can later return credential metadata, but not
  the secret.

## OAuth2 flow protections

- **PKCE is always required, S256 only.** The authorization endpoint requires
  `code_challenge_method` to be `S256`. It does not accept `plain`.
- **Authorization codes are single-use and short-lived.** A code is valid for one exchange
  and expires quickly.
- **`client_id` is the agent's internal identifier.** At the authorization endpoint,
  `client_id` is the agent system-generated UUID. It is not a reusable upstream OAuth2
  client ID.

## Consent anti-spoofing

The broker seals consent context in a short-lived JWE session token on the server. This
context identifies the agent and principal. It also contains the original authorization
URL and client metadata document. The broker does not pass these values as browser-visible
parameters. The consent interface cannot use attacker-provided values. The broker rejects
a session when its principal differs from the authenticated user.

## SSRF hardening

An agent identifies itself with an HTTPS URL that points to a Client ID Metadata Document
(CIMD). The broker fetches the document and shows its metadata in the consent interface.
Configurable SSRF controls prevent an agent-provided URL from accessing internal network
targets.

## Trust boundary and transport

- **Reverse-proxy pre-authentication.** The broker does not authenticate users. A trusted
  reverse proxy authenticates the caller and injects a principal into `X-Remote-User`.
  Deploy the broker so its ports are reachable only through that proxy.
- **Admin privilege at the proxy.** The proxy enforces administrator authorization before
  requests reach the admin server on port 14000.
- **HTTPS expectations.** Service issuer URIs use HTTPS. Tokens cross the network only over
  TLS. Terminate TLS in front of the broker. A development-only flag can skip third-party
  HTTPS validation. Do not enable it in production.

See [Configure authentication](/docs/guides/configure-authentication) for how to establish the
proxy trust boundary.

## Automated security verification

The `CI / security` status blocks pull requests. It runs the scanner script from the trusted base branch with read-only repository access.

- `gosec` finds security defects in Go source code.
- `govulncheck` finds known Go vulnerabilities that project code reaches.
- OSV-Scanner finds vulnerable dependencies in all Go manifests and npm lockfiles.

Run `just security` for the focused scanner command. Run `just verify` for the full verification gate.

## Reporting a vulnerability

Please disclose security issues responsibly rather than opening a public issue.

The project participates in **Zalando's responsible-disclosure and bug-bounty program**.
To report a vulnerability, submit the report through the Zalando vulnerability-reporting
form. You can join the bug-bounty program, but it is not required.

- **[Report a vulnerability](https://corporate.zalando.com/en/about-us/report-vulnerability)**

Code can contain security problems. The maintainers aim to issue patches as quickly as
possible. Allow time for a fix before public disclosure.
