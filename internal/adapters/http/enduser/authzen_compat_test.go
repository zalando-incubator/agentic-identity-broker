package enduser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestAuthZENContract_RetainsExchangeFieldsDuringDeprecation(t *testing.T) {
	t.Parallel()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../../api/enduser/openapi.yaml"))
	require.NoError(t, err)
	var contract struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
		Components struct {
			Schemas map[string]yaml.Node `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &contract))
	assert.Equal(t, "1.2.0", contract.Info.Version)
	node, ok := contract.Components.Schemas["TokenExchangeResponse"]
	require.True(t, ok)
	var response struct {
		Properties map[string]struct {
			Type                 string `yaml:"type"`
			Deprecated           bool   `yaml:"deprecated"`
			AdditionalProperties struct {
				Type  string `yaml:"type"`
				Items struct {
					Type string `yaml:"type"`
				} `yaml:"items"`
			} `yaml:"additionalProperties"`
		} `yaml:"properties"`
	}
	require.NoError(t, node.Decode(&response))
	permissions, ok := response.Properties["granted_permission_sets"]
	require.True(t, ok)
	assert.True(t, permissions.Deprecated)
	assert.Equal(t, "object", permissions.Type)
	assert.Equal(t, "array", permissions.AdditionalProperties.Type)
	assert.Equal(t, "string", permissions.AdditionalProperties.Items.Type)
	for _, name := range []string{"principal", "agent_id"} {
		property, present := response.Properties[name]
		require.True(t, present, name)
		assert.Equal(t, "string", property.Type, name)
		assert.False(t, property.Deprecated, name)
	}
}
