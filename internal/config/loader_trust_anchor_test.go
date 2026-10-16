package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const trustAnchorTestKeySet = `{"keys":[{"kty":"EC","crv":"P-256","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0","kid":"gw-1","alg":"ES256"}]}`

const trustAnchorMultilineKeySet = `{
  "keys": [
    {"kty": "EC", "crv": "P-256", "kid": "gw-1", "alg": "ES256",
     "x": "f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU",
     "y": "x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0"}
  ]
}`

func writeTrustAnchorConfig(t *testing.T, yaml string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
}

func TestTrustAnchorKeySourceEnvironmentVariables(t *testing.T) {
	t.Run("client-assertion jwks loads multi-line JSON without expansion", func(t *testing.T) {
		setMinimalConfigEnv(t)
		t.Setenv("IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_ISSUER_URI", "https://gateway.example.com")
		t.Setenv("IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS", trustAnchorMultilineKeySet)

		cfg, err := NewLoader().GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, trustAnchorMultilineKeySet, cfg.TokenExchange.ClientAssertion.JWKS)
	})

	t.Run("client-assertion jwks_file and the trust directory load", func(t *testing.T) {
		setMinimalConfigEnv(t)
		t.Setenv("IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_ISSUER_URI", "https://gateway.example.com")
		t.Setenv("IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_FILE", "/etc/agentic-identity-broker/trust/gateway.json")
		t.Setenv("IDENTITY_BROKER_SECURITY_TRUST_MATERIAL_DIR", "/etc/agentic-identity-broker/trust")

		cfg, err := NewLoader().GetConfig(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "/etc/agentic-identity-broker/trust/gateway.json", cfg.TokenExchange.ClientAssertion.JWKSFile)
		assert.Equal(t, "/etc/agentic-identity-broker/trust", cfg.Security.TrustMaterialDir)
	})
}

func TestTrustAnchorKeySourceYAML(t *testing.T) {
	t.Run("a jwks block scalar loads for every trust anchor", func(t *testing.T) {
		setMinimalConfigEnv(t)
		writeTrustAnchorConfig(t, `token_exchange:
  client_assertion:
    issuer_uri: https://gateway.example.com
    jwks: |
      `+trustAnchorTestKeySet+`
oauth2_authorization_server:
  impersonation:
    audience_prefix: https://broker.example.com/impersonation
    rules:
      - name: gateway
        roles:
          client_assertion: {expected_audience: https://broker.example.com/impersonation, principal_expression: client_assertion.sub}
          actor: {expected_audience: https://broker.example.com/impersonation, principal_expression: actor_token.sub}
          subject: {expected_audience: https://broker.example.com/impersonation, principal_expression: subject_token.sub}
        trusted_issuers:
          - issuer_uri: https://gateway.example.com
            jwks: |
              `+trustAnchorTestKeySet+`
            allowed_algorithms: [ES256]
            signs_roles: [client_assertion, actor, subject]
        authorization: {type: cel, cel: {expression: "true"}}
server:
  enduser:
    authentication:
      jwt:
        jwks: |
          `+trustAnchorTestKeySet+`
  admin:
    authentication:
      jwt:
        jwks: |
          `+trustAnchorTestKeySet+`
`)

		cfg, err := NewLoader().GetConfig(context.Background())

		require.NoError(t, err)
		assert.JSONEq(t, trustAnchorTestKeySet, cfg.TokenExchange.ClientAssertion.JWKS)
		require.NotNil(t, cfg.OAuth2AuthServer.Impersonation)
		assert.JSONEq(t, trustAnchorTestKeySet, cfg.OAuth2AuthServer.Impersonation.Rules[0].TrustedIssuers[0].JWKS)
		require.NotNil(t, cfg.Server.EndUser.Authentication.JWT)
		assert.JSONEq(t, trustAnchorTestKeySet, cfg.Server.EndUser.Authentication.JWT.JWKS)
		require.NotNil(t, cfg.Server.Admin.Authentication.JWT)
		assert.JSONEq(t, trustAnchorTestKeySet, cfg.Server.Admin.Authentication.JWT.JWKS)
	})

	t.Run("a whole-value ${VAR} reference loads a multi-line environment value", func(t *testing.T) {
		setMinimalConfigEnv(t)
		t.Setenv("MESH_JWKS", trustAnchorMultilineKeySet)
		writeTrustAnchorConfig(t, `server:
  enduser:
    authentication:
      jwt:
        jwks: ${MESH_JWKS}
`)

		cfg, err := NewLoader().GetConfig(context.Background())

		require.NoError(t, err)
		require.NotNil(t, cfg.Server.EndUser.Authentication.JWT)
		assert.Equal(t, trustAnchorMultilineKeySet, cfg.Server.EndUser.Authentication.JWT.JWKS)
	})

	t.Run("a multi-line value that contains a ${VAR} reference is rejected, as before", func(t *testing.T) {
		setMinimalConfigEnv(t)
		t.Setenv("MESH_KEY", `{"kty":"EC"}`)
		writeTrustAnchorConfig(t, `server:
  enduser:
    authentication:
      jwt:
        jwks: |
          {"keys": [
            ${MESH_KEY}
          ]}
`)

		_, err := NewLoader().GetConfig(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "shell metacharacter")
	})
}
