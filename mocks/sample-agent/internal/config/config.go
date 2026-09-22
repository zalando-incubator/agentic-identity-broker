package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for the sample OAuth2 client
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	OAuth2       OAuth2Config       `yaml:"oauth2"`
	BrokerInfo   BrokerInfoConfig   `yaml:"broker"`
	AgentGateway AgentGatewayConfig `yaml:"agentgateway"`
}

// AgentGatewayConfig holds agentgateway connection settings
type AgentGatewayConfig struct {
	MCPURL string `yaml:"mcp_url"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

// OAuth2Config holds OAuth2 client configuration
type OAuth2Config struct {
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURI  string   `yaml:"redirect_uri"`
	Scopes       []string `yaml:"scopes"`

	// Per-type agent UUIDs populated at runtime from env vars set by start-sample-agent.sh.
	ProxyAgentID  string
	LocalAgentID  string
	CIMDAgentID   string
	CIMDClientURI string // The client_uri used as client_id in the authorize request
}

// BrokerInfoConfig holds identity broker information
type BrokerInfoConfig struct {
	BaseURL           string `yaml:"base_url"`
	AuthorizeEndpoint string `yaml:"authorize_endpoint"`
	TokenEndpoint     string `yaml:"token_endpoint"`
	MetadataEndpoint  string `yaml:"metadata_endpoint"`
}

// Load loads configuration from a YAML file
func Load(configDir string) (*Config, error) {
	configPath := fmt.Sprintf("%s/config.yaml", configDir)

	// Read the config file
	data, err := os.ReadFile(configPath) // #nosec G304 -- local sample-agent configuration path is selected by its operator.
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// BROKER_AGENT_ID overrides oauth2.client_id (legacy single-agent mode).
	if agentID := os.Getenv("BROKER_AGENT_ID"); agentID != "" {
		cfg.OAuth2.ClientID = agentID
	}

	// Per-type agent UUIDs injected by start-sample-agent.sh.
	cfg.OAuth2.ProxyAgentID = os.Getenv("BROKER_PROXY_AGENT_ID")
	cfg.OAuth2.LocalAgentID = os.Getenv("BROKER_LOCAL_AGENT_ID")
	cfg.OAuth2.CIMDAgentID = os.Getenv("BROKER_CIMD_AGENT_ID")
	cfg.OAuth2.CIMDClientURI = os.Getenv("BROKER_CIMD_CLIENT_URI")

	// Set defaults if not specified
	if cfg.Server.Bind == "" {
		cfg.Server.Bind = "127.0.0.1"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8001
	}
	if cfg.BrokerInfo.BaseURL == "" {
		cfg.BrokerInfo.BaseURL = "http://localhost:8000"
	}
	if cfg.BrokerInfo.AuthorizeEndpoint == "" {
		cfg.BrokerInfo.AuthorizeEndpoint = cfg.BrokerInfo.BaseURL + "/oauth2/authorize"
	}
	if cfg.BrokerInfo.TokenEndpoint == "" {
		cfg.BrokerInfo.TokenEndpoint = cfg.BrokerInfo.BaseURL + "/oauth2/token"
	}
	if cfg.BrokerInfo.MetadataEndpoint == "" {
		cfg.BrokerInfo.MetadataEndpoint = cfg.BrokerInfo.BaseURL + "/.well-known/oauth-authorization-server"
	}
	if cfg.OAuth2.RedirectURI == "" {
		cfg.OAuth2.RedirectURI = fmt.Sprintf("http://%s:%d/oauth2/callback", cfg.Server.Bind, cfg.Server.Port)
	}
	if len(cfg.OAuth2.Scopes) == 0 {
		cfg.OAuth2.Scopes = []string{"openid", "profile", "email"}
	}
	if cfg.AgentGateway.MCPURL == "" {
		cfg.AgentGateway.MCPURL = "http://localhost:4000/mcp"
	}

	return &cfg, nil
}
