package fixtures

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// TestPrincipalFixtures verifies all principal fixtures work correctly.
func TestPrincipalFixtures(t *testing.T) {
	tests := []struct {
		name      string
		principal Principal
		expected  string
	}{
		{
			name:      "DefaultPrincipal",
			principal: DefaultPrincipal(),
			expected:  "user@example.com",
		},
		{
			name:      "AnotherPrincipal",
			principal: AnotherPrincipal(),
			expected:  "another-user@example.com",
		},
		{
			name:      "AdminPrincipal",
			principal: AdminPrincipal(),
			expected:  "admin@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.principal.String() != tt.expected {
				t.Errorf("got %q, want %q", tt.principal.String(), tt.expected)
			}
		})
	}
}

// TestAgentFixtures verifies all agent fixtures generate valid agents.
func TestAgentFixtures(t *testing.T) {
	// TestAgentFixtures directly uses fixture functions since they're deterministic for validation
	t.Run("ValidAgent", func(t *testing.T) {
		a := ValidAgent()
		if a == nil {
			t.Fatal("ValidAgent returned nil")
		}
		if a.ID.IsZero() {
			t.Error("agent ID is empty")
		}
		if a.ClientID == nil || *a.ClientID != id.ClientID("test-client-valid") {
			t.Errorf("got ClientID %v, want %q", a.ClientID, "test-client-valid")
		}
		if a.DisplayName != "Test Agent Valid" {
			t.Errorf("got DisplayName %q, want %q", a.DisplayName, "Test Agent Valid")
		}
		if err := a.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("AnotherAgent", func(t *testing.T) {
		a := AnotherAgent()
		if a == nil {
			t.Fatal("AnotherAgent returned nil")
		}
		if a.ClientID == nil || *a.ClientID != id.ClientID("test-client-another") {
			t.Errorf("got ClientID %v, want %q", a.ClientID, "test-client-another")
		}
		if err := a.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("AgentWithClientID", func(t *testing.T) {
		a := AgentWithClientID("custom-client-id")
		if a == nil {
			t.Fatal("AgentWithClientID returned nil")
		}
		if a.ClientID == nil || *a.ClientID != id.ClientID("custom-client-id") {
			t.Errorf("got ClientID %v, want %q", a.ClientID, "custom-client-id")
		}
		if err := a.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})
}

// TestGrantFixtures verifies all grant fixtures generate valid grants.
func TestGrantFixtures(t *testing.T) {
	principalEmail := "test@example.com"
	agentID := id.NewAgentID().String()
	serviceID := id.NewServiceID().String()

	t.Run("ActiveGrant", func(t *testing.T) {
		g := ActiveGrant(principalEmail, agentID, serviceID, []string{"read", "write"})
		if g == nil {
			t.Fatal("ActiveGrant returned nil")
		}
		if !g.IsActive(time.Now()) {
			t.Error("ActiveGrant should be active")
		}
		if g.Principal.String() != principalEmail {
			t.Errorf("got Principal %q, want %q", g.Principal, principalEmail)
		}
		if g.AgentID.String() != agentID {
			t.Errorf("got AgentID %q, want %q", g.AgentID, agentID)
		}
		if err := g.Validate(time.Now()); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("ExpiredGrant", func(t *testing.T) {
		g := ExpiredGrant(principalEmail, agentID, serviceID, []string{"read", "write"})
		if g == nil {
			t.Fatal("ExpiredGrant returned nil")
		}
		if g.IsActive(time.Now()) {
			t.Error("ExpiredGrant should not be active")
		}
		if g.Principal.String() != principalEmail {
			t.Errorf("got Principal %q, want %q", g.Principal, principalEmail)
		}
		if g.AgentID.String() != agentID {
			t.Errorf("got AgentID %q, want %q", g.AgentID, agentID)
		}
	})

	t.Run("GrantExpiringIn", func(t *testing.T) {
		g := GrantExpiringIn(principalEmail, agentID, serviceID, []string{"read", "write"}, 24*time.Hour)
		if g == nil {
			t.Fatal("GrantExpiringIn returned nil")
		}
		if !g.IsActive(time.Now()) {
			t.Error("GrantExpiringIn should be active")
		}
		if g.Principal.String() != principalEmail {
			t.Errorf("got Principal %q, want %q", g.Principal, principalEmail)
		}
		if err := g.Validate(time.Now()); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("IndefiniteGrant", func(t *testing.T) {
		g := IndefiniteGrant(principalEmail, agentID, serviceID, []string{"read", "write"})
		if g == nil {
			t.Fatal("IndefiniteGrant returned nil")
		}
		if !g.IsActive(time.Now()) {
			t.Error("IndefiniteGrant should always be active")
		}
		if g.ValidUntil != nil {
			t.Error("IndefiniteGrant should have nil ValidUntil")
		}
		if g.Principal.String() != principalEmail {
			t.Errorf("got Principal %q, want %q", g.Principal, principalEmail)
		}
		if err := g.Validate(time.Now()); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})
}

// TestConfigFixtures verifies all config fixtures generate valid configurations.
func TestConfigFixtures(t *testing.T) {
	t.Run("DefaultOAuth2Config", func(t *testing.T) {
		c := DefaultOAuth2Config()
		if c == nil {
			t.Fatal("DefaultOAuth2Config returned nil")
		}
		if c.Storage.Backend != "memory" {
			t.Errorf("got Backend %q, want %q", c.Storage.Backend, "memory")
		}
		if err := c.OAuth2AuthServer.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("OAuth2ConfigWithUpstream", func(t *testing.T) {
		c := OAuth2ConfigWithUpstream("http://custom-upstream:8080")
		if c == nil {
			t.Fatal("OAuth2ConfigWithUpstream returned nil")
		}
		if c.OAuth2AuthServer.Proxy.UpstreamIssuerURI != "http://custom-upstream:8080" {
			t.Errorf("got UpstreamIssuerURI %q, want %q",
				c.OAuth2AuthServer.Proxy.UpstreamIssuerURI,
				"http://custom-upstream:8080")
		}
		if err := c.OAuth2AuthServer.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("OAuth2ConfigWithTimeout", func(t *testing.T) {
		c := OAuth2ConfigWithTimeout(60 * time.Second)
		if c == nil {
			t.Fatal("OAuth2ConfigWithTimeout returned nil")
		}
		if c.OAuth2AuthServer.Proxy.UpstreamTimeout != 60*time.Second {
			t.Errorf("got UpstreamTimeout %v, want %v",
				c.OAuth2AuthServer.Proxy.UpstreamTimeout, 60*time.Second)
		}
		if err := c.OAuth2AuthServer.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("OAuth2ConfigWithLogLevel", func(t *testing.T) {
		c := OAuth2ConfigWithLogLevel("debug")
		if c == nil {
			t.Fatal("OAuth2ConfigWithLogLevel returned nil")
		}
		if c.Log.Level != "debug" {
			t.Errorf("got Log.Level %q, want %q", c.Log.Level, "debug")
		}
		if err := c.OAuth2AuthServer.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})

	t.Run("OAuth2ConfigWithPublicURL", func(t *testing.T) {
		c := OAuth2ConfigWithPublicURL("https://broker.example.com")
		if c == nil {
			t.Fatal("OAuth2ConfigWithPublicURL returned nil")
		}
		if c.Server.EndUser.PublicURL != "https://broker.example.com" {
			t.Errorf("got PublicURL %q, want %q",
				c.Server.EndUser.PublicURL,
				"https://broker.example.com")
		}
		if err := c.OAuth2AuthServer.Validate(); err != nil {
			t.Errorf("validation failed: %v", err)
		}
	})
}

// TestFixtureDeterminism verifies that fixtures produce consistent data structures.
// Principal and Config fixtures should return identical values.
// Agent and Grant fixtures should return different IDs (UUIDs are generated).
func TestFixtureDeterminism(t *testing.T) {
	// Principals and configs should be deterministic (same input = same output)
	p1 := DefaultPrincipal()
	p2 := DefaultPrincipal()
	if p1.String() != p2.String() {
		t.Errorf("DefaultPrincipal not deterministic: got %q and %q", p1.String(), p2.String())
	}

	// Agents should get fresh UUIDs each time
	a1 := ValidAgent()
	a2 := ValidAgent()
	if a1.ID == a2.ID {
		t.Error("ValidAgent should generate fresh UUIDs, but got same ID twice")
	}

	// Grants should get fresh UUIDs each time
	grantAgentID := id.NewAgentID().String()
	grantServiceID := id.NewServiceID().String()
	g1 := ActiveGrant("user@example.com", grantAgentID, grantServiceID, []string{"read"})
	g2 := ActiveGrant("user@example.com", grantAgentID, grantServiceID, []string{"read"})
	if g1.ID == g2.ID {
		t.Error("ActiveGrant should generate fresh UUIDs, but got same ID twice")
	}
}
