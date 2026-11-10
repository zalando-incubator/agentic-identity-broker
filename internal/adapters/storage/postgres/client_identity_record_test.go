package postgres

import (
	"context"
	"encoding/json"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderClientIdentityMapping(t *testing.T) {
	entity := newTestEntity()
	entity.ClientIdentity = new(id.ClientID("reports"))
	entity.ClientIdentityRevision = 7
	record, err := entityToRecord(entity)
	require.NoError(t, err)
	require.Equal(t, new("reports"), record.ClientIdentity)
	require.Equal(t, int64(7), record.ClientIdentityRevision)
	*entity.ClientIdentity = "mutated"
	found, err := recordToEntity(record)
	require.NoError(t, err)
	assert.Equal(t, new(id.ClientID("reports")), found.ClientIdentity)
	assert.Equal(t, int64(7), found.ClientIdentityRevision)
	*found.ClientIdentity = "also mutated"
	assert.Equal(t, "reports", *record.ClientIdentity)
}

func TestSessionClientRevisionStorageValidation(t *testing.T) {
	session := &storage.UserSession{ID: id.NewSessionID(), Principal: "alice@example.com", ServiceID: id.NewServiceID(), UpstreamClientRevision: -1, TokenType: "Bearer", EncryptedAccessToken: []byte("ciphertext")}
	err := NewUserSessionRepository(&Adapter{}).Create(context.Background(), session)
	var validation *storage.StorageError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, storage.ErrorKindValidation, validation.Kind)
}

func TestSessionClientRevisionMapping(t *testing.T) {
	record := &userSessionRecord{UpstreamClientID: new("reports"), UpstreamClientRevision: 7}
	session := recordToSession(record)
	assert.Equal(t, int64(7), session.UpstreamClientRevision)
	body, err := json.Marshal(session)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "upstream_client")
	*session.UpstreamClientID = "mutated"
	assert.Equal(t, "reports", *record.UpstreamClientID)
}
