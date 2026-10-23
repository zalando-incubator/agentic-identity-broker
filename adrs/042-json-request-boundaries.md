# ADR 042: Shared JSON Request Boundaries

**Status**: Accepted
**Date**: 2026-10-22
**Approval**: Stakeholder accepted this decision on 2026-10-22 in the introducing branch.

## Context

The admin, approval, and consent HTTP handlers accept JSON requests. Separate decoders can apply different size limits and accept different request formats.
A decoder can accept the start of a body without reading the rest. A Content-Length check alone cannot limit the size of chunked requests.

These checks belong at the HTTP adapter boundary, before domain calls.
Like [ADR 035](035-shared-tool-pattern-matching.md), this decision puts code used by multiple callers in one shared package.

## Decision

`internal/adapters/http/requestbody` limits body size, decodes JSON, and selects HTTP status codes for request errors.
Handlers declare their route limits and keep their existing response formats. Domain services keep business validation and do not depend on this HTTP package.

The boundary applies these rules:

- The reader checks the size of the whole body before JSON decoding. The size includes whitespace and trailing content.
- The reader enforces the limit even without a declared Content-Length. This includes chunked requests.
- Bodies larger than the limit return HTTP 413. This takes priority over malformed JSON or unknown fields earlier in the body.
- Nonempty bodies must contain exactly one JSON value. Malformed JSON, unknown fields in closed request objects, and trailing non-whitespace content return HTTP 400.
- Open map fields accept arbitrary keys. Custom JSON unmarshalers must also reject unknown fields in closed request objects.
- Handlers keep their existing rules for optional empty bodies. Handlers must not treat a read error as a successful empty request.
- Handlers must reject invalid bodies before they call domain operations.

### Route limits

Limits count bytes, not characters. The reader allows a body exactly at the limit to proceed to JSON decoding.

| Operations | Maximum body size |
|---|---:|
| Agent, service, and permission-set create/update, and consent grants | 64 KiB (65536 bytes) |
| Approval creation | 7 MiB (7340032 bytes) |
| Approval decisions that grant access, and scope previews | 512 KiB (524288 bytes) |
| Protected-resource add/rename | 8 KiB (8192 bytes) |
| Signing-key generation, CIMD-key generation, session sweep, and approval denial | 4 KiB (4096 bytes) |

Approval creation allows 7 MiB: sixfold JSON re-encoding expansion of the ExtProc `authorization.max_body_size` default of 1 MiB, plus 1 MiB of envelope headroom.
Go's JSON encoder can expand characters such as `<` to six bytes when ExtProc re-marshals the tool arguments. The limits do not share a configuration value.
Approval decisions and scope previews allow the largest stored parameter pattern: 64 entries with 255-byte keys and 1024-byte values, each byte requiring up to six JSON bytes, plus object syntax and the request envelope.
Smaller administrative operations use smaller limits instead of the largest approval limit.

Each JSON operation must declare its limit and error responses in the relevant OpenAPI document.
New JSON handlers must use this boundary rather than introduce independent decoding rules.

This decision does not change OAuth2 form parsing, upstream response limits, or ExtProc body handling.
Those boundaries keep their protocol-specific rules.

## Rationale

One shared reader keeps size checks and JSON parsing consistent across handlers.
The reader checks the size of the whole body before decoding. As a result, oversized requests return HTTP 413 rather than a JSON error.
Separate route limits restrict small operations without rejecting supported approval bodies.

## Consequences

**Positive**:

- JSON handlers share one set of rules. HTTP request checks stay outside the domain.
- Chunked bodies and trailing content cannot bypass the byte limit.
- OpenAPI documents describe the limits and error responses for clients.

**Negative**:

- The reader stores each body in memory up to its route limit before decoding. Multiple requests can still use more memory together.
- Clients that send unknown fields or trailing JSON documents receive HTTP 400.
- Changes to route limits require matching updates to handlers, OpenAPI documents, and boundary tests.

## Alternatives Considered

We rejected these alternatives:

1. **Separate handler decoders**: Size checks and field rules can differ across handlers.
2. **One global body limit**: Small administrative requests and approval bodies have different size requirements.
3. **Content-Length checks only**: Chunked requests can omit the declared length.
4. **Decode before reading the whole body with a size limit**: An early parsing error can hide an oversized body.

## References

- [Architecture: JSON request boundaries](../ARCHITECTURE.md)
- [Shared request reader](../internal/adapters/http/requestbody/json.go)
- [Admin API](../api/admin/openapi.yaml)
- [End-user API](../api/enduser/openapi.yaml)
- [Constitution, Principle II](../.specify/memory/constitution.md)
