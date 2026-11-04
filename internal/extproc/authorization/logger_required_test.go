package authorization_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

func TestNewOPAAuthorizerRequiresLogger(t *testing.T) {
	authorizer, err := authorization.NewOPAAuthorizer(&config.AuthorizationConfig{}, nil)

	require.EqualError(t, err, "opa authorizer: logger must not be nil")
	require.Nil(t, authorizer)
}
