package aibctlcmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

var keyAlgorithms = []string{"ES256"}

type keyFamily struct {
	use      string
	aliases  []string
	short    string
	noun     string
	title    string
	basePath string
}

func (c *cli) newSigningKeyCommand() *cobra.Command {
	return c.newKeyFamilyCommand(keyFamily{
		use: "signing-key", aliases: []string{"signing-keys"},
		short: "Manage OAuth2 server token signing keys", noun: "signing key", title: "Signing key", basePath: client.SigningKeysPath,
	})
}

func (c *cli) newCIMDClientKeyCommand() *cobra.Command {
	return c.newKeyFamilyCommand(keyFamily{
		use: "cimd-client-key", aliases: []string{"cimd-client-keys"},
		short: "Manage CIMD client-authentication keys", noun: "CIMD client key", title: "CIMD client key", basePath: client.CIMDClientKeysPath,
	})
}

func keyDetails(k *client.Key) [][2]string {
	return [][2]string{
		{"KID", k.KID},
		{"Algorithm", k.Algorithm},
		{"Current", output.Bool(k.IsCurrent)},
		{"Activates At", k.ActivatesAt},
		{"Created", k.CreatedAt},
		{"Removed", orDash(output.Deref(k.RemovedAt))},
	}
}

func (c *cli) newKeyFamilyCommand(family keyFamily) *cobra.Command {
	cmd := &cobra.Command{Use: family.use, Aliases: family.aliases, Short: family.short}

	completeKIDs := c.completeResource(func(ctx context.Context, cl *client.Client, _ []string) ([]completionItem, error) {
		keys, err := cl.ListKeys(ctx, family.basePath)
		if err != nil {
			return nil, err
		}
		items := make([]completionItem, 0, len(keys))
		for _, k := range keys {
			desc := k.Algorithm
			if k.IsCurrent {
				desc += " (current)"
			}
			items = append(items, completionItem{id: k.KID, description: desc})
		}
		return items, nil
	})

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List " + family.noun + "s, newest first",
		Args:    exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			keys, err := rt.Client.ListKeys(ctx, family.basePath)
			if err != nil {
				return err
			}
			if keys == nil {
				keys = []client.Key{}
			}
			rt.Printer.NoHeaders = noHeaders(cmd)
			return rt.Printer.PrintList(keys, func() output.Table {
				t := output.Table{Header: []string{"KID", "ALGORITHM", "CURRENT", "ACTIVATES_AT", "CREATED_AT"}}
				for _, k := range keys {
					t.Rows = append(t.Rows, []string{k.KID, k.Algorithm, output.Bool(k.IsCurrent), k.ActivatesAt, k.CreatedAt})
				}
				return t
			})
		},
	}
	addNoHeadersFlag(list)

	create := &cobra.Command{
		Use:   "create",
		Short: "Generate a new " + family.noun,
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			algorithm, _ := cmd.Flags().GetString("algorithm")
			k, err := rt.Client.CreateKey(ctx, family.basePath, algorithm)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "%s %q created.\n", family.title, k.KID)
			return rt.Printer.PrintObject(k, func() [][2]string { return keyDetails(k) })
		},
	}
	create.Flags().String("algorithm", "", "signature algorithm (default: ES256)")
	_ = create.RegisterFlagCompletionFunc("algorithm", cobra.FixedCompletions(keyAlgorithms, cobra.ShellCompDirectiveNoFileComp))

	setCurrent := &cobra.Command{
		Use:               "set-current KID",
		Aliases:           []string{"promote"},
		Short:             "Make a " + family.noun + " the current one",
		Args:              exactArgs(1, "KID"),
		ValidArgsFunction: completeKIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			k, err := rt.Client.PromoteKey(ctx, family.basePath, args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "%s %q is now current.\n", family.title, k.KID)
			return rt.Printer.PrintObject(k, func() [][2]string { return keyDetails(k) })
		},
	}

	del := &cobra.Command{
		Use:               "delete KID",
		Aliases:           []string{"rm"},
		Short:             "Remove a " + family.noun + " (not the current or last one)",
		Args:              exactArgs(1, "KID"),
		ValidArgsFunction: completeKIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.confirm(cmd, fmt.Sprintf("Delete %s %q", family.noun, args[0])); err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			if err := rt.Client.DeleteKey(ctx, family.basePath, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "%s %q deleted.\n", family.title, args[0])
			return nil
		},
	}
	addYesFlag(del)

	cmd.AddCommand(list, create, setCurrent, del)
	return cmd
}
