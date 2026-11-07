# Contract Delta: ExtProc Challenge and Metadata Input

**Status**: Proposed. Approval items API-EXCHANGE-CHALLENGE and API-METADATA-CONFIG (see
[plan.md](../plan.md#approval-items)). This contract extends the following:

- the ExtProc metadata input contract
  (`specs/043-extproc-metadata-input/contracts/extproc-metadata-input.md`, ADR 036);
- the Feature 023 URL-elicitation response.

It does not change ADR 011, ADR 012, ADR 036, or the broker Helm chart (Principle VII). ExtProc is
deployed separately.

## 1. Optional metadata field `aib.tokenexchange.resource_metadata_url` (API-METADATA-CONFIG)

| Field | Required | Type | Validation | Invalid value |
|---|---|---|---|---|
| `subject_token` | yes | string | unchanged | unchanged (503 `invalid_subject_token`) |
| `resource_uri` | yes | string | unchanged | unchanged (503 `invalid_resource`) |
| `resource_metadata_url` | **no** | string | The value must be an absolute `https` URL with a non-empty host. It must have no userinfo and no fragment. Every byte must be in `0x21`–`0x7E`, excluding `"` and `\`. The value must be at most 2048 bytes. | Omitted from the challenge. ExtProc logs a structured diagnostic (`challenge_metadata_invalid`) without the value. The request outcome is unchanged. |

ExtProc validates the field only when a challenge is produced. The field never affects token
exchange, OPA input, caching, or authorization. It cannot come from any HTTP header (ADR 036).

Operator configuration: a CEL string literal on the Agentgateway route, beside `resource_uri`.

```yaml
extProc:
  host: "extproc-token-exchange:50051"
  failureMode: failClosed
  metadataContext:
    aib.tokenexchange:
      subject_token: "jwt.rawToken.unredacted()"
      resource_uri: "'https://agentgateway.example.com/mcp'"
      resource_metadata_url: "'https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp'"
```

There is no ExtProc YAML key, `EXTPROC_` environment variable, or CLI flag for this value. It is
specific to each route, as RFC 9728 metadata is specific to each resource.

## 2. Broker error decoding

`BrokerExchangeError` gains a `Scope string` field, decoded from the broker error JSON `scope`
member. Transient classification does not change. A 429 or 5xx response is never challenged, and a
response without `error_uri` keeps its existing outcome.

## 3. Challenge for a non-transient broker error with `error_uri`

Status, body, and elicitation are unchanged: HTTP 403 for every protocol, and the JSON-RPC
`-32042` URL elicitation with `id: null` and the unchanged `url`. The header is built in this fixed
order. Each optional attribute is included only when its value passes validation:

```text
WWW-Authenticate: Bearer error="insufficient_scope"
                  [, error_description="<broker description>"]
                  [, error_uri="<broker error_uri>"]
                  [, scope="<broker scope>"]
                  [, resource_metadata="<metadata resource_metadata_url>"]
```

| Attribute | Source | Validation | When invalid |
|---|---|---|---|
| `scope` | broker `scope` | Exactly one token. It must parse as an absolute `http` or `https` URL with a host and pass `validChallengeURI`, which rejects spaces, quotes, backslashes, and control characters. | Omitted. The challenge stays valid. |
| `resource_metadata` | metadata field | As in §1 | Omitted |

Example: spec Journey 1, Step 2, with metadata configured.

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
WWW-Authenticate: Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub", scope="https://broker.example.com/sessions/github", resource_metadata="https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp"

{"jsonrpc":"2.0","id":null,"error":{"code":-32042,"message":"User session is unavailable. Please re-authenticate.","data":{"elicitations":[{"mode":"url","elicitationId":"<uuid>","url":"https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub","message":"User session is unavailable. Please re-authenticate."}]}}}
```

Without the metadata field, the header is identical except that `resource_metadata` is absent
(Story 4, Scenario 1).

## 4. Logging

ExtProc never logs credentials. It logs the scope only as `challenge.scope_present=true|false` and
`challenge.resource_metadata_present=true|false`. The broker-provided URLs are already public
action links, but the logging footprint stays unchanged apart from these booleans.
