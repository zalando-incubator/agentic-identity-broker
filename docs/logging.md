# Structured Logging Guide (Proposed)

Status: **Proposal**. This guide collects the logging rules that exist today in
`ARCHITECTURE.md`, ADR 011, ADR 027, ADR 033, `internal/extproc/AGENTS.md`, and
`docs/reference/token-exchange.md`. It adds conventions taken from the code that
follows those rules best. Read [Current state](#current-state-survey) for the survey
behind each rule.

## 1. Plumbing

| Rule | Why |
|---|---|
| Use `log/slog` only. Do not use `log`, `fmt.Print*`, or a third-party logger in `internal/`. | One handler chain: stdout (text/JSON) → OTel bridge → `ContextHandler`. |
| Inject `*slog.Logger` through the builder (`internal/app/builder.go`) or constructor. Domain services can take a logger. | Constitution XII. The builder logger wraps the `telemetry.NewContextHandler`. |
| Do not fall back to `slog.Default()` in new code. Require a logger, and use a discard logger in tests. | A fallback hides wiring mistakes. It also skips any handler the builder adds later. |
| **Pass the context.** Use `InfoContext(ctx, …)`, `LogAttrs(ctx, …)`, or `Log(ctx, …)` whenever a `ctx` is in scope. | `ContextHandler` adds `trace_id`, `actor`, and `calling_peer` only from the context (ADR 033, SR-004). A call without ctx gives an uncorrelated record. |
| Do not add `trace_id`, `actor`, or `calling_peer` by hand. | The handler owns these fields. Duplicates can disagree. |
| In ExtProc, use `loggerFromContext(ctx, s.logger)` and `logDiagnostic` for exchange outcomes. | Keeps log semantics the same as the broker without importing broker domain packages (ADR 027). |
| Configure only through `log.level` and `log.format` (`ports.LogConfig`). | Constitution VII. |

## 2. Record shape

```go
s.logger.InfoContext(ctx, "oauth2 session refreshed",
    "event", "session.oauth2.token_refreshed",
    "principal", session.Principal,
    "session_id", session.ID,
    "service_id", session.ServiceID,
    "trigger", trigger,
)
```

### 2.1 Message (`msg`)

- Write a short, lowercase, human-readable phrase with no trailing period. Examples: `"grant revoked"` and `"cimd document fetch failed"`.
- Keep it a constant string. Do not use `fmt.Sprintf` or put values in the message. Values go in attributes, so the message stays groupable and untrusted input cannot change it.
- Do not use PascalCase event names (`TokenIssued`) or snake_case identifiers (`provider_created`) as the message. Put them in `event`.

### 2.2 Event identifier (`event`)

Each audit, security, or lifecycle record MUST carry `event`, a stable machine identifier:

```
<bounded-context>.<subject>.<past_tense_verb>
session.oauth2.token_refreshed     (existing, the reference pattern)
service.thirdparty.provider_created (existing)
consent.grant.revoked              (proposed for today's action=grant_revoked)
oauth2_server.token.issued         (proposed for today's event=TokenIssued)
```

- Use one key, `event`. Do not use `action` or a second discriminator.
- An event name is a contract with dashboards and alerts. To rename one, treat it as a breaking change and note it in the changelog.
- Diagnostic (non-audit) records can omit `event`.

### 2.3 Attribute keys

- Use `snake_case`. A namespaced family uses dots plus snake_case, for example `token_exchange.failure_stage`.
- Prefer typed constructors (`slog.String`, `slog.Int`, …) or `LogAttrs` on hot paths. Alternating key/value pairs are acceptable elsewhere.
- Reuse these canonical keys. Do not invent synonyms.

| Concept | Key | Notes |
|---|---|---|
| Error | `error` | Never `err` or `cause`. See §4. |
| Error classification | `error_kind` | Enumerated value. |
| Entity IDs | `<entity>_id` | `agent_id`, `service_id`, `grant_id`, `session_id`, `approval_id`, `kid`. Use `.String()` on typed IDs. |
| Authenticated user | `principal` | The owner of the affected resource. Request `actor` is added automatically. |
| Operator | `operator_principal` | Admin-initiated operations, such as a sweep. |
| OAuth client | `client_id` | Public identifier. |
| Outcome | `outcome` | Enumerated value. Do not use free text. |
| Short reason | `reason` | Enumerated snake_case value. Do not use a sentence. |
| Timestamps | `<name>_at` | UTC RFC3339Nano strings, as in grant audits. |
| Durations | `duration_ms` | Integer milliseconds. |
| Counts | `<noun>_count` | `key_count`, `row_count`. |
| Operation | `operation` | Enumerated value, such as `encrypt` or `decrypt`. |

- **Bounded values:** Attribute values that dashboards group by MUST come from a closed set. Use typed string constants, such as `token_exchange.*`, `Outcome`, and `ErrorKind`.
- Do not use a logged attribute as a metric label unless it is bounded. `docs/reference/token-exchange.md` lists the bounded set for exchange diagnostics.

## 3. Levels

| Level | Use for | Examples |
|---|---|---|
| `Debug` | Per-request success detail, cache hits, and internal decisions. Off in production. | ExtProc `"token exchanged successfully"`, `cimd_document_cache_hit` |
| `Info` | Committed state changes, audit and security events, and process lifecycle | grant created or revoked, session established, token issued, server start/stop |
| `Warn` | Rejected or suspicious input (4xx), denied access, or a degraded dependency that the broker survives | PKCE or state validation failure, SSRF block, invalid CIMD document, retry |
| `Error` | A server-side failure that needs an operator: 5xx, a broken invariant, configuration, or infrastructure | encryption failure, repository unavailable, sweep aborted |

- A client error is never `Error`. Fail-closed denial is `Warn`, or `Info` when denial is the expected business outcome.
- Log a failure once, at the boundary that handles it. When a lower layer returns the error, it does not log it too.

## 4. Errors

- Log errors as `"error", err`. Use `err.Error()` only when the type would otherwise render as `{}` in JSON.
- **Wrapped causes can contain sensitive data.** For example, `*url.Error` includes the full URL and query, and a provider error can echo a response body. At credential boundaries such as token exchange, session refresh, and provider calls, log the **typed metadata** and not the cause:

  ```go
  // oauth2session.ErrorMetadata implements slog.LogValuer
  s.logger.WarnContext(ctx, "session refresh failed", "event", "session.oauth2.refresh_failed", "failure", opErr.Metadata())
  ```

- Only allowlisted OAuth error codes can reach logs (`IsSafeOAuthErrorCode`). Never log a provider `error_description`.

## 5. What is permissible to log

These classes are **not** permitted under any log level, including `Debug`:

### 5.1 Never log

| Class | Examples |
|---|---|
| Bearer credentials | Access, refresh, and ID tokens. ExtProc `subject_token`. Exchanged tokens. `client_assertion` JWTs. `Authorization` header. |
| OAuth2 flow secrets | Authorization `code`, `code_verifier`, the `state` value, `nonce`, and device codes |
| Client credentials | `client_secret` in plaintext **or ciphertext**. Credential-file contents. |
| Key material | Private JWK parameters, KEK, DEK, branch-key material, and `encryption.memory.raw_key` |
| Encrypted payloads | Ciphertext, JWE-sealed `AuthorizationSessionClaims`, and encrypted session tokens |
| Session state | Cookies and `Set-Cookie` |
| Raw payloads | Request or response bodies, upstream error bodies, OPA input documents, and MCP tool `arguments` |
| Full URLs with query strings or fragments | Authorization redirects, callback URLs, and URLs from a `*url.Error` |
| Connection strings with credentials | Use `ConnectionParameters.Redacted()`. |
| Rejected ExtProc input | The rejected `resource_uri` (`internal/extproc/AGENTS.md`) |
| End-user profile data | Email, name, and claims beyond the identifier, from IdP tokens or the user-info endpoint |

### 5.2 Log only in derived or bounded form

| Data | Permitted form |
|---|---|
| URLs (CIMD `client_id`, `redirect_uri`, resource URI) | Use scheme + host + path. Remove the query and fragment. Prefer the registered ID (`service_id`, `agent_id`) when one exists (`token_exchange.service.id`, not the requested URI). |
| Errors from external systems | Use typed metadata (`LogValuer`), an enumerated `error_kind`, or an allowlisted code. See §4. |
| User-Agent | `security.TruncateUserAgent`, or the `SecurityContext.UserAgent` that is already truncated |
| Request path, method, and client IP | Use `sanitizeLogValue`. Values come from `SecurityContext`, which trusts forwarded headers only in trusted-proxy mode. |
| Caller-supplied headers (`X-Request-ID`) | Log only after you validate the format and bound the length. Otherwise generate a value. |
| JWTs | Metadata only: `kid`, `alg`, `iss`, `aud`, `jti`, `exp`, and the validation failure reason. Never the compact serialization. |
| Inline JWK Sets | `kid` list or diff (`DiffKeyIDs()`), not the set |
| Scopes and tool names | Permitted. They are bounded by configuration, so do not echo unvalidated input. |

### 5.3 Permitted

- Internal typed IDs (`AgentID`, `ServiceID`, `GrantID`, …), canonical IDs, `kid`, `client_id`, and `grant_type`
- Enumerated outcomes, stages, details, reasons, and error kinds
- `principal`, `actor`, and `calling_peer` identifiers **for audit and security purposes**. A principal is a user identifier, which is personal data. Log it on audit, security, and per-owner events, not on every debug line.
- Operator-defined labels (service `display_name`, permission-set names)
- Counts, durations, sizes, HTTP status codes, and timestamps

### 5.4 Make types safe by construction

- A value object that holds sensitive data MUST implement `slog.LogValuer`, and preferably `fmt.Stringer` and `fmt.GoStringer` too. Each returns a redacted form. Then a careless `"service", entity` cannot leak.
  - **Known gap:** `model.Secret` has `Redacted()`, but no `LogValue`, `String`, or `GoString`. `slog.NewTextHandler` (the default `log.format`) renders `"secret", s` as `{plaintext:hunter2 ciphertext:[] absent:false}`.
- Do not pass whole entities or request structs to `slog.Any`. Log the specific fields you need, or use a purpose-limited snapshot, as grant lifecycle audits do.

## 6. Untrusted input

- Untrusted values go into attributes only, never into `msg`.
- The access and recovery middleware escape line breaks (`sanitizeLogValue`). Other adapters that log raw header or path values MUST do the same, and bound the length.
- Prefer classification over echo. Log `reason=invalid_redirect_uri` instead of the bad value. Echo the value only when an operator cannot diagnose the problem without it, and then only in derived form (§5.2).

## 7. Audit events

Audit records are structured operational logs, not a durable ledger (`ARCHITECTURE.md`, "Grant and Definition Lifecycle Audit").

- Emit the record **after** the mutation commits. Unchanged or absent operations do not emit success.
- Required fields: `event`, the affected resource IDs, and `principal` or `operator_principal`. The context supplies `trace_id`, `actor`, and `calling_peer`, so the call MUST use a `*Context` method.
- Prefix pre-write observations with `previous_observed_`.
- Constitution I requires an audit record for every security-critical operation (create, update, or delete of credentials, grants, and keys; token issuance; authentication or authorization failures).

## 8. Testing

- Assert security-relevant logs through a `slog.NewJSONHandler(&buf, …)` logger injected through the production constructor. Do not add test-only APIs.
- Each code path that handles a credential needs a **negative assertion**: the buffer does not contain the token, secret, or code under test. Follow `internal/extproc/server/exchange_safety_test.go` and `internal/extproc/authorization/authorizer_safety_test.go`.
- Assert `event` and the key attributes, not message text.

## 9. Review checklist

- [ ] Uses `*Context` with the request `ctx`
- [ ] Constant lowercase message. `event` is set for audit and security records.
- [ ] Keys are snake_case and canonical (§2.3). The error key is `error`.
- [ ] Level matches §3. The failure is logged once.
- [ ] No §5.1 data. §5.2 data is in derived form. No whole structs.
- [ ] New sensitive types implement `LogValuer`.
- [ ] A test asserts that the credentials in this path are absent from logs.

## Current state (survey)

This survey sampled production Go under `internal/` and `cmd/` (about 570 log calls in 77 files), on 2026-10-31.

**Consistent and worth keeping**

- `log/slog` is used everywhere. `log` appears only in tooling and tests.
- Wiring follows ADR 011, 027, and 033: stdout handler → optional `otelslog` multi-handler → `ContextHandler`, which adds `trace_id`, `actor`, and `calling_peer`.
- Attribute keys are snake_case throughout. There are no camelCase keys. `error` (128 uses) is the dominant error key, with `err` in 5 stragglers.
- Good examples include ExtProc `logDiagnostic` (bounded `token_exchange.*` attributes and `LogAttrs`), `oauth2session.ErrorMetadata.LogValue()` (typed, credential-free error metadata), the `session.oauth2.*` `event` family, and the access/recovery middleware with `sanitizeLogValue`.
- No message uses `fmt.Sprintf`.

**Inconsistent or risky**

| Finding | Where |
|---|---|
| About 480 calls do not pass ctx, so they lose `trace_id` and `actor`. Many are inside request-scoped methods. | e.g. `internal/domain/oauth2/service.go:208`, `internal/domain/thirdparty/service.go:516`, `internal/domain/oauth2server/strategies.go:120` |
| `Secret` renders plaintext in text-format logs. | `internal/domain/model/secret.go` (no `LogValue` or `String`) |
| Four message styles: lowercase sentences (most common), Capitalized sentences (`"OAuth2 Authorization Request"`), snake_case identifiers (`provider_created`, `cimd_document_invalid`), and PascalCase events (`TokenIssued`, `AgentIDClaimMissing`) | `middleware/oauth2_audit.go`, `domain/thirdparty/service.go`, `domain/oauth2/cimd/service.go`, `enduser/token_grant_strategy.go` |
| Two event discriminators: `event` (`session.oauth2.*`, `TokenIssued`) and `action` (`grant_revoked`) | `domain/oauth2session/*`, `domain/consent/service.go:656` |
| `OAuth2AuditMiddleware` logs `path` without `sanitizeLogValue`. It also logs a caller-supplied `X-Request-ID` without validation or length limits. Its generated fallback ID (`string(rune(n%10000))`) is not a useful identifier. | `internal/adapters/http/middleware/oauth2_audit.go` |
| Full CIMD `client_id` URLs are logged, including the query string, which the code itself warns about. `redirect_uri` is also logged in full. | `domain/oauth2/cimd/service.go:53-101`, `domain/oauth2/service.go:208` |
| 25 constructors fall back silently to `slog.Default()`. | e.g. `domain/agents/service.go:46` |
| Whole wrapped errors are logged at provider and CIMD boundaries. A `*url.Error` cause includes the full URL. | `domain/oauth2/cimd/service.go:60-85` |
| `AgentIDClaimMissing` is logged for a response-read failure that is not a missing claim. | `adapters/http/enduser/token_grant_strategy.go:92` |

**Proposed follow-ups** (each a small, separate PR)

1. Add `LogValue`, `String`, and `GoString` to `model.Secret`, with a test that asserts redaction in both handlers.
2. Sanitize and bound `path` and `X-Request-ID` in `OAuth2AuditMiddleware`, and replace the fallback ID generator.
3. Convert request-scoped calls to `*Context` methods. A `sloglint` rule (`context: scope`) in `.golangci` can then enforce it.
4. Normalize messages and event names (`TokenIssued` and `action=grant_revoked` → `event`). Add `sloglint` `key-naming-case: snake`, `static-msg`, and `no-raw-keys` if the team adopts a key-constant registry.
5. Remove query strings from logged URLs through a shared helper.
6. Link this guide from `AGENTS.md` and `internal/AGENTS.md`. Then decide whether its normative parts need an ADR (Constitution II).
