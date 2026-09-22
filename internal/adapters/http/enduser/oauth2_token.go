package enduser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/impersonation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ImpersonationService supplies the impersonation operations required by the token handler.
type ImpersonationService interface {
	ResolveTarget(ctx context.Context, audiences []string) (*impersonation.Target, bool, error)
	Impersonate(ctx context.Context, req *impersonation.Request, target *impersonation.Target) (*impersonation.Outcome, error)
	AudiencePrefix() string
}

// OAuth2TokenHandler handles OAuth2 token endpoint requests.
// Routes RFC 8693 token exchange to handleTokenExchange; all other grants are
// resolved via OAuth2Service.ResolveForTokenGrant then delegated to GrantHandler.
type OAuth2TokenHandler struct {
	TokenExchange *tokenexchange.TokenExchangeService
	OAuth2Service ports.OAuth2Service
	Logger        *slog.Logger
	GrantHandler  TokenGrantStrategy // always non-nil: proxy, local, or hybrid

	// Impersonation is non-nil only in local mode when configured. Its routing prefix resolves a
	// canonical registered target agent before third-party token-exchange parameter validation.
	Impersonation ImpersonationService
}

// ServeHTTP implements http.Handler for the token endpoint.
func (h *OAuth2TokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeOAuth2ErrorJSON(w, http.StatusMethodNotAllowed, "invalid_request", "method not allowed")
		return
	}

	// OAuth 2.0 token endpoint must accept application/x-www-form-urlencoded per RFC 6749 Section 4.1.3.
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/x-www-form-urlencoded" {
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "invalid Content-Type: expected application/x-www-form-urlencoded")
		return
	}

	defer func() { _ = r.Body.Close() }()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "failed to read request body")
		return
	}

	if len(body) == 0 {
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "request body cannot be empty")
		return
	}

	formData, err := url.ParseQuery(string(body))
	if err != nil {
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "failed to parse form data")
		return
	}

	grantType := formData.Get("grant_type")
	if grantType == "" {
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "grant_type is required")
		return
	}

	if grantType == tokenexchange.TokenExchangeGrantType {
		h.handleTokenExchange(w, r, formData)
		return
	}
	// Finalize the request security context at the /oauth2/token seam before
	// delegating to the grant handler. SecurityContextMiddleware defers finalization
	// for every POST /oauth2/token (it cannot read grant_type without consuming the
	// one-shot request body), so non-RFC8693 grants must finalize here — the
	// downstream grant handler emits context-aware TokenIssued audit logs that must
	// carry actor and trace_id, and the perimeter safety net only finalizes after
	// ServeHTTP returns. Delegated token exchange finalizes later at its own seam
	// with the calling peer (see tokenexchange.Exchange).
	httpmiddleware.FinalizeRequestSecurityContext(r.Context())

	rawClientID := formData.Get("client_id")
	if rawClientID == "" {
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	if h.OAuth2Service == nil || h.GrantHandler == nil {
		if h.Logger != nil {
			h.Logger.ErrorContext(r.Context(), "OAuth2 token handler invoked with nil OAuth2Service or GrantHandler — check builder wiring")
		}
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "OAuth2 authorization server not configured")
		return
	}

	resolution, resolveErr := h.OAuth2Service.ResolveForTokenGrant(r.Context(), id.ClientID(rawClientID))
	if resolveErr != nil {
		var clientErr *ports.ClientIDError
		if errors.As(resolveErr, &clientErr) {
			status := tokenEndpointStatus(clientErr.Code)
			writeOAuth2ErrorJSON(w, status, clientErr.Code, clientErr.Desc)
		} else {
			if h.Logger != nil {
				h.Logger.ErrorContext(r.Context(), "unexpected error during client resolution",
					"error", resolveErr, "client_id", rawClientID)
			}
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "client resolution failed")
		}
		return
	}

	h.GrantHandler.HandleTokenGrant(w, r, grantType, formData, resolution)
}

// handleTokenExchange processes RFC 8693 token exchange requests.
func (h *OAuth2TokenHandler) handleTokenExchange(w http.ResponseWriter, r *http.Request, formData url.Values) {
	// Resolve audience-target activation before the generic resource guard. Unselected audiences
	// retain third-party exchange behavior; malformed and unknown targets fail closed.
	if h.Impersonation != nil {
		target, activated, err := h.Impersonation.ResolveTarget(r.Context(), formData["audience"])
		if err != nil {
			h.logImpersonationDecision(r.Context(), h.impersonationParseAudit(err))
			h.handleTokenExchangeError(w, err)
			return
		}
		if activated {
			h.handleImpersonation(w, r, formData, target)
			return
		}
	}

	if h.TokenExchange == nil {
		if h.Logger != nil {
			h.Logger.WarnContext(r.Context(), "token exchange not wired, returning unsupported_grant_type")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "unsupported_grant_type",
			"error_description": "token exchange is not available in this deployment mode",
		})
		return
	}

	req := tokenexchange.NewTokenExchangeRequest(
		formData.Get("grant_type"),
		formData.Get("subject_token"),
		formData.Get("subject_token_type"),
		formData.Get("client_assertion"),
		formData.Get("client_assertion_type"),
		formData.Get("resource"),
		formData.Get("scope"),
	)

	// Per FR-008: resource parameter is mandatory and validation occurs at HTTP layer
	if req.Resource == "" {
		if h.Logger != nil {
			h.Logger.WarnContext(r.Context(), "Resource parameter missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_request",
			"error_description": "resource parameter is required",
		})
		return
	}

	sanitizedResource := sanitizeResourceURI(req.Resource)
	ctx, span := otel.Tracer("tokenexchange").Start(r.Context(), "tokenexchange.exchange")
	defer span.End()
	span.SetAttributes(
		attribute.String("token_exchange.resource", sanitizedResource),
		attribute.String("token_exchange.grant_type", req.GrantType),
	)
	response, err := h.TokenExchange.Exchange(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		var tokenErrForSpan *tokenexchange.TokenExchangeError
		if errors.As(err, &tokenErrForSpan) {
			if details := tokenErrForSpan.Details(); details != "" {
				span.SetAttributes(attribute.String("token_exchange.validation_details", truncateSpanAttribute(details, 512)))
			}
			span.SetAttributes(
				attribute.String("token_exchange.error_code", tokenErrForSpan.Code()),
				attribute.String("token_exchange.error_description", tokenErrForSpan.Description()),
			)
		}
		if h.Logger != nil {
			logAttrs := []any{
				"error", err.Error(),
				"error_type", fmt.Sprintf("%T", err),
				"resource", sanitizedResource,
			}
			var tokenErrForLog *tokenexchange.TokenExchangeError
			if errors.As(err, &tokenErrForLog) {
				if cause := errors.Unwrap(tokenErrForLog); cause != nil {
					logAttrs = append(logAttrs, "cause", cause.Error())
				}
				if details := tokenErrForLog.Details(); details != "" {
					logAttrs = append(logAttrs, "details", details)
				}
			}
			h.Logger.ErrorContext(ctx, "Token exchange failed", logAttrs...)
		}
		h.handleTokenExchangeError(w, err)
		return
	}

	body, err := json.Marshal(response) // #nosec G117 -- OAuth2 token response is serialized for its direct HTTP response, not logging.
	if err != nil {
		if h.Logger != nil {
			h.Logger.ErrorContext(ctx, "failed to encode token exchange response", "error", err)
		}
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)

	if h.Logger != nil {
		h.Logger.InfoContext(ctx, "token_exchange_succeeded",
			"resource", sanitizedResource,
			"issued_token_type", response.IssuedTokenType,
		)
	}
}

// handleImpersonation processes a target-resolved impersonation request end to end. It always
// emits a credential-free audit event, then writes the RFC 8693 success response or mapped error.
func (h *OAuth2TokenHandler) handleImpersonation(w http.ResponseWriter, r *http.Request, formData url.Values, target *impersonation.Target) {
	ctx, span := otel.Tracer("tokenexchange").Start(r.Context(), "tokenexchange.impersonation")
	defer span.End()
	span.SetAttributes(attribute.String("impersonation.target_agent_id", target.Agent.ID.String()))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	req, err := impersonation.ParseRequest(formData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		record := h.impersonationParseAudit(err)
		record.TargetAgentID = target.Agent.ID.String()
		h.logImpersonationDecision(ctx, record)
		h.handleTokenExchangeError(w, err)
		return
	}

	outcome, err := h.Impersonation.Impersonate(ctx, req, target)
	h.logImpersonationDecision(ctx, outcome.Audit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.handleTokenExchangeError(w, err)
		return
	}
	span.SetAttributes(attribute.String("impersonation.outcome", outcome.Audit.Outcome))

	body, marshalErr := json.Marshal(outcome.Response) // #nosec G117 -- OAuth2 token response is serialized for its direct HTTP response, not logging.
	if marshalErr != nil {
		if h.Logger != nil {
			h.Logger.Error("failed to encode impersonation response", "error", marshalErr)
		}
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// impersonationParseAudit builds an audit record for a failure before rule evaluation.
func (h *OAuth2TokenHandler) impersonationParseAudit(err error) impersonation.AuditRecord {
	record := impersonation.AuditRecord{Audience: h.Impersonation.AudiencePrefix()}
	var tokenErr *tokenexchange.TokenExchangeError
	if errors.As(err, &tokenErr) {
		record.Outcome = tokenErr.Code()
		record.OAuthErrorCode = tokenErr.Code()
		record.FailureCategory = tokenErr.Details()
	} else {
		record.Outcome = "server_error"
		record.OAuthErrorCode = "server_error"
	}
	return record
}

// logImpersonationDecision emits the credential-free impersonation_decision audit event. Only
// whitelisted, non-secret fields are recorded (FR-012, SC-005).
func (h *OAuth2TokenHandler) logImpersonationDecision(ctx context.Context, record impersonation.AuditRecord) {
	if h.Logger == nil {
		return
	}
	attrs := []any{
		"event", "impersonation_decision",
		"outcome", record.Outcome,
		"audience", record.Audience,
	}
	if record.TargetAgentID != "" {
		attrs = append(attrs, "target_agent_id", record.TargetAgentID)
	}
	if record.SelectedRule != "" {
		attrs = append(attrs, "rule", record.SelectedRule)
	}
	if len(record.IssuerIdentifiers) > 0 {
		attrs = append(attrs, "issuer_identifiers", record.IssuerIdentifiers)
	}
	if len(record.IssuerRoles) > 0 {
		attrs = append(attrs, "issuer_roles", record.IssuerRoles)
	}
	if record.PrivilegedClientIdentity != "" {
		attrs = append(attrs, "privileged_client_identity", record.PrivilegedClientIdentity)
	}
	if record.ActorIdentity != "" {
		attrs = append(attrs, "actor_identity", record.ActorIdentity)
	}
	if record.SubjectIdentity != "" {
		attrs = append(attrs, "subject_identity", record.SubjectIdentity)
	}
	if record.OAuthErrorCode != "" {
		attrs = append(attrs, "oauth_error_code", record.OAuthErrorCode)
	}
	if record.FailureCategory != "" {
		attrs = append(attrs, "failure_category", record.FailureCategory)
	}
	if requestID := httpctx.RequestIDFromContext(ctx); requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	h.Logger.InfoContext(ctx, "impersonation_decision", attrs...)
}

// handleTokenExchangeError maps domain-layer token exchange errors to RFC 8693 error responses.
func (h *OAuth2TokenHandler) handleTokenExchangeError(w http.ResponseWriter, err error) {
	var (
		status  int
		errBody map[string]string
	)

	var tokenExchangeErr *tokenexchange.TokenExchangeError
	if errors.As(err, &tokenExchangeErr) {
		status = tokenExchangeErr.HTTPStatus()
		errBody = map[string]string{
			"error":             tokenExchangeErr.Code(),
			"error_description": tokenExchangeErr.Description(),
		}
		if tokenExchangeErr.ErrorURI() != "" {
			errBody["error_uri"] = tokenExchangeErr.ErrorURI()
		}
	} else {
		if h.Logger != nil {
			h.Logger.Error("unrecognized token exchange error", "error", err)
		}
		status = http.StatusInternalServerError
		errBody = map[string]string{
			"error":             "server_error",
			"error_description": "internal server error during token exchange",
		}
	}

	body, marshalErr := json.Marshal(errBody)
	if marshalErr != nil {
		if h.Logger != nil {
			h.Logger.Error("failed to marshal token exchange error response", "error", marshalErr)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// tokenEndpointStatus maps an OAuth2 error code to the appropriate HTTP status for the token endpoint.
// RFC 6749 §5.2: invalid_client → 401, server_error → 500, all other codes → 400.
func tokenEndpointStatus(code string) int {
	switch code {
	case "invalid_client":
		return http.StatusUnauthorized
	case "server_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

// truncateSpanAttribute trims s to at most maxRunes runes and replaces newlines
// with spaces, producing a single-line string safe to emit as an OTel span attribute.
func truncateSpanAttribute(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	result := make([]rune, len(runes))
	for i, r := range runes {
		if r == '\n' || r == '\r' {
			result[i] = ' '
		} else {
			result[i] = r
		}
	}
	return string(result)
}

// sanitizeResourceURI strips caller-controlled credentials, query strings, and fragments
// before recording an RFC 8693 resource in telemetry or logs.
func sanitizeResourceURI(resource string) string {
	if i := strings.IndexByte(resource, '#'); i >= 0 {
		resource = resource[:i]
	}
	u, err := url.ParseRequestURI(resource)
	if err != nil {
		return "[invalid resource URI]"
	}
	u.User = nil
	u.RawQuery = ""
	return u.String()
}
