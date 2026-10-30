# Data Model: Unified Session Token State Transport

**Feature**: 20260517-120000-unified-session-token
**Date**: 2026-05-18

## Entities

### AuthorizationSessionClaims (existing — no changes)

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | `id.AgentID` | Agent that initiated the authorization request |
| `principal` | `id.Principal` | Authenticated user the token is bound to |
| `original_url` | `string` | Full authorize request URL (contains redirect_uri, scope, etc.) |
| `cimd_metadata` | `*cimd.ClientIDMetadataDocument` | CIMD metadata (nil for local/proxy agents) |
| `iat` | `time.Time` | Token issuance timestamp |
| `exp` | `time.Time` | Token expiry (iat + 10 minutes) |

### State Transitions

```
[Authorize Request] → buildConsentURL() → [JWE session_token in redirect]
                                                    ↓
[Consent Page Load] → decrypt + validate → [Render consent with agent context]
                                                    ↓
[Grant Submission] → decrypt + validate → [Process grant, redirect to original_url callback]
```

## Validation Rules

1. **Token expiry**: `time.Now().After(claims.ExpiresAt)` → reject with 400
2. **Principal binding**: `claims.Principal != authenticatedPrincipal` → reject with 403
3. **Agent ID binding**: `claims.AgentID != urlPathAgentID` → reject with 400
4. **JWE integrity**: Decryption failure (tampered/malformed) → reject with 400

## Relationships

- `AuthorizationSessionClaims.AgentID` references `storage.Agent.ID`
- `AuthorizationSessionClaims.Principal` matches `X-Remote-User` header value
- No database relationships — entirely stateless
