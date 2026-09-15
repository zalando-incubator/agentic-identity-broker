# Feature Specification: ExtProc Metadata Input

**Feature Branch**: `043-extproc-metadata-input`  
**Created**: 2026-09-14  
**Status**: Draft  
**Input**: User description: "Use dynamic metadata for the bearer token and MCP resource URL. Use `aib.tokenexchange.subject_token` and `aib.tokenexchange.resource_uri`. Remove raw-attribute extraction without backwards compatibility."

## Supersedes

Feature 043 controls subject-token and resource-URI provenance, metadata validation order and 503 responses, raw-attribute removal, no-pass-through behavior, and the external response matrix for exchange outcomes.

Feature 015 remains authoritative for the standalone process, sidecar configuration, cache, circuit-breaker, and exchange mechanics that Feature 043 does not clarify.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Exchange From Dynamic Metadata (Priority: P1)

An operator configures an Agentgateway instance that protects one MCP resource. Its `jwtAuth` policy validates the bearer token. Its global ExtProc policy supplies the validated raw token and resource URL as dynamic metadata.


**Why this priority**: This capability creates the required token-exchange boundary. It removes dependency on mutable transport attributes.

**Independent Test**: Send an ExtProc request with both metadata values. Use missing or conflicting raw HTTP attributes. Confirm that the exchange receives only the metadata values.

**Acceptance Scenarios**:

1. **Given** an Agentgateway `jwtAuth` policy validates the bearer token. **And** the `aib.tokenexchange` namespace contains string fields `subject_token` and `resource_uri`. **When** ExtProc processes the request. **Then** it exchanges the validated raw token for the configured MCP resource.
2. **Given** raw HTTP attributes conflict with both metadata values. **When** ExtProc processes the request. **Then** it uses only the metadata values for token exchange.
3. **Given** valid metadata and a failed or unauthorized token exchange. **When** ExtProc processes the request. **Then** it returns the `exchangeErrorResponse` matrix outcome and does not forward a credential.

4. **Given** either required value is outside the `aib.tokenexchange` namespace or is not a string field. **When** ExtProc processes the request. **Then** it rejects the request without token exchange.

---

### User Story 2 - Configure Instance Resource (Priority: P2)

An operator configures the global ExtProc policy for an Agentgateway instance. The policy supplies the resource URL for the single MCP resource that the instance protects.

**Why this priority**: The resource URL binds token exchange to the intended MCP resource. One ExtProc policy applies to the entire Agentgateway instance.

**Independent Test**: Process a request through an Agentgateway instance with configured resource metadata. Confirm that token exchange uses that configured resource URL.

**Acceptance Scenarios**:

1. **Given** an Agentgateway instance has global ExtProc metadata with `aib.tokenexchange.resource_uri`. **When** ExtProc processes a request from that instance. **Then** token exchange uses the configured resource URL.
2. **Given** a valid subject-token value and missing, empty, non-string, or invalid resource metadata. **When** ExtProc processes the request. **Then** it returns a 503 JSON immediate response with `error` set to `invalid_resource`.
---

### User Story 3 - Remove Raw Attribute Dependency (Priority: P3)

An operator upgrades to metadata-only token-exchange inputs. ExtProc does not extract the subject credential or resource URL from raw HTTP attributes.

**Why this priority**: Removing the old path prevents ambiguous input precedence. This completes the clean cutover.

**Independent Test**: Send a request with raw HTTP authorization and target attributes only. Confirm that ExtProc neither exchanges nor forwards the raw credential.

**Acceptance Scenarios**:

1. **Given** subject-token metadata is missing, empty, whitespace-only, scheme-prefixed, or not a string. **When** ExtProc processes the request. **Then** it returns a 503 JSON immediate response with `error` set to `invalid_subject_token`.

2. **Given** a deployment uses only raw HTTP attribute extraction. **When** it does not supply both metadata values. **Then** it receives the `invalid_subject_token` 503 response.

### Edge Cases

- A request has no dynamic metadata, only one value, or whitespace-only values. ExtProc returns the defined 503 JSON immediate response.
- A subject-token value has a `Bearer ` scheme prefix. ExtProc returns the `invalid_subject_token` 503 response and does not forward it.
- A resource-URL value is not an absolute HTTP or HTTPS URL with a host. ExtProc returns the `invalid_resource` 503 response.
- Both required metadata values are invalid. ExtProc validates the subject token first and returns the `invalid_subject_token` 503 response.
- Metadata values conflict with raw HTTP attributes. ExtProc uses the metadata values as its only exchange inputs.
- Dynamic metadata has unrelated fields. These fields do not affect token-exchange input selection.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST get the subject credential only from `aib.tokenexchange.subject_token`.
- **FR-002**: The system MUST get the resource identifier only from `aib.tokenexchange.resource_uri`.
- **FR-003**: `aib.tokenexchange.subject_token` MUST contain the raw bearer credential without the `Bearer ` scheme prefix.
- **FR-004**: The system MUST pass the subject-token value to token exchange without parsing, trimming, or normalizing it.

  All-whitespace values are rejected. Every accepted nonblank value, including leading or trailing whitespace, reaches `Exchanger.Exchange` byte-for-byte.

- **FR-005**: `aib.tokenexchange.resource_uri` MUST contain a non-empty absolute HTTP or HTTPS URL with a non-empty host.
- **FR-006**: When both metadata values are valid, the system MUST use them for token exchange.
- **FR-007**: After a successful exchange, the system MUST set the downstream `Authorization` request header to `Bearer <exchanged token>`, whether the inbound header was conflicting, absent, or removed by `jwtAuth`. No failed exchange may emit an authorization mutation.
- **FR-008**: The system MUST reject a request if either required metadata value is missing, empty, or invalid.
- **FR-009**: The system MUST return the outcome in the `exchangeErrorResponse` matrix when token exchange fails or is unauthorized.

- **FR-010**: The system MUST NOT use raw HTTP authorization, request-target, or other raw HTTP attributes as token-exchange input sources.
- **FR-011**: The system MUST NOT fall back from either required metadata field to raw HTTP attributes.
- **FR-012**: The system MUST read both token-exchange inputs from the `aib.tokenexchange` dynamic-metadata namespace. It MUST read its string fields `subject_token` and `resource_uri`.
- **FR-013**: For missing, empty, whitespace-only, scheme-prefixed, or non-string subject-token metadata, the system MUST return a 503 JSON immediate response with `error` set to `invalid_subject_token`.

- **FR-014**: For missing, empty, non-string, or invalid resource metadata, the system MUST return a 503 JSON immediate response with `error` set to `invalid_resource`.
- **FR-015**: The system MUST validate subject-token metadata before resource metadata. If both are invalid, it MUST return `invalid_subject_token`.
- **FR-016**: The system MUST document the Agentgateway `metadataContext.aib.tokenexchange` configuration in the gateway token-exchange guide. It MUST document the raw subject-token format and resource-URL rules.
- **FR-017**: The Agentgateway `jwtAuth` policy MUST validate the bearer token before its global ExtProc policy evaluates `jwt.rawToken.unredacted()` for subject-token metadata.


#### `exchangeErrorResponse` matrix

| Condition | Response |
|---|---|
| Generic exchange error, broker 4xx without `error_uri`, transient 429 or 5xx, network error, or timeout | HTTP 500, `content-type: application/json`, `{"error":"token_exchange_failed","error_description":"token exchange request failed"}` |
| `ErrAssertionExpired` | HTTP 503, `{"error":"service_unavailable","error_description":"client assertion expired"}` |
| `ErrCircuitOpen` | HTTP 503, `{"error":"service_unavailable","error_description":"circuit breaker is open"}` |
| Nontransient broker error with `error_uri` for MCP | HTTP 200 JSON-RPC 2.0 URL elicitation with code `-32042` |
| Nontransient broker error with `error_uri` for non-MCP | HTTP 503, `{"error":"service_unavailable","error_description":"token exchange requires re-authentication"}` |

Every matrix branch rejects without forwarding a credential.


### Metadata Producer Contract

Agentgateway declares `ExtProc.metadata_context` as a namespace-to-fields map of CEL expressions. It serializes this map as `MetadataContext.FilterMetadata[namespace].Fields[field]`.

- `metadataContext.aib.tokenexchange.subject_token` receives `jwt.rawToken.unredacted()` from the validated `jwtAuth` policy.
- `metadataContext.aib.tokenexchange.resource_uri` supplies the MCP resource URL.

**Sources**: [Agentgateway ExtProc metadata producer](https://github.com/agentgateway/agentgateway/blob/v1.5.0/crates/agentgateway/src/http/ext_proc.rs) and [Agentgateway CEL context](https://agentgateway.dev/docs/standalone/latest/reference/cel/cel-context/).


### Agentgateway Configuration Example

This illustrative configuration uses the Agentgateway v1.5.0 policy nesting. It applies `jwtAuth` and `extProc` to the MCP backend served by this Agentgateway instance.

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

The `jwtAuth` policy validates the request token. The ExtProc policy evaluates `jwt.rawToken.unredacted()` to provide the raw token without the `Bearer ` prefix. The resource URI is a CEL string literal.


### Security Requirements

- **SR-001**: The token-exchange input trust boundary MUST be the ExtProc dynamic metadata context. Raw HTTP attributes are outside this boundary.
- **SR-002**: The system MUST fail closed when metadata is missing or invalid. It MUST use the `exchangeErrorResponse` matrix for every exchange failure and must not forward a credential.

- **SR-003**: The system MUST NOT expose subject-token or exchanged-token values in diagnostics, audit records, or telemetry.
- **SR-005**: The Agentgateway `jwtAuth` policy MUST use strict validation before `jwt.rawToken.unredacted()` supplies the subject token.


### Key Entities

- **Subject Token Metadata**: Dynamic metadata named `aib.tokenexchange.subject_token`. It holds the raw bearer credential without its scheme prefix.
- **Resource URI Metadata**: Dynamic metadata named `aib.tokenexchange.resource_uri`. It holds the HTTP or HTTPS URL for the MCP server resource.
- **Agentgateway Instance**: An operator-managed gateway instance with global `jwtAuth` and ExtProc policies. It validates tokens and supplies metadata for one MCP resource.


## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of successful acceptance-test exchanges use both required metadata values. This includes requests with conflicting raw HTTP attributes.
- **SC-002**: 100% of requests with invalid required metadata receive the defined 503 JSON immediate response. If both values are invalid, the response uses `invalid_subject_token`.
- **SC-003**: 100% of acceptance-test requests through an Agentgateway instance use its configured HTTP or HTTPS MCP resource URL for token exchange.
- **SC-004**: 100% of acceptance-test subject-token values reach token exchange unchanged and without a `Bearer ` scheme prefix.
- **SC-005**: Credential-free diagnostics state the metadata rejection reason in 100% of defined invalid-metadata scenarios.

## Assumptions

- An Agentgateway instance can publish both required metadata values before ExtProc processes a request. Its global ExtProc policy protects one MCP resource.
- Dynamic metadata follows Agentgateway's `metadataContext` namespace-and-field producer contract. It serializes to `MetadataContext.FilterMetadata[namespace].Fields[field]`.
- The global ExtProc policy for each Agentgateway instance publishes one intended absolute HTTP or HTTPS resource URL with a host.
- This is a clean cutover. Deployments must publish both required values before upgrade. Raw HTTP attribute compatibility is out of scope.
- If either required metadata value is absent, ExtProc rejects the request. This applies even when the request has no raw authorization credential.

## Configuration Documentation

The existing sidecar settings remain documented in [the gateway token-exchange guide](../../docs/guides/token-exchange-gateway.md) and [the ExtProc configuration example](../../examples/config/extproc-token-exchange.yaml). The gateway guide contains the canonical `metadataContext` producer block. The ExtProc configuration example remains the sidecar `EXTPROC_` schema.
