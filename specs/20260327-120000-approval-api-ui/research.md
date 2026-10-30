# Research: Tool Approval API & UI

**Feature Branch**: `20260327-120000-approval-api-ui`
**Date**: 2026-03-28

## R1: PostgreSQL LISTEN/NOTIFY for Cross-Instance Long-Poll Wake-Up

**Decision**: Use pgx v5 direct connection (separate from sqlx pool) for LISTEN/NOTIFY. A dedicated goroutine per broker instance subscribes to the `approval_sync` channel and wakes local long-poll connections via `sync.Cond` or channel broadcast.

**Rationale**: The project uses pgx v5 as the database driver (via sqlx compatibility layer). pgx supports LISTEN/NOTIFY natively via `pgx.Conn.WaitForNotification()`, but this requires a raw pgx connection — not a sqlx-wrapped one. The LISTEN connection is long-lived and must be separate from the query connection pool to avoid blocking other operations.

**Alternatives considered**:

- **Redis Pub/Sub**: Adds an infrastructure dependency not present in the project. PostgreSQL is already a hard dependency, so LISTEN/NOTIFY avoids adding another data store.
- **Polling from ExtProc**: Would require ExtProc to poll the broker at frequent intervals, generating O(N×polling_rate) requests per second. Unacceptable at 100+ concurrent gateway connections.
- **HTTP SSE (Server-Sent Events)**: Would work for push notifications but requires ExtProc to maintain SSE connections. Long-poll is simpler for the client (standard HTTP with conditional headers) and fits ExtProc's request-response model.

**Implementation pattern**:

```go
// Dedicated LISTEN goroutine (one per broker instance)
func (s *ApprovalSyncSubscriber) Listen(ctx context.Context) error {
    conn, err := pgx.Connect(ctx, s.connString)
    // ...
    _, err = conn.Exec(ctx, "LISTEN approval_sync")
    for {
        notification, err := conn.WaitForNotification(ctx)
        // Wake all waiting long-poll goroutines
        s.wakeAll()
    }
}
```

**Key constraint**: The LISTEN connection must be separate from the sqlx connection pool. The PostgreSQL adapter will need a `pgx.ConnConfig` in addition to the existing `sqlx.DB` handle.

---

## R2: Long-Poll HTTP Pattern with chi v5

**Decision**: Implement long-poll as a standard chi handler that blocks on a `select{}` between a change channel, timeout timer, and request context cancellation. No new middleware or streaming infrastructure needed.

**Rationale**: Long-poll is a simple blocking HTTP pattern — the response is held open until data changes or a timeout expires. chi v5 handlers receive standard `http.ResponseWriter` and `*http.Request`, which support blocking via `r.Context()`. This fits naturally into the existing handler pattern without introducing WebSockets or SSE.

**Alternatives considered**:

- **WebSocket**: Adds client-side complexity in ExtProc (maintain WebSocket connection, handle reconnection). Long-poll with `If-None-Match` is simpler and more resilient.
- **SSE (Server-Sent Events)**: Similar complexity to WebSocket with the added issue that chi doesn't have native SSE support. Long-poll is more standard for this use case.
- **Polling**: Simple but wasteful. At 100 gateway connections polling every second, that's 100 req/s of pure overhead. Long-poll eliminates this.

**Handler pattern**:

```go
func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    etag := r.Header.Get("If-None-Match")
    timeout := parseLongPollTimeout(r.Header.Get("X-Long-Poll-Timeout"))
    
    currentVersion, data := h.service.GetCurrentState(r.Context(), filter)
    currentETag := formatETag(currentVersion)
    
    if etag == "" || etag != currentETag {
        // Return immediately with current state
        w.Header().Set("ETag", currentETag)
        writeJSON(w, 200, data)
        return
    }
    
    // Block until change or timeout
    timer := time.NewTimer(timeout)
    defer timer.Stop()
    ch := h.service.Subscribe()
    defer h.service.Unsubscribe(ch)
    
    select {
    case <-ch:
        // Data changed — fetch and return
    case <-timer.C:
        w.WriteHeader(304) // Not Modified
    case <-r.Context().Done():
        // Client disconnected
    }
}
```

---

## R3: ETag Generation from Version Counter

**Decision**: Use the `approval_sync_state.version` bigint directly as the ETag value, formatted as a quoted decimal string (e.g., `"42"`). No hashing needed.

**Rationale**: The version counter is already a unique, monotonically increasing value maintained by the database. It perfectly satisfies ETag semantics: if two responses have the same version, they have the same content. Using the version directly (rather than hashing) is simpler, debuggable, and consistent with RFC 7232.

**Alternatives considered**:

- **SHA-256 hash of response body**: Expensive to compute on every request and unnecessary when a version counter already exists.
- **Timestamp-based**: Timestamps can collide in high-throughput scenarios. The version counter is strictly monotonic.

**Format**: `ETag: "v42"` (prefixed with `v` for clarity, quoted per RFC 7232).

---

## R4: Rate Limiting for Approval Creation

**Decision**: Implement an in-memory per-pair rate limiter using `golang.org/x/time/rate` (token bucket). The rate limiter is keyed by `(principal, agent_id)` pair. Combined with a database count query for max pending check.

**Rationale**: The spec requires two limits: (1) max pending approvals per pair (default 50) and (2) max creation rate per pair (default 10/min). The first is a database query; the second is a classic token bucket. `golang.org/x/time/rate` is the standard Go library for this pattern and is already used widely in Go infrastructure.

**Alternatives considered**:

- **Redis-based rate limiting**: Adds infrastructure dependency. Given the rate limits are per-instance (approval creation is idempotent via unique index), in-memory is sufficient.
- **Database-only counting**: Would work for the max-pending check but is too slow for per-second rate limiting (requires a query per request).
- **Existing `sony/gobreaker`**: Circuit breaker, not a rate limiter. Different pattern.

**Implementation**:

```go
type ApprovalRateLimiter struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter // key: "principal|agent_id"
    maxPending int
    ratePerMin int
}
```

**Note**: `golang.org/x/time/rate` is not currently in go.mod — will need to be added.

---

## R5: Deterministic Arguments Hash Computation

**Decision**: Canonicalize arguments JSON via `json.Marshal()` on Go's `map[string]interface{}`, then SHA-256 hash. Store as hex string.

**Rationale**: Go's `encoding/json.Marshal` produces deterministic output for the same input (sorted map keys). Since arguments arrive as JSON from the API, they are unmarshaled into `map[string]interface{}`, then re-marshaled for canonical form before hashing. This ensures consistent hashing regardless of input key order.

**Alternatives considered**:

- **Raw string hashing**: Would produce different hashes for semantically identical JSON (`{"a":1,"b":2}` vs `{"b":2,"a":1}`). Not acceptable.
- **JCS (JSON Canonicalization Scheme, RFC 8785)**: More rigorous but adds a dependency for a simple need. Go's built-in marshal is sufficient because the broker controls both write and read sides.
- **PostgreSQL-side hashing**: Would require computing the hash in SQL. Keeping it in Go makes the logic testable and portable to the in-memory adapter.

**Implementation**:

```go
func ComputeArgumentsHash(arguments map[string]interface{}) string {
    canonical, _ := json.Marshal(arguments) // Go sorts map keys
    hash := sha256.Sum256(canonical)
    return hex.EncodeToString(hash[:])
}
```

---

## R6: OpenTelemetry Span Linking to External Trace

**Decision**: The broker reads W3C `traceparent` from the approval-create HTTP request header, persists it with the approval, and links later lifecycle spans to that remote span context.

**Rationale**: `traceparent` is propagation metadata, not approval business data. ExtProc's `otelhttp` client transport already injects it on outbound broker calls. Persisting the validated header keeps later browser-driven approval operations causally linked after the original request ends.

**Implementation**:

```go
ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(r.Header))
spanContext := trace.SpanContextFromContext(ctx)
carrier := propagation.HeaderCarrier{}
propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(context.Background(), spanContext), carrier)
```

---

## R7: Approval Configuration Integration

**Decision**: Add `ApprovalsConfig` struct to the config schema, mapped to `approvals:` YAML key. Expose through the existing `ConfigPort` interface as a field on the `Config` struct.

**Rationale**: Follows Constitution Principle VII — all config through unified system. The existing pattern adds nested structs to `Config` and maps them via Viper YAML paths. No custom config loading.

**Implementation in config schema**:

```go
type ApprovalsConfig struct {
    PendingTTL          time.Duration `mapstructure:"pending_ttl"`
    SyncCoalesceWindow  time.Duration `mapstructure:"sync_coalesce_window"`
    RateLimit           ApprovalRateLimitConfig `mapstructure:"rate_limit"`
}

type ApprovalRateLimitConfig struct {
    MaxPendingPerPair    int `mapstructure:"max_pending_per_pair"`
    MaxRequestsPerMinute int `mapstructure:"max_requests_per_minute"`
}
```

---

## R8: Authentication Strategy for Approval Endpoints

**Decision**: Three authentication paths matching the spec:

| Endpoint | Auth Mechanism | Existing Infrastructure |
|----------|---------------|------------------------|
| `POST /api/approvals` | Subject token (Bearer) + client assertion (CEL) | Reuse token exchange middleware |
| `GET /api/approvals/{id}`, `POST .../consume` | Subject token (Bearer) only | Reuse JWT pre-auth middleware |
| `POST .../approve`, `POST .../deny` | `X-Remote-User` header (browser proxy) | Reuse existing `RequirePrincipalMiddleware` |
| `GET /api/approvals` (sync) | Client assertion (CEL) only | Reuse CEL evaluator from token exchange |

**Rationale**: All authentication mechanisms already exist in the codebase. The approval endpoints combine them in new ways but don't require new auth infrastructure.

**Key observation**: `POST /api/approvals` requires **dual auth** (both subject token and client assertion). This is new — existing endpoints use one or the other. Will need a combined middleware or handler-level verification.
