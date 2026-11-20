package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

const githubScope = "https://broker.example.com/sessions/github"

var githubServiceID = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")

func TestServiceScopeBindingsValidate(t *testing.T) {
	uuidScope := "https://broker.example.com/sessions/" + githubServiceID.String()
	tests := []struct {
		name     string
		bindings ServiceScopeBindings
		scope    string
		inScope  bool
		valid    bool
	}{
		{"legacy nil bindings", nil, "openid offline_access", true, true},
		{"code binding within scope", ServiceScopeBindings{githubScope: githubServiceID}, "openid " + githubScope, true, true},
		{"UUID-form binding", ServiceScopeBindings{uuidScope: githubServiceID}, uuidScope, true, true},
		{"code binding outside scope", ServiceScopeBindings{githubScope: githubServiceID}, "openid", true, false},
		{"refresh assurance binding may outlive narrowing", ServiceScopeBindings{githubScope: githubServiceID}, "openid", false, true},
		{"zero service", ServiceScopeBindings{githubScope: {}}, githubScope, true, false},
		{"key without identifier", ServiceScopeBindings{"https://broker.example.com/sessions/": githubServiceID}, "https://broker.example.com/sessions/", true, false},
		{"key with malformed identifier", ServiceScopeBindings{githubScope + "/": githubServiceID}, githubScope + "/", true, false},
		{"relative key", ServiceScopeBindings{"/sessions/github": githubServiceID}, "/sessions/github", true, false},
		{"empty map", ServiceScopeBindings{}, "openid", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bindings.Validate(tt.scope, tt.inScope)
			if tt.valid {
				assert.NoError(t, err)
				return
			}
			var storageErr *StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, ErrorKindValidation, storageErr.Kind)
		})
	}
}

func TestServiceScopeBindingsCopyAndEquality(t *testing.T) {
	original := ServiceScopeBindings{githubScope: githubServiceID}
	clone := original.Clone()
	clone[githubScope] = id.NewServiceID()
	assert.Equal(t, githubServiceID, original[githubScope], "clones isolate caller mutation")
	assert.Nil(t, ServiceScopeBindings(nil).Clone())

	assert.True(t, original.Equal(ServiceScopeBindings{githubScope: githubServiceID}))
	assert.True(t, ServiceScopeBindings(nil).Equal(ServiceScopeBindings{}))
	assert.False(t, original.Equal(nil), "a successor cannot drop an assurance binding")
	assert.False(t, original.Equal(ServiceScopeBindings{githubScope: githubServiceID, "https://broker.example.com/sessions/jira": id.NewServiceID()}), "nor add one")
	assert.False(t, original.Equal(ServiceScopeBindings{githubScope: id.NewServiceID()}), "nor rebind one")
}

func TestScopeSubset(t *testing.T) {
	assert.True(t, ScopeSubset("openid", "openid offline_access "+githubScope))
	assert.True(t, ScopeSubset("", "openid"))
	assert.False(t, ScopeSubset("openid "+githubScope, "openid offline_access"))
}

func TestRecordsValidateBindings(t *testing.T) {
	code := validAuthorizationCodeForBindings()
	code.Scope = "openid"
	code.ServiceScopeBindings = ServiceScopeBindings{githubScope: githubServiceID}
	assert.Error(t, code.Validate(), "code binding keys must be granted scopes")
	code.Scope = "openid " + githubScope
	assert.NoError(t, code.Validate())

	session := validRefreshSessionForBindings()
	session.Scope = "openid"
	session.ServiceScopeBindings = ServiceScopeBindings{githubScope: githubServiceID}
	assert.NoError(t, session.Validate(), "refresh assurance bindings may outlive narrowing")
	session.ServiceScopeBindings = ServiceScopeBindings{githubScope: {}}
	assert.Error(t, session.Validate())
}

func validAuthorizationCodeForBindings() *AuthorizationCode {
	return &AuthorizationCode{
		CodeHash: "hash", AgentID: id.NewAgentID(), Principal: "alice@example.com",
		RedirectURI: "https://client.example.com/cb", CodeChallenge: "challenge", ExpiresAt: time.Now().Add(time.Minute),
	}
}

func validRefreshSessionForBindings() *RefreshTokenSession {
	return &RefreshTokenSession{
		Signature: "signature", RequestID: "request", AgentID: id.NewAgentID(), ClientID: "client",
		Principal: "alice@example.com", ExpiresAt: time.Now().Add(time.Minute),
	}
}
