package oauth2session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
)

type capturedTokenExchangeRequest struct {
	method string
	body   url.Values
	header http.Header
	query  url.Values
	err    error
}

// T052: Public clients must use client_id in the POST body without sending a secret or HTTP credentials.
func TestHandleCallbackSecurity_PublicClientUsesBodyClientIDWithoutCredentialsAcrossRetries(t *testing.T) {
	logOutput := new(strings.Builder)
	service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(logOutput, nil)), 2)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	var (
		requests   []capturedTokenExchangeRequest
		requestsMu sync.Mutex
	)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		form, parseErr := url.ParseQuery(string(body))
		capture := capturedTokenExchangeRequest{
			method: r.Method,
			body:   form,
			header: r.Header.Clone(),
			query:  r.URL.Query(),
			err:    errorsJoin(readErr, parseErr),
		}

		requestsMu.Lock()
		requests = append(requests, capture)
		attempt := len(requests)
		requestsMu.Unlock()

		if hasCredentialMaterial(capture) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}

		if attempt == 1 {
			http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"public-access-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	provider := createTestService(serviceID)
	provider.ClientID = id.ClientID("public-client-id")
	provider.Endpoints.TokenEndpoint = tokenServer.URL
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	require.NoError(t, providerService.Create(context.Background(), provider))

	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	require.NotNil(t, flow)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)
	require.NotEmpty(t, claims.PKCEVerifier)

	result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "public-authorization-code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	requestsMu.Lock()
	capturedRequests := append([]capturedTokenExchangeRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, capturedRequests, 2, "the transient failure must retry exactly once")

	for _, request := range capturedRequests {
		assertPublicTokenExchangeRequest(t, request, "public-client-id", "public-authorization-code")
		assert.Equal(t, claims.PKCEVerifier, request.body.Get("code_verifier"))
	}

	assertSecurityEventsMarkPublicClient(t, logOutput.String(),
		"session.oauth2.flow_initiated",
		"session.oauth2.session_established",
	)
}

// T052: A token endpoint can reflect secrets and tokens in its error response; those values must stay internal.
func TestHandleCallbackSecurity_RedactsUpstreamCredentialMaterialFromErrorsAndLogs(t *testing.T) {
	const (
		sentinelSecret       = "sentinel-client-secret"
		sentinelCode         = "sentinel-authorization-code"
		sentinelAccessToken  = "sentinel-access-token"
		sentinelRefreshToken = "sentinel-refresh-token"
	)

	logOutput := new(strings.Builder)
	service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(logOutput, nil)), 2)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	var (
		requests   []capturedTokenExchangeRequest
		requestsMu sync.Mutex
		verifier   string
	)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		form, parseErr := url.ParseQuery(string(body))
		capture := capturedTokenExchangeRequest{
			method: r.Method,
			body:   form,
			header: r.Header.Clone(),
			query:  r.URL.Query(),
			err:    errorsJoin(readErr, parseErr),
		}

		requestsMu.Lock()
		requests = append(requests, capture)
		verifier = form.Get("code_verifier")
		requestsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid_grant",
			"error_description": fmt.Sprintf(
				"secret=%s verifier=%s code=%s access_token=%s refresh_token=%s",
				sentinelSecret,
				capture.body.Get("code_verifier"),
				sentinelCode,
				sentinelAccessToken,
				sentinelRefreshToken,
			),
		})
	}))
	defer tokenServer.Close()

	provider := createTestService(serviceID)
	provider.ClientID = id.ClientID("public-client-id")
	provider.Endpoints.TokenEndpoint = tokenServer.URL
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	require.NoError(t, providerService.Create(context.Background(), provider))

	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)

	require.NotNil(t, flow)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)
	require.NotEmpty(t, claims.PKCEVerifier)

	result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      sentinelCode,
		State:     flow.StateToken,
	})
	require.Error(t, err)
	assert.Nil(t, result)

	requestsMu.Lock()
	capturedRequests := append([]capturedTokenExchangeRequest(nil), requests...)
	capturedVerifier := verifier
	requestsMu.Unlock()
	require.Len(t, capturedRequests, 2, "a public-client failure must retry without changing authentication")
	for _, request := range capturedRequests {
		assertPublicTokenExchangeRequest(t, request, "public-client-id", sentinelCode)
	}
	require.Equal(t, claims.PKCEVerifier, capturedVerifier)

	logs := logOutput.String()
	for _, sentinel := range []string{
		sentinelSecret,
		claims.PKCEVerifier,
		sentinelCode,
		sentinelAccessToken,
		sentinelRefreshToken,
	} {
		assert.NotContains(t, err.Error(), sentinel)
		assert.NotContains(t, logs, sentinel)
	}
	assert.Contains(t, logs, `"error":"token exchange failed: invalid_grant"`)

	assertSecurityEventsMarkPublicClient(t, logs,
		"session.oauth2.flow_initiated",
		"session.oauth2.pkce_validation_failed",
	)
}

// T060: A rejected public refresh must never retry with credentials or expose upstream credential material.
func TestForceRefreshSessionSecurity_PublicClientOmitsCredentialsAndRedactsFailure(t *testing.T) {
	const (
		sentinelSecret       = "sentinel-client-secret"
		sentinelCode         = "sentinel-authorization-code"
		sentinelAccessToken  = "sentinel-access-token"
		sentinelRefreshToken = "sentinel-refresh-token"
	)

	logOutput := new(strings.Builder)
	service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(logOutput, nil)), 2)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	var (
		requests   []capturedTokenExchangeRequest
		requestsMu sync.Mutex
		verifier   string
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		form, parseErr := url.ParseQuery(string(body))
		capture := capturedTokenExchangeRequest{
			method: r.Method,
			body:   form,
			header: r.Header.Clone(),
			query:  r.URL.Query(),
			err:    errorsJoin(readErr, parseErr),
		}

		requestsMu.Lock()
		requests = append(requests, capture)
		if form.Get("grant_type") == "authorization_code" {
			verifier = form.Get("code_verifier")
		}
		capturedVerifier := verifier
		requestsMu.Unlock()

		if hasCredentialMaterial(capture) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch form.Get("grant_type") {
		case "authorization_code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  sentinelAccessToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": sentinelRefreshToken,
			})
		case "refresh_token":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "invalid_grant",
				"error_description": fmt.Sprintf(
					"secret=%s verifier=%s code=%s access_token=%s refresh_token=%s",
					sentinelSecret,
					capturedVerifier,
					sentinelCode,
					sentinelAccessToken,
					sentinelRefreshToken,
				),
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
		}
	}))
	defer tokenServer.Close()

	provider := createTestService(serviceID)
	provider.ClientID = id.ClientID("public-client-id")
	provider.Endpoints.TokenEndpoint = tokenServer.URL
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	require.NoError(t, providerService.Create(context.Background(), provider))

	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	require.NotNil(t, flow)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)
	require.NotEmpty(t, claims.PKCEVerifier)

	result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      sentinelCode,
		State:     flow.StateToken,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	logOutput.Reset()

	refreshedSession, err := service.ForceRefreshSession(context.Background(), principal, serviceID)
	require.Error(t, err)
	assert.Nil(t, refreshedSession)
	assert.ErrorIs(t, err, oauth2session.ErrRefreshFailed)

	requestsMu.Lock()
	capturedRequests := append([]capturedTokenExchangeRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, capturedRequests, 2, "a rejected refresh must not retry with a credential")

	refreshRequest := capturedRequests[1]
	require.NoError(t, refreshRequest.err)
	assert.Equal(t, http.MethodPost, refreshRequest.method)
	assert.Equal(t, "refresh_token", refreshRequest.body.Get("grant_type"))
	assert.Equal(t, sentinelRefreshToken, refreshRequest.body.Get("refresh_token"))
	assert.Equal(t, "public-client-id", refreshRequest.body.Get("client_id"))
	assert.NotContains(t, refreshRequest.body, "client_secret")
	assert.Empty(t, refreshRequest.header.Values("Authorization"))
	assert.Empty(t, refreshRequest.query)

	logs := logOutput.String()
	for _, sentinel := range []string{
		sentinelSecret,
		claims.PKCEVerifier,
		sentinelCode,
		sentinelAccessToken,
		sentinelRefreshToken,
	} {
		assert.NotContains(t, err.Error(), sentinel)
		assert.NotContains(t, logs, sentinel)
	}
	assert.Contains(t, logs, `"error":"upstream token endpoint returned error status 400"`)

	assertSecurityEventsMarkPublicClient(t, logs, "session.oauth2.refresh_failed")
}

func TestForceRefreshSessionSecurity_PublicClientEmitsRedactedSuccessAudit(t *testing.T) {
	const (
		sentinelSecret       = "sentinel-client-secret"
		sentinelCode         = "sentinel-authorization-code"
		sentinelAccessToken  = "sentinel-access-token"
		sentinelRefreshToken = "sentinel-refresh-token"
	)

	logOutput := new(strings.Builder)
	service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(logOutput, nil)), 1)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  sentinelAccessToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": sentinelRefreshToken,
			})
		case "refresh_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "rotated-" + sentinelAccessToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "rotated-" + sentinelRefreshToken,
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
		}
	}))
	defer tokenServer.Close()

	provider := createTestService(serviceID)
	provider.ClientID = id.ClientID("public-client-id")
	provider.Endpoints.TokenEndpoint = tokenServer.URL
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	provider.AuthorizationParams = map[string]string{"audience": sentinelSecret}
	require.NoError(t, providerService.Create(context.Background(), provider))

	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)

	_, err = service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      sentinelCode,
		State:     flow.StateToken,
	})
	require.NoError(t, err)

	logOutput.Reset()
	refreshedSession, err := service.ForceRefreshSession(context.Background(), principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, refreshedSession)

	var auditEvent map[string]any
	logs := logOutput.String()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["event"] == "session.oauth2.token_refreshed" {
			auditEvent = entry
			break
		}
	}

	require.NotNil(t, auditEvent, "expected token refresh audit event")
	assert.Equal(t, serviceID.String(), auditEvent["service_id"])
	assert.Equal(t, true, auditEvent["public_client"])
	assert.Equal(t, "token_refresh_succeeded", auditEvent["reason"])
	for _, field := range []string{"client_secret", "client_assertion", "code", "code_verifier", "code_challenge", "access_token", "refresh_token"} {
		assert.NotContains(t, auditEvent, field)
	}
	for _, sentinel := range []string{sentinelSecret, sentinelCode, claims.PKCEVerifier, sentinelAccessToken, sentinelRefreshToken} {
		assert.NotContains(t, logs, sentinel)
	}
}
func newSecurityTestOAuth2SessionService(
	t *testing.T,
	logger *slog.Logger,
	maxRetries int,
) (*oauth2session.OAuth2SessionService, *thirdparty.ThirdpartyOAuth2ProviderService) {
	t.Helper()

	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))

	encryption := newTestEncryption(t)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		memory.NewInMemoryThirdpartyOAuth2ProviderRepository(),
		encryption,
		newNoopBranchKeyManager(),
		nil,
		false,
		logger,
	)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"
	config.MaxRetries = maxRetries
	config.RetryBaseDelay = time.Millisecond

	return oauth2session.NewOAuth2SessionService(
		providerService,
		memory.NewInMemoryUserSessionRepository(),
		memory.NewUserGrantRepository(),
		memory.NewAgentRepository(),
		encryption,
		&http.Client{},
		domjwe.New(key),
		config,
		logger,
	), providerService
}

func assertSecurityEventsMarkPublicClient(t *testing.T, logs string, wantedEvents ...string) {
	t.Helper()

	events := make(map[string]map[string]any, len(wantedEvents))
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}

		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		event, _ := entry["event"].(string)
		if event != "" {
			events[event] = entry
		}
	}

	for _, event := range wantedEvents {
		entry, found := events[event]
		require.True(t, found, "expected %s audit event", event)
		assert.Equal(t, true, entry["public_client"], "%s must identify the client as public", event)
	}
}

func assertPublicTokenExchangeRequest(
	t *testing.T,
	request capturedTokenExchangeRequest,
	clientID string,
	code string,
) {
	t.Helper()

	require.NoError(t, request.err)
	assert.Equal(t, clientID, request.body.Get("client_id"))
	assert.Equal(t, code, request.body.Get("code"))
	assert.NotEmpty(t, request.body.Get("code_verifier"))
	assert.NotContains(t, request.body, "client_secret")
	assert.Equal(t, http.MethodPost, request.method)
	assert.Empty(t, request.header.Values("Authorization"))
	assert.NotContains(t, request.query, "client_id")
	assert.NotContains(t, request.query, "client_secret")
}

func hasCredentialMaterial(request capturedTokenExchangeRequest) bool {
	if _, hasClientSecret := request.body["client_secret"]; hasClientSecret {
		return true
	}
	if len(request.header.Values("Authorization")) != 0 {
		return true
	}
	if _, hasClientIDQuery := request.query["client_id"]; hasClientIDQuery {
		return true
	}
	_, hasClientSecretQuery := request.query["client_secret"]
	return hasClientSecretQuery
}

func errorsJoin(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
