package storage

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// RefreshSessionAuditIdentity is the non-credential ownership projection used
// to report committed transitions even after an agent's roots are cascaded.
type RefreshSessionAuditIdentity struct {
	ID        id.RefreshSessionID
	AgentID   id.AgentID
	Principal id.Principal
	ClientID  id.ClientID
}

// AuditIdentity projects immutable ownership without retry ciphertext or token signatures.
func (s *RefreshSession) AuditIdentity() RefreshSessionAuditIdentity {
	return RefreshSessionAuditIdentity{ID: s.ID, AgentID: s.AgentID, Principal: s.Principal, ClientID: s.ClientID}
}

type RefreshRevocationReceipt struct {
	SessionID       id.RefreshSessionID
	AgentID         id.AgentID
	Principal       id.Principal
	ClientID        id.ClientID
	Reason          RefreshRevocationReason
	At              time.Time
	RedactedContext map[string]string
}
