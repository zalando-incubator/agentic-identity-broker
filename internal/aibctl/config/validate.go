package config

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

var contextNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// managedHeaders are set by aibctl on every request; extra headers must not replace them.
var managedHeaders = []struct{ name, hint string }{
	{"Accept", "always application/json"},
	{"Content-Type", "always application/json"},
	{"Prefer", "always reference-id=canonical"},
	{"If-Match", "use service update --if-match"},
	{"Authorization", "use auth.token or auth.token_command"},
	{"Host", "use host_header"},
}

func validateHeaders(field string, headers map[string]string, principalHeader string) []error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		trimmed := strings.TrimSpace(name)
		if strings.EqualFold(trimmed, principalHeader) {
			errs = append(errs, fmt.Errorf("%s.%s: the principal header is set from auth.principal", field, name))
			continue
		}
		for _, h := range managedHeaders {
			if strings.EqualFold(trimmed, h.name) {
				errs = append(errs, fmt.Errorf("%s.%s: %s is set by aibctl (%s)", field, name, h.name, h.hint))
			}
		}
	}
	return errs
}

// Validate checks the structural rules of a configuration file.
func Validate(cfg *Config) error {
	var errs []error
	seen := map[string]bool{}
	for i, c := range cfg.Contexts {
		field := fmt.Sprintf("contexts[%d]", i)
		if !contextNamePattern.MatchString(c.Name) {
			errs = append(errs, fmt.Errorf("%s: name %q must match %s", field, c.Name, contextNamePattern))
		}
		if seen[c.Name] {
			errs = append(errs, fmt.Errorf("%s: duplicate context name %q", field, c.Name))
		}
		seen[c.Name] = true
		errs = append(errs, validateServer(field+".server", c.Server)...)
		errs = append(errs, validateAuth(field+".auth", c.Auth)...)
		errs = append(errs, validateOutput(field+".output", c.Output)...)
		errs = append(errs, validateHeaders(field+".headers", c.Headers, firstNonEmpty(c.Auth.PrincipalHeader, DefaultPrincipalHeader))...)
		errs = append(errs, validateTimeout(field+".timeout", c.Timeout)...)
	}
	if cfg.CurrentContext != "" && !seen[cfg.CurrentContext] {
		errs = append(errs, fmt.Errorf("current_context %q does not name a configured context", cfg.CurrentContext))
	}
	errs = append(errs, validateOutput("defaults.output", cfg.Defaults.Output)...)
	errs = append(errs, validateTimeout("defaults.timeout", cfg.Defaults.Timeout)...)
	errs = append(errs, validateTimeout("defaults.token_command_timeout", cfg.Defaults.TokenCommandTimeout)...)
	return errors.Join(errs...)
}

func validateResolved(r *Resolved) error {
	var errs []error
	if r.Server == "" {
		errs = append(errs, errors.New("server is required"))
	} else {
		errs = append(errs, validateServer("server", r.Server)...)
	}
	errs = append(errs, validateAuth("auth", AuthConfig{
		Principal: r.Principal, PrincipalHeader: r.PrincipalHeader, Token: r.Token, TokenCommand: r.TokenCommand,
	})...)
	if r.Principal == "" && r.Token == "" && len(r.TokenCommand) == 0 {
		errs = append(errs, errors.New("auth: configure a principal, a token, or a token_command"))
	}
	errs = append(errs, validateHeaders("headers", r.Headers, r.PrincipalHeader)...)
	if err := CheckOutputFormat(r.Output); err != nil {
		errs = append(errs, err)
	}
	if r.Timeout <= 0 {
		errs = append(errs, errors.New("timeout must be positive"))
	}
	if r.TokenCommandTimeout <= 0 {
		errs = append(errs, errors.New("token_command_timeout must be positive"))
	}
	return errors.Join(errs...)
}

func validateServer(field, server string) []error {
	if server == "" {
		return []error{fmt.Errorf("%s is required", field)}
	}
	u, err := url.Parse(server)
	if err != nil {
		return []error{fmt.Errorf("%s: %w", field, err)}
	}
	var errs []error
	if u.Scheme != "http" && u.Scheme != "https" {
		errs = append(errs, fmt.Errorf("%s: scheme must be http or https", field))
	}
	if u.Host == "" {
		errs = append(errs, fmt.Errorf("%s: host is required", field))
	}
	if strings.Trim(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
		errs = append(errs, fmt.Errorf("%s: must not contain a path, query, or fragment", field))
	}
	return errs
}

func validateAuth(field string, a AuthConfig) []error {
	var errs []error
	if a.Token != "" && len(a.TokenCommand) > 0 {
		errs = append(errs, fmt.Errorf("%s: token and token_command are mutually exclusive", field))
	}
	if a.PrincipalHeader != "" && strings.ContainsAny(a.PrincipalHeader, " :\r\n\t") {
		errs = append(errs, fmt.Errorf("%s.principal_header: %q is not a valid header name", field, a.PrincipalHeader))
	}
	return errs
}

// CheckOutputFormat rejects formats other than table, wide, json, and yaml.
func CheckOutputFormat(format string) error {
	if !slices.Contains(OutputFormats, format) {
		return fmt.Errorf("output %q must be one of %s", format, strings.Join(OutputFormats, ", "))
	}
	return nil
}

func validateOutput(field, output string) []error {
	if output != "" && !slices.Contains(OutputFormats, output) {
		return []error{fmt.Errorf("%s: %q must be one of %s", field, output, strings.Join(OutputFormats, ", "))}
	}
	return nil
}

func validateTimeout(field string, d time.Duration) []error {
	if d < 0 {
		return []error{fmt.Errorf("%s: must not be negative", field)}
	}
	return nil
}
