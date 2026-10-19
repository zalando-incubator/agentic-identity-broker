package aibctlcmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

func (c *cli) newClientCredentialsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "client-credentials",
		Aliases: []string{"creds"},
		Short:   "Manage the broker-issued client credentials of an agent",
	}
	completeAgents := c.completeResource(listAgentItems)

	get := &cobra.Command{
		Use:               "get AGENT",
		Short:             "Show credential metadata (never the secret)",
		Args:              exactArgs(1, "AGENT"),
		ValidArgsFunction: completeAgents,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			info, err := rt.Client.GetClientCredentials(ctx, args[0])
			if err != nil {
				return err
			}
			return rt.Printer.PrintObject(info, func() [][2]string {
				return [][2]string{
					{"Client ID", info.ClientID},
					{"Created", info.CreatedAt},
					{"Rotated", orDash(output.Deref(info.RotatedAt))},
				}
			})
		},
	}

	create := &cobra.Command{
		Use:               "create AGENT",
		Short:             "Generate client credentials for an agent that has none",
		Args:              exactArgs(1, "AGENT"),
		ValidArgsFunction: completeAgents,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			exists, err := clientCredentialsExist(cmd, rt, args[0])
			if err != nil {
				return err
			}
			if exists {
				return &PreconditionError{Err: fmt.Errorf("agent %q already has client credentials; replace them with \"aibctl agent client-credentials rotate %s\"", args[0], args[0])}
			}
			return c.generateClientCredentials(cmd, rt, args[0])
		},
	}

	rotate := &cobra.Command{
		Use:               "rotate AGENT",
		Short:             "Replace an agent's client credentials; the current secret stops working immediately",
		Args:              exactArgs(1, "AGENT"),
		ValidArgsFunction: completeAgents,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			exists, err := clientCredentialsExist(cmd, rt, args[0])
			if err != nil {
				return err
			}
			if !exists {
				return &client.APIError{Status: http.StatusNotFound, Code: "not_found", Message: fmt.Sprintf("agent %q has no client credentials yet; use \"aibctl agent client-credentials create %s\"", args[0], args[0])}
			}
			if err := c.confirm(cmd, fmt.Sprintf("Rotate the client credentials of agent %q and invalidate the current secret", args[0])); err != nil {
				return err
			}
			return c.generateClientCredentials(cmd, rt, args[0])
		},
	}
	addYesFlag(rotate)

	del := &cobra.Command{
		Use:               "delete AGENT",
		Aliases:           []string{"revoke", "rm"},
		Short:             "Revoke the agent's client credentials",
		Args:              exactArgs(1, "AGENT"),
		ValidArgsFunction: completeAgents,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.confirm(cmd, fmt.Sprintf("Revoke client credentials of agent %q", args[0])); err != nil {
				return err
			}
			rt, err := c.runtime(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context(cmd)
			defer cancel()
			if err := rt.Client.RevokeClientCredentials(ctx, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.deps.Stderr, "Client credentials of %q revoked.\n", args[0])
			return nil
		},
	}
	addYesFlag(del)

	cmd.AddCommand(
		get,
		create,
		rotate,
		del,
	)
	return cmd
}

// clientCredentialsExist reports whether the agent has broker-issued credentials.
func clientCredentialsExist(cmd *cobra.Command, rt *Runtime, agent string) (bool, error) {
	ctx, cancel := rt.Context(cmd)
	defer cancel()
	_, err := rt.Client.GetClientCredentials(ctx, agent)
	switch {
	case err == nil:
		return true, nil
	case client.IsStatus(err, http.StatusNotFound):
		return false, nil
	default:
		return false, err
	}
}

func (c *cli) generateClientCredentials(cmd *cobra.Command, rt *Runtime, agent string) error {
	ctx, cancel := rt.Context(cmd)
	defer cancel()
	creds, rotated, err := rt.Client.GenerateClientCredentials(ctx, agent)
	if err != nil {
		return err
	}
	if rotated {
		_, _ = fmt.Fprintf(c.deps.Stderr, "Client credentials for %q rotated; the previous secret is invalid.\n", agent)
	} else {
		_, _ = fmt.Fprintf(c.deps.Stderr, "Client credentials for %q created.\n", agent)
	}
	_, _ = fmt.Fprintln(c.deps.Stderr, "The client secret is shown once and cannot be retrieved later.")
	return rt.Printer.PrintObject(creds, func() [][2]string {
		return [][2]string{
			{"Client ID", creds.ClientID},
			{"Client Secret", creds.ClientSecret},
			{"Created", creds.CreatedAt},
			{"Previous Invalidated", orDash(output.Deref(creds.PreviousInvalidatedAt))},
		}
	})
}
