package fixtures

import (
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-viper/mapstructure/v2"
)

type RefreshClient struct {
	Agent     *storage.Agent
	Grant     *storage.UserGrant
	Principal id.Principal
	Secret    string
}

func RefreshAgent() *storage.Agent {
	agent := LocalAgent()
	agent.AllowedScopes = []string{"read", "write", "offline_access"}
	return agent
}

func RefreshGrant(principal id.Principal, agentID id.AgentID) *storage.UserGrant {
	return IndefiniteGrant(principal.String(), agentID.String(), PlaceholderServiceID.String(), []string{"read"})
}

func RefreshConfig(reuse, absolute, inactivity string) (*ports.Config, error) {
	cfg := LocalConfig()
	values := map[string]any{}
	for key, value := range map[string]string{
		"refresh_token_reuse_interval": reuse,
		"absolute_session_lifetime":    absolute,
		"refresh_token_ttl":            inactivity,
	} {
		if value != "" {
			values[key] = value
		}
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &cfg.OAuth2AuthServer.Local,
		TagName:    "mapstructure",
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return nil, fmt.Errorf("create refresh configuration decoder: %w", err)
	}
	if err := decoder.Decode(values); err != nil {
		return nil, fmt.Errorf("decode refresh configuration: %w", err)
	}
	return cfg, nil
}

func RefreshExpiredGrant(grant *storage.UserGrant, at time.Time) *storage.UserGrant {
	copy := grant.Copy()
	copy.ValidUntil = &at
	return copy
}
