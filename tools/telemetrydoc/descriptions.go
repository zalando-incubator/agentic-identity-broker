package main

import (
	"fmt"
	"os"
	"sort"

	"go.yaml.in/yaml/v3"
)

// descriptionsPath is the file with the span and attribute descriptions,
// relative to the repository root.
const descriptionsPath = "docs/reference/telemetry-descriptions.yaml"

// Descriptions are the texts that the code cannot provide. Metric
// descriptions come from the code (metric.WithDescription).
type Descriptions struct {
	Spans      map[string]string `yaml:"spans"`
	Attributes map[string]string `yaml:"attributes"`
}

// LoadDescriptions reads the descriptions file.
func LoadDescriptions(path string) (*Descriptions, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Descriptions
	if err := yaml.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &d, nil
}

// checkDescriptions returns a problem for each span or attribute key of ref
// without a description, and for each description of a span or a key that
// ref does not have.
func checkDescriptions(ref *Reference, d *Descriptions) []string {
	var problems []string
	compare := func(kind, verb string, code map[string]bool, described map[string]string) {
		for n := range code {
			if described[n] == "" {
				problems = append(problems, fmt.Sprintf("%s %s has no description", kind, n))
			}
		}
		for n := range described {
			if !code[n] {
				problems = append(problems, fmt.Sprintf("%s %s is described, but the code does not %s it", kind, n, verb))
			}
		}
	}
	spans := map[string]bool{}
	for n := range ref.Spans {
		spans[n] = true
	}
	compare("span", "create", spans, d.Spans)
	compare("attribute", "set", allAttrKeys(ref), d.Attributes)
	sort.Strings(problems)
	return problems
}

// allAttrKeys returns the attribute keys of all spans and metrics in ref.
func allAttrKeys(ref *Reference) map[string]bool {
	keys := map[string]bool{}
	for _, s := range ref.Spans {
		for k := range s.Attrs {
			keys[k] = true
		}
	}
	for _, m := range ref.Metrics {
		for k := range m.Attrs {
			keys[k] = true
		}
	}
	return keys
}
