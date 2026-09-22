package oauth2server

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelError})), buf
}

type mockCredentialRepo struct {
	getByAgentIDFunc  func(context.Context, id.AgentID) (*storage.ClientCredential, error)
	getByClientIDFunc func(context.Context, id.ClientID) (*storage.ClientCredential, error)
}

func (m *mockCredentialRepo) Create(_ context.Context, _ *storage.ClientCredential) error {
	return nil
}
func (m *mockCredentialRepo) GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	if m.getByAgentIDFunc != nil {
		return m.getByAgentIDFunc(ctx, agentID)
	}
	return nil, storage.NewStorageError("mockCredentialRepo.GetByAgentID", storage.ErrorKindNotFound, nil, "not found")
}
func (m *mockCredentialRepo) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.ClientCredential, error) {
	if m.getByClientIDFunc != nil {
		return m.getByClientIDFunc(ctx, clientID)
	}
	return nil, nil
}
func (m *mockCredentialRepo) Delete(_ context.Context, _ id.AgentID) error { return nil }
func (m *mockCredentialRepo) Rotate(_ context.Context, _ id.AgentID, _ *storage.ClientCredential) error {
	return nil
}

func TestArgon2Hasher_HashAndCompare(t *testing.T) {
	hasher := &Argon2Hasher{}

	t.Run("hash then compare succeeds", func(t *testing.T) {
		secret := "test-secret-value-1234"
		hash, err := hasher.Hash(secret)
		require.NoError(t, err)
		require.NotEmpty(t, hash)

		err = hasher.Compare(hash, secret)
		assert.NoError(t, err)
	})

	t.Run("wrong secret fails comparison", func(t *testing.T) {
		secret := "correct-secret"
		hash, err := hasher.Hash(secret)
		require.NoError(t, err)

		err = hasher.Compare(hash, "wrong-secret")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
	})

	t.Run("PHC string format is correct", func(t *testing.T) {
		hash, err := hasher.Hash("test")
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(hash, "$argon2id$v="))
		parts := strings.Split(hash, "$")
		assert.Len(t, parts, 6) // empty, argon2id, version, params, salt, hash
		assert.Equal(t, "argon2id", parts[1])
		assert.Contains(t, parts[3], "m=65536,t=3,p=4")
	})

	t.Run("different hashes for same secret", func(t *testing.T) {
		secret := "same-secret"
		hash1, err := hasher.Hash(secret)
		require.NoError(t, err)
		hash2, err := hasher.Hash(secret)
		require.NoError(t, err)

		// Different salts should produce different hashes
		assert.NotEqual(t, hash1, hash2)

		// Both should verify against the same secret
		assert.NoError(t, hasher.Compare(hash1, secret))
		assert.NoError(t, hasher.Compare(hash2, secret))
	})

	t.Run("invalid PHC string fails", func(t *testing.T) {
		err := hasher.Compare("not-a-valid-phc-string", "secret")
		assert.Error(t, err)
	})

	t.Run("PHC hash length must match the configured key length", func(t *testing.T) {
		hash, err := hasher.Hash("test-secret")
		require.NoError(t, err)

		parts := strings.Split(hash, "$")
		parts[5] = parts[5][:len(parts[5])-1]

		err = hasher.Compare(strings.Join(parts, "$"), "test-secret")
		assert.ErrorContains(t, err, "hash must be 32 bytes")
	})
}

func TestClientAuthService_GenerateCredentials(t *testing.T) {
	t.Run("client_id equals the agent UUID", func(t *testing.T) {
		svc := &ClientAuthService{hasher: &Argon2Hasher{}}
		agentID := testAgentID()
		cred, secret, err := svc.GenerateCredentials(agentID)
		require.NoError(t, err)
		require.NotNil(t, cred)
		require.NotEmpty(t, secret)

		assert.Equal(t, agentID, cred.AgentID)

		// Verify secret hash is not the plaintext
		assert.NotEqual(t, secret, cred.SecretHash)
		assert.True(t, strings.HasPrefix(cred.SecretHash, "$argon2id$"))
	})

	t.Run("generates unique secrets each call, same client_id for same agent", func(t *testing.T) {
		svc := &ClientAuthService{hasher: &Argon2Hasher{}}
		agentID := testAgentID()
		cred1, secret1, err := svc.GenerateCredentials(agentID)
		require.NoError(t, err)
		cred2, secret2, err := svc.GenerateCredentials(agentID)
		require.NoError(t, err)

		// Client ID is deterministic (= agentID)
		assert.Equal(t, cred1.AgentID, cred2.AgentID)
		// Secret is random each time (rotation scenario)
		assert.NotEqual(t, secret1, secret2)
	})
}

func TestClientAuthService_Authenticate(t *testing.T) {
	t.Run("valid credentials succeed", func(t *testing.T) {
		credRepo := memory.NewClientCredentialStore()
		agentRepo := memory.NewAgentRepository()

		// Create test agent
		agent := testAgent()
		err := agentRepo.Create(context.Background(), agent)
		require.NoError(t, err)

		// Create test credential
		svc := NewClientAuthService(credRepo, &testClientResolver{agentRepo: agentRepo}, testSlogger())
		cred, plaintext, err := svc.GenerateCredentials(agent.ID)
		require.NoError(t, err)
		err = credRepo.Create(context.Background(), cred)
		require.NoError(t, err)

		// Authenticate should succeed using agent UUID
		result, err := svc.Authenticate(context.Background(), agent.ID, plaintext)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, agent.ID, result.Agent.ID)
	})

	t.Run("unknown agent_id fails", func(t *testing.T) {
		credRepo := memory.NewClientCredentialStore()
		agentRepo := memory.NewAgentRepository()
		svc := NewClientAuthService(credRepo, &testClientResolver{agentRepo: agentRepo}, testSlogger())

		_, err := svc.Authenticate(context.Background(), id.NewAgentID(), "secret")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
	})

	t.Run("wrong secret fails", func(t *testing.T) {
		credRepo := memory.NewClientCredentialStore()
		agentRepo := memory.NewAgentRepository()

		// Create test agent + credential
		agent := testAgent()
		err := agentRepo.Create(context.Background(), agent)
		require.NoError(t, err)

		svc := NewClientAuthService(credRepo, &testClientResolver{agentRepo: agentRepo}, testSlogger())
		cred, _, err := svc.GenerateCredentials(agent.ID)
		require.NoError(t, err)
		err = credRepo.Create(context.Background(), cred)
		require.NoError(t, err)

		// Authenticate with wrong secret should fail
		_, err = svc.Authenticate(context.Background(), agent.ID, "wrong-secret")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
	})

	t.Run("credential repo connection error logs at error level", func(t *testing.T) {
		agentID := id.NewAgentID()
		credRepo := &mockCredentialRepo{
			getByAgentIDFunc: func(_ context.Context, _ id.AgentID) (*storage.ClientCredential, error) {
				return nil, storage.NewStorageError("GetByAgentID", storage.ErrorKindConnection, nil, "connection refused")
			},
		}
		logger, buf := bufLogger()
		svc := NewClientAuthService(credRepo, &testClientResolver{agentRepo: memory.NewAgentRepository()}, logger)

		_, err := svc.Authenticate(context.Background(), agentID, "secret")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
		assert.Contains(t, buf.String(), "storage failure during client authentication")
	})

	t.Run("credential repo not-found does not log an error", func(t *testing.T) {
		agentID := id.NewAgentID()
		credRepo := &mockCredentialRepo{
			getByAgentIDFunc: func(_ context.Context, _ id.AgentID) (*storage.ClientCredential, error) {
				return nil, storage.NewStorageError("GetByAgentID", storage.ErrorKindNotFound, nil, "not found")
			},
		}
		logger, buf := bufLogger()
		svc := NewClientAuthService(credRepo, &testClientResolver{agentRepo: memory.NewAgentRepository()}, logger)

		_, err := svc.Authenticate(context.Background(), agentID, "secret")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
		assert.Empty(t, buf.String(), "not-found must not produce an error log")
	})

	t.Run("client resolver timeout error logs at error level", func(t *testing.T) {
		credRepo := memory.NewClientCredentialStore()
		agentID := id.NewAgentID()
		cred := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agentID,
			SecretHash: "irrelevant",
		}
		require.NoError(t, credRepo.Create(context.Background(), cred))

		resolver := &mockClientResolver{
			resolveFunc: func(_ context.Context, _ id.ClientID) (*ports.ClientResolution, error) {
				return nil, storage.NewStorageError("Get", storage.ErrorKindTimeout, nil, "query timeout")
			},
		}
		logger, buf := bufLogger()
		svc := NewClientAuthService(credRepo, resolver, logger)

		_, err := svc.Authenticate(context.Background(), agentID, "irrelevant")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
		assert.Contains(t, buf.String(), "storage failure during client authentication")
	})

	t.Run("client resolver not-found does not log an error", func(t *testing.T) {
		credRepo := memory.NewClientCredentialStore()
		agentID := id.NewAgentID()
		cred := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agentID,
			SecretHash: "irrelevant",
		}
		require.NoError(t, credRepo.Create(context.Background(), cred))

		resolver := &mockClientResolver{
			resolveFunc: func(_ context.Context, _ id.ClientID) (*ports.ClientResolution, error) {
				return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "not found"}
			},
		}
		logger, buf := bufLogger()
		svc := NewClientAuthService(credRepo, resolver, logger)

		_, err := svc.Authenticate(context.Background(), agentID, "irrelevant")
		assert.ErrorIs(t, err, fosite.ErrInvalidClient)
		assert.Empty(t, buf.String(), "not-found on agent lookup must not produce an error log")
	})
}
