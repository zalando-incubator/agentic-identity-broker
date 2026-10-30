# Quickstart: Validating Request Security Context Propagation

**Feature**: 20260629-120000-request-security-context | **Spec**: [spec.md](./spec.md) | **Contracts**: [contracts/request-security-context.md](./contracts/request-security-context.md)

This guide describes how to validate the feature end-to-end. It is a **validation/run guide** — implementation
details live in `tasks.md` and the implementation phase. Each scenario maps to an acceptance scenario in the
spec and to a red-phase E2E test.

## Prerequisites

- Go toolchain (see `go.mod`) and `just` runner.
- No external services required for the HTTP perimeter validation (in-memory storage backend).
- For the gRPC parity scenario: the ExtProc suite harness under `tests/e2e/extproc/`.

## Build & test commands

```bash
just build                                  # build ./bin/agentic-identity-broker
just test                                   # fast unit/package tests (includes new security pkg + middleware)
ginkgo -v ./tests/e2e/ -focus "Request Security Context"   # HTTP perimeter E2E
ginkgo -v ./tests/e2e/extproc/ -focus "trace"              # gRPC perimeter parity E2E
just check                                  # fmt + vet + lint gate
```

## Validation scenarios

> Each scenario lists the spec mapping, the action, and the expected observable outcome. The E2E suite asserts these against a real server booted via `tests/e2e/bootstrap/` (production `app.Builder`).

### V1 — Implicit capture + correlation (US1 S1, US2 S3/S4 · FR-001, FR-006, FR-007)

- **Action**: Send an authenticated request (valid `X-Remote-User`) to any endpoint, e.g. `GET /api/me`.
- **Expect**:
  - Response carries a `traceresponse` header (W3C form `00-<trace-id>-<child-id>-<flags>`, non-empty).
  - The access-log line for the request includes `trace_id` equal to the header's `<trace-id>` field **and** `actor` equal to the authenticated principal.
  - `calling_peer` is absent/empty on this non-delegated request.
  - When an inbound `traceparent` was supplied and distributed tracing is enabled, the `traceresponse` header's `<trace-id>` field matches the inbound trace ID (US2 S1 / SC-007).

### V2 — Distinct calling peer on delegated token exchange (US1 S5 · FR-001, FR-015)

- **Action**: Submit a delegated token-exchange request whose validated `subject_token` principal and validated `client_assertion.sub` are different.
- **Expect**:
  - `actor` is the validated subject identity.
  - `calling_peer` is the validated `client_assertion` subject.
  - The request log record keeps those two identities separate rather than collapsing them into one field.

### V3 — Downstream propagation to service + persistence (US1 S2 service, US1 S3 repository · FR-004)

- **Action**: Exercise an endpoint whose handler calls a domain service that reaches a repository (for delegated coverage, use the token-exchange path or another route that propagates a distinct calling peer).
- **Expect**: The `SecurityContext` retrieved via `security.FromContext` inside the service and inside the repository layer equals the finalized request context (same `trace_id`, `actor`, `calling_peer` when present, `client_ip`). Verified by a test seam that records the context observed at the repository boundary.

### V4 — Trace generated when none supplied (US2 S2 · FR-003)

- **Action**: Send a request **without** any inbound trace header.
- **Expect**: A unique trace id is generated; it appears identically in the `traceresponse` header's `<trace-id>` field and all log lines for that request.

### V5 — Concurrency isolation (US2 S5)

- **Action**: Fire two concurrent requests with distinct inbound trace IDs.
- **Expect**: Each request's logs carry only its own `trace_id`; no cross-request leakage.

### V6 — Graceful degradation to anonymous (US3 S1/S2/S4 · FR-002, FR-008)

- **Action**: Send an **unauthenticated** request to a public endpoint (e.g. `/health` or `/.well-known/oauth-authorization-server`), and a request with missing/garbled metadata.
- **Expect**: Request succeeds (not rejected by capture); `actor` recorded as `anonymous`; `calling_peer` omitted; absent fields use safe defaults; the `traceresponse` header is still present.

### V7 — Access control unchanged (US3 S3 · FR-009, SR-003)

- **Action**: Send an unauthenticated request to a **protected** route (e.g. `/api/consent/agents`).
- **Expect**: Still rejected `401` by `RequirePrincipalMiddleware` (the anonymous fallback does NOT grant access); response still carries the `traceresponse` header.

### V8 — Proxy-trust spoofing guard (Edge case · FR-010, SR-006)

- **Action (default config, `trusted_proxy.enabled: false`)**: Send a request with a forged `X-Forwarded-For: 1.2.3.4`.
- **Expect**: Recorded `client_ip` is the real connection address (`RemoteAddr`), NOT `1.2.3.4`.
- **Action (with `trusted_proxy.enabled: true`)**: Send `X-Forwarded-For: 9.9.9.9, 10.0.0.5` through the trusted-proxy fixture.
- **Expect**: Recorded `client_ip` is the **right-most** trusted entry (`10.0.0.5`), not the spoofable left-most `9.9.9.9`.

### V9 — No secrets in context/logs (FR-012, SR-005)

- **Action**: Send a request carrying credentials (Authorization header, cookies, `code=` query param).
- **Expect**: No token/secret/authorization-code value appears in the `SecurityContext` fields or any log line; `request_target` excludes the query string.

### V10 — gRPC perimeter parity (US4 S1/S2/S3 · FR-014)

- **Action**: Drive a request through the ExtProc gRPC perimeter with, then without, an inbound `traceparent`.
- **Expect**: The service's structured logs for that request include a `trace_id` (propagated when supplied, generated otherwise) and `actor=anonymous`, with `calling_peer` omitted. The direct token-exchange path sees only an opaque bearer token today, so no distinct authenticated peer is available; the field contract leaves room for a future authenticated-peer source to populate `calling_peer` without changing the log shape.

## Success signals

- All E2E `It()` blocks under `-focus "Request Security Context"` and the gRPC `-focus "trace"` scenarios pass.
- `just check` is clean.
- Manual smoke: `curl -i http://localhost:14000/health` shows a `traceresponse` header (W3C form) and a matching
  `trace_id` (its `<trace-id>` field) in the server logs with `actor=anonymous`.
