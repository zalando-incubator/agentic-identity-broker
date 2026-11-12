// Package fixture holds deliberate imports that ADR 027 forbids in ExtProc.
package fixture

import (
	_ "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry" // want depguard `ExtProc must not import internal/adapters/telemetry`
	_ "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"          // want depguard `ExtProc must not import internal/domain/id`
	_ "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"              // want depguard `ExtProc must not import internal/ports`
)
