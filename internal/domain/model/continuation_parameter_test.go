package model

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContinuationParameterRedactsToken(t *testing.T) {
	continuation := ContinuationParameter{Token: "synthetic-sealed-JWE", Status: ContinuationPresent}
	assert.Equal(t, "[REDACTED]", fmt.Sprint(continuation))
	assert.Equal(t, "[REDACTED]", fmt.Sprintf("%#v", continuation))
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("continuation", "continuation", continuation)
	assert.NotContains(t, logs.String(), continuation.Token)
}
