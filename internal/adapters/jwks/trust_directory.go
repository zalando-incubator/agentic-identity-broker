package jwks

import "os"

// TrustDirectory opens key-set files strictly beneath security.trust_material_dir.
type TrustDirectory struct{}

// OpenTrustDirectory validates dir and holds it open for contained file access.
func OpenTrustDirectory(dir string) (*TrustDirectory, error) {
	_ = dir
	return nil, errNotImplemented
}

// Open opens the regular file at the absolute path, refusing any path or symbolic link that
// leaves the trust directory.
func (d *TrustDirectory) Open(path string) (*os.File, error) {
	_ = path
	return nil, errNotImplemented
}

// Close releases the directory handle.
func (d *TrustDirectory) Close() error {
	return nil
}
