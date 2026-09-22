package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa/v1/logging"
)

// BuildOPAInput constructs an OPA input document from a request's protocol, body, and headers.
//
// The base layer is produced by the opa-envoy-plugin's envoyauth.RequestToInput function,
// which converts an ext_authz v3 CheckRequest into a map[string]any. This ensures full
// compatibility with existing opa-envoy-plugin Rego libraries and policies.
//
// ExtProc-specific extensions (type, mcp, context) are added on top so policies can
// reference both envoy-plugin fields (input.attributes.request.http.method) and
// MCP-specific fields (input.mcp.tool_name).
//
// Dispatches on protocol to perform protocol-specific parsing:
//   - "mcp": parses body as JSON-RPC 2.0, sets type discriminator, extracts MCP fields
//   - any other value: type="unknown", body stored as raw string
//
// targetServerName populates mcp.target_server_name when protocol == "mcp" and
// non-empty; it is agentgateway routing metadata, not an MCP protocol field, so it
// is never set for other protocols.
//
// The returned OPAInput (map[string]any) is safe for direct use as the OPA input document.
func BuildOPAInput(protocol string, body []byte, headers map[string]string, targetServerName string, grantedPermissionSets map[string][]string) (OPAInput, error) {
	// Build an ext_authz v3 CheckRequest from the ExtProc data so we can delegate
	// the base input construction to the opa-envoy-plugin library.
	checkReq := buildCheckRequest(headers, body)

	// Use the opa-envoy-plugin's RequestToInput to produce the envoy-compatible base layer.
	// skipRequestBodyParse=true because we handle body parsing ourselves (MCP-aware).
	input, err := envoyauth.RequestToInput(checkReq, nopLogger{}, nil, true)
	if err != nil {
		return nil, fmt.Errorf("input builder: envoyauth.RequestToInput failed: %w", err)
	}

	// Parse the body ourselves for parsed_body (MCP-aware JSON parsing).
	input["parsed_body"] = parseJSONBody(body)
	input["truncated_body"] = false

	// Add ExtProc-specific extensions on top of the envoy-compatible base.
	contextInput := ContextInput{}
	if grantedPermissionSets != nil {
		contextInput.GrantedPermissionSetsAvailable = true
		contextInput.GrantedPermissionSets = grantedPermissionSets
	}
	input["context"] = contextInput

	switch protocol {
	case "mcp":
		return buildMCPInput(input, body, headers, targetServerName)
	default:
		input["type"] = "unknown"
		return input, nil
	}
}

// buildCheckRequest constructs an ext_authz v3 CheckRequest from ExtProc header data.
// This bridges the ExtProc gRPC types to the ext_authz types expected by the opa-envoy-plugin.
func buildCheckRequest(headers map[string]string, body []byte) *ext_authz_v3.CheckRequest {
	// Filter out pseudo-headers from the headers map for the ext_authz representation.
	// The ext_authz HttpRequest has dedicated fields for method, path, host, scheme.
	httpHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		if !strings.HasPrefix(k, ":") {
			httpHeaders[k] = v
		}
	}

	return &ext_authz_v3.CheckRequest{
		Attributes: &ext_authz_v3.AttributeContext{
			Request: &ext_authz_v3.AttributeContext_Request{
				Http: &ext_authz_v3.AttributeContext_HttpRequest{
					Method:   headers[":method"],
					Path:     headers[":path"],
					Host:     headers[":authority"],
					Scheme:   headers[":scheme"],
					Protocol: headers[":protocol"],
					Headers:  httpHeaders,
					Body:     string(body),
				},
			},
		},
	}
}

// BuildOPAInputHeadersOnly constructs an OPA input document for header-only requests
// (end_of_stream=true in the RequestHeaders phase, no body phase follows).
//
// Header-only requests are evaluated before token exchange, so
// context.granted_permission_sets is unavailable and omitted by design.
// The explicit availability bit lets policies deny on unavailable context.
// For "mcp" protocol: type="mcp_headers_only" with session ID and target_server_name
// (agentgateway routing metadata, present regardless of header-only status). This lets
// OPA policies distinguish MCP header-only requests (SSE/WebSocket upgrades, GET /mcp)
// from non-MCP traffic rather than mapping both to type="unknown".
// For any other protocol: type="unknown".
func BuildOPAInputHeadersOnly(protocol string, headers map[string]string, targetServerName string) (OPAInput, error) {
	checkReq := buildCheckRequest(headers, nil)
	input, err := envoyauth.RequestToInput(checkReq, nopLogger{}, nil, true)
	if err != nil {
		return nil, fmt.Errorf("input builder: envoyauth.RequestToInput failed: %w", err)
	}
	input["parsed_body"] = nil
	input["truncated_body"] = false
	input["context"] = ContextInput{GrantedPermissionSetsAvailable: false}

	if protocol == "mcp" {
		input["type"] = "mcp_headers_only"
		input["mcp"] = &MCPInput{SessionID: extractSessionID(headers), TargetServerName: targetServerName}
	} else {
		input["type"] = "unknown"
	}
	return input, nil
}

// buildMCPInput adds MCP-specific fields to the OPA input map.
// For tools/call: type="mcp_tool_call", populates mcp.tool_name + mcp.arguments.
// For other methods: type="mcp_method", populates mcp.method + mcp.params.
func buildMCPInput(input OPAInput, body []byte, headers map[string]string, targetServerName string) (OPAInput, error) {
	msg, err := ParseMCPMessage(body)
	if err != nil {
		return nil, fmt.Errorf("input builder: %w", err)
	}

	mcpInput := &MCPInput{
		JSONRPC:          msg.JSONRPC,
		Method:           msg.Method,
		ID:               msg.ID,
		SessionID:        extractSessionID(headers),
		TargetServerName: targetServerName,
	}

	if msg.Method == "tools/call" {
		// tools/call requires a non-empty params.name to identify the tool.
		// A missing or non-string name is a malformed request that cannot be
		// meaningfully authorized, so we return a parse error to deny it.
		if msg.Params == nil {
			return nil, fmt.Errorf("input builder: tools/call missing params")
		}
		name, ok := msg.Params["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("input builder: tools/call missing or invalid params.name")
		}
		mcpInput.ToolName = name
		if args, ok := msg.Params["arguments"].(map[string]any); ok {
			mcpInput.Arguments = args
		}
		input["type"] = "mcp_tool_call"
	} else {
		input["type"] = "mcp_method"
		mcpInput.Params = msg.Params
	}

	input["mcp"] = mcpInput
	return input, nil
}

// extractSessionID extracts the Mcp-Session-Id header (case-insensitive) from the header map.
// Returns empty string if the header is absent (per FR-020).
func extractSessionID(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "mcp-session-id") {
			return v
		}
	}
	return ""
}

// parseJSONBody attempts to parse body as JSON. Returns the parsed value on success,
// or nil if the body is empty or not valid JSON.
func parseJSONBody(body []byte) any {
	if len(body) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return parsed
}

// nopLogger implements the OPA logging.Logger interface with no-op methods.
// Used when calling envoyauth.RequestToInput since we handle our own logging.
type nopLogger struct{}

func (nopLogger) Debug(string, ...any)                       {}
func (nopLogger) Info(string, ...any)                        {}
func (nopLogger) Warn(string, ...any)                        {}
func (nopLogger) Error(string, ...any)                       {}
func (nopLogger) WithFields(map[string]any) logging.Logger   { return nopLogger{} }
func (nopLogger) WithContext(context.Context) logging.Logger { return nopLogger{} }
func (nopLogger) GetLevel() logging.Level                    { return logging.Error }
func (nopLogger) SetLevel(logging.Level)                     {}

// NewOPALogger creates an OPA logging.Logger that delegates to slog.
func NewOPALogger(logger *slog.Logger) logging.Logger {
	if logger == nil {
		return nopLogger{}
	}
	return &slogOPALogger{logger: logger}
}

type slogOPALogger struct {
	logger *slog.Logger
	fields map[string]any
}

func (l *slogOPALogger) Debug(fmt string, a ...any) { l.logger.Debug(fmt, toSlogAttrs(l.fields, a)...) }
func (l *slogOPALogger) Info(fmt string, a ...any)  { l.logger.Info(fmt, toSlogAttrs(l.fields, a)...) }
func (l *slogOPALogger) Warn(fmt string, a ...any)  { l.logger.Warn(fmt, toSlogAttrs(l.fields, a)...) }
func (l *slogOPALogger) Error(fmt string, a ...any) { l.logger.Error(fmt, toSlogAttrs(l.fields, a)...) }
func (l *slogOPALogger) WithFields(fields map[string]any) logging.Logger {
	merged := make(map[string]any, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &slogOPALogger{logger: l.logger, fields: merged}
}
func (l *slogOPALogger) WithContext(_ context.Context) logging.Logger { return l }
func (l *slogOPALogger) GetLevel() logging.Level                      { return logging.Debug }
func (l *slogOPALogger) SetLevel(logging.Level)                       {}

func toSlogAttrs(fields map[string]any, args []any) []any {
	attrs := make([]any, 0, len(fields)*2+len(args))
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}
	attrs = append(attrs, args...)
	return attrs
}
