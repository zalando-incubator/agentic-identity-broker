package authorization

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/open-policy-agent/opa-envoy-plugin/envoyauth"
	"github.com/open-policy-agent/opa/v1/logging"
)

// InputBuilder converts ExtProc headers to the opa-envoy-plugin input shape once
// and builds an independent OPA document for each decoded body or batch element.
type InputBuilder struct {
	base             OPAInput
	protocol         string
	sessionID        string
	targetServerName string
	context          ContextInput
}

func NewInputBuilder(protocol string, headers map[string]string, targetServerName string, contextInput ContextInput) (*InputBuilder, error) {
	input, err := envoyauth.RequestToInput(buildCheckRequest(headers), nopLogger{}, nil, true)
	if err != nil {
		return nil, fmt.Errorf("input builder: envoyauth.RequestToInput failed: %w", err)
	}
	return &InputBuilder{base: input, protocol: protocol, sessionID: extractSessionID(headers), targetServerName: targetServerName, context: contextInput}, nil
}

func (b *InputBuilder) Build(body []byte, parsed any) (OPAInput, error) {
	input := maps.Clone(b.base)
	attributes := maps.Clone(input["attributes"].(map[string]any))
	request := maps.Clone(attributes["request"].(map[string]any))
	http := maps.Clone(request["http"].(map[string]any))
	if len(body) > 0 {
		http["body"] = string(body)
	}
	request["http"] = http
	attributes["request"] = request
	input["attributes"] = attributes
	input["parsed_body"] = parsed
	input["truncated_body"] = false
	input["context"] = b.context
	if b.protocol != "mcp" {
		input["type"] = "unknown"
		return input, nil
	}
	return buildMCPInput(input, parsed, b.sessionID, b.targetServerName)
}

// buildCheckRequest bridges ExtProc headers to the ext_authz type used by the plugin.
func buildCheckRequest(headers map[string]string) *ext_authz_v3.CheckRequest {
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
	checkReq := buildCheckRequest(headers)
	input, err := envoyauth.RequestToInput(checkReq, nopLogger{}, nil, true)
	if err != nil {
		return nil, fmt.Errorf("input builder: envoyauth.RequestToInput failed: %w", err)
	}
	input["parsed_body"] = nil
	input["truncated_body"] = false
	input["context"] = ContextInput{GrantedPermissionSetsAvailable: false, AgentSessionID: extractSessionID(headers)}

	if protocol == "mcp" {
		input["type"] = "mcp_headers_only"
		mcp := map[string]any{}
		if sessionID := extractSessionID(headers); sessionID != "" {
			mcp["session_id"] = sessionID
		}
		if targetServerName != "" {
			mcp["target_server_name"] = targetServerName
		}
		input["mcp"] = mcp
	} else {
		input["type"] = "unknown"
	}
	return input, nil
}

// buildMCPInput adds MCP-specific fields to the OPA input map.
func buildMCPInput(input OPAInput, parsed any, sessionID, targetServerName string) (OPAInput, error) {
	msg, err := ParseMCPMessage(parsed)
	if err != nil {
		return nil, fmt.Errorf("input builder: %w", err)
	}

	mcpInput := map[string]any{"jsonrpc": msg.JSONRPC, "method": msg.Method}
	if msg.ID != nil {
		mcpInput["id"] = msg.ID
	}
	if sessionID != "" {
		mcpInput["session_id"] = sessionID
	}
	if targetServerName != "" {
		mcpInput["target_server_name"] = targetServerName
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
		mcpInput["tool_name"] = name
		if args, ok := msg.Params["arguments"].(map[string]any); ok && len(args) > 0 {
			mcpInput["arguments"] = args
		}
		input["type"] = "mcp_tool_call"
	} else {
		input["type"] = "mcp_method"
		if len(msg.Params) > 0 {
			mcpInput["params"] = msg.Params
		}
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
