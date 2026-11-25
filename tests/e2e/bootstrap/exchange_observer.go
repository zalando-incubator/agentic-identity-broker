package bootstrap

import (
	"bytes"
	"io"
	"net/http"
	"sync/atomic"
)

// ObservedExchange is one request and response seen by a TestServer client.
type ObservedExchange struct {
	Request      *http.Request
	RequestBody  []byte
	Response     *http.Response
	ResponseBody []byte
}

var exchangeObserver atomic.Pointer[func(ObservedExchange)]

// ObserveHTTPExchanges reports every exchange of every TestServer client and
// of the shared helpers client in this process to observe. Pass nil to stop. Bodies are only buffered while
// an observer is set.
func ObserveHTTPExchanges(observe func(ObservedExchange)) {
	if observe == nil {
		exchangeObserver.Store(nil)
		return
	}
	exchangeObserver.Store(&observe)
}

// ObserveTransport wraps a test client's transport so that its exchanges reach the observer.
func ObserveTransport(next http.RoundTripper) http.RoundTripper {
	return observingTransport{next: next}
}

type observingTransport struct{ next http.RoundTripper }

func (t observingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	observe := exchangeObserver.Load()
	if observe == nil {
		return t.next.RoundTrip(req)
	}
	var requestBody []byte
	if req.Body != nil {
		var err error
		if requestBody, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(requestBody))
	}
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	(*observe)(ObservedExchange{Request: req, RequestBody: requestBody, Response: resp, ResponseBody: responseBody})
	return resp, nil
}
