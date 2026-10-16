package jwks

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
)

// TrustDirectory reads key-set files strictly beneath security.trust_material_dir. It holds the
// directory open through os.Root, which resolves every path component relative to the
// directory and refuses any component that leaves it, including absolute symbolic links.
type TrustDirectory struct {
	configured string
	root       *os.Root
}

// OpenTrustDirectory validates dir and holds it open for contained file access.
func OpenTrustDirectory(dir string) (*TrustDirectory, error) {
	if err := trustanchor.ValidateTrustDirectory(dir); err != nil {
		return nil, err
	}
	invalid := &trustanchor.Error{
		Category: trustanchor.CategoryInvalidTrustDirectory,
		Detail:   map[string]string{trustanchor.DetailDir: dir},
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || filepath.Clean(resolved) == string(filepath.Separator) {
		return nil, invalid
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return nil, invalid
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, invalid
	}
	return &TrustDirectory{configured: filepath.Clean(dir), root: root}, nil
}

// ReadFile reads the regular file at the absolute path, which must lie beneath the trust
// directory. The containment check, the regular-file check, and the read use one descriptor,
// and the read is bounded to trustanchor.MaxKeySetBytes plus one byte.
func (d *TrustDirectory) ReadFile(path string) ([]byte, error) {
	if !trustanchor.Contains(d.configured, path) {
		return nil, d.fileError(trustanchor.CategoryOutsideTrustDirectory, path)
	}
	rel, err := filepath.Rel(d.configured, filepath.Clean(path))
	if err != nil {
		return nil, d.fileError(trustanchor.CategoryOutsideTrustDirectory, path)
	}

	file, err := d.root.OpenFile(rel, openFlags, 0)
	if err != nil {
		return nil, d.fileError(classifyOpenError(err), path)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, d.fileError(trustanchor.CategoryUnreadable, path)
	}
	if !info.Mode().IsRegular() {
		return nil, d.fileError(trustanchor.CategoryNotRegularFile, path)
	}
	doc, err := io.ReadAll(io.LimitReader(file, trustanchor.MaxKeySetBytes+1))
	if err != nil {
		return nil, d.fileError(trustanchor.CategoryUnreadable, path)
	}
	if len(doc) > trustanchor.MaxKeySetBytes {
		return nil, &trustanchor.Error{Category: trustanchor.CategoryTooLarge}
	}
	return doc, nil
}

// Close releases the directory handle.
func (d *TrustDirectory) Close() error {
	return d.root.Close()
}

func (d *TrustDirectory) fileError(category trustanchor.Category, path string) error {
	return &trustanchor.Error{
		Category: category,
		Detail:   map[string]string{trustanchor.DetailPath: path, trustanchor.DetailDir: d.configured},
	}
}

// classifyOpenError maps an os.Root open error to a category. os.Root reports a path that leaves
// the root with an unexported error, so its stable message is matched.
func classifyOpenError(err error) trustanchor.Category {
	switch {
	case strings.Contains(err.Error(), "path escapes from parent"):
		return trustanchor.CategoryOutsideTrustDirectory
	case errors.Is(err, fs.ErrNotExist):
		return trustanchor.CategoryNotFound
	default:
		return trustanchor.CategoryUnreadable
	}
}
