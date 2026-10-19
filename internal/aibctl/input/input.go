// Package input reads request documents from a file or stdin in JSON or YAML.
package input

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

// ReadDocument returns the document at path ("-" for stdin) as JSON bytes.
// YAML input is converted so that typed decoding uses JSON tags only.
func ReadDocument(path string, stdin io.Reader) ([]byte, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path) // #nosec G304 -- user-supplied input document.
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ToJSON(data)
}

// ToJSON normalizes JSON or YAML input to JSON.
func ToJSON(data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("input document is empty")
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		if !json.Valid(trimmed) {
			return nil, fmt.Errorf("input document is not valid JSON")
		}
		return trimmed, nil
	}
	var generic any
	if err := yaml.Unmarshal(trimmed, &generic); err != nil {
		return nil, fmt.Errorf("parse YAML input: %w", err)
	}
	out, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("convert YAML input: %w", err)
	}
	return out, nil
}

// readOnlyKeys are server-managed top-level fields that "get" output carries
// and that write bodies never accept. They are dropped so that
// "get -o yaml | update -f -" round-trips.
var readOnlyKeys = []string{"id", "created_at", "updated_at"}

// Decode reads the document and unmarshals it into out, rejecting unknown
// fields other than the server-managed read-only ones.
func Decode(path string, stdin io.Reader, out any) error {
	data, err := ReadDocument(path, stdin)
	if err != nil {
		return err
	}
	data, err = stripReadOnly(data)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func stripReadOnly(data []byte) ([]byte, error) {
	if bytes.TrimSpace(data)[0] != '{' {
		return data, nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	for _, key := range readOnlyKeys {
		delete(doc, key)
	}
	return json.Marshal(doc)
}
