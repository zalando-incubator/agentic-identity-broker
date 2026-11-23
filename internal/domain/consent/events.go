package consent

import "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/domainevent"

var GrantRevoked = domainevent.MustDefine("consent.grant.revoked", domainevent.OwnerVisible())
