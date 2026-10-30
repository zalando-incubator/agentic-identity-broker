# Implementation Plan: Request Security Context Propagation

**Branch**: `20260629-120000-request-security-context` | **Date**: 2026-06-29 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/20260629-120000-request-security-context/spec.md`

**Artifacts**: [research.md](./research.md) · [data-model.md](./data-model.md) · [contracts/request-security-context.md](./contracts/request-security-context.md) · [quickstart.md](./quickstart.md)

## Summary

Establish a system-wide mechanism that, at the HTTP and gRPC perimeters, captures a per-request **security context** — actor, optional calling peer when distinct, client IP, user agent, request method/target, receipt timestamp, and a Trace ID — and propagates it implicitly via Go's `context.Context` to downstream layers, returning the Trace ID to callers. For ordinary HTTP traffic the final `SecurityContext` is established in the generic middleware from the existing principal context. For token-exchange flows, the middleware captures transport metadata and Trace ID once, but defers identity establishment until the post-validation token-exchange seam — the route's effective perimeter for identity fields — where the validated subject-token principal becomes the actor and the validated `client_assertion` subject becomes the `calling_peer` when distinct. The Trace ID is carried on `SecurityContext.TraceID` (the single authority) — the OpenTelemetry span trace ID when a span exists, else a `crypto/rand` fallback — read identically by the log handler and the `traceresponse` response header, so spans, logs, and the header agree in both tracing-on and tracing-off modes. Capture is fail-open (never rejects a request; degrades to `anonymous` with no calling peer) and layered beneath unchanged fail-closed authentication. Implementation centers on one new domain value object (`internal/domain/security`), one new HTTP middleware registered in the shared `http.NewHandler` seam (with the perimeter stack reordered so capture runs after `otelchi`), a token-exchange finalization seam so validated subject/client identities produce the final context exactly once, a context-enriching `slog.Handler` so every context-aware log line carries `trace_id`/`actor`/`calling_peer` when present, a new `request_context` config block (trusted-proxy aware, secure by default), and behavior parity in the standalone ExtProc gRPC service.
## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5, `riandyrn/otelchi`, `go.opentelemetry.io/otel` + `otel/trace` v1.44.0,
`otelslog` bridge, `log/slog`; ExtProc uses `envoyproxy/go-control-plane` + shared
telemetry (ADR 20260421-120000)
**Storage**: N/A — no persistence, no schema changes (in-process `context.Context` value only)
**Testing**: Go `testing` + `testify` (unit); Ginkgo/Gomega (E2E, `tests/e2e/` and `tests/e2e/extproc/`)
**Target Platform**: Linux server (dual HTTP servers :8000 admin / :14000 enduser; standalone ExtProc gRPC)
**Project Type**: single (Go backend monorepo, hexagonal)
**Performance Goals**: < 1 ms median per-request capture overhead (SC-008); no added allocations on the hot path
beyond one context value + one response header
**Constraints**: Capture must be fail-open (never reject); Trace ID = `SecurityContext.TraceID` (the span trace ID when a span exists, else a `crypto/rand` fallback), read identically by logs and the `traceresponse` header; no secrets in context/logs; access-control behavior unchanged; actor remains the authenticated identity while `calling_peer` is optional and only present when distinct; token-exchange routes MUST treat the post-validation token-exchange seam as the effective perimeter for identity fields and MUST build the final `SecurityContext` there exactly once from captured transport metadata plus validated `subject_token` / `client_assertion` claims; ExtProc must not import core packages (ADR 20260223-120000)
**Scale/Scope**: Every inbound request on all three perimeters; ~1 new domain pkg, 1 HTTP middleware, 1 token-exchange finalization seam, 1 slog handler, config + Helm + examples, ExtProc per-request logger, ADR + glossary, E2E suites

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: `SecurityContext` (value object), `Actor`, `CallingPeer`, and `TraceID` identified and documented in [data-model.md](./data-model.md).
- [x] **Domain Concepts**: New terms (`SecurityContext`, `Actor`, `CallingPeer`, `anonymous`, `Trace ID`) will be added to the ARCHITECTURE.md Glossary (research D9).
- [x] **Entity IDs**: N/A — `SecurityContext` is a value object with no UUID primary key; no typed ID added
  (ADR 20260313-120000 does not apply; research D5).
- [x] **Configuration Design**: `request_context` block designed with YAML example (contracts C2,
  [research.md](./research.md) D6).
- [x] **Config Examples**: `examples/config/request-context.yaml` will be added and referenced in
  `examples/config/README.md`.
- [x] **Helm Chart**: Config params added → `charts/agentic-identity-broker/values.yaml`, ConfigMap template,
  and README will be updated (contracts C2).
- [x] **API Design First**: No new endpoints; no `openapi.yaml` endpoint changes. The only API-surface change
  is one additive global response header (W3C `traceresponse`, contracts C1).
- [x] **API Documentation**: The additive response header will be documented in ARCHITECTURE.md; no
  per-endpoint OpenAPI bodies change.
- [x] **API Changes**: The global `traceresponse` response header is additive/non-breaking, but
  Constitution API-006 requires explicit stakeholder confirmation **before** implementation. This has been obtained for the `traceresponse` header.
- [x] **Database Design**: N/A — no schema changes, no migrations (data-model: Persistence = None).
- [x] **E2E Acceptance Tests**: E2E tests will be written for all spec scenarios before implementation (Testing Strategy below; quickstart V1–V10).
- [x] **E2E Test Mapping**: Each acceptance scenario maps 1:1 to one `It()` block (scenario table below).
- [x] **E2E Red Phase**: E2E tests will assert concrete values (`traceresponse` presence + W3C format, `trace_id`/`actor`/`calling_peer` log fields when applicable, `401` on protected routes, recorded IP) and fail before implementation.
- [x] **Frontend Playwright E2E**: N/A — no React UI changes.
- [x] **Frontend Screenshots**: N/A — no UI changes.

**Implementation Considerations**:

- [x] **Security-First**: Capture always on; trusted-proxy off by default; client IP not spoofable
  (right-most XFF entry); no weakening of auth (research D4, D8).
- [x] **Architecture Docs**: ARCHITECTURE.md updated (glossary, middleware ordering, `traceresponse` header, request-security-context security NFRs SR-001–SR-006, and performance budget SC-008).
- [x] **ADRs**: New `adrs/20260703-120100-request-security-context-propagation.md` (Proposed) records the seam, ordering,
  and trace-id authority rule — `SecurityContext.TraceID` = span trace ID, else `crypto/rand` fallback (research D9).
- [x] **Library-First Security**: Trace ID from OTel (no custom); fallback ID via `crypto/rand`; no custom
  crypto.
- [x] **Zalando Guidelines**: No endpoint/contract shape changes. The response uses the W3C-standard
  `traceresponse` header (Trace Context Level 2), so there is **no** RFC 6648 / Zalando "Do Not Use X- Headers"
  concern — standards-compliant by construction; recorded in ADR 20260703-120100 (contract C1 / research D10).
- [x] **End-User Docs**: `docs/configuration.md` updated with the `request_context` block.
- [x] **Migration Testing**: N/A — no migrations.
- [x] **Hexagonal Architecture**: Domain value object (`internal/domain/security`) imports no infra; capture is
  a driving adapter (HTTP middleware); config via `ports.Config` (research D5, D6).
- [x] **Persistence Patterns**: N/A — no persistence added.

*All design preconditions satisfied. No unresolved spec clarifications. Planning may proceed to `/speckit-tasks` or `/speckit-implement`.*

**Post-Design re-check (after Phase 1)**: PASS with no open actions — the design introduces no new
architectural violations and stays within hexagonal boundaries. The API-006 stakeholder confirmation for the
additive `traceresponse` header has been obtained (recorded in the PR and ADR 20260703-120100); the header is additionally
opt-out via `request_context.trace.response_enabled`.

## Project Structure

### Documentation (this feature)

```text
specs/20260629-120000-request-security-context/
├── plan.md              # This file
├── research.md          # Phase 0 decisions (D1–D10)
├── data-model.md        # SecurityContext / Actor / CallingPeer / TraceID value objects
├── quickstart.md        # Validation scenarios V1–V10
├── contracts/
│   └── request-security-context.md   # Response header, config, ordering, log, gRPC contracts
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── security/
│   │   ├── context.go                  # NEW SecurityContext value object + accessors (final actor + calling_peer)
│   │   └── context_test.go
│   └── tokenexchange/
│       ├── service.go                  # BUILD the final SecurityContext once from captured transport data + validated subject_token/client_assertion
│       └── service_test.go
├── adapters/
│   ├── http/
│   │   ├── server.go                   # EXTEND ServerConfig; reorder NewHandler; register otelchi + SecurityContextMiddleware
│   │   ├── middleware.go               # UPDATE LoggingMiddleware/RecoveryMiddleware to emit via *Context
│   │   ├── enduser/
│   │   │   ├── oauth2_token.go         # FINALIZE delegated token-exchange identity context + pass it through logs / response path
│   │   │   └── oauth2_token_test.go
│   │   ├── middleware/
│   │   │   ├── security_context.go     # NEW SecurityContextMiddleware (capture trace/transport metadata, finalize immediately or via holder, set traceresponse header)
│   │   │   ├── security_context_test.go
│   │   │   ├── clientip.go             # NEW trusted-proxy-aware client IP resolver (right-most XFF)
│   │   │   └── clientip_test.go
│   │   └── routing/
│   │       ├── admin.go                # REMOVE per-router otelchi registration (now in NewHandler)
│   │       └── enduser.go              # REMOVE per-router otelchi registration
│   └── telemetry/
│       ├── context_handler.go          # NEW slog.Handler: stamps trace_id + actor + calling_peer from ctx
│       └── context_handler_test.go
├── app/
│   └── builder.go                      # WRAP base handler with context_handler; pass name/telemetry/request_context into http.ServerConfig
├── ports/
│   └── config.go                       # ADD RequestContextConfig + Config.RequestContext field
├── config/
│   ├── loader.go                       # defaults for request_context
│   └── validator.go                    # validation for request_context
└── extproc/
    └── server/
        ├── server.go                   # per-request child logger (trace_id, actor, optional calling_peer)
        └── server_test.go

charts/agentic-identity-broker/
├── values.yaml                         # request_context params
├── templates/...                       # ConfigMap/env wiring
└── README.md                           # config reference table

examples/config/
├── request-context.yaml                # NEW example
└── README.md                           # reference the new example

docs/configuration.md                   # document request_context

adrs/20260703-120100-request-security-context-propagation.md   # NEW (Proposed)
ARCHITECTURE.md                          # glossary + middleware ordering + traceresponse header

tests/e2e/
├── request_security_context_test.go    # NEW HTTP perimeter E2E (V1–V9)
└── extproc/
    └── request_trace_context_test.go   # NEW gRPC perimeter parity E2E (V10)
```

**Structure Decision**: Single Go backend monorepo (hexagonal). The new domain value object lives in `internal/domain/security`; generic transport capture stays in the HTTP middleware, while token-exchange-specific actor/calling-peer resolution happens where the broker already validates `subject_token` and `client_assertion` (`internal/domain/tokenexchange/service.go`). Configuration flows through `internal/ports/config.go` + `internal/config`. The ExtProc parity change remains confined to `internal/extproc/server` (no core imports, per ADR 20260223-120000), sharing the same value-object semantics by behavior rather than by package reuse.
## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring — relocate `otelchi` from routing into `NewHandler` (no behavior change) | **Included** |
| **Phase 1** | Setup — none beyond existing deps (all libs already in `go.mod`) | Minimal |
| **Phase 2** | Design Preconditions (domain model, config, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | **Skipped** — no new persisted entities/CRUD |
| **Phase 2.5** | Foundational Infrastructure — `security` pkg, context slog handler, config plumbing | Included |
| **Phase 3+** | User Stories — middleware capture (US1/US2), degradation (US3), gRPC parity (US4) | Included |
| **Phase N** | Constitution Compliance verification (ADR, glossary, Helm, docs, `just check`) | **MANDATORY** |

- [x] Phase 0 (refactoring): **include** — moving `otelchi` registration from `SetupAdminRoutes`/
  `SetupEnduserRoutes` into `NewHandler` is a structural change with no behavior change (spans still created
  identically); isolating it lets reviewers approve the reordering before feature logic lands, and is a
  prerequisite for the security middleware reading the span trace ID.
- [x] Phase 2.7 (entity boilerplate): **skip** — the feature adds a value object and middleware, not a
  persisted entity with CRUD handlers/repositories.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/request_security_context_test.go` (HTTP); `tests/e2e/extproc/request_trace_context_test.go` (gRPC)

**Framework**: Ginkgo/Gomega following `tests/e2e/README.md`; ExtProc suite under `tests/e2e/extproc/`.

**Test Organization**: `Describe("Request Security Context")` → `Context` (perimeter / auth state / proxy config / token-exchange identity state) → `It` (one per acceptance scenario).

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1 S1 capture at perimeter | `request_security_context_test.go` | `It("returns traceresponse and logs trace_id+actor for an authenticated request")` |
| US1 S2 downstream service propagation | `request_security_context_test.go` | `It("exposes the same SecurityContext to a downstream service")` |
| US1 S3 repository propagation | `request_security_context_test.go` | `It("exposes the same SecurityContext to the repository layer")` |
| US1 S4 implicit for new endpoints | `request_security_context_test.go` | `It("populates context without per-endpoint capture code")` |
| US1 S5 distinct calling peer | `request_security_context_test.go` | `It("records actor and calling_peer separately for delegated token exchange")` |
| US2 S1 reuse inbound traceparent | `request_security_context_test.go` | `It("reuses an inbound traceparent as the trace id")` |
| US2 S2 generate when absent | `request_security_context_test.go` | `It("generates a unique trace id when none supplied")` |
| US2 S3 trace_id on every log line | `request_security_context_test.go` | `It("stamps trace_id on every log entry for the request")` |
| US2 S4 trace id returned to caller | `request_security_context_test.go` | `It("returns the trace id in the response header")` |
| US2 S5 concurrency isolation | `request_security_context_test.go` | `It("keeps trace ids isolated across concurrent requests")` |
| US3 S1 unauthenticated → anonymous | `request_security_context_test.go` | `It("records actor=anonymous for unauthenticated public requests")` |
| US3 S2 missing metadata not rejected | `request_security_context_test.go` | `It("degrades gracefully without rejecting the request")` |
| US3 S3 protected route still rejected | `request_security_context_test.go` | `It("still returns 401 on protected routes for unauthenticated requests")` |
| US3 S4 trace id under degradation | `request_security_context_test.go` | `It("still emits a trace id for degraded requests")` |
| Edge: spoofed XFF (proxy off) | `request_security_context_test.go` | `It("ignores forged X-Forwarded-For when proxy trust is disabled")` |
| Edge: trusted proxy (proxy on) | `request_security_context_test.go` | `It("uses the right-most forwarded entry when proxy trust is enabled")` |
| US4 S1 gRPC reuse inbound trace | `extproc/request_trace_context_test.go` | `It("reuses an inbound traceparent as the trace id at the gRPC perimeter")` |
| US4 S2 gRPC generate when absent | `extproc/request_trace_context_test.go` | `It("generates a trace id at the gRPC perimeter when none supplied")` |
| US4 S3 gRPC context in logs | `extproc/request_trace_context_test.go` | `It("records actor=anonymous and omits calling_peer consistently across the OPA-disabled and OPA-enabled paths")` |

**Red Phase Requirements**: Assertions target concrete output — `Expect(resp.Header.Get("traceresponse")).To(MatchRegexp("^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$"))`, `Expect(logLine).To(HaveKeyWithValue("actor", "anonymous"))`, `Expect(logLine).To(HaveKeyWithValue("calling_peer", "privileged-client-1"))` for delegated flows, `Expect(resp.StatusCode).To(Equal(401))`, `Expect(recordedClientIP).To(Equal("10.0.0.5"))`. No placeholder always-fail assertions; no `XIt`/`Skip`; no red-phase comments.

**Test Data Strategy**: Reuse `tests/e2e/fixtures/` (principals, agents, config). New fixtures: a trusted-proxy-enabled config variant, a log-capture sink (`slog` handler writing to a buffer), and a delegated token-exchange request fixture containing a validated `subject_token` / `client_assertion` pair with distinct identities. A test seam records the `SecurityContext` observed at the repository boundary for V3 — a grey-box assertion on that injected observer (the context value is invisible at the HTTP boundary), not black-box output; US1 S2/S3 are grey-box by necessity.

**Test Execution Flow**: Write E2E (red) → verify semantic failure → implement → green → minimal fixture-only changes.

**Bootstrap Strategy**: Production bootstrap via `tests/e2e/bootstrap/` (`app.Builder`, dual servers, routing). Fresh server + in-memory storage per test (BeforeEach/AfterEach). gRPC tests use the existing `tests/e2e/extproc/bootstrap` harness.

**Helper Utilities**: New helper to drain the buffered log sink and parse JSON records; reuse existing HTTP helpers in `tests/e2e/helpers/`. Add token-exchange fixture helpers that mint signed `subject_token` / `client_assertion` pairs for delegated-request scenarios.

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/security/context_test.go` — `WithSecurityContext`/`FromContext` round-trip, immutability, absent-context returns `(_, false)`, `calling_peer` omitted when empty.
- `internal/adapters/http/middleware/security_context_test.go` — builds context with actor/anonymous, sets or omits `calling_peer`, sets the `traceresponse` header, never rejects, excludes query string.
- `internal/domain/tokenexchange/service_test.go` — validated subject-token principal remains actor and `client_assertion.sub` becomes `calling_peer` when distinct.
- `internal/adapters/http/enduser/oauth2_token_test.go` — token-exchange handler logs via request context and preserves `trace_id` / `actor` / `calling_peer`.
- `internal/adapters/http/middleware/clientip_test.go` — table-driven: proxy off uses `RemoteAddr`; proxy on takes right-most XFF entry; forged left-most ignored; malformed values → safe default.
- `internal/adapters/telemetry/context_handler_test.go` — stamps `trace_id`, `actor`, and `calling_peer` from `SecurityContext`; no-op when absent.
- `internal/config/...` — defaults + validation for `request_context`.
- `internal/extproc/server/server_test.go` — per-request logger carries propagated/generated `trace_id` and actor, adding `calling_peer` only when available.

**Integration Tests**: N/A for persistence (no DB). HTTP integration covered by E2E.

**Test Coverage Goals**:
- Unit: critical paths (capture, identity-source precedence, IP resolution, handler enrichment, config) fully covered, table-driven for IP + degradation.
- E2E: 100% of acceptance scenarios (mandatory per Principle XIII).
## Complexity Tracking

> No Constitution Check violations. Section intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
