package approval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
	cache           *Cache
	broker          Broker
	decisionCounter metric.Int64Counter
}

func NewGate(cache *Cache, broker Broker) *Gate {
	decisionCounter, err := otel.GetMeterProvider().Meter("extproc").Int64Counter("extproc.approval.gate.decisions")
	if err != nil {
		slog.Warn("failed to create approval gate decision counter instrument", "error", err)
	}
	return &Gate{cache: cache, broker: broker, decisionCounter: decisionCounter}
}

func (g *Gate) Evaluate(ctx context.Context, invocation Invocation) (outcome Outcome) {
	ctx, span := otel.Tracer("extproc").Start(ctx, "extproc.approval.gate")
	defer func() {
		approvalOutcome := "deny"
		if outcome.Proceed {
			approvalOutcome = "proceed"
		} else if outcome.URL != "" {
			approvalOutcome = "elicit"
		}

		span.SetAttributes(attribute.String("approval.outcome", approvalOutcome))
		span.End()
		if g != nil && g.decisionCounter != nil {
			g.decisionCounter.Add(context.WithoutCancel(ctx), 1, metric.WithAttributes(attribute.String("outcome", approvalOutcome)))
		}
	}()

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

	pairs, etag, err := g.broker.Read(ctx, invocation.Identity.Principal, g.cache.ActiveSessions())
	if err != nil {
		if isRateLimited(err) {
			return Outcome{Reason: "approval state could not be refreshed — broker rate limit; retry shortly"}
		}
		return Outcome{Reason: "approval state could not be refreshed"}
	}
	if !g.cache.ReplaceForPrincipal(invocation.Identity.Principal, pairs, etag) {
		return Outcome{Reason: "approval state could not be refreshed"}
	}
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
		if isRateLimited(err) {
			return Outcome{Reason: "approval could not be initiated — broker rate limit; retry shortly"}
		}
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

func isRateLimited(err error) bool {
	var statusErr *StatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusTooManyRequests
}
