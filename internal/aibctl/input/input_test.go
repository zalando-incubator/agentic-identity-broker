package input_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/input"
)

func TestDecodeYAMLFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte("display_name: R\ndescription: d\ncanonical_id: null\npermission_sets:\n  - permission_set_id: ps\n    requirement_type: mandatory\n"), 0o600))
	var w client.AgentWrite
	require.NoError(t, input.Decode(path, nil, &w))
	assert.Equal(t, "R", w.DisplayName)
	assert.True(t, w.CanonicalID.Set)
	assert.True(t, w.CanonicalID.Null)
	assert.False(t, w.ClientID.Set)
	assert.Equal(t, "ps", w.PermissionSets[0].PermissionSetID)
}

func TestDecodeJSONFromStdin(t *testing.T) {
	var w client.AgentWrite
	require.NoError(t, input.Decode("-", strings.NewReader(`{"display_name":"R","description":"d","canonical_id":"r","permission_sets":[]}`), &w))
	assert.Equal(t, "r", w.CanonicalID.Value)
}

func TestDecodeDropsReadOnlyFields(t *testing.T) {
	var w client.AgentWrite
	require.NoError(t, input.Decode("-", strings.NewReader(`{"id":"u","created_at":"c","updated_at":"u","display_name":"R","description":"d","permission_sets":[]}`), &w))
	assert.Equal(t, "R", w.DisplayName)
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	var w client.AgentWrite
	err := input.Decode("-", strings.NewReader(`{"display_name":"R","nope":1}`), &w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

func TestDecodeRejectsEmptyAndInvalid(t *testing.T) {
	var w client.AgentWrite
	assert.Error(t, input.Decode("-", strings.NewReader("   "), &w))
	assert.Error(t, input.Decode("-", strings.NewReader("{bad"), &w))
	assert.Error(t, input.Decode("-", strings.NewReader("a: [b"), &w))
	assert.Error(t, input.Decode(filepath.Join(t.TempDir(), "missing.yaml"), nil, &w))
}
