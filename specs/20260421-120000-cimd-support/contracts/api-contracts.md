# API Contracts: CIMD Support

**Feature**: 20260421-120000-cimd-support | **Date**: 2026-04-23

## Admin API Changes

### POST /api/agents (MODIFIED — API-004)

Extended `AgentRequest` to include `client_uris` (optional, defaults to `[]`).

```json
{
  "client_id": "my-agent",
  "display_name": "My Agent",
  "client_uris": ["https://agent.example.com/client"],
  "redirect_uris": ["https://agent.example.com/callback"],
  ...
}
```

**Validation**:
- Each URI in `client_uris` must be a valid `ClientIDMetadataDocumentURL` (HTTPS, no fragment, no credentials, no dot-segments, port 443 only, non-empty path)
- URIs must be globally unique — reject if any URI is already registered to a different agent

**Errors**:
- `400 Bad Request`: Invalid URL format in `client_uris`
- `409 Conflict`: Client URI already registered to another agent

### PUT /api/agents/{agent-id} (MODIFIED — API-004)

Extended `AgentRequest` to include `client_uris` (optional, defaults to `[]`).

```json
{
  "client_id": "my-agent",
  "display_name": "My Agent",
  "client_uris": ["https://agent.example.com/client"],
  "redirect_uris": ["https://agent.example.com/callback"],
  ...
}
```

### GET /api/agents/{agent-id} (MODIFIED)

Extended `AgentResponse` to include `client_uris`.

```json
{
  "id": "uuid",
  "client_id": "my-agent",
  "display_name": "My Agent",
  "client_uris": ["https://agent.example.com/client"],
  "redirect_uris": ["https://agent.example.com/callback"],
  ...
}
```

## End-User API Changes

### GET /.well-known/oauth-authorization-server (MODIFIED — API-001)

New field when CIMD enabled:
```json
{
  "issuer": "https://broker.example.com",
  "authorization_endpoint": "https://broker.example.com/oauth2/authorize",
  ...
  "client_id_metadata_document_supported": true
}
```

Field absent when CIMD disabled.

### GET /oauth2/authorize (UNCHANGED — API-002)

Transparently accepts `client_id=https://agent.example.com/client` (URL format). No schema change to the authorization endpoint itself. CIMD resolution is internal.

**New error responses** (no redirect — displayed as error page):
- `invalid_client` / `"URL-format client_id is not supported"` — URL-format client_id when CIMD is disabled
- `invalid_client` / `"CIMD document fetch failed"` — network error, non-200, timeout, oversized
- `invalid_client` / `"CIMD document client_id mismatch"` — document `client_id` ≠ request URL
- `invalid_client` / `"Client ID Metadata Document URL not registered"` — URL not pre-registered on any agent
- `invalid_client` / `"CIMD document uses forbidden authentication method"` — client-secret-based auth method
- `invalid_client` / `"CIMD document client_name is blocked"` — keyword blocklist match
- `invalid_client` / `"SSRF protection: blocked address"` — URL resolves to blocked IP range
- `invalid_request` / `"Invalid client_id URL format"` — malformed URL (scheme, path, fragment, etc.)
- `invalid_request` / `"redirect_uri not in CIMD document"` — redirect URI validation failure

### GET /api/consent/session (NEW — FR-028)

Decode endpoint for CIMD consent sessions. Called by the consent page with the opaque `session_token` received via the consent redirect URL.

**Request**: `GET /api/consent/session?token=<jwe>`

**Response** (200 OK):
```json
{
  "agent_id": "uuid",
  "client_name": "My Cool Agent",
  "client_id_url": "https://agent.example.com/client",
  "redirect_uri": "https://agent.example.com/callback",
  "verified_domain": "agent.example.com",
  "is_localhost_redirect": false,
  "requested_scopes": ["repo", "user:email"],
  "logo_uri": "https://agent.example.com/logo.png",
  "scope": "repo user:email",
  "expires_at": "2026-05-05T12:10:00Z"
}
```

**Errors**:
- `400 Bad Request`: Token is missing, malformed, or expired

### GET /api/consent/agent/{agent-id} (MODIFIED)

When the consent session originates from a CIMD-based authorization request, the response includes additional `cimd_metadata`. The consent page obtains this data by first calling the decode endpoint above.

```json
{
  "data": {
    "agent": { "...existing fields..." },
    "services": ["..."],
    "cimd_metadata": {
      "client_name": "My Cool Agent",
      "client_id_url": "https://agent.example.com/client",
      "redirect_uri": "https://agent.example.com/callback",
      "verified_domain": "agent.example.com",
      "is_localhost_redirect": false,
      "requested_scopes": ["repo", "user:email"],
      "logo_uri": "https://agent.example.com/logo.png"
    }
  }
}
```

`cimd_metadata` is `null` or absent for non-CIMD authorization requests.

## OpenAPI Specification Updates

### `/api/admin/openapi.yaml`

- Extend `AgentRequest` schema with `client_uris` (array of strings, optional, defaults to `[]`)
- Extend `AgentResponse` schema with `client_uris`

### `/api/enduser/openapi.yaml`

- Extend `MetadataResponse` schema with `client_id_metadata_document_supported` (boolean, optional)
- Add `GET /api/consent/session` endpoint schema: query param `token` (string, required), response body with CIMD display fields
- Extend consent agent detail response with `cimd_metadata` object schema
- Document new error responses for `/oauth2/authorize`
