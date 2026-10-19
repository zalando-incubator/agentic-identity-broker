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

// RenderedChart holds the parts of a rendered broker chart that tests assert on.
type RenderedChart struct {
	// ConfigYAML is the broker config.yaml from the rendered ConfigMap.
	ConfigYAML string
	// TrustVolume is the Deployment's trust-material volume, nil when absent.
	TrustVolume map[string]any
	// TrustMount is the broker container's trust-material volume mount, nil when absent.
	TrustMount map[string]any
}

// ChartDir returns the absolute path of charts/agentic-identity-broker.
func ChartDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "charts", "agentic-identity-broker")
}

// RenderChart runs `helm template` with valuesYAML and returns the rendered broker configuration
// and trust-material volume. A missing helm binary is an error, never a skip.
func RenderChart(valuesYAML string) (*RenderedChart, error) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		return nil, fmt.Errorf("helm binary not found on PATH; install Helm 3 to run chart-rendering E2E tests: %w", err)
	}

	file, err := os.CreateTemp("", "helm-values-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create values file: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.WriteString(valuesYAML); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write values file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close values file: %w", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(helm, "template", "broker", ChartDir(), "--values", file.Name())
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template failed: %w: %s", err, stderr.String())
	}

	return parseRenderedChart(stdout.Bytes())
}

func parseRenderedChart(manifests []byte) (*RenderedChart, error) {
	rendered := &RenderedChart{}
	decoder := yaml.NewDecoder(bytes.NewReader(manifests))
	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode rendered manifest: %w", err)
		}
		switch doc["kind"] {
		case "ConfigMap":
			if data, ok := doc["data"].(map[string]any); ok {
				if config, ok := data["config.yaml"].(string); ok {
					rendered.ConfigYAML = config
				}
			}
		case "Deployment":
			rendered.TrustVolume, rendered.TrustMount = trustMaterial(doc)
		}
	}
	if rendered.ConfigYAML == "" {
		return nil, fmt.Errorf("rendered chart has no ConfigMap config.yaml")
	}
	return rendered, nil
}

func trustMaterial(deployment map[string]any) (volume, mount map[string]any) {
	spec := nested(deployment, "spec", "template", "spec")
	for _, item := range list(spec["volumes"]) {
		if item["name"] == "trust-material" {
			volume = item
		}
	}
	for _, container := range list(spec["containers"]) {
		for _, item := range list(container["volumeMounts"]) {
			if item["name"] == "trust-material" {
				mount = item
			}
		}
	}
	return volume, mount
}

func nested(root map[string]any, path ...string) map[string]any {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		current = next
	}
	return current
}

func list(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if entry, ok := item.(map[string]any); ok {
			result = append(result, entry)
		}
	}
	return result
}
