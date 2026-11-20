package telemetrylint_test

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/telemetrylint"
)

func TestCustomSettings(t *testing.T) {
	analyzer, err := telemetrylint.NewAnalyzer(telemetrylint.Settings{
		Scopes:                     []string{"billing"},
		PrincipalKeyExemptPackages: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, root+"custom")
}

func TestInvalidSettings(t *testing.T) {
	for name, settings := range map[string]telemetrylint.Settings{
		"scope not snake_case":     {Scopes: []string{"Billing"}},
		"principal not qualified":  {PrincipalType: "Principal"},
		"credential not qualified": {CredentialTypes: []string{"example.com/model."}},
		"exempt pattern invalid":   {PrincipalKeyExemptPackages: []string{"("}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := telemetrylint.NewAnalyzer(settings); err == nil {
				t.Fatal("NewAnalyzer accepted invalid settings")
			}
		})
	}
}

func TestPlugin(t *testing.T) {
	newPlugin, err := register.GetPlugin("telemetrylint")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newPlugin(map[string]any{"unknown": true}); err == nil {
		t.Fatal("plugin accepted an unknown setting")
	}
	plugin, err := newPlugin(map[string]any{"scopes": []string{"storage"}})
	if err != nil {
		t.Fatal(err)
	}
	if plugin.GetLoadMode() != register.LoadModeTypesInfo {
		t.Fatalf("load mode = %q, want %q", plugin.GetLoadMode(), register.LoadModeTypesInfo)
	}
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil || len(analyzers) != 1 || analyzers[0].Name != "telemetrylint" {
		t.Fatalf("BuildAnalyzers() = %v, %v", analyzers, err)
	}
}
