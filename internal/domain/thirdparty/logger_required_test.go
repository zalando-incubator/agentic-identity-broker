package thirdparty

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewThirdpartyOAuth2ProviderServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "thirdparty.NewThirdpartyOAuth2ProviderService: logger must not be nil", func() {
		NewThirdpartyOAuth2ProviderService(nil, nil, nil, nil, false, nil)
	})
}
