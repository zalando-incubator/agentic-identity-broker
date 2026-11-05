package helpers

import (
	"net/url"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

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
