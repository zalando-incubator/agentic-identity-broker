package ports

import (
	"context"
	"errors"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

var (
	ErrLastActiveKey           = errors.New("cannot remove the last active signing key")
	ErrCurrentKey              = errors.New("cannot remove the current signing key; promote another key first")
	ErrEffectiveCurrentKey     = errors.New("cannot remove a key that is still signing tokens; wait for the promoted key to activate or promote a different key")
	ErrCredentialAgentNotFound = errors.New("agent not found")
)

// SigningKeyManager is the port for signing key lifecycle operations.
// Implemented by domain/oauth2server.SigningKeyService.
type SigningKeyManager interface {
	GenerateAndStoreKey(ctx context.Context, algorithm string, makeCurrent bool) (*storage.SigningKey, error)
	BuildJWKS(ctx context.Context) (jwk.Set, error)
	ListKeys(ctx context.Context) ([]*storage.SigningKey, error)
	PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error)
	DeleteKey(ctx context.Context, kid id.KeyID) error
}

// SigningKeyBootstrapCoordinator serializes initial signing-key bootstrap work
// across broker instances that share the same backend.
type SigningKeyBootstrapCoordinator interface {
	// WithBootstrapLock serializes initial signing-key bootstrap across replicas.
	// The coordinator may derive a deadline-bounded context from ctx and pass it
	// to fn. Callers must not assume fn receives the original ctx unchanged.
	WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error
}

// CredentialGenerator is the port for generating broker client credentials.
// Implemented by domain/oauth2server.ClientAuthService.
type CredentialGenerator interface {
	GenerateCredentials(agentID id.AgentID) (credential *storage.ClientCredential, plaintextSecret string, err error)
}

type CredentialGenerationResult struct {
	Credential      *storage.ClientCredential
	PlaintextSecret string
	Rotated         bool
}

// ClientCredentialManager owns broker credential creation, replacement, and removal.
type ClientCredentialManager interface {
	Generate(ctx context.Context, agentID id.AgentID) (CredentialGenerationResult, error)
	Get(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error)
	Revoke(ctx context.Context, agentID id.AgentID) error
}
