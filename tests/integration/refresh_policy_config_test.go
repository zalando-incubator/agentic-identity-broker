package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func loadRefreshDelivery(t *testing.T, contents string, environment map[string]string) (*ports.Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "broker.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
	for key, value := range environment {
		t.Setenv(key, value)
	}
	return config.NewLoader().GetConfig(context.Background())
}

func refreshDeliveryYAML(reuse, absolute, idle string) string {
	return fmt.Sprintf(`server:
  enduser:
    public_url: http://localhost:8000
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    authentication:
      preauth:
        principal_header_name: X-Remote-User
storage:
  backend: memory
oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 5s
    refresh_token_reuse_interval: %q
    absolute_session_lifetime: %q
    refresh_token_ttl: %q
third_party_oauth2:
  jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
`, reuse, absolute, idle)
}

func requireRefreshDecision(t *testing.T, cfg *ports.Config, wantRetry bool) {
	t.Helper()
	ctx := context.Background()
	broker, err := bootstrap.NewRefreshEnvironment(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(broker.Close)
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, broker.Storage))
	client, err := broker.AddClient(ctx, id.Principal("config-policy@example.test"), true)
	require.NoError(t, err)
	issued, err := helpers.AuthorizeRefreshSession(ctx, broker.Enduser.BaseURL(), *client, "read offline_access")
	require.NoError(t, err)
	rotated, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rotated.Status, rotated.Error)
	require.NotEmpty(t, rotated.Tokens.AccessToken)
	require.NotEmpty(t, rotated.Tokens.RefreshToken)
	retry, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
	require.NoError(t, err)
	if wantRetry {
		require.Equal(t, http.StatusOK, retry.Status, retry.Error)
		require.Equal(t, rotated.Tokens.AccessToken, retry.Tokens.AccessToken)
		require.Equal(t, rotated.Tokens.RefreshToken, retry.Tokens.RefreshToken)
	} else {
		require.Equal(t, http.StatusBadRequest, retry.Status)
		require.Equal(t, "invalid_grant", retry.Error)
		require.Empty(t, retry.Tokens.RefreshToken)
		require.Empty(t, retry.Tokens.AccessToken)
		blocked, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, blocked.Status)
		require.Equal(t, "invalid_grant", blocked.Error)
		require.Empty(t, blocked.Tokens.AccessToken)
		require.Empty(t, blocked.Tokens.RefreshToken)
	}
}

func TestRefreshPolicyEnvironmentOverridesYAMLInRealRefresh(t *testing.T) {
	// Source precedence must affect the issuer's decisions, not just a decoded struct.
	cfg, err := loadRefreshDelivery(t, refreshDeliveryYAML("1s", "2s", "3s"), map[string]string{
		"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL": "0s",
		"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME":    "0s",
		"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL":            "0s",
	})
	require.NoError(t, err)
	requireRefreshDecision(t, cfg, false)
}

func renderRefreshPolicyConfigMap(t *testing.T, settings ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	require.NoError(t, err)
	args := []string{
		"template", "broker", chartPath,
		"--show-only", "templates/configmap.yaml",
		"--set-string", "broker.server.enduser.publicUrl=http://localhost:8000",
		"--set-string", "broker.server.enduser.authentication.preauth.principalHeaderName=X-Remote-User",
		"--set-string", "broker.server.admin.authentication.preauth.principalHeaderName=X-Remote-User",
		"--set-string", "broker.encryption.memory.rawKey=provided-by-test-env",
	}
	args = append(args, settings...)
	output, err := exec.Command("helm", args...).CombinedOutput()
	require.NoError(t, err, "render policy ConfigMap: %s", output)
	var rendered struct {
		Data map[string]string `json:"data"`
	}
	require.NoError(t, yaml.Unmarshal(output, &rendered))
	require.NotEmpty(t, rendered.Data["config.yaml"])
	return rendered.Data["config.yaml"]
}

func chartRefreshTrustAnchor(t *testing.T) []string {
	t.Helper()
	issuer := helpers.NewMockUpstreamOAuth2Server()
	t.Cleanup(issuer.Close)
	return []string{
		"--set", "broker.security.skipThirdpartyHttpsValidation=true",
		"--set-string", "broker.tokenExchange.clientAssertion.issuerUri=" + issuer.URL(),
		"--set-string", "broker.tokenExchange.clientAssertion.jwksUri=" + issuer.URL() + "/.well-known/jwks.json",
	}
}

func chartHybridRefreshSettings(t *testing.T) []string {
	t.Helper()
	issuer := helpers.NewMockUpstreamOAuth2Server()
	t.Cleanup(issuer.Close)
	return []string{
		"--set", "broker.oauth2AuthorizationServer.mode=hybrid",
		"--set", "broker.security.skipThirdpartyHttpsValidation=true",
		"--set-string", "broker.tokenExchange.clientAssertion.issuerUri=" + issuer.URL(),
		"--set-string", "broker.tokenExchange.clientAssertion.jwksUri=" + issuer.URL() + "/.well-known/jwks.json",
		"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=" + issuer.URL(),
		"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=" + issuer.URL() + "/oauth/authorize",
		"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=" + issuer.URL() + "/oauth/token",
	}
}

func TestRefreshPolicyChartDefaultsLoadInTheirModes(t *testing.T) {
	for _, tc := range []struct {
		mode     string
		settings []string
	}{
		{mode: "proxy", settings: []string{
			"--set", "broker.oauth2AuthorizationServer.mode=proxy",
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/authorize",
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/token",
		}},
		{mode: "local"},
		{mode: "hybrid", settings: []string{
			"--set", "broker.oauth2AuthorizationServer.mode=hybrid",
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/authorize",
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/token",
		}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			cfg, err := loadRefreshDelivery(t, renderRefreshPolicyConfigMap(t, tc.settings...), nil)
			require.NoError(t, err)
			resolved, err := cfg.OAuth2AuthServer.Resolve()
			require.NoError(t, err)
			switch policy := resolved.(type) {
			case *ports.ProxyOAuth2Config:
				require.Equal(t, "proxy", tc.mode)
				require.Nil(t, cfg.OAuth2AuthServer.Local.RefreshTokenReuseInterval)
				require.Zero(t, cfg.OAuth2AuthServer.Local.RefreshTokenTTL)
			case *ports.LocalOAuth2Config:
				require.Equal(t, "local", tc.mode)
				require.Equal(t, 30*time.Second, policy.RefreshTokenReuseInterval)
				require.Zero(t, policy.AbsoluteSessionLifetime)
				require.Equal(t, 720*time.Hour, policy.RefreshTokenTTL)
			case *ports.HybridOAuth2Config:
				require.Equal(t, "hybrid", tc.mode)
				require.Equal(t, 30*time.Second, policy.Local.RefreshTokenReuseInterval)
				require.Zero(t, policy.Local.AbsoluteSessionLifetime)
				require.Equal(t, 720*time.Hour, policy.Local.RefreshTokenTTL)
			default:
				t.Fatalf("unexpected chart mode %T", policy)
			}
		})
	}
}

func TestRefreshPolicyChartZeroAndFiniteValuesDriveRefresh(t *testing.T) {
	for _, tc := range []struct {
		name, reuse, absolute, idle string
		wantRetry                   bool
	}{
		{name: "explicit zero", reuse: "0s", absolute: "0s", idle: "0s"},
		{name: "finite reuse", reuse: "1s", absolute: "4s", idle: "3s", wantRetry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := append(chartRefreshTrustAnchor(t),
				"--set-string", "broker.oauth2AuthorizationServer.local.tokenTtl=5s",
				"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval="+tc.reuse,
				"--set-string", "broker.oauth2AuthorizationServer.local.absoluteSessionLifetime="+tc.absolute,
				"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenTtl="+tc.idle,
			)
			configMap := renderRefreshPolicyConfigMap(t, settings...)
			cfg, err := loadRefreshDelivery(t, configMap, nil)
			require.NoError(t, err)
			requireRefreshDecision(t, cfg, tc.wantRetry)
		})
	}
}

func TestRefreshPolicyChartOmittedReuseFollowsAccessTTLInRealRefresh(t *testing.T) {
	for _, mode := range []string{"local", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name, accessTTL string
				wantRetry       bool
			}{
				{name: "below 30s", accessTTL: "20s"},
				{name: "equal to 30s", accessTTL: "30s"},
				{name: "above 30s", accessTTL: "31s", wantRetry: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var settings []string
					if mode == "hybrid" {
						settings = chartHybridRefreshSettings(t)
					} else {
						settings = chartRefreshTrustAnchor(t)
					}
					settings = append(settings, "--set-string", "broker.oauth2AuthorizationServer.local.tokenTtl="+tc.accessTTL)
					rendered := renderRefreshPolicyConfigMap(t, settings...)
					cfg, err := loadRefreshDelivery(t, rendered, nil)
					require.NoError(t, err, "omitted reuse must load at token_ttl=%s", tc.accessTTL)
					requireRefreshDecision(t, cfg, tc.wantRetry)
				})
			}
		})
	}
}

func TestRefreshPolicyChartExplicitThirtySecondReuseRejectsShortAccessTTL(t *testing.T) {
	for _, mode := range []string{"local", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			for _, accessTTL := range []string{"20s", "30s"} {
				t.Run(accessTTL, func(t *testing.T) {
					var settings []string
					if mode == "hybrid" {
						settings = chartHybridRefreshSettings(t)
					} else {
						settings = chartRefreshTrustAnchor(t)
					}
					settings = append(settings,
						"--set-string", "broker.oauth2AuthorizationServer.local.tokenTtl="+accessTTL,
						"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=30s",
					)
					_, err := loadRefreshDelivery(t, renderRefreshPolicyConfigMap(t, settings...), nil)
					require.ErrorContains(t, err, "refresh_token_reuse_interval")
				})
			}
		})
	}
}

func TestRefreshPolicyChartInvalidRelationsFailAtProductionLoader(t *testing.T) {
	for _, tc := range []struct {
		name, reuse, absolute, idle, field string
	}{
		{"reuse equals access lifetime", "5s", "0s", "720h", "refresh_token_reuse_interval"},
		{"reuse equals inactivity lifetime", "2s", "0s", "2s", "refresh_token_reuse_interval"},
		{"finite absolute no longer than reuse", "2s", "2s", "720h", "absolute_session_lifetime"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configMap := renderRefreshPolicyConfigMap(t,
				"--set-string", "broker.oauth2AuthorizationServer.local.tokenTtl=5s",
				"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval="+tc.reuse,
				"--set-string", "broker.oauth2AuthorizationServer.local.absoluteSessionLifetime="+tc.absolute,
				"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenTtl="+tc.idle,
			)
			_, err := loadRefreshDelivery(t, configMap, nil)
			require.ErrorContains(t, err, tc.field)
		})
	}
}

func TestRefreshPolicyChartWarnsForShortAbsoluteLifetime(t *testing.T) {
	for _, mode := range []string{"local", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{
				"--set", "broker.oauth2AuthorizationServer.mode=" + mode,
				"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=1s",
				"--set-string", "broker.oauth2AuthorizationServer.local.absoluteSessionLifetime=2s",
				"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenTtl=4s",
			}
			if mode == "hybrid" {
				upstream := helpers.NewMockUpstreamOAuth2Server()
				t.Cleanup(upstream.Close)
				args = append(args,
					"--set", "broker.security.skipThirdpartyHttpsValidation=true",
					"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri="+upstream.URL(),
					"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint="+upstream.URL()+"/oauth/authorize",
					"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint="+upstream.URL()+"/oauth/token",
				)
			}
			if mode == "local" {
				args = append(args, chartRefreshTrustAnchor(t)...)
			}
			cfg, err := loadRefreshDelivery(t, renderRefreshPolicyConfigMap(t, args...), nil)
			require.NoError(t, err, "finite absolute lifetime shorter than inactivity is a warning, not rejection")
			store, err := storageadapter.NewAdapter(&cfg.Storage)
			require.NoError(t, err)
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			application, err := bootstrap.NewServerFactory(cfg, logger).BuildApp(store)
			require.NoError(t, err)
			t.Cleanup(func() {
				if application.Shutdown != nil {
					require.NoError(t, application.Shutdown(context.Background()))
				}
				require.NoError(t, store.Close(context.Background()))
			})
			require.Contains(t, logs.String(), "absolute refresh-session lifetime is shorter than inactivity lifetime")
		})
	}
}

func TestRefreshPolicyChartRejectsInvalidSchemaAndProxyOverrides(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, mode, option, value string
	}{
		{"numeric reuse is not a duration", "local", "--set", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=0"},
		{"empty reuse is not a duration", "local", "--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval="},
		{"malformed reuse is not a duration", "local", "--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=one-day"},
		{"negative reuse is not a duration", "local", "--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=-1s"},
		{"negative absolute lifetime", "local", "--set-string", "broker.oauth2AuthorizationServer.local.absoluteSessionLifetime=-1s"},
		{"proxy rejects local overrides", "proxy", "--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=0s"},
		{"proxy rejects explicitly selected default reuse", "proxy", "--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval=30s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := []string{"--set", "broker.oauth2AuthorizationServer.mode=" + tc.mode}
			if tc.mode == "proxy" {
				base = append(base,
					"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
					"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/authorize",
					"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/token",
				)
			}
			// Baseline must render before an invalid value can count as a schema rejection.
			renderRefreshPolicyConfigMap(t, base...)
			args := append([]string{"template", "broker", chartPath, "--show-only", "templates/configmap.yaml"}, base...)
			args = append(args, tc.option, tc.value)
			_, err := exec.Command("helm", args...).CombinedOutput()
			require.Error(t, err, "Helm must reject the invalid policy value")
		})
	}
}
