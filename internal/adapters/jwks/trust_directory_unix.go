//go:build unix

package jwks

import (
	"os"
	"syscall"
)

// openFlags opens without blocking, so a FIFO in the trust directory is rejected as not a
// regular file instead of waiting for a writer.
const openFlags = os.O_RDONLY | syscall.O_NONBLOCK
