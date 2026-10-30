package sessiontoken

import (
	"errors"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Service handles creation and validation of JWE-sealed authorization session tokens.
type Service struct {
	jweTokenService *jwe.TokenService
}

var _ ports.SessionTokenValidator = (*Service)(nil)

// NewService creates a SessionTokenService backed by the given JWE token service.
func NewService(jweTokenService *jwe.TokenService) *Service {
	if jweTokenService == nil {
		panic("sessiontoken.NewService: jweTokenService must not be nil")
	}
	return &Service{jweTokenService: jweTokenService}
}

// Create seals claims into a compact JWE string.
func (s *Service) Create(claims *AuthorizationSessionClaims) (string, error) {
	if claims == nil {
		return "", fmt.Errorf("claims must not be nil")
	}
	if claims.AgentID.IsZero() || claims.Principal.IsZero() || claims.OriginalURL == "" {
		return "", fmt.Errorf("claims must have non-zero AgentID, Principal, and OriginalURL")
	}
	return s.jweTokenService.Encrypt(claims)
}

// ValidateAuthorizationSessionToken decrypts a JWE session token and validates
// expiry, agent binding, and principal binding. Returns a port-local DTO.
func (s *Service) ValidateAuthorizationSessionToken(token string, agentID id.AgentID, principal id.Principal) (*ports.AuthorizationSession, error) {
	var claims AuthorizationSessionClaims
	if err := s.jweTokenService.DecryptAndValidate(token, &claims); err != nil {
		if errors.Is(err, jwe.ErrExpired) {
			return nil, ports.ErrSessionExpired
		}
		return nil, ports.ErrSessionInvalidToken
	}
	if claims.AgentID != agentID {
		return nil, ports.ErrSessionAgentMismatch
	}
	if claims.Principal != principal {
		return nil, ports.ErrSessionPrincipalMismatch
	}
	return &ports.AuthorizationSession{
		AgentID:              claims.AgentID,
		Principal:            claims.Principal,
		OriginalURL:          claims.OriginalURL,
		CIMDMetadata:         claims.CIMDMetadata,
		RequestedServiceIDs:  claims.RequestedServiceIDs,
		ServiceScopeBindings: claims.ServiceScopeBindings,
		ExpiresAt:            claims.ExpiresAt,
	}, nil
}
