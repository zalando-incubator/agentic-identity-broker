package aws

import (
	"context"
	"fmt"

	mpl "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygenerated"
	mpltypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygeneratedtypes"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
)

// createHierarchicalKeyring creates an AWS KMS hierarchical keyring with DynamoDB caching.
// The hierarchical keyring uses a branch key supplier that caches branch keys in DynamoDB,
// reducing the number of KMS API calls and improving performance.
//
// Architecture:
// 1. KeyStore (manages branch key creation and caching in DynamoDB)
// 2. BranchKeySupplier (maps service_id to branch key identifiers)
// 3. Material Providers Client (creates the hierarchical keyring)
// 4. Hierarchical Keyring (coordinates KEK wrapping with DynamoDB-cached branch keys)
//
// Parameters:
//   - ctx: Context for AWS API calls
//   - ks: Pre-initialized KeyStore
//   - supplier: BranchKeySupplier for mapping context to branch key IDs
//
// Returns:
//   - mpltypes.IKeyring: The configured hierarchical keyring
//   - error: If keyring creation fails
func createHierarchicalKeyring(ctx context.Context, ks *KeyStore, supplier mpltypes.IBranchKeyIdSupplier) (mpltypes.IKeyring, error) {
	if ks == nil {
		return nil, encryption.NewKEKUnavailableError("KeyStore is required for hierarchical keyring", nil)
	}

	if ks.config.BranchKeyTTL == 0 {
		ks.config.BranchKeyTTL = DefaultBranchKeyTTL
	}

	if err := lockSDK(ctx); err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create Material Providers client: %v", err), err,
		)
	}
	defer unlockSDK()

	// Create Material Providers client
	matProvider, err := mpl.NewClient(mpltypes.MaterialProvidersConfig{})
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create Material Providers client: %v", err),
			err,
		)
	}

	// Create hierarchical keyring using the KeyStore and supplier
	// The hierarchical keyring will cache branch keys in DynamoDB via the KeyStore
	// This reduces KMS API calls significantly for high-throughput scenarios
	hierarchicalKeyringInput := mpltypes.CreateAwsKmsHierarchicalKeyringInput{
		KeyStore:            ks.client,
		BranchKeyIdSupplier: supplier,
		TtlSeconds:          int64(ks.config.BranchKeyTTL.Seconds()),
	}

	keyring, err := matProvider.CreateAwsKmsHierarchicalKeyring(ctx, hierarchicalKeyringInput)
	if err != nil {
		return nil, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to create AWS KMS hierarchical keyring: %v", err),
			err,
		)
	}

	return keyring, nil
}
