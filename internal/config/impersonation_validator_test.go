package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func validImpersonationConfig() *ports.ImpersonationConfig {
	return &ports.ImpersonationConfig{
		AudiencePrefix: "https://broker.example.com/impersonation",
		Rules: []ports.ImpersonationRuleConfig{{
			Name: "internal-gateway",
			Roles: map[string]ports.ImpersonationRoleConfig{
				"client_assertion": {ExpectedAudience: "https://broker.example.com/impersonation", PrincipalExpression: "client_assertion.sub"},
				"actor":            {ExpectedAudience: "https://broker.example.com/impersonation", PrincipalExpression: "actor_token.sub"},
				"subject":          {ExpectedAudience: "https://broker.example.com/impersonation", PrincipalExpression: "subject_token.sub", EmailExpression: "has(subject_token.email) ? subject_token.email : ''"},
			},
			TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
				IssuerURI:         "https://idp.example.com",
				AllowedAlgorithms: []string{"ES256", "RS256"},
				SignsRoles:        []string{"client_assertion", "actor", "subject"},
			}},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL:  ports.CELAuthorizationConfig{Expression: `client_assertion.sub == "gw"`},
			},
		}},
	}
}

func TestValidateImpersonationSubjectCategories(t *testing.T) {
	for _, tc := range []struct {
		categories []string
		valid      bool
	}{
		{nil, true}, {[]string{"human"}, true}, {[]string{"agent"}, true},
		{[]string{"human", "agent"}, true}, {[]string{"agent", "human"}, true},
		{[]string{}, false}, {[]string{"machine"}, false}, {[]string{"Human"}, false},
		{[]string{"agent", "agent"}, false}, {[]string{"human", "agent", "human"}, false},
	} {
		t.Run(strings.Join(tc.categories, ","), func(t *testing.T) {
			cfg := validImpersonationConfig()
			cfg.Rules[0].AllowedSubjectCategories = tc.categories
			err := validateImpersonationConfig(cfg, servermode.Local, &ports.SecurityConfig{})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), "oauth2_authorization_server.impersonation.rules[0].allowed_subject_categories")
			}
		})
	}
	cfg := validUnverifiedImpersonationConfig()
	cfg.Rules[0].AllowedSubjectCategories = []string{"agent"}
	cfg.Rules[0].Authorization.CEL.Expression = "subject_token.sub != ''"
	require.NoError(t, validateImpersonationConfig(cfg, servermode.Local, &ports.SecurityConfig{}))
}

func TestValidateImpersonationConfig_Valid(t *testing.T) {
	err := validateImpersonationConfig(validImpersonationConfig(), servermode.Local, &ports.SecurityConfig{})
	require.NoError(t, err)
}

// validUnverifiedImpersonationConfig accepts an unverified subject: verification none, no
// expected_audience on the subject, subject absent from signs_roles (FR-003d/CR-008).
func validUnverifiedImpersonationConfig() *ports.ImpersonationConfig {
	cfg := validImpersonationConfig()
	subject := cfg.Rules[0].Roles["subject"]
	subject.Verification = ports.SubjectVerificationNone
	subject.ExpectedAudience = ""
	cfg.Rules[0].Roles["subject"] = subject
	cfg.Rules[0].TrustedIssuers[0].SignsRoles = []string{"client_assertion", "actor"}
	return cfg
}

func TestValidateImpersonationConfig_UnverifiedValid(t *testing.T) {
	err := validateImpersonationConfig(validUnverifiedImpersonationConfig(), servermode.Local, &ports.SecurityConfig{})
	require.NoError(t, err)
}

func TestValidateImpersonationConfig_Rejections(t *testing.T) {
	tests := []struct {
		name      string
		mode      servermode.Mode
		mutate    func(c *ports.ImpersonationConfig)
		wantField string
	}{
		{"proxy mode", servermode.Proxy, func(*ports.ImpersonationConfig) {}, "impersonation"},
		{"hybrid mode", servermode.Hybrid, func(*ports.ImpersonationConfig) {}, "impersonation"},
		{"empty audience prefix", servermode.Local, func(c *ports.ImpersonationConfig) { c.AudiencePrefix = "" }, "audience_prefix"},
		{"no rules", servermode.Local, func(c *ports.ImpersonationConfig) { c.Rules = nil }, "rules"},
		{"duplicate rule name", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules = append(c.Rules, c.Rules[0])
		}, "name"},
		{"missing subject role", servermode.Local, func(c *ports.ImpersonationConfig) {
			delete(c.Rules[0].Roles, "subject")
		}, "roles.subject"},
		{"unknown role key", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].Roles["extra"] = ports.ImpersonationRoleConfig{PrincipalExpression: "x"}
		}, "roles"},
		{"missing principal_expression", servermode.Local, func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["actor"]
			r.PrincipalExpression = ""
			c.Rules[0].Roles["actor"] = r
		}, "principal_expression"},
		{"email on non-subject role", servermode.Local, func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["actor"]
			r.EmailExpression = "x"
			c.Rules[0].Roles["actor"] = r
		}, "email_expression"},
		{"missing expected_audience", servermode.Local, func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["client_assertion"]
			r.ExpectedAudience = ""
			c.Rules[0].Roles["client_assertion"] = r
		}, "expected_audience"},
		{"unverified subject with expected_audience", servermode.Local, func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["subject"]
			r.Verification = ports.SubjectVerificationNone
			c.Rules[0].Roles["subject"] = r
		}, "expected_audience"},
		{"invalid subject verification", servermode.Local, func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["subject"]
			r.Verification = "sometimes"
			c.Rules[0].Roles["subject"] = r
		}, "verification"},
		{"unverified subject listed in signs_roles", servermode.Local, func(c *ports.ImpersonationConfig) {
			r := c.Rules[0].Roles["subject"]
			r.Verification = ports.SubjectVerificationNone
			r.ExpectedAudience = ""
			c.Rules[0].Roles["subject"] = r
		}, "signs_roles"},
		{"non-cel authorization", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].Authorization.Type = "opa"
		}, "authorization.type"},
		{"empty authorization expression", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].Authorization.CEL.Expression = ""
		}, "authorization.cel.expression"},
		{"empty allowed_algorithms", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].TrustedIssuers[0].AllowedAlgorithms = nil
		}, "allowed_algorithms"},
		{"symmetric algorithm", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"HS256"}
		}, "allowed_algorithms"},
		{"duplicate issuer_uri", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].TrustedIssuers = append(c.Rules[0].TrustedIssuers, c.Rules[0].TrustedIssuers[0])
		}, "issuer_uri"},
		{"uncovered signed role", servermode.Local, func(c *ports.ImpersonationConfig) {
			c.Rules[0].TrustedIssuers[0].SignsRoles = []string{"client_assertion", "actor"}
		}, "trusted_issuers"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validImpersonationConfig()
			tc.mutate(cfg)
			err := validateImpersonationConfig(cfg, tc.mode, &ports.SecurityConfig{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantField)
		})
	}
}

func TestValidateImpersonationConfig_AudienceRequirementAbsent(t *testing.T) {
	for _, roleName := range []string{"client_assertion", "actor", "subject"} {
		t.Run("accepts "+roleName, func(t *testing.T) {
			cfg := validImpersonationConfig()
			role := cfg.Rules[0].Roles[roleName]
			role.ExpectedAudience = ""
			role.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
			cfg.Rules[0].Roles[roleName] = role
			require.NoError(t, validateImpersonationConfig(cfg, servermode.Local, &ports.SecurityConfig{}))
		})
	}

	tests := []struct {
		name      string
		config    func() *ports.ImpersonationConfig
		mutate    func(*ports.ImpersonationConfig)
		wantField string
	}{
		{
			name:   "rejects expected audience with absent requirement",
			config: validImpersonationConfig,
			mutate: func(cfg *ports.ImpersonationConfig) {
				role := cfg.Rules[0].Roles["actor"]
				role.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
				cfg.Rules[0].Roles["actor"] = role
			},
			wantField: ".expected_audience",
		},
		{
			name:   "rejects unknown audience requirement",
			config: validImpersonationConfig,
			mutate: func(cfg *ports.ImpersonationConfig) {
				role := cfg.Rules[0].Roles["actor"]
				role.ExpectedAudience = ""
				role.AudienceRequirement = "optional"
				cfg.Rules[0].Roles["actor"] = role
			},
			wantField: ".audience_requirement",
		},
		{
			name:   "rejects unverified subject audience requirement",
			config: validUnverifiedImpersonationConfig,
			mutate: func(cfg *ports.ImpersonationConfig) {
				role := cfg.Rules[0].Roles["subject"]
				role.AudienceRequirement = ports.ImpersonationAudienceRequirementAbsent
				cfg.Rules[0].Roles["subject"] = role
			},
			wantField: ".audience_requirement",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.config()
			tc.mutate(cfg)
			err := validateImpersonationConfig(cfg, servermode.Local, &ports.SecurityConfig{})
			require.Error(t, err)
			configErr, ok := err.(*domconfig.ConfigError)
			require.True(t, ok, "expected ConfigError, got %T", err)
			assert.True(t, strings.HasSuffix(configErr.Field, tc.wantField), "field %q must end in %q", configErr.Field, tc.wantField)
		})
	}
}
