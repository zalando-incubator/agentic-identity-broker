package enduser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	xoauth2 "golang.org/x/oauth2"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/impersonation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureTokenExchangeSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	return recorder
}

func assertTokenExchangeTelemetryContainsNoSecrets(t *testing.T, records []tokenEndpointLogRecord, spans []sdktrace.ReadOnlySpan, secrets ...string) {
	t.Helper()
	var telemetry strings.Builder
	for _, record := range records {
		fmt.Fprintf(&telemetry, "%s %v\n", record.message, record.attrs)
		for _, forbidden := range []string{"error", "cause", "details", "resource", "error_type", "token_exchange.error_description", "token_exchange.validation_details", "token_exchange.resource", "token_exchange.service.name", "audience", "issuer_identifiers"} {
			assert.NotContains(t, record.attrs, forbidden)
		}
	}
	for _, span := range spans {
		fmt.Fprintf(&telemetry, "%s %v %v %v\n", span.Name(), span.Attributes(), span.Events(), span.Status())
		for _, attr := range span.Attributes() {
			assert.NotContains(t, []string{"token_exchange.resource", "token_exchange.error_description", "token_exchange.validation_details", "token_exchange.service.name", "url.full", "http.url"}, string(attr.Key))
		}
	}
	for _, secret := range secrets {
		assert.NotContains(t, telemetry.String(), secret, "secret must not occur in log messages/attributes or span names/attributes/events/status")
	}
}

func diagnosticAttributeValues(d tokenexchange.Diagnostic) map[string]any {
	attrs := map[string]any{
		"token_exchange.outcome":         string(d.Outcome()),
		"token_exchange.recovery_action": string(d.RecoveryAction()),
		"token_exchange.recovery_target": string(d.RecoveryTarget()),
		"token_exchange.exchange_kind":   string(d.ExchangeKind()),
	}
	if d.Outcome() != tokenexchange.OutcomeSuccess {
		attrs["token_exchange.failure_stage"] = string(d.Stage())
		attrs["token_exchange.failure_detail"] = string(d.Detail())
	}
	return attrs
}

func assertTokenExchangeDiagnostic(t *testing.T, record tokenEndpointLogRecord, span sdktrace.ReadOnlySpan, diagnostic tokenexchange.Diagnostic) {
	t.Helper()
	spanAttrs := make(map[string]any, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		spanAttrs[string(attr.Key)] = attr.Value.AsInterface()
	}
	for name, value := range diagnosticAttributeValues(diagnostic) {
		assert.Equal(t, value, record.attrs[name], name)
		assert.Equal(t, value, spanAttrs[name], name)
	}
	if diagnostic.Outcome() == tokenexchange.OutcomeSuccess {
		assert.NotContains(t, record.attrs, "token_exchange.failure_stage")
		assert.NotContains(t, record.attrs, "token_exchange.failure_detail")
		assert.NotContains(t, spanAttrs, "token_exchange.failure_stage")
		assert.NotContains(t, spanAttrs, "token_exchange.failure_detail")
		assert.NotEqual(t, codes.Error, span.Status().Code)
	} else {
		assert.Equal(t, codes.Error, span.Status().Code)
		assert.Equal(t, "token exchange failed", span.Status().Description)
	}
	assert.Empty(t, span.Events(), "raw exception events must never be recorded")
}

func TestOAuth2TokenHandler_MissingResourceIsObserved(t *testing.T) {
	spans := captureTokenExchangeSpans(t)
	logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	handler := &OAuth2TokenHandler{TokenExchange: &tokenexchange.TokenExchangeService{}, Logger: slog.New(logs)}
	form := url.Values{
		"grant_type":       {tokenexchange.TokenExchangeGrantType},
		"subject_token":    {"SUBJECT_SENTINEL"},
		"client_assertion": {"ASSERTION_SENTINEL"},
		"client_secret":    {"FORM_SECRET_SENTINEL"},
	}
	request := httptest.NewRequest(http.MethodPost, "https://REQUEST_HOST_SENTINEL.example/oauth2/token?token=REQUEST_QUERY_SENTINEL", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "Bearer HEADER_SENTINEL")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "invalid_request", body["error"])
	assert.Equal(t, "resource parameter is required", body["error_description"])
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", response.Header().Get("Pragma"))
	require.Len(t, spans.Ended(), 1)
	require.Len(t, *logs.records, 1)
	assertTokenExchangeDiagnostic(t, (*logs.records)[0], spans.Ended()[0], tokenexchange.NewDiagnostic(tokenexchange.StageRequestValidation, tokenexchange.DetailResourceMissing))
	assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "SUBJECT_SENTINEL", "ASSERTION_SENTINEL", "FORM_SECRET_SENTINEL", "HEADER_SENTINEL", "REQUEST_HOST_SENTINEL", "REQUEST_QUERY_SENTINEL")
}

func TestTokenExchangeFailureTelemetryAndProtocol(t *testing.T) {
	const recoveryURI = "https://broker.example.com/oauth2/services/reauth?state=RECOVERY_URI_SENTINEL"
	providerCause := &xoauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusBadRequest, Status: "PROVIDER_STATUS_SENTINEL"},
		Body:             []byte(`{"error":"invalid_grant","error_description":"PROVIDER_BODY_SENTINEL"}`),
		ErrorCode:        "invalid_grant",
		ErrorDescription: "PROVIDER_DESCRIPTION_SENTINEL",
		ErrorURI:         "https://PROVIDER_HOST_SENTINEL.example/PROVIDER_PATH_SENTINEL?client_secret=PROVIDER_QUERY_SENTINEL",
	}
	tests := []struct {
		name      string
		stage     tokenexchange.FailureStage
		detail    tokenexchange.FailureDetail
		makeError func(string) *tokenexchange.TokenExchangeError
		status    int
		code      string
		recovery  bool
	}{
		{"malformed request", tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed, tokenexchange.NewInvalidRequestError, http.StatusBadRequest, "invalid_request", false},
		{"invalid subject", tokenexchange.StageSubjectValidation, tokenexchange.DetailSubjectInvalid, tokenexchange.NewInvalidGrantError, http.StatusBadRequest, "invalid_grant", false},
		{"invalid client", tokenexchange.StageClientValidation, tokenexchange.DetailClientInvalid, tokenexchange.NewInvalidClientError, http.StatusUnauthorized, "invalid_client", false},
		{"missing consent", tokenexchange.StageGrantAuthorization, tokenexchange.DetailGrantMissing, tokenexchange.NewAccessDeniedError, http.StatusForbidden, "access_denied", true},
		{"missing session", tokenexchange.StageSessionLookup, tokenexchange.DetailSessionMissing, tokenexchange.NewInvalidGrantError, http.StatusBadRequest, "invalid_grant", true},
		{"local refresh expiry", tokenexchange.StageRefresh, tokenexchange.DetailRefreshTokenExpired, tokenexchange.NewInvalidGrantError, http.StatusBadRequest, "invalid_grant", true},
		{"provider refresh rejected", tokenexchange.StageRefresh, tokenexchange.DetailRefreshRejected, tokenexchange.NewInvalidGrantError, http.StatusBadRequest, "invalid_grant", true},
		{"provider client rejected", tokenexchange.StageRefresh, tokenexchange.DetailProviderClientRejected, tokenexchange.NewServerError, http.StatusInternalServerError, "server_error", false},
		{"provider unavailable", tokenexchange.StageRefresh, tokenexchange.DetailProviderUnavailable, tokenexchange.NewServerError, http.StatusInternalServerError, "server_error", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spans := captureTokenExchangeSpans(t)
			logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
			handler := &OAuth2TokenHandler{Logger: slog.New(logs)}
			diagnostic := tokenexchange.NewDiagnostic(test.stage, test.detail)
			cause := errors.Join(errors.New("NESTED_CAUSE_SENTINEL"), providerCause)
			err := test.makeError("DOMAIN_DESCRIPTION_SENTINEL").WithCause(cause).WithDiagnostic(diagnostic)
			if test.recovery {
				err = err.WithErrorURI(recoveryURI)
			}
			ctx, span := otel.Tracer("tokenexchange-test").Start(context.Background(), "tokenexchange.exchange")
			response := httptest.NewRecorder()
			handler.writeTokenExchangeFailure(ctx, span, response, fmt.Errorf("OUTER_CAUSE_SENTINEL: %w", err), tokenexchange.NewDiagnostic(tokenexchange.StageExchangeRouting, tokenexchange.DetailInternalUnclassified))
			span.End()

			assert.Equal(t, test.status, response.Code)
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			assert.Equal(t, "no-cache", response.Header().Get("Pragma"))
			var body map[string]string
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, test.code, body["error"])
			assert.NotEmpty(t, body["error_description"])
			if test.recovery {
				assert.Equal(t, recoveryURI, body["error_uri"], "recovery URI belongs in the direct protocol response only")
			} else {
				assert.NotContains(t, body, "error_uri")
			}
			assert.NotContains(t, response.Body.String(), "DOMAIN_DESCRIPTION_SENTINEL")
			require.Len(t, *logs.records, 1)
			require.Len(t, spans.Ended(), 1)
			assertTokenExchangeDiagnostic(t, (*logs.records)[0], spans.Ended()[0], diagnostic)
			assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "DOMAIN_DESCRIPTION_SENTINEL", "NESTED_CAUSE_SENTINEL", "OUTER_CAUSE_SENTINEL", "PROVIDER_STATUS_SENTINEL", "PROVIDER_BODY_SENTINEL", "PROVIDER_DESCRIPTION_SENTINEL", "PROVIDER_HOST_SENTINEL", "PROVIDER_PATH_SENTINEL", "PROVIDER_QUERY_SENTINEL", "RECOVERY_URI_SENTINEL")
		})
	}
}

func TestTokenExchangeTelemetryUsesSessionMetadataOnly(t *testing.T) {
	for _, code := range []string{"invalid_grant", "PROVIDER_CODE_SENTINEL", ""} {
		t.Run(code, func(t *testing.T) {
			spans := captureTokenExchangeSpans(t)
			logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
			metadata := oauth2session.NewErrorMetadata(oauth2session.OperationRefresh, oauth2session.DetailRefreshRejected).WithProviderResponse(http.StatusBadRequest, code)
			operationErr := oauth2session.NewOperationError(metadata, errors.New("RAW_SESSION_CAUSE_SENTINEL"))
			diagnostic := tokenexchange.NewDiagnostic(tokenexchange.StageRefresh, tokenexchange.DetailRefreshRejected)
			err := tokenexchange.NewInvalidGrantError("RAW_SESSION_DESCRIPTION_SENTINEL").WithCause(operationErr).WithDiagnostic(diagnostic)
			handler := &OAuth2TokenHandler{Logger: slog.New(logs)}
			ctx, span := otel.Tracer("tokenexchange-test").Start(context.Background(), "tokenexchange.exchange")
			handler.writeTokenExchangeFailure(ctx, span, httptest.NewRecorder(), err, diagnostic)
			span.End()
			require.Len(t, *logs.records, 1)
			require.Len(t, spans.Ended(), 1)
			attrs := (*logs.records)[0].attrs
			assert.Equal(t, string(metadata.Operation()), attrs["token_exchange.session.operation"])
			assert.Equal(t, string(metadata.Kind()), attrs["token_exchange.session.kind"])
			assert.Equal(t, string(metadata.Dependency()), attrs["token_exchange.session.dependency"])
			assert.EqualValues(t, metadata.StatusCode(), attrs["token_exchange.session.status_code"])
			if metadata.OAuthCode() == "" {
				assert.NotContains(t, attrs, "token_exchange.session.oauth_code")
			} else {
				assert.Equal(t, metadata.OAuthCode(), attrs["token_exchange.session.oauth_code"])
			}
			spanAttrs := make(map[string]any)
			for _, attr := range spans.Ended()[0].Attributes() {
				spanAttrs[string(attr.Key)] = attr.Value.AsInterface()
			}
			for key, value := range attrs {
				assert.Equal(t, value, spanAttrs[key], key)
			}
			assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "RAW_SESSION_CAUSE_SENTINEL", "RAW_SESSION_DESCRIPTION_SENTINEL", "PROVIDER_CODE_SENTINEL")
		})
	}
}

type diagnosticImpersonationService struct {
	target      *impersonation.Target
	resolveErr  error
	exchangeErr error
	response    *tokenexchange.TokenExchangeResponse
}

func (s *diagnosticImpersonationService) ResolveTarget(context.Context, []string) (*impersonation.Target, bool, error) {
	return s.target, true, s.resolveErr
}

func (s *diagnosticImpersonationService) Impersonate(context.Context, *impersonation.Request, *impersonation.Target) (*impersonation.Outcome, error) {
	return &impersonation.Outcome{Response: s.response, Audit: impersonation.AuditRecord{Outcome: "success"}}, s.exchangeErr
}

func diagnosticImpersonationForm() url.Values {
	return url.Values{
		"grant_type":            {tokenexchange.TokenExchangeGrantType},
		"audience":              {"https://AUDIENCE_HOST_SENTINEL.example/AUDIENCE_PATH_SENTINEL"},
		"client_assertion_type": {tokenexchange.JWTBearerType},
		"client_assertion":      {"ASSERTION_SENTINEL"},
		"actor_token_type":      {impersonation.JWTTokenType},
		"actor_token":           {"ACTOR_TOKEN_SENTINEL"},
		"subject_token_type":    {impersonation.JWTTokenType},
		"subject_token":         {"SUBJECT_TOKEN_SENTINEL"},
	}
}

type tokenExchangeFailingWriter struct {
	*httptest.ResponseRecorder
	err error
}

func (w *tokenExchangeFailingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestOAuth2TokenHandler_ImpersonationTelemetryAndResponseWrites(t *testing.T) {
	for _, test := range []struct {
		name            string
		resolveFailure  bool
		parseFailure    bool
		exchangeFailure bool
		writeFailure    bool
		shortWrite      bool
	}{
		{name: "short success response write", writeFailure: true, shortWrite: true},
		{name: "short error response write", exchangeFailure: true, writeFailure: true, shortWrite: true},
		{name: "success"},
		{name: "routing failure", resolveFailure: true},
		{name: "parse failure", parseFailure: true},
		{name: "exchange failure", exchangeFailure: true},
		{name: "success response write failure", writeFailure: true},
		{name: "error response write failure", exchangeFailure: true, writeFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			spans := captureTokenExchangeSpans(t)
			logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
			service := &diagnosticImpersonationService{
				target:   &impersonation.Target{Agent: &storage.Agent{ID: id.NewAgentID()}},
				response: tokenexchange.NewTokenExchangeResponse("ACCESS_TOKEN_SENTINEL", "Bearer", tokenexchange.AccessTokenType),
			}
			diagnostic := tokenexchange.SuccessDiagnostic(tokenexchange.ExchangeImpersonation)
			if test.resolveFailure {
				diagnostic = tokenexchange.NewDiagnostic(tokenexchange.StageExchangeRouting, tokenexchange.DetailRequestMalformed).WithExchangeKind(tokenexchange.ExchangeImpersonation)
				service.resolveErr = tokenexchange.NewInvalidRequestError("DESCRIPTION_SENTINEL").WithCause(errors.New("CAUSE_SENTINEL")).WithDiagnostic(diagnostic)
			}
			if test.exchangeFailure {
				diagnostic = tokenexchange.NewDiagnostic(tokenexchange.StageSubjectValidation, tokenexchange.DetailSubjectInvalid).WithExchangeKind(tokenexchange.ExchangeImpersonation)
				service.exchangeErr = tokenexchange.NewInvalidGrantError("DESCRIPTION_SENTINEL").WithCause(errors.New("CAUSE_SENTINEL")).WithDiagnostic(diagnostic)
			}
			form := diagnosticImpersonationForm()
			if test.parseFailure {
				form.Set("resource", "https://RESOURCE_HOST_SENTINEL.example/RESOURCE_PATH_SENTINEL")
				diagnostic = tokenexchange.NewDiagnostic(tokenexchange.StageRequestValidation, tokenexchange.DetailRequestMalformed).WithExchangeKind(tokenexchange.ExchangeImpersonation)
			}
			handler := &OAuth2TokenHandler{Impersonation: service, Logger: slog.New(logs)}
			request := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			var writer http.ResponseWriter = response
			if test.writeFailure {
				writer = &tokenExchangeFailingWriter{ResponseRecorder: response, err: errors.New("RESPONSE_WRITE_SENTINEL")}
				if test.shortWrite {
					writer = &tokenExchangeFailingWriter{ResponseRecorder: response}
				}
				diagnostic = tokenexchange.NewDiagnostic(tokenexchange.StageResponseWrite, tokenexchange.DetailResponseWriteFailed).WithExchangeKind(tokenexchange.ExchangeImpersonation)
			}
			handler.ServeHTTP(writer, request)
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			assert.Equal(t, "no-cache", response.Header().Get("Pragma"))
			require.Len(t, spans.Ended(), 1)
			var record tokenEndpointLogRecord
			var found bool
			for _, candidate := range *logs.records {
				if candidate.message == "Token exchange failed" || candidate.message == "token_exchange_succeeded" {
					record, found = candidate, true
				}
			}
			require.True(t, found, "exchange outcome must be observed")
			assertTokenExchangeDiagnostic(t, record, spans.Ended()[0], diagnostic)
			if !test.writeFailure && !test.exchangeFailure && !test.resolveFailure && !test.parseFailure {
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Contains(t, response.Body.String(), "ACCESS_TOKEN_SENTINEL", "access token must still reach its intended client")
			} else {
				assert.NotContains(t, response.Body.String(), "DESCRIPTION_SENTINEL")
			}
			assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "DESCRIPTION_SENTINEL", "CAUSE_SENTINEL", "RESPONSE_WRITE_SENTINEL", "RESOURCE_HOST_SENTINEL", "RESOURCE_PATH_SENTINEL", "ASSERTION_SENTINEL", "ACTOR_TOKEN_SENTINEL", "SUBJECT_TOKEN_SENTINEL", "ACCESS_TOKEN_SENTINEL")
		})
	}
}

func TestTokenExchangeSuccessOmitsFailureMetadataAndCredentials(t *testing.T) {
	spans := captureTokenExchangeSpans(t)
	logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	handler := &OAuth2TokenHandler{Logger: slog.New(logs)}
	response := tokenexchange.NewTokenExchangeResponseFull("ISSUED_ACCESS_SENTINEL", "Bearer", tokenexchange.AccessTokenType, "ISSUED_REFRESH_SENTINEL", "SCOPE_SENTINEL", 3600)
	response.Service = tokenexchange.ServiceRef{ID: id.NewServiceID()}
	ctx, span := otel.Tracer("tokenexchange-test").Start(context.Background(), "tokenexchange.exchange")
	writer := httptest.NewRecorder()
	handler.writeTokenExchangeSuccess(ctx, span, writer, response, tokenexchange.ExchangeThirdParty)
	span.End()
	require.Equal(t, http.StatusOK, writer.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &body))
	assert.Equal(t, response.AccessToken, body["access_token"])
	assert.Equal(t, response.RefreshToken, body["refresh_token"])
	assert.Equal(t, response.TokenType, body["token_type"])
	assert.Equal(t, response.IssuedTokenType, body["issued_token_type"])
	assert.EqualValues(t, response.ExpiresIn, body["expires_in"])
	assert.Equal(t, response.Scope, body["scope"])
	assert.Equal(t, "no-store", writer.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", writer.Header().Get("Pragma"))
	require.Len(t, *logs.records, 1)
	require.Len(t, spans.Ended(), 1)
	assertTokenExchangeDiagnostic(t, (*logs.records)[0], spans.Ended()[0], tokenexchange.SuccessDiagnostic(tokenexchange.ExchangeThirdParty))
	assert.Equal(t, response.Service.ID.String(), (*logs.records)[0].attrs["token_exchange.service.id"])
	assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "ISSUED_ACCESS_SENTINEL", "ISSUED_REFRESH_SENTINEL", "SCOPE_SENTINEL")
}

func TestTokenExchangeUnknownFailureUsesKnownStageFallback(t *testing.T) {
	spans := captureTokenExchangeSpans(t)
	logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	handler := &OAuth2TokenHandler{Logger: slog.New(logs)}
	fallback := tokenexchange.NewDiagnostic(tokenexchange.StageRefresh, tokenexchange.DetailInternalUnclassified)
	ctx, span := otel.Tracer("tokenexchange-test").Start(context.Background(), "tokenexchange.exchange")
	writer := httptest.NewRecorder()
	handler.writeTokenExchangeFailure(ctx, span, writer, fmt.Errorf("UNKNOWN_OUTER_SENTINEL: %w", errors.New("UNKNOWN_CAUSE_SENTINEL")), fallback)
	span.End()
	require.Equal(t, http.StatusInternalServerError, writer.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &body))
	assert.Equal(t, "server_error", body["error"])
	assert.NotContains(t, body, "error_uri")
	require.Len(t, *logs.records, 1)
	require.Len(t, spans.Ended(), 1)
	assertTokenExchangeDiagnostic(t, (*logs.records)[0], spans.Ended()[0], fallback)
	assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "UNKNOWN_OUTER_SENTINEL", "UNKNOWN_CAUSE_SENTINEL")
}

func TestTokenExchangeResponseWriteCancellationKeepsCurrentStage(t *testing.T) {
	spans := captureTokenExchangeSpans(t)
	logs := newTokenEndpointLogCaptureHandler(slog.LevelInfo)
	handler := &OAuth2TokenHandler{Logger: slog.New(logs)}
	ctx, span := otel.Tracer("tokenexchange-test").Start(context.Background(), "tokenexchange.exchange")
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	writer := &tokenExchangeFailingWriter{ResponseRecorder: httptest.NewRecorder(), err: context.Canceled}
	handler.writeTokenExchangeSuccess(ctx, span, writer, &tokenexchange.TokenExchangeResponse{AccessToken: "RESPONSE_TOKEN_SENTINEL"}, tokenexchange.ExchangeThirdParty)
	span.End()
	want := tokenexchange.NewDiagnostic(tokenexchange.StageResponseWrite, tokenexchange.DetailCallerCanceled)
	require.Len(t, *logs.records, 1)
	require.Len(t, spans.Ended(), 1)
	assertTokenExchangeDiagnostic(t, (*logs.records)[0], spans.Ended()[0], want)
	assertTokenExchangeTelemetryContainsNoSecrets(t, *logs.records, spans.Ended(), "RESPONSE_TOKEN_SENTINEL")
}
