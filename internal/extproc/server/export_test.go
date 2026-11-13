package server

import "time"

// IsServerError exposes isServerError for package-external unit tests.
var IsServerError = isServerError

// IsTransientBrokerError exposes isTransientBrokerError for package-external unit tests.
var IsTransientBrokerError = isTransientBrokerError

// ExpireAssertion injects an already-expired assertion state into te for tests.
func (te *TokenExchanger) ExpireAssertion() {
	te.assertion.Store(&assertionState{
		value:     "expired-assertion-value",
		issuedAt:  time.Now().Add(-2 * time.Hour),
		expiresAt: time.Now().Add(-time.Minute),
	})
}

// TripCircuitBreaker forces n consecutive circuit-breaker failures by calling Execute
// with a synthetic 5xx error. After MaxFailures failures the circuit opens.
func (te *TokenExchanger) TripCircuitBreaker(n int) {
	if te.cb == nil {
		return
	}
	for range n {
		_, _ = te.cb.Execute(func() (ExchangeResult, error) {
			return ExchangeResult{}, &BrokerExchangeError{StatusCode: 500, Code: "server_error"}
		})
	}
}

// SetClock replaces the server clock used for delegation deadline checks.
func (s *Server) SetClock(now func() time.Time) { s.now = now }

// ContextInputFromDelegation exposes the enabled-mode policy-context mapping.
var ContextInputFromDelegation = contextInputFromDelegation
