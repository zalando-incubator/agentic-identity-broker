package cimd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestNewClientKeySetServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "cimd.NewClientKeySetService: logger must not be nil", func() {
		NewClientKeySetService(nil, ports.CIMDClientAuthenticationConfig{}, nil, nil)
	})
}

func TestNewServiceRequiresLogger(t *testing.T) {
	assert.PanicsWithValue(t, "cimd.NewService: logger must not be nil", func() {
		NewService(nil, nil, nil, nil)
	})
}
