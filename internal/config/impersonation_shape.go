package config

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
)

func validateImpersonationCategoryShape(data []byte) error {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return &domconfig.ConfigError{Field: "config_file", Expected: "valid YAML syntax", Source: "yaml"}
	}
	authValue, _ := impersonationYAMLValue(document, "oauth2_authorization_server")
	auth, _ := authValue.(map[string]any)
	impersonationValue, _ := impersonationYAMLValue(auth, "impersonation")
	impersonation, _ := impersonationValue.(map[string]any)
	rulesValue, _ := impersonationYAMLValue(impersonation, "rules")
	rules, _ := rulesValue.([]any)
	for i, raw := range rules {
		rule, _ := raw.(map[string]any)
		value, present := impersonationYAMLValue(rule, "allowed_subject_categories")
		if !present {
			continue
		}
		values, sequence := value.([]any)
		valid := sequence
		for _, entry := range values {
			if _, ok := entry.(string); !ok {
				valid = false
			}
		}
		if !valid {
			return &domconfig.ConfigError{
				Field:    fmt.Sprintf("oauth2_authorization_server.impersonation.rules[%d].allowed_subject_categories", i),
				Expected: "a sequence of strings; omit for the human-only default", Source: "yaml",
			}
		}
	}
	return nil
}

func impersonationYAMLValue(mapping map[string]any, name string) (any, bool) {
	for key, value := range mapping {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
}
