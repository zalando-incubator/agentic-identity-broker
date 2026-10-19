package aibctlcmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

func (c *cli) newProtectedResourceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "protected-resource",
		Aliases: []string{"protected-resources", "pr"},
		Short:   "Manage the protected resource URIs of a service",
	}
	cmd.AddCommand(
		c.newProtectedResourceListCommand(),
		c.newProtectedResourceAddCommand(),
		c.newProtectedResourceRemoveCommand(),
		c.newProtectedResourceRenameCommand(),
	)
	return cmd
}

// completeServiceThenResource completes the service for the first argument and
// that service's resources afterwards.
func (c *cli) completeServiceThenResource(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return c.completeResource(listServiceItems)(cmd, args, toComplete)
	}
	return c.completeResource(func(ctx context.Context, cl *client.Client, args []string) ([]completionItem, error) {
		resources, _, err := cl.ListProtectedResources(ctx, args[0])
		if err != nil {
			return nil, err
		}
		items := make([]completionItem, 0, len(resources))
		for _, r := range resources {
			items = append(items, completionItem{id: r})
		}
		return items, nil
	})(cmd, args, toComplete)
}

func printResourceSet(p *output.Printer, resources []string) error {
	if resources == nil {
		resources = []string{}
	}
	return p.PrintList(client.ProtectedResourceSet{ProtectedResources: resources}, func() output.Table {
		t := output.Table{Header: []string{"RESOURCE"}}
		for _, r := range resources {
			t.Rows = append(t.Rows, []string{r})
		}
		return t
	})
}

func (c *cli) newProtectedResourceListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list SERVICE",
		Aliases:           []string{"ls"},
		Short:             "List the protected resources of a service",
		Args:              exactArgs(1, "SERVICE"),
		ValidArgsFunction: c.completeServiceThenResource,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			resources, _, err := rt.Client.ListProtectedResources(ctx, args[0])
			if err != nil {
				return err
			}
			rt.Printer.NoHeaders = noHeaders(cmd)
			return printResourceSet(rt.Printer, resources)
		},
	}
	addNoHeadersFlag(cmd)
	return cmd
}

func (c *cli) newProtectedResourceAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "add SERVICE URI...",
		Short:             "Add protected resource URIs (idempotent)",
		Args:              minArgs(2, "SERVICE", "URI"),
		ValidArgsFunction: c.completeServiceThenResource,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			var last *client.ProtectedResourceMutation
			for _, uri := range args[1:] {
				result, created, err := rt.Client.AddProtectedResource(ctx, args[0], uri)
				if err != nil {
					return fmt.Errorf("add %s: %w", uri, err)
				}
				if created {
					_, _ = fmt.Fprintf(c.deps.Stderr, "Added %s.\n", result.Resource)
				} else {
					_, _ = fmt.Fprintf(c.deps.Stderr, "%s was already protected.\n", result.Resource)
				}
				last = result
			}
			return printResourceSet(rt.Printer, last.ProtectedResources)
		},
	}
}

func (c *cli) newProtectedResourceRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove SERVICE URI...",
		Aliases:           []string{"rm"},
		Short:             "Remove protected resource URIs",
		Args:              minArgs(2, "SERVICE", "URI"),
		ValidArgsFunction: c.completeServiceThenResource,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.confirm(cmd, fmt.Sprintf("Remove %d protected resource(s) from %q", len(args)-1, args[0])); err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			var last *client.ProtectedResourceMutation
			for _, uri := range args[1:] {
				result, err := rt.Client.RemoveProtectedResource(ctx, args[0], uri)
				if err != nil {
					return fmt.Errorf("remove %s: %w", uri, err)
				}
				_, _ = fmt.Fprintf(c.deps.Stderr, "Removed %s.\n", result.Resource)
				last = result
			}
			return printResourceSet(rt.Printer, last.ProtectedResources)
		},
	}
	addYesFlag(cmd)
	return cmd
}

func (c *cli) newProtectedResourceRenameCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "rename SERVICE FROM TO",
		Short:             "Rename a protected resource URI",
		Args:              exactArgs(3, "SERVICE", "FROM", "TO"),
		ValidArgsFunction: c.completeServiceThenResource,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			result, err := rt.Client.RenameProtectedResource(ctx, args[0], args[1], args[2])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Renamed %s to %s.\n", args[1], result.Resource)
			return printResourceSet(rt.Printer, result.ProtectedResources)
		},
	}
}
