package oauth2server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClientAssertionVerifierRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "oauth2server.NewClientAssertionVerifier: logger must not be nil", func() {
		NewClientAssertionVerifier(nil, nil, nil, nil, nil)
	})
}
