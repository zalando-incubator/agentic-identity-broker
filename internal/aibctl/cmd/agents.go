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

var agentFieldFlags = []string{
	"display-name", "description", flagCanonicalID, flagClearCanonicalID, "client-id", "clear-client-id",
	"external-id", "governance-url", "user-documentation-url", "agent-interface-url",
	"permission-set", "service-requirement", "service-requirement-all", "redirect-uri", "allowed-scope", "client-uri",
}

func (c *cli) newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agent",
		Aliases: []string{"agents"},
		Short:   "Manage agents",
	}
	cmd.AddCommand(
		c.newAgentListCommand(),
		c.newAgentGetCommand(),
		c.newAgentCreateCommand(),
		c.newAgentUpdateCommand(),
		c.newAgentDeleteCommand(),
		c.newClientCredentialsCommand(),
	)
	return cmd
}

func addAgentFieldFlags(cmd *cobra.Command, update bool) {
	f := cmd.Flags()
	f.String("display-name", "", "human-readable name")
	f.String("description", "", "description")
	f.String(flagCanonicalID, "", "canonical ID ([A-Za-z0-9._-]{1,128})")
	f.String("client-id", "", "OAuth2 client_id")
	f.String("external-id", "", "identifier in an external governance system")
	f.String("governance-url", "", "governance documentation URL")
	f.String("user-documentation-url", "", "end-user documentation URL")
	f.String("agent-interface-url", "", "agent interface URL")
	f.StringArray("permission-set", nil, "permission set ID[:mandatory|optional] (repeatable; default optional)")
	f.StringArray("service-requirement", nil, "service ID=scope1,scope2[:mandatory|optional] (repeatable; default optional; only a trailing :mandatory or :optional sets the type)")
	f.StringArray("service-requirement-all", nil, "service ID[:mandatory|optional] whose requirement receives every scope the agent's permission sets grant for it (repeatable; default optional)")
	f.StringArray("redirect-uri", nil, "allowed redirect URI (repeatable)")
	f.StringArray("allowed-scope", nil, "allowed scope (repeatable)")
	f.StringArray("client-uri", nil, "CIMD client URI (repeatable)")
	if update {
		f.Bool(flagClearCanonicalID, false, "remove the canonical ID")
		f.Bool("clear-client-id", false, "remove the client_id")
	}
}

func (c *cli) registerAgentFlagCompletions(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("permission-set", c.completeResource(listPermissionSetItems))
	_ = cmd.RegisterFlagCompletionFunc("service-requirement", c.completeResource(listServiceItems))
	_ = cmd.RegisterFlagCompletionFunc("service-requirement-all", c.completeResource(listServiceItems))
}

// applyAgentFlags overlays changed field flags on w.
func applyAgentFlags(f *pflag.FlagSet, w *client.AgentWrite) error {
	if err := exclusiveFlags(f, flagCanonicalID, flagClearCanonicalID); err != nil {
		return err
	}
	if err := exclusiveFlags(f, "client-id", "clear-client-id"); err != nil {
		return err
	}
	if f.Changed("display-name") {
		w.DisplayName = stringFlag(f, "display-name")
	}
	if f.Changed("description") {
		w.Description = stringFlag(f, "description")
	}
	if f.Changed(flagCanonicalID) {
		w.CanonicalID = client.SetString(stringFlag(f, flagCanonicalID))
	}
	if cleared, _ := f.GetBool(flagClearCanonicalID); cleared {
		w.CanonicalID = client.NullString()
	}
	if f.Changed("client-id") {
		w.ClientID = client.SetString(stringFlag(f, "client-id"))
	}
	if cleared, _ := f.GetBool("clear-client-id"); cleared {
		w.ClientID = client.NullString()
	}
	for _, name := range []struct {
		flag string
		dst  **string
	}{
		{"external-id", &w.ExternalID}, {"governance-url", &w.GovernanceURL},
		{"user-documentation-url", &w.UserDocumentationURL}, {"agent-interface-url", &w.AgentInterfaceURL},
	} {
		if f.Changed(name.flag) {
			*name.dst = optionalString(f, name.flag)
		}
	}
	if f.Changed("permission-set") {
		w.PermissionSets = nil
		for _, v := range stringSliceFlag(f, "permission-set") {
			id, typ, err := parseRef(v, "optional")
			if err != nil {
				return fmt.Errorf("--permission-set %w", err)
			}
			w.PermissionSets = append(w.PermissionSets, client.PermissionSetRef{PermissionSetID: id, RequirementType: typ})
		}
	}
	if f.Changed("service-requirement") || f.Changed("service-requirement-all") {
		w.ServiceRequirements = nil
		for _, v := range stringSliceFlag(f, "service-requirement") {
			id, scopes, typ, err := parseScopedRef(v, "optional")
			if err != nil {
				return fmt.Errorf("--service-requirement %w", err)
			}
			w.ServiceRequirements = append(w.ServiceRequirements, client.ServiceRequirement{ServiceID: id, RequirementType: typ, RequiredScopes: scopes})
		}
		for _, v := range stringSliceFlag(f, "service-requirement-all") {
			id, typ, err := parseRef(v, "optional")
			if err != nil {
				return fmt.Errorf("--service-requirement-all %w", err)
			}
			w.ServiceRequirements = append(w.ServiceRequirements, client.ServiceRequirement{ServiceID: id, RequirementType: typ, RequiredScopes: []string{}, RequireAllScopes: true})
		}
	}
	if f.Changed("redirect-uri") {
		w.RedirectURIs = stringSliceFlag(f, "redirect-uri")
	}
	if f.Changed("allowed-scope") {
		w.AllowedScopes = stringSliceFlag(f, "allowed-scope")
	}
	if f.Changed("client-uri") {
		w.ClientURIs = stringSliceFlag(f, "client-uri")
	}
	if w.PermissionSets == nil {
		w.PermissionSets = []client.PermissionSetRef{}
	}
	return nil
}

func (c *cli) newAgentListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List agents",
		Args:    exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			agents, err := rt.Client.ListAgents(ctx)
			if err != nil {
				return err
			}
			if agents == nil {
				agents = []client.Agent{}
			}
			rt.Printer.NoHeaders = noHeaders(cmd)
			return rt.Printer.PrintList(agents, func() output.Table {
				t := output.Table{Header: []string{"ID", "DISPLAY_NAME", "CLIENT_ID", "PERMISSION_SETS", "SERVICES", "UPDATED"}}
				if rt.Printer.IDColumns() {
					t.Header = append(t.Header, "UUID")
				}
				if rt.Printer.Wide() {
					t.Header = append(t.Header, "EXTERNAL_ID", "REDIRECT_URIS", "CLIENT_URIS")
				}
				for _, a := range agents {
					row := []string{
						output.DisplayID(a.CanonicalID, a.ID), a.DisplayName, orDash(output.Deref(a.ClientID)),
						strconv.Itoa(len(a.PermissionSets)), strconv.Itoa(len(a.ServiceRequirements)), a.UpdatedAt,
					}
					if rt.Printer.IDColumns() {
						row = append(row, a.ID)
					}
					if rt.Printer.Wide() {
						row = append(row, orDash(output.Deref(a.ExternalID)), strconv.Itoa(len(a.RedirectURIs)), strconv.Itoa(len(a.ClientURIs)))
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

// stripServiceNames removes the read-only service_name that "get" output carries.
func stripServiceNames(w *client.AgentWrite) {
	for i := range w.ServiceRequirements {
		w.ServiceRequirements[i].ServiceName = ""
	}
}

func permissionSetRefs(refs []client.PermissionSetRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.PermissionSetID+":"+r.RequirementType)
	}
	return out
}

func agentDetails(a *client.Agent) [][2]string {
	var requirements []string
	for _, r := range a.ServiceRequirements {
		scopes := strings.Join(r.RequiredScopes, " ")
		if r.RequireAllScopes {
			scopes = "all scopes"
		}
		requirements = append(requirements, fmt.Sprintf("%s (%s: %s)", r.ServiceID, r.RequirementType, scopes))
	}
	return [][2]string{
		{"ID", output.DisplayID(a.CanonicalID, a.ID)},
		{"UUID", a.ID},
		{"Display Name", a.DisplayName},
		{"Description", a.Description},
		{"Client ID", orDash(output.Deref(a.ClientID))},
		{"External ID", orDash(output.Deref(a.ExternalID))},
		{"Permission Sets", cell(permissionSetRefs(a.PermissionSets))},
		{"Service Requirements", cell(requirements)},
		{"Redirect URIs", cell(a.RedirectURIs)},
		{"Allowed Scopes", cell(a.AllowedScopes)},
		{"Client URIs", cell(a.ClientURIs)},
		{"Governance URL", orDash(output.Deref(a.GovernanceURL))},
		{"User Documentation URL", orDash(output.Deref(a.UserDocumentationURL))},
		{"Agent Interface URL", orDash(output.Deref(a.AgentInterfaceURL))},
		{"Created", a.CreatedAt},
		{"Updated", a.UpdatedAt},
	}
}

func (c *cli) newAgentGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "get ID",
		Aliases:           []string{"describe"},
		Short:             "Show an agent (canonical ID or UUID)",
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listAgentItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			a, err := rt.Client.GetAgent(ctx, args[0])
			if err != nil {
				return err
			}
			return rt.Printer.PrintObject(a, func() [][2]string { return agentDetails(a) })
		},
	}
}

func (c *cli) newAgentCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an agent from a document or field flags",
		Example: `  aibctl agent create -f agent.yaml
  aibctl agent create --display-name "Research Agent" --description "..." \
      --canonical-id research-agent --permission-set github-read:mandatory`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasFile, err := requireFileOrFlags(cmd, agentFieldFlags, true)
			if err != nil {
				return err
			}
			var body client.AgentWrite
			if hasFile {
				if err := c.readDocument(cmd, &body); err != nil {
					return err
				}
				stripServiceNames(&body)
			} else if err := applyAgentFlags(cmd.Flags(), &body); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			a, err := rt.Client.CreateAgent(ctx, body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Agent %q created.\n", output.DisplayID(a.CanonicalID, a.ID))
			return rt.Printer.PrintObject(a, func() [][2]string { return agentDetails(a) })
		},
	}
	addFileFlag(cmd)
	addAgentFieldFlags(cmd, false)
	c.registerAgentFlagCompletions(cmd)
	return cmd
}

func (c *cli) newAgentUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update ID",
		Short: "Replace an agent from a document, or change fields with flags",
		Long: `Update an agent. With --file the document is the complete new representation
(use "aibctl agent get ID -o yaml" as a starting point). With field flags the
current agent is fetched and only the given fields change; list flags such as
--permission-set and --service-requirement-all replace their whole lists.`,
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listAgentItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasFile, err := requireFileOrFlags(cmd, agentFieldFlags, false)
			if err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			var body client.AgentWrite
			if hasFile {
				if err := c.readDocument(cmd, &body); err != nil {
					return err
				}
				stripServiceNames(&body)
			} else {
				current, err := rt.Client.GetAgent(ctx, args[0])
				if err != nil {
					return err
				}
				body = current.Write()
				if err := applyAgentFlags(cmd.Flags(), &body); err != nil {
					return &UsageError{Err: err, Cmd: cmd}
				}
			}
			a, err := rt.Client.UpdateAgent(ctx, args[0], body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Agent %q updated.\n", output.DisplayID(a.CanonicalID, a.ID))
			return rt.Printer.PrintObject(a, func() [][2]string { return agentDetails(a) })
		},
	}
	addFileFlag(cmd)
	addAgentFieldFlags(cmd, true)
	c.registerAgentFlagCompletions(cmd)
	return cmd
}

func (c *cli) newAgentDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete ID",
		Aliases:           []string{"rm"},
		Short:             "Delete an agent and its user grants",
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listAgentItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.confirm(cmd, fmt.Sprintf("Delete agent %q and its user grants", args[0])); err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			if err := rt.Client.DeleteAgent(ctx, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Agent %q deleted.\n", args[0])
			return nil
		},
	}
	addYesFlag(cmd)
	return cmd
}
