package oauth2session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

func TestHandleCallback_StateValidationLogsCarryRequestContext(t *testing.T) {
	for _, reason := range []string{"expired", "principal_mismatch", "service_id_mismatch"} {
		t.Run(reason, func(t *testing.T) {
			var logs bytes.Buffer
			handler := &stateContextHandler{Handler: slog.NewJSONHandler(&logs, nil)}
			service, _ := newSecurityTestOAuth2SessionService(t, slog.New(handler), 0)
			principal := id.NewPrincipal("owner@example.com")
			serviceID := id.NewServiceID()
			claims := &oauth2session.OAuth2StateTokenClaims{
				Principal:    principal,
				ServiceID:    serviceID,
				PKCEVerifier: "state-verifier-marker",
				RedirectURI:  "https://broker.example.com/sessions",
				IssuedAt:     time.Now().Add(-time.Minute),
				ExpiresAt:    time.Now().Add(time.Minute),
			}
			expectedErr := oauth2session.ErrStateTokenExpired
			event := "session.oauth2.state_validation_failed"
			switch reason {
			case "expired":
				claims.ExpiresAt = time.Now().Add(-time.Second)
				event = "session.oauth2.state_expired"
			case "principal_mismatch":
				principal = id.NewPrincipal("other@example.com")
				expectedErr = oauth2session.ErrPrincipalMismatch
			case "service_id_mismatch":
				serviceID = id.NewServiceID()
				expectedErr = oauth2session.ErrServiceIDMismatch
			}
			state, err := service.CreateStateToken(claims)
			require.NoError(t, err)
			sc := security.SecurityContext{
				TraceID:     "0123456789abcdef0123456789abcdef",
				Actor:       principal,
				CallingPeer: "test-peer",
			}
			ctx := security.WithSecurityContext(t.Context(), sc)

			result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
				ServiceID: serviceID, State: state, Code: "callback-code-marker",
			})

			require.ErrorIs(t, err, expectedErr)
			assert.Nil(t, result)
			require.Len(t, handler.stateContexts, 1)
			captured, ok := security.FromContext(handler.stateContexts[0])
			require.True(t, ok)
			assert.Equal(t, sc, captured)
			assert.NotContains(t, logs.String(), state)
			assert.NotContains(t, logs.String(), "state-verifier-marker")
			assert.NotContains(t, logs.String(), "callback-code-marker")
			decoder := json.NewDecoder(&logs)
			found := false
			for decoder.More() {
				var entry map[string]any
				require.NoError(t, decoder.Decode(&entry))
				if entry["event"] == event {
					found = true
					if reason != "expired" {
						assert.Equal(t, reason, entry["reason"])
					}
				}
			}
			assert.True(t, found)
		})
	}
}

type stateContextHandler struct {
	slog.Handler
	stateContexts []context.Context
}

func (h *stateContextHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "event" && (attr.Value.String() == "session.oauth2.state_expired" || attr.Value.String() == "session.oauth2.state_validation_failed") {
			h.stateContexts = append(h.stateContexts, ctx)
		}
		return true
	})
	return h.Handler.Handle(ctx, record)
}
