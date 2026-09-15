# ADR 036: ExtProc Metadata Token-Exchange Input

**Status**: Accepted
**Date**: 2026-09-14

---

## Supersedes

This decision and [Feature 043](../specs/043-extproc-metadata-input/spec.md) control the ExtProc input trust boundary and exchange-response outcomes. They supersede raw-input descriptions in Feature 015's [specification](../specs/015-extproc-token-exchange/spec.md), [plan](../specs/015-extproc-token-exchange/plan.md), [data model](../specs/015-extproc-token-exchange/data-model.md), [research](../specs/015-extproc-token-exchange/research.md), [quickstart](../specs/015-extproc-token-exchange/quickstart.md), [tasks](../specs/015-extproc-token-exchange/tasks.md), and [gRPC contract](../specs/015-extproc-token-exchange/contracts/extproc-grpc.md).

Feature 015 remains authoritative for standalone-process, sidecar-configuration, cache, circuit-breaker, and exchange mechanics not clarified here.

## Context

The ExtProc token-exchange service previously derived the subject token from the `Authorization` header and the protected resource URI from request pseudo-headers. Those attributes are outside the validated gateway-to-ExtProc trust boundary.

With Agentgateway's required `jwtAuth` policy in `mode: strict`, `preserveToken: false` removes the validated Authorization header before the ExtProc policy runs. Agentgateway retains the token in its in-process claims extension, so `jwt.rawToken.unredacted()` remains available to the following ExtProc metadata expression. The extension does not cross the gRPC boundary. The only gateway-validated credential available to ExtProc is the dynamic metadata projection.

Agentgateway writes `metadataContext` expressions to `ProcessingRequest.MetadataContext.FilterMetadata`. An expression that fails to evaluate produces an absent field. The consumer must therefore treat missing metadata as invalid and fail closed.

## Decision

ExtProc reads token-exchange inputs only from the literal `aib.tokenexchange` metadata namespace:

- `subject_token` is the raw bearer credential supplied by `jwt.rawToken.unredacted()`.
- `resource_uri` is the configured absolute HTTP or HTTPS MCP resource URI.

Agentgateway must apply `jwtAuth` with `mode: strict` before the ExtProc policy and set `preserveToken: false` explicitly. The ExtProc service validates `subject_token` before `resource_uri`. It rejects absent, empty, whitespace-only, scheme-prefixed, or non-string subject-token metadata with a 503 `invalid_subject_token` response. It rejects absent, empty, non-string, or invalid resource metadata with a 503 `invalid_resource` response.


All-whitespace subject-token metadata is rejected. Every accepted nonblank subject token, including one with leading or trailing whitespace, reaches `Exchanger.Exchange` byte-for-byte. After a successful exchange, ExtProc sets the downstream `Authorization` request header to `Bearer <exchanged token>`, whether the inbound header was conflicting, absent, or removed by `jwtAuth`. No failed exchange emits an authorization mutation.


The [Feature 043 response matrix](../specs/043-extproc-metadata-input/spec.md#exchangeerrorresponse-matrix) defines retained exchange-error outcomes.


## Consequences

The raw-header and pseudo-header extraction path is removed. There is no compatibility window. The following alternatives are rejected:

- Raw-header fallback would bypass the gateway-validated metadata trust boundary.
- Dual-source precedence would retain ambiguous input provenance.
- A configuration toggle would make the required fail-closed boundary optional.

A gateway that does not publish both metadata fields receives a fail-closed rejection.

This supersedes the resource-URI construction invariant in `internal/extproc/AGENTS.md` and the raw-input request-processing description in the ExtProc architecture flow. It does not change ADR 011's standalone-process boundary or ADR 012's in-memory token cache.

The service must not log, return, or expose either the subject credential or an unvalidated resource URI. Operators must configure the producer expression after JWT validation; a missing or failed producer field is deliberately indistinguishable from other invalid metadata at the ExtProc boundary.

---

## References

- Supersedes raw token-exchange input extraction described in `specs/015-extproc-token-exchange/`
- Related: [ADR 011: ExtProc Standalone Binary](011-extproc-standalone-binary.md)
- Related: [ADR 012: ExtProc In-Memory Token Cache](012-extproc-in-memory-token-cache.md)
- Feature contract: `specs/043-extproc-metadata-input/contracts/extproc-metadata-input.md`
- Constitution: `.specify/memory/constitution.md` — Principles I, II, VIII, and XIII
