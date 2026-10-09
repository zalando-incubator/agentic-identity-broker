package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/configutil"
	domainconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationDefaults(t *testing.T) {
	// Set valid JWESigningKey for all tests

	t.Run("missing authentication config fails validation", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		// Do NOT set principal_header_name or JWT — no authentication method configured
		loader := NewLoader()
		_, err := loader.GetConfig(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication")
	})

	t.Run("custom principal header name from environment variable", func(t *testing.T) {
		// Set environment variable
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Authenticated-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Admin-User")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "X-Authenticated-User", cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName)
		assert.Equal(t, "X-Admin-User", cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName)
	})

	t.Run("authentication configuration is not nil", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.NotNil(t, cfg.Server.EndUser.Authentication)
		assert.NotNil(t, cfg.Server.Admin.Authentication)
		assert.NotNil(t, cfg.Server.EndUser.Authentication.Preauth)
		assert.NotNil(t, cfg.Server.Admin.Authentication.Preauth)
	})
}

func TestDefaultServerConfig(t *testing.T) {
	defaults := ports.DefaultServerConfig()

	t.Run("enduser server has no default principal header", func(t *testing.T) {
		assert.Empty(t, defaults.EndUser.Authentication.Preauth.PrincipalHeaderName)
	})

	t.Run("admin server has no default principal header", func(t *testing.T) {
		assert.Empty(t, defaults.Admin.Authentication.Preauth.PrincipalHeaderName)
	})
}

func TestConfigurationPrecedence(t *testing.T) {
	t.Run("environment variable sets principal header", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Custom-Header")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Admin-Header")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "X-Custom-Header", cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName)
		assert.Equal(t, "X-Admin-Header", cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName)
	})

	t.Run("admin and enduser can have different headers", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-User-Header")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Admin-Header")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "X-User-Header", cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName)
		assert.Equal(t, "X-Admin-Header", cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName)
	})
}

func TestConfigurationSources(t *testing.T) {
	// Set valid JWESigningKey, KeyEncryptionKey, and required principal headers
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
	t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")

	loader := NewLoader()
	_, err := loader.GetConfig(context.Background())
	require.NoError(t, err)

	sources := loader.GetSources()
	assert.Greater(t, len(sources), 0)

	// Verify defaults source is present
	hasDefaults := false
	for _, source := range sources {
		if source.Type == ports.SourceTypeDefault {
			hasDefaults = true
			// Verify authentication defaults are recorded
			// principal_header_name should NOT have a default (security: must be explicit)
			hasAuthEnduser := false
			hasAuthAdmin := false
			for _, key := range source.Keys {
				if key == "server.enduser.authentication.preauth.principal_header_name" {
					hasAuthEnduser = true
				}
				if key == "server.admin.authentication.preauth.principal_header_name" {
					hasAuthAdmin = true
				}
			}
			assert.False(t, hasAuthEnduser, "enduser principal_header_name should not have a default")
			assert.False(t, hasAuthAdmin, "admin principal_header_name should not have a default")
		}
	}
	assert.True(t, hasDefaults, "defaults source should be present")
}

func TestAWSKMSConfigurationEnvironmentVariables(t *testing.T) {
	t.Run("AWS KMS configuration from environment variables", func(t *testing.T) {
		// Set basic required env vars
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN", "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME", "MyBranchKeys")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_REGION", "us-east-1")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ASSUME_ROLE_ARN", "arn:aws:iam::123456789012:role/EncryptionRole-prod")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ENDPOINT", "http://localhost:4566")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_ENDPOINT", "http://localhost:8000")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_PROFILE", "production")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DISABLE_SSL", "false")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		require.NotNil(t, cfg.Encryption.AWSKMS)
		assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012", cfg.Encryption.AWSKMS.KeyARN)
		assert.Equal(t, "MyBranchKeys", cfg.Encryption.AWSKMS.DynamoDBTableName)
		assert.Equal(t, "us-east-1", cfg.Encryption.AWSKMS.Region)
		assert.Equal(t, "arn:aws:iam::123456789012:role/EncryptionRole-prod", cfg.Encryption.AWSKMS.AssumeRoleARN)
		assert.Equal(t, "http://localhost:4566", cfg.Encryption.AWSKMS.KMSEndpoint)
		assert.Equal(t, "http://localhost:8000", cfg.Encryption.AWSKMS.DynamoDBEndpoint)
		assert.Equal(t, "production", cfg.Encryption.AWSKMS.Profile)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", cfg.Encryption.AWSKMS.AccessKeyID)
		assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", cfg.Encryption.AWSKMS.SecretAccessKey)
		assert.Equal(t, false, cfg.Encryption.AWSKMS.DisableSSL)
	})

	t.Run("DynamoDB timeout configuration from environment variables", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN", "arn:aws:kms:us-east-1:123456789012:key/12345678")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT", "10s")
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL", "30m")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		require.NotNil(t, cfg.Encryption.AWSKMS)
		assert.Equal(t, "10s", cfg.Encryption.AWSKMS.DynamoDBTimeout)
		assert.Equal(t, "30m", cfg.Encryption.AWSKMS.BranchKeyTTL)
	})
}

func TestAWSKMSDisableSSLStartupValidation(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		disableSSL  string
		yaml        bool
		wantError   bool
	}{
		{name: "production environment variable", environment: "production", disableSSL: "true", wantError: true},
		{name: "production YAML", environment: "production", disableSSL: "true", yaml: true, wantError: true},
		{name: "staging", environment: "staging", disableSSL: "true", wantError: true},
		{name: "unset environment", disableSSL: "true", wantError: true},
		{name: "unknown environment", environment: "prod", disableSSL: "true", wantError: true},
		{name: "explicit development", environment: "development", disableSSL: "true"},
		{name: "production with verification", environment: "production", disableSSL: "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setMinimalConfigEnv(t)
			t.Setenv("GO_ENV", tt.environment)
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", "")
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN", "arn:aws:kms:us-east-1:123456789012:key/test-key")
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", "config.yaml")
			if tt.yaml {
				t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DISABLE_SSL", "")
				require.NoError(t, os.WriteFile("config.yaml", []byte("encryption:\n  aws_kms:\n    disable_ssl: "+tt.disableSSL+"\n"), 0o600))
			} else {
				t.Setenv("IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DISABLE_SSL", tt.disableSSL)
			}

			cfg, err := NewLoader().GetConfig(context.Background())
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, cfg)
				var configErr *domainconfig.ConfigError
				require.ErrorAs(t, err, &configErr)
				assert.Equal(t, "encryption.aws_kms.disable_ssl", configErr.Field)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg.Encryption.AWSKMS)
				assert.Equal(t, tt.disableSSL == "true", cfg.Encryption.AWSKMS.DisableSSL)
			}
		})
	}
}

// setMinimalConfigEnv configures the minimum set of environment variables required to
// pass config validation, matching the pattern used throughout this test file.
// It uses t.Setenv so all variables are automatically restored after the test.
func setMinimalConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
	t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
}

func TestImpersonationEnvironmentVariableNoLongerOverridesYAML(t *testing.T) {
	setMinimalConfigEnv(t)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`oauth2_authorization_server:
  impersonation:
    audience_prefix: https://yaml.example.com/impersonation
    rules:
      - name: gateway
        roles:
          client_assertion: {expected_audience: https://yaml.example.com/impersonation, principal_expression: client_assertion.sub}
          actor: {expected_audience: https://yaml.example.com/impersonation, principal_expression: actor_token.sub}
          subject: {expected_audience: https://yaml.example.com/impersonation, principal_expression: subject_token.sub}
        trusted_issuers:
          - {issuer_uri: https://idp.example.com, allowed_algorithms: [ES256], signs_roles: [client_assertion, actor, subject]}
        authorization: {type: cel, cel: {expression: "true"}}
`), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_IMPERSONATION", `{
		"audience_prefix": "https://env.example.com/impersonation",
		"rules": [{
			"name": "gateway",
			"roles": {
				"client_assertion": {"expected_audience": "https://env.example.com/impersonation", "principal_expression": "client_assertion.sub"},
				"actor": {"expected_audience": "https://env.example.com/impersonation", "principal_expression": "actor_token.sub"},
				"subject": {"expected_audience": "https://env.example.com/impersonation", "principal_expression": "subject_token.sub"}
			},
			"trusted_issuers": [{"issuer_uri": "https://idp.example.com", "allowed_algorithms": ["ES256"], "signs_roles": ["client_assertion", "actor", "subject"]}],
			"authorization": {"type": "cel", "cel": {"expression": "true"}}
		}]
	}`)

	loader := NewLoader()
	cfg, err := loader.GetConfig(context.Background())

	require.NoError(t, err)
	require.NotNil(t, cfg.OAuth2AuthServer.Impersonation)
	assert.Equal(t, "https://yaml.example.com/impersonation", cfg.OAuth2AuthServer.Impersonation.AudiencePrefix)
}

func TestConfigLoader_MissingOAuth2Mode(t *testing.T) {
	t.Run("GetConfig fails when oauth2_authorization_server.mode is not set", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		// Intentionally not setting IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE

		loader := NewLoader()
		_, err := loader.GetConfig(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "oauth2_authorization_server.mode")
	})
}

func TestTokenExchangeExpectedAudienceConfiguration(t *testing.T) {
	t.Run("default expected_audience is token-exchange-broker when not configured", func(t *testing.T) {
		setMinimalConfigEnv(t)
		// No IDENTITY_BROKER_TOKEN_EXCHANGE_EXPECTED_AUDIENCE set

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "token-exchange-broker", cfg.TokenExchange.ExpectedAudience,
			"expected_audience should default to token-exchange-broker via Viper SetDefault")
	})

	t.Run("expected_audience is overridden by environment variable", func(t *testing.T) {
		setMinimalConfigEnv(t)
		t.Setenv("IDENTITY_BROKER_TOKEN_EXCHANGE_EXPECTED_AUDIENCE", "my-custom-gateway-audience")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "my-custom-gateway-audience", cfg.TokenExchange.ExpectedAudience,
			"expected_audience should be overridden by IDENTITY_BROKER_TOKEN_EXCHANGE_EXPECTED_AUDIENCE env var")
	})
}

func TestSigningKeyBootstrapTimeoutConfiguration(t *testing.T) {
	t.Run("bootstrap timeout is overridden by environment variable", func(t *testing.T) {
		setMinimalConfigEnv(t)
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_SIGNING_KEYS_BOOTSTRAP_TIMEOUT", "37s")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, 37*time.Second, cfg.OAuth2AuthServer.Local.SigningKeys.BootstrapTimeout)
	})
}

func TestConfigLoader_UpstreamTimeout(t *testing.T) {
	t.Run("proxy upstream timeout can be configured via environment variable", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "proxy")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_ISSUER_URI", "https://idp.example.com")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_AUTHORIZE_ENDPOINT", "https://idp.example.com/oauth2/authorize")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TIMEOUT", "45s")

		loader := NewLoader()
		cfg, err := loader.GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, 45*time.Second, cfg.OAuth2AuthServer.Proxy.UpstreamTimeout)
	})

	assertLoaderRejectsInvalidUpstreamTimeout := func(t *testing.T, upstreamTimeoutYAML string) {
		t.Helper()

		t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", generateBase64EncodedString(t, 32))
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
		t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")

		configPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(`oauth2_authorization_server:
  mode: proxy
  proxy:
    upstream_issuer_uri: https://idp.example.com
    upstream_authorize_endpoint: https://idp.example.com/oauth2/authorize
    upstream_token_endpoint: https://idp.example.com/oauth2/token
    upstream_timeout: `+upstreamTimeoutYAML+`
`), 0o600))
		t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)

		loader := NewLoader()
		_, err := loader.GetConfig(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream_timeout")
		assert.Contains(t, err.Error(), "duration")
		assert.NotContains(t, err.Error(), `field "config"`)
	}

	t.Run("bare numeric YAML upstream timeout is rejected", func(t *testing.T) {
		assertLoaderRejectsInvalidUpstreamTimeout(t, "30")
	})

	t.Run("empty string YAML upstream timeout is rejected", func(t *testing.T) {
		assertLoaderRejectsInvalidUpstreamTimeout(t, `""`)
	})

	t.Run("float YAML upstream timeout is rejected", func(t *testing.T) {
		assertLoaderRejectsInvalidUpstreamTimeout(t, "30.5")
	})
}

func TestRequestContextConfigurationDefaults(t *testing.T) {
	setMinimalConfigEnv(t)

	loader := NewLoader()
	cfg, err := loader.GetConfig(context.Background())

	require.NoError(t, err)
	assert.False(t, cfg.RequestContext.TrustedProxy.Enabled)
	assert.Equal(t, "X-Forwarded-For", cfg.RequestContext.TrustedProxy.ForwardedHeader)
	assert.True(t, cfg.RequestContext.Trace.ResponseEnabled)
}

func TestRequestContextConfigurationEnvOverrides(t *testing.T) {
	setMinimalConfigEnv(t)
	t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_ENABLED", "true")
	t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_FORWARDED_HEADER", "X-Real-IP")
	t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRACE_RESPONSE_ENABLED", "false")

	loader := NewLoader()
	cfg, err := loader.GetConfig(context.Background())

	require.NoError(t, err)
	assert.True(t, cfg.RequestContext.TrustedProxy.Enabled)
	assert.Equal(t, "X-Real-IP", cfg.RequestContext.TrustedProxy.ForwardedHeader)
	assert.False(t, cfg.RequestContext.Trace.ResponseEnabled)
}

func TestRequestContextConfigurationSources(t *testing.T) {
	setMinimalConfigEnv(t)

	loader := NewLoader()
	_, err := loader.GetConfig(context.Background())
	require.NoError(t, err)

	for _, source := range loader.GetSources() {
		if source.Type != ports.SourceTypeDefault {
			continue
		}

		assert.Contains(t, source.Keys, "request_context.trusted_proxy.enabled")
		assert.Contains(t, source.Keys, "request_context.trusted_proxy.forwarded_header")
		assert.Contains(t, source.Keys, "request_context.trace.response_enabled")
		return
	}

	t.Fatal("defaults source not found")
}

func TestRequestContextConfigurationCLIOverridesEnvironment(t *testing.T) {
	setMinimalConfigEnv(t)
	t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_ENABLED", "false")
	t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_FORWARDED_HEADER", "X-Forwarded-For")
	t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRACE_RESPONSE_ENABLED", "true")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("request_context.trusted_proxy.enabled", false, "")
	cmd.Flags().String("request_context.trusted_proxy.forwarded_header", "", "")
	cmd.Flags().Bool("request_context.trace.response_enabled", true, "")
	require.NoError(t, cmd.ParseFlags([]string{
		"--request_context.trusted_proxy.enabled=true",
		"--request_context.trusted_proxy.forwarded_header=X-Real-IP",
		"--request_context.trace.response_enabled=false",
	}))

	loader := NewLoader()
	loader.SetCommand(cmd)
	cfg, err := loader.GetConfig(context.Background())

	require.NoError(t, err)
	assert.True(t, cfg.RequestContext.TrustedProxy.Enabled)
	assert.Equal(t, "X-Real-IP", cfg.RequestContext.TrustedProxy.ForwardedHeader)
	assert.False(t, cfg.RequestContext.Trace.ResponseEnabled)

	for _, source := range loader.GetSources() {
		if source.Type != ports.SourceTypeCLI {
			continue
		}

		assert.ElementsMatch(t, []string{
			"request_context.trusted_proxy.enabled",
			"request_context.trusted_proxy.forwarded_header",
			"request_context.trace.response_enabled",
		}, source.Keys)
		return
	}

	t.Fatal("CLI source not found")
}

func TestConfigLoader_CIMDSSRFValidationEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		skip        bool
		cimdEnabled bool
		wantErr     bool
	}{
		{name: "production rejects bypass", environment: "production", skip: true, cimdEnabled: true, wantErr: true},
		{name: "production rejects dormant bypass", environment: "production", skip: true, wantErr: true},
		{name: "production retains protection", environment: "production", cimdEnabled: true},
		{name: "development permits local mocks", environment: "development", skip: true, cimdEnabled: true},
		{name: "test rejects bypass", environment: "test", skip: true, cimdEnabled: true, wantErr: true},
		{name: "staging rejects bypass", environment: "staging", skip: true, cimdEnabled: true, wantErr: true},
		{name: "empty environment rejects bypass", skip: true, cimdEnabled: true, wantErr: true},
		{name: "misspelled development rejects bypass", environment: "developement", skip: true, cimdEnabled: true, wantErr: true},
		{name: "uppercase development rejects bypass", environment: "DEVELOPMENT", skip: true, cimdEnabled: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			setMinimalConfigEnv(t)
			t.Setenv("GO_ENV", tt.environment)
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`security:
  skip_cimd_ssrf_validation: %t
oauth2_authorization_server:
  cimd:
    enabled: %t
`, tt.skip, tt.cimdEnabled)), 0o600))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)

			cfg, err := NewLoader().GetConfig(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), "security.skip_cimd_ssrf_validation")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.skip, cfg.Security.SkipCIMDSSRFValidation)
		})
	}
}

func TestConfigLoaderTelemetryLiteralMaps(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		attributes map[string]string
		headers    map[string]string
	}{
		{
			name: "literal dotted resource and header keys",
			yaml: `telemetry:
  resource_attributes:
    service.namespace: production
    deployment.environment.name: staging
  exporter:
    headers:
      x.vendor.token: secret
      x.vendor.region: eu
`,
			attributes: map[string]string{"service.namespace": "production", "deployment.environment.name": "staging"},
			headers:    map[string]string{"x.vendor.token": "secret", "x.vendor.region": "eu"},
		},
		{
			name: "scalar keys coexist with dotted keys",
			yaml: `telemetry:
  resource_attributes:
    service: broker
    service.namespace: production
    service.version: release
  exporter:
    headers:
      x: ordinary
      x.vendor.token: secret
      x.vendor.region: eu
`,
			attributes: map[string]string{"service": "broker", "service.namespace": "production", "service.version": "release"},
			headers:    map[string]string{"x": "ordinary", "x.vendor.token": "secret", "x.vendor.region": "eu"},
		},
		{
			name: "multiple keys sharing several prefixes",
			yaml: `telemetry:
  resource_attributes:
    service.namespace: production
    service.namespace.region: eu
    deployment.environment.name: staging
    deployment.environment.region: us
  exporter:
    headers:
      x.vendor.token: secret
      x.vendor.token.scope: traces
      x.region.name: eu
      x.region.zone: west
`,
			attributes: map[string]string{
				"service.namespace": "production", "service.namespace.region": "eu",
				"deployment.environment.name": "staging", "deployment.environment.region": "us",
			},
			headers: map[string]string{
				"x.vendor.token": "secret", "x.vendor.token.scope": "traces",
				"x.region.name": "eu", "x.region.zone": "west",
			},
		},
		{
			name: "ordinary string map keys",
			yaml: `telemetry:
  resource_attributes:
    region: eu
    environment: staging
  exporter:
    headers:
      authorization: Bearer secret
      tenant: production
`,
			attributes: map[string]string{"region": "eu", "environment": "staging"},
			headers:    map[string]string{"authorization": "Bearer secret", "tenant": "production"},
		},
		{name: "missing maps stay nil", yaml: "telemetry: {}\n"},
		{
			name: "explicit empty maps stay nil",
			yaml: "telemetry:\n  resource_attributes: {}\n  exporter:\n    headers: {}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalConfigEnv(t)
			setConfigYAML(t, tt.yaml)
			loader := NewLoader()

			cfg, err := loader.GetConfig(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.attributes, cfg.Telemetry.ResourceAttributes)
			assert.Equal(t, tt.headers, cfg.Telemetry.Exporter.Headers)
		})
	}
}

func TestConfigLoaderRejectsNestedTelemetryMaps(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		field string
	}{
		{
			name:  "nested resource attributes are not string values",
			yaml:  "telemetry:\n  resource_attributes:\n    service:\n      namespace: production\n",
			field: "resource_attributes",
		},
		{
			name:  "nested exporter headers are not string values",
			yaml:  "telemetry:\n  exporter:\n    headers:\n      x:\n        vendor: secret\n",
			field: "headers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalConfigEnv(t)
			setConfigYAML(t, tt.yaml)

			_, err := NewLoader().GetConfig(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
			assert.Contains(t, err.Error(), "string")
		})
	}
}

func TestConfigLoaderTelemetryMapInterpolation(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		token     string
	}{
		{name: "production map values", namespace: "production", token: "production-secret"},
		{name: "staging map values", namespace: "staging", token: "staging-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalConfigEnv(t)
			t.Setenv("BROKER_RESOURCE_NAMESPACE", tt.namespace)
			t.Setenv("BROKER_EXPORTER_TOKEN", tt.token)
			setConfigYAML(t, `telemetry:
  resource_attributes:
    service: broker
    service.namespace: ${BROKER_RESOURCE_NAMESPACE}
    service.namespace.region: eu
  exporter:
    headers:
      x.vendor.token: Bearer ${BROKER_EXPORTER_TOKEN}
      x.vendor.region: eu
`)

			cfg, err := NewLoader().GetConfig(context.Background())

			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				"service": "broker", "service.namespace": tt.namespace, "service.namespace.region": "eu",
			}, cfg.Telemetry.ResourceAttributes)
			assert.Equal(t, map[string]string{
				"x.vendor.token": "Bearer " + tt.token, "x.vendor.region": "eu",
			}, cfg.Telemetry.Exporter.Headers)
		})
	}
}

func TestConfigLoaderTelemetryMapsUnmarshalDoesNotMutateSource(t *testing.T) {
	loader := NewLoader()
	loader.v.SetConfigType("yaml")
	require.NoError(t, loader.v.ReadConfig(strings.NewReader(`telemetry:
  resource_attributes:
    service: broker
    service.namespace: production
    service.namespace.region: eu
  exporter:
    headers:
      x.vendor.token: secret
      x.vendor.token.scope: traces
`)))
	source := loader.v.Get(configutil.Key("telemetry"))
	snapshot, err := json.Marshal(source)
	require.NoError(t, err)
	var first ports.Config
	require.NoError(t, loader.v.Unmarshal(&first))
	afterFirst, err := json.Marshal(source)
	require.NoError(t, err)
	assert.Equal(t, snapshot, afterFirst, "the first Unmarshal must preserve the deep source snapshot")
	var second ports.Config
	require.NoError(t, loader.v.Unmarshal(&second))

	attributes := map[string]string{"service": "broker", "service.namespace": "production", "service.namespace.region": "eu"}
	headers := map[string]string{"x.vendor.token": "secret", "x.vendor.token.scope": "traces"}
	assert.Equal(t, attributes, first.Telemetry.ResourceAttributes)
	assert.Equal(t, headers, first.Telemetry.Exporter.Headers)
	assert.Equal(t, attributes, second.Telemetry.ResourceAttributes)
	assert.Equal(t, headers, second.Telemetry.Exporter.Headers)
	assert.Equal(t, first.Telemetry, second.Telemetry)
	unchanged, err := json.Marshal(source)
	require.NoError(t, err)
	assert.Equal(t, snapshot, unchanged, "Unmarshal must not modify the original nested source maps")
	current, err := json.Marshal(loader.v.Get(configutil.Key("telemetry")))
	require.NoError(t, err)
	assert.Equal(t, snapshot, current, "the same Viper instance must retain its original maps")
}

func TestConfigLoaderScalarPrecedence(t *testing.T) {
	const scalarYAML = `log:
  level: warn
server:
  enduser:
    port: 8100
  shutdown:
    timeout: 25s
`
	tests := []struct {
		name        string
		yaml        string
		envLevel    string
		envPort     string
		envTimeout  string
		flags       []string
		wantLevel   string
		wantPort    int
		wantTimeout time.Duration
	}{
		{
			name: "CLI overrides environment YAML and defaults", yaml: scalarYAML,
			envLevel: "debug", envPort: "8200", envTimeout: "40s",
			flags:     []string{"--log-level=error", "--server.enduser.port=8300", "--server.shutdown.timeout=45s"},
			wantLevel: "error", wantPort: 8300, wantTimeout: 45 * time.Second,
		},
		{
			name: "environment overrides YAML and defaults", yaml: scalarYAML,
			envLevel: "debug", envPort: "8200", envTimeout: "40s",
			wantLevel: "debug", wantPort: 8200, wantTimeout: 40 * time.Second,
		},
		{
			name: "YAML overrides defaults", yaml: scalarYAML,
			wantLevel: "warn", wantPort: 8100, wantTimeout: 25 * time.Second,
		},
		{
			name: "defaults remain when higher sources are absent", yaml: "{}\n",
			wantLevel: "info", wantPort: ports.DefaultServerConfig().EndUser.Port,
			wantTimeout: ports.DefaultServerConfig().Shutdown.Timeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalConfigEnv(t)
			setConfigYAML(t, tt.yaml)
			t.Setenv("IDENTITY_BROKER_LOG_LEVEL", tt.envLevel)
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_PORT", tt.envPort)
			t.Setenv("IDENTITY_BROKER_SERVER_SHUTDOWN_TIMEOUT", tt.envTimeout)
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("log-level", "info", "")
			cmd.Flags().Int("server.enduser.port", 8000, "")
			cmd.Flags().Duration("server.shutdown.timeout", 30*time.Second, "")
			require.NoError(t, cmd.ParseFlags(tt.flags))
			loader := NewLoader()
			loader.SetCommand(cmd)

			cfg, err := loader.GetConfig(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.wantLevel, string(cfg.Log.Level))
			assert.Equal(t, tt.wantPort, cfg.Server.EndUser.Port)
			assert.Equal(t, tt.wantTimeout, cfg.Server.Shutdown.Timeout)
		})
	}
}

func TestConfigLoaderCLIFlagsPreservePublicNames(t *testing.T) {
	setMinimalConfigEnv(t)
	setConfigYAML(t, "{}\n")
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("log-level", "info", "")
	cmd.Flags().String("log-format", "text", "")
	cmd.Flags().Int("server.enduser.port", 8000, "")
	cmd.Flags().String("server.enduser.bind", "::", "")
	cmd.Flags().Int("server.admin.port", 14000, "")
	cmd.Flags().String("server.admin.bind", "::", "")
	cmd.Flags().Duration("server.shutdown.timeout", 30*time.Second, "")
	require.NoError(t, cmd.ParseFlags([]string{
		"--log-level=debug", "--log-format=json",
		"--server.enduser.port=8100", "--server.enduser.bind=127.0.0.1",
		"--server.admin.port=14100", "--server.admin.bind=::1",
		"--server.shutdown.timeout=45s",
	}))
	loader := NewLoader()
	loader.SetCommand(cmd)

	cfg, err := loader.GetConfig(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "debug", string(cfg.Log.Level))
	assert.Equal(t, "json", string(cfg.Log.Format))
	assert.Equal(t, 8100, cfg.Server.EndUser.Port)
	assert.Equal(t, "127.0.0.1", cfg.Server.EndUser.Bind)
	assert.Equal(t, 14100, cfg.Server.Admin.Port)
	assert.Equal(t, "::1", cfg.Server.Admin.Bind)
	assert.Equal(t, 45*time.Second, cfg.Server.Shutdown.Timeout)
	for _, source := range loader.GetSources() {
		if source.Type != ports.SourceTypeCLI {
			continue
		}
		assert.ElementsMatch(t, []string{
			"log.level", "log.format", "server.enduser.port", "server.enduser.bind",
			"server.admin.port", "server.admin.bind", "server.shutdown.timeout",
		}, source.Keys)
		return
	}
	t.Fatal("CLI source not found")
}

func TestConfigLoaderFalseAndZeroOverrides(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		envResponse string
		envSampling string
		flags       []string
	}{
		{
			name: "YAML false and zero override defaults",
			yaml: "request_context:\n  trace:\n    response_enabled: false\ntelemetry:\n  traces:\n    sampling_rate: 0\n",
		},
		{
			name:        "environment false and zero override YAML",
			yaml:        "request_context:\n  trace:\n    response_enabled: true\ntelemetry:\n  traces:\n    sampling_rate: 0.5\n",
			envResponse: "false", envSampling: "0",
		},
		{
			name:        "CLI false overrides true environment and YAML",
			yaml:        "request_context:\n  trace:\n    response_enabled: true\ntelemetry:\n  traces:\n    sampling_rate: 0.5\n",
			envResponse: "true", envSampling: "0",
			flags: []string{"--request_context.trace.response_enabled=false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalConfigEnv(t)
			setConfigYAML(t, tt.yaml)
			t.Setenv("IDENTITY_BROKER_REQUEST_CONTEXT_TRACE_RESPONSE_ENABLED", tt.envResponse)
			t.Setenv("IDENTITY_BROKER_TELEMETRY_TRACES_SAMPLING_RATE", tt.envSampling)
			t.Setenv("IDENTITY_BROKER_TELEMETRY_ENABLED", "true")
			t.Setenv("IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT", "collector:4317")
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Bool("request_context.trace.response_enabled", true, "")
			require.NoError(t, cmd.ParseFlags(tt.flags))
			loader := NewLoader()
			loader.SetCommand(cmd)

			cfg, err := loader.GetConfig(context.Background())

			require.NoError(t, err)
			assert.False(t, cfg.RequestContext.Trace.ResponseEnabled)
			assert.Zero(t, cfg.Telemetry.Traces.SamplingRate)
		})
	}
}

func TestConfigLoaderSourceMetadataRemainsDotReadable(t *testing.T) {
	t.Run("YAML keys preserve readable structural and literal dots", func(t *testing.T) {
		setMinimalConfigEnv(t)
		setConfigYAML(t, `log:
  level: warn
server:
  enduser:
    port: 8100
telemetry:
  resource_attributes:
    service.namespace: production
  exporter:
    headers:
      x.vendor.token: secret
`)
		loader := NewLoader()
		_, err := loader.GetConfig(context.Background())
		require.NoError(t, err)
		for _, source := range loader.GetSources() {
			for _, key := range source.Keys {
				assert.NotContains(t, key, configutil.Delimiter)
			}
			if source.Type != ports.SourceTypeYAML {
				continue
			}
			assert.Contains(t, source.Keys, "log.level")
			assert.Contains(t, source.Keys, "server.enduser.port")
			assert.Contains(t, source.Keys, "telemetry.resource_attributes.service.namespace")
			assert.Contains(t, source.Keys, "telemetry.exporter.headers.x.vendor.token")
			return
		}
		t.Fatal("YAML source not found")
	})

	t.Run("env file paths use internal delimiter and readable metadata", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_LOG_LEVEL", "")
		t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_PORT", "")
		t.Setenv("CUSTOM_VALUE", "")
		envPath := filepath.Join(t.TempDir(), ".env")
		require.NoError(t, os.WriteFile(envPath, []byte("IDENTITY_BROKER_LOG_LEVEL=debug\nIDENTITY_BROKER_SERVER_ENDUSER_PORT=8200\nCUSTOM_VALUE=ordinary\n"), 0o600))
		loader := NewLoader()
		loader.setDefaults()
		require.NoError(t, loader.loadEnvFile(envPath))
		var cfg ports.Config
		require.NoError(t, loader.v.Unmarshal(&cfg))
		assert.Equal(t, "debug", string(cfg.Log.Level))
		assert.Equal(t, 8200, cfg.Server.EndUser.Port)
		assert.Equal(t, "ordinary", loader.v.GetString(configutil.Key("custom", "value")))
		for _, source := range loader.GetSources() {
			if source.Type != ports.SourceTypeEnvFile {
				continue
			}
			assert.ElementsMatch(t, []string{"log.level", "server.enduser.port", "custom.value"}, source.Keys)
			return
		}
		t.Fatal("env file source not found")
	})
}

func setConfigYAML(t *testing.T, contents string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(contents), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)
}
