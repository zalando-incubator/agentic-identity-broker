package oauth2_sessions

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Detail contract from specs/055-incremental-service-authorization (API-SESSION-DETAIL).
func TestServiceDetailDataContract(t *testing.T) {
	serviceID := id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	agentID := id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000")
	canonical := "github"
	detail := &oauth2session.ServiceSessionDetail{
		Service:         &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, DisplayName: "GitHub Production"},
		ScopeURL:        "https://broker.example.com/sessions/github",
		DependentAgents: []oauth2session.AgentInfo{{ID: agentID, DisplayName: "Data Analysis Assistant"}},
	}
	encoded, err := json.Marshal(toServiceDetailData(detail))
	require.NoError(t, err)
	assert.JSONEq(t, `{"service":{"id":"880e8400-e29b-41d4-a716-446655440010","canonical_id":"github","display_name":"GitHub Production","scope_url":"https://broker.example.com/sessions/github"},"session":null,"dependent_agents":[{"agent_id":"550e8400-e29b-41d4-a716-446655440000","display_name":"Data Analysis Assistant"}]}`, string(encoded))

	expired := time.Now().Add(-time.Minute)
	detail.Session = &storage.UserSession{ID: id.NewSessionID(), EncryptedRefreshToken: []byte("sealed"), AccessTokenExpiresAt: &expired, RefreshTokenExpiresAt: &expired, Scope: []string{"repo"}}
	var decoded struct {
		Session map[string]any `json:"session"`
	}
	encoded, err = json.Marshal(toServiceDetailData(detail))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, true, decoded.Session["is_expired"], "a persisted rejection is unusable")
	assert.Equal(t, true, decoded.Session["has_refresh_token"])
	assert.Equal(t, true, decoded.Session["tokens_encrypted"])
	assert.Equal(t, float64(1), decoded.Session["dependent_agent_count"])
}
