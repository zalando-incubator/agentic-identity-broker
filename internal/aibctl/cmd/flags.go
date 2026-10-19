package aibctlcmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	flagFile      = "file"
	flagYes       = "yes"
	flagNoHeaders = "no-headers"
	assumeYesEnv  = "AIBCTL_ASSUME_YES"
)

func addFileFlag(cmd *cobra.Command) {
	cmd.Flags().StringP(flagFile, "f", "", "JSON or YAML document to send (\"-\" for stdin)")
	_ = cmd.MarkFlagFilename(flagFile, "yaml", "yml", "json")
}

func addYesFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP(flagYes, "y", false, "skip the confirmation prompt")
}

func addNoHeadersFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(flagNoHeaders, false, "omit the header row in table output")
}

func noHeaders(cmd *cobra.Command) bool {
	b, _ := cmd.Flags().GetBool(flagNoHeaders)
	return b
}

// confirm asks before a destructive action unless --yes or AIBCTL_ASSUME_YES is set.
// Without an interactive terminal the action is refused as a usage error.
func (c *cli) confirm(cmd *cobra.Command, action string) error {
	if yes, _ := cmd.Flags().GetBool(flagYes); yes {
		return nil
	}
	if v, ok := os.LookupEnv(assumeYesEnv); ok && (v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")) {
		return nil
	}
	if !isTerminal(c.deps.Stdin) {
		return &UsageError{Err: fmt.Errorf("%s requires confirmation: pass --yes or set %s=1", action, assumeYesEnv), Cmd: cmd}
	}
	_, _ = fmt.Fprintf(c.deps.Stderr, "%s? [y/N]: ", action)
	line, err := bufio.NewReader(c.deps.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("aborted")
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return errors.New("aborted")
	}
	return nil
}

func isTerminal(r any) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) // #nosec G115 -- file descriptors fit in int.
}
