package aibctl_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePrefix = "github.com/agentic-identity-broker/agentic-identity-broker/internal/"

var forbiddenPrefixes = []string{"domain", "ports", "adapters", "app", "extproc", "config"}

// TestAibctlDoesNotImportBrokerPackages keeps the CLI a standalone client of
// the HTTP contract, independent of the broker's internal packages.
func TestAibctlDoesNotImportBrokerPackages(t *testing.T) {
	roots := []string{".", filepath.Join("..", "..", "cmd", "aibctl")}
	var offenders []string
	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range file.Imports {
				importPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return err
				}
				if !strings.HasPrefix(importPath, modulePrefix) {
					continue
				}
				rest := strings.TrimPrefix(importPath, modulePrefix)
				for _, forbidden := range forbiddenPrefixes {
					if rest == forbidden || strings.HasPrefix(rest, forbidden+"/") {
						offenders = append(offenders, path+" imports "+importPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("aibctl must not import broker packages:\n%s", strings.Join(offenders, "\n"))
	}
}
