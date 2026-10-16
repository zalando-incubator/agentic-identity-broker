package cimd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

// ClientIDMetadataDocument is a parsed and validated CIMD JSON document.
// Immutable after construction via ParseDocument.
type ClientIDMetadataDocument struct {
	ClientID      string          `json:"client_id"`
	ClientName    string          `json:"client_name,omitempty"`
	LogoURI       string          `json:"logo_uri,omitempty"`
	RedirectURIs  []string        `json:"redirect_uris"`
	AuthMethod    string          `json:"token_endpoint_auth_method,omitempty"`
	SigningAlg    string          `json:"token_endpoint_auth_signing_alg,omitempty"`
	JWKSURI       string          `json:"jwks_uri,omitempty"`
	JWKS          json.RawMessage `json:"jwks,omitempty"`
	KeySet        jwk.Set         `json:"-"`
	GrantTypes    []string        `json:"grant_types,omitempty"`
	ResponseTypes []string        `json:"response_types,omitempty"`
	PolicyURI     string          `json:"policy_uri,omitempty"`
	TosURI        string          `json:"tos_uri,omitempty"`
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

// ParseDocument parses and validates a CIMD JSON document fetched from fetchURL.
// Returns an error if the document is malformed, the client_id field does not
// match fetchURL, redirect_uris is empty, auth_method is a secret-bearing method,
// any redirect_uri violates same-origin with fetchURL, or logo_uri (when present)
// is not an absolute HTTPS URL. private_key_jwt documents must provide exactly one
// public key source: an HTTPS jwks_uri or an inline jwks.
// nameBlocklist is a list of forbidden client_name values (case-insensitive exact match).
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

	if doc.AuthMethod != "none" && doc.AuthMethod != "private_key_jwt" {
		return nil, fmt.Errorf("token_endpoint_auth_method %q is not supported", doc.AuthMethod)
	}

	if doc.AuthMethod == "private_key_jwt" {
		if (doc.JWKSURI == "") == (len(doc.JWKS) == 0) {
			return nil, fmt.Errorf("private_key_jwt requires exactly one of jwks_uri or jwks")
		}
		if doc.SigningAlg == "" {
			doc.SigningAlg = "RS256"
		}
		if doc.SigningAlg != "RS256" && doc.SigningAlg != "ES256" {
			return nil, fmt.Errorf("token_endpoint_auth_signing_alg %q is not supported", doc.SigningAlg)
		}
		if doc.JWKSURI != "" {
			if err := urivalidation.ValidateCIMDClientURL(doc.JWKSURI); err != nil {
				return nil, fmt.Errorf("jwks_uri: %w", err)
			}
		} else {
			if err := rejectPrivateJWKMaterial(doc.JWKS); err != nil {
				return nil, err
			}
			keySet, err := jwk.Parse(doc.JWKS)
			if err != nil {
				return nil, fmt.Errorf("invalid inline jwks: %w", err)
			}
			doc.KeySet = keySet
		}
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

func rejectPrivateJWKMaterial(data []byte) error {
	var keySet struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(data, &keySet); err != nil {
		return fmt.Errorf("invalid inline jwks: %w", err)
	}
	privateMembers := []string{"d", "p", "q", "dp", "dq", "qi", "k"}
	for keyIndex, key := range keySet.Keys {
		for _, member := range privateMembers {
			if _, ok := key[member]; ok {
				return fmt.Errorf("inline jwks key %d contains private member %q", keyIndex, member)
			}
		}
	}
	return nil
}

func validateLogoURI(logoURI string) error {
	u, err := url.Parse(logoURI)
	if err != nil || !u.IsAbs() || u.Scheme != "https" {
		return fmt.Errorf("logo_uri %q must be an absolute HTTPS URL", logoURI)
	}
	return nil
}

// validateRedirectOrigin enforces same-origin between a redirect URI and the
// client_id URL, with exceptions for loopback and private-use redirect URIs.
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
