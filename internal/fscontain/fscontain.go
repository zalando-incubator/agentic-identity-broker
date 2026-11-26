// Package fscontain reads files strictly beneath one directory. It holds the directory open
// through os.Root, which resolves every path component relative to the directory and refuses any
// component that leaves it, including absolute symbolic links. Errors carry a closed reason and a
// relative name, never file content.
package fscontain

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Reason classifies a containment failure.
type Reason string

// Containment failure reasons. ReasonUnreadable wraps the underlying error, so callers can
// still detect fs.ErrNotExist with errors.Is.
const (
	ReasonEscapingLink  Reason = "escaping_link"
	ReasonRootDirectory Reason = "root_directory"
	ReasonNotAbsolute   Reason = "not_absolute"
	ReasonNotRegular    Reason = "not_regular"
	ReasonFileTooLarge  Reason = "file_too_large"
	ReasonTooManyFiles  Reason = "too_many_files"
	ReasonUnreadable    Reason = "unreadable"
)

// Error is a containment failure for one relative name.
type Error struct {
	Reason Reason
	Name   string
	Err    error
}

func (e *Error) Error() string {
	if e.Name == "" {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %s", e.Name, e.Reason)
}

func (e *Error) Unwrap() error { return e.Err }

// ReasonOf returns the reason of a containment error, or "" when err is not one.
func ReasonOf(err error) Reason {
	var containErr *Error
	if errors.As(err, &containErr) {
		return containErr.Reason
	}
	return ""
}

// Dir is an open directory whose files are read only through its root.
type Dir struct {
	root *os.Root
}

// OpenDir opens an os.Root over path after resolving its symbolic links. path must be absolute
// and must not resolve to the file system root.
func OpenDir(path string) (*Dir, error) {
	if !filepath.IsAbs(path) {
		return nil, &Error{Reason: ReasonNotAbsolute}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, &Error{Reason: ReasonUnreadable, Err: err}
	}
	if filepath.Clean(resolved) == string(filepath.Separator) {
		return nil, &Error{Reason: ReasonRootDirectory}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, &Error{Reason: ReasonUnreadable, Err: err}
	}
	if !info.IsDir() {
		return nil, &Error{Reason: ReasonNotRegular}
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, &Error{Reason: ReasonUnreadable, Err: err}
	}
	return &Dir{root: root}, nil
}

// ReadFile opens name relative to the root, checks on the same descriptor that it is a regular
// file, and reads at most max bytes. A larger file fails with ReasonFileTooLarge.
func (d *Dir) ReadFile(name string, max int64) ([]byte, error) {
	file, err := d.root.OpenFile(name, openFlags, 0)
	if err != nil {
		return nil, &Error{Reason: ClassifyOpenError(err), Name: name, Err: err}
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, &Error{Reason: ReasonUnreadable, Name: name, Err: err}
	}
	if !info.Mode().IsRegular() {
		return nil, &Error{Reason: ReasonNotRegular, Name: name}
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, &Error{Reason: ReasonUnreadable, Name: name, Err: err}
	}
	if int64(len(data)) > max {
		return nil, &Error{Reason: ReasonFileTooLarge, Name: name}
	}
	return data, nil
}

// Walk visits the files below the root in lexical order. Every non-directory entry counts toward
// maxFiles and must be, or link inside the root to, a regular file. accept selects the entries
// whose bytes are read and passed to fn; their cumulative size must not exceed maxBytes.
func (d *Dir) Walk(maxFiles int, maxBytes int64, accept func(name string) bool, fn func(name string, data []byte) error) error {
	files := 0
	var total int64
	return fs.WalkDir(d.root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return &Error{Reason: ReasonUnreadable, Name: name, Err: walkErr}
		}
		if entry.IsDir() {
			return nil
		}
		files++
		if files > maxFiles {
			return &Error{Reason: ReasonTooManyFiles, Name: name}
		}
		if !accept(name) {
			return d.requireRegular(name)
		}
		data, err := d.ReadFile(name, maxBytes-total)
		if err != nil {
			return err
		}
		total += int64(len(data))
		return fn(name, data)
	})
}

func (d *Dir) requireRegular(name string) error {
	file, err := d.root.OpenFile(name, openFlags, 0)
	if err != nil {
		return &Error{Reason: ClassifyOpenError(err), Name: name, Err: err}
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return &Error{Reason: ReasonUnreadable, Name: name, Err: err}
	}
	if !info.Mode().IsRegular() {
		return &Error{Reason: ReasonNotRegular, Name: name}
	}
	return nil
}

// Close releases the directory handle.
func (d *Dir) Close() error {
	return d.root.Close()
}

// ClassifyOpenError maps an os.Root open error to a reason. os.Root reports a path that leaves
// the root with an unexported error, so its stable message is matched.
func ClassifyOpenError(err error) Reason {
	if strings.Contains(err.Error(), "path escapes from parent") {
		return ReasonEscapingLink
	}
	return ReasonUnreadable
}
