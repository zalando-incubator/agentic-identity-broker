package jwtauth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainjwtauth "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestAuthenticate_OptionalClaimLogsCarryRequestContext(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(telemetry.NewContextHandler(slog.NewJSONHandler(&logs, nil)))
	evaluator, err := domainjwtauth.NewCELEvaluator(domainjwtauth.CELEvaluatorConfig{
		PrincipalExpression:   "claims.sub",
		DisplayNameExpression: "claims.missing_name",
		EmailExpression:       "claims.missing_email",
		PictureURLExpression:  "claims.missing_picture",
	}, logger)
	require.NoError(t, err)
	_, _, key := generateTestKeyPair(t)
	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig:    &ports.JWTConfig{Verification: "jwks", HeaderName: "Authorization"},
		CELEvaluator: evaluator,
		Logger:       logger,
		KeySet:       &fakeKeySet{set: publicSet(t, key)},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Shutdown(t.Context())) })
	token := createSignedJWT(t, map[string]interface{}{
		"sub":  "subject@example.com",
		"exp":  time.Now().Add(time.Hour),
		"name": "private-profile-name",
	}, key)
	ctx := security.WithSecurityContext(t.Context(), security.SecurityContext{
		TraceID:     "0123456789abcdef0123456789abcdef",
		Actor:       id.NewPrincipal("operator@example.com"),
		CallingPeer: "test-peer",
	})

	result, err := auth.Authenticate(ctx, token)

	require.NoError(t, err)
	assert.Equal(t, id.NewPrincipal("subject@example.com"), result.Principal)
	assert.Nil(t, result.DisplayName)
	assert.Nil(t, result.Email)
	assert.Nil(t, result.PictureURL)
	assert.NotContains(t, logs.String(), token)
	assert.NotContains(t, logs.String(), "private-profile-name")
	warnings := 0
	decoder := json.NewDecoder(&logs)
	for decoder.More() {
		var entry map[string]any
		require.NoError(t, decoder.Decode(&entry))
		if entry["level"] != "WARN" {
			continue
		}
		warnings++
		assert.Equal(t, "0123456789abcdef0123456789abcdef", entry["trace_id"])
		assert.Equal(t, "operator@example.com", entry["actor"])
		assert.Equal(t, "test-peer", entry["calling_peer"])
	}
	assert.Equal(t, 3, warnings)
}
