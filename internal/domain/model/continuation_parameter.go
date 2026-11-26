package model

import "log/slog"

type ContinuationStatus uint8

const (
	ContinuationAbsent ContinuationStatus = iota
	ContinuationPresent
	ContinuationInvalid
)

// ContinuationParameter is the parsed session_token query parameter: absent, present or invalid.
// Present means a token was parsed, not that it is a valid sealed authorization session.
type ContinuationParameter struct {
	Token  string
	Status ContinuationStatus
}

func (ContinuationParameter) String() string         { return "[REDACTED]" }
func (c ContinuationParameter) GoString() string     { return c.String() }
func (c ContinuationParameter) LogValue() slog.Value { return slog.StringValue(c.String()) }
