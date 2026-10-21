package model

import "errors"

// TokenEndpointAuthMethod identifies how an OAuth2 provider client authenticates at its token endpoint.
type TokenEndpointAuthMethod string

const (
	TokenEndpointAuthMethodNone              TokenEndpointAuthMethod = "none"
	TokenEndpointAuthMethodPrivateKeyJWT     TokenEndpointAuthMethod = "private_key_jwt"     // #nosec G101 -- OAuth2 authentication-method identifier, not a credential.
	TokenEndpointAuthMethodClientSecretBasic TokenEndpointAuthMethod = "client_secret_basic" // #nosec G101 -- OAuth2 method identifier, not a credential.
	TokenEndpointAuthMethodClientSecretPost  TokenEndpointAuthMethod = "client_secret_post"  // #nosec G101 -- OAuth2 method identifier, not a credential.
)

func (m TokenEndpointAuthMethod) Validate() error {
	switch m {
	case "", TokenEndpointAuthMethodNone, TokenEndpointAuthMethodPrivateKeyJWT,
		TokenEndpointAuthMethodClientSecretBasic, TokenEndpointAuthMethodClientSecretPost:
		return nil
	default:
		return errors.New("token_endpoint_auth_method is unsupported")
	}
}

func (m TokenEndpointAuthMethod) IsAbsent() bool {
	return m == ""
}

// IsCIMDConfidential reports whether the method uses broker-managed asymmetric client authentication.
func (m TokenEndpointAuthMethod) IsCIMDConfidential() bool {
	return m == TokenEndpointAuthMethodPrivateKeyJWT
}
