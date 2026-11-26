//go:build !unix

package fscontain

import "os"

const openFlags = os.O_RDONLY
