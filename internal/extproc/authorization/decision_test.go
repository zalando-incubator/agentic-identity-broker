package authorization_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
)

func TestActionConstants(t *testing.T) {
	assert.Equal(t, "allow", authorization.ActionAllow)
	assert.Equal(t, "deny", authorization.ActionDeny)
	assert.Equal(t, "approval_required", authorization.ActionApprovalRequired)
	assert.Equal(t, "ciba_required", authorization.ActionCIBARequired)
}

func TestParseDecision_Allow(t *testing.T) {
	result := map[string]any{"action": authorization.ActionAllow}
	d := authorization.ParseDecision(result)
	assert.Equal(t, authorization.ActionAllow, d.Action)
	assert.Empty(t, d.Reasons)
}

func TestParseDecision_DenyWithReasons(t *testing.T) {
	result := map[string]any{
		"action":  authorization.ActionDeny,
		"reasons": []any{"r1", "r2"},
	}
	d := authorization.ParseDecision(result)
	assert.Equal(t, authorization.ActionDeny, d.Action)
	assert.Equal(t, []string{"r1", "r2"}, d.Reasons)
}

func TestParseDecision_ApprovalRequired(t *testing.T) {
	result := map[string]any{"action": authorization.ActionApprovalRequired, "approval_context": map[string]any{"description": "review this", "risk_level": "medium"}}
	d := authorization.ParseDecision(result)
	assert.Equal(t, authorization.ActionApprovalRequired, d.Action)
	assert.Equal(t, &authorization.ApprovalContext{Description: "review this", RiskLevel: "medium"}, d.ApprovalContext)
}

func TestParseDecision_CIBARequired_MappedToDeny(t *testing.T) {
	result := map[string]any{"action": authorization.ActionCIBARequired}
	d := authorization.ParseDecision(result)
	assert.Equal(t, authorization.ActionDeny, d.Action)
	assert.Equal(t, []string{"ciba_required is not yet supported"}, d.Reasons)
}

func TestParseDecision_NilResult_Deny(t *testing.T) {
	d := authorization.ParseDecision(nil)
	assert.Equal(t, "deny", d.Action)
}

func TestParseDecision_EmptyResult_Deny(t *testing.T) {
	d := authorization.ParseDecision(map[string]any{})
	assert.Equal(t, "deny", d.Action)
}
