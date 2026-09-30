package authorization_test

import (
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
			assert.Contains(t, err.Error(), "duplicate")

			_, err = authorization.BuildOPAInput("mcp", []byte(tc.body), nil, "", authorization.ContextInput{})
			require.Error(t, err)
		})
	}
}

func TestParseMCPBatchRejectsAmbiguousElement(t *testing.T) {
	_, err := authorization.ParseMCPBatch([]byte(`[{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe"}},{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","Name":"dangerous"}}]`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch element 1")
}
