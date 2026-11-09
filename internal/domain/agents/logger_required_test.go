package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "agents.NewService: logger must not be nil", func() {
		NewService(nil, nil, nil, false, nil)
	})
}
