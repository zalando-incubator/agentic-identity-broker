package app

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

type trustAnchorKey struct {
	private jwk.Key
	set     string
}

func newTrustAnchorKey(t *testing.T, kid string) trustAnchorKey {
	t.Helper()
	raw, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	private, err := jwk.Import[jwk.Key](raw)
	require.NoError(t, err)
	require.NoError(t, private.Set(jwk.KeyIDKey, kid))
	require.NoError(t, private.Set(jwk.AlgorithmKey, jwa.ES256()))
	public, err := private.PublicKey()
	require.NoError(t, err)
	set := jwk.NewSet()
	require.NoError(t, set.AddKey(public))
	doc, err := json.Marshal(set)
	require.NoError(t, err)
	return trustAnchorKey{private: private, set: string(doc)}
}

func (k trustAnchorKey) bearer(t *testing.T, issuer, audience, subject string) string {
	t.Helper()
	token, err := jwt.NewBuilder().Issuer(issuer).Audience([]string{audience}).Subject(subject).
		Expiration(time.Now().Add(time.Hour)).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), k.private))
	require.NoError(t, err)
	return "Bearer " + string(signed)
}

// countingIssuer is an issuer host that records every request and publishes nothing.
func countingIssuer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		http.NotFound(w, nil)
	}))
	t.Cleanup(server.Close)
	return server, &count
}

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) records(t *testing.T, message string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil && record["msg"] == message {
			out = append(out, record)
		}
	}
	return out
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func trustAnchorTestConfig(trustDir string) *ports.Config {
	preauth := ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}}
	return &ports.Config{
		Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server: ports.ServerConfig{
			EndUser:  ports.ServerInstanceConfig{Port: 8000, Bind: "::1", PublicURL: "http://localhost:8000", Authentication: preauth},
			Admin:    ports.ServerInstanceConfig{Port: 14000, Bind: "::1", PublicURL: "http://localhost:14000", Authentication: preauth},
			Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
		},
		Storage: ports.StorageConfig{
			Backend: "memory", Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		},
		Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{Mode: "local", Local: ports.LocalModeConfig{TokenTTL: time.Hour}},
		Security:         ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true, TrustMaterialDir: trustDir},
	}
}

func withClientAssertion(cfg *ports.Config, trust ports.ClientAssertionTrustConfig) *ports.Config {
	cfg.TokenExchange = ports.TokenExchangeConfig{
		ClientAssertion: trust,
		ClaimExtraction: ports.ClaimExtractionConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp"},
		Authorization: ports.AuthorizationConfig{
			Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true", EvaluationTimeout: time.Second},
		},
	}
	return cfg
}

func buildTrustAnchorApp(t *testing.T, cfg *ports.Config) (*App, *logBuffer, error) {
	t.Helper()
	store, err := storage.NewAdapter(&cfg.Storage)
	require.NoError(t, err)
	logs := &logBuffer{}
	app, err := NewBuilder().WithConfig(cfg).WithStorage(store).
		WithLogger(slog.New(slog.NewJSONHandler(logs, nil))).Build()
	if err == nil {
		t.Cleanup(func() { assert.NoError(t, app.Shutdown(context.Background())) })
	}
	return app, logs, err
}

func newTrustDir(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(base, "trust")
	require.NoError(t, os.Mkdir(dir, 0o755))
	return dir
}

func writeTrustFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestBuilderClientAssertionInlineKeySetSkipsDiscovery(t *testing.T) {
	issuer, requests := countingIssuer(t)
	gateway := newTrustAnchorKey(t, "gw-1")

	app, logs, err := buildTrustAnchorApp(t, withClientAssertion(trustAnchorTestConfig(""), ports.ClientAssertionTrustConfig{
		IssuerURI: issuer.URL, JWKS: gateway.set,
	}))

	require.NoError(t, err)
	assert.Zero(t, requests.Load(), "an inline key set needs no discovery or key fetch")
	loaded := logs.records(t, "trust anchor key set loaded")
	require.Len(t, loaded, 1)
	assert.Equal(t, "token_exchange.client_assertion", loaded[0]["trust_anchor"])
	assert.Equal(t, "inline", loaded[0]["key_source"])
	assert.EqualValues(t, 1, loaded[0]["key_count"])
	assert.Equal(t, []any{"gw-1"}, loaded[0]["key_ids"])
	assert.NotContains(t, logs.String(), gateway.set)
	assert.NotContains(t, app.EnduserHealthComponents(), "trust_anchor_key_files", "inline key sets never contribute")
}

func TestBuilderKeySourceErrors(t *testing.T) {
	issuer, requests := countingIssuer(t)
	gateway := newTrustAnchorKey(t, "gw-1")
	trustDir := newTrustDir(t)

	tests := []struct {
		name  string
		cfg   *ports.Config
		error string
	}{
		{name: "invalid inline document names the jwks setting",
			cfg:   withClientAssertion(trustAnchorTestConfig(""), ports.ClientAssertionTrustConfig{IssuerURI: issuer.URL, JWKS: `{"keys":[]}`}),
			error: "token_exchange.client_assertion.jwks: key set contains no keys"},
		{name: "conflicting sources name the trust anchor",
			cfg: withClientAssertion(trustAnchorTestConfig(""), ports.ClientAssertionTrustConfig{
				IssuerURI: issuer.URL, JWKSURI: issuer.URL + "/keys", JWKS: gateway.set,
			}),
			error: "token_exchange.client_assertion: only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)"},
		{name: "missing file names the trust anchor and path",
			cfg: withClientAssertion(trustAnchorTestConfig(trustDir), ports.ClientAssertionTrustConfig{
				IssuerURI: issuer.URL, JWKSFile: filepath.Join(trustDir, "missing.json"),
			}),
			error: `token_exchange.client_assertion: jwks_file "` + filepath.Join(trustDir, "missing.json") + `" does not exist`},
		{name: "a missing trust directory fails before any read",
			cfg: withClientAssertion(trustAnchorTestConfig(filepath.Join(trustDir, "absent")), ports.ClientAssertionTrustConfig{
				IssuerURI: issuer.URL, JWKSFile: filepath.Join(trustDir, "absent", "gw.json"),
			}),
			error: `security.trust_material_dir "` + filepath.Join(trustDir, "absent") + `" must be an absolute path to an existing directory other than /`},
		{name: "JWT pre-authentication document errors name the server",
			cfg: func() *ports.Config {
				cfg := trustAnchorTestConfig("")
				cfg.Server.Admin.Authentication.JWT = &ports.JWTConfig{
					HeaderName: "Authorization", Verification: "jwks", JWKS: `{"kty":"EC"}`,
					ClaimExtraction: ports.JWTClaimExtractionConfig{PrincipalExpression: "claims.sub"},
				}
				return cfg
			}(),
			error: "server.admin.authentication.jwt.jwks: key set is not a valid JWK Set document"},
		{name: "a later invalid anchor fails before an earlier anchor's discovery",
			cfg: func() *ports.Config {
				cfg := withClientAssertion(trustAnchorTestConfig(""), ports.ClientAssertionTrustConfig{IssuerURI: issuer.URL})
				cfg.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
					HeaderName: "Authorization", Verification: "none", JWKS: gateway.set,
					ClaimExtraction: ports.JWTClaimExtractionConfig{PrincipalExpression: "claims.sub"},
				}
				return cfg
			}(),
			error: "server.enduser.authentication.jwt: verification 'none' and jwks are mutually exclusive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := requests.Load()
			_, logs, err := buildTrustAnchorApp(t, tc.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.error)
			assert.Equal(t, before, requests.Load(), "no request to the issuer before key sources resolve")
			assert.NotContains(t, err.Error(), gateway.set)
			assert.NotContains(t, logs.String(), gateway.set)
		})
	}
}

func TestBuilderJWTLocalKeySetsPerServer(t *testing.T) {
	trustDir := newTrustDir(t)
	enduserKey, adminKey := newTrustAnchorKey(t, "enduser-1"), newTrustAnchorKey(t, "admin-1")
	cfg := trustAnchorTestConfig(trustDir)
	cfg.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
		HeaderName: "Authorization", Verification: "jwks", JWKS: enduserKey.set,
		ClaimExtraction: ports.JWTClaimExtractionConfig{PrincipalExpression: "claims.sub"},
	}
	cfg.Server.Admin.Authentication.JWT = &ports.JWTConfig{
		HeaderName: "Authorization", Verification: "jwks", JWKSFile: writeTrustFile(t, trustDir, "admin.json", adminKey.set),
		ClaimExtraction: ports.JWTClaimExtractionConfig{PrincipalExpression: "claims.sub"},
	}

	app, logs, err := buildTrustAnchorApp(t, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	user, err := app.EnduserJWTAuthenticator.Authenticate(ctx, enduserKey.bearer(t, "mesh", "broker", "alice"))
	require.NoError(t, err)
	assert.Equal(t, "alice", user.Principal)
	operator, err := app.AdminJWTAuthenticator.Authenticate(ctx, adminKey.bearer(t, "proxy", "admin", "operator"))
	require.NoError(t, err)
	assert.Equal(t, "operator", operator.Principal)

	_, err = app.AdminJWTAuthenticator.Authenticate(ctx, enduserKey.bearer(t, "mesh", "broker", "alice"))
	assert.Error(t, err, "the admin server verifies only against its own key set")
	_, err = app.EnduserJWTAuthenticator.Authenticate(ctx, adminKey.bearer(t, "proxy", "admin", "operator"))
	assert.Error(t, err, "the end-user server verifies only against its own key set")

	loaded := logs.records(t, "trust anchor key set loaded")
	require.Len(t, loaded, 2)
	byAnchor := map[any]map[string]any{loaded[0]["trust_anchor"]: loaded[0], loaded[1]["trust_anchor"]: loaded[1]}
	require.Contains(t, byAnchor, "server.enduser.authentication.jwt")
	require.Contains(t, byAnchor, "server.admin.authentication.jwt")
	assert.Equal(t, "inline", byAnchor["server.enduser.authentication.jwt"]["key_source"])
	assert.Equal(t, "file", byAnchor["server.admin.authentication.jwt"]["key_source"])
	assert.Equal(t, filepath.Join(trustDir, "admin.json"), byAnchor["server.admin.authentication.jwt"]["path"])

	assert.Equal(t, "healthy", app.EnduserHealthComponents()["trust_anchor_key_files"],
		"the end-user component aggregates the admin server's key-set file")
}

func TestBuilderKeySetFileHealth(t *testing.T) {
	issuer, _ := countingIssuer(t)
	trustDir := newTrustDir(t)
	gateway := newTrustAnchorKey(t, "gw-1")
	path := writeTrustFile(t, trustDir, "gw.json", gateway.set)

	app, _, err := buildTrustAnchorApp(t, withClientAssertion(trustAnchorTestConfig(trustDir), ports.ClientAssertionTrustConfig{
		IssuerURI: issuer.URL, JWKSFile: path, JWKSMinRefresh: 20 * time.Millisecond, JWKSMaxRefresh: time.Minute,
	}))
	require.NoError(t, err)
	assert.Equal(t, "healthy", app.EnduserHealthComponents()["trust_anchor_key_files"])

	require.NoError(t, os.WriteFile(path, []byte(`{"keys":[`), 0o644))
	require.Eventually(t, func() bool {
		return app.EnduserHealthComponents()["trust_anchor_key_files"] == "degraded"
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, os.WriteFile(path, []byte(gateway.set), 0o644))
	require.Eventually(t, func() bool {
		return app.EnduserHealthComponents()["trust_anchor_key_files"] == "healthy"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBuilderNoTrustDirectoryAccessWithoutKeySetFiles(t *testing.T) {
	_, _, err := buildTrustAnchorApp(t, trustAnchorTestConfig("/does/not/exist"))
	require.NoError(t, err, "an unused trust directory is never opened")
}

func TestImpersonationJWKSFactorySharesOnlyIdenticalSources(t *testing.T) {
	keyA, keyB := newTrustAnchorKey(t, "a"), newTrustAnchorKey(t, "b")
	issuerA := ports.TrustedTokenIssuerConfig{IssuerURI: "https://gateway.example.com", JWKS: keyA.set}
	issuerB := ports.TrustedTokenIssuerConfig{IssuerURI: "https://gateway.example.com", JWKS: keyB.set}
	require.NotEqual(t, impersonationSourceKeyOf(issuerA), impersonationSourceKeyOf(issuerB),
		"the same issuer with a different key source never shares a provider")
	assert.Equal(t, impersonationSourceKeyOf(issuerA), impersonationSourceKeyOf(issuerA))

	providerA, providerB := &stubKeyProvider{}, &stubKeyProvider{}
	factory := (&Builder{}).newImpersonationJWKSFactory(nil, map[impersonationSourceKey]tokenexchange.JWKSProvider{
		impersonationSourceKeyOf(issuerA): providerA,
		impersonationSourceKeyOf(issuerB): providerB,
	})

	gotA, err := factory(issuerA)
	require.NoError(t, err)
	gotB, err := factory(issuerB)
	require.NoError(t, err)
	assert.Same(t, providerA, gotA)
	assert.Same(t, providerB, gotB)
}

type stubKeyProvider struct{}

func (*stubKeyProvider) GetKeySet(context.Context) (jwk.Set, error) { return jwk.NewSet(), nil }

func (*stubKeyProvider) GetKey(context.Context, string) (jwk.Key, error) { return nil, nil }
