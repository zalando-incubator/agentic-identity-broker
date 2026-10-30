# Research Document: Domain Model and Consent APIs

**Feature**: 20251217-120000-domain-model-apis
**Date**: 2025-12-17
**Status**: Complete

## Overview

This document captures research decisions for implementing the domain model and consent APIs feature. All research areas have been resolved with no blockers identified.

## 1. Client Secret Encryption Strategy

### Decision
Define **EncryptionPort interface** with pluggable adapter implementations. Start with no-op adapter for development.

### Rationale
- **Flexibility**: Port interface allows switching encryption strategies without changing domain/storage code
- **Hexagonal Architecture**: Follows established pattern from 20251215-120100-persistence-layer
- **Incremental Implementation**: No-op adapter enables feature development without blocking on encryption complexity
- **Security Ready**: Interface design supports future AES-256-GCM or external KMS adapters
- **Context Support**: `encryptionContext` parameter enables authenticated encryption (AEAD) when implemented

### Port Interface
```go
// Location: internal/ports/encryption.go

// EncryptionPort defines the interface for encrypting/decrypting sensitive data.
// Implementations can range from no-op (development) to AES-256-GCM (production)
// to external KMS (cloud deployments).
type EncryptionPort interface {
    // Encrypt encrypts plaintext with optional encryption context.
    // encryptionContext provides additional authenticated data for AEAD ciphers.
    // Returns encrypted bytes or error if encryption fails.
    Encrypt(plaintext string, encryptionContext map[string]string) ([]byte, error)

    // Decrypt decrypts ciphertext with optional encryption context.
    // encryptionContext must match the context used during encryption for AEAD ciphers.
    // Returns decrypted plaintext or error if decryption fails.
    Decrypt(ciphertext []byte, encryptionContext map[string]string) (string, error)
}
```

### No-Op Adapter (Current Implementation)
```go
// Location: internal/adapters/storage/noop/encryption.go

// NoOpEncryption implements EncryptionPort by storing plaintext.
// Suitable for development and testing environments only.
// WARNING: Do not use in production - secrets are stored unencrypted.
type NoOpEncryption struct{}

func NewNoOpEncryption() *NoOpEncryption {
    return &NoOpEncryption{}
}

func (n *NoOpEncryption) Encrypt(plaintext string, encryptionContext map[string]string) ([]byte, error) {
    // Store plaintext as bytes (no actual encryption)
    return []byte(plaintext), nil
}

func (n *NoOpEncryption) Decrypt(ciphertext []byte, encryptionContext map[string]string) (string, error) {
    // Return bytes as plaintext string
    return string(ciphertext), nil
}
```

### Storage Format
- **Database Column**: `client_secret_encrypted BYTEA`
- **No-Op Format**: Plaintext bytes (no encryption applied)
- **Future AES-256-GCM Format**: `[nonce][ciphertext][tag]` (12-byte nonce + encrypted payload + 16-byte GCM tag)

### Encryption Context Usage
```go
// Example: Encrypting client secret with context
encryptionContext := map[string]string{
    "service_id": service.ID,
    "purpose":    "oauth2_client_secret",
}

encrypted, err := encryptionPort.Encrypt(service.ClientSecret, encryptionContext)
if err != nil {
    return fmt.Errorf("failed to encrypt client secret: %w", err)
}

service.ClientSecretEncrypted = encrypted
```

### Future AES-256-GCM Adapter (Not Implemented)
```go
// Location: internal/adapters/storage/aes/encryption.go (future)

// AES256GCMEncryption implements EncryptionPort using AES-256-GCM.
// - AEAD cipher with authenticated encryption
// - Hardware-accelerated on modern CPUs (AES-NI)
// - NIST approved, industry standard
// - Uses encryptionContext as additional authenticated data (AAD)
type AES256GCMEncryption struct {
    key []byte // 32-byte encryption key
}

func NewAES256GCMEncryption(key []byte) (*AES256GCMEncryption, error) {
    if len(key) != 32 {
        return nil, errors.New("key must be 32 bytes for AES-256")
    }
    return &AES256GCMEncryption{key: key}, nil
}

// Implementation would use crypto/aes and crypto/cipher packages
```

### Key Management (Future)
- **Configuration**: 32-byte key (base64 encoded) when using AES adapter
- **Production**: Key from secure key management system (AWS KMS, HashiCorp Vault, etc.)
- **Development**: No-op adapter requires no key management
- **Rotation**: Future enhancement - support multiple keys with key versioning

### Alternatives Considered
- **Direct AES-256-GCM Implementation**: Rejected (blocks feature development on encryption complexity)
- **AES-256-CBC**: Rejected (no integrity protection, requires padding)
- **ChaCha20-Poly1305**: Viable future alternative to AES-GCM
- **RSA**: Rejected (asymmetric encryption overkill for symmetric secret storage)

---

## 2. OAuth2 Endpoint Discovery Implementation

### Decision
Use **well-known metadata path with optional override** pattern.

### Rationale
- **Standard Compliance**: OAuth 2.0 Authorization Server Metadata (RFC 8414) defines `.well-known/oauth-authorization-server`
- **OIDC Compatibility**: OpenID Connect Discovery also uses `.well-known/openid-configuration`
- **Flexibility**: Supports non-standard providers via explicit `metadata_url` override
- **Simplicity**: Automatic construction reduces configuration burden

### Implementation Approach
```go
// Location: internal/domain/consent/discovery.go

func DiscoverOAuth2Endpoints(ctx context.Context, service *ports.ThirdpartyOAuth2Service) (*OAuth2Endpoints, error) {
    // Determine metadata URL
    metadataURL := service.Discovery.MetadataURL
    if metadataURL == "" {
        // Construct standard well-known path
        issuerURL, err := url.Parse(service.IssuerURI)
        if err != nil {
            return nil, fmt.Errorf("invalid issuer_uri: %w", err)
        }
        metadataURL = issuerURL.ResolveReference(&url.URL{
            Path: ".well-known/oauth-authorization-server",
        }).String()
    }

    // Fetch metadata with timeout
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", metadataURL, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                // SR-009: Validate SSL certificates in production
                InsecureSkipVerify: false,
            },
        },
    }

    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch metadata: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("metadata endpoint returned status %d", resp.StatusCode)
    }

    var metadata OAuth2Metadata
    if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
        return nil, fmt.Errorf("failed to parse metadata: %w", err)
    }

    return &OAuth2Endpoints{
        TokenEndpoint:     metadata.TokenEndpoint,
        AuthorizeEndpoint: metadata.AuthorizationEndpoint,
    }, nil
}
```

### Well-Known Paths
- **OAuth 2.0**: `{issuer}/.well-known/oauth-authorization-server`
- **OIDC**: `{issuer}/.well-known/openid-configuration` (fallback)

### Fallback Strategy
1. Try discovery if `enable_discovery = true`
2. On discovery failure, fall back to manually configured endpoints
3. If both fail, return error requiring manual configuration

### Metadata Fields Used
```json
{
  "issuer": "https://accounts.google.com",
  "authorization_endpoint": "https://accounts.google.com/o/oauth2/v2/auth",
  "token_endpoint": "https://oauth2.googleapis.com/token",
  "jwks_uri": "https://www.googleapis.com/oauth2/v3/certs",
  "scopes_supported": ["openid", "email", "profile"]
}
```

### Edge Cases
- **Non-standard providers**: Use explicit `metadata_url` override
- **Network timeouts**: 10-second timeout with proper error wrapping
- **Invalid SSL**: Reject in production (SR-009), configurable for development
- **Missing fields**: Validate required fields (token_endpoint, authorization_endpoint)

### Alternatives Considered
- **Always require explicit metadata_url**: Rejected (too much configuration burden)
- **Always construct from issuer**: Rejected (doesn't support non-standard providers)
- **Cache metadata**: Deferred to future enhancement (TTL-based caching)

---

## 3. Grant Expiration Enforcement Patterns

### Decision
Use **query-time filtering** with `WHERE valid_until IS NULL OR valid_until > NOW()`.

### Rationale
- **Simplicity**: No background jobs or cron processes required
- **Accuracy**: Always current (no eventual consistency issues)
- **Performance**: Indexed column makes filtering efficient
- **Atomic**: Expiration check happens in same transaction as grant retrieval

### Implementation Approach
```sql
-- Query active grants (filters expired at read time)
SELECT id, principal, agent_id, valid_until, delegated_oauth2_tokens, created_at, updated_at
FROM user_grants
WHERE agent_id = $1
  AND principal = $2
  AND (valid_until IS NULL OR valid_until > NOW());

-- Index for efficient filtering
CREATE INDEX idx_user_grants_active_by_principal_agent
ON user_grants(principal, agent_id)
WHERE valid_until IS NULL OR valid_until > NOW();
```

### Query Patterns
```go
// Location: internal/adapters/storage/postgres/user_grants.go

func (a *Adapter) ListActiveGrants(ctx context.Context, principal, agentID string) ([]*ports.UserGrant, error) {
    query := `
        SELECT id, principal, agent_id, valid_until, delegated_oauth2_tokens, created_at, updated_at
        FROM user_grants
        WHERE agent_id = $1
          AND principal = $2
          AND (valid_until IS NULL OR valid_until > NOW())
        ORDER BY created_at DESC
    `

    var grants []*ports.UserGrant
    err := a.db.SelectContext(ctx, &grants, query, agentID, principal)
    if err != nil {
        return nil, storage.NewStorageError("ListActiveGrants", storage.ErrorKindUnknown, err, "failed to list grants")
    }

    return grants, nil
}
```

### Indefinite Grants
- **Representation**: `valid_until = NULL` in database
- **JSON**: `"valid_until": null` in API responses
- **Behavior**: Never expire, remain active until explicitly revoked

### Periodic Cleanup (Optional Future Enhancement)
```sql
-- Background job to purge old expired grants (retention policy)
DELETE FROM user_grants
WHERE valid_until < NOW() - INTERVAL '90 days';
```

### Performance Considerations
- **Partial Index**: Index only on active grants reduces index size
- **NOW() Function**: PostgreSQL evaluates at query start, consistent within transaction
- **Pagination**: Support limit/offset for large grant lists

### Alternatives Considered
- **Background job**: Rejected (adds complexity, eventual consistency issues)
- **Application-level filtering**: Rejected (miss grants that expired between read and filter)
- **TTL column with trigger**: Rejected (over-engineered for simple use case)

---

## 4. Service Deletion Blocking Strategy

### Decision
Use **explicit count check before deletion** with 409 Conflict response.

### Rationale
- **Clear Error Messages**: Can return count of affected grants in error response
- **Foreign Key Protection**: Database constraint prevents accidental deletion
- **User Guidance**: Error message guides admin to clean up grants first
- **Referential Integrity**: Maintains data consistency

### Implementation Approach
```go
// Location: internal/adapters/storage/postgres/thirdparty_services.go

func (a *Adapter) Delete(ctx context.Context, id string) error {
    // Check if any grants reference this service
    count, err := a.CountGrantsReferencingService(ctx, id)
    if err != nil {
        return storage.NewStorageError("DeleteService", storage.ErrorKindUnknown, err, "failed to check grants")
    }

    if count > 0 {
        return storage.NewStorageError(
            "DeleteService",
            storage.ErrorKindConflict,
            nil,
            fmt.Sprintf("cannot delete service: %d active grants reference it", count),
        )
    }

    // Proceed with deletion (foreign key constraint provides additional safety)
    query := "DELETE FROM thirdparty_oauth2_services WHERE id = $1"
    result, err := a.db.ExecContext(ctx, query, id)
    if err != nil {
        return storage.NewStorageError("DeleteService", storage.ErrorKindUnknown, err, "failed to delete service")
    }

    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return storage.NewStorageError("DeleteService", storage.ErrorKindUnknown, err, "failed to check rows affected")
    }

    if rowsAffected == 0 {
        return storage.NewStorageError("DeleteService", storage.ErrorKindNotFound, nil, "service not found")
    }

    return nil
}

func (a *Adapter) CountGrantsReferencingService(ctx context.Context, serviceID string) (int, error) {
    query := `
        SELECT COUNT(*)
        FROM user_grants
        WHERE delegated_oauth2_tokens @> jsonb_build_array(jsonb_build_object('thirdparty_oauth2_service_id', $1))
    `

    var count int
    err := a.db.GetContext(ctx, &count, query, serviceID)
    if err != nil {
        return 0, err
    }

    return count, nil
}
```

### Database Constraints
```sql
-- Foreign key constraint (additional safety layer)
ALTER TABLE user_grants
ADD CONSTRAINT fk_grants_service
FOREIGN KEY (thirdparty_oauth2_service_id)
REFERENCES thirdparty_oauth2_services(id)
ON DELETE RESTRICT;  -- Explicit restriction
```

### API Response
```json
{
  "error": "conflict",
  "message": "Cannot delete service: 15 active grants reference it",
  "details": {
    "service_id": "svc-123",
    "grant_count": 15,
    "suggestion": "Revoke or modify grants before deleting service"
  }
}
```

### Cleanup Workflow
1. Admin attempts to delete service
2. System returns 409 Conflict with grant count
3. Admin revokes/modifies affected grants
4. Admin retries service deletion (succeeds)

### Alternatives Considered
- **Cascade delete**: Rejected (data loss risk, violates user expectations)
- **Soft delete**: Rejected (adds complexity, "deleted" services still in queries)
- **Force delete flag**: Rejected (security risk, violates fail-safe principle)

---

## 5. JSONB Handling in sqlx

### Decision
Use **explicit JSON marshaling/unmarshaling** with `json.Marshal` and custom `Scan` methods.

### Rationale
- **Type Safety**: Compile-time type checking for Go structs
- **Flexibility**: Full control over JSON structure and validation
- **sqlx Compatibility**: Works seamlessly with `sqlx.Get` and `sqlx.Select`
- **Validation**: Can validate JSON structure before storage

### Implementation Approach
```go
// Location: internal/ports/oauth_scope.go

type OAuthScope struct {
    ScopeValue  string `json:"scope_value" db:"-"`
    Description string `json:"description" db:"-"`
}

// Location: internal/ports/thirdparty_service.go

type ThirdpartyOAuth2Service struct {
    ID          string         `db:"id" json:"id"`
    DisplayName string         `db:"display_name" json:"display_name"`
    ClientID    string         `db:"client_id" json:"client_id"`
    // ... other fields
    Scopes      []OAuthScope   `db:"scopes" json:"scopes"`  // sqlx handles via Scan
}

// Scan implements sql.Scanner for JSONB column
func (s *ThirdpartyOAuth2Service) Scan(value interface{}) error {
    if value == nil {
        s.Scopes = []OAuthScope{}
        return nil
    }

    bytes, ok := value.([]byte)
    if !ok {
        return fmt.Errorf("failed to scan scopes: expected []byte, got %T", value)
    }

    return json.Unmarshal(bytes, &s.Scopes)
}
```

### Insert/Update Pattern
```go
// Marshal to JSON before storage
scopesJSON, err := json.Marshal(service.Scopes)
if err != nil {
    return storage.NewStorageError("CreateService", storage.ErrorKindUnknown, err, "failed to marshal scopes")
}

query := `
    INSERT INTO thirdparty_oauth2_services (id, display_name, client_id, scopes)
    VALUES ($1, $2, $3, $4::jsonb)
`

_, err = a.db.ExecContext(ctx, query, service.ID, service.DisplayName, service.ClientID, scopesJSON)
```

### Query Pattern
```go
// sqlx automatically calls Scan method
var service ports.ThirdpartyOAuth2Service
err := a.db.GetContext(ctx, &service, "SELECT * FROM thirdparty_oauth2_services WHERE id = $1", id)
// service.Scopes populated via Scan
```

### JSONB Queries (PostgreSQL-specific)
```sql
-- Check if grant references a service (JSONB array containment)
SELECT COUNT(*)
FROM user_grants
WHERE delegated_oauth2_tokens @> jsonb_build_array(jsonb_build_object('thirdparty_oauth2_service_id', $1));

-- Extract specific scope from JSONB array
SELECT jsonb_array_elements(scopes)->>'scope_value' AS scope
FROM thirdparty_oauth2_services
WHERE id = $1;
```

### Validation
```go
// Validate JSON structure before marshaling
func (s *ThirdpartyOAuth2Service) Validate() error {
    if len(s.Scopes) == 0 {
        return errors.New("at least one scope is required")
    }

    for i, scope := range s.Scopes {
        if scope.ScopeValue == "" {
            return fmt.Errorf("scope %d: scope_value is required", i)
        }
    }

    return nil
}
```

### Alternatives Considered
- **String column with manual parsing**: Rejected (type-unsafe, error-prone)
- **Separate scopes table**: Rejected (over-normalized for simple array storage)
- **gorm JSON tags**: Rejected (not using GORM per Principle IX)

---

## 6. Agent-Service Relationship (Clarified)

### Decision
**Implicit relationship**: All configured third-party OAuth2 services are available to all agents.

### Rationale
- **Simplicity**: No junction table or many-to-many relationship needed
- **User Clarification**: Explicitly requested during specification clarification
- **Flexibility**: Agent-to-service associations deferred to future "content" domain feature
- **MVP Scope**: Reduces initial complexity while delivering core functionality

### Implementation
```go
// Location: internal/domain/consent/service.go

func (s *ConsentService) GetAgentConsentInfo(ctx context.Context, agentID string) (*AgentConsentInfo, error) {
    // Fetch agent
    agent, err := s.agentRepo.Get(ctx, agentID)
    if err != nil {
        return nil, fmt.Errorf("failed to get agent: %w", err)
    }

    // Fetch ALL third-party services (per clarification)
    filter := &ports.ThirdpartyServiceFilter{Limit: 1000}
    services, err := s.serviceRepo.List(ctx, filter)
    if err != nil {
        return nil, fmt.Errorf("failed to list services: %w", err)
    }

    return &AgentConsentInfo{
        Agent:    agent,
        Services: services,  // All available services
    }, nil
}
```

### Future Extension Point
```go
// Future enhancement: Filter services by agent configuration
// services, err := s.serviceRepo.ListByAgent(ctx, agentID)
```

### API Behavior
- **GET /api/consent/agent/:agent-id**: Returns agent metadata + ALL configured services
- **No filtering**: Client receives complete list of available services and scopes
- **Client-side selection**: Frontend UI presents all options to user for grant creation

---

## Summary of Decisions

| Area | Decision | Status |
|------|----------|--------|
| Client Secret Encryption | AES-256-GCM with 32-byte key | ✅ Complete |
| OAuth2 Discovery | Well-known path + metadata_url override | ✅ Complete |
| Grant Expiration | Query-time filtering with indexed WHERE clause | ✅ Complete |
| Service Deletion | Explicit count check + 409 Conflict response | ✅ Complete |
| JSONB Handling | Explicit json.Marshal with custom Scan methods | ✅ Complete |
| Agent-Service Relationship | Implicit (all services available to all agents) | ✅ Complete |

**No blockers identified**. All research complete and ready for implementation.

## Next Steps

1. ✅ Research complete - proceed to Phase 1 design
2. Create data-model.md with entity definitions
3. Create quickstart.md with implementation guide
4. Generate API contracts (OpenAPI spec)
5. Begin Phase 2 implementation

## References

- **Specification**: [spec.md](spec.md)
- **Constitution**: [../../.specify/memory/constitution.md](../../.specify/memory/constitution.md)
- **ADR 20251215-120300**: [../../adrs/20251215-120300-storage-layer-architecture.md](../../adrs/20251215-120300-storage-layer-architecture.md)
- **Go Crypto**: https://pkg.go.dev/crypto/aes
- **RFC 8414**: OAuth 2.0 Authorization Server Metadata
- **PostgreSQL JSONB**: https://www.postgresql.org/docs/current/datatype-json.html
