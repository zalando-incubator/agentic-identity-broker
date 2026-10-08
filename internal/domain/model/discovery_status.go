package model

import (
	"errors"
	"time"
)

// DiscoveryStatus records the latest attempt and latest committed configuration.
// The service owns this value; a failed refresh leaves the active fields unchanged.
type DiscoveryStatus struct {
	LastAttemptAt *time.Time
	LastSuccessAt *time.Time
	FailureReason *string
}

// Status derives the public state from the active discovery source and last attempt.
func (s DiscoveryStatus) Status(resourceURL *string) string {
	if resourceURL == nil {
		return "not_applicable"
	}
	if s.FailureReason != nil {
		return "failed"
	}
	return "ready"
}

// ValidDiscoveryFailureCode accepts only a short code without remote response data.
func ValidDiscoveryFailureCode(code string) bool {
	if len(code) == 0 || len(code) > 64 || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for i := 1; i < len(code); i++ {
		c := code[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// Validate rejects status that cannot represent a committed discovery result.
func (s DiscoveryStatus) Validate(resourceURL *string) error {
	if resourceURL == nil {
		if s.LastAttemptAt != nil || s.LastSuccessAt != nil || s.FailureReason != nil {
			return errors.New("manual service cannot have discovery status")
		}
		return nil
	}
	if s.LastAttemptAt == nil || s.LastSuccessAt == nil {
		return errors.New("discovery needs attempt and success times")
	}
	if s.FailureReason == nil {
		if !s.LastAttemptAt.Equal(*s.LastSuccessAt) {
			return errors.New("ready discovery needs equal attempt and success times")
		}
		return nil
	}
	if !ValidDiscoveryFailureCode(*s.FailureReason) {
		return errors.New("discovery failure code is invalid")
	}
	if !s.LastAttemptAt.After(*s.LastSuccessAt) {
		return errors.New("failed discovery attempt must follow the last success")
	}
	return nil
}
