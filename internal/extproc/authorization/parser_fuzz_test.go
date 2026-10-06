package authorization

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzParseMCPMessage(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`[`))

	f.Fuzz(func(t *testing.T, body []byte) {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var parsed any
		if decoder.Decode(&parsed) != nil || len(bytes.TrimSpace(body[decoder.InputOffset():])) != 0 {
			return
		}
		message, err := ParseMCPMessage(parsed)
		if err == nil {
			assertValidMCPMessage(t, message)
		}
		if batch, ok := parsed.([]any); ok {
			for _, element := range batch {
				message, err := ParseMCPMessage(element)
				if err == nil {
					assertValidMCPMessage(t, message)
				}
			}
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
