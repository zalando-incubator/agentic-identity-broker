package fscontain

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func openTestDir(t *testing.T, path string) *Dir {
	t.Helper()
	dir, err := OpenDir(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dir.Close() })
	return dir
}

func TestOpenDir(t *testing.T) {
	t.Run("a relative path is rejected", func(t *testing.T) {
		_, err := OpenDir("relative/dir")
		assert.Equal(t, ReasonNotAbsolute, ReasonOf(err))
	})
	t.Run("the file system root is rejected", func(t *testing.T) {
		_, err := OpenDir(string(filepath.Separator))
		assert.Equal(t, ReasonRootDirectory, ReasonOf(err))
	})
	t.Run("a link resolving to the file system root is rejected", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "root")
		require.NoError(t, os.Symlink(string(filepath.Separator), link))
		_, err := OpenDir(link)
		assert.Equal(t, ReasonRootDirectory, ReasonOf(err))
	})
	t.Run("a missing directory is unreadable", func(t *testing.T) {
		_, err := OpenDir(filepath.Join(t.TempDir(), "missing"))
		assert.Equal(t, ReasonUnreadable, ReasonOf(err))
		assert.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("a regular file is not a directory", func(t *testing.T) {
		_, err := OpenDir(writeFile(t, t.TempDir(), "file", "x"))
		assert.Equal(t, ReasonNotRegular, ReasonOf(err))
	})
}

func TestReadFile(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	writeFile(t, root, "file.txt", "content")
	writeFile(t, root, "nested/file.txt", "nested")
	outside := writeFile(t, base, "outside.txt", "secret-outside-content")
	require.NoError(t, os.Symlink("file.txt", filepath.Join(root, "inside-link")))
	require.NoError(t, os.Symlink("../outside.txt", filepath.Join(root, "relative-escape")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "absolute-link")))
	require.NoError(t, os.Symlink(filepath.Join(root, "file.txt"), filepath.Join(root, "absolute-inside")))
	dir := openTestDir(t, root)

	data, err := dir.ReadFile("file.txt", 64)
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))

	data, err = dir.ReadFile("inside-link", 64)
	require.NoError(t, err, "a relative link that stays inside is followed")
	assert.Equal(t, "content", string(data))

	data, err = dir.ReadFile("nested/file.txt", 64)
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))

	for _, name := range []string{"relative-escape", "absolute-link", "absolute-inside", "../outside.txt"} {
		_, err := dir.ReadFile(name, 64)
		assert.Equal(t, ReasonEscapingLink, ReasonOf(err), name)
		assert.NotContains(t, err.Error(), "secret-outside-content")
	}

	_, err = dir.ReadFile("missing", 64)
	assert.Equal(t, ReasonUnreadable, ReasonOf(err))
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	_, err = dir.ReadFile("nested", 64)
	assert.Equal(t, ReasonNotRegular, ReasonOf(err))

	data, err = dir.ReadFile("file.txt", int64(len("content")))
	require.NoError(t, err, "a file of exactly the bound is accepted")
	assert.Equal(t, "content", string(data))

	_, err = dir.ReadFile("file.txt", int64(len("content"))-1)
	assert.Equal(t, ReasonFileTooLarge, ReasonOf(err))
	assert.NotContains(t, err.Error(), "content")
	assert.Equal(t, "file.txt: file_too_large", err.Error())
}

func TestWalk(t *testing.T) {
	collect := func(t *testing.T, dir *Dir, maxFiles int, maxBytes int64, accept func(string) bool) (map[string]string, []string, error) {
		t.Helper()
		got := map[string]string{}
		var order []string
		err := dir.Walk(maxFiles, maxBytes, accept, func(name string, data []byte) error {
			got[name] = string(data)
			order = append(order, name)
			return nil
		})
		return got, order, err
	}
	all := func(string) bool { return true }

	t.Run("visits accepted regular files in lexical order", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "b.rego", "b")
		writeFile(t, root, "a/data.json", "{}")
		writeFile(t, root, "skip.txt", "ignored")
		got, order, err := collect(t, openTestDir(t, root), 10, 64, func(name string) bool { return !strings.HasSuffix(name, ".txt") })
		require.NoError(t, err)
		assert.Equal(t, []string{"a/data.json", "b.rego"}, order)
		assert.Equal(t, map[string]string{"a/data.json": "{}", "b.rego": "b"}, got)
	})
	t.Run("too many files", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a", "1")
		writeFile(t, root, "b", "2")
		writeFile(t, root, "c", "3")
		_, _, err := collect(t, openTestDir(t, root), 2, 64, all)
		assert.Equal(t, ReasonTooManyFiles, ReasonOf(err))
	})
	t.Run("cumulative size above the bound", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a", "1234")
		writeFile(t, root, "b", "5678")
		_, _, err := collect(t, openTestDir(t, root), 10, 7, all)
		assert.Equal(t, ReasonFileTooLarge, ReasonOf(err))
		_, _, err = collect(t, openTestDir(t, root), 10, 8, all)
		assert.NoError(t, err)
	})
	t.Run("an escaping link fails even when not accepted", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "root")
		writeFile(t, root, "a.rego", "a")
		writeFile(t, base, "outside.json", "{}")
		require.NoError(t, os.Symlink("../outside.json", filepath.Join(root, "data.json")))
		_, _, err := collect(t, openTestDir(t, root), 10, 64, all)
		assert.Equal(t, ReasonEscapingLink, ReasonOf(err))
		assert.Equal(t, "data.json", err.(*Error).Name)
		_, _, err = collect(t, openTestDir(t, root), 10, 64, func(string) bool { return false })
		assert.Equal(t, ReasonEscapingLink, ReasonOf(err))
	})
	t.Run("a link to a directory is not a regular file", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "sub/a.rego", "a")
		require.NoError(t, os.Symlink("sub", filepath.Join(root, "linked")))
		_, _, err := collect(t, openTestDir(t, root), 10, 64, all)
		assert.Equal(t, ReasonNotRegular, ReasonOf(err))
	})
	t.Run("a callback error stops the walk", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a", "1")
		stop := errors.New("stop")
		err := openTestDir(t, root).Walk(10, 64, all, func(string, []byte) error { return stop })
		assert.ErrorIs(t, err, stop)
	})
}
