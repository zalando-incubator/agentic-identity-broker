package encryption

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchitectureGlossaryDocumentsBranchKeySubjects(t *testing.T) {
	architecture := readRepoFile(t, "docs/ARCHITECTURE.md")
	encryptionDomain := extractSection(t, architecture, "### Encryption Domain", "### Session Management Domain")

	assert.Contains(t, encryptionDomain, "**BranchKeySubject**", "architecture glossary must define BranchKeySubject")
	assert.Contains(t, encryptionDomain, "**BranchKeySubjectKind**", "architecture glossary must define BranchKeySubjectKind")
	assert.Contains(t, encryptionDomain, "**ContextKeyKID**", "architecture glossary must define ContextKeyKID")
	assert.Regexp(
		t,
		regexp.MustCompile(`(?s)\*\*EncryptionContext\*\*:.*service_id.*kid`),
		encryptionDomain,
		"architecture glossary must document both approved encryption-context subject keys",
	)
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve caller path")

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	content, err := os.ReadFile(filepath.Join(repoRoot, relativePath))
	require.NoError(t, err)
	return string(content)
}

func extractSection(t *testing.T, content, startHeading, endHeading string) string {
	t.Helper()

	start := strings.Index(content, startHeading)
	require.NotEqualf(t, -1, start, "missing start heading %q", startHeading)
	end := strings.Index(content[start:], endHeading)
	require.NotEqualf(t, -1, end, "missing end heading %q", endHeading)
	return content[start : start+end]
}
