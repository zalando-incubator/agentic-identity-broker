package sessiontoken

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	key, err := jwk.Import[jwk.Key]([]byte("test-32-byte-key-must-be-exact-x"))
	require.NoError(t, err)
	return NewService(jwe.New(key))
}

func TestNewAuthorizationSessionClaims_NilCIMDMetadata(t *testing.T) {
	agentID := id.NewAgentID()
	principal := id.NewPrincipal("user@example.com")
	originalURL := "https://broker.example.com/oauth2/authorize?client_id=test"

	before := time.Now()
	claims, err := NewAuthorizationSessionClaims(agentID, principal, originalURL, nil)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, claims)

	assert.Equal(t, agentID, claims.AgentID)
	assert.Equal(t, principal, claims.Principal)
	assert.Equal(t, originalURL, claims.OriginalURL)
	assert.Nil(t, claims.CIMDMetadata)

	assert.True(t, !claims.IssuedAt.Before(before) && !claims.IssuedAt.After(after))
	assert.InDelta(t, (10 * time.Minute).Seconds(), claims.ExpiresAt.Sub(claims.IssuedAt).Seconds(), 1)
	assert.False(t, claims.IsExpired())
}

func TestNewAuthorizationSessionClaims_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		agentID     id.AgentID
		principal   id.Principal
		originalURL string
		wantErr     string
	}{
		{
			name:        "zero agent ID",
			agentID:     id.AgentID{},
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: "https://example.com",
			wantErr:     "agentID must not be zero",
		},
		{
			name:        "zero principal",
			agentID:     id.NewAgentID(),
			principal:   id.Principal(""),
			originalURL: "https://example.com",
			wantErr:     "principal must not be zero",
		},
		{
			name:        "empty originalURL",
			agentID:     id.NewAgentID(),
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: "",
			wantErr:     "originalURL must not be empty",
		},
		{
			name:        "ftp scheme rejected",
			agentID:     id.NewAgentID(),
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: "ftp://bad.example.com",
			wantErr:     "originalURL must be a valid relative or http(s) URL",
		},
		{
			name:        "javascript scheme rejected",
			agentID:     id.NewAgentID(),
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: "javascript:void(0)",
			wantErr:     "originalURL must be a valid relative or http(s) URL",
		},
		{
			name:        "protocol-relative URL rejected",
			agentID:     id.NewAgentID(),
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: "//evil.example.com/path",
			wantErr:     "originalURL must be a valid relative or http(s) URL",
		},
		{
			name:        "backslash protocol-relative URL rejected",
			agentID:     id.NewAgentID(),
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: `\\evil.example.com`,
			wantErr:     "originalURL must be a valid relative or http(s) URL",
		},
		{
			name:        "mixed-slash protocol-relative URL rejected",
			agentID:     id.NewAgentID(),
			principal:   id.NewPrincipal("user@example.com"),
			originalURL: `/\evil.example.com`,
			wantErr:     "originalURL must be a valid relative or http(s) URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := NewAuthorizationSessionClaims(tt.agentID, tt.principal, tt.originalURL, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, claims)
		})
	}
}

func TestNewAuthorizationSessionClaims_RelativeURLAllowed(t *testing.T) {
	claims, err := NewAuthorizationSessionClaims(
		id.NewAgentID(),
		id.NewPrincipal("user@example.com"),
		"/oauth2/authorize?client_id=test",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "/oauth2/authorize?client_id=test", claims.OriginalURL)
}

func TestNewAuthorizationSessionClaims_BackslashNormalizationPreservesQueryParams(t *testing.T) {
	// Backslashes in the path/scheme are normalized to block \\evil.com browser redirect
	// tricks, but backslashes in query parameters (e.g. state tokens) must be preserved
	// exactly to avoid breaking client-side CSRF validation.
	claims, err := NewAuthorizationSessionClaims(
		id.NewAgentID(),
		id.NewPrincipal("user@example.com"),
		"/oauth2/authorize?state=foo%5Cbar&client_id=test",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "/oauth2/authorize?state=foo%5Cbar&client_id=test", claims.OriginalURL,
		"query parameters must not be mutated by backslash normalization")
}

func TestService_ValidateAuthorizationSessionToken_Success(t *testing.T) {
	svc := newTestService(t)
	agentID := id.NewAgentID()
	p := id.NewPrincipal("user@example.com")

	claims, _ := NewAuthorizationSessionClaims(agentID, p, "https://example.com/authorize?client_id=test", nil)
	token, err := svc.Create(claims)
	require.NoError(t, err)

	validated, err := svc.ValidateAuthorizationSessionToken(token, agentID, p)
	require.NoError(t, err)
	assert.Equal(t, agentID, validated.AgentID)
	assert.Equal(t, p, validated.Principal)
	assert.Equal(t, "https://example.com/authorize?client_id=test", validated.OriginalURL)
}

func TestService_ValidateAuthorizationSessionToken_Expired(t *testing.T) {
	svc := newTestService(t)
	agentID := id.NewAgentID()
	p := id.NewPrincipal("user@example.com")

	past := time.Now().Add(-time.Hour)
	claims := &AuthorizationSessionClaims{
		AgentID:     agentID,
		Principal:   p,
		OriginalURL: "https://example.com/authorize",
		IssuedAt:    past,
		ExpiresAt:   past,
	}
	token, err := svc.jweTokenService.Encrypt(claims)
	require.NoError(t, err)

	_, err = svc.ValidateAuthorizationSessionToken(token, agentID, p)
	assert.ErrorIs(t, err, ports.ErrSessionExpired)
}

func TestService_ValidateAuthorizationSessionToken_AgentMismatch(t *testing.T) {
	svc := newTestService(t)
	agentID := id.NewAgentID()
	otherAgent := id.NewAgentID()
	p := id.NewPrincipal("user@example.com")

	claims, _ := NewAuthorizationSessionClaims(agentID, p, "https://example.com/authorize?client_id=test", nil)
	token, err := svc.Create(claims)
	require.NoError(t, err)

	_, err = svc.ValidateAuthorizationSessionToken(token, otherAgent, p)
	assert.ErrorIs(t, err, ports.ErrSessionAgentMismatch)
}

func TestService_ValidateAuthorizationSessionToken_PrincipalMismatch(t *testing.T) {
	svc := newTestService(t)
	agentID := id.NewAgentID()
	p := id.NewPrincipal("user@example.com")
	otherP := id.NewPrincipal("other@example.com")

	claims, _ := NewAuthorizationSessionClaims(agentID, p, "https://example.com/authorize?client_id=test", nil)
	token, err := svc.Create(claims)
	require.NoError(t, err)

	_, err = svc.ValidateAuthorizationSessionToken(token, agentID, otherP)
	assert.ErrorIs(t, err, ports.ErrSessionPrincipalMismatch)
}

func TestService_ValidateAuthorizationSessionToken_InvalidToken(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.ValidateAuthorizationSessionToken("garbage-token", id.NewAgentID(), id.NewPrincipal("x"))
	assert.ErrorIs(t, err, ports.ErrSessionInvalidToken)
}

// Continuity claims from specs/055-incremental-service-authorization (FR-007, FR-011, FR-019, SR-002).
func TestRequestedServiceClaimsSurviveSealing(t *testing.T) {
	svc := newTestService(t)
	agentID := id.NewAgentID()
	principal := id.NewPrincipal("alice@example.com")
	serviceID := id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	scope := "https://broker.example.com/sessions/github"

	claims, err := NewAuthorizationSessionClaims(agentID, principal, "/oauth2/authorize?client_id=agent&scope=openid", nil)
	require.NoError(t, err)
	encoded, err := json.Marshal(claims)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "requested_service_ids", "empty fields are omitted")
	assert.NotContains(t, string(encoded), "service_scope_bindings")

	claims.RequestedServiceIDs = []id.ServiceID{serviceID}
	claims.ServiceScopeBindings = map[string]id.ServiceID{scope: serviceID}
	token, err := svc.Create(claims)
	require.NoError(t, err)

	session, err := svc.ValidateAuthorizationSessionToken(token, agentID, principal)
	require.NoError(t, err)
	assert.Equal(t, []id.ServiceID{serviceID}, session.RequestedServiceIDs)
	assert.Equal(t, map[string]id.ServiceID{scope: serviceID}, session.ServiceScopeBindings)
	assert.WithinDuration(t, claims.ExpiresAt, session.ExpiresAt, time.Second, "the original expiry is exposed, never renewed")

	_, err = svc.ValidateAuthorizationSessionToken(token, id.NewAgentID(), principal)
	assert.ErrorIs(t, err, ports.ErrSessionAgentMismatch)
	_, err = svc.ValidateAuthorizationSessionToken(token, agentID, id.NewPrincipal("mallory@example.com"))
	assert.ErrorIs(t, err, ports.ErrSessionPrincipalMismatch)
	_, err = svc.ValidateAuthorizationSessionToken(token[:len(token)-2]+"xx", agentID, principal)
	assert.ErrorIs(t, err, ports.ErrSessionInvalidToken)
}
