// Package httpclient constructs traced HTTP clients for ExtProc outbound requests.
package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// New constructs an http.Client respecting the TLS configuration.
// Supports InsecureSkipVerify and CaBundlePath from the TLS config block.
// Returns an error if CaBundlePath is set but the file cannot be read or parsed.
func New(cfg *extprocconfig.Config, timeout time.Duration) (*http.Client, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.OAuth2.TLS.InsecureSkipVerify, // #nosec G402 -- explicit operator-controlled development TLS option.
	}

	if cfg.OAuth2.TLS.CaBundlePath != "" {
		pemData, err := os.ReadFile(cfg.OAuth2.TLS.CaBundlePath)
		if err != nil {
			return nil, fmt.Errorf("reading ca_bundle_path %q: %w", cfg.OAuth2.TLS.CaBundlePath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("ca_bundle_path %q contains no valid PEM certificates", cfg.OAuth2.TLS.CaBundlePath)
		}
		tlsCfg.RootCAs = pool
	}

	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	// Wrap with otelhttp for automatic span creation on outbound requests.
	// otelhttp resolves the TracerProvider lazily (from otel.GetTracerProvider() at
	// request time, not construction time), so this transport correctly picks up
	// provider changes made after construction — including E2E test global swaps.
	tracedTransport := otelhttp.NewTransport(transport)

	// http.Client.Timeout is the hard deadline for the entire request lifecycle.
	return &http.Client{
		Timeout:   timeout,
		Transport: tracedTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}
