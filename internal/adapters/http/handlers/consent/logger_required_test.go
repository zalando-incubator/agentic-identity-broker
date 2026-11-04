package consent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandlerConstructorsRequireLogger(t *testing.T) {
	tests := []struct {
		name      string
		construct func()
	}{
		{"NewUserInfoHandler", func() { NewUserInfoHandler(nil) }},
		{"NewAgentDetailHandler", func() { NewAgentDetailHandler(nil, nil, nil) }},
		{"NewGrantsHandler", func() { NewGrantsHandler(nil, nil, nil) }},
		{"NewAgentsHandler", func() { NewAgentsHandler(nil, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.PanicsWithValue(t, "consent."+tt.name+": logger must not be nil", tt.construct)
		})
	}
}
