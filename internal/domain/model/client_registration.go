package model

import "errors"

// ClientRegistration identifies who supplies the service's OAuth2 client identifier.
type ClientRegistration string

const (
	ClientRegistrationStatic ClientRegistration = "static"
	ClientRegistrationCIMD   ClientRegistration = "cimd"
)

func (r ClientRegistration) Validate() error {
	switch r {
	case ClientRegistrationStatic, ClientRegistrationCIMD:
		return nil
	default:
		return errors.New("client_registration must be static or cimd")
	}
}

// ResolveClientRegistration selects the source for an omitted registration field.
func ResolveClientRegistration(method TokenEndpointAuthMethod) ClientRegistration {
	if method == TokenEndpointAuthMethodPrivateKeyJWT {
		return ClientRegistrationCIMD
	}
	return ClientRegistrationStatic
}
