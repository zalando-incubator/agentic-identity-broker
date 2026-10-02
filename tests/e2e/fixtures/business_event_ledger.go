package fixtures

import (
	"io/fs"
	"testing/fstest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const LedgerFixtureEventType = "agentic-identity-broker.fixture-recorded"

func LedgerPrincipal() id.Principal {
	return id.NewPrincipal("ledger-user@example.com")
}

func LedgerOtherPrincipal() id.Principal {
	return id.NewPrincipal("ledger-other-user@example.com")
}

func LedgerAdminID() string { return "ledger-admin@example.com" }

func LedgerGatewayClientID() id.ClientID {
	return id.NewClientID("ledger-trusted-gateway")
}

type LedgerWorkflowData struct {
	Principal id.Principal
	Agent     *storagedomain.Agent
	Service   *model.ThirdpartyOAuth2ProviderEntity
	Grant     *storagedomain.UserGrant
	Canaries  CredentialCanaries
}

func LedgerWorkflowFixtures(principal id.Principal) LedgerWorkflowData {
	agent := ValidAgent()
	service := GitHubService()
	return LedgerWorkflowData{
		Principal: principal,
		Agent:     agent,
		Service:   service,
		Grant:     ActiveGrant(principal.String(), agent.ID.String(), service.ID.String(), []string{"repo", "user"}),
		Canaries:  LedgerCredentialCanaries(),
	}
}

// CredentialCanaries keeps all seven classes explicit. Workflows may replace
// fields with the actual credentials returned by their real HTTP exchanges.
type CredentialCanaries struct {
	AccessToken       string
	RefreshToken      string
	ClientSecret      string
	ClientAssertion   string
	RawJWT            string
	AuthorizationCode string
	PKCEVerifier      string
}

func LedgerCredentialCanaries() CredentialCanaries {
	return CredentialCanaries{
		AccessToken:       "ledger-access-token-canary-048-2fb536becd8e49d9",
		RefreshToken:      "ledger-refresh-token-canary-048-6d124d48e8cb43ca",
		ClientSecret:      "ledger-client-secret-canary-048-779aa726379e4b27",
		ClientAssertion:   "ledger-client-assertion-canary-048-d1d7d43c56c84a45",
		RawJWT:            "ledger-raw-jwt-canary-048-542584b02c7549ae",
		AuthorizationCode: "ledger-authorization-code-canary-048-330718782bcd4d02",
		PKCEVerifier:      "ledger-pkce-verifier-canary-048-4e67ad75836f4aa9",
	}
}

// LedgerCatalogueTypes returns the reviewed catalogue, independently allocated
// for each scenario. The additional fixture type is deliberately excluded.
func LedgerCatalogueTypes() []string {
	return []string{
		"agentic-identity-broker.grant-created",
		"agentic-identity-broker.grant-updated",
		"agentic-identity-broker.grant-revoked",
		"agentic-identity-broker.grant-expired",
		"agentic-identity-broker.session-established",
		"agentic-identity-broker.session-refreshed",
		"agentic-identity-broker.session-refresh-failed",
		"agentic-identity-broker.session-terminated",
		"agentic-identity-broker.authorization-requested",
		"agentic-identity-broker.token-issued",
		"agentic-identity-broker.token-request-failed",
		"agentic-identity-broker.token-exchanged",
		"agentic-identity-broker.token-exchange-denied",
		"agentic-identity-broker.impersonation-granted",
		"agentic-identity-broker.impersonation-denied",
		"agentic-identity-broker.approval-requested",
		"agentic-identity-broker.approval-approved",
		"agentic-identity-broker.approval-denied",
		"agentic-identity-broker.approval-consumed",
		"agentic-identity-broker.approval-revoked",
		"agentic-identity-broker.approval-expired",
		"agentic-identity-broker.agent-registered",
		"agentic-identity-broker.agent-updated",
		"agentic-identity-broker.agent-deleted",
		"agentic-identity-broker.credential-generated",
		"agentic-identity-broker.credential-rotated",
		"agentic-identity-broker.credential-revoked",
		"agentic-identity-broker.signing-key-promoted",
	}
}

// LedgerFixtureSchemas appends one closed, offline-only type without redefining
// the published envelope. Its data requires a marker and rejects extra fields.
func LedgerFixtureSchemas() fs.FS {
	return fstest.MapFS{
		"fixture-recorded.schema.json": {Data: []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:agentic-identity-broker:events:v1:fixture-recorded",
  "title": "fixture-recorded",
  "description": "The fixture action completed.",
  "allOf": [
    {"$ref": "urn:agentic-identity-broker:events:v1:envelope"},
    {
      "properties": {
        "type": {"const": "agentic-identity-broker.fixture-recorded"},
        "outcome": {"const": "success"},
        "reason_user": {"const": "The fixture action completed."},
        "reason_admin": {"const": "The reviewed fixture action completed."},
        "data": {
          "type": "object",
          "additionalProperties": false,
          "required": ["marker"],
          "properties": {"marker": {"type": "string", "minLength": 1}}
        }
      }
    }
  ]
}`)},
	}
}
