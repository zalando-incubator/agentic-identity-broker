# Quickstart: OAuth2 Provider Flavor Support

**Feature**: `20260312-120000-oauth2-provider-flavors`

## Overview

This feature extends `ThirdpartyOAuth2Service` with an `oauth2_flavor` field, enabling Google service account JSON credentials alongside the existing plain-text `client_secret`. The `oauth2_flavor` defaults to `standard`, preserving all existing behavior.

---

## Standard Flavor (No Change Required)

Existing services continue to work without modification. The `oauth2_flavor` field is optional and defaults to `standard`:

```bash
curl -X POST http://localhost:14000/api/services \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -d '{
    "display_name": "GitHub Production",
    "client_id": "Iv1.1234567890abcdef",
    "client_secret": "ghp_secretkey1234567890abcdef",
    "issuer_uri": "https://github.com",
    "discovery": { "enable_discovery": false },
    "endpoints": {
      "token_endpoint": "https://github.com/login/oauth/access_token",
      "authorize_endpoint": "https://github.com/login/oauth/authorize"
    },
    "scopes": [
      { "scope_value": "repo", "description": "Full control of private repositories" }
    ]
  }'
```

Response includes `"oauth2_flavor": "standard"`.

---

## Google Flavor: Configuring a Google Service Account

### Step 1: Get your service account JSON key

Download the service account key from Google Cloud Console. The key file looks like:

```json
{
  "type": "service_account",
  "project_id": "my-project-123",
  "private_key_id": "abc123",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "my-sa@my-project-123.iam.gserviceaccount.com",
  "client_id": "123456789012345678901",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}
```

### Step 2: Serialize the JSON as a single-line string

The `client_secret` field must be a JSON string (the service account JSON, serialized as a string value). Escape the newlines in `private_key`:

```bash
# Compact the JSON to a single line
SERVICE_ACCOUNT_JSON=$(cat service-account-key.json | jq -c .)
```

### Step 3: Create the service

```bash
curl -X POST http://localhost:14000/api/services \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -d "{
    \"display_name\": \"Google Workspace APIs\",
    \"oauth2_flavor\": \"google\",
    \"client_secret\": $(echo $SERVICE_ACCOUNT_JSON | jq -R .),
    \"discovery\": { \"enable_discovery\": false },
    \"scopes\": [
      { \"scope_value\": \"https://www.googleapis.com/auth/drive.readonly\", \"description\": \"Read Google Drive\" }
    ],
    \"protected_resources\": [\"https://www.googleapis.com\"]
  }"
```

**Note**: `client_id` and `issuer_uri` are optional for `google` flavor — `client_id` is extracted automatically from the service account JSON.

### Expected Response

```json
{
  "id": "770e8400-e29b-41d4-a716-446655440099",
  "display_name": "Google Workspace APIs",
  "oauth2_flavor": "google",
  "client_id": "123456789012345678901",
  "client_secret": "REDACTED",
  "issuer_uri": "https://oauth2.googleapis.com",
  "discovery": { "enable_discovery": false },
  "endpoints": {
    "token_endpoint": "https://oauth2.googleapis.com/token",
    "authorize_endpoint": "https://accounts.google.com/o/oauth2/auth"
  },
  "scopes": [
    { "scope_value": "https://www.googleapis.com/auth/drive.readonly", "description": "Read Google Drive" }
  ],
  "protected_resources": ["https://www.googleapis.com"],
  "created_at": "2026-03-12T10:00:00Z",
  "updated_at": "2026-03-12T10:00:00Z"
}
```

**Notice**: `client_id` is populated from the service account JSON, `issuer_uri` is derived from `token_uri`, endpoints are auto-populated, and `client_secret` is redacted.

---

## Validation Error Examples

### Invalid flavor value

```bash
curl -X POST http://localhost:14000/api/services -d '{"oauth2_flavor": "azure", ...}'
# HTTP 400
# {"error": "validation failed", "message": "oauth2_flavor must be one of: standard, google (got: azure)"}
```

### Google flavor with malformed JSON credential

```bash
curl -X POST http://localhost:14000/api/services \
  -d '{"oauth2_flavor": "google", "client_secret": "not-json", ...}'
# HTTP 400
# {"error": "validation failed", "message": "credential must be valid JSON for google flavor"}
```

### Google flavor with wrong type

```bash
curl -X POST http://localhost:14000/api/services \
  -d '{"oauth2_flavor": "google", "client_secret": "{\"type\": \"authorized_user\", ...}", ...}'
# HTTP 400
# {"error": "validation failed", "message": "credential type must be \"service_account\" (got \"authorized_user\")"}
```

### Google flavor with missing required field

```bash
# Missing private_key:
# HTTP 400
# {"error": "validation failed", "message": "private_key is required (must be a non-empty string)"}
```

### Google flavor with issuer_uri host mismatch

```bash
curl -X POST http://localhost:14000/api/services \
  -d '{"oauth2_flavor": "google", "issuer_uri": "https://login.microsoftonline.com", ...}'
# HTTP 400
# {"error": "validation failed", "message": "issuer_uri host (login.microsoftonline.com) must match token_uri host (oauth2.googleapis.com) in service account credential"}
```

---

## Updating Flavor

When updating a service's flavor, you must supply a valid credential for the new flavor in the same request:

```bash
# Update existing standard service to google flavor
curl -X PUT http://localhost:14000/api/services/{service-id} \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com" \
  -d "{
    \"display_name\": \"Google Workspace APIs\",
    \"oauth2_flavor\": \"google\",
    \"client_secret\": $(echo $SERVICE_ACCOUNT_JSON | jq -R .),
    \"discovery\": { \"enable_discovery\": false },
    \"scopes\": [...]
  }"
```

Switching back from `google` to `standard` requires providing a plain-text `client_secret` and `client_id`:

```bash
curl -X PUT http://localhost:14000/api/services/{service-id} \
  -d '{
    "display_name": "GitHub Production",
    "oauth2_flavor": "standard",
    "client_id": "Iv1.1234567890abcdef",
    "client_secret": "ghp_newsecret",
    "issuer_uri": "https://github.com",
    ...
  }'
```

---

## Key Points

| Field | Standard Flavor | Google Flavor |
|---|---|---|
| `oauth2_flavor` | `"standard"` (default) | `"google"` |
| `client_id` | Required in request | Extracted from JSON (optional in request) |
| `client_secret` | Plain secret string | Serialized service account JSON (≤32 KB) |
| `issuer_uri` | Required (HTTPS) | Optional; if provided, host must match `token_uri` |
| `endpoints` | Required when discovery disabled | Derived from service account JSON |
| Credential validation | Non-empty string | Full structural validation via `golang.org/x/oauth2/google` |

---

## Implementation Notes (Developer)

**Validation library**: `golang.org/x/oauth2/google.JWTConfigFromJSON` (already in `go.mod`) validates:
- JSON structure (returns error for invalid JSON)
- `type == "service_account"` (returns error otherwise)
- Extracts `client_email`, `private_key`, `token_uri`

**`client_id` extraction**: Auxiliary JSON unmarshal via a minimal struct — the oauth2 library doesn't expose `client_id` for service accounts directly.

**Crypto**: No cryptographic validation of the private key. The value is stored as raw bytes from the JSON and will be used when the Google JWT Bearer flow is implemented (follow-up feature).

**Existing tests**: All existing `standard` flavor tests continue to pass unchanged. Flavor defaults to `standard` when the field is absent.
