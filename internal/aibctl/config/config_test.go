package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/config"
)

const sampleConfig = `current_context: local
defaults:
  output: table
  timeout: 20s
contexts:
  - name: local
    server: http://localhost:14000
    auth:
      principal: admin@example.com
  - name: staging
    server: https://admin.example.com
    host_header: admin.internal
    headers:
      X-Tenant: platform
      X-Proxy-Token: ${AIBCTL_TEST_PROXY_TOKEN}
    auth:
      token_command: ["ztoken", "--audience", "broker"]
    output: json
    timeout: 5s
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func newCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	config.RegisterFlags(cmd)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return cmd
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	loaded, err := config.LoadFromViper(viper.New(), newCmd(t, "--config", path))
	require.NoError(t, err)
	assert.Equal(t, path, loaded.Path)
	assert.Empty(t, loaded.File.Contexts)
}

func TestLoadInvalidYAMLIsUsageError(t *testing.T) {
	path := writeConfig(t, "contexts: [")
	_, err := config.LoadFromViper(viper.New(), newCmd(t, "--config", path))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config.yaml")
}

func TestResolvePrecedence(t *testing.T) {
	path := writeConfig(t, sampleConfig)
	t.Setenv("AIBCTL_TEST_PROXY_TOKEN", "proxy-secret")

	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    func(t *testing.T, r *config.Resolved)
		wantErr string
	}{
		{
			name: "current context from file with defaults",
			want: func(t *testing.T, r *config.Resolved) {
				assert.Equal(t, "local", r.Name)
				assert.Equal(t, "http://localhost:14000", r.Server)
				assert.Equal(t, "admin@example.com", r.Principal)
				assert.Equal(t, "X-Remote-User", r.PrincipalHeader)
				assert.Equal(t, "table", r.Output)
				assert.Equal(t, 20*time.Second, r.Timeout)
				assert.Equal(t, 10*time.Second, r.TokenCommandTimeout)
			},
		},
		{
			name: "context overrides defaults and expands header env",
			args: []string{"--context", "staging"},
			want: func(t *testing.T, r *config.Resolved) {
				assert.Equal(t, "json", r.Output)
				assert.Equal(t, 5*time.Second, r.Timeout)
				assert.Equal(t, "admin.internal", r.HostHeader)
				assert.Equal(t, []string{"ztoken", "--audience", "broker"}, r.TokenCommand)
				assert.Equal(t, "proxy-secret", r.Headers["X-Proxy-Token"])
				assert.Equal(t, "platform", r.Headers["X-Tenant"])
			},
		},
		{
			name: "env overrides context",
			env:  map[string]string{"AIBCTL_CONTEXT": "staging", "AIBCTL_OUTPUT": "yaml", "AIBCTL_TOKEN": "env-token"},
			want: func(t *testing.T, r *config.Resolved) {
				assert.Equal(t, "staging", r.Name)
				assert.Equal(t, "yaml", r.Output)
				assert.Equal(t, "env-token", r.Token)
				assert.Empty(t, r.TokenCommand, "explicit token disables the token command")
			},
		},
		{
			name: "flags override env",
			env:  map[string]string{"AIBCTL_OUTPUT": "yaml", "AIBCTL_SERVER": "http://env:1"},
			args: []string{"-o", "json", "--server", "http://flag:2", "--show-uuid", "--timeout", "3s"},
			want: func(t *testing.T, r *config.Resolved) {
				assert.Equal(t, "json", r.Output)
				assert.Equal(t, "http://flag:2", r.Server)
				assert.True(t, r.ShowUUID)
				assert.Equal(t, 3*time.Second, r.Timeout)
			},
		},
		{
			name: "token command from env is split on whitespace",
			env:  map[string]string{"AIBCTL_CONTEXT": "local", "AIBCTL_TOKEN_COMMAND": "ztoken --audience x"},
			want: func(t *testing.T, r *config.Resolved) {
				assert.Equal(t, []string{"ztoken", "--audience", "x"}, r.TokenCommand)
			},
		},
		{
			name:    "principal header override collides with context header",
			args:    []string{"--context", "staging", "--principal-header", "X-Tenant"},
			wantErr: "the principal header is set from auth.principal",
		},
		{
			name:    "unknown context",
			args:    []string{"--context", "nope"},
			wantErr: `context "nope" not found`,
		},
		{
			name:    "invalid output format",
			args:    []string{"-o", "xml"},
			wantErr: "output",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			args := append([]string{"--config", path}, tc.args...)
			loaded, err := config.LoadFromViper(viper.New(), newCmd(t, args...))
			require.NoError(t, err)
			resolved, err := config.Resolve(loaded)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.want(t, resolved)
		})
	}
}

func TestResolveWithoutFileUsesServerFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	loaded, err := config.LoadFromViper(viper.New(), newCmd(t, "--config", path, "--server", "http://localhost:14000", "--principal", "me"))
	require.NoError(t, err)
	resolved, err := config.Resolve(loaded)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:14000", resolved.Server)
	assert.Equal(t, "me", resolved.Principal)
}

func TestResolveWithoutAnyServerFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	loaded, err := config.LoadFromViper(viper.New(), newCmd(t, "--config", path))
	require.NoError(t, err)
	_, err = config.Resolve(loaded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no context")
}

func TestResolveRequiresAuth(t *testing.T) {
	path := writeConfig(t, "current_context: a\ncontexts:\n  - name: a\n    server: http://x\n")
	loaded, err := config.LoadFromViper(viper.New(), newCmd(t, "--config", path))
	require.NoError(t, err)
	_, err = config.Resolve(loaded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth")
}

func TestResolveUnsetTokenEnvFails(t *testing.T) {
	path := writeConfig(t, "current_context: a\ncontexts:\n  - name: a\n    server: http://x\n    auth:\n      token: ${AIBCTL_TEST_DEFINITELY_UNSET}\n")
	loaded, err := config.LoadFromViper(viper.New(), newCmd(t, "--config", path))
	require.NoError(t, err)
	_, err = config.Resolve(loaded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AIBCTL_TEST_DEFINITELY_UNSET")
}

func TestValidateFile(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr string
	}{
		{name: "empty", cfg: config.Config{}},
		{
			name:    "duplicate names",
			cfg:     config.Config{Contexts: []config.Context{{Name: "a", Server: "http://x"}, {Name: "a", Server: "http://y"}}},
			wantErr: "duplicate",
		},
		{
			name:    "bad name",
			cfg:     config.Config{Contexts: []config.Context{{Name: "bad name", Server: "http://x"}}},
			wantErr: "name",
		},
		{
			name:    "current context missing",
			cfg:     config.Config{CurrentContext: "zzz"},
			wantErr: "current_context",
		},
		{
			name:    "server with path",
			cfg:     config.Config{Contexts: []config.Context{{Name: "a", Server: "http://x/api"}}},
			wantErr: "path",
		},
		{
			name:    "server with bad scheme",
			cfg:     config.Config{Contexts: []config.Context{{Name: "a", Server: "ftp://x"}}},
			wantErr: "scheme",
		},
		{
			name:    "token and token_command",
			cfg:     config.Config{Contexts: []config.Context{{Name: "a", Server: "http://x", Auth: config.AuthConfig{Token: "t", TokenCommand: []string{"c"}}}}},
			wantErr: "token_command",
		},
		{
			name:    "negative timeout",
			cfg:     config.Config{Defaults: config.Defaults{Timeout: -time.Second}},
			wantErr: "timeout",
		},
		{
			name:    "managed Prefer header",
			cfg:     config.Config{Contexts: []config.Context{{Name: "a", Server: "http://x", Headers: map[string]string{"prefer": "return=minimal"}}}},
			wantErr: "Prefer is set by aibctl",
		},
		{
			name:    "principal header spoof",
			cfg:     config.Config{Contexts: []config.Context{{Name: "a", Server: "http://x", Headers: map[string]string{"X-Remote-User": "spoof"}}}},
			wantErr: "the principal header is set from auth.principal",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := config.Validate(&tc.cfg)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSaveCreatesDirectoryAndRestrictsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "aibctl", "config.yaml")
	cfg := &config.Config{CurrentContext: "a", Contexts: []config.Context{{Name: "a", Server: "http://x", Auth: config.AuthConfig{Principal: "p"}}}}
	require.NoError(t, config.Save(path, cfg))

	reloaded, err := config.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, cfg, reloaded)

	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestContextMutations(t *testing.T) {
	cfg := &config.Config{}
	cfg.Upsert(config.Context{Name: "a", Server: "http://a"})
	cfg.Upsert(config.Context{Name: "b", Server: "http://b"})
	cfg.Upsert(config.Context{Name: "a", Server: "http://a2"})
	require.Len(t, cfg.Contexts, 2)
	got, ok := cfg.Find("a")
	require.True(t, ok)
	assert.Equal(t, "http://a2", got.Server)

	require.NoError(t, cfg.Use("b"))
	assert.Equal(t, "b", cfg.CurrentContext)
	assert.Error(t, cfg.Use("missing"))

	require.NoError(t, cfg.Delete("b"))
	assert.Empty(t, cfg.CurrentContext, "deleting the current context clears it")
	assert.Error(t, cfg.Delete("b"))
}

func TestRedacted(t *testing.T) {
	cfg := &config.Config{Contexts: []config.Context{{
		Name:    "a",
		Server:  "http://a",
		Headers: map[string]string{"X-Tenant": "t", "X-Proxy-Token": "secret", "Cookie": "c"},
		Auth:    config.AuthConfig{Token: "tok", TokenCommand: nil, Principal: "p"},
	}}}
	red := config.Redacted(cfg)
	assert.Equal(t, config.RedactedValue, red.Contexts[0].Auth.Token)
	assert.Equal(t, config.RedactedValue, red.Contexts[0].Headers["X-Proxy-Token"])
	assert.Equal(t, config.RedactedValue, red.Contexts[0].Headers["Cookie"])
	assert.Equal(t, "t", red.Contexts[0].Headers["X-Tenant"])
	assert.Equal(t, "p", red.Contexts[0].Auth.Principal)
	assert.Equal(t, "tok", cfg.Contexts[0].Auth.Token, "original is untouched")
}

func TestDefaultPathUsesUserConfigDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG semantics are linux-specific")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	path, err := config.DefaultPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/xdg/aibctl/config.yaml", path)

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/someone")
	path, err = config.DefaultPath()
	require.NoError(t, err)
	assert.Equal(t, "/home/someone/.config/aibctl/config.yaml", path)
}
