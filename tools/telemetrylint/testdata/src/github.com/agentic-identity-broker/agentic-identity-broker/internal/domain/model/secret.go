// Package model is a minimal stub of the broker's credential value object for analyzer tests.
package model

import "log/slog"

type Secret struct{ plaintext string }

func (s Secret) Plaintext() string { return s.plaintext }

func (s Secret) Redacted() string { return "[REDACTED]" }

func (s Secret) LogValue() slog.Value { return slog.StringValue(s.Redacted()) }
