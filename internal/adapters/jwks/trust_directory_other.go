//go:build !unix

package jwks

import "os"

const openFlags = os.O_RDONLY
