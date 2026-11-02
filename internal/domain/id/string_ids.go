package id

import "log/slog"

// ClientID is an OAuth2 client identifier (VARCHAR(255), not a UUID).
type ClientID string

func NewClientID(s string) ClientID { return ClientID(s) }
func (id ClientID) String() string  { return string(id) }
func (id ClientID) IsZero() bool    { return id == "" }

// ExternalID is an optional external governance system identifier (VARCHAR(255)).
type ExternalID string

func NewExternalID(s string) ExternalID { return ExternalID(s) }
func (id ExternalID) String() string    { return string(id) }
func (id ExternalID) IsZero() bool      { return id == "" }

// Principal is an authenticated user identity string (email, subject).
type Principal string

func NewPrincipal(s string) Principal { return Principal(s) }
func (id Principal) String() string   { return string(id) }
func (id Principal) IsZero() bool     { return id == "" }

// LogValue renders the raw principal. Log output passes through the telemetry
// principal policy handler, which recognizes this type and applies the policy.
func (id Principal) LogValue() slog.Value { return slog.StringValue(string(id)) }

// KeyID is a JWT Key ID (kid claim) identifying a signing key within JWKS.
type KeyID string

func NewKeyID(s string) KeyID   { return KeyID(s) }
func (id KeyID) String() string { return string(id) }
func (id KeyID) IsZero() bool   { return id == "" }
