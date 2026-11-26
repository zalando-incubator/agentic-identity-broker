package opa

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-policy-agent/opa/v1/bundle"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// bundleServer serves bundles built with bundle.Write, each publication with a new ETag.
type bundleServer struct {
	*httptest.Server
	mu       sync.Mutex
	bundles  map[string][]byte
	etags    map[string]string
	gen      int
	failing  bool
	latency  time.Duration
	token    string
	requests int
}

func newBundleServer(t *testing.T) *bundleServer {
	s := &bundleServer{bundles: map[string][]byte{}, etags: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/bundles/"), ".tar.gz")
		s.mu.Lock()
		s.requests++
		body, etag, failing, latency, token := s.bundles[name], s.etags[name], s.failing, s.latency, s.token
		s.mu.Unlock()
		if latency > 0 {
			select {
			case <-time.After(latency):
			case <-r.Context().Done():
				return
			}
		}
		switch {
		case token != "" && r.Header.Get("Authorization") != "Bearer "+token:
			w.WriteHeader(http.StatusUnauthorized)
		case failing:
			w.WriteHeader(http.StatusInternalServerError)
		case body == nil:
			w.WriteHeader(http.StatusNotFound)
		case r.Header.Get("If-None-Match") == etag:
			w.WriteHeader(http.StatusNotModified)
		default:
			w.Header().Set("ETag", etag)
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *bundleServer) publish(t *testing.T, name, revision string, modules map[string]string, data map[string]any, roots ...string) {
	t.Helper()
	files := make([]bundle.ModuleFile, 0, len(modules))
	for path, raw := range modules {
		files = append(files, bundle.ModuleFile{URL: path, Path: path, Raw: []byte(raw)})
	}
	if data == nil {
		data = map[string]any{}
	}
	manifest := bundle.Manifest{Revision: revision}
	if len(roots) > 0 {
		manifest.Roots = &roots
	}
	var buf bytes.Buffer
	require.NoError(t, bundle.Write(&buf, bundle.Bundle{Manifest: manifest, Data: data, Modules: files}))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	s.bundles[name] = buf.Bytes()
	s.etags[name] = fmt.Sprintf(`"%d"`, s.gen)
}

func (s *bundleServer) set(f func(*bundleServer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *bundleServer) bootstrap(t *testing.T, extra string, names ...string) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "services:\n  governance:\n    url: %s\n%s", s.URL, extra)
	b.WriteString("bundles:\n")
	for _, name := range names {
		fmt.Fprintf(&b, "  %s:\n    service: governance\n    resource: bundles/%s.tar.gz\n    polling: {min_delay_seconds: 1, max_delay_seconds: 1}\n", name, name)
	}
	path := filepath.Join(t.TempDir(), "bootstrap.yaml")
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))
	return path
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func admitAll(map[string]bool) error { return nil }

// requireDecisions admits a candidate only when every listed rule's decision exists.
func requireDecisions(rules ...string) ports.ImpersonationPolicyAdmission {
	return func(present map[string]bool) error {
		for _, rule := range rules {
			if !present[rule] {
				return &ports.ImpersonationPolicyAdmissionError{Rule: rule, Err: fmt.Errorf("decision_absent")}
			}
		}
		return nil
	}
}

const allowPolicy = "package company.impersonation\n\nimport rego.v1\n\ndefault result := {\"action\": \"deny\"}\n\nresult := {\"action\": \"allow\", \"reasons\": [\"ignored\"]} if data.governance.allowed\n"

func governance(allowed bool) map[string]any {
	return map[string]any{"governance": map[string]any{"allowed": allowed}}
}

func decision(rule, pkg, name string) ports.ImpersonationPolicyDecisionRef {
	return ports.ImpersonationPolicyDecisionRef{Rule: rule, Package: pkg, Decision: name, EvaluationTimeout: time.Second}
}

// lockedBuffer is a log sink safe for concurrent engine writes and test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
