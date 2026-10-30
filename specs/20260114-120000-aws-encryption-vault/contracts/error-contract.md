# Contract: Error Types and Handling

**Location**: `internal/domain/encryption/errors.go`

**Purpose**: Defines error types, categorization, and handling patterns for encryption operations. Enables fail-closed security with clear error diagnostics.

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

---

## Error Type Definitions

### ErrorKind Enumeration

```go
package encryption

type ErrorKind string

const (
    ErrorKindEncryptionFailed   ErrorKind = "encryption_failed"
    ErrorKindDecryptionFailed   ErrorKind = "decryption_failed"
    ErrorKindContextMismatch    ErrorKind = "context_mismatch"
    ErrorKindIntegrityViolation ErrorKind = "integrity_violation"
    ErrorKindKEKUnavailable     ErrorKind = "kek_unavailable"
)
```

---

## Error Structure

```go
type EncryptionError struct {
    Kind    ErrorKind
    Message string // Sanitized (no key material, no full tokens)
    Wrapped error  // Underlying error from AWS SDK
}

// Error implements the error interface
func (e *EncryptionError) Error() string {
    return fmt.Sprintf("[%s] %s", e.Kind, e.Message)
}

// Unwrap enables error wrapping chains
func (e *EncryptionError) Unwrap() error {
    return e.Wrapped
}

// Is enables error type comparison with errors.Is()
func (e *EncryptionError) Is(target error) bool {
    if t, ok := target.(*EncryptionError); ok {
        return e.Kind == t.Kind
    }
    return false
}
```

---

## Error Categories

### 1. ErrorKindEncryptionFailed

**When**: Encryption operation fails during token encryption

**Causes**:

- DEK generation failure (insufficient entropy)
- AESGCMSIV encryption failure
- AWS SDK internal error
- Memory allocation failure
- Context timeout

**Example**:

```go
EncryptionError{
    Kind:    ErrorKindEncryptionFailed,
    Message: "failed to encrypt token: DEK generation failed",
    Wrapped: originalError,
}
```

**Service Layer Handling**:

```go
_, err := s.encryptionPort.Encrypt(ctx, token, context)
if err != nil {
    if errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindEncryptionFailed}) {
        s.eventPublisher.Publish(SessionEncryptionFailed{
            ServiceID: serviceID,
            ErrorKind: "encryption_failed",
            Timestamp: time.Now(),
        })
        return nil, fmt.Errorf("failed to encrypt token: %w", err)
    }
    return nil, err
}
```

**Logging**:

```
[ERROR] Session encryption failed
        session_id=<id>
        service_id=oauth2
        error_kind=encryption_failed
```

**Action**: Fail request immediately; do not retry (likely persistent error)

---

### 2. ErrorKindDecryptionFailed

**When**: Decryption operation fails during token retrieval

**Causes**:

- DEK unwrapping failure (corrupted wrapped DEK)
- AESGCMSIV decryption failure
- AWS KMS operation failure
- AWS SDK internal error
- Memory allocation failure
- Context timeout

**Example**:

```go
EncryptionError{
    Kind:    ErrorKindDecryptionFailed,
    Message: "failed to decrypt token: DEK unwrapping failed",
    Wrapped: originalError,
}
```

**Service Layer Handling**:

```go
plaintext, err := s.encryptionPort.Decrypt(ctx, ciphertext, context)
if err != nil {
    if errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindDecryptionFailed}) {
        s.eventPublisher.Publish(SessionDecryptionFailed{
            ServiceID: serviceID,
            ErrorKind: "decryption_failed",
            Timestamp: time.Now(),
        })
        return nil, fmt.Errorf("failed to decrypt token: %w", err)
    }
    return nil, err
}
```

**Logging**:

```
[ERROR] Session decryption failed
        session_id=<id>
        service_id=oauth2
        error_kind=decryption_failed
```

**Action**: Fail request; do not return plaintext; consider retry with backoff if KMS timeout

---

### 3. ErrorKindContextMismatch

**When**: Decryption context doesn't match encryption context (AAD verification failure)

**Causes**:

- Wrong `service_id` provided to Decrypt
- Ciphertext encrypted with different service, now attempting cross-service decryption
- Context binding enforces service isolation

**Example**:

```go
// Encrypted with service_id="oauth2"
ciphertext, _ := port.Encrypt(ctx, token, map[string]string{"service_id": "oauth2"})

// Attempted decryption with service_id="github" (FAILS)
plaintext, err := port.Decrypt(ctx, ciphertext, map[string]string{"service_id": "github"})
// Returns: EncryptionError{Kind: ErrorKindContextMismatch}
```

**Service Layer Handling**:

```go
plaintext, err := s.encryptionPort.Decrypt(ctx, ciphertext, wrongContext)
if err != nil {
    if errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindContextMismatch}) {
        s.logger.Warn("context mismatch",
            "session_id", sessionID,
            "expected_service", expectedService,
            "provided_service", providedService,
        )
        s.eventPublisher.Publish(SessionDecryptionFailed{
            SessionID: sessionID,
            ServiceID: providedService,
            ErrorKind: "context_mismatch",
        })
        return nil, fmt.Errorf("token context verification failed: %w", err)
    }
    return nil, err
}
```

**Logging**:

```
[WARN] Context mismatch detected (security event)
       session_id=<id>
       expected_service_id=oauth2
       provided_service_id=github
```

**Action**: Fail request; log security event; consider alerting (possible attack or misconfiguration)

---

### 4. ErrorKindIntegrityViolation

**When**: AESGCMSIV authentication tag verification fails (ciphertext tampered)

**Causes**:

- Ciphertext corrupted in storage or transit
- Bit flip in BYTEA column
- Intentional tampering attempt
- Memory corruption during transmission

**Example**:

```go
// Original ciphertext
ciphertext, _ := port.Encrypt(ctx, token, context)

// Ciphertext modified (bit flip)
tampered := modifyBits(ciphertext)

// Attempted decryption of tampered ciphertext
plaintext, err := port.Decrypt(ctx, tampered, context)
// Returns: EncryptionError{Kind: ErrorKindIntegrityViolation}
```

**Service Layer Handling**:

```go
plaintext, err := s.encryptionPort.Decrypt(ctx, ciphertext, context)
if err != nil {
    if errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindIntegrityViolation}) {
        s.logger.Error("integrity violation (possible tampering)",
            "session_id", sessionID,
            "service_id", serviceID,
        )
        s.eventPublisher.Publish(SessionDecryptionFailed{
            SessionID: sessionID,
            ServiceID: serviceID,
            ErrorKind: "integrity_violation",
        })
        // ALERT SECURITY TEAM
        s.securityAlertService.Alert("possible_token_tampering", sessionID)
        return nil, fmt.Errorf("token integrity verification failed: %w", err)
    }
    return nil, err
}
```

**Logging**:

```
[ALERT] Token integrity violation detected (SECURITY EVENT)
        session_id=<id>
        service_id=oauth2
        action=security_team_notified
```

**Action**: Fail request immediately; alert security team; consider invalidating token; investigate storage integrity

---

### 5. ErrorKindKEKUnavailable

**When**: Key Encryption Key is not accessible

**Causes** (AWS KMS):

- AWS KMS service unavailable
- Network connectivity failure
- IAM permissions missing (`kms:Decrypt`, `kms:GenerateDataKey`)
- KMS key deleted or disabled
- AWS credentials expired

**Causes** (Environment Variable):

- ENCRYPTION_KEK environment variable not set at initialization
- Key material corrupted or invalid

**Example**:

```go
// AWS KMS unreachable
adapter := NewAWSEncryptionAdapter("arn:aws:kms:us-east-1:123456789012:key/...")
plaintext, err := adapter.Decrypt(ctx, ciphertext, context)
// Returns: EncryptionError{Kind: ErrorKindKEKUnavailable}
```

**Service Layer Handling**:

```go
plaintext, err := s.encryptionPort.Decrypt(ctx, ciphertext, context)
if err != nil {
    if errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindKEKUnavailable}) {
        s.logger.Error("KEK unavailable",
            "session_id", sessionID,
            "service_id", serviceID,
            "retry_after_ms", 5000,
        )
        s.eventPublisher.Publish(SessionDecryptionFailed{
            SessionID: sessionID,
            ServiceID: serviceID,
            ErrorKind: "kek_unavailable",
        })
        s.metricsRegistry.Counter("kek_unavailable_errors_total").Inc()

        // Caller may retry with backoff (e.g., exponential backoff)
        return nil, fmt.Errorf("encryption key unavailable (try again later): %w", err)
    }
    return nil, err
}
```

**Logging**:

```
[ERROR] KEK unavailable
        error_source=aws_kms
        kms_endpoint=https://kms.us-east-1.amazonaws.com
        last_error=connection_timeout
        retry_after_seconds=5
```

**Action**: Fail request with retryable status (HTTP 503 Service Unavailable); implement exponential backoff retry

---

## Error Handling Patterns

### Service Layer Error Wrapping

```go
// Pattern 1: Direct error check with errors.Is()
if err := port.Encrypt(ctx, data, context); err != nil {
    var encErr *encryption.EncryptionError
    if errors.As(err, &encErr) {
        switch encErr.Kind {
        case encryption.ErrorKindEncryptionFailed:
            // Handle encryption failure
        case encryption.ErrorKindKEKUnavailable:
            // Handle KEK unavailable (retryable)
        default:
            // Handle other errors
        }
    }
    return err
}

// Pattern 2: Named return with custom error
plaintext, err := port.Decrypt(ctx, ciphertext, context)
if err != nil {
    return nil, fmt.Errorf("session decryption failed: %w", err)
}

// Pattern 3: Check error kind then log
if err := port.Encrypt(ctx, data, context); err != nil {
    if errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindContextMismatch}) {
        log.Warn("context mismatch (security event)")
        metrics.Inc("context_mismatch_total")
    }
    return err
}
```

### HTTP Mapping

| Error Kind | HTTP Status | Caller Action |
|------------|------------|---------------|
| `encryption_failed` | 500 Internal Server Error | Retry (likely transient) |
| `decryption_failed` | 500 Internal Server Error | Retry (likely transient) |
| `context_mismatch` | 403 Forbidden | Do not retry (token invalid for this service) |
| `integrity_violation` | 403 Forbidden | Do not retry; alert security (tampering) |
| `kek_unavailable` | 503 Service Unavailable | Retry with backoff (transient) |

---

## Error Logging Best Practices

### What TO Log (Sanitized)

```go
logger.Error("encryption operation failed",
    "session_id", sessionID,           // ✅ Safe: opaque ID
    "service_id", serviceID,            // ✅ Safe: service name
    "error_kind", err.ErrorKind(),      // ✅ Safe: error category
    "duration_ms", elapsed,             // ✅ Safe: timing
)
```

### What NOT TO Log (NEVER)

```go
logger.Error(fmt.Sprintf("encryption failed: %+v", err))  // ❌ May leak key material
logger.Error("KEK: %s", keyMaterial)                      // ❌ NEVER log KEK
logger.Error("token: %s", token)                          // ❌ NEVER log full token
logger.Error("ciphertext: %x", ciphertext)                // ❌ May leak ciphertext
logger.Error("plaintext: %s", plaintext)                  // ❌ NEVER log plaintext
```

---

## Error Monitoring & Metrics

### Metrics to Track

```go
// Track encryption success/failure
metricsRegistry.Counter("encryption_operations_total",
    labels{"status": "success|failure", "operation": "encrypt|decrypt"})

// Track encryption latency
metricsRegistry.Histogram("encryption_duration_seconds",
    labels{"operation": "encrypt|decrypt"})

// Track error kinds
metricsRegistry.Counter("encryption_errors_total",
    labels{"error_kind": "encryption_failed|decryption_failed|context_mismatch|integrity_violation|kek_unavailable"})

// Track context verification failures
metricsRegistry.Counter("context_verification_failures_total")

// Track KEK unavailability
metricsRegistry.Counter("kek_unavailable_errors_total")
```

### Alert Rules

| Alert | Condition | Severity | Action |
|-------|-----------|----------|--------|
| High Error Rate | >5% encryption failures over 5 min | Critical | Page on-call; investigate KEK/AWS KMS |
| Context Mismatch Spike | >10 mismatches/min | Warning | Investigate application logic or security |
| Integrity Violation | Any integrity violation | Critical | Alert security team; begin forensics |
| KEK Unavailable | >10 consecutive failures | Critical | Page on-call; verify AWS KMS access |
| Latency Spike | Encryption latency >500ms | Warning | Check KMS performance; network latency |

---

## Testing Error Scenarios

### Unit Test: Context Mismatch

```go
func TestContextMismatchDetection(t *testing.T) {
    adapter := NewAWSEncryptionAdapter(testKEK)

    plaintext := []byte("test-token")
    encContext := map[string]string{"service_id": "oauth2"}

    // Encrypt with oauth2 context
    ciphertext, err := adapter.Encrypt(context.Background(), plaintext, encContext)
    require.NoError(t, err)

    // Attempt decryption with different context
    wrongContext := map[string]string{"service_id": "github"}
    _, err = adapter.Decrypt(context.Background(), ciphertext, wrongContext)

    require.Error(t, err)
    require.True(t, errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindContextMismatch}))
}
```

### Unit Test: Integrity Violation

```go
func TestIntegrityViolationDetection(t *testing.T) {
    adapter := NewAWSEncryptionAdapter(testKEK)

    plaintext := []byte("test-token")
    context := map[string]string{"service_id": "oauth2"}

    // Encrypt
    ciphertext, err := adapter.Encrypt(context.Background(), plaintext, context)
    require.NoError(t, err)

    // Tamper with ciphertext (flip a bit)
    tampered := make([]byte, len(ciphertext))
    copy(tampered, ciphertext)
    tampered[len(tampered)-1] ^= 0x01

    // Attempt decryption of tampered ciphertext
    _, err = adapter.Decrypt(context.Background(), tampered, context)

    require.Error(t, err)
    require.True(t, errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindIntegrityViolation}))
}
```

### Unit Test: KEK Unavailable

```go
func TestKEKUnavailable(t *testing.T) {
    // Create adapter with invalid KMS ARN
    _, err := NewAWSEncryptionAdapter("arn:aws:kms:us-east-1:999999999999:key/invalid")

    require.Error(t, err)
    require.True(t, errors.Is(err, &encryption.EncryptionError{Kind: encryption.ErrorKindKEKUnavailable}))
}
```

---

**Version**: 1.0 | **Status**: Design Phase | **Last Updated**: 2026-01-16
