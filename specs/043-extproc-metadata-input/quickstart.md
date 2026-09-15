# Quickstart: ExtProc Metadata Input

**Feature**: 043-extproc-metadata-input | **Date**: 2026-09-14

## What This Feature Does

The ExtProc token-exchange service takes its two token-exchange inputs — the subject credential and
the protected resource URL — exclusively from Envoy dynamic metadata published by Agentgateway in
the `aib.tokenexchange` namespace. Raw HTTP attributes (the `Authorization` header and the
`:scheme`/`:authority`/`:path` pseudo-headers) are no longer input sources. A request that does not
carry both valid metadata values is rejected with a 503 JSON immediate response.

Full input rules: [`contracts/extproc-metadata-input.md`](contracts/extproc-metadata-input.md).
Types and telemetry vocabulary: [`data-model.md`](data-model.md).

## Configure the Gateway

Add the producer to the Agentgateway route policy. The `jwtAuth` policy must come first — it validates the token and populates `jwt.rawToken`.


```yaml
policies:
  jwtAuth:
    mode: strict
    preserveToken: false
    providers:
      - issuer: "https://issuer.example.com"
        audiences: ["mcp-server"]
        jwks:
          url: "https://issuer.example.com/.well-known/jwks.json"
  extProc:
    host: "extproc-token-exchange:50051"
    failureMode: failClosed
    metadataContext:
      aib.tokenexchange:
        subject_token: "jwt.rawToken.unredacted()"
        resource_uri: "'https://mcp.example.com/mcp'"
```

`resource_uri` is a CEL string literal, so it needs quotes inside quotes. One ExtProc policy applies
to one Agentgateway instance, which protects one MCP resource.

No ExtProc sidecar configuration changes. `examples/config/extproc-token-exchange.yaml` is
unaffected.

## Validate Locally with Docker Compose

```bash
just compose-extproc-up
```

The Compose gateway configuration (`mocks/agentgateway/config.yaml`) publishes the namespace. Drive
an MCP request through port 4000, then confirm that ExtProc recorded a successful exchange:

```bash
just compose-extproc-logs
```

Expected on success: `token exchanged successfully`. When a resource URI is emitted, it must use
the `sanitizeURIForTelemetry` representation: scheme, host, and path only, with query and fragment
removed. Expected when the gateway is missing the producer: a 503 and a log line naming the rejected
field. Output must never contain a credential, query, fragment, or rejected raw resource value.

## Validate the Rejection Paths

Send an ExtProc request directly over gRPC with a raw `Authorization` header and `:path`, and no
`aib.tokenexchange` metadata. The service must answer 503 with:

```json
{"error":"invalid_subject_token","error_description":"subject token metadata is missing or invalid"}
```

With a valid `subject_token` but a missing, blank, non-string, or relative `resource_uri`:

```json
{"error":"invalid_resource","error_description":"resource metadata is missing or invalid"}
```

When both values are invalid, the response is `invalid_subject_token` (FR-015).

## Run the Tests

```bash
# ExtProc unit tests with the race detector
just extproc-test

# ExtProc E2E suite (includes Docker agentgateway scenarios)
just test-e2e-extproc

# Focused metadata-input scenarios
ginkgo -v --focus "Metadata Input" ./tests/e2e/extproc/
```

Docker-backed scenarios skip automatically when Docker is unavailable. They pin
`cr.agentgateway.dev/agentgateway:v1.5.0`; override with `AGENTGATEWAY_IMAGE`.

## Key Implementation Files

| Path | Role |
|---|---|
| `internal/extproc/server/server.go` | Metadata extraction, validation, both header paths |
| `internal/extproc/server/server_test.go` | Unit coverage for extraction, validation, rejection |
| `tests/e2e/extproc/metadata_input_test.go` | Direct-gRPC acceptance scenarios |
| `tests/e2e/extproc/agentgateway_metadata_e2e_test.go` | Real gateway JWT → metadata producer path |
| `tests/e2e/extproc/helpers/grpc_helpers.go` | `WithTokenExchangeMetadata` request builder |
| `mocks/agentgateway/config.yaml` | Compose gateway producer configuration |
| `docs/guides/token-exchange-gateway.md` | Operator guide |
| `adrs/036-extproc-metadata-token-exchange-input.md` | Binding decision record |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Every request returns `invalid_subject_token` | Gateway has no `aib.tokenexchange` producer | Add the `metadataContext` block |
| `invalid_subject_token` only for some clients | `jwtAuth.mode: optional` lets unauthenticated requests through | Set `mode: strict` |
| Broker rejects the exchange with an invalid subject token | Producer uses `jwt.rawToken` instead of `jwt.rawToken.unredacted()` | Add `.unredacted()` |
| `invalid_resource` with a plausible URL | `resource_uri` is not quoted as a CEL string literal, or is relative | Use `"'https://host/path'"` |
| Requests worked before the upgrade, now all 503 | Clean cutover — raw attribute extraction was removed | Update the gateway configuration |
