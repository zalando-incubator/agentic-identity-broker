package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientAssertionReplayValidate(t *testing.T) {
	valid := ClientAssertionReplay{
		ClientID:  "https://client.example/metadata.json",
		JTI:       "assertion-1",
		ExpiresAt: time.Date(2026, 10, 16, 12, 5, 30, 0, time.UTC),
	}
	tests := []struct {
		name   string
		record ClientAssertionReplay
		valid  bool
	}{
		{name: "valid", record: valid, valid: true},
		{name: "client id is required", record: ClientAssertionReplay{JTI: valid.JTI, ExpiresAt: valid.ExpiresAt}},
		{name: "jti is required", record: ClientAssertionReplay{ClientID: valid.ClientID, ExpiresAt: valid.ExpiresAt}},
		{name: "expiry is required", record: ClientAssertionReplay{ClientID: valid.ClientID, JTI: valid.JTI}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.record.Validate()
			if test.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var storageErr *StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, ErrorKindValidation, storageErr.Kind)
		})
	}
}
