package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// DiscoveryError reports a safe failure code. Issuer choices are returned only
// when a matching protected resource advertises multiple validated issuers.
type DiscoveryError struct {
	Code                 string
	AuthorizationServers []string
}

func (e *DiscoveryError) Error() string { return e.Code }

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type authorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	JWKSURI                           *string  `json:"jwks_uri"`
	RegistrationEndpoint              *string  `json:"registration_endpoint"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	TokenEndpointAuthSigningAlgs      []string `json:"token_endpoint_auth_signing_alg_values_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

type discoveredProviderConfiguration struct {
	issuer    string
	endpoints model.OAuth2Endpoints
	method    model.ClientBootstrapMethod
	auth      model.TokenEndpointAuthMethod
	clientID  id.ClientID
	secret    model.Secret
}

func discoveryFailure(code string) error { return &DiscoveryError{Code: code} }

// discoveryReadFailure translates untrusted transport errors into the fixed API
// vocabulary. Neither network errors nor response bodies reach callers or logs.
func discoveryReadFailure(ctx context.Context, err error, document string) error {
	switch ctx.Err() {
	case context.Canceled:
		return context.Canceled
	case context.DeadlineExceeded:
		return discoveryFailure("timeout")
	}
	switch {
	case errors.Is(err, ports.ErrOAuthDiscoveryTimeout), errors.Is(err, context.DeadlineExceeded):
		return discoveryFailure("timeout")
	case errors.Is(err, ports.ErrOAuthDiscoveryUnsafeDestination):
		return discoveryFailure("unsafe_destination")
	case errors.Is(err, ports.ErrOAuthDiscoveryResponseTooLarge):
		return discoveryFailure("response_too_large")
	case errors.Is(err, ports.ErrOAuthDiscoveryNotFound):
		return discoveryFailure(document + "_not_found")
	case errors.Is(err, ports.ErrOAuthDiscoveryInvalidResponse):
		return discoveryFailure(document + "_invalid")
	default:
		return discoveryFailure(document + "_unavailable")
	}
}

func discoveryURL(raw, invalidCode string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return discoveryFailure(invalidCode)
	}
	if err := model.ValidatePublicHTTPSURL(raw); err != nil {
		return discoveryFailure("unsafe_destination")
	}
	return nil
}

// splitChallenges respects quoted commas and quoted-pair escapes. The port
// preserves individual WWW-Authenticate fields so duplicate values remain
// visible here, even when the server sends two separate headers.
func splitChallenges(header string) ([]string, bool) {
	var segments []string
	start := 0
	quoted, escaped := false, false
	for index := range len(header) {
		switch {
		case escaped:
			escaped = false
		case quoted && header[index] == '\\':
			escaped = true
		case header[index] == '"':
			quoted = !quoted
		case header[index] == ',' && !quoted:
			segments = append(segments, strings.TrimSpace(header[start:index]))
			start = index + 1
		}
	}
	if quoted || escaped {
		return nil, false
	}
	return append(segments, strings.TrimSpace(header[start:])), true
}

func authTokenLength(value string) int {
	for i := range len(value) {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			continue
		}
		return i
	}
	return len(value)
}

func challengeParameter(segment string) (name, value string, quoted, valid bool) {
	length := authTokenLength(segment)
	if length == 0 {
		return "", "", false, false
	}
	name = segment[:length]
	rest := strings.TrimSpace(segment[length:])
	if !strings.HasPrefix(rest, "=") {
		return name, "", false, false
	}
	rest = strings.TrimSpace(rest[1:])
	if len(rest) == 0 {
		return name, "", false, false
	}
	if rest[0] != '"' {
		return name, rest, false, authTokenLength(rest) == len(rest)
	}
	var result strings.Builder
	chunkStart := 1
	for i := 1; i < len(rest); i++ {
		switch rest[i] {
		case '"':
			if strings.TrimSpace(rest[i+1:]) != "" {
				return name, "", false, false
			}
			if chunkStart == 1 {
				return name, rest[1:i], true, true
			}
			result.WriteString(rest[chunkStart:i])
			return name, result.String(), true, true
		case '\\':
			result.WriteString(rest[chunkStart:i])
			i++
			if i >= len(rest) {
				return name, "", false, false
			}
			if rest[i] < ' ' || rest[i] == 127 {
				return name, "", false, false
			}
			result.WriteByte(rest[i])
			chunkStart = i + 1
		default:
			if rest[i] < ' ' || rest[i] == 127 {
				return name, "", false, false
			}
		}
	}
	return name, "", false, false
}

// challengeResourceMetadata selects a single validated URI from an eligible
// challenge. Any malformed or unsupported resource_metadata is terminal;
// callers must not try well-known URLs after that error.
func challengeResourceMetadata(status int, headers []string) (string, error) {
	metadataURL := ""
	for _, header := range headers {
		segments, valid := splitChallenges(header)
		if !valid {
			if strings.Contains(strings.ToLower(header), "resource_metadata") {
				return "", discoveryFailure("resource_metadata_invalid")
			}
			continue
		}
		scheme := ""
		for _, segment := range segments {
			if segment == "" {
				if strings.Contains(strings.ToLower(header), "resource_metadata") {
					return "", discoveryFailure("resource_metadata_invalid")
				}
				continue
			}
			length := authTokenLength(segment)
			if length == 0 {
				if strings.Contains(strings.ToLower(segment), "resource_metadata") {
					return "", discoveryFailure("resource_metadata_invalid")
				}
				continue
			}
			word := segment[:length]
			rest := strings.TrimSpace(segment[length:])
			if !strings.HasPrefix(rest, "=") {
				if strings.Contains(strings.ToLower(word), "resource_metadata") {
					return "", discoveryFailure("resource_metadata_invalid")
				}
				scheme = word
				segment = rest
				if segment == "" {
					continue
				}
			}
			name, value, quoted, valid := challengeParameter(segment)
			if !valid {
				if strings.Contains(strings.ToLower(segment), "resource_metadata") {
					return "", discoveryFailure("resource_metadata_invalid")
				}
				continue
			}
			if !strings.EqualFold(name, "resource_metadata") {
				if strings.Contains(strings.ToLower(name), "resource_metadata") {
					return "", discoveryFailure("resource_metadata_invalid")
				}
				continue
			}
			if metadataURL != "" || status != 401 || !quoted || !strings.EqualFold(scheme, "Bearer") && !strings.EqualFold(scheme, "DPoP") {
				return "", discoveryFailure("resource_metadata_invalid")
			}
			if err := discoveryURL(value, "resource_metadata_invalid"); err != nil {
				return "", err
			}
			metadataURL = value
		}
	}
	return metadataURL, nil
}

// decodeUniqueJSONMembers keeps duplicate metadata claims visible until they
// can be rejected, rather than letting json.Unmarshal silently choose the last.
func decodeUniqueJSONMembers(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("invalid JSON object")
	}
	members := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("invalid JSON member")
		}
		if _, duplicate := members[name]; duplicate {
			return nil, errors.New("duplicate JSON member")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members[name] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return nil, errors.New("invalid JSON object")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing JSON value")
	}
	return members, nil
}

func discoveryDocument(ctx context.Context, client ports.OAuthDiscoveryClient, locations []string, kind string) ([]byte, error) {
	for index, location := range locations {
		if err := ctx.Err(); err != nil {
			return nil, discoveryReadFailure(ctx, err, kind)
		}
		body, err := client.GetJSON(ctx, location)
		if err := ctx.Err(); err != nil {
			return nil, discoveryReadFailure(ctx, err, kind)
		}
		if err == nil {
			return body, nil
		}
		if errors.Is(err, ports.ErrOAuthDiscoveryNotFound) && index+1 < len(locations) {
			continue
		}
		return nil, discoveryReadFailure(ctx, err, kind)
	}
	return nil, discoveryFailure(kind + "_not_found")
}

func (s *ThirdpartyOAuth2ProviderService) resourceIssuer(ctx context.Context, resourceURL, requestedIssuer string) (string, error) {
	status, challenges, err := s.oauthDiscoveryClient.Probe(ctx, resourceURL)
	if err := ctx.Err(); err != nil {
		return "", discoveryReadFailure(ctx, err, "resource_metadata")
	}
	if err != nil {
		if errors.Is(err, ports.ErrOAuthDiscoveryNotFound) {
			return "", discoveryFailure("resource_metadata_unavailable")
		}
		return "", discoveryReadFailure(ctx, err, "resource_metadata")
	}
	challengeURL, err := challengeResourceMetadata(status, challenges)
	if err != nil {
		return "", err
	}
	var locations []string
	if challengeURL != "" {
		locations = []string{challengeURL}
	} else {
		resource, _ := url.Parse(resourceURL) // request shape was validated before the probe
		root := resource.Scheme + "://" + resource.Host + "/.well-known/oauth-protected-resource"
		if path := resource.EscapedPath(); path != "" && path != "/" {
			locations = append(locations, root+path)
		}
		locations = append(locations, root)
	}
	body, err := discoveryDocument(ctx, s.oauthDiscoveryClient, locations, "resource_metadata")
	if err != nil {
		return "", err
	}
	if _, err := decodeUniqueJSONMembers(body); err != nil {
		return "", discoveryFailure("resource_metadata_invalid")
	}
	var metadata protectedResourceMetadata
	if err := json.Unmarshal(body, &metadata); err != nil || metadata.Resource == "" {
		return "", discoveryFailure("resource_metadata_invalid")
	}
	if metadata.Resource != resourceURL {
		return "", discoveryFailure("resource_mismatch")
	}
	if len(metadata.AuthorizationServers) == 0 {
		return "", discoveryFailure("authorization_server_missing")
	}
	seen := make(map[string]struct{}, len(metadata.AuthorizationServers))
	for _, issuer := range metadata.AuthorizationServers {
		if err := discoveryURL(issuer, "resource_metadata_invalid"); err != nil {
			return "", err
		}
		parsed, _ := url.Parse(issuer)
		if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
			return "", discoveryFailure("resource_metadata_invalid")
		}
		if _, duplicate := seen[issuer]; duplicate {
			return "", discoveryFailure("resource_metadata_invalid")
		}
		seen[issuer] = struct{}{}
	}
	if requestedIssuer != "" {
		if _, advertised := seen[requestedIssuer]; !advertised {
			return "", discoveryFailure("issuer_not_advertised")
		}
		return requestedIssuer, nil
	}
	if len(metadata.AuthorizationServers) > 1 {
		return "", &DiscoveryError{Code: "issuer_selection_required", AuthorizationServers: metadata.AuthorizationServers}
	}
	return metadata.AuthorizationServers[0], nil
}

func (s *ThirdpartyOAuth2ProviderService) issuerMetadata(ctx context.Context, issuer string) (authorizationServerMetadata, model.OAuth2Endpoints, error) {
	parsed, _ := url.Parse(issuer) // every advertised issuer passed public HTTPS validation
	origin := parsed.Scheme + "://" + parsed.Host
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	locations := []string{
		origin + "/.well-known/oauth-authorization-server" + path,
		origin + "/.well-known/openid-configuration" + path,
	}
	if path != "" {
		locations = append(locations, origin+path+"/.well-known/openid-configuration")
	}
	body, err := discoveryDocument(ctx, s.oauthDiscoveryClient, locations, "authorization_server_metadata")
	if err != nil {
		return authorizationServerMetadata{}, model.OAuth2Endpoints{}, err
	}
	members, err := decodeUniqueJSONMembers(body)
	if err != nil {
		return authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("authorization_server_metadata_invalid")
	}
	var metadata authorizationServerMetadata
	if err := json.Unmarshal(body, &metadata); err != nil || metadata.Issuer == "" {
		return authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("authorization_server_metadata_invalid")
	}
	// An explicitly null methods list is not an omitted RFC 8414 field.
	if metadata.TokenEndpointAuthMethodsSupported == nil {
		if _, present := members["token_endpoint_auth_methods_supported"]; present {
			return authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("authorization_server_metadata_invalid")
		}
	}
	if metadata.Issuer != issuer {
		return authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("issuer_mismatch")
	}
	for _, endpoint := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint} {
		if err := discoveryURL(endpoint, "authorization_server_metadata_invalid"); err != nil {
			return authorizationServerMetadata{}, model.OAuth2Endpoints{}, err
		}
	}
	if metadata.JWKSURI != nil {
		if err := discoveryURL(*metadata.JWKSURI, "authorization_server_metadata_invalid"); err != nil {
			return authorizationServerMetadata{}, model.OAuth2Endpoints{}, err
		}
	}
	if metadata.RegistrationEndpoint != nil {
		if err := discoveryURL(*metadata.RegistrationEndpoint, "authorization_server_metadata_invalid"); err != nil {
			return authorizationServerMetadata{}, model.OAuth2Endpoints{}, err
		}
	}
	tokenURL, _ := url.Parse(metadata.TokenEndpoint)
	query, err := url.ParseQuery(tokenURL.RawQuery)
	if err != nil {
		return authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("authorization_server_metadata_invalid")
	}
	for name := range query {
		if model.IsDiscoveryResourceParamName(name) {
			return authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("authorization_server_metadata_invalid")
		}
	}
	endpoints := model.OAuth2Endpoints{AuthorizeEndpoint: metadata.AuthorizationEndpoint, TokenEndpoint: metadata.TokenEndpoint}
	if metadata.JWKSURI != nil {
		endpoints.JWKsURI = *metadata.JWKSURI
	}
	return metadata, endpoints, nil
}

func containsDiscoveryValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// readProtectedResourceMetadata validates the resource and its selected issuer
// before any client method is chosen or remote registration is attempted.
func (s *ThirdpartyOAuth2ProviderService) readProtectedResourceMetadata(ctx context.Context, resourceURL, requestedIssuer string) (string, authorizationServerMetadata, model.OAuth2Endpoints, error) {
	if s.oauthDiscoveryClient == nil {
		return "", authorizationServerMetadata{}, model.OAuth2Endpoints{}, discoveryFailure("resource_metadata_unavailable")
	}
	issuer, err := s.resourceIssuer(ctx, resourceURL, requestedIssuer)
	if err != nil {
		return "", authorizationServerMetadata{}, model.OAuth2Endpoints{}, err
	}
	metadata, endpoints, err := s.issuerMetadata(ctx, issuer)
	if err != nil {
		return "", authorizationServerMetadata{}, model.OAuth2Endpoints{}, err
	}
	return issuer, metadata, endpoints, nil
}

func supportsCIMD(metadata authorizationServerMetadata) bool {
	return metadata.ClientIDMetadataDocumentSupported &&
		containsDiscoveryValue(metadata.TokenEndpointAuthMethodsSupported, string(model.TokenEndpointAuthMethodPrivateKeyJWT)) &&
		containsDiscoveryValue(metadata.TokenEndpointAuthSigningAlgs, "ES256")
}

func (s *ThirdpartyOAuth2ProviderService) requireCIMDKey(ctx context.Context) error {
	if s.cimdKeyReadiness == nil {
		return discoveryFailure("cimd_unavailable")
	}
	if err := s.cimdKeyReadiness.RequireUsablePublishedKey(ctx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return discoveryReadFailure(ctx, ctxErr, "cimd")
		}
		return discoveryFailure("cimd_unavailable")
	}
	return nil
}

func supportsDCRMethod(metadata authorizationServerMetadata, method model.TokenEndpointAuthMethod) bool {
	if metadata.RegistrationEndpoint == nil {
		return false
	}
	return supportsDCRAuthentication(metadata, method)
}

func supportsDCRAuthentication(metadata authorizationServerMetadata, method model.TokenEndpointAuthMethod) bool {
	methods := metadata.TokenEndpointAuthMethodsSupported
	if methods == nil { // RFC 8414 default when the field is omitted.
		methods = []string{string(model.TokenEndpointAuthMethodClientSecretBasic)}
	}
	if !containsDiscoveryValue(methods, string(method)) {
		return false
	}
	return method != model.TokenEndpointAuthMethodNone || containsDiscoveryValue(metadata.CodeChallengeMethodsSupported, "S256")
}

// discoverProtectedResource performs creation-only CIMD > Basic > POST > public
// selection. A selected method's failure is terminal, not a fallback.
func (s *ThirdpartyOAuth2ProviderService) discoverProtectedResource(ctx context.Context, resourceURL, requestedIssuer string, serviceID id.ServiceID) (discoveredProviderConfiguration, error) {
	issuer, metadata, endpoints, err := s.readProtectedResourceMetadata(ctx, resourceURL, requestedIssuer)
	if err != nil {
		return discoveredProviderConfiguration{}, err
	}
	if supportsCIMD(metadata) {
		if err := s.requireCIMDKey(ctx); err != nil {
			return discoveredProviderConfiguration{}, err
		}
		return discoveredProviderConfiguration{
			issuer: issuer, endpoints: endpoints,
			method: model.ClientBootstrapCIMD, auth: model.TokenEndpointAuthMethodPrivateKeyJWT,
		}, nil
	}
	for _, method := range []model.TokenEndpointAuthMethod{
		model.TokenEndpointAuthMethodClientSecretBasic,
		model.TokenEndpointAuthMethodClientSecretPost,
		model.TokenEndpointAuthMethodNone,
	} {
		if !supportsDCRMethod(metadata, method) {
			continue
		}
		clientID, secret, err := s.registerDCRClient(ctx, *metadata.RegistrationEndpoint, serviceID, method)
		if err != nil {
			return discoveredProviderConfiguration{}, err
		}
		return discoveredProviderConfiguration{
			issuer: issuer, endpoints: endpoints, method: model.ClientBootstrapDCR,
			auth: method, clientID: clientID, secret: secret,
		}, nil
	}
	return discoveredProviderConfiguration{}, discoveryFailure("no_compatible_client_method")
}

// decodeRegistrationMember distinguishes an absent optional member from a
// supplied member with an incompatible JSON type (including null).
func decodeRegistrationMember(document map[string]json.RawMessage, name string, target any) (bool, error) {
	value, present := document[name]
	if !present {
		return false, nil
	}
	if string(value) == "null" || json.Unmarshal(value, target) != nil {
		return true, discoveryFailure("client_registration_invalid")
	}
	return true, nil
}

func validateRegistrationResponse(body []byte, callback string, method model.TokenEndpointAuthMethod) (id.ClientID, model.Secret, error) {
	invalid := func() (id.ClientID, model.Secret, error) {
		return "", model.Secret{}, discoveryFailure("client_registration_invalid")
	}
	document, err := decodeUniqueJSONMembers(body)
	if err != nil {
		return invalid()
	}
	var clientID string
	if present, err := decodeRegistrationMember(document, "client_id", &clientID); err != nil || !present || strings.TrimSpace(clientID) == "" {
		return invalid()
	}
	var returnedMethod model.TokenEndpointAuthMethod
	if _, err := decodeRegistrationMember(document, "token_endpoint_auth_method", &returnedMethod); err != nil {
		return invalid()
	}
	if _, present := document["token_endpoint_auth_method"]; present && returnedMethod != method {
		return invalid()
	}
	var redirects []string
	if present, err := decodeRegistrationMember(document, "redirect_uris", &redirects); err != nil || present && !containsDiscoveryValue(redirects, callback) {
		return invalid()
	}
	var grants []string
	if present, err := decodeRegistrationMember(document, "grant_types", &grants); err != nil || present && !containsDiscoveryValue(grants, "authorization_code") {
		return invalid()
	}
	var expiry int64
	if _, err := decodeRegistrationMember(document, "client_secret_expires_at", &expiry); err != nil || expiry != 0 {
		return invalid()
	}
	var credential string
	if _, err := decodeRegistrationMember(document, "client_secret", &credential); err != nil {
		return invalid()
	}
	if method == model.TokenEndpointAuthMethodNone {
		if credential != "" {
			return invalid()
		}
		return id.ClientID(clientID), model.NewAbsentSecret(), nil
	}
	if credential == "" {
		return invalid()
	}
	return id.ClientID(clientID), model.NewPlaintextSecret(credential), nil
}

func (s *ThirdpartyOAuth2ProviderService) registerDCRClient(ctx context.Context, endpoint string, serviceID id.ServiceID, method model.TokenEndpointAuthMethod) (id.ClientID, model.Secret, error) {
	if strings.TrimSpace(s.dcrClientName) == "" {
		return "", model.Secret{}, discoveryFailure("client_name_unconfigured")
	}
	publicURL, err := url.Parse(s.cimdPublicURL)
	if err != nil || model.ValidatePublicHTTPSURL(s.cimdPublicURL) != nil || publicURL.RawQuery != "" || publicURL.ForceQuery || serviceID.IsZero() {
		return "", model.Secret{}, discoveryFailure("client_registration_invalid")
	}
	callback := strings.TrimRight(s.cimdPublicURL, "/") + "/api/third-party/" + serviceID.String() + "/oauth2/callback"
	request := struct {
		RedirectURIs            []string                      `json:"redirect_uris"`
		ClientName              string                        `json:"client_name"`
		ApplicationType         string                        `json:"application_type"`
		GrantTypes              []string                      `json:"grant_types"`
		ResponseTypes           []string                      `json:"response_types"`
		TokenEndpointAuthMethod model.TokenEndpointAuthMethod `json:"token_endpoint_auth_method"`
	}{[]string{callback}, s.dcrClientName, "web", []string{"authorization_code", "refresh_token"}, []string{"code"}, method}
	body, err := json.Marshal(request)
	if err != nil {
		return "", model.Secret{}, discoveryFailure("client_registration_invalid")
	}
	response, err := s.oauthDiscoveryClient.PostJSON(ctx, endpoint, body)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", model.Secret{}, discoveryReadFailure(ctx, ctxErr, "client_registration")
	}
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrOAuthDiscoveryTimeout), errors.Is(err, context.DeadlineExceeded):
			return "", model.Secret{}, discoveryFailure("timeout")
		case errors.Is(err, ports.ErrOAuthDiscoveryUnsafeDestination):
			return "", model.Secret{}, discoveryFailure("unsafe_destination")
		case errors.Is(err, ports.ErrOAuthDiscoveryResponseTooLarge):
			return "", model.Secret{}, discoveryFailure("response_too_large")
		case errors.Is(err, ports.ErrOAuthDiscoveryInvalidResponse):
			return "", model.Secret{}, discoveryFailure("client_registration_invalid")
		default:
			return "", model.Secret{}, discoveryFailure("client_registration_rejected")
		}
	}
	return validateRegistrationResponse(response, callback, method)
}
