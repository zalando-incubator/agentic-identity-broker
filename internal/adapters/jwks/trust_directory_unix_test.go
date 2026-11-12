//go:build unix

package jwks

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
)

func TestTrustDirectoryNeverBlocksOnAFIFO(t *testing.T) {
	layout := newTrustLayout(t)
	trust := openTestTrustDirectory(t, layout.trust)
	fifo := filepath.Join(layout.trust, "pipe.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := trust.ReadFile(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		assert.Equal(t, trustanchor.CategoryNotRegularFile, trustanchor.CategoryOf(err))
	case <-time.After(2 * time.Second):
		t.Fatal("ReadFile blocked on a FIFO")
	}
}
