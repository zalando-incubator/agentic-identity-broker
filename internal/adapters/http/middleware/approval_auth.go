package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtclaims"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

const (
	ApprovalClientAssertionHeaderName = "X-Client-Assertion"

	approvalCreateGrantType = "approval_create"
	approvalSyncGrantType   = "approval_sync"
)

type ApprovalAuthContext struct {
	Principal       id.Principal
	AgentID         id.AgentID
	GatewayClientID string
}

type approvalAuthContextKey struct{}

func ApprovalAuthFromContext(ctx context.Context) (ApprovalAuthContext, bool) {
	values, ok := ctx.Value(approvalAuthContextKey{}).(ApprovalAuthContext)
	return values, ok
}

func withApprovalAuthContext(ctx context.Context, values ApprovalAuthContext) context.Context {
	return context.WithValue(ctx, approvalAuthContextKey{}, values)
}

type ApprovalRequestAuthenticator struct {
	jwtValidator *tokenexchange.JWTValidator
	celEvaluator *tokenexchange.CELEvaluator
}

func NewApprovalRequestAuthenticator(jwtValidator *tokenexchange.JWTValidator, celEvaluator *tokenexchange.CELEvaluator) *ApprovalRequestAuthenticator {
	return &ApprovalRequestAuthenticator{jwtValidator: jwtValidator, celEvaluator: celEvaluator}
}

type approvalAuthError struct {
	status  int
	code    string
	message string
	cause   error
}

func (e *approvalAuthError) Error() string {
	return e.message
}

func (e *approvalAuthError) Unwrap() error {
	return e.cause
}

func RequireApprovalSubjectTokenAndClientAssertion(auth *ApprovalRequestAuthenticator) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principalValue, agentID, gatewayClientID, err := auth.authenticateCreate(
				r.Context(),
				r.Header.Get("Authorization"),
				r.Header.Get(ApprovalClientAssertionHeaderName),
			)
			if err != nil {
				if mapApprovalAuthError(w, err) {
					return
				}
				writeApprovalAuthError(w, http.StatusInternalServerError, "internal_error", "failed to authenticate approval create request")
				return
			}

			ctx := principal.WithPrincipal(r.Context(), string(principalValue))
			ctx = withApprovalAuthContext(ctx, ApprovalAuthContext{
				Principal:       principalValue,
				AgentID:         agentID,
				GatewayClientID: gatewayClientID,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireApprovalClientAssertion(auth *ApprovalRequestAuthenticator) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var principalFilter *id.Principal
			if p := strings.TrimSpace(r.URL.Query().Get("principal")); p != "" {
				principalValue := id.Principal(p)
				principalFilter = &principalValue
			}

			gatewayClientID, err := auth.authenticateSync(r.Context(), r.Header.Get("Authorization"), principalFilter)
			if err != nil {
				if mapApprovalAuthError(w, err) {
					return
				}
				writeApprovalAuthError(w, http.StatusInternalServerError, "internal_error", "failed to authenticate approval sync request")
				return
			}

			ctx := withApprovalAuthContext(r.Context(), ApprovalAuthContext{GatewayClientID: gatewayClientID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireApprovalSubjectToken(auth *ApprovalRequestAuthenticator) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principalValue, err := auth.authenticateConsume(r.Context(), r.Header.Get("Authorization"))
			if err != nil {
				if mapApprovalAuthError(w, err) {
					return
				}
				writeApprovalAuthError(w, http.StatusInternalServerError, "internal_error", "failed to authenticate approval consume request")
				return
			}

			ctx := principal.WithPrincipal(r.Context(), string(principalValue))
			ctx = withApprovalAuthContext(ctx, ApprovalAuthContext{Principal: principalValue})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func mapApprovalAuthError(w http.ResponseWriter, err error) bool {
	var authErr *approvalAuthError
	if !errors.As(err, &authErr) {
		return false
	}
	writeApprovalAuthError(w, authErr.status, authErr.code, authErr.message)
	return true
}

func writeApprovalAuthError(w http.ResponseWriter, statusCode int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   errCode,
		"message": message,
	})
}

func newUnauthorizedApprovalAuthError(message string, cause error) *approvalAuthError {
	return &approvalAuthError{status: http.StatusUnauthorized, code: "unauthorized", message: message, cause: cause}
}

func newForbiddenApprovalAuthError(message string, cause error) *approvalAuthError {
	return &approvalAuthError{status: http.StatusForbidden, code: "forbidden", message: message, cause: cause}
}

func newServiceUnavailableApprovalAuthError(message string, cause error) *approvalAuthError {
	return &approvalAuthError{status: http.StatusServiceUnavailable, code: "service_unavailable", message: message, cause: cause}
}

func (a *ApprovalRequestAuthenticator) authenticateCreate(ctx context.Context, authorizationHeader, clientAssertionHeader string) (id.Principal, id.AgentID, string, error) {
	principalValue, agentID, subjectClaims, err := a.authenticateSubjectToken(ctx, authorizationHeader)
	if err != nil {
		return "", id.AgentID{}, "", err
	}

	gatewayClientID, err := a.authenticateClientAssertion(ctx, clientAssertionHeader, subjectClaims, tokenexchange.CELRequestContext{
		GrantType: approvalCreateGrantType,
		Principal: string(principalValue),
		AgentID:   agentID.String(),
	})
	if err != nil {
		return "", id.AgentID{}, "", err
	}

	return principalValue, agentID, gatewayClientID, nil
}

func (a *ApprovalRequestAuthenticator) authenticateSync(ctx context.Context, authorizationHeader string, principalFilter *id.Principal) (string, error) {
	requestContext := tokenexchange.CELRequestContext{GrantType: approvalSyncGrantType}
	if principalFilter != nil {
		requestContext.Principal = string(*principalFilter)
	}

	clientAssertion, err := extractBearerToken(authorizationHeader, "client assertion")
	if err != nil {
		return "", err
	}

	return a.authorizeClientAssertion(ctx, clientAssertion, map[string]any{}, requestContext)
}

func (a *ApprovalRequestAuthenticator) authenticateConsume(ctx context.Context, authorizationHeader string) (id.Principal, error) {
	principalValue, _, _, err := a.authenticateSubjectToken(ctx, authorizationHeader)
	if err != nil {
		return "", err
	}
	return principalValue, nil
}

func (a *ApprovalRequestAuthenticator) authenticateSubjectToken(ctx context.Context, authorizationHeader string) (id.Principal, id.AgentID, map[string]any, error) {
	if a == nil || a.jwtValidator == nil || a.celEvaluator == nil {
		return "", id.AgentID{}, nil, newServiceUnavailableApprovalAuthError("approval subject token validation is not configured", nil)
	}

	subjectToken, err := extractBearerToken(authorizationHeader, "subject token")
	if err != nil {
		return "", id.AgentID{}, nil, err
	}

	subjectJWT, err := a.jwtValidator.ValidateSubjectToken(ctx, subjectToken)
	if err != nil {
		if isServerSideTokenExchangeError(err) {
			return "", id.AgentID{}, nil, newServiceUnavailableApprovalAuthError("subject token validation is unavailable", err)
		}
		return "", id.AgentID{}, nil, newUnauthorizedApprovalAuthError("invalid subject token", err)
	}

	subjectClaims := jwtclaims.FromToken(subjectJWT)
	principalValue, agentIDValue, err := a.celEvaluator.ResolveHumanSubject(subjectClaims)
	switch {
	case errors.Is(err, tokenexchange.ErrAgentSubjectToken):
		return "", id.AgentID{}, nil, newForbiddenApprovalAuthError("agent-subject tokens cannot authenticate approval requests", err)
	case errors.Is(err, tokenexchange.ErrAgentSubjectDetection):
		return "", id.AgentID{}, nil, newServiceUnavailableApprovalAuthError("agent-subject detection is unavailable", err)
	case err != nil && principalValue == "":
		return "", id.AgentID{}, nil, newUnauthorizedApprovalAuthError("subject token principal extraction failed", err)
	case err != nil:
		return "", id.AgentID{}, nil, newUnauthorizedApprovalAuthError("subject token agent extraction failed", err)
	}

	agentID, err := id.ParseAgentID(agentIDValue)
	if err != nil {
		return "", id.AgentID{}, nil, newUnauthorizedApprovalAuthError("subject token agent identity is invalid", err)
	}

	return id.Principal(principalValue), agentID, subjectClaims, nil
}

func (a *ApprovalRequestAuthenticator) authenticateClientAssertion(ctx context.Context, tokenSource string, subjectClaims map[string]any, requestContext tokenexchange.CELRequestContext) (string, error) {
	clientAssertion, err := extractTokenValue(tokenSource, "client assertion")
	if err != nil {
		return "", err
	}

	return a.authorizeClientAssertion(ctx, clientAssertion, subjectClaims, requestContext)
}

func (a *ApprovalRequestAuthenticator) authorizeClientAssertion(ctx context.Context, clientAssertion string, subjectClaims map[string]any, requestContext tokenexchange.CELRequestContext) (string, error) {
	if a == nil || a.jwtValidator == nil || a.celEvaluator == nil {
		return "", newServiceUnavailableApprovalAuthError("approval client assertion validation is not configured", nil)
	}

	clientAssertionJWT, err := a.jwtValidator.ValidateClientAssertion(ctx, clientAssertion)
	if err != nil {
		if isServerSideTokenExchangeError(err) {
			return "", newServiceUnavailableApprovalAuthError("client assertion validation is unavailable", err)
		}
		return "", newUnauthorizedApprovalAuthError("invalid client assertion", err)
	}

	clientAssertionClaims := jwtclaims.FromToken(clientAssertionJWT)
	authorized, err := a.celEvaluator.AuthorizePrivilegedClient(clientAssertionClaims, subjectClaims, requestContext)
	if err != nil {
		if isServerSideTokenExchangeError(err) {
			return "", newServiceUnavailableApprovalAuthError("client assertion authorization is unavailable", err)
		}
		return "", newUnauthorizedApprovalAuthError("client assertion not authorized", err)
	}
	if !authorized {
		return "", newUnauthorizedApprovalAuthError("client assertion not authorized", nil)
	}

	gatewayClientID, _ := clientAssertionJWT.Subject()
	if gatewayClientID == "" {
		return "", newUnauthorizedApprovalAuthError("client assertion subject is missing", nil)
	}

	return gatewayClientID, nil
}

func extractBearerToken(headerValue, label string) (string, error) {
	if strings.TrimSpace(headerValue) == "" {
		return "", newUnauthorizedApprovalAuthError(fmt.Sprintf("missing %s", label), nil)
	}

	if !strings.HasPrefix(strings.ToLower(headerValue), "bearer ") {
		return "", newUnauthorizedApprovalAuthError(fmt.Sprintf("%s must be supplied as a Bearer token", label), nil)
	}

	tokenValue := strings.TrimSpace(headerValue[len("Bearer "):])
	if tokenValue == "" {
		return "", newUnauthorizedApprovalAuthError(fmt.Sprintf("missing %s", label), nil)
	}

	return tokenValue, nil
}

func extractTokenValue(rawValue, label string) (string, error) {
	trimmed := strings.TrimSpace(rawValue)
	if trimmed == "" {
		return "", newUnauthorizedApprovalAuthError(fmt.Sprintf("missing %s", label), nil)
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "bearer ") {
		trimmed = strings.TrimSpace(trimmed[len("Bearer "):])
	}
	if trimmed == "" {
		return "", newUnauthorizedApprovalAuthError(fmt.Sprintf("missing %s", label), nil)
	}
	return trimmed, nil
}

func isServerSideTokenExchangeError(err error) bool {
	var tokenErr *tokenexchange.TokenExchangeError
	if !errors.As(err, &tokenErr) {
		return false
	}
	return tokenErr.HTTPStatus() >= http.StatusInternalServerError
}
