package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	policyReuseEnv    = "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL"
	policyAbsoluteEnv = "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME"
	policyIdleEnv     = "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL"
)

func policyPrincipal() id.Principal {
	return id.NewPrincipal(fixtures.DefaultPrincipal().String())
}

func policyCloseMemory(environment *bootstrap.RefreshEnvironment) {
	if environment == nil {
		return
	}
	environment.Stop()
	if environment.App != nil && environment.App.Shutdown != nil {
		Expect(environment.App.Shutdown(context.Background())).To(Succeed())
	}
	environment.Close()
}

func policyWaitUntil(deadline time.Time) {
	Eventually(time.Now, time.Until(deadline)+time.Second, 10*time.Millisecond).Should(BeTemporally(">=", deadline))
}

func policyAssertInvalidGrant(result *helpers.RefreshTokenResult) {
	Expect(result.Status).To(Equal(http.StatusBadRequest))
	Expect(result.Error).To(Equal("invalid_grant"))
	_, accessReturned := result.Body["access_token"]
	_, refreshReturned := result.Body["refresh_token"]
	Expect(accessReturned).To(BeFalse(), "denial cannot return an access token")
	Expect(refreshReturned).To(BeFalse(), "denial cannot return a refresh token")
}

// Unlike a static echo upstream, each refresh credential has exactly one valid
// rotation; consuming it creates a new credential and invalidates the prior one.
type policyUpstream struct {
	server     *httptest.Server
	mu         sync.Mutex
	valid      map[string]bool
	challenges map[string]string
	next       int
	rotated    int
}

func newPolicyUpstream() *policyUpstream {
	upstream := &policyUpstream{valid: make(map[string]bool), challenges: make(map[string]string)}
	_, publicKey, err := helpers.GenerateTestRSAKeyPair()
	Expect(err).NotTo(HaveOccurred())
	keys, err := helpers.GenerateJWKSFromPublicKey(publicKey)
	Expect(err).NotTo(HaveOccurred())
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration", "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": upstream.server.URL, "authorization_endpoint": upstream.server.URL + "/oauth/authorize",
				"token_endpoint": upstream.server.URL + "/oauth/token", "jwks_uri": upstream.server.URL + "/.well-known/jwks.json",
			})
		case "/.well-known/jwks.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(keys)
		case "/oauth/authorize":
			query := r.URL.Query()
			if query.Get("client_id") != "upstream-policy-client" || query.Get("redirect_uri") == "" {
				http.Error(w, "invalid upstream authorization", http.StatusBadRequest)
				return
			}
			upstream.mu.Lock()
			upstream.next++
			code := "upstream-code-" + strconv.Itoa(upstream.next)
			upstream.valid[code] = true
			upstream.challenges[code] = query.Get("code_challenge")
			upstream.mu.Unlock()
			location, err := url.Parse(query.Get("redirect_uri"))
			if err != nil {
				http.Error(w, "invalid redirect", http.StatusBadRequest)
				return
			}
			values := location.Query()
			values.Set("code", code)
			values.Set("state", query.Get("state"))
			location.RawQuery = values.Encode()
			http.Redirect(w, r, location.String(), http.StatusFound)
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			upstream.mu.Lock()
			key := r.FormValue("code")
			if r.FormValue("grant_type") == "refresh_token" {
				key = r.FormValue("refresh_token")
			}
			allowed := upstream.valid[key]
			if r.FormValue("grant_type") == "authorization_code" {
				allowed = allowed && upstream.challenges[key] != "" &&
					helpers.GenerateCodeChallenge(r.FormValue("code_verifier")) == upstream.challenges[key]
			}
			if allowed {
				delete(upstream.valid, key)
				delete(upstream.challenges, key)
				upstream.next++
				if r.FormValue("grant_type") == "refresh_token" {
					upstream.rotated++
				}
				upstream.valid["upstream-refresh-"+strconv.Itoa(upstream.next)] = true
			}
			serial := upstream.next
			upstream.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if !allowed {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "upstream-access-" + strconv.Itoa(serial),
				"refresh_token": "upstream-refresh-" + strconv.Itoa(serial),
				"token_type":    "Bearer", "expires_in": 600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return upstream
}

func (u *policyUpstream) rotations() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.rotated
}

func policyProxyAuthorization(ctx context.Context, brokerURL string, client fixtures.RefreshClient, upstreamURL string) *helpers.RefreshAuthorization {
	verifier := helpers.PKCEVerifier()
	query := url.Values{
		"response_type": {"code"}, "client_id": {client.Agent.ID.String()},
		"redirect_uri": {client.Agent.RedirectURIs[0]}, "state": {"upstream-policy"},
		"code_challenge":        {helpers.GenerateCodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	noFollow := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, brokerURL+"/oauth2/authorize?"+query.Encode(), nil)
	Expect(err).NotTo(HaveOccurred())
	request.Header.Set("X-Remote-User", client.Principal.String())
	response, err := noFollow.Do(request)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	_ = response.Body.Close()
	Expect(response.Header.Get("Location")).To(HavePrefix(upstreamURL + "/oauth/authorize"))
	upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, response.Header.Get("Location"), nil)
	Expect(err).NotTo(HaveOccurred())
	upstreamResponse, err := noFollow.Do(upstreamRequest)
	Expect(err).NotTo(HaveOccurred())
	Expect(upstreamResponse.StatusCode).To(Equal(http.StatusFound))
	_ = upstreamResponse.Body.Close()
	location, err := url.Parse(upstreamResponse.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	code := location.Query().Get("code")
	Expect(code).NotTo(BeEmpty())
	result, err := helpers.ExchangeRefreshCode(ctx, brokerURL, client, code, verifier)
	Expect(err).NotTo(HaveOccurred())
	Expect(result.Status).To(Equal(http.StatusOK))
	Expect(result.Tokens.RefreshToken).NotTo(BeEmpty())
	return &helpers.RefreshAuthorization{Code: code, Verifier: verifier, Tokens: result.Tokens}
}

var _ = Describe("Consent-Bound Refresh Sessions / US6", func() {
	// US6-S1 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S1 applies equivalent file, environment, CLI and rendered-chart policy to live binary refresh decisions", func() {
		ctx := context.Background()
		binary := policyCurrentBinary()
		jweKey := base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x"))
		chartEnv := map[string]string{
			"IDENTITY_BROKER_JWE_SIGNING_KEY":           jweKey,
			"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY": fixtures.TestKEKMaterialDeterministic(),
		}
		cases := []struct {
			name, contents string
			env            map[string]string
			flags          []string
		}{
			{name: "file", contents: policyConfigYAML("1s", "5s", "3s", "")},
			{name: "environment overrides YAML", contents: policyConfigYAML("20s", "0s", "720h", ""), env: map[string]string{
				policyReuseEnv: "1s", policyAbsoluteEnv: "5s", policyIdleEnv: "3s",
			}},
			{name: "CLI overrides environment and YAML", contents: policyConfigYAML("20s", "0s", "720h", ""), env: map[string]string{
				policyReuseEnv: "2s", policyAbsoluteEnv: "10s", policyIdleEnv: "8s",
			}, flags: []string{
				"--oauth2_authorization_server.local.refresh_token_reuse_interval=1s",
				"--oauth2_authorization_server.local.absolute_session_lifetime=5s",
				"--oauth2_authorization_server.local.refresh_token_ttl=3s",
			}},
			{name: "rendered Helm ConfigMap", contents: policyRenderChart("1s", "5s", "3s"), env: chartEnv},
		}
		for _, tc := range cases {
			By(tc.name + " issues and enforces a bounded local refresh session")
			broker := launchPolicyBroker(binary, policyConfigPath(tc.contents), tc.env, tc.flags...)
			Expect(helpers.ProvisionSigningKey(broker.AdminURL)).To(Succeed())
			client, issued := policyBinaryClient(ctx, broker)
			rotated, err := helpers.RotateRefreshSession(ctx, broker.EnduserURL, client, issued.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rotated.Status).To(Equal(http.StatusOK))
			rotatedAt := time.Now()
			retry, err := helpers.RotateRefreshSession(ctx, broker.EnduserURL, client, issued.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(retry.Status).To(Equal(http.StatusOK))
			Expect(retry.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue(), "a retry must return the original access token")
			Expect(retry.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue(), "a retry must return the original successor")
			policyWaitUntil(rotatedAt.Add(3200 * time.Millisecond))
			idle, err := helpers.RotateRefreshSession(ctx, broker.EnduserURL, client, rotated.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			policyAssertInvalidGrant(idle)

			other, second := policyBinaryClient(ctx, broker)
			initialAt := time.Now()
			policyWaitUntil(initialAt.Add(2500 * time.Millisecond))
			beforeAbsolute, err := helpers.RotateRefreshSession(ctx, broker.EnduserURL, other, second.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(beforeAbsolute.Status).To(Equal(http.StatusOK))
			policyWaitUntil(initialAt.Add(5200 * time.Millisecond))
			afterAbsolute, err := helpers.RotateRefreshSession(ctx, broker.EnduserURL, other, beforeAbsolute.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			policyAssertInvalidGrant(afterAbsolute)
			broker.Close()
		}
	})

	// US6-S2 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S2 keeps the existing refresh_token_ttl key tied to fresh rotation, not allowed duplicate replies", func() {
		ctx := context.Background()
		config, err := fixtures.RefreshConfig("2s", "0s", "5s")
		Expect(err).NotTo(HaveOccurred())
		environment, err := bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { policyCloseMemory(environment) })
		Expect(fixtures.SeedPlaceholderGrantData(ctx, environment.Storage)).To(Succeed())
		client, err := environment.AddClient(ctx, policyPrincipal(), true)
		Expect(err).NotTo(HaveOccurred())
		issued, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		issuedAt := time.Now()
		policyWaitUntil(issuedAt.Add(1200 * time.Millisecond))
		first, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Status).To(Equal(http.StatusOK))
		firstAt := time.Now()
		retry, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(retry.Status).To(Equal(http.StatusOK))
		Expect(retry.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())
		Expect(retry.Tokens.AccessToken == first.Tokens.AccessToken).To(BeTrue())
		policyWaitUntil(issuedAt.Add(5100 * time.Millisecond))
		Expect(time.Now()).To(BeTemporally("<", firstAt.Add(5*time.Second)))
		second, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Status).To(Equal(http.StatusOK), "a fresh rotation must move inactivity past initial issuance")
		secondAt := time.Now()
		retry, err = helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(retry.Status).To(Equal(http.StatusOK))
		Expect(retry.Tokens.RefreshToken == second.Tokens.RefreshToken).To(BeTrue())
		policyWaitUntil(secondAt.Add(5200 * time.Millisecond))
		ended, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, second.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(ended)
	})

	// US6-S3 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S3 rejects malformed and disallowed durations before either binary server becomes ready", func() {
		binary := policyCurrentBinary()
		control := launchPolicyBroker(binary, policyConfigPath(policyConfigYAML("1s", "0s", "720h", "")), nil)
		Expect(helpers.ProvisionSigningKey(control.AdminURL)).To(Succeed())
		control.Close()
		for _, tc := range []struct {
			name, reuse, absolute, inactivity, offendingKey string
		}{
			{"malformed reuse", "thirty", "0s", "720h", "refresh_token_reuse_interval"},
			{"negative inactivity", "1s", "0s", "-1s", "refresh_token_ttl"},
			{"reuse exceeds access TTL", "30s", "0s", "720h", "refresh_token_reuse_interval"},
			{"reuse exceeds inactivity", "3s", "0s", "2s", "refresh_token_reuse_interval"},
			{"absolute not longer than reuse", "1s", "1s", "720h", "absolute_session_lifetime"},
		} {
			By(tc.name)
			file := policyConfigPath(policyConfigYAML(tc.reuse, tc.absolute, tc.inactivity, ""))
			output, timedOut, err := policyRejectedStartup(binary, file, nil)
			Expect(timedOut).To(BeFalse(), "invalid configuration must fail rather than continue serving")
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(output, tc.offendingKey)).To(BeTrue(), "startup must name %s", tc.offendingKey)
		}
	})

	// US6-S5 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S5 denies revoked local authority in hybrid mode while upstream-controlled hybrid and proxy rotations continue", func() {
		ctx := context.Background()
		upstream := newPolicyUpstream()
		DeferCleanup(upstream.server.Close)
		config, err := fixtures.RefreshConfig("1s", "20s", "5s")
		Expect(err).NotTo(HaveOccurred())
		config.OAuth2AuthServer.Mode = "hybrid"
		config.OAuth2AuthServer.Proxy = fixtures.HybridConfig(upstream.server.URL).OAuth2AuthServer.Proxy
		environment, err := bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { policyCloseMemory(environment) })
		Expect(fixtures.SeedPlaceholderGrantData(ctx, environment.Storage)).To(Succeed())
		local, err := environment.AddClient(ctx, policyPrincipal(), true)
		Expect(err).NotTo(HaveOccurred())
		localIssued, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *local, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		localRotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *local, localIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(localRotated.Status).To(Equal(http.StatusOK))

		proxyAgent := fixtures.RefreshAgent()
		proxyAgent.ClientID = ptr.To(id.ClientID("upstream-policy-client"))
		Expect(environment.Storage.Agents().Create(ctx, proxyAgent)).To(Succeed())
		proxyGrant := fixtures.RefreshGrant(policyPrincipal(), proxyAgent.ID)
		Expect(environment.Storage.UserGrants().Create(ctx, proxyGrant)).To(Succeed())
		proxyClient := fixtures.RefreshClient{Agent: proxyAgent, Grant: proxyGrant, Principal: policyPrincipal()}
		proxied := policyProxyAuthorization(ctx, environment.Enduser.BaseURL(), proxyClient, upstream.server.URL)
		hybridIssuedAt := time.Now()
		hybridRotation, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), proxyClient, proxied.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(hybridRotation.Status).To(Equal(http.StatusOK))
		Expect(hybridRotation.Tokens.RefreshToken != proxied.Tokens.RefreshToken).To(BeTrue())
		Expect(upstream.rotations()).To(Equal(1))
		replayedUpstream, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), proxyClient, proxied.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(replayedUpstream)

		request, err := http.NewRequestWithContext(ctx, http.MethodDelete, environment.Enduser.BaseURL()+"/api/consent/agents/"+local.Agent.ID.String()+"/grants", nil)
		Expect(err).NotTo(HaveOccurred())
		request.Header.Set("X-Remote-User", local.Principal.String())
		response, err := helpers.HTTPClient().Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusNoContent))
		_ = response.Body.Close()
		blockedCurrent, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *local, localRotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(blockedCurrent)
		blockedRetry, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *local, localIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(blockedRetry)
		localPolicyClient, err := environment.AddClient(ctx, policyPrincipal(), true)
		Expect(err).NotTo(HaveOccurred())
		localPolicyIssued, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *localPolicyClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		localPolicyRotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *localPolicyClient, localPolicyIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(localPolicyRotated.Status).To(Equal(http.StatusOK))
		localPolicyAt := time.Now()
		localPolicyRetry, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *localPolicyClient, localPolicyIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(localPolicyRetry.Status).To(Equal(http.StatusOK))
		Expect(localPolicyRetry.Tokens.RefreshToken == localPolicyRotated.Tokens.RefreshToken).To(BeTrue())
		policyWaitUntil(localPolicyAt.Add(5200 * time.Millisecond))
		localIdle, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *localPolicyClient, localPolicyRotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(localIdle)
		policyWaitUntil(hybridIssuedAt.Add(5200 * time.Millisecond))
		upstreamResult, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), proxyClient, hybridRotation.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreamResult.Status).To(Equal(http.StatusOK))
		Expect(upstreamResult.Tokens.RefreshToken != hybridRotation.Tokens.RefreshToken).To(BeTrue())
		Expect(upstream.rotations()).To(Equal(2))

		proxyConfig := fixtures.OAuth2ConfigWithUpstream(upstream.server.URL)
		logger := bootstrap.TestLogger(slog.LevelInfo)
		proxyStore, err := bootstrap.NewStorageFactory(logger).NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = proxyStore.Close(ctx) })
		Expect(fixtures.SeedPlaceholderGrantData(ctx, proxyStore)).To(Succeed())
		proxyApp, err := bootstrap.NewServerFactory(proxyConfig, logger).BuildApp(proxyStore)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			if proxyApp.Shutdown != nil {
				Expect(proxyApp.Shutdown(ctx)).To(Succeed())
			}
		})
		proxyServer, err := bootstrap.NewEndUserTestServer(proxyApp, logger)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(proxyServer.Close)
		proxyOnlyAgent := fixtures.RefreshAgent()
		proxyOnlyAgent.ClientID = ptr.To(id.ClientID("upstream-policy-client"))
		Expect(proxyStore.Agents().Create(ctx, proxyOnlyAgent)).To(Succeed())
		proxyOnlyGrant := fixtures.RefreshGrant(policyPrincipal(), proxyOnlyAgent.ID)
		Expect(proxyStore.UserGrants().Create(ctx, proxyOnlyGrant)).To(Succeed())
		proxyOnlyClient := fixtures.RefreshClient{Agent: proxyOnlyAgent, Grant: proxyOnlyGrant, Principal: policyPrincipal()}
		proxyIssued := policyProxyAuthorization(ctx, proxyServer.BaseURL(), proxyOnlyClient, upstream.server.URL)
		proxyIssuedAt := time.Now()
		policyWaitUntil(proxyIssuedAt.Add(5200 * time.Millisecond))
		proxyRotated, err := helpers.RotateRefreshSession(ctx, proxyServer.BaseURL(), proxyOnlyClient, proxyIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyRotated.Status).To(Equal(http.StatusOK))
		Expect(proxyRotated.Tokens.RefreshToken != proxyIssued.Tokens.RefreshToken).To(BeTrue())
		Expect(upstream.rotations()).To(Equal(3))
	})

	// US6-S8 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S8 recovers an identical result on memory before a fresh store rejects both session credentials", func() {
		ctx := context.Background()
		config, err := fixtures.RefreshConfig("20s", "0s", "720h")
		Expect(err).NotTo(HaveOccurred())
		environment, err := bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { policyCloseMemory(environment) })
		Expect(fixtures.SeedPlaceholderGrantData(ctx, environment.Storage)).To(Succeed())
		client, err := environment.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		issued, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.Status).To(Equal(http.StatusOK))
		recovered, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered.Status).To(Equal(http.StatusOK))
		Expect(recovered.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue())
		Expect(recovered.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue())
		policyCloseMemory(environment)
		environment = nil

		fresh, err := bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { policyCloseMemory(fresh) })
		Expect(fixtures.SeedPlaceholderGrantData(ctx, fresh.Storage)).To(Succeed())
		Expect(fresh.Storage.Agents().Create(ctx, client.Agent)).To(Succeed())
		Expect(fresh.Storage.UserGrants().Create(ctx, client.Grant)).To(Succeed())
		control, err := helpers.AuthorizeRefreshSession(ctx, fresh.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		valid, err := helpers.RotateRefreshSession(ctx, fresh.Enduser.BaseURL(), *client, control.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(valid.Status).To(Equal(http.StatusOK))
		for _, lost := range []string{issued.Tokens.RefreshToken, rotated.Tokens.RefreshToken} {
			denied, err := helpers.RotateRefreshSession(ctx, fresh.Enduser.BaseURL(), *client, lost, "")
			Expect(err).NotTo(HaveOccurred())
			policyAssertInvalidGrant(denied)
		}
		binary := policyCurrentBinary()
		file := policyConfigPath(policyConfigYAML("20s", "0s", "720h", ""))
		oldProcess := launchPolicyBroker(binary, file, nil)
		Expect(helpers.ProvisionSigningKey(oldProcess.AdminURL)).To(Succeed())
		binaryClient, binaryIssued := policyBinaryClient(ctx, oldProcess)
		binaryRotated, err := helpers.RotateRefreshSession(ctx, oldProcess.EnduserURL, binaryClient, binaryIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(binaryRotated.Status).To(Equal(http.StatusOK))
		binaryRecovered, err := helpers.RotateRefreshSession(ctx, oldProcess.EnduserURL, binaryClient, binaryIssued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(binaryRecovered.Status).To(Equal(http.StatusOK))
		Expect(binaryRecovered.Tokens.AccessToken == binaryRotated.Tokens.AccessToken).To(BeTrue())
		Expect(binaryRecovered.Tokens.RefreshToken == binaryRotated.Tokens.RefreshToken).To(BeTrue())
		oldProcess.Close()

		newProcess := launchPolicyBroker(binary, file, nil)
		Expect(helpers.ProvisionSigningKey(newProcess.AdminURL)).To(Succeed())
		newClient, newSession := policyBinaryClient(ctx, newProcess)
		newValid, err := helpers.RotateRefreshSession(ctx, newProcess.EnduserURL, newClient, newSession.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(newValid.Status).To(Equal(http.StatusOK))
		for _, lost := range []string{binaryIssued.Tokens.RefreshToken, binaryRotated.Tokens.RefreshToken} {
			denied, err := helpers.RotateRefreshSession(ctx, newProcess.EnduserURL, binaryClient, lost, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(denied.Status).To(Equal(http.StatusUnauthorized))
			Expect(denied.Error).To(Equal("invalid_client"))
			_, accessReturned := denied.Body["access_token"]
			_, refreshReturned := denied.Body["refresh_token"]
			Expect(accessReturned).To(BeFalse())
			Expect(refreshReturned).To(BeFalse())
		}
		newProcess.Close()
	})
})
