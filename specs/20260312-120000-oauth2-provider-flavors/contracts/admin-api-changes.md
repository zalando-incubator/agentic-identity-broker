# Admin API Contract Changes: OAuth2 Provider Flavor Support

**Feature**: `20260312-120000-oauth2-provider-flavors`
**File to modify**: `api/admin/openapi.yaml`

## Summary of Changes

Three schemas in `api/admin/openapi.yaml` require changes:
1. `Service` (response) — add `oauth2_flavor` to properties and required list
2. `ServiceCreateRequest` — add `oauth2_flavor` as optional field; relax `client_id` and `issuer_uri` required constraints for google flavor
3. `ServiceUpdateRequest` — same as Create

---

## Schema: `Service` (response object)

### Add to `required` list
Add `oauth2_flavor` after `updated_at`.

### Add to `properties` (after `client_secret`)

```yaml
oauth2_flavor:
  type: string
  enum:
    - standard
    - google
  default: standard
  description: |
    OAuth2 authentication variant for this service.

    - **standard**: Plain client secret string (default behavior, backward compatible)
    - **google**: Google service account JSON key; `client_id` is derived from the credential

    For `google` flavor, the `client_secret` field contains a serialized Google service account
    JSON document. The `client_id` in responses is extracted automatically from the JSON's
    `client_id` field; it need not be provided in requests.

    **Future flavors** may be added without breaking API changes (extensible enum).
  example: standard
```

### Update `client_id` description (clarify google behavior)

```yaml
client_id:
  type: string
  description: |
    OAuth2 client_id for this service.

    For `oauth2_flavor: standard`: must be provided in the request.
    For `oauth2_flavor: google`: automatically extracted from the `client_id` field
    in the service account JSON credential; the request `client_id` field is ignored.
  example: Iv1.1234567890abcdef
```

### Update `client_secret` description (clarify flavor-specific semantics, per API-005)

```yaml
client_secret:
  type: string
  description: |
    Authentication credential for this service.

    The meaning varies by `oauth2_flavor`:
    - **standard**: Plain OAuth2 client secret string
    - **google**: Serialized Google service account JSON key document

    **SECURITY**: Always returns "REDACTED" in API responses per SR-003.
    Credentials are never transmitted to the frontend.
    Credentials are encrypted at rest.
  example: REDACTED
  readOnly: true
```

### Update `issuer_uri` description (clarify optional for google flavor)

```yaml
issuer_uri:
  type: string
  format: uri
  description: |
    OAuth2 issuer URI.

    For `oauth2_flavor: standard`: required, must be HTTPS.
    For `oauth2_flavor: google`: optional. If provided, its scheme and host must be consistent
    with the `token_uri` in the service account JSON. If omitted, the `token_uri` from the
    service account JSON is used as the authoritative token endpoint.
  example: https://github.com
```

---

## Schema: `ServiceCreateRequest`

### Add `oauth2_flavor` to properties (optional field, NOT in required list)

```yaml
oauth2_flavor:
  type: string
  enum:
    - standard
    - google
  default: standard
  description: |
    OAuth2 authentication variant. Defaults to `standard` if omitted.

    - **standard**: `client_secret` must be a non-empty plain string; `client_id` is required
    - **google**: `client_secret` must be a valid Google service account JSON key (≤32 KB);
      `client_id` is derived from the JSON's `client_id` field and need not be provided;
      `issuer_uri` is optional; `endpoints` are derived from the service account JSON
  example: standard
```

### Add conditional description for `client_id`

```yaml
client_id:
  type: string
  description: |
    OAuth2 client_id for this service.

    Required when `oauth2_flavor` is `standard`.
    Optional when `oauth2_flavor` is `google` — automatically extracted from the service account JSON.
  example: Iv1.1234567890abcdef
  minLength: 1
```

### Update `client_secret` description for flavor-specific semantics

```yaml
client_secret:
  type: string
  description: |
    Authentication credential for this service.

    The content depends on `oauth2_flavor`:
    - **standard**: Non-empty OAuth2 client secret string
    - **google**: Serialized Google service account JSON key document. Must contain:
        `type` (must be `"service_account"`), `private_key` (non-empty),
        `client_email` (non-empty), `token_uri` (non-empty), `client_id` (non-empty).
        Maximum size: 32 KB.

    **SECURITY**: Encrypted at rest using the AWS KMS Hierarchical Keyring.
    Never returned in API responses (always "REDACTED").
  example: ghp_secretkey1234567890abcdef
  minLength: 1
  writeOnly: true
```

### Update `issuer_uri` to be optional for google flavor

```yaml
issuer_uri:
  type: string
  format: uri
  description: |
    OAuth2 issuer URI.

    Required when `oauth2_flavor` is `standard` (must be HTTPS).
    Optional when `oauth2_flavor` is `google`. If provided, its scheme and host must match
    the `token_uri` in the service account JSON. If omitted, the `token_uri` from the
    service account JSON serves as the token endpoint.
  example: https://github.com
```

### Remove `client_id` and `issuer_uri` from the `required` list (for google flavor compatibility)

The `required` list for `ServiceCreateRequest` should NOT include `client_id` or `issuer_uri` unconditionally, since both are optional for `google` flavor. The validation semantics are documented in field descriptions. Alternatively, use `oneOf` discriminated by `oauth2_flavor`.

> **Implementation note**: The OpenAPI spec uses field descriptions to document conditional requirements. Server-side validation enforces the flavor-specific rules. This avoids complex `oneOf` discriminators which would complicate client code generation.

**Updated required list** for `ServiceCreateRequest`:
```yaml
required:
  - display_name
  - client_secret
  - discovery
  - scopes
```

(`client_id` and `issuer_uri` removed from required since they're optional for `google` flavor; documentation and server validation enforce the conditional requirement.)

---

## Schema: `ServiceUpdateRequest`

Apply the same changes as `ServiceCreateRequest`:
- Add `oauth2_flavor` optional field
- Update `client_id`, `client_secret`, `issuer_uri` descriptions
- Remove `client_id` and `issuer_uri` from required list

---

## Error Response Additions (API-004)

The `ErrorResponse` schema already has `error` + `message` fields. For flavor-specific validation failures, `message` will contain a detailed description identifying which required credential fields are missing or invalid.

Examples:
```json
{
  "error": "validation failed",
  "message": "google service account credential is missing required field: private_key"
}
```
```json
{
  "error": "validation failed",
  "message": "oauth2_flavor must be one of: standard, google (got: azure)"
}
```
```json
{
  "error": "validation failed",
  "message": "issuer_uri scheme and host (accounts.google.com) must match token_uri scheme and host (oauth2.googleapis.com) from service account credential"
}
```

---

## Example Request: Create Google Flavor Service

```yaml
# POST /api/services
{
  "display_name": "Google Workspace APIs",
  "oauth2_flavor": "google",
  "client_secret": "{\"type\":\"service_account\",\"client_id\":\"123456789\",\"client_email\":\"my-sa@project.iam.gserviceaccount.com\",\"private_key\":\"-----BEGIN RSA PRIVATE KEY-----\\n...\",\"token_uri\":\"https://oauth2.googleapis.com/token\"}",
  "discovery": { "enable_discovery": false },
  "scopes": [
    { "scope_value": "https://www.googleapis.com/auth/drive.readonly", "description": "Read Google Drive" }
  ],
  "protected_resources": ["https://www.googleapis.com"]
}
```

## Example Response: Google Flavor Service

```yaml
{
  "id": "770e8400-e29b-41d4-a716-446655440099",
  "display_name": "Google Workspace APIs",
  "oauth2_flavor": "google",
  "client_id": "123456789",
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
