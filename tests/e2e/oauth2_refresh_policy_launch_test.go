package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

// Build the actual command at test runtime; an external build can be supplied for
// reproducible rolling-upgrade runs. Only the old binary requires an explicit locator.
func policyCurrentBinary() string {
	if binary := os.Getenv("AIB_REFRESH_BROKER_BINARY"); binary != "" {
		return binary
	}
	_, file, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	root := filepath.Join(filepath.Dir(file), "..", "..")
	binary := filepath.Join(GinkgoT().TempDir(), "agentic-identity-broker")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/agentic-identity-broker")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "build the broker for a real startup: %s", output)
	return binary
}

func policyConfigYAML(reuse, absolute, inactivity, postgresURL string) string {
	storageConfig := "  backend: memory\n"
	if postgresURL != "" {
		storageConfig = "  backend: postgres\n  postgres:\n    connection_url: " + strconv.Quote(postgresURL) + "\n"
	}
	return fmt.Sprintf(`server:
  enduser:
    bind: "127.0.0.1"
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    bind: "127.0.0.1"
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  shutdown:
    timeout: 2s
storage:
%soauth2_authorization_server:
  mode: local
  local:
    token_ttl: 30s
    refresh_token_reuse_interval: %q
    absolute_session_lifetime: %q
    refresh_token_ttl: %q
third_party_oauth2:
  jwe_signing_key: %q
encryption:
  memory:
    raw_key: %q
`, storageConfig, reuse, absolute, inactivity,
		base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")),
		fixtures.TestKEKMaterialDeterministic())
}

func policyConfigPath(contents string) string {
	path := filepath.Join(GinkgoT().TempDir(), "broker.yaml")
	Expect(os.WriteFile(path, []byte(contents), 0o600)).To(Succeed())
	return path
}

func policyBrokerEnv(enduserPort, adminPort int, overrides map[string]string) []string {
	// Isolate delivery-source tests from the developer's broker environment.
	var values []string
	for _, pair := range os.Environ() {
		if !strings.HasPrefix(pair, "IDENTITY_BROKER_") && !strings.HasPrefix(pair, "APPROVAL_") {
			values = append(values, pair)
		}
	}
	values = append(values,
		"IDENTITY_BROKER_SERVER_ENDUSER_PORT="+strconv.Itoa(enduserPort),
		"IDENTITY_BROKER_SERVER_ADMIN_PORT="+strconv.Itoa(adminPort),
	)
	for key, value := range overrides {
		values = append(values, key+"="+value)
	}
	return values
}

func policyFreePort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	port := listener.Addr().(*net.TCPAddr).Port
	Expect(listener.Close()).To(Succeed())
	return port
}

type policyBroker struct {
	EnduserURL string
	AdminURL   string
	command    *exec.Cmd
	done       chan error
	stop       sync.Once
	output     bytes.Buffer
}

func launchPolicyBroker(binary, configPath string, overrides map[string]string, flags ...string) *policyBroker {
	enduserPort, adminPort := policyFreePort(), policyFreePort()
	for adminPort == enduserPort {
		adminPort = policyFreePort()
	}
	args := append([]string{"--config", configPath}, flags...)
	broker := &policyBroker{
		EnduserURL: fmt.Sprintf("http://127.0.0.1:%d", enduserPort),
		AdminURL:   fmt.Sprintf("http://127.0.0.1:%d", adminPort),
		done:       make(chan error, 1),
	}
	broker.command = exec.Command(binary, args...)
	broker.command.Env = policyBrokerEnv(enduserPort, adminPort, overrides)
	broker.command.Env = append(broker.command.Env, "IDENTITY_BROKER_CONFIG_PATH="+configPath)
	broker.command.Env = append(broker.command.Env,
		"IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL="+broker.EnduserURL,
		"IDENTITY_BROKER_SECURITY_SKIP_THIRDPARTY_HTTPS_VALIDATION=true",
	)
	broker.command.Stdout = &broker.output
	broker.command.Stderr = &broker.output
	Expect(broker.command.Start()).To(Succeed())
	go func() { broker.done <- broker.command.Wait() }()
	DeferCleanup(broker.Close)

	client := &http.Client{Timeout: 300 * time.Millisecond}
	Eventually(func() error {
		select {
		case err := <-broker.done:
			broker.done <- err
			return fmt.Errorf("broker exited before readiness: %v", err)
		default:
		}
		response, err := client.Get(broker.EnduserURL + "/.well-known/oauth-authorization-server")
		if err != nil {
			return err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("discovery returned %d", response.StatusCode)
		}
		return nil
	}, 12*time.Second, 50*time.Millisecond).Should(Succeed())
	return broker
}

func (b *policyBroker) Close() {
	b.stop.Do(func() {
		_ = b.command.Process.Signal(os.Interrupt)
		select {
		case <-b.done:
		case <-time.After(5 * time.Second):
			_ = b.command.Process.Kill()
			<-b.done
		}
	})
}

func policyRejectedStartup(binary, configPath string, overrides map[string]string, flags ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	enduserPort, adminPort := policyFreePort(), policyFreePort()
	for adminPort == enduserPort {
		adminPort = policyFreePort()
	}
	cmd := exec.CommandContext(ctx, binary, append([]string{"--config", configPath}, flags...)...)
	cmd.Env = policyBrokerEnv(enduserPort, adminPort, overrides)
	cmd.Env = append(cmd.Env, "IDENTITY_BROKER_CONFIG_PATH="+configPath)
	output, err := cmd.CombinedOutput()
	return string(output), ctx.Err() != nil, err
}

func policyRenderChart(reuse, absolute, inactivity string) string {
	trustAnchor := helpers.NewMockUpstreamOAuth2Server()
	DeferCleanup(trustAnchor.Close)
	_, file, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	root := filepath.Join(filepath.Dir(file), "..", "..")
	cmd := exec.Command("helm", "template", "refresh-policy", "charts/agentic-identity-broker",
		"--show-only", "templates/configmap.yaml",
		"--set", "broker.oauth2AuthorizationServer.mode=local",
		"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval="+reuse,
		"--set-string", "broker.oauth2AuthorizationServer.local.absoluteSessionLifetime="+absolute,
		"--set-string", "broker.oauth2AuthorizationServer.local.refreshTokenTtl="+inactivity,
		"--set-string", "broker.oauth2AuthorizationServer.local.tokenTtl=30s",
		"--set-string", "broker.server.enduser.authentication.preauth.principalHeaderName=X-Remote-User",
		"--set-string", "broker.server.admin.authentication.preauth.principalHeaderName=X-Remote-User",
		"--set-string", "broker.encryption.memory.rawKey=provided-by-test-env",
		"--set", "broker.security.skipThirdpartyHttpsValidation=true",
		"--set-string", "broker.tokenExchange.clientAssertion.issuerUri="+trustAnchor.URL(),
		"--set-string", "broker.tokenExchange.clientAssertion.jwksUri="+trustAnchor.URL()+"/.well-known/jwks.json")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "render the real chart: %s", output)
	var rendered struct {
		Data map[string]string `json:"data"`
	}
	Expect(yaml.Unmarshal(output, &rendered)).To(Succeed())
	Expect(rendered.Data).To(HaveKey("config.yaml"))
	return rendered.Data["config.yaml"]
}

// The binary has no test-only data injection path: create service, permission set,
// agent and consent through their actual HTTP APIs before code/PKCE issuance.
func policyBinaryClient(ctx context.Context, broker *policyBroker) (fixtures.RefreshClient, *helpers.RefreshAuthorization) {
	principal := policyPrincipal()
	upstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
	DeferCleanup(upstream.Close)
	transport := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	authorize := func(path string) *http.Response {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, broker.EnduserURL+path, nil)
		Expect(err).NotTo(HaveOccurred())
		request.Header.Set("X-Remote-User", principal.String())
		response, err := transport.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		_ = response.Body.Close()
		return response
	}
	post := func(path string, body any) map[string]any {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, broker.AdminURL+path, bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Remote-User", fixtures.AdminPrincipal().String())
		response, err := helpers.HTTPClient().Do(request)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusCreated), "POST %s", path)
		var resource map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&resource)).To(Succeed())
		if data, ok := resource["data"].(map[string]any); ok {
			return data
		}
		return resource
	}
	service := post("/api/services/", map[string]any{
		"display_name": "Refresh policy service", "client_id": "refresh-policy-service-" + id.NewServiceID().String(),
		"client_secret": "test-service-secret", "issuer_uri": upstream.URL(),
		"discovery": map[string]any{"enable_discovery": false},
		"endpoints": map[string]any{
			"authorize_endpoint": upstream.URL() + "/oauth/authorize", "token_endpoint": upstream.URL() + "/oauth/token",
		},
		"scopes": []map[string]string{{"scope_value": "read", "description": "Read"}},
	})
	serviceID, ok := service["id"].(string)
	Expect(ok).To(BeTrue())
	provider := authorize("/api/third-party/" + serviceID + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(broker.EnduserURL+"/delegations"))
	providerRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.Header.Get("Location"), nil)
	Expect(err).NotTo(HaveOccurred())
	providerResponse, err := transport.Do(providerRequest)
	Expect(err).NotTo(HaveOccurred())
	Expect(providerResponse.StatusCode).To(Equal(http.StatusFound))
	_ = providerResponse.Body.Close()
	callback, err := url.Parse(providerResponse.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	authorize(callback.RequestURI())
	set := post("/api/permission-sets/", map[string]any{
		"name": "Refresh policy " + serviceID, "description": "Required read permission",
		"service_scopes": []map[string]any{{"service_id": serviceID, "scopes": []string{"read"}, "requirement_type": "mandatory"}},
	})
	setID, ok := set["id"].(string)
	Expect(ok).To(BeTrue())
	agent := post("/api/agents/", map[string]any{
		"display_name": "Refresh policy client", "description": "Public local client",
		"redirect_uris":   []string{"https://client.example.com/callback"},
		"allowed_scopes":  []string{"read", "offline_access"},
		"permission_sets": []map[string]string{{"permission_set_id": setID, "requirement_type": "mandatory"}},
	})
	agentID, ok := agent["id"].(string)
	Expect(ok).To(BeTrue())
	parsedAgentID, err := id.ParseAgentID(agentID)
	Expect(err).NotTo(HaveOccurred())
	verifier := helpers.PKCEVerifier()
	query := url.Values{
		"client_id": {agentID}, "response_type": {"code"}, "scope": {"read offline_access"},
		"redirect_uri": {"https://client.example.com/callback"}, "state": {"policy-consent"},
		"code_challenge": {helpers.GenerateCodeChallenge(verifier)}, "code_challenge_method": {"S256"},
	}
	redirect := authorize("/oauth2/authorize?" + query.Encode())
	location, err := url.Parse(redirect.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	sessionToken := location.Query().Get("session_token")
	Expect(sessionToken).NotTo(BeEmpty())
	grantBody, err := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{setID: {serviceID}}})
	Expect(err).NotTo(HaveOccurred())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		broker.EnduserURL+"/api/consent/agents/"+agentID+"/grants?session_token="+url.QueryEscape(sessionToken), bytes.NewReader(grantBody))
	Expect(err).NotTo(HaveOccurred())
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Remote-User", principal.String())
	response, err := helpers.HTTPClient().Do(request)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	Expect(response.StatusCode).To(Equal(http.StatusCreated))
	var grant map[string]any
	Expect(json.NewDecoder(response.Body).Decode(&grant)).To(Succeed())
	resume, ok := grant["redirect_url"].(string)
	Expect(ok).To(BeTrue())
	resumed, err := url.Parse(resume)
	Expect(err).NotTo(HaveOccurred())
	codeResponse := authorize(resumed.RequestURI())
	issued, err := url.Parse(codeResponse.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	code := issued.Query().Get("code")
	Expect(code).NotTo(BeEmpty())
	client := fixtures.RefreshClient{
		Agent:     &storage.Agent{ID: parsedAgentID, RedirectURIs: []string{"https://client.example.com/callback"}},
		Principal: principal,
	}
	tokens, err := helpers.ExchangeRefreshCode(ctx, broker.EnduserURL, client, code, verifier)
	Expect(err).NotTo(HaveOccurred())
	Expect(tokens.Status).To(Equal(http.StatusOK))
	Expect(tokens.Tokens.RefreshToken).NotTo(BeEmpty())
	return client, &helpers.RefreshAuthorization{Code: code, Verifier: verifier, Tokens: tokens.Tokens}
}
