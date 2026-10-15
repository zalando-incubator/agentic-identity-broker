package aibctlcmd

import (
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

const sweepDefaultTimeout = 15 * time.Minute

func (c *cli) newSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"sessions"},
		Short:   "Operate on users' stored third-party OAuth2 sessions",
	}
	sweep := &cobra.Command{
		Use:   "sweep",
		Short: "Refresh third-party sessions whose access tokens expire soon",
		Long: `Refresh every stored third-party session whose access token expires within
the lookahead window, across all users and services. Without --dry-run this is
a real sweep. aibctl waits up to 15 minutes unless --timeout or AIBCTL_TIMEOUT
sets another limit; a timeout stops the sweep after the session in progress,
and earlier refreshes stay saved.`,
		Example: `  aibctl session sweep --dry-run --lookahead PT10M
  aibctl session sweep --lookahead PT10M --page-size 100`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			overlay, err := config.LoadOverlay(cmd)
			if err != nil {
				return &UsageError{Err: err, Cmd: cmd}
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			var body client.SessionSweepRequest
			f := cmd.Flags()
			if f.Changed("lookahead") {
				v, _ := f.GetString("lookahead")
				body.LookaheadDuration = &v
			}
			if f.Changed("dry-run") {
				v, _ := f.GetBool("dry-run")
				body.DryRun = &v
			}
			if f.Changed("page-size") {
				v, _ := f.GetInt("page-size")
				body.PageSize = &v
			}
			timeout := sweepDefaultTimeout
			if overlay.Timeout != nil {
				timeout = *overlay.Timeout
			}
			ctx, cancel := requestContext(cmd, timeout)
			defer cancel()
			result, err := rt.Client.SweepSessions(ctx, body)
			if err != nil {
				return err
			}
			return rt.Printer.PrintObject(result, func() [][2]string {
				return [][2]string{
					{"Dry Run", output.Bool(result.DryRun)},
					{"Refreshed", strconv.Itoa(result.Refreshed)},
					{"Skipped", strconv.Itoa(result.Skipped)},
					{"Failed", strconv.Itoa(result.Failed)},
					{"Total Evaluated", strconv.Itoa(result.TotalEvaluated)},
				}
			})
		},
	}
	sweep.Flags().String("lookahead", "", "ISO 8601 window such as PT10M or P1D (default: the broker's configured lookahead)")
	sweep.Flags().Bool("dry-run", false, "count candidates without refreshing or saving anything")
	sweep.Flags().Int("page-size", 0, "sessions read per page, 1-1000 (default: the broker's configured page size)")
	cmd.AddCommand(sweep)
	return cmd
}
