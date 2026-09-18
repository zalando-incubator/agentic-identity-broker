package authorization

import (
	"bytes"
	"testing"
)

func FuzzParseMCPMessage(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`[`))

	f.Fuzz(func(t *testing.T, body []byte) {
		message, err := ParseMCPMessage(body)
		if err == nil {
			assertValidMCPMessage(t, message)
		}

		messages, err := ParseMCPBatch(body)
		if err != nil {
			return
		}

		trimmed := bytes.TrimLeft(body, " \t\r\n")
		if len(trimmed) > 0 && trimmed[0] == '[' && messages == nil {
			t.Fatal("valid JSON-RPC batch parsed as nil")
		}
		for _, message := range messages {
			assertValidMCPMessage(t, message)
		}
	})
}

func assertValidMCPMessage(t *testing.T, message *MCPMessage) {
	t.Helper()
	if message == nil {
		t.Fatal("successful MCP parse returned nil message")
	}
	if message.JSONRPC != "2.0" {
		t.Fatalf("successful MCP parse version = %q, want 2.0", message.JSONRPC)
	}
	if message.Method == "" {
		t.Fatal("successful MCP parse returned empty method")
	}
}
