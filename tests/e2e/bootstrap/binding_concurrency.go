package bootstrap

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"

	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// BindingConcurrencyBroker holds the second exchange response after the real broker completes it.
type BindingConcurrencyBroker struct {
	Server       *httptest.Server
	RetryStarted chan struct{}
	Calls        atomic.Int32
	release      chan struct{}
	once         sync.Once
}

func NewBindingConcurrencyBroker(brokerURL string) *BindingConcurrencyBroker {
	b := &BindingConcurrencyBroker{RetryStarted: make(chan struct{}), release: make(chan struct{})}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := b.Calls.Add(1)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, brokerURL+"/oauth2/token", r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if call == 2 {
			close(b.RetryStarted)
			select {
			case <-b.release:
			case <-r.Context().Done():
				return
			}
		}
		for k, values := range resp.Header {
			w.Header()[k] = values
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	return b
}
func (b *BindingConcurrencyBroker) Release() { b.once.Do(func() { close(b.release) }) }
func (b *BindingConcurrencyBroker) Close()   { b.Release(); b.Server.Close() }

// BindingConcurrencyExchanger captures both initial handles before allowing either validation.
// Both peers use the same production exchanger and independent production permit resolvers.
type BindingConcurrencyExchanger struct {
	inner       extprocserver.Exchanger
	Ready       chan struct{}
	RetryJoined chan struct{}
	release     chan struct{}
	once        sync.Once
}

func NewBindingConcurrencyExchanger(parent *ExtProcEnvironment) *BindingConcurrencyExchanger {
	return &BindingConcurrencyExchanger{inner: parent.exchanger, Ready: make(chan struct{}, 2), RetryJoined: make(chan struct{}, 2), release: make(chan struct{})}
}
func (e *BindingConcurrencyExchanger) Exchange(ctx context.Context, s, r string) (extprocserver.ExchangeResult, error) {
	return e.inner.Exchange(ctx, s, r)
}
func (e *BindingConcurrencyExchanger) ExchangeBound(ctx context.Context, s, r string, observed *extprocserver.ExchangeResult, b extprocserver.TokenBinding) (extprocserver.ExchangeResult, error) {
	if observed == nil {
		result, err := e.inner.Exchange(ctx, s, r)
		if err != nil {
			return result, err
		}
		observed = &result
	}
	e.Ready <- struct{}{}
	select {
	case <-e.release:
	case <-ctx.Done():
		return extprocserver.ExchangeResult{}, ctx.Err()
	}
	return e.inner.ExchangeBound(&bindingJoinedContext{Context: ctx, joined: e.RetryJoined}, s, r, observed, b)
}
func (e *BindingConcurrencyExchanger) Release()  { e.once.Do(func() { close(e.release) }) }
func (e *BindingConcurrencyExchanger) Shutdown() {}

type bindingJoinedContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (c *bindingJoinedContext) Done() <-chan struct{} {
	c.once.Do(func() { c.joined <- struct{}{} })
	return c.Context.Done()
}
