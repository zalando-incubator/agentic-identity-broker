package helpers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
)

func ReconcileAgent(server *bootstrap.TestServer, principal string, externalID id.ExternalID, body any, prefer string) (*http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return server.DirectRequest(http.MethodPut, "/api/agents?external_id="+url.QueryEscape(string(externalID)),
		principal, map[string]string{"Content-Type": "application/json", "Prefer": prefer}, bytes.NewReader(encoded))
}
