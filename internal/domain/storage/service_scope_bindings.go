package storage

import (
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// ServiceScopeBindings maps each originally granted Virtual Service Scope to its service UUID
// (ADR 046). It is immutable after issuance and inherited unchanged by refresh successors.
type ServiceScopeBindings map[string]id.ServiceID

var lowercaseServiceUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Validate checks that every key is an absolute `/sessions/<identifier>` scope with a valid
// identifier and every value a non-zero service. keysWithinScope requires every key to be a
// granted scope, as for authorization codes; refresh assurance bindings may outlive narrowing.
// Whether every virtual scope in scope has a binding depends on the configured public URL and
// is enforced by the issuing domain service.
func (b ServiceScopeBindings) Validate(scope string, keysWithinScope bool) error {
	granted := strings.Fields(scope)
	for key, serviceID := range b {
		if serviceID.IsZero() {
			return NewStorageError("ServiceScopeBindings.Validate", ErrorKindValidation, nil, "service_scope_bindings value must be a service ID")
		}
		if !validBindingKey(key) {
			return NewStorageError("ServiceScopeBindings.Validate", ErrorKindValidation, nil, "service_scope_bindings key must be a virtual service scope")
		}
		if keysWithinScope && !slices.Contains(granted, key) {
			return NewStorageError("ServiceScopeBindings.Validate", ErrorKindValidation, nil, "service_scope_bindings key must be a granted scope")
		}
	}
	return nil
}

func validBindingKey(key string) bool {
	base, identifier, found := strings.Cut(key, "/sessions/")
	if !found || strings.Contains(identifier, "/sessions/") {
		return false
	}
	origin, err := url.Parse(base)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") {
		return false
	}
	return lowercaseServiceUUID.MatchString(identifier) || canonical.Validate(&identifier) == nil
}

// Clone returns an independent copy; nil stays nil.
func (b ServiceScopeBindings) Clone() ServiceScopeBindings {
	if b == nil {
		return nil
	}
	return maps.Clone(b)
}

// Equal reports whether both binding sets are identical. Nil equals empty.
func (b ServiceScopeBindings) Equal(other ServiceScopeBindings) bool {
	return maps.Equal(b, other)
}

// ScopeSubset reports whether every token of scope is also in of.
func ScopeSubset(scope, of string) bool {
	allowed := strings.Fields(of)
	for _, token := range strings.Fields(scope) {
		if !slices.Contains(allowed, token) {
			return false
		}
	}
	return true
}
