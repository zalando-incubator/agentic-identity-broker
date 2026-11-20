package consent

import "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/bizevent"

var GrantRevoked = bizevent.MustDefine("consent.grant.revoked", bizevent.OwnerVisible())
