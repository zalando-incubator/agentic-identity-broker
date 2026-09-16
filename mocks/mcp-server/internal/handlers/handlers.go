// Package handlers provides MCP tool handlers and HTTP health checks for the
// MCP server mock. It uses the mcp-go library for full protocol compliance.
//
// Registered tools:
//   - echo — echoes the provided message back to the caller
//   - whoami — returns the Authorization token scheme and JWT claims visible to the MCP server
//   - create_issue — simulates a GitHub issue creation after user approval
//
// The Authorization header is captured via HTTPContextFunc so that tool
// handlers can inspect the token injected by ExtProc.
package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// contextKey is used for storing the Authorization header in context.
type contextKey string

const authHeaderKey contextKey = "authHeader"

// Health returns 200 OK for container liveness probes.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok"))
}

// NewMCPServer creates an MCP server with the demo tools registered.
func NewMCPServer() *server.MCPServer {
	s := server.NewMCPServer("mock-mcp-server", "1.0.0")

	echoTool := mcp.NewTool("echo",
		mcp.WithDescription("Echoes the provided message back to the caller."),
		mcp.WithString("message",
			mcp.Description("The message to echo"),
			mcp.Required(),
		),
	)

	whoamiTool := mcp.NewTool("whoami",
		mcp.WithDescription("Returns the Authorization token scheme visible to the MCP server."),
	)

	createIssueTool := mcp.NewTool("create_issue",
		mcp.WithDescription("Creates a demo GitHub issue after approval."),
		mcp.WithString("repository", mcp.Description("Repository that receives the issue"), mcp.Required()),
		mcp.WithString("title", mcp.Description("Issue title"), mcp.Required()),
	)

	s.AddTool(echoTool, handleEcho)
	s.AddTool(whoamiTool, handleWhoami)
	s.AddTool(createIssueTool, handleCreateIssue)
	return s
}

// AuthHeaderContextFunc is an HTTPContextFunc that captures the Authorization
// header from incoming requests and stores it in the context for tool handlers.
func AuthHeaderContextFunc(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, authHeaderKey, r.Header.Get("Authorization"))
}

// handleEcho implements the "echo" tool.
func handleEcho(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	msg, _ := request.GetArguments()["message"].(string)
	return mcp.NewToolResultText(fmt.Sprintf("echo: %s", msg)), nil
}

// handleWhoami implements the "whoami" tool: extracts Authorization info and
// JWT claims from the bearer token captured in the request context.
func handleWhoami(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	authHeader, _ := ctx.Value(authHeaderKey).(string)

	scheme := extractScheme(authHeader)

	slog.Info("MCP whoami request",
		"auth_present", authHeader != "",
		"auth_scheme", scheme)

	// Extract JWT claims from bearer token if available
	var claims map[string]any
	if scheme == "Bearer" {
		token := extractBearerToken(authHeader)
		if token != "" {
			claims = extractJWTClaims(token)
			slog.Info("JWT claims extraction", "claims_count", len(claims), "has_claims", claims != nil)
		}
	}

	// Build content text with claims serialized for display
	contentText := fmt.Sprintf("auth_scheme=%s", scheme)
	if claims != nil && len(claims) > 0 {
		claimsJSON, _ := json.Marshal(claims)
		contentText = fmt.Sprintf("%s\njwt_claims=%s", contentText, string(claimsJSON))
	}

	slog.Info("MCP whoami response", "auth_scheme", scheme, "claims_included", claims != nil)
	return mcp.NewToolResultText(contentText), nil
}

// handleCreateIssue simulates the approved action in the Compose demonstration.
func handleCreateIssue(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Created demo issue #1"), nil
}

// extractScheme returns just the scheme word from an Authorization header value (e.g., "Bearer").
func extractScheme(authHeader string) string {
	if authHeader == "" {
		return "none"
	}
	parts := strings.SplitN(authHeader, " ", 2)
	return parts[0]
}

// extractBearerToken extracts the token from a "Bearer <token>" Authorization header.
func extractBearerToken(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}
	return ""
}

// extractJWTClaims extracts all claims from a JWT token without verification.
func extractJWTClaims(token string) map[string]any {
	// JWT format: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}

	// Decode the payload (second part)
	payload := parts[1]

	// Add padding if needed for base64 decoding
	switch len(payload) % 4 {
	case 1:
		payload += "==="
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		slog.Debug("failed to decode JWT payload", "error", err)
		return nil
	}

	// Parse the JSON payload
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		slog.Debug("failed to parse JWT claims", "error", err)
		return nil
	}

	return claims
}
