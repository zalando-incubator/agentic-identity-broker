package jwks

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
)

const testKeySet = `{"keys":[{"kty":"EC","crv":"P-256","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0","kid":"k1"}]}`

type trustLayout struct {
	root    string
	trust   string
	outside string
}

func newTrustLayout(t *testing.T) trustLayout {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	layout := trustLayout{root: base, trust: filepath.Join(base, "trust"), outside: filepath.Join(base, "outside")}
	require.NoError(t, os.Mkdir(layout.trust, 0o755))
	require.NoError(t, os.Mkdir(layout.outside, 0o755))
	return layout
}

func (l trustLayout) write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// kubernetes reproduces the Secret volume layout: name -> ..data/name -> ..<ts>/name.
func (l trustLayout) kubernetes(t *testing.T, name, content string) string {
	t.Helper()
	generation := filepath.Join(l.trust, "..2026_10_16_08_50_00.1")
	require.NoError(t, os.Mkdir(generation, 0o755))
	l.write(t, generation, name, content)
	require.NoError(t, os.Symlink(filepath.Base(generation), filepath.Join(l.trust, "..data")))
	link := filepath.Join(l.trust, name)
	require.NoError(t, os.Symlink(filepath.Join("..data", name), link))
	return link
}

func openTestTrustDirectory(t *testing.T, dir string) *TrustDirectory {
	t.Helper()
	trust, err := OpenTrustDirectory(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = trust.Close() })
	return trust
}

func TestTrustDirectoryReadsContainedFiles(t *testing.T) {
	layout := newTrustLayout(t)
	trust := openTestTrustDirectory(t, layout.trust)

	t.Run("a plain file", func(t *testing.T) {
		doc, err := trust.ReadFile(layout.write(t, layout.trust, "plain.json", testKeySet))
		require.NoError(t, err)
		assert.Equal(t, testKeySet, string(doc))
	})

	t.Run("the Kubernetes relative-link layout", func(t *testing.T) {
		doc, err := trust.ReadFile(layout.kubernetes(t, "jwks.json", testKeySet))
		require.NoError(t, err)
		assert.Equal(t, testKeySet, string(doc))
	})

	t.Run("a relative link to a file inside", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(layout.trust, "sub"), 0o755))
		layout.write(t, filepath.Join(layout.trust, "sub"), "target.json", testKeySet)
		link := filepath.Join(layout.trust, "relative.json")
		require.NoError(t, os.Symlink("sub/target.json", link))
		_, err := trust.ReadFile(link)
		require.NoError(t, err)
	})
}

func TestTrustDirectoryRefusesUnsafePaths(t *testing.T) {
	layout := newTrustLayout(t)
	trust := openTestTrustDirectory(t, layout.trust)
	outsideFile := layout.write(t, layout.outside, "secret.json", testKeySet)
	insideFile := layout.write(t, layout.trust, "inside.json", testKeySet)

	relativeEscape := filepath.Join(layout.trust, "escape.json")
	require.NoError(t, os.Symlink("../outside/secret.json", relativeEscape))
	absoluteOutside := filepath.Join(layout.trust, "absolute-outside.json")
	require.NoError(t, os.Symlink(outsideFile, absoluteOutside))
	absoluteInside := filepath.Join(layout.trust, "absolute-inside.json")
	require.NoError(t, os.Symlink(insideFile, absoluteInside))
	directory := filepath.Join(layout.trust, "a-directory")
	require.NoError(t, os.Mkdir(directory, 0o755))
	dangling := filepath.Join(layout.trust, "dangling.json")
	require.NoError(t, os.Symlink("missing-target.json", dangling))

	tests := []struct {
		name string
		path string
		want trustanchor.Category
	}{
		{name: "dot-dot escape", path: layout.trust + "/../outside/secret.json", want: trustanchor.CategoryOutsideTrustDirectory},
		{name: "path outside", path: outsideFile, want: trustanchor.CategoryOutsideTrustDirectory},
		{name: "relative link escape", path: relativeEscape, want: trustanchor.CategoryOutsideTrustDirectory},
		{name: "absolute link outside", path: absoluteOutside, want: trustanchor.CategoryOutsideTrustDirectory},
		{name: "absolute link inside is still refused", path: absoluteInside, want: trustanchor.CategoryOutsideTrustDirectory},
		{name: "directory", path: directory, want: trustanchor.CategoryNotRegularFile},
		{name: "missing file", path: filepath.Join(layout.trust, "missing.json"), want: trustanchor.CategoryNotFound},
		{name: "dangling link", path: dangling, want: trustanchor.CategoryNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := trust.ReadFile(tc.path)
			require.Error(t, err)
			assert.Equal(t, tc.want, trustanchor.CategoryOf(err), err.Error())
		})
	}

	t.Run("an outside path names the path and the trust directory", func(t *testing.T) {
		_, err := trust.ReadFile(absoluteInside)
		require.Error(t, err)
		assert.Equal(t, `jwks_file "`+absoluteInside+`" is outside security.trust_material_dir "`+layout.trust+`"`, err.Error())
	})
}

func TestTrustDirectoryBoundsTheRead(t *testing.T) {
	layout := newTrustLayout(t)
	trust := openTestTrustDirectory(t, layout.trust)

	exact := layout.write(t, layout.trust, "exact.json", strings.Repeat("a", trustanchor.MaxKeySetBytes))
	doc, err := trust.ReadFile(exact)
	require.NoError(t, err)
	assert.Len(t, doc, trustanchor.MaxKeySetBytes)

	over := layout.write(t, layout.trust, "over.json", strings.Repeat("a", trustanchor.MaxKeySetBytes+10))
	_, err = trust.ReadFile(over)
	assert.Equal(t, trustanchor.CategoryTooLarge, trustanchor.CategoryOf(err))
}

func TestOpenTrustDirectoryValidation(t *testing.T) {
	layout := newTrustLayout(t)
	file := layout.write(t, layout.root, "not-a-dir", "x")
	link := filepath.Join(layout.root, "trust-link")
	require.NoError(t, os.Symlink(layout.trust, link))

	for name, dir := range map[string]string{
		"filesystem root":     "/",
		"relative":            "trust",
		"missing":             filepath.Join(layout.root, "missing"),
		"a file":              file,
		"resolves to root":    layout.root + "/../../../../../../../..",
		"empty":               "",
		"root via dot-dot":    "/tmp/..",
		"root with separator": "//",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := OpenTrustDirectory(dir)
			assert.Equal(t, trustanchor.CategoryInvalidTrustDirectory, trustanchor.CategoryOf(err))
		})
	}

	t.Run("a symbolic link to a directory resolves", func(t *testing.T) {
		trust := openTestTrustDirectory(t, link)
		doc, err := trust.ReadFile(layout.write(t, layout.trust, "via-link.json", testKeySet))
		require.Error(t, err, "the configured path is the link, so a file named through the real directory is outside it")
		assert.Nil(t, doc)

		doc, err = trust.ReadFile(filepath.Join(link, "via-link.json"))
		require.NoError(t, err)
		assert.Equal(t, testKeySet, string(doc))
	})
}

func TestClassifyOpenError(t *testing.T) {
	assert.Equal(t, trustanchor.CategoryNotFound, classifyOpenError(fs.ErrNotExist))
	assert.Equal(t, trustanchor.CategoryUnreadable, classifyOpenError(fs.ErrPermission))
	assert.Equal(t, trustanchor.CategoryUnreadable, classifyOpenError(errors.New("i/o error")))

	layout := newTrustLayout(t)
	root, err := os.OpenRoot(layout.trust)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	_, err = root.Open("../outside")
	require.Error(t, err)
	assert.Equal(t, trustanchor.CategoryOutsideTrustDirectory, classifyOpenError(err),
		"os.Root's escape error must still be recognised: %v", err)
}

func TestTrustDirectoryClose(t *testing.T) {
	layout := newTrustLayout(t)
	trust, err := OpenTrustDirectory(layout.trust)
	require.NoError(t, err)
	require.NoError(t, trust.Close())
	_, err = trust.ReadFile(layout.write(t, layout.trust, "after-close.json", testKeySet))
	require.Error(t, err)
}
