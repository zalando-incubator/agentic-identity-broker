package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// MockAuthZENBroker is a counting AuthZEN PDP for ExtProc startup scenarios. It records
// discovery and evaluation requests and every request that carried credentials.
type MockAuthZENBroker struct {
	server *httptest.Server

	// AdvertisedPDP overrides the advertised policy_decision_point; empty advertises the
	// mock's own URL.
	AdvertisedPDP string
	// DiscoveryStatus is the discovery response status; zero means 200.
	DiscoveryStatus int
	// DiscoveryBody overrides the discovery body when non-empty.
	DiscoveryBody string

	discoveries     atomic.Int32
	evaluations     atomic.Int32
	credentialCalls atomic.Int32
}

func NewMockAuthZENBroker() *MockAuthZENBroker {
	m := &MockAuthZENBroker{}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *MockAuthZENBroker) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "" {
		m.credentialCalls.Add(1)
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/authzen-configuration":
		m.discoveries.Add(1)
		if m.DiscoveryStatus != 0 {
			w.WriteHeader(m.DiscoveryStatus)
		}
		if m.DiscoveryBody != "" {
			_, _ = w.Write([]byte(m.DiscoveryBody))
			return
		}
		pdp := m.AdvertisedPDP
		if pdp == "" {
			pdp = m.server.URL
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"policy_decision_point":      pdp,
			"access_evaluation_endpoint": pdp + "/access/v1/evaluation",
		})
	case "/access/v1/evaluation":
		m.evaluations.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	default:
		http.NotFound(w, r)
	}
}

func (m *MockAuthZENBroker) URL() string              { return m.server.URL }
func (m *MockAuthZENBroker) Stop()                    { m.server.Close() }
func (m *MockAuthZENBroker) DiscoveryCount() int      { return int(m.discoveries.Load()) }
func (m *MockAuthZENBroker) EvaluationCount() int     { return int(m.evaluations.Load()) }
func (m *MockAuthZENBroker) CredentialCallCount() int { return int(m.credentialCalls.Load()) }
func (m *MockAuthZENBroker) TotalRequestCount() int   { return m.DiscoveryCount() + m.EvaluationCount() }
