package aws

import (
	"testing"

	mpltypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygeneratedtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/branchkey"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
)

func TestBranchKeyIdSupplier_GetBranchKeyId(t *testing.T) {
	supplier := NewBranchKeyIdSupplier(branchkey.NewDefaultProvider())

	tests := []struct {
		name              string
		encryptionContext map[string]string
		wantID            string
		wantErr           bool
	}{
		{
			name:              "service subject keeps existing naming",
			encryptionContext: map[string]string{"service_id": "550e8400-e29b-41d4-a716-446655440000"},
			wantID:            "service_550e8400-e29b-41d4-a716-446655440000_branch_key",
		},
		{
			name:              "signing key subject uses kid namespace",
			encryptionContext: map[string]string{"kid": "kid-123"},
			wantID:            "key_kid-123_branch_key",
		},
		{
			name:              "CIMD client-authentication key subject uses CIMD namespace",
			encryptionContext: map[string]string{"kid": "cimd-kid-123"},
			wantID:            "cimd_key_cimd-kid-123_branch_key",
		},
		{
			name:              "refresh session subject has its own namespace",
			encryptionContext: map[string]string{domainencryption.ContextKeyRefreshSessionID: "550e8400-e29b-41d4-a716-446655440001"},
			wantID:            "refresh_550e8400-e29b-41d4-a716-446655440001_branch_key",
		},
		{
			name:              "missing subject",
			encryptionContext: map[string]string{},
			wantErr:           true,
		},
		{
			name:              "ambiguous subject",
			encryptionContext: map[string]string{"service_id": "550e8400-e29b-41d4-a716-446655440000", "kid": "kid-123"},
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := supplier.GetBranchKeyId(mpltypes.GetBranchKeyIdInput{EncryptionContext: tt.encryptionContext})
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, output)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, output)
			assert.Equal(t, tt.wantID, output.BranchKeyId)
		})
	}
}

func TestBranchKeyIdSupplier_RejectsInvalidRefreshSessionContexts(t *testing.T) {
	supplier := NewBranchKeyIdSupplier(branchkey.NewDefaultProvider())

	tests := []struct {
		name    string
		context map[string]string
	}{
		{"malformed session ID", map[string]string{domainencryption.ContextKeyRefreshSessionID: "not-a-uuid"}},
		{"empty session ID", map[string]string{domainencryption.ContextKeyRefreshSessionID: ""}},
		{"zero session ID", map[string]string{domainencryption.ContextKeyRefreshSessionID: "00000000-0000-0000-0000-000000000000"}},
		{"session and service subjects", map[string]string{
			domainencryption.ContextKeyRefreshSessionID: "550e8400-e29b-41d4-a716-446655440001",
			domainencryption.ContextKeyServiceID:        "550e8400-e29b-41d4-a716-446655440000",
		}},
		{"session and signing key subjects", map[string]string{
			domainencryption.ContextKeyRefreshSessionID: "550e8400-e29b-41d4-a716-446655440001",
			domainencryption.ContextKeyKID:              "kid-123",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := supplier.GetBranchKeyId(mpltypes.GetBranchKeyIdInput{EncryptionContext: tt.context})
			require.Error(t, err)
			assert.Nil(t, output)
		})
	}
}
