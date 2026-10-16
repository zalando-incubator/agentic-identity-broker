package helpers

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// CountingServer counts every request it receives, whatever the path, and answers 404.
// Tests point an issuer URL at it to prove that the broker makes no outbound request.
type CountingServer struct {
	server *httptest.Server
	count  atomic.Int64
}

// NewCountingServer starts a counting server. Callers must Close it.
func NewCountingServer() *CountingServer {
	s := &CountingServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.count.Add(1)
		http.NotFound(w, nil)
	}))
	return s
}

// Count returns the number of requests received so far.
func (s *CountingServer) Count() int {
	return int(s.count.Load())
}

// URL returns the server's base URL.
func (s *CountingServer) URL() string {
	return s.server.URL
}

// Close stops the server.
func (s *CountingServer) Close() {
	s.server.Close()
}
