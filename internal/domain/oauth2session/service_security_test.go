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
	"sync/atomic"
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
	signer := &cimdAssertionSignerSpy{}
	service = service.WithCIMDAssertionSigner(signer)
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
	assert.Zero(t, assertionSignerCallCount(signer), "public code exchange behavior must not invoke the CIMD signer")
}

// T057: CIMD clients must use a fresh private_key_jwt assertion for every code-exchange attempt.
func TestHandleCallbackSecurity_CIMDClientReSignsEachRetryWithoutCredentialDowngrade(t *testing.T) {
	logOutput := new(strings.Builder)
	service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(logOutput, nil)), 2)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	clientID := cimdClientIDForService(serviceID)
	signer := &cimdAssertionSignerSpy{}
	service = service.WithCIMDAssertionSigner(signer)

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

		if attempt == 1 {
			http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"cimd-access-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	provider := createCIMDTestProvider(serviceID, tokenServer.URL)
	require.NoError(t, providerService.Create(context.Background(), provider))

	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)

	result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "cimd-authorization-code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	requestsMu.Lock()
	capturedRequests := append([]capturedTokenExchangeRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, capturedRequests, 2, "the transient failure must retry exactly once")

	for _, request := range capturedRequests {
		assertCIMDTokenExchangeRequest(t, request, clientID, "cimd-authorization-code")
		assert.Equal(t, claims.PKCEVerifier, request.body.Get("code_verifier"))
	}

	signedRequests := signer.Requests()
	require.Len(t, signedRequests, 2)
	for _, signedRequest := range signedRequests {
		assert.Equal(t, id.ClientID(clientID), signedRequest.clientID)
		assert.Equal(t, tokenServer.URL, signedRequest.tokenEndpoint)
	}
	assert.Len(t, map[string]struct{}{
		capturedRequests[0].body.Get("client_assertion"): {},
		capturedRequests[1].body.Get("client_assertion"): {},
	}, 2, "each retry must carry a freshly signed assertion")
	assertCIMDTokenAcquisitionAudit(t, logOutput.String(), serviceID, "code_exchange", "success")
}

// T057: A missing or rejected CIMD signer must prevent all token-endpoint traffic.
func TestHandleCallbackSecurity_CIMDClientFailsClosedBeforeTokenRequestWhenSigningUnavailable(t *testing.T) {
	tests := []struct {
		name      string
		signer    *cimdAssertionSignerSpy
		wantCalls int
	}{
		{
			name:      "missing signer",
			wantCalls: 0,
		},
		{
			name: "signer rejects assertion",
			signer: &cimdAssertionSignerSpy{
				err: fmt.Errorf("assertion signing unavailable"),
			},
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs strings.Builder
			service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(&logs, nil)), 2)
			if tt.signer != nil {
				service = service.WithCIMDAssertionSigner(tt.signer)
			}

			serviceID := id.NewServiceID()
			var tokenRequests atomic.Int64
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenRequests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer tokenServer.Close()

			provider := createCIMDTestProvider(serviceID, tokenServer.URL)
			require.NoError(t, providerService.Create(context.Background(), provider))

			principal := id.Principal("user@example.com")
			flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
			require.NoError(t, err)

			result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
				ServiceID: serviceID,
				Code:      "cimd-authorization-code",
				State:     flow.StateToken,
			})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, tt.wantCalls, assertionSignerCallCount(tt.signer))
			assert.Zero(t, tokenRequests.Load(), "signing must fail before a token request can leave the broker")
			assertCIMDTokenAcquisitionAudit(t, logs.String(), serviceID, "code_exchange", "rejected")
		})
	}
}

func TestRefreshAccessTokenSecurity_CIMDClientAuditsSuccessAndRejection(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var logs strings.Builder
		service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(&logs, nil)), 1)
		service = service.WithCIMDAssertionSigner(&cimdAssertionSignerSpy{})
		serviceID := id.NewServiceID()
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"cimd-access-token","token_type":"Bearer","expires_in":3600}`))
		}))
		defer tokenServer.Close()
		provider := createCIMDTestProvider(serviceID, tokenServer.URL)
		require.NoError(t, providerService.Create(context.Background(), provider))

		_, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
		require.NoError(t, err)
		assertCIMDTokenAcquisitionAudit(t, logs.String(), serviceID, "refresh", "success")
	})

	t.Run("empty refresh token", func(t *testing.T) {
		var logs strings.Builder
		service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(&logs, nil)), 1)
		serviceID := id.NewServiceID()
		provider := createCIMDTestProvider(serviceID, "https://issuer.example.com/oauth/token")
		require.NoError(t, providerService.Create(context.Background(), provider))

		_, err := service.RefreshAccessToken(context.Background(), provider, "")

		require.Error(t, err)
		assertCIMDTokenAcquisitionAudit(t, logs.String(), serviceID, "refresh", "rejected")
	})
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
	).WithCIMDPublicURL("https://broker.example.com").WithCIMDKeyReadiness(readyCIMDKeyReadiness{})

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

func assertCIMDTokenAcquisitionAudit(t *testing.T, logs string, serviceID id.ServiceID, operation, outcome string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] != "CIMD client token acquisition" {
			continue
		}
		if record["service_id"] != serviceID.String() || record["operation"] != operation || record["outcome"] != outcome {
			continue
		}
		allowed := map[string]struct{}{"time": {}, "level": {}, "msg": {}, "service_id": {}, "operation": {}, "outcome": {}}
		for field := range record {
			_, ok := allowed[field]
			assert.Truef(t, ok, "audit field %q must not be recorded", field)
		}
		for _, forbidden := range []string{"client_secret", "client_assertion", "authorization_code", "access_token", "refresh_token", "ciphertext", "private_key"} {
			assert.NotContains(t, logs, forbidden)
		}
		return
	}
	assert.Failf(t, "missing CIMD token acquisition audit", "service_id=%q operation=%q outcome=%q", serviceID, operation, outcome)
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

func assertCIMDTokenExchangeRequest(
	t *testing.T,
	request capturedTokenExchangeRequest,
	clientID string,
	code string,
) {
	t.Helper()

	require.NoError(t, request.err)
	assert.Equal(t, http.MethodPost, request.method)
	assert.Equal(t, clientID, request.body.Get("client_id"))
	assert.Equal(t, code, request.body.Get("code"))
	assert.NotEmpty(t, request.body.Get("code_verifier"))
	assert.Equal(t, []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, request.body["client_assertion_type"])
	require.Len(t, request.body["client_assertion"], 1)
	assert.NotEmpty(t, request.body.Get("client_assertion"))
	assert.NotContains(t, request.body, "client_secret")
	assert.Empty(t, request.header.Values("Authorization"))
	assert.Empty(t, request.query)
}

func cimdClientIDForService(serviceID id.ServiceID) string {
	return "https://broker.example.com/.well-known/oauth-client/" + serviceID.String()
}

func createCIMDTestProvider(serviceID id.ServiceID, tokenEndpoint string) *model.ThirdpartyOAuth2ProviderEntity {
	provider := createTestService(serviceID)
	provider.ClientID = ""
	provider.Endpoints.TokenEndpoint = tokenEndpoint
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodPrivateKeyJWT
	provider.Secret = model.NewAbsentSecret()
	return provider
}

type cimdAssertionSignerCall struct {
	clientID      id.ClientID
	tokenEndpoint string
}

type cimdAssertionSignerSpy struct {
	mu    sync.Mutex
	calls []cimdAssertionSignerCall
	err   error
}

func (s *cimdAssertionSignerSpy) SignClientAssertion(_ context.Context, clientID id.ClientID, tokenEndpoint string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, cimdAssertionSignerCall{
		clientID:      clientID,
		tokenEndpoint: tokenEndpoint,
	})
	if s.err != nil {
		return "", s.err
	}
	return fmt.Sprintf("signed-cimd-assertion-%d", len(s.calls)), nil
}

func (s *cimdAssertionSignerSpy) Requests() []cimdAssertionSignerCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]cimdAssertionSignerCall(nil), s.calls...)
}

func assertionSignerCallCount(signer *cimdAssertionSignerSpy) int {
	if signer == nil {
		return 0
	}
	return len(signer.Requests())
}
