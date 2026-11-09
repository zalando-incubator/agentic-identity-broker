package storage

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenSessionClientAuthMethodValidation(t *testing.T) {
	base := RefreshTokenSession{
		Signature: "signature",
		RequestID: "request",
		AgentID:   id.NewAgentID(),
		ClientID:  id.NewClientID("client"),
		Principal: id.NewPrincipal("user@example.com"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	for _, test := range []struct {
		name   string
		method *string
		valid  bool
	}{
		{name: "nil", valid: true},
		{name: "private key jwt", method: ptrString("private_key_jwt"), valid: true},
		{name: "unsupported method", method: ptrString("none")},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := base
			session.ClientAuthMethod = test.method
			err := session.Validate()
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func ptrString(value string) *string { return &value }
