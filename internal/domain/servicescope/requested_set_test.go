package servicescope

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func newTestResolver(services *fakeServices, sets *fakePermissionSets) *Resolver {
	return NewResolver(services, sets, testPublicURL+"/")
}

func TestResolveRequestedServices(t *testing.T) {
	github := testPublicURL + "/sessions/github"
	githubUUID := testPublicURL + "/sessions/" + githubID.String()
	jira := testPublicURL + "/sessions/jira"

	t.Run("ordinary scopes need no lookup", func(t *testing.T) {
		services := newFakeServices()
		set, err := newTestResolver(services, &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), "openid https://other.example.com/sessions/github", castAgent(), nil)
		require.NoError(t, err)
		assert.True(t, set.Empty())
		assert.Zero(t, services.calls)
	})

	t.Run("duplicate exact tokens and both identifier forms merge into one service", func(t *testing.T) {
		set, err := newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), "openid "+github+" "+githubUUID+" "+github+" "+jira, castAgent(), nil)
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{githubID, jiraID}, set.ServiceIDs())
		require.Len(t, set.Services, 2)
		assert.Equal(t, []string{github, githubUUID}, rawScopes(set.Services[0].Scopes))
		assert.Equal(t, map[string]id.ServiceID{github: githubID, githubUUID: githubID, jira: jiraID}, set.Bindings)
	})

	t.Run("eligibility without explicit requirements uses declared permission sets", func(t *testing.T) {
		agent := castAgent()
		agent.ServiceRequirements = nil
		set, err := newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), github, agent, nil)
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{githubID}, set.ServiceIDs())

		agent.PermissionSets = agent.PermissionSets[:1]
		_, err = newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), github, agent, nil)
		assert.ErrorIs(t, err, ErrServiceNotEligible)
	})

	t.Run("explicit requirements limit eligibility", func(t *testing.T) {
		agent := castAgent()
		agent.ServiceRequirements = agent.ServiceRequirements[:1]
		_, err := newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), jira, agent, nil)
		assert.ErrorIs(t, err, ErrServiceNotEligible)
		_, err = newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), testPublicURL+"/sessions/salesforce", castAgent(), nil)
		assert.ErrorIs(t, err, ErrServiceNotEligible)
	})

	t.Run("unknown and malformed scopes are permission failures", func(t *testing.T) {
		for _, scope := range []string{testPublicURL + "/sessions/unknown", testPublicURL + "/sessions/" + id.NewServiceID().String()} {
			_, err := newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), scope, castAgent(), nil)
			assert.ErrorIs(t, err, ErrUnknownService, scope)
		}
		_, err := newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), github+"/", castAgent(), nil)
		assert.ErrorIs(t, err, ErrMalformedScope)
		assert.True(t, IsPermissionFailure(err))
	})

	t.Run("repository failures are never treated as unknown", func(t *testing.T) {
		services := newFakeServices()
		services.err = errStorage
		_, err := newTestResolver(services, &fakePermissionSets{sets: castPermissionSets()}).Resolve(context.Background(), github, castAgent(), nil)
		assert.ErrorIs(t, err, errStorage)
		assert.False(t, IsPermissionFailure(err))

		agent := castAgent()
		agent.ServiceRequirements = nil
		_, err = newTestResolver(newFakeServices(), &fakePermissionSets{err: errStorage}).Resolve(context.Background(), github, agent, nil)
		assert.ErrorIs(t, err, errStorage)
		assert.False(t, IsPermissionFailure(err))
	})

	t.Run("sealed bindings pin identity across renames and reuse", func(t *testing.T) {
		services := newFakeServices()
		resolver := newTestResolver(services, &fakePermissionSets{sets: castPermissionSets()})
		set, err := resolver.Resolve(context.Background(), github, castAgent(), map[string]id.ServiceID{github: githubID})
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{githubID}, set.ServiceIDs())

		services.services[jiraID].CanonicalID = ptrTo("github")
		services.services[githubID].CanonicalID = ptrTo("github-production")
		_, err = resolver.Resolve(context.Background(), github, castAgent(), map[string]id.ServiceID{github: githubID})
		assert.ErrorIs(t, err, ErrBindingMismatch)

		_, err = resolver.Resolve(context.Background(), githubUUID, castAgent(), map[string]id.ServiceID{github: githubID})
		assert.ErrorIs(t, err, ErrBindingMismatch, "a virtual scope without a sealed binding fails closed")
		set, err = resolver.Resolve(context.Background(), githubUUID, castAgent(), map[string]id.ServiceID{githubUUID: githubID})
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{githubID}, set.ServiceIDs())
	})

	t.Run("assurance bindings are resolved even when their scope was narrowed away", func(t *testing.T) {
		set, err := newTestResolver(newFakeServices(), &fakePermissionSets{sets: castPermissionSets()}).ResolveBindings(context.Background(), map[string]id.ServiceID{github: githubID}, castAgent())
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{githubID}, set.ServiceIDs())
	})
}

func rawScopes(scopes []VirtualServiceScope) []string {
	raw := make([]string, len(scopes))
	for i, scope := range scopes {
		raw[i] = scope.Raw
	}
	return raw
}
