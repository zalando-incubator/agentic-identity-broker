package model

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type BusinessEventOutcome string

const (
	BusinessEventSuccess BusinessEventOutcome = "success"
	BusinessEventFailure BusinessEventOutcome = "failure"
	BusinessEventDenied  BusinessEventOutcome = "denied"
	BusinessEventPending BusinessEventOutcome = "pending"
)

type BusinessEventActor struct {
	Kind       string        `json:"kind"`
	ID         *string       `json:"id"`
	OnBehalfOf *id.Principal `json:"on_behalf_of"`
}

type BusinessEventClient struct {
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

type BusinessEvent struct {
	ID               id.BusinessEventID   `json:"id"`
	Type             string               `json:"type"`
	Source           string               `json:"source"`
	OccurredAt       time.Time            `json:"occurred_at"`
	RecordedAt       time.Time            `json:"recorded_at"`
	Subject          *id.Principal        `json:"subject"`
	Actor            BusinessEventActor   `json:"actor"`
	AgentID          id.AgentID           `json:"agent_id,omitempty"`
	GatewayClientID  id.ClientID          `json:"gateway_client_id,omitempty"`
	ServiceID        id.ServiceID         `json:"service_id,omitempty"`
	PermissionSetIDs []id.PermissionSetID `json:"permission_set_ids,omitempty"`
	GrantID          id.GrantID           `json:"grant_id,omitempty"`
	SessionID        id.SessionID         `json:"session_id,omitempty"`
	ApprovalID       id.ApprovalID        `json:"approval_id,omitempty"`
	MCPSessionID     string               `json:"mcp_session_id,omitempty"`
	AgentSessionID   string               `json:"agent_session_id,omitempty"`
	Outcome          BusinessEventOutcome `json:"outcome"`
	ReasonUser       string               `json:"reason_user"`
	ReasonAdmin      string               `json:"reason_admin"`
	TraceID          string               `json:"trace_id,omitempty"`
	SpanID           string               `json:"span_id,omitempty"`
	Client           *BusinessEventClient `json:"client,omitempty"`
	Data             map[string]any       `json:"data"`
}

type BusinessEventKey struct {
	RecordedAt time.Time
	ID         id.BusinessEventID
}

type BusinessEventSubject struct {
	Principal id.Principal
	NoSubject bool
}

type BusinessEventCursor struct {
	OccurredAt time.Time
	ID         id.BusinessEventID
}

type BusinessEventQuery struct {
	Subject BusinessEventSubject
	Type    string
	Outcome BusinessEventOutcome
	Start   time.Time
	End     time.Time
	After   *BusinessEventCursor
	Limit   int
}
