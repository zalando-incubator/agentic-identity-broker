package config

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
)

func TestRedact(t *testing.T) {
	evidence.ForTest(t, "configuration.summary.redaction")
	tests := []struct {
		name  string
		key   string
		value interface{}
		want  interface{}
	}{
		{"identity broker prefix", "IDENTITY_BROKER_API_KEY", "secret123", RedactedValue},
		{"identity broker lowercase", "identity_broker_api_key", "secret123", RedactedValue},
		{"password keyword", "DB_PASSWORD", "secret123", RedactedValue},
		{"secret keyword", "API_SECRET", "secret123", RedactedValue},
		{"token keyword", "AUTH_TOKEN", "secret123", RedactedValue},
		{"key keyword", "PRIVATE_KEY", "secret123", RedactedValue},
		{"credential keyword", "USER_CREDENTIAL", "secret123", RedactedValue},
		{"auth keyword", "OAUTH_TOKEN", "secret123", RedactedValue},
		{"non-sensitive", "LOG_LEVEL", "info", "info"},
		{"non-sensitive number", "PORT", 8080, 8080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.key, tt.value)
			if got != tt.want {
				t.Errorf("Redact(%q, %v) = %v, want %v", tt.key, tt.value, got, tt.want)
			}
		})
	}
}

func TestIsSensitive(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"identity broker prefix", "IDENTITY_BROKER_API_KEY", true},
		{"identity broker lowercase", "identity_broker_api_key", true},
		{"password keyword", "DB_PASSWORD", true},
		{"secret keyword", "API_SECRET", true},
		{"token keyword", "AUTH_TOKEN", true},
		{"key keyword", "PRIVATE_KEY", true},
		{"credential keyword", "USER_CREDENTIAL", true},
		{"auth keyword", "OAUTH_TOKEN", true},
		{"non-sensitive", "LOG_LEVEL", false},
		{"non-sensitive host", "SERVER_HOST", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSensitive(tt.key)
			if got != tt.want {
				t.Errorf("IsSensitive(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
