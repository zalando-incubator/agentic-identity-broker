package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for the upstream OAuth2 mock server
type Config struct {
	Server ServerConfig `yaml:"server"`
	OAuth2 OAuth2Config `yaml:"oauth2"`
	User   UserConfig   `yaml:"mock_user"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

// OAuth2Config holds OAuth2 server configuration
type OAuth2Config struct {
	ClientID             string        `yaml:"client_id"`
	ClientSecret         string        `yaml:"client_secret"`
	AccessTokenTTL       time.Duration `yaml:"access_token_ttl"`
	RefreshTokenTTL      time.Duration `yaml:"refresh_token_ttl"`
	AuthorizationCodeTTL time.Duration `yaml:"authorization_code_ttl"`
	Scopes               []ScopeConfig `yaml:"scopes"`
}

// ScopeConfig describes a scope
type ScopeConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// UserConfig holds mock user configuration
type UserConfig struct {
	Sub   string `yaml:"sub"`
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// Load loads configuration from a YAML file
func Load(configDir string) (*Config, error) {
	configPath := fmt.Sprintf("%s/config.yaml", configDir)

	// Read the config file
	data, err := os.ReadFile(configPath) // #nosec G304 -- local mock configuration path is selected by its operator.
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults if not specified
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "127.0.0.1"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 9001
	}
	if cfg.OAuth2.AccessTokenTTL == 0 {
		cfg.OAuth2.AccessTokenTTL = 3600 * time.Second
	}
	if cfg.OAuth2.RefreshTokenTTL == 0 {
		cfg.OAuth2.RefreshTokenTTL = 604800 * time.Second
	}
	if cfg.OAuth2.AuthorizationCodeTTL == 0 {
		cfg.OAuth2.AuthorizationCodeTTL = 300 * time.Second
	}

	return &cfg, nil
}
