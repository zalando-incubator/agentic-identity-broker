package aibctlcmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/input"
)

const (
	flagCanonicalID      = "canonical-id"
	flagClearCanonicalID = "clear-canonical-id"
)

// readDocument decodes the --file document into out.
func (c *cli) readDocument(cmd *cobra.Command, out any) error {
	path, _ := cmd.Flags().GetString(flagFile)
	if err := input.Decode(path, c.deps.Stdin, out); err != nil {
		return &UsageError{Err: err, Cmd: cmd}
	}
	return nil
}

// requireFileOrFlags rejects mixing -f with field flags and requires one of them
// on create.
func requireFileOrFlags(cmd *cobra.Command, fieldFlags []string, create bool) (bool, error) {
	f := cmd.Flags()
	hasFile := f.Changed(flagFile)
	if path, _ := f.GetString(flagFile); hasFile && path == "" {
		return false, &UsageError{Err: errors.New(`--file needs a path, or "-" for stdin`), Cmd: cmd}
	}
	var changed []string
	for _, name := range fieldFlags {
		if f.Changed(name) {
			changed = append(changed, "--"+name)
		}
	}
	if hasFile && len(changed) > 0 {
		return false, &UsageError{Err: fmt.Errorf("--file cannot be combined with %s", strings.Join(changed, ", ")), Cmd: cmd}
	}
	if create && !hasFile && len(changed) == 0 {
		return false, &UsageError{Err: fmt.Errorf("pass --file or field flags (see --help)"), Cmd: cmd}
	}
	return hasFile, nil
}

// parseRef splits "ID[:requirement_type]" with a default requirement type.
func parseRef(value, defaultType string) (string, string, error) {
	id, typ, found := strings.Cut(value, ":")
	if !found {
		typ = defaultType
	}
	if strings.TrimSpace(id) == "" {
		return "", "", fmt.Errorf("%q: identifier is empty", value)
	}
	if typ != "mandatory" && typ != "optional" {
		return "", "", fmt.Errorf("%q: requirement type must be mandatory or optional", value)
	}
	return id, typ, nil
}

// parseScopedRef splits "ID=scope1,scope2[:mandatory|optional]". Only a trailing
// ":mandatory" or ":optional" is the requirement type, so scopes such as
// "read:org" keep their colons.
func parseScopedRef(value, defaultType string) (id string, scopes []string, typ string, err error) {
	id, rest, found := strings.Cut(value, "=")
	if !found || strings.TrimSpace(id) == "" {
		return "", nil, "", fmt.Errorf("%q: expected ID=scope1,scope2[:mandatory|optional]", value)
	}
	typ = defaultType
	if i := strings.LastIndexByte(rest, ':'); i >= 0 {
		if suffix := rest[i+1:]; suffix == "mandatory" || suffix == "optional" {
			rest, typ = rest[:i], suffix
		}
	}
	for _, s := range strings.Split(rest, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return id, scopes, typ, nil
}

// exclusiveFlags rejects a and b being set together. The flag b may be a bool
// flag, in which case only a true value counts.
func exclusiveFlags(f *pflag.FlagSet, a, b string) error {
	if !f.Changed(a) {
		return nil
	}
	if flag := f.Lookup(b); flag == nil || !f.Changed(b) || flag.Value.String() == "false" {
		return nil
	}
	return fmt.Errorf("--%s and --%s are mutually exclusive", a, b)
}

func stringFlag(f *pflag.FlagSet, name string) string {
	v, _ := f.GetString(name)
	return v
}

func optionalString(f *pflag.FlagSet, name string) *string {
	if !f.Changed(name) {
		return nil
	}
	v, _ := f.GetString(name)
	if v == "" {
		return nil
	}
	return &v
}

func stringSliceFlag(f *pflag.FlagSet, name string) []string {
	v, _ := f.GetStringArray(name)
	return v
}

func cell(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
