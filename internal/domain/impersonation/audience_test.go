package impersonation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
)

func TestResolveTarget(t *testing.T) {
	targetID := id.NewAgentID()
	canonicalID := "impersonation-target"
	targetAgent := &storage.Agent{ID: targetID, CanonicalID: &canonicalID, DisplayName: "Target", Description: "Target agent"}
	lookupErr := errors.New("storage unavailable")

	tests := []struct {
		name            string
		audiences       []string
		lookup          func(context.Context, id.AgentID) (*storage.Agent, error)
		canonicalLookup func(context.Context, string) (*storage.Agent, error)
		activated       bool
		code            string
	}{
		{name: "absent audience", activated: false},
		{name: "different audience", audiences: []string{"https://elsewhere.example.com"}, activated: false},
		{name: "multiple audiences", audiences: []string{"https://broker/impersonation/" + targetID.String(), "https://elsewhere.example.com"}, activated: false},
		{name: "bare prefix", audiences: []string{"https://broker/impersonation"}, activated: true, code: tokenexchange.InvalidRequestError},
		{name: "empty suffix", audiences: []string{"https://broker/impersonation/"}, activated: true, code: tokenexchange.InvalidRequestError},
		{name: "unknown canonical target", audiences: []string{"https://broker/impersonation/not-a-uuid"}, canonicalLookup: func(context.Context, string) (*storage.Agent, error) { return nil, ports.ErrNotFound }, activated: true, code: tokenexchange.InvalidTargetError},
		{name: "invalid canonical grammar", audiences: []string{"https://broker/impersonation/not a canonical id"}, activated: true, code: tokenexchange.InvalidRequestError},
		{name: "oversized canonical suffix", audiences: []string{"https://broker/impersonation/" + strings.Repeat("a", 129)}, activated: true, code: tokenexchange.InvalidRequestError},
		{name: "noncanonical suffix", audiences: []string{"https://broker/impersonation/" + targetID.String()[0:35] + "A"}, activated: true, code: tokenexchange.InvalidRequestError},
		{name: "multi-segment suffix", audiences: []string{"https://broker/impersonation/" + targetID.String() + "/extra"}, activated: true, code: tokenexchange.InvalidRequestError},
		{name: "missing target", audiences: []string{"https://broker/impersonation/" + targetID.String()}, lookup: func(context.Context, id.AgentID) (*storage.Agent, error) { return nil, ports.ErrNotFound }, activated: true, code: tokenexchange.InvalidTargetError},
		{name: "lookup failure", audiences: []string{"https://broker/impersonation/" + targetID.String()}, lookup: func(context.Context, id.AgentID) (*storage.Agent, error) { return nil, lookupErr }, activated: true, code: tokenexchange.ServerErrorCode},
		{name: "canonical target", audiences: []string{"https://broker/impersonation/impersonation-target"}, canonicalLookup: func(_ context.Context, got string) (*storage.Agent, error) {
			assert.Equal(t, "impersonation-target", got)
			return targetAgent, nil
		}, activated: true},
		{name: "canonical lookup failure", audiences: []string{"https://broker/impersonation/impersonation-target"}, canonicalLookup: func(context.Context, string) (*storage.Agent, error) { return nil, lookupErr }, activated: true, code: tokenexchange.ServerErrorCode},
		{name: "registered target", audiences: []string{"https://broker/impersonation/" + targetID.String()}, lookup: func(_ context.Context, got id.AgentID) (*storage.Agent, error) {
			assert.Equal(t, targetID, got)
			return targetAgent, nil
		}, activated: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewService(testImpersonationConfig(), func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return stubJWKSProvider{}, nil
			}, stubAgentRepository{get: tc.lookup, getCanonical: tc.canonicalLookup}, &stubIssuer{}, 0, nil, allowDelegationVerifier{}, "https://broker.example.com", ledgerfixture.NewRecorder())
			require.NoError(t, err)

			target, activated, err := svc.ResolveTarget(context.Background(), tc.audiences)
			assert.Equal(t, tc.activated, activated)
			if tc.code == "" {
				require.NoError(t, err)
				if tc.activated {
					require.NotNil(t, target)
					assert.Same(t, targetAgent, target.Agent)
				} else {
					assert.Nil(t, target)
				}
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.code, codeOf(t, err))
			assert.Nil(t, target)
		})
	}
}

func TestValidateAudiencePrefix(t *testing.T) {
	for _, prefix := range []string{"", "relative", "ftp://broker.example.com", "https:///missing-host", "https://user@broker.example.com", "https://broker.example.com?", "https://broker.example.com?query=x", "https://broker.example.com#fragment", "https://broker.example.com/"} {
		assert.Error(t, ValidateAudiencePrefix(prefix), prefix)
	}
	assert.NoError(t, ValidateAudiencePrefix("https://broker.example.com/impersonation"))
}
