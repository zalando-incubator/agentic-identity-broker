package ledger

import (
	"context"
	"net/netip"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"go.opentelemetry.io/otel/trace"
)

type requestSpanKey struct{}

type workflowActorKey struct{}

// WithWorkflowActor carries actor facts established by a domain workflow.
func WithWorkflowActor(ctx context.Context, actor model.BusinessEventActor) context.Context {
	return context.WithValue(ctx, workflowActorKey{}, actor)
}

func WorkflowActorFromContext(ctx context.Context) (model.BusinessEventActor, bool) {
	actor, ok := ctx.Value(workflowActorKey{}).(model.BusinessEventActor)
	return actor, ok
}

func CaptureRequestSpan(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestSpanKey{}, trace.SpanContextFromContext(ctx))
}

// ContextFacts enriches domain-established facts with trusted request context.
func ContextFacts(ctx context.Context, facts model.BusinessEvent) model.BusinessEvent {
	facts.TraceID, facts.SpanID, facts.Client = "", "", nil
	if facts.Actor.ID != nil && *facts.Actor.ID == security.AnonymousActor {
		facts.Actor.ID = nil
	}
	resolved, authenticated := security.FromContext(ctx)
	if authenticated {
		switch facts.Actor.Kind {
		case "agent", "gateway":
			if resolved.CallingPeer != "" {
				facts.Actor.ID = &resolved.CallingPeer
			}
		case "user", "admin":
			if resolved.Actor != "" && resolved.Actor != security.AnonymousActor {
				facts.Actor.ID = &resolved.Actor
			}
		}
	} else if facts.Actor.Kind == "user" || facts.Actor.Kind == "admin" {
		if actor, ok := principal.FromContext(ctx); ok && actor != "" {
			facts.Actor.ID = &actor
		}
	}
	var captured security.TransportCapture
	if authenticated {
		captured = security.TransportCapture{TraceID: resolved.TraceID, ClientIP: resolved.ClientIP, UserAgent: resolved.UserAgent}
	} else if holder, ok := security.CaptureHolderFromContext(ctx); ok {
		captured = holder.Capture()
	}
	if validHexID(captured.TraceID, 32) {
		facts.TraceID = captured.TraceID
		span := trace.SpanContextFromContext(ctx)
		if capturedSpan, ok := ctx.Value(requestSpanKey{}).(trace.SpanContext); ok {
			span = capturedSpan
		}
		if span.IsValid() && span.TraceID().String() == captured.TraceID {
			facts.SpanID = span.SpanID().String()
		}
	}
	client := model.BusinessEventClient{UserAgent: userAgentFamily(captured.UserAgent)}
	if address, err := netip.ParseAddr(captured.ClientIP); err == nil && address.Zone() == "" {
		client.IP = address.Unmap().String()
	}
	if client.IP != "" || client.UserAgent != "" {
		facts.Client = &client
	}
	return facts
}

func userAgentFamily(raw string) string {
	switch {
	case strings.Contains(raw, "Edg/") || strings.Contains(raw, "Edge/"):
		return "Edge"
	case strings.Contains(raw, "Chrome/") || strings.Contains(raw, "Chromium/"):
		return "Chrome"
	case strings.Contains(raw, "Firefox/"):
		return "Firefox"
	case strings.Contains(raw, "Safari/") && strings.Contains(raw, "Version/"):
		return "Safari"
	case strings.HasPrefix(raw, "curl/"):
		return "curl"
	case strings.HasPrefix(raw, "Go-http-client/"):
		return "Go-http-client"
	default:
		return ""
	}
}
