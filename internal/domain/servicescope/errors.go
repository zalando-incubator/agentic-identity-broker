package servicescope

import "errors"

// Resolution failures. Each one is a request the agent is not permitted to make and surfaces
// as invalid_scope; any other resolution error is a repository failure (server_error).
var (
	ErrMalformedScope     = errors.New("malformed service scope")
	ErrUnknownService     = errors.New("unknown service")
	ErrServiceNotEligible = errors.New("service is not eligible for the agent")
	ErrBindingMismatch    = errors.New("service scope no longer resolves to its bound service")
)

// IsPermissionFailure reports whether err rejects the requested scope rather than reporting a
// repository failure.
func IsPermissionFailure(err error) bool {
	return errors.Is(err, ErrMalformedScope) || errors.Is(err, ErrUnknownService) ||
		errors.Is(err, ErrServiceNotEligible) || errors.Is(err, ErrBindingMismatch)
}
