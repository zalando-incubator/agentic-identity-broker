package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mpl "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygenerated"
	mpltypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygeneratedtypes"
	client "github.com/aws/aws-encryption-sdk/releases/go/encryption-sdk/awscryptographyencryptionsdksmithygenerated"
	esdktypes "github.com/aws/aws-encryption-sdk/releases/go/encryption-sdk/awscryptographyencryptionsdksmithygeneratedtypes"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/branchkey"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	// DefaultKEKValidationTimeout is the timeout for KMS key accessibility checks
	DefaultKEKValidationTimeout = 30 * time.Second
	// DefaultBranchKeyTTL is the TTL for branch keys in the DynamoDB cache
	DefaultBranchKeyTTL = 1 * time.Hour
	// DefaultBranchKeyTableName is the default DynamoDB table for caching branch keys
	DefaultBranchKeyTableName = "IdentityBrokerEncryptionBranchKeys"
)

// errDynamoDBTimeout is set as the context cause when the adapter's internal dynamoDBTimeout fires.
// Using context.Cause against this sentinel distinguishes adapter-triggered timeouts from a caller
// deadline that coincidentally expires as context.DeadlineExceeded at the same time.
var errDynamoDBTimeout = errors.New("adapter dynamodb_timeout")

// The generated MPL/ESDK Go code writes to dafny.EmptySeq.SetString() during
// SDK calls (including signed-message Base64 encoding). EmptySeq is shared by
// every client and keyring, so an adapter-local mutex cannot protect it.
// All Dafny-backed SDK entry points in this package share this gate.
var sdkGate = make(chan struct{}, 1)

func lockSDK(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case sdkGate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			unlockSDK()
			return err
		}
		return nil
	}
}

func unlockSDK() { <-sdkGate }

func callSDK[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	if err := lockSDK(ctx); err != nil {
		var zero T
		return zero, err
	}
	defer unlockSDK()
	return operation()
}

// encryptionSDKClient is the minimal subset of the AWS Encryption SDK client used by AWSAdapter,
// enabling mock injection in tests without requiring real AWS infrastructure.
type encryptionSDKClient interface {
	Encrypt(ctx context.Context, params esdktypes.EncryptInput) (*esdktypes.EncryptOutput, error)
	Decrypt(ctx context.Context, params esdktypes.DecryptInput) (*esdktypes.DecryptOutput, error)
}

// AWSAdapter implements the EncryptionPort interface using AWS Encryption SDK.
// Focuses solely on runtime encryption/decryption operations.
// Supports envelope encryption with AWS KMS hierarchical keyring (production) and environment variable KEK injection (development).
// The hierarchical keyring uses DynamoDB for caching branch keys, reducing KMS API calls and improving performance.
//
// Branch key provisioning is handled separately by AWSBranchKeyManager to maintain clean separation of concerns.
type AWSAdapter struct {
	encryptionClient encryptionSDKClient // AWS Encryption SDK client for encrypt/decrypt operations
	keyring          mpltypes.IKeyring   // Keyring (AWS KMS hierarchical, KMS, or Raw AES)
	dynamoDBTimeout  time.Duration       // per-operation context timeout applied to DynamoDB branch key fetches
}

// NewAWSEncryption creates an AWS Encryption SDK adapter with automatic fan-out to supported scenarios:
// Scenario A (development): Base64-encoded AES-256 key (raw material)
// Scenario B (production): Hierarchical keyring with AWS KMS and DynamoDB branch key caching (KMS ARN)
//
// keyMaterial can be either:
//   - AWS KMS ARN: "arn:aws:kms:region:account:key/key-id" or "arn:aws:kms:region:account:alias/alias-name"
//   - Base64-encoded 32-byte AES-256 key: "aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890AB=="
//
// Environment variable interpolation is handled by the config loader before this function is called.
// Configuration example with environment variable:
//   - YAML: encryption.memory.raw_key: "${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}"
//   - Config loader expands ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
//   - This function receives: the actual base64 key value (not the ${...} reference)
//
// Parameters:
//   - keyMaterial: AWS KMS ARN or base64-encoded 32-byte AES-256 key
//   - dynamoDBTableName: DynamoDB table for branch key caching (uses default if empty, ignored for base64 scenario)
//   - branchKeyTTL: TTL for cached branch keys (uses default if zero, ignored for base64 scenario)
//
// Returns:
//   - adapter: EncryptionPort implementation for Encrypt/Decrypt operations
//   - manager: BranchKeyManager implementation for provisioning/managing branch keys (nil for base64 scenario)
//   - error: If initialization fails
func NewAWSEncryption(keyMaterial, dynamoDBTableName string, branchKeyTTL time.Duration) (*AWSAdapter, *AWSBranchKeyManager, error) {
	if keyMaterial == "" {
		return nil, nil, encryption.NewKEKUnavailableError("key encryption key material is required", nil)
	}

	// Scenario B: AWS KMS ARN for production (hierarchical keyring with DynamoDB caching)
	// Note: this path synthesises an AWSKMSConfig with only KeyARN set, so dynamoDBTimeout
	// is always 0 (disabled). Use NewEncryptionAdapter with a fully populated AWSKMSConfig
	// to enable the named dynamodb_timeout field.
	// It applies to the full top-level encryption operation.
	if strings.HasPrefix(keyMaterial, "arn:aws:kms:") {
		awsConfig := &ports.AWSKMSConfig{
			KeyARN: keyMaterial,
		}
		adapter, keyStore, err := newAdapterWithKMSARNAndKeyStore(keyMaterial, dynamoDBTableName, branchKeyTTL, awsConfig)
		if err != nil {
			return nil, nil, err
		}
		manager := NewAWSBranchKeyManager(keyStore)
		return adapter, manager, nil
	}

	// Scenario A: Base64-encoded AES-256 key for development (raw AES keyring)
	// Note: Environment variable interpolation (${VAR_NAME}) is handled by the config loader
	// This function receives the actual base64-encoded key value
	adapter, err := newAdapterWithBase64KEK(keyMaterial)
	if err != nil {
		return nil, nil, err
	}
	// No branch key manager for raw AES keyring
	return adapter, nil, nil
}

// newAdapterWithKMSARNAndKeyStore creates an adapter using AWS KMS hierarchical keyring and returns the KeyStore.
// The hierarchical keyring uses DynamoDB for caching branch keys, reducing KMS API calls.
// This variant returns both the adapter and the KeyStore for branch key manager creation.
// awsCfg provides AWS SDK configuration including region, credentials, and endpoints.
func newAdapterWithKMSARNAndKeyStore(kmsARN, dynamoDBTableName string, branchKeyTTL time.Duration, awsCfg *ports.AWSKMSConfig) (*AWSAdapter, *KeyStore, error) {
	ctx := context.Background()

	// Create KeyStore with configured or default values
	dynamoDBTable := dynamoDBTableName
	if dynamoDBTable == "" {
		dynamoDBTable = DefaultBranchKeyTableName
	}

	ttl := branchKeyTTL
	if ttl == 0 {
		ttl = DefaultBranchKeyTTL
	}

	keyStoreCfg := KeyStoreConfig{
		KMSKeyARN:         kmsARN,
		DynamoDBTableName: dynamoDBTable,
		BranchKeyTTL:      ttl,
	}

	keyStore, err := createKeyStore(ctx, keyStoreCfg, awsCfg, "IdentityBrokerEncryptionVault")
	if err != nil {
		return nil, nil, err
	}

	// Create branch key supplier
	supplier := NewBranchKeyIdSupplier(branchkey.NewDefaultProvider())

	// Create hierarchical keyring using KeyStore and supplier
	keyring, err := createHierarchicalKeyring(ctx, keyStore, supplier)
	if err != nil {
		return nil, nil, err
	}

	// Create Encryption SDK client with commitment policy for key commitment
	policy := mpltypes.ESDKCommitmentPolicyRequireEncryptRequireDecrypt
	encryptionClient, err := callSDK(ctx, func() (*client.Client, error) {
		return client.NewClient(esdktypes.AwsEncryptionSdkConfig{CommitmentPolicy: &policy})
	})
	if err != nil {
		return nil, nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create encryption SDK client: %v", err),
			err,
		)
	}

	adapter := &AWSAdapter{
		encryptionClient: encryptionClient,
		keyring:          keyring,
		dynamoDBTimeout:  keyStore.dynamoDBTimeout,
	}

	return adapter, keyStore, nil
}

// newAdapterWithBase64KEK creates an adapter with a base64-encoded AES-256 key.
// The keyMaterial should be a base64-encoded 32-byte key (already interpolated from environment variables by config loader).
func newAdapterWithBase64KEK(keyMaterial string) (*AWSAdapter, error) {
	if keyMaterial == "" {
		return nil, encryption.NewKEKUnavailableError(
			"encryption key material is empty; must be AWS KMS ARN or base64-encoded 32-byte key",
			nil,
		)
	}

	// Decode base64-encoded KEK
	kekBytes, err := base64.StdEncoding.DecodeString(keyMaterial)
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to decode encryption key material: key must be base64-encoded 32-byte AES-256 key, got: %v", err),
			err,
		)
	}

	// Validate KEK length (must be 32 bytes for AES-256)
	if len(kekBytes) != 32 {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("KEK must be 32 bytes (256-bit), got %d bytes", len(kekBytes)),
			nil,
		)
	}

	// This factory has no caller context; protect all Dafny-backed construction.
	sdkGate <- struct{}{}
	defer unlockSDK()
	// Create Material Providers client
	matProvider, err := mpl.NewClient(mpltypes.MaterialProvidersConfig{})
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create Material Providers client: %v", err),
			err,
		)
	}

	// Create Raw AES keyring for local key material
	keyringInput := mpltypes.CreateRawAesKeyringInput{
		KeyNamespace: "my-application",
		KeyName:      "encryption-key",
		WrappingKey:  kekBytes,
		WrappingAlg:  mpltypes.AesWrappingAlgAlgAes256GcmIv12Tag16,
	}
	keyring, err := matProvider.CreateRawAesKeyring(context.Background(), keyringInput)
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create raw AES keyring: %v", err),
			err,
		)
	}

	// Create Encryption SDK client with commitment policy for key commitment
	policy := mpltypes.ESDKCommitmentPolicyRequireEncryptRequireDecrypt
	encryptionClient, err := client.NewClient(esdktypes.AwsEncryptionSdkConfig{
		CommitmentPolicy: &policy,
	})
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create encryption SDK client: %v", err),
			err,
		)
	}

	return &AWSAdapter{
		encryptionClient: encryptionClient,
		keyring:          keyring,
	}, nil
}

// Encrypt encrypts plaintext using envelope encryption with DEK + KEK wrapping.
// Uses AWS Encryption SDK with AESGCMSIV authenticated encryption.
// Context binding is enforced at both DEK and KEK layers.
func (a *AWSAdapter) Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	ctx, span := otel.Tracer("encryption").Start(ctx, "encryption.encrypt")
	defer span.End()
	span.SetAttributes(attribute.String("encryption.key_type", "aws_kms"))

	if a == nil || a.encryptionClient == nil {
		slog.Error("encryption_failed",
			"operation", "encrypt",
			"error_kind", encryption.ErrorKindKEKUnavailable,
			"reason", "adapter_not_initialized",
		)
		return nil, encryption.NewKEKUnavailableError("encryption adapter not properly initialized", nil)
	}
	if _, refresh := encryptionContext[encryption.ContextKeyRefreshSessionID]; refresh {
		if _, err := encryption.BranchKeySubjectFromEncryptionContext(encryptionContext); err != nil {
			return nil, encryption.NewContextMismatchError("invalid refresh-session encryption context", err)
		}
	}

	// Extract service_id from context for logging (sanitized)
	serviceID := encryptionContext["service_id"]

	// Apply a per-operation timeout when configured. This deadline is shared by
	// all downstream calls within the operation (DynamoDB branch key fetches and
	// KMS key-wrapping calls made by the hierarchical keyring).
	if a.dynamoDBTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, a.dynamoDBTimeout, errDynamoDBTimeout)
		defer cancel()
	}

	// Fail fast if the caller's context is already done before touching the SDK.
	// Keyrings that perform no network I/O (raw AES) succeed even on a cancelled
	// context, so without this pre-check the ctx.Err() guard in the error path
	// would never fire for them.  The post-call guard below handles contexts that
	// expire DURING a KMS network call.
	if ctxErr := ctx.Err(); ctxErr != nil {
		slog.Error("encryption_failed",
			"operation", "encrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindKEKUnavailable,
			"reason", "context_error",
		)
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("encryption cancelled: %v", ctxErr),
			ctxErr,
		)
	}

	// Encrypt using AWS Encryption SDK
	// The SDK handles:
	// - Fresh DEK generation per call
	// - DEK encryption with plaintext using AESGCMSIV
	// - DEK wrapping with KEK using encryptionContext for AAD
	// - Serialization of envelope (wrapped DEK + ciphertext + auth tag)
	encryptInput := esdktypes.EncryptInput{
		Plaintext:         plaintext,
		EncryptionContext: encryptionContext,
		Keyring:           a.keyring,
	}

	result, err := callSDK(ctx, func() (*esdktypes.EncryptOutput, error) {
		return a.encryptionClient.Encrypt(ctx, encryptInput)
	})

	if err != nil {
		// Classify context errors that materialised DURING the SDK call (e.g. KMS
		// network timeout).  context.DeadlineExceeded → "context deadline exceeded"
		// and context.Canceled → "context canceled" both contain the substring
		// "context" and would otherwise be misclassified as ErrorKindContextMismatch
		// (encryption context AAD mismatch), signalling "do not retry" to callers
		// instead of the correct "transient, retry after backoff" semantics.
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(context.Cause(ctx), errDynamoDBTimeout) {
				slog.Error("encryption_failed",
					"operation", "encrypt",
					"service_id", serviceID,
					"error_kind", encryption.ErrorKindKEKUnavailable,
					"reason", "dynamodb_timeout",
					"dynamodb_timeout", a.dynamoDBTimeout,
					"sdk_error", err,
				)
				return nil, encryption.NewKEKUnavailableError(
					fmt.Sprintf("encryption timed out after configured dynamodb_timeout=%s", a.dynamoDBTimeout),
					err,
				)
			}
			slog.Error("encryption_failed",
				"operation", "encrypt",
				"service_id", serviceID,
				"error_kind", encryption.ErrorKindKEKUnavailable,
				"reason", "context_error",
			)
			return nil, encryption.NewKEKUnavailableError(
				fmt.Sprintf("encryption cancelled: %v", ctxErr),
				ctxErr,
			)
		}

		// Map AWS SDK errors to domain error types
		// Note: AWS Encryption SDK (Smithy-generated) does not expose typed error constants.
		// Error classification is based on error message content analysis.
		// This is the recommended approach per AWS Encryption SDK documentation.
		errMsg := err.Error()

		// Context mismatch errors contain "context" in the error message
		// This occurs when encryption context AAD verification fails
		if strings.Contains(errMsg, "context") || strings.Contains(errMsg, "Context") {
			slog.Error("encryption_failed",
				"operation", "encrypt",
				"service_id", serviceID,
				"error_kind", encryption.ErrorKindContextMismatch,
			)
			return nil, encryption.NewContextMismatchError(
				fmt.Sprintf("context verification failed during encryption: %v", err),
				err,
			)
		}

		// Integrity/authentication errors indicate AEAD tag verification failure
		if strings.Contains(errMsg, "integrity") || strings.Contains(errMsg, "authentication") ||
			strings.Contains(errMsg, "Integrity") || strings.Contains(errMsg, "Authentication") {
			slog.Error("encryption_failed",
				"operation", "encrypt",
				"service_id", serviceID,
				"error_kind", encryption.ErrorKindIntegrityViolation,
			)
			return nil, encryption.NewIntegrityViolationError(
				fmt.Sprintf("integrity verification failed during encryption: %v", err),
				err,
			)
		}

		// Generic encryption failure
		slog.Error("encryption_failed",
			"operation", "encrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindEncryptionFailed,
		)
		return nil, encryption.NewEncryptionFailedError(
			fmt.Sprintf("encryption failed: %v", err),
			err,
		)
	}

	if result == nil {
		slog.Error("encryption_failed",
			"operation", "encrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindEncryptionFailed,
			"reason", "nil_result",
		)
		return nil, encryption.NewEncryptionFailedError("encryption returned nil result", nil)
	}

	// Ensure ciphertext is not empty
	if len(result.Ciphertext) == 0 {
		slog.Error("encryption_failed",
			"operation", "encrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindEncryptionFailed,
			"reason", "empty_ciphertext",
		)
		return nil, encryption.NewEncryptionFailedError("encryption produced empty ciphertext", nil)
	}

	// Success audit logging
	slog.Debug("token_encrypted",
		"service_id", serviceID,
		"operation", "encrypt",
		"size_bytes", len(result.Ciphertext),
	)

	return result.Ciphertext, nil
}

// Decrypt decrypts ciphertext using envelope encryption with context verification.
// The AWS Encryption SDK automatically verifies:
// - Context binding at DEK decryption layer
// - Authentication tag verification
// - DEK unwrapping with KEK using same context
func (a *AWSAdapter) Decrypt(ctx context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error) {
	ctx, span := otel.Tracer("encryption").Start(ctx, "encryption.decrypt")
	defer span.End()
	span.SetAttributes(attribute.String("encryption.key_type", "aws_kms"))

	if a == nil || a.encryptionClient == nil {
		slog.Error("decryption_failed",
			"operation", "decrypt",
			"error_kind", encryption.ErrorKindKEKUnavailable,
			"reason", "adapter_not_initialized",
		)
		return nil, encryption.NewKEKUnavailableError("encryption adapter not properly initialized", nil)
	}

	// Extract service_id from context for logging (sanitized)
	serviceID := encryptionContext["service_id"]

	// Validate ciphertext is not empty
	if len(ciphertext) == 0 {
		slog.Error("decryption_failed",
			"operation", "decrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindDecryptionFailed,
			"reason", "empty_ciphertext",
		)
		return nil, encryption.NewDecryptionFailedError("ciphertext cannot be empty", nil)
	}
	if _, refresh := encryptionContext[encryption.ContextKeyRefreshSessionID]; refresh {
		if _, err := encryption.BranchKeySubjectFromEncryptionContext(encryptionContext); err != nil {
			return nil, encryption.NewContextMismatchError("invalid refresh-session encryption context", err)
		}
	}

	// Apply a per-operation timeout when configured. This deadline is shared by
	// all downstream calls within the operation (DynamoDB branch key fetches and
	// KMS key-wrapping calls made by the hierarchical keyring).
	if a.dynamoDBTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, a.dynamoDBTimeout, errDynamoDBTimeout)
		defer cancel()
	}

	// Fail fast if the caller's context is already done before touching the SDK.
	// Keyrings that perform no network I/O (raw AES) succeed even on a cancelled
	// context, so without this pre-check the ctx.Err() guard in the error path
	// would never fire for them.  The post-call guard below handles contexts that
	// expire DURING a KMS network call.
	if ctxErr := ctx.Err(); ctxErr != nil {
		slog.Error("decryption_failed",
			"operation", "decrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindKEKUnavailable,
			"reason", "context_error",
		)
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("decryption cancelled: %v", ctxErr),
			ctxErr,
		)
	}

	// Decrypt using AWS Encryption SDK
	// The SDK handles:
	// - Envelope parsing (extract wrapped DEK, ciphertext, auth tag)
	// - DEK unwrapping with KEK (verifying encryptionContext as AAD)
	// - Ciphertext decryption with DEK (verifying auth tag)
	// - Context verification at both layers (fail-closed on mismatch)
	decryptInput := esdktypes.DecryptInput{
		Ciphertext:        ciphertext,
		EncryptionContext: encryptionContext,
		Keyring:           a.keyring,
	}

	result, err := callSDK(ctx, func() (*esdktypes.DecryptOutput, error) {
		return a.encryptionClient.Decrypt(ctx, decryptInput)
	})
	if err != nil {
		// Classify context errors that materialised DURING the SDK call (e.g. KMS
		// network timeout).  context.DeadlineExceeded → "context deadline exceeded"
		// and context.Canceled → "context canceled" both contain the substring
		// "context" and would otherwise be misclassified as ErrorKindContextMismatch
		// (encryption context AAD mismatch), signalling "do not retry" to callers
		// instead of the correct "transient, retry after backoff" semantics.
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(context.Cause(ctx), errDynamoDBTimeout) {
				slog.Error("decryption_failed",
					"operation", "decrypt",
					"service_id", serviceID,
					"error_kind", encryption.ErrorKindKEKUnavailable,
					"reason", "dynamodb_timeout",
					"dynamodb_timeout", a.dynamoDBTimeout,
					"sdk_error", err,
				)
				return nil, encryption.NewKEKUnavailableError(
					fmt.Sprintf("decryption timed out after configured dynamodb_timeout=%s", a.dynamoDBTimeout),
					err,
				)
			}
			slog.Error("decryption_failed",
				"operation", "decrypt",
				"service_id", serviceID,
				"error_kind", encryption.ErrorKindKEKUnavailable,
				"reason", "context_error",
			)
			return nil, encryption.NewKEKUnavailableError(
				fmt.Sprintf("decryption cancelled: %v", ctxErr),
				ctxErr,
			)
		}

		// Map AWS SDK errors to domain error types
		// Note: AWS Encryption SDK (Smithy-generated) does not expose typed error constants.
		// Error classification is based on error message content analysis.
		// This is the recommended approach per AWS Encryption SDK documentation.
		errMsg := err.Error()

		// Context mismatch errors contain "context" in the error message
		// This occurs when encryption context AAD verification fails during decryption
		if strings.Contains(errMsg, "context") || strings.Contains(errMsg, "Context") {
			slog.Error("decryption_failed",
				"operation", "decrypt",
				"service_id", serviceID,
				"error_kind", encryption.ErrorKindContextMismatch,
			)
			return nil, encryption.NewContextMismatchError(
				fmt.Sprintf("context verification failed during decryption: %v", err),
				err,
			)
		}

		// Integrity/authentication errors indicate AEAD tag verification failure
		// This could indicate tampering or corrupted ciphertext
		if strings.Contains(errMsg, "integrity") || strings.Contains(errMsg, "authentication") ||
			strings.Contains(errMsg, "Integrity") || strings.Contains(errMsg, "Authentication") {
			slog.Error("decryption_failed",
				"operation", "decrypt",
				"service_id", serviceID,
				"error_kind", encryption.ErrorKindIntegrityViolation,
			)
			return nil, encryption.NewIntegrityViolationError(
				fmt.Sprintf("integrity verification failed during decryption: %v", err),
				err,
			)
		}

		slog.Error("decryption_failed",
			"operation", "decrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindDecryptionFailed,
		)
		return nil, encryption.NewDecryptionFailedError(
			fmt.Sprintf("decryption failed: %v", err),
			err,
		)
	}

	if result == nil {
		slog.Error("decryption_failed",
			"operation", "decrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindDecryptionFailed,
			"reason", "nil_result",
		)
		return nil, encryption.NewDecryptionFailedError("decryption returned nil result", nil)
	}
	expectedRoot, expectsRefresh := encryptionContext[encryption.ContextKeyRefreshSessionID]
	actualRoot, authenticatedRefresh := result.EncryptionContext[encryption.ContextKeyRefreshSessionID]
	if expectsRefresh || authenticatedRefresh {
		if !expectsRefresh || !authenticatedRefresh || actualRoot != expectedRoot {
			return nil, encryption.NewContextMismatchError("authenticated refresh-session subject does not match the expected root", nil)
		}
		if _, err := encryption.BranchKeySubjectFromEncryptionContext(result.EncryptionContext); err != nil {
			return nil, encryption.NewContextMismatchError("invalid authenticated refresh-session subject", err)
		}
	}

	// Ensure plaintext is not empty
	if len(result.Plaintext) == 0 {
		slog.Error("decryption_failed",
			"operation", "decrypt",
			"service_id", serviceID,
			"error_kind", encryption.ErrorKindDecryptionFailed,
			"reason", "empty_plaintext",
		)
		return nil, encryption.NewDecryptionFailedError("decryption produced empty plaintext", nil)
	}

	// Success audit logging
	slog.Debug("token_decrypted",
		"service_id", serviceID,
		"operation", "decrypt",
		"size_bytes", len(result.Plaintext),
	)

	return result.Plaintext, nil
}

// verifyKMSKeyAccessible checks that the KMS key is accessible.
func verifyKMSKeyAccessible(ctx context.Context, client *kms.Client, keyArn string) error {
	_, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: &keyArn,
	})
	if err != nil {
		return encryption.NewKEKUnavailableError(
			fmt.Sprintf("KMS key not accessible: %v", err),
			err,
		)
	}
	return nil
}

// NewEncryptionAdapter creates an encryption adapter based on the configuration backend.
// This factory function implements the backend-explicit configuration design.
// Returns adapter, branch key manager, and error if configuration is invalid or adapter creation fails.
func NewEncryptionAdapter(config *ports.EncryptionConfig) (*AWSAdapter, ports.BranchKeyManager, error) {
	if config == nil {
		return nil, nil, encryption.NewKEKUnavailableError("encryption configuration is required", nil)
	}

	// Count configured backends to ensure exactly one is set
	backendCount := 0
	if config.AWSKMS != nil {
		backendCount++
	}
	if config.Memory != nil {
		backendCount++
	}

	// Validate exactly one backend is configured
	if backendCount == 0 {
		return nil, nil, encryption.NewKEKUnavailableError(
			"exactly one encryption backend must be configured (aws_kms or memory)",
			nil,
		)
	}
	if backendCount > 1 {
		return nil, nil, encryption.NewKEKUnavailableError(
			"exactly one encryption backend must be configured, not both aws_kms and memory",
			nil,
		)
	}

	// Create adapter based on configured backend
	if config.AWSKMS != nil {
		return createAWSKMSAdapter(config.AWSKMS)
	}

	if config.Memory != nil {
		return createMemoryAdapter(config.Memory)
	}

	// Should never reach here due to validation above
	return nil, nil, encryption.NewKEKUnavailableError("no valid backend configuration found", nil)
}

// createAWSKMSAdapter creates an adapter for AWS KMS backend
func createAWSKMSAdapter(config *ports.AWSKMSConfig) (*AWSAdapter, ports.BranchKeyManager, error) {
	// Apply defaults for optional fields
	dynamoDBTableName := config.DynamoDBTableName
	if dynamoDBTableName == "" {
		dynamoDBTableName = "IdentityBrokerEncryptionBranchKeys"
	}

	// Parse optional duration fields with defaults
	var branchKeyTTL time.Duration
	branchKeyTTLStr := config.BranchKeyTTL
	if branchKeyTTLStr == "" {
		branchKeyTTLStr = "1h" // Default
	}

	if branchKeyTTLStr != "" {
		ttl, err := time.ParseDuration(branchKeyTTLStr)
		if err != nil {
			return nil, nil, encryption.NewKEKUnavailableError(
				fmt.Sprintf("invalid branch_key_ttl duration: %v", err),
				err,
			)
		}
		branchKeyTTL = ttl
	}

	// Create adapter using new configuration-aware function
	adapter, keyStore, err := newAdapterWithKMSARNAndKeyStore(
		config.KeyARN,
		dynamoDBTableName,
		branchKeyTTL,
		config, // Pass full AWS configuration
	)
	if err != nil {
		return nil, nil, err
	}

	// Create branch key manager from the KeyStore
	var manager ports.BranchKeyManager
	if keyStore != nil {
		manager = NewAWSBranchKeyManager(keyStore)
	}

	return adapter, manager, nil
}

// createMemoryAdapter creates an adapter for Memory backend
func createMemoryAdapter(config *ports.MemoryConfig) (*AWSAdapter, ports.BranchKeyManager, error) {
	// Create adapter using existing function with base64 key
	adapter, manager, err := NewAWSEncryption(config.RawKey, "", 0)
	if err != nil {
		return nil, nil, err
	}

	// Handle interface nil pointer issue: if manager is a nil pointer, return nil interface
	if manager == nil {
		return adapter, nil, nil
	}

	return adapter, manager, nil
}
