package enduser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCIMDMetadataHandlerRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "enduser.NewCIMDMetadataHandler: logger must not be nil", func() {
		NewCIMDMetadataHandler(nil, nil)
	})
}
