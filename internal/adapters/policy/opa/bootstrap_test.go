package opa

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secretValue = "SYNTHETIC-SECRET-91c2"

const minimalBootstrap = `
services:
  governance:
    url: https://policies.example.com
bundles:
  impersonation:
    service: governance
    resource: bundles/impersonation.tar.gz
`

func writeBootstrapFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "bootstrap.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func expectBootstrapError(t *testing.T, err error, reason, key string) {
	t.Helper()
	require.Error(t, err)
	var cfgErr *configError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "policy.config_file", cfgErr.field)
	assert.Equal(t, reason, cfgErr.reason)
	assert.Equal(t, key, cfgErr.key)
	assert.NotContains(t, err.Error(), secretValue)
}

func TestReadBootstrapAcceptsTheAllowlist(t *testing.T) {
	tokenDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tokenDir, "token"), []byte("  token-value\n"), 0o600))
	content := `
services:
  bearer-svc:
    url: https://a.example.com
    headers: {X-Tenant: acme}
    response_header_timeout_seconds: 10
    credentials:
      bearer: {token: ` + secretValue + `, scheme: Bearer}
  oauth-svc:
    url: https://b.example.com
    credentials:
      oauth2:
        grant_type: client_credentials
        token_url: https://idp.example.com/token
        client_id: broker
        client_secret: ` + secretValue + `
        scopes: [bundles]
        additional_headers: {X-A: b}
        additional_parameters: {audience: bundles}
  s3-env:
    url: https://s3.example.com
    credentials:
      s3_signing: {environment_credentials: {}, service: s3, signature_version: "4"}
  s3-metadata:
    url: https://s3.example.com
    credentials:
      s3_signing: {metadata_credentials: {iam_role: reader, aws_region: eu-central-1}}
  s3-assume:
    url: https://s3.example.com
    credentials:
      s3_signing:
        assume_role_credentials:
          aws_region: eu-central-1
          iam_role_arn: arn:aws:iam::1:role/r
          session_name: broker
          aws_domain: amazonaws.com
          aws_signing: {environment_credentials: {}}
  gcp:
    url: https://storage.googleapis.com
    credentials:
      gcp_metadata: {audience: x, endpoint: http://metadata, access_token_path: /computeMetadata/v1/token, identity_token_path: /id, scopes: [a]}
  azure:
    url: https://blob.example.com
    credentials:
      azure_managed_identity: {endpoint: http://169.254.169.254, api_version: "2018", resource: r, object_id: o, client_id: c, mi_res_id: m, use_app_service_msi: false}
  token-file:
    url: https://c.example.com
    credentials:
      plugin: aib_bearer_token_file
plugins:
  aib_bearer_token_file:
    path: ` + filepath.Join(tokenDir, "token") + `
    scheme: Token
bundles:
  impersonation:
    service: bearer-svc
    resource: bundles/impersonation.tar.gz
    signing: {keyid: governance, scope: write, exclude_files: [x]}
    polling: {min_delay_seconds: 1, max_delay_seconds: 2, long_polling_timeout_seconds: 3}
    size_limit_bytes: 1048576
    trigger: periodic
    persist: false
  extra:
    service: oauth-svc
    resource: bundles/extra.tar.gz
keys:
  governance: {algorithm: RS256, key: "PEM", scope: write}
labels: {app: broker}
`
	boot, err := readBootstrap(writeBootstrapFile(t, t.TempDir(), content))
	require.NoError(t, err)
	assert.Equal(t, []string{"extra", "impersonation"}, boot.bundles)
	require.NotNil(t, boot.tokenFile)
	assert.Equal(t, filepath.Join(tokenDir, "token"), boot.tokenFile.path)
	assert.Equal(t, "Token", boot.tokenFile.scheme)

	json := `{"services": {"s": {"url": "https://a.example.com"}}, "bundles": {"b": {"service": "s", "resource": "r"}}}`
	boot, err = readBootstrap(writeBootstrapFile(t, t.TempDir(), json))
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, boot.bundles)
	assert.Nil(t, boot.tokenFile)
}

func TestReadBootstrapRejectsEveryOtherMember(t *testing.T) {
	cases := map[string]struct {
		content, reason, key string
	}{
		"decision logs":                  {minimalBootstrap + "decision_logs: {console: true}\n", reasonUnsupportedKey, "decision_logs"},
		"status":                         {minimalBootstrap + "status: {console: true}\n", reasonUnsupportedKey, "status"},
		"discovery":                      {minimalBootstrap + "discovery: {resource: x}\n", reasonUnsupportedKey, "discovery"},
		"server":                         {minimalBootstrap + "server: {encoding: {}}\n", reasonUnsupportedKey, "server"},
		"persistence":                    {minimalBootstrap + "persistence_directory: /tmp\n", reasonUnsupportedKey, "persistence_directory"},
		"caching":                        {minimalBootstrap + "caching: {}\n", reasonUnsupportedKey, "caching"},
		"tracing":                        {minimalBootstrap + "distributed_tracing: {}\n", reasonUnsupportedKey, "distributed_tracing"},
		"nd cache":                       {minimalBootstrap + "nd_builtin_cache: true\n", reasonUnsupportedKey, "nd_builtin_cache"},
		"storage":                        {minimalBootstrap + "storage: {}\n", reasonUnsupportedKey, "storage"},
		"default decision":               {minimalBootstrap + "default_decision: x\n", reasonUnsupportedKey, "default_decision"},
		"default authorization decision": {minimalBootstrap + "default_authorization_decision: x\n", reasonUnsupportedKey, "default_authorization_decision"},
		"other plugin":                   {minimalBootstrap + "plugins: {envoy_ext_authz_grpc: {addr: \":9191\"}}\n", reasonUnsupportedKey, "plugins.envoy_ext_authz_grpc"},
		"no bundles":                     {"services:\n  governance:\n    url: https://a.example.com\n", reasonNoBundles, ""},
		"empty bundles":                  {"services:\n  governance:\n    url: https://a.example.com\nbundles: {}\n", reasonNoBundles, ""},
		"persist":                        {strings.Replace(minimalBootstrap, "    resource:", "    persist: true\n    resource:", 1), reasonPersistUnsupported, "bundles.impersonation.persist"},
		"bearer token_path":              {servicesWith("credentials:\n      bearer:\n        token_path: /var/run/" + secretValue), reasonLocalFileUnsupported, "services.governance.credentials.bearer.token_path"},
		"oauth2 signing key":             {servicesWith("credentials:\n      oauth2:\n        token_url: https://idp\n        signing_key: k"), reasonUnsupportedKey, "services.governance.credentials.oauth2.signing_key"},
		"oauth2 assertion":               {servicesWith("credentials:\n      oauth2:\n        token_url: https://idp\n        client_assertion: " + secretValue), reasonUnsupportedKey, "services.governance.credentials.oauth2.client_assertion"},
		"oauth2 assertion path":          {servicesWith("credentials:\n      oauth2:\n        token_url: https://idp\n        client_assertion_path: /x"), reasonLocalFileUnsupported, "services.governance.credentials.oauth2.client_assertion_path"},
		"oauth2 jwt grant":               {servicesWith("credentials:\n      oauth2:\n        grant_type: jwt_bearer\n        token_url: https://idp"), reasonUnsupportedKey, "services.governance.credentials.oauth2.grant_type"},
		"s3 web identity":                {servicesWith("credentials:\n      s3_signing:\n        web_identity_credentials: {aws_region: x}"), reasonLocalFileUnsupported, "services.governance.credentials.s3_signing.web_identity_credentials"},
		"s3 profile":                     {servicesWith("credentials:\n      s3_signing:\n        profile_credentials: {path: /x}"), reasonLocalFileUnsupported, "services.governance.credentials.s3_signing.profile_credentials"},
		"s3 sso":                         {servicesWith("credentials:\n      s3_signing:\n        sso_credentials: {}"), reasonLocalFileUnsupported, "services.governance.credentials.s3_signing.sso_credentials"},
		"s3 nested web identity":         {servicesWith("credentials:\n      s3_signing:\n        assume_role_credentials:\n          aws_signing: {web_identity_credentials: {}}"), reasonLocalFileUnsupported, "services.governance.credentials.s3_signing.assume_role_credentials.aws_signing.web_identity_credentials"},
		"client tls":                     {servicesWith("credentials:\n      client_tls: {cert: /c, private_key: /k}"), reasonLocalFileUnsupported, "services.governance.credentials.client_tls"},
		"two credentials":                {servicesWith("credentials:\n      bearer: {token: a}\n      gcp_metadata: {}"), reasonInvalidDocument, "services.governance.credentials"},
		"other credential plugin":        {servicesWith("credentials:\n      plugin: other"), reasonUnsupportedKey, "services.governance.credentials.plugin"},
		"service tls":                    {servicesWith("tls: {ca_cert: /ca.pem}"), reasonLocalFileUnsupported, "services.governance.tls"},
		"allow insecure tls":             {servicesWith("allow_insecure_tls: true"), reasonUnsupportedKey, "services.governance.allow_insecure_tls"},
		"service type":                   {servicesWith("type: oci"), reasonUnsupportedKey, "services.governance.type"},
		"private key":                    {minimalBootstrap + "keys:\n  k: {algorithm: RS256, private_key: " + secretValue + "}\n", reasonUnsupportedKey, "keys.k.private_key"},
		"unsigned delta":                 {strings.Replace(minimalBootstrap, "    resource:", "    signing: {allow_unsigned_delta_bundles: true}\n    resource:", 1), reasonUnsupportedKey, "bundles.impersonation.signing.allow_unsigned_delta_bundles"},
		"file bundle resource":           {strings.Replace(minimalBootstrap, "resource: bundles/impersonation.tar.gz", "resource: file:///etc/bundle.tar.gz", 1), reasonLocalFileUnsupported, "bundles.impersonation.resource"},
		"file service url":               {strings.Replace(minimalBootstrap, "url: https://policies.example.com", "url: file:///etc", 1), reasonLocalFileUnsupported, "services.governance.url"},
		"unknown bundle member":          {strings.Replace(minimalBootstrap, "    resource:", "    trusted: true\n    resource:", 1), reasonUnsupportedKey, "bundles.impersonation.trusted"},
		"not an object":                  {"- services\n", reasonInvalidDocument, ""},
		"not yaml":                       {"services: [\n", reasonInvalidDocument, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := readBootstrap(writeBootstrapFile(t, t.TempDir(), tc.content))
			expectBootstrapError(t, err, tc.reason, tc.key)
		})
	}
}

func servicesWith(member string) string {
	return "services:\n  governance:\n    url: https://a.example.com\n    " + member + "\nbundles:\n  impersonation:\n    service: governance\n    resource: r\n"
}

func TestReadBootstrapContainsTheFile(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		_, err := readBootstrap("bootstrap.yaml")
		expectBootstrapError(t, err, reasonPathNotAbsolute, "")
	})
	t.Run("missing file", func(t *testing.T) {
		_, err := readBootstrap(filepath.Join(t.TempDir(), "missing.yaml"))
		expectBootstrapError(t, err, reasonUnreadable, "")
	})
	t.Run("a directory", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "bootstrap.yaml"), 0o700))
		_, err := readBootstrap(filepath.Join(dir, "bootstrap.yaml"))
		expectBootstrapError(t, err, reasonUnreadable, "")
	})
	t.Run("a file in the root directory", func(t *testing.T) {
		_, err := readBootstrap("/bootstrap.yaml")
		expectBootstrapError(t, err, reasonRootDirectory, "")
	})
	t.Run("oversized", func(t *testing.T) {
		big := minimalBootstrap + "labels:\n  padding: " + strings.Repeat("x", maxBootstrapBytes) + "\n"
		_, err := readBootstrap(writeBootstrapFile(t, t.TempDir(), big))
		expectBootstrapError(t, err, reasonFileTooLarge, "")
	})
	t.Run("a FIFO", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bootstrap.yaml")
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Skipf("mkfifo unsupported: %v", err)
		}
		_, err := readBootstrap(path)
		expectBootstrapError(t, err, reasonUnreadable, "")
	})

	base := t.TempDir()
	dir := filepath.Join(base, "config")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(base, "outside.yaml"), []byte(minimalBootstrap), 0o600))

	t.Run("a relative link that escapes", func(t *testing.T) {
		link := filepath.Join(dir, "escape.yaml")
		require.NoError(t, os.Symlink("../outside.yaml", link))
		_, err := readBootstrap(link)
		expectBootstrapError(t, err, reasonEscapingLink, "")
	})
	t.Run("an absolute link", func(t *testing.T) {
		link := filepath.Join(dir, "absolute.yaml")
		require.NoError(t, os.Symlink(filepath.Join(base, "outside.yaml"), link))
		_, err := readBootstrap(link)
		expectBootstrapError(t, err, reasonEscapingLink, "")
	})
	t.Run("a Kubernetes ..data link", func(t *testing.T) {
		generation := filepath.Join(dir, "..2026_11_26")
		require.NoError(t, os.Mkdir(generation, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(generation, "bootstrap.yaml"), []byte(minimalBootstrap), 0o600))
		require.NoError(t, os.Symlink("..2026_11_26", filepath.Join(dir, "..data")))
		require.NoError(t, os.Symlink("..data/bootstrap.yaml", filepath.Join(dir, "bootstrap.yaml")))
		boot, err := readBootstrap(filepath.Join(dir, "bootstrap.yaml"))
		require.NoError(t, err)
		assert.Equal(t, []string{"impersonation"}, boot.bundles)
	})
}

func TestReadBootstrapValidatesTheTokenFile(t *testing.T) {
	withPlugin := func(path string) string {
		return strings.Replace(minimalBootstrap, "    url: https://policies.example.com\n", "    url: https://policies.example.com\n    credentials:\n      plugin: aib_bearer_token_file\n", 1) +
			"plugins:\n  aib_bearer_token_file:\n    path: " + path + "\n"
	}
	expectTokenError := func(t *testing.T, err error, reason string) {
		t.Helper()
		var cfgErr *configError
		require.ErrorAs(t, err, &cfgErr)
		assert.Equal(t, "plugins.aib_bearer_token_file.path", cfgErr.field)
		assert.Equal(t, reason, cfgErr.reason)
		assert.NotContains(t, err.Error(), secretValue)
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte(secretValue), 0o600))
	escape := filepath.Join(dir, "escape")
	require.NoError(t, os.Symlink(outside, escape))

	for name, tc := range map[string]struct{ path, reason string }{
		"missing":       {filepath.Join(dir, "missing"), reasonUnreadable},
		"relative":      {"token", reasonPathNotAbsolute},
		"empty":         {write("empty", " \n"), reasonTokenInvalid},
		"too large":     {write("large", strings.Repeat("a", maxTokenBytes+1)), reasonFileTooLarge},
		"absolute link": {escape, reasonEscapingLink},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readBootstrap(writeBootstrapFile(t, t.TempDir(), withPlugin(tc.path)))
			expectTokenError(t, err, tc.reason)
		})
	}

	t.Run("plugin without settings", func(t *testing.T) {
		content := strings.Replace(minimalBootstrap, "    url: https://policies.example.com\n", "    url: https://policies.example.com\n    credentials:\n      plugin: aib_bearer_token_file\n", 1)
		_, err := readBootstrap(writeBootstrapFile(t, t.TempDir(), content))
		expectTokenError(t, err, reasonTokenInvalid)
	})
	t.Run("unknown plugin setting", func(t *testing.T) {
		_, err := readBootstrap(writeBootstrapFile(t, t.TempDir(), withPlugin(write("t", "x"))+"    refresh: 1s\n"))
		expectBootstrapError(t, err, reasonUnsupportedKey, "plugins.aib_bearer_token_file.refresh")
	})
}
