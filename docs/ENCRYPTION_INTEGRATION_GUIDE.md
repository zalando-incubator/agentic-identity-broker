# Encryption Integration Guide

This guide explains how applications use the Agentic Identity Broker encryption vault.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Service Integration](#service-integration)
- [Error Handling](#error-handling)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

## Overview

The encryption vault encrypts OAuth2 tokens at rest with envelope encryption. Each token uses:

1. **DEK (Data Encryption Key):** A new 256-bit AES key for one token.
2. **KEK (Key Encryption Key):** A master key in AWS KMS or an environment variable.
3. **Context binding:** Service isolation that prevents token reuse between services.

`OAuth2SessionService` encrypts tokens before storage and decrypts them on retrieval. Storage
adapters receive only ciphertext.

## Quick Start

### 1. Configure Encryption

**For Production (AWS KMS):**

```yaml
# config.yaml
encryption:
  aws_kms:
    key_arn: ${IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN}
```

**For Development (Memory backend):**

`.env.local` does not evaluate shell command substitution. Paste a generated base64 key when
you edit this file.

```dotenv
# .env.local
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=base64-encoded-32-byte-key
```

```bash
# shell
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(openssl rand -base64 32)"
```

```yaml
# config.yaml
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
```

### 2. Initialize Application

The application builder initializes encryption with the configured backend:

```go
// From internal/app/builder.go - automatic initialization
encryptor, branchKeyManager, err := awsencryption.NewEncryptionAdapter(&config.Encryption)
if err != nil {
    return nil, fmt.Errorf("failed to initialize encryption adapter: %w", err)
}
```

### 3. Use OAuth2SessionService

The service layer encrypts and decrypts tokens without application code:

```go
// Creating a session (tokens automatically encrypted by service)
session, err := oauth2Service.CreateSessionFromCallback(
    ctx,
    principal,
    &HandleCallbackRequest{
        ServiceID: "github",
        Code:      authCode,
        State:     stateToken,
    },
)
// ✅ Service encrypts tokens before storing in database
// ✅ Database receives encrypted ciphertext only

// Retrieving a session (tokens automatically decrypted by service)
session, err := oauth2Service.GetSessionWithAgents(ctx, principal, "github")
// ✅ Service decrypts tokens after retrieving from database
// ✅ Application receives plaintext tokens
```

## Configuration

### AWS KMS Setup (Production)

#### Step 1: Create KMS Key

```bash
# Create customer-managed key
aws kms create-key \
  --description "Identity Broker OAuth Token Encryption Key" \
  --region eu-central-1
```

Output example:

```
KeyId: arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012
```

#### Step 2: Create Alias

```bash
aws kms create-alias \
  --alias-name "alias/identity-broker-encryption" \
  --target-key-id "arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012"
```

#### Step 3: Grant Service Permissions

```bash
aws kms create-grant \
  --key-id "arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012" \
  --grantee-principal "arn:aws:iam::123456789012:role/IdentityBrokerRole" \
  --operations "Encrypt" "Decrypt" "GenerateDataKey" "DescribeKey"
```

#### Step 4: Configure Identity Broker

```yaml
# config.yaml
encryption:
  aws_kms:
    key_arn: "arn:aws:kms:eu-central-1:123456789012:alias/identity-broker-encryption"
    dynamodb_table_name: "IdentityBrokerEncryptionBranchKeys"
    branch_key_ttl: "1h"
```

### Environment Variable Setup (Development)

#### Step 1: Generate Key

```bash
# Using OpenSSL
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=$(openssl rand -base64 32)

# Using Python
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=$(python3 -c "import os, base64; print(base64.b64encode(os.urandom(32)).decode())")

# Using Go
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=$(go run -c 'package main; import ("crypto/rand"; "encoding/base64"; "fmt"; "os"); func main() { key := make([]byte, 32); rand.Read(key); fmt.Println(base64.StdEncoding.EncodeToString(key)) }')
```

#### Step 2: Configure Identity Broker

```dotenv
# .env.local
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=base64-encoded-32-byte-key
```

```bash
# shell
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(openssl rand -base64 32)"
```

```yaml
# config.yaml
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
```

## Service Integration

### OAuth2SessionService Integration

`OAuth2SessionService` encrypts and decrypts tokens automatically:

```go
// Service method that automatically encrypts tokens
func (s *OAuth2SessionService) CreateSessionFromCallback(
    ctx context.Context,
    principal string,
    req *HandleCallbackRequest,
) (*storage.UserSession, error) {
    // ... exchange code for tokens ...

    // Encryption context for this service
    encryptionContext := map[string]string{"service_id": serviceID}

    // Encrypt access token (transparent to caller)
    encryptedAccess, err := s.encryption.Encrypt(ctx, []byte(token.AccessToken), encryptionContext)

    // Encrypt refresh token
    encryptedRefresh, err := s.encryption.Encrypt(ctx, []byte(token.RefreshToken), encryptionContext)

    // Create session with encrypted tokens
    session := &storage.UserSession{
        EncryptedAccessToken: encryptedAccess,
        EncryptedRefreshToken: encryptedRefresh,
        // ... other fields ...
    }

    // Store to database - receives encrypted ciphertext
    return s.sessionRepo.Create(ctx, session)
}

// Service method that automatically decrypts tokens
func (s *OAuth2SessionService) GetSessionWithAgents(
    ctx context.Context,
    principal string,
    serviceID string,
) (*SessionWithAgents, error) {
    // Retrieve session from database
    session, err := s.sessionRepo.FindByPrincipalAndService(ctx, principal, serviceID)

    // Decryption context must match encryption context
    encryptionContext := session.EncryptionContext

    // Decrypt access token (transparent to caller)
    plainAccessToken, err := s.encryption.Decrypt(ctx, session.EncryptedAccessToken, encryptionContext)

    // Decrypt refresh token
    plainRefreshToken, err := s.encryption.Decrypt(ctx, session.EncryptedRefreshToken, encryptionContext)

    // Return session with plaintext tokens
    session.AccessToken = string(plainAccessToken)
    session.RefreshToken = string(plainRefreshToken)

    return &SessionWithAgents{Session: session, DependentAgents: agents}, nil
}
```

### Using EncryptionPort Interface Directly

For custom encryption operations, use `EncryptionPort` directly:

```go
// EncryptionPort interface
type EncryptionPort interface {
    Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error)
    Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error)
}

// Access from app builder
encryptionPort := app.EncryptionPort

// Encrypt a token
encryptionContext := map[string]string{"service_id": "github"}
ciphertext, err := encryptionPort.Encrypt(ctx, []byte(token), encryptionContext)
if err != nil {
    // Handle encryption error
}

// Decrypt a token
plaintext, err := encryptionPort.Decrypt(ctx, ciphertext, encryptionContext)
if err != nil {
    // Handle decryption error
}
```

## Error Handling

### Encryption Errors

The encryption port returns domain errors with error kinds:

```go
import "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"

_, err := encryptionPort.Encrypt(ctx, plaintext, encryptionContext)
if err != nil {
    // Check error kind
    switch err := err.(type) {
    case *encryption.EncryptionError:
        switch err.Kind {
        case encryption.ErrorKindContextMismatch:
            // Context verification failed - token not for this service
            log.Error("context mismatch", err.Message)
        case encryption.ErrorKindIntegrityViolation:
            // Token has been tampered with
            log.Error("integrity violation", err.Message)
        case encryption.ErrorKindKEKUnavailable:
            // AWS KMS or environment variable not accessible
            log.Error("kek unavailable", err.Message)
        case encryption.ErrorKindEncryptionFailed:
            // General encryption failure
            log.Error("encryption failed", err.Message)
        }
    }
}
```

### Common Scenarios

#### AWS KMS Key Not Found

**Error:** `kek_unavailable: KMS key not accessible`

**Solution:**

1. Make sure that the KMS key ARN in the configuration is correct.
2. Make sure that the IAM role has `kms:DescribeKey` permission.
3. Make sure that the KMS key is in the configured Region.

#### Environment variable not set

**Error:** `kek_unavailable: encryption key material is empty`

**Solution:**

```bash
# Generate and export the key
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=$(openssl rand -base64 32)

# Verify it's set
echo $IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY
```

#### Context mismatch during decryption

**Error:** `context_mismatch: context verification failed during decryption`

**Solution:**

- Make sure that the decryption context matches the encryption context.
- A token for service A cannot be decrypted with service B context.
- Make sure that the encryption context contains the correct `service_id`.

## Testing

### Unit Tests

```go
func TestEncryption(t *testing.T) {
    // Create mock encryption port
    mockEncryption := &MockEncryptionPort{
        encryptFn: func(ctx context.Context, plaintext []byte, ctx map[string]string) ([]byte, error) {
            // Return deterministic ciphertext for testing
            return append([]byte("encrypted_"), plaintext...), nil
        },
        decryptFn: func(ctx context.Context, ciphertext []byte, ctx map[string]string) ([]byte, error) {
            // Extract plaintext from test ciphertext
            return ciphertext[10:], nil
        },
    }

    // Use in service for testing
    service := oauth2session.NewOAuth2SessionService(
        serviceRepo, sessionRepo, grantRepo, agentRepo,
        mockEncryption,
        jweKey, config, logger,
    )

    // Test token encryption
    session, err := service.CreateSessionFromCallback(ctx, principal, cbReq)
    require.NoError(t, err)
    require.NotNil(t, session.EncryptedAccessToken)
}
```

### Integration Tests

```go
func TestEncryptionWithRealAdapter(t *testing.T) {
    // Use a real AWS adapter with a LocalStack-compatible AWS emulator for testing
    adapter, manager, err := aws.NewAWSEncryption(
        "arn:aws:kms:eu-central-1:123456789012:key/12345678",
        "IdentityBrokerEncryptionBranchKeys",
        1*time.Hour,
    )
    require.NoError(t, err)

    // Test encryption/decryption roundtrip
    plaintext := []byte("test-token")
    context := map[string]string{"service_id": "github"}

    ciphertext, err := adapter.Encrypt(ctx, plaintext, context)
    require.NoError(t, err)
    require.NotEqual(t, plaintext, ciphertext)

    decrypted, err := adapter.Decrypt(ctx, ciphertext, context)
    require.NoError(t, err)
    require.Equal(t, plaintext, decrypted)
}
```

### E2E Tests

See `tests/e2e/encryption_vault_raw_test.go` for E2E scenarios that cover:

- Envelope encryption with context binding
- AWS KMS storage
- Environment variable KEK injection
- DEK generation and isolation
- Context verification errors
- Cross-service token reuse prevention

## Troubleshooting

### Issue: "Adapter not properly initialized"

**Cause:** Encryption adapter initialization failed at startup.

**Solution:**

1. Make sure that the KEK configuration is valid.
2. Make sure that AWS credentials are available.
3. If you use an environment-variable backend, make sure that the key variable is set.
4. Examine the application logs for the error.

### Issue: "KMS key not accessible"

**Cause:** The configured AWS KMS key cannot be accessed.

**Solution:**

1. Make sure that the KMS key exists in the configured Region.
2. Make sure that the IAM role has the required permissions.
3. Examine the AWS Region configuration.
4. Make sure that the key is not scheduled for deletion.

### Issue: "Context verification failed"

**Cause:** The token was encrypted with a different context.

**Solution:**

1. Make sure that encryption and decryption use the same `service_id`.
2. Make sure that the token is used with the correct service.
3. Make sure that the encryption context is constructed correctly.

### Issue: Performance degradation

**Cause:** AWS KMS latency or DynamoDB cache problems.

**Solution:**

1. Examine AWS KMS CloudTrail logs for throttling.
2. If cache eviction is frequent, increase branch-key TTL.
3. Consider DynamoDB provisioned capacity.
4. Measure network latency to AWS services.

### Issue: Tokens cannot be decrypted after restart

**Cause:** A different KEK is used after restart.

**Solution:**

1. Make sure that KEK configuration is identical.
2. Make sure that the environment variable did not change.
3. Make sure that the AWS KMS key is compatible with the previous key.
4. Make sure that the database contains encrypted tokens from the previous run.

## Additional Resources

- Encryption feature specification: see `specs/012-aws-encryption-vault/spec.md` in the repository
- Architecture documentation: see `docs/ARCHITECTURE.md`
- [Configuration Guide](./configuration.md)
- [AWS KMS Documentation](https://docs.aws.amazon.com/kms/)
- [AWS Encryption SDK Documentation](https://docs.aws.amazon.com/encryption-sdk/)
