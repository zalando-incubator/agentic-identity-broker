// Package telemetrylint checks the rules of coding-standards/OPENTELEMETRY.md and
// coding-standards/LOGGING.md that need type information or repository-specific grammars.
//
// It is loaded into golangci-lint as a module plugin (.custom-gcl.yml) and configured under
// linters.settings.custom.telemetrylint in .golangci.yml.
package telemetrylint

import (
	"fmt"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("telemetrylint", New)
}

type plugin struct {
	settings Settings
}

// New builds the golangci-lint plugin from the settings in .golangci.yml.
func New(raw any) (register.LinterPlugin, error) {
	settings, err := register.DecodeSettings[Settings](raw)
	if err != nil {
		return nil, fmt.Errorf("telemetrylint settings: %w", err)
	}
	if _, err := newConfig(settings); err != nil {
		return nil, err
	}
	return &plugin{settings: settings}, nil
}

func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	analyzer, err := NewAnalyzer(p.settings)
	if err != nil {
		return nil, err
	}
	return []*analysis.Analyzer{analyzer}, nil
}

func (p *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
