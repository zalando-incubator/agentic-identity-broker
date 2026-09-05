package approval

import (
	"context"
	"fmt"
	"strings"
)

type Broker interface {
	Read(context.Context, string, []string) ([]Pair, string, error)
	Create(context.Context, string, CreateRequest) (string, error)
	Consume(context.Context, string, string) error
}

type Invocation struct {
	Identity       Identity
	ToolName       string
	Arguments      map[string]any
	AgentSessionID string
	MCPSessionID   string
	RequestID      string
	SubjectToken   string
	Description    string
	RiskLevel      string
}

type Outcome struct {
	Proceed bool
	URL     string
	Reason  string
}

type Gate struct {
	cache  *Cache
	broker Broker
}

func NewGate(cache *Cache, broker Broker) *Gate {
	return &Gate{cache: cache, broker: broker}
}

func (g *Gate) Evaluate(ctx context.Context, invocation Invocation) Outcome {
	if g == nil || g.cache == nil || g.broker == nil {
		return Outcome{Reason: "approval gate unavailable"}
	}
	if !invocation.Identity.Valid() {
		return Outcome{Reason: "approval identity unavailable"}
	}
	if strings.TrimSpace(invocation.SubjectToken) == "" {
		return Outcome{Reason: "approval subject token unavailable"}
	}

	g.cache.RecordSession(invocation.AgentSessionID)
	if record, matched := g.cache.Match(invocation.Identity, invocation.AgentSessionID, invocation.ToolName, invocation.Arguments); matched {
		return g.consumeOrProceed(ctx, invocation, record)
	}

	pairs, _, err := g.broker.Read(ctx, invocation.Identity.Principal, g.cache.ActiveSessions())
	if err != nil {
		return Outcome{Reason: "approval state could not be refreshed"}
	}
	g.cache.ReplaceForPrincipal(invocation.Identity.Principal, pairs)
	if record, matched := g.cache.Match(invocation.Identity, invocation.AgentSessionID, invocation.ToolName, invocation.Arguments); matched {
		return g.consumeOrProceed(ctx, invocation, record)
	}

	arguments := invocation.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}
	create := CreateRequest{ToolName: invocation.ToolName, Arguments: arguments, RiskLevel: invocation.RiskLevel}
	create.Metadata.MCPSessionID = invocation.MCPSessionID
	create.Metadata.AgentSessionID = invocation.AgentSessionID
	create.Metadata.InvocationID = invocation.RequestID
	create.Metadata.Description = invocation.Description
	if create.Metadata.Description == "" {
		create.Metadata.Description = fmt.Sprintf("Approval required for %s", invocation.ToolName)
	}
	approvalURL, err := g.broker.Create(ctx, invocation.SubjectToken, create)
	if err != nil || strings.TrimSpace(approvalURL) == "" {
		return Outcome{Reason: "approval could not be initiated"}
	}
	return Outcome{URL: approvalURL}
}

func (g *Gate) consumeOrProceed(ctx context.Context, invocation Invocation, record Record) Outcome {
	if record.Persistence == nil || *record.Persistence != "once" {
		return Outcome{Proceed: true}
	}
	if err := g.broker.Consume(ctx, invocation.SubjectToken, record.ID); err != nil {
		g.cache.Release(invocation.Identity, record.ID)
		return Outcome{Reason: "approval could not be consumed"}
	}
	g.cache.ConfirmConsumed(invocation.Identity, record.ID)
	return Outcome{Proceed: true}
}
