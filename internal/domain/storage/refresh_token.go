package storage

import (
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type RefreshToken struct {
	Signature string
	SessionID id.RefreshSessionID
	IssuedAt  time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (t *RefreshToken) Validate() error {
	if t == nil || !validRefreshDigest(t.Signature) || t.SessionID.IsZero() {
		return errors.New("refresh token requires a canonical signature and root")
	}
	if !validRefreshTime(t.IssuedAt) || !validRefreshTime(t.ExpiresAt) || !t.ExpiresAt.After(t.IssuedAt) {
		return errors.New("refresh token requires finite issued expiry")
	}
	if t.UsedAt != nil && (!validRefreshTime(*t.UsedAt) || t.UsedAt.Before(t.IssuedAt)) {
		return errors.New("refresh token consumption cannot precede issuance")
	}
	return nil
}

func (t *RefreshToken) ValidateTransition(previous *RefreshToken) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if previous == nil || t.Signature != previous.Signature || t.SessionID != previous.SessionID || !t.IssuedAt.Equal(previous.IssuedAt) || !t.ExpiresAt.Equal(previous.ExpiresAt) {
		return errors.New("refresh token identity and issued expiry are immutable")
	}
	if previous.UsedAt != nil && !sameRefreshTime(t.UsedAt, previous.UsedAt) {
		return errors.New("first refresh consumption cannot change or disappear")
	}
	return nil
}
