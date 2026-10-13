package id

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventIDGeneration(t *testing.T) {
	eventID := NewBusinessEventID()
	require.Equal(t, uuid.Version(7), uuid.UUID(eventID).Version())
	require.Equal(t, uuid.RFC4122, uuid.UUID(eventID).Variant())
	encoded, err := json.Marshal(eventID)
	require.NoError(t, err)
	var decoded BusinessEventID
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, eventID, decoded)
	value, err := eventID.Value()
	require.NoError(t, err)
	var scanned BusinessEventID
	require.NoError(t, scanned.Scan(value))
	require.Equal(t, eventID, scanned)
}

func TestOtherEntityIDsRemainUUIDv4(t *testing.T) {
	constructors := map[string]func() uuid.UUID{
		"agent":              func() uuid.UUID { return uuid.UUID(NewAgentID()) },
		"approval":           func() uuid.UUID { return uuid.UUID(NewApprovalID()) },
		"service":            func() uuid.UUID { return uuid.UUID(NewServiceID()) },
		"grant":              func() uuid.UUID { return uuid.UUID(NewGrantID()) },
		"session":            func() uuid.UUID { return uuid.UUID(NewSessionID()) },
		"user":               func() uuid.UUID { return uuid.UUID(NewUserID()) },
		"permission set":     func() uuid.UUID { return uuid.UUID(NewPermissionSetID()) },
		"credential":         func() uuid.UUID { return uuid.UUID(NewCredentialID()) },
		"signing key":        func() uuid.UUID { return uuid.UUID(NewSigningKeyID()) },
		"authorization code": func() uuid.UUID { return uuid.UUID(NewAuthorizationCodeID()) },
	}
	for name, create := range constructors {
		t.Run(name, func(t *testing.T) { require.Equal(t, uuid.Version(4), create().Version()) })
	}
}
