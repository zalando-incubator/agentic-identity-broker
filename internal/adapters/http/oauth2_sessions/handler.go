package oauth2_sessions

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

// Handler handles HTTP requests for OAuth2 sessions.
type Handler struct {
	service *oauth2session.OAuth2SessionService
	logger  *slog.Logger
}

// NewHandler creates a new handler.
func NewHandler(service *oauth2session.OAuth2SessionService) *Handler {
	return &Handler{
		service: service,
		logger:  slog.Default(),
	}
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}

// ListSessions handles GET /api/third-party/sessions
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract principal from context (set by middleware)
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "principal not found in context",
		})
		return
	}

	// Fetch sessions from service
	summaries, err := h.service.ListUserSessions(ctx, id.Principal(principalValue))
	if err != nil {
		h.logger.Error("failed to list sessions", "principal", principalValue, "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "internal_error",
			"message": "failed to list sessions",
		})
		return
	}

	h.logger.Info("listed sessions", "principal", principalValue, "count", len(summaries))

	// Return response
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"sessions": summaries,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// InitiateFlow handles GET and POST /api/third-party/{serviceId}/oauth2/authorize.
// It initiates an OAuth2 authorization code flow with PKCE.
func (h *Handler) InitiateFlow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context in authorize request")
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "principal not found in context")
		return
	}

	serviceIDStr := chi.URLParam(r, "serviceId")
	safeServiceIDStr := strings.ReplaceAll(serviceIDStr, "\n", "")
	safeServiceIDStr = strings.ReplaceAll(safeServiceIDStr, "\r", "")
	if serviceIDStr == "" {
		h.logger.Warn("missing serviceId in URL path")
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "serviceId parameter required in path")
		return
	}

	parsedServiceID, err := id.ParseServiceID(serviceIDStr)
	if err != nil {
		h.logger.Warn("invalid serviceId format in URL path", "service_id", safeServiceIDStr)
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "serviceId must be a valid UUID")
		return
	}

	var redirectURI, consentStateID string
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			h.logger.Warn("invalid OAuth2 flow request", "service_id", safeServiceIDStr, "error", "invalid authorize request form")
			writeJSONError(w, http.StatusBadRequest, "invalid_request", "invalid authorize request form")
			return
		}
		consentStateID = r.PostForm.Get("consent_state_id")
		if _, err := uuid.Parse(consentStateID); err != nil {
			h.logger.Warn("invalid OAuth2 flow request", "service_id", safeServiceIDStr, "error", "consent_state_id must be a UUID")
			writeJSONError(w, http.StatusBadRequest, "invalid_request", "consent_state_id must be a UUID")
			return
		}
		redirectURI = r.PostForm.Get("redirect_uri")
	} else {
		redirectURI = r.URL.Query().Get("redirect_uri")
	}
	if redirectURI == "" {
		h.logger.Warn("missing redirect_uri", "service_id", safeServiceIDStr)
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is required")
		return
	}

	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		h.logger.Warn("invalid redirect_uri", "redirect_uri", redirectURI, "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri is not a valid URL")
		return
	}
	callbackURL, err := url.Parse(h.service.GetCallbackBaseURL())
	if err != nil {
		h.logger.Error("failed to parse callback base URL", "callback_url", h.service.GetCallbackBaseURL(), "error", err)
		writeJSONError(w, http.StatusInternalServerError, "server_error", "failed to validate redirect_uri")
		return
	}
	if redirectURL.Host != callbackURL.Host || redirectURL.Scheme != callbackURL.Scheme {
		h.logger.Warn("redirect_uri origin mismatch",
			"redirect_host", redirectURL.Host,
			"configured_host", callbackURL.Host,
			"service_id", serviceIDStr)
		writeJSONError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri must be on the same origin as the configured public URL")
		return
	}
	redirectQuery := redirectURL.Query()
	redirectQuery.Del("consent_state")
	redirectQuery.Del("consent_state_id")
	redirectURL.RawQuery = redirectQuery.Encode()
	redirectURI = redirectURL.String()

	var result *oauth2session.InitiateFlowResult
	if consentStateID != "" {
		result, err = h.service.InitiateOAuth2FlowWithConsentState(
			ctx,
			id.Principal(principalValue),
			parsedServiceID,
			redirectURI,
			consentStateID,
		)
	} else {
		result, err = h.service.InitiateOAuth2Flow(ctx, id.Principal(principalValue), parsedServiceID, redirectURI)
	}
	if err != nil {
		if errors.Is(err, oauth2session.ErrStateTokenTooLarge) {
			writeJSONError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri produces an oversized OAuth2 state token")
			return
		}

		if errors.Is(err, oauth2session.ErrServiceNotFound) {
			h.logger.Warn("service not found", "service_id", serviceIDStr)
			writeJSONError(w, http.StatusNotFound, "service_not_found", "third-party service not found")
			return
		}

		h.logger.Error("failed to initiate OAuth2 flow",
			"principal", principalValue,
			"service_id", serviceIDStr,
			"error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to initiate OAuth2 flow")
		return
	}

	h.logger.Info("OAuth2 flow initiated",
		"principal", principalValue,
		"service_id", serviceIDStr,
		"auth_url_domain", extractDomain(result.AuthorizationURL))
	http.Redirect(w, r, result.AuthorizationURL, http.StatusFound) // #nosec G710 -- domain service builds this URL from the stored provider endpoint.
}

// extractDomain extracts domain from URL for logging
func extractDomain(urlStr string) string {
	if u, err := url.Parse(urlStr); err == nil {
		return u.Host
	}
	return "unknown"
}

// HandleCallback handles GET /api/third-party/{serviceId}/oauth2/callback
// Processes OAuth2 callback, exchanges code for tokens, stores encrypted session
func (h *Handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract principal from context (set by middleware)
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context in callback")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "principal not found in context",
		})
		return
	}

	// Extract serviceId from URL path parameter
	serviceIDStr := chi.URLParam(r, "serviceId")
	if serviceIDStr == "" {
		h.logger.Warn("missing serviceId in callback URL path")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request",
			"message": "serviceId parameter required in path",
		})
		return
	}

	parsedServiceID, err := id.ParseServiceID(serviceIDStr)
	if err != nil {
		h.logger.Warn("invalid serviceId format in callback URL path", "service_id", serviceIDStr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request",
			"message": "serviceId must be a valid UUID",
		})
		return
	}

	// Extract callback parameters from query
	query := r.URL.Query()
	callbackReq := &oauth2session.HandleCallbackRequest{
		ServiceID: parsedServiceID,
		Code:      query.Get("code"),
		State:     query.Get("state"),
		Error:     query.Get("error"),
		ErrorDesc: query.Get("error_description"),
	}

	// T060: Handle OAuth2 error responses
	if callbackReq.Error != "" {
		h.logger.Warn("OAuth2 error in callback",
			"service_id", serviceIDStr,
			"error", callbackReq.Error,
			"error_description", callbackReq.ErrorDesc)

		// Redirect to sessions page with error in query
		redirectURL := "/sessions?error=" + url.QueryEscape(callbackReq.Error)
		if callbackReq.ErrorDesc != "" {
			redirectURL += "&error_description=" + url.QueryEscape(callbackReq.ErrorDesc)
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Validate that code and state are present
	if callbackReq.Code == "" || callbackReq.State == "" {
		h.logger.Warn("missing code or state in callback",
			"service_id", serviceIDStr,
			"has_code", callbackReq.Code != "",
			"has_state", callbackReq.State != "")

		redirectURL := "/sessions?error=invalid_callback&error_description=" +
			url.QueryEscape("Missing required OAuth2 parameters")
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Process the callback via service
	result, err := h.service.HandleCallback(ctx, id.Principal(principalValue), callbackReq)
	if err != nil {
		// Check for specific error types
		if errors.Is(err, oauth2session.ErrPrincipalMismatch) {
			h.logger.Error("principal mismatch in OAuth2 callback",
				"service_id", serviceIDStr,
				"error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "forbidden",
				"message": "principal mismatch",
			})
			return
		}

		// Check for service_id mismatch (cross-service attack attempt)
		if errors.Is(err, oauth2session.ErrServiceIDMismatch) {
			h.logger.Error("service_id mismatch in OAuth2 callback - possible cross-service attack",
				"service_id", serviceIDStr,
				"error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "bad_request",
				"message": "service_id mismatch",
			})
			return
		}

		// Check for state token validation errors
		if errors.Is(err, oauth2session.ErrStateTokenExpired) {
			h.logger.Warn("state token expired in callback",
				"service_id", serviceIDStr,
				"error", err)

			redirectURL := "/sessions?error=expired_token&error_description=" +
				url.QueryEscape("OAuth2 state token expired - please try again")
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		if errors.Is(err, oauth2session.ErrInvalidStateToken) {
			h.logger.Warn("state token validation failed in callback",
				"service_id", serviceIDStr,
				"error", err)

			redirectURL := "/sessions?error=invalid_state&error_description=" +
				url.QueryEscape("OAuth2 state token invalid - please try again")
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		// Generic callback error - log and redirect
		h.logger.Error("failed to handle OAuth2 callback",
			"principal", principalValue,
			"service_id", serviceIDStr,
			"error", err)

		redirectURL := "/sessions?error=callback_failed&error_description=" +
			url.QueryEscape("Authorization failed - please try again")
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	h.logger.Info("OAuth2 callback successful",
		"principal", principalValue,
		"service_id", serviceIDStr,
		"session_id", result.Session.ID)

	// Redirect to the original page that initiated the OAuth2 flow
	redirectURIStr := result.RedirectURI
	if redirectURIStr == "" {
		// Fallback to sessions page if redirectURI is not set (shouldn't happen)
		redirectURIStr = "/sessions"
	}

	// Parse redirect URI and add success parameters securely
	redirectURL, err := url.Parse(redirectURIStr)
	if err != nil {
		// If parsing fails, fall back to sessions page
		h.logger.Error("failed to parse redirect URI",
			"redirect_uri", redirectURIStr,
			"error", err)
		http.Redirect(w, r, "/sessions", http.StatusFound)
		return
	}

	// Add success parameters to query
	redirectQuery := redirectURL.Query()
	redirectQuery.Set("success", "true")
	redirectQuery.Set("service_id", serviceIDStr)
	if result.ConsentStateID != "" {
		redirectQuery.Set("consent_state_id", result.ConsentStateID)
	}
	redirectURL.RawQuery = redirectQuery.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// GetSessionDetails handles GET /api/third-party/{serviceId}/session
// Returns session details including list of dependent agents.
func (h *Handler) GetSessionDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract principal from context (set by middleware)
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context in get session details request")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "principal not found in context",
		})
		return
	}

	// Extract serviceId from URL path parameter
	serviceIDStr := chi.URLParam(r, "serviceId")
	if serviceIDStr == "" {
		h.logger.Warn("missing serviceId in URL path")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request",
			"message": "serviceId parameter required in path",
		})
		return
	}

	parsedServiceID, err := id.ParseServiceID(serviceIDStr)
	if err != nil {
		h.logger.Warn("invalid serviceId format in URL path", "service_id", serviceIDStr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request",
			"message": "serviceId must be a valid UUID",
		})
		return
	}

	// Fetch session details via service
	sessionWithAgents, err := h.service.GetSessionWithAgents(ctx, id.Principal(principalValue), parsedServiceID)
	if err != nil {
		// Check for specific error types
		if errors.Is(err, oauth2session.ErrSessionNotFound) {
			h.logger.Warn("session not found", "service_id", serviceIDStr, "principal", principalValue)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "not_found",
				"message": "session not found",
			})
			return
		}

		if errors.Is(err, oauth2session.ErrUnauthorized) {
			h.logger.Error("unauthorized access to session",
				"service_id", serviceIDStr,
				"principal", principalValue,
				"error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "forbidden",
				"message": "you do not have access to this session",
			})
			return
		}

		// Generic error
		h.logger.Error("failed to get session details",
			"principal", principalValue,
			"service_id", serviceIDStr,
			"error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "internal_error",
			"message": "failed to retrieve session details",
		})
		return
	}

	h.logger.Info("retrieved session details",
		"principal", principalValue,
		"service_id", serviceIDStr,
		"dependent_agents", len(sessionWithAgents.DependentAgents))

	// Return response
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"session":               sessionWithAgents.Session,
			"dependent_agents":      sessionWithAgents.DependentAgents,
			"dependent_agent_count": len(sessionWithAgents.DependentAgents),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// TerminateSession handles DELETE /api/third-party/{serviceId}/session
// Deletes the session and its encrypted tokens.
func (h *Handler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context in terminate session request")
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "principal not found in context")
		return
	}

	serviceIDStr := chi.URLParam(r, "serviceId")
	if serviceIDStr == "" {
		h.logger.Warn("missing serviceId in URL path")
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "serviceId parameter required in path")
		return
	}

	parsedServiceID, err := id.ParseServiceID(serviceIDStr)
	if err != nil {
		h.logger.Warn("invalid serviceId format in URL path", "service_id", serviceIDStr)
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "serviceId must be a valid UUID")
		return
	}

	err = h.service.TerminateSession(ctx, id.Principal(principalValue), parsedServiceID)
	if err != nil {
		if errors.Is(err, oauth2session.ErrSessionNotFound) {
			h.logger.Warn("session not found for termination", "service_id", serviceIDStr, "principal", principalValue)
			writeJSONError(w, http.StatusNotFound, "not_found", "session not found")
			return
		}

		if errors.Is(err, oauth2session.ErrUnauthorized) {
			h.logger.Error("unauthorized termination attempt",
				"service_id", serviceIDStr,
				"principal", principalValue,
				"error", err)
			writeJSONError(w, http.StatusForbidden, "forbidden", "you do not have permission to terminate this session")
			return
		}

		h.logger.Error("failed to terminate session",
			"principal", principalValue,
			"service_id", serviceIDStr,
			"error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to terminate session")
		return
	}

	h.logger.Info("session terminated successfully",
		"principal", principalValue,
		"service_id", serviceIDStr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "session terminated successfully",
	})
}

// RefreshSession handles POST /api/third-party/{serviceId}/session/refresh
// Forces an OAuth2 access-token refresh using the stored refresh token.
func (h *Handler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context in refresh session request")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "principal not found in context",
		})
		return
	}

	serviceIDStr := chi.URLParam(r, "serviceId")
	if serviceIDStr == "" {
		h.logger.Warn("missing serviceId in refresh session request")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request",
			"message": "serviceId parameter required in path",
		})
		return
	}

	parsedServiceID, err := id.ParseServiceID(serviceIDStr)
	if err != nil {
		h.logger.Warn("invalid serviceId format in refresh session request", "service_id", serviceIDStr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_request",
			"message": "serviceId must be a valid UUID",
		})
		return
	}

	summary, err := h.service.ForceRefreshSession(ctx, id.Principal(principalValue), parsedServiceID)
	if err != nil {
		switch {
		case errors.Is(err, oauth2session.ErrSessionNotFound):
			h.logger.Warn("session not found for refresh", "service_id", serviceIDStr, "principal", principalValue)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "not_found",
				"message": "session not found",
			})
		case errors.Is(err, oauth2session.ErrRefreshNotAvailable):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "refresh_unavailable",
				"message": "no valid refresh token available for this session",
			})
		case errors.Is(err, oauth2session.ErrRefreshFailed):
			h.logger.Error("force refresh failed at upstream",
				"principal", principalValue,
				"service_id", serviceIDStr,
				"error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "refresh_failed",
				"message": "failed to refresh token with the third-party provider",
			})
		default:
			h.logger.Error("failed to refresh session",
				"principal", principalValue,
				"service_id", serviceIDStr,
				"error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "internal_error",
				"message": "failed to refresh session",
			})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": summary})
}

// RegisterRoutes registers all OAuth2 session routes with the router.
// Routes are relative to /api (e.g., "/third-party/sessions" becomes "/api/third-party/sessions")
func (h *Handler) RegisterRoutes(router chi.Router) {
	router.Get("/third-party/sessions", h.ListSessions)
	router.Get("/third-party/{serviceId}/oauth2/authorize", h.InitiateFlow)
	router.With(http.NewCrossOriginProtection().Handler).Post("/third-party/{serviceId}/oauth2/authorize", h.InitiateFlow)
	router.Get("/third-party/{serviceId}/oauth2/callback", h.HandleCallback)
	router.Get("/third-party/{serviceId}/session", h.GetSessionDetails)
	router.Delete("/third-party/{serviceId}/session", h.TerminateSession)
	router.Post("/third-party/{serviceId}/session/refresh", h.RefreshSession)
}
