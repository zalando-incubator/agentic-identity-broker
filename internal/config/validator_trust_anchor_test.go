package config

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	testTrustDir   = "/etc/agentic-identity-broker/trust"
	inlineRedacted = "[inline JWK Set redacted]"
)

func requireConfigError(t *testing.T, err error, field, value, expected string) {
	t.Helper()
	require.Error(t, err)
	var configErr *domconfig.ConfigError
	require.True(t, errors.As(err, &configErr), "expected *ConfigError, got %T: %v", err, err)
	assert.Equal(t, field, configErr.Field)
	assert.Equal(t, value, configErr.Value)
	assert.Equal(t, expected, configErr.Expected)
	assert.NotContains(t, err.Error(), trustAnchorTestKeySet)
	assert.NotContains(t, err.Error(), "f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU")
}

func signedJWT() *ports.JWTConfig {
	return &ports.JWTConfig{HeaderName: "Authorization", Verification: "jwks"}
}

func TestValidateTokenExchangeKeySources(t *testing.T) {
	security := &ports.SecurityConfig{TrustMaterialDir: testTrustDir}
	tests := []struct {
		name     string
		trust    ports.ClientAssertionTrustConfig
		field    string
		value    string
		expected string
	}{
		{name: "two sources", trust: ports.ClientAssertionTrustConfig{JWKSURI: "https://idp.example.com/keys", JWKS: trustAnchorTestKeySet},
			field: "token_exchange.client_assertion", value: inlineRedacted,
			expected: "only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)"},
		{name: "inline with refresh bounds", trust: ports.ClientAssertionTrustConfig{JWKS: trustAnchorTestKeySet, JWKSMaxRefresh: time.Hour},
			field: "token_exchange.client_assertion", value: inlineRedacted,
			expected: "jwks_min_refresh and jwks_max_refresh are not allowed with jwks; an inline key set never refreshes"},
		{name: "relative file", trust: ports.ClientAssertionTrustConfig{JWKSFile: "trust/gw.json"},
			field: "token_exchange.client_assertion", value: "trust/gw.json",
			expected: `jwks_file "trust/gw.json" must be an absolute path`},
		{name: "file outside the trust directory", trust: ports.ClientAssertionTrustConfig{JWKSFile: "/etc/agentic-identity-broker/config.yaml"},
			field: "token_exchange.client_assertion", value: "/etc/agentic-identity-broker/config.yaml",
			expected: `jwks_file "/etc/agentic-identity-broker/config.yaml" is outside security.trust_material_dir "` + testTrustDir + `"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTokenExchangeConfig(&ports.TokenExchangeConfig{ClientAssertion: tc.trust}, security)
			requireConfigError(t, err, tc.field, tc.value, tc.expected)
		})
	}

	t.Run("a file without a trust directory", func(t *testing.T) {
		err := validateTokenExchangeConfig(&ports.TokenExchangeConfig{ClientAssertion: ports.ClientAssertionTrustConfig{
			JWKSFile: testTrustDir + "/gw.json",
		}}, &ports.SecurityConfig{})
		requireConfigError(t, err, "token_exchange.client_assertion", testTrustDir+"/gw.json",
			"jwks_file requires security.trust_material_dir to be set")
	})

	t.Run("valid inline and file sources", func(t *testing.T) {
		require.NoError(t, validateTokenExchangeConfig(&ports.TokenExchangeConfig{ClientAssertion: ports.ClientAssertionTrustConfig{
			IssuerURI: "https://gateway.example.com", JWKS: trustAnchorTestKeySet,
		}}, security))
		require.NoError(t, validateTokenExchangeConfig(&ports.TokenExchangeConfig{ClientAssertion: ports.ClientAssertionTrustConfig{
			JWKSFile: testTrustDir + "/gw.json", JWKSMinRefresh: time.Minute,
		}}, security))
	})
}

func TestValidateClientAssertionIssuerInLocalMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		local bool
		trust ports.ClientAssertionTrustConfig
		fails bool
	}{
		{name: "local JWKS URI without issuer", local: true, trust: ports.ClientAssertionTrustConfig{JWKSURI: "https://gateway.example.com/keys"}, fails: true},
		{name: "local inline keys without issuer", local: true, trust: ports.ClientAssertionTrustConfig{JWKS: trustAnchorTestKeySet}, fails: true},
		{name: "local file keys without issuer", local: true, trust: ports.ClientAssertionTrustConfig{JWKSFile: testTrustDir + "/gw.json"}, fails: true},
		{name: "local without an anchor", local: true},
		{name: "local keys with explicit issuer", local: true, trust: ports.ClientAssertionTrustConfig{IssuerURI: "https://gateway.example.com", JWKS: trustAnchorTestKeySet}},
		{name: "proxy keys use upstream issuer", trust: ports.ClientAssertionTrustConfig{JWKS: trustAnchorTestKeySet}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			if tc.local {
				cfg.OAuth2AuthServer.Mode = servermode.Local
				cfg.OAuth2AuthServer.Proxy = ports.ProxyModeConfig{}
			}
			cfg.Security.TrustMaterialDir = testTrustDir
			cfg.TokenExchange.ClientAssertion = tc.trust
			err := Validate(cfg)
			if tc.fails {
				requireConfigError(t, err, "token_exchange.client_assertion.issuer_uri", "",
					"a configured client-assertion key source requires an issuer when no proxy upstream issuer is available")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateImpersonationKeySources(t *testing.T) {
	cfg := validImpersonationConfig()
	cfg.Rules[0].TrustedIssuers = append(cfg.Rules[0].TrustedIssuers, ports.TrustedTokenIssuerConfig{
		IssuerURI:         "https://gateway.example.com",
		JWKS:              trustAnchorTestKeySet,
		JWKSFile:          testTrustDir + "/gw.json",
		AllowedAlgorithms: []string{"ES256"},
		SignsRoles:        []string{"client_assertion"},
	})

	err := validateImpersonationConfig(cfg, servermode.Local, &ports.SecurityConfig{TrustMaterialDir: testTrustDir})

	requireConfigError(t, err, "oauth2_authorization_server.impersonation.rules[0].trusted_issuers[1]", inlineRedacted,
		"only one of jwks_uri, jwks, jwks_file may be set (found: jwks, jwks_file)")

	cfg.Rules[0].TrustedIssuers[1].JWKSFile = ""
	require.NoError(t, validateImpersonationConfig(cfg, servermode.Local, &ports.SecurityConfig{TrustMaterialDir: testTrustDir}))
}

func TestValidateJWTKeySourcesOnBothServers(t *testing.T) {
	security := &ports.SecurityConfig{TrustMaterialDir: testTrustDir}
	for _, prefix := range []string{"server.enduser.authentication.jwt", "server.admin.authentication.jwt"} {
		t.Run(prefix, func(t *testing.T) {
			inline := signedJWT()
			inline.JWKS = trustAnchorTestKeySet
			require.NoError(t, validateJWTConfig(inline, prefix, security))

			file := signedJWT()
			file.JWKSFile = testTrustDir + "/mesh.json"
			require.NoError(t, validateJWTConfig(file, prefix, security))

			unsigned := signedJWT()
			unsigned.Verification = "none"
			unsigned.JWKS = trustAnchorTestKeySet
			requireConfigError(t, validateJWTConfig(unsigned, prefix, security), prefix, inlineRedacted,
				"verification 'none' and jwks are mutually exclusive")

			unsignedFile := signedJWT()
			unsignedFile.Verification = "none"
			unsignedFile.JWKSFile = testTrustDir + "/mesh.json"
			requireConfigError(t, validateJWTConfig(unsignedFile, prefix, security), prefix, testTrustDir+"/mesh.json",
				"verification 'none' and jwks_file are mutually exclusive")

			both := signedJWT()
			both.JWKSURI = "https://idp.example.com/keys"
			both.JWKSFile = testTrustDir + "/mesh.json"
			requireConfigError(t, validateJWTConfig(both, prefix, security), prefix, testTrustDir+"/mesh.json",
				"only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks_file)")

			none := signedJWT()
			requireConfigError(t, validateJWTConfig(none, prefix, security), prefix+".jwks_uri", "",
				"one of jwks_uri, jwks, or jwks_file (required when verification is 'jwks')")
		})
	}
}

func TestValidateTrustMaterialDir(t *testing.T) {
	for _, dir := range []string{"trust", "/", "/etc/.."} {
		t.Run(dir, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Security.TrustMaterialDir = dir
			requireConfigError(t, Validate(cfg), "security.trust_material_dir", dir,
				"absolute path to an existing directory other than /")
		})
	}

	t.Run("an absolute directory is accepted without touching the filesystem", func(t *testing.T) {
		cfg := validTestConfig()
		cfg.Security.TrustMaterialDir = "/does/not/exist/yet"
		require.NoError(t, Validate(cfg))
	})
}
