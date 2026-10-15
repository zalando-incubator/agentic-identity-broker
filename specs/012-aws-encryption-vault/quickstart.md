# Quickstart: Integrating Encryption Vault for OAuth Tokens

**Date**: 2026-01-15 | **Feature**: `012-aws-encryption-vault` | **Spec**: [spec.md](./spec.md)

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

## Overview

This quickstart shows developers how to integrate token encryption into the oauth2session service. Encryption happens transparently at the service layer; storage adapters and callers see only the high-level API.

---

## 1. Configuration

### Step 1: Set KEK (Key Encryption Key)

**Production (AWS KMS)**:

```yaml
# .env
encryption:
  key_encryption_key: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
```

**Development (Environment Variable)**:

```bash
# .env
encryption:
  key_encryption_key: ${ENCRYPTION_KEK}

# Export environment variable
export ENCRYPTION_KEK="your-base64-encoded-key-material"
```

### Step 2: Validate Configuration at Startup

The AWS Encryption SDK adapter validates KEK accessibility when the application starts:

- If KEK is unavailable, application fails with clear error (no silent fallback)
- Check startup logs for "Encryption vault initialized successfully"

---

## 2. Service Layer Integration

### Inject EncryptionPort into OAuth2Session Service

**Constructor**:

```go
type OAuth2SessionService struct {
    repository        UserSessionRepository
    encryptionPort    ports.EncryptionPort
    eventPublisher    EventPublisher
}

func NewOAuth2SessionService(
    repo UserSessionRepository,
    port ports.EncryptionPort,
    publisher EventPublisher,
) *OAuth2SessionService {
    return &OAuth2SessionService{
        repository:     repo,
        encryptionPort: port,
        eventPublisher: publisher,
    }
}
```

### Create Session (with encryption)

The provider supplies the access-token expiry. Stored refresh ciphertext shows credential presence, not provider validity.
The broker records terminal provider rejection instead of a refresh-token expiry date.

```go
func (s *OAuth2SessionService) CreateSession(
    ctx context.Context,
    principal string,
    serviceID string,
    accessToken, refreshToken string,
    accessExpiry time.Time,
) (*UserSession, error) {
    // Step 1: Construct encryption context
    encryptionContext := map[string]string{
        "service_id": serviceID,
    }

    // Step 2: Encrypt access token
    encryptedAccess, err := s.encryptionPort.Encrypt(
        ctx,
        []byte(accessToken),
        encryptionContext,
    )
    if err != nil {
        // Fail securely (no fallback to plaintext)
        s.eventPublisher.Publish(SessionEncryptionFailed{
            SessionID: "", // Not yet assigned
            ServiceID: serviceID,
            ErrorKind: "encryption_failed",
            Timestamp: time.Now(),
        })
        return nil, fmt.Errorf("failed to encrypt access token: %w", err)
    }

    // Step 3: Encrypt refresh token
    encryptedRefresh, err := s.encryptionPort.Encrypt(
        ctx,
        []byte(refreshToken),
        encryptionContext,
    )
    if err != nil {
        s.eventPublisher.Publish(SessionEncryptionFailed{...})
        return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
    }

    // Step 4: Create UserSession with encrypted tokens
    session := &UserSession{
        ID:                       uuid.New(),
        Principal:                principal,
        ServiceID:                serviceID,
        EncryptedAccessToken:     encryptedAccess,
        EncryptedRefreshToken:    encryptedRefresh,
        EncryptionContext:        encryptionContext,
        AccessTokenExpiry:        accessExpiry,
        CreatedAt:                time.Now(),
        UpdatedAt:                time.Now(),
    }

    // Step 5: Store session (adapter sees encrypted tokens only)
    if err := s.repository.Create(ctx, session); err != nil {
        return nil, fmt.Errorf("failed to store session: %w", err)
    }

    // Step 6: Publish success event
    s.eventPublisher.Publish(SessionEncrypted{
        SessionID: session.ID,
        ServiceID: serviceID,
        Timestamp: time.Now(),
    })

    return session, nil
}
```

### Get Session (with decryption)

```go
func (s *OAuth2SessionService) GetSession(
    ctx context.Context,
    sessionID uuid.UUID,
) (*UserSession, error) {
    // Step 1: Retrieve encrypted session from repository
    encryptedSession, err := s.repository.Get(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve session: %w", err)
    }

    // Step 2: Decrypt access token
    plainAccessToken, err := s.encryptionPort.Decrypt(
        ctx,
        encryptedSession.EncryptedAccessToken,
        encryptedSession.EncryptionContext,
    )
    if err != nil {
        // Fail securely (no fallback)
        s.eventPublisher.Publish(SessionDecryptionFailed{
            SessionID: sessionID,
            ServiceID: encryptedSession.ServiceID,
            ErrorKind: "decryption_failed",
            Timestamp: time.Now(),
        })
        return nil, fmt.Errorf("failed to decrypt access token: %w", err)
    }

    // Step 3: Decrypt refresh token
    plainRefreshToken, err := s.encryptionPort.Decrypt(
        ctx,
        encryptedSession.EncryptedRefreshToken,
        encryptedSession.EncryptionContext,
    )
    if err != nil {
        s.eventPublisher.Publish(SessionDecryptionFailed{...})
        return nil, fmt.Errorf("failed to decrypt refresh token: %w", err)
    }

    // Step 4: Create new session object with plaintext tokens for caller
    // (Note: Don't modify encryptedSession; create a view object if needed)
    decryptedSession := &SessionView{
        SessionID:       encryptedSession.ID,
        Principal:       encryptedSession.Principal,
        ServiceID:       encryptedSession.ServiceID,
        AccessToken:     string(plainAccessToken),
        RefreshToken:    string(plainRefreshToken),
        AccessExpiry:    encryptedSession.AccessTokenExpiry,
        RefreshTokenRejected: encryptedSession.RefreshTokenRejected,
    }

    // Step 5: Publish success event
    s.eventPublisher.Publish(SessionDecrypted{
        SessionID: sessionID,
        ServiceID: encryptedSession.ServiceID,
        Timestamp: time.Now(),
    })

    return decryptedSession, nil
}
```

---

## 3. Error Handling

### Context Mismatch (Service_ID Mismatch)

```go
// If caller provides wrong service_id to Decrypt:
plaintext, err := s.encryptionPort.Decrypt(ctx, ciphertext, map[string]string{
    "service_id": "wrong_service", // Doesn't match encryption
})
if err != nil {
    // Error: context_mismatch (failed at DEK or KEK verification layer)
    // Action: Fail request, log sanitized error, publish event
    return fmt.Errorf("context verification failed: %w", err)
}
```

### KEK Unavailable

```go
// If AWS KMS is unreachable:
plaintext, err := s.encryptionPort.Decrypt(ctx, ciphertext, encryptionContext)
if err != nil {
    // Error: kek_unavailable (AWS KMS connection failed)
    // Action: Fail request, log (no key material), retry with backoff if appropriate
    return fmt.Errorf("key management service unreachable: %w", err)
}
```

### Integrity Violation (Tampered Ciphertext)

```go
// If ciphertext is corrupted:
plaintext, err := s.encryptionPort.Decrypt(ctx, tampered_ciphertext, context)
if err != nil {
    // Error: integrity_violation (AESGCMSIV auth tag verification failed)
    // Action: Fail request, alert security team (possible tampering)
    return fmt.Errorf("token integrity verification failed: %w", err)
}
```

---

## 4. Domain Events

### Subscribe to Encryption Events

```go
// Listen for successful encryption
eventBus.Subscribe("SessionEncrypted", func(event SessionEncrypted) {
    logger.Info("session encrypted",
        "session_id", event.SessionID,
        "service_id", event.ServiceID,
    )
})

// Listen for encryption failures
eventBus.Subscribe("SessionEncryptionFailed", func(event SessionEncryptionFailed) {
    logger.Error("encryption failed",
        "session_id", event.SessionID,
        "error_kind", event.ErrorKind,
    )
    metrics.IncrementCounter("encryption_failures_total")
})

// Listen for successful decryption
eventBus.Subscribe("SessionDecrypted", func(event SessionDecrypted) {
    logger.Info("session decrypted",
        "session_id", event.SessionID,
        "service_id", event.ServiceID,
    )
})

// Listen for decryption failures
eventBus.Subscribe("SessionDecryptionFailed", func(event SessionDecryptionFailed) {
    logger.Error("decryption failed",
        "session_id", event.SessionID,
        "error_kind", event.ErrorKind,
    )
    metrics.IncrementCounter("decryption_failures_total")
})
```

---

## 5. Application Builder Integration

### Wire EncryptionPort into Service

```go
type AppBuilder struct {
    config Config
    // ... other fields
}

func (b *AppBuilder) buildEncryptionPort() (ports.EncryptionPort, error) {
    keyMaterial := b.config.EncryptionKeyEncryptionKey

    // Auto-detect: AWS KMS ARN or environment variable
    if strings.HasPrefix(keyMaterial, "arn:aws:kms:") {
        // Use AWS KMS
        return adapters.NewAWSEncryptionAdapter(keyMaterial)
    } else if strings.HasPrefix(keyMaterial, "${") {
        // Use environment variable (interpolated by config system)
        return adapters.NewAWSEncryptionAdapter(keyMaterial)
    }

    return nil, fmt.Errorf("invalid KEK configuration")
}

func (b *AppBuilder) buildOAuth2SessionService(
    repo UserSessionRepository,
) (*services.OAuth2SessionService, error) {
    port, err := b.buildEncryptionPort()
    if err != nil {
        return nil, err
    }

    return services.NewOAuth2SessionService(repo, port, b.eventPublisher), nil
}
```

---

## 6. Testing

### Unit Test: Encrypt/Decrypt Roundtrip

```go
func TestEncryptDecryptRoundtrip(t *testing.T) {
    port := adapters.NewAWSEncryptionAdapter(testKEK)

    plaintext := []byte("test-token-12345")
    context := map[string]string{"service_id": "oauth2"}

    // Encrypt
    ciphertext, err := port.Encrypt(context.Background(), plaintext, context)
    require.NoError(t, err)
    require.NotEqual(t, plaintext, ciphertext)

    // Decrypt
    decrypted, err := port.Decrypt(context.Background(), ciphertext, context)
    require.NoError(t, err)
    require.Equal(t, plaintext, decrypted)
}
```

### E2E Test: Service → Repository Roundtrip

```go
func TestServiceEncryptionRoundtrip(t *testing.T) {
    repo := memory.NewUserSessionRepository()
    port := adapters.NewAWSEncryptionAdapter(testKEK)
    service := services.NewOAuth2SessionService(repo, port, noopPublisher)

    // Create with encryption
    session, err := service.CreateSession(
        context.Background(),
        "user@example.com",
        "oauth2",
        "access-token-xyz",
        "refresh-token-xyz",
        time.Now().Add(time.Hour),
    )
    require.NoError(t, err)
    require.NotNil(t, session)

    // Retrieve with decryption
    retrieved, err := service.GetSession(context.Background(), session.ID)
    require.NoError(t, err)
    require.Equal(t, "access-token-xyz", retrieved.AccessToken)
    require.Equal(t, "refresh-token-xyz", retrieved.RefreshToken)
}
```

### E2E Test: Context Mismatch Detection

```go
func TestContextMismatchDetection(t *testing.T) {
    port := adapters.NewAWSEncryptionAdapter(testKEK)

    plaintext := []byte("token")
    context := map[string]string{"service_id": "oauth2"}

    // Encrypt with oauth2 context
    ciphertext, err := port.Encrypt(context.Background(), plaintext, context)
    require.NoError(t, err)

    // Try to decrypt with different context (should fail)
    wrongContext := map[string]string{"service_id": "github"}
    _, err = port.Decrypt(context.Background(), ciphertext, wrongContext)
    require.Error(t, err)
    require.Contains(t, err.Error(), "context")
}
```

---

## 7. Monitoring & Logging

### Metrics to Track

```go
// Create metrics for encryption operations
metricsRegistry.Counter("encryption_operations_total",
    labels{"status": "success|failure", "operation": "encrypt|decrypt"})

metricsRegistry.Histogram("encryption_duration_seconds",
    labels{"operation": "encrypt|decrypt"})

metricsRegistry.Counter("context_verification_failures_total")

metricsRegistry.Counter("kek_unavailable_errors_total")
```

### Logging Best Practices

```go
// ✅ Good: Sanitized logging (no key material, no full tokens)
logger.Info("encryption operation",
    "session_id", sessionID,
    "service_id", serviceID,
    "operation", "encrypt",
    "duration_ms", elapsed,
)

// ❌ Bad: Never log these
logger.Error("encryption failed: %s", token)              // Full token
logger.Error("KEK: %s", keyEncryptionKey)                 // Key material
logger.Error("error details: %+v", err)                    // May leak values
```

---

## 8. Performance Considerations

### Target: <100ms per Operation (excl. KMS latency)

**Local Encryption (env var KEK)**: ~1-5ms

- DEK generation: <1ms
- Token encryption: <1ms
- Total: <5ms

**AWS KMS Encryption (network latency)**: 50-200ms

- Network roundtrip to KMS: 50-200ms (dominant cost)
- Wrapping operation: <1ms
- Total: 50-200ms (depends on network and KMS availability)

**Optimization Tips**:

- Use AWS SDK's built-in connection pooling
- Consider KMS request batching for high-throughput scenarios
- Use regional KMS endpoints (lower latency)
- Monitor KMS request latency; alert if > 500ms

---

**Version**: 1.0 | **Status**: Implementation Guide | **Last Updated**: 2026-01-15
