# ADR 044: Client Registration Independent of Authentication

**Status**: Accepted (stakeholder approval, 2026-10-22, feature 046 Increment 2)
**Date**: 2026-10-22
**Feature**: 046-cimd-upstream-client, Increment 2 (issue #264)
**Supersedes**: None

## Context

Feature 046 pairs a broker-hosted Client ID Metadata Document (CIMD) with `private_key_jwt`. Some authorization servers accept CIMD but cannot verify client assertions. ServiceNow requires `none`, and the MCP authorization specification shows a CIMD example with `none`.

Issue #264 approves an independent service setting, `client_registration`. It selects a static operator identity or a broker-hosted client identifier URL. `token_endpoint_auth_method` separately selects shared-secret, no-authentication, or signed-assertion behavior. CIMD draft-02 §4.1 forbids shared-secret methods for CIMD. Section 8.2 recommends authentication when available. Thus `private_key_jwt` remains recommended, and CIMD public is an explicit per-service choice.

Issue #264 approves the field, its four-mode matrix, the omission rule, read and list reporting, and classification of existing services. On 2026-10-22, the stakeholder explicitly accepted this ADR, including D-E–D-J and the D-I rolling-upgrade limitation. The stakeholder also confirmed D-A–D-D in writing under API-014 before implementation. Plan approval alone did not grant either approval.

## Decision

### 1. D-A — Reject explicit invalid registration

`client_registration` is optional on create and full-replacement `PUT`, but it is not nullable. Explicit `null`, an empty string, and unknown values return `400` with the existing `{"error":"validation failed","message":...}` representation. This follows the `credential_source` precedent in ADR 041.

### 2. D-B — Preserve discovery ownership

A protected-resource discovery request (`discovery.resource_url` present) must omit `client_registration`. A supplied field returns `400` before discovery traffic. Discovery writes `cimd` when it selects hosted CIMD, which always uses `private_key_jwt`. It writes `static` when it selects DCR with `client_secret_basic`, `client_secret_post`, or `none`. `discovery.client_method` remains the authoritative discovery axis. ADR 040 selection stays unchanged and never selects CIMD public. The stakeholder confirmed the `static` report for DCR as part of D-B.

### 3. D-C — Serve keyless CIMD public metadata

A CIMD public service keeps the existing client identifier URL, `<public_url>/.well-known/oauth-client/<service-id>`. Its document returns `200`, `application/json`, and `Cache-Control: public, max-age=300`. It contains exactly these members:

- `client_id`: The client identifier URL.
- `redirect_uris`: One callback, `<public_url>/api/third-party/<service-id>/oauth2/callback`.
- `grant_types`: `authorization_code` and `refresh_token`.
- `response_types`: `code`.
- `token_endpoint_auth_method`: `none`.

The document omits `jwks_uri` and `token_endpoint_auth_signing_alg`. It remains available without a CIMD client-authentication key. `GET <client-id>/jwks.json` returns the uniform feature 046 JSON `404`.

### 4. D-D — Version the API descriptions

Admin OpenAPI `info.version` increases from `2.0.0` to `2.1.0`. End-user OpenAPI `info.version` increases from `1.0.0` to `1.1.0`. Both changes describe additive representations. The stakeholder confirmed these versions on 2026-10-22 under Principle X.

### 5. D-E — Keep Google and filesystem identities static

Reject `client_registration: cimd` with `400` for `oauth2_flavor: google` or `credential_source: filesystem`. Explicit `static` remains valid for both. A CIMD public service has no client credential, so filesystem credentials cannot apply.

### 6. D-F — Check hosted identity and limit key bootstrap

At startup, every persisted `cimd` service must satisfy `client_id == <server.enduser.public_url>/.well-known/oauth-client/<service-id>`. A mismatch stops startup. Run `EnsureInitialKey` and `RequireUsablePublishedKey` only if at least one CIMD confidential service exists. CIMD public services never create, require, or block on a CIMD client-authentication key. This extends feature 046 identity checks without changing ADR 037 key lifecycle.

PostgreSQL reads check the stored registration and authentication state. An invalid state returns an error with the service ID. Reads do not repeat write-time validation of `discovery.resource_url`. An older URL that becomes blocked must not stop service lists or startup. Writes still validate the URL, and the guarded outbound client rejects unsafe destinations.

### 7. D-G — Make full-replacement transitions explicit

The valid manual service combinations are:

| `client_registration` | `token_endpoint_auth_method` | Mode |
|---|---|---|
| `static` | Absent or `null` | Static confidential |
| `static` | `none` | Static public |
| `cimd` | `private_key_jwt` | CIMD confidential |
| `cimd` | `none` | CIMD public |

Reject `static` with `private_key_jwt` and `cimd` with an absent or `null` method. Return `400` before discovery or provider traffic and preserve the old service. On create or `PUT`, omitted registration resolves to `cimd` only for `private_key_jwt` and to `static` otherwise. The broker never infers CIMD public. A `PUT` for a CIMD public service must repeat `client_registration: cimd`. Read and list responses always report the resolved value. Classify stored `private_key_jwt` services as `cimd` and all others as `static`, without changing identity, sessions, or authentication.

A CIMD confidential ↔ CIMD public switch keeps the client identifier URL. Switching to CIMD confidential requires a usable CIMD key. A `static` → `cimd` switch replaces the operator client ID with the broker URL. Remove a stored secret only after the full replacement validates. A `cimd` → `static` switch requires an operator `client_id`. Static confidential also requires a non-empty `client_secret`. The former hosted document then returns `404`.

The broker never falls back between `private_key_jwt` and `none` at runtime. Existing session behavior remains unchanged. Established-identity checks apply only to shared-secret services. A provider can reject old refresh tokens after a client-ID switch, and the user reconnects. Provider caches can retain the previous document for up to 300 seconds.

### 8. D-H — Keep audit records credential-free

Keep the existing `CIMD confidential service` lifecycle records. Create, update, and rejection records for CIMD public use `CIMD public service`. These records contain `service_id`, `operation`, and `outcome`. They contain no credentials, tokens, assertions, or codes. The existing mode-neutral `CIMD metadata access` record covers public document serving. Do not emit a second record for that request. Token acquisition uses existing public-client session audit records with `public_client: true`.

### 9. D-I — Accept fail-closed rolling upgrades

Migration 044 adds a `static` column default before new replicas roll out. A pre-upgrade replica cannot create a `private_key_jwt` service, because the default conflicts with the registration constraint. It also cannot rewrite a `cimd` service to an operator client ID. PostgreSQL rejects both writes. Reads and other writes continue. Retry these writes after all replicas upgrade. Do not add a trigger that silently derives the new field.

### 10. D-J — Record the independent setting without supersession

This Accepted ADR supersedes no ADR. ADR 036 public wire behavior applies to both public modes. ADR 037 key-domain separation remains unchanged. ADR 040 discovery selection stays unchanged. ADR 041 filesystem limits remain unchanged.

## Persistence

Persist `thirdparty_oauth2_services.client_registration TEXT NOT NULL DEFAULT 'static'`. Add `chk_thirdparty_oauth2_services_client_registration` separately from `chk_thirdparty_oauth2_services_client_auth`. Its permitted values are `static` and `cimd`:

- `static` excludes `private_key_jwt` and `discovery.client_method = cimd`.
- `cimd` requires `private_key_jwt` or `none`, no encrypted client secret, and `credential_source = stored`.
- `cimd` excludes `discovery.client_method = dcr` and requires a non-null client ID ending with `/.well-known/oauth-client/` plus the service ID.

The database suffix rule is a coarse guard. PostgreSQL cannot read `server.enduser.public_url`. Domain validation and the startup check enforce the exact client identifier URL. The existing client-authentication constraint remains active.

Migration `044_add_thirdparty_client_registration` uses a one-second statement timeout and `ACCESS EXCLUSIVE NOWAIT`. Its up migration adds the defaulted column and the constraint as `NOT VALID`, avoiding a table scan under that lock. Its down migration refuses rollback while a `cimd` plus `none` row exists. It then drops the constraint and column.

Migration `045_validate_thirdparty_client_registration` uses `SHARE UPDATE EXCLUSIVE NOWAIT`. Its up migration backfills `cimd` for every `private_key_jwt` row and validates the constraint. A legacy CIMD row with a nonconforming client ID stops validation. Its down migration restores `NOT VALID` and keeps the values.

### Guarded rollback runbook

If migration 044 refuses rollback, convert or delete every CIMD public service. Run `migrate force 44` to clear the dirty migration state. Then retry the down migration. Never reinterpret a broker-hosted CIMD public identifier as a static public client ID.

## Consequences

CIMD public sends no token-endpoint client authentication. Authorization uses PKCE S256 and the one redirect URI in the hosted document. The authorization server remains responsible for refresh-token protection, including rotation. The broker retains the existing public-client code-exchange and refresh behavior: the URL appears as `client_id`, and no secret, assertion, or authorization header is sent. Provider rejection fails closed without an authentication-method fallback. This explicit opt-in trades confidential client authentication for compatibility with providers such as ServiceNow. `private_key_jwt` remains recommended under CIMD draft-02 §8.2.

A CIMD-public-only broker can operate in `proxy`, `local`, or `hybrid` mode with an empty CIMD key set. Proxy mode uses the hosted service identity, not proxy upstream credentials. No configuration key, Helm value, frontend, ExtProc, or CIMD key-lifecycle change is introduced. The migration and all service adapters must preserve the four-mode invariant. Operators must account for the 300-second document cache and the rolling-upgrade write limitation.

## Alternatives Considered

- **Add another authentication-method value:** Rejected. Registration and token-endpoint authentication are independent decisions.
- **Infer CIMD from `none` and an absent `client_id`:** Rejected. The broker must never infer CIMD public or change static public behavior.
- **Derive the column with a database trigger:** Rejected. Hidden writes obscure the API contract and migration behavior.
- **Use one combined migration:** Rejected. Backfill and validation under an exclusive schema lock increase disruption.

## References

- [Feature 046 specification](../specs/046-cimd-upstream-client/spec.md)
- [Feature 046 Increment 2 research](../specs/046-cimd-upstream-client/research.md#increment-2-issue-264-cimd-public-clients)
- [ADR 036: Public Client Token-Endpoint Authentication](036-public-client-token-endpoint-auth.md)
- [ADR 037: CIMD Client-Authentication Key Domain](037-cimd-client-authentication-key-domain.md)
- [ADR 040: Protected Resource Discovery and Dynamic Client Registration](040-protected-resource-discovery-and-dcr.md)
- [ADR 041: OAuth2 Client-Secret File Overrides](041-oauth2-client-secret-file-overrides.md)
- [Constitution](../.specify/memory/constitution.md), Principles II and X
