# Structured Logging Standard

This standard applies to all Go code in `cmd/` and `internal/`. Read it before you add or change a log call.

Scope: log records only. OpenTelemetry traces and metrics are out of scope.

Related binding sources: ADR 011 and ADR 027 (OpenTelemetry), ADR 033 (request security context),
`docs/reference/token-exchange.md#diagnostic-attributes`, and Constitution Principle I.

## 1. Logger Plumbing

| Rule | Reason |
|---|---|
| Use `log/slog` only. Do not use `log`, `fmt.Print*`, or a third-party logger. | One handler chain applies context, principal policy, and export to every record. |
| Inject a `*slog.Logger` through the builder (`internal/app/builder.go`) or a constructor. | Constitution Principle XII. |
| Require a logger in each constructor. Do not fall back to `slog.Default()`. In tests, inject a discard or buffer logger. | A fallback hides wiring errors and skips the builder handler chain. |
| When a `ctx` is in scope, use `DebugContext`, `InfoContext`, `WarnContext`, `ErrorContext`, `LogAttrs`, or `Log`. | `telemetry.ContextHandler` adds `trace_id`, `actor`, and `calling_peer` only from the context (ADR 033, SR-004). |
| Do not add `trace_id`, `actor`, or `calling_peer` manually. | The context handler owns these attributes. |
| In ExtProc, get the logger with `loggerFromContext(ctx, s.logger)`. Log exchange outcomes with `logDiagnostic`. | ExtProc keeps log parity without importing broker domain packages (ADR 027). |
| Configure the logger only through `log.level` and `log.format` (`ports.LogConfig`). | Constitution Principle VII. |

## 2. Record Shape

```go
s.logger.InfoContext(ctx, "oauth2 session refreshed",
    "event", "session.oauth2.token_refreshed",
    "principal", session.Principal, // id.Principal, not .String()
    "session_id", session.ID,
    "service_id", session.ServiceID,
    "trigger", trigger,
)
```

### 2.1 Message

- Write a short lowercase phrase. Do not add a period. Examples: `"grant revoked"`, `"cimd document fetch failed"`.
- Use a constant string. Do not use `fmt.Sprintf`. Put all values in attributes.
- Do not use an event identifier (`TokenIssued`, `provider_created`) as the message. Put it in `event`.

### 2.2 Event Identifier

Each audit, security, or lifecycle record must have an `event` attribute:

```text
<bounded_context>.<subject>.<past_tense_verb>

session.oauth2.token_refreshed
service.thirdparty.provider_created
consent.grant.revoked
oauth2_server.token.issued
```

- Use only the key `event`. Do not use `action` or other discriminator keys.
- An event identifier is a contract for dashboards and alerts. A rename is a breaking change. Record it in the changelog.
- A diagnostic record that is not an audit, security, or lifecycle record can omit `event`.

### 2.3 Attribute Keys

- Use `snake_case`. A namespaced family uses dots between `snake_case` segments, for example `token_exchange.failure_stage`.
- Use typed constructors (`slog.String`, `slog.Int`) or `LogAttrs` on hot paths. Key-value pairs are permitted on other paths.
- Use the canonical keys in this table. Do not make synonyms.

| Concept | Key | Value |
|---|---|---|
| Error | `error` | The `error` value. Do not use `err` or `cause`. Refer to section 4. |
| Error classification | `error_kind` | Enumerated value |
| Entity ID | `<entity>_id` | Typed ID, for example `agent_id`, `service_id`, `grant_id`, `session_id`, `approval_id` |
| Key ID | `kid` | String |
| Resource owner | `principal` | `id.Principal`. Refer to section 5. |
| Operator | `operator_principal` | `id.Principal`. Refer to section 5. |
| OAuth client | `client_id` | Public identifier |
| Outcome | `outcome` | Enumerated value |
| Reason | `reason` | Enumerated `snake_case` value. Do not use a sentence. |
| Point in time | `<name>_at` | UTC RFC 3339 with nanoseconds |
| Duration | `duration_ms` | Integer milliseconds |
| Count | `<noun>_count` | Integer, for example `key_count`, `row_count` |
| Operation | `operation` | Enumerated value, for example `encrypt`, `decrypt` |

- An attribute that a dashboard groups by must have a closed value set. Use typed string constants.

## 3. Levels

| Level | Use | Examples |
|---|---|---|
| `Debug` | Per-request success detail, cache hits, internal decisions. Disabled in production. | Token exchange success in ExtProc, CIMD cache hit |
| `Info` | Committed state change, audit or security event, process lifecycle | Grant created or revoked, session established, token issued, server start and stop |
| `Warn` | Rejected or suspicious input (4xx), access denial, degraded dependency that the process survives | PKCE or state validation failure, SSRF block, invalid CIMD document, retry |
| `Error` | Server-side failure that needs operator action (5xx, broken invariant, configuration, infrastructure) | Encryption failure, repository unavailable, sweep aborted |

- Do not log a client error at `Error`. Log a fail-closed denial at `Warn`. If denial is the expected business outcome, log it at `Info`.
- Log a failure one time, at the layer that handles it. If a layer returns the error, that layer does not log it.

## 4. Errors

- Log an error as `"error", err`.
- A wrapped cause can contain sensitive data. For example, `*url.Error` contains the full URL and query. A provider error can contain a response body.
- At a credential boundary (token exchange, session refresh, provider call, CIMD fetch), log typed metadata, not the cause:

  ```go
  // oauth2session.ErrorMetadata implements slog.LogValuer.
  s.logger.WarnContext(ctx, "session refresh failed",
      "event", "session.oauth2.refresh_failed",
      "failure", opErr.Metadata())
  ```

- Log only allowlisted OAuth error codes (`IsSafeOAuthErrorCode`). Do not log a provider `error_description`.
- Do not put a principal or another section 6 value into an error string. Error strings bypass the principal policy and the redaction rules.

## 5. Principals

A principal identifies a person, for example by email or subject. It is personal data. All principal values in logs go through one central policy. Then a change to redaction or pseudonymisation is one change in one place.

### 5.1 Rules

1. Represent a principal as `id.Principal` (`internal/domain/id`). Do not use `string`.
2. Log the typed value: `"principal", p`. Do not log `p.String()`, `string(p)`, or `slog.String("principal", ...)`.
3. `id.Principal` implements `slog.LogValuer`. Do not log a principal through another type.
4. The principal policy handler in `internal/adapters/telemetry` renders each `id.Principal` attribute. It is the only component that renders a principal. It runs before the fan-out to stdout and OpenTelemetry, so all sinks get the same output.
5. The keys `principal`, `operator_principal`, and `actor` are reserved for principals. The policy handler also applies to a plain string value under a reserved key. This is a safety net, not a substitute for rule 2.
6. `SecurityContext.Actor` is an `id.Principal`. The context handler adds it to records through the same policy.
7. Do not put a principal in a message or an error string.
8. Log a principal only on audit, security, and per-owner records. Do not log a principal on `Debug` records.
9. Do not log `principal.PrincipalProfile` or any of its fields (display name, email, picture URL).
10. ExtProc must not import `internal/domain/id`. ExtProc logs a principal only under a reserved key, so the shared telemetry handler applies the same policy.

### 5.2 Example

```go
// Correct
s.logger.InfoContext(ctx, "grant revoked",
    "event", "consent.grant.revoked",
    "principal", grant.Principal, // id.Principal
    "agent_id", grant.AgentID)

// Incorrect: a plain string bypasses the type-based policy
slog.String("principal", grant.Principal.String())

// Incorrect: a principal in an error string bypasses the policy
fmt.Errorf("grant for %s not found", grant.Principal)
```

## 6. Permitted Data

### 6.1 Never Log

These data classes are prohibited at all levels, including `Debug`.

| Class | Examples |
|---|---|
| Bearer credentials | Access, refresh, and ID tokens. ExtProc `subject_token`. Exchanged tokens. `client_assertion` JWTs. The `Authorization` header. |
| OAuth2 flow secrets | Authorization `code`, `code_verifier`, `state` value, `nonce` |
| Client credentials | `client_secret` as plaintext or ciphertext. Credential file contents. |
| Key material | Private JWK parameters, KEK, DEK, branch key material, `encryption.memory.raw_key` |
| Encrypted payloads | Ciphertext, JWE-sealed `AuthorizationSessionClaims`, encrypted session tokens |
| Session state | `Cookie` and `Set-Cookie` values |
| Raw payloads | Request and response bodies, upstream error bodies, OPA input documents, MCP tool `arguments` |
| URL query and fragment | Authorization redirects, callback URLs, URLs inside a `*url.Error` |
| Connection strings with credentials | Use `ConnectionParameters.Redacted()`. |
| Rejected ExtProc input | The rejected `resource_uri` (`internal/extproc/AGENTS.md`) |
| End-user profile data | Email, name, picture URL, and identity claims other than the principal |

### 6.2 Log Only in Derived Form

| Data | Permitted form |
|---|---|
| Principal | `id.Principal` through the principal policy (section 5) |
| URL (CIMD `client_id`, `redirect_uri`, resource URI) | Scheme, host, and path only. If a registered ID exists, log the ID (`service_id`, `agent_id`), not the URL. |
| Error from an external system | Typed metadata (`slog.LogValuer`), an enumerated `error_kind`, or an allowlisted code (section 4) |
| User-Agent | `SecurityContext.UserAgent` or `security.TruncateUserAgent` |
| Request method, path, client IP | `sanitizeLogValue`. Take values from `SecurityContext`. |
| Caller-supplied header (for example `X-Request-ID`) | Validate the format and limit the length before you log it. If it is not valid, generate a new value. |
| JWT | Header and claim metadata only: `kid`, `alg`, `iss`, `aud`, `jti`, `exp`, validation failure reason |
| JWK Set | `kid` list or diff (`DiffKeyIDs()`), not the set |
| Scope, tool name | Only values that configuration bounds. Do not echo values that are not validated. |

### 6.3 Permitted

- Typed entity IDs, canonical IDs, `kid`, `client_id`, `grant_type`
- Enumerated outcomes, stages, details, reasons, error kinds
- Operator-defined labels, for example service `display_name` and permission set names
- Counts, durations, sizes, HTTP status codes, timestamps

### 6.4 Safe Types

- A value object that contains sensitive data must implement `slog.LogValuer`, `fmt.Stringer`, and `fmt.GoStringer`. Each method returns a redacted form. This applies to `model.Secret`.
- Do not log a full entity or request struct with `slog.Any`. Log the necessary fields, or log a purpose-limited snapshot type.

## 7. Untrusted Input

- Put untrusted values only in attributes. Do not put them in the message.
- Escape line breaks with `sanitizeLogValue` and limit the length of raw header and path values.
- Log a classification, not the value. For example, log `reason=invalid_redirect_uri`, not the rejected URI. If an operator needs the value for diagnosis, log the derived form from section 6.2.

## 8. Audit Records

Audit records are structured operational logs. They are not a durable ledger.

- Write the record after the mutation commits. Do not write a success record for an operation that changed nothing.
- Each audit record contains `event`, the IDs of the affected resources, and `principal` or `operator_principal`.
- Use a `*Context` method. The context supplies `trace_id`, `actor`, and `calling_peer`.
- Use the prefix `previous_observed_` for state that the operation read before the write.
- Write an audit record for each security-critical operation (Constitution Principle I). These include create, update, and delete of credentials, grants, and keys; token issuance; and authentication and authorization failures.

## 9. Tests

- Inject a `slog.NewJSONHandler(&buf, ...)` logger through the production constructor. Do not add test-only APIs.
- For each path that handles a credential, assert that the buffer does not contain the credential under test. Examples: `internal/extproc/server/exchange_safety_test.go`, `internal/extproc/authorization/authorizer_safety_test.go`.
- For each path that logs a principal, assert that the attribute value is the policy output.
- Assert `event` and the key attributes. Do not assert message text.

## 10. Review Checklist

- [ ] The call uses a `*Context` method with the request `ctx`.
- [ ] The message is a constant lowercase phrase. Audit and security records have `event`.
- [ ] Keys are `snake_case` and canonical (section 2.3). The error key is `error`.
- [ ] The level agrees with section 3. The failure is logged one time.
- [ ] Principals are logged as `id.Principal` under a reserved key and only where section 5 permits.
- [ ] The record contains no section 6.1 data. Section 6.2 data is in derived form. No full structs are logged.
- [ ] Each new sensitive type implements `slog.LogValuer`, `fmt.Stringer`, and `fmt.GoStringer`.
- [ ] A test asserts that credentials on this path are not in the logs.
