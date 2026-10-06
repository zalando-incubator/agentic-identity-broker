package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func loadRefreshPolicyYAML(t *testing.T, yaml string, mode ...string) (*ports.Config, error) {
	t.Helper()
	setMinimalConfigEnv(t)
	if len(mode) != 0 {
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", mode[0])
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	return NewLoader().GetConfig(context.Background())
}

func resolvedLocalRefreshPolicy(t *testing.T, cfg *ports.OAuth2AuthServerConfig) ports.LocalOAuth2Config {
	t.Helper()
	resolved, err := cfg.Resolve()
	require.NoError(t, err)
	switch policy := resolved.(type) {
	case *ports.LocalOAuth2Config:
		return *policy
	case *ports.HybridOAuth2Config:
		return policy.Local
	default:
		t.Fatalf("expected locally issued policy, got %T", resolved)
		return ports.LocalOAuth2Config{}
	}
}

func TestRefreshPolicyDirectConfigurationResolvesLocalAndHybrid(t *testing.T) {
	strict := time.Duration(0)
	bounded := 20 * time.Second
	boundary := 30*time.Second - time.Nanosecond
	cases := []struct {
		name      string
		local     ports.LocalModeConfig
		wantReuse time.Duration
		wantAbs   time.Duration
		wantIdle  time.Duration
	}{
		{
			name:      "omitted reuse and zero inactivity use bounded retry and thirty-day inactivity",
			local:     ports.LocalModeConfig{TokenTTL: time.Hour},
			wantReuse: 30 * time.Second,
			wantIdle:  720 * time.Hour,
		},
		{
			name:     "omitted reuse is strict when access lifetime is below thirty seconds",
			local:    ports.LocalModeConfig{TokenTTL: 20 * time.Second},
			wantIdle: 720 * time.Hour,
		},
		{
			name:     "omitted reuse is strict when access lifetime equals thirty seconds",
			local:    ports.LocalModeConfig{TokenTTL: 30 * time.Second},
			wantIdle: 720 * time.Hour,
		},
		{
			name:      "omitted reuse keeps thirty seconds above the access lifetime boundary",
			local:     ports.LocalModeConfig{TokenTTL: 30*time.Second + time.Nanosecond},
			wantReuse: 30 * time.Second,
			wantIdle:  720 * time.Hour,
		},
		{
			name:     "explicit zero disables reuse even when a finite absolute limit is below thirty seconds",
			local:    ports.LocalModeConfig{TokenTTL: time.Hour, RefreshTokenReuseInterval: &strict, AbsoluteSessionLifetime: time.Second},
			wantAbs:  time.Second,
			wantIdle: 720 * time.Hour,
		},
		{
			name:      "explicit zero inactivity preserves its existing default without replacing configured reuse",
			local:     ports.LocalModeConfig{TokenTTL: time.Hour, RefreshTokenReuseInterval: &bounded, RefreshTokenTTL: 0, AbsoluteSessionLifetime: 45 * time.Second},
			wantReuse: bounded,
			wantAbs:   45 * time.Second,
			wantIdle:  720 * time.Hour,
		},
		{
			name:      "shorter absolute lifetime is accepted instead of rejected",
			local:     ports.LocalModeConfig{TokenTTL: time.Hour, RefreshTokenReuseInterval: &bounded, RefreshTokenTTL: 8 * time.Hour, AbsoluteSessionLifetime: 2 * time.Hour},
			wantReuse: bounded,
			wantAbs:   2 * time.Hour,
			wantIdle:  8 * time.Hour,
		},
		{
			name:      "one nanosecond inside each lifetime bound is valid",
			local:     ports.LocalModeConfig{TokenTTL: 30 * time.Second, RefreshTokenTTL: 30 * time.Second, RefreshTokenReuseInterval: &boundary, AbsoluteSessionLifetime: 30 * time.Second},
			wantReuse: boundary,
			wantAbs:   30 * time.Second,
			wantIdle:  30 * time.Second,
		},
	}

	for _, mode := range []string{"local", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					cfg := ports.OAuth2AuthServerConfig{Mode: servermode.Mode(mode), Local: tt.local}
					if mode == "hybrid" {
						cfg.Proxy = refreshPolicyProxyConfig()
					}
					local := resolvedLocalRefreshPolicy(t, &cfg)
					require.Equal(t, tt.wantReuse, local.RefreshTokenReuseInterval)
					require.Equal(t, tt.wantAbs, local.AbsoluteSessionLifetime)
					require.Equal(t, tt.wantIdle, local.RefreshTokenTTL)
				})
			}
		})
	}
}

func refreshPolicyProxyConfig() ports.ProxyModeConfig {
	return ports.ProxyModeConfig{
		UpstreamIssuerURI:         "https://idp.example.com",
		UpstreamAuthorizeEndpoint: "https://idp.example.com/authorize",
		UpstreamTokenEndpoint:     "https://idp.example.com/token",
	}
}

func TestRefreshPolicyDirectConfigurationRejectsInvalidLifetimes(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ports.LocalModeConfig)
		field  string
	}{
		{"negative reuse", func(c *ports.LocalModeConfig) { d := -time.Second; c.RefreshTokenReuseInterval = &d }, "refresh_token_reuse_interval"},
		{"negative absolute lifetime", func(c *ports.LocalModeConfig) { c.AbsoluteSessionLifetime = -time.Second }, "absolute_session_lifetime"},
		{"negative inactivity lifetime", func(c *ports.LocalModeConfig) { c.RefreshTokenTTL = -time.Second }, "refresh_token_ttl"},
		{"reuse equals access lifetime", func(c *ports.LocalModeConfig) { d := time.Minute; c.RefreshTokenReuseInterval = &d }, "refresh_token_reuse_interval"},
		{"reuse exceeds access lifetime", func(c *ports.LocalModeConfig) { d := 61 * time.Second; c.RefreshTokenReuseInterval = &d }, "refresh_token_reuse_interval"},
		{"reuse equals inactivity lifetime", func(c *ports.LocalModeConfig) {
			d := 2 * time.Hour
			c.TokenTTL = 4 * time.Hour
			c.RefreshTokenReuseInterval = &d
		}, "refresh_token_reuse_interval"},
		{"reuse exceeds inactivity lifetime", func(c *ports.LocalModeConfig) {
			d := 3 * time.Hour
			c.TokenTTL = 4 * time.Hour
			c.RefreshTokenReuseInterval = &d
		}, "refresh_token_reuse_interval"},
		{"finite absolute equals reuse", func(c *ports.LocalModeConfig) { c.AbsoluteSessionLifetime = 30 * time.Second }, "absolute_session_lifetime"},
		{"finite absolute below reuse", func(c *ports.LocalModeConfig) { c.AbsoluteSessionLifetime = 29 * time.Second }, "absolute_session_lifetime"},
		{"explicit reuse equals short access lifetime", func(c *ports.LocalModeConfig) {
			d := 20 * time.Second
			c.TokenTTL = d
			c.RefreshTokenReuseInterval = &d
		}, "refresh_token_reuse_interval"},
		{"zero inactivity still defaults before relation validation", func(c *ports.LocalModeConfig) {
			d := 720 * time.Hour
			c.TokenTTL = 800 * time.Hour
			c.RefreshTokenTTL = 0
			c.RefreshTokenReuseInterval = &d
		}, "refresh_token_reuse_interval"},
	}
	for _, mode := range []string{"local", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					cfg := ports.OAuth2AuthServerConfig{Mode: servermode.Mode(mode), Local: ports.LocalModeConfig{TokenTTL: time.Minute, RefreshTokenTTL: 2 * time.Hour}}
					if mode == "hybrid" {
						cfg.Proxy = refreshPolicyProxyConfig()
					}
					tt.change(&cfg.Local)
					_, err := cfg.Resolve()
					require.ErrorContains(t, err, tt.field)
				})
			}
		})
	}
}

func TestRefreshPolicyProxyModeIsNotLocalPolicy(t *testing.T) {
	cfg := ports.OAuth2AuthServerConfig{Mode: "proxy", Proxy: refreshPolicyProxyConfig()}
	resolved, err := cfg.Resolve()
	require.NoError(t, err)
	require.IsType(t, &ports.ProxyOAuth2Config{}, resolved)
	require.Nil(t, cfg.Local.RefreshTokenReuseInterval)
	require.Zero(t, cfg.Local.AbsoluteSessionLifetime)
	require.Zero(t, cfg.Local.RefreshTokenTTL)

	for _, tt := range []struct {
		name   string
		change func(*ports.LocalModeConfig)
	}{
		{"strict reuse setting", func(c *ports.LocalModeConfig) { d := time.Duration(0); c.RefreshTokenReuseInterval = &d }},
		{"finite absolute lifetime", func(c *ports.LocalModeConfig) { c.AbsoluteSessionLifetime = time.Hour }},
		{"inactivity lifetime", func(c *ports.LocalModeConfig) { c.RefreshTokenTTL = time.Hour }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxy := ports.OAuth2AuthServerConfig{Mode: "proxy", Proxy: refreshPolicyProxyConfig()}
			tt.change(&proxy.Local)
			_, err := proxy.Resolve()
			require.ErrorContains(t, err, "oauth2_authorization_server.local")
		})
	}
}

func TestRefreshPolicyLoaderPreservesSourcesAndExplicitZero(t *testing.T) {
	for _, mode := range []string{"local", "hybrid"} {
		t.Run("YAML "+mode, func(t *testing.T) {
			yaml := fmt.Sprintf("oauth2_authorization_server:\n  mode: %s\n", mode)
			if mode == "hybrid" {
				yaml += `  proxy:
    upstream_issuer_uri: https://idp.example.com
    upstream_authorize_endpoint: https://idp.example.com/authorize
    upstream_token_endpoint: https://idp.example.com/token
`
			}
			yaml += `  local:
    token_ttl: 1h
    refresh_token_reuse_interval: 0s
    absolute_session_lifetime: 1s
    refresh_token_ttl: 96h
`
			cfg, err := loadRefreshPolicyYAML(t, yaml, mode)
			require.NoError(t, err)
			local := resolvedLocalRefreshPolicy(t, &cfg.OAuth2AuthServer)
			require.Zero(t, local.RefreshTokenReuseInterval)
			require.Equal(t, time.Second, local.AbsoluteSessionLifetime)
			require.Equal(t, 96*time.Hour, local.RefreshTokenTTL)
		})
	}

	t.Run("YAML finite policy reaches the resolved local issuer", func(t *testing.T) {
		cfg, err := loadRefreshPolicyYAML(t, `oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 1h
    refresh_token_reuse_interval: 13s
    absolute_session_lifetime: 4h
    refresh_token_ttl: 48h
`)
		require.NoError(t, err)
		local := resolvedLocalRefreshPolicy(t, &cfg.OAuth2AuthServer)
		require.Equal(t, 13*time.Second, local.RefreshTokenReuseInterval)
		require.Equal(t, 4*time.Hour, local.AbsoluteSessionLifetime)
		require.Equal(t, 48*time.Hour, local.RefreshTokenTTL)
	})

	t.Run("YAML short-lived access tokens retain strict single use when reuse is omitted", func(t *testing.T) {
		cfg, err := loadRefreshPolicyYAML(t, `oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 20s
`)
		require.NoError(t, err)
		local := resolvedLocalRefreshPolicy(t, &cfg.OAuth2AuthServer)
		require.Zero(t, local.RefreshTokenReuseInterval)
	})

	t.Run("environment overrides YAML with strict zero and unlimited lifetime", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "0s")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "0s")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "48h")
		cfg, err := loadRefreshPolicyYAML(t, `oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 1h
    refresh_token_reuse_interval: 45s
    absolute_session_lifetime: 4h
    refresh_token_ttl: 96h
`)
		require.NoError(t, err)
		local := resolvedLocalRefreshPolicy(t, &cfg.OAuth2AuthServer)
		require.Zero(t, local.RefreshTokenReuseInterval)
		require.Zero(t, local.AbsoluteSessionLifetime)
		require.Equal(t, 48*time.Hour, local.RefreshTokenTTL)
	})

	t.Run("environment alone parses durations and zero inactivity retains its default", func(t *testing.T) {
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "7s")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "3h")
		t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "0s")
		setMinimalConfigEnv(t)
		cfg, err := NewLoader().GetConfig(context.Background())
		require.NoError(t, err)
		local := resolvedLocalRefreshPolicy(t, &cfg.OAuth2AuthServer)
		require.Equal(t, 7*time.Second, local.RefreshTokenReuseInterval)
		require.Equal(t, 3*time.Hour, local.AbsoluteSessionLifetime)
		require.Equal(t, 720*time.Hour, local.RefreshTokenTTL)
	})
}

func TestRefreshPolicyLoaderRejectsBadDurationSources(t *testing.T) {
	for _, tt := range []struct {
		name, key, value string
	}{
		{"reuse numeric zero", "refresh_token_reuse_interval", "0"},
		{"reuse quoted numeric", "refresh_token_reuse_interval", `"30"`},
		{"reuse malformed", "refresh_token_reuse_interval", "thirty-seconds"},
		{"reuse negative", "refresh_token_reuse_interval", "-1s"},
		{"reuse overflow", "refresh_token_reuse_interval", "2562048h"},
		{"absolute numeric zero", "absolute_session_lifetime", "0"},
		{"absolute malformed", "absolute_session_lifetime", "one-day"},
		{"absolute negative", "absolute_session_lifetime", "-1s"},
		{"absolute overflow", "absolute_session_lifetime", "2562048h"},
		{"inactivity numeric zero", "refresh_token_ttl", "0"},
		{"inactivity malformed", "refresh_token_ttl", "one-month"},
		{"inactivity negative", "refresh_token_ttl", "-1s"},
		{"inactivity overflow", "refresh_token_ttl", "2562048h"},
	} {
		t.Run("YAML "+tt.name, func(t *testing.T) {
			cfg, err := loadRefreshPolicyYAML(t, fmt.Sprintf("oauth2_authorization_server:\n  mode: local\n  local:\n    token_ttl: 1h\n    %s: %s\n", tt.key, tt.value))
			require.Nil(t, cfg)
			require.ErrorContains(t, err, tt.key)
		})
	}

	for _, tt := range []struct {
		name, env, key, value string
	}{
		{"reuse bare numeric", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "refresh_token_reuse_interval", "0"},
		{"reuse malformed", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "refresh_token_reuse_interval", "not-a-duration"},
		{"absolute negative", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "absolute_session_lifetime", "-1s"},
		{"absolute overflow", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "absolute_session_lifetime", "2562048h"},
		{"inactivity numeric zero", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "refresh_token_ttl", "0"},
		{"inactivity malformed", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "refresh_token_ttl", "bad"},
	} {
		t.Run("environment "+tt.name, func(t *testing.T) {
			setMinimalConfigEnv(t)
			t.Setenv(tt.env, tt.value)
			cfg, err := NewLoader().GetConfig(context.Background())
			require.Nil(t, cfg)
			require.ErrorContains(t, err, tt.key)
		})
	}
}

func TestRefreshPolicyLoaderRejectsInvalidRelations(t *testing.T) {
	for _, tt := range []struct {
		name, settings, field string
	}{
		{"reuse equals access", "token_ttl: 30s\n    refresh_token_reuse_interval: 30s", "refresh_token_reuse_interval"},
		{"reuse equals inactivity", "token_ttl: 1h\n    refresh_token_reuse_interval: 30s\n    refresh_token_ttl: 30s", "refresh_token_reuse_interval"},
		{"absolute equals reuse", "token_ttl: 1h\n    refresh_token_reuse_interval: 30s\n    absolute_session_lifetime: 30s", "absolute_session_lifetime"},
	} {
		t.Run("YAML "+tt.name, func(t *testing.T) {
			_, err := loadRefreshPolicyYAML(t, "oauth2_authorization_server:\n  mode: local\n  local:\n    "+tt.settings+"\n")
			require.ErrorContains(t, err, tt.field)
		})
	}

	for _, tt := range []struct {
		name, env, value, field string
	}{
		{"reuse equals access", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "1h", "refresh_token_reuse_interval"},
		{"inactivity equals reuse", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "30s", "refresh_token_reuse_interval"},
		{"absolute equals reuse", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "30s", "absolute_session_lifetime"},
	} {
		t.Run("environment "+tt.name, func(t *testing.T) {
			t.Setenv(tt.env, tt.value)
			_, err := loadRefreshPolicyYAML(t, `oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 1h
    refresh_token_reuse_interval: 30s
    absolute_session_lifetime: 2h
    refresh_token_ttl: 3h
`)
			require.ErrorContains(t, err, tt.field)
		})
	}
}

func TestRefreshPolicyLoaderProxyRejectsExplicitLocalPolicy(t *testing.T) {
	proxyYAML := `oauth2_authorization_server:
  mode: proxy
  proxy:
    upstream_issuer_uri: https://idp.example.com
    upstream_authorize_endpoint: https://idp.example.com/authorize
    upstream_token_endpoint: https://idp.example.com/token
`
	t.Run("proxy without local policy stays proxy", func(t *testing.T) {
		cfg, err := loadRefreshPolicyYAML(t, proxyYAML, "proxy")
		require.NoError(t, err)
		resolved, err := cfg.OAuth2AuthServer.Resolve()
		require.NoError(t, err)
		require.IsType(t, &ports.ProxyOAuth2Config{}, resolved)
		require.Nil(t, cfg.OAuth2AuthServer.Local.RefreshTokenReuseInterval)
		require.Zero(t, cfg.OAuth2AuthServer.Local.AbsoluteSessionLifetime)
		require.Zero(t, cfg.OAuth2AuthServer.Local.RefreshTokenTTL)
	})

	for _, tt := range []struct{ name, key, env string }{
		{"explicit strict reuse", "refresh_token_reuse_interval", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL"},
		{"explicit unlimited absolute lifetime", "absolute_session_lifetime", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME"},
		{"explicit default inactivity lifetime", "refresh_token_ttl", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL"},
	} {
		t.Run("YAML "+tt.name, func(t *testing.T) {
			_, err := loadRefreshPolicyYAML(t, fmt.Sprintf("%s  local:\n    %s: 0s\n", proxyYAML, tt.key), "proxy")
			require.ErrorContains(t, err, "oauth2_authorization_server.local")
		})
		t.Run("environment "+tt.name, func(t *testing.T) {
			t.Setenv(tt.env, "0s")
			_, err := loadRefreshPolicyYAML(t, proxyYAML, "proxy")
			require.ErrorContains(t, err, "oauth2_authorization_server.local")
		})
	}
}
