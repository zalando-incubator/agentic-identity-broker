package oauth2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCIMDFetcher is a test double for ports.CIMDFetcher.
type mockCIMDFetcher struct {
	result *ports.CIMDFetchResult
	err    error
}

func (m *mockCIMDFetcher) Fetch(_ context.Context, _ string) (*ports.CIMDFetchResult, error) {
	return m.result, m.err
}

// cimdServiceForTest creates a cimd.Service backed by the given fetch result.
func cimdServiceForTest(t *testing.T, fetchResult *ports.CIMDFetchResult, fetchErr error) *cimd.Service {
	t.Helper()
	fetcher := &mockCIMDFetcher{result: fetchResult, err: fetchErr}
	cache, err := cimd.NewCIMDCache(60*time.Second, time.Hour, 1000)
	require.NoError(t, err)
	return cimd.NewService(fetcher, cache, nil, slog.Default())
}

// --- CIMD enabled tests ---

func TestAgentClientResolver_OpaqueUUID_Success(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000001")
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID(agentID.String())),
		DisplayName: "Test Agent",
	}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	svc := cimdServiceForTest(t, nil, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	resolution, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))
	require.NoError(t, err)
	assert.Equal(t, agent.ID, resolution.Agent.ID)
	assert.Nil(t, resolution.CIMDMetadata)
}

func TestAgentClientResolver_OpaqueUUID_NotFound(t *testing.T) {
	repo := NewMockAgentRepository()
	svc := cimdServiceForTest(t, nil, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	_, err := resolver.ResolveClient(context.Background(), id.ClientID("00000000-0000-0000-0000-000000000099"))
	require.Error(t, err)

	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
}

func TestAgentClientResolver_InvalidURL_Rejected(t *testing.T) {
	repo := NewMockAgentRepository()
	svc := cimdServiceForTest(t, nil, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	_, err := resolver.ResolveClient(context.Background(), "http://agent.example.com/client")
	require.Error(t, err)

	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
}

func TestAgentClientResolver_URLNotRegistered(t *testing.T) {
	repo := NewMockAgentRepository()
	svc := cimdServiceForTest(t, nil, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	_, err := resolver.ResolveClient(context.Background(), "https://agent.example.com/client")
	require.Error(t, err)

	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
	assert.Equal(t, "Client not registered", clientErr.Desc)
}

func TestAgentClientResolver_AmbiguousCIMDPatternRejected(t *testing.T) {
	repo := NewMockAgentRepository()
	repo.getByClientURIErr = storage.NewStorageError(
		"GetAgentByClientURI",
		storage.ErrorKindConflict,
		nil,
		"CIMD client URI matches multiple agents",
	)
	resolver := NewAgentClientResolverWithCIMD(repo, cimdServiceForTest(t, nil, nil), slog.Default())

	_, err := resolver.ResolveClient(context.Background(), "https://agent.example.com/client")
	require.Error(t, err)
	var clientErr *ports.ClientIDError
	require.ErrorAs(t, err, &clientErr)
	assert.Equal(t, "invalid_client", clientErr.Code)
	assert.Equal(t, "Client not registered", clientErr.Desc)
}

func TestAgentClientResolver_CIMDFetchFails(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000001")
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("https://agent.example.com/client")),
		DisplayName: "Test Agent",
	}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI("https://agent.example.com/client", agent)
	svc := cimdServiceForTest(t, nil, &ports.CIMDUnavailableError{Cause: fmt.Errorf("connection refused")})
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	_, err := resolver.ResolveClient(context.Background(), "https://agent.example.com/client")
	require.Error(t, err)

	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "server_error", clientErr.Code)
}

func TestAgentClientResolver_CIMDUnavailableDoesNotLogClientURI(t *testing.T) {
	const clientURI = "https://agent.example.com/client?marker=untrusted"
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Test Agent"}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI(clientURI, agent)
	var logs bytes.Buffer
	resolver := NewAgentClientResolverWithCIMD(
		repo,
		cimdServiceForTest(t, nil, &ports.CIMDUnavailableError{Cause: errors.New("connection refused")}),
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	_, err := resolver.ResolveClient(context.Background(), id.ClientID(clientURI))
	require.Error(t, err)
	assert.NotContains(t, logs.String(), clientURI)
}

func TestAgentClientResolver_CIMDAvailabilityFailuresReturnServerError(t *testing.T) {
	clientURL := "https://agent.example.com/client"
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Test Agent"}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI(clientURL, agent)
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "timeout", err: &ports.CIMDUnavailableError{Cause: context.DeadlineExceeded}},
		{name: "connect failure", err: &ports.CIMDUnavailableError{Cause: errors.New("connection refused")}},
		{name: "server failure", err: &ports.CIMDUnavailableError{Cause: errors.New("upstream returned 503")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := NewAgentClientResolverWithCIMD(repo, cimdServiceForTest(t, nil, test.err), slog.Default())
			_, err := resolver.ResolveClient(context.Background(), id.ClientID(clientURL))
			var clientErr *ports.ClientIDError
			require.ErrorAs(t, err, &clientErr)
			assert.Equal(t, "server_error", clientErr.Code)
		})
	}
}

func TestAgentClientResolver_CIMDValidationFailuresRemainInvalidClient(t *testing.T) {
	clientURL := "https://agent.example.com/client"
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Test Agent"}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI(clientURL, agent)
	for _, test := range []struct {
		name   string
		result *ports.CIMDFetchResult
		err    error
	}{
		{name: "SSRF blocked", err: &ports.SSRFBlockedError{IP: "127.0.0.1"}},
		{name: "malformed document", result: &ports.CIMDFetchResult{Body: []byte("{")}},
		{name: "policy violation", result: &ports.CIMDFetchResult{Body: []byte(`{"client_id":"https://other.example/client","redirect_uris":["https://agent.example.com/cb"]}`)}},
		{name: "client error response", err: errors.New("CIMD endpoint returned status 404")},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := NewAgentClientResolverWithCIMD(repo, cimdServiceForTest(t, test.result, test.err), slog.Default())
			_, err := resolver.ResolveClient(context.Background(), id.ClientID(clientURL))
			var clientErr *ports.ClientIDError
			require.ErrorAs(t, err, &clientErr)
			assert.Equal(t, "invalid_client", clientErr.Code)
			assert.Equal(t, "CIMD document validation failed", clientErr.Desc)
		})
	}
}

func TestAgentClientResolver_URLFormat_Success(t *testing.T) {
	const clientURL = "https://agent.example.com/client"
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000001")
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID(clientURL)),
		DisplayName: "Test Agent",
	}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI(clientURL, agent)
	body := fmt.Sprintf(
		`{"client_id":%q,"client_name":"Test Agent","redirect_uris":["https://agent.example.com/cb"]}`,
		clientURL,
	)
	fetchResult := &ports.CIMDFetchResult{Body: []byte(body), CacheControl: "max-age=300"}
	svc := cimdServiceForTest(t, fetchResult, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	resolution, err := resolver.ResolveClient(context.Background(), clientURL)
	require.NoError(t, err)
	require.NotNil(t, resolution)
	assert.Equal(t, agent.ID, resolution.Agent.ID)
	require.NotNil(t, resolution.CIMDMetadata)
	assert.Equal(t, clientURL, resolution.CIMDMetadata.ClientID)
	assert.Equal(t, []string{"https://agent.example.com/cb"}, resolution.CIMDMetadata.RedirectURIs)
}

func TestAgentClientResolver_PrivateKeyJWTMetadataIsMapped(t *testing.T) {
	const clientURL = "https://agent.example.com/client"
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000001")
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID(clientURL)),
		DisplayName: "Test Agent",
	}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI(clientURL, agent)
	body := fmt.Sprintf(`{"client_id":%q,"redirect_uris":["https://agent.example.com/cb"],"token_endpoint_auth_method":"private_key_jwt","jwks_uri":"https://agent.example.com/jwks.json"}`, clientURL)
	svc := cimdServiceForTest(t, &ports.CIMDFetchResult{Body: []byte(body), CacheControl: "max-age=300"}, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	resolution, err := resolver.ResolveClient(context.Background(), id.ClientID(clientURL))
	require.NoError(t, err)
	require.NotNil(t, resolution.CIMDMetadata)
	assert.Equal(t, "private_key_jwt", resolution.CIMDMetadata.AuthMethod)
	assert.Equal(t, "RS256", resolution.CIMDMetadata.SigningAlg)
	assert.Equal(t, "https://agent.example.com/jwks.json", resolution.CIMDMetadata.JWKSURI)
}

// TestAgentClientResolver_ResolvesViaCIMDURINotClientID proves the resolver uses
// GetByClientURI (pre-registered URI lookup) rather than GetByClientID. When the
// agent's ClientID is a UUID and the CIMD URL is only in ClientURIs, the resolver
// must still find the agent.
func TestAgentClientResolver_ResolvesViaCIMDURINotClientID(t *testing.T) {
	const cimdURI = "https://agent.example.com/cimd-endpoint"
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000099")
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID(agentID.String())), // UUID-form ClientID, not the CIMD URL
		DisplayName: "URI-Only Agent",
	}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	repo.RegisterURI(cimdURI, agent) // registered as ClientURI, not ClientID

	body := fmt.Sprintf(`{"client_id":%q,"client_name":"URI-Only Agent","redirect_uris":["https://agent.example.com/cb"]}`, cimdURI)
	fetchResult := &ports.CIMDFetchResult{Body: []byte(body), CacheControl: "max-age=300"}
	svc := cimdServiceForTest(t, fetchResult, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	resolution, err := resolver.ResolveClient(context.Background(), cimdURI)
	require.NoError(t, err, "should resolve via ClientURI even when ClientID is a different value")
	assert.Equal(t, agentID, resolution.Agent.ID)
}

// --- CIMD disabled tests ---

func TestAgentClientResolver_Disabled_URLFormat_Rejected(t *testing.T) {
	repo := NewMockAgentRepository()
	resolver := NewAgentClientResolver(repo, nil)

	for _, clientID := range []string{
		"https://agent.example.com/client",
		"http://agent.example.com/client",
	} {
		_, err := resolver.ResolveClient(context.Background(), id.ClientID(clientID))
		require.Error(t, err, "expected error for %s", clientID)

		var clientErr *ports.ClientIDError
		require.True(t, errors.As(err, &clientErr))
		assert.Equal(t, "invalid_client", clientErr.Code)
	}
}

func TestAgentClientResolver_Disabled_NonUUID_Rejected(t *testing.T) {
	repo := NewMockAgentRepository()
	resolver := NewAgentClientResolver(repo, nil)

	_, err := resolver.ResolveClient(context.Background(), "not-a-uuid")
	require.Error(t, err)

	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
}

func TestAgentClientResolver_Disabled_OpaqueUUID_Success(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000001")
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID(agentID.String())),
		DisplayName: "Test Agent",
	}
	repo := NewMockAgentRepository()
	require.NoError(t, repo.Create(context.Background(), agent))
	resolver := NewAgentClientResolver(repo, nil)

	resolution, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))
	require.NoError(t, err)
	require.NotNil(t, resolution)
	assert.Equal(t, agent.ID, resolution.Agent.ID)
	assert.Nil(t, resolution.CIMDMetadata)
}

func TestAgentClientResolver_Disabled_AgentNotFound(t *testing.T) {
	repo := NewMockAgentRepository()
	resolver := NewAgentClientResolver(repo, nil)

	_, err := resolver.ResolveClient(context.Background(), id.ClientID("00000000-0000-0000-0000-000000000099"))
	require.Error(t, err)

	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
}

func TestOpaqueClientResolver_Success(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000001")
	repo := NewMockAgentRepository()
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID(agentID.String())),
		DisplayName: "Test Agent",
	}
	require.NoError(t, repo.Create(context.Background(), agent))

	resolver := NewAgentClientResolver(repo, nil)
	resolution, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))

	require.NoError(t, err)
	require.NotNil(t, resolution)
	assert.Equal(t, agent.ID, resolution.Agent.ID)
	assert.Nil(t, resolution.CIMDMetadata)
}

// T043: Universal client resolver format detection — opaque (UUID) path.
// UUID format → lookup by Agent.ID.
func TestOpaqueClientResolver_UUID_ResolvesById(t *testing.T) {
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000002")
	repo := NewMockAgentRepository()
	agent := &storage.Agent{
		ID:          agentID,
		DisplayName: "Local Agent",
		Description: "Plain local agent resolved by UUID",
	}
	require.NoError(t, repo.Create(context.Background(), agent))

	resolver := NewAgentClientResolver(repo, nil)
	resolution, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))

	require.NoError(t, err)
	require.NotNil(t, resolution)
	assert.Equal(t, agentID, resolution.Agent.ID)
	assert.Equal(t, storage.LocalClient, resolution.Agent.ClientType())
}

// T044: CIMD agent UUID rejection (FR-005) via OpaqueClientResolver.
// OpaqueClientResolver rejects URL-format client IDs before any lookup.
func TestOpaqueClientResolver_CIMDAgent_URLRejectedBefore_Lookup(t *testing.T) {
	repo := NewMockAgentRepository()
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000003")
	// Store a CIMD agent (with ClientURIs, no ClientID)
	agent := &storage.Agent{
		ID:          agentID,
		ClientURIs:  []string{"https://agent.example.com/.well-known/openid-configuration"},
		DisplayName: "CIMD Agent",
		Description: "Agent identified by URL",
	}
	require.NoError(t, repo.Create(context.Background(), agent))

	// Attempting to access the CIMD agent via its URL-form client_id must be rejected by
	// OpaqueClientResolver before any lookup when CIMD is disabled (CIMD gate).
	resolver := NewAgentClientResolver(repo, nil)
	_, err := resolver.ResolveClient(context.Background(), id.ClientID("https://agent.example.com/.well-known/openid-configuration"))

	require.Error(t, err)
	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
	assert.Contains(t, clientErr.Desc, "CIMD")
}

// TestOpaqueClientResolver_CIMDAgent_UUIDRejectedWithCIMDEnabled verifies that when CIMD
// is enabled, addressing a CIMDClient agent by UUID returns a message that tells the client
// to use URL-form client_id (not falsely claiming CIMD is disabled).
func TestOpaqueClientResolver_CIMDAgent_UUIDRejectedWithCIMDEnabled(t *testing.T) {
	repo := NewMockAgentRepository()
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000020")
	agent := &storage.Agent{
		ID:          agentID,
		ClientURIs:  []string{"https://agent.example.com/.well-known/openid-configuration"},
		DisplayName: "CIMD Agent",
	}
	require.NoError(t, repo.Create(context.Background(), agent))

	// CIMD is enabled — use NewAgentClientResolverWithCIMD.
	svc := cimdServiceForTest(t, nil, nil)
	resolver := NewAgentClientResolverWithCIMD(repo, svc, slog.Default())

	_, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))

	require.Error(t, err)
	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
	assert.Equal(t, "CIMD client must use URL-form client_id", clientErr.Desc,
		"error should not say CIMD is disabled when CIMD is enabled")
}

// TestOpaqueClientResolver_CIMDAgent_UUIDRejected verifies that a CIMD agent cannot be
// addressed by its bare entity UUID when CIMD is disabled.
func TestOpaqueClientResolver_CIMDAgent_UUIDRejected(t *testing.T) {
	repo := NewMockAgentRepository()
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000010")
	agent := &storage.Agent{
		ID:          agentID,
		ClientURIs:  []string{"https://agent.example.com/.well-known/openid-configuration"},
		DisplayName: "CIMD Agent",
		Description: "Agent identified by URL",
	}
	require.NoError(t, repo.Create(context.Background(), agent))

	resolver := NewAgentClientResolver(repo, nil)
	_, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))

	require.Error(t, err)
	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
}

// TestOpaqueClientResolver_AmbiguousAgent_UUIDRejected verifies that an agent with both
// ClientID and ClientURIs (storage invariant violation) is always rejected.
func TestOpaqueClientResolver_AmbiguousAgent_UUIDRejected(t *testing.T) {
	repo := NewMockAgentRepository()
	agentID := id.MustParseAgentID("00000000-0000-0000-0000-000000000011")
	cid := id.ClientID("some-upstream-client")
	// Bypass domain Validate() to simulate a storage invariant violation.
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    &cid,
		ClientURIs:  []string{"https://agent.example.com/.well-known/openid-configuration"},
		DisplayName: "Ambiguous Agent",
		Description: "Has both ClientID and ClientURIs",
	}
	require.NoError(t, repo.Create(context.Background(), agent))

	resolver := NewAgentClientResolver(repo, nil)
	_, err := resolver.ResolveClient(context.Background(), id.ClientID(agentID.String()))

	require.Error(t, err)
	var clientErr *ports.ClientIDError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, "invalid_client", clientErr.Code)
}

// T045: Mode enforcement — proxy mode rejects local/CIMD agents.
func TestModeStrategy_ProxyMode_RejectsLocalAndCIMD(t *testing.T) {
	s := NewProxyModeStrategy()
	assert.False(t, s.AcceptsClientType(storage.LocalClient), "proxy mode must reject LocalClient")
	assert.False(t, s.AcceptsClientType(storage.CIMDClient), "proxy mode must reject CIMDClient")
}

// T045: Mode enforcement — local mode rejects proxy agents.
func TestModeStrategy_LocalMode_RejectsProxy(t *testing.T) {
	s := NewLocalModeStrategy()
	assert.False(t, s.AcceptsClientType(storage.ProxyClient), "local mode must reject ProxyClient")
}
