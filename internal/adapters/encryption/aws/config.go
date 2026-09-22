package aws

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// buildAWSConfig constructs an aws.Config from AWSKMSConfig.
// Supports configurable AWS SDK settings for various deployment scenarios.
//
// Configuration scenarios supported:
//   - Default: Uses AWS SDK default credential chain and region resolution
//   - Region override: Explicitly sets AWS region for all services
//   - Custom endpoints: For a LocalStack-compatible AWS emulator or custom AWS implementations
//   - Static credentials: Access key ID and secret for testing/CI environments
//   - AWS profile: Uses named profile from ~/.aws/credentials
//   - IAM role assumption: Assumes role using default credentials then switches
//   - SSL disable: For LocalStack-compatible AWS emulator use only (development only, DANGEROUS in production)
//
// Parameters:
//   - ctx: Context for AWS API calls and credential resolution
//   - cfg: AWSKMSConfig with AWS SDK configuration
//
// Returns:
//   - aws.Config: Configured AWS SDK config ready for service client creation
//   - error: If configuration is invalid (bad URLs, missing credentials, etc.)
func buildAWSConfig(ctx context.Context, cfg *ports.AWSKMSConfig) (aws.Config, error) {
	if cfg == nil {
		return aws.Config{}, encryption.NewKEKUnavailableError("AWSKMSConfig cannot be nil", nil)
	}

	// Validate security settings to prevent insecure configurations
	if err := validateSecuritySettings(cfg); err != nil {
		return aws.Config{}, err
	}

	// Build configuration options slice
	var options []func(*config.LoadOptions) error

	// Region configuration
	if cfg.Region != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}

	// Credentials configuration (priority: static > profile > default chain)
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		// Static credentials for testing/CI
		staticCredentials := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"", // No session token
		)
		options = append(options, config.WithCredentialsProvider(staticCredentials))
	} else if cfg.Profile != "" {
		// Named AWS profile
		options = append(options, config.WithSharedConfigProfile(cfg.Profile))
	}

	// Custom HTTP client for SSL disable (AWS emulator testing)
	if cfg.DisableSSL {
		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // #nosec G402 -- cfg.DisableSSL is reserved for explicitly configured AWS emulator tests.
				},
			},
		}
		options = append(options, config.WithHTTPClient(httpClient))
	}

	// Note: Custom endpoint resolution is applied at the service client level
	// (in createKeyStore) using modern EndpointResolverV2 pattern to avoid deprecated APIs

	// Load base AWS configuration with all options
	awsConfig, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return aws.Config{}, encryption.NewKEKUnavailableError(
			fmt.Sprintf("failed to load AWS configuration: %v", err),
			err,
		)
	}

	// IAM role assumption (must happen after base config is loaded)
	if cfg.AssumeRoleARN != "" {
		if err := validateRoleARN(cfg.AssumeRoleARN); err != nil {
			return aws.Config{}, err
		}

		// Create STS client for role assumption
		stsClient := sts.NewFromConfig(awsConfig)

		// Create assume role credential provider
		assumeRoleProvider := stscreds.NewAssumeRoleProvider(stsClient, cfg.AssumeRoleARN)

		// Update config with assume role credentials
		awsConfig.Credentials = assumeRoleProvider
	}

	return awsConfig, nil
}

// GetKMSEndpoint returns the configured KMS endpoint URL, or empty string for default.
// This should be passed to the KMS client at creation time using modern endpoint resolution.
func GetKMSEndpoint(cfg *ports.AWSKMSConfig) (string, error) {
	if cfg == nil || cfg.KMSEndpoint == "" {
		return "", nil
	}
	if err := validateEndpointURL(cfg.KMSEndpoint, "kms_endpoint"); err != nil {
		return "", err
	}
	return cfg.KMSEndpoint, nil
}

// GetDynamoDBEndpoint returns the configured DynamoDB endpoint URL, or empty string for default.
// This should be passed to the DynamoDB client at creation time using modern endpoint resolution.
func GetDynamoDBEndpoint(cfg *ports.AWSKMSConfig) (string, error) {
	if cfg == nil || cfg.DynamoDBEndpoint == "" {
		return "", nil
	}
	if err := validateEndpointURL(cfg.DynamoDBEndpoint, "dynamodb_endpoint"); err != nil {
		return "", err
	}
	return cfg.DynamoDBEndpoint, nil
}

// validateEndpointURL validates that a custom endpoint URL is properly formatted.
func validateEndpointURL(endpoint, fieldName string) error {
	if endpoint == "" {
		return nil // Empty endpoint is valid (uses default AWS endpoints)
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return encryption.NewKEKUnavailableError(
			fmt.Sprintf("invalid %s URL format: %s", fieldName, endpoint),
			err,
		)
	}

	// Check for valid schemes (http or https)
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return encryption.NewKEKUnavailableError(
			fmt.Sprintf("%s must include scheme (http:// or https://): %s", fieldName, endpoint),
			nil,
		)
	}

	if parsedURL.Host == "" {
		return encryption.NewKEKUnavailableError(
			fmt.Sprintf("%s must include host: %s", fieldName, endpoint),
			nil,
		)
	}

	return nil
}

// validateRoleARN validates that an IAM role ARN is properly formatted.
func validateRoleARN(roleARN string) error {
	if roleARN == "" {
		return nil // Empty is valid (no role assumption)
	}

	// Basic ARN format validation
	// Expected format: arn:aws:iam::account-id:role/role-name
	if len(roleARN) < 20 { // Minimum realistic ARN length
		return encryption.NewKEKUnavailableError(
			fmt.Sprintf("assume_role_arn too short to be valid ARN: %s", roleARN),
			nil,
		)
	}

	if len(roleARN) < 7 || roleARN[:7] != "arn:aws" {
		return encryption.NewKEKUnavailableError(
			fmt.Sprintf("assume_role_arn must start with 'arn:aws': %s", roleARN),
			nil,
		)
	}

	// More detailed validation could be added here if needed
	// For now, let AWS SDK validate the ARN during actual assumption

	return nil
}

// parseDynamoDBTimeout parses the named dynamodb_timeout value from config.
// Despite the field name, it applies to full top-level AWS encryption operations.
// Returns (0, nil) when awsCfg is nil or DynamoDBTimeout is not set.
func parseDynamoDBTimeout(awsCfg *ports.AWSKMSConfig) (time.Duration, error) {
	if awsCfg == nil || awsCfg.DynamoDBTimeout == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(awsCfg.DynamoDBTimeout)
	if err != nil {
		return 0, fmt.Errorf("invalid dynamodb_timeout %q: %w", awsCfg.DynamoDBTimeout, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid dynamodb_timeout %q: must be a positive duration", awsCfg.DynamoDBTimeout)
	}
	return d, nil
}

// validateSecuritySettings performs basic security validation for configuration warnings.
func validateSecuritySettings(cfg *ports.AWSKMSConfig) error {
	if cfg == nil {
		return nil
	}

	// Warn about disabled SSL verification
	if cfg.DisableSSL {
		fmt.Fprintf(os.Stderr, "WARNING: SSL verification is DISABLED (disable_ssl: true). "+
			"This is DANGEROUS in production and should only be used for AWS emulator testing.\n")
	}

	return nil
}
