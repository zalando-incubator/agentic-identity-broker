# Contract: Configuration for Encryption Vault

**Location**: `.env` file (feature 20251214-120100-flexible-configuration)

**Purpose**: Defines how to configure the Key Encryption Key (KEK) for envelope encryption. Supports both production (AWS KMS) and development (environment variable) modes.

> **Implementation note (superseded configuration):**
> This document records the original single-field encryption proposal
> (`encryption.key` / `key_encryption_key` with `${ENCRYPTION_KEK}`).
> The shipped implementation uses a backend-explicit contract:
> `encryption.aws_kms` or `encryption.memory`.
> See `internal/ports/config.go` and `docs/configuration.md`
> for the live schema.

---

## Configuration Field

### `encryption.key`

**Type**: String (required for feature 20260114-120000 activation)

**Format**: Either AWS KMS ARN or environment variable reference

**Default**: Empty (encryption disabled if not set; application fails at startup if encryption expected)

---

## Configuration Examples

### Production: AWS KMS

**Use Case**: Production environment with managed key service

**Configuration**:

```yaml
# .env
encryption:
  key_encryption_key: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
```

**Resolution**:

1. Config system reads the value
2. EncryptionAdapter detects `arn:aws:kms:` prefix
3. Extracts region (us-east-1), account (123456789012), key ID
4. Creates AWS KMS keyring using AWS SDK v2
5. At startup, validates KEK is accessible (fails with clear error if not)

**Benefits**:

- Key managed by AWS (automatic rotation, compliance)
- Audit trail of key usage via AWS CloudTrail
- High availability (AWS KMS replicated)
- Cost: ~$1/month per key + per-call charges (network latency 50-200ms)

**Requirements**:

- IAM permissions: `kms:Decrypt`, `kms:GenerateDataKey`
- Network access: HTTPS to AWS KMS endpoint
- Environment: AWS account credentials in EC2 instance role or environment variables

---

### Development: Environment Variable

**Use Case**: Local development, CI/CD pipelines, testing

**Configuration**:

```yaml
# .env
encryption:
  key_encryption_key: ${ENCRYPTION_KEK}
```

**Resolution**:

1. Config system applies `${var}` interpolation (feature 20251214-120100-flexible-configuration)
2. Resolves `ENCRYPTION_KEK` from environment (exported in shell or CI/CD secrets)
3. Expected value: base64-encoded 32-byte (256-bit) random key material
4. EncryptionAdapter detects it's raw key material (not ARN)
5. At startup, validates key material is valid base64 and 32 bytes (fails with clear error if not)

**Environment Variable Setup** (bash):

```bash
# Generate random 256-bit key material (32 bytes)
ENCRYPTION_KEK=$(openssl rand -base64 32)
export ENCRYPTION_KEK

# Verify
echo $ENCRYPTION_KEK  # Base64 string, ~44 characters
```

**Environment Variable Setup** (Go test):

```go
// In test setup
os.Setenv("ENCRYPTION_KEK", "YOUR_BASE64_ENCODED_KEY_MATERIAL_HERE")

// Or via testify/assert
t.Helper()
testutil.SetEnv(t, "ENCRYPTION_KEK", "YOUR_BASE64_ENCODED_KEY_MATERIAL_HERE")
```

**CI/CD Integration** (GitHub Actions):

```yaml
# .github/workflows/test.yml
env:
  ENCRYPTION_KEK: ${{ secrets.ENCRYPTION_KEK_DEV }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go test -v ./...
```

**Docker Environment** (docker-compose.yml):

```yaml
services:
  app:
    image: agentic-identity-broker:latest
    environment:
      ENCRYPTION_KEK: ${ENCRYPTION_KEK}  # From host .env
    volumes:
      - .env:/app/.env:ro
```

**Benefits**:

- No network calls (fast, <5ms encryption/decryption)
- No AWS account required (local development)
- Simple for testing (generate per test)
- No costs

**Requirements**:

- Environment variable must be set before app startup
- Key material must be 32 bytes (256-bit) base64-encoded
- Keep key material secret (never commit to git; use .env.local)

---

## Configuration Resolution Algorithm

```go
// In EncryptionAdapter initialization
func NewAWSEncryptionAdapter(keyMaterialInput string) (ports.EncryptionPort, error) {
    // Step 1: Detect format
    if strings.HasPrefix(keyMaterialInput, "arn:aws:kms:") {
        // Step 2a: Parse AWS KMS ARN
        keyID := parseKMSArn(keyMaterialInput)

        // Step 2b: Create AWS KMS client and keyring
        client := createKMSClient()
        keyring := aws.NewKmsKeyring(client, keyID)

        // Step 2c: Validate KEK accessibility (fail-fast at startup)
        if err := validateKMSAccess(client, keyID); err != nil {
            return nil, fmt.Errorf("KEK unavailable: %w", err)
        }

        return &AWSEncryptionAdapter{keyring: keyring}, nil
    }

    // Step 3: Assume raw key material (base64-encoded)
    keyBytes, err := base64.StdEncoding.DecodeString(keyMaterialInput)
    if err != nil {
        return nil, fmt.Errorf("invalid key material encoding: %w", err)
    }

    // Step 4: Validate key length (256-bit = 32 bytes)
    if len(keyBytes) != 32 {
        return nil, fmt.Errorf("key material must be 256-bit (32 bytes), got %d bytes", len(keyBytes))
    }

    // Step 5: Create keyring from raw key material
    keyring := aws.NewRawAESKeyring(keyBytes)

    return &AWSEncryptionAdapter{keyring: keyring}, nil
}
```

---

## Startup Validation

**Application Startup**:

```go
// In AppBuilder or server lifecycle
func (b *AppBuilder) validateEncryption(ctx context.Context) error {
    keyMaterial := b.config.EncryptionKeyEncryptionKey

    if keyMaterial == "" {
        return fmt.Errorf("encryption.key not configured (required for feature 20260114-120000)")
    }

    // Attempt to create adapter (triggers KEK validation)
    adapter, err := adapters.NewAWSEncryptionAdapter(keyMaterial)
    if err != nil {
        return fmt.Errorf("encryption vault initialization failed: %w", err)
    }

    // Perform a test encrypt/decrypt to verify KEK is accessible
    testPlaintext := []byte("test-token")
    testContext := map[string]string{"service_id": "test"}

    ciphertext, err := adapter.Encrypt(ctx, testPlaintext, testContext)
    if err != nil {
        return fmt.Errorf("encryption test failed: %w", err)
    }

    decrypted, err := adapter.Decrypt(ctx, ciphertext, testContext)
    if err != nil {
        return fmt.Errorf("decryption test failed: %w", err)
    }

    if !bytes.Equal(decrypted, testPlaintext) {
        return fmt.Errorf("encryption roundtrip verification failed")
    }

    logger.Info("Encryption vault initialized successfully")
    return nil
}
```

**Startup Logging**:

**Success**:

```
[INFO] Encryption vault initialized successfully
       kek_type=aws_kms
       kms_region=us-east-1
```

**Failure (AWS KMS unavailable)**:

```
[ERROR] Encryption vault initialization failed
        reason=kms_unreachable
        error=Failed to connect to AWS KMS
        action=Check IAM permissions and network connectivity
```

**Failure (Environment variable not set)**:

```
[ERROR] Encryption vault initialization failed
        reason=kek_not_configured
        error=encryption.key not set
        action=Set ENCRYPTION_KEK environment variable or configure AWS KMS ARN
```

---

## Environment Variable Interpolation

The configuration system (feature 20251214-120100-flexible-configuration) supports `${VAR_NAME}` interpolation:

**Configuration File** (.env):

```yaml
encryption:
  key_encryption_key: ${ENCRYPTION_KEK}
```

**Resolution Process**:

1. Config system detects `${ENCRYPTION_KEK}` pattern
2. Looks up `ENCRYPTION_KEK` in environment
3. Substitutes value into configuration
4. EncryptionAdapter receives resolved value (raw key material)

**Example Flow**:

```bash
# Shell environment
export ENCRYPTION_KEK="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij=="

# .env file contains
encryption:
  key_encryption_key: ${ENCRYPTION_KEK}

# After interpolation, adapter receives
"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij=="
```

---

## Security Considerations

### AWS KMS

**Key Material Never Exposed**:

- KEK never leaves AWS KMS
- Only DEK wrapping/unwrapping happens on AWS side
- Application never has access to raw KEK

**Audit Trail**:

- All KMS operations logged to CloudTrail
- Decryption attempts visible (success/failure)
- Access pattern visible for compliance audits

**Key Rotation**:

- Automatic rotation handled by AWS (optional annual)
- DEK format includes version byte enabling algorithm migration
- Old tokens remain decryptable after rotation

### Environment Variable

**Key Material in Memory**:

- Raw key material handled securely at startup
- Memory protection deferred to future memory hardening feature
- Zeroed on application shutdown

**Risks**:

- Key visible in process environment (`ps aux` shows env vars)
- Key in .env file (commit to git by mistake)
- Key in CI/CD logs (if not masked)

**Mitigations**:

- Store .env.local in .gitignore (never commit)
- Use CI/CD secrets (masked in logs)
- Rotate keys frequently (development environments)
- Audit access to .env files (file permissions)

---

## Configuration Validation

**Validation Rules**:

| Rule | Condition | Error Message |
|------|-----------|---------------|
| Required | `encryption.key` not set | "encryption.key not configured" |
| Format | Not ARN or valid base64 | "invalid key material encoding" |
| Key Length (env var) | Not 256-bit (32 bytes) after decoding | "key material must be 256-bit (32 bytes), got X bytes" |
| KMS Access (AWS KMS) | Cannot reach AWS KMS endpoint | "KEK unavailable: connection failed" |
| KMS Permissions (AWS KMS) | Missing `kms:Decrypt`, `kms:GenerateDataKey` | "KEK unavailable: insufficient permissions" |
| Test Roundtrip | Encrypt/decrypt test fails | "encryption roundtrip verification failed" |

---

## Configuration Examples by Environment

### Local Development

**.env.local** (never commit):

```yaml
encryption:
  key_encryption_key: ${ENCRYPTION_KEK}
```

**Export in shell**:

```bash
export ENCRYPTION_KEK=$(openssl rand -base64 32)
just dev
```

---

### CI/CD Testing

**.github/workflows/test.yml**:

```yaml
env:
  ENCRYPTION_KEK: ${{ secrets.ENCRYPTION_KEK_DEV }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: just verify
```

---

### Docker Containerization

**Dockerfile**:

```dockerfile
FROM golang:1.24-alpine

# Accept KEK at build time or runtime
ARG ENCRYPTION_KEK=""
ENV ENCRYPTION_KEK=${ENCRYPTION_KEK}

# Copy app
COPY ./bin/agentic-identity-broker /app/
COPY ./config/.env.docker /.env

ENTRYPOINT ["/app/agentic-identity-broker"]
```

**docker-compose.yml**:

```yaml
services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        ENCRYPTION_KEK: ${ENCRYPTION_KEK}
    environment:
      ENCRYPTION_KEK: ${ENCRYPTION_KEK}
    env_file:
      - .env.local
    ports:
      - "8000:8000"
```

**Run**:

```bash
export ENCRYPTION_KEK=$(openssl rand -base64 32)
docker-compose up
```

---

### Kubernetes Deployment

**secret.yaml**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: encryption-kek
type: Opaque
data:
  # Base64-encoded KEK value (Kubernetes encodes again)
  key_encryption_key: <BASE64_OF_BASE64>
```

**deployment.yaml**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agentic-identity-broker
spec:
  containers:
  - name: app
    image: agentic-identity-broker:v1.0.0
    env:
    - name: ENCRYPTION_KEK
      valueFrom:
        secretKeyRef:
          name: encryption-kek
          key: key_encryption_key
    - name: ENCRYPTION_KEY_ENCRYPTION_KEY
      value: ${ENCRYPTION_KEK}
```

---

**Version**: 1.0 | **Status**: Design Phase | **Last Updated**: 2026-01-16
