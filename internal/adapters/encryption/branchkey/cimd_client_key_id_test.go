package branchkey

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestCIMDClientAuthenticationBranchKeyID(t *testing.T) {
	subject := domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(id.NewKeyID("cimd-kid-123"))

	branchKeyID, err := GenerateBranchKeyId(subject)
	require.NoError(t, err)
	assert.Equal(t, "cimd_key_cimd-kid-123_branch_key", branchKeyID)

	extracted, err := ExtractSubject(branchKeyID)
	require.NoError(t, err)
	assert.Equal(t, domainencryption.BranchKeySubjectKindCIMDClientAuthenticationKey, extracted.Kind())
	assert.Equal(t, "cimd-kid-123", extracted.Identifier())
}
