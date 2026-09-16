package model

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
)

// isAllowedHTTPSScheme checks if a URL uses an allowed scheme.
// HTTPS is always allowed. HTTP is only allowed for localhost addresses
// or when skipHTTPSValidation is true (dev/test mode).
func isAllowedHTTPSScheme(urlStr string, skipHTTPSValidation bool) bool {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme == "http" {
		if skipHTTPSValidation {
			return true
		}
		host := parsed.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	return false
}

var reservedAuthorizationParamNames = map[string]struct{}{
	"client_id": {}, "client_secret": {}, "redirect_uri": {}, "response_type": {}, "scope": {}, "state": {},
	"code": {}, "grant_type": {}, "code_verifier": {}, "code_challenge": {}, "code_challenge_method": {},
	"nonce": {}, "request": {}, "request_uri": {}, "refresh_token": {},
}

func isPublicClientCredentialParameter(name string) bool {
	switch strings.ToLower(name) {
	case "client_assertion", "client_assertion_type", "client_secret":
		return true
	}
	return false
}

func validatePublicClientOutboundConfiguration(params map[string]string, tokenEndpoint string) error {
	for name := range params {
		if isPublicClientCredentialParameter(name) {
			return fmt.Errorf("authorization parameter is not allowed for public clients: %s", name)
		}
	}

	if tokenEndpoint == "" {
		return nil
	}
	endpoint, err := url.Parse(tokenEndpoint)
	if err != nil {
		return fmt.Errorf("token_endpoint is invalid: %w", err)
	}
	if endpoint.User != nil {
		return errors.New("token_endpoint must not include userinfo for public clients")
	}
	query, err := url.ParseQuery(endpoint.RawQuery)
	if err != nil {
		return errors.New("token_endpoint query parameters are invalid")
	}
	for name := range query {
		if isPublicClientCredentialParameter(name) {
			return fmt.Errorf("token_endpoint must not include client authentication parameter for public clients: %s", name)
		}
	}
	return nil
}

// IsReservedAuthorizationParamName reports whether name is controlled by the broker.
func IsReservedAuthorizationParamName(name string) bool {
	_, reserved := reservedAuthorizationParamNames[strings.ToLower(name)]
	return reserved
}

func validateAuthorizationParams(params map[string]string) error {
	for name, value := range params {
		if strings.TrimSpace(name) == "" {
			return errors.New("authorization parameter name cannot be blank")
		}
		if strings.TrimSpace(value) == "" {
			return errors.New("authorization parameter value cannot be blank")
		}
		if IsReservedAuthorizationParamName(name) {
			return fmt.Errorf("authorization parameter name is reserved: %s", name)
		}
	}
	return nil
}

// ThirdpartyOAuth2ProviderEntity is the domain entity for an external OAuth2 provider that
// agents can access on behalf of users (e.g., GitHub, Google, Databricks).
//
// The Secret field uses the Secret value object with three mutually exclusive states:
//   - Plaintext state: used when creating or updating a confidential provider and
//     returned by Get/List after successful decryption.
//   - Encrypted state: used internally by the repository and returned when confidential
//     decryption fails.
//   - Absent state: carried by public providers, which have no client credential.
//
// Encryption and decryption happens exclusively in domain services, not in this entity.
type ThirdpartyOAuth2ProviderEntity struct {
	ID                      id.ServiceID
	CanonicalID             *string
	ClearCanonicalID        bool
	DisplayName             string
	ClientID                id.ClientID
	Secret                  Secret
	TokenEndpointAuthMethod TokenEndpointAuthMethod
	Flavor                  OAuth2Flavor // defaults to OAuth2FlavorStandard when zero
	IssuerURI               string
	Discovery               DiscoveryConfig
	Endpoints               OAuth2Endpoints
	Scopes                  []OAuthScope
	AuthorizationParams     map[string]string
	ProtectedResources      []string
	Version                 int64
	ServiceRequirements     []ServiceRequirement
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// IsPublicClient reports whether the provider uses no token-endpoint client authentication.
func (e *ThirdpartyOAuth2ProviderEntity) IsPublicClient() bool {
	return e.TokenEndpointAuthMethod == TokenEndpointAuthMethodNone
}

// Validate performs basic validation on the entity.
// It checks that required fields are present and the secret is in a valid state.
func (e *ThirdpartyOAuth2ProviderEntity) Validate() error {
	if e.ID.IsZero() {
		return errors.New("provider ID cannot be empty")
	}
	if e.Version < 0 {
		return errors.New("version cannot be negative")
	}
	if err := canonical.Validate(e.CanonicalID); err != nil {
		return err
	}

	if e.DisplayName == "" {
		return errors.New("display_name is required")
	}
	if len(e.DisplayName) > 255 {
		return errors.New("display_name exceeds 255 characters")
	}
	if e.ClientID.IsZero() {
		return errors.New("client_id is required")
	}

	// Secret must be in a valid state (absent, plaintext, or encrypted)
	if !e.Secret.IsAbsent() {
		if e.Secret.IsPlaintext() {
			if _, err := e.Secret.GetPlaintext(); err != nil {
				return err
			}
		} else if e.Secret.IsEncrypted() {
			if _, err := e.Secret.GetCiphertext(); err != nil {
				return err
			}
		}
	}

	if e.IssuerURI == "" {
		return errors.New("issuer_uri is required")
	}

	return nil
}

func (e *ThirdpartyOAuth2ProviderEntity) validateClientAuthentication(flavor OAuth2Flavor) (bool, error) {
	if err := e.TokenEndpointAuthMethod.Validate(); err != nil {
		return false, err
	}

	isPublicClient := e.IsPublicClient()
	if isPublicClient {
		if flavor == OAuth2FlavorGoogle {
			return false, errors.New(`token_endpoint_auth_method "none" is not supported for the google flavor: the client identifier is derived from the credential document`)
		}
		if !e.Secret.IsAbsent() {
			return false, errors.New(`client_secret must not have a non-empty value when token_endpoint_auth_method is "none"`)
		}
		if e.ClientID == "" {
			return false, errors.New("client_id is required")
		}
		return true, nil
	}

	if e.Secret.IsAbsent() {
		return false, errors.New("client_secret is required")
	}
	return false, nil
}

// ValidateForCreate validates the entity for a create operation.
// skipHTTPSValidation allows HTTP URLs for development/testing.
// Does not require ID (will be generated by the domain service).
func (e *ThirdpartyOAuth2ProviderEntity) ValidateForCreate(skipHTTPSValidation bool) error {
	if err := canonical.Validate(e.CanonicalID); err != nil {
		return err
	}

	if e.DisplayName == "" {
		return errors.New("display_name is required")
	}
	if len(e.DisplayName) > 255 {
		return fmt.Errorf("display_name exceeds 255 characters (got %d)", len(e.DisplayName))
	}

	flavor := e.Flavor
	if flavor == "" {
		flavor = DefaultOAuth2Flavor
	}
	isPublicClient, err := e.validateClientAuthentication(flavor)
	if err != nil {
		return err
	}
	if err := flavor.Validate(); err != nil {
		return err
	}

	credential := ""
	if !isPublicClient {
		if !e.Secret.IsPlaintext() {
			return errors.New("client_secret is required for create")
		}
		credential, err = e.Secret.GetPlaintext()
		if err != nil {
			return errors.New("client_secret is required")
		}
	}

	switch flavor {
	case OAuth2FlavorStandard, OAuth2FlavorGitHub:
		// Standard/GitHub flavor: client_id and issuer_uri are required; confidential clients require a credential.
		if e.ClientID == "" {
			return errors.New("client_id is required")
		}
		if !isPublicClient && credential == "" {
			return errors.New("client_secret is required")
		}
		if e.IssuerURI == "" {
			return errors.New("issuer_uri is required")
		}
		if !isAllowedHTTPSScheme(e.IssuerURI, skipHTTPSValidation) {
			return errors.New("issuer_uri must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)")
		}
		if !e.Discovery.EnableDiscovery {
			if e.Endpoints.TokenEndpoint == "" {
				return errors.New("token_endpoint is required when discovery is disabled")
			}
			if e.Endpoints.AuthorizeEndpoint == "" {
				return errors.New("authorize_endpoint is required when discovery is disabled")
			}
		}

	case OAuth2FlavorGoogle:
		// Google flavor: parse credential once, enrich derived fields, and validate host
		// consistency. enrichForGoogleFlavor mutates the entity (sets ClientID, Endpoints,
		// IssuerURI) so callers need not perform a separate enrichment step.
		if err := e.enrichForGoogleFlavor(credential); err != nil {
			return err
		}
	}

	if e.Discovery.MetadataURL != nil {
		if !isAllowedHTTPSScheme(*e.Discovery.MetadataURL, skipHTTPSValidation) {
			return errors.New("metadata_url must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)")
		}
	}

	for i, scope := range e.Scopes {
		if err := scope.Validate(); err != nil {
			return fmt.Errorf("scope %d: %w", i, err)
		}
	}

	if err := e.ValidateProtectedResources(); err != nil {
		return err
	}
	if err := validateAuthorizationParams(e.AuthorizationParams); err != nil {
		return err
	}
	if isPublicClient {
		if err := validatePublicClientOutboundConfiguration(e.AuthorizationParams, e.Endpoints.TokenEndpoint); err != nil {
			return err
		}
	}

	return nil
}

// ValidateForUpdate validates the entity for an update operation.
// skipHTTPSValidation allows HTTP URLs for development/testing.
// Requires ID. Confidential clients must supply the new secret in plaintext.
func (e *ThirdpartyOAuth2ProviderEntity) ValidateForUpdate(skipHTTPSValidation bool) error {
	if err := canonical.Validate(e.CanonicalID); err != nil {
		return err
	}

	if e.ID.IsZero() {
		return errors.New("provider ID cannot be empty")
	}
	if e.DisplayName == "" {
		return errors.New("display_name is required")
	}
	if len(e.DisplayName) > 255 {
		return fmt.Errorf("display_name exceeds 255 characters (got %d)", len(e.DisplayName))
	}

	flavor := e.Flavor
	if flavor == "" {
		flavor = DefaultOAuth2Flavor
	}
	isPublicClient, err := e.validateClientAuthentication(flavor)
	if err != nil {
		return err
	}
	if err := flavor.Validate(); err != nil {
		return err
	}

	credential := ""
	if !isPublicClient {
		if !e.Secret.IsPlaintext() {
			return errors.New("client_secret is required for update")
		}
		credential, err = e.Secret.GetPlaintext()
		if err != nil {
			return errors.New("client_secret is required")
		}
	}

	switch flavor {
	case OAuth2FlavorStandard, OAuth2FlavorGitHub:
		// Standard/GitHub flavor: client_id and issuer_uri are required; confidential clients require a credential.
		if e.ClientID == "" {
			return errors.New("client_id is required")
		}
		if !isPublicClient && credential == "" {
			return errors.New("client_secret is required")
		}
		if e.IssuerURI == "" {
			return errors.New("issuer_uri is required")
		}
		if !isAllowedHTTPSScheme(e.IssuerURI, skipHTTPSValidation) {
			return errors.New("issuer_uri must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)")
		}
		if !e.Discovery.EnableDiscovery {
			if e.Endpoints.TokenEndpoint == "" {
				return errors.New("token_endpoint is required when discovery is disabled")
			}
			if e.Endpoints.AuthorizeEndpoint == "" {
				return errors.New("authorize_endpoint is required when discovery is disabled")
			}
		}

	case OAuth2FlavorGoogle:
		// Google flavor: parse credential once, enrich derived fields, and validate host
		// consistency. enrichForGoogleFlavor mutates the entity (sets ClientID, Endpoints,
		// IssuerURI) so callers need not perform a separate enrichment step.
		if err := e.enrichForGoogleFlavor(credential); err != nil {
			return err
		}
	}

	if e.Discovery.MetadataURL != nil {
		if !isAllowedHTTPSScheme(*e.Discovery.MetadataURL, skipHTTPSValidation) {
			return errors.New("metadata_url must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)")
		}
	}

	for i, scope := range e.Scopes {
		if err := scope.Validate(); err != nil {
			return fmt.Errorf("scope %d: %w", i, err)
		}
	}

	if err := e.ValidateProtectedResources(); err != nil {
		return err
	}
	if err := validateAuthorizationParams(e.AuthorizationParams); err != nil {
		return err
	}
	if isPublicClient {
		if err := validatePublicClientOutboundConfiguration(e.AuthorizationParams, e.Endpoints.TokenEndpoint); err != nil {
			return err
		}
	}

	return nil
}

// validateIssuerURIAgainstTokenURI validates that the scheme+host of issuerURI matches
// the scheme+host of tokenURI. Used for google flavor to ensure the provided issuer_uri
// is consistent with the token_uri embedded in the service account JSON.
func validateIssuerURIAgainstTokenURI(issuerURI, tokenURI string) error {
	parsedIssuer, err := url.Parse(issuerURI)
	if err != nil {
		return fmt.Errorf("issuer_uri is invalid: %w", err)
	}
	parsedToken, err := url.Parse(tokenURI)
	if err != nil {
		return fmt.Errorf("token_uri in service account JSON is invalid: %w", err)
	}
	issuerBase := parsedIssuer.Scheme + "://" + parsedIssuer.Host
	tokenBase := parsedToken.Scheme + "://" + parsedToken.Host
	if issuerBase != tokenBase {
		return fmt.Errorf("issuer_uri host %q must match token_uri host %q from service account JSON", parsedIssuer.Host, parsedToken.Host)
	}
	return nil
}

// enrichForGoogleFlavor parses the Google service account JSON credential (already extracted
// as plaintext) and mutates the entity to populate derived fields: ClientID, Endpoints, and
// IssuerURI. This ensures parsing happens exactly once (during validation) rather than once
// in the HTTP handler and again in domain validation.
//
// Behaviour for IssuerURI:
//   - Non-empty: treated as a user-supplied override; host is validated against token_uri.
//   - Empty: derived from token_uri base URL (scheme + host).
func (e *ThirdpartyOAuth2ProviderEntity) enrichForGoogleFlavor(credential string) error {
	gsk, err := ParseGoogleServiceAccountKey(credential)
	if err != nil {
		return err
	}

	e.ClientID = id.ClientID(gsk.ClientID)
	e.Endpoints = OAuth2Endpoints{
		TokenEndpoint:     gsk.TokenURI,
		AuthorizeEndpoint: googleOAuthAuthURL,
	}

	if e.IssuerURI != "" {
		// User-supplied override: validate host consistency with token_uri.
		if err := validateIssuerURIAgainstTokenURI(e.IssuerURI, gsk.TokenURI); err != nil {
			return err
		}
	} else {
		// Derive issuer_uri from token_uri base URL.
		parsed, err := url.Parse(gsk.TokenURI)
		if err != nil {
			return fmt.Errorf("token_uri in service account JSON is not a valid URL: %w", err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("token_uri in service account JSON must be an absolute URL with scheme and host")
		}
		e.IssuerURI = parsed.Scheme + "://" + parsed.Host
	}

	return nil
}

// NormalizeProtectedResources canonicalizes protected_resources in place so lookups
// and uniqueness checks compare stable URI forms.
func (e *ThirdpartyOAuth2ProviderEntity) NormalizeProtectedResources() {
	for i, resourceURI := range e.ProtectedResources {
		e.ProtectedResources[i] = urivalidation.NormalizeResourceURI(resourceURI)
	}
}

// NormalizeAndValidateProtectedResource canonicalizes resourceURI and verifies it is an absolute URI.
func NormalizeAndValidateProtectedResource(resourceURI string) (string, error) {
	normalized := urivalidation.NormalizeResourceURI(resourceURI)
	if normalized == "" {
		return "", errors.New("cannot be empty")
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid URI: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("must be an absolute URL with scheme and host")
	}

	return normalized, nil
}

// ValidateProtectedResources validates all protected resource URIs are valid absolute URLs.
// Protected resources are optional; an empty slice is valid.
func (e *ThirdpartyOAuth2ProviderEntity) ValidateProtectedResources() error {
	resources := make(map[string]struct{}, len(e.ProtectedResources))
	for i, resourceURI := range e.ProtectedResources {
		normalized, err := NormalizeAndValidateProtectedResource(resourceURI)
		if err != nil {
			return fmt.Errorf("protected_resources[%d]: %w", i, err)
		}
		if _, exists := resources[normalized]; exists {
			return fmt.Errorf("protected_resources[%d]: duplicate resource %q", i, normalized)
		}
		resources[normalized] = struct{}{}
	}
	return nil
}

// RedactedCopy returns a deep copy of the entity with Secret replaced by a plaintext
// "REDACTED" value. Use for API responses and logs to comply with SR-003.
func (e *ThirdpartyOAuth2ProviderEntity) RedactedCopy() *ThirdpartyOAuth2ProviderEntity {
	if e == nil {
		return nil
	}
	c := e.Copy()
	c.Secret = NewPlaintextSecret("REDACTED")
	return c
}

// Copy creates a deep copy of the entity to prevent external mutation.
// The Secret is copied by value (immutable), with internal slices independently copied.
func (e *ThirdpartyOAuth2ProviderEntity) Copy() *ThirdpartyOAuth2ProviderEntity {
	if e == nil {
		return nil
	}

	result := &ThirdpartyOAuth2ProviderEntity{
		ID:                      e.ID,
		DisplayName:             e.DisplayName,
		ClientID:                e.ClientID,
		Secret:                  e.Secret, // Value type; internal ciphertext slice independently copied by Secret
		TokenEndpointAuthMethod: e.TokenEndpointAuthMethod,
		Flavor:                  e.Flavor,
		IssuerURI:               e.IssuerURI,
		Discovery: DiscoveryConfig{
			EnableDiscovery: e.Discovery.EnableDiscovery,
		},
		Endpoints: OAuth2Endpoints{
			TokenEndpoint:     e.Endpoints.TokenEndpoint,
			AuthorizeEndpoint: e.Endpoints.AuthorizeEndpoint,
		},
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		Version:   e.Version,
	}
	if e.CanonicalID != nil {
		canonicalID := *e.CanonicalID
		result.CanonicalID = &canonicalID
	}

	// Deep copy optional MetadataURL pointer
	if e.Discovery.MetadataURL != nil {
		metadataURL := *e.Discovery.MetadataURL
		result.Discovery.MetadataURL = &metadataURL
	}

	// Deep copy slices
	if e.Scopes != nil {
		result.Scopes = make([]OAuthScope, len(e.Scopes))
		for i, scope := range e.Scopes {
			result.Scopes[i] = scope.Copy()
		}
	}

	result.AuthorizationParams = maps.Clone(e.AuthorizationParams)

	if e.ProtectedResources != nil {
		result.ProtectedResources = make([]string, len(e.ProtectedResources))
		copy(result.ProtectedResources, e.ProtectedResources)
	}

	if e.ServiceRequirements != nil {
		result.ServiceRequirements = make([]ServiceRequirement, len(e.ServiceRequirements))
		for i, sr := range e.ServiceRequirements {
			result.ServiceRequirements[i] = sr.Copy()
		}
	}

	return result
}
