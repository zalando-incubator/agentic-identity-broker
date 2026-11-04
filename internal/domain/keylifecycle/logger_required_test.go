package keylifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEngineRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "keylifecycle.NewEngine: logger must not be nil", func() {
		NewEngine(nil, nil, nil, nil, nil)
	})
}
