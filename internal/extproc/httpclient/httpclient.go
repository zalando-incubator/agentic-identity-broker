// Package httpclient constructs traced HTTP clients for ExtProc outbound requests.
package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/telemetryhttp"

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
			return nil, &configurationError{cause: err}
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, &configurationError{}
		}
		tlsCfg.RootCAs = pool
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	transport.MaxIdleConnsPerHost = 100
	transport.MaxConnsPerHost = 100
	transport.ForceAttemptHTTP2 = true
	transport.DialContext = (&net.Dialer{Timeout: time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 2 * time.Second

	// Explicit instrumentation excludes every URL component and all error text.
	tracedTransport := telemetryhttp.NewTransport(transport)

	// http.Client.Timeout is the hard deadline for the entire request lifecycle.
	return &http.Client{
		Timeout:   timeout,
		Transport: tracedTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

type configurationError struct {
	cause error
}

func (e *configurationError) Error() string { return "HTTP client CA bundle configuration failed" }
func (e *configurationError) Unwrap() error { return e.cause }
