package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/mock-oauth2-service/internal/config"
)

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

// renderConsentPage renders the consent page HTML
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
    <title>Authorization Request</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            max-width: 600px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
            margin-top: 0;
        }
        .info {
            background-color: #f9f9f9;
            padding: 15px;
            border-radius: 4px;
            margin-bottom: 20px;
            border-left: 4px solid #007bff;
        }
        .scopes {
            margin: 20px 0;
        }
        .scopes ul {
            list-style-type: none;
            padding: 0;
        }
        .scopes li {
            padding: 8px 0;
            border-bottom: 1px solid #eee;
        }
        .scopes li:last-child {
            border-bottom: none;
        }
        .buttons {
            display: flex;
            gap: 10px;
            margin-top: 30px;
        }
        button {
            flex: 1;
            padding: 10px 20px;
            font-size: 14px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            transition: background-color 0.2s;
        }
        .approve {
            background-color: #28a745;
            color: white;
        }
        .approve:hover {
            background-color: #218838;
        }
        .deny {
            background-color: #dc3545;
            color: white;
        }
        .deny:hover {
            background-color: #c82333;
        }
        .params {
            font-size: 12px;
            color: #666;
            margin-top: 20px;
            padding-top: 20px;
            border-top: 1px solid #eee;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Authorization Request</h1>
        <div class="info">
            <p><strong>Application:</strong> %s</p>
            <p><strong>Redirect URI:</strong> <code>%s</code></p>
        </div>

        <div class="scopes">
            <p><strong>This application is requesting access to:</strong></p>
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
                <button type="submit" name="approval" value="approve" class="approve">Approve</button>
                <button type="submit" name="approval" value="deny" class="deny">Deny</button>
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
