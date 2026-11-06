// Package servicescope owns Virtual Service Scopes: the broker-only scope grammar, requested
// service resolution, grant coverage and requested-service evaluation (ADR 046).
package servicescope

import (
	"regexp"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// Kind classifies one space-delimited scope token.
type Kind int

const (
	// KindOrdinary does not start with the configured `<public URL>/sessions/` prefix.
	KindOrdinary Kind = iota
	// KindVirtual names a service by its lowercase UUID or canonical ID.
	KindVirtual
	// KindMalformed has the prefix but an invalid identifier.
	KindMalformed
)

// Form names the identifier form of a Virtual Service Scope.
type Form string

const (
	FormUUID      Form = "uuid"
	FormCanonical Form = "canonical"
)

// VirtualServiceScope is one parsed `<public URL>/sessions/<identifier>` scope.
type VirtualServiceScope struct {
	// Raw is the exact requested string. It is never rewritten when granted.
	Raw        string
	Identifier string
	Form       Form
}

var lowercaseUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// prefix returns `<public URL>/sessions/` for the configured public URL. The origin always
// comes from configuration, never from a request Host.
func prefix(publicURL string) string {
	return strings.TrimRight(publicURL, "/") + "/sessions/"
}

// Parse classifies one scope token against the configured public URL.
func Parse(raw, publicURL string) (VirtualServiceScope, Kind) {
	if strings.TrimRight(publicURL, "/") == "" {
		return VirtualServiceScope{}, KindOrdinary
	}
	identifier, found := strings.CutPrefix(raw, prefix(publicURL))
	if !found {
		return VirtualServiceScope{}, KindOrdinary
	}
	if lowercaseUUID.MatchString(identifier) {
		return VirtualServiceScope{Raw: raw, Identifier: identifier, Form: FormUUID}, KindVirtual
	}
	if canonical.Validate(&identifier) == nil {
		return VirtualServiceScope{Raw: raw, Identifier: identifier, Form: FormCanonical}, KindVirtual
	}
	return VirtualServiceScope{}, KindMalformed
}

// Format emits the current scope for a service: its canonical ID when it has one, its UUID
// otherwise.
func Format(publicURL string, serviceID id.ServiceID, canonicalID *string) string {
	if canonicalID != nil && *canonicalID != "" {
		return prefix(publicURL) + *canonicalID
	}
	return prefix(publicURL) + serviceID.String()
}

// IsBrokerScope reports whether a token carries the broker's virtual-scope prefix, valid or not.
func IsBrokerScope(raw, publicURL string) bool {
	_, kind := Parse(raw, publicURL)
	return kind != KindOrdinary
}

// StripVirtual removes every broker-prefixed token from a space-delimited scope so no broker
// scope reaches an upstream identity provider. Ordinary tokens keep their order.
func StripVirtual(scope, publicURL string) string {
	var kept []string
	for _, token := range strings.Fields(scope) {
		if !IsBrokerScope(token, publicURL) {
			kept = append(kept, token)
		}
	}
	return strings.Join(kept, " ")
}
