# Data Model: Provider Authorization Parameters

## Existing Aggregate: ThirdpartyOAuth2ProviderEntity

Extend the existing `ThirdpartyOAuth2ProviderEntity` aggregate with an optional `AuthorizationParams` string-to-string map.

| Field | Type | Required | Rules |
|---|---|---:|---|
| `AuthorizationParams` | map of string to string | No | `nil` or empty means no additional upstream parameters. |

The map is part of the provider configuration lifecycle. It is neither a secret nor user-session data, so it is returned in administrative read models and is not encrypted.

## Value Semantics

- A map is owned by exactly one third-party OAuth2 service.
- The map has no independent identifier, endpoint, or lifecycle.
- Copies of the entity must deep-copy the map so in-memory storage and callers cannot mutate persisted configuration by alias.
- PostgreSQL stores the value as a non-null JSON object with an empty-object default. In-memory storage stores it through the entity's deep-copy behavior.

## Validation Invariants

| Invariant | Enforcement |
|---|---|
| Parameter names are not blank or whitespace-only. | Entity create and update validation. |
| Parameter values are not blank or whitespace-only. | Entity create and update validation. |
| A name cannot be a broker-owned OAuth2 field. | Entity create and update validation, case-insensitive. |
| Reserved names are `client_id`, `client_secret`, `redirect_uri`, `response_type`, `scope`, `state`, `code_challenge`, `code_challenge_method`, `code_verifier`, `nonce`, `request`, `request_uri`, `code`, and `grant_type`. | Entity create and update validation. |
| A JSON object has only one final value per name. | Standard request decoding semantics; no separate duplicate collection is persisted. |

## Update Semantics

| Administrative request value | Stored result |
|---|---|
| Field omitted | Preserve the previously stored map. |
| Empty object | Replace the existing map with no parameters. |
| Non-empty object | Replace the existing map after validation. |

## OAuth2 Flows

1. The end-user handler validates only its existing broker-owned input, including `redirect_uri`.
2. `OAuth2SessionService` fetches the persisted third-party service.
3. The session service builds the normal OAuth2 authorization URL with broker-owned client, callback, scope, state, and PKCE values, then appends stored `AuthorizationParams` values.
4. After callback validation, the session service exchanges the authorization code with normal broker-owned `code`, `grant_type`, and PKCE verifier fields plus stored `AuthorizationParams` values.
5. When refreshing a session, the service sends normal broker-owned `grant_type`, `refresh_token`, and client credentials plus stored `AuthorizationParams` values.
6. Browser query values other than supported broker inputs are not read or forwarded; they cannot override stored values.

## Persistence Migration

Add `authorization_params JSONB NOT NULL DEFAULT '{}'::jsonb` to `thirdparty_oauth2_services`, with a descriptive column comment. The down migration removes the column. Existing rows therefore behave exactly as services without additional parameters.
