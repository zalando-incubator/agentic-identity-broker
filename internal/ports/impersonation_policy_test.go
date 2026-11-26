package ports

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImpersonationPolicyInputRendersOnlyARedactedConstant(t *testing.T) {
	const marker = "SYNTHETIC-CLAIM-7f3a"
	populated := ImpersonationPolicyInput{
		"context": map[string]any{
			"subject_token": map[string]any{"sub": marker, "email": marker + "@example.com"},
		},
		"attributes": map[string]any{"request": map[string]any{"http": map[string]any{"headers": map[string]any{"x-tenant": marker}}}},
	}
	inputs := map[string]ImpersonationPolicyInput{
		"nil":       nil,
		"empty":     {},
		"populated": populated,
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			var asAny any = input
			_, isValuer := asAny.(slog.LogValuer)
			_, isStringer := asAny.(fmt.Stringer)
			_, isGoStringer := asAny.(fmt.GoStringer)
			assert.True(t, isValuer && isStringer && isGoStringer)

			pointer := &input
			for _, format := range []string{"%s", "%v", "%+v", "%#v"} {
				for _, value := range []any{input, pointer} {
					rendered := fmt.Sprintf(format, value)
					assert.NotContains(t, rendered, marker, format)
					assert.Contains(t, rendered, "[REDACTED]", format)
				}
			}

			for _, handler := range []func(*bytes.Buffer) slog.Handler{
				func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
				func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
			} {
				var buf bytes.Buffer
				slog.New(handler(&buf)).Info("evaluate", slog.Any("input", input), slog.Any("direct", input))
				assert.NotContains(t, buf.String(), marker)
				assert.Contains(t, buf.String(), "[REDACTED]")
			}
		})
	}

	assert.Equal(t, marker, populated["context"].(map[string]any)["subject_token"].(map[string]any)["sub"],
		"evaluation still receives the full typed document")
}
