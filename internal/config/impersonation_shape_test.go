package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/configutil"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func decodeYAMLDocument(t *testing.T, document string) (ports.Config, error) {
	t.Helper()
	v := configutil.NewViper()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(document)))
	return unmarshalConfig(v)
}

func TestImpersonationCategoryShapeFollowsYAMLKeys(t *testing.T) {
	for _, document := range []string{
		"OAUTH2_AUTHORIZATION_SERVER:\n  Impersonation:\n    Rules:\n      - ALLOWED_SUBJECT_CATEGORIES: null\n",
		"defaults: &rule\n  allowed_subject_categories: null\noauth2_authorization_server:\n  impersonation:\n    rules:\n      - <<: *rule\n",
	} {
		t.Run(document, func(t *testing.T) {
			_, err := decodeYAMLDocument(t, document)
			require.ErrorContains(t, err, "oauth2_authorization_server.impersonation.rules[0].allowed_subject_categories")
		})
	}
}

func TestImpersonationCategoryYAMLShape(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"", true}, {"[human]", true}, {"[agent, human]", true}, {"[]", true},
		{"null", false}, {"agent", false}, {"{agent: true}", false}, {"[agent, 1]", false}, {"[null]", false}, {"[true]", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			document := "oauth2_authorization_server:\n  impersonation:\n    rules:\n      - name: test\n"
			if tc.value != "" {
				document += "        allowed_subject_categories: " + tc.value + "\n"
			}
			cfg, err := decodeYAMLDocument(t, document)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "test", cfg.OAuth2AuthServer.Impersonation.Rules[0].Name)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), "oauth2_authorization_server.impersonation.rules[0].allowed_subject_categories")
			}
		})
	}
}
