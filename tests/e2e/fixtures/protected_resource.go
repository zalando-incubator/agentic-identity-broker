package fixtures

import (
	"encoding/json"
	"net/http"
	"time"
)

// ProtectedResourceReply configures one response from a synthetic HTTPS host.
// Method and Path identify the request; Delay permits bounded latency scenarios.
type ProtectedResourceReply struct {
	Host    string
	Method  string
	Path    string
	Status  int
	Headers http.Header
	Body    string
	Delay   time.Duration
}

// ProtectedResourceScenario contains independent provider responses for one E2E case.
type ProtectedResourceScenario struct {
	ResourceURL     string
	IssuerURLs      []string
	RegistrationURL string
	ClientName      string
	Replies         []ProtectedResourceReply
}

const (
	protectedResourcePath              = "/mcp"
	protectedResourcePathMetadata      = "/.well-known/oauth-protected-resource/mcp"
	protectedResourceRootMetadata      = "/.well-known/oauth-protected-resource"
	protectedIssuerPathOAuthMetadata   = "/.well-known/oauth-authorization-server/tenant"
	protectedIssuerPathOpenIDMetadata  = "/.well-known/openid-configuration/tenant"
	protectedIssuerOtherOpenIDMetadata = "/tenant/.well-known/openid-configuration"
	protectedIssuerRootOAuthMetadata   = "/.well-known/oauth-authorization-server"
	protectedIssuerRootOpenIDMetadata  = "/.well-known/openid-configuration"
	protectedResourceClientName        = "Example Platform"
)

// CIMDOnlyProtectedResource advertises a path issuer that accepts hosted CIMD,
// with no dynamic-registration endpoint.
func CIMDOnlyProtectedResource() ProtectedResourceScenario {
	const (
		resourceHost = "cimd-only.example.test"
		issuerHost   = "cimd-issuer.example.test"
		resourceURL  = "https://cimd-only.example.test/mcp"
		issuerURL    = "https://cimd-issuer.example.test/tenant"
	)
	issuers := []string{issuerURL}
	return ProtectedResourceScenario{
		ResourceURL: resourceURL,
		IssuerURLs:  issuers,
		Replies: []ProtectedResourceReply{
			protectedResourceChallenge(resourceHost, `Bearer resource_metadata="`+"https://"+resourceHost+protectedResourcePathMetadata+`"`),
			protectedResourceJSONReply(resourceHost, http.MethodGet, protectedResourcePathMetadata, http.StatusOK, protectedResourceDocument(resourceURL, issuers)),
			protectedResourceStatusReply(resourceHost, http.MethodGet, protectedResourceRootMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodGet, protectedIssuerPathOAuthMetadata, http.StatusOK, protectedIssuerDocument(issuerURL, "", []string{"private_key_jwt"}, true)),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerPathOpenIDMetadata, http.StatusNotFound),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerOtherOpenIDMetadata, http.StatusNotFound),
			protectedResourceStatusReply(issuerHost, http.MethodGet, "/tenant/authorize", http.StatusFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/tenant/token", http.StatusOK, protectedResourceToken()),
		},
	}
}

// CIMDAndDCRProtectedResource offers both bootstrap methods. Hosted CIMD
// takes precedence, so its registration reply should remain unused.
func CIMDAndDCRProtectedResource() ProtectedResourceScenario {
	const (
		resourceHost    = "cimd-and-dcr.example.test"
		issuerHost      = "hybrid-issuer.example.test"
		resourceURL     = "https://cimd-and-dcr.example.test/mcp"
		issuerURL       = "https://hybrid-issuer.example.test/tenant"
		registrationURL = "https://hybrid-issuer.example.test/tenant/register"
	)
	issuers := []string{issuerURL}
	return ProtectedResourceScenario{
		ResourceURL:     resourceURL,
		IssuerURLs:      issuers,
		RegistrationURL: registrationURL,
		ClientName:      protectedResourceClientName,
		Replies: []ProtectedResourceReply{
			protectedResourceChallenge(resourceHost, `DPoP resource_metadata="`+"https://"+resourceHost+protectedResourcePathMetadata+`"`),
			protectedResourceJSONReply(resourceHost, http.MethodGet, protectedResourcePathMetadata, http.StatusOK, protectedResourceDocument(resourceURL, issuers)),
			protectedResourceStatusReply(resourceHost, http.MethodGet, protectedResourceRootMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodGet, protectedIssuerPathOAuthMetadata, http.StatusOK, protectedIssuerDocument(issuerURL, registrationURL, []string{"private_key_jwt", "client_secret_basic", "none"}, true)),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerPathOpenIDMetadata, http.StatusNotFound),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerOtherOpenIDMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/tenant/register", http.StatusCreated, protectedRegistrationDocument("hybrid-dcr-client", "client_secret_basic", "fixture-hybrid-secret")),
			protectedResourceStatusReply(issuerHost, http.MethodGet, "/tenant/authorize", http.StatusFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/tenant/token", http.StatusOK, protectedResourceToken()),
		},
	}
}

// ConfidentialDCRProtectedResource requires Basic registration, with a
// path-specific PRM 404 followed by a valid root PRM.
func ConfidentialDCRProtectedResource() ProtectedResourceScenario {
	const (
		resourceHost    = "files.example.test"
		issuerHost      = "login.example.test"
		resourceURL     = "https://files.example.test/mcp"
		issuerURL       = "https://login.example.test"
		registrationURL = "https://login.example.test/register"
	)
	issuers := []string{issuerURL}
	return ProtectedResourceScenario{
		ResourceURL:     resourceURL,
		IssuerURLs:      issuers,
		RegistrationURL: registrationURL,
		ClientName:      protectedResourceClientName,
		Replies: []ProtectedResourceReply{
			protectedResourceChallenge(resourceHost, `Bearer realm="files"`),
			protectedResourceStatusReply(resourceHost, http.MethodGet, protectedResourcePathMetadata, http.StatusNotFound),
			protectedResourceJSONReply(resourceHost, http.MethodGet, protectedResourceRootMetadata, http.StatusOK, protectedResourceDocument(resourceURL, issuers)),
			protectedResourceJSONReply(issuerHost, http.MethodGet, protectedIssuerRootOAuthMetadata, http.StatusOK, protectedIssuerDocument(issuerURL, registrationURL, []string{"client_secret_basic", "none"}, false)),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerRootOpenIDMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/register", http.StatusCreated, protectedRegistrationDocument("dcr-client-123", "client_secret_basic", "fixture-provider-secret")),
			protectedResourceStatusReply(issuerHost, http.MethodGet, "/authorize", http.StatusFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/token", http.StatusOK, protectedResourceToken()),
		},
	}
}

// PublicDCRProtectedResource offers only a public client, with PKCE S256.
func PublicDCRProtectedResource() ProtectedResourceScenario {
	const (
		resourceHost    = "public.example.test"
		issuerHost      = "public-login.example.test"
		resourceURL     = "https://public.example.test/mcp"
		issuerURL       = "https://public-login.example.test"
		registrationURL = "https://public-login.example.test/register"
	)
	issuers := []string{issuerURL}
	return ProtectedResourceScenario{
		ResourceURL:     resourceURL,
		IssuerURLs:      issuers,
		RegistrationURL: registrationURL,
		ClientName:      protectedResourceClientName,
		Replies: []ProtectedResourceReply{
			protectedResourceChallenge(resourceHost, `Bearer realm="public"`),
			protectedResourceJSONReply(resourceHost, http.MethodGet, protectedResourcePathMetadata, http.StatusOK, protectedResourceDocument(resourceURL, issuers)),
			protectedResourceStatusReply(resourceHost, http.MethodGet, protectedResourceRootMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodGet, protectedIssuerRootOAuthMetadata, http.StatusOK, protectedIssuerDocument(issuerURL, registrationURL, []string{"none"}, false)),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerRootOpenIDMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/register", http.StatusCreated, protectedRegistrationDocument("public-dcr-client", "none", "")),
			protectedResourceStatusReply(issuerHost, http.MethodGet, "/authorize", http.StatusFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/token", http.StatusOK, protectedResourceToken()),
		},
	}
}

// MultiIssuerProtectedResource requires an administrator to choose one of two
// separately hosted authorization servers before their metadata is fetched.
func MultiIssuerProtectedResource() ProtectedResourceScenario {
	const (
		resourceHost = "shared.example.test"
		firstHost    = "login-a.example.test"
		secondHost   = "login-b.example.test"
		resourceURL  = "https://shared.example.test/mcp"
		firstIssuer  = "https://login-a.example.test"
		secondIssuer = "https://login-b.example.test"
	)
	issuers := []string{firstIssuer, secondIssuer}
	return ProtectedResourceScenario{
		ResourceURL: resourceURL,
		IssuerURLs:  issuers,
		Replies: []ProtectedResourceReply{
			protectedResourceChallenge(resourceHost, `Bearer resource_metadata="`+"https://"+resourceHost+protectedResourcePathMetadata+`"`),
			protectedResourceJSONReply(resourceHost, http.MethodGet, protectedResourcePathMetadata, http.StatusOK, protectedResourceDocument(resourceURL, issuers)),
			protectedResourceStatusReply(resourceHost, http.MethodGet, protectedResourceRootMetadata, http.StatusNotFound),
			protectedResourceJSONReply(firstHost, http.MethodGet, protectedIssuerRootOAuthMetadata, http.StatusOK, protectedIssuerDocument(firstIssuer, "", []string{"private_key_jwt"}, true)),
			protectedResourceStatusReply(firstHost, http.MethodGet, protectedIssuerRootOpenIDMetadata, http.StatusNotFound),
			protectedResourceStatusReply(firstHost, http.MethodGet, "/authorize", http.StatusFound),
			protectedResourceJSONReply(firstHost, http.MethodPost, "/token", http.StatusOK, protectedResourceToken()),
			protectedResourceJSONReply(secondHost, http.MethodGet, protectedIssuerRootOAuthMetadata, http.StatusOK, protectedIssuerDocument(secondIssuer, "", []string{"private_key_jwt"}, true)),
			protectedResourceStatusReply(secondHost, http.MethodGet, protectedIssuerRootOpenIDMetadata, http.StatusNotFound),
			protectedResourceStatusReply(secondHost, http.MethodGet, "/authorize", http.StatusFound),
			protectedResourceJSONReply(secondHost, http.MethodPost, "/token", http.StatusOK, protectedResourceToken()),
		},
	}
}

// RejectedProtectedResource has valid DCR metadata but no configured broker
// client name. Its registration endpoint also rejects requests if a test sets
// ClientName, allowing either failure to be exercised without a new provider.
func RejectedProtectedResource() ProtectedResourceScenario {
	const (
		resourceHost    = "rejected.example.test"
		issuerHost      = "rejected-login.example.test"
		resourceURL     = "https://rejected.example.test/mcp"
		issuerURL       = "https://rejected-login.example.test"
		registrationURL = "https://rejected-login.example.test/register"
	)
	issuers := []string{issuerURL}
	return ProtectedResourceScenario{
		ResourceURL:     resourceURL,
		IssuerURLs:      issuers,
		RegistrationURL: registrationURL,
		Replies: []ProtectedResourceReply{
			protectedResourceChallenge(resourceHost, `Bearer realm="rejected"`),
			protectedResourceJSONReply(resourceHost, http.MethodGet, protectedResourcePathMetadata, http.StatusOK, protectedResourceDocument(resourceURL, issuers)),
			protectedResourceStatusReply(resourceHost, http.MethodGet, protectedResourceRootMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodGet, protectedIssuerRootOAuthMetadata, http.StatusOK, protectedIssuerDocument(issuerURL, registrationURL, []string{"client_secret_basic", "none"}, false)),
			protectedResourceStatusReply(issuerHost, http.MethodGet, protectedIssuerRootOpenIDMetadata, http.StatusNotFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/register", http.StatusForbidden, map[string]any{"error": "invalid_client_metadata"}),
			protectedResourceStatusReply(issuerHost, http.MethodGet, "/authorize", http.StatusFound),
			protectedResourceJSONReply(issuerHost, http.MethodPost, "/token", http.StatusOK, protectedResourceToken()),
		},
	}
}

func protectedResourceChallenge(host, challenge string) ProtectedResourceReply {
	return ProtectedResourceReply{
		Host:    host,
		Method:  http.MethodGet,
		Path:    protectedResourcePath,
		Status:  http.StatusUnauthorized,
		Headers: http.Header{"Www-Authenticate": {challenge}},
	}
}

func protectedResourceStatusReply(host, method, path string, status int) ProtectedResourceReply {
	return ProtectedResourceReply{Host: host, Method: method, Path: path, Status: status}
}

func protectedResourceJSONReply(host, method, path string, status int, document map[string]any) ProtectedResourceReply {
	body, err := json.Marshal(document)
	if err != nil {
		panic(err) // Fixture documents contain only JSON-compatible values.
	}
	return ProtectedResourceReply{
		Host:    host,
		Method:  method,
		Path:    path,
		Status:  status,
		Headers: http.Header{"Content-Type": {"application/json"}},
		Body:    string(body),
	}
}

func protectedResourceDocument(resourceURL string, issuers []string) map[string]any {
	return map[string]any{
		"resource":              resourceURL,
		"authorization_servers": issuers,
	}
}

func protectedIssuerDocument(issuerURL, registrationURL string, methods []string, supportsCIMD bool) map[string]any {
	document := map[string]any{
		"issuer":                                issuerURL,
		"authorization_endpoint":                issuerURL + "/authorize",
		"token_endpoint":                        issuerURL + "/token",
		"token_endpoint_auth_methods_supported": methods,
		"code_challenge_methods_supported":      []string{"S256"},
	}
	if registrationURL != "" {
		document["registration_endpoint"] = registrationURL
	}
	if supportsCIMD {
		document["client_id_metadata_document_supported"] = true
		document["token_endpoint_auth_signing_alg_values_supported"] = []string{"ES256"}
	}
	return document
}

func protectedRegistrationDocument(clientID, method, secret string) map[string]any {
	document := map[string]any{
		"client_id":                  clientID,
		"token_endpoint_auth_method": method,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		// The provider double copies the callback from the registration request.
		"redirect_uris": []string{},
	}
	if secret != "" {
		document["client_secret"] = secret
		document["client_secret_expires_at"] = 0
	}
	return document
}

func protectedResourceToken() map[string]any {
	return map[string]any{
		"access_token":  "fixture-access-token",
		"refresh_token": "fixture-refresh-token",
		"token_type":    "Bearer",
		"expires_in":    3600,
	}
}
