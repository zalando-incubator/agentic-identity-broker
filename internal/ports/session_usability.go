package ports

import (
	"context"
	"errors"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type UsabilityMode int

const (
	UsabilityGate UsabilityMode = iota
	UsabilityDisplay
)

// SessionUsabilityEvaluator combines refreshability with current client identity evidence.
// Gate reads fail closed; display reads may use persisted evidence on source failure.
type SessionUsabilityEvaluator interface {
	RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode UsabilityMode) (map[id.ServiceID]bool, error)
}

// SessionUsabilityError identifies the service whose Gate evaluation failed.
// Its cause remains inspectable, but only bounded metadata is safe to log.
type SessionUsabilityError struct {
	ServiceID id.ServiceID
	Err       error
}

func (e *SessionUsabilityError) Error() string { return "session usability evaluation failed" }
func (e *SessionUsabilityError) Unwrap() error { return e.Err }

func (e *SessionUsabilityError) ReasonAndLevel() (model.CredentialSourceReason, slog.Level) {
	reason := model.CredentialSourceReasonReadFailed
	var source *model.CredentialSourceError
	if errors.As(e.Err, &source) {
		reason = model.NewCredentialSourceError(source.Reason).Reason
	}
	if reason.IsIdentity() {
		return reason, slog.LevelWarn
	}
	return reason, slog.LevelError
}

func (e *SessionUsabilityError) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("operation", "session_lookup"),
		slog.String("failure_detail", "credential_source_unavailable"),
		slog.String("error_kind", "infrastructure"),
		slog.String("dependency", "credential_source"),
		slog.Int("http_status_code", 0),
		slog.String("oauth_error_code", ""),
	)
}
