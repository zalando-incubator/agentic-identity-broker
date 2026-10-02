package storage

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type RefreshRevocationReceipt struct {
	SessionID       id.RefreshSessionID
	AgentID         id.AgentID
	Principal       id.Principal
	ClientID        id.ClientID
	Reason          RefreshRevocationReason
	At              time.Time
	RedactedContext map[string]string
}
