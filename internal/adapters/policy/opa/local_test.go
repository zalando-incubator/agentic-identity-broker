package opa

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
}

func loadLocalSource(t *testing.T, dir string, logger *slog.Logger) (ports.ImpersonationPolicySource, error) {
	t.Helper()
	src, err := NewImpersonationPolicySource(ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{Path: dir}},
		[]ports.ImpersonationPolicyDecisionRef{governedDecision}, requireDecisions("governed"), logger)
	if src != nil {
		t.Cleanup(func() { src.Stop(context.Background()) })
	}
	return src, err
}

func TestLocalBundleIsAdmittedOnce(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"policy/governance.rego":     allowPolicy,
		"governance/data.json":       `{"allowed": true}`,
		"governance/extra/data.yaml": "flag: yes\n",
		".manifest":                  `{"revision": "local-7"}`,
	})
	var logs bytes.Buffer
	src, err := loadLocalSource(t, dir, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	require.NoError(t, awaitReady(t, src, time.Second))
	assert.Equal(t, "local=local-7", src.Current().Label())
	assert.Equal(t, ports.ImpersonationPolicyAllow, evaluate(t, src.Current()))
	assert.Equal(t, 1, strings.Count(logs.String(), "oauth2_server.impersonation_policy.local_mode"))
	assert.NotContains(t, logs.String(), "allowed")

	writeTree(t, dir, map[string]string{"governance/data.json": `{"allowed": false}`})
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, ports.ImpersonationPolicyAllow, evaluate(t, src.Current()))

	without := t.TempDir()
	writeTree(t, without, map[string]string{"p.rego": allowPolicy, "data.json": `{"governance": {"allowed": true}}`})
	src, err = loadLocalSource(t, without, discardLogger())
	require.NoError(t, err)
	require.NoError(t, awaitReady(t, src, time.Second))
	assert.Equal(t, "local=", src.Current().Label())
}

func TestLocalBundleRejections(t *testing.T) {
	expectConfigError := func(t *testing.T, err error, reason, file string) {
		t.Helper()
		var cfgErr *configError
		require.ErrorAs(t, err, &cfgErr)
		assert.Equal(t, "policy.path", cfgErr.field)
		assert.Equal(t, reason, cfgErr.reason)
		assert.Equal(t, file, cfgErr.file)
	}
	t.Run("unsupported file type", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"p.rego": allowPolicy, "notes/readme.md": secretValue})
		_, err := loadLocalSource(t, dir, discardLogger())
		expectConfigError(t, err, reasonUnsupportedFile, "notes/readme.md")
		assert.NotContains(t, err.Error(), secretValue)
	})
	t.Run("escaping link", func(t *testing.T) {
		base := t.TempDir()
		dir := filepath.Join(base, "bundle")
		writeTree(t, dir, map[string]string{"p.rego": allowPolicy})
		writeTree(t, base, map[string]string{"token": secretValue})
		require.NoError(t, os.Symlink("../token", filepath.Join(dir, "data.json")))
		_, err := loadLocalSource(t, dir, discardLogger())
		expectConfigError(t, err, reasonEscapingLink, "data.json")
	})
	t.Run("relative path", func(t *testing.T) {
		_, err := loadLocalSource(t, "bundle", discardLogger())
		expectConfigError(t, err, reasonPathNotAbsolute, "")
	})
	t.Run("the root directory", func(t *testing.T) {
		_, err := loadLocalSource(t, "/", discardLogger())
		expectConfigError(t, err, reasonRootDirectory, "")
	})
	t.Run("missing directory", func(t *testing.T) {
		_, err := loadLocalSource(t, filepath.Join(t.TempDir(), "missing"), discardLogger())
		expectConfigError(t, err, reasonUnreadable, "")
	})
	t.Run("too many files", func(t *testing.T) {
		dir := t.TempDir()
		files := map[string]string{}
		for i := range maxLocalFiles + 1 {
			files[filepath.Join("d", strings.Repeat("a", 1)+string(rune('a'+i%26))+strings.Repeat("b", i/26)+".rego")] = "package x\n"
		}
		writeTree(t, dir, files)
		_, err := loadLocalSource(t, dir, discardLogger())
		var cfgErr *configError
		require.ErrorAs(t, err, &cfgErr)
		assert.Equal(t, reasonTooManyFiles, cfgErr.reason)
	})
	t.Run("too large", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"data.json": `{"x": "` + strings.Repeat("a", maxLocalBytes) + `"}`})
		_, err := loadLocalSource(t, dir, discardLogger())
		var cfgErr *configError
		require.ErrorAs(t, err, &cfgErr)
		assert.Equal(t, reasonFileTooLarge, cfgErr.reason)
	})
	t.Run("rejected candidate fails readiness immediately", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"p.rego": "package other\n\nimport rego.v1\n\nresult := 1\n"})
		src, err := loadLocalSource(t, dir, discardLogger())
		require.NoError(t, err)
		start := time.Now()
		err = awaitReady(t, src, 10*time.Second)
		var notReady *ports.ImpersonationPolicyNotReadyError
		require.ErrorAs(t, err, &notReady)
		assert.Equal(t, string(reasonDecisionAbsent), notReady.Reason)
		assert.Less(t, time.Since(start), time.Second)
	})
}

func TestInvalidEngineConfigurationLogsStartupFailure(t *testing.T) {
	for reason, policy := range map[string]ports.ImpersonationOPAPolicySource{
		"bootstrap_invalid":    {ConfigFile: filepath.Join(t.TempDir(), "missing.yaml")},
		"local_bundle_invalid": {Path: filepath.Join(t.TempDir(), "missing")},
	} {
		var logs bytes.Buffer
		_, err := NewImpersonationPolicySource(ports.ImpersonationOPAEngineConfig{Policy: policy},
			[]ports.ImpersonationPolicyDecisionRef{governedDecision}, admitAll, slog.New(slog.NewJSONHandler(&logs, nil)))
		require.Error(t, err)
		assert.Contains(t, logs.String(), `"event":"oauth2_server.impersonation_policy.startup_failed"`)
		assert.Contains(t, logs.String(), `"reason":"`+reason+`"`)
	}
}
