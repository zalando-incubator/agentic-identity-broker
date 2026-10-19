// Package config loads the per-user aibctl configuration: named contexts that
// describe how to reach one broker admin API and how to authenticate to it.
package config

import "time"

const (
	DefaultOutput              = "table"
	DefaultPrincipalHeader     = "X-Remote-User"
	DefaultTimeout             = 30 * time.Second
	DefaultTokenCommandTimeout = 10 * time.Second
	EnvPrefix                  = "AIBCTL"
)

var OutputFormats = []string{"table", "wide", "json", "yaml"}

type Config struct {
	CurrentContext string    `yaml:"current_context,omitempty"`
	Defaults       Defaults  `yaml:"defaults,omitempty"`
	Contexts       []Context `yaml:"contexts,omitempty"`
}

type Defaults struct {
	Output              string        `yaml:"output,omitempty"`
	ShowUUID            bool          `yaml:"show_uuid,omitempty"`
	Timeout             time.Duration `yaml:"timeout,omitempty"`
	TokenCommandTimeout time.Duration `yaml:"token_command_timeout,omitempty"`
}

type Context struct {
	Name       string            `yaml:"name"`
	Server     string            `yaml:"server"`
	HostHeader string            `yaml:"host_header,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
	Auth       AuthConfig        `yaml:"auth,omitempty"`
	Output     string            `yaml:"output,omitempty"`
	Timeout    time.Duration     `yaml:"timeout,omitempty"`
}

// AuthConfig describes the credentials sent with every request. A Bearer
// token (static or produced by a command) targets the reverse proxy in front
// of the admin API; the principal header targets the broker itself.
type AuthConfig struct {
	Principal       string   `yaml:"principal,omitempty"`
	PrincipalHeader string   `yaml:"principal_header,omitempty"`
	Token           string   `yaml:"token,omitempty"`
	TokenCommand    []string `yaml:"token_command,omitempty"`
}

// Resolved is the effective configuration for one invocation after the
// selected context, defaults, environment variables and flags are merged.
type Resolved struct {
	Name                string
	Server              string
	HostHeader          string
	Headers             map[string]string
	Principal           string
	PrincipalHeader     string
	Token               string
	TokenCommand        []string
	Output              string
	ShowUUID            bool
	Timeout             time.Duration
	TokenCommandTimeout time.Duration
	Verbose             bool
}
