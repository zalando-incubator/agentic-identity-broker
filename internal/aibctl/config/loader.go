package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// Overlay carries the values supplied through AIBCTL_* environment variables
// and command-line flags. A nil pointer means the value was not supplied.
type Overlay struct {
	Context         *string
	Server          *string
	HostHeader      *string
	Principal       *string
	PrincipalHeader *string
	Token           *string
	TokenCommand    []string
	Output          *string
	ShowUUID        *bool
	Timeout         *time.Duration
	Verbose         bool
}

// Loaded is the raw result of reading the configuration sources.
type Loaded struct {
	Path    string
	File    *Config
	Overlay Overlay
}

// SelectedContext is the context named by --context or AIBCTL_CONTEXT, else current_context.
func (l *Loaded) SelectedContext() string {
	if l.Overlay.Context != nil {
		return *l.Overlay.Context
	}
	return l.File.CurrentContext
}

// LocalOutput chooses the format for commands that only read the configuration
// file. A context's output applies only to commands that contact its server.
func (l *Loaded) LocalOutput() string {
	if l.Overlay.Output != nil {
		return *l.Overlay.Output
	}
	return firstNonEmpty(l.File.Defaults.Output, DefaultOutput)
}

const (
	keyConfig          = "config"
	keyContext         = "context"
	keyServer          = "server"
	keyHostHeader      = "host_header"
	keyPrincipal       = "principal"
	keyPrincipalHeader = "principal_header"
	keyToken           = "token"
	keyTokenCommand    = "token_command"
	keyOutput          = "output"
	keyShowUUID        = "show_uuid"
	keyTimeout         = "timeout"
	keyVerbose         = "verbose"
)

var overlayKeys = []string{
	keyConfig, keyContext, keyServer, keyHostHeader, keyPrincipal, keyPrincipalHeader,
	keyToken, keyTokenCommand, keyOutput, keyShowUUID, keyTimeout, keyVerbose,
}

// RegisterFlags adds the global flags to cmd as persistent flags.
func RegisterFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.String(keyConfig, "", "config file (default: OS user config dir/aibctl/config.yaml)")
	f.String(keyContext, "", "context name (default: current_context from the config file)")
	f.String(keyServer, "", "admin API base URL, e.g. http://localhost:14000")
	f.String("host-header", "", "Host header to send when the server URL differs from the broker's admin public_url")
	f.String(keyPrincipal, "", "principal sent in the principal header (admin identity)")
	f.String("principal-header", "", "header carrying the principal (default: X-Remote-User)")
	f.String(keyToken, "", "static Bearer token (overrides the context token command)")
	f.String("token-command", "", "command printing a Bearer token, split on whitespace")
	f.StringP(keyOutput, "o", "", "output format: table, wide, json, yaml (default: table)")
	f.Bool("show-uuid", false, "show UUIDs next to canonical IDs in table output")
	f.Duration(keyTimeout, 0, "request timeout (default: 30s)")
	f.BoolP(keyVerbose, "v", false, "log requests to stderr")
}

// Load reads the configuration file and the AIBCTL_* environment plus the
// flags registered on cmd. Precedence: flags > environment > file.
func Load(cmd *cobra.Command) (*Loaded, error) {
	return LoadFromViper(viper.New(), cmd)
}

// LoadFromViper is Load with a caller-provided Viper instance for tests.
func LoadFromViper(v *viper.Viper, cmd *cobra.Command) (*Loaded, error) {
	if err := bindOverlay(v, cmd); err != nil {
		return nil, err
	}
	path := v.GetString(keyConfig)
	if path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return nil, fmt.Errorf("resolve config path: %w", err)
		}
		path = defaultPath
	}

	file, err := ReadFile(path)
	if err != nil {
		return nil, err
	}

	loaded := &Loaded{Path: path, File: file}
	loaded.Overlay = overlayFromViper(v)
	return loaded, nil
}

// ReadFile parses a configuration file. A missing file yields an empty Config.
func ReadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the user's own config file.
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// LoadOverlay reads only the AIBCTL_* environment and the flags on cmd; it
// never reads the configuration file.
func LoadOverlay(cmd *cobra.Command) (Overlay, error) {
	v := viper.New()
	if err := bindOverlay(v, cmd); err != nil {
		return Overlay{}, err
	}
	return overlayFromViper(v), nil
}

func bindOverlay(v *viper.Viper, cmd *cobra.Command) error {
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	for _, key := range overlayKeys {
		if err := v.BindEnv(key); err != nil {
			return err
		}
	}
	if cmd != nil {
		bindFlags(v, cmd)
	}
	return nil
}

func bindFlags(v *viper.Viper, cmd *cobra.Command) {
	flags := cmd.Flags()
	bindings := []struct {
		flag string
		key  string
		get  func() any
	}{
		{keyConfig, keyConfig, func() any { s, _ := flags.GetString(keyConfig); return s }},
		{keyContext, keyContext, func() any { s, _ := flags.GetString(keyContext); return s }},
		{keyServer, keyServer, func() any { s, _ := flags.GetString(keyServer); return s }},
		{"host-header", keyHostHeader, func() any { s, _ := flags.GetString("host-header"); return s }},
		{keyPrincipal, keyPrincipal, func() any { s, _ := flags.GetString(keyPrincipal); return s }},
		{"principal-header", keyPrincipalHeader, func() any { s, _ := flags.GetString("principal-header"); return s }},
		{keyToken, keyToken, func() any { s, _ := flags.GetString(keyToken); return s }},
		{"token-command", keyTokenCommand, func() any { s, _ := flags.GetString("token-command"); return s }},
		{keyOutput, keyOutput, func() any { s, _ := flags.GetString(keyOutput); return s }},
		{"show-uuid", keyShowUUID, func() any { b, _ := flags.GetBool("show-uuid"); return b }},
		{keyTimeout, keyTimeout, func() any { d, _ := flags.GetDuration(keyTimeout); return d }},
		{keyVerbose, keyVerbose, func() any { b, _ := flags.GetBool(keyVerbose); return b }},
	}
	for _, b := range bindings {
		if flags.Changed(b.flag) {
			v.Set(b.key, b.get())
		}
	}
}

func overlayFromViper(v *viper.Viper) Overlay {
	str := func(key string) *string {
		if !v.IsSet(key) {
			return nil
		}
		s := v.GetString(key)
		return &s
	}
	o := Overlay{
		Context:         str(keyContext),
		Server:          str(keyServer),
		HostHeader:      str(keyHostHeader),
		Principal:       str(keyPrincipal),
		PrincipalHeader: str(keyPrincipalHeader),
		Token:           str(keyToken),
		Output:          str(keyOutput),
		Verbose:         v.GetBool(keyVerbose),
	}
	if v.IsSet(keyTokenCommand) {
		o.TokenCommand = strings.Fields(v.GetString(keyTokenCommand))
	}
	if v.IsSet(keyShowUUID) {
		b := v.GetBool(keyShowUUID)
		o.ShowUUID = &b
	}
	if v.IsSet(keyTimeout) {
		d := v.GetDuration(keyTimeout)
		o.Timeout = &d
	}
	return o
}

// Resolve merges the selected context with defaults and the overlay and
// validates the result.
func Resolve(loaded *Loaded) (*Resolved, error) {
	file := loaded.File
	o := loaded.Overlay

	name := loaded.SelectedContext()

	var ctx Context
	switch {
	case name != "":
		found, ok := file.Find(name)
		if !ok {
			return nil, fmt.Errorf("context %q not found in %s", name, loaded.Path)
		}
		ctx = *found
	case o.Server != nil:
		ctx = Context{}
	default:
		return nil, fmt.Errorf("no context selected: run \"aibctl context set NAME --server URL ...\" or pass --server (config: %s)", loaded.Path)
	}

	r := &Resolved{
		Name:                ctx.Name,
		Server:              ctx.Server,
		HostHeader:          ctx.HostHeader,
		Headers:             map[string]string{},
		Principal:           ctx.Auth.Principal,
		PrincipalHeader:     firstNonEmpty(ctx.Auth.PrincipalHeader, DefaultPrincipalHeader),
		Token:               ctx.Auth.Token,
		TokenCommand:        append([]string(nil), ctx.Auth.TokenCommand...),
		Output:              firstNonEmpty(ctx.Output, file.Defaults.Output, DefaultOutput),
		ShowUUID:            file.Defaults.ShowUUID,
		Timeout:             firstDuration(ctx.Timeout, file.Defaults.Timeout, DefaultTimeout),
		TokenCommandTimeout: firstDuration(file.Defaults.TokenCommandTimeout, DefaultTokenCommandTimeout),
		Verbose:             o.Verbose,
	}
	for k, v := range ctx.Headers {
		r.Headers[k] = v
	}

	if o.Server != nil {
		r.Server = *o.Server
	}
	if o.HostHeader != nil {
		r.HostHeader = *o.HostHeader
	}
	if o.Principal != nil {
		r.Principal = *o.Principal
	}
	if o.PrincipalHeader != nil {
		r.PrincipalHeader = *o.PrincipalHeader
	}
	if o.TokenCommand != nil {
		r.TokenCommand = o.TokenCommand
		r.Token = ""
	}
	if o.Token != nil {
		r.Token = *o.Token
		r.TokenCommand = nil
	}
	if o.Output != nil {
		r.Output = *o.Output
	}
	if o.ShowUUID != nil {
		r.ShowUUID = *o.ShowUUID
	}
	if o.Timeout != nil {
		r.Timeout = *o.Timeout
	}

	var err error
	if r.Token, err = expandEnv("auth.token", r.Token); err != nil {
		return nil, err
	}
	for k, v := range r.Headers {
		if r.Headers[k], err = expandEnv("headers."+k, v); err != nil {
			return nil, err
		}
	}

	if err := validateResolved(r); err != nil {
		return nil, err
	}
	return r, nil
}

func expandEnv(field, value string) (string, error) {
	var missing []string
	expanded := os.Expand(value, func(name string) string {
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("%s references unset environment variable(s): %s", field, strings.Join(missing, ", "))
	}
	return expanded, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstDuration(values ...time.Duration) time.Duration {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
