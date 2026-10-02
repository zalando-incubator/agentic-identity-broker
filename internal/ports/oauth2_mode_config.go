package ports

import (
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
)

// OAuth2ModeConfig is a sealed interface representing a validated, mode-specific
// OAuth2 authorization server configuration. Produced by OAuth2AuthServerConfig.Resolve()
// after deserialization and validation.
//
// Consumers type-switch on the concrete type to access mode-specific fields.
// This eliminates runtime "is this field relevant?" checks that plague the flat union struct.
type OAuth2ModeConfig interface {
	oauth2ModeConfig() // seal — only this package can implement
	ServerMode() servermode.Mode
	ResponseTypes() []string
	GrantTypes() []string
}

// ProxyOAuth2Config is the resolved configuration for proxy mode.
// All requests are forwarded to an upstream OAuth2 authorization server.
type ProxyOAuth2Config struct {
	UpstreamIssuerURI         string
	UpstreamAuthorizeEndpoint string
	UpstreamTokenEndpoint     string
	UpstreamTimeout           time.Duration
	UpstreamJWKSMinRefresh    time.Duration
	UpstreamJWKSMaxRefresh    time.Duration
	SupportedResponseTypes    []string
	SupportedGrantTypes       []string
	SupportedScopes           []string
	MultiAgentClient          MultiAgentClientConfig
}

func (*ProxyOAuth2Config) oauth2ModeConfig()           {}
func (*ProxyOAuth2Config) ServerMode() servermode.Mode { return servermode.Proxy }
func (c *ProxyOAuth2Config) ResponseTypes() []string   { return c.SupportedResponseTypes }
func (c *ProxyOAuth2Config) GrantTypes() []string      { return c.SupportedGrantTypes }

func (c *ProxyOAuth2Config) JWKSMinRefresh() time.Duration {
	if c.UpstreamJWKSMinRefresh == 0 {
		return 15 * time.Minute
	}
	return c.UpstreamJWKSMinRefresh
}

func (c *ProxyOAuth2Config) JWKSMaxRefresh() time.Duration {
	min := c.JWKSMinRefresh()
	if c.UpstreamJWKSMaxRefresh == 0 || c.UpstreamJWKSMaxRefresh < min {
		// Preserve the invariant max >= max(min, 1h) when the configured max is
		// omitted or smaller than the resolved minimum refresh interval.
		if min > time.Hour {
			return min
		}
		return time.Hour
	}
	return c.UpstreamJWKSMaxRefresh
}

// LocalOAuth2Config is the resolved configuration for local mode.
// The broker mints its own JWT access tokens — no upstream server required.
type LocalOAuth2Config struct {
	IssuerURI                 string
	TokenTTL                  time.Duration
	RefreshTokenTTL           time.Duration
	RefreshTokenReuseInterval time.Duration
	AbsoluteSessionLifetime   time.Duration
	TokenClaimsExpression     string
	SigningKeys               LocalSigningKeysConfig
	SupportedResponseTypes    []string
	SupportedGrantTypes       []string
	SupportedScopes           []string
	CIMD                      CIMDConfig
}

func (*LocalOAuth2Config) oauth2ModeConfig()           {}
func (*LocalOAuth2Config) ServerMode() servermode.Mode { return servermode.Local }
func (c *LocalOAuth2Config) ResponseTypes() []string   { return c.SupportedResponseTypes }
func (c *LocalOAuth2Config) GrantTypes() []string      { return c.SupportedGrantTypes }

// HybridOAuth2Config is the resolved configuration for hybrid mode.
// Both proxy agents (upstream) and local agents (broker-minted) coexist.
type HybridOAuth2Config struct {
	Proxy ProxyOAuth2Config
	Local LocalOAuth2Config
}

func (*HybridOAuth2Config) oauth2ModeConfig()           {}
func (*HybridOAuth2Config) ServerMode() servermode.Mode { return servermode.Hybrid }

// ResponseTypes returns the deduplicated union of proxy and local supported response types.
// Both sides are currently populated with the same values by Resolve(), but returning
// the union makes the method correct if they ever diverge.
func (c *HybridOAuth2Config) ResponseTypes() []string {
	return unionStrings(c.Proxy.SupportedResponseTypes, c.Local.SupportedResponseTypes)
}

// GrantTypes returns the deduplicated union of proxy and local supported grant types.
// Both sides are currently populated with the same values by Resolve(), but returning
// the union makes the method correct if they ever diverge.
func (c *HybridOAuth2Config) GrantTypes() []string {
	return unionStrings(c.Proxy.SupportedGrantTypes, c.Local.SupportedGrantTypes)
}

// unionStrings returns a new slice containing each element of a and b exactly once,
// preserving order (a first, then new elements from b).
func unionStrings(a, b []string) []string {
	result := slices.Clone(a)
	for _, v := range b {
		if !slices.Contains(result, v) {
			result = append(result, v)
		}
	}
	return result
}
