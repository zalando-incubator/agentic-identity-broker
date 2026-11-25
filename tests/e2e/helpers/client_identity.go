package helpers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/lestrrat-go/jwx/v4/jwk"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func (j *OAuth2CredentialJourney) StartBrowserDocument(document map[string]any, barriers ...*bootstrap.ClientIdentityReadBarrier) error {
	encoded, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if err := os.WriteFile(j.ConfigPath, encoded, 0o600); err != nil {
		return err
	}
	j.Config, err = bootstrap.LoadCredentialConfiguration(j.ConfigPath, nil, nil)
	if err != nil {
		return err
	}
	j.Storage, err = bootstrap.NewStorageFactory(j.Logger).NewTestStorage()
	if err != nil {
		return err
	}
	factory := bootstrap.NewServerFactory(j.Config, j.Logger)
	for _, barrier := range barriers {
		factory.WithClientIdentityReadBarrier(barrier)
	}
	builder, err := bootstrap.NewTestServerBuilder(j.Config, j.Storage, factory, j.Logger)
	if err != nil {
		return err
	}
	j.EndUser, err = builder.Build()
	if err != nil {
		return err
	}
	j.Application = j.EndUser.App()
	j.Admin, err = bootstrap.NewAdminTestServer(j.Application, j.Logger)
	return err
}

func ClientIdentityExchangeForm(journey *OAuth2CredentialJourney, agentID id.AgentID, principal string) (url.Values, error) {
	tokens, err := BuildTokenExchangeJWTs(journey.Upstream.GetPrivateKeyPEM(), journey.Upstream.URL(), "token-exchange-broker", principal, agentID.String(), "identity-gateway", time.Now())
	if err != nil {
		return nil, err
	}
	return url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {tokens.SubjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {tokens.ClientAssertion},
		"resource":              {journey.Upstream.URL() + "/resource"},
	}, nil
}

// TokenGrantBarrier pauses one matching provider request after capturing its response.
// Other grants and subsequent matching requests remain free to complete.
type TokenGrantBarrier struct {
	Entered   <-chan struct{}
	entered   chan struct{}
	released  chan struct{}
	grantType string
	release   func()
}

func (b *TokenGrantBarrier) Release() { b.release() }

func (m *MockUpstreamOAuth2Server) GateTokenGrant(grantType string) *TokenGrantBarrier {
	entered, released := make(chan struct{}), make(chan struct{})
	barrier := &TokenGrantBarrier{
		Entered: entered, entered: entered, released: released, grantType: grantType,
		release: sync.OnceFunc(func() { close(released) }),
	}
	m.requestMutex.Lock()
	m.grantBarrier = barrier
	m.requestMutex.Unlock()
	return barrier
}

func (m *MockUpstreamOAuth2Server) TokenGrantCount(grantType string) int {
	m.requestMutex.RLock()
	defer m.requestMutex.RUnlock()
	return m.tokenGrantCounts[grantType]
}

// RemoveClientIdentityStateClaims preserves the authentic flow's principal, PKCE,
// expiry, and return target while constructing a pre-upgrade state payload.
func RemoveClientIdentityStateClaims(journey *OAuth2CredentialJourney, callbackURL *url.URL, claims ...string) (*url.URL, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(journey.Config.ThirdPartyOAuth2.JWESigningKey)
	if err != nil {
		return nil, err
	}
	key, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		return nil, err
	}
	sealer := domjwe.New(key)
	query := callbackURL.Query()
	var payload map[string]any
	if err := sealer.Decrypt(query.Get("state"), &payload); err != nil {
		return nil, err
	}
	for _, claim := range claims {
		delete(payload, claim)
	}
	state, err := sealer.Encrypt(payload)
	if err != nil {
		return nil, err
	}
	query.Set("state", state)
	result := *callbackURL
	result.RawQuery = query.Encode()
	return &result, nil
}

func PublishCredentialPair(binding map[string]string, clientID, clientSecret string) error {
	directory := filepath.Dir(binding["client_id_file"])
	generation, err := os.MkdirTemp(directory, "generation-")
	if err != nil {
		return err
	}
	if err := os.Chmod(generation, 0o755); err != nil { // #nosec G302 -- the non-root E2E broker process must traverse this directory.
		return err
	}
	for name, value := range map[string]string{"identity": clientID, "credential": clientSecret} {
		if err := os.WriteFile(filepath.Join(generation, name), []byte(value), 0o644); err != nil { // #nosec G306 -- the non-root E2E broker process must read these fake credentials.
			return err
		}
	}
	next := filepath.Join(directory, "..data.next")
	if err := os.Symlink(filepath.Base(generation), next); err != nil {
		return err
	}
	return os.Rename(next, filepath.Join(directory, "..data"))
}

type ClientIdentityPriorRelease struct {
	*bootstrap.CredentialServer
	Container testcontainers.Container
	Image     string
}

func ClientIdentityPriorReleaseImage() string {
	if image := os.Getenv("CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE"); image != "" {
		return image
	}
	return fixtures.ClientIdentityPriorReleaseImage
}

// StartClientIdentityPriorRelease runs only the published binary. Loopback relays
// use the existing host tunnel so persisted issuer/endpoint URLs are identical on
// host and container. A non-empty credentialDirectory lets both brokers read
// credential publications from the same mounted directory.
func StartClientIdentityPriorRelease(ctx context.Context, credentialDirectory string, document map[string]any, hostPorts []int) (*ClientIdentityPriorRelease, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(encoded, &config); err != nil {
		return nil, err
	}
	servers := config["server"].(map[string]any)
	for name, port := range map[string]int{"enduser": 8000, "admin": 14000} {
		instance := servers[name].(map[string]any)
		instance["bind"], instance["port"] = "0.0.0.0", port
	}
	adminPublicURL, err := url.Parse(servers["admin"].(map[string]any)["public_url"].(string))
	if err != nil {
		return nil, err
	}
	encoded, err = json.Marshal(config)
	if err != nil {
		return nil, err
	}
	var script strings.Builder
	// BusyBox nc is part of the release's Alpine base; -ll forks per connection.
	for _, port := range hostPorts {
		if port < 1 || port > 65535 || port == 8000 || port == 14000 {
			return nil, fmt.Errorf("invalid prior-release host relay port %d", port)
		}
		fmt.Fprintf(&script, "/bin/busybox nc -ll -p %d -e /bin/busybox nc host.testcontainers.internal %d &\n", port, port)
	}
	// HostAccessPorts connects in PostReadies, so hold the binary until that hook completes.
	script.WriteString("mkfifo /tmp/client-identity-start\necho client-identity-relays-ready\ncat /tmp/client-identity-start > /dev/null\nexec /app/agentic-identity-broker --config /tmp/client-identity-config.json\n")
	image := ClientIdentityPriorReleaseImage()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:      image,
			Entrypoint: []string{"/bin/sh", "-ec"}, Cmd: []string{script.String()},
			ExposedPorts: []string{"8000/tcp", "14000/tcp"}, HostAccessPorts: hostPorts,
			HostConfigModifier: func(host *dockercontainer.HostConfig) {
				if credentialDirectory != "" {
					host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeBind, Source: credentialDirectory, Target: credentialDirectory, ReadOnly: true})
				}
			},
			Files:      []testcontainers.ContainerFile{{Reader: strings.NewReader(string(encoded)), ContainerFilePath: "/tmp/client-identity-config.json", FileMode: 0o644}},
			WaitingFor: wait.ForLog("client-identity-relays-ready").WithStartupTimeout(30 * time.Second),
		}, Started: true,
	})
	if err == nil {
		for _, port := range hostPorts {
			probe := wait.ForExec([]string{"/bin/sh", "-ec", fmt.Sprintf("grep -q ':%04X ' /proc/net/tcp /proc/net/tcp6", port)}).WithStartupTimeout(10 * time.Second)
			if err = probe.WaitUntilReady(ctx, container); err != nil {
				break
			}
		}
	}
	if err == nil {
		var code int
		code, _, err = container.Exec(ctx, []string{"/bin/sh", "-ec", "echo start > /tmp/client-identity-start"})
		if err == nil && code != 0 {
			err = fmt.Errorf("release prior-release startup barrier: exit %d", code)
		}
	}
	if err == nil {
		err = wait.ForAll(wait.ForHTTP("/health").WithPort("8000/tcp"), wait.ForHTTP("/health").WithPort("14000/tcp")).WithDeadline(90*time.Second).WaitUntilReady(ctx, container)
	}
	if err != nil {
		if container != nil {
			logs, logErr := container.Logs(ctx)
			if logErr == nil {
				output, _ := io.ReadAll(logs)
				_ = logs.Close()
				err = fmt.Errorf("start prior release %s: %w; logs: %s", image, err, output)
			}
			_ = container.Terminate(context.Background())
		}
		return nil, err
	}
	admin, err := container.PortEndpoint(ctx, "14000/tcp", "http")
	if err != nil {
		_ = container.Terminate(context.Background())
		return nil, err
	}
	endUser, err := container.PortEndpoint(ctx, "8000/tcp", "http")
	if err != nil {
		_ = container.Terminate(context.Background())
		return nil, err
	}
	return &ClientIdentityPriorRelease{CredentialServer: bootstrap.RemoteCredentialServer(admin, endUser, adminPublicURL.Host, servers["enduser"].(map[string]any)["public_url"].(string), fixtures.DefaultPrincipal().String()), Container: container, Image: image}, nil
}

func ClientIdentityHostPort(endpoint string) (int, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return 0, err
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(port)
}

func ClientIdentityCallbackClaims(journey *OAuth2CredentialJourney, callback *url.URL) (map[string]any, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(journey.Config.ThirdPartyOAuth2.JWESigningKey)
	if err != nil {
		return nil, err
	}
	key, err := jwk.Import[jwk.Key](keyBytes)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := domjwe.New(key).Decrypt(callback.Query().Get("state"), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
