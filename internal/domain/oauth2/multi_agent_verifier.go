package oauth2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// AgentIDMismatchError is returned by VerifyAgentIDClaim when the claim value in the
// upstream token does not match the expected agent ID. It carries structured fields so
// callers can log or handle the mismatch without parsing the error string.
type AgentIDMismatchError struct {
	Expected  string
	Received  string
	ClaimName string
}

func (e *AgentIDMismatchError) Error() string {
	return fmt.Sprintf("agent ID claim mismatch: expected %q, got %q for claim %q", e.Expected, e.Received, e.ClaimName)
}

// MultiAgentTokenVerifier verifies that an upstream token response contains the expected
// agent ID claim, as required by feature 20260320-120000 (multi-agent client sharing).
//
// When multiple agents share a single upstream OAuth2 client_id, the upstream server
// is expected to embed the requesting agent's UUID into the issued token as a custom claim.
// This verifier checks that the claim is present and matches the agent that initiated the
// authorization request (fail-closed per SR-001).
//
// The verifier parses the JSON token response body, extracts the access_token JWT,
// verifies its signature using the upstream JWKS (defense-in-depth), and reads the
// configured custom claim.
type MultiAgentTokenVerifier struct {
	agentIDClaimName string
	jwksPort         ports.JWKSPort
}

// NewMultiAgentTokenVerifier creates a new MultiAgentTokenVerifier.
// Returns an error if agentIDClaimName is empty or jwksPort is nil.
func NewMultiAgentTokenVerifier(agentIDClaimName string, jwksPort ports.JWKSPort) (*MultiAgentTokenVerifier, error) {
	if agentIDClaimName == "" {
		return nil, fmt.Errorf("agentIDClaimName cannot be empty")
	}
	if jwksPort == nil {
		return nil, fmt.Errorf("jwksPort cannot be nil")
	}
	return &MultiAgentTokenVerifier{
		agentIDClaimName: agentIDClaimName,
		jwksPort:         jwksPort,
	}, nil
}

// VerifyAgentIDClaim verifies that the upstream token response body contains an access_token
// JWT with the configured agent ID claim matching expectedAgentID.
//
// Steps:
//  1. Parse JSON response body to extract "access_token"
//  2. Fetch JWKS and verify JWT signature (defense-in-depth: broker is the relying party)
//  3. Look up the configured claim name directly in the parsed token
//  4. Compare the claim value against expectedAgentID.String()
//
// Returns nil on success.
// Returns error with "agent ID claim absent" prefix if the claim is missing or cannot be extracted.
// Returns error with "agent ID claim mismatch" prefix if the claim value does not match.
func (v *MultiAgentTokenVerifier) VerifyAgentIDClaim(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error {
	// Step 1: Extract access_token from JSON response body
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(responseBody, &tokenResponse); err != nil {
		return fmt.Errorf("agent ID claim absent from upstream token: failed to parse response body: %w", err)
	}
	if tokenResponse.AccessToken == "" {
		return fmt.Errorf("agent ID claim absent from upstream token: access_token not found in response")
	}

	// Step 2: Fetch JWKS and verify JWT signature.
	// The broker is the relying party — a manipulated upstream response body could inject a
	// forged agent ID claim if we skip signature verification (defense-in-depth per SR-001).
	keySet, err := v.jwksPort.GetKeySet(ctx)
	if err != nil {
		return fmt.Errorf("agent ID claim absent from upstream token: failed to fetch JWKS for signature verification: %w", err)
	}

	tok, err := jwt.Parse([]byte(tokenResponse.AccessToken),
		jwt.WithVerify(true),
		jwt.WithKeySet(keySet),
		jwt.WithValidate(false), // only verify signature; upstream handles full token validation
	)
	if err != nil {
		return fmt.Errorf("agent ID claim absent from upstream token: failed to verify access_token signature: %w", err)
	}

	// Step 3: Look up the configured claim directly.
	claimValue, err := jwt.Get[any](tok, v.agentIDClaimName)
	if err != nil {
		return fmt.Errorf("agent ID claim absent from upstream token: claim %q not found in JWT", v.agentIDClaimName)
	}

	claimStr, ok := claimValue.(string)
	if !ok {
		return fmt.Errorf("agent ID claim absent from upstream token: claim %q has type %T, expected string", v.agentIDClaimName, claimValue)
	}

	// Step 4: Compare against expected agent ID
	if claimStr != expectedAgentID.String() {
		return &AgentIDMismatchError{
			Expected:  expectedAgentID.String(),
			Received:  claimStr,
			ClaimName: v.agentIDClaimName,
		}
	}

	return nil
}
