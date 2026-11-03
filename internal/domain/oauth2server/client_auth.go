package oauth2server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ory/fosite"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const clientSecretBytes = 32

// ClientAuthService handles client authentication and credential management.
type ClientAuthService struct {
	credentialRepo ports.ClientCredentialRepository
	clientResolver ports.ClientResolver
	hasher         *Argon2Hasher
	logger         *slog.Logger
}

// NewClientAuthService creates a new ClientAuthService.
func NewClientAuthService(
	credentialRepo ports.ClientCredentialRepository,
	clientResolver ports.ClientResolver,
	hasher *Argon2Hasher,
	logger *slog.Logger,
) *ClientAuthService {
	return &ClientAuthService{
		credentialRepo: credentialRepo,
		clientResolver: clientResolver,
		hasher:         hasher,
		logger:         logger,
	}
}

// AuthenticatedClient represents a successfully authenticated client.
type AuthenticatedClient struct {
	Agent      *storage.Agent
	Credential *storage.ClientCredential
}

// Authenticate verifies client credentials and returns the associated agent.
func (s *ClientAuthService) Authenticate(ctx context.Context, agentID id.AgentID, secret string) (*AuthenticatedClient, error) {
	cred, err := s.credentialRepo.GetByAgentID(ctx, agentID)
	if err != nil {
		s.logResolverFailure(ctx, err)
		return nil, fosite.ErrInvalidClient
	}

	resolution, err := s.clientResolver.ResolveClient(ctx, id.ClientID(agentID.String()))
	if err != nil {
		s.logResolverFailure(ctx, err)
		return nil, fosite.ErrInvalidClient
	}

	if err := s.hasher.Compare(ctx, cred.SecretHash, secret); err != nil {
		return nil, fosite.ErrInvalidClient
	}

	return &AuthenticatedClient{
		Agent:      resolution.Agent,
		Credential: cred,
	}, nil
}

// logResolverFailure logs infrastructure-level errors from credential lookup or
// client resolution. Not-found storage errors are expected during authentication
// and silently ignored. ClientIDError with server_error code indicates an
// infrastructure failure that is logged.
func (s *ClientAuthService) logResolverFailure(ctx context.Context, err error) {
	var se *storage.StorageError
	if errors.As(err, &se) && se.Kind != storage.ErrorKindNotFound {
		s.logger.ErrorContext(ctx, "storage failure during client authentication",
			"storage_op", se.Operation,
			"storage_kind", string(se.Kind),
		)
		return
	}
	var clientErr *ports.ClientIDError
	if errors.As(err, &clientErr) && clientErr.Code == "server_error" {
		s.logger.ErrorContext(ctx, "infrastructure error during client resolution",
			"error", clientErr.Desc,
		)
	}
}

// GenerateCredentials generates a new broker client secret for the given agent.
// The client_id is always the agent's UUID string. Returns the plaintext secret
// (shown once to the user) and the credential for storage.
func (s *ClientAuthService) GenerateCredentials(agentID id.AgentID) (credential *storage.ClientCredential, plaintextSecret string, err error) {
	secretBytes := make([]byte, clientSecretBytes)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate secret: %w", err)
	}
	plaintextSecret = base64.RawURLEncoding.EncodeToString(secretBytes)

	secretHash, err := s.hasher.Hash(plaintextSecret)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash secret: %w", err)
	}

	credential = &storage.ClientCredential{
		ID:         id.NewCredentialID(),
		AgentID:    agentID,
		SecretHash: secretHash,
	}

	return credential, plaintextSecret, nil
}
