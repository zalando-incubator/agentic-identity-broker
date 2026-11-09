package storage

import "time"

// ClientAssertionReplay records a client assertion jti that has already been consumed.
type ClientAssertionReplay struct {
	ClientID  string    `db:"client_id"`
	JTI       string    `db:"jti"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}

// Validate checks the natural key and expiry required for replay protection.
func (r *ClientAssertionReplay) Validate() error {
	if r.ClientID == "" {
		return NewStorageError("ClientAssertionReplay.Validate", ErrorKindValidation, nil, "client_id is required")
	}
	if r.JTI == "" {
		return NewStorageError("ClientAssertionReplay.Validate", ErrorKindValidation, nil, "jti is required")
	}
	if r.ExpiresAt.IsZero() {
		return NewStorageError("ClientAssertionReplay.Validate", ErrorKindValidation, nil, "expires_at is required")
	}
	return nil
}
