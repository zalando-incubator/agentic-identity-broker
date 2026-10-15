package aibctlcmd

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/output"
)

const (
	completionTimeoutEnv     = "AIBCTL_COMPLETION_TIMEOUT"
	defaultCompletionTimeout = 2 * time.Second
)

type completionItem struct {
	id, uuid, description string
}

type completionLister func(ctx context.Context, c *client.Client, args []string) ([]completionItem, error)

// completeResource queries the API for identifiers. Failures produce no
// completions and never write to stdout, so a broken context cannot corrupt
// the shell's completion output.
func (c *cli) completeResource(list completionLister) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		rt, err := c.runtime(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
		}
		ctx, cancel := context.WithTimeout(context.Background(), completionTimeout())
		defer cancel()
		items, err := list(ctx, rt.Client, args)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
		}
		var out []string
		for _, it := range items {
			if strings.HasPrefix(it.id, toComplete) {
				out = append(out, it.id+"\t"+it.description)
			} else if it.uuid != "" && it.uuid != it.id && strings.HasPrefix(it.uuid, toComplete) {
				out = append(out, it.uuid+"\t"+it.description)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func completionTimeout() time.Duration {
	if v := os.Getenv(completionTimeoutEnv); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultCompletionTimeout
}

func listAgentItems(ctx context.Context, c *client.Client, _ []string) ([]completionItem, error) {
	agents, err := c.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]completionItem, 0, len(agents))
	for _, a := range agents {
		items = append(items, completionItem{id: output.DisplayID(a.CanonicalID, a.ID), uuid: a.ID, description: a.DisplayName})
	}
	return items, nil
}

func listPermissionSetItems(ctx context.Context, c *client.Client, _ []string) ([]completionItem, error) {
	sets, err := c.ListPermissionSets(ctx, "")
	if err != nil {
		return nil, err
	}
	items := make([]completionItem, 0, len(sets))
	for _, ps := range sets {
		items = append(items, completionItem{id: output.DisplayID(ps.CanonicalID, ps.ID), uuid: ps.ID, description: ps.Name})
	}
	return items, nil
}

func listServiceItems(ctx context.Context, c *client.Client, _ []string) ([]completionItem, error) {
	services, err := c.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]completionItem, 0, len(services))
	for _, s := range services {
		items = append(items, completionItem{id: output.DisplayID(s.CanonicalID, s.ID), uuid: s.ID, description: s.DisplayName})
	}
	return items, nil
}
