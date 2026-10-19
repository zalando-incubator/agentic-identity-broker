package oauth2session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SessionHealthState classifies a stored session for the oauth2_sessions gauge.
type SessionHealthState string

const (
	SessionHealthValid                SessionHealthState = "valid"
	SessionHealthExpiringRefreshable  SessionHealthState = "expiring_refreshable"
	SessionHealthExpiredUnrefreshable SessionHealthState = "expired_unrefreshable"
	SessionHealthUnknownExpiry        SessionHealthState = "unknown_expiry"
)

var sessionHealthStates = []SessionHealthState{
	SessionHealthValid, SessionHealthExpiringRefreshable, SessionHealthExpiredUnrefreshable, SessionHealthUnknownExpiry,
}

// ClassifySessionHealth maps a session group to its health state. A session without a recorded
// access-token expiry is unknown_expiry; one expiring after the refresh lookahead is valid; a due
// session is expiring_refreshable only while its refresh token is usable.
func ClassifySessionHealth(group storage.SessionExpiryGroup) (SessionHealthState, error) {
	switch group.AccessExpiry {
	case storage.AccessExpiryUnknown:
		return SessionHealthUnknownExpiry, nil
	case storage.AccessExpiryAfterThreshold:
		return SessionHealthValid, nil
	case storage.AccessExpiryDue:
		if group.RefreshUsable {
			return SessionHealthExpiringRefreshable, nil
		}
		return SessionHealthExpiredUnrefreshable, nil
	}
	return "", fmt.Errorf("unknown access expiry class %q", group.AccessExpiry)
}

// SessionHealthSnapshot holds per-service session counts for every health state.
type SessionHealthSnapshot struct {
	Counts map[id.ServiceID]map[SessionHealthState]int
}

// SessionHealthMetrics publishes the oauth2_sessions gauge. Each metric collection runs one
// grouped count query; a failed query publishes nothing for that collection.
type SessionHealthMetrics struct {
	repo         ports.UserSessionHealthRepository
	lookahead    time.Duration
	queryTimeout time.Duration
	logger       *slog.Logger
	gauge        metric.Int64ObservableGauge
	registration metric.Registration
	closeOnce    sync.Once
	closeErr     error
}

// NewSessionHealthMetrics registers the oauth2_sessions gauge on the global meter provider.
func NewSessionHealthMetrics(repo ports.UserSessionHealthRepository, lookahead, queryTimeout time.Duration, logger *slog.Logger) (*SessionHealthMetrics, error) {
	meter := otel.Meter("oauth2session")
	gauge, err := meter.Int64ObservableGauge("oauth2_sessions",
		metric.WithDescription("Stored third-party sessions by service and health state"), metric.WithUnit("{session}"))
	if err != nil {
		return nil, fmt.Errorf("create oauth2_sessions gauge: %w", err)
	}
	m := &SessionHealthMetrics{repo: repo, lookahead: lookahead, queryTimeout: queryTimeout, logger: logger, gauge: gauge}
	m.registration, err = meter.RegisterCallback(m.observe, gauge)
	if err != nil {
		return nil, fmt.Errorf("register oauth2_sessions callback: %w", err)
	}
	return m, nil
}

func (m *SessionHealthMetrics) snapshot(ctx context.Context) (SessionHealthSnapshot, error) {
	queryCtx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()
	now := time.Now()
	groups, err := m.repo.CountSessionsByExpiry(queryCtx, now.Add(m.lookahead), now)
	if err != nil {
		return SessionHealthSnapshot{}, err
	}
	snapshot := SessionHealthSnapshot{Counts: make(map[id.ServiceID]map[SessionHealthState]int)}
	for _, group := range groups {
		if group.Count <= 0 {
			return SessionHealthSnapshot{}, fmt.Errorf("session expiry group count %d must be positive", group.Count)
		}
		state, err := ClassifySessionHealth(group)
		if err != nil {
			return SessionHealthSnapshot{}, err
		}
		states := snapshot.Counts[group.ServiceID]
		if states == nil {
			states = make(map[SessionHealthState]int, len(sessionHealthStates))
			snapshot.Counts[group.ServiceID] = states
		}
		states[state] += group.Count
	}
	return snapshot, nil
}

func (m *SessionHealthMetrics) observe(ctx context.Context, observer metric.Observer) error {
	snapshot, err := m.snapshot(ctx)
	if err != nil {
		kind := storage.ErrorKindUnknown
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) {
			kind = storageErr.Kind
		}
		m.logger.WarnContext(ctx, "session health query failed", "error_kind", string(kind))
		return nil
	}
	for serviceID, states := range snapshot.Counts {
		service := attribute.String("service_id", serviceID.String())
		for _, state := range sessionHealthStates {
			observer.ObserveInt64(m.gauge, int64(states[state]),
				metric.WithAttributeSet(attribute.NewSet(service, attribute.String("state", string(state)))))
		}
	}
	return nil
}

// Close unregisters the gauge callback. It is safe to call more than once.
func (m *SessionHealthMetrics) Close() error {
	m.closeOnce.Do(func() { m.closeErr = m.registration.Unregister() })
	return m.closeErr
}
