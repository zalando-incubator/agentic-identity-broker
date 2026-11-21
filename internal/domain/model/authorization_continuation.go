package model

import "log/slog"

type ContinuationStatus uint8

const (
	ContinuationAbsent ContinuationStatus = iota
	ContinuationPresent
	ContinuationInvalid
)

// AuthorizationContinuation is parsed input, not proof that its sealed token is valid.
type AuthorizationContinuation struct {
	Token  string
	Status ContinuationStatus
}

func (AuthorizationContinuation) String() string         { return "[REDACTED]" }
func (c AuthorizationContinuation) GoString() string     { return c.String() }
func (c AuthorizationContinuation) LogValue() slog.Value { return slog.StringValue(c.String()) }
