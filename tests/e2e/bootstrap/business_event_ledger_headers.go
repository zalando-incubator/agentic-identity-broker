package bootstrap

import "net/http"

// SetLedgerRequestHeaders applies test metadata before issuing any requests.
func (ts *TestServer) SetLedgerRequestHeaders(headers http.Header) {
	base := ts.client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	ts.client.Transport = &ledgerHeaderTransport{base: base, headers: headers.Clone()}
}

type ledgerHeaderTransport struct {
	base    http.RoundTripper
	headers http.Header
}

func (t *ledgerHeaderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	for name, values := range t.headers {
		request.Header[name] = append([]string(nil), values...)
	}
	return t.base.RoundTrip(request)
}
