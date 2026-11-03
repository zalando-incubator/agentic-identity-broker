package model

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	leakPlaintext  = "hunter2-plaintext-canary"
	leakCiphertext = "ciphertext-canary-bytes"
)

func secretLeakCases() []struct {
	name   string
	secret Secret
} {
	return []struct {
		name   string
		secret Secret
	}{
		{name: "plaintext", secret: NewPlaintextSecret(leakPlaintext)},
		{name: "encrypted", secret: NewEncryptedSecret([]byte(leakCiphertext))},
		{name: "absent", secret: NewAbsentSecret()},
	}
}

func assertNoSecretLeak(t *testing.T, out string) {
	t.Helper()
	assert.NotContains(t, out, leakPlaintext)
	assert.NotContains(t, out, leakCiphertext)
	assert.NotContains(t, out, fmt.Sprintf("%v", []byte(leakCiphertext)))
	assert.NotContains(t, out, fmt.Sprintf("%x", []byte(leakCiphertext)))
	assert.NotContains(t, out, "plaintext:")
	assert.NotContains(t, out, "ciphertext:")
}

func TestSecret_SlogHandlersRedact(t *testing.T) {
	t.Parallel()
	handlers := map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
		"json": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
	}
	for handlerName, newHandler := range handlers {
		for _, tc := range secretLeakCases() {
			t.Run(handlerName+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				var buf bytes.Buffer
				slog.New(newHandler(&buf)).Info("x", "secret", tc.secret)

				out := buf.String()
				assertNoSecretLeak(t, out)
				assert.Contains(t, out, "REDACTED")
			})
		}
	}
}

func TestSecret_FmtVerbsRedact(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
		for _, tc := range secretLeakCases() {
			t.Run(verb+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				out := fmt.Sprintf(verb, tc.secret)
				assertNoSecretLeak(t, out)
				assert.Contains(t, out, "REDACTED")
			})
		}
	}
}

func TestSecret_EmbeddingEntityFmtRedacts(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"%v", "%+v"} {
		for _, tc := range secretLeakCases() {
			t.Run(verb+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				entity := ThirdpartyOAuth2ProviderEntity{Secret: tc.secret}
				assertNoSecretLeak(t, fmt.Sprintf(verb, entity))
				assertNoSecretLeak(t, fmt.Sprintf(verb, &entity))
			})
		}
	}
}
