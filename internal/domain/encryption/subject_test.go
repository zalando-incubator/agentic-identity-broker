package encryption

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestBranchKeySubject_Service(t *testing.T) {
	subject := NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"))

	require.NoError(t, subject.Validate())
	assert.Equal(t, BranchKeySubjectKindService, subject.Kind())
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", subject.Identifier())
	assert.Equal(t, map[string]string{ContextKeyServiceID: "550e8400-e29b-41d4-a716-446655440000"}, subject.EncryptionContext())

	serviceID, ok := subject.ServiceID()
	require.True(t, ok)
	assert.Equal(t, id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"), serviceID)

	_, ok = subject.KeyID()
	assert.False(t, ok)
}

func TestBranchKeySubject_SigningKey(t *testing.T) {
	subject := NewSigningKeyBranchKeySubject(id.NewKeyID("kid-123"))

	require.NoError(t, subject.Validate())
	assert.Equal(t, BranchKeySubjectKindSigningKey, subject.Kind())
	assert.Equal(t, "kid-123", subject.Identifier())
	assert.Equal(t, map[string]string{ContextKeyKID: "kid-123"}, subject.EncryptionContext())

	signingKeyID, ok := subject.KeyID()
	require.True(t, ok)
	assert.Equal(t, id.NewKeyID("kid-123"), signingKeyID)

	_, ok = subject.ServiceID()
	assert.False(t, ok)
}

func TestBranchKeySubject_CIMDClientAuthenticationKey(t *testing.T) {
	subject := NewCIMDClientAuthenticationKeyBranchKeySubject(id.NewKeyID("cimd-kid-123"))

	require.NoError(t, subject.Validate())
	assert.Equal(t, BranchKeySubjectKindCIMDClientAuthenticationKey, subject.Kind())
	assert.Equal(t, "cimd-kid-123", subject.Identifier())

	encryptionContext := subject.EncryptionContext()
	require.Len(t, encryptionContext, 1, "the CIMD key AAD must have exactly one subject")
	assert.Equal(t, map[string]string{ContextKeyKID: "cimd-kid-123"}, encryptionContext)

	keyID, ok := subject.KeyID()
	require.True(t, ok)
	assert.Equal(t, id.NewKeyID("cimd-kid-123"), keyID)

	_, ok = subject.ServiceID()
	assert.False(t, ok)
}

func TestBranchKeySubject_IdentifierZeroValue(t *testing.T) {
	assert.Equal(t, "<unknown>", BranchKeySubject{}.Identifier())
}

func TestBranchKeySubject_EncryptionContextZeroValue(t *testing.T) {
	assert.Equal(t, map[string]string{}, BranchKeySubject{}.EncryptionContext())
}

func TestBranchKeySubjectFromEncryptionContext(t *testing.T) {
	tests := []struct {
		name              string
		encryptionContext map[string]string
		assertSubject     func(t *testing.T, subject BranchKeySubject)
		wantErr           string
	}{
		{
			name:              "service subject",
			encryptionContext: map[string]string{ContextKeyServiceID: "550e8400-e29b-41d4-a716-446655440000"},
			assertSubject: func(t *testing.T, subject BranchKeySubject) {
				t.Helper()
				assert.Equal(t, BranchKeySubjectKindService, subject.Kind())
				serviceID, ok := subject.ServiceID()
				require.True(t, ok)
				assert.Equal(t, id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"), serviceID)
			},
		},
		{
			name:              "signing key subject",
			encryptionContext: map[string]string{ContextKeyKID: "kid-123"},
			assertSubject: func(t *testing.T, subject BranchKeySubject) {
				t.Helper()
				assert.Equal(t, BranchKeySubjectKindSigningKey, subject.Kind())
				signingKeyID, ok := subject.KeyID()
				require.True(t, ok)
				assert.Equal(t, id.NewKeyID("kid-123"), signingKeyID)
			},
		},
		{
			name:              "missing subject",
			encryptionContext: map[string]string{},
			wantErr:           "missing branch key subject",
		},
		{
			name:              "ambiguous subject",
			encryptionContext: map[string]string{ContextKeyServiceID: "550e8400-e29b-41d4-a716-446655440000", ContextKeyKID: "kid-123"},
			wantErr:           "exactly one branch key subject",
		},
		{
			name:              "invalid service id",
			encryptionContext: map[string]string{ContextKeyServiceID: "not-a-uuid"},
			wantErr:           "invalid service_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, err := BranchKeySubjectFromEncryptionContext(tt.encryptionContext)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			tt.assertSubject(t, subject)
		})
	}
}

func TestBranchKeySubjectFromEncryptionContextCIMDClientAuthenticationKey(t *testing.T) {
	subject, err := BranchKeySubjectFromEncryptionContext(map[string]string{ContextKeyKID: "cimd-kid-123"})
	require.NoError(t, err)
	assert.Equal(t, BranchKeySubjectKindCIMDClientAuthenticationKey, subject.Kind())
	keyID, ok := subject.KeyID()
	require.True(t, ok)
	assert.Equal(t, id.NewKeyID("cimd-kid-123"), keyID)
}

func TestBranchKeySubjectValidate(t *testing.T) {
	tests := []struct {
		name    string
		subject BranchKeySubject
		wantErr string
	}{
		{
			name:    "missing kind",
			subject: BranchKeySubject{},
			wantErr: "branch key subject kind is required",
		},
		{
			name:    "missing service id",
			subject: NewServiceBranchKeySubject(id.ServiceID{}),
			wantErr: "service_id is required",
		},
		{
			name:    "missing signing key id",
			subject: NewSigningKeyBranchKeySubject(id.KeyID("")),
			wantErr: "kid is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.subject.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
