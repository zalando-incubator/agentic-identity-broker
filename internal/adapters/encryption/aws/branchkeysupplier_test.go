package aws

import (
	"fmt"
	"testing"

	mpltypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygeneratedtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/branchkey"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
)

type fakeBranchKeyIDProvider struct {
	generatedID string
	lastSubject domainencryption.BranchKeySubject
}

func (f *fakeBranchKeyIDProvider) GenerateBranchKeyId(subject domainencryption.BranchKeySubject) (string, error) {
	f.lastSubject = subject
	return f.generatedID, nil
}

func (f *fakeBranchKeyIDProvider) ExtractSubjectFromBranchKey(branchKeyID string) (domainencryption.BranchKeySubject, error) {
	return domainencryption.BranchKeySubject{}, fmt.Errorf("unexpected ExtractSubjectFromBranchKey call for %q", branchKeyID)
}

func TestBranchKeyIdSupplier_GetBranchKeyId(t *testing.T) {
	supplier := NewBranchKeyIdSupplier(branchkey.NewDefaultProvider())

	tests := []struct {
		name              string
		encryptionContext map[string]string
		wantID            string
		wantErr           string
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
			name:              "missing subject",
			encryptionContext: map[string]string{},
			wantErr:           "missing branch key subject",
		},
		{
			name:              "ambiguous subject",
			encryptionContext: map[string]string{"service_id": "550e8400-e29b-41d4-a716-446655440000", "kid": "kid-123"},
			wantErr:           "exactly one branch key subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := supplier.GetBranchKeyId(mpltypes.GetBranchKeyIdInput{EncryptionContext: tt.encryptionContext})
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, output)
			assert.Equal(t, tt.wantID, output.BranchKeyId)
		})
	}
}

func TestBranchKeyIdSupplier_GetBranchKeyId_UsesInjectedProvider(t *testing.T) {
	provider := &fakeBranchKeyIDProvider{generatedID: "custom-branch-key-id"}
	supplier := NewBranchKeyIdSupplier(provider)

	output, err := supplier.GetBranchKeyId(mpltypes.GetBranchKeyIdInput{
		EncryptionContext: map[string]string{"kid": "kid-123"},
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "custom-branch-key-id", output.BranchKeyId)
	assert.Equal(t, domainencryption.BranchKeySubjectKindSigningKey, provider.lastSubject.Kind())
	keyID, ok := provider.lastSubject.KeyID()
	require.True(t, ok)
	assert.Equal(t, "kid-123", keyID.String())
}
