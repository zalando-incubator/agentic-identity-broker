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

var permissionSetFieldFlags = []string{"name", "description", flagCanonicalID, flagClearCanonicalID, "service-scope"}

func (c *cli) newPermissionSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "permission-set",
		Aliases: []string{"permission-sets", "ps"},
		Short:   "Manage permission sets",
	}
	cmd.AddCommand(
		c.newPermissionSetListCommand(),
		c.newPermissionSetGetCommand(),
		c.newPermissionSetCreateCommand(),
		c.newPermissionSetUpdateCommand(),
		c.newPermissionSetDeleteCommand(),
	)
	return cmd
}

func (c *cli) addPermissionSetFieldFlags(cmd *cobra.Command, update bool) {
	f := cmd.Flags()
	f.String("name", "", "unique name")
	f.String("description", "", "description")
	f.String(flagCanonicalID, "", "canonical ID ([A-Za-z0-9._-]{1,128})")
	f.StringArray("service-scope", nil, "service ID=scope1,scope2[:mandatory|optional] (repeatable; default optional; only a trailing :mandatory or :optional sets the type)")
	_ = cmd.RegisterFlagCompletionFunc("service-scope", c.completeResource(listServiceItems))
	if update {
		f.Bool(flagClearCanonicalID, false, "remove the canonical ID")
	}
}

func applyPermissionSetFlags(f *pflag.FlagSet, w *client.PermissionSetWrite) error {
	if err := exclusiveFlags(f, flagCanonicalID, flagClearCanonicalID); err != nil {
		return err
	}
	if f.Changed("name") {
		w.Name = stringFlag(f, "name")
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
	if f.Changed("service-scope") {
		w.ServiceScopes = nil
		for _, v := range stringSliceFlag(f, "service-scope") {
			id, scopes, typ, err := parseScopedRef(v, "optional")
			if err != nil {
				return fmt.Errorf("--service-scope %w", err)
			}
			w.ServiceScopes = append(w.ServiceScopes, client.ServiceScope{ServiceID: id, Scopes: scopes, RequirementType: typ})
		}
	}
	if w.ServiceScopes == nil {
		w.ServiceScopes = []client.ServiceScope{}
	}
	return nil
}

func serviceScopeRefs(scopes []client.ServiceScope) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, fmt.Sprintf("%s=%s:%s", s.ServiceID, strings.Join(s.Scopes, "+"), s.RequirementType))
	}
	return out
}

func scopeCount(scopes []client.ServiceScope) int {
	count := 0
	for _, s := range scopes {
		count += len(s.Scopes)
	}
	return count
}

func permissionSetDetails(p *client.PermissionSet) [][2]string {
	return [][2]string{
		{"ID", output.DisplayID(p.CanonicalID, p.ID)},
		{"UUID", p.ID},
		{"Name", p.Name},
		{"Description", p.Description},
		{"Service Scopes", cell(serviceScopeRefs(p.ServiceScopes))},
		{"Created", p.CreatedAt},
		{"Updated", p.UpdatedAt},
	}
}

func (c *cli) newPermissionSetListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List permission sets",
		Args:    exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			service, _ := cmd.Flags().GetString("service")
			sets, err := rt.Client.ListPermissionSets(ctx, service)
			if err != nil {
				return err
			}
			if sets == nil {
				sets = []client.PermissionSet{}
			}
			rt.Printer.NoHeaders = noHeaders(cmd)
			return rt.Printer.PrintList(sets, func() output.Table {
				t := output.Table{Header: []string{"ID", "NAME", "SERVICES", "SCOPES", "UPDATED"}}
				if rt.Printer.IDColumns() {
					t.Header = append(t.Header, "UUID")
				}
				if rt.Printer.Wide() {
					t.Header = append(t.Header, "DESCRIPTION")
				}
				for _, p := range sets {
					row := []string{output.DisplayID(p.CanonicalID, p.ID), p.Name, strconv.Itoa(len(p.ServiceScopes)), strconv.Itoa(scopeCount(p.ServiceScopes)), p.UpdatedAt}
					if rt.Printer.IDColumns() {
						row = append(row, p.ID)
					}
					if rt.Printer.Wide() {
						row = append(row, p.Description)
					}
					t.Rows = append(t.Rows, row)
				}
				return t
			})
		},
	}
	cmd.Flags().String("service", "", "only permission sets that reference this service (canonical ID or UUID)")
	_ = cmd.RegisterFlagCompletionFunc("service", c.completeResource(listServiceItems))
	addNoHeadersFlag(cmd)
	return cmd
}

func (c *cli) newPermissionSetGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "get ID",
		Aliases:           []string{"describe"},
		Short:             "Show a permission set (canonical ID or UUID)",
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listPermissionSetItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			p, err := rt.Client.GetPermissionSet(ctx, args[0])
			if err != nil {
				return err
			}
			return rt.Printer.PrintObject(p, func() [][2]string { return permissionSetDetails(p) })
		},
	}
}

func (c *cli) newPermissionSetCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a permission set from a document or field flags",
		Example: `  aibctl permission-set create -f permission-set.yaml
  aibctl permission-set create --name "GitHub Read" --description "..." \
      --canonical-id github-read --service-scope github=read,repo:mandatory`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			hasFile, err := requireFileOrFlags(cmd, permissionSetFieldFlags, true)
			if err != nil {
				return err
			}
			var body client.PermissionSetWrite
			if hasFile {
				if err := c.readDocument(cmd, &body); err != nil {
					return err
				}
			} else if err := applyPermissionSetFlags(cmd.Flags(), &body); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			p, err := rt.Client.CreatePermissionSet(ctx, body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Permission set %q created.\n", output.DisplayID(p.CanonicalID, p.ID))
			return rt.Printer.PrintObject(p, func() [][2]string { return permissionSetDetails(p) })
		},
	}
	addFileFlag(cmd)
	c.addPermissionSetFieldFlags(cmd, false)
	return cmd
}

func (c *cli) newPermissionSetUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update ID",
		Short: "Replace a permission set from a document, or change fields with flags",
		Long: `Update a permission set. With --file the document is the complete new
representation. With field flags the current permission set is fetched and only
the given fields change; --service-scope replaces the whole list.`,
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listPermissionSetItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasFile, err := requireFileOrFlags(cmd, permissionSetFieldFlags, false)
			if err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			var body client.PermissionSetWrite
			if hasFile {
				if err := c.readDocument(cmd, &body); err != nil {
					return err
				}
			} else {
				current, err := rt.Client.GetPermissionSet(ctx, args[0])
				if err != nil {
					return err
				}
				body = current.Write()
				if err := applyPermissionSetFlags(cmd.Flags(), &body); err != nil {
					return &UsageError{Err: err, Cmd: cmd}
				}
			}
			p, err := rt.Client.UpdatePermissionSet(ctx, args[0], body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Permission set %q updated.\n", output.DisplayID(p.CanonicalID, p.ID))
			return rt.Printer.PrintObject(p, func() [][2]string { return permissionSetDetails(p) })
		},
	}
	addFileFlag(cmd)
	c.addPermissionSetFieldFlags(cmd, true)
	return cmd
}

func (c *cli) newPermissionSetDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete ID",
		Aliases:           []string{"rm"},
		Short:             "Delete a permission set",
		Args:              exactArgs(1, "ID"),
		ValidArgsFunction: c.completeResource(listPermissionSetItems),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.confirm(cmd, fmt.Sprintf("Delete permission set %q", args[0])); err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			if err := rt.Client.DeletePermissionSet(ctx, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Permission set %q deleted.\n", args[0])
			return nil
		},
	}
	addYesFlag(cmd)
	return cmd
}
