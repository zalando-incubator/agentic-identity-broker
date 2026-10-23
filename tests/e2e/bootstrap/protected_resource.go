package bootstrap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/outboundhttp"
	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
)

// NewProtectedResourceHTTPClient trusts one TLS test server and routes only the
// explicitly supplied synthetic public hosts to it. All other hosts fail closed.
func NewProtectedResourceHTTPClient(server *httptest.Server, hosts ...string) *http.Client {
	if server == nil || server.Certificate() == nil {
		panic("protected-resource test server must be started with TLS")
	}

	allowed := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(host)
		if len(host) > len(".example.test") && strings.HasSuffix(host, ".example.test") && !strings.ContainsAny(host, ":/") {
			allowed[host] = struct{}{}
		}
	}

	serverAddr := server.Listener.Addr().String()
	serverHost, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		panic("protected-resource test server has no TCP address")
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())

	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
			ServerName: serverHost,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid protected-resource test destination: %w", err)
			}
			if port != "443" {
				return nil, fmt.Errorf("protected-resource test destination must use HTTPS")
			}
			if _, ok := allowed[strings.ToLower(host)]; !ok {
				return nil, fmt.Errorf("protected-resource test destination is not allowlisted")
			}
			return (&net.Dialer{}).DialContext(ctx, network, serverAddr)
		},
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// BuildAppWithProtectedResource uses one TLS client for the discovery port and
// discovery-backed token requests. The Builder's manual-token client is unchanged.
func (f *ServerFactory) BuildAppWithProtectedResource(storage interface{}, server *httptest.Server, hosts ...string) (*app.App, error) {
	if f == nil || f.config == nil || f.logger == nil {
		return nil, fmt.Errorf("protected-resource server factory requires config and logger")
	}
	storageAdapter, ok := storage.(*storageadapter.Adapter)
	if !ok || storageAdapter == nil {
		return nil, fmt.Errorf("storage must be *storageadapter.Adapter")
	}
	if server == nil || server.Certificate() == nil {
		return nil, fmt.Errorf("protected-resource test server must be started with TLS")
	}

	client := NewProtectedResourceHTTPClient(server, hosts...)
	return app.NewBuilder().
		WithConfig(f.config).
		WithStorage(storageAdapter).
		WithLogger(f.logger).
		WithStaticWebResourcesPath(webDistPath()).
		WithOAuthDiscoveryClient(outboundhttp.NewDiscoveryClientWithClient(client)).
		WithDiscoveryTokenHTTPClient(client).
		Build()
}
