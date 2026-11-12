package telemetrylint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/telemetrylint"
)

const root = "github.com/agentic-identity-broker/agentic-identity-broker/internal/"

func TestAnalyzer(t *testing.T) {
	analyzer, err := telemetrylint.NewAnalyzer(telemetrylint.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{"scopes", "spans", "metrics", "attrs", "extproc/exempt", "domain/id", "domain/model"} {
		t.Run(pkg, func(t *testing.T) {
			analysistest.Run(t, analysistest.TestData(), analyzer, root+pkg)
		})
	}
}
