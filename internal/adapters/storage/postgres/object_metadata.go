package postgres

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

var errObjectMetadataColumn = errors.New("stored metadata is not a JSON object of strings")

// marshalObjectMetadata returns the labels and annotations columns. Empty maps
// are written as '{}', never as JSON null.
func marshalObjectMetadata(metadata model.ObjectMetadata) ([]byte, []byte, error) {
	labels, err := json.Marshal(metadata.Labels())
	if err != nil {
		return nil, nil, err
	}
	annotations, err := json.Marshal(metadata.Annotations())
	if err != nil {
		return nil, nil, err
	}
	return labels, annotations, nil
}

// restoreObjectMetadata rebuilds stored metadata without applying any write
// rule. A column that is not a JSON object of strings fails the read.
func restoreObjectMetadata(operation string, labels, annotations []byte) (model.ObjectMetadata, error) {
	decodedLabels, err := decodeMetadataColumn(labels)
	if err != nil {
		return model.ObjectMetadata{}, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "failed to decode stored labels")
	}
	decodedAnnotations, err := decodeMetadataColumn(annotations)
	if err != nil {
		return model.ObjectMetadata{}, storage.NewStorageError(operation, storage.ErrorKindUnknown, err, "failed to decode stored annotations")
	}
	return model.RestoreObjectMetadata(decodedLabels, decodedAnnotations), nil
}

func decodeMetadataColumn(column []byte) (map[string]string, error) {
	trimmed := bytes.TrimSpace(column)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errObjectMetadataColumn
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, errObjectMetadataColumn
	}
	decoded := make(map[string]string, len(raw))
	for key, value := range raw {
		if len(value) == 0 || value[0] != '"' {
			return nil, errObjectMetadataColumn
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return nil, errObjectMetadataColumn
		}
		decoded[key] = text
	}
	return decoded, nil
}
