package cimdclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKeyServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "cimdclient.NewKeyService: logger must not be nil", func() {
		NewKeyService(nil, nil, nil, nil, nil)
	})
}

func TestNewAssertionSignerRequiresKeyService(t *testing.T) {
	assert.PanicsWithValue(t, "cimdclient.NewAssertionSigner: keyService must not be nil", func() {
		NewAssertionSigner(nil)
	})
}
