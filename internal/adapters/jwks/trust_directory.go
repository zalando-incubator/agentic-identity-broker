package jwks

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/fscontain"
)

// TrustDirectory reads key-set files strictly beneath security.trust_material_dir. It holds the
// directory open through fscontain, which resolves every path component relative to the
// directory and refuses any component that leaves it, including absolute symbolic links.
type TrustDirectory struct {
	configured string
	dir        *fscontain.Dir
}

// OpenTrustDirectory validates dir and holds it open for contained file access.
func OpenTrustDirectory(dir string) (*TrustDirectory, error) {
	if err := trustanchor.ValidateTrustDirectory(dir); err != nil {
		return nil, err
	}
	contained, err := fscontain.OpenDir(dir)
	if err != nil {
		return nil, &trustanchor.Error{
			Category: trustanchor.CategoryInvalidTrustDirectory,
			Detail:   map[string]string{trustanchor.DetailDir: dir},
		}
	}
	return &TrustDirectory{configured: filepath.Clean(dir), dir: contained}, nil
}

// ReadFile reads the regular file at the absolute path, which must lie beneath the trust
// directory. The containment check, the regular-file check, and the read use one descriptor,
// and the read is bounded to trustanchor.MaxKeySetBytes.
func (d *TrustDirectory) ReadFile(path string) ([]byte, error) {
	if !trustanchor.Contains(d.configured, path) {
		return nil, d.fileError(trustanchor.CategoryOutsideTrustDirectory, path)
	}
	rel, err := filepath.Rel(d.configured, filepath.Clean(path))
	if err != nil {
		return nil, d.fileError(trustanchor.CategoryOutsideTrustDirectory, path)
	}

	doc, err := d.dir.ReadFile(rel, trustanchor.MaxKeySetBytes)
	if err != nil {
		if fscontain.ReasonOf(err) == fscontain.ReasonFileTooLarge {
			return nil, &trustanchor.Error{Category: trustanchor.CategoryTooLarge}
		}
		return nil, d.fileError(classifyOpenError(err), path)
	}
	return doc, nil
}

// Close releases the directory handle.
func (d *TrustDirectory) Close() error {
	return d.dir.Close()
}

func (d *TrustDirectory) fileError(category trustanchor.Category, path string) error {
	return &trustanchor.Error{
		Category: category,
		Detail:   map[string]string{trustanchor.DetailPath: path, trustanchor.DetailDir: d.configured},
	}
}

// classifyOpenError maps a containment failure, or a raw os.Root open error, to its trust-anchor
// category.
func classifyOpenError(err error) trustanchor.Category {
	reason := fscontain.ReasonOf(err)
	if reason == "" {
		reason = fscontain.ClassifyOpenError(err)
	}
	switch reason {
	case fscontain.ReasonEscapingLink:
		return trustanchor.CategoryOutsideTrustDirectory
	case fscontain.ReasonNotRegular:
		return trustanchor.CategoryNotRegularFile
	}
	if errors.Is(err, fs.ErrNotExist) {
		return trustanchor.CategoryNotFound
	}
	return trustanchor.CategoryUnreadable
}
