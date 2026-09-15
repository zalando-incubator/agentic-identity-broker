# Contract: ExtProc Token-Exchange Metadata Input

**Feature**: 043-extproc-metadata-input | **Status**: Proposed | **Consumers**: agentgateway operators

This is the wire contract between an Agentgateway instance (producer) and the ExtProc token-exchange
service (consumer). It is not an HTTP API; it is the Envoy `ext_proc` dynamic-metadata channel.
Breaking it fails every request closed.

## Producer: Agentgateway

### Required policy order

The `jwtAuth` policy MUST validate the request before the `extProc` policy evaluates its `metadataContext` expressions (FR-017, SR-005). `jwt.rawToken` is populated from the claims the JWT policy attaches to the request; without a preceding validated JWT the expression fails to evaluate and the field is absent.


```yaml
binds:
  - port: 4000
    listeners:
      - routes:
          - policies:
              jwtAuth:
                mode: strict
                preserveToken: false
                providers:
                  - issuer: "https://issuer.example.com"
                    audiences:
                      - "mcp-server"
                    jwks:
                      url: "https://issuer.example.com/.well-known/jwks.json"
              extProc:
                host: "extproc-token-exchange:50051"
                failureMode: failClosed
                metadataContext:
                  aib.tokenexchange:
                    subject_token: "jwt.rawToken.unredacted()"
                    resource_uri: "'https://mcp.example.com/mcp'"
            backends:
              - mcp:
                  targets:
                    - name: tools
                      mcp:
                        host: "https://mcp.example.com/mcp"
```

`jwks` also accepts `{ file: <path> }` or an inline JWKS string.

### Expression rules

| Field | Expression | Rule |
|---|---|---|
| `subject_token` | `jwt.rawToken.unredacted()` | MUST resolve to the raw credential with no `Bearer ` prefix |
| `resource_uri` | CEL string literal, e.g. `"'https://mcp.example.com/mcp'"` | MUST resolve to an absolute `http`/`https` URL with a non-empty host |

Note the quoting: a CEL *string literal* inside a YAML string needs both sets of quotes.

`jwt.rawToken` without `.unredacted()` is a configuration error. The value is redacted by default
and does not yield the credential; requests will reach the broker with an unusable subject token and
be rejected there rather than at the gateway.

### `jwtAuth.mode: strict` is required

`mode: optional` permits requests with no JWT. Under this contract such a request produces an absent `subject_token` and a 503. Use `jwtAuth` with `mode: strict` so the gateway rejects unauthenticated traffic itself, with a proper 401.

### The Authorization header is not part of this contract

`preserveToken` defaults to `false`, which makes the JWT policy **remove** the validated credential
from the request after validation. By the time ExtProc runs, the header the old contract read is
gone. Set `preserveToken: false` explicitly rather than inheriting it: this contract depends on the
property, so the configuration should state it.

Stripping the header does **not** break the producer expression. The JWT policy attaches a `Claims`
object holding its own copy of the raw token, and `jwt.rawToken` resolves from that copy rather than
from the request — the claims are attached after the header is already removed. So
`jwt.rawToken.unredacted()` keeps working in the ExtProc `metadataContext` evaluated later in the
same request, while nothing remains for a raw-header reader to find. That asymmetry is the point of
this contract.

The `Claims` copy stays inside agentgateway — it is a request extension, not something ExtProc can
read. The only credential that crosses the gRPC boundary is what your `metadataContext` expressions
project into `MetadataContext.FilterMetadata`. If the producer block is missing or misspelled,
nothing reaches ExtProc, regardless of how the request was authenticated.

ExtProc does not read the Authorization request header and cannot be configured to. It only writes
it, replacing the value with the exchanged token on success.

## Consumer: ExtProc token-exchange service

### Read location

```text
ProcessingRequest
  .metadata_context                             (envoy.service.common.v3.Metadata)
    .filter_metadata["aib.tokenexchange"]       (google.protobuf.Struct)
      .fields["subject_token"]                  (google.protobuf.Value, kind = string_value)
      .fields["resource_uri"]                   (google.protobuf.Value, kind = string_value)
```

The namespace key is the literal string `aib.tokenexchange`. It is a flat map key and is **not**
interpreted as a nested path.

Both values are read only from the `RequestHeaders` phase message. Unrelated fields in the namespace
are ignored and never influence input selection.

### Validation

Validated in this order; the first failure wins (FR-015).

**1. `subject_token` → `invalid_subject_token` on failure**

| Check | Rejected when |
|---|---|
| Namespace present | `metadata_context` is nil, or `aib.tokenexchange` is absent |
| Field present | `subject_token` is absent |
| Kind | value kind is not `string_value` |
| Non-blank | value is empty or whitespace-only |
| No scheme prefix | value begins with `bearer ` (ASCII case-insensitive) |

**2. `resource_uri` → `invalid_resource` on failure**

| Check | Rejected when |
|---|---|
| Field present | `resource_uri` is absent |
| Kind | value kind is not `string_value` |
| Non-blank | value is empty or whitespace-only |
| Absolute URI | value does not parse as an absolute request URI |
| Scheme | scheme is not `http` or `https` |
| Host | host is empty |

The accepted `subject_token` is passed to token exchange **byte-for-byte**. No trimming, decoding,
or normalization is applied (FR-004).

### Responses

Both rejections are Envoy `ImmediateResponse` messages with HTTP status **503** and
`content-type: application/json`.

```json
{"error":"invalid_subject_token","error_description":"subject token metadata is missing or invalid"}
```

```json
{"error":"invalid_resource","error_description":"resource metadata is missing or invalid"}
```

Error descriptions are fixed strings. They never echo a metadata value, a URI, or a credential
(SR-003, SR-001). The specific reason is available in the service's structured logs and in the
`error.type` span attribute.

On success the service performs RFC 8693 token exchange and replaces the `authorization` request header with `Bearer <exchanged token>`. No failure emits an authorization mutation.

#### Exchange outcome matrix

| Condition | Response |
|---|---|
| Generic exchange error, broker 4xx without `error_uri`, transient 429 or 5xx, network error, or timeout | HTTP 500, `content-type: application/json`, `{"error":"token_exchange_failed","error_description":"token exchange request failed"}` |
| `ErrAssertionExpired` | HTTP 503, `{"error":"service_unavailable","error_description":"client assertion expired"}` |
| `ErrCircuitOpen` | HTTP 503, `{"error":"service_unavailable","error_description":"circuit breaker is open"}` |
| Nontransient broker error with `error_uri` for MCP | HTTP 200 JSON-RPC 2.0 URL elicitation with code `-32042` |
| Nontransient broker error with `error_uri` for non-MCP | HTTP 503, `{"error":"service_unavailable","error_description":"token exchange requires re-authentication"}` |



### Non-goals

- No fallback to HTTP request attributes exists for either input (FR-010, FR-011).
- No configuration key enables, disables, or renames this contract. The namespace and field names
  are fixed.
- The separate `agentgateway.protocol` metadata namespace consumed by OPA authorization is
  unaffected and remains independently required when authorization is enabled.

## Compatibility

This is a clean cutover with no compatibility window. A deployment that upgrades ExtProc without
adding the `aib.tokenexchange` producer to its gateway receives `invalid_subject_token` 503s on
every request. Update the gateway configuration first, or in the same change.
