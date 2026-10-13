package enduser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/impersonation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ImpersonationService supplies the impersonation operations required by the token handler.
type ImpersonationService interface {
	ResolveTarget(ctx context.Context, audiences []string) (*impersonation.Target, bool, error)
	Impersonate(ctx context.Context, req *impersonation.Request, target *impersonation.Target) (*impersonation.Outcome, error)
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
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)

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
				h.Logger.ErrorContext(r.Context(), "unexpected error during client resolution", "error_code", "server_error")
			}
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "client resolution failed")
		}
		return
	}

	h.GrantHandler.HandleTokenGrant(w, r, grantType, formData, resolution)
}

// handleTokenExchange processes RFC 8693 token exchange requests.
func (h *OAuth2TokenHandler) handleTokenExchange(w http.ResponseWriter, r *http.Request, formData url.Values) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	ctx, span := otel.Tracer("tokenexchange").Start(r.Context(), "tokenexchange.exchange")
	defer span.End()

	if h.Impersonation != nil {
		target, activated, err := h.Impersonation.ResolveTarget(ctx, formData["audience"])
		if err != nil {
			span.SetName("tokenexchange.impersonation")
			h.logImpersonationDecision(ctx, h.impersonationParseAudit(err))
			h.writeTokenExchangeFailure(ctx, span, w, err, tokenexchange.NewDiagnostic(tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified).WithExchangeKind(tokenexchange.ExchangeImpersonation))
			return
		}
		if activated {
			span.SetName("tokenexchange.impersonation")
			h.handleImpersonation(ctx, span, w, formData, target)
			return
		}
	}

	if h.TokenExchange == nil {
		diagnostic := tokenexchange.NewDiagnostic(tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified)
		h.observeTokenExchange(ctx, span, diagnostic, nil, tokenexchange.ServiceRef{}, tokenexchange.AuthorizationRef{})
		writeErr := writeTokenExchangeErrorJSON(w, http.StatusBadRequest, map[string]string{
			"error":             "unsupported_grant_type",
			"error_description": "token exchange is not available in this deployment mode",
		})
		if writeErr != nil {
			h.observeTokenExchangeResponseWriteFailure(ctx, span, writeErr, diagnostic.ExchangeKind(), tokenexchange.ServiceRef{}, tokenexchange.AuthorizationRef{})
		}
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
	if req.Resource == "" {
		diagnostic := tokenexchange.NewDiagnostic(tokenexchange.StageRequestValidation, tokenexchange.DetailResourceMissing)
		err := tokenexchange.NewInvalidRequestError("resource parameter is required").WithDiagnostic(diagnostic)
		h.writeTokenExchangeFailure(ctx, span, w, err, diagnostic)
		return
	}

	response, err := h.TokenExchange.Exchange(ctx, req)
	if err != nil {
		h.writeTokenExchangeFailure(ctx, span, w, err, tokenexchange.NewDiagnostic(tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified))
		return
	}
	h.writeTokenExchangeSuccess(ctx, span, w, response, tokenexchange.ExchangeThirdParty)
}

// handleImpersonation always emits its credential-free audit event before the response.
func (h *OAuth2TokenHandler) handleImpersonation(ctx context.Context, span trace.Span, w http.ResponseWriter, formData url.Values, target *impersonation.Target) {
	span.SetAttributes(attribute.String("impersonation.target_agent_id", target.Agent.ID.String()))
	req, err := impersonation.ParseRequest(formData)
	if err != nil {
		record := h.impersonationParseAudit(err)
		record.TargetAgentID = target.Agent.ID.String()
		h.logImpersonationDecision(ctx, record)
		h.writeTokenExchangeFailure(ctx, span, w, err, tokenexchange.NewDiagnostic(tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed).WithExchangeKind(tokenexchange.ExchangeImpersonation))
		return
	}

	outcome, err := h.Impersonation.Impersonate(ctx, req, target)
	if outcome != nil {
		h.logImpersonationDecision(ctx, outcome.Audit)
	}
	if err != nil {
		h.writeTokenExchangeFailure(ctx, span, w, err, tokenexchange.NewDiagnostic(tokenexchange.StageSubjectValidation, tokenexchange.DetailInternalUnclassified).WithExchangeKind(tokenexchange.ExchangeImpersonation))
		return
	}
	h.writeTokenExchangeSuccess(ctx, span, w, outcome.Response, tokenexchange.ExchangeImpersonation)
}

// impersonationParseAudit builds an audit record for a failure before rule evaluation.
func (h *OAuth2TokenHandler) impersonationParseAudit(err error) impersonation.AuditRecord {
	record := impersonation.AuditRecord{}
	var tokenErr *tokenexchange.TokenExchangeError
	if errors.As(err, &tokenErr) {
		record.Outcome = tokenErr.Code()
		record.OAuthErrorCode = tokenErr.Code()
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
	}
	if record.TargetAgentID != "" {
		attrs = append(attrs, "target_agent_id", record.TargetAgentID)
	}
	if record.SelectedRule != "" {
		attrs = append(attrs, "rule", record.SelectedRule)
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
	if requestID := httpctx.RequestIDFromContext(ctx); requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	h.Logger.InfoContext(ctx, "impersonation_decision", attrs...)
}

func (h *OAuth2TokenHandler) writeTokenExchangeFailure(ctx context.Context, span trace.Span, w http.ResponseWriter, err error, fallback tokenexchange.Diagnostic) {
	diagnostic := fallback
	service := tokenexchange.ServiceRef{}
	authorization := tokenexchange.AuthorizationRef{}
	var exchangeErr *tokenexchange.TokenExchangeError
	if errors.As(err, &exchangeErr) {
		diagnostic = exchangeErr.Diagnostic().WithExchangeKind(fallback.ExchangeKind())
		service = exchangeErr.Service()
		if fallback.ExchangeKind() == tokenexchange.ExchangeThirdParty {
			authorization = exchangeErr.Authorization()
		}
	}
	h.observeTokenExchange(ctx, span, diagnostic, err, service, authorization)
	if writeErr := h.handleTokenExchangeError(w, err); writeErr != nil {
		h.observeTokenExchangeResponseWriteFailure(ctx, span, writeErr, diagnostic.ExchangeKind(), service, authorization)
	}
}

func (h *OAuth2TokenHandler) writeTokenExchangeSuccess(ctx context.Context, span trace.Span, w http.ResponseWriter, response *tokenexchange.TokenExchangeResponse, kind tokenexchange.ExchangeKind) {
	authorization := tokenexchange.AuthorizationRef{}
	if kind == tokenexchange.ExchangeThirdParty {
		authorization = response.Authorization
	}
	body, err := json.Marshal(response) // #nosec G117 -- OAuth2 token response is serialized for its direct HTTP response, not logging.
	if err != nil {
		diagnostic := tokenexchange.NewDiagnostic(tokenexchange.StageResponseWrite, tokenexchange.DetailResponseWriteFailed).WithExchangeKind(kind)
		h.writeTokenExchangeFailure(ctx, span, w, tokenexchange.NewServerError("token exchange response could not be encoded").WithCause(err).WithObservation(diagnostic, response.Service, authorization), diagnostic)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	if n, writeErr := w.Write(body); writeErr != nil || n != len(body) {
		h.observeTokenExchangeResponseWriteFailure(ctx, span, writeErr, kind, response.Service, authorization)
		return
	}
	h.observeTokenExchange(ctx, span, tokenexchange.SuccessDiagnostic(kind), nil, response.Service, authorization)
}

func (h *OAuth2TokenHandler) observeTokenExchangeResponseWriteFailure(ctx context.Context, span trace.Span, cause error, kind tokenexchange.ExchangeKind, service tokenexchange.ServiceRef, authorization tokenexchange.AuthorizationRef) {
	diagnostic := tokenexchange.NewDiagnostic(tokenexchange.StageResponseWrite, tokenexchange.DetailResponseWriteFailed).WithExchangeKind(kind)
	if ctx.Err() != nil && errors.Is(cause, ctx.Err()) {
		diagnostic = tokenexchange.NewDiagnostic(tokenexchange.StageResponseWrite, tokenexchange.DetailCallerCanceled).WithExchangeKind(kind)
	}
	h.observeTokenExchange(ctx, span, diagnostic, nil, service, authorization)
}

func (h *OAuth2TokenHandler) observeTokenExchange(ctx context.Context, span trace.Span, diagnostic tokenexchange.Diagnostic, err error, service tokenexchange.ServiceRef, authorization tokenexchange.AuthorizationRef) {
	attrs := []attribute.KeyValue{
		attribute.String("token_exchange.outcome", string(diagnostic.Outcome())),
		attribute.String("token_exchange.recovery_action", string(diagnostic.RecoveryAction())),
		attribute.String("token_exchange.recovery_target", string(diagnostic.RecoveryTarget())),
		attribute.String("token_exchange.exchange_kind", string(diagnostic.ExchangeKind())),
	}
	level, message := slog.LevelInfo, "token_exchange_succeeded"
	if diagnostic.Outcome() != tokenexchange.OutcomeSuccess {
		attrs = append(attrs,
			attribute.String("token_exchange.failure_stage", string(diagnostic.Stage())),
			attribute.String("token_exchange.failure_detail", string(diagnostic.Detail())),
		)
		level, message = slog.LevelError, "Token exchange failed"
		span.SetStatus(codes.Error, "token exchange failed")
	}
	if !service.ID.IsZero() {
		attrs = append(attrs, attribute.String("token_exchange.service.id", service.ID.String()))
	}
	if !authorization.AgentID.IsZero() {
		attrs = append(attrs, attribute.String("token_exchange.agent.id", authorization.AgentID.String()))
	}
	if !authorization.GrantID.IsZero() {
		attrs = append(attrs, attribute.String("token_exchange.grant.id", authorization.GrantID.String()))
	}
	if !authorization.GrantUpdatedAt.IsZero() {
		attrs = append(attrs, attribute.String("token_exchange.grant.updated_at", authorization.GrantUpdatedAt.UTC().Format(time.RFC3339Nano)))
	}
	if !authorization.GrantID.IsZero() && authorization.GrantHasValidUntil {
		attrs = append(attrs, attribute.String("token_exchange.grant.valid_until", authorization.GrantValidUntil.UTC().Format(time.RFC3339Nano)))
	}
	var operationErr *oauth2session.OperationError
	if errors.As(err, &operationErr) {
		metadata := operationErr.Metadata()
		attrs = append(attrs,
			attribute.String("token_exchange.session.operation", string(metadata.Operation())),
			attribute.String("token_exchange.session.detail", string(metadata.Detail())),
			attribute.String("token_exchange.session.kind", string(metadata.Kind())),
			attribute.String("token_exchange.session.dependency", string(metadata.Dependency())),
		)
		if metadata.StatusCode() != 0 {
			attrs = append(attrs, attribute.Int("token_exchange.session.status_code", metadata.StatusCode()))
		}
		if metadata.OAuthCode() != "" {
			attrs = append(attrs, attribute.String("token_exchange.session.oauth_code", metadata.OAuthCode()))
		}
	}
	span.SetAttributes(attrs...)
	if h.Logger != nil {
		logAttrs := make([]slog.Attr, 0, len(attrs))
		for _, attr := range attrs {
			logAttrs = append(logAttrs, slog.Any(string(attr.Key), attr.Value.AsInterface()))
		}
		h.Logger.LogAttrs(ctx, level, message, logAttrs...)
	}
}

// handleTokenExchangeError keeps recovery links in the direct OAuth response only.
func (h *OAuth2TokenHandler) handleTokenExchangeError(w http.ResponseWriter, err error) error {
	status := http.StatusInternalServerError
	errBody := map[string]string{
		"error":             "server_error",
		"error_description": "internal server error during token exchange",
	}
	var exchangeErr *tokenexchange.TokenExchangeError
	if errors.As(err, &exchangeErr) {
		status = exchangeErr.HTTPStatus()
		errBody["error"] = exchangeErr.Code()
		errBody["error_description"] = tokenExchangeErrorDescription(exchangeErr)
		if exchangeErr.ErrorURI() != "" {
			errBody["error_uri"] = exchangeErr.ErrorURI()
		}
	}
	return writeTokenExchangeErrorJSON(w, status, errBody)
}

func writeTokenExchangeErrorJSON(w http.ResponseWriter, status int, body map[string]string) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	n, err := w.Write(encoded)
	if err == nil && n != len(encoded) {
		return io.ErrShortWrite
	}
	return err
}

func tokenExchangeErrorDescription(err *tokenexchange.TokenExchangeError) string {
	diagnostic := err.Diagnostic()
	if diagnostic.Detail() == tokenexchange.DetailResourceMissing {
		return "resource parameter is required"
	}
	if diagnostic.RecoveryTarget() == tokenexchange.TargetProviderSession && diagnostic.RecoveryAction() == tokenexchange.RecoveryReauthenticate {
		return "User session is unavailable. Please re-authenticate."
	}
	if diagnostic.RecoveryAction() == tokenexchange.RecoveryReconsent {
		return "User authorization is insufficient. Please re-consent."
	}
	switch err.Code() {
	case tokenexchange.InvalidRequestError:
		return "token exchange request is invalid"
	case tokenexchange.InvalidClientError:
		return "client authentication failed"
	case tokenexchange.InvalidGrantError:
		return "token exchange grant is invalid or unavailable"
	case tokenexchange.InvalidTargetError:
		return "token exchange target is invalid or unavailable"
	case tokenexchange.InvalidScopeError:
		return "requested scope is not permitted"
	case tokenexchange.AccessDeniedError:
		return "token exchange is not authorized"
	default:
		return "internal server error during token exchange"
	}
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
