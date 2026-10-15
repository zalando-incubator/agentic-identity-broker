// Package aibctlcmd builds the aibctl command tree. NewRootCommand takes all
// process dependencies explicitly so tests can run commands in-process.
package aibctlcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/auth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/version"
)

// Deps are the process-level dependencies of a command invocation.
type Deps struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	HTTPClient *http.Client
}

// Runtime is the resolved configuration and the API client for one invocation.
type Runtime struct {
	Config  *config.Resolved
	Client  *client.Client
	Printer *output.Printer
}

// Context derives a request context bounded by the configured timeout.
func (rt *Runtime) Context(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return requestContext(cmd, rt.Config.Timeout)
}

func requestContext(cmd *cobra.Command, timeout time.Duration) (context.Context, context.CancelFunc) {
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

type cli struct {
	deps Deps

	mu sync.Mutex
	rt *Runtime
}

// NewRootCommand builds the full command tree.
func NewRootCommand(deps Deps) *cobra.Command {
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}
	if deps.Stdin == nil {
		deps.Stdin = os.Stdin
	}
	c := &cli{deps: deps}

	root := &cobra.Command{
		Use:   "aibctl",
		Short: "Administer the Agentic Identity Broker",
		Long: `aibctl manages agents, services, permission sets, keys, and session sweeps
through the broker's Admin API. Connection and credentials come from named
contexts in the per-user configuration file (see "aibctl config path").`,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: false,
		},
	}
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.SetIn(deps.Stdin)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &UsageError{Err: err, Cmd: cmd}
	})
	config.RegisterFlags(root)
	_ = root.RegisterFlagCompletionFunc("output", cobra.FixedCompletions(config.OutputFormats, cobra.ShellCompDirectiveNoFileComp))
	_ = root.RegisterFlagCompletionFunc("context", c.completeContextNames)
	_ = root.MarkPersistentFlagFilename("config", "yaml", "yml")

	root.AddCommand(
		c.newVersionCommand(),
		c.newConfigCommand(),
		c.newContextCommand(),
		c.newHealthCommand(),
		c.newAgentCommand(),
		c.newServiceCommand(),
		c.newPermissionSetCommand(),
		c.newSigningKeyCommand(),
		c.newCIMDClientKeyCommand(),
		c.newSessionCommand(),
	)
	return root
}

// Execute runs the command tree with args and returns the process exit code.
func Execute(deps Deps, args []string) int {
	root := NewRootCommand(deps)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	stderr := deps.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
	var usageErr *UsageError
	if errors.As(err, &usageErr) && usageErr.Cmd != nil {
		_, _ = fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", usageErr.Cmd.CommandPath())
	}
	return ExitCode(err)
}

// loadConfig reads the configuration sources without contacting a server.
func (c *cli) loadConfig(cmd *cobra.Command) (*config.Loaded, error) {
	loaded, err := config.Load(cmd)
	if err != nil {
		return nil, &UsageError{Err: err}
	}
	return loaded, nil
}

// runtime resolves the active context and builds the API client once.
func (c *cli) runtime(cmd *cobra.Command) (*Runtime, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rt != nil {
		return c.rt, nil
	}
	loaded, err := c.loadConfig(cmd)
	if err != nil {
		return nil, err
	}
	resolved, err := config.Resolve(loaded)
	if err != nil {
		return nil, &UsageError{Err: err}
	}

	var source auth.TokenSource
	switch {
	case resolved.Token != "":
		source = auth.StaticToken(resolved.Token)
	case len(resolved.TokenCommand) > 0:
		source = auth.CommandToken{Argv: resolved.TokenCommand, Timeout: resolved.TokenCommandTimeout}
	}

	httpClient := c.deps.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	var verbose io.Writer
	if resolved.Verbose {
		verbose = c.deps.Stderr
	}
	c.rt = &Runtime{
		Config: resolved,
		Client: &client.Client{
			BaseURL:    resolved.Server,
			HTTP:       httpClient,
			Auth:       auth.New(source, resolved.PrincipalHeader, resolved.Principal),
			HostHeader: resolved.HostHeader,
			Headers:    resolved.Headers,
			Verbose:    verbose,
			UserAgent:  "aibctl/" + version.Version,
		},
		Printer: &output.Printer{
			Out:      c.deps.Stdout,
			Format:   output.Format(resolved.Output),
			ShowUUID: resolved.ShowUUID,
		},
	}
	return c.rt, nil
}

// exactArgs validates positional arguments and reports failures as usage errors.
func exactArgs(n int, names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return &UsageError{Err: fmt.Errorf("expected %d argument(s): %s", n, strings.Join(names, " ")), Cmd: cmd}
		}
		return nil
	}
}

// minArgs validates at least n positional arguments as a usage error.
func minArgs(n int, names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return &UsageError{Err: fmt.Errorf("expected at least %d argument(s): %s", n, strings.Join(names, " ")), Cmd: cmd}
		}
		return nil
	}
}
