package ports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validBaseOAuth2Config returns a minimally valid OAuth2AuthServerConfig for test setup.
func validBaseOAuth2Config() OAuth2AuthServerConfig {
	return OAuth2AuthServerConfig{
		Mode: "proxy",
		Proxy: ProxyModeConfig{
			UpstreamIssuerURI:         "https://issuer.example.com",
			UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
			UpstreamTokenEndpoint:     "https://issuer.example.com/token",
		},
	}
}

func validLocalOAuth2Config() OAuth2AuthServerConfig {
	return OAuth2AuthServerConfig{
		Mode: "local",
		Local: LocalModeConfig{
			TokenTTL: time.Hour,
		},
	}
}

// TestOAuth2AuthServerConfig_Validate_OppositeModeRejection verifies that proxy mode rejects
// local.* settings and local mode rejects proxy.* settings.
func TestOAuth2AuthServerConfig_Validate_OppositeModeRejection(t *testing.T) {
	t.Run("proxy mode with local.token_ttl set returns error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Local.TokenTTL = time.Hour
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("proxy mode with local.token_claims_expression set returns error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Local.TokenClaimsExpression = `{"sub": claims.sub}`
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("proxy mode with local.signing_keys.bootstrap_timeout set returns error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Local.SigningKeys.BootstrapTimeout = 45 * time.Second
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("proxy mode with no local settings — no error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("local mode with proxy.upstream_issuer_uri set returns error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Proxy.UpstreamIssuerURI = "https://issuer.example.com"
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("local mode with proxy.upstream_authorize_endpoint set returns error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Proxy.UpstreamAuthorizeEndpoint = "https://issuer.example.com/authorize"
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("local mode with proxy.upstream_token_endpoint set returns error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Proxy.UpstreamTokenEndpoint = "https://issuer.example.com/token"
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("local mode with proxy.upstream_timeout set returns error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Proxy.UpstreamTimeout = 30 * time.Second
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("local mode with no proxy settings — no error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

// T032: Unit tests for OAuth2AuthServerConfig.Validate() — multi-agent client fields.
func TestOAuth2AuthServerConfig_Validate_MultiAgentClient(t *testing.T) {
	t.Run("Enabled=true missing AgentIDParamName returns startup error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.MultiAgentClient = MultiAgentClientConfig{
			Enabled:          true,
			AgentIDParamName: "",
			AgentIDClaimName: "x_agent_id",
		}
		err := cfg.Validate()
		require.Error(t, err)
		type fieldError interface{ Field() string }
		fe, ok := err.(fieldError)
		require.True(t, ok, "error must implement Field() method")
		assert.Contains(t, fe.Field(), "agent_id_param_name")
		assert.Contains(t, fe.Field(), "required")
	})

	t.Run("Enabled=true missing AgentIDClaimName returns startup error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.MultiAgentClient = MultiAgentClientConfig{
			Enabled:          true,
			AgentIDParamName: "x_agent_id",
			AgentIDClaimName: "",
		}
		err := cfg.Validate()
		require.Error(t, err)
		type fieldError interface{ Field() string }
		fe, ok := err.(fieldError)
		require.True(t, ok, "error must implement Field() method")
		assert.Contains(t, fe.Field(), "agent_id_claim_name")
		assert.Contains(t, fe.Field(), "required")
	})

	t.Run("Enabled=false with empty param and claim names — no error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.MultiAgentClient = MultiAgentClientConfig{
			Enabled:          false,
			AgentIDParamName: "",
			AgentIDClaimName: "",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("Enabled=true with both fields set — no error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.MultiAgentClient = MultiAgentClientConfig{
			Enabled:          true,
			AgentIDParamName: "x_agent_id",
			AgentIDClaimName: "x_agent_id",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

// T033: Proxy mode config validation.
func TestOAuth2AuthServerConfig_Validate_ProxyMode(t *testing.T) {
	t.Run("proxy mode with required proxy section — no error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("proxy mode missing upstream_issuer_uri — error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Proxy.UpstreamIssuerURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream_issuer_uri")
	})

	t.Run("proxy mode missing upstream_authorize_endpoint — error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Proxy.UpstreamAuthorizeEndpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("proxy mode missing upstream_token_endpoint — error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Proxy.UpstreamTokenEndpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("proxy mode with negative upstream_timeout — error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Proxy.UpstreamTimeout = -5 * time.Second
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream_timeout")
		assert.Contains(t, err.Error(), "positive duration")
	})

	t.Run("proxy mode with local section set — error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.Local.TokenTTL = time.Hour
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "local")
	})
}

// T034: Local mode config validation.
func TestOAuth2AuthServerConfig_Validate_LocalMode(t *testing.T) {
	t.Run("local mode with local section — no error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("local mode sets default token_ttl when empty", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{Mode: "local"}
		err := cfg.Validate()
		require.NoError(t, err)
		assert.Equal(t, time.Hour, cfg.Local.TokenTTL)
		assert.Equal(t, DefaultSigningKeyBootstrapTimeout, cfg.Local.SigningKeys.BootstrapTimeout)
	})

	t.Run("local mode preserves explicit signing key bootstrap timeout", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Local.SigningKeys.BootstrapTimeout = 45 * time.Second
		err := cfg.Validate()
		require.NoError(t, err)
		assert.Equal(t, 45*time.Second, cfg.Local.SigningKeys.BootstrapTimeout)
	})

	t.Run("local mode rejects negative signing key bootstrap timeout", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Local.SigningKeys.BootstrapTimeout = -1 * time.Second
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signing_keys.bootstrap_timeout")
	})

	t.Run("local mode rejects negative refresh token TTL", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Local.RefreshTokenTTL = -time.Hour

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "local.refresh_token_ttl")
	})

	t.Run("local mode with proxy section set — error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Proxy.UpstreamIssuerURI = "https://issuer.example.com"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxy")
	})

	t.Run("local mode preserves explicit issuer_uri", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.Local.IssuerURI = "https://auth.cdn.example.com"
		err := cfg.Validate()
		require.NoError(t, err)
		assert.Equal(t, "https://auth.cdn.example.com", cfg.Local.IssuerURI)
	})
}

// T035: Hybrid mode config validation.
func TestOAuth2AuthServerConfig_Validate_HybridMode(t *testing.T) {
	validHybridConfig := func() OAuth2AuthServerConfig {
		return OAuth2AuthServerConfig{
			Mode: "hybrid",
			Proxy: ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
			Local: LocalModeConfig{
				TokenTTL: time.Hour,
			},
		}
	}

	t.Run("hybrid mode with both sections — no error", func(t *testing.T) {
		cfg := validHybridConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("hybrid mode missing proxy.upstream_issuer_uri — error", func(t *testing.T) {
		cfg := validHybridConfig()
		cfg.Proxy.UpstreamIssuerURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream_issuer_uri")
	})

	t.Run("hybrid mode missing proxy.upstream_authorize_endpoint — error", func(t *testing.T) {
		cfg := validHybridConfig()
		cfg.Proxy.UpstreamAuthorizeEndpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("hybrid mode missing proxy.upstream_token_endpoint — error", func(t *testing.T) {
		cfg := validHybridConfig()
		cfg.Proxy.UpstreamTokenEndpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
	})

	t.Run("hybrid mode sets default token_ttl when empty", func(t *testing.T) {
		cfg := validHybridConfig()
		cfg.Local.TokenTTL = 0
		err := cfg.Validate()
		require.NoError(t, err)
		assert.Equal(t, time.Hour, cfg.Local.TokenTTL)
	})

	t.Run("hybrid mode rejects negative refresh token TTL", func(t *testing.T) {
		cfg := validHybridConfig()
		cfg.Local.RefreshTokenTTL = -time.Hour

		err := cfg.Validate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "local.refresh_token_ttl")
	})
}

// T036: issue_token deprecation error.
func TestOAuth2AuthServerConfig_Validate_IssueTokenDeprecation(t *testing.T) {
	t.Run("issue_token mode returns deprecation error", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{Mode: "issue_token"}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "issue_token")
		assert.Contains(t, err.Error(), "local")
	})
}

// T036a: empty mode defaults to proxy when other proxy fields are provided.
func TestOAuth2AuthServerConfig_Validate_EmptyModeDefaultsToProxy(t *testing.T) {
	t.Run("empty mode with proxy fields returns error — mode is required", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode: "",
			Proxy: ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mode is required")
	})
}

// T052-T053: CIMD gating by mode.
func TestOAuth2AuthServerConfig_Validate_CIMDGating(t *testing.T) {
	t.Run("CIMD enabled in proxy mode — error", func(t *testing.T) {
		cfg := validBaseOAuth2Config()
		cfg.CIMD = CIMDConfig{Enabled: true}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cimd")
	})

	t.Run("CIMD enabled in local mode — no error", func(t *testing.T) {
		cfg := validLocalOAuth2Config()
		cfg.CIMD = DefaultCIMDConfig()
		cfg.CIMD.Enabled = true
		cfg.CIMD.Cache = CIMDCacheConfig{MinTTL: time.Minute, MaxTTL: time.Hour}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("CIMD enabled in hybrid mode — no error", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode: "hybrid",
			Proxy: ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
			Local: LocalModeConfig{TokenTTL: time.Hour},
			CIMD:  DefaultCIMDConfig(),
		}
		cfg.CIMD.Enabled = true
		cfg.CIMD.Cache = CIMDCacheConfig{MinTTL: time.Minute, MaxTTL: time.Hour}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("hybrid mode preserves explicit signing key bootstrap timeout", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode: "hybrid",
			Proxy: ProxyModeConfig{
				UpstreamIssuerURI:         "https://issuer.example.com",
				UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
				UpstreamTokenEndpoint:     "https://issuer.example.com/token",
			},
			Local: LocalModeConfig{
				TokenTTL: time.Hour,
				SigningKeys: LocalSigningKeysConfig{
					BootstrapTimeout: 45 * time.Second,
				},
			},
		}
		err := cfg.Validate()
		require.NoError(t, err)
		assert.Equal(t, 45*time.Second, cfg.Local.SigningKeys.BootstrapTimeout)
	})
}

func TestCIMDClientAuthenticationConfig(t *testing.T) {
	t.Run("defaults are bounded and non-zero", func(t *testing.T) {
		cfg := DefaultCIMDConfig().ClientAuthentication
		assert.Equal(t, time.Hour, cfg.KeySetCacheTTL)
		assert.Equal(t, 5*time.Minute, cfg.KeySetMinRefetchInterval)
		assert.Equal(t, 65536, cfg.KeySetMaxBytes)
		assert.Equal(t, 5*time.Second, cfg.KeySetFetchTimeout)
	})

	tests := []struct {
		name   string
		mutate func(*CIMDClientAuthenticationConfig)
	}{
		{"zero cache TTL", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetCacheTTL = 0 }},
		{"negative cache TTL", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetCacheTTL = -time.Second }},
		{"cache TTL above 24 hours", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetCacheTTL = 25 * time.Hour }},
		{"zero refetch interval", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetMinRefetchInterval = 0 }},
		{"negative refetch interval", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetMinRefetchInterval = -time.Second }},
		{"zero maximum bytes", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetMaxBytes = 0 }},
		{"negative maximum bytes", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetMaxBytes = -1 }},
		{"zero fetch timeout", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetFetchTimeout = 0 }},
		{"negative fetch timeout", func(cfg *CIMDClientAuthenticationConfig) { cfg.KeySetFetchTimeout = -time.Second }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validLocalOAuth2Config()
			cfg.CIMD = DefaultCIMDConfig()
			cfg.CIMD.Enabled = true
			tt.mutate(&cfg.CIMD.ClientAuthentication)
			require.Error(t, cfg.Validate())
		})
	}
}

// TestOAuth2AuthServerConfig_Resolve verifies that Resolve() propagates all config
// fields into the correct mode-specific struct for all three modes. Non-default values
// are used throughout to catch zero-value masking bugs.
func TestOAuth2AuthServerConfig_Resolve(t *testing.T) {
	proxyFields := ProxyModeConfig{
		UpstreamIssuerURI:         "https://issuer.example.com",
		UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
		UpstreamTokenEndpoint:     "https://issuer.example.com/token",
		UpstreamTimeout:           45 * time.Second,
	}
	localFields := LocalModeConfig{
		IssuerURI:             "https://auth.cdn.example.com",
		TokenTTL:              2 * time.Hour,
		RefreshTokenTTL:       30 * 24 * time.Hour,
		TokenClaimsExpression: `{"sub": claims.sub}`,
		SigningKeys: LocalSigningKeysConfig{
			BootstrapTimeout: 45 * time.Second,
		},
	}
	cimd := DefaultCIMDConfig()
	cimd.Enabled = true
	cimd.Cache = CIMDCacheConfig{MinTTL: 30 * time.Second, MaxTTL: 2 * time.Hour}
	mac := MultiAgentClientConfig{
		Enabled:          true,
		AgentIDParamName: "x_agent_id",
		AgentIDClaimName: "x_agent_id",
	}
	sharedResponseTypes := []string{"code", "token"}
	sharedGrantTypes := []string{"authorization_code", "client_credentials", "refresh_token"}
	sharedScopes := []string{"offline_access"}

	t.Run("proxy mode propagates all upstream fields", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode:                   "proxy",
			Proxy:                  proxyFields,
			SupportedResponseTypes: sharedResponseTypes,
			SupportedGrantTypes:    sharedGrantTypes,
			SupportedScopes:        sharedScopes,
			MultiAgentClient:       mac,
		}
		result, err := cfg.Resolve()
		require.NoError(t, err)
		require.IsType(t, &ProxyOAuth2Config{}, result)
		p := result.(*ProxyOAuth2Config)
		assert.Equal(t, proxyFields.UpstreamIssuerURI, p.UpstreamIssuerURI)
		assert.Equal(t, proxyFields.UpstreamAuthorizeEndpoint, p.UpstreamAuthorizeEndpoint)
		assert.Equal(t, proxyFields.UpstreamTokenEndpoint, p.UpstreamTokenEndpoint)
		assert.Equal(t, proxyFields.UpstreamTimeout, p.UpstreamTimeout)
		assert.Equal(t, sharedResponseTypes, p.SupportedResponseTypes)
		assert.Equal(t, sharedGrantTypes, p.SupportedGrantTypes)
		assert.Equal(t, mac, p.MultiAgentClient)
		assert.Equal(t, sharedScopes, p.SupportedScopes)
	})

	t.Run("proxy mode applies default timeout and grant types when zero", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ProxyModeConfig{
				UpstreamIssuerURI:         proxyFields.UpstreamIssuerURI,
				UpstreamAuthorizeEndpoint: proxyFields.UpstreamAuthorizeEndpoint,
				UpstreamTokenEndpoint:     proxyFields.UpstreamTokenEndpoint,
				UpstreamTimeout:           0,
			},
		}
		result, err := cfg.Resolve()
		require.NoError(t, err)
		p := result.(*ProxyOAuth2Config)
		assert.Equal(t, 30*time.Second, p.UpstreamTimeout, "default timeout must be 30s")
		assert.Equal(t, []string{"code"}, p.SupportedResponseTypes, "default response type must be 'code'")
		assert.Equal(t, []string{"authorization_code"}, p.SupportedGrantTypes, "default proxy grant type")
		assert.Empty(t, p.SupportedScopes)
	})

	t.Run("local mode propagates all local fields", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode:                   "local",
			Local:                  localFields,
			SupportedResponseTypes: sharedResponseTypes,
			SupportedGrantTypes:    sharedGrantTypes,
			SupportedScopes:        sharedScopes,
			CIMD:                   cimd,
		}
		result, err := cfg.Resolve()
		require.NoError(t, err)
		require.IsType(t, &LocalOAuth2Config{}, result)
		l := result.(*LocalOAuth2Config)
		assert.Equal(t, localFields.IssuerURI, l.IssuerURI)
		assert.Equal(t, localFields.TokenTTL, l.TokenTTL)
		assert.Equal(t, localFields.RefreshTokenTTL, l.RefreshTokenTTL)
		assert.Equal(t, localFields.TokenClaimsExpression, l.TokenClaimsExpression)
		assert.Equal(t, localFields.SigningKeys, l.SigningKeys)
		assert.Equal(t, sharedResponseTypes, l.SupportedResponseTypes)
		assert.Equal(t, sharedGrantTypes, l.SupportedGrantTypes)
		assert.Equal(t, cimd, l.CIMD)
		assert.Equal(t, sharedScopes, l.SupportedScopes)
	})

	t.Run("local mode applies default token TTL and grant types when zero", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{Mode: "local"}
		result, err := cfg.Resolve()
		require.NoError(t, err)
		l := result.(*LocalOAuth2Config)
		assert.Equal(t, time.Hour, l.TokenTTL, "default local token TTL must be 1h")
		assert.Equal(t, 30*24*time.Hour, l.RefreshTokenTTL, "default local refresh token TTL must be 30d")
		assert.Equal(t, DefaultSigningKeyBootstrapTimeout, l.SigningKeys.BootstrapTimeout)
		assert.Equal(t, []string{"code"}, l.SupportedResponseTypes)
		assert.Equal(t, []string{"authorization_code", "client_credentials", "refresh_token"}, l.SupportedGrantTypes)
		assert.Equal(t, []string{"offline_access"}, l.SupportedScopes)
	})

	t.Run("hybrid mode propagates proxy and local fields independently", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode:                   "hybrid",
			Proxy:                  proxyFields,
			Local:                  localFields,
			SupportedResponseTypes: sharedResponseTypes,
			SupportedGrantTypes:    sharedGrantTypes,
			SupportedScopes:        sharedScopes,
			MultiAgentClient:       mac,
			CIMD:                   cimd,
		}
		result, err := cfg.Resolve()
		require.NoError(t, err)
		require.IsType(t, &HybridOAuth2Config{}, result)
		h := result.(*HybridOAuth2Config)

		assert.Equal(t, proxyFields.UpstreamIssuerURI, h.Proxy.UpstreamIssuerURI)
		assert.Equal(t, proxyFields.UpstreamAuthorizeEndpoint, h.Proxy.UpstreamAuthorizeEndpoint)
		assert.Equal(t, proxyFields.UpstreamTokenEndpoint, h.Proxy.UpstreamTokenEndpoint)
		assert.Equal(t, proxyFields.UpstreamTimeout, h.Proxy.UpstreamTimeout)
		assert.Equal(t, sharedResponseTypes, h.Proxy.SupportedResponseTypes)
		assert.Equal(t, sharedGrantTypes, h.Proxy.SupportedGrantTypes)
		assert.Equal(t, mac, h.Proxy.MultiAgentClient)
		assert.Equal(t, sharedScopes, h.Proxy.SupportedScopes)

		assert.Equal(t, localFields.IssuerURI, h.Local.IssuerURI)
		assert.Equal(t, localFields.TokenTTL, h.Local.TokenTTL)
		assert.Equal(t, localFields.TokenClaimsExpression, h.Local.TokenClaimsExpression)
		assert.Equal(t, localFields.RefreshTokenTTL, h.Local.RefreshTokenTTL)
		assert.Equal(t, localFields.SigningKeys, h.Local.SigningKeys)
		assert.Equal(t, sharedResponseTypes, h.Local.SupportedResponseTypes)
		assert.Equal(t, sharedGrantTypes, h.Local.SupportedGrantTypes)
		assert.Equal(t, cimd, h.Local.CIMD)
		assert.Equal(t, sharedScopes, h.Local.SupportedScopes)
	})

	t.Run("hybrid mode applies default token TTL and grant types when zero", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{
			Mode: "hybrid",
			Proxy: ProxyModeConfig{
				UpstreamIssuerURI:         proxyFields.UpstreamIssuerURI,
				UpstreamAuthorizeEndpoint: proxyFields.UpstreamAuthorizeEndpoint,
				UpstreamTokenEndpoint:     proxyFields.UpstreamTokenEndpoint,
			},
		}
		result, err := cfg.Resolve()
		require.NoError(t, err)
		h := result.(*HybridOAuth2Config)
		assert.Equal(t, time.Hour, h.Local.TokenTTL, "default hybrid local token TTL must be 1h")
		assert.Equal(t, 30*24*time.Hour, h.Local.RefreshTokenTTL, "default hybrid local refresh token TTL must be 30d")
		assert.Equal(t, DefaultSigningKeyBootstrapTimeout, h.Local.SigningKeys.BootstrapTimeout)
		assert.Equal(t, []string{"code"}, h.Proxy.SupportedResponseTypes, "default proxy response type")
		assert.Equal(t, []string{"authorization_code", "client_credentials", "refresh_token"}, h.Proxy.SupportedGrantTypes, "default hybrid grant types")
		assert.Equal(t, []string{"code"}, h.Local.SupportedResponseTypes, "default local response type")
		assert.Equal(t, []string{"authorization_code", "client_credentials", "refresh_token"}, h.Local.SupportedGrantTypes, "default hybrid grant types")
		assert.Equal(t, []string{"offline_access"}, h.Proxy.SupportedScopes)
		assert.Equal(t, []string{"offline_access"}, h.Local.SupportedScopes)
		assert.Equal(t, 30*time.Second, h.Proxy.UpstreamTimeout, "default hybrid proxy timeout must be 30s")
	})

	t.Run("invalid mode returns error without panicking", func(t *testing.T) {
		cfg := OAuth2AuthServerConfig{Mode: "invalid"}
		_, err := cfg.Resolve()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mode")
	})
}

// TestOAuth2AuthServerConfig_ImpersonationIsOptional verifies the optional-pointer contract
// (research R7): Impersonation defaults to nil (preserving existing behavior), and ports-level
// mode validation is unaffected by its presence — the detailed CR-001..CR-008 validation is
// layered in internal/config, not here.
func TestOAuth2AuthServerConfig_ImpersonationIsOptional(t *testing.T) {
	base := validLocalOAuth2Config()
	require.Nil(t, base.Impersonation, "impersonation defaults to nil")
	require.NoError(t, base.Validate())

	withImp := validLocalOAuth2Config()
	withImp.Impersonation = &ImpersonationConfig{
		AudiencePrefix: "https://broker.example.com/impersonation",
		Rules: []ImpersonationRuleConfig{{
			Name: "internal-gateway",
			Roles: map[string]ImpersonationRoleConfig{
				"client_assertion": {ExpectedAudience: "https://broker.example.com/impersonation", PrincipalExpression: "client_assertion.sub"},
				"actor":            {ExpectedAudience: "https://broker.example.com/impersonation", PrincipalExpression: "actor_token.sub"},
				"subject":          {ExpectedAudience: "https://broker.example.com/impersonation", PrincipalExpression: "subject_token.sub"},
			},
			TrustedIssuers: []TrustedTokenIssuerConfig{{
				IssuerURI:         "https://idp.example.com",
				AllowedAlgorithms: []string{"RS256"},
				SignsRoles:        []string{"client_assertion", "actor", "subject"},
			}},
			Authorization: AuthorizationConfig{Type: "cel", CEL: CELAuthorizationConfig{Expression: "true"}},
		}},
	}
	require.NoError(t, withImp.Validate(), "ports-level mode validation is unaffected by the impersonation block")
	require.NotNil(t, withImp.Impersonation)
	assert.Equal(t, "https://broker.example.com/impersonation", withImp.Impersonation.AudiencePrefix)
	require.Len(t, withImp.Impersonation.Rules, 1)
	assert.Len(t, withImp.Impersonation.Rules[0].Roles, 3)
	assert.Equal(t, CredentialRoleSubject, CredentialRole("subject"))
}
