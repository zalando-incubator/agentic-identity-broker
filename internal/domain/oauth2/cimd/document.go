package cimd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
)

// ClientIDMetadataDocument is a parsed and validated CIMD JSON document.
// Immutable after construction via ParseDocument.
type ClientIDMetadataDocument struct {
	ClientID      string   `json:"client_id"`
	ClientName    string   `json:"client_name,omitempty"`
	LogoURI       string   `json:"logo_uri,omitempty"`
	RedirectURIs  []string `json:"redirect_uris"`
	AuthMethod    string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes    []string `json:"grant_types,omitempty"`
	ResponseTypes []string `json:"response_types,omitempty"`
	PolicyURI     string   `json:"policy_uri,omitempty"`
	TosURI        string   `json:"tos_uri,omitempty"`
}

// ValidatePrivateUseRedirectURIs checks native callbacks against the resolved Agent.
// Call it on every resolution, including cache hits.
func (d *ClientIDMetadataDocument) ValidatePrivateUseRedirectURIs(agentRedirectURIs []string) error {
	for i, redirectURI := range d.RedirectURIs {
		if urivalidation.IsValidPrivateUseRedirectURI(redirectURI) && !slices.Contains(agentRedirectURIs, redirectURI) {
			return fmt.Errorf("redirect_uris[%d]: private-use redirect_uri %q is not listed in the agent's redirect_uris", i, redirectURI)
		}
	}
	return nil
}

// ParseDocument validates a CIMD document fetched from fetchURL. It checks web
// redirect origins and native callback structure. Callers must validate native
// callbacks against the resolved Agent with ValidatePrivateUseRedirectURIs before use.
// nameBlocklist forbids client_name values by case-insensitive exact match.
func ParseDocument(data []byte, fetchURL string, nameBlocklist []string) (*ClientIDMetadataDocument, error) {
	var doc ClientIDMetadataDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("malformed CIMD document: %w", err)
	}

	// RFC §4.1: client_secret and client_secret_expires_at MUST NOT be used
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["client_secret"]; ok {
			return nil, fmt.Errorf("CIMD document must not contain client_secret")
		}
		if _, ok := raw["client_secret_expires_at"]; ok {
			return nil, fmt.Errorf("CIMD document must not contain client_secret_expires_at")
		}
	}

	// RFC 7591 §2: omitted token_endpoint_auth_method defaults to "none"
	if doc.AuthMethod == "" {
		doc.AuthMethod = "none"
	}

	// SR-008: client_id must exactly match the fetch URL
	if doc.ClientID != fetchURL {
		return nil, fmt.Errorf("client_id mismatch: document has %q, expected %q", doc.ClientID, fetchURL)
	}

	// FR-004: redirect_uris must not be empty
	if len(doc.RedirectURIs) == 0 {
		return nil, fmt.Errorf("redirect_uris is required and must not be empty")
	}

	// FR-022: only "none" is permitted; CIMD clients are always public clients
	if doc.AuthMethod != "none" {
		return nil, fmt.Errorf("token_endpoint_auth_method %q is not supported; CIMD clients must use \"none\"", doc.AuthMethod)
	}

	// FR-023b: client_name keyword blocklist (case-insensitive exact match)
	if doc.ClientName != "" {
		lowerName := strings.ToLower(doc.ClientName)
		for _, blocked := range nameBlocklist {
			if lowerName == strings.ToLower(blocked) {
				return nil, fmt.Errorf("client_name matches blocked term %q", blocked)
			}
		}
	}

	// FR-004a: enforce web origins and validate private-use URI structure.
	clientURL, err := url.Parse(fetchURL)
	if err != nil {
		return nil, fmt.Errorf("invalid fetch URL: %w", err)
	}
	for i, ruri := range doc.RedirectURIs {
		if err := validateRedirectOrigin(clientURL, ruri); err != nil {
			return nil, fmt.Errorf("redirect_uris[%d]: %w", i, err)
		}
	}

	if doc.LogoURI != "" {
		if err := validateLogoURI(doc.LogoURI); err != nil {
			return nil, fmt.Errorf("logo_uri: %w", err)
		}
	}

	return &doc, nil
}

func validateLogoURI(logoURI string) error {
	u, err := url.Parse(logoURI)
	if err != nil || !u.IsAbs() || u.Scheme != "https" {
		return fmt.Errorf("logo_uri %q must be an absolute HTTPS URL", logoURI)
	}
	return nil
}

// validateRedirectOrigin enforces web same-origin (with loopback exceptions).
// The resolved Agent's allowlist for private-use callbacks is checked separately.
func validateRedirectOrigin(clientURL *url.URL, redirectURI string) error {
	if urivalidation.IsValidPrivateUseRedirectURI(redirectURI) {
		return nil
	}
	// Structural validity check first (catches ftp://, fragments, missing host, etc.)
	if !urivalidation.IsValidRedirectURI(redirectURI) {
		return fmt.Errorf("redirect_uri %q is not a valid redirect URI", redirectURI)
	}
	r, _ := url.Parse(redirectURI) // safe: IsValidRedirectURI already validated
	host := r.Hostname()
	// Localhost exception applies only to the same-origin comparison, not structural validity
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	// Same-origin: scheme + normalized host must match.
	// Normalize by comparing scheme and hostname + effective port separately
	// so that https://example.com:443/cb matches https://example.com/client.
	if r.Scheme != clientURL.Scheme {
		return fmt.Errorf("redirect_uri %q is not same-origin with client_id %q", redirectURI, clientURL.String())
	}
	if r.Hostname() != clientURL.Hostname() {
		return fmt.Errorf("redirect_uri %q is not same-origin with client_id %q", redirectURI, clientURL.String())
	}
	if effectivePort(r) != effectivePort(clientURL) {
		return fmt.Errorf("redirect_uri %q is not same-origin with client_id %q", redirectURI, clientURL.String())
	}
	return nil
}

// effectivePort returns the explicit port or the default for the scheme.
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}
