package enduser

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// AuthorizationProceedStrategy handles the "proceed" action from an AuthorizationDecision.
// This is the only part that differs between proxy mode and local mode; all other
// decision actions (redirect_to_consent, error) are handled by the shared ServeHTTP flow.
type AuthorizationProceedStrategy interface {
	HandleProceed(w http.ResponseWriter, r *http.Request, decision *ports.AuthorizationDecision, req *ports.AuthorizationRequest, principal id.Principal)
}

// proxyProceedStrategy redirects to the upstream OAuth2 server URL from the decision.
type proxyProceedStrategy struct{}

// NewProxyProceedStrategy returns a ProceedStrategy that redirects to the upstream OAuth2 server.
func NewProxyProceedStrategy() AuthorizationProceedStrategy {
	return &proxyProceedStrategy{}
}

func (s *proxyProceedStrategy) HandleProceed(w http.ResponseWriter, r *http.Request, decision *ports.AuthorizationDecision, _ *ports.AuthorizationRequest, _ id.Principal) {
	if decision.RedirectURL == "" {
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "empty upstream redirect URL")
		return
	}
	http.Redirect(w, r, decision.RedirectURL, http.StatusFound)
}

// localProceedStrategy issues a local authorization code and redirects back to the client.
type localProceedStrategy struct {
	issuer ports.AuthorizationCodeIssuer
	logger *slog.Logger
}

// NewLocalProceedStrategy returns a ProceedStrategy that issues authorization codes locally.
func NewLocalProceedStrategy(issuer ports.AuthorizationCodeIssuer, logger *slog.Logger) AuthorizationProceedStrategy {
	return &localProceedStrategy{issuer: issuer, logger: logger}
}

func (s *localProceedStrategy) HandleProceed(w http.ResponseWriter, r *http.Request, decision *ports.AuthorizationDecision, req *ports.AuthorizationRequest, principal id.Principal) {
	code, err := s.issuer.IssueAuthorizationCode(r.Context(), req, principal, decision.ServiceScopeBindings)
	if err != nil {
		// Per RFC 6749 §4.1.2.1: never redirect when the client or redirect_uri is invalid/unverified.
		// Treat server-side failures the same way because redirect_uri validation may not have happened yet.
		if shouldWriteDirectOAuth2Error(err) {
			writeDirectOAuth2Error(w, err)
			return
		}

		// All other OAuth2 errors: redirect with error params (redirect_uri has been validated).
		var rfc6749Err *oauth2server.RFC6749Error
		if errors.As(err, &rfc6749Err) {
			redirectWithError(w, r, req.RedirectURI, req.State, rfc6749Err.Code(), rfc6749Err.Description())
			return
		}

		if s.logger != nil {
			s.logger.Error("unexpected error issuing authorization code", "error", err)
		}
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "authorization failed")
		return
	}

	redirectURL, err := url.Parse(req.RedirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	q := redirectURL.Query()
	q.Set("code", code)
	if req.State != "" {
		q.Set("state", req.State)
	}
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func shouldWriteDirectOAuth2Error(err error) bool {
	return errors.Is(err, oauth2server.ErrInvalidClient) ||
		errors.Is(err, oauth2server.ErrInvalidRedirectURI) ||
		errors.Is(err, oauth2server.ErrServerError)
}

func writeDirectOAuth2Error(w http.ResponseWriter, err error) {
	var rfc6749Err *oauth2server.RFC6749Error
	if errors.As(err, &rfc6749Err) {
		writeOAuth2ErrorJSON(w, rfc6749Err.HTTPStatus(), rfc6749Err.Code(), rfc6749Err.Description())
		return
	}

	switch {
	case errors.Is(err, oauth2server.ErrServerError):
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "authorization failed")
	case errors.Is(err, oauth2server.ErrInvalidClient):
		writeOAuth2ErrorJSON(w, http.StatusUnauthorized, "invalid_client", "client authentication failed")
	default:
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "request validation failed")
	}
}

// hybridProceedStrategy dispatches to proxy or local based on the resolved ClientType.
type hybridProceedStrategy struct {
	proxy  AuthorizationProceedStrategy
	local  AuthorizationProceedStrategy
	logger *slog.Logger
}

// NewHybridProceedStrategy returns a ProceedStrategy that dispatches by client mode.
// proxy handles ProxyClient agents; local handles LocalClient and CIMDClient agents.
func NewHybridProceedStrategy(proxy, local AuthorizationProceedStrategy, logger *slog.Logger) AuthorizationProceedStrategy {
	if proxy == nil {
		panic("NewHybridProceedStrategy: proxy strategy must not be nil")
	}
	if local == nil {
		panic("NewHybridProceedStrategy: local strategy must not be nil")
	}
	return &hybridProceedStrategy{proxy: proxy, local: local, logger: logger}
}

func (s *hybridProceedStrategy) HandleProceed(w http.ResponseWriter, r *http.Request, decision *ports.AuthorizationDecision, req *ports.AuthorizationRequest, principal id.Principal) {
	switch decision.ClientType {
	case storage.ProxyClient:
		s.proxy.HandleProceed(w, r, decision, req, principal)
	case storage.CIMDClient, storage.LocalClient:
		s.local.HandleProceed(w, r, decision, req, principal)
	default:
		if s.logger != nil {
			s.logger.ErrorContext(r.Context(), "unexpected client type in hybrid proceed dispatch",
				"client_type", decision.ClientType)
		}
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "unexpected client mode in hybrid dispatch")
	}
}

func writeOAuth2ErrorJSON(w http.ResponseWriter, status int, errorCode, errorDescription string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q,"error_description":%q}`, errorCode, errorDescription)
}
