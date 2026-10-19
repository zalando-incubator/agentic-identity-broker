package aibctlcmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/version"
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

func (c *cli) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the aibctl version",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionInfo{
				Version:   version.Version,
				Commit:    version.Commit,
				GoVersion: runtime.Version(),
				Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			}
			overlay, err := config.LoadOverlay(cmd)
			if err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			format := ""
			if overlay.Output != nil {
				format = *overlay.Output
			}
			switch format {
			case string(output.FormatJSON), string(output.FormatYAML):
				p := &output.Printer{Out: c.deps.Stdout, Format: output.Format(format)}
				return p.PrintObject(info, nil)
			case "", string(output.FormatTable), string(output.FormatWide):
			default:
				return &UsageError{Err: config.CheckOutputFormat(format), Cmd: cmd}
			}
			_, err = fmt.Fprintf(c.deps.Stdout, "aibctl %s (commit %s, %s, %s)\n", info.Version, info.Commit, info.GoVersion, info.Platform)
			return err
		},
	}
}
