package enduser

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// OAuth2AuthorizeHandler handles OAuth2 authorization endpoint requests.
// The ProceedHandler strategy determines the response for an active grant: proxy mode
// redirects to the upstream OAuth2 server, local mode issues a local authorization code.
type OAuth2AuthorizeHandler struct {
	Service        ports.OAuth2Service
	Logger         *slog.Logger
	ProceedHandler AuthorizationProceedStrategy
}

// ServeHTTP implements http.Handler for the authorization endpoint.
// The shared flow handles consent checks and error decisions; the ProceedHandler
// strategy determines the response when the user has an active grant.
func (h *OAuth2AuthorizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		if h.Logger != nil {
			h.Logger.ErrorContext(r.Context(), "OAuth2 authorization handler invoked with nil Service — check builder wiring")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"error":"server_error","error_description":"OAuth2 authorization server not configured"}`)
		return
	}
	principalValue := principal.MustFromContext(r.Context())

	query := r.URL.Query()

	rawClientID := query.Get("client_id")
	if rawClientID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":"invalid_request","error_description":"missing required parameter: client_id"}`)
		return
	}

	redirectURI := query.Get("redirect_uri")
	if redirectURI == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":"invalid_request","error_description":"missing required parameter: redirect_uri"}`)
		return
	}

	responseType := query.Get("response_type")
	if responseType == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":"invalid_request","error_description":"missing required parameter: response_type"}`)
		return
	}

	authReq := &ports.AuthorizationRequest{
		ClientID:            id.ClientID(rawClientID),
		RedirectURI:         redirectURI,
		ResponseType:        responseType,
		Scope:               query.Get("scope"),
		State:               query.Get("state"),
		CodeChallenge:       query.Get("code_challenge"),
		CodeChallengeMethod: query.Get("code_challenge_method"),
		OriginalURL:         r.URL.String(),
	}

	decision, err := h.Service.HandleAuthorization(r.Context(), authReq, id.NewPrincipal(principalValue))
	if err != nil {
		if h.Logger != nil {
			h.Logger.ErrorContext(r.Context(), "authorization_request_failed", "error", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `{"error":"server_error"}`)
		return
	}

	switch decision.Action {
	case "proceed":
		if h.ProceedHandler == nil {
			redirectWithError(w, r, authReq.RedirectURI, authReq.State, "server_error", "OAuth2 authorization server not configured")
			return
		}
		h.ProceedHandler.HandleProceed(w, r, decision, authReq, id.NewPrincipal(principalValue))

	case "redirect_to_consent":
		http.Redirect(w, r, decision.RedirectURL, http.StatusFound)

	case "error":
		respondWithDecisionError(w, r, decision)

	default:
		if h.Logger != nil {
			h.Logger.Error("unexpected authorization decision action", "action", decision.Action)
		}
		redirectWithError(w, r, authReq.RedirectURI, authReq.State, "server_error", "unexpected authorization decision")
	}
}

// respondWithDecisionError writes an OAuth2 error response for an "error" decision.
// Redirects with error params when a redirect_uri is available; otherwise returns JSON.
func respondWithDecisionError(w http.ResponseWriter, r *http.Request, decision *ports.AuthorizationDecision) {
	if decision.RedirectURL != "" {
		http.Redirect(w, r, decision.RedirectURL, http.StatusFound)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(errorDecisionStatus(decision.ErrorCode))
		_, _ = fmt.Fprintf(w, `{"error":%q,"error_description":%q}`, decision.ErrorCode, decision.ErrorDesc)
	}
}

// errorDecisionStatus maps an OAuth2 error code to an HTTP status for direct (non-redirect)
// error responses. Infrastructure errors map to 500; all others map to 400.
func errorDecisionStatus(errorCode string) int {
	if errorCode == "server_error" {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

// redirectWithError performs an OAuth2 error redirect per RFC 6749 Section 4.1.2.1.
func redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, state, errorCode, errorDescription string) {
	target, err := oauth2.BuildErrorRedirectURL(redirectURI, state, errorCode, errorDescription)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, target, http.StatusFound) // #nosec G710 -- authorization service validates redirect_uri against registered client URIs.
}
