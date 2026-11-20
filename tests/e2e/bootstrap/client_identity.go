package bootstrap

import (
	"fmt"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/credentialfile"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ClientIdentityReadBarrier delegates real file acquisition, pausing one captured
// identity before the domain can submit its observation. Install before serving.
type ClientIdentityReadBarrier struct {
	Entered  <-chan struct{}
	entered  chan struct{}
	released chan struct{}
	release  func()
	mu       sync.Mutex
	path     string
	armed    bool
	reads    map[string]int
	next     ports.CredentialFileReader
}

func NewClientIdentityReadBarrier() *ClientIdentityReadBarrier {
	entered, released := make(chan struct{}), make(chan struct{})
	return &ClientIdentityReadBarrier{
		Entered: entered, entered: entered, released: released,
		release: sync.OnceFunc(func() { close(released) }),
		reads:   make(map[string]int), next: credentialfile.New(),
	}
}

func (b *ClientIdentityReadBarrier) Arm(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.path, b.armed = path, true
}

func (b *ClientIdentityReadBarrier) Release() { b.release() }

func (b *ClientIdentityReadBarrier) ReadCount(path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads[path]
}

func (b *ClientIdentityReadBarrier) ReadClientID(path string) (string, error) {
	value, err := b.next.ReadClientID(path)
	b.mu.Lock()
	b.reads[path]++
	pause := b.armed && b.path == path && err == nil
	if pause {
		b.armed = false
		close(b.entered)
	}
	b.mu.Unlock()
	if pause {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-b.released:
		case <-timer.C:
			return "", fmt.Errorf("client identity read barrier was not released")
		}
	}
	return value, err
}

func (b *ClientIdentityReadBarrier) ReadPair(clientIDPath, clientSecretPath string) (string, string, error) {
	return b.next.ReadPair(clientIDPath, clientSecretPath)
}
