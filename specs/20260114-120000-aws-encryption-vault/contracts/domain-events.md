# Contract: Domain Events

**Location**: `internal/domain/storage/events.go`

**Purpose**: Defines domain events emitted by the encryption vault feature for audit, monitoring, and downstream subscriptions. Events provide observability into encryption operations and enable reactive workflows.

---

## Event Principles

1. **Published by Service Layer**: OAuth2SessionService publishes events after encryption/decryption operations
2. **Immutable**: Events represent facts that already happened (past tense naming)
3. **Sanitized**: Events contain only metadata (no plaintext tokens, no key material)
4. **Timestamped**: All events include timestamp for correlation with metrics/logs
5. **Traceable**: Events include session_id and service_id for audit trails

---

## Event Definitions

### SessionEncrypted

**When**: Tokens encrypted successfully and session stored

**Emitted**: After `repository.Create()` succeeds in OAuth2SessionService.CreateSession()

**Structure**:
```go
type SessionEncrypted struct {
    SessionID string    `json:"session_id"`
    Principal string    `json:"principal"`
    ServiceID string    `json:"service_id"`
    Timestamp time.Time `json:"timestamp"`
}
```

**Fields**:
- `SessionID` - Unique session identifier (UUID-like string)
- `Principal` - User/agent identifier (email, service account name)
- `ServiceID` - OAuth service identifier (e.g., "oauth2", "github", "google")
- `Timestamp` - When encryption completed (time.Now())

**Subscribers**:
- Audit logger (compliance: record all encryption operations)
- Metrics collector (success counter)
- Security monitoring (detect anomalies)

**Example**:
```go
event := SessionEncrypted{
    SessionID: "550e8400-e29b-41d4-a716-446655440000",
    Principal: "user@example.com",
    ServiceID: "oauth2",
    Timestamp: time.Now(),
}
```

**Publishing** (in OAuth2SessionService):
```go
func (s *OAuth2SessionService) CreateSession(
    ctx context.Context,
    principal, serviceID, accessToken, refreshToken string,
    accessExpiry, refreshExpiry time.Time,
) (*UserSession, error) {
    // ... encryption logic ...

    if err := s.repository.Create(ctx, session); err != nil {
        return nil, fmt.Errorf("failed to store session: %w", err)
    }

    // Publish event AFTER successful storage
    s.eventPublisher.Publish(SessionEncrypted{
        SessionID: session.ID,
        Principal: principal,
        ServiceID: serviceID,
        Timestamp: time.Now(),
    })

    return session, nil
}
```

**Logging** (subscriber example):
```
[INFO] Session encrypted
       session_id=550e8400-e29b-41d4-a716-446655440000
       principal=user@example.com
       service_id=oauth2
       timestamp=2026-01-16T10:30:45Z
```

**Metrics** (subscriber example):
```
encryption_operations_total{status="success",operation="encrypt"} +1
```

---

### SessionDecrypted

**When**: Tokens decrypted successfully and returned to caller

**Emitted**: After `port.Decrypt()` succeeds in OAuth2SessionService.GetSession()

**Structure**:
```go
type SessionDecrypted struct {
    SessionID string    `json:"session_id"`
    Principal string    `json:"principal"`
    ServiceID string    `json:"service_id"`
    Timestamp time.Time `json:"timestamp"`
}
```

**Fields**:
- `SessionID` - Unique session identifier
- `Principal` - User/agent identifier
- `ServiceID` - OAuth service identifier
- `Timestamp` - When decryption completed

**Subscribers**:
- Audit logger (compliance: record all decryption operations)
- Metrics collector (success counter)
- Access log (track token retrieval patterns)

**Example**:
```go
event := SessionDecrypted{
    SessionID: "550e8400-e29b-41d4-a716-446655440000",
    Principal: "user@example.com",
    ServiceID: "oauth2",
    Timestamp: time.Now(),
}
```

**Publishing** (in OAuth2SessionService):
```go
func (s *OAuth2SessionService) GetSession(
    ctx context.Context,
    sessionID string,
) (*UserSession, error) {
    encryptedSession, err := s.repository.Get(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve session: %w", err)
    }

    // ... decryption logic ...

    // Publish event AFTER successful decryption
    s.eventPublisher.Publish(SessionDecrypted{
        SessionID: encryptedSession.ID,
        Principal: encryptedSession.Principal,
        ServiceID: encryptedSession.ServiceID,
        Timestamp: time.Now(),
    })

    return decryptedSession, nil
}
```

**Logging** (subscriber example):
```
[INFO] Session decrypted
       session_id=550e8400-e29b-41d4-a716-446655440000
       principal=user@example.com
       service_id=oauth2
       timestamp=2026-01-16T10:30:46Z
```

---

### SessionEncryptionFailed

**When**: Encryption operation fails; session NOT stored

**Emitted**: Inside error handler in OAuth2SessionService.CreateSession(), BEFORE returning error

**Structure**:
```go
type SessionEncryptionFailed struct {
    SessionID string    `json:"session_id"`
    Principal string    `json:"principal"`
    ServiceID string    `json:"service_id"`
    ErrorKind string    `json:"error_kind"`
    Message   string    `json:"message"` // Sanitized
    Timestamp time.Time `json:"timestamp"`
}
```

**Fields**:
- `SessionID` - Session ID (may be empty if not yet assigned)
- `Principal` - User/agent attempting to create session
- `ServiceID` - OAuth service identifier
- `ErrorKind` - Machine-readable error category (from encryption.ErrorKind)
- `Message` - Human-readable error message (sanitized, no key material or full tokens)
- `Timestamp` - When encryption failed

**ErrorKind Values**:
- `"encryption_failed"` - DEK generation or AESGCMSIV encryption failed
- `"kek_unavailable"` - Key Encryption Key not accessible

**Subscribers**:
- Alert manager (critical failures warrant alerts)
- Metrics collector (failure counter)
- Audit logger (compliance: record failures)
- Support team (incident investigation)

**Example**:
```go
event := SessionEncryptionFailed{
    SessionID: "",  // Not yet assigned
    Principal: "user@example.com",
    ServiceID: "oauth2",
    ErrorKind: "kek_unavailable",
    Message:   "KEK unavailable: AWS KMS connection failed",
    Timestamp: time.Now(),
}
```

**Publishing** (in OAuth2SessionService):
```go
func (s *OAuth2SessionService) CreateSession(
    ctx context.Context,
    principal, serviceID, accessToken, refreshToken string,
    accessExpiry, refreshExpiry time.Time,
) (*UserSession, error) {
    // Encrypt access token
    encryptedAccess, err := s.encryptionPort.Encrypt(ctx, []byte(accessToken), context)
    if err != nil {
        // Publish failure event BEFORE returning error
        s.eventPublisher.Publish(SessionEncryptionFailed{
            SessionID: "",  // Not yet assigned
            Principal: principal,
            ServiceID: serviceID,
            ErrorKind: extractErrorKind(err),
            Message:   "failed to encrypt access token",
            Timestamp: time.Now(),
        })
        return nil, fmt.Errorf("failed to encrypt access token: %w", err)
    }

    // ... rest of encryption logic ...
}
```

**Logging** (subscriber example):
```
[ERROR] Session encryption failed
        principal=user@example.com
        service_id=oauth2
        error_kind=kek_unavailable
        message=AWS KMS connection failed
        timestamp=2026-01-16T10:30:47Z
```

**Metrics** (subscriber example):
```
encryption_operations_total{status="failure",operation="encrypt"} +1
encryption_failures_total{error_kind="kek_unavailable"} +1
```

---

### SessionDecryptionFailed

**When**: Decryption operation fails; session NOT returned to caller

**Emitted**: Inside error handler in OAuth2SessionService.GetSession(), BEFORE returning error

**Structure**:
```go
type SessionDecryptionFailed struct {
    SessionID string    `json:"session_id"`
    Principal string    `json:"principal"`
    ServiceID string    `json:"service_id"`
    ErrorKind string    `json:"error_kind"`
    Message   string    `json:"message"` // Sanitized
    Timestamp time.Time `json:"timestamp"`
}
```

**Fields**:
- `SessionID` - Session ID (known since we retrieved from storage)
- `Principal` - User/agent attempting to retrieve session
- `ServiceID` - OAuth service identifier
- `ErrorKind` - Machine-readable error category
- `Message` - Sanitized error message
- `Timestamp` - When decryption failed

**ErrorKind Values**:
- `"decryption_failed"` - DEK unwrapping or AESGCMSIV decryption failed
- `"context_mismatch"` - Service context doesn't match (possible cross-service access)
- `"integrity_violation"` - Auth tag verification failed (possible tampering)
- `"kek_unavailable"` - Key Encryption Key not accessible

**Subscribers**:
- Alert manager (integrity violations warrant immediate alerts)
- Metrics collector (failure counter)
- Audit logger (compliance: record all failures)
- Security team (integrity violations indicate possible attack)

**Example**:
```go
event := SessionDecryptionFailed{
    SessionID: "550e8400-e29b-41d4-a716-446655440000",
    Principal: "user@example.com",
    ServiceID: "oauth2",
    ErrorKind: "integrity_violation",
    Message:   "token integrity verification failed (possible tampering)",
    Timestamp: time.Now(),
}
```

**Publishing** (in OAuth2SessionService):
```go
func (s *OAuth2SessionService) GetSession(
    ctx context.Context,
    sessionID string,
) (*UserSession, error) {
    encryptedSession, err := s.repository.Get(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve session: %w", err)
    }

    // Decrypt access token
    plainAccessToken, err := s.encryptionPort.Decrypt(ctx, encryptedSession.EncryptedAccessToken, context)
    if err != nil {
        // Publish failure event BEFORE returning error
        s.eventPublisher.Publish(SessionDecryptionFailed{
            SessionID: encryptedSession.ID,
            Principal: encryptedSession.Principal,
            ServiceID: encryptedSession.ServiceID,
            ErrorKind: extractErrorKind(err),
            Message:   "failed to decrypt access token",
            Timestamp: time.Now(),
        })
        return nil, fmt.Errorf("failed to decrypt access token: %w", err)
    }

    // ... rest of decryption logic ...
}
```

**Logging** (subscriber example):
```
[ALERT] Session decryption failed
        session_id=550e8400-e29b-41d4-a716-446655440000
        principal=user@example.com
        service_id=oauth2
        error_kind=integrity_violation
        message=token integrity verification failed (possible tampering)
        timestamp=2026-01-16T10:30:48Z
```

**Metrics** (subscriber example):
```
encryption_operations_total{status="failure",operation="decrypt"} +1
decryption_failures_total{error_kind="integrity_violation"} +1
security_alerts_total{alert_type="integrity_violation"} +1
```

---

## Event Publishing Pattern

### Service Layer Publishing

```go
// In OAuth2SessionService
type OAuth2SessionService struct {
    repository     UserSessionRepository
    encryptionPort ports.EncryptionPort
    eventPublisher EventPublisher  // Injected dependency
}

// Events emitted at key points:

// 1. After successful encryption and storage
s.eventPublisher.Publish(SessionEncrypted{...})

// 2. After successful decryption and retrieval
s.eventPublisher.Publish(SessionDecrypted{...})

// 3. On encryption failure (before returning error)
s.eventPublisher.Publish(SessionEncryptionFailed{...})

// 4. On decryption failure (before returning error)
s.eventPublisher.Publish(SessionDecryptionFailed{...})
```

### EventPublisher Interface

```go
type EventPublisher interface {
    Publish(event interface{}) error
}
```

**Implementation Options**:
- In-memory event bus (local subscribers)
- Message broker (Kafka, RabbitMQ for distributed subscribers)
- Event sourcing (append to event store)

---

## Event Subscription Examples

### Audit Logger Subscriber

```go
eventBus.Subscribe(SessionEncrypted{}, func(event SessionEncrypted) {
    logger.Info("session encrypted",
        "session_id", event.SessionID,
        "principal", event.Principal,
        "service_id", event.ServiceID,
        "timestamp", event.Timestamp,
    )
})

eventBus.Subscribe(SessionEncrypted{}, func(event SessionEncrypted) {
    logger.Info("session decrypted",
        "session_id", event.SessionID,
        "principal", event.Principal,
        "service_id", event.ServiceID,
        "timestamp", event.Timestamp,
    )
})

eventBus.Subscribe(SessionEncryptionFailed{}, func(event SessionEncryptionFailed) {
    logger.Error("session encryption failed",
        "session_id", event.SessionID,
        "error_kind", event.ErrorKind,
    )
})

eventBus.Subscribe(SessionDecryptionFailed{}, func(event SessionDecryptionFailed) {
    logger.Error("session decryption failed",
        "session_id", event.SessionID,
        "error_kind", event.ErrorKind,
    )
})
```

### Metrics Collector Subscriber

```go
eventBus.Subscribe(SessionEncrypted{}, func(event SessionEncrypted) {
    metricsRegistry.Counter("encryption_operations_total",
        labels{"status": "success", "operation": "encrypt"}).Inc()
})

eventBus.Subscribe(SessionDecrypted{}, func(event SessionDecrypted) {
    metricsRegistry.Counter("encryption_operations_total",
        labels{"status": "success", "operation": "decrypt"}).Inc()
})

eventBus.Subscribe(SessionEncryptionFailed{}, func(event SessionEncryptionFailed) {
    metricsRegistry.Counter("encryption_operations_total",
        labels{"status": "failure", "operation": "encrypt"}).Inc()
    metricsRegistry.Counter("encryption_failures_total",
        labels{"error_kind": event.ErrorKind}).Inc()
})

eventBus.Subscribe(SessionDecryptionFailed{}, func(event SessionDecryptionFailed) {
    metricsRegistry.Counter("encryption_operations_total",
        labels{"status": "failure", "operation": "decrypt"}).Inc()
    metricsRegistry.Counter("decryption_failures_total",
        labels{"error_kind": event.ErrorKind}).Inc()

    if event.ErrorKind == "integrity_violation" {
        metricsRegistry.Counter("security_alerts_total",
            labels{"alert_type": "integrity_violation"}).Inc()
    }
})
```

### Security Alert Subscriber

```go
eventBus.Subscribe(SessionDecryptionFailed{}, func(event SessionDecryptionFailed) {
    if event.ErrorKind == "integrity_violation" {
        logger.Alert("SECURITY: Token integrity violation",
            "session_id", event.SessionID,
            "service_id", event.ServiceID,
        )
        securityAlertService.SendAlert("token_tampering_detected", map[string]string{
            "session_id": event.SessionID,
            "service_id": event.ServiceID,
            "timestamp":  event.Timestamp.String(),
        })
    }

    if event.ErrorKind == "context_mismatch" {
        logger.Warn("Context mismatch (possible cross-service access attempt)",
            "session_id", event.SessionID,
            "service_id", event.ServiceID,
        )
    }
})
```

---

## Event Sequencing

### Successful Create Flow

```
1. CreateSession(principal, serviceID, accessToken, refreshToken)
   ↓
2. [Encrypt access token via port.Encrypt()]
   ↓
3. [Encrypt refresh token via port.Encrypt()]
   ↓
4. [Create UserSession with encrypted tokens]
   ↓
5. repository.Create(session)
   ↓
6. ✅ Publish SessionEncrypted
   ↓
7. Return session to caller
```

### Successful Get Flow

```
1. GetSession(sessionID)
   ↓
2. repository.Get(sessionID) → encryptedSession
   ↓
3. [Decrypt access token via port.Decrypt()]
   ↓
4. [Decrypt refresh token via port.Decrypt()]
   ↓
5. ✅ Publish SessionDecrypted
   ↓
6. Return decrypted session to caller
```

### Failed Create Flow

```
1. CreateSession(principal, serviceID, accessToken, refreshToken)
   ↓
2. [Attempt encrypt access token via port.Encrypt()]
   ↓
3. ❌ Encryption fails
   ↓
4. ❌ Publish SessionEncryptionFailed
   ↓
5. Return error to caller
   ↓
6. Session NOT stored
```

### Failed Get Flow with Integrity Violation

```
1. GetSession(sessionID)
   ↓
2. repository.Get(sessionID) → encryptedSession
   ↓
3. [Attempt decrypt access token via port.Decrypt()]
   ↓
4. ❌ Auth tag verification fails (ciphertext tampered)
   ↓
5. ❌ Publish SessionDecryptionFailed (error_kind="integrity_violation")
   ↓
6. 🚨 Security alert sent
   ↓
7. Return error to caller
```

---

## Event Testing

### Unit Test: SessionEncrypted Event

```go
func TestSessionEncryptedEventPublished(t *testing.T) {
    repo := memory.NewUserSessionRepository()
    port := adapters.NewAWSEncryptionAdapter(testKEK)
    mockPublisher := &MockEventPublisher{}
    service := services.NewOAuth2SessionService(repo, port, mockPublisher)

    // Create session
    session, err := service.CreateSession(
        context.Background(),
        "user@example.com",
        "oauth2",
        "access-token",
        "refresh-token",
        time.Now().Add(time.Hour),
        time.Now().Add(24*time.Hour),
    )

    require.NoError(t, err)

    // Verify SessionEncrypted event was published
    require.Len(t, mockPublisher.PublishedEvents, 1)
    event := mockPublisher.PublishedEvents[0].(SessionEncrypted)
    require.Equal(t, session.ID, event.SessionID)
    require.Equal(t, "user@example.com", event.Principal)
    require.Equal(t, "oauth2", event.ServiceID)
}
```

### Unit Test: SessionEncryptionFailed Event

```go
func TestSessionEncryptionFailedEventPublished(t *testing.T) {
    repo := memory.NewUserSessionRepository()
    port := &FailingEncryptionPort{}  // Always fails
    mockPublisher := &MockEventPublisher{}
    service := services.NewOAuth2SessionService(repo, port, mockPublisher)

    // Attempt to create session
    _, err := service.CreateSession(
        context.Background(),
        "user@example.com",
        "oauth2",
        "access-token",
        "refresh-token",
        time.Now().Add(time.Hour),
        time.Now().Add(24*time.Hour),
    )

    require.Error(t, err)

    // Verify SessionEncryptionFailed event was published
    require.Len(t, mockPublisher.PublishedEvents, 1)
    event := mockPublisher.PublishedEvents[0].(SessionEncryptionFailed)
    require.Equal(t, "user@example.com", event.Principal)
    require.Equal(t, "oauth2", event.ServiceID)
    require.Equal(t, "encryption_failed", event.ErrorKind)
}
```

---

**Version**: 1.0 | **Status**: Design Phase | **Last Updated**: 2026-01-16
