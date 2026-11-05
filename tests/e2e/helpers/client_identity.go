package helpers

import (
	"encoding/json"
	"net/url"
	"os"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
)

func (j *OAuth2CredentialJourney) StartBrowserDocument(document map[string]any) error {
	encoded, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if err := os.WriteFile(j.ConfigPath, encoded, 0o600); err != nil {
		return err
	}
	j.Config, err = bootstrap.LoadCredentialConfiguration(j.ConfigPath, nil, nil)
	if err != nil {
		return err
	}
	j.Storage, err = bootstrap.NewStorageFactory(j.Logger).NewTestStorage()
	if err != nil {
		return err
	}
	factory := bootstrap.NewServerFactory(j.Config, j.Logger)
	builder, err := bootstrap.NewTestServerBuilder(j.Config, j.Storage, factory, j.Logger)
	if err != nil {
		return err
	}
	j.EndUser, err = builder.Build()
	if err != nil {
		return err
	}
	j.Application = j.EndUser.App()
	j.Admin, err = bootstrap.NewAdminTestServer(j.Application, j.Logger)
	return err
}

func ClientIdentityExchangeForm(journey *OAuth2CredentialJourney, agentID id.AgentID, principal string) (url.Values, error) {
	tokens, err := BuildTokenExchangeJWTs(journey.Upstream.GetPrivateKeyPEM(), journey.Upstream.URL(), "token-exchange-broker", principal, agentID.String(), "identity-gateway", time.Now())
	if err != nil {
		return nil, err
	}
	return url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {tokens.SubjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {tokens.ClientAssertion},
		"resource":              {journey.Upstream.URL() + "/resource"},
	}, nil
}
