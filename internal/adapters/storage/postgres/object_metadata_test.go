package postgres

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalObjectMetadataWritesEmptyObjects(t *testing.T) {
	labels, annotations, err := marshalObjectMetadata(model.ObjectMetadata{})
	require.NoError(t, err)
	assert.Equal(t, `{}`, string(labels))
	assert.Equal(t, `{}`, string(annotations))
}

func TestRestoreObjectMetadataColumns(t *testing.T) {
	t.Run("round trip without applying write rules", func(t *testing.T) {
		stored := model.RestoreObjectMetadata(map[string]string{"agenticidentitybroker.dev/x": "v", "bad key": "v"}, map[string]string{"note": "é"})
		labels, annotations, err := marshalObjectMetadata(stored)
		require.NoError(t, err)
		restored, err := restoreObjectMetadata("Read", labels, annotations)
		require.NoError(t, err)
		assert.True(t, stored.Equal(restored))
	})
	for _, column := range []string{``, `null`, `[]`, `"text"`, `1`, `{"k":1}`, `{"k":null}`, `{"k":{}}`} {
		t.Run("rejects "+column, func(t *testing.T) {
			_, err := restoreObjectMetadata("Read", []byte(column), []byte(`{}`))
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindUnknown, storageErr.Kind)
			_, err = restoreObjectMetadata("Read", []byte(`{}`), []byte(column))
			require.ErrorAs(t, err, &storageErr)
		})
	}
}
