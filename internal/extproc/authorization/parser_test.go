package authorization_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
)

func TestParseMCPMessageRejectsAmbiguousKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"duplicate envelope method", `{"jsonrpc":"2.0","method":"tools/call","method":"initialize","params":{"name":"safe"}}`},
		{"folded envelope method", `{"jsonrpc":"2.0","method":"tools/call","Method":"initialize","params":{"name":"safe"}}`},
		{"folded envelope id", `{"jsonrpc":"2.0","method":"tools/call","id":1,"ID":2,"params":{"name":"safe"}}`},
		{"duplicate params name", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","name":"dangerous"}}`},
		{"folded params name", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","Name":"dangerous"}}`},
		{"folded params arguments", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","arguments":{"path":"/safe"},"Arguments":{"path":"/dangerous"}}}`},
		{"escaped params name", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","\u006eame":"dangerous"}}`},
		{"Unicode fold", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","K":1,"\u212a":2}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := authorization.ParseMCPMessage([]byte(tc.body))
			require.Error(t, err)

			_, err = authorization.BuildOPAInput("mcp", []byte(tc.body), nil, "", authorization.ContextInput{})
			require.Error(t, err)
		})
	}
}

func TestParseMCPBatchRejectsAmbiguousElement(t *testing.T) {
	_, err := authorization.ParseMCPBatch([]byte(`[{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe"}},{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","Name":"dangerous"}}]`))
	require.Error(t, err)
}

func TestParseMCPMessageRejectsNoncanonicalKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"tool arguments alias", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"deploy","Arguments":{"path":"/prod"}}}`},
		{"escaped tool arguments alias", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"deploy","\u0041rguments":{"path":"/prod"}}}`},
		{"non-tool params alias", `{"jsonrpc":"2.0","method":"initialize","Params":{"capabilities":{"tools":true}}}`},
		{"request id alias", `{"jsonrpc":"2.0","method":"initialize","ID":42,"params":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := authorization.ParseMCPMessage([]byte(tc.body))
			require.Error(t, err)
		})
	}

	_, err := authorization.ParseMCPBatch([]byte(`[{"jsonrpc":"2.0","method":"initialize","params":{}},{"jsonrpc":"2.0","method":"initialize","Params":{}}]`))
	require.Error(t, err)
}

func TestParseMCPMessagePreservesNumericLexemes(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"tools/call","id":9007199254740993,"params":{"name":"deploy","arguments":{"count":9007199254740993,"nested":[1.234567890123456789]}}}`)
	msg, err := authorization.ParseMCPMessage(body)
	require.NoError(t, err)
	assert.Equal(t, json.Number("9007199254740993"), msg.ID)
	args := msg.Params["arguments"].(map[string]any)
	assert.Equal(t, json.Number("9007199254740993"), args["count"])
	assert.Equal(t, json.Number("1.234567890123456789"), args["nested"].([]any)[0])

	input, err := authorization.BuildOPAInput("mcp", body, nil, "", authorization.ContextInput{})
	require.NoError(t, err)
	assert.Equal(t, json.Number("9007199254740993"), input["mcp"].(*authorization.MCPInput).ID)
	assert.Equal(t, args, input["mcp"].(*authorization.MCPInput).Arguments)
}
