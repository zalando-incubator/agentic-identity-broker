package config

import (
	"encoding/json"
	"os"
	"strings"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"go.yaml.in/yaml/v3"
)

const (
	credentialFilesConfigKey = "third_party_oauth2.credential_files"
	credentialFilesEnvKey    = "IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES"
)

func (l *Loader) credentialFilesJSONOverride() (map[string]ports.CredentialFileBinding, string, error) {
	if l.cmd != nil && l.cmd.Flags().Changed(credentialFilesConfigKey) {
		value, err := l.cmd.Flags().GetString(credentialFilesConfigKey)
		if err != nil {
			return nil, "cli", credentialFilesConfigError("cli", "a JSON object string")
		}
		bindings, err := decodeCredentialFilesJSON(value, "cli")
		return bindings, "cli", err
	}
	if value, present := os.LookupEnv(credentialFilesEnvKey); present {
		bindings, err := decodeCredentialFilesJSON(value, "environment")
		return bindings, "environment", err
	}
	return nil, "", nil
}

func decodeCredentialFilesJSON(value, source string) (map[string]ports.CredentialFileBinding, error) {
	var raw any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, credentialFilesConfigError(source, "a valid JSON object with complete file-pair objects")
	}
	return decodeCredentialFilesMap(raw, source)
}

func decodeCredentialFilesMap(raw any, source string) (map[string]ports.CredentialFileBinding, error) {
	mapping, ok := raw.(map[string]any)
	if !ok {
		return nil, credentialFilesConfigError(source, "an object mapping literal string canonical IDs to file-pair objects")
	}
	bindings := make(map[string]ports.CredentialFileBinding, len(mapping))
	for key, value := range mapping {
		pair, ok := value.(map[string]any)
		if !ok {
			return nil, credentialFilesConfigError(source, "a file-pair object for each canonical ID")
		}
		clientIDFile, ok := pair["client_id_file"].(string)
		if !ok {
			return nil, credentialFilesConfigError(source, "each pair must contain client_id_file as a string")
		}
		clientSecretFile, ok := pair["client_secret_file"].(string)
		if !ok {
			return nil, credentialFilesConfigError(source, "each pair must contain client_secret_file as a string")
		}
		bindings[key] = ports.CredentialFileBinding{ClientIDFile: clientIDFile, ClientSecretFile: clientSecretFile}
	}
	return bindings, nil
}

func (l *Loader) decodeCredentialFilesYAML(data []byte) (map[string]ports.CredentialFileBinding, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, &domconfig.ConfigError{
			Field:    "config_file",
			Expected: "valid YAML syntax",
			Source:   "yaml",
		}
	}
	if len(document.Content) == 0 {
		return nil, nil
	}
	section := credentialFilesYAMLField(document.Content[0], "third_party_oauth2")
	node := credentialFilesYAMLField(section, "credential_files")
	if node == nil {
		return nil, nil
	}
	var raw any
	if err := node.Decode(&raw); err != nil {
		return nil, credentialFilesConfigError("yaml", "a valid mapping with literal string keys and complete file-pair objects")
	}
	bindings, err := decodeCredentialFilesMap(raw, "yaml")
	if err != nil {
		return nil, err
	}
	for key, pair := range bindings {
		pair.ClientIDFile, err = l.expandCredentialFilePath(pair.ClientIDFile)
		if err != nil {
			return nil, err
		}
		pair.ClientSecretFile, err = l.expandCredentialFilePath(pair.ClientSecretFile)
		if err != nil {
			return nil, err
		}
		bindings[key] = pair
	}
	return bindings, nil
}

func credentialFilesYAMLField(node *yaml.Node, field string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && strings.EqualFold(key.Value, field) {
			return node.Content[i+1]
		}
	}
	return nil
}

func (l *Loader) expandCredentialFilePath(value string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	if err := l.validateSecurePattern(value); err != nil {
		return "", credentialFilesConfigError("yaml", "path references without command substitution or unsafe shell patterns")
	}
	expanded, err := l.expandWithCircularCheck(value, make(map[string]bool), 0)
	if err != nil {
		return "", credentialFilesConfigError("yaml", "resolvable path environment references without circular expansion")
	}
	return expanded, nil
}

func credentialFilesConfigError(source, expected string) error {
	return &domconfig.ConfigError{
		Field:    credentialFilesConfigKey,
		Value:    RedactedValue,
		Source:   source,
		Expected: expected,
	}
}
