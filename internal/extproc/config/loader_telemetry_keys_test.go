package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/configutil"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

const extprocRequiredYAML = `oauth2:
  token_endpoint: https://idp.example.com/oauth2/token
  issuer: https://idp.example.com
  client_id: test-client
  client_secret: test-secret
`

func TestLoad_TelemetryLiteralDottedKeys(t *testing.T) {
	t.Setenv("EXTPROC_MAP_VALUE", "expanded-value")
	tests := []struct {
		name       string
		yaml       string
		attributes map[string]string
		headers    map[string]string
	}{
		{
			name: "attributes coexist with same-prefix dotted keys",
			yaml: `telemetry:
  resource_attributes:
    service: plain-service
    service.namespace: team
    service.namespace.region: west
    service.version: v2
`,
			attributes: map[string]string{
				"service": "plain-service", "service.namespace": "team",
				"service.namespace.region": "west", "service.version": "v2",
			},
		},
		{
			name: "headers coexist with same-prefix dotted keys",
			yaml: `telemetry:
  exporter:
    headers:
      auth: plain-auth
      auth.token: token
      auth.token.region: west
      auth.version: v2
`,
			headers: map[string]string{
				"auth": "plain-auth", "auth.token": "token",
				"auth.token.region": "west", "auth.version": "v2",
			},
		},
		{
			name: "only header values interpolate",
			yaml: `telemetry:
  resource_attributes:
    service.namespace: ${EXTPROC_MAP_VALUE}
  exporter:
    headers:
      auth.token: ${EXTPROC_MAP_VALUE}
`,
			attributes: map[string]string{"service.namespace": "${EXTPROC_MAP_VALUE}"},
			headers:    map[string]string{"auth.token": "expanded-value"},
		},
	}
	loaders := []struct {
		name string
		load func() (*config.Config, error)
	}{
		{"Load", config.Load},
		{"LoadWithCommand", func() (*config.Config, error) { return config.LoadWithCommand(newTestCommand()) }},
		{"LoadFromViper", func() (*config.Config, error) { return config.LoadFromViper(configutil.NewViper()) }},
	}
	for _, loader := range loaders {
		t.Run(loader.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "extproc.yaml")
					require.NoError(t, os.WriteFile(path, []byte(extprocRequiredYAML+tt.yaml), 0o600))
					t.Setenv("EXTPROC_CONFIG_PATH", path)
					cfg, err := loader.load()
					require.NoError(t, err)
					assert.Equal(t, tt.attributes, cfg.Telemetry.ResourceAttributes)
					assert.Equal(t, tt.headers, cfg.Telemetry.Exporter.Headers)
				})
			}
		})
	}
}

func TestLoadFromViper_TelemetryEmptyMapsAreNil(t *testing.T) {
	for _, telemetry := range []string{
		"",
		"telemetry:\n  resource_attributes: {}\n  exporter:\n    headers: {}\n",
	} {
		name := "missing"
		if telemetry != "" {
			name = "explicitly empty"
		}
		t.Run(name, func(t *testing.T) {
			v := configutil.NewViper()
			v.SetConfigType("yaml")
			require.NoError(t, v.ReadConfig(strings.NewReader(extprocRequiredYAML+telemetry)))
			cfg, err := config.LoadFromViper(v)
			require.NoError(t, err)
			assert.Nil(t, cfg.Telemetry.ResourceAttributes)
			assert.Nil(t, cfg.Telemetry.Exporter.Headers)
		})
	}
}

func TestLoadFromViper_TelemetryNestedMapsReturnDecodeErrors(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		field string
	}{
		{
			name:  "resource attributes",
			yaml:  "telemetry:\n  resource_attributes:\n    service:\n      namespace: team\n",
			field: "resource_attributes",
		},
		{
			name:  "exporter headers",
			yaml:  "telemetry:\n  exporter:\n    headers:\n      auth:\n        token: secret\n",
			field: "headers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := configutil.NewViper()
			v.SetConfigType("yaml")
			require.NoError(t, v.ReadConfig(strings.NewReader(extprocRequiredYAML+tt.yaml)))
			_, err := config.LoadFromViper(v)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to unmarshal configuration")
			assert.Contains(t, err.Error(), tt.field)
			assert.Contains(t, err.Error(), "string")
		})
	}
}

func TestUnmarshal_TelemetryLiteralDottedKeysPreservesSource(t *testing.T) {
	v := configutil.NewViper()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`telemetry:
  service_name: extproc
  resource_attributes:
    service: plain-service
    service.namespace: team
    service.namespace.region: west
  exporter:
    headers:
      auth: plain-auth
      auth.token: ${EXTPROC_MAP_VALUE}
      auth.token.region: west
`)))
	original := v.Get("telemetry")
	encoded, err := json.Marshal(original)
	require.NoError(t, err)
	var snapshot map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &snapshot))
	want := config.Config{Telemetry: config.TelemetryConfig{
		ServiceName: "extproc",
		ResourceAttributes: map[string]string{
			"service": "plain-service", "service.namespace": "team", "service.namespace.region": "west",
		},
		Exporter: config.OTLPExporterConfig{Headers: map[string]string{
			"auth": "plain-auth", "auth.token": "${EXTPROC_MAP_VALUE}", "auth.token.region": "west",
		}},
	}}
	var first, second config.Config
	require.NoError(t, v.Unmarshal(&first))
	assert.Equal(t, want, first)
	assert.Equal(t, snapshot, original)
	require.NoError(t, v.Unmarshal(&second))
	assert.Equal(t, want, second)
	assert.Equal(t, first, second)
	assert.Equal(t, snapshot, original)
	assert.Equal(t, snapshot, v.Get("telemetry"))
}

func TestLoadFromViper_TelemetryKnownDottedLeafEnvironment(t *testing.T) {
	t.Setenv("EXTPROC_TELEMETRY_RESOURCE_ATTRIBUTES_SERVICE_NAMESPACE", "env-team")
	t.Setenv("EXTPROC_TELEMETRY_EXPORTER_HEADERS_AUTH_TOKEN", "env-token")
	v := configutil.NewViper()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(extprocRequiredYAML+`telemetry:
  resource_attributes:
    service.namespace: yaml-team
  exporter:
    headers:
      auth.token: yaml-token
`)))
	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"service.namespace": "env-team"}, cfg.Telemetry.ResourceAttributes)
	assert.Equal(t, map[string]string{"auth.token": "env-token"}, cfg.Telemetry.Exporter.Headers)
}

func TestLoadWithCommand_ScalarPrecedence(t *testing.T) {
	const yamlScalars = `grpc:
  port: 7777
circuit_breaker:
  enabled: false
oauth2:
  exchange_timeout: 2s
telemetry:
  traces:
    sampling_rate: 0
`
	tests := []struct {
		name    string
		yaml    bool
		env     bool
		cli     bool
		port    int
		enabled bool
		rate    float64
		timeout time.Duration
	}{
		{"defaults", false, false, false, 50051, true, 1, 5 * time.Second},
		{"YAML overrides defaults including false and zero", true, false, false, 7777, false, 0, 2 * time.Second},
		{"environment overrides YAML including false and zero", true, true, false, 8888, false, 0, 3 * time.Second},
		{"CLI overrides environment including false and zero", true, true, true, 9999, false, 0, 4 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "telemetry:\n  enabled: true\n  exporter:\n    endpoint: collector:4317\n"
			if tt.yaml {
				yaml = yamlScalars + "  enabled: true\n  exporter:\n    endpoint: collector:4317\n"
				if tt.env {
					yaml = strings.ReplaceAll(yaml, "enabled: false", "enabled: true")
					yaml = strings.ReplaceAll(yaml, "sampling_rate: 0", "sampling_rate: 0.7")
				}
			}
			// Supply required OAuth2 fields through the environment so YAML can
			// independently exercise the scalar exchange_timeout field.
			t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
			t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://idp.example.com")
			t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "test-client")
			t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "test-secret")
			path := filepath.Join(t.TempDir(), "extproc.yaml")
			require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
			t.Setenv("EXTPROC_CONFIG_PATH", path)
			if tt.env {
				t.Setenv("EXTPROC_GRPC_PORT", "8888")
				t.Setenv("EXTPROC_CIRCUIT_BREAKER_ENABLED", "false")
				t.Setenv("EXTPROC_TELEMETRY_TRACES_SAMPLING_RATE", "0")
				t.Setenv("EXTPROC_OAUTH2_EXCHANGE_TIMEOUT", "3s")
			}
			cmd := newTestCommand()
			if tt.cli {
				t.Setenv("EXTPROC_CIRCUIT_BREAKER_ENABLED", "true")
				t.Setenv("EXTPROC_TELEMETRY_TRACES_SAMPLING_RATE", "0.5")
				require.NoError(t, cmd.Flags().Set("grpc.port", "9999"))
				require.NoError(t, cmd.Flags().Set("circuit_breaker.enabled", "false"))
				require.NoError(t, cmd.Flags().Set("telemetry.traces.sampling_rate", "0"))
				require.NoError(t, cmd.Flags().Set("oauth2.exchange_timeout", "4s"))
			}
			cfg, err := config.LoadWithCommand(cmd)
			require.NoError(t, err)
			assert.Equal(t, tt.port, cfg.GRPC.Port)
			assert.Equal(t, tt.enabled, cfg.CircuitBreaker.Enabled)
			assert.Equal(t, tt.rate, cfg.Telemetry.Traces.SamplingRate)
			assert.Equal(t, tt.timeout, cfg.OAuth2.ExchangeTimeout)
		})
	}
}
