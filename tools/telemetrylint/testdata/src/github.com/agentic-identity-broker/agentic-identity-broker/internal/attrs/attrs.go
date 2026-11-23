package attrs

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

const ownerKey attribute.Key = "approval.principal"

func keys(span trace.Span, dynamic string, p id.Principal, s model.Secret) {
	span.SetAttributes(attribute.String("approval.id", "a1"), attribute.Int("approval.count", 1))
	span.SetAttributes(attribute.Key("approval.status").String("pending"))
	span.SetAttributes(attribute.String(dynamic, "a1"))                        // want `attribute-key: the attribute key must be a constant string`
	span.SetAttributes(attribute.Key(dynamic).String("a1"))                    // want `attribute-key: the attribute key must be a constant string`
	span.SetAttributes(attribute.String("approval.principal", string(p)))      // want `principal-key: key "approval.principal" is reserved for principals`
	span.SetAttributes(attribute.Key("approval.principal").String(p.String())) // want `principal-key: key "approval.principal" is reserved for principals`
	span.SetAttributes(ownerKey.String("someone"))                             // want `principal-key: key "approval.principal" is reserved for principals`
	span.SetAttributes(p.SpanAttribute("approval.principal"))
	span.SetAttributes(p.SpanAttribute("approval.owner"))                       // want `principal-key: the SpanAttribute key must be a constant that ends in .principal`
	span.SetAttributes(p.SpanAttribute(dynamic))                                // want `principal-key: the SpanAttribute key must be a constant that ends in .principal`
	span.SetAttributes(attribute.String("oauth2.client_secret", s.Plaintext())) // want `credential: a Secret value must not be a log or span attribute value`
}

func errs(p id.Principal, agentID string) error {
	if agentID == "" {
		return fmt.Errorf("grant for agent %s not found", agentID)
	}
	return fmt.Errorf("grant for %s not found", p) // want `principal-sink: a principal must not be in an error string`
}

func logs(ctx context.Context, logger *slog.Logger, s model.Secret) {
	logger.InfoContext(ctx, "client registered", "client_id", "c1")
	logger.InfoContext(ctx, "client registered", "client_secret", s)                          // want `credential: a Secret value must not be a log or span attribute value`
	logger.InfoContext(ctx, "client registered", slog.String("client_secret", s.Plaintext())) // want `credential: a Secret value must not be a log or span attribute value`
	logger.LogAttrs(ctx, slog.LevelInfo, "client registered", slog.Any("client_secret", &s))  // want `credential: a Secret value must not be a log or span attribute value`
}
