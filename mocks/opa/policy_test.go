package opa_test

import (
	"context"
	"testing"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/stretchr/testify/require"
)

func TestComposeMCPConnectionPolicy(t *testing.T) {
	policy, err := rego.New(
		rego.Query("data.aib.extproc.authz.result"),
		rego.Load([]string{"bundle/aib/extproc/authz/main.rego"}, nil),
	).PrepareForEval(context.Background())
	require.NoError(t, err)

	tests := []struct {
		name       string
		input      map[string]any
		wantAction string
	}{
		{
			name: "tool discovery",
			input: map[string]any{
				"type": "mcp_method", "mcp": map[string]any{"method": "tools/list"},
			},
			wantAction: "allow",
		},
		{
			name: "session stream",
			input: map[string]any{
				"type": "mcp_headers_only",
				"attributes": map[string]any{"request": map[string]any{"http": map[string]any{
					"method": "GET", "path": "/mcp",
				}}},
			},
			wantAction: "allow",
		},
		{
			name: "other path is not a session stream",
			input: map[string]any{
				"type": "mcp_headers_only",
				"attributes": map[string]any{"request": map[string]any{"http": map[string]any{
					"method": "GET", "path": "/other",
				}}},
			},
		},
		{
			name: "header-only POST is not a session stream",
			input: map[string]any{
				"type": "mcp_headers_only",
				"attributes": map[string]any{"request": map[string]any{"http": map[string]any{
					"method": "POST", "path": "/mcp",
				}}},
			},
		},
		{
			name: "destructive tool stays denied",
			input: map[string]any{
				"type": "mcp_tool_call", "mcp": map[string]any{"tool_name": "delete_repository"},
			},
			wantAction: "deny",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := policy.Eval(context.Background(), rego.EvalInput(tt.input))
			require.NoError(t, err)
			if tt.wantAction == "" {
				require.Empty(t, result, "unlisted operations must remain fail-closed")
				return
			}
			require.Len(t, result, 1)
			require.Len(t, result[0].Expressions, 1)
			decision, ok := result[0].Expressions[0].Value.(map[string]any)
			require.True(t, ok)
			require.Equal(t, tt.wantAction, decision["action"])
		})
	}
}
