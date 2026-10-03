package enduser

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// TokenGrantStrategy handles OAuth2 token grant requests at the HTTP transport layer.
// The resolution is provided by the caller (OAuth2TokenHandler) after domain-level
// resolution and mode enforcement via OAuth2Service.ResolveForTokenGrant.
type TokenGrantStrategy interface {
	HandleTokenGrant(w http.ResponseWriter, r *http.Request, grantType string, formData url.Values, resolution *ports.TokenGrantResolution)
}

// proxyTokenGrantStrategy forwards token grant requests to an upstream OAuth2 server.
type proxyTokenGrantStrategy struct {
	upstreamTokenURL string
	outcomes         ports.TokenOutcomeService
	logger           *slog.Logger
}

var proxyTokenResponseHeaders = [...]string{
	"Content-Type",
	"Cache-Control",
	"Pragma",
	"WWW-Authenticate",
}

// NewProxyTokenGrantStrategy forwards responses after domain completion.
func NewProxyTokenGrantStrategy(
	upstreamTokenURL string,
	outcomes ports.TokenOutcomeService,
	logger *slog.Logger,
) *proxyTokenGrantStrategy {
	return &proxyTokenGrantStrategy{upstreamTokenURL: upstreamTokenURL, outcomes: outcomes, logger: logger}
}

// HandleTokenGrant forwards only the response staged by domain completion.
func (s *proxyTokenGrantStrategy) HandleTokenGrant(w http.ResponseWriter, r *http.Request, _ string, formData url.Values, resolution *ports.TokenGrantResolution) {
	ctx, span := otel.Tracer("upstream").Start(r.Context(), "oauth2.token_proxy")
	defer span.End()
	span.SetAttributes(attribute.String("http.method", "POST"))

	response, failure, err := s.outcomes.Proxy(ctx, formData, r.Header.Get("Content-Type"), resolution)
	if err != nil {
		switch failure {
		case ports.TokenProxyRecordingFailed:
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "failed to record token outcome")
		case ports.TokenProxyClientMissing:
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "agent has no upstream client_id configured")
		case ports.TokenProxyRequestCreationFailed:
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to create upstream token request",
					"upstream_url", s.upstreamTokenURL, "error", err)
			}
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "failed to create upstream request")
		default:
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "upstream token request failed",
					"upstream_url", s.upstreamTokenURL, "error", err)
			}
			writeOAuth2ErrorJSON(w, http.StatusBadGateway, "server_error", "failed to contact upstream server")
		}
		return
	}
	defer func() { _ = response.CloseBody() }()

	statusCode := response.ResponseStatus()
	span.SetAttributes(attribute.Int("http.status_code", statusCode))

	agentID := resolution.AgentID
	completion, failure, err := s.outcomes.CompleteProxy(r.Context(), response, agentID)
	if err != nil {
		if failure == ports.TokenProxyRecordingFailed {
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "failed to record token outcome")
			return
		}
		if failure == ports.TokenProxyResponseReadFailed {
			if s.logger != nil {
				s.logger.ErrorContext(r.Context(), "AgentIDClaimMissing",
					"agent_id", agentID.String(),
					"reason", "failed to read upstream response body",
					"error", err,
				)
			}
			w.Header().Del("Content-Length")
			w.Header().Del("Transfer-Encoding")
			writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "failed to read upstream response")
			return
		}

		if s.logger != nil {
			if mismatch, ok := err.(*oauth2.AgentIDMismatchError); ok {
				s.logger.ErrorContext(r.Context(), "AgentIDClaimMismatch",
					"expected_agent_id", mismatch.Expected,
					"received_agent_id", mismatch.Received,
					"claim_name", mismatch.ClaimName,
				)
			} else {
				s.logger.ErrorContext(r.Context(), "AgentIDClaimMissing",
					"agent_id", agentID.String(),
					"error", err.Error(),
				)
			}
		}
		w.Header().Del("Content-Length")
		w.Header().Del("Transfer-Encoding")
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "agent ID claim verification failed")
		return
	}

	if completion.Verified {
		if s.logger != nil {
			s.logger.InfoContext(r.Context(), "AgentIDClaimVerified",
				"agent_id", agentID.String(),
			)
		}
	}
	for _, headerName := range proxyTokenResponseHeaders {
		for _, value := range response.HeaderValues(headerName) {
			w.Header().Add(headerName, value)
		}
	}
	w.WriteHeader(statusCode)
	if _, err := w.Write(completion.Body); err != nil && s.logger != nil {
		s.logger.ErrorContext(r.Context(), "failed to stream upstream token response", "error", err)
	}
}

// localGrantStrategy handles token grants locally using a TokenMintingStrategy.
type localGrantStrategy struct {
	minting ports.TokenMintingStrategy
	logger  *slog.Logger
}

// NewLocalGrantStrategy returns a strategy that mints tokens locally.
func NewLocalGrantStrategy(minting ports.TokenMintingStrategy, logger *slog.Logger) *localGrantStrategy {
	return &localGrantStrategy{minting: minting, logger: logger}
}

// HandleTokenGrant dispatches client_credentials, authorization_code, and refresh_token grants to the local minting strategy.
func (s *localGrantStrategy) HandleTokenGrant(w http.ResponseWriter, r *http.Request, grantType string, formData url.Values, _ *ports.TokenGrantResolution) {
	switch grantType {
	case "client_credentials":
		rawClientID := formData.Get("client_id")
		clientSecret := formData.Get("client_secret")
		scope := formData.Get("scope")

		if rawClientID == "" || clientSecret == "" {
			writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "client_id and client_secret are required")
			return
		}

		resp, err := s.minting.HandleClientCredentials(r.Context(), id.ClientID(rawClientID), clientSecret, scope)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(r.Context(), "client_credentials grant failed", "error", err, "client_id", rawClientID)
			}
			s.handleMintingError(w, r.Context(), err, "client_credentials", rawClientID)
			return
		}

		if s.logger != nil {
			s.logger.InfoContext(r.Context(), "TokenIssued",
				"event", "TokenIssued",
				"grant_type", "client_credentials",
				"client_id", rawClientID,
				"scope", scope,
			)
		}

		s.writeTokenResponse(w, resp)

	case "authorization_code":
		rawClientID := formData.Get("client_id")
		clientSecret := formData.Get("client_secret")
		code := formData.Get("code")
		redirectURI := formData.Get("redirect_uri")
		codeVerifier := formData.Get("code_verifier")

		if rawClientID == "" {
			writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "client_id is required")
			return
		}
		if code == "" {
			writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "code is required")
			return
		}

		resp, err := s.minting.HandleAuthorizationCodeExchange(r.Context(), id.ClientID(rawClientID), clientSecret, code, redirectURI, codeVerifier)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(r.Context(), "authorization_code exchange failed", "error", err, "client_id", rawClientID)
			}
			s.handleMintingError(w, r.Context(), err, "authorization_code", rawClientID)
			return
		}

		if s.logger != nil {
			s.logger.InfoContext(r.Context(), "TokenIssued",
				"event", "TokenIssued",
				"grant_type", "authorization_code",
				"client_id", rawClientID,
			)
		}

		s.writeTokenResponse(w, resp)

	case "refresh_token":
		rawClientID := formData.Get("client_id")
		clientSecret := formData.Get("client_secret")
		refreshToken := formData.Get("refresh_token")
		scope := formData.Get("scope")

		if rawClientID == "" {
			writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "client_id is required")
			return
		}
		if refreshToken == "" {
			writeOAuth2ErrorJSON(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
			return
		}

		resp, err := s.minting.HandleRefreshToken(r.Context(), id.ClientID(rawClientID), clientSecret, refreshToken, scope)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(r.Context(), "refresh_token grant failed", "error", err, "client_id", rawClientID)
			}
			s.handleMintingError(w, r.Context(), err, "refresh_token", rawClientID)
			return
		}

		if s.logger != nil {
			s.logger.InfoContext(r.Context(), "TokenIssued",
				"event", "TokenIssued",
				"grant_type", "refresh_token",
				"client_id", rawClientID,
				"scope", scope,
			)
		}

		s.writeTokenResponse(w, resp)

	default:
		writeOAuth2ErrorJSON(w, http.StatusBadRequest, "unsupported_grant_type",
			"grant_type must be 'client_credentials', 'authorization_code', or 'refresh_token'")
	}
}

func (s *localGrantStrategy) handleMintingError(w http.ResponseWriter, ctx context.Context, err error, grantType, clientID string) {
	var errorCode, errorDesc string
	var statusCode int

	var rfc6749Err *oauth2server.RFC6749Error
	if errors.As(err, &rfc6749Err) {
		errorCode = rfc6749Err.Code()
		errorDesc = rfc6749Err.Description()
		statusCode = rfc6749Err.HTTPStatus()
	} else {
		errorCode = "server_error"
		errorDesc = "internal server error"
		statusCode = http.StatusInternalServerError
	}

	writeOAuth2ErrorJSON(w, statusCode, errorCode, errorDesc)

	if s.logger != nil {
		attrs := []any{
			"event", "TokenRequestFailed",
			"grant_type", grantType,
			"error_code", errorCode,
			"error_description", errorDesc,
			"client_id", clientID,
		}
		if statusCode >= http.StatusInternalServerError {
			s.logger.ErrorContext(ctx, "TokenRequestFailed", attrs...)
		} else {
			s.logger.WarnContext(ctx, "TokenRequestFailed", attrs...)
		}
	}
}

func (s *localGrantStrategy) writeTokenResponse(w http.ResponseWriter, resp *ports.TokenResponse) {
	tokenResp := map[string]interface{}{
		"access_token": resp.AccessToken,
		"token_type":   resp.TokenType,
		"expires_in":   resp.ExpiresIn,
	}
	if resp.RefreshToken != "" {
		tokenResp["refresh_token"] = resp.RefreshToken
	}
	if resp.Scope != "" {
		tokenResp["scope"] = resp.Scope
	}

	body, err := json.Marshal(tokenResp)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to encode token response", "error", err)
		}
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// hybridTokenGrantStrategy dispatches token grants to proxy or local based on the agent's ClientType.
// The agent is pre-resolved by the domain layer — this strategy only routes.
type hybridTokenGrantStrategy struct {
	proxy  TokenGrantStrategy
	local  TokenGrantStrategy
	logger *slog.Logger
}

// NewHybridTokenGrantStrategy returns a TokenGrantStrategy that dispatches based on client mode.
func NewHybridTokenGrantStrategy(proxy, local TokenGrantStrategy, logger *slog.Logger) TokenGrantStrategy {
	if proxy == nil {
		panic("NewHybridTokenGrantStrategy: proxy strategy must not be nil")
	}
	if local == nil {
		panic("NewHybridTokenGrantStrategy: local strategy must not be nil")
	}
	return &hybridTokenGrantStrategy{proxy: proxy, local: local, logger: logger}
}

func (s *hybridTokenGrantStrategy) HandleTokenGrant(w http.ResponseWriter, r *http.Request, grantType string, formData url.Values, resolution *ports.TokenGrantResolution) {
	switch resolution.ClientType {
	case storage.ProxyClient:
		s.proxy.HandleTokenGrant(w, r, grantType, formData, resolution)
	case storage.CIMDClient, storage.LocalClient:
		s.local.HandleTokenGrant(w, r, grantType, formData, resolution)
	default:
		if s.logger != nil {
			s.logger.ErrorContext(r.Context(), "unexpected client type in hybrid token grant dispatch",
				"client_type", resolution.ClientType,
				"agent_id", resolution.AgentID)
		}
		writeOAuth2ErrorJSON(w, http.StatusInternalServerError, "server_error", "unexpected client mode in hybrid dispatch")
	}
}
