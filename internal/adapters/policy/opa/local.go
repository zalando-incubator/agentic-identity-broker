package opa

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"path"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/bundle"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/fscontain"
)

const (
	maxLocalFiles = 1000
	maxLocalBytes = 16 << 20
	localBundle   = "local"
)

// loadLocal reads the bundle directory once through an os.Root over that directory and admits one
// candidate through the same capability check and admission as a managed revision. Nothing is
// polled: a change needs a restart.
func (s *source) loadLocal(dirPath string) error {
	const field = "policy.path"
	s.localMode = true
	close(s.sdkReady)
	dir, err := fscontain.OpenDir(dirPath)
	if err != nil {
		return &configError{field: field, reason: containmentReason(err)}
	}
	defer func() { _ = dir.Close() }()

	modules := map[string]string{}
	others := map[string][]byte{}
	unsupported := ""
	err = dir.Walk(maxLocalFiles, maxLocalBytes, func(name string) bool {
		if !localFileSupported(name) {
			if unsupported == "" {
				unsupported = name
			}
			return false
		}
		return true
	}, func(name string, data []byte) error {
		if strings.HasSuffix(name, ".rego") {
			modules["/"+name] = string(data)
		} else {
			others[name] = data
		}
		return nil
	})
	if err != nil {
		file := ""
		var containErr *fscontain.Error
		if asContain(err, &containErr) {
			file = containErr.Name
		}
		return &configError{field: field, reason: containmentReason(err), file: file}
	}
	if unsupported != "" {
		return &configError{field: field, reason: reasonUnsupportedFile, file: unsupported}
	}
	b, err := readLocalData(others)
	if err != nil {
		return &configError{field: field, reason: reasonInvalidDocument}
	}
	label := localBundle + "=" + b.Manifest.Revision
	s.telemetry.localMode(label, len(modules)+len(others))
	if !s.consider(context.Background(), candidateInput{
		modules:    modules,
		data:       b.Data,
		label:      label,
		configured: []string{localBundle},
		activated:  []string{localBundle},
	}) {
		close(s.final)
	}
	return nil
}

// localFileSupported accepts Rego modules, data documents, and the root manifest.
func localFileSupported(name string) bool {
	base := path.Base(name)
	switch {
	case strings.HasSuffix(base, ".rego"):
		return true
	case base == "data.json" || base == "data.yaml" || base == "data.yml":
		return true
	case name == ".manifest":
		return true
	}
	return false
}

// readLocalData reads the data documents and manifest with OPA's bundle reader from the bytes
// already read through the contained directory.
func readLocalData(files map[string][]byte) (bundle.Bundle, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "/" + name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return bundle.Bundle{}, err
		}
		if _, err := tw.Write(data); err != nil {
			return bundle.Bundle{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return bundle.Bundle{}, err
	}
	if err := gz.Close(); err != nil {
		return bundle.Bundle{}, err
	}
	return bundle.NewCustomReader(bundle.NewTarballLoaderWithBaseURL(&buf, "")).
		WithRegoVersion(ast.RegoV1).
		WithSizeLimitBytes(maxLocalBytes).
		Read()
}

func asContain(err error, target **fscontain.Error) bool {
	return errors.As(err, target)
}
