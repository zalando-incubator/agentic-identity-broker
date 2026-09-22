package model

import "errors"

// TokenEndpointAuthMethod identifies how an OAuth2 provider client authenticates at its token endpoint.
type TokenEndpointAuthMethod string

const (
	TokenEndpointAuthMethodNone          TokenEndpointAuthMethod = "none"
	TokenEndpointAuthMethodPrivateKeyJWT TokenEndpointAuthMethod = "private_key_jwt" // #nosec G101 -- OAuth2 authentication-method identifier, not a credential.
)

func (m TokenEndpointAuthMethod) Validate() error {
	switch m {
	case "", TokenEndpointAuthMethodNone, TokenEndpointAuthMethodPrivateKeyJWT:
		return nil
	default:
		return errors.New(`token_endpoint_auth_method: only "none" and "private_key_jwt" are accepted`)
	}
}

func (m TokenEndpointAuthMethod) IsAbsent() bool {
	return m == ""
}

// IsCIMDConfidential reports whether the method uses broker-managed asymmetric client authentication.
func (m TokenEndpointAuthMethod) IsCIMDConfidential() bool {
	return m == TokenEndpointAuthMethodPrivateKeyJWT
}
