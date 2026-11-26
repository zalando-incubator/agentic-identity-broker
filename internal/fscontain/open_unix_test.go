//go:build unix

package fscontain

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReadFileNeverBlocksOnAFIFO(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	dir := openTestDir(t, root)

	for name, read := range map[string]func() error{
		"ReadFile": func() error { _, err := dir.ReadFile("pipe", 64); return err },
		"Walk": func() error {
			return dir.Walk(10, 64, func(string) bool { return true }, func(string, []byte) error { return nil })
		},
	} {
		done := make(chan error, 1)
		go func() { done <- read() }()
		select {
		case err := <-done:
			assert.Equal(t, ReasonNotRegular, ReasonOf(err), name)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s blocked on a FIFO", name)
		}
	}
}
