package oauth2server

import (
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type RefreshSessionDependencies struct {
	Agents      ports.AgentRepository
	Sessions    ports.RefreshSessionRepository
	Tokens      ports.RefreshTokenRepository
	Revocations ports.RefreshSessionRevocationRepository
	Coordinator ports.AuthorizationSessionCoordinator
	Clock       ports.AuthorizationClock
	Verifier    ports.UserDelegationVerifier
	Encryption  ports.EncryptionPort
	BranchKeys  ports.BranchKeyManager
	Policy      storage.RefreshSessionPolicy
}
