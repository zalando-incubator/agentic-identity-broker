// Package server — circuit_breaker.go wraps github.com/sony/gobreaker/v2 to
// protect outbound HTTP calls to the identity broker. It prevents a
// thundering herd when the broker recovers after an outage by fast-failing
// requests while the circuit is open.
//
// Only server-side (5xx) errors trip the circuit. Client errors (4xx) are
// excluded because they indicate a problem with the request itself — not
// with the backend — and should not contribute to opening the circuit.
package server

import (
	"errors"
	"log/slog"

	"github.com/sony/gobreaker/v2"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// ErrCircuitOpen is returned by Exchange when the circuit breaker is open.
// Callers should surface this as a 503 Service Unavailable to signal a transient
// infrastructure failure rather than a client error.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// isServerError reports whether err represents a server-side (5xx) failure
// that should count towards circuit breaker tripping. Client errors (4xx
// BrokerExchangeError) are excluded because they indicate a problem with
// the specific request, not with the backend infrastructure.
func isServerError(err error) bool {
	if err == nil {
		return false
	}
	var brokerErr *BrokerExchangeError
	if errors.As(err, &brokerErr) {
		return brokerErr.StatusCode >= 500
	}
	return true
}

// newGobreakerCB creates a gobreaker CircuitBreaker configured from cfg.
// The returned breaker uses consecutive-failure counting: it opens after
// cfg.CircuitBreaker.MaxFailures consecutive failures and allows a single
// probe after cfg.CircuitBreaker.ResetTimeout.
//
// 4xx BrokerExchangeErrors are excluded from failure counting via IsSuccessful
// because they represent client errors (e.g., invalid subject tokens), not
// infrastructure failures.
func newGobreakerCB(cfg *extprocconfig.Config, logger *slog.Logger) *gobreaker.CircuitBreaker[ExchangeResult] {
	maxFail := uint32(cfg.CircuitBreaker.MaxFailures) // #nosec G115 -- Validate rejects values outside the uint32 range.
	timeout := cfg.CircuitBreaker.ResetTimeout

	st := gobreaker.Settings{
		Name:        "token-exchange",
		MaxRequests: 1, // only one probe allowed in half-open state
		Timeout:     timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= maxFail
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			// 4xx client errors are not failures from the circuit breaker's perspective.
			return !isServerError(err)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			if to == gobreaker.StateOpen {
				logger.Warn("circuit breaker opened",
					"circuit", name,
					"from", from.String(),
					"to", to.String())
				return
			}
			logger.Info("circuit breaker state changed",
				"circuit", name,
				"from", from.String(),
				"to", to.String())
		},
	}

	return gobreaker.NewCircuitBreaker[ExchangeResult](st)
}
