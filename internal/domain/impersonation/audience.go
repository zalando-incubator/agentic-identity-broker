package impersonation

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Target is the registered broker agent selected by the impersonation audience suffix.
type Target struct {
	Agent *storage.Agent
}

// ValidateAudiencePrefix validates the routing-only audience prefix.
func ValidateAudiencePrefix(prefix string) error {
	u, err := url.Parse(prefix)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(u.Path, "/") {
		return fmt.Errorf("audience_prefix must be an absolute HTTP(S) URI with host and no userinfo, query, fragment, or trailing slash")
	}
	return nil
}

// ResolveTarget determines whether an audience activates impersonation and resolves its target.
func (s *Service) ResolveTarget(ctx context.Context, audiences []string) (*Target, bool, error) {
	if len(audiences) != 1 {
		return nil, false, nil
	}
	if audiences[0] == s.audiencePrefix {
		return nil, true, invalidTargetSuffix()
	}
	if !strings.HasPrefix(audiences[0], s.audiencePrefix+"/") {
		return nil, false, nil
	}

	suffix := audiences[0][len(s.audiencePrefix)+1:]

	var agent *storage.Agent
	var lookupErr error
	if targetID, parseErr := id.ParseAgentID(suffix); parseErr == nil {
		if targetID.String() != suffix {
			return nil, true, invalidTargetSuffix()
		}
		agent, lookupErr = s.agents.Get(ctx, targetID)
	} else {
		if canonical.Validate(&suffix) != nil {
			return nil, true, invalidTargetSuffix()
		}
		agent, lookupErr = s.canonicalAgents.GetByCanonicalID(ctx, suffix)
	}
	if lookupErr != nil {
		if ports.IsNotFoundErr(lookupErr) {
			return nil, true, originError(ctx, tokenexchange.NewInvalidTargetErrorWithDetails("audience target agent was not found", "target_agent_not_found"), lookupErr, tokenexchange.StageIdentityResolution, tokenexchange.DetailAgentMissing)
		}
		return nil, true, originError(ctx, serverError("audience target agent lookup failed", "target_agent_lookup_failed"), lookupErr, tokenexchange.StageIdentityResolution, tokenexchange.DetailAgentRepositoryUnavailable)
	}
	if agent == nil || agent.ID.IsZero() {
		return nil, true, originError(ctx, serverError("audience target agent lookup returned an invalid agent", "target_agent_lookup_failed"), nil, tokenexchange.StageIdentityResolution, tokenexchange.DetailAgentInvalid)
	}
	return &Target{Agent: agent}, true, nil
}

func invalidTargetSuffix() *tokenexchange.TokenExchangeError {
	return invalidRequest("audience target must be an agent UUID or canonical ID", "audience_target_invalid")
}
