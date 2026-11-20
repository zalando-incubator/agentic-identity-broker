package model

import (
	"encoding/json"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderClientIdentityCopy(t *testing.T) {
	for _, identity := range []*id.ClientID{nil, new(id.ClientID("reports-client-v1"))} {
		entity := &ThirdpartyOAuth2ProviderEntity{ClientIdentity: identity, ClientIdentityRevision: 7}
		copied := entity.Copy()
		require.Equal(t, entity, copied)
		if identity != nil {
			*copied.ClientIdentity = "other"
			assert.Equal(t, id.ClientID("reports-client-v1"), *entity.ClientIdentity)
		}
		body, err := json.Marshal(entity)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "ClientIdentity")
		assert.NotContains(t, string(body), "client_identity")
	}
}

func TestProviderClientIdentityValidation(t *testing.T) {
	for _, test := range []struct {
		name     string
		identity *id.ClientID
		revision int64
		valid    bool
	}{
		{name: "legacy", valid: true},
		{name: "recorded", identity: new(id.ClientID("reports-client-v1")), revision: 7, valid: true},
		{name: "negative revision", revision: -1},
		{name: "empty identity", identity: new(id.ClientID(""))},
	} {
		t.Run(test.name, func(t *testing.T) {
			entity := &ThirdpartyOAuth2ProviderEntity{ID: id.NewServiceID(), DisplayName: "Reports", ClientID: "reports", Secret: NewEncryptedSecret([]byte("ciphertext")), IssuerURI: "https://reports.example.com", ClientIdentity: test.identity, ClientIdentityRevision: test.revision}
			if test.valid {
				require.NoError(t, entity.Validate())
			} else {
				require.Error(t, entity.Validate())
			}
		})
	}
}
