package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsufficientScopeChallenge(t *testing.T) {
	const recoveryURI = "https://broker.example.com/sessions"
	tests := []struct {
		name        string
		description string
		errorURI    string
		want        string
	}{
		{
			name:        "all attributes",
			description: "User session is unavailable. Please re-authenticate.",
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:     "empty description",
			errorURI: recoveryURI,
			want:     `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "quote in description",
			description: `Session "expired"`,
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "backslash in description",
			description: `Session \expired`,
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "CRLF in description",
			description: "Session expired\r\nInjected: value",
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "non-ASCII description",
			description: "Sitzung abgelaufen – bitte neu anmelden",
			errorURI:    recoveryURI,
			want:        `Bearer error="insufficient_scope", error_uri="https://broker.example.com/sessions"`,
		},
		{
			name:        "space in URI",
			description: "Session expired",
			errorURI:    "https://broker.example.com/sess ions",
			want:        `Bearer error="insufficient_scope", error_description="Session expired"`,
		},
		{
			name:        "quote in URI",
			description: "Session expired",
			errorURI:    `https://broker.example.com/"sessions`,
			want:        `Bearer error="insufficient_scope", error_description="Session expired"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, insufficientScopeChallenge(tt.description, tt.errorURI))
		})
	}
}
