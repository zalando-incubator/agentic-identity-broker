# OAuth2 Authorization Server Proxy API Contracts

**Feature**: 20251222-120100-oauth2-auth-server
**OpenAPI Version**: 3.0.3
**Spec Location**: [openapi.yaml](./openapi.yaml)

## Overview

This directory contains the OpenAPI 3.0 specification for the OAuth2 Authorization Server proxy endpoints. The identity broker acts as an RFC 6749/8414 compliant OAuth2 proxy that:

1. Validates client_id against registered agents
2. Checks user consent via existing grant system
3. Proxies authorized requests to upstream OAuth2 server

**Key Principle**: The broker does NOT issue, validate, or store tokens. All token operations are handled by the configured upstream OAuth2 server.

---

## Endpoints

### 1. GET /oauth2/authorize
**Purpose**: OAuth2 authorization endpoint (RFC 6749 Section 4.1.1)

**Behavior**:
- Validates `client_id` parameter against Agent registry
- Checks if active grant exists for (principal, agent) pair
- **No grant**: HTTP 302 redirect to consent UI
- **Active grant**: HTTP 302 redirect to upstream OAuth2 server

**Parameters** (all in query string):
- `response_type` (required): Must be "code"
- `client_id` (required): Must match registered Agent.ClientID
- `redirect_uri` (required): Client's callback URL
- `scope` (optional): Requested OAuth2 scopes
- `state` (recommended): CSRF protection value
- `code_challenge`, `code_challenge_method` (optional): PKCE parameters

**Responses**:
- `302 Found`: Redirect to consent UI or upstream OAuth2 server
- `400 Bad Request`: Invalid request (missing required parameters)
- `500 Internal Server Error`: Upstream unreachable or TLS validation failed

**Example Flow**:
```
1. Client → GET /oauth2/authorize?client_id=my-agent&redirect_uri=...&state=xyz

2. Broker validates client_id → finds Agent

3. Broker checks consent → No grant found

4. Broker → 302 Location: /consent/agent/agent-123?redirect_uri=<full original URL>

5. User completes consent → Consent UI redirects back to step 1

6. Broker checks consent → Active grant found

7. Broker → 302 Location: https://upstream-oauth2.example.com/oauth2/authorize?client_id=...

8. Upstream processes authorization → 302 to client's redirect_uri with code
```

---

### 2. POST /oauth2/token
**Purpose**: OAuth2 token endpoint (RFC 6749 Section 4.1.3)

**Behavior**:
- Validates Content-Type (application/x-www-form-urlencoded or application/json)
- Creates server-to-server HTTP POST to upstream token endpoint
- Copies all parameters and headers (except hop-by-hop headers)
- Streams response back to client without modification

**Request Body** (form-urlencoded or JSON):

**Authorization Code Grant**:
- `grant_type=authorization_code` (required)
- `code=<authorization_code>` (required)
- `redirect_uri=<callback_url>` (required, must match authorization request)
- `client_id=<client_identifier>` (required)
- `client_secret=<client_secret>` (required)

**Refresh Token Grant**:
- `grant_type=refresh_token` (required)
- `refresh_token=<refresh_token>` (required)
- `client_id=<client_identifier>` (required)
- `client_secret=<client_secret>` (required)
- `scope=<requested_scope>` (optional)

**Responses**:
- `200 OK`: Token response (proxied from upstream)
  ```json
  {
    "access_token": "...",
    "token_type": "Bearer",
    "expires_in": 3600,
    "refresh_token": "...",
    "scope": "read write"
  }
  ```
- `400 Bad Request`: Invalid request (proxied from upstream)
- `401 Unauthorized`: Client authentication failed (proxied from upstream)
- `415 Unsupported Media Type`: Invalid Content-Type (broker-generated)
- `502 Bad Gateway`: Upstream unreachable (broker-generated)

**Security Notes**:
- The broker does NOT log token values (access_token, refresh_token, authorization codes)
- Client authentication handled entirely by upstream OAuth2 server
- Content-Type validation provides CSRF protection per SR-008

---

### 3. GET /.well-known/oauth-authorization-server
**Purpose**: OAuth2 metadata discovery (RFC 8414)

**Behavior**:
- Returns JSON document describing broker's OAuth2 configuration
- Enables OAuth2 clients to auto-discover endpoints and capabilities
- Metadata reflects broker's configuration, not upstream server

**Response** (200 OK):
```json
{
  "issuer": "https://identity-broker.example.com",
  "authorization_endpoint": "https://identity-broker.example.com/oauth2/authorize",
  "token_endpoint": "https://identity-broker.example.com/oauth2/token",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "token_endpoint_auth_methods_supported": ["client_secret_post", "client_secret_basic"]
}
```

**Usage**:
OAuth2 clients can fetch this endpoint to automatically configure their OAuth2 library:
```python
# Python example with authlib
from authlib.integrations.requests_client import OAuth2Session

metadata_url = "https://identity-broker.example.com/.well-known/oauth-authorization-server"
client = OAuth2Session(
    client_id="my-agent",
    client_secret="SECRET",
    server_metadata_url=metadata_url
)
```

---

## OAuth2 Error Codes

The broker generates or proxies the following RFC 6749 error codes:

### Broker-Generated Errors

| Error Code | When Used | HTTP Status |
|-----------|-----------|-------------|
| `invalid_client` | client_id doesn't match registered Agent | 302 (in redirect), 400 (in JSON) |
| `server_error` | Upstream unreachable, TLS validation failed | 500 |
| `temporarily_unavailable` | Upstream OAuth2 server temporarily down | 502 |
| `invalid_request` | Missing required parameter, invalid Content-Type | 400, 415 |

### Proxied from Upstream

| Error Code | Meaning (from upstream) | HTTP Status |
|-----------|------------------------|-------------|
| `invalid_grant` | Invalid/expired authorization code, mismatched redirect_uri | 400 |
| `invalid_request` | Malformed request | 400 |
| `unauthorized_client` | Client not authorized for grant type | 401 |
| `unsupported_grant_type` | Grant type not supported | 400 |
| `invalid_scope` | Invalid or unknown scope | 400 |

---

## Integration with `/api/enduser/openapi.yaml`

**Action Required**: These endpoint definitions must be merged into `/api/enduser/openapi.yaml` during implementation.

**Merge Strategy**:
1. Copy path definitions (`/oauth2/authorize`, `/oauth2/token`, `/.well-known/oauth-authorization-server`)
2. Copy component schemas (OAuth2Error, TokenResponse, OAuth2Metadata, etc.)
3. Add `OAuth2` tag to tags section
4. Ensure no conflicts with existing paths

**Note**: The broker's enduser API will contain both consent management endpoints (from feature 20251217-120100) and OAuth2 endpoints (this feature).

---

## Testing the API

### Authorization Endpoint Test
```bash
# Simulate OAuth2 client initiating authorization
curl -v "https://identity-broker.example.com/oauth2/authorize?response_type=code&client_id=my-agent&redirect_uri=https://my-agent.example.com/callback&state=xyz123"

# Expected: HTTP 302 redirect to either:
# - /consent/agent/:agent-id (if no consent)
# - upstream OAuth2 server (if consent exists)
```

### Token Endpoint Test
```bash
# Exchange authorization code for access token
curl -X POST https://identity-broker.example.com/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code" \
  -d "code=AUTHORIZATION_CODE" \
  -d "redirect_uri=https://my-agent.example.com/callback" \
  -d "client_id=my-agent" \
  -d "client_secret=SECRET"

# Expected: JSON token response from upstream (proxied)
```

### Metadata Endpoint Test
```bash
# Fetch OAuth2 metadata
curl https://identity-broker.example.com/.well-known/oauth-authorization-server

# Expected: JSON document with issuer, endpoints, supported types
```

---

## Security Considerations

### Authorization Endpoint
- **CSRF Protection**: `state` parameter recommended (enforced by upstream)
- **Consent Enforcement**: Active grant required before proxying to upstream
- **URL Validation**: redirect_uri validated by upstream OAuth2 server
- **Agent Validation**: client_id must match registered Agent.ClientID

### Token Endpoint
- **TLS Required**: All upstream communication uses HTTPS with certificate validation
- **Content-Type Validation**: Rejects requests without proper Content-Type (CSRF protection)
- **No Token Logging**: Broker never logs access_token, refresh_token, or authorization codes
- **Credential Protection**: Client secrets forwarded to upstream, never logged or stored by broker

### Metadata Endpoint
- **Public Information**: Metadata endpoint is publicly accessible (no authentication required)
- **No Secrets**: Metadata contains only public configuration (endpoints, supported types)

---

## References

- [RFC 6749: OAuth 2.0 Authorization Framework](https://datatracker.ietf.org/doc/html/rfc6749)
- [RFC 8414: OAuth 2.0 Authorization Server Metadata](https://datatracker.ietf.org/doc/html/rfc8414)
- [OpenAPI 3.0 Specification](https://spec.openapis.org/oas/v3.0.3)
- [Zalando RESTful API Guidelines](https://opensource.zalando.com/restful-api-guidelines/)
