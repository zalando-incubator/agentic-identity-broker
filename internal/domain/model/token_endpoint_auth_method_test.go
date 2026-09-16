package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenEndpointAuthMethod_Validate(t *testing.T) {
	tests := []struct {
		name    string
		method  TokenEndpointAuthMethod
		wantErr bool
	}{
		{"absent method is valid", "", false},
		{"none is valid", TokenEndpointAuthMethodNone, false},
		{"client secret basic is invalid", "client_secret_basic", true},
		{"client secret post is invalid", "client_secret_post", true},
		{"case variant is invalid", "NONE", true},
		{"whitespace variant is invalid", "none ", true},
		{"unknown value is invalid", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.method.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), `only "none" is accepted`)
				assert.Contains(t, err.Error(), string(TokenEndpointAuthMethodNone))
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestTokenEndpointAuthMethod_IsAbsent(t *testing.T) {
	tests := []struct {
		name   string
		method TokenEndpointAuthMethod
		want   bool
	}{
		{"empty method is absent", "", true},
		{"none method is not absent", TokenEndpointAuthMethodNone, false},
		{"unknown method is not absent", "client_secret_basic", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.method.IsAbsent())
		})
	}
}
