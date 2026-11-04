package oauth2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAgentClientResolverRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "oauth2.NewAgentClientResolver: logger must not be nil", func() {
		NewAgentClientResolver(nil, nil)
	})
}

func TestNewAgentClientResolverWithCIMDRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "oauth2.NewAgentClientResolverWithCIMD: logger must not be nil", func() {
		NewAgentClientResolverWithCIMD(nil, nil, nil)
	})
}
