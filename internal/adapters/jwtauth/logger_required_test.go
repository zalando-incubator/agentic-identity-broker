package jwtauth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewJWXAuthenticatorRequiresLogger(t *testing.T) {
	authenticator, err := NewJWXAuthenticator(JWXAuthenticatorConfig{})

	require.EqualError(t, err, "logger is required")
	require.Nil(t, authenticator)
}
