package impersonation

import (
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"net/url"
)

// Request is the parsed, validated impersonation request. It is built from the token endpoint
// form after impersonation activates (audience match). It never retains multi-valued or
// unrelated parameters.
type Request struct {
	ClientAssertion    string
	ActorToken         string
	SubjectToken       string
	SubjectTokenType   string
	RequestedTokenType string
	Scope              string
	Scopes             []string
}

// ParseRequest validates the impersonation form and returns a Request or an invalid_request
// error. Activation (audience match) has already occurred in the handler; this enforces the
// per-parameter contract (FR-002a/003/003a/003b/003c). Legacy token-exchange defaults
// (empty→access-token, empty→jwt-bearer) MUST NOT apply — every type is validated explicitly.
func ParseRequest(form map[string][]string) (*Request, error) {
	// FR-002a: impersonation accepts optional scope but never accepts resource.
	if present(form, tokenexchange.ResourceParam) {
		return nil, invalidRequest("resource parameter is not permitted for impersonation")
	}

	clientAssertion, err := singleton(form, tokenexchange.ClientAssertionParam)
	if err != nil {
		return nil, err
	}
	clientAssertionType, err := singleton(form, tokenexchange.ClientAssertionTypeParam)
	if err != nil {
		return nil, err
	}
	if clientAssertionType != tokenexchange.JWTBearerType {
		return nil, invalidRequest("client_assertion_type must be the JWT bearer type")
	}

	actorToken, err := singleton(form, tokenexchange.ActorTokenParam)
	if err != nil {
		return nil, err
	}
	actorTokenType, err := singleton(form, tokenexchange.ActorTokenTypeParam)
	if err != nil {
		return nil, err
	}
	if actorTokenType != JWTTokenType {
		return nil, invalidRequest("actor_token_type must be the RFC 8693 JWT token type")
	}

	subjectToken, err := singleton(form, tokenexchange.SubjectTokenParam)
	if err != nil {
		return nil, err
	}
	subjectTokenType, err := singleton(form, tokenexchange.SubjectTokenTypeParam)
	if err != nil {
		return nil, err
	}
	// FR-003a/FR-003d: subject_token_type must be present, singleton, and non-empty (enforced above).
	// Rule evaluation requires the RFC 8693 JWT type and selects the signed or unverified path from
	// the matching rule's verification mode. Unsupported types are rejected with invalid_request.

	// FR-003c: requested_token_type is optional; when present it must be the access-token type.
	requestedTokenType := ""
	scope := url.Values(form).Get(tokenexchange.ScopeParam)
	if present(form, "requested_token_type") {
		requestedTokenType, err = singleton(form, "requested_token_type")
		if err != nil {
			return nil, err
		}
		if requestedTokenType != AccessTokenType {
			return nil, invalidRequest("requested_token_type must be the access-token type")
		}
	}

	return &Request{
		ClientAssertion:    clientAssertion,
		ActorToken:         actorToken,
		SubjectToken:       subjectToken,
		SubjectTokenType:   subjectTokenType,
		RequestedTokenType: requestedTokenType,
		Scope:              scope,
		Scopes:             oauth2.SplitScope(scope),
	}, nil
}

// singleton returns the single non-empty value for key or an invalid_request error when the
// parameter is missing, empty, or repeated (FR-003).
func singleton(form map[string][]string, key string) (string, error) {
	values := form[key]
	if len(values) == 0 {
		return "", invalidRequest(key + " is required")
	}
	if len(values) > 1 {
		return "", invalidRequest(key + " must not be repeated")
	}
	if values[0] == "" {
		return "", invalidRequest(key + " must not be empty")
	}
	return values[0], nil
}

// present reports whether the parameter key appears in the form at all — including a bare or
// empty value (e.g. "scope="). Impersonation forbids resource while scope is optional and
// validated in Service.Impersonate (FR-002a/003b/003c).
func present(form map[string][]string, key string) bool {
	_, ok := form[key]
	return ok
}
