package enduser

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

func TestTokenPreAuthenticationInputErrorsBypassLedger(t *testing.T) {
	agentID := id.NewAgentID()
	const formContentType = "application/x-www-form-urlencoded"
	tests := []struct {
		name, method, contentType, body, code, description string
		status                                             int
		exchange, readFailure                              bool
	}{
		{name: "method", method: http.MethodGet, contentType: formContentType, body: "credential-canary", status: http.StatusMethodNotAllowed, code: "invalid_request", description: "method not allowed"},
		{name: "content type", method: http.MethodPost, contentType: "application/json", body: "credential-canary", status: http.StatusBadRequest, code: "invalid_request", description: "invalid Content-Type: expected application/x-www-form-urlencoded"},
		{name: "missing content type", method: http.MethodPost, body: "credential-canary", status: http.StatusBadRequest, code: "invalid_request", description: "invalid Content-Type: expected application/x-www-form-urlencoded"},
		{name: "body read", method: http.MethodPost, contentType: formContentType, body: "credential-canary", status: http.StatusBadRequest, code: "invalid_request", description: "failed to read request body", readFailure: true},
		{name: "oversized body", method: http.MethodPost, contentType: formContentType, body: strings.Repeat("x", 256*1024+1), status: http.StatusBadRequest, code: "invalid_request", description: "failed to read request body"},
		{name: "empty body", method: http.MethodPost, contentType: formContentType, status: http.StatusBadRequest, code: "invalid_request", description: "request body cannot be empty"},
		{name: "malformed form", method: http.MethodPost, contentType: formContentType, body: "grant_type=%ZZ", status: http.StatusBadRequest, code: "invalid_request", description: "failed to parse form data"},
		{name: "missing grant type", method: http.MethodPost, contentType: formContentType, body: "client_id=" + agentID.String(), status: http.StatusBadRequest, code: "invalid_request", description: "grant_type is required"},
		{name: "missing client id", method: http.MethodPost, contentType: formContentType, body: "grant_type=client_credentials&client_secret=secret", status: http.StatusBadRequest, code: "invalid_request", description: "client_id is required"},
		{name: "missing local secret", method: http.MethodPost, contentType: formContentType, body: "grant_type=client_credentials&client_id=" + agentID.String(), status: http.StatusBadRequest, code: "invalid_request", description: "client_id and client_secret are required"},
		{name: "missing authorization code", method: http.MethodPost, contentType: formContentType, body: "grant_type=authorization_code&client_id=" + agentID.String(), status: http.StatusBadRequest, code: "invalid_request", description: "code is required"},
		{name: "missing refresh token", method: http.MethodPost, contentType: formContentType, body: "grant_type=refresh_token&client_id=" + agentID.String(), status: http.StatusBadRequest, code: "invalid_request", description: "refresh_token is required"},
		{name: "unsupported local grant", method: http.MethodPost, contentType: formContentType, body: "grant_type=password&client_id=" + agentID.String(), status: http.StatusBadRequest, code: "unsupported_grant_type", description: "grant_type must be 'client_credentials', 'authorization_code', or 'refresh_token'"},
		{name: "unavailable exchange", method: http.MethodPost, contentType: formContentType, body: "grant_type=" + tokenexchange.TokenExchangeGrantType, status: http.StatusBadRequest, code: "unsupported_grant_type", description: "token exchange is not available in this deployment mode"},
		{name: "missing exchange resource", method: http.MethodPost, contentType: formContentType, body: "grant_type=" + tokenexchange.TokenExchangeGrantType, status: http.StatusBadRequest, code: "invalid_request", description: "resource parameter is required", exchange: true},
	}
	for _, tc := range tests {
		for _, ledgerDown := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/ledger-available", true: "/ledger-unavailable"}[ledgerDown], func(t *testing.T) {
				store := &ledgerfixture.Store{}
				if ledgerDown {
					store.AppendError = errors.New("ledger unavailable")
				}
				outcomes := oauth2.NewTokenOutcomeService(nil, nil, store.Recorder(t))
				handler := &OAuth2TokenHandler{
					Outcomes: outcomes, OAuth2Service: newLocalModeOAuth2Service(),
					GrantHandler: NewLocalGrantStrategy(fixedMinting(nil, errors.New("minting must not run")), nil),
				}
				if tc.exchange {
					handler.TokenExchange = &tokenexchange.TokenExchangeService{}
				}
				request := httptest.NewRequest(tc.method, "/oauth2/token", strings.NewReader(tc.body))
				if tc.contentType != "" {
					request.Header.Set("Content-Type", tc.contentType)
				}
				if tc.readFailure {
					request.Body = &failingReadCloser{err: errors.New("read failed")}
				}
				writer := httptest.NewRecorder()
				handler.ServeHTTP(writer, request)
				require.Equal(t, tc.status, writer.Code)
				require.Equal(t, "application/json", writer.Header().Get("Content-Type"))
				require.JSONEq(t, `{"error":"`+tc.code+`","error_description":"`+tc.description+`"}`, writer.Body.String())
				require.Empty(t, store.Events, "input rejected before authentication is not a business outcome")
			})
		}
	}
}

func TestTokenExchangeStructuralErrorsBypassLedger(t *testing.T) {
	for _, tc := range []struct {
		name, missing, description string
		invalidResource            bool
	}{
		{name: "missing subject token", missing: "subject_token", description: "subject_token parameter is required"},
		{name: "missing client assertion", missing: "client_assertion", description: "client_assertion parameter is required"},
		{name: "invalid resource", invalidResource: true, description: "resource URI must include a scheme (e.g., https://)"},
	} {
		for _, ledgerDown := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/ledger-available", true: "/ledger-unavailable"}[ledgerDown], func(t *testing.T) {
				store := &ledgerfixture.Store{}
				if ledgerDown {
					store.AppendError = errors.New("ledger unavailable")
				}
				recorder := store.Recorder(t)
				// The request must fail structural validation before using any authentication dependency.
				service, err := tokenexchange.NewTokenExchangeService(
					&tokenexchange.JWTValidator{}, &tokenexchange.CELEvaluator{},
					&thirdparty.ThirdpartyOAuth2ProviderService{}, &oauth2session.OAuth2SessionService{},
					&consent.Service{}, &permissionset.Service{}, newStubAgentRepo(id.NewAgentID(), "upstream-client"),
					&ports.TokenExchangeConfig{}, recorder,
				)
				require.NoError(t, err)
				handler := &OAuth2TokenHandler{TokenExchange: service, Outcomes: oauth2.NewTokenOutcomeService(nil, nil, recorder)}
				form := url.Values{
					"grant_type": {tokenexchange.TokenExchangeGrantType}, "subject_token": {"subject-canary"},
					"client_assertion": {"assertion-canary"}, "resource": {"https://api.example.com"},
				}
				if tc.invalidResource {
					form.Set("resource", "not-absolute")
				} else {
					form.Del(tc.missing)
				}
				request := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				writer := httptest.NewRecorder()
				handler.ServeHTTP(writer, request)
				require.Equal(t, http.StatusBadRequest, writer.Code)
				require.JSONEq(t, `{"error":"invalid_request","error_description":"`+tc.description+`"}`, writer.Body.String())
				require.Empty(t, store.Events)
			})
		}
	}
}

func TestTokenImpersonationParseErrorBypassesLedger(t *testing.T) {
	for _, ledgerDown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ledger-available", true: "ledger-unavailable"}[ledgerDown], func(t *testing.T) {
			handler, issuer, target := newImpersonationHeaderHandler(t)
			store := &ledgerfixture.Store{}
			if ledgerDown {
				store.AppendError = errors.New("ledger unavailable")
			}
			handler.Outcomes = oauth2.NewTokenOutcomeService(nil, nil, store.Recorder(t))
			writer := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader("grant_type="+tokenexchange.TokenExchangeGrantType+"&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2F"+target.ID.String()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			handler.ServeHTTP(writer, request)
			require.Equal(t, http.StatusBadRequest, writer.Code)
			require.JSONEq(t, `{"error":"invalid_request","error_description":"client_assertion is required"}`, writer.Body.String())
			require.False(t, issuer.called)
			require.Empty(t, store.Events)
		})
	}
}

func TestTokenAuthenticationFailureStillWaitsForLedger(t *testing.T) {
	for _, ledgerDown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ledger-available", true: "ledger-unavailable"}[ledgerDown], func(t *testing.T) {
			store := &ledgerfixture.Store{}
			if ledgerDown {
				store.AppendError = errors.New("ledger unavailable")
			}
			outcomes := oauth2.NewTokenOutcomeService(nil, nil, store.Recorder(t))
			handler := &OAuth2TokenHandler{Outcomes: outcomes, OAuth2Service: newFailingOAuth2Service(&ports.ClientIDError{Code: "invalid_client", Desc: "client authentication failed"}), GrantHandler: NewLocalGrantStrategy(fixedMinting(nil, errors.New("minting must not run")), nil)}
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader("grant_type=client_credentials&client_id="+id.NewAgentID().String()+"&client_secret=wrong"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			writer := httptest.NewRecorder()
			handler.ServeHTTP(writer, request)
			if ledgerDown {
				require.Equal(t, http.StatusInternalServerError, writer.Code)
				require.JSONEq(t, `{"error":"server_error","error_description":"failed to record token outcome"}`, writer.Body.String())
				require.Empty(t, store.Events)
				return
			}
			require.Equal(t, http.StatusUnauthorized, writer.Code)
			require.JSONEq(t, `{"error":"invalid_client","error_description":"client authentication failed"}`, writer.Body.String())
			require.Len(t, store.Events, 1)
			require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
			require.Equal(t, "authentication_failed", store.Events[0].Data["reason_code"])
		})
	}
}
