package aibctlcmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

func (c *cli) newContextCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "context",
		Aliases: []string{"ctx"},
		Short:   "Manage named broker contexts",
	}
	cmd.AddCommand(
		c.newContextListCommand(),
		c.newContextCurrentCommand(),
		c.newContextUseCommand(),
		c.newContextSetCommand(),
		c.newContextDeleteCommand(),
	)
	return cmd
}

func authSummary(a config.AuthConfig) string {
	var parts []string
	if a.Principal != "" {
		parts = append(parts, "principal")
	}
	if a.Token != "" {
		parts = append(parts, "token")
	}
	if len(a.TokenCommand) > 0 {
		parts = append(parts, "token_command")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "+")
}

func (c *cli) newContextListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List configured contexts",
		Args:    exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			format := loaded.LocalOutput()
			if err := config.CheckOutputFormat(format); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			p := &output.Printer{Out: c.deps.Stdout, Format: output.Format(format), NoHeaders: noHeaders(cmd)}
			cfg := config.Redacted(loaded.File)
			doc, err := config.Document(cfg.Contexts)
			if err != nil {
				return err
			}
			return p.PrintList(doc, func() output.Table {
				t := output.Table{Header: []string{"CURRENT", "NAME", "SERVER", "AUTH", "OUTPUT"}}
				for _, ctx := range cfg.Contexts {
					current := ""
					if ctx.Name == loaded.SelectedContext() {
						current = "*"
					}
					t.Rows = append(t.Rows, []string{current, ctx.Name, ctx.Server, authSummary(ctx.Auth), ctx.Output})
				}
				return t
			})
		},
	}
	addNoHeadersFlag(cmd)
	return cmd
}

func (c *cli) newContextCurrentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the name of the current context",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			name := loaded.SelectedContext()
			if name == "" {
				return &UsageError{Err: fmt.Errorf("no current context set (config: %s)", loaded.Path), Cmd: cmd}
			}
			_, err = fmt.Fprintln(c.deps.Stdout, name)
			return err
		},
	}
}

func (c *cli) newContextUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "use NAME",
		Short:             "Select the current context",
		Args:              exactArgs(1, "NAME"),
		ValidArgsFunction: c.completeContextNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			if err := loaded.File.Use(args[0]); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			if err := config.Save(loaded.Path, loaded.File); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Switched to context %q.\n", args[0])
			return nil
		},
	}
}

func (c *cli) newContextSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set NAME",
		Short: "Create or update a context",
		Long: `Create or update a context. Only the flags you pass are changed; an existing
context keeps its other settings. Pass --use to make it the current context.`,
		Args:              exactArgs(1, "NAME"),
		ValidArgsFunction: c.completeContextNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			name := args[0]
			ctx := config.Context{Name: name}
			if existing, ok := loaded.File.Find(name); ok {
				ctx = *existing
			}
			f := cmd.Flags()
			if f.Changed("server") {
				ctx.Server, _ = f.GetString("server")
			}
			if f.Changed("host-header") {
				ctx.HostHeader, _ = f.GetString("host-header")
			}
			if f.Changed("principal") {
				ctx.Auth.Principal, _ = f.GetString("principal")
			}
			if f.Changed("principal-header") {
				ctx.Auth.PrincipalHeader, _ = f.GetString("principal-header")
			}
			if f.Changed("token") {
				ctx.Auth.Token, _ = f.GetString("token")
				if ctx.Auth.Token != "" {
					ctx.Auth.TokenCommand = nil
				}
			}
			if f.Changed("token-command") {
				raw, _ := f.GetString("token-command")
				ctx.Auth.TokenCommand = strings.Fields(raw)
				if len(ctx.Auth.TokenCommand) > 0 {
					ctx.Auth.Token = ""
				}
			}
			if f.Changed("output") {
				ctx.Output, _ = f.GetString("output")
			}
			if f.Changed("timeout") {
				ctx.Timeout, _ = f.GetDuration("timeout")
			}
			if f.Changed("header") {
				headers, _ := f.GetStringArray("header")
				if ctx.Headers == nil {
					ctx.Headers = map[string]string{}
				}
				for _, h := range headers {
					k, v, ok := strings.Cut(h, "=")
					if !ok || strings.TrimSpace(k) == "" {
						return &UsageError{Err: fmt.Errorf("--header %q must be KEY=VALUE", h), Cmd: cmd}
					}
					if v == "" {
						delete(ctx.Headers, strings.TrimSpace(k))
						continue
					}
					ctx.Headers[strings.TrimSpace(k)] = v
				}
			}
			if ctx.Server == "" {
				return &UsageError{Err: fmt.Errorf("context %q needs --server", name), Cmd: cmd}
			}
			loaded.File.Upsert(ctx)
			if use, _ := f.GetBool("use"); use || loaded.File.CurrentContext == "" {
				loaded.File.CurrentContext = name
			}
			if err := config.Save(loaded.Path, loaded.File); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Context %q saved to %s.\n", name, loaded.Path)
			return nil
		},
	}
	f := cmd.Flags()
	f.String("server", "", "admin API base URL, e.g. http://localhost:14000")
	f.String("host-header", "", "Host header to send (when the URL differs from the broker's admin public_url)")
	f.String("principal", "", "principal sent in the principal header")
	f.String("principal-header", "", "header carrying the principal (default: X-Remote-User)")
	f.String("token", "", "static Bearer token (supports ${ENV_VAR} expansion)")
	f.String("token-command", "", "command printing a Bearer token, split on whitespace")
	f.StringArray("header", nil, "extra request header KEY=VALUE (repeatable; empty VALUE removes)")
	f.StringP("output", "o", "", "default output format for this context")
	f.Duration("timeout", 0, "request timeout for this context")
	f.Bool("use", false, "make this the current context")
	_ = cmd.RegisterFlagCompletionFunc("output", cobra.FixedCompletions(config.OutputFormats, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func (c *cli) newContextDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete NAME",
		Aliases:           []string{"rm"},
		Short:             "Remove a context",
		Args:              exactArgs(1, "NAME"),
		ValidArgsFunction: c.completeContextNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			if err := c.confirm(cmd, fmt.Sprintf("Delete context %q", args[0])); err != nil {
				return err
			}
			if err := loaded.File.Delete(args[0]); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			if err := config.Save(loaded.Path, loaded.File); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Context %q deleted.\n", args[0])
			return nil
		},
	}
	addYesFlag(cmd)
	return cmd
}

func (c *cli) completeContextNames(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	loaded, err := config.Load(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	var names []string
	for _, ctx := range loaded.File.Contexts {
		if strings.HasPrefix(ctx.Name, toComplete) {
			names = append(names, ctx.Name+"\t"+ctx.Server)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}
