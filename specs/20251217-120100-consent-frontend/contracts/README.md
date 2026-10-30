# API Contracts: Consent Management

This directory contains API contract specifications for the consent management frontend feature.

## Files

- **openapi.yaml**: Complete OpenAPI 3.0.3 specification for all consent management endpoints

## Viewing the API Documentation

### Option 1: Swagger UI (Online)
Visit [Swagger Editor](https://editor.swagger.io/) and paste the contents of `openapi.yaml`.

### Option 2: Swagger UI (Local - Recommended)
```bash
# Install swagger-ui-watcher
npm install -g swagger-ui-watcher

# Serve the API docs
cd specs/20251217-120100-consent-frontend/contracts
swagger-ui-watcher openapi.yaml
# Opens http://localhost:8080 with interactive docs
```

### Option 3: Redoc (Local)
```bash
# Install redoc-cli
npm install -g redoc-cli

# Generate static HTML
cd specs/20251217-120100-consent-frontend/contracts
redoc-cli bundle openapi.yaml -o api-docs.html

# Open api-docs.html in browser
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/me` | Get current user information |
| GET | `/api/consent/agents` | List agents with active grants |
| GET | `/api/consent/agent/:agentId` | Get agent detail and available services |
| GET | `/api/consent/agent/:agentId/grants` | Get user's grants for an agent |
| POST | `/api/consent/agent/:agentId/grants` | Create or update grant for an agent |

## Authentication

All endpoints require session-based authentication via the `session_token` cookie. The cookie is set by the existing authentication system and contains the user's principal.

## Request/Response Examples

See the OpenAPI specification for comprehensive examples, including:
- User with/without profile picture
- Multiple delegations vs. no delegations
- Agent detail with available services
- Creating indefinite grants
- Creating expiring grants
- Revoking grants (empty delegatedTokens array)
- Error responses with field-level validation details

## Validation Rules

### Grant Creation/Update (`POST /api/consent/agent/:agentId/grants`)

**Request Body Validation:**
- `delegatedTokens` (required): Array of service delegations
  - Can be empty array to revoke grant
  - Each element must have:
    - `serviceId` (required, UUID): Service identifier
    - `scopes` (required, min 1 item): Array of OAuth2 scope values
- `validUntil` (optional, future date): Grant expiration timestamp
  - Omit or set to `null` for indefinite grants
  - If provided, must be in the future

**Backend Validation:**
- All `serviceId` values must exist in configured services
- All `scopes` values must exist for the specified service
- User principal extracted from session (not in request body)
- One grant per user-agent pair (upsert semantics)

## Error Handling

All error responses follow this structure:
```json
{
  "status": 400,
  "code": "VALIDATION_ERROR",
  "message": "Request validation failed",
  "details": {
    "fieldName": ["Error message 1", "Error message 2"]
  }
}
```

**Common Error Codes:**
- `UNAUTHORIZED` (401): Authentication required or session expired
- `AGENT_NOT_FOUND` (404): Agent with specified ID does not exist
- `VALIDATION_ERROR` (400): Request validation failed (see details)
- `INVALID_SCOPES` (400): One or more scopes don't exist for service
- `INTERNAL_ERROR` (500): Unexpected server error

## Testing with curl

### Get User Info
```bash
curl -X GET http://localhost:8080/api/me \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Accept: application/json"
```

### List Agent Delegations
```bash
curl -X GET http://localhost:8080/api/consent/agents \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Accept: application/json"
```

### Get Agent Detail
```bash
curl -X GET http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000 \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Accept: application/json"
```

### Create Grant (Indefinite)
```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "delegatedTokens": [
      {
        "serviceId": "google-drive-service-id",
        "scopes": ["read:files", "write:files"]
      }
    ]
  }'
```

### Create Grant (With Expiration)
```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "delegatedTokens": [
      {
        "serviceId": "github-service-id",
        "scopes": ["read:repos"]
      }
    ],
    "validUntil": "2026-06-01T00:00:00Z"
  }'
```

### Revoke Grant
```bash
curl -X POST http://localhost:8080/api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants \
  -H "Cookie: session_token=YOUR_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "delegatedTokens": []
  }'
```

## Implementation Notes

1. **Session Handling**: Backend extracts principal from session context, not from request body
2. **Upsert Semantics**: POST creates new grant or updates existing (one grant per user-agent pair)
3. **Grant Revocation**: Submit empty `delegatedTokens` array to revoke grant
4. **Expiration Handling**: Backend filters expired grants from listings automatically
5. **CORS**: Frontend SPA requires CORS middleware for `/api/consent/*` routes
6. **Rate Limiting**: Consider implementing rate limiting on POST endpoint

## Next Steps

- Implement backend handlers matching this contract
- Generate TypeScript types from OpenAPI spec (optional, or use data-model.md types)
- Add integration tests validating contract compliance
- Document any deviations from contract in ADRs
