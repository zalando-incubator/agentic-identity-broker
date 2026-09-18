package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/mock-upstream-oauth2-service/internal/config"
)

// Health is a simple health check handler
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status": "ok", "service": "upstream-oauth2-server"}`)); err != nil {
		return
	}
}

// NewUserAuthorizationHandler creates a user authorization handler for the OAuth2 server
// This is called by SetUserAuthorizationHandler to handle the consent flow
func NewUserAuthorizationHandler(cfg *config.Config) func(w http.ResponseWriter, r *http.Request) (string, error) {
	return func(w http.ResponseWriter, r *http.Request) (string, error) {
		// Handle form submission (approval/denial)
		if r.Method == http.MethodPost {
			return handleUserAuthorizationPost(w, r, cfg)
		}

		// GET request - show consent page
		return handleUserAuthorizationGet(w, r, cfg)
	}
}

// handleUserAuthorizationGet displays the consent page
func handleUserAuthorizationGet(w http.ResponseWriter, r *http.Request, cfg *config.Config) (string, error) {
	// Parse query parameters from the authorization request
	clientID := r.URL.Query().Get("client_id")
	scope := r.URL.Query().Get("scope")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")
	responseType := r.URL.Query().Get("response_type")

	// Log incoming authorization request with full PKCE details
	slog.Info("Received authorization request",
		"client_id", clientID,
		"response_type", responseType,
		"redirect_uri_full", redirectURI,
		"scope", scope,
		"state_prefix", state[:min(len(state), 20)],
		"code_challenge", codeChallenge,
		"code_challenge_length", len(codeChallenge),
		"code_challenge_method", codeChallengeMethod,
	)

	// Validate required parameters
	if clientID == "" || responseType == "" || redirectURI == "" {
		const errorMsg = "missing required OAuth2 authorization parameters"
		slog.Error(errorMsg)
		http.Error(w, errorMsg, http.StatusBadRequest)
		return "", fmt.Errorf("invalid_request: %s", errorMsg)
	}

	// Render consent page
	consentHTML := renderConsentPage(clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod, cfg.OAuth2.Scopes)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(consentHTML)); err != nil {
		return "", fmt.Errorf("write consent page: %w", err)
	}

	// Return empty string and nil to indicate we're handling the response via HTTP
	return "", nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleUserAuthorizationPost processes the consent form submission
func handleUserAuthorizationPost(w http.ResponseWriter, r *http.Request, cfg *config.Config) (string, error) {
	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		return "", fmt.Errorf("failed to parse form: %w", err)
	}

	approval := r.FormValue("approval")
	clientID := r.FormValue("client_id")

	slog.Info("Processing consent submission",
		"approval", approval,
		"client_id", clientID,
	)

	// Handle denial - return error to let OAuth2 server generate error response
	if approval != "approve" {
		slog.Info("User denied authorization", "client_id", clientID)
		return "", fmt.Errorf("access_denied")
	}

	// Handle approval - return the user ID
	// The OAuth2 server will use this to generate the authorization code
	slog.Info("User approved authorization", "client_id", clientID, "user_id", cfg.User.Sub)
	return cfg.User.Sub, nil
}

// renderConsentPage renders the consent page HTML with distinctive styling for upstream OAuth2 server
// Uses teal/cyan background to clearly differentiate from third-party service mock (blue)
func renderConsentPage(clientID, redirectURI, scope, state, codeChallenge, codeChallengeMethod string, scopes []config.ScopeConfig) string {
	// Parse requested scopes
	scopeDescriptions := ""
	if scope != "" {
		// Parse space-separated scopes and build descriptions
		scopesHTML := `<ul>`
		for _, s := range scopes {
			scopesHTML += fmt.Sprintf(`<li><strong>%s</strong>: %s</li>`, s.Name, s.Description)
		}
		scopesHTML += `</ul>`
		scopeDescriptions = scopesHTML
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Authorization Request - Upstream OAuth2 Server</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            max-width: 600px;
            margin: 50px auto;
            padding: 20px;
            background: linear-gradient(135deg, #00d4aa 0%%, #00bfa5 100%%);
            min-height: 100vh;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 8px 16px rgba(0,0,0,0.2);
        }
        .header {
            display: flex;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 20px;
            border-bottom: 3px solid #00d4aa;
        }
        .header-badge {
            display: inline-block;
            background-color: #00d4aa;
            color: white;
            padding: 6px 12px;
            border-radius: 20px;
            font-size: 12px;
            font-weight: bold;
            margin-right: 10px;
            letter-spacing: 0.5px;
        }
        h1 {
            color: #00b591;
            margin: 0;
            font-size: 24px;
        }
        .subtitle {
            color: #666;
            font-size: 14px;
            margin-top: 5px;
        }
        .info {
            background: linear-gradient(135deg, #e0f7f4 0%%, #b2ebf2 100%%);
            padding: 15px;
            border-radius: 4px;
            margin-bottom: 20px;
            border-left: 4px solid #00d4aa;
        }
        .info p {
            margin: 8px 0;
            color: #333;
        }
        .info code {
            background-color: rgba(0,212,170,0.1);
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Courier New', monospace;
            color: #00695c;
        }
        .scopes {
            margin: 20px 0;
        }
        .scopes-title {
            color: #333;
            font-weight: bold;
            margin-bottom: 10px;
        }
        .scopes ul {
            list-style-type: none;
            padding: 0;
        }
        .scopes li {
            padding: 8px 0;
            border-bottom: 1px solid #e0e0e0;
            color: #555;
        }
        .scopes li:last-child {
            border-bottom: none;
        }
        .scopes li strong {
            color: #00b591;
        }
        .buttons {
            display: flex;
            gap: 10px;
            margin-top: 30px;
        }
        button {
            flex: 1;
            padding: 12px 20px;
            font-size: 14px;
            font-weight: bold;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            transition: all 0.2s;
        }
        .approve {
            background-color: #00d4aa;
            color: white;
        }
        .approve:hover {
            background-color: #00b591;
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(0,212,170,0.3);
        }
        .deny {
            background-color: #e0e0e0;
            color: #333;
        }
        .deny:hover {
            background-color: #d0d0d0;
            transform: translateY(-2px);
        }
        .params {
            font-size: 12px;
            color: #999;
            margin-top: 20px;
            padding-top: 20px;
            border-top: 1px solid #eee;
        }
        .params code {
            background-color: #f5f5f5;
            padding: 2px 4px;
            border-radius: 2px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <span class="header-badge">🌐 UPSTREAM OAUTH2</span>
            <div>
                <h1>Authorization Request</h1>
                <p class="subtitle">Upstream OAuth2 Server</p>
            </div>
        </div>

        <div class="info">
            <p><strong>Application:</strong> %s</p>
            <p><strong>Redirect URI:</strong> <code>%s</code></p>
        </div>

        <div class="scopes">
            <p class="scopes-title">This application is requesting access to:</p>
            %s
        </div>

        <form method="POST" action="">
            <input type="hidden" name="client_id" value="%s">
            <input type="hidden" name="redirect_uri" value="%s">
            <input type="hidden" name="scope" value="%s">
            <input type="hidden" name="state" value="%s">
            <input type="hidden" name="code_challenge" value="%s">
            <input type="hidden" name="code_challenge_method" value="%s">

            <div class="buttons">
                <button type="submit" name="approval" value="approve" class="approve">✓ Approve</button>
                <button type="submit" name="approval" value="deny" class="deny">✗ Deny</button>
            </div>
        </form>

        <div class="params">
            <p><strong>Developer Info:</strong></p>
            <p>Client ID: <code>%s</code><br>
            Code Challenge: <code>%s</code><br>
            State: <code>%s</code></p>
        </div>
    </div>
</body>
</html>
`,
		clientID,
		redirectURI,
		scopeDescriptions,
		clientID,
		redirectURI,
		scope,
		state,
		codeChallenge,
		codeChallengeMethod,
		clientID,
		codeChallenge,
		state,
	)

	return html
}
