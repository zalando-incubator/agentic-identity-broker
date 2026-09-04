package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/toolpattern"
)

// ApprovalStatus represents the lifecycle state of a tool approval.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusDenied   ApprovalStatus = "denied"
)

// IsValid returns true if the status is a known state.
func (s ApprovalStatus) IsValid() bool {
	switch s {
	case ApprovalStatusPending, ApprovalStatusApproved, ApprovalStatusDenied:
		return true
	}
	return false
}

// ApprovalPersistence represents the persistence scope of an approval decision.
type ApprovalPersistence string

const (
	ApprovalPersistenceOnce      ApprovalPersistence = "once"
	ApprovalPersistenceSession   ApprovalPersistence = "session"
	ApprovalPersistencePermanent ApprovalPersistence = "permanent"
)

// IsValid returns true if the persistence scope is a known value.
func (p ApprovalPersistence) IsValid() bool {
	switch p {
	case ApprovalPersistenceOnce, ApprovalPersistenceSession, ApprovalPersistencePermanent:
		return true
	}
	return false
}

// ApprovalDecision is the persisted outcome of a user approving a tool call.
type ApprovalDecision struct {
	Persistence   ApprovalPersistence
	ToolPattern   string
	ParamsPattern map[string]string
}

// Approval domain errors.
var (
	ErrApprovalNotPending        = errors.New("approval is not in pending state")
	ErrApprovalExpired           = errors.New("approval has expired")
	ErrApprovalPrincipalMismatch = errors.New("acting principal does not match approval principal")
	ErrApprovalNotApproved       = errors.New("approval is not in approved state")
	ErrApprovalNotOnce           = errors.New("only once-persistence approvals can be consumed")
	ErrApprovalAlreadyConsumed   = errors.New("approval has already been consumed")
	ErrApprovalPatternMissing    = errors.New("approval is missing its tool pattern")
)

// ToolApproval represents a pending or resolved human-in-the-loop tool call approval.
// Aggregate root with full lifecycle invariants.
type ToolApproval struct {
	ID                       id.ApprovalID        `db:"id" json:"id"`
	Principal                id.Principal         `db:"principal" json:"principal"`
	AgentID                  id.AgentID           `db:"agent_id" json:"agent_id"`
	GatewayClientID          string               `db:"gateway_client_id" json:"-"`
	ToolName                 string               `db:"tool_name" json:"tool_name"`
	Arguments                map[string]any       `db:"arguments" json:"arguments"`
	ArgumentsHash            string               `db:"arguments_hash" json:"-"`
	ToolPattern              string               `db:"tool_pattern" json:"-"`
	ParamsPattern            map[string]string    `db:"params_pattern" json:"-"`
	Description              string               `db:"description" json:"description"`
	RiskLevel                string               `db:"risk_level" json:"risk_level"`
	MCPSessionID             *string              `db:"mcp_session_id" json:"-"`
	AgentSessionID           *string              `db:"agent_session_id" json:"-"`
	ToolInvocationID         *string              `db:"tool_invocation_id" json:"-"`
	OpenTelemetryTraceparent *string              `db:"opentelemetry_traceparent" json:"-"`
	Status                   ApprovalStatus       `db:"status" json:"status"`
	Persistence              *ApprovalPersistence `db:"persistence" json:"persistence"`
	Consumed                 bool                 `db:"consumed" json:"consumed"`
	ApprovalURL              string               `db:"approval_url" json:"approval_url"`
	CreatedAt                time.Time            `db:"created_at" json:"created_at"`
	ApprovedAt               *time.Time           `db:"approved_at" json:"approved_at,omitempty"`
	DeniedAt                 *time.Time           `db:"denied_at" json:"denied_at,omitempty"`
	ConsumedAt               *time.Time           `db:"consumed_at" json:"consumed_at,omitempty"`
	ExpiresAt                time.Time            `db:"expires_at" json:"expires_at"`
}

// IsExpired returns true if the approval has passed its TTL.
func (a *ToolApproval) IsExpired(now time.Time) bool {
	return now.After(a.ExpiresAt)
}

// IsActionable returns true if the approval can still be approved or denied.
func (a *ToolApproval) IsActionable(now time.Time) bool {
	return a.Status == ApprovalStatusPending && !a.IsExpired(now)
}

// Approve transitions a pending approval to approved state.
// Returns error if not pending, expired, or principal mismatch.
func (a *ToolApproval) Approve(actingPrincipal id.Principal, decision ApprovalDecision, now time.Time) error {
	if a.Principal != actingPrincipal {
		return ErrApprovalPrincipalMismatch
	}
	if a.Status != ApprovalStatusPending {
		return ErrApprovalNotPending
	}
	if a.IsExpired(now) {
		return ErrApprovalExpired
	}

	a.Status = ApprovalStatusApproved
	a.Persistence = &decision.Persistence
	a.ToolPattern = decision.ToolPattern
	a.ParamsPattern = decision.ParamsPattern
	a.ApprovedAt = &now
	return nil
}

// ApplyExactPatterns sets the pattern fields to the exact coverage of this approval's own tool
// name and arguments.
func (a *ToolApproval) ApplyExactPatterns() {
	a.ToolPattern = a.ToolName
	a.ParamsPattern = toolpattern.ExactParams(a.Arguments)
}

// Deny transitions a pending approval to denied state.
// Returns error if not pending, expired, or principal mismatch.
func (a *ToolApproval) Deny(actingPrincipal id.Principal, persistence *ApprovalPersistence, now time.Time) error {
	if a.Principal != actingPrincipal {
		return ErrApprovalPrincipalMismatch
	}
	if a.Status != ApprovalStatusPending {
		return ErrApprovalNotPending
	}
	if a.IsExpired(now) {
		return ErrApprovalExpired
	}

	a.Status = ApprovalStatusDenied
	a.Persistence = persistence
	a.DeniedAt = &now
	return nil
}

// Consume marks a once-persistence approved approval as consumed.
// Returns error if not approved, not once-persistence, or already consumed.
func (a *ToolApproval) Consume(now time.Time) error {
	if a.Status != ApprovalStatusApproved {
		return ErrApprovalNotApproved
	}
	if a.Persistence == nil || *a.Persistence != ApprovalPersistenceOnce {
		return ErrApprovalNotOnce
	}
	if a.Consumed {
		return nil // Idempotent
	}

	a.Consumed = true
	a.ConsumedAt = &now
	return nil
}

// ComputeArgumentsHash computes a deterministic SHA-256 hash of the arguments map.
// Go's json.Marshal sorts map keys alphabetically, ensuring deterministic output.
func ComputeArgumentsHash(arguments map[string]any) string {
	canonical, err := json.Marshal(arguments)
	if err != nil {
		// Fallback to empty hash for non-serializable arguments
		canonical = []byte("{}")
	}
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:])
}
