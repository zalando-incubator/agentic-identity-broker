// Package principal provides domain types and functions for managing authenticated principals.
package principal

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// profileContextKey is an unexported type used as the context key for PrincipalProfile.
// Separate from principalContextKey to maintain backward compatibility — both keys
// are set by middleware; existing callers of FromContext continue to work unchanged.
type profileContextKey struct{}

// PrincipalProfile is an enriched user identity value object extracted from the
// pre-authentication source (JWT or plain header). Immutable once constructed.
// Carried through request context via WithProfile/ProfileFromContext and surfaced
// through the /api/me endpoint.
//
// Invariants:
//   - Principal must be non-empty and ≤ 200 characters
//   - DisplayName is never empty — defaults to Principal if not explicitly set
//   - Email and PictureURL are truly optional (nil when absent, not empty string)
//   - Immutable after construction (builder pattern returns modified copies)
type PrincipalProfile struct {
	// Principal is the unique user identifier (e.g., email, username, sub claim).
	// Required, non-empty, max 200 chars.
	principal id.Principal

	// DisplayName is a human-readable display name. Falls back to Principal if not set.
	displayName string

	// Email is the user's email address. Nil if not extracted from JWT or not configured.
	email *string

	// PictureURL is the URL to the user's profile picture. Nil if not extracted.
	// Treated as untrusted input.
	pictureURL *string
}

// NewProfile creates a new PrincipalProfile with the given principal.
// DisplayName defaults to the principal value. Email and PictureURL are nil.
//
// Preconditions:
//   - principal must be non-empty and ≤ 200 characters
func NewProfile(principal id.Principal) PrincipalProfile {
	return PrincipalProfile{
		principal:   principal,
		displayName: principal.String(), // Default: display name equals principal
	}
}

// WithDisplayName returns a new PrincipalProfile with the given display name.
// If displayName is empty, the principal value is used instead.
func (p PrincipalProfile) WithDisplayName(displayName string) PrincipalProfile {
	if displayName != "" {
		p.displayName = displayName
	}
	return p
}

// WithEmail returns a new PrincipalProfile with the given email address.
// Pass nil to indicate no email is available.
func (p PrincipalProfile) WithEmail(email *string) PrincipalProfile {
	p.email = email
	return p
}

// WithPictureURL returns a new PrincipalProfile with the given picture URL.
// Pass nil to indicate no picture URL is available.
func (p PrincipalProfile) WithPictureURL(pictureURL *string) PrincipalProfile {
	p.pictureURL = pictureURL
	return p
}

// Principal returns the unique user identifier.
func (p PrincipalProfile) Principal() id.Principal {
	return p.principal
}

// DisplayName returns the human-readable display name.
// Never empty — falls back to Principal if not explicitly set.
func (p PrincipalProfile) DisplayName() string {
	return p.displayName
}

// Email returns the user's email address, or nil if not available.
func (p PrincipalProfile) Email() *string {
	return p.email
}

// PictureURL returns the profile picture URL, or nil if not available.
func (p PrincipalProfile) PictureURL() *string {
	return p.pictureURL
}

// WithProfile creates a new context derived from the parent with the PrincipalProfile embedded.
// This is set alongside WithPrincipal for backward compatibility — downstream handlers
// that only need the principal can continue using FromContext.
//
// Parameters:
//   - ctx: Parent context (typically from HTTP request)
//   - profile: Validated PrincipalProfile value
//
// Returns: New context with profile embedded
func WithProfile(ctx context.Context, profile PrincipalProfile) context.Context {
	return context.WithValue(ctx, profileContextKey{}, profile)
}

// ProfileFromContext retrieves the PrincipalProfile from the context.
// Returns (profile, true) if present, (zero value, false) if not present.
// This is the enriched profile retrieval — use FromContext for just the principal.
//
// Parameters:
//   - ctx: Context potentially containing a PrincipalProfile
//
// Returns:
//   - PrincipalProfile: Profile value (zero value if not present)
//   - bool: True if profile present, false otherwise
func ProfileFromContext(ctx context.Context) (PrincipalProfile, bool) {
	profile, ok := ctx.Value(profileContextKey{}).(PrincipalProfile)
	return profile, ok
}
