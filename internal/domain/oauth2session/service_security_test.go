package oauth2session_test

import (
	"context"
	"encoding/json"
	"errors"
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

func TestHandleCallbackSecurity_ProviderErrorValidatesStateBeforeRejecting(t *testing.T) {
	const hostile = "secret-<script>alert(1)</script>"
	principal, serviceID := id.Principal("owner@example.com"), id.NewServiceID()
	for _, tc := range []struct {
		name, state, callbackPrincipal, code string
		want                                 error
	}{
		{name: "invalid state", state: "forged", callbackPrincipal: principal.String(), code: "access_denied", want: oauth2session.ErrInvalidStateToken},
		{name: "wrong principal", callbackPrincipal: "other@example.com", code: "access_denied", want: oauth2session.ErrPrincipalMismatch},
		{name: "unknown error code", callbackPrincipal: principal.String(), code: hostile},
		{name: "known error code", callbackPrincipal: principal.String(), code: "access_denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := new(strings.Builder)
			service, _ := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(logs, nil)), 1)
			state := tc.state
			if state == "" {
				var err error
				state, err = service.CreateStateToken(&oauth2session.OAuth2StateTokenClaims{
					Principal: principal, ServiceID: serviceID, PKCEVerifier: strings.Repeat("a", 43),
					RedirectURI: "https://broker.example.com/sessions", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute),
				})
				require.NoError(t, err)
			}
			result, err := service.HandleCallback(context.Background(), id.Principal(tc.callbackPrincipal), &oauth2session.HandleCallbackRequest{
				ServiceID: serviceID, State: state, Error: tc.code, ErrorDesc: hostile,
			})
			require.Error(t, err)
			assert.Nil(t, result)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
				assert.NotContains(t, err.Error(), "OAuth2 authorization failed", "invalid state cannot accept a provider error")
			} else if tc.code == "access_denied" {
				assert.Contains(t, err.Error(), "access_denied")
			} else {
				assert.NotContains(t, err.Error(), tc.code)
			}
			assert.NotContains(t, err.Error(), hostile)
			assert.NotContains(t, logs.String(), hostile)
			assert.NotContains(t, logs.String(), state, "sealed state must not be logged")
		})
	}
}

func TestHandleCallbackSecurity_DiscoveredIssuerChangeRejectsOldStateBeforeTokenExchange(t *testing.T) {
	ctx := context.Background()
	var tokenCalls atomic.Int64
	client := &http.Client{Transport: tokenRoundTripFunc(func(*http.Request) (*http.Response, error) {
		tokenCalls.Add(1)
		return nil, fmt.Errorf("unexpected token request")
	})}
	service, repo, _, _, _, _ := setupServiceWithConfig(t, nil, client)
	service = service.WithDiscoveryTokenHTTPClient(client)
	serviceID := id.NewServiceID()
	storeDiscoveredCIMDProvider(t, repo, serviceID, "https://mcp.example.test/mcp")
	principal := id.Principal("owner@example.com")
	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://broker.example.com/sessions")
	require.NoError(t, err)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "https://auth.example.test", claims.IssuerURI)

	provider, err := repo.Get(ctx, serviceID)
	require.NoError(t, err)
	provider.IssuerURI = "https://other-issuer.example.test"
	completedAt := provider.DiscoveryStatus.LastAttemptAt.Add(time.Microsecond)
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	require.NoError(t, repo.Update(ctx, provider, nil))
	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID, Code: "old-issuer-code", State: flow.StateToken,
	})
	require.ErrorIs(t, err, oauth2session.ErrInvalidStateToken)
	assert.Nil(t, result)
	assert.Zero(t, tokenCalls.Load(), "old issuer code must never reach the changed token endpoint")
}

func TestHandleCallbackSecurity_DiscoveredAudienceChangeRejectsOldStateBeforeTokenExchange(t *testing.T) {
	const audienceA = "https://mcp.example.test/a"
	const audienceB = "https://mcp.example.test/b"
	ctx := context.Background()
	var tokenCalls atomic.Int64
	client := &http.Client{Transport: tokenRoundTripFunc(func(*http.Request) (*http.Response, error) {
		tokenCalls.Add(1)
		return nil, fmt.Errorf("unexpected token request")
	})}
	service, repo, sessions, _, _, _ := setupServiceWithConfig(t, nil, rejectingTokenHTTPClient())
	service = service.WithDiscoveryTokenHTTPClient(client)
	serviceID := id.NewServiceID()
	storeDiscoveredCIMDProvider(t, repo, serviceID, audienceA)
	principal := id.Principal("owner@example.com")
	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://broker.example.com/sessions")
	require.NoError(t, err)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "https://auth.example.test", claims.IssuerURI)
	assert.Equal(t, audienceA, claims.Resource)

	count, err := sessions.CountByService(ctx, serviceID)
	require.NoError(t, err)
	require.Zero(t, count)
	provider, err := repo.Get(ctx, serviceID)
	require.NoError(t, err)
	provider.AuthorizationParams["resource"] = audienceB
	provider.ResourceExplicit = true
	completedAt := provider.DiscoveryStatus.LastAttemptAt.Add(time.Microsecond)
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	require.NoError(t, repo.Update(ctx, provider, nil))
	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID, Code: "old-audience-code", State: flow.StateToken,
	})
	require.ErrorIs(t, err, oauth2session.ErrInvalidStateToken)
	assert.Nil(t, result)
	assert.Zero(t, tokenCalls.Load(), "old audience code must not reach the token endpoint")
	count, err = sessions.CountByService(ctx, serviceID)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestHandleCallbackSecurity_RejectsLegacyDiscoveredStateWithoutAudience(t *testing.T) {
	ctx := context.Background()
	var tokenCalls atomic.Int64
	client := &http.Client{Transport: tokenRoundTripFunc(func(*http.Request) (*http.Response, error) {
		tokenCalls.Add(1)
		return nil, fmt.Errorf("unexpected token request")
	})}
	service, repo, _, _, _, _ := setupServiceWithConfig(t, nil, rejectingTokenHTTPClient())
	service = service.WithDiscoveryTokenHTTPClient(client)
	serviceID := id.NewServiceID()
	storeDiscoveredCIMDProvider(t, repo, serviceID, "https://mcp.example.test/mcp")
	principal := id.Principal("owner@example.com")
	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://broker.example.com/sessions")
	require.NoError(t, err)
	claims, err := service.ValidateStateToken(flow.StateToken, principal, serviceID)
	require.NoError(t, err)
	require.Equal(t, "https://auth.example.test", claims.IssuerURI)
	claims.Resource = ""
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))
	legacyState, err := domjwe.New(key).Encrypt(claims)
	require.NoError(t, err)
	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID, Code: "legacy-code", State: legacyState,
	})
	require.ErrorIs(t, err, oauth2session.ErrInvalidStateToken)
	assert.Nil(t, result)
	assert.Zero(t, tokenCalls.Load(), "legacy state cannot authorize token exchange")
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

func TestRefreshAccessTokenSecurity_ResponseFailuresAuditOnlyCIMDClients(t *testing.T) {
	for _, failure := range []string{"drain read error", "oversized response"} {
		t.Run(failure, func(t *testing.T) {
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				const response = `{"access_token":"upstream-access-token","token_type":"Bearer"}`
				w.Header().Set("Content-Type", "application/json")
				if failure == "drain read error" {
					w.Header().Set("Content-Length", fmt.Sprint(len(response)+1))
				}
				_, _ = io.WriteString(w, response)
				if failure == "oversized response" {
					_, _ = io.WriteString(w, strings.Repeat(" ", 1<<20))
				}
			}))
			defer tokenServer.Close()

			for _, client := range []string{"CIMD", "public", "static confidential"} {
				t.Run(client, func(t *testing.T) {
					var logs strings.Builder
					service, _ := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewJSONHandler(&logs, nil)), 1)
					service = service.WithCIMDAssertionSigner(&cimdAssertionSignerSpy{})
					serviceID := id.NewServiceID()
					provider := createTestService(serviceID)
					provider.Endpoints.TokenEndpoint = tokenServer.URL
					switch client {
					case "CIMD":
						provider = createCIMDTestProvider(serviceID, tokenServer.URL)
						provider.ClientID = id.ClientID(cimdClientIDForService(serviceID))
					case "public":
						provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
						provider.Secret = model.NewAbsentSecret()
					}

					token, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
					require.Error(t, err)
					assert.Nil(t, token)
					if failure == "drain read error" {
						require.ErrorIs(t, err, io.ErrUnexpectedEOF)
					}
					if client == "CIMD" {
						assertCIMDTokenAcquisitionAudit(t, logs.String(), serviceID, "refresh", "rejected")
					} else {
						assert.NotContains(t, logs.String(), "CIMD client token acquisition")
					}
				})
			}
		})
	}
}

func TestRefreshAccessTokenSecurity_RejectsPersistedCIMDCredentialInTokenURL(t *testing.T) {
	service, _ := newSecurityTestOAuth2SessionService(t, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	service = service.WithCIMDAssertionSigner(&cimdAssertionSignerSpy{})
	var requests atomic.Int64
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"unexpected","token_type":"Bearer"}`))
	}))
	defer tokenServer.Close()

	provider := createCIMDTestProvider(id.NewServiceID(), tokenServer.URL+"?client_secret=stale-secret")
	provider.ClientID = id.ClientID(cimdClientIDForService(provider.ID))
	token, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
	require.ErrorContains(t, err, "client_secret")
	assert.Nil(t, token)
	assert.Zero(t, requests.Load(), "an invalid persisted CIMD configuration must not send a token request")
}

func TestHandleCallbackSecurity_RejectsPersistedCIMDCredentialBeforeCodeExchange(t *testing.T) {
	service, repository, _, _, _, providerService := setupServiceWithConfig(t, nil)
	providerService.WithCIMDPublicURL("https://broker.example.com")
	service = service.WithCIMDAssertionSigner(&cimdAssertionSignerSpy{})
	var requests atomic.Int64
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"unexpected","token_type":"Bearer"}`))
	}))
	defer tokenServer.Close()

	serviceID := id.NewServiceID()
	provider := createCIMDTestProvider(serviceID, tokenServer.URL)
	require.NoError(t, providerService.Create(context.Background(), provider))
	principal := id.Principal("user@example.com")
	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)

	stored, err := repository.Get(context.Background(), serviceID)
	require.NoError(t, err)
	stored.Endpoints.TokenEndpoint += "?client_secret=stale-secret"
	require.NoError(t, repository.Update(context.Background(), stored, nil))

	result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID, Code: "authorization-code", State: flow.StateToken,
	})
	require.ErrorContains(t, err, "client_secret")
	assert.Nil(t, result)
	assert.Zero(t, requests.Load(), "invalid persisted CIMD configuration must stop before code exchange")
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
	require.NotEmpty(t, capturedRequests)
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

type refreshLogContextKey struct{}

type contextRecordingHandler struct {
	mu       *sync.Mutex
	messages map[string]any
}

func (h contextRecordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h contextRecordingHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages[record.Message] = ctx.Value(refreshLogContextKey{})
	return nil
}
func (h contextRecordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h contextRecordingHandler) WithGroup(string) slog.Handler      { return h }

// Refresh-path logs must carry the operation context so the OTLP log bridge can correlate
// them with the request trace.
func TestRefreshLogsCarryOperationContext(t *testing.T) {
	recorder := contextRecordingHandler{mu: &sync.Mutex{}, messages: map[string]any{}}
	service, providerService := newSecurityTestOAuth2SessionService(t, slog.New(recorder), 1)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	var rejectRefresh atomic.Bool
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("grant_type") == "refresh_token" && rejectRefresh.Load() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"access","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh"}`))
	}))
	defer tokenServer.Close()

	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = tokenServer.URL
	require.NoError(t, providerService.Create(context.Background(), provider))
	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	_, err = service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), refreshLogContextKey{}, "request-marker")
	_, err = service.ForceRefreshSession(ctx, principal, serviceID)
	require.NoError(t, err)
	rejectRefresh.Store(true)
	_, err = service.ForceRefreshSession(ctx, principal, serviceID)
	require.ErrorIs(t, err, oauth2session.ErrRefreshFailed)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, message := range []string{"access token refreshed", "oauth2_token_refreshed", "oauth2_refresh_failed"} {
		assert.Equal(t, "request-marker", recorder.messages[message], "%s must be logged with the operation context", message)
	}
}

func TestGetValidAccessTokenRefreshLogsCarryOperationContext(t *testing.T) {
	recorder := contextRecordingHandler{mu: &sync.Mutex{}, messages: map[string]any{}}
	sessions := memory.NewInMemoryUserSessionRepository()
	service, providerService := newSecurityTestOAuth2SessionServiceWithSessions(t, slog.New(recorder), 1, sessions)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh"}`))
	}))
	defer tokenServer.Close()

	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = tokenServer.URL
	require.NoError(t, providerService.Create(context.Background(), provider))
	flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	_, err = service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)
	stored, err := sessions.FindByPrincipalAndService(context.Background(), principal, serviceID)
	require.NoError(t, err)
	expiredSession := *stored
	expired := time.Now().Add(-time.Hour)
	expiredSession.AccessTokenExpiresAt = &expired
	require.NoError(t, sessions.Create(context.Background(), &expiredSession))

	ctx := context.WithValue(context.Background(), refreshLogContextKey{}, "request-marker")
	_, _, err = service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, message := range []string{"access token refreshed", "oauth2_token_refreshed"} {
		assert.Equal(t, "request-marker", recorder.messages[message], "%s must be logged with the operation context", message)
	}
}

func TestDiscoveredDCRInvalidTargetKeepsExistingSessionAndNeverBroadensTokenRequest(t *testing.T) {
	const (
		resourceURL = "https://mcp.example.test/mcp"
		secret      = "registered-secret-not-for-errors"
		code        = "rejected-authorization-code-not-for-errors"
	)
	for _, tc := range []struct {
		name       string
		grantType  string
		statusCode int
	}{
		{name: "code exchange", grantType: "authorization_code", statusCode: http.StatusServiceUnavailable},
		{name: "renewal", grantType: "refresh_token", statusCode: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var reject atomic.Bool
			requests := make(chan capturedTokenExchangeRequest, 8)
			tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, readErr := io.ReadAll(r.Body)
				form, parseErr := url.ParseQuery(string(body))
				requests <- capturedTokenExchangeRequest{
					method: r.Method, body: form, header: r.Header.Clone(), query: r.URL.Query(),
					err: errorsJoin(readErr, parseErr),
				}
				w.Header().Set("Content-Type", "application/json")
				if reject.Load() && len(form["resource"]) == 0 {
					_, _ = io.WriteString(w, `{"access_token":"unbound-replacement-token","token_type":"Bearer","expires_in":3600}`)
					return
				}
				if reject.Load() {
					w.WriteHeader(tc.statusCode)
					_, _ = fmt.Fprintf(w, `{"error":"invalid_target","error_description":"%s %s"}`, secret, code)
					return
				}
				_, _ = io.WriteString(w, `{"access_token":"original-bound-access","token_type":"Bearer","refresh_token":"original-bound-refresh","expires_in":3600}`)
			}))
			defer tokenServer.Close()

			serviceID := id.NewServiceID()
			provider := storedDCRSessionProvider(serviceID, "https://auth.example.test/token", model.TokenEndpointAuthMethodClientSecretBasic, secret, resourceURL, resourceURL)
			service, repo, _ := newStoredDCRSessionService(t, mappedDiscoveryTokenTLSClient(t, tokenServer), provider)
			principal := id.Principal("user@example.com")
			flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://broker.example.com/sessions")
			require.NoError(t, err)
			connected, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
				ServiceID: serviceID, Code: "original-authorization-code", State: flow.StateToken,
			})
			require.NoError(t, err)
			require.NotNil(t, connected.Session)
			initial := <-requests
			require.NoError(t, initial.err)
			assert.Equal(t, []string{resourceURL}, initial.body["resource"])

			reject.Store(true)
			if tc.grantType == "authorization_code" {
				flow, err = service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://broker.example.com/sessions")
				require.NoError(t, err)
				replacement, callbackErr := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
					ServiceID: serviceID, Code: code, State: flow.StateToken,
				})
				require.Error(t, callbackErr)
				assert.Nil(t, replacement)
				assert.ErrorContains(t, callbackErr, "invalid_target")
				assert.NotContains(t, callbackErr.Error(), secret)
				assert.NotContains(t, callbackErr.Error(), code)
			} else {
				refreshed, refreshErr := service.ForceRefreshSession(ctx, principal, serviceID)
				require.ErrorIs(t, refreshErr, oauth2session.ErrRefreshFailed)
				assert.Nil(t, refreshed)
				var rejection *oauth2session.RefreshRejectedError
				if assert.ErrorAs(t, refreshErr, &rejection) {
					assert.Equal(t, "invalid_target", rejection.OAuthError)
				}
				assert.NotContains(t, refreshErr.Error(), secret)
				assert.NotContains(t, refreshErr.Error(), code)
			}

			require.Len(t, requests, 1, "a rejected audience must not trigger a second token request without resource")
			rejected := <-requests
			require.NoError(t, rejected.err)
			assert.Equal(t, http.MethodPost, rejected.method)
			assert.Equal(t, []string{tc.grantType}, rejected.body["grant_type"])
			assert.Equal(t, []string{resourceURL}, rejected.body["resource"])
			assert.Empty(t, rejected.query)
			clientID, sentSecret, usedBasic := (&http.Request{Header: rejected.header}).BasicAuth()
			assert.True(t, usedBasic)
			assert.Equal(t, provider.ClientID.String(), clientID)
			assert.Equal(t, secret, sentSecret)
			assert.NotContains(t, rejected.body, "client_secret")

			persisted, access, err := service.GetValidAccessToken(ctx, principal, serviceID)
			require.NoError(t, err)
			assert.Equal(t, connected.Session.ID, persisted.ID)
			assert.Equal(t, "original-bound-access", access)
			refresh, err := service.DecryptRefreshToken(ctx, persisted)
			require.NoError(t, err)
			assert.Equal(t, "original-bound-refresh", refresh)
			assert.Zero(t, repo.createCalls.Load(), "token failure must not re-register a client")
			assert.Zero(t, repo.registrationCalls.Load(), "token failure must not contact the registration endpoint")
		})
	}
}

func newSecurityTestOAuth2SessionService(
	t *testing.T,
	logger *slog.Logger,
	maxRetries int,
) (*oauth2session.OAuth2SessionService, *thirdparty.ThirdpartyOAuth2ProviderService) {
	t.Helper()
	return newSecurityTestOAuth2SessionServiceWithSessions(t, logger, maxRetries, memory.NewInMemoryUserSessionRepository())
}

func newSecurityTestOAuth2SessionServiceWithSessions(
	t *testing.T,
	logger *slog.Logger,
	maxRetries int,
	sessionRepo *memory.InMemoryUserSessionRepository,
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
		sessionRepo,
		sessionRepo,
		memory.NewUserGrantRepository(),
		memory.NewAgentRepository(),
		encryption,
		&http.Client{},
		domjwe.New(key),
		config,
		logger,
	), providerService
}

func TestHandleCallbackSecurity_DiscoveredTransportErrorOmitsURLQueryAndSecrets(t *testing.T) {
	const (
		marker = "provider-query-marker-private"
		secret = "provider-transport-secret-private"
		code   = "authorization-code-private"
	)
	for _, method := range []string{"dcr", "cimd"} {
		t.Run(method, func(t *testing.T) {
			var logs strings.Builder
			var tokenCalls atomic.Int64
			upstreamCause := fmt.Errorf("upstream failure: %s %s", marker, secret)
			client := &http.Client{Transport: tokenRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				tokenCalls.Add(1)
				assert.Equal(t, "https", request.URL.Scheme)
				assert.Equal(t, marker, request.URL.Query().Get("trace"))
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				form, err := url.ParseQuery(string(body))
				require.NoError(t, err)
				assert.Equal(t, "https://mcp.example.test/mcp", form.Get("resource"))
				return nil, upstreamCause
			})}
			service, providers, sessions := newDiscoveredSecurityTestService(t, slog.New(slog.NewJSONHandler(&logs, nil)), client)
			if method == "cimd" {
				service.WithCIMDAssertionSigner(&cimdAssertionSignerSpy{})
			}
			serviceID := id.NewServiceID()
			storeSecurityDiscoveryProvider(t, providers, serviceID, method, "https://auth.example.test/token?trace="+marker)
			principal := id.Principal("owner@example.com")
			flow, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://broker.example.com/sessions")
			require.NoError(t, err)
			result, err := service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
				ServiceID: serviceID, Code: code, State: flow.StateToken,
			})
			require.ErrorIs(t, err, oauth2session.ErrTokenExchange)
			assert.NotErrorIs(t, err, upstreamCause)
			assert.Nil(t, result)
			assert.Positive(t, tokenCalls.Load(), "the transport failure must occur after a token request")
			stored, lookupErr := sessions.FindByPrincipalAndService(context.Background(), principal, serviceID)
			require.NoError(t, lookupErr)
			assert.Nil(t, stored)
			for _, private := range []string{marker, secret, code} {
				assert.NotContains(t, err.Error(), private)
				assert.NotContains(t, logs.String(), private)
			}
		})
	}
}

type failedDiscoveredTokenBody struct {
	sent bool
	err  error
}

func (b *failedDiscoveredTokenBody) Read(p []byte) (int, error) {
	if b.sent {
		return 0, b.err
	}
	b.sent = true
	return copy(p, `{"access_token":"unexpected-rotation","token_type":"Bearer"}`), nil
}

func TestForceRefreshSessionSecurity_DiscoveredFailuresKeepTokensAndRedactUntrustedErrors(t *testing.T) {
	const (
		marker        = "refresh-query-marker-private"
		secret        = "refresh-provider-secret-private"
		originalCode  = "original-code-private"
		originalToken = "original-bound-access"
		originalRenew = "original-bound-refresh"
	)
	for _, failure := range []string{"transport", "body read"} {
		t.Run(failure, func(t *testing.T) {
			var logs strings.Builder
			var refreshCalls atomic.Int64
			var fail atomic.Bool
			upstreamCause := fmt.Errorf("upstream failure: %s %s", marker, secret)
			client := &http.Client{Transport: tokenRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				assert.Equal(t, marker, request.URL.Query().Get("trace"))
				body, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				form, err := url.ParseQuery(string(body))
				require.NoError(t, err)
				assert.Equal(t, "https://mcp.example.test/mcp", form.Get("resource"))
				if form.Get("grant_type") == "authorization_code" && !fail.Load() {
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{"access_token":"original-bound-access","refresh_token":"original-bound-refresh","token_type":"Bearer","expires_in":3600}`))}, nil
				}
				refreshCalls.Add(1)
				assert.Equal(t, "refresh_token", form.Get("grant_type"))
				assert.Equal(t, originalRenew, form.Get("refresh_token"))
				if failure == "transport" {
					return nil, upstreamCause
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(&failedDiscoveredTokenBody{err: upstreamCause})}, nil
			})}
			service, providers, sessions := newDiscoveredSecurityTestService(t, slog.New(slog.NewJSONHandler(&logs, nil)), client)
			service.WithCIMDAssertionSigner(&cimdAssertionSignerSpy{})
			serviceID := id.NewServiceID()
			storeSecurityDiscoveryProvider(t, providers, serviceID, "cimd", "https://auth.example.test/token?trace="+marker)
			principal := id.Principal("owner@example.com")
			ctx := context.Background()
			flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://broker.example.com/sessions")
			require.NoError(t, err)
			connected, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
				ServiceID: serviceID, Code: originalCode, State: flow.StateToken,
			})
			require.NoError(t, err)
			require.NotNil(t, connected.Session)
			logs.Reset()
			fail.Store(true)
			refreshed, err := service.ForceRefreshSession(ctx, principal, serviceID)
			require.ErrorIs(t, err, oauth2session.ErrRefreshFailed)
			assert.NotErrorIs(t, err, upstreamCause)
			assert.Nil(t, refreshed)
			assert.EqualValues(t, 1, refreshCalls.Load())
			for _, private := range []string{marker, secret, originalCode, originalToken, originalRenew} {
				assert.NotContains(t, err.Error(), private)
				assert.NotContains(t, logs.String(), private)
			}
			stored, lookupErr := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, lookupErr)
			require.NotNil(t, stored)
			assert.Equal(t, connected.Session.ID, stored.ID)
			_, access, accessErr := service.GetValidAccessToken(ctx, principal, serviceID)
			require.NoError(t, accessErr)
			assert.Equal(t, originalToken, access)
			renew, renewErr := service.DecryptRefreshToken(ctx, stored)
			require.NoError(t, renewErr)
			assert.Equal(t, originalRenew, renew)
		})
	}
}

func TestRefreshAccessTokenSecurity_DiscoveredFailuresPreserveContextErrors(t *testing.T) {
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			var calls atomic.Int64
			client := &http.Client{Transport: tokenRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, want
			})}
			service, _, _ := newDiscoveredSecurityTestService(t, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
			provider := storedDCRSessionProvider(id.NewServiceID(), "https://auth.example.test/token", model.TokenEndpointAuthMethodNone, "", "https://mcp.example.test/mcp", "https://mcp.example.test/mcp")
			token, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
			require.ErrorIs(t, err, want)
			assert.Nil(t, token)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestRefreshAccessTokenSecurity_DiscoveredUnsafeEndpointRejectedBeforeRequest(t *testing.T) {
	const marker = "unsafe-provider-query-private"
	var calls atomic.Int64
	client := &http.Client{Transport: tokenRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unsafe destination was contacted")
	})}
	service, _, _ := newDiscoveredSecurityTestService(t, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
	for _, endpoint := range []string{
		"http://auth.example.test/token?trace=" + marker,
		"https://127.0.0.1/token?trace=" + marker,
	} {
		provider := storedDCRSessionProvider(id.NewServiceID(), endpoint, model.TokenEndpointAuthMethodNone, "", "https://mcp.example.test/mcp", "https://mcp.example.test/mcp")
		token, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
		require.Error(t, err)
		assert.Nil(t, token)
		assert.NotContains(t, err.Error(), marker)
	}
	assert.Zero(t, calls.Load(), "unsafe discovery endpoints must never reach the token client")
}

func newDiscoveredSecurityTestService(t *testing.T, logger *slog.Logger, client *http.Client) (*oauth2session.OAuth2SessionService, *memory.InMemoryThirdpartyOAuth2ProviderRepository, *memory.InMemoryUserSessionRepository) {
	t.Helper()
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))

	sessions := memory.NewInMemoryUserSessionRepository()
	providers := memory.NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(sessions)
	encryption := newTestEncryption(t)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(providers, encryption, newNoopBranchKeyManager(), nil, false, logger)
	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"
	config.MaxRetries = 2
	config.RetryBaseDelay = time.Millisecond
	service := oauth2session.NewOAuth2SessionService(
		providerService, sessions, sessions, memory.NewUserGrantRepository(), memory.NewAgentRepository(),
		encryption, rejectingTokenHTTPClient(), domjwe.New(key), config, logger,
	).WithDiscoveryTokenHTTPClient(client)
	return service, providers, sessions
}

func storeSecurityDiscoveryProvider(t *testing.T, repo *memory.InMemoryThirdpartyOAuth2ProviderRepository, serviceID id.ServiceID, method, endpoint string) {
	t.Helper()
	const resource = "https://mcp.example.test/mcp"
	if method == "cimd" {
		storeDiscoveredCIMDProvider(t, repo, serviceID, resource)
		provider, err := repo.Get(context.Background(), serviceID)
		require.NoError(t, err)
		provider.Endpoints.TokenEndpoint = endpoint
		completedAt := provider.DiscoveryStatus.LastAttemptAt.Add(time.Microsecond)
		provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
		require.NoError(t, repo.Update(context.Background(), provider, nil))
		return
	}
	provider := storedDCRSessionProvider(serviceID, endpoint, model.TokenEndpointAuthMethodNone, "", resource, resource)
	readyAt := time.Now().UTC()
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &readyAt, LastSuccessAt: &readyAt}
	require.NoError(t, repo.Create(context.Background(), provider))
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
