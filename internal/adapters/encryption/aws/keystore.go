package aws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	keystore "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographykeystoresmithygenerated"
	keystoretypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographykeystoresmithygeneratedtypes"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/branchkey"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// KeyStoreConfig contains configuration for AWS KMS KeyStore setup.
// The KeyStore manages branch key creation and caching in DynamoDB.
type KeyStoreConfig struct {
	KMSKeyARN         string        // AWS KMS key ARN for root key
	DynamoDBTableName string        // DynamoDB table for caching branch keys
	BranchKeyTTL      time.Duration // TTL for cached branch keys
}

// KeyStore wraps the AWS KeyStore client and manages its lifecycle.
// The KeyStore is responsible for creating and managing branch keys in DynamoDB,
// which are then used by the hierarchical keyring for envelope encryption.
type KeyStore struct {
	client          *keystore.Client
	config          KeyStoreConfig
	dynamoDBTimeout time.Duration // timeout applied to the full branch-key operation
	// getActiveBranchKeyFn overrides client.GetActiveBranchKey when set. Used only in tests
	// to inject a mock without requiring real DynamoDB infrastructure.
	getActiveBranchKeyFn func(ctx context.Context, params keystoretypes.GetActiveBranchKeyInput) (*keystoretypes.GetActiveBranchKeyOutput, error)
	// createKeyFn overrides client.CreateKey when set. Used only in tests to inject a mock
	// without requiring real DynamoDB infrastructure.
	createKeyFn func(ctx context.Context, params keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error)
}

// createKeyStore creates an AWS KMS KeyStore client with DynamoDB caching.
// The KeyStore handles branch key creation and caching, reducing KMS API calls.
//
// Architecture:
// - AWS SDK Config: Configurable AWS configuration (region, credentials, endpoints)
// - KMS Client: For KMS key operations
// - DynamoDB Client: For branch key storage and caching
// - KeyStore Client: Wraps above for branch key management
//
// Parameters:
//   - ctx: Context for AWS API calls
//   - ksCfg: KeyStore configuration (KMS ARN, DynamoDB table, TTL)
//   - awsCfg: AWS SDK configuration (region, credentials, endpoints, etc.)
//   - logicalKeyStoreName: Identifier for this KeyStore instance
//
// Returns:
//   - *KeyStore: The initialized KeyStore (or nil on error)
//   - error: If KeyStore initialization fails
func createKeyStore(ctx context.Context, ksCfg KeyStoreConfig, awsCfg *ports.AWSKMSConfig, logicalKeyStoreName string) (*KeyStore, error) {
	if ksCfg.KMSKeyARN == "" {
		return nil, encryption.NewKEKUnavailableError("KMS key ARN is required for KeyStore", nil)
	}

	if ksCfg.DynamoDBTableName == "" {
		ksCfg.DynamoDBTableName = DefaultBranchKeyTableName
	}

	if ksCfg.BranchKeyTTL == 0 {
		ksCfg.BranchKeyTTL = DefaultBranchKeyTTL
	}

	// Build AWS SDK configuration from provided settings
	sdkCfg, err := buildAWSConfig(ctx, awsCfg)
	if err != nil {
		return nil, err
	}

	// Create KMS client with configured AWS settings
	// Apply custom endpoint if configured (modern EndpointResolverV2 pattern)
	kmsEndpoint, err := GetKMSEndpoint(awsCfg)
	if err != nil {
		return nil, err
	}
	var kmsOpts []func(*kms.Options)
	if kmsEndpoint != "" {
		kmsOpts = append(kmsOpts, func(o *kms.Options) {
			o.BaseEndpoint = &kmsEndpoint
		})
	}
	kmsClient := kms.NewFromConfig(sdkCfg, kmsOpts...)

	// Create DynamoDB client with configured AWS settings
	// Override region if DynamoDBRegion is specifically configured
	dynamoDBConfig := sdkCfg
	if awsCfg != nil && awsCfg.DynamoDBRegion != "" && awsCfg.DynamoDBRegion != sdkCfg.Region {
		dynamoDBConfig = sdkCfg.Copy()
		dynamoDBConfig.Region = awsCfg.DynamoDBRegion
	}
	// Apply custom endpoint if configured (modern EndpointResolverV2 pattern)
	dynamoDBEndpoint, err := GetDynamoDBEndpoint(awsCfg)
	if err != nil {
		return nil, err
	}
	var dynamoDBOpts []func(*dynamodb.Options)
	if dynamoDBEndpoint != "" {
		dynamoDBOpts = append(dynamoDBOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = &dynamoDBEndpoint
		})
	}
	dynamoDBClient := dynamodb.NewFromConfig(dynamoDBConfig, dynamoDBOpts...)

	// Parse the named dynamodb_timeout value for branch-key work.
	// The timeout is stored on the KeyStore and applied via context.WithTimeout
	// to the full branch-key operation and any KMS work it triggers.
	dynamoDBTimeout, err := parseDynamoDBTimeout(awsCfg)
	if err != nil {
		return nil, err
	}

	// Verify KMS key is accessible
	if err := verifyKMSKeyAccessible(ctx, kmsClient, ksCfg.KMSKeyARN); err != nil {
		return nil, err
	}

	// Create KeyStore client (wraps KMS + DynamoDB for branch key management)
	keystoreConfig := keystoretypes.KeyStoreConfig{
		DdbTableName:        ksCfg.DynamoDBTableName,
		KmsClient:           kmsClient,
		DdbClient:           dynamoDBClient,
		LogicalKeyStoreName: logicalKeyStoreName,
		KmsConfiguration: &keystoretypes.KMSConfigurationMemberkmsKeyArn{
			Value: ksCfg.KMSKeyARN,
		},
	}

	keystoreClient, err := callSDK(ctx, func() (*keystore.Client, error) {
		return keystore.NewClient(keystoreConfig)
	})
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create KeyStore client: %v", err),
			err,
		)
	}

	return &KeyStore{
		client:          keystoreClient,
		config:          ksCfg,
		dynamoDBTimeout: dynamoDBTimeout,
	}, nil
}

// CreateBranchKey creates a branch key in DynamoDB for the provided subject.
// This operation first checks for an existing branch key and returns it when the lookup
// succeeds. If the lookup fails, CreateKey is attempted so callers can invoke
// CreateBranchKey unconditionally without failing on pre-existing keys.
//
// Returns the branch key identifier or error if provisioning fails.
func (ks *KeyStore) CreateBranchKey(ctx context.Context, subject encryption.BranchKeySubject) (string, error) {
	if ks == nil || (ks.client == nil && ks.createKeyFn == nil) {
		return "", encryption.NewKEKUnavailableError("KeyStore not initialized", nil)
	}

	if err := subject.Validate(); err != nil {
		return "", encryption.NewKEKUnavailableError(fmt.Sprintf("invalid branch key subject: %v", err), err)
	}

	branchKeyID, err := branchkey.GenerateBranchKeyId(subject)
	if err != nil {
		return "", encryption.NewKEKUnavailableError(fmt.Sprintf("failed to generate branch key ID from subject: %v", err), err)
	}

	encryptionCtx := subject.EncryptionContext()
	serviceID := encryptionCtx[encryption.ContextKeyServiceID]

	if ks.dynamoDBTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, ks.dynamoDBTimeout, errDynamoDBTimeout)
		defer cancel()
	}

	// Idempotency check: if the branch key already exists, return it without creating a duplicate.
	// This handles the migration case where a service was created with a different encryption
	// backend (e.g. raw AES) and then updated after switching to KMS — the branch key for
	// that service ID has not yet been provisioned in DynamoDB, but for services already using
	// KMS the branch key exists and CreateKey would fail.
	getActiveBranchKey := ks.getActiveBranchKeyFn
	if getActiveBranchKey == nil && ks.client != nil {
		getActiveBranchKey = ks.client.GetActiveBranchKey
	}
	var getErr error
	if getActiveBranchKey != nil {
		_, getErr = callSDK(ctx, func() (*keystoretypes.GetActiveBranchKeyOutput, error) {
			return getActiveBranchKey(ctx, keystoretypes.GetActiveBranchKeyInput{BranchKeyIdentifier: branchKeyID})
		})
		if getErr == nil {
			return branchKeyID, nil
		}
		if errors.Is(context.Cause(ctx), errDynamoDBTimeout) {
			slog.Error("branch_key_creation_failed",
				"operation", "get_active_branch_key",
				"branch_key_id", branchKeyID,
				"service_id", serviceID,
				"subject_kind", subject.Kind(),
				"subject_identifier", subject.Identifier(),
				"error_kind", encryption.ErrorKindKEKUnavailable,
				"reason", "dynamodb_timeout",
				"dynamodb_timeout", ks.dynamoDBTimeout,
				"sdk_error", getErr,
			)
			return "", encryption.NewKEKUnavailableError(
				fmt.Sprintf("getting active branch key %s timed out after configured dynamodb_timeout=%s", branchKeyID, ks.dynamoDBTimeout),
				getErr,
			)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", encryption.NewKEKUnavailableError(
				fmt.Sprintf("getting active branch key %s cancelled: %v", branchKeyID, ctxErr), ctxErr,
			)
		}
	}
	// getErr is non-nil and the operation is still live: either the key does not
	// exist yet (expected migration path) or DynamoDB/KMS is unavailable. We
	// attempt CreateKey in either case so the caller receives a clear error.

	createKey := ks.createKeyFn
	if createKey == nil {
		createKey = ks.client.CreateKey
	}

	branchKey, err := callSDK(ctx, func() (*keystoretypes.CreateKeyOutput, error) {
		return createKey(ctx, keystoretypes.CreateKeyInput{
			BranchKeyIdentifier: &branchKeyID,
			EncryptionContext:   encryptionCtx,
		})
	})
	if err != nil {
		if errors.Is(context.Cause(ctx), errDynamoDBTimeout) {
			slog.Error("branch_key_creation_failed",
				"operation", "create_branch_key",
				"branch_key_id", branchKeyID,
				"service_id", serviceID,
				"subject_kind", subject.Kind(),
				"subject_identifier", subject.Identifier(),
				"error_kind", encryption.ErrorKindKEKUnavailable,
				"reason", "dynamodb_timeout",
				"dynamodb_timeout", ks.dynamoDBTimeout,
				"sdk_error", err,
			)
			return "", encryption.NewKEKUnavailableError(
				fmt.Sprintf("creating branch key %s timed out after configured dynamodb_timeout=%s", branchKeyID, ks.dynamoDBTimeout),
				err,
			)
		}
		slog.Error("branch_key_creation_failed",
			"operation", "create_branch_key",
			"branch_key_id", branchKeyID,
			"service_id", serviceID,
			"subject_kind", subject.Kind(),
			"subject_identifier", subject.Identifier(),
			"error_kind", encryption.ErrorKindKEKUnavailable,
			"reason", "create_key_failed",
			"sdk_error", err,
		)
		return "", encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create branch key %s in DynamoDB: %v", branchKeyID, err),
			err,
		)
	}
	return branchKey.BranchKeyIdentifier, nil
}
