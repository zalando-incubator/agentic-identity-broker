package noop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestBranchKeyManagerCreate(t *testing.T) {
	manager := &BranchKeyManager{}

	tests := []struct {
		name    string
		subject domainencryption.BranchKeySubject
	}{
		{
			name:    "service subject",
			subject: domainencryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000")),
		},
		{
			name:    "signing key subject",
			subject: domainencryption.NewSigningKeyBranchKeySubject(id.NewKeyID("kid-123")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchKeyID, err := manager.Create(context.Background(), tt.subject)
			require.NoError(t, err)
			assert.Empty(t, branchKeyID)
		})
	}
}

func TestRefreshSessionBranchKeyRegistration(t *testing.T) {
	manager := &BranchKeyManager{}
	sessionID := id.NewRefreshSessionID()
	keyID, err := manager.Create(context.Background(), domainencryption.NewRefreshSessionBranchKeySubject(sessionID))
	require.NoError(t, err)
	require.Equal(t, "refresh_"+sessionID.String()+"_branch_key", keyID)
	keyID, err = manager.Create(context.Background(), domainencryption.NewRefreshSessionBranchKeySubject(id.RefreshSessionID{}))
	require.Error(t, err)
	require.Empty(t, keyID)
}
