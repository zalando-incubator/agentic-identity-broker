package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
)

func TestAgentSubjectExpressionCLI(t *testing.T) {
	const name = "token_exchange.claim_extraction.agent_subject_expression"
	const environment = "IDENTITY_BROKER_TOKEN_EXCHANGE_CLAIM_EXTRACTION_AGENT_SUBJECT_EXPRESSION"
	flag := rootCmd.PersistentFlags().Lookup(name)
	require.NotNil(t, flag, "production root command must register the detector flag")
	require.Empty(t, flag.DefValue)
	for _, tc := range []struct {
		name, yaml string
		env, cli   *string
		noFile     bool
		want       string
	}{
		{name: "omission leaves evaluator default"},
		{name: "yaml", yaml: "true", want: "true"},
		{name: "environment without yaml key", env: stringPointer("false"), want: "false"},
		{name: "environment wins yaml", yaml: "true", env: stringPointer("false"), want: "false"},
		{name: "CLI wins all", yaml: "true", env: stringPointer("false"), cli: stringPointer("has(subject_token.kind)"), want: "has(subject_token.kind)"},
		{name: "explicit empty CLI", yaml: "true", env: stringPointer("false"), cli: stringPointer("")},
		{name: "empty environment selects default", yaml: "true", env: stringPointer("")},
		{name: "environment without file", env: stringPointer("false"), noFile: true, want: "false"},
		{name: "CLI without file", cli: stringPointer("false"), noFile: true, want: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous, changed := flag.Value.String(), flag.Changed
			t.Cleanup(func() { require.NoError(t, flag.Value.Set(previous)); flag.Changed = changed })
			require.NoError(t, flag.Value.Set(""))
			flag.Changed = false
			path := filepath.Join(t.TempDir(), "broker.yaml")
			setBrokerConfigPath(t, path)
			t.Setenv(environment, "")
			require.NoError(t, os.Unsetenv(environment))
			if tc.env != nil {
				t.Setenv(environment, *tc.env)
			}
			t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", brokerTestKey)
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", brokerTestKey)
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
			if !tc.noFile {
				document := "oauth2_authorization_server:\n  mode: local\n"
				if tc.yaml != "" {
					document += "token_exchange:\n  claim_extraction:\n    agent_subject_expression: '" + tc.yaml + "'\n"
				}
				require.NoError(t, os.WriteFile(path, []byte(document), 0600))
			}
			args := []string{}
			if tc.cli != nil {
				args = append(args, "--"+name+"="+*tc.cli)
			}
			require.NoError(t, rootCmd.ParseFlags(args))
			loader := config.NewLoader()
			loader.SetCommand(rootCmd)
			cfg, err := loader.GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.TokenExchange.ClaimExtraction.AgentSubjectExpression)
		})
	}
}

func stringPointer(value string) *string { return &value }
