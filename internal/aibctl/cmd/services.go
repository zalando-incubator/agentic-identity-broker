package aibctlcmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

const flagIfMatch = "if-match"

var serviceFieldFlags = []string{
	"display-name", flagCanonicalID, flagClearCanonicalID, "client-id", "client-secret", "auth-method", "oauth2-flavor",
	"issuer-uri", "enable-discovery", "metadata-url", "token-endpoint", "authorize-endpoint", "scope",
	"protected-resource", "authorization-param",
}

var (
	oauth2Flavors = []string{"standard", "google", "github"}
	authMethods   = []string{"none", "private_key_jwt"}
)

func (c *cli) newServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"services", "svc"},
		Short:   "Manage third-party OAuth2 services",
	}
	cmd.AddCommand(
		c.newServiceListCommand(),
		c.newServiceGetCommand(),
		c.newServiceCreateCommand(),
		c.newServiceUpdateCommand(),
		c.newServiceDeleteCommand(),
		c.newProtectedResourceCommand(),
	)
	return cmd
}

func addServiceFieldFlags(cmd *cobra.Command, update bool) {
	f := cmd.Flags()
	f.String("display-name", "", "human-readable name")
	f.String(flagCanonicalID, "", "canonical ID ([A-Za-z0-9._-]{1,128})")
	f.String("client-id", "", "OAuth2 client_id registered at the provider")
	f.String("client-secret", "", "OAuth2 client_secret (static confidential services; PUT always replaces it)")
	f.String("auth-method", "", "token_endpoint_auth_method: none (public) or private_key_jwt (CIMD); omit for a static secret")
	f.String("oauth2-flavor", "", "provider flavor: standard, google, github")
	f.String("issuer-uri", "", "provider issuer URI")
	f.Bool("enable-discovery", false, "discover endpoints from the provider metadata")
	f.String("metadata-url", "", "explicit RFC 8414 metadata URL")
	f.String("token-endpoint", "", "token endpoint (when discovery is disabled)")
	f.String("authorize-endpoint", "", "authorization endpoint (when discovery is disabled)")
	f.StringArray("scope", nil, "scope VALUE[=DESCRIPTION] (repeatable)")
	f.StringArray("protected-resource", nil, "protected resource URI (repeatable; replaces the set on update)")
	f.StringArray("authorization-param", nil, "extra authorization request parameter KEY=VALUE (repeatable)")
	_ = cmd.RegisterFlagCompletionFunc("oauth2-flavor", cobra.FixedCompletions(oauth2Flavors, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("auth-method", cobra.FixedCompletions(authMethods, cobra.ShellCompDirectiveNoFileComp))
	if update {
		f.Bool(flagClearCanonicalID, false, "remove the canonical ID")
		f.String(flagIfMatch, "", "ETag the service must still have (default: the ETag read before updating)")
	}
}

func applyServiceFlags(f *pflag.FlagSet, w *client.ServiceWrite) error {
	if err := exclusiveFlags(f, flagCanonicalID, flagClearCanonicalID); err != nil {
		return err
	}
	if f.Changed("display-name") {
		w.DisplayName = stringFlag(f, "display-name")
	}
	if f.Changed(flagCanonicalID) {
		w.CanonicalID = client.SetString(stringFlag(f, flagCanonicalID))
	}
	if cleared, _ := f.GetBool(flagClearCanonicalID); cleared {
		w.CanonicalID = client.NullString()
	}
	if f.Changed("client-id") {
		w.ClientID = stringFlag(f, "client-id")
	}
	if f.Changed("client-secret") {
		w.ClientSecret = stringFlag(f, "client-secret")
	}
	if f.Changed("auth-method") {
		w.TokenEndpointAuthMethod = optionalString(f, "auth-method")
	}
	if f.Changed("oauth2-flavor") {
		w.OAuth2Flavor = stringFlag(f, "oauth2-flavor")
	}
	if f.Changed("issuer-uri") {
		w.IssuerURI = stringFlag(f, "issuer-uri")
	}
	if f.Changed("enable-discovery") {
		w.Discovery.EnableDiscovery, _ = f.GetBool("enable-discovery")
	}
	if f.Changed("metadata-url") {
		w.Discovery.MetadataURL = optionalString(f, "metadata-url")
	}
	if f.Changed("token-endpoint") || f.Changed("authorize-endpoint") {
		if w.Endpoints == nil {
			w.Endpoints = &client.Endpoints{}
		}
		if f.Changed("token-endpoint") {
			w.Endpoints.TokenEndpoint = stringFlag(f, "token-endpoint")
		}
		if f.Changed("authorize-endpoint") {
			w.Endpoints.AuthorizeEndpoint = stringFlag(f, "authorize-endpoint")
		}
	}
	if f.Changed("scope") {
		w.Scopes = nil
		for _, v := range stringSliceFlag(f, "scope") {
			value, description, _ := strings.Cut(v, "=")
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("--scope %q: value is empty", v)
			}
			w.Scopes = append(w.Scopes, client.Scope{ScopeValue: value, Description: description})
		}
	}
	if f.Changed("protected-resource") {
		w.ProtectedResources = stringSliceFlag(f, "protected-resource")
		if w.ProtectedResources == nil {
			w.ProtectedResources = []string{}
		}
	}
	if f.Changed("authorization-param") {
		w.AuthorizationParams = map[string]string{}
		for _, v := range stringSliceFlag(f, "authorization-param") {
			k, val, ok := strings.Cut(v, "=")
			if !ok || strings.TrimSpace(k) == "" {
				return fmt.Errorf("--authorization-param %q must be KEY=VALUE", v)
			}
			w.AuthorizationParams[k] = val
		}
	}
	if w.Scopes == nil {
		w.Scopes = []client.Scope{}
	}
	return nil
}

// omitGeneratedClientID drops the client_id copied from the current service when the
// replacement uses private_key_jwt: the broker generates that ID and rejects any
// client_id in such a request. A client_id the operator changed is kept so the
// server reports it.
func omitGeneratedClientID(w *client.ServiceWrite, current *client.Service) {
	if output.Deref(w.TokenEndpointAuthMethod) == "private_key_jwt" && w.ClientID == current.ClientID {
		w.ClientID = ""
	}
}

// checkServiceSecret refuses to send the redacted placeholder and, for a static
// confidential service, requires a new secret because PUT replaces it.
func checkServiceSecret(w *client.ServiceWrite, current *client.Service) error {
	if w.ClientSecret == client.RedactedSecret {
		return fmt.Errorf("client_secret is the redacted placeholder %q; set the real secret (PUT replaces the stored secret)", client.RedactedSecret)
	}
	if w.ClientSecret != "" {
		return nil
	}
	method := output.Deref(w.TokenEndpointAuthMethod)
	if method == "" && current != nil && current.ClientSecret != nil {
		return fmt.Errorf("service uses a static client_secret and PUT replaces it: pass --client-secret (or client_secret in the document)")
	}
	return nil
}

func authMethodLabel(method *string) string {
	if method == nil || *method == "" {
		return "client_secret"
	}
	return *method
}

func discoveryLabel(d client.Discovery) string {
	if !d.EnableDiscovery {
		return "manual"
	}
	if d.MetadataURL != nil && *d.MetadataURL != "" {
		return *d.MetadataURL
	}
	return "issuer"
}

func scopeValues(scopes []client.Scope) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, s.ScopeValue)
	}
	return out
}

func serviceDetails(s *client.Service) [][2]string {
	var params []string
	for k, v := range s.AuthorizationParams {
		params = append(params, k+"="+v)
	}
	return [][2]string{
		{"ID", output.DisplayID(s.CanonicalID, s.ID)},
		{"UUID", s.ID},
		{"Display Name", s.DisplayName},
		{"OAuth2 Flavor", s.OAuth2Flavor},
		{"Auth Method", authMethodLabel(s.TokenEndpointAuthMethod)},
		{"Client ID", orDash(s.ClientID)},
		{"Client Secret", orDash(output.Deref(s.ClientSecret))},
		{"Issuer URI", orDash(s.IssuerURI)},
		{"Discovery", discoveryLabel(s.Discovery)},
		{"Token Endpoint", orDash(s.Endpoints.TokenEndpoint)},
		{"Authorize Endpoint", orDash(s.Endpoints.AuthorizeEndpoint)},
		{"Scopes", cell(scopeValues(s.Scopes))},
		{"Protected Resources", cell(s.ProtectedResources)},
		{"Authorization Params", cell(params)},
		{"Created", s.CreatedAt},
		{"Updated", s.UpdatedAt},
	}
}

func (c *cli) newServiceListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List services",
		Args:    exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			services, err := rt.Client.ListServices(ctx)
			if err != nil {
				return err
			}
			if services == nil {
				services = []client.Service{}
			}
			rt.Printer.NoHeaders = noHeaders(cmd)
			return rt.Printer.PrintList(services, func() output.Table {
				t := output.Table{Header: []string{"ID", "DISPLAY_NAME", "FLAVOR", "AUTH_METHOD", "ISSUER", "PROTECTED_RESOURCES", "UPDATED"}}
				if rt.Printer.IDColumns() {
					t.Header = append(t.Header, "UUID")
				}
				if rt.Printer.Wide() {
					t.Header = append(t.Header, "CLIENT_ID", "DISCOVERY", "SCOPES")
				}
				for _, s := range services {
					row := []string{
						output.DisplayID(s.CanonicalID, s.ID), s.DisplayName, s.OAuth2Flavor, authMethodLabel(s.TokenEndpointAuthMethod),
						orDash(s.IssuerURI), strconv.Itoa(len(s.ProtectedResources)), s.UpdatedAt,
					}
					if rt.Printer.IDColumns() {
						row = append(row, s.ID)
					}
					if rt.Printer.Wide() {
						row = append(row, orDash(s.ClientID), discoveryLabel(s.Discovery), strconv.Itoa(len(s.Scopes)))
					}
					t.Rows = append(t.Rows, row)
				}
				return t
			})
		},
	}
	addNoHeadersFlag(cmd)
	return cmd
}

func (c *cli) newServiceGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "get ID",
		Aliases:           []string{"describe"},
		Short:             "Show a service (canonical ID or UUID)",
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listServiceItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			s, _, err := rt.Client.GetService(ctx, args[0])
			if err != nil {
				return err
			}
			return rt.Printer.PrintObject(s, func() [][2]string { return serviceDetails(s) })
		},
	}
}

func (c *cli) newServiceCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Register a service from a document or field flags",
		Example: `  aibctl service create -f service.yaml
  aibctl service create --display-name GitHub --canonical-id github \
      --client-id "$GITHUB_CLIENT_ID" --client-secret "$GITHUB_CLIENT_SECRET" \
      --issuer-uri https://github.com --oauth2-flavor github \
      --token-endpoint https://github.com/login/oauth/access_token \
      --authorize-endpoint https://github.com/login/oauth/authorize \
      --scope repo="Repository access" --protected-resource https://api.github.com/`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasFile, err := requireFileOrFlags(cmd, serviceFieldFlags, true)
			if err != nil {
				return err
			}
			var body client.ServiceWrite
			if hasFile {
				if err := c.readDocument(cmd, &body); err != nil {
					return err
				}
			} else if err := applyServiceFlags(cmd.Flags(), &body); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			if err := checkServiceSecret(&body, nil); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			s, err := rt.Client.CreateService(ctx, body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Service %q created.\n", output.DisplayID(s.CanonicalID, s.ID))
			return rt.Printer.PrintObject(s, func() [][2]string { return serviceDetails(s) })
		},
	}
	addFileFlag(cmd)
	addServiceFieldFlags(cmd, false)
	return cmd
}

func (c *cli) newServiceUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update ID",
		Short: "Replace a service from a document, or change fields with flags",
		Long: `Update a service. With --file the document is the complete new representation
(use "aibctl service get ID -o yaml" as a starting point and replace the redacted
client_secret). With field flags the current service is fetched and only the
given fields change. The update is conditional on the ETag read beforehand, or
on --if-match, so a concurrent change fails with exit code 5.`,
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listServiceItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasFile, err := requireFileOrFlags(cmd, serviceFieldFlags, false)
			if err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			current, etag, err := rt.Client.GetService(ctx, args[0])
			if err != nil {
				return err
			}
			var body client.ServiceWrite
			if hasFile {
				if err := c.readDocument(cmd, &body); err != nil {
					return err
				}
			} else {
				body = current.Write()
				if err := applyServiceFlags(cmd.Flags(), &body); err != nil {
					return &UsageError{Err: err, Cmd: cmd}
				}
			}
			omitGeneratedClientID(&body, current)
			if err := checkServiceSecret(&body, current); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			if v, _ := cmd.Flags().GetString(flagIfMatch); v != "" {
				etag = v
			}
			s, _, err := rt.Client.UpdateService(ctx, args[0], body, etag)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Service %q updated.\n", output.DisplayID(s.CanonicalID, s.ID))
			return rt.Printer.PrintObject(s, func() [][2]string { return serviceDetails(s) })
		},
	}
	addFileFlag(cmd)
	addServiceFieldFlags(cmd, true)
	return cmd
}

func (c *cli) newServiceDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete ID",
		Aliases:           []string{"rm"},
		Short:             "Delete a service (fails while user grants reference it)",
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listServiceItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.confirm(cmd, fmt.Sprintf("Delete service %q", args[0])); err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			if err := rt.Client.DeleteService(ctx, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Service %q deleted.\n", args[0])
			return nil
		},
	}
	addYesFlag(cmd)
	return cmd
}
