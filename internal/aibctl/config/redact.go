package config

import (
	"maps"
	"slices"
	"strings"
)

const RedactedValue = "***REDACTED***"

var sensitiveHeaderMarkers = []string{"authorization", "token", "secret", "cookie", "password"}

// Redacted returns a deep copy of cfg with secrets masked for display.
func Redacted(cfg *Config) *Config {
	out := *cfg
	out.Contexts = slices.Clone(cfg.Contexts)
	for i := range out.Contexts {
		ctx := &out.Contexts[i]
		if ctx.Auth.Token != "" {
			ctx.Auth.Token = RedactedValue
		}
		ctx.Auth.TokenCommand = slices.Clone(ctx.Auth.TokenCommand)
		ctx.Headers = maps.Clone(ctx.Headers)
		for k := range ctx.Headers {
			if isSensitiveHeader(k) {
				ctx.Headers[k] = RedactedValue
			}
		}
	}
	return &out
}

func isSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range sensitiveHeaderMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
