package storage

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type RefreshRevocationReason string

const (
	RefreshReasonGrantDeleted        RefreshRevocationReason = "grant_deleted"
	RefreshReasonExpiredGrantRenewal RefreshRevocationReason = "expired_grant_renewal"
	RefreshReasonAgentDeleted        RefreshRevocationReason = "agent_deleted"
	RefreshReasonCredentialRevoked   RefreshRevocationReason = "credential_revoked" // #nosec G101 -- Public reason enum, not a credential.
	RefreshReasonCodeReplay          RefreshRevocationReason = "code_replay"
	RefreshReasonProhibitedReuse     RefreshRevocationReason = "prohibited_reuse"
	RefreshReasonAbsoluteExpiry      RefreshRevocationReason = "absolute_expiry"
	RefreshReasonInactivityExpiry    RefreshRevocationReason = "inactivity_expiry"
	RefreshReasonRestoreInvalidation RefreshRevocationReason = "restore_invalidation"
)

type RefreshSessionPolicy struct {
	ReuseInterval      time.Duration
	AbsoluteLifetime   time.Duration
	InactivityLifetime time.Duration
}

type RefreshSession struct {
	ID                                id.RefreshSessionID
	OriginalGrantID                   id.GrantID
	OriginalTokenSignature            string
	AgentID                           id.AgentID
	Principal                         id.Principal
	ClientID                          id.ClientID
	Scope                             string
	Email                             *string
	DisplayName                       string
	StartedAt                         time.Time
	LastFreshAt                       time.Time
	AbsoluteExpiresAt                 *time.Time
	InactivityExpiresAt               time.Time
	RetainUntil                       time.Time
	BranchKeyID                       string
	CurrentSignature                  string
	PreviousSignature                 *string
	PreviousConsumedAt                *time.Time
	ReuseUntil                        *time.Time
	OriginalRequestedScope            *string
	OriginalRequestContextFingerprint *string
	RetryCiphertext                   []byte
	RetryAccessExpiresAt              *time.Time
	RetryExpiresAt                    *time.Time
	RetryCount                        int
	RevokedAt                         *time.Time
	ExpiredAt                         *time.Time
	TerminalReason                    *RefreshRevocationReason
}

func (s *RefreshSession) Validate() error {
	if s == nil || s.ID.IsZero() || s.OriginalGrantID.IsZero() || s.AgentID.IsZero() || s.Principal.IsZero() || s.ClientID.IsZero() {
		return errors.New("refresh session requires immutable origin and ownership")
	}
	if !validRefreshDigest(s.OriginalTokenSignature) || !validRefreshDigest(s.CurrentSignature) {
		return errors.New("refresh session requires canonical token signatures")
	}
	if s.BranchKeyID != "refresh_"+s.ID.String()+"_branch_key" || !canonicalRefreshScope(s.Scope) {
		return errors.New("refresh session namespace or scope ceiling is invalid")
	}
	if !validRefreshTime(s.StartedAt) || !validRefreshTime(s.LastFreshAt) || s.LastFreshAt.Before(s.StartedAt) || !validRefreshTime(s.InactivityExpiresAt) || !s.InactivityExpiresAt.After(s.LastFreshAt) || !validRefreshTime(s.RetainUntil) || !s.RetainUntil.After(s.StartedAt) {
		return errors.New("refresh session clocks are invalid")
	}
	if s.AbsoluteExpiresAt != nil && (!validRefreshTime(*s.AbsoluteExpiresAt) || !s.AbsoluteExpiresAt.After(s.StartedAt)) {
		return errors.New("refresh session absolute deadline is invalid")
	}
	if s.RetryCount < 0 || s.RetryCount > 3 {
		return errors.New("refresh session retry count is outside 0..3")
	}
	if err := s.validatePredecessor(); err != nil {
		return err
	}
	return s.validateTerminal()
}

func (s *RefreshSession) ValidateTransition(previous *RefreshSession, at time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if previous == nil || !validRefreshTime(at) {
		return errors.New("refresh transition requires previous state and shared time")
	}
	if s.ID != previous.ID || s.OriginalGrantID != previous.OriginalGrantID || s.OriginalTokenSignature != previous.OriginalTokenSignature || s.AgentID != previous.AgentID || s.Principal != previous.Principal || s.ClientID != previous.ClientID || s.Scope != previous.Scope || s.BranchKeyID != previous.BranchKeyID || !s.StartedAt.Equal(previous.StartedAt) || !sameRefreshValue(s.Email, previous.Email) || s.DisplayName != previous.DisplayName {
		return errors.New("refresh origin, owner, profile and ceiling are immutable")
	}
	if s.RetainUntil.Before(previous.RetainUntil) || s.LastFreshAt.Before(previous.LastFreshAt) || s.LastFreshAt.After(at) {
		return errors.New("refresh activity and retention cannot move backwards")
	}
	if previous.TerminalReason != nil {
		if !sameRefreshValue(s.TerminalReason, previous.TerminalReason) || !sameRefreshTime(s.RevokedAt, previous.RevokedAt) || !sameRefreshTime(s.ExpiredAt, previous.ExpiredAt) {
			return errors.New("terminal refresh authority cannot change or resume")
		}
	}
	if s.TerminalReason == nil && ((previous.AbsoluteExpiresAt != nil && !at.Before(*previous.AbsoluteExpiresAt)) || !at.Before(previous.InactivityExpiresAt)) {
		return errors.New("elapsed refresh authority cannot be extended")
	}
	if s.CurrentSignature == previous.CurrentSignature {
		if !s.LastFreshAt.Equal(previous.LastFreshAt) || s.InactivityExpiresAt.After(previous.InactivityExpiresAt) || !sameRefreshValue(s.PreviousSignature, previous.PreviousSignature) || !sameRefreshTime(s.PreviousConsumedAt, previous.PreviousConsumedAt) || !sameRefreshTime(s.ReuseUntil, previous.ReuseUntil) || !sameRefreshValue(s.OriginalRequestedScope, previous.OriginalRequestedScope) || !sameRefreshValue(s.OriginalRequestContextFingerprint, previous.OriginalRequestContextFingerprint) || !sameRefreshTime(s.RetryAccessExpiresAt, previous.RetryAccessExpiresAt) {
			return errors.New("only fresh rotation can replace activity or predecessor evidence")
		}
		if previous.RetryExpiresAt != nil && (s.RetryExpiresAt == nil || s.RetryExpiresAt.After(*previous.RetryExpiresAt)) {
			return errors.New("effective retry deadline cannot increase or disappear")
		}
		if s.RetryCount < previous.RetryCount || s.RetryCount > previous.RetryCount+1 {
			return errors.New("one committed retry increments the counter at most once")
		}
		if len(s.RetryCiphertext) != 0 && !bytes.Equal(s.RetryCiphertext, previous.RetryCiphertext) {
			return errors.New("an original retry result cannot be replaced or restored")
		}
	} else if s.TerminalReason != nil || previous.TerminalReason != nil || s.PreviousSignature == nil || *s.PreviousSignature != previous.CurrentSignature || s.PreviousConsumedAt == nil || !s.PreviousConsumedAt.Equal(s.LastFreshAt) || s.RetryCount != 0 {
		return errors.New("fresh rotation requires the consumed current token and one successor")
	}
	return nil
}

func (s *RefreshSession) validatePredecessor() error {
	if s.PreviousSignature == nil {
		if s.PreviousConsumedAt != nil || s.ReuseUntil != nil || s.OriginalRequestedScope != nil || s.OriginalRequestContextFingerprint != nil || s.RetryAccessExpiresAt != nil || s.RetryExpiresAt != nil || len(s.RetryCiphertext) != 0 || s.RetryCount != 0 || s.CurrentSignature != s.OriginalTokenSignature || !s.LastFreshAt.Equal(s.StartedAt) {
			return errors.New("initial refresh state cannot contain predecessor metadata")
		}
		return nil
	}
	if s.PreviousConsumedAt == nil || s.ReuseUntil == nil || s.OriginalRequestedScope == nil || s.OriginalRequestContextFingerprint == nil || s.RetryAccessExpiresAt == nil || s.RetryExpiresAt == nil {
		return errors.New("refresh predecessor metadata must be complete")
	}
	if !validRefreshDigest(*s.PreviousSignature) || *s.PreviousSignature == s.CurrentSignature || !validRefreshDigest(*s.OriginalRequestContextFingerprint) || !canonicalRefreshScope(*s.OriginalRequestedScope) {
		return errors.New("refresh predecessor bindings are invalid")
	}
	if !validRefreshTime(*s.PreviousConsumedAt) || s.PreviousConsumedAt.Before(s.StartedAt) || !s.PreviousConsumedAt.Equal(s.LastFreshAt) || !validRefreshTime(*s.ReuseUntil) || s.ReuseUntil.Before(*s.PreviousConsumedAt) || !validRefreshTime(*s.RetryAccessExpiresAt) || !s.RetryAccessExpiresAt.After(s.StartedAt) || !validRefreshTime(*s.RetryExpiresAt) || s.RetryExpiresAt.After(*s.ReuseUntil) || s.RetryExpiresAt.After(*s.RetryAccessExpiresAt) || s.RetryExpiresAt.After(s.InactivityExpiresAt) || (s.AbsoluteExpiresAt != nil && s.RetryExpiresAt.After(*s.AbsoluteExpiresAt)) {
		return errors.New("refresh predecessor clocks are invalid")
	}
	if s.ReuseUntil.Equal(*s.PreviousConsumedAt) && len(s.RetryCiphertext) != 0 {
		return errors.New("zero reuse cannot retain an encrypted retry result")
	}
	return nil
}

func (s *RefreshSession) validateTerminal() error {
	if s.RevokedAt != nil && s.ExpiredAt != nil {
		return errors.New("refresh session cannot be both revoked and expired")
	}
	terminal := s.RevokedAt
	if terminal == nil {
		terminal = s.ExpiredAt
	}
	if terminal == nil {
		if s.TerminalReason != nil {
			return errors.New("active refresh session cannot have a terminal reason")
		}
		return nil
	}
	if !validRefreshTime(*terminal) || terminal.Before(s.StartedAt) || s.TerminalReason == nil || !validRefreshReason(*s.TerminalReason) || len(s.RetryCiphertext) != 0 {
		return errors.New("terminal refresh state requires a valid reason and erased result")
	}
	expiry := *s.TerminalReason == RefreshReasonAbsoluteExpiry || *s.TerminalReason == RefreshReasonInactivityExpiry
	if expiry != (s.ExpiredAt != nil) {
		return errors.New("refresh terminal timestamp must match its reason")
	}
	return nil
}

func validRefreshDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := range value {
		character := value[i]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validRefreshTime(at time.Time) bool {
	_, offset := at.Zone()
	return !at.IsZero() && offset == 0 && at.Year() >= 1 && at.Year() <= 9999
}

func canonicalRefreshScope(scope string) bool {
	previous := ""
	for scope != "" {
		part, remainder, more := strings.Cut(scope, " ")
		if part == "" || (previous != "" && part <= previous) || (more && remainder == "") {
			return false
		}
		for i := range part {
			if part[i] < 0x21 || part[i] > 0x7e || part[i] == '"' || part[i] == '\\' {
				return false
			}
		}
		previous, scope = part, remainder
	}
	return true
}

func validRefreshReason(reason RefreshRevocationReason) bool {
	switch reason {
	case RefreshReasonGrantDeleted, RefreshReasonExpiredGrantRenewal, RefreshReasonAgentDeleted, RefreshReasonCredentialRevoked, RefreshReasonCodeReplay, RefreshReasonProhibitedReuse, RefreshReasonAbsoluteExpiry, RefreshReasonInactivityExpiry, RefreshReasonRestoreInvalidation:
		return true
	default:
		return false
	}
}

func sameRefreshValue[T comparable](left, right *T) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func sameRefreshTime(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
}
