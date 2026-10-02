package enduser

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

func TestTokenParseLedgerRecordsBeforeHTTPError(t *testing.T) {
	for _, boundary := range []string{"invalid-content-type", "missing-local-secret"} {
		t.Run(boundary, func(t *testing.T) {
			store := &ledgerfixture.Store{}
			outcomes := oauth2.NewTokenOutcomeService(nil, nil, store.Recorder(t))
			agentID := id.NewAgentID()
			serve := func() *httptest.ResponseRecorder {
				writer := httptest.NewRecorder()
				store.BeforeCommit = func() {
					require.Equal(t, http.StatusOK, writer.Code, "HTTP error status must wait for the recording commit")
					require.Empty(t, writer.Body.String(), "HTTP error bytes must wait for the recording commit")
				}
				if boundary == "invalid-content-type" {
					handler := &OAuth2TokenHandler{Outcomes: outcomes}
					request := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader("credential-canary"))
					request.Header.Set("Content-Type", "application/json")
					handler.ServeHTTP(writer, request)
				} else {
					strategy := NewLocalGrantStrategy(fixedMinting(nil, errors.New("must not reach minting")), outcomes, nil)
					strategy.HandleTokenGrant(writer, httptest.NewRequest(http.MethodPost, "/oauth2/token", nil), "client_credentials", map[string][]string{"client_id": {agentID.String()}}, &ports.TokenGrantResolution{AgentID: agentID})
				}
				return writer
			}
			writer := serve()
			require.Equal(t, http.StatusBadRequest, writer.Code)
			require.JSONEq(t, `{"error":"invalid_request","error_description":"`+map[string]string{"invalid-content-type": "invalid Content-Type: expected application/x-www-form-urlencoded", "missing-local-secret": "client_id and client_secret are required"}[boundary]+`"}`, writer.Body.String())
			require.Len(t, store.Events, 1)
			require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
			require.Nil(t, store.Events[0].Subject)
			require.Nil(t, store.Events[0].Actor.ID, "parsed client IDs are not verified callers")
			store.AppendError = errors.New("recording unavailable")
			writer = serve()
			require.Equal(t, http.StatusInternalServerError, writer.Code)
			require.NotContains(t, writer.Body.String(), "credential-canary")
			require.Len(t, store.Events, 1, "a failed append cannot claim another recorded outcome")
		})
	}
}
