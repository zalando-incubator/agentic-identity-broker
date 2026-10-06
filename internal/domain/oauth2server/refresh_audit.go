package oauth2server

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ory/fosite"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func newRefreshMetrics() (metric.Int64Counter, metric.Int64Counter, error) {
	meter := otel.Meter("identity-broker.oauth2.refresh")
	retries, err := meter.Int64Counter("oauth2.refresh.retry.accepted", metric.WithDescription("Committed stored-result refresh authorizations"))
	if err != nil {
		return nil, nil, err
	}
	rejections, err := meter.Int64Counter("oauth2.refresh.rejected", metric.WithDescription("Rejected local refresh presentations"))
	return retries, rejections, err
}

func (p *Provider) auditRefresh(ctx context.Context, event, reason, clientID string, root *storage.RefreshSession) {
	boundClientID := clientID
	if root != nil {
		boundClientID = root.ClientID.String()
	}
	fields := []any{"event", event, "reason", reason, "client_id", boundClientID}
	if root != nil {
		fields = append(fields, "session_id", root.ID.String(), "principal", root.Principal.String(), "agent_id", root.AgentID.String())
		if boundClientID != clientID {
			fields = append(fields, "presented_client_id", clientID)
		}
		if root.OriginalRequestContextFingerprint != nil {
			fields = append(fields, "request_context_changed", *root.OriginalRequestContextFingerprint != refreshRequestFingerprint(ctx))
		}
	}
	audit := security.AuditContextFields(ctx)
	fields = append(fields, "origin", audit.Origin)
	if audit.ClientIP != "" {
		fields = append(fields, "client_ip", audit.ClientIP)
	}
	if audit.UserAgent != "" {
		fields = append(fields, "user_agent", audit.UserAgent)
	}
	switch event {
	case "RefreshRetryAccepted":
		p.retryAccepted.Add(ctx, 1)
	case "RefreshRejected":
		p.refreshRejected.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
	}
	p.logger.InfoContext(ctx, event, fields...)
}

func logRefreshTransition(logger *slog.Logger, ctx context.Context, root storage.RefreshSessionAuditIdentity, reason storage.RefreshRevocationReason) {
	event := "RefreshSessionRevoked"
	if reason == storage.RefreshReasonAbsoluteExpiry || reason == storage.RefreshReasonInactivityExpiry {
		event = "RefreshSessionExpired"
	}
	audit := security.AuditContextFields(ctx)
	fields := []any{"event", event, "reason", string(reason), "session_id", root.ID.String(),
		"principal", root.Principal.String(), "agent_id", root.AgentID.String(), "client_id", root.ClientID.String(), "origin", audit.Origin}
	if audit.ClientIP != "" {
		fields = append(fields, "client_ip", audit.ClientIP)
	}
	if audit.UserAgent != "" {
		fields = append(fields, "user_agent", audit.UserAgent)
	}
	logger.InfoContext(ctx, event, fields...)
}

func refreshFailureReason(err error, stage string) string {
	var oauthError *fosite.RFC6749Error
	if errors.As(err, &oauthError) && oauthError.ErrorField == fosite.ErrInvalidGrant.ErrorField {
		return stage
	}
	switch {
	case errors.Is(err, fosite.ErrInvalidClient):
		return "client_authentication"
	case errors.Is(err, fosite.ErrUnauthorizedClient):
		return "refresh_capability"
	case errors.Is(err, fosite.ErrInvalidScope), errors.Is(err, fosite.ErrScopeNotGranted):
		return "response_scope"
	case errors.Is(err, fosite.ErrServerError):
		return stage + "_unavailable"
	default:
		return stage
	}
}
