package oauth2session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var ErrInvalidSweepRequest = errors.New("invalid session sweep request")

var sweepMetricAttributes = metric.WithAttributeSet(attribute.NewSet(attribute.String("triggered_by", string(RefreshTriggerSweep))))

type SweepRequest struct {
	// Lookahead zero means use token_refresh.lookahead_duration; after defaulting, it must be > 0.
	Lookahead time.Duration
	// DryRun zero means a real sweep.
	DryRun bool
	// PageSize zero means use token_refresh.sweep.default_page_size; after defaulting, 1 <= n <= 1000.
	PageSize int
}

// SweepResult classifies every evaluated candidate exactly once:
// Refreshed + Skipped + Failed == TotalEvaluated.
type SweepResult struct {
	Refreshed          int
	Skipped            int
	Failed             int
	TotalEvaluated     int
	DryRun             bool
	EffectiveLookahead time.Duration
	EffectivePageSize  int
}

type SessionSweepService struct {
	expiry           ports.UserSessionExpiryRepository
	sessions         *OAuth2SessionService
	logger           *slog.Logger
	refreshedCounter metric.Int64Counter
	failedCounter    metric.Int64Counter
}

func NewSessionSweepService(expiry ports.UserSessionExpiryRepository, sessions *OAuth2SessionService, logger *slog.Logger) *SessionSweepService {
	meter := otel.Meter("oauth2session")
	refreshed, _ := meter.Int64Counter("session_sweep_refreshed_total",
		metric.WithDescription("Sessions refreshed and committed by an admin sweep"), metric.WithUnit("{refresh}"))
	failed, _ := meter.Int64Counter("session_sweep_failed_total",
		metric.WithDescription("Admin sweep candidates that could not be refreshed"), metric.WithUnit("{refresh}"))
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "oauth2session")
	return &SessionSweepService{
		expiry: expiry, sessions: sessions, logger: logger,
		refreshedCounter: refreshed, failedCounter: failed,
	}
}

// Sweep evaluates due sessions in expiry/ID order. A fixed threshold and the
// locked refresh re-check ensure that concurrent refreshes are counted as skipped.
func (s *SessionSweepService) Sweep(ctx context.Context, req SweepRequest) (SweepResult, error) {
	result := SweepResult{DryRun: req.DryRun}
	if req.Lookahead == 0 {
		req.Lookahead = s.sessions.config.RefreshLookahead
	}
	if req.PageSize == 0 {
		req.PageSize = s.sessions.config.SweepDefaultPageSize
	}
	result.EffectiveLookahead = req.Lookahead
	result.EffectivePageSize = req.PageSize
	if req.Lookahead <= 0 {
		return result, fmt.Errorf("%w: lookahead_duration must be greater than zero", ErrInvalidSweepRequest)
	}
	if req.PageSize < 1 || req.PageSize > 1000 {
		return result, fmt.Errorf("%w: page_size must be between 1 and 1000", ErrInvalidSweepRequest)
	}

	threshold := time.Now().Add(req.Lookahead)
	var cursor storage.SessionExpiryCursor
	seen := make(map[id.SessionID]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		page, err := s.expiry.ListExpiringSessions(ctx, threshold, cursor, req.PageSize)
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			failure := sessionOperationError(ctx, OperationSessionLookup, DetailRepositoryUnavailable, err)
			s.logAbort(ctx, result, failure)
			return result, failure
		}
		nextCursor, err := nextSweepCursor(page, cursor)
		if err != nil {
			failure := sessionOperationError(ctx, OperationSessionLookup, DetailRepositoryUnavailable, err)
			s.logAbort(ctx, result, failure)
			return result, failure
		}
		cursor = nextCursor
		for _, candidate := range page {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if _, exists := seen[candidate.ID]; exists {
				continue
			}
			seen[candidate.ID] = struct{}{}
			if req.DryRun {
				switch {
				case !candidate.AccessTokenExpiresBy(threshold):
					result.Skipped++
				case !candidate.CanRefresh():
					result.Failed++
				default:
					result.Refreshed++
				}
			} else {
				refreshCtx, cancel := s.sessions.refreshOperationContext(context.WithoutCancel(ctx))
				var refreshed bool
				var provider *model.ThirdpartyOAuth2ProviderEntity
				var refreshErr error
				if !candidate.CanRefresh() {
					var needsRefresh bool
					needsRefresh, refreshErr = s.sessions.checkUnrefreshableSweepCandidate(refreshCtx, candidate.Principal, candidate.ServiceID, threshold)
					if refreshErr == nil && needsRefresh {
						_, refreshed, provider, refreshErr = s.sessions.refreshDueSession(refreshCtx, candidate.Principal, candidate.ServiceID, threshold, RefreshTriggerSweep)
					}
				} else {
					_, refreshed, provider, refreshErr = s.sessions.refreshDueSession(refreshCtx, candidate.Principal, candidate.ServiceID, threshold, RefreshTriggerSweep)
				}
				cancel()
				if refreshErr != nil {
					metadata := sessionFailureMetadata(refreshErr)
					switch {
					case metadata.Detail() == DetailSessionMissing:
						result.Skipped++
						s.logger.DebugContext(ctx, "session sweep candidate no longer exists", "session_id", candidate.ID, "service_id", candidate.ServiceID, "triggered_by", RefreshTriggerSweep)
					case metadata.Kind() == KindCanceled:
						return result, refreshErr
					case metadata.Kind() == KindInfrastructure && (metadata.Dependency() == DependencySessionRepository || metadata.Dependency() == DependencyProviderRepository):
						s.logAbort(ctx, result, refreshErr)
						return result, refreshErr
					default:
						result.Failed++
						s.failedCounter.Add(ctx, 1, sweepMetricAttributes)
						level := slog.LevelWarn
						if metadata.Kind() == KindInternal || (metadata.Kind() == KindInfrastructure && metadata.Dependency() != DependencyProvider) {
							level = slog.LevelError
						}
						attrs := []slog.Attr{
							slog.String("event", "session.oauth2.refresh_failed"),
							slog.String("session_id", candidate.ID.String()),
							slog.String("service_id", candidate.ServiceID.String()),
							slog.String("triggered_by", string(RefreshTriggerSweep)),
							slog.Any("oauth2_session", metadata),
						}
						if client := clientInfoFromProvider(provider); client.known {
							attrs = append(attrs, slog.Bool("public_client", client.public))
						}
						s.logger.LogAttrs(ctx, level, "session sweep refresh failed", attrs...)
					}
				} else if refreshed {
					result.Refreshed++
					s.refreshedCounter.Add(ctx, 1, sweepMetricAttributes)
				} else {
					result.Skipped++
				}
			}
			result.TotalEvaluated++
		}
		if len(page) < req.PageSize {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			return result, nil
		}
	}
}

func nextSweepCursor(page []*storage.UserSession, previous storage.SessionExpiryCursor) (storage.SessionExpiryCursor, error) {
	for _, candidate := range page {
		if candidate == nil || candidate.AccessTokenExpiresAt == nil || candidate.AccessTokenExpiresAt.IsZero() || candidate.ID.IsZero() {
			return previous, errors.New("session expiry repository returned an invalid cursor tuple")
		}
		next := storage.SessionExpiryCursor{AccessTokenExpiresAt: *candidate.AccessTokenExpiresAt, ID: candidate.ID}
		if !previous.ID.IsZero() {
			order := next.AccessTokenExpiresAt.Compare(previous.AccessTokenExpiresAt)
			if order < 0 || (order == 0 && bytes.Compare(next.ID[:], previous.ID[:]) <= 0) {
				return previous, errors.New("session expiry repository returned a non-increasing cursor tuple")
			}
		}
		previous = next
	}
	return previous, nil
}

func (s *SessionSweepService) logAbort(ctx context.Context, result SweepResult, err error) {
	s.logger.ErrorContext(ctx, "session sweep aborted", "event", "session.oauth2.sweep_aborted", "triggered_by", RefreshTriggerSweep,
		"oauth2_session", sessionFailureMetadata(err), "refreshed", result.Refreshed, "skipped", result.Skipped,
		"failed", result.Failed, "total_evaluated", result.TotalEvaluated)
}
