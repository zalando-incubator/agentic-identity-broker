//go:build integration

package storage

import (
	"fmt"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestRefreshTokenSessionRepoConsent(t *testing.T) {
	pg, dbName, connStr, cleanup := setupCIMDKeyDomainDatabase(t, 36)
	defer cleanup()
	store, cleanupStore := newCIMDKeyDomainStorage(t, connStr)
	defer cleanupStore()
	runRefreshTokenSessionConsentContract(t, func(t *testing.T) (ports.RefreshTokenSessionRepository, id.AgentID, id.AgentID) {
		agentID, otherAgentID := id.NewAgentID(), id.NewAgentID()
		pg.ExecuteSQL(t, dbName, fmt.Sprintf(`INSERT INTO agents (id, display_name, description)
			VALUES ('%s', 'refresh agent', 'contract test'), ('%s', 'other agent', 'contract test')`, agentID, otherAgentID))
		return store.RefreshTokenSessions(), agentID, otherAgentID
	})
}
