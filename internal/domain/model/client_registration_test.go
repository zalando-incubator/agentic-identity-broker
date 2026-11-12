package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientRegistration_Validate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		registration ClientRegistration
		valid        bool
	}{
		{"static", ClientRegistrationStatic, true},
		{"cimd", ClientRegistrationCIMD, true},
		{"empty", "", false},
		{"unknown", "dynamic", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.registration.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "client_registration")
			}
		})
	}
}

func TestResolveClientRegistration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		method TokenEndpointAuthMethod
		want   ClientRegistration
	}{
		{"", ClientRegistrationStatic},
		{TokenEndpointAuthMethodNone, ClientRegistrationStatic},
		{TokenEndpointAuthMethodPrivateKeyJWT, ClientRegistrationCIMD},
		{TokenEndpointAuthMethodClientSecretBasic, ClientRegistrationStatic},
		{TokenEndpointAuthMethodClientSecretPost, ClientRegistrationStatic},
	} {
		require.Equal(t, tc.want, ResolveClientRegistration(tc.method))
	}
}

func TestThirdpartyOAuth2ProviderEntity_ClientRegistrationClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		registration               ClientRegistration
		method                     TokenEndpointAuthMethod
		cimd, public, confidential bool
	}{
		{"static confidential", ClientRegistrationStatic, "", false, false, false},
		{"static public", ClientRegistrationStatic, TokenEndpointAuthMethodNone, false, false, false},
		{"CIMD confidential", ClientRegistrationCIMD, TokenEndpointAuthMethodPrivateKeyJWT, true, false, true},
		{"CIMD public", ClientRegistrationCIMD, TokenEndpointAuthMethodNone, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &ThirdpartyOAuth2ProviderEntity{ClientRegistration: tc.registration, TokenEndpointAuthMethod: tc.method}
			require.Equal(t, tc.cimd, e.IsCIMDClient())
			require.Equal(t, tc.public, e.IsCIMDPublicClient())
			require.Equal(t, tc.confidential, e.IsCIMDConfidentialClient())
		})
	}
}
