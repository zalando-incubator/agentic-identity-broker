package config

import "go.yaml.in/yaml/v3"

// Document converts a *Config or []Context into generic data with the file's
// key names and duration strings, so JSON and YAML match the configuration file.
func Document(v any) (any, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}
