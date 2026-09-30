package authorization

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// MCPMessage represents a parsed JSON-RPC 2.0 message from the MCP protocol.
// Params is kept as a raw map to support arbitrary method parameter schemas.
type MCPMessage struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	ID      any            `json:"id"`
	Params  map[string]any `json:"params"`
	body    map[string]any
}

// ParseMCPMessage parses a single JSON-RPC 2.0 message from body bytes.
// Returns an error for empty body, malformed JSON, non-object JSON values,
// missing/invalid jsonrpc version, or empty method.
// Batch messages (JSON arrays) are handled by ParseMCPBatch.
func ParseMCPMessage(body []byte) (*MCPMessage, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("mcp parser: empty body")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: %w", err)
	}
	if start != json.Delim('{') {
		return nil, fmt.Errorf("mcp parser: JSON-RPC message must be an object")
	}
	envelope, err := parseMCPObject(decoder, "envelope")
	if err != nil {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("mcp parser: invalid JSON-RPC: trailing data")
	}

	msg := &MCPMessage{body: envelope, ID: envelope["id"]}
	msg.JSONRPC, _ = envelope["jsonrpc"].(string)
	msg.Method, _ = envelope["method"].(string)
	if params := envelope["params"]; params != nil {
		msg.Params = params.(map[string]any)
	}
	if msg.JSONRPC != "2.0" {
		return nil, fmt.Errorf("mcp parser: missing or invalid jsonrpc field (expected \"2.0\", got %q)", msg.JSONRPC)
	}
	if msg.Method == "" {
		return nil, fmt.Errorf("mcp parser: missing or empty method field")
	}
	return msg, nil
}

func parseMCPObject(decoder *json.Decoder, scope string) (map[string]any, error) {
	values := make(map[string]any)
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		folded := strings.Map(func(r rune) rune {
			minimum := r
			for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
				if next < minimum {
					minimum = next
				}
			}
			return minimum
		}, key)
		if _, exists := seen[folded]; exists {
			return nil, fmt.Errorf("duplicate %s key", scope)
		}
		seen[folded] = struct{}{}

		var value any
		if scope == "envelope" && key == "params" {
			start, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			switch start {
			case nil:
			case json.Delim('{'):
				value, err = parseMCPObject(decoder, "params")
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("params must be an object")
			}
		} else if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	end, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if end != json.Delim('}') {
		return nil, fmt.Errorf("expected end of %s object", scope)
	}
	return values, nil
}

// ParseMCPBatch detects and parses JSON-RPC 2.0 batch requests (FR-023).
// If body begins with '[', it is parsed as a batch and each element is parsed
// individually. Returns an error if any element is malformed.
// Returns nil, nil if body is not a JSON array (caller should use ParseMCPMessage).
func ParseMCPBatch(body []byte) ([]*MCPMessage, error) {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, nil // not a batch
	}

	var rawMessages []json.RawMessage
	if err := json.Unmarshal(body, &rawMessages); err != nil {
		return nil, fmt.Errorf("mcp parser: invalid batch JSON: %w", err)
	}

	messages := make([]*MCPMessage, 0, len(rawMessages))
	for i, raw := range rawMessages {
		msg, err := ParseMCPMessage(raw)
		if err != nil {
			return nil, fmt.Errorf("mcp parser: batch element %d: %w", i, err)
		}
		messages = append(messages, msg)
	}

	return messages, nil
}
