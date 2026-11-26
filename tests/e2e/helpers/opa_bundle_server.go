package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/bundle"
)

// OPABundle is one bundle the server publishes: Rego modules by path, a data document, and a
// manifest revision.
type OPABundle struct {
	Modules  map[string]string
	Data     map[string]any
	Revision string
	Roots    []string
}

// OPABundleServer is an in-process OPA bundle service. It serves gzipped tarballs built with
// bundle.Write under /bundles/<name>.tar.gz, with a fresh ETag for every publication, and can
// fail, delay, or require a bearer token.
type OPABundleServer struct {
	server *httptest.Server

	mu            sync.Mutex
	bundles       map[string][]byte
	etags         map[string]string
	generation    int
	failing       bool
	latency       time.Duration
	expectedToken string
	requests      map[string]int
	authorized    map[string]int
}

// NewOPABundleServer starts an empty bundle server.
func NewOPABundleServer() *OPABundleServer {
	s := &OPABundleServer{
		bundles:    map[string][]byte{},
		etags:      map[string]string{},
		requests:   map[string]int{},
		authorized: map[string]int{},
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// URL is the service URL for the bootstrap's services.<name>.url.
func (s *OPABundleServer) URL() string { return s.server.URL }

// Resource is the bundle resource path for the bootstrap's bundles.<name>.resource.
func (s *OPABundleServer) Resource(name string) string { return "bundles/" + name + ".tar.gz" }

// Close stops the server.
func (s *OPABundleServer) Close() { s.server.Close() }

// Publish replaces the named bundle with a new revision and ETag.
func (s *OPABundleServer) Publish(name string, b OPABundle) error {
	modules := make([]bundle.ModuleFile, 0, len(b.Modules))
	for path, raw := range b.Modules {
		modules = append(modules, bundle.ModuleFile{URL: path, Path: path, Raw: []byte(raw)})
	}
	data := b.Data
	if data == nil {
		data = map[string]any{}
	}
	manifest := bundle.Manifest{Revision: b.Revision}
	if b.Roots != nil {
		roots := append([]string(nil), b.Roots...)
		manifest.Roots = &roots
	}
	var buf bytes.Buffer
	if err := bundle.Write(&buf, bundle.Bundle{Manifest: manifest, Data: data, Modules: modules}); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	s.bundles[name] = buf.Bytes()
	s.etags[name] = fmt.Sprintf(`"%s-%d"`, name, s.generation)
	return nil
}

// SetFailing makes every bundle request fail with 500 until it is reset.
func (s *OPABundleServer) SetFailing(failing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failing = failing
}

// SetLatency delays every bundle response.
func (s *OPABundleServer) SetLatency(latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latency = latency
}

// RequireToken makes the server answer 401 unless a request carries "Bearer <token>". An empty
// token disables the check.
func (s *OPABundleServer) RequireToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectedToken = token
}

// Requests returns how many requests the named bundle received.
func (s *OPABundleServer) Requests(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[name]
}

// AuthorizedRequests returns how many requests for the named bundle carried the expected token.
func (s *OPABundleServer) AuthorizedRequests(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authorized[name]
}

// TotalRequests returns how many requests the server received.
func (s *OPABundleServer) TotalRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, n := range s.requests {
		total += n
	}
	return total
}

func (s *OPABundleServer) serve(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/bundles/"), ".tar.gz")
	s.mu.Lock()
	s.requests[name]++
	latency, failing, expected := s.latency, s.failing, s.expectedToken
	body, etag := s.bundles[name], s.etags[name]
	authorized := expected == "" || r.Header.Get("Authorization") == "Bearer "+expected
	if authorized && expected != "" {
		s.authorized[name]++
	}
	s.mu.Unlock()

	if latency > 0 {
		select {
		case <-time.After(latency):
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case !authorized:
		w.WriteHeader(http.StatusUnauthorized)
	case failing:
		w.WriteHeader(http.StatusInternalServerError)
	case body == nil:
		w.WriteHeader(http.StatusNotFound)
	case r.Header.Get("If-None-Match") == etag:
		w.WriteHeader(http.StatusNotModified)
	default:
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(body)
	}
}

// RequestRecorder is an HTTP server that only counts requests. It is the destination of
// policy-time network calls that must never happen.
type RequestRecorder struct {
	server *httptest.Server
	mu     sync.Mutex
	count  int
}

// NewRequestRecorder starts a recording server.
func NewRequestRecorder() *RequestRecorder {
	rec := &RequestRecorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rec.mu.Lock()
		rec.count++
		rec.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"approved": true})
	}))
	return rec
}

// URL is the recorder's base URL.
func (r *RequestRecorder) URL() string { return r.server.URL }

// Count returns how many requests reached the recorder.
func (r *RequestRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Close stops the recorder.
func (r *RequestRecorder) Close() { r.server.Close() }
