package aibctlcmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func (c *cli) newHealthCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check the admin server health endpoint",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			h, err := rt.Client.Health(ctx)
			if err != nil {
				return err
			}
			return rt.Printer.PrintObject(h, func() [][2]string {
				return [][2]string{
					{"Status", h.Status},
					{"Uptime", formatDuration(time.Duration(h.UptimeSeconds) * time.Second)},
					{"Timestamp", h.Timestamp},
					{"Server", fmt.Sprint(rt.Config.Server)},
				}
			})
		},
	}
}
