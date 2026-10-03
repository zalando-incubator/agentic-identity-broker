package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentEncryptionDocsUseUnifiedAWSemulatorNarrative(t *testing.T) {
	t.Run("vendor-neutral example config exists", func(t *testing.T) {
		_, err := os.Stat(repoPath(t, "examples/config/config.aws-emulator.yaml"))
		require.NoError(t, err, "expected vendor-neutral AWS emulator example config")

		_, err = os.Stat(repoPath(t, "examples/config/config.aws-localstack.yaml"))
		assert.ErrorIs(t, err, os.ErrNotExist, "vendor-branded LocalStack example should be retired")
	})

	cases := []struct {
		path           string
		mustContain    []string
		mustNotContain []string
	}{
		{
			path:           "docs/ENCRYPTION_INTEGRATION_GUIDE.md",
			mustContain:    []string{"LocalStack-compatible AWS emulator"},
			mustNotContain: []string{"with LocalStack for testing"},
		},
		{
			path:           "examples/config/config.aws-emulator.yaml",
			mustContain:    []string{"LocalStack-compatible AWS emulator", "floci/floci", "localstack/localstack"},
			mustNotContain: []string{"Use this configuration for local development and testing with LocalStack"},
		},
		{
			path:           "examples/config/config.aws-production-advanced.yaml",
			mustContain:    []string{"AWS emulator scenarios"},
			mustNotContain: []string{"development/LocalStack scenarios"},
		},
		{
			path:           "internal/ports/config.go",
			mustContain:    []string{"LocalStack-compatible AWS emulator", "Use explicit localhost origins during development"},
			mustNotContain: []string{"testing with LocalStack", "development/testing with LocalStack", "allow all origins"},
		},
		{
			path:           "internal/adapters/encryption/aws/config.go",
			mustContain:    []string{"LocalStack-compatible AWS emulator"},
			mustNotContain: []string{"For LocalStack or custom AWS implementations", "For LocalStack only", "LocalStack testing"},
		},
		{
			path:           "docs/operations/deployment-checklist.md",
			mustContain:    []string{"AWS emulator testing"},
			mustNotContain: []string{"LocalStack testing"},
		},
		{
			path:           "specs/012-aws-encryption-vault/contracts/encryption-port.md",
			mustContain:    []string{"AWS emulator"},
			mustNotContain: []string{"testcontainers with LocalStack"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			content, err := os.ReadFile(repoPath(t, tc.path))
			require.NoError(t, err)

			text := string(content)
			for _, want := range tc.mustContain {
				assert.Contains(t, text, want)
			}
			for _, forbidden := range tc.mustNotContain {
				assert.NotContains(t, text, forbidden)
			}
		})
	}
}

func TestEncryptionTimeoutDocsDescribeOperationScope(t *testing.T) {
	cases := []struct {
		path           string
		mustContain    []string
		mustNotContain []string
	}{
		{
			path:           "docs/operations/deployment-checklist.md",
			mustContain:    []string{"IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT", "encrypt/decrypt and branch-key operations"},
			mustNotContain: []string{"IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_READ_TIMEOUT", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_WRITE_TIMEOUT"},
		},
		{
			path:           "examples/config/config.aws-production-advanced.yaml",
			mustContain:    []string{"all AWS service calls within an encryption operation"},
			mustNotContain: []string{"Timeout configuration for DynamoDB operations."},
		},
		{
			path:           "examples/config/encryption-aws-kms.yaml",
			mustContain:    []string{"all AWS service calls", "encryption operation"},
			mustNotContain: []string{"Context timeout applied to each DynamoDB operation"},
		},
		{
			path:           "internal/adapters/encryption/aws/adapter.go",
			mustContain:    []string{"full top-level encryption operation"},
			mustNotContain: []string{"per-operation DynamoDB timeout"},
		},
		{
			path:           "internal/adapters/encryption/aws/keystore.go",
			mustContain:    []string{"timeout applied to the full branch-key operation"},
			mustNotContain: []string{"timeout applied to each DynamoDB operation"},
		},
		{
			path:           "internal/adapters/encryption/aws/config.go",
			mustContain:    []string{"named dynamodb_timeout value from config", "full top-level AWS encryption operations"},
			mustNotContain: []string{"parses the DynamoDB context timeout duration"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			content, err := os.ReadFile(repoPath(t, tc.path))
			require.NoError(t, err)

			text := string(content)
			for _, want := range tc.mustContain {
				assert.Contains(t, text, want)
			}
			for _, forbidden := range tc.mustNotContain {
				assert.NotContains(t, text, forbidden)
			}
		})
	}
}

func TestHistoricalEncryptionDocsCarrySupersessionNotice(t *testing.T) {
	paths := []string{
		"adrs/009-envelope-encryption-design.md",
		"specs/012-aws-encryption-vault/spec.md",
		"specs/012-aws-encryption-vault/contracts/configuration.md",
		"specs/012-aws-encryption-vault/contracts/aws-kms-architecture.md",
		"specs/012-aws-encryption-vault/contracts/encryption-port.md",
		"specs/012-aws-encryption-vault/contracts/error-contract.md",
		"specs/012-aws-encryption-vault/quickstart.md",
		"specs/012-aws-encryption-vault/plan.md",
		"specs/012-aws-encryption-vault/research.md",
		"specs/012-aws-encryption-vault/data-model.md",
		"specs/012-aws-encryption-vault/tasks.md",
		"specs/006-domain-model-apis/implementation-plan.md",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content, err := os.ReadFile(repoPath(t, path))
			require.NoError(t, err)

			text := string(content)
			assert.Contains(t, text, "superseded configuration")
			assert.Contains(t, text, "encryption.aws_kms")
			assert.Contains(t, text, "encryption.memory")
		})
	}
}

func repoPath(t *testing.T, relativePath string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve caller path")

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	return filepath.Join(repoRoot, relativePath)
}
