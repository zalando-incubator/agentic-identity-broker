package oauth2server

import (
	"context"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// SessionCleanup removes expired records retained by the local OAuth2 issuer.
type SessionCleanup struct {
	repositories [3]sessionCleanupRepository
	logger       *slog.Logger
}

type sessionCleanupRepository struct {
	name          string
	deleteExpired func(context.Context) (int, error)
}

func NewSessionCleanup(
	codes ports.AuthorizationCodeRepository,
	pkce ports.PKCESessionRepository,
	refreshTokens ports.RefreshTokenSessionRepository,
	logger *slog.Logger,
) *SessionCleanup {
	return &SessionCleanup{
		repositories: [3]sessionCleanupRepository{
			{name: "authorization_codes", deleteExpired: codes.DeleteExpired},
			{name: "pkce_sessions", deleteExpired: pkce.DeleteExpired},
			{name: "refresh_token_sessions", deleteExpired: refreshTokens.DeleteExpired},
		},
		logger: logger,
	}
}

// Run sweeps immediately and every minute until ctx is canceled. It returns only
// after the current repository operation has finished.
func (s *SessionCleanup) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		s.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *SessionCleanup) sweep(ctx context.Context) {
	for _, repository := range s.repositories {
		if ctx.Err() != nil {
			return
		}
		if _, err := repository.deleteExpired(ctx); err != nil && ctx.Err() == nil {
			// Repository errors can contain credentials; log only the affected store.
			s.logger.WarnContext(ctx, "expired OAuth2 record cleanup failed", "repository", repository.name)
		}
	}
}
