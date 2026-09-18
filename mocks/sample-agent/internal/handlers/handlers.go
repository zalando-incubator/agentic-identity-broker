package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentic-identity-broker/sample-agent/internal/config"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/oauth2"
)

// UserInfo represents user information retrieved from OAuth2
type UserInfo struct {
	Sub    string
	Claims map[string]interface{}
}

// Session represents a user session
type Session struct {
	Token        *oauth2.Token
	UserInfo     *UserInfo
	CreatedAt    time.Time
	ExpiresAt    int64
	CSRFState    string
	ClientType   string         // "proxy", "local", or "cimd"
	OAuth2Config *oauth2.Config // per-flow config (correct client_id/secret)
	PKCEVerifier string         // non-empty for local/cimd flows that require PKCE
	MCPClient    *mcpclient.Client
	MCPMu        sync.Mutex
	mcpClosed    bool
}

// Handlers handles HTTP requests
type Handlers struct {
	oauth2Config *oauth2.Config
	cfg          *config.Config
	sessions     map[string]*Session
	sessionsMu   sync.RWMutex
}

// New creates a new Handlers instance
func New(oauth2Config *oauth2.Config, cfg *config.Config) *Handlers {
	return &Handlers{
		oauth2Config: oauth2Config,
		cfg:          cfg,
		sessions:     make(map[string]*Session),
	}
}

// Health returns a health check response
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "sample-oauth2-client",
	})
}

// Home renders the home page with login or user info
func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getSessionID(r)

	h.sessionsMu.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionsMu.RUnlock()

	if exists && session.Token.Valid() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		slog.Info("Displaying user info",
			"sub", session.UserInfo.Sub,
			"expiresAt", session.ExpiresAt)
		rawToken := ""
		if session.Token != nil {
			rawToken = session.Token.AccessToken
		}
		keyID := ""
		if rawToken != "" {
			if header, err := extractTokenHeader(rawToken); err == nil {
				if k, ok := header["kid"].(string); ok {
					keyID = k
				}
			}
		}
		fmt.Fprint(w, renderUserPage(session.UserInfo, session.ExpiresAt, session.ClientType, rawToken, keyID))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, renderSelectorPage(h.cfg))
}

// Login initiates the OAuth2 authorization flow for a chosen client type.
// Accepts ?type=proxy|local|cimd; defaults to proxy.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	clientType := r.URL.Query().Get("type")
	var agentID string
	switch clientType {
	case "local":
		agentID = h.cfg.OAuth2.LocalAgentID
	case "cimd":
		// For CIMD, the client_id in the authorize URL is the metadata URI, not the broker UUID.
		agentID = h.cfg.OAuth2.CIMDClientURI
	default:
		clientType = "proxy"
		agentID = h.cfg.OAuth2.ProxyAgentID
		if agentID == "" {
			agentID = h.cfg.OAuth2.ClientID // fallback for non-compose environments
		}
	}

	if agentID == "" {
		http.Error(w, "agent ID not available for type "+clientType+" — seed data may still be loading", http.StatusServiceUnavailable)
		return
	}

	state := generateRandomString(32)
	sessionID := generateRandomString(32)

	// Build per-flow OAuth2 config with the selected agent's credentials.
	flowConfig := *h.oauth2Config
	flowConfig.ClientID = agentID
	// Local and CIMD agents are public clients (no client secret).
	if clientType == "local" || clientType == "cimd" {
		flowConfig.ClientSecret = ""
	}

	// Generate PKCE verifier for local/CIMD flows (broker enforces S256).
	var pkceVerifier string
	if clientType == "local" || clientType == "cimd" {
		pkceVerifier = oauth2.GenerateVerifier()
	}

	h.sessionsMu.Lock()
	h.sessions[sessionID] = &Session{
		CreatedAt:    time.Now(),
		CSRFState:    state,
		UserInfo:     &UserInfo{},
		ClientType:   clientType,
		OAuth2Config: &flowConfig,
		PKCEVerifier: pkceVerifier,
	}
	h.sessionsMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	var authURL string
	if pkceVerifier != "" {
		authURL = flowConfig.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.S256ChallengeOption(pkceVerifier))
	} else {
		authURL = flowConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
	}

	slog.Info("Initiating OAuth2 authorization flow",
		"client_type", clientType,
		"client_id", agentID,
		"state", state[:8])

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// Callback handles the OAuth2 callback from the authorization server
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getSessionID(r)
	if sessionID == "" {
		http.Error(w, "Session not found", http.StatusBadRequest)
		return
	}

	// Get authorization code and state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errMsg := r.URL.Query().Get("error")

	if errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Error("OAuth2 authorization error",
			"error", errMsg,
			"error_description", errDesc)
		http.Error(w, fmt.Sprintf("Authorization error: %s", errMsg), http.StatusBadRequest)
		return
	}

	if code == "" {
		slog.Error("Missing authorization code in callback")
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Validate CSRF state
	h.sessionsMu.RLock()
	storedState := h.sessions[sessionID].CSRFState
	h.sessionsMu.RUnlock()

	if state != storedState {
		slog.Error("CSRF state mismatch in callback",
			"expected_state_length", len(storedState),
			"received_state_length", len(state))
		http.Error(w, "Invalid CSRF state", http.StatusBadRequest)
		return
	}

	slog.Info("Received authorization code and valid CSRF state",
		"code_length", len(code),
		"state_length", len(state))

	// Exchange code for token using the per-flow config stored at login time.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	h.sessionsMu.RLock()
	flowCfg := h.sessions[sessionID].OAuth2Config
	pkceVerifier := h.sessions[sessionID].PKCEVerifier
	h.sessionsMu.RUnlock()
	if flowCfg == nil {
		flowCfg = h.oauth2Config // fallback for sessions started before this change
	}

	var exchangeOpts []oauth2.AuthCodeOption
	if pkceVerifier != "" {
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(pkceVerifier))
	}

	token, err := flowCfg.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		slog.Error("Failed to exchange code for token",
			"error", err.Error())
		http.Error(w, fmt.Sprintf("Failed to exchange token: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("Successfully exchanged code for token",
		"token_type", token.TokenType,
		"token_expiry", token.Expiry.Format(time.RFC3339))

	// Extract 'sub' claim from access token
	// The golang oauth2 library stores the access token in the AccessToken field, not in Extra
	accessToken := token.AccessToken
	if accessToken == "" {
		slog.Error("Failed to extract access token from response")
		http.Error(w, "Failed to extract access token", http.StatusInternalServerError)
		return
	}

	// Decode JWT claims from access token (best-effort; opaque tokens get empty claims).
	sub := "N/A"
	var claims map[string]interface{}
	if c, err := extractTokenClaims(accessToken); err == nil {
		claims = c
		if s, ok := c["sub"].(string); ok {
			sub = s
		}
		slog.Info("Extracted JWT claims", "sub", sub)
	} else {
		slog.Info("Token is not JWT format (opaque token)", "error", err.Error())
	}

	// Store token and user info in session
	h.sessionsMu.Lock()
	if session, exists := h.sessions[sessionID]; exists {
		session.Token = token
		session.ExpiresAt = token.Expiry.Unix()
		session.UserInfo.Sub = sub
		session.UserInfo.Claims = claims
		slog.Info("Updated session in callback",
			"sub", session.UserInfo.Sub,
			"expiresAt", session.ExpiresAt)
	} else {
		slog.Error("Session not found in callback")
	}
	h.sessionsMu.Unlock()

	// Redirect to home
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout clears the session
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	sessionID := h.getSessionID(r)

	var session *Session
	h.sessionsMu.Lock()
	session = h.sessions[sessionID]
	delete(h.sessions, sessionID)
	h.sessionsMu.Unlock()
	if session != nil {
		session.closeMCPClient()
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// CallMCP handles MCP tool calls with transparent token exchange through agentgateway.
// It uses the mcp-go client library for full MCP protocol compliance.
func (h *Handlers) CallMCP(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "method_not_allowed",
		})
		return
	}

	// Extract session and validate token
	sessionID := h.getSessionID(r)
	if sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unauthorized",
			"error_description": "session not found",
		})
		return
	}

	h.sessionsMu.RLock()
	session, exists := h.sessions[sessionID]
	h.sessionsMu.RUnlock()

	if !exists || !session.Token.Valid() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unauthorized",
			"error_description": "session token invalid or expired",
		})
		return
	}

	// Get agentgateway URL from config
	gatewayURL := h.cfg.AgentGateway.MCPURL
	if gatewayURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "gateway_not_configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	session.MCPMu.Lock()
	defer session.MCPMu.Unlock()
	if session.mcpClosed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unauthorized",
			"error_description": "session not found",
		})
		return
	}
	mcpClient, err := h.mcpClient(ctx, session, gatewayURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "mcp_call_failed",
			"error_description": "failed to initialize MCP client",
		})
		slog.Error("MCP client initialization failed", "error", err.Error())
		return
	}

	// Step 2: call MCP tool — name from request body
	var body struct {
		Tool string `json:"tool"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	toolReq := mcp.CallToolRequest{}
	toolReq.Params.Name = body.Tool
	toolReq.Params.Arguments = toolArguments(body.Tool)

	toolResult, err := mcpClient.CallTool(ctx, toolReq)
	if approvalURL, ok := approvalElicitationURL(err); ok {
		writeApprovalRequired(w, approvalURL, gatewayURL)
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "mcp_call_failed",
			"error_description": "tools/call request failed",
		})
		slog.Error("MCP tools/call failed", "error", err.Error())
		return
	}

	// Extract result text and JWT claims from the tool response
	var toolResultText string
	var jwtClaims map[string]interface{}

	if toolResult != nil && len(toolResult.Content) > 0 {
		if tc, ok := mcp.AsTextContent(toolResult.Content[0]); ok {
			toolResultText = tc.Text
			slog.Info("MCP tool text content", "text", toolResultText)
			// Parse jwt_claims embedded in the text: "auth_scheme=Bearer\njwt_claims={...}"
			if idx := strings.Index(toolResultText, "\njwt_claims="); idx >= 0 {
				claimsJSON := toolResultText[idx+len("\njwt_claims="):]
				var claims map[string]interface{}
				if err := json.Unmarshal([]byte(claimsJSON), &claims); err == nil {
					jwtClaims = claims
					slog.Info("JWT claims parsed from MCP tool text",
						"claims_count", len(claims))
				}
			}
		}
	}

	slog.Info("MCP call successful",
		"tool", body.Tool,
		"has_jwt_claims", jwtClaims != nil)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"tool_result": toolResultText,
		"gateway_url": gatewayURL,
		"jwt_claims":  jwtClaims,
	})
}

func (h *Handlers) mcpClient(ctx context.Context, session *Session, gatewayURL string) (*mcpclient.Client, error) {
	if session.MCPClient != nil {
		return session.MCPClient, nil
	}

	accessToken := session.Token.AccessToken
	client, err := mcpclient.NewStreamableHttpClient(gatewayURL,
		transport.WithHTTPHeaderFunc(func(_ context.Context) map[string]string {
			return map[string]string{"Authorization": fmt.Sprintf("Bearer %s", accessToken)}
		}),
		transport.WithHTTPTimeout(15*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create MCP client: %w", err)
	}
	if err := client.Start(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("start MCP client: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "sample-agent", Version: "1.0"}
	if _, err := client.Initialize(ctx, initReq); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("initialize MCP client: %w", err)
	}

	session.MCPClient = client
	return client, nil
}

func (s *Session) closeMCPClient() {
	s.MCPMu.Lock()
	defer s.MCPMu.Unlock()
	s.mcpClosed = true
	if s.MCPClient == nil {
		return
	}
	_ = s.MCPClient.Close()
	s.MCPClient = nil
}

func approvalElicitationURL(err error) (string, bool) {
	var elicitation mcp.URLElicitationRequiredError
	if !errors.As(err, &elicitation) || len(elicitation.Elicitations) != 1 || elicitation.Elicitations[0].URL == "" {
		return "", false
	}
	return elicitation.Elicitations[0].URL, true
}

func writeApprovalRequired(w http.ResponseWriter, approvalURL, gatewayURL string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":             "approval_required",
		"error_description": "Approve this tool call to continue.",
		"approval_url":      approvalURL,
		"gateway_url":       gatewayURL,
	})
}

func toolArguments(tool string) map[string]any {
	if tool != "create_issue" {
		return nil
	}
	return map[string]any{
		"repository": "acme/sample-agent",
		"title":      "Review Compose approval",
	}
}

// getSessionID retrieves the session ID from cookies
func (h *Handlers) getSessionID(r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// extractTokenHeader decodes the header of a JWT access token without verification.
func extractTokenHeader(accessToken string) (map[string]interface{}, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid token format")
	}
	header := parts[0]
	switch len(header) % 4 {
	case 1:
		header += "==="
	case 2:
		header += "=="
	case 3:
		header += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(header)
	if err != nil {
		return nil, fmt.Errorf("failed to decode token header: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse token header: %w", err)
	}
	return claims, nil
}

// extractTokenClaims decodes the payload of a JWT access token without verification.
func extractTokenClaims(accessToken string) (map[string]interface{}, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid token format")
	}
	payload := parts[1]
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
		return nil, fmt.Errorf("failed to decode token payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}
	return claims, nil
}

// generateRandomString generates a cryptographically secure random string of given length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		// This should never happen in practice, but if it does, panic to avoid using weak randomness
		panic(fmt.Sprintf("failed to read random bytes: %v", err))
	}
	for i := range b {
		b[i] = charset[randomBytes[i]%byte(len(charset))]
	}
	return string(b)
}

// renderSelectorPage renders the client-type selector home page.
func renderSelectorPage(cfg *config.Config) string {
	proxyAvail := cfg.OAuth2.ProxyAgentID != ""
	localAvail := cfg.OAuth2.LocalAgentID != ""
	cimdAvail := cfg.OAuth2.CIMDClientURI != ""

	btnProxy := `<a href="/login?type=proxy" class="btn btn-proxy">Login as Proxy Client<span class="badge">forwards to upstream OAuth2</span></a>`
	if !proxyAvail {
		btnProxy = `<span class="btn btn-disabled">Proxy Client<span class="badge">not seeded yet — reload in a moment</span></span>`
	}
	btnLocal := `<a href="/login?type=local" class="btn btn-local">Login as Local Client<span class="badge">broker issues JWT locally</span></a>`
	if !localAvail {
		btnLocal = `<span class="btn btn-disabled">Local Client<span class="badge">not seeded yet — reload in a moment</span></span>`
	}
	btnCIMD := `<a href="/login?type=cimd" class="btn btn-cimd">Login as CIMD Client<span class="badge">resolved via metadata doc (placeholder URL)</span></a>`
	if !cimdAvail {
		btnCIMD = `<span class="btn btn-disabled">CIMD Client<span class="badge">not seeded yet — reload in a moment</span></span>`
	}

	return `<!DOCTYPE html>
<html>
<head>
    <title>Sample OAuth2 Client</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            max-width: 640px;
            margin: 80px auto;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
        }
        .container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 8px 16px rgba(0,0,0,0.2); }
        h1 { color: #333; margin-bottom: 4px; }
        .subtitle { color: #666; margin-bottom: 32px; }
        .btn {
            display: flex; flex-direction: column; align-items: flex-start;
            width: 100%%; padding: 14px 20px; margin-bottom: 12px;
            border-radius: 6px; font-size: 16px; font-weight: 600;
            text-decoration: none; color: white; cursor: pointer;
            transition: opacity 0.15s; box-sizing: border-box;
        }
        .btn:hover { opacity: 0.88; }
        .btn-proxy  { background: #667eea; }
        .btn-local  { background: #27ae60; }
        .btn-cimd   { background: #e67e22; }
        .btn-disabled { background: #bdc3c7; cursor: default; }
        .btn-disabled:hover { opacity: 1; }
        .badge { font-size: 12px; font-weight: 400; opacity: 0.85; margin-top: 3px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Sample OAuth2 Client</h1>
        <p class="subtitle">Select a client type to start an authorization flow through the identity broker</p>
        ` + btnProxy + `
        ` + btnLocal + `
        ` + btnCIMD + `
    </div>
</body>
</html>`
}

// renderUserPage renders the post-login user info page.
func renderUserPage(userInfo *UserInfo, expiresAt int64, clientType, rawToken, keyID string) string {
	var flowLabel string
	switch clientType {
	case "local":
		flowLabel = "Local Client — token issued directly by broker (no upstream)"
	case "cimd":
		flowLabel = "CIMD Client — agent resolved via Client ID Metadata Document"
	default:
		flowLabel = "Proxy Client — token forwarded from upstream OAuth2 server"
	}

	// Build claims rows: well-known claims first, then remaining alphabetically.
	wellKnown := []string{"iss", "sub", "aud", "exp", "iat", "jti", "azp", "scope"}
	seen := map[string]bool{}
	var claimRows strings.Builder
	if keyID != "" {
		claimRows.WriteString(fmt.Sprintf(
			`<tr><td class="ck">kid <span class="hdr">(header)</span></td><td class="cv">%s</td></tr>`,
			keyID))
	}
	formatVal := func(v interface{}) string {
		switch t := v.(type) {
		case float64:
			if t > 1e9 {
				return fmt.Sprintf("%s (%g)", time.Unix(int64(t), 0).UTC().Format(time.RFC3339), t)
			}
			return fmt.Sprintf("%g", t)
		case []interface{}:
			b, _ := json.Marshal(t)
			return string(b)
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	if userInfo.Claims != nil {
		for _, k := range wellKnown {
			if v, ok := userInfo.Claims[k]; ok {
				seen[k] = true
				claimRows.WriteString(fmt.Sprintf(
					`<tr><td class="ck">%s</td><td class="cv">%s</td></tr>`,
					k, formatVal(v)))
			}
		}
		var extra []string
		for k := range userInfo.Claims {
			if !seen[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
		for _, k := range extra {
			claimRows.WriteString(fmt.Sprintf(
				`<tr><td class="ck">%s</td><td class="cv">%s</td></tr>`,
				k, formatVal(userInfo.Claims[k])))
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>User Info - Sample OAuth2 Client</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 680px; margin: 60px auto; padding: 20px; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); min-height: 100vh; }
        .container { background: white; padding: 36px; border-radius: 8px; box-shadow: 0 8px 16px rgba(0,0,0,0.2); }
        h1 { color: #333; margin-bottom: 10px; }
        .flow-badge { background: #eef0ff; border-left: 4px solid #667eea; padding: 10px 16px; border-radius: 4px; margin-bottom: 20px; color: #3d4db7; font-size: 14px; }
        h2 { font-size: 15px; color: #555; margin: 20px 0 8px; }
        table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; font-size: 13px; }
        td { padding: 7px 10px; border-bottom: 1px solid #eee; vertical-align: top; }
        .ck { color: #667eea; font-weight: 600; white-space: nowrap; width: 1%%; }
        .cv { word-break: break-all; font-family: monospace; }
        details { margin-bottom: 20px; }
        summary { cursor: pointer; font-size: 13px; color: #667eea; font-weight: 600; margin-bottom: 6px; }
        .raw { font-family: monospace; font-size: 12px; background: #f7f7f7; padding: 10px; border-radius: 4px; word-break: break-all; border: 1px solid #e0e0e0; }
        .hdr { font-size: 11px; color: #999; font-weight: 400; }
        .buttons { display: flex; gap: 10px; margin-top: 16px; }
        a, button { display: inline-block; padding: 10px 20px; text-decoration: none; border-radius: 4px; font-weight: bold; border: none; cursor: pointer; transition: background-color 0.2s; }
        .btn-home { background: #667eea; color: white; flex: 1; }
        .btn-home:hover { background: #764ba2; }
        .btn-mcp { background: #27ae60; color: white; width: 100%%; margin-bottom: 10px; font-size: 14px; }
        .btn-mcp:hover { background: #219a52; }
        .btn-mcp:disabled { background: #95a5a6; cursor: not-allowed; }
        .btn-mcp-deny { background: #c0392b; }
        .btn-mcp-deny:hover { background: #a93226; }
        .mcp-result { margin-top: 15px; padding: 15px; border-radius: 4px; display: none; }
        .mcp-result.success { background: #d4edda; border: 1px solid #c3e6cb; color: #155724; }
        .mcp-result.error   { background: #f8d7da; border: 1px solid #f5c6cb; color: #721c24; }
        .mcp-result pre { margin: 8px 0 0 0; font-size: 13px; white-space: pre-wrap; word-break: break-all; }
    </style>
</head>
<body>
    <div class="container">
        <h1>&#x2713; OAuth2 Flow Successful</h1>
        <div class="flow-badge">%s</div>
        <h2>Access Token Claims</h2>
        <table><tbody>%s</tbody></table>
        <details>
            <summary>Raw access token</summary>
            <div class="raw">%s</div>
        </details>
        <button id="mcp-btn" class="btn-mcp" onclick="callMCPTool('whoami')">Call MCP Tool: whoami (allowed)</button>
        <button id="mcp-approval-btn" class="btn-mcp" onclick="callMCPTool('create_issue')">Call MCP Tool: create_issue (approval required)</button>
        <button id="mcp-deny-btn" class="btn-mcp btn-mcp-deny" onclick="callMCPTool('delete_repository')">Call MCP Tool: delete_repository (denied)</button>
        <div id="mcp-result" class="mcp-result"></div>
        <div class="buttons">
            <a href="/logout" class="btn-home">Switch Client</a>
        </div>
    </div>
    <script>
    function callMCPTool(tool) {
        var labels = {
            whoami: 'Call MCP Tool: whoami (allowed)',
            create_issue: 'Call MCP Tool: create_issue (approval required)',
            delete_repository: 'Call MCP Tool: delete_repository (denied)'
        };
        var btn = document.getElementById(tool === 'whoami' ? 'mcp-btn' : tool === 'create_issue' ? 'mcp-approval-btn' : 'mcp-deny-btn');
        var result = document.getElementById('mcp-result');
        btn.disabled = true; btn.textContent = 'Calling...';
        result.style.display = 'none'; result.className = 'mcp-result';
        fetch('/call-mcp', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({tool: tool})})
            .then(function(r) { return r.json(); })
            .then(function(d) {
                result.style.display = 'block';
                if (d.success) {
                    result.classList.add('success');
                    var html = '<strong>Token Exchange Successful!</strong><pre>Tool: ' + esc(tool) + '\nResult: ' + esc(d.tool_result) + '\nGateway: ' + esc(d.gateway_url);
                    if (d.jwt_claims && Object.keys(d.jwt_claims).length > 0) { html += '\n\nJWT Claims:\n' + formatJSON(d.jwt_claims); }
                    html += '</pre>';
                    result.innerHTML = html;
                } else if (d.approval_url) {
                    result.classList.add('error');
                    result.innerHTML = '<strong>Approval Required</strong><pre>' + esc(d.error_description) + '</pre><a class="btn-mcp" href="' + esc(d.approval_url) + '">Review and approve tool call</a>';
                } else {
                    result.classList.add('error');
                    result.innerHTML = '<strong>MCP Call Failed</strong><pre>' + esc(d.error_description || d.error) + '\nGateway: ' + esc(d.gateway_url) + '</pre>';
                }
            })
            .catch(function(e) { result.style.display='block'; result.classList.add('error'); result.innerHTML='<strong>Request Failed</strong><pre>'+esc(String(e))+'</pre>'; })
            .finally(function() { btn.disabled=false; btn.textContent=labels[tool]; });
    }
    function formatJSON(obj) { return esc(JSON.stringify(obj, null, 2)); }
    function esc(s) { return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }
    </script>
</body>
</html>`, flowLabel, claimRows.String(), rawToken)
}
