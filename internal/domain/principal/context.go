// Package principal provides domain types and functions for managing authenticated principals.
package principal

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// principalContextKey is an unexported type used as the context key for principals.
// Using an unexported type prevents external packages from accessing the principal directly
// via context.Value(), ensuring all access goes through the public API (FromContext, MustFromContext).
// This prevents context key collisions and ensures type safety.
type principalContextKey struct{}

// WithPrincipal creates a new context derived from the parent context with the principal embedded.
// The principal MUST be validated before calling this function.
//
// Parameters:
//   - ctx: Parent context (typically from HTTP request)
//   - principal: Validated principal value (non-empty, ≤ 200 chars, trimmed)
//
// Returns: New context with principal embedded
//
// Preconditions:
//   - principal MUST be non-empty
//   - principal MUST be trimmed
//   - principal MUST be ≤ 200 characters
//   - principal MUST be valid UTF-8
//
// Postconditions:
//   - New context contains principal accessible via FromContext
//   - Parent context is unmodified (immutability)
//   - New context inherits cancellation, deadlines from parent
func WithPrincipal(ctx context.Context, principal id.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// FromContext retrieves the principal from the context.
// Returns (principal, true) if present and valid, (zero Principal, false) if not present.
// This is the safe retrieval method for use in handlers that may or may not have principals.
//
// Parameters:
//   - ctx: Context potentially containing principal
//
// Returns:
//   - id.Principal: Principal value (zero value if not present)
//   - bool: True if principal present and valid, false otherwise
//
// Postconditions:
//   - If bool is true, the principal is guaranteed non-empty and ≤ 200 characters
//   - If bool is false, the principal is the zero value
//
// Usage:
//
//	principal, ok := principal.FromContext(r.Context())
//	if !ok {
//	    http.Error(w, "unauthorized", http.StatusUnauthorized)
//	    return
//	}
//	log.Info("request from principal", "principal", principal)
func FromContext(ctx context.Context) (id.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(id.Principal)
	return principal, ok
}

// MustFromContext retrieves the principal from the context.
// Panics if principal is not present. This is intended for use on protected routes
// where middleware guarantees principal presence.
//
// Parameters:
//   - ctx: Context MUST contain principal
//
// Returns: Principal value (guaranteed non-empty)
//
// Preconditions:
//   - Context MUST contain principal (enforced by middleware on protected routes)
//
// Error Handling:
//   - Panics if principal not in context (programming error, not runtime error)
//   - Panic message: "principal not found in context"
//
// Usage (protected routes only):
//
//	principal := principal.MustFromContext(r.Context())
//	// Safe to use—middleware guarantees presence
//	users := userService.GetUsersForPrincipal(principal)
//
// When to Use:
//   - Protected routes with RequirePrincipalMiddleware applied
//   - Internal functions called only from protected routes
//   - Code paths guaranteed to have principal by design
//
// When NOT to Use:
//   - Optional routes (use FromContext instead)
//   - Public routes (use FromContext instead)
//   - Code paths where principal may be missing
func MustFromContext(ctx context.Context) id.Principal {
	principal, ok := FromContext(ctx)
	if !ok {
		panic("principal not found in context")
	}
	return principal
}
