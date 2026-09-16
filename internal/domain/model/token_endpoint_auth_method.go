package model

import "errors"

// TokenEndpointAuthMethod identifies how an OAuth2 provider client authenticates at its token endpoint.
type TokenEndpointAuthMethod string

const TokenEndpointAuthMethodNone TokenEndpointAuthMethod = "none"

func (m TokenEndpointAuthMethod) Validate() error {
	if m == "" || m == TokenEndpointAuthMethodNone {
		return nil
	}
	return errors.New(`token_endpoint_auth_method: only "none" is accepted`)
}

func (m TokenEndpointAuthMethod) IsAbsent() bool {
	return m == ""
}
