package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImpersonationCategoryShapeFollowsYAMLKeys(t *testing.T) {
	for _, document := range []string{
		"OAUTH2_AUTHORIZATION_SERVER:\n  Impersonation:\n    Rules:\n      - ALLOWED_SUBJECT_CATEGORIES: null\n",
		"defaults: &rule\n  allowed_subject_categories: null\noauth2_authorization_server:\n  impersonation:\n    rules:\n      - <<: *rule\n",
	} {
		t.Run(document, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(document), 0600))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
			_, err := NewLoader().loadYAML(false)
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
			path := filepath.Join(t.TempDir(), "config.yaml")
			document := "oauth2_authorization_server:\n  impersonation:\n    rules:\n      - name: test\n"
			if tc.value != "" {
				document += "        allowed_subject_categories: " + tc.value + "\n"
			}
			require.NoError(t, os.WriteFile(path, []byte(document), 0600))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
			_, err := NewLoader().loadYAML(false)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), "oauth2_authorization_server.impersonation.rules[0].allowed_subject_categories")
			}
		})
	}
}
