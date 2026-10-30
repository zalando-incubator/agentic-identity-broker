# Research: OAuth2 Provider Flavor Support

**Feature**: `20260312-120000-oauth2-provider-flavors`
**Date**: 2026-03-12

## Research Questions & Findings

### Q1: What does `golang.org/x/oauth2/google` provide for service account JSON parsing?

**Decision**: Use `google.JWTConfigFromJSON(jsonKey []byte, scope ...string) (*jwt.Config, error)` as the primary parsing and type-validation function.

**Findings**:

The `golang.org/x/oauth2` library is already in `go.mod` at v0.35.0. The `google` subpackage contains `JWTConfigFromJSON` which:

1. Unmarshals `jsonKey` into an internal `credentialsFile` struct — returns error for invalid JSON (**satisfies FR-008**)
2. Validates `credentialsFile.Type == "service_account"` — returns error otherwise (**satisfies FR-007**)
3. Returns `*jwt.Config` with:
   - `Email` ← `client_email` field
   - `PrivateKey` ← `[]byte(private_key)` (raw PEM bytes, no parse, no crypto validation — **satisfies FR-014**)
   - `TokenURL` ← `token_uri` field
   - `PrivateKeyID` ← `private_key_id` field
4. Does **not** expose `client_id` — the internal `credentialsFile` struct carries it but is unexported

**For `client_id` extraction**: A minimal auxiliary unmarshal `{ ClientID string \`json:"client_id"\` }` after the primary parse is sufficient. This is not custom crypto; it's trivial JSON field access.

**Internal source** (go module cache, relevant snippet):
```go
// credentialsFile is the unmarshalled representation of a credentials file.
type credentialsFile struct {
    Type         string `json:"type"`
    ClientEmail  string `json:"client_email"`
    PrivateKey   string `json:"private_key"`
    TokenURL     string `json:"token_uri"`
    ClientID     string `json:"client_id"`
    // ...
}

func JWTConfigFromJSON(jsonKey []byte, scope ...string) (*jwt.Config, error) {
    var f credentialsFile
    if err := json.Unmarshal(jsonKey, &f); err != nil {
        return nil, err
    }
    if f.Type != serviceAccountKey { // "service_account"
        return nil, fmt.Errorf("google: read JWT from JSON credentials: 'type' field is %q (expected %q)", ...)
    }
    return f.jwtConfig(scope, ""), nil
}
```

**Rationale**: Delegates JSON parsing and type checking to a well-tested, battle-tested library. No custom parsing. Aligns with Constitution Principle III (Library-First Security).

**Alternatives considered**:
- Writing a custom JSON struct for validation: Rejected — duplicates code already in the oauth2 library; violates library-first principle.
- Using `google.CredentialsFromJSON`: This function requires a `context` and is designed for runtime token acquisition, not credential validation at configuration time. Returns `*google.Credentials` which wraps `TokenSource` — too heavyweight for our parse-and-store use case.

---

### Q2: Where should `GoogleServiceAccountKey` parsing live in the hexagonal architecture?

**Decision**: In `internal/domain/model/google_service_account.go` as a pure value object.

**Rationale**:
- `domain/` AGENTS.md explicitly allows "declared external libraries (e.g., `jwx`, `oauth2`, `cel-go`) where domain logic requires them"
- `golang.org/x/oauth2` is mentioned by name as allowed in domain packages
- No I/O, no database, no HTTP — the parse function takes a string and returns a struct or error
- Follows pattern of `model/secret.go` (value object with validation) and `model/service_requirement.go`

**Alternatives considered**:
- Putting validation in the handler layer: Rejected — would break the hexagonal principle of domain-driven validation; handlers would contain business logic.
- Putting validation in a dedicated `domain/google/` package: Rejected — over-engineering for a single value object; `model/` is the natural home alongside other value objects.

---

### Q3: How should `issuer_uri` interact with `token_uri` for Google flavor?

**Decision**: If `issuer_uri` is provided, validate `scheme + host` matches the `scheme + host` of `token_uri` in the service account JSON. If omitted, derive `issuer_uri` as `scheme + host` of `token_uri` from JSON.

**From spec clarifications**:
> Optional. If provided, it is validated for consistency against the `token_uri` in the service account JSON (scheme and host must match). If omitted, the `token_uri` from the service account JSON is the authoritative token endpoint.

**Implementation approach**:
```go
// If issuer_uri provided: parse both URLs, compare scheme + host
issuerParsed, _ := url.Parse(issuerURI)
tokenParsed, _ := url.Parse(gsk.TokenURI)
if issuerParsed.Scheme != tokenParsed.Scheme || issuerParsed.Host != tokenParsed.Host {
    // return HTTP 400: inconsistency between issuer_uri and token_uri in credential
}

// If issuer_uri omitted: construct base URL from token_uri
issuerURI = tokenParsed.Scheme + "://" + tokenParsed.Host
```

---

### Q4: What are the endpoint values stored for Google flavor services?

**Decision**: Derive endpoints from the service account JSON; do not require manual endpoint specification from callers.

- `token_endpoint` → `token_uri` from service account JSON (e.g., `https://oauth2.googleapis.com/token`)
- `authorize_endpoint` → Google's standard auth URL: `https://accounts.google.com/o/oauth2/auth` (from `google.Endpoint.AuthURL`)
- These are populated in the handler before entity validation, bypassing the "required when discovery disabled" check for google flavor

**Rationale**: The actual Google service account token flow (JWT Bearer, RFC 7523) is a follow-up feature. For now, we store reasonable defaults that will be used when the flow is implemented. Requiring admins to manually specify these for a well-known provider would be error-prone.

---

### Q5: What is the correct validation order in `ValidateForCreate` / `ValidateForUpdate`?

**Decision**: Introduce flavor-dispatched validation as a private method on `ThirdpartyOAuth2ProviderEntity`. The existing validation methods call it after common checks.

```
ValidateForCreate:
  1. Common checks: display_name, scopes (existing)
  2. Flavor validation: e.Flavor.Validate() — reject unknown flavors
  3. Credential dispatch:
     if standard: validate non-empty string (existing behavior)
     if google:   ParseGoogleServiceAccountKey(credential) — validates all google rules
  4. issuer_uri: for standard, required + HTTPS check (existing)
                 for google, optional; if provided check scheme+host vs token_uri
  5. Endpoints: for standard, required when discovery disabled (existing)
                for google, skip — derived from JSON
```

This preserves all existing `standard` behavior unchanged (SC-003).

---

### Q6: Which fields change in the Postgres record and how?

**Decision**: Add `oauth2_flavor` to `ThirdpartyOAuth2ProviderRecord` with `db:"oauth2_flavor"` tag. Include it in all SELECT, INSERT, and UPDATE statements.

The column type is `VARCHAR(50) NOT NULL DEFAULT 'standard'`. The string value matches the `OAuth2Flavor` constants exactly.

**Migration approach**:
- `007_add_oauth2_flavor.up.sql`: `ALTER TABLE thirdparty_oauth2_services ADD COLUMN oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard'`
- `007_add_oauth2_flavor.down.sql`: `ALTER TABLE thirdparty_oauth2_services DROP COLUMN oauth2_flavor`
- Existing rows get `'standard'` automatically — no data migration needed (SC-003)

---

### Q7: What test fixtures are needed for E2E tests?

**Decision**: Add to `tests/e2e/fixtures/services.go`:
- `GoogleServiceAccountJSON()` — returns a realistic but non-functional Google service account JSON string for testing
- `ValidGoogleServiceRequest()` — returns a `map[string]interface{}` suitable for HTTP POST to create a google-flavor service

The `GoogleServiceAccountJSON()` fixture will contain all required fields (`type`, `client_email`, `private_key`, `token_uri`, `client_id`) with realistic-looking but non-functional values. The private key will be a valid PEM-formatted RSA key structure (syntactically correct but not a real key) to avoid `JWTConfigFromJSON` returning a JSON parse error.

Wait — FR-014 states the system MUST NOT validate the private key's cryptographic format. And `JWTConfigFromJSON` does not validate it either (it stores `[]byte(f.PrivateKey)` directly). So the fixture's `private_key` just needs to be a non-empty string. A dummy PEM block string suffices.

---

### Summary Table

| Question | Decision | Library/Pattern Used |
|---|---|---|
| Google JSON parsing | `google.JWTConfigFromJSON` + auxiliary `client_id` unmarshal | `golang.org/x/oauth2/google` |
| Validation layer | `internal/domain/model/google_service_account.go` | Value object pattern |
| `client_id` extraction | Minimal auxiliary JSON unmarshal struct | Go stdlib `encoding/json` |
| `issuer_uri` for google | Optional; validate scheme+host vs `token_uri` if provided | `net/url` |
| Endpoint derivation | `token_uri` from JSON; Google auth URL default | `golang.org/x/oauth2/google.Endpoint` |
| Postgres column | `oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard'` | Migration 007 |
| Memory adapter | Add `Flavor` field round-trip in `thirdparty_provider.go` | Existing memory pattern |
