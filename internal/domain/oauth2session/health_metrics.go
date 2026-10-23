package oauth2session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SessionHealthExpiry classifies a stored session's access-token expiry for the oauth2_sessions gauge.
type SessionHealthExpiry string

const (
	// SessionHealthExpiryValid means the access token expires after now plus the refresh lookahead.
	SessionHealthExpiryValid SessionHealthExpiry = "valid"
	// SessionHealthExpiryExpiring means the access token expires within the refresh lookahead.
	SessionHealthExpiryExpiring SessionHealthExpiry = "expiring"
	// SessionHealthExpiryExpired means the access token has already expired.
	SessionHealthExpiryExpired SessionHealthExpiry = "expired"
	// SessionHealthExpiryUnknown means no access-token expiry is recorded.
	SessionHealthExpiryUnknown SessionHealthExpiry = "unknown"
)

var sessionHealthExpiries = []SessionHealthExpiry{
	SessionHealthExpiryValid, SessionHealthExpiryExpiring, SessionHealthExpiryExpired, SessionHealthExpiryUnknown,
}

// SessionHealthKey identifies one oauth2_sessions series of a service: the access-token expiry
// class and whether the refresh token is present and not expired.
type SessionHealthKey struct {
	Expiry      SessionHealthExpiry
	Refreshable bool
}

// ClassifySessionHealth maps a session group to its gauge series.
func ClassifySessionHealth(group storage.SessionExpiryGroup) (SessionHealthKey, error) {
	key := SessionHealthKey{Refreshable: group.RefreshUsable}
	switch group.AccessExpiry {
	case storage.AccessExpiryUnknown:
		key.Expiry = SessionHealthExpiryUnknown
	case storage.AccessExpiryAfterThreshold:
		key.Expiry = SessionHealthExpiryValid
	case storage.AccessExpiryDue:
		key.Expiry = SessionHealthExpiryExpiring
	case storage.AccessExpiryExpired:
		key.Expiry = SessionHealthExpiryExpired
	default:
		return SessionHealthKey{}, fmt.Errorf("unknown access expiry class %q", group.AccessExpiry)
	}
	return key, nil
}

// SessionHealthSnapshot holds per-service session counts for every gauge series.
type SessionHealthSnapshot struct {
	Counts map[id.ServiceID]map[SessionHealthKey]int
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
		metric.WithDescription("Stored third-party sessions by service, access-token expiry class and refreshability"), metric.WithUnit("{session}"))
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
	snapshot := SessionHealthSnapshot{Counts: make(map[id.ServiceID]map[SessionHealthKey]int)}
	for _, group := range groups {
		if group.Count <= 0 {
			return SessionHealthSnapshot{}, fmt.Errorf("session expiry group count %d must be positive", group.Count)
		}
		key, err := ClassifySessionHealth(group)
		if err != nil {
			return SessionHealthSnapshot{}, err
		}
		series := snapshot.Counts[group.ServiceID]
		if series == nil {
			series = make(map[SessionHealthKey]int, 2*len(sessionHealthExpiries))
			snapshot.Counts[group.ServiceID] = series
		}
		series[key] += group.Count
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
	for serviceID, series := range snapshot.Counts {
		service := attribute.String("service_id", serviceID.String())
		for _, expiry := range sessionHealthExpiries {
			for _, refreshable := range []bool{true, false} {
				key := SessionHealthKey{Expiry: expiry, Refreshable: refreshable}
				observer.ObserveInt64(m.gauge, int64(series[key]), metric.WithAttributeSet(attribute.NewSet(service,
					attribute.String("expiry", string(expiry)), attribute.String("refreshable", strconv.FormatBool(refreshable)))))
			}
		}
	}
	return nil
}

// Close unregisters the gauge callback. It is safe to call more than once.
func (m *SessionHealthMetrics) Close() error {
	m.closeOnce.Do(func() { m.closeErr = m.registration.Unregister() })
	return m.closeErr
}
