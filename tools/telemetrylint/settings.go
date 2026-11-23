package telemetrylint

import (
	"fmt"
	"regexp"
	"strings"
)

// Settings is the telemetrylint configuration in .golangci.yml. An empty field takes its default.
type Settings struct {
	// Scopes is the instrumentation scope allowlist (OPENTELEMETRY.md section 2).
	Scopes []string `json:"scopes"`
	// PrincipalType is the qualified principal type, "<import path>.<name>" (LOGGING.md section 5).
	PrincipalType string `json:"principal-type"`
	// PrincipalKeyExemptPackages are package path patterns that may set a .principal span attribute
	// without PrincipalType.SpanAttribute. ExtProc must not import the principal type (ADR 027).
	PrincipalKeyExemptPackages []string `json:"principal-key-exempt-packages"`
	// CredentialTypes are qualified credential types that must not be log or span attribute values
	// (LOGGING.md section 6.4).
	CredentialTypes []string `json:"credential-types"`
}

// DefaultScopes is the scope table of OPENTELEMETRY.md section 2.
var DefaultScopes = []string{
	"storage",
	"encryption",
	"jwks",
	"upstream",
	"telemetryhttp",
	"approval",
	"oauth2session",
	"tokenexchange",
	"extproc",
}

const (
	defaultPrincipalType   = "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id.Principal"
	defaultSensitiveType   = "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model.Secret"
	defaultExemptPrincipal = `/internal/extproc(/|$)`
)

type qualifiedType struct {
	path string
	name string
}

func (q qualifiedType) String() string {
	return q.path + "." + q.name
}

type config struct {
	scopes          map[string]bool
	principal       qualifiedType
	principalExempt []*regexp.Regexp
	credentials     []qualifiedType
}

func newConfig(s Settings) (*config, error) {
	c := &config{scopes: map[string]bool{}}
	scopes := s.Scopes
	if len(scopes) == 0 {
		scopes = DefaultScopes
	}
	for _, scope := range scopes {
		if !segment.MatchString(scope) {
			return nil, fmt.Errorf("telemetrylint: scope %q is not a lowercase snake_case segment", scope)
		}
		c.scopes[scope] = true
	}

	principal := s.PrincipalType
	if principal == "" {
		principal = defaultPrincipalType
	}
	var err error
	if c.principal, err = parseQualifiedType(principal); err != nil {
		return nil, err
	}

	exempt := s.PrincipalKeyExemptPackages
	if exempt == nil {
		exempt = []string{defaultExemptPrincipal}
	}
	for _, pattern := range exempt {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("telemetrylint: principal-key-exempt-packages %q: %w", pattern, err)
		}
		c.principalExempt = append(c.principalExempt, re)
	}

	credentials := s.CredentialTypes
	if credentials == nil {
		credentials = []string{defaultSensitiveType}
	}
	for _, credential := range credentials {
		q, err := parseQualifiedType(credential)
		if err != nil {
			return nil, err
		}
		c.credentials = append(c.credentials, q)
	}
	return c, nil
}

func parseQualifiedType(s string) (qualifiedType, error) {
	dot := strings.LastIndex(s, ".")
	slash := strings.LastIndex(s, "/")
	if dot <= 0 || dot < slash || dot == len(s)-1 {
		return qualifiedType{}, fmt.Errorf("telemetrylint: %q is not a qualified type \"<import path>.<name>\"", s)
	}
	return qualifiedType{path: s[:dot], name: s[dot+1:]}, nil
}
