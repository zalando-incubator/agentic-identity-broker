package helpers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"go.yaml.in/yaml/v3"
)

// ChartReleaseName and ChartNamespace are the release coordinates RenderChart uses.
const (
	ChartReleaseName = "broker"
	ChartNamespace   = "identity"
)

// RenderChart renders the broker Helm chart with the given values via the helm binary and
// returns every non-empty manifest document. A render failure returns helm's stderr.
func RenderChart(values map[string]any) ([]map[string]any, error) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		return nil, fmt.Errorf("helm binary not found on PATH; install Helm 3 or later to run chart E2E tests: %w", err)
	}
	chartDir, err := chartDirectory()
	if err != nil {
		return nil, err
	}
	valuesFile, err := os.CreateTemp("", "chart-values-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create values file: %w", err)
	}
	defer func() { _ = os.Remove(valuesFile.Name()) }()
	if err := yaml.NewEncoder(valuesFile).Encode(values); err != nil {
		_ = valuesFile.Close()
		return nil, fmt.Errorf("write values file: %w", err)
	}
	if err := valuesFile.Close(); err != nil {
		return nil, fmt.Errorf("close values file: %w", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(helm, "template", ChartReleaseName, chartDir, "--namespace", ChartNamespace, "--values", valuesFile.Name()) // #nosec G204 -- helm path from LookPath, arguments are test-controlled.
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template failed: %w: %s", err, stderr.String())
	}
	return decodeManifests(stdout.Bytes())
}

// RenderChartRaw renders the chart like RenderChart and returns the raw manifest stream.
func RenderChartRaw(values map[string]any) (string, error) {
	docs, err := RenderChart(values)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	for _, doc := range docs {
		if err := encoder.Encode(doc); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

// FindResource returns the first document with the given kind and metadata.name, or nil.
func FindResource(docs []map[string]any, kind, name string) map[string]any {
	for _, doc := range docs {
		if doc["kind"] != kind {
			continue
		}
		if metadata, ok := doc["metadata"].(map[string]any); ok && metadata["name"] == name {
			return doc
		}
	}
	return nil
}

// FindResourcesByKind returns every document with the given kind.
func FindResourcesByKind(docs []map[string]any, kind string) []map[string]any {
	var matches []map[string]any
	for _, doc := range docs {
		if doc["kind"] == kind {
			matches = append(matches, doc)
		}
	}
	return matches
}

// Path walks nested maps by key and returns the value, or nil when any step is missing.
func Path(doc map[string]any, keys ...string) any {
	var current any = doc
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func decodeManifests(stream []byte) ([]map[string]any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(stream))
	var docs []map[string]any
	for {
		var doc map[string]any
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode rendered manifests: %w", err)
		}
		if len(doc) > 0 {
			docs = append(docs, doc)
		}
	}
}

func chartDirectory() (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate E2E helpers source")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "charts", "agentic-identity-broker"), nil
}
