package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestServiceScopeBindingsColumn(t *testing.T) {
	serviceID := id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	original := serviceScopeBindingsColumn{"https://broker.example.com/sessions/github": serviceID}
	value, err := original.Value()
	require.NoError(t, err)
	assert.JSONEq(t, `{"https://broker.example.com/sessions/github":"880e8400-e29b-41d4-a716-446655440010"}`, string(value.([]byte)))

	var decoded serviceScopeBindingsColumn
	require.NoError(t, decoded.Scan(value))
	assert.Equal(t, original, decoded)
	require.NoError(t, decoded.Scan(string(value.([]byte))))
	assert.Equal(t, original, decoded)

	value, err = serviceScopeBindingsColumn(nil).Value()
	require.NoError(t, err)
	assert.Nil(t, value)
	require.NoError(t, decoded.Scan(nil))
	assert.Nil(t, decoded)

	assert.Error(t, decoded.Scan([]byte(`{"https://broker.example.com/sessions/github":"not-a-uuid"}`)))
	assert.Error(t, decoded.Scan(42))
}
