package authorization

import (
	"fmt"
	"strings"
)

// MCPMessage represents a parsed JSON-RPC 2.0 message from the MCP protocol.
// Params is kept as a raw map to support arbitrary method parameter schemas.
type MCPMessage struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	ID      any            `json:"id"`
	Params  map[string]any `json:"params"`
}

// ParseMCPMessage validates a decoded JSON-RPC 2.0 message.
// Batch elements use the same validation as standalone messages.
func ParseMCPMessage(value any) (*MCPMessage, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: expected object")
	}
	if field := duplicateEnvelopeField(object); field != "" {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: duplicate %s field", field)
	}
	msg := &MCPMessage{ID: object["id"]}
	if version, ok := object["jsonrpc"].(string); ok {
		msg.JSONRPC = version
	} else if object["jsonrpc"] != nil {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: jsonrpc must be a string")
	}
	if method, ok := object["method"].(string); ok {
		msg.Method = method
	} else if object["method"] != nil {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: method must be a string")
	}
	if params, ok := object["params"].(map[string]any); ok {
		msg.Params = params
	} else if object["params"] != nil {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: params must be an object")
	}
	if msg.JSONRPC != "2.0" {
		return nil, fmt.Errorf("mcp parser: missing or invalid jsonrpc field (expected \"2.0\", got %q)", msg.JSONRPC)
	}
	if msg.Method == "" {
		return nil, fmt.Errorf("mcp parser: missing or empty method field")
	}
	return msg, nil
}

func duplicateEnvelopeField(object map[string]any) string {
	for _, field := range [...]string{"jsonrpc", "id", "method", "params"} {
		matches := 0
		for key := range object {
			if strings.EqualFold(key, field) {
				matches++
			}
		}
		if matches > 1 {
			return field
		}
	}
	return ""
}
