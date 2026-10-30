# Admin Service Contract: `authorization_params`

This contract extends the existing admin service resources:

- `POST /api/services`
- `GET /api/services/{service-id}`
- `GET /api/services`
- `PUT /api/services/{service-id}`

## Field

```yaml
authorization_params:
  type: object
  additionalProperties:
    type: string
  example:
    business_partner_id: "12345"
```

`authorization_params` is optional. Omission or an empty object means the provider receives no additional static parameters during authorization, authorization-code exchange, or refresh-token requests.

## Read Behavior

The field is included in the service response for create, get, list, and update. It is provider configuration, not a client secret.

## Create Behavior

A supplied map replaces the new service's initial empty configuration. Omission creates a service with no additional authorization parameters.

## Update Behavior

| Request field | Result |
|---|---|
| Omitted | Existing configuration remains unchanged. |
| `{}` | Existing configuration is cleared. |
| Object with entries | Existing configuration is replaced by the supplied map. |

## Validation

Each key and value must be a non-blank string after trimming whitespace. Keys are rejected case-insensitively when they are one of:

```text
client_id, client_secret, redirect_uri, response_type, scope, state,
code_challenge, code_challenge_method, code_verifier, nonce, request, request_uri,
code, grant_type
```

Validation errors use the existing administrative validation-error response format.

## Security Boundary

Values originate only from stored administrator configuration. The end-user authorization-start endpoint does not accept or forward arbitrary provider parameters. Broker-generated OAuth2 authorization and token fields remain authoritative.
