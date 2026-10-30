# Research: OAuth2 Server Mode Implementation

**Branch**: `20260328-120000-oauth2-server` | **Date**: 2026-03-28

## Decision 1: OAuth2 Library — ory/fosite (headless) vs Custom Implementation

### Decision: **ory/fosite as headless domain library** — use handler layer only, bypass HTTP orchestration

### Context

Two integration approaches for fosite were evaluated:

1. **Full fosite** — use `Fosite.NewAuthorizeRequest(*http.Request)` and `WriteAccessResponse(http.ResponseWriter)` as HTTP-level entry points. **Rejected** — these orchestrators are coupled to `*http.Request` parsing, which conflicts with the project's own chi handlers and hexagonal architecture.

2. **Headless fosite** — bypass the HTTP orchestrators entirely, use only the handler layer (`AuthorizeExplicitGrantHandler`, `ClientCredentialsGrantHandler`, `pkce.Handler`). These handlers operate on clean interfaces (`fosite.AuthorizeRequester`, `fosite.AccessRequester`) with no `*http.Request` dependency. Strategy interfaces (`AccessTokenStrategy`, `AuthorizeCodeStrategy`) are pluggable — we implement them with our own libraries. **Chosen.**

### Architecture: Headless Fosite Integration

```
chi handler (our code)
  → parse HTTP request
  → look up Agent + BrokerClientCredential → wrap in fosite.DefaultClient
  → construct fosite.AuthorizeRequest / AccessRequest (plain structs, no HTTP)
  → call fosite handler methods:
      handler.HandleAuthorizeEndpointRequest(ctx, authorizeReq, authorizeResp)
      handler.HandleTokenEndpointRequest(ctx, accessReq)
      handler.PopulateTokenEndpointResponse(ctx, accessReq, accessResp)
  → extract code/token from fosite response objects
  → write HTTP response (our code)
```

Fosite lives entirely inside `internal/domain/oauth2server/` as an implementation detail behind the existing port interfaces. No fosite types leak into `internal/ports/` or HTTP handlers.

### Handler Layer Verification (Source-Verified)

The handler interfaces take domain-level types, **not** `*http.Request`:

```go
// handler/oauth2/flow_authorize_code_auth.go
func (c *AuthorizeExplicitGrantHandler) HandleAuthorizeEndpointRequest(
    ctx context.Context,
    ar fosite.AuthorizeRequester,    // ← clean interface
    resp fosite.AuthorizeResponder,  // ← clean interface
) error

// handler/oauth2/flow_client_credentials.go
func (c *ClientCredentialsGrantHandler) HandleTokenEndpointRequest(
    ctx context.Context,
    request fosite.AccessRequester,  // ← clean interface
) error

// handler/pkce/handler.go
func (c *Handler) HandleAuthorizeEndpointRequest(ctx context.Context,
    ar fosite.AuthorizeRequester, resp fosite.AuthorizeResponder) error
func (c *Handler) HandleTokenEndpointRequest(ctx context.Context,
    request fosite.AccessRequester) error
```

`fosite.AuthorizeRequest` and `fosite.AccessRequest` are plain structs with public fields (`ResponseTypes`, `RedirectURI`, `State`, `Client`, `Form`, `Session`, `GrantTypes`). We construct them directly in our domain service.

### Strategy Interfaces — Our Implementations

Fosite's strategies are **pluggable interfaces** — we implement them with our own libraries, not `go-jose`:

| Fosite Strategy Interface | Our Implementation | Library |
|---|---|---|
| `AccessTokenStrategy.GenerateAccessToken(ctx, Requester)` | JWT signing with current signing key | `lestrrat-go/jwx/v3` (`jwt.Sign()`, `jwa.ES256()`) |
| `AccessTokenStrategy.ValidateAccessToken(ctx, Requester, token)` | JWT verification via JWKS | `lestrrat-go/jwx/v3` |
| `AccessTokenStrategy.AccessTokenSignature(ctx, token)` | Extract `jti` or hash from JWT | stdlib `crypto/sha256` |
| `AuthorizeCodeStrategy.GenerateAuthorizeCode(ctx, Requester)` | Random code + SHA-256 signature | stdlib `crypto/rand` + `crypto/sha256` |
| `AuthorizeCodeStrategy.ValidateAuthorizeCode(ctx, Requester, code)` | Verify code matches stored hash | stdlib `crypto/subtle` |

This means **we never use `go-jose` types in our code** — fosite's internal dependency on `go-jose` is a transitive compile artifact, not a runtime concern in our codebase.

### Storage Adapters — Wrapping Our Repositories

Fosite storage interfaces store `fosite.Requester` objects. Our adapters translate between fosite's representation and our domain entities:

```go
// internal/domain/oauth2server/fosite_storage.go
type fositeStorage struct {
    codeRepo    ports.AuthorizationCodeRepository
    agentRepo   ports.AgentRepository
    credRepo    ports.BrokerClientCredentialRepository
}

// CreateAuthorizeCodeSession stores fosite's Requester as our AuthorizationCode entity
func (s *fositeStorage) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
    // Extract domain fields from fosite Requester
    authCode := &storage.AuthorizationCode{
        ID:            id.NewAuthorizationCodeID(),
        CodeHash:      sha256Hex(code),
        AgentID:       extractAgentID(req.GetClient()),
        Principal:     id.NewPrincipal(req.GetSession().GetSubject()),
        RedirectURI:   req.GetRequestForm().Get("redirect_uri"),
        CodeChallenge: req.GetRequestForm().Get("code_challenge"),
        Scope:         strings.Join(req.GetRequestedScopes(), " "),
        ExpiresAt:     req.GetSession().GetExpiresAt(fosite.AuthorizeCode),
    }
    return s.codeRepo.Create(ctx, authCode)
}
```

**Adapter LOC estimate**: ~150 LOC total for `AuthorizeCodeStorage`, `AccessTokenStorage` (no-op for stateless JWT), and `PKCERequestStorage`.

### Client Wrapper

```go
// Wraps Agent + BrokerClientCredential as fosite.Client
type brokerClient struct {
    agent      *storage.Agent
    credential *storage.BrokerClientCredential
}

func (c *brokerClient) GetID() string            { return string(c.credential.ClientID) }
func (c *brokerClient) GetHashedSecret() []byte   { return []byte(c.credential.SecretHash) }
func (c *brokerClient) GetRedirectURIs() []string  { return c.agent.RedirectURIs }
func (c *brokerClient) GetGrantTypes() fosite.Arguments {
    return fosite.Arguments{"authorization_code", "client_credentials"}
}
func (c *brokerClient) GetResponseTypes() fosite.Arguments { return fosite.Arguments{"code"} }
func (c *brokerClient) GetScopes() fosite.Arguments        { return fosite.Arguments{} }
func (c *brokerClient) IsPublic() bool                     { return false }
func (c *brokerClient) GetAudience() fosite.Arguments      { return fosite.Arguments{} }
```

~50 LOC. Lives in the domain service package — fosite types stay internal.

### What Fosite Handlers Provide (Battle-Tested Protocol Logic)

| Handler | What it handles | Edge cases covered |
|---|---|---|
| `AuthorizeExplicitGrantHandler` | Auth code generation via strategy, session storage, code invalidation on reuse (`ErrInvalidatedAuthorizeCode`), scope validation, redirect URI security, response parameter assembly | Replay detection, scope filtering, insecure redirect rejection |
| `ClientCredentialsGrantHandler` | Public client rejection, scope validation, token lifetime management, access token issuance via strategy | Public client abuse, scope escalation |
| `pkce.Handler` | S256 computation + verification, verifier length validation (43-128 chars), format regex (`[a-Z0-9._~-]`), `plain` method rejection, missing challenge enforcement, challenge/verifier storage coordination | Short verifiers, format attacks, method downgrade, missing challenge, orphaned verifiers |
| `HandleHelper.IssueAccessToken` | Token generation via strategy, storage, expiry calculation, response population (token_type, expires_in, scope) | — |
| `RFC6749Error` | Structured error types with error codes, hints, debug info, wrapping | All standard OAuth2 error codes |

**The PKCE handler alone covers ~8 edge cases** that would each need individual test coverage in a custom implementation.

### What We Still Implement Ourselves (Same as Custom)

- Client authentication (Argon2id verification before calling fosite handlers)
- Signing key management (CRUD, rotation, auto-generation, encryption)
- JWKS endpoint (serving public keys)
- RFC 8414 discovery endpoint
- HTTP handlers (chi, parse requests, write responses)
- Builder wiring, configuration
- All repositories (BrokerClientCredential, SigningKey, AuthorizationCode) and their adapters

### Dependency Assessment (Revised)

Importing `github.com/ory/fosite` (root package, needed for types like `AuthorizeRequester`) transitively compiles:

| Dependency | Compiled? | Used by our code? | Concern |
|---|---|---|---|
| `go-jose/go-jose/v3` | Yes (via `client.go`) | **No** — we implement strategies with `jwx/v3` | Binary size only; no type conflict in our code |
| `ory/x` | Yes (`errorsx` in handlers) | **No** — fosite uses it internally | Transitive tree |
| `pkg/errors` | Yes (handler error wrapping) | **No** | Redundant but harmless |
| `mohae/deepcopy` | Yes (`Session.Clone()`) | **No** | Small |
| `gorilla/mux` | **No** — only in fosite's test files | **No** | Not compiled |
| `cristalhq/jwt/v4` | **No** — test helpers only | **No** | Not compiled |

**Key insight**: `gorilla/mux` and `cristalhq/jwt` are **test dependencies** — they don't compile into our binary. `go-jose/v3` does compile (via the root `fosite` package) but we never import or use its types. The practical impact is binary size (~2MB) and `go.sum` entries.

### Effort Comparison

| Component | Custom LOC | Fosite + Adapters LOC | Delta |
|---|---|---|---|
| Protocol logic (PKCE, auth code, client creds, errors) | ~510 | ~0 (fosite provides) | -510 |
| Fosite storage adapters | ~0 | ~150 | +150 |
| Fosite client wrapper | ~0 | ~50 | +50 |
| Strategy implementations (JWT, auth code) | ~0 | ~120 | +120 |
| Fosite wiring (handler construction, config) | ~0 | ~80 | +80 |
| Shared work (handlers, repos, entities, migrations, keys, JWKS, discovery) | ~1,040 | ~1,040 | 0 |
| **Total** | **~1,550** | **~1,440** | **-110** |

The LOC savings are modest (~110 lines), but the **quality** difference matters: fosite's protocol logic is battle-tested across Ory Hydra's production deployments, covering edge cases that a custom implementation would need to independently discover and test.

### Trade-offs

**Arguments for fosite (headless):**
- Battle-tested PKCE implementation with 8+ edge cases
- Authorization code replay detection via `ErrInvalidatedAuthorizeCode`
- RFC 6749 error types with proper codes/hints — don't reinvent
- Scope validation strategy pattern — reusable
- Reduces novel protocol code that needs test coverage
- If requirements expand later (refresh tokens, introspection), fosite handlers exist

**Arguments for custom:**
- Zero new dependencies — cleaner `go.sum` and smaller binary
- No fosite version risk (still v0.x, breaking changes between minors)
- Storage interfaces are native to project patterns (no adapter layer)
- Conceptually simpler — no need to understand fosite's Requester model
- Full control over all protocol logic — no surprises from upstream changes

### Recommendation

**Use fosite headless.** The handler layer is clean, strategy interfaces allow using our existing libraries, and the protocol logic it provides (especially PKCE and auth code lifecycle) is genuinely valuable. The dependency cost is real but manageable — `go-jose/v3` compiles transitively but we never use its types. The adapter layer (~150 LOC) is modest and contained within the domain service package.

The ADR (`20260328-120100-oauth2-server-mode.md`) should document:
- Fosite is used as a headless protocol library, not as an HTTP framework
- Only the handler layer is used; `NewAuthorizeRequest`/`NewAccessRequest` are not called
- Strategy interfaces are implemented with `lestrrat-go/jwx/v3` (our `AccessTokenStrategy`) and stdlib crypto (our `AuthorizeCodeStrategy`)
- Fosite types do not leak outside `internal/domain/oauth2server/`
- If fosite's v0.x instability becomes a problem, the handler logic is replaceable — the domain service port interface remains stable

### Alternatives Considered

| Library | Assessment |
|---|---|
| **go-oauth2/oauth2 v4** | **Rejected.** Evaluated v4.5.4 (~3,600 stars, MIT). Four of eleven requirements lack built-in support: no JWKS endpoint, no key rotation, no custom JWT claims, no client secret hashing (PR #284 unmerged). The `go-oauth2-pg` adapter stores tokens as JSON blobs — incompatible with go-migrate (Constitution Principle IX). `JWTAccessGenerate` holds a single key only; full replacement required for rotation. Known PKCE edge-case bugs in issue tracker. The library's value proposition (flow scaffolding) is negated by the extent of custom work needed. |
| **ory/fosite (full — HTTP orchestrators)** | `NewAuthorizeRequest(*http.Request)` couples to HTTP. Conflicts with project's chi handlers. Rejected. |
| **ory/fosite (headless — handlers only)** | Clean handler interfaces; pluggable strategies. **Chosen.** |
| **coreos/go-oidc** | Client-side OIDC library, not a server framework. Not applicable. |
| **zitadel/oidc** | Full OIDC server framework. Maintains its own HTTP handling. Similar issues to fosite-full. |
| **Custom with lestrrat-go/jwx/v3** | Zero dependencies, native fit. Viable fallback if fosite v0.x instability becomes a problem. |

---

## Decision 2: JWT Signing Algorithm

### Decision: ES256 (ECDSA P-256) as default, with RS256 as documented alternative

### Rationale
- **ES256** produces compact signatures (~64 bytes vs ~256 bytes for RS256), reducing token size
- P-256 is universally supported by JWT libraries and hardware security modules
- FIPS 186-4 compliant
- Key generation is fast (~<1ms) — important for auto-generation at startup
- `lestrrat-go/jwx/v3` has first-class ES256 support via `jwa.ES256()`

### Alternatives Considered
- **RS256**: Wider legacy support but larger keys and signatures. Documented as alternative if operators need RSA compatibility.
- **EdDSA (Ed25519)**: Not yet universally supported by all JWT consumer libraries. Future consideration.

---

## Decision 3: Client Secret Generation

### Decision: 32 bytes of `crypto/rand` entropy, base64url-encoded (43 characters)

### Rationale
- Exceeds SR-001 requirement (minimum 32 bytes of entropy = 256 bits)
- `crypto/rand` is the Go stdlib CSPRNG — no custom randomness
- Base64url encoding is URL-safe and copy-paste friendly
- 43 characters is practical for operator use (not excessively long)

### Alternatives Considered
- **UUID v4**: Only 122 bits of randomness — insufficient for SR-001
- **Hex encoding**: Twice as long (64 characters) for same entropy — less practical
- **Raw bytes**: Not operator-friendly for initial credential display

---

## Decision 4: Client Secret Hashing

### Decision: Argon2id via `golang.org/x/crypto/argon2`

### Parameters
- **Memory**: 64 MiB (`64 * 1024` KiB)
- **Iterations**: 3 (time cost)
- **Parallelism**: 4 threads
- **Salt**: 16 bytes from `crypto/rand`
- **Key length**: 32 bytes
- **Encoding**: PHC string format (`$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`)

### Rationale
- Argon2id is the OWASP-recommended algorithm for new applications (SR-002)
- Strongest memory-hardness among password hashing functions — resistant to GPU/ASIC brute-force attacks
- PHC string format embeds algorithm, version, params, and salt — self-describing, no separate salt column needed
- `golang.org/x/crypto/argon2` is the vetted Go library (Constitution Principle III)
- With 256-bit entropy secrets, brute force is infeasible regardless of hash function choice — Argon2id adds defense-in-depth for the scenario where secrets are shorter (e.g., future API key support)
- Hash time ~100ms with these params — acceptable for token endpoint authentication

### Implementation

Custom `Argon2Hasher` struct (~30 LOC) in `internal/domain/oauth2server/`:

```go
type Argon2Hasher struct {
    memory      uint32 // 64 * 1024
    iterations  uint32 // 3
    parallelism uint8  // 4
    saltLength  uint32 // 16
    keyLength   uint32 // 32
}

func (h *Argon2Hasher) Hash(secret string) (string, error) {
    salt := make([]byte, h.saltLength)
    if _, err := crypto_rand.Read(salt); err != nil {
        return "", err
    }
    hash := argon2.IDKey([]byte(secret), salt, h.iterations, h.memory, h.parallelism, h.keyLength)
    return encodePHC(hash, salt, h), nil
}

func (h *Argon2Hasher) Compare(hash, secret string) error {
    params, salt, expectedHash, err := decodePHC(hash)
    if err != nil {
        return err
    }
    actualHash := argon2.IDKey([]byte(secret), salt, params.iterations, params.memory, params.parallelism, params.keyLength)
    if subtle.ConstantTimeCompare(expectedHash, actualHash) != 1 {
        return ErrMismatch
    }
    return nil
}
```

### Alternatives Considered
- **bcrypt (cost 12)**: Industry standard, simpler API, built-in salt. But lacks memory-hardness — vulnerable to GPU attacks. Less future-proof than Argon2id. Sufficient for 256-bit secrets but not the strongest choice.
- **scrypt**: Memory-hard like Argon2id, but Argon2id won the Password Hashing Competition and is the OWASP recommendation. scrypt's parameter tuning is less intuitive.

---

## Decision 5: Signing Key Storage Format

### Decision: PEM-encoded PKCS#8 private keys, encrypted at rest via existing `EncryptionPort`

### Rationale
- PEM/PKCS#8 is the standard format for asymmetric key serialization
- `lestrrat-go/jwx/v3` can import PEM-encoded keys directly via `jwk.FromRaw()`
- Encryption via `EncryptionPort` reuses the existing envelope encryption infrastructure (ADR 20260116-120200/20260223-120100)
- Encryption context: `{"kid": "<kid>"}` — a single-field signing-key subject, distinct from the `service_id` context used for OAuth2 user-session tokens in ADR 20260116-120000

### Alternatives Considered
- **JWK JSON format**: Viable but PEM is more widely understood for key storage and interoperability
- **No encryption (rely on DB-level encryption)**: Violates SR-004 (application-level encryption required for key material)
- **Raw DER bytes**: PEM provides headers for format identification

---

## Decision 6: Authorization Code Storage

### Decision: SHA-256 hash of code stored in database; plaintext code returned to client once

### Rationale
- Never store plaintext authorization codes (defense in depth — even if DB is compromised, codes cannot be replayed)
- SHA-256 is suitable because codes are high-entropy random values (not passwords) — no need for bcrypt's slow hashing
- Single-use enforcement via atomic `UPDATE ... SET used_at = NOW() WHERE used_at IS NULL AND code_hash = ? RETURNING ...`
- 60-second TTL enforced by `expires_at` column check in queries

### Alternatives Considered
- **bcrypt hashing**: Unnecessary overhead for high-entropy codes; SHA-256 is sufficient
- **Plaintext storage**: Violates defense-in-depth principle
- **In-memory only**: Doesn't survive restarts; fails multi-replica requirement

---

## Decision 7: Migration Numbering

### Decision: Migrations 009–012 (next sequence after existing 008)

### Rationale
- Existing migrations end at `008_drop_agent_client_id_unique`
- Sequential numbering per go-migrate convention:
  - `009_add_agent_redirect_uris.{up,down}.sql`
  - `010_create_broker_client_credentials.{up,down}.sql`
  - `011_create_signing_keys.{up,down}.sql`
  - `012_create_authorization_codes.{up,down}.sql`

---

## Decision 8: Encryption Context for Signing Keys

### Decision: Use `{"kid": "<kid>"}` as encryption context

### Rationale
- ADR 20260116-120000 optimized encryption context to a single subject field for UserSession tokens (`service_id`) and now permits `kid` as the signing-key equivalent
- Signing keys are not per-service — they are broker-global. Using `service_id` would be semantically incorrect
- `kid` provides AAD binding specific to signing key material while preserving the one-field minimal-context rule
- Consistent with principle: encryption context identifies WHAT is encrypted, never contains secrets
- Verified on decrypt: mismatched context causes decryption failure (fail closed)

### Alternatives Considered
- **No encryption context**: Violates EncryptionPort contract and constitution requirement for AAD binding
- **Reuse `service_id` pattern**: Semantically incorrect — signing keys aren't per-service
- **Include algorithm in context**: Additional binding, but algorithm is already stored as a column — redundant

---

## Resolved NEEDS CLARIFICATION Items

All technical context items from `plan.md` are resolved:

| Item | Resolution |
|---|---|
| OAuth2 library choice | Custom implementation with `lestrrat-go/jwx/v3` (Decision 1) |
| JWT signing algorithm | ES256 default (Decision 2) |
| Client secret entropy | 32 bytes `crypto/rand`, base64url-encoded (Decision 3) |
| Client secret hashing | Argon2id (Decision 4) |
| Signing key format | PEM PKCS#8, encrypted via EncryptionPort (Decision 5) |
| Auth code storage | SHA-256 hash, single-use atomic update (Decision 6) |
| Migration numbering | 009–012 (Decision 7) |
| Encryption context for keys | `entity_type` + `key_id` (Decision 8) |
| CEL token claims expression | Global config, reuse existing `CELCompilerPort`, fail-closed (Decision 9) |

---

## Decision 9: CEL Token Claims Expression

### Decision: **Reuse existing `CELCompilerPort`** with a new token-claims-specific CEL environment

### Context

The spec (FR-013 through FR-013e) requires an optional CEL expression evaluated at token issuance time to produce custom claims merged into locally-issued access tokens. Three design decisions were clarified:

1. **Scope**: Global config under `oauth2_authorization_server.token_claims_expression` — applies to all agents uniformly. Per-agent customization is out of scope; the global expression receives the agent as a variable, so per-agent logic can be encoded in the expression itself (e.g., `agent.display_name == "internal" ? {"role": "admin"} : {}`).

2. **Context variables**: The expression receives `agent` (object: `id`, `client_id`, `display_name`, `metadata`), `principal` (object: `id` always present, `email` and `display_name` optional), and `request` (object: `grant_type`, `scopes`). These are constructed from the fosite `Requester` and project domain entities at issuance time.

3. **Failure semantics**: Fail-closed — if the expression fails at runtime (evaluation error, type mismatch, unexpected return type), token issuance is rejected. No token with partial or missing custom claims is ever issued. This is consistent with Constitution Principle I (Security-First: fail closed).

### Architecture

```
oauth2_authorization_server.token_claims_expression (config)
    │
    ▼
CELCompilerPort.CompileExpression() at startup
    │ ← Fails startup if expression invalid (FR-013b)
    ▼
Compiled CELProgram (cached, immutable)
    │
    ▼  [per token issuance]
CELProgram.Eval(ctx, variables)
    │ ← variables: {agent, principal, request}
    ▼
map[string]dyn → merged into JWT claims
    │ ← Base claims (iss, sub, iat, exp, jti, kid) protected from override
    ▼
Signed JWT access token
```

### Implementation Approach

The `token_claims_cel.go` file in `internal/domain/oauth2server/` will:

1. At construction time (via `NewProvider`), compile the expression using the existing `CELCompilerPort`
2. Declare CEL variables with types matching the context objects
3. At issuance time (`GenerateAccessToken`), evaluate the compiled program
4. Validate the result is `map[string]dyn`, strip any base claim keys, merge into JWT builder
5. On any evaluation error, return an error that causes token issuance to fail

### CEL Variable Schema

```go
// Variables provided to the CEL expression
variables := map[string]interface{}{
    "agent": map[string]interface{}{
        "id":           string(agent.ID),
        "client_id":    string(cred.ClientID),
        "display_name": agent.DisplayName,
        "metadata":     agent.Metadata, // map[string]string or nil
    },
    "principal": map[string]interface{}{
        "id":           string(principal),
        "email":        principalEmail,        // may be ""
        "display_name": principalDisplayName,   // may be ""
    },
    "request": map[string]interface{}{
        "grant_type": grantType,
        "scopes":     grantedScopes, // []string
    },
}
```

### Base Claim Protection

The following claim keys are reserved and MUST NOT be overridden by the CEL expression:
`iss`, `sub`, `iat`, `exp`, `jti`, `kid`, `agent_id`, `scope`

If the expression returns any of these keys, they are silently dropped (logged at warn level) rather than causing a failure — the intent is to prevent accidental override, not to punish misconfiguration.

### Rationale

- **Reuse `CELCompilerPort`**: The project already has a production CEL compiler adapter (ADR 20260116-120100). Reusing it avoids duplicating CEL infrastructure and follows the hexagonal architecture pattern.
- **Global config**: Consistent with how CEL is used elsewhere in the project (token exchange authorization, claim extraction). Simpler than per-agent storage + Admin API + migration.
- **Fail-closed**: Constitution Principle I. A token missing expected custom claims could cause downstream authorization failures — better to surface the error immediately.
- **Default `{}`**: When no expression is configured, no custom claims are added. Zero behavioral change for existing deployments.

### Alternatives Considered

1. **Per-agent CEL expression stored in DB**: More flexible but significantly more complex (new column, Admin API endpoint, migration, per-agent compilation cache). Deferred — can be added later if global config proves insufficient.
2. **Go template instead of CEL**: Simpler syntax but no type safety, no sandboxing, no startup validation. CEL is already a project dependency and provides all of these.
3. **Static claim map in config (no expressions)**: Too rigid — can't vary claims by agent or grant type. CEL provides the minimal dynamic logic needed.
