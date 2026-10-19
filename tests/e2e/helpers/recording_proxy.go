package helpers

import (
	"crypto/tls"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
)

// RecordedRequest is what a RecordingProxy saw for one request.
type RecordedRequest struct {
	Method string
	Path   string
	Host   string
	Header http.Header
}

// RecordingProxy records each request and forwards it to a target, rewriting Host to the
// target's so the admin Host check still passes. With a TLS config it terminates TLS
// itself, which lets tests require client certificates in front of the admin server.
type RecordingProxy struct {
	server *httptest.Server
	mu     sync.Mutex
	seen   []RecordedRequest
}

// NewRecordingProxy starts a proxy in front of target. A nil tlsConfig serves plain HTTP.
func NewRecordingProxy(target string, tlsConfig *tls.Config) (*RecordingProxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	p := &RecordingProxy{}
	forward := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(targetURL) }}
	p.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.seen = append(p.seen, RecordedRequest{Method: r.Method, Path: r.URL.Path, Host: r.Host, Header: r.Header.Clone()})
		p.mu.Unlock()
		forward.ServeHTTP(w, r)
	}))
	if tlsConfig != nil {
		p.server.TLS = tlsConfig
		p.server.StartTLS()
	} else {
		p.server.Start()
	}
	return p, nil
}

// URL returns the proxy's base URL.
func (p *RecordingProxy) URL() string { return p.server.URL }

// CertificatePEM returns the proxy's TLS server certificate, for use as a client CA bundle.
func (p *RecordingProxy) CertificatePEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.server.Certificate().Raw})
}

// Requests returns a copy of the recorded requests.
func (p *RecordingProxy) Requests() []RecordedRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]RecordedRequest(nil), p.seen...)
}

// Close stops the proxy.
func (p *RecordingProxy) Close() { p.server.Close() }
