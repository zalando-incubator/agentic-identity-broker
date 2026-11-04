package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandlerConstructorsRequireLogger(t *testing.T) {
	tests := []struct {
		name      string
		construct func()
	}{
		{"NewServicesHandler", func() { NewServicesHandler(nil, nil, nil) }},
		{"NewPermissionSetsHandler", func() { NewPermissionSetsHandler(nil, nil, nil) }},
		{"NewAgentsHandler", func() { NewAgentsHandler(nil, nil, nil, nil) }},
		{"NewCIMDClientKeysHandler", func() { NewCIMDClientKeysHandler(nil, nil) }},
		{"NewProtectedResourcesHandler", func() { NewProtectedResourcesHandler(nil, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.PanicsWithValue(t, "admin."+tt.name+": logger must not be nil", tt.construct)
		})
	}
}
