package enduser

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// readEnduserContract reads the contract relative to the package directory, which go test uses as
// the working directory, so -trimpath builds find it too.
func readEnduserContract(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../../../api/enduser/openapi.yaml")
	require.NoError(t, err)
	return raw
}

func TestAuthZENContract_RetainsExchangeFieldsDuringDeprecation(t *testing.T) {
	t.Parallel()
	raw := readEnduserContract(t)
	var contract struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
		Components struct {
			Schemas map[string]yaml.Node `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &contract))
	assert.Equal(t, "1.3.0", contract.Info.Version)
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

func TestAuthZENContract_EvaluationErrorsDeclareNoStore(t *testing.T) {
	t.Parallel()
	raw := readEnduserContract(t)
	type header struct {
		Ref    string `yaml:"$ref"`
		Schema struct {
			Type string   `yaml:"type"`
			Enum []string `yaml:"enum"`
		} `yaml:"schema"`
	}
	var contract struct {
		Paths      map[string]map[string]yaml.Node `yaml:"paths"`
		Components struct {
			Headers map[string]header `yaml:"headers"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &contract))
	var operation struct {
		Responses map[string]struct {
			Headers map[string]header `yaml:"headers"`
		} `yaml:"responses"`
	}
	post := contract.Paths["/access/v1/evaluation"]["post"]
	require.NoError(t, post.Decode(&operation))
	responses := operation.Responses
	for _, status := range []string{"400", "401", "403", "500"} {
		declared, ok := responses[status].Headers["Cache-Control"]
		require.True(t, ok, "%s must declare the no-store Cache-Control header", status)
		if name, isRef := strings.CutPrefix(declared.Ref, "#/components/headers/"); isRef {
			declared = contract.Components.Headers[name]
		}
		assert.Equal(t, "string", declared.Schema.Type, status)
		assert.Equal(t, []string{"no-store"}, declared.Schema.Enum, status)
	}
}
