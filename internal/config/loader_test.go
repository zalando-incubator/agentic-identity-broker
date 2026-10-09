package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
