package aibctlcmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

func (c *cli) newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the aibctl configuration file",
	}

	view := &cobra.Command{
		Use:   "view",
		Short: "Print the configuration file with secrets redacted",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			format := loaded.LocalOutput()
			if err := config.CheckOutputFormat(format); err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			cfg := loaded.File
			if show, _ := cmd.Flags().GetBool("show-secrets"); !show {
				cfg = config.Redacted(cfg)
			}
			if format == string(output.FormatJSON) {
				doc, err := config.Document(cfg)
				if err != nil {
					return err
				}
				return (&output.Printer{Out: c.deps.Stdout, Format: output.FormatJSON}).PrintObject(doc, nil)
			}
			enc := yaml.NewEncoder(c.deps.Stdout)
			enc.SetIndent(2)
			if err := enc.Encode(cfg); err != nil {
				return err
			}
			return enc.Close()
		},
	}
	view.Flags().Bool("show-secrets", false, "print tokens and sensitive header values in clear text")

	path := &cobra.Command{
		Use:   "path",
		Short: "Print the resolved configuration file path",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := c.loadConfig(cmd)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.deps.Stdout, loaded.Path)
			return err
		},
	}

	cmd.AddCommand(view, path)
	return cmd
}
