# Contract: Authorisation-Server Metadata Delta

**Feature**: `051-private-key-jwt-auth` | **Date**: 2026-10-15
**Canonical source**: [`api/enduser/openapi.yaml`](../../../api/enduser/openapi.yaml), `OAuth2MetadataResponse` and the metadata endpoint example
**Status**: **PROPOSED**. Requires stakeholder confirmation before implementation (API-003).

## Summary of the delta

| Field | Change |
|---|---|
| `token_endpoint_auth_methods_supported` | Includes `private_key_jwt` in `local` and `hybrid` mode when CIMD is enabled. |
| `token_endpoint_auth_signing_alg_values_supported` | New in responses (the schema already defines the property). `["RS256","ES256"]` under the same condition; absent otherwise. |

The schema property `token_endpoint_auth_signing_alg_values_supported` already exists with a generic description; the schema change is a clearer description and an example. The Go response type gains the matching field.

## Advertised values

| Configuration | `token_endpoint_auth_methods_supported` | `token_endpoint_auth_signing_alg_values_supported` |
|---|---|---|
| `proxy` mode | `["client_secret_post","client_secret_basic"]` (unchanged) | absent |
| `local` / `hybrid`, CIMD disabled | `["client_secret_post"]` (unchanged) | absent |
| `local` / `hybrid`, CIMD enabled | `["none","client_secret_post","private_key_jwt"]` | `["RS256","ES256"]` |

The broker never advertises a method it will reject (FR-025).

## Example (local mode, CIMD enabled)

```json
{
  "issuer": "https://broker.example.com",
  "token_endpoint": "https://broker.example.com/oauth2/token",
  "token_endpoint_auth_methods_supported": ["none", "client_secret_post", "private_key_jwt"],
  "token_endpoint_auth_signing_alg_values_supported": ["RS256", "ES256"],
  "client_id_metadata_document_supported": true
}
```

## Compatibility

Additive. Existing consumers that ignore unknown array members are unaffected. Tests that pin the exact arrays (`service_metadata_test.go`, `service_test.go`) are updated in the same change.
